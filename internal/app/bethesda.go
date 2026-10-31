package app

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"hermit/internal/bethesda"
	"hermit/internal/launch"
	"hermit/internal/library"
	"hermit/internal/platform"
	"hermit/internal/plugininfo"
)

// BethesdaService manages the profiles of Creation Engine games, whose mods
// each keep a folder of their own and are deployed into Data in order.
type BethesdaService struct {
	lib        *library.Library
	steamRoots []string
}

func NewBethesdaService(lib *library.Library, steamRoots []string) *BethesdaService {
	return &BethesdaService{lib: lib, steamRoots: steamRoots}
}

// InspectMO2 reads a Mod Organizer 2 instance, to show what importing it
// would bring.
func (s *BethesdaService) InspectMO2(dir string) (bethesda.MO2Instance, error) {
	return bethesda.ReadMO2(dir)
}

// ImportMO2 fills an empty profile with the mods, their order, the plugin
// list and the INI files of a profile of an MO2 instance, together with MO2's
// overwrite folder.
func (s *BethesdaService) ImportMO2(gameID, profileID, dir, mo2Profile string) (library.Profile, error) {
	game, bg, err := s.creationGame(gameID)
	if err != nil {
		return library.Profile{}, err
	}
	if err := s.notRunning(gameID); err != nil {
		return library.Profile{}, err
	}
	profile, err := s.lib.GetProfile(gameID, profileID)
	if err != nil {
		return library.Profile{}, err
	}
	if len(profile.Mods) > 0 {
		return library.Profile{}, errors.New("import into an empty profile: this one already has mods")
	}
	in, err := bethesda.ReadMO2(dir)
	if err != nil {
		return library.Profile{}, err
	}
	if in.GameName != bg.MO2Name {
		return library.Profile{}, fmt.Errorf("this MO2 instance is set up for %s, not %s", in.GameName, game.Name)
	}
	p, err := in.ReadProfile(mo2Profile)
	if err != nil {
		return library.Profile{}, err
	}
	profileDir, err := s.lib.ProfileDir(gameID, profileID)
	if err != nil {
		return library.Profile{}, err
	}
	data, err := gameData(game.Path)
	if err != nil {
		return library.Profile{}, err
	}

	ids := make([]string, len(p.Mods))
	mods := make([]library.Mod, len(p.Mods))
	for i, m := range p.Mods {
		ids[i] = uniqueModID(m.Folder, ids[:i])
		if m.Separator {
			mods[i] = separator(ids[i], m.SeparatorName())
			continue
		}
		source := library.ModSource{Type: library.SourceLocal, URL: m.File}
		if m.NexusID > 0 {
			source = library.ModSource{Type: library.SourceNexus, URL: bg.NexusURL(m.NexusID), Asset: m.File}
		}
		mods[i] = library.Mod{
			ID:                ids[i],
			Name:              m.Folder,
			Version:           m.Version,
			Enabled:           m.Enabled,
			Active:            m.Enabled,
			UnmetDependencies: []string{},
			Source:            source,
			Dependencies:      []string{},
			Files:             []string{},
			Plugins:           []plugininfo.Plugin{},
			Notes:             m.Comments,
		}
	}
	if err := in.ImportMO2(p, ids, profileDir, data, bg.INIs); err != nil {
		// Take back what was brought over, so the import can be tried again.
		os.RemoveAll(filepath.Join(profileDir, bethesda.ModsDir))
		os.RemoveAll(filepath.Join(profileDir, bethesda.OverwriteDir))
		os.RemoveAll(filepath.Join(profileDir, bethesda.INIDir))
		os.MkdirAll(filepath.Join(profileDir, bethesda.ModsDir), 0o755)
		return library.Profile{}, err
	}
	return s.lib.UpdateProfile(gameID, profileID, func(p *library.Profile) error {
		p.Mods = mods
		return nil
	})
}

// ImportMO2AsProfile creates a profile called name, in a folder of its own
// inside location unless that is empty, and imports a profile of an MO2
// instance into it. Nothing is left behind when the import fails.
func (s *BethesdaService) ImportMO2AsProfile(gameID, dir, mo2Profile, name, location string) (library.Profile, error) {
	if _, _, err := s.creationGame(gameID); err != nil {
		return library.Profile{}, err
	}
	// A folder that is no MO2 instance should not cost an empty profile.
	if _, err := bethesda.ReadMO2(dir); err != nil {
		return library.Profile{}, err
	}
	created, err := s.lib.CreateProfile(gameID, strings.TrimSpace(name))
	if err != nil {
		return library.Profile{}, err
	}
	if location != "" {
		if _, err := s.lib.MoveProfile(gameID, created.ID, location); err != nil {
			s.lib.RemoveProfile(gameID, created.ID)
			return library.Profile{}, err
		}
	}
	p, err := s.ImportMO2(gameID, created.ID, dir, mo2Profile)
	if err != nil {
		s.lib.RemoveProfile(gameID, created.ID)
		return library.Profile{}, err
	}
	return p, nil
}

// AddSeparator puts a new separator, a heading over the mods after it, at
// index in the profile's order.
func (s *BethesdaService) AddSeparator(gameID, profileID, name string, index int) (library.Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return library.Profile{}, library.ErrEmptyName
	}
	return s.lib.UpdateProfile(gameID, profileID, func(p *library.Profile) error {
		taken := make([]string, len(p.Mods))
		for i, m := range p.Mods {
			taken[i] = m.ID
		}
		index = max(0, min(index, len(p.Mods)))
		p.Mods = slices.Insert(p.Mods, index, separator(uniqueModID(name+" separator", taken), name))
		return nil
	})
}

// RenameSeparator changes the heading of a separator.
func (s *BethesdaService) RenameSeparator(gameID, profileID, id, name string) (library.Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return library.Profile{}, library.ErrEmptyName
	}
	return s.lib.UpdateProfile(gameID, profileID, func(p *library.Profile) error {
		i := slices.IndexFunc(p.Mods, func(m library.Mod) bool { return m.ID == id && m.Separator })
		if i < 0 {
			return fmt.Errorf("separator %q: %w", id, library.ErrNotFound)
		}
		p.Mods[i].Name = name
		return nil
	})
}

func separator(id, name string) library.Mod {
	return library.Mod{
		ID:                id,
		Name:              name,
		Separator:         true,
		UnmetDependencies: []string{},
		Dependencies:      []string{},
		Files:             []string{},
		Plugins:           []plugininfo.Plugin{},
	}
}

// Conflicts tells, for each mod of the profile, which other mods have files
// in common with it and how many; the order decides which of them wins.
func (s *BethesdaService) Conflicts(gameID, profileID string) (map[string]map[string]int, error) {
	profile, err := s.lib.GetProfile(gameID, profileID)
	if err != nil {
		return nil, err
	}
	profileDir, err := s.lib.ProfileDir(gameID, profileID)
	if err != nil {
		return nil, err
	}
	var mods []string
	for _, m := range profile.Mods {
		if !m.Separator {
			mods = append(mods, m.ID)
		}
	}
	return bethesda.Conflicts(profileDir, mods)
}

// MoveMods moves mods, in the order they have among themselves, to just
// before the mod before, or to the end when before is empty; later mods win
// over earlier ones. Dragging a folded separator moves the mods under it too.
func (s *BethesdaService) MoveMods(gameID, profileID string, ids []string, before string) (library.Profile, error) {
	return s.lib.UpdateProfile(gameID, profileID, func(p *library.Profile) error {
		var picked, rest []library.Mod
		for _, m := range p.Mods {
			if slices.Contains(ids, m.ID) {
				picked = append(picked, m)
			} else {
				rest = append(rest, m)
			}
		}
		if len(picked) != len(ids) {
			return fmt.Errorf("mods %v: %w", ids, library.ErrNotFound)
		}
		at := len(rest)
		if before != "" {
			if at = slices.IndexFunc(rest, func(m library.Mod) bool { return m.ID == before }); at < 0 {
				return fmt.Errorf("mod %q to move before: %w", before, library.ErrNotFound)
			}
		}
		p.Mods = slices.Concat(rest[:at], picked, rest[at:])
		return nil
	})
}

// RemoveMod deletes a mod and its folder from the profile, or a separator
// from its list.
func (s *BethesdaService) RemoveMod(gameID, profileID, modID string) (library.Profile, error) {
	if err := s.notRunning(gameID); err != nil {
		return library.Profile{}, err
	}
	profileDir, err := s.lib.ProfileDir(gameID, profileID)
	if err != nil {
		return library.Profile{}, err
	}
	return s.lib.UpdateProfile(gameID, profileID, func(p *library.Profile) error {
		i := slices.IndexFunc(p.Mods, func(m library.Mod) bool { return m.ID == modID })
		if i < 0 {
			return fmt.Errorf("mod %q: %w", modID, library.ErrNotFound)
		}
		if err := os.RemoveAll(filepath.Join(profileDir, bethesda.ModsDir, modID)); err != nil {
			return err
		}
		p.Mods = slices.Delete(p.Mods, i, i+1)
		return nil
	})
}

// Tool is a modding tool a mod of the profile brings, such as Pandora or
// BodySlide: a program run with the profile's mods in Data.
type Tool struct {
	// Path is where the tool is in Data while the profile is deployed,
	// slash-separated.
	Path string `json:"path"`
	Name string `json:"name"`
	// Mod is the name of the mod the tool comes with.
	Mod string `json:"mod"`
}

// notTools are programs mods carry along that tools run themselves, never
// the user: converters, compilers, installers of the libraries they ship.
var notTools = regexp.MustCompile(`(?i)^(hkxcmd|papyrus(compiler|assembler)|wininst-.*|unins.*|vc_?redist.*|.*crash.*report.*)\.exe$`)

// Tools lists the tools the enabled mods of the active profile bring, in
// the order of the mods.
func (s *BethesdaService) Tools(gameID string) ([]Tool, error) {
	game, _, err := s.creationGame(gameID)
	if err != nil {
		return nil, err
	}
	profile, err := s.lib.GetProfile(gameID, game.ActiveProfile)
	if err != nil {
		return nil, err
	}
	profileDir, err := s.lib.ProfileDir(gameID, game.ActiveProfile)
	if err != nil {
		return nil, err
	}
	tools := []Tool{}
	for _, m := range profile.Mods {
		if !m.Enabled || m.Separator {
			continue
		}
		modDir := filepath.Join(profileDir, bethesda.ModsDir, m.ID)
		filepath.WalkDir(modDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(modDir, p)
			if d.IsDir() {
				// Root holds what starts the game, Lib a tool's own libraries.
				if name := strings.ToLower(d.Name()); rel != "." && (name == bethesda.RootDir && filepath.Dir(rel) == "." || name == "lib") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.EqualFold(filepath.Ext(p), ".exe") && !notTools.MatchString(d.Name()) {
				tools = append(tools, Tool{Path: filepath.ToSlash(rel), Name: strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())), Mod: m.Name})
			}
			return nil
		})
	}
	return tools, nil
}

// RunTool runs a tool of the profile in the game's Proton prefix, the way
// Steam runs the game, with the profile deployed for as long as it runs;
// what the tool writes into Data is kept in the profile's overwrite folder.
// It returns once the tool has started.
func (s *BethesdaService) RunTool(gameID, path string) error {
	game, _, err := s.creationGame(gameID)
	if err != nil {
		return err
	}
	if err := s.notRunning(gameID); err != nil {
		return err
	}
	tools, err := s.Tools(gameID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(tools, func(t Tool) bool { return t.Path == path }) {
		return fmt.Errorf("no mod of the profile brings %s", path)
	}
	compatData := bethesda.SteamCompatData(game.Path, game.SteamAppID)
	if compatData == "" {
		return errors.New("the game's Proton prefix was not found; Steam makes it the first time it starts the game")
	}
	data, err := gameData(game.Path)
	if err != nil {
		return err
	}
	command, err := launch.SteamProtonCommand(s.steamRoots, compatData, game.SteamAppID, filepath.Join(data, filepath.FromSlash(path)))
	if err != nil {
		return err
	}
	// The wrapper deploys the profile, runs the tool and cleans up after it,
	// as a process of its own: the tool may outlive Hermit's window.
	cmd := platform.Command(executablePath(), append([]string{"run", "--game", game.ID, "--tool", "--"}, command...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", filepath.Base(path), err)
	}
	return cmd.Process.Release()
}

// LogFile is a log the script extender or one of its plugins wrote.
type LogFile struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// maxLog is how much of a log ReadLog returns: the end of a longer one, where
// the last session is.
const maxLog = 2 << 20

// Logs lists the logs in the script extender's folder, SKSE for Skyrim,
// newest first. A game not started yet has none.
func (s *BethesdaService) Logs(gameID string) ([]LogFile, error) {
	dir, err := s.logDir(gameID)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []LogFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	logs := []LogFile{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		logs = append(logs, LogFile{Name: e.Name(), Size: fi.Size(), Modified: fi.ModTime()})
	}
	slices.SortFunc(logs, func(a, b LogFile) int { return b.Modified.Compare(a.Modified) })
	return logs, nil
}

// ReadLog returns a log from the script extender's folder, or its last
// maxLog bytes when it is longer.
func (s *BethesdaService) ReadLog(gameID, name string) (string, error) {
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		return "", fmt.Errorf("%q is not a log file", name)
	}
	dir, err := s.logDir(gameID)
	if err != nil {
		return "", err
	}
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	cut := ""
	if fi.Size() > maxLog {
		if _, err := f.Seek(-maxLog, io.SeekEnd); err != nil {
			return "", err
		}
		cut = fmt.Sprintf("… only the last %d MB of %d MB are shown\n", maxLog>>20, fi.Size()>>20)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return cut + string(data), nil
}

// OpenLogs shows the script extender's log folder in the file manager.
func (s *BethesdaService) OpenLogs(gameID string) error {
	dir, err := s.logDir(gameID)
	if err != nil {
		return err
	}
	cmd := platform.Command("xdg-open", dir)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening %s: %w", dir, err)
	}
	go cmd.Wait()
	return nil
}

// logDir is where the script extender of a game run by Steam's Proton
// writes its logs, in the game's prefix.
func (s *BethesdaService) logDir(gameID string) (string, error) {
	game, bg, err := s.creationGame(gameID)
	if err != nil {
		return "", err
	}
	compatData := bethesda.SteamCompatData(game.Path, game.SteamAppID)
	if compatData == "" {
		return "", errors.New("the game's Proton prefix was not found; Steam makes it the first time it starts the game")
	}
	return bethesda.ExtenderLogs(compatData, bg), nil
}

func (s *BethesdaService) creationGame(gameID string) (library.Game, bethesda.Game, error) {
	game, err := s.lib.GetGame(gameID)
	if err != nil {
		return library.Game{}, bethesda.Game{}, err
	}
	bg, ok := bethesda.Find(game.Executable)
	if game.Engine != library.EngineCreation || !ok {
		return library.Game{}, bethesda.Game{}, fmt.Errorf("%s is not a Creation Engine game", game.Name)
	}
	return game, bg, nil
}

// notRunning refuses changes to mod folders while the game has them linked in.
func (s *BethesdaService) notRunning(gameID string) error {
	dataDir, err := s.lib.GameDataDir(gameID)
	if err != nil {
		return err
	}
	if session, err := launch.ReadSession(dataDir); err == nil && session.Running() {
		return errors.New("the game is running; change its mods once it has exited")
	}
	return nil
}

// gameData returns the game's Data folder, in whatever letter case it has.
func gameData(gamePath string) (string, error) {
	entries, err := os.ReadDir(gamePath)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() && strings.EqualFold(e.Name(), "Data") {
			return filepath.Join(gamePath, e.Name()), nil
		}
	}
	return "", errors.New("the game folder has no Data folder")
}

var nonModID = regexp.MustCompile(`[^a-z0-9]+`)

// uniqueModID makes a folder name for a mod from its name, unlike the ones
// in taken.
func uniqueModID(name string, taken []string) string {
	base := strings.Trim(nonModID.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if base == "" {
		base = "mod"
	}
	id := base
	for n := 2; slices.Contains(taken, id); n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}
