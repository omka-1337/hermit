package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hermit/internal/deploy"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/adrg/xdg"

	"hermit/internal/library"
	"hermit/internal/nexus"
	"hermit/internal/platform"
)

const (
	// DownloadsEvent carries the list of downloads whenever it changes.
	DownloadsEvent = "downloads:changed"
	// DownloadFinishedEvent says a download is ready to be installed; the
	// main window takes it with TakeFinished.
	DownloadFinishedEvent = "downloads:finished"
	// DownloadErrorEvent says why an nxm:// link that came from outside,
	// with nobody to return an error to, was not downloaded.
	DownloadErrorEvent = "downloads:error"
)

type DownloadStatus string

const (
	DownloadQueued    DownloadStatus = "queued"
	DownloadRunning   DownloadStatus = "downloading"
	DownloadDone      DownloadStatus = "done"
	DownloadFailed    DownloadStatus = "failed"
	DownloadCancelled DownloadStatus = "cancelled"
)

// Download is a file from Nexus Mods, queued, on its way or there.
type Download struct {
	ID       string `json:"id"`
	GameID   string `json:"gameId"`
	GameName string `json:"gameName"`
	// ProfileID is the profile the mod goes into: the game's active one
	// when the link came. Its archive is kept in the profile's downloads.
	ProfileID string `json:"profileId"`
	ModID     int    `json:"modId"`
	FileID    int    `json:"fileId"`
	Name      string `json:"name"`
	FileName  string `json:"fileName"`
	Version   string `json:"version"`
	Size      int64  `json:"size"`
	Done      int64  `json:"done"`
	// Speed is in bytes a second, while downloading.
	Speed  int64          `json:"speed"`
	Status DownloadStatus `json:"status"`
	Error  string         `json:"error"`
	// Path is where the file is once it is there.
	Path string `json:"path"`

	link   nexus.Link
	taken  bool
	cancel context.CancelFunc
}

// NexusAccount is the Nexus Mods account downloads are made with.
type NexusAccount struct {
	HasKey  bool   `json:"hasKey"`
	Name    string `json:"name"`
	Premium bool   `json:"premium"`
}

// DownloadService downloads mods from Nexus Mods, one at a time, for the
// links its "Mod Manager Download" buttons open, and hands the files to the
// main window to install into the active profile of their game. The archives
// stay in that profile's downloads folder, as MO2 keeps its downloads.
type DownloadService struct {
	lib     *library.Library
	root    string
	agent   string
	baseURL string

	mu        sync.Mutex
	downloads []*Download
	running   bool
	nextID    int
	// errors are the reasons links from outside were not downloaded, kept
	// for a window that was not there yet to hear them.
	errors []string
}

func NewDownloadService(lib *library.Library, root, agent string) *DownloadService {
	return &DownloadService{lib: lib, root: root, agent: agent, baseURL: nexus.DefaultAPIURL}
}

// keyFile keeps the API key apart from the other settings, readable by the
// user alone.
func (s *DownloadService) keyFile() string { return filepath.Join(s.root, "nexus.json") }

type storedKey struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Premium bool   `json:"premium"`
}

func (s *DownloadService) stored() storedKey {
	var k storedKey
	if data, err := os.ReadFile(s.keyFile()); err == nil {
		json.Unmarshal(data, &k)
	}
	return k
}

func (s *DownloadService) client(key string) *nexus.Client {
	return &nexus.Client{BaseURL: s.baseURL, Key: key, UserAgent: s.agent}
}

// Account says which Nexus Mods account downloads are made with.
func (s *DownloadService) Account() NexusAccount {
	k := s.stored()
	return NexusAccount{HasKey: k.Key != "", Name: k.Name, Premium: k.Premium}
}

// SetAPIKey checks a personal API key with Nexus Mods and keeps it; an empty
// key forgets the one kept.
func (s *DownloadService) SetAPIKey(ctx context.Context, key string) (NexusAccount, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		os.Remove(s.keyFile())
		return NexusAccount{}, nil
	}
	account, err := s.client(key).Validate(ctx)
	if err != nil {
		return NexusAccount{}, err
	}
	data, _ := json.Marshal(storedKey{Key: key, Name: account.Name, Premium: account.Premium})
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return NexusAccount{}, err
	}
	if err := os.WriteFile(s.keyFile(), data, 0o600); err != nil {
		return NexusAccount{}, err
	}
	return NexusAccount{HasKey: true, Name: account.Name, Premium: account.Premium}, nil
}

// NXMHandler says what opens nxm:// links: whether it is Hermit, and the
// desktop entry that does.
func (s *DownloadService) NXMHandler() NXMHandler {
	entry := platform.NXMHandler()
	return NXMHandler{Hermit: entry == platform.NXMDesktopFile, Entry: entry}
}

type NXMHandler struct {
	Hermit bool   `json:"hermit"`
	Entry  string `json:"entry"`
}

// RegisterNXM makes Hermit open the links of "Mod Manager Download" buttons.
func (s *DownloadService) RegisterNXM() (NXMHandler, error) {
	if err := platform.RegisterNXM(filepath.Join(xdg.DataHome, "applications"), executablePath()); err != nil {
		return NXMHandler{}, err
	}
	return s.NXMHandler(), nil
}

// HandleLink is HandleNXM for a link that came from outside, through the
// command line: the reason it is not downloaded goes to the main window.
func (s *DownloadService) HandleLink(link string) {
	if err := s.HandleNXM(link); err != nil {
		s.mu.Lock()
		s.errors = append(s.errors, err.Error())
		s.mu.Unlock()
		emit(DownloadErrorEvent, err.Error())
	}
}

// TakeErrors returns, once, why links from outside were not downloaded.
func (s *DownloadService) TakeErrors() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	errs := s.errors
	s.errors = nil
	return errs
}

// HandleNXM queues the file an nxm:// link is to, for the game of Hermit's
// whose part of Nexus Mods it is in.
func (s *DownloadService) HandleNXM(link string) error {
	l, err := nexus.ParseLink(link)
	if err != nil {
		return err
	}
	if !s.Account().HasKey {
		return nexus.ErrNoKey
	}
	game, err := s.gameFor(l.Game)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.nextID++
	d := &Download{
		ID: fmt.Sprint(s.nextID), GameID: game.ID, GameName: game.Name, ProfileID: game.ActiveProfile,
		ModID: l.ModID, FileID: l.FileID, Name: fmt.Sprintf("Mod %d", l.ModID),
		Status: DownloadQueued, link: l,
	}
	s.downloads = append(s.downloads, d)
	start := !s.running
	s.running = true
	s.mu.Unlock()
	s.changed()
	if start {
		go s.run()
	}
	return nil
}

// gameFor finds the game of Hermit's a part of Nexus Mods is for, among those
// whose mods are deployed.
func (s *DownloadService) gameFor(domain string) (library.Game, error) {
	games, err := s.lib.ListGames()
	if err != nil {
		return library.Game{}, err
	}
	for _, g := range games {
		if m, ok := g.Modding(); ok && strings.EqualFold(m.Nexus, domain) {
			return g, nil
		}
	}
	if name, ok := library.NexusGame(domain); ok {
		return library.Game{}, fmt.Errorf("the mod is for %s, which is not added to Hermit; add the game first", name)
	}
	return library.Game{}, fmt.Errorf("the mod is for a game Hermit cannot install mods from Nexus Mods into (%s)", domain)
}

// List returns the downloads, oldest first.
func (s *DownloadService) List() []Download {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]Download, len(s.downloads))
	for i, d := range s.downloads {
		list[i] = *d
	}
	return list
}

// Cancel stops a download, or takes it out of the queue.
func (s *DownloadService) Cancel(id string) {
	s.mu.Lock()
	for _, d := range s.downloads {
		if d.ID == id && (d.Status == DownloadQueued || d.Status == DownloadRunning) {
			d.Status = DownloadCancelled
			if d.cancel != nil {
				d.cancel()
			}
		}
	}
	s.mu.Unlock()
	s.changed()
}

// ClearFinished forgets the downloads that are over.
func (s *DownloadService) ClearFinished() {
	s.mu.Lock()
	s.downloads = slices.DeleteFunc(s.downloads, func(d *Download) bool {
		return d.Status != DownloadQueued && d.Status != DownloadRunning
	})
	s.mu.Unlock()
	s.changed()
}

// TakeFinished returns the downloads that are there and not installed yet,
// once each: the main window installs them.
func (s *DownloadService) TakeFinished() []Download {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Download
	for _, d := range s.downloads {
		if d.Status == DownloadDone && !d.taken {
			d.taken = true
			out = append(out, *d)
		}
	}
	return out
}

func (s *DownloadService) changed() { emit(DownloadsEvent, s.List()) }

// run downloads what is queued, one file at a time, until nothing is.
func (s *DownloadService) run() {
	for {
		s.mu.Lock()
		i := slices.IndexFunc(s.downloads, func(d *Download) bool { return d.Status == DownloadQueued })
		if i < 0 {
			s.running = false
			s.mu.Unlock()
			return
		}
		d := s.downloads[i]
		ctx, cancel := context.WithCancel(context.Background())
		d.Status, d.cancel = DownloadRunning, cancel
		s.mu.Unlock()
		s.changed()

		err := s.download(ctx, d)
		cancel()
		s.mu.Lock()
		switch {
		case d.Status == DownloadCancelled:
		case err != nil:
			d.Status, d.Error = DownloadFailed, err.Error()
		default:
			d.Status = DownloadDone
		}
		d.Speed, d.cancel = 0, nil
		done := d.Status == DownloadDone
		s.mu.Unlock()
		s.changed()
		if done {
			emit(DownloadFinishedEvent, d.ID)
		}
	}
}

func (s *DownloadService) download(ctx context.Context, d *Download) error {
	c := s.client(s.stored().Key)
	file, err := c.File(ctx, d.link)
	if err != nil {
		return err
	}
	name := file.Name
	if mod, err := c.Mod(ctx, d.link); err == nil && mod.Name != "" {
		name = mod.Name
	}
	s.mu.Lock()
	d.Name, d.FileName, d.Version, d.Size = name, filepath.Base(file.FileName), file.Version, file.Size
	s.mu.Unlock()
	s.changed()

	profileDir, err := s.lib.ProfileDir(d.GameID, d.ProfileID)
	if err != nil {
		return err
	}
	dir := filepath.Join(profileDir, deploy.DownloadsDir)
	target := filepath.Join(dir, d.FileName)
	// Downloaded before: the profile keeps the archives of its mods.
	if fi, err := os.Stat(target); err == nil && fi.Size() == file.Size {
		s.mu.Lock()
		d.Done, d.Path = file.Size, target
		s.mu.Unlock()
		return nil
	}
	uri, err := c.DownloadURL(ctx, d.link)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", s.agent)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the download server answered %s", resp.Status)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	part := target + ".part"
	out, err := os.Create(part)
	if err != nil {
		return err
	}
	defer os.Remove(part)
	if _, err := io.Copy(out, &progressReader{r: resp.Body, report: func(done int64, speed int64) {
		s.mu.Lock()
		d.Done, d.Speed = done, speed
		s.mu.Unlock()
		s.changed()
	}}); err != nil {
		out.Close()
		if errors.Is(ctx.Err(), context.Canceled) {
			return ctx.Err()
		}
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Rename(part, target); err != nil {
		return err
	}
	s.mu.Lock()
	d.Path = target
	s.mu.Unlock()
	return nil
}

// progressReader reports how much was read, and how fast, a few times a
// second.
type progressReader struct {
	r      io.Reader
	report func(done, speed int64)
	done   int64
	last   time.Time
	lastAt int64
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	now := time.Now()
	if p.last.IsZero() {
		p.last = now
	}
	if elapsed := now.Sub(p.last); elapsed >= 250*time.Millisecond || err != nil {
		speed := int64(float64(p.done-p.lastAt) / max(elapsed.Seconds(), 0.001))
		p.last, p.lastAt = now, p.done
		p.report(p.done, speed)
	}
	return n, err
}
