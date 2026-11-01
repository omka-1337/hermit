package app

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"

	"hermit/internal/platform"
)

const (
	// BrowserChangedEvent carries the platform.BrowserState of the browser
	// whenever its page changes.
	BrowserChangedEvent = "browser:changed"
	// BrowserDownloadEvent says the page handed a file to Hermit, so the
	// browser closes and the user goes back where they came from.
	BrowserDownloadEvent = "browser:download"
	// BrowserCrashedEvent says, with why, that the page is gone.
	BrowserCrashedEvent = "browser:crashed"
)

// BrowserService shows websites inside Hermit's window, over the page
// where the app's pages are, in a browser that shares nothing with them:
// for now the pages of Nexus Mods whose "Mod Manager Download" hands files
// to Hermit, which it then downloads without the system's browser.
type BrowserService struct {
	// Window returns the app's main window, for the browser to be put in;
	// main sets it.
	Window func() unsafe.Pointer

	downloads *DownloadService
	dataDir   string
	cacheDir  string

	mu      sync.Mutex
	browser *platform.Browser
	// place is where the page last asked the browser to be, and shown
	// whether it is to be seen: the page can ask before the browser is
	// made, the first time a page opens.
	place [4]int
	shown bool
}

func NewBrowserService(downloads *DownloadService, root string) *BrowserService {
	return &BrowserService{
		downloads: downloads,
		dataDir:   filepath.Join(root, "browser"),
		cacheDir:  filepath.Join(root, "cache", "browser"),
	}
}

// Open shows a page of Nexus Mods; Place puts the browser where it goes.
func (s *BrowserService) Open(link string) error {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || (u.Host != "nexusmods.com" && !strings.HasSuffix(u.Host, ".nexusmods.com")) {
		return errors.New("only pages of Nexus Mods open in Hermit")
	}
	return application.InvokeSyncWithError(func() error {
		b, err := s.get()
		if err != nil {
			return err
		}
		b.Load(link)
		return nil
	})
}

// Place shows the browser over the part of the window at x, y, w by h, in
// the page's pixels.
func (s *BrowserService) Place(x, y, w, h float64) {
	application.InvokeSync(func() {
		s.mu.Lock()
		s.place = [4]int{int(x + 0.5), int(y + 0.5), int(w + 0.5), int(h + 0.5)}
		s.shown = true
		b := s.browser
		s.mu.Unlock()
		if b != nil {
			b.Place(s.place[0], s.place[1], s.place[2], s.place[3])
		}
	})
}

// Hide takes the browser off the window; it keeps its page.
func (s *BrowserService) Hide() {
	application.InvokeSync(func() {
		s.mu.Lock()
		s.shown = false
		b := s.browser
		s.mu.Unlock()
		if b != nil {
			b.Hide()
		}
	})
}

// Back goes to the page before.
func (s *BrowserService) Back() {
	application.InvokeSync(func() {
		if b := s.current(); b != nil {
			b.Back()
		}
	})
}

// Reload loads the page again.
func (s *BrowserService) Reload() {
	application.InvokeSync(func() {
		if b := s.current(); b != nil {
			b.Reload()
		}
	})
}

// State is what the browser shows.
func (s *BrowserService) State() platform.BrowserState {
	return application.InvokeSyncWithResult(func() platform.BrowserState {
		if b := s.current(); b != nil {
			return b.State()
		}
		return platform.BrowserState{}
	})
}

func (s *BrowserService) current() *platform.Browser {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.browser
}

// get makes the browser the first time it is needed, on the GTK thread.
func (s *BrowserService) get() (*platform.Browser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.browser != nil {
		return s.browser, nil
	}
	if s.Window == nil {
		return nil, errors.New("there is no window to show the page in")
	}
	b, err := platform.NewBrowser(s.Window(), s.dataDir, s.cacheDir)
	if err != nil {
		return nil, err
	}
	// The callbacks run on the GTK thread; events go out from elsewhere.
	b.OnChange = func() {
		state := b.State()
		go emit(BrowserChangedEvent, state)
	}
	b.OnCrash = func(reason string) {
		go emit(BrowserCrashedEvent, reason)
	}
	b.OnNXM = func(link string) {
		s.mu.Lock()
		s.shown = false
		s.mu.Unlock()
		b.Hide()
		go func() {
			s.downloads.HandleLink(link)
			emit(BrowserDownloadEvent, map[string]string{"link": link})
		}()
	}
	s.browser = b
	// Placed before it was made.
	if s.shown {
		b.Place(s.place[0], s.place[1], s.place[2], s.place[3])
	}
	return b, nil
}
