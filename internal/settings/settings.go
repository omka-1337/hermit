// Package settings stores manager-wide preferences in config.json.
package settings

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// UIMode selects the interface layout. Auto follows the hardware: the Steam
// Deck gets the deck layout, everything else the desktop one.
type UIMode string

const (
	UIModeAuto    UIMode = "auto"
	UIModeDesktop UIMode = "desktop"
	UIModeDeck    UIMode = "deck"
)

// BrowseView is how the mod browser lists packages. Auto follows the layout:
// cards on the deck, a list on the desktop.
type BrowseView string

const (
	BrowseViewAuto  BrowseView = "auto"
	BrowseViewCards BrowseView = "cards"
	BrowseViewList  BrowseView = "list"
)

type Settings struct {
	// SetupCompleted is set once the first-run setup is finished or skipped.
	SetupCompleted bool `json:"setupCompleted"`
	// AllowNSFW shows packages marked NSFW when browsing mods.
	AllowNSFW bool `json:"allowNsfw"`
	// UIMode is the interface layout; empty means auto.
	UIMode UIMode `json:"uiMode"`
	// BrowseView is the mod browser layout; empty means auto.
	BrowseView BrowseView `json:"browseView"`
	// LastGame is the game that was open last, opened again on the next
	// start; its active profile comes with it.
	LastGame string `json:"lastGame,omitempty"`
	// DownloadLimit is the speed all downloads share at most, in KB a
	// second; 0 lets them go as fast as the connection does.
	DownloadLimit int `json:"downloadLimit,omitempty"`
	// PanelWidths are the widths, in pixels, the panels of the desktop
	// layout were dragged to, by panel.
	PanelWidths map[string]int `json:"panelWidths,omitempty"`
}

type Store struct {
	path string
	mu   sync.Mutex
	// OnUpdate, when set, is told the settings Update saved.
	OnUpdate func(Settings)
}

func NewStore(root string) *Store {
	return &Store{path: filepath.Join(root, "config.json")}
}

func (s *Store) Get() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *Store) CompleteSetup() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.load()
	if err != nil {
		return Settings{}, err
	}
	st.SetupCompleted = true
	return st, s.save(st)
}

// Update replaces user-editable preferences; SetupCompleted, LastGame and
// PanelWidths are kept as stored.
func (s *Store) Update(st Settings) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.load()
	if err != nil {
		return Settings{}, err
	}
	st.SetupCompleted = current.SetupCompleted
	st.LastGame = current.LastGame
	st.PanelWidths = current.PanelWidths
	st.DownloadLimit = max(0, st.DownloadLimit)
	if err := s.save(st); err != nil {
		return st, err
	}
	if s.OnUpdate != nil {
		s.OnUpdate(st)
	}
	return st, nil
}

// SetPanelWidth records the width a panel was dragged to, in pixels, to
// give it the same width next time.
func (s *Store) SetPanelWidth(panel string, width int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.load()
	if err != nil {
		return err
	}
	if st.PanelWidths == nil {
		st.PanelWidths = map[string]int{}
	}
	st.PanelWidths[panel] = width
	return s.save(st)
}

// SetLastGame records the game that is open, to open it again next time.
func (s *Store) SetLastGame(gameID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.load()
	if err != nil {
		return err
	}
	if st.LastGame == gameID {
		return nil
	}
	st.LastGame = gameID
	return s.save(st)
}

func (s *Store) load() (Settings, error) {
	st := Settings{UIMode: UIModeAuto, BrowseView: BrowseViewAuto}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, err
	}
	if st.UIMode != UIModeDesktop && st.UIMode != UIModeDeck {
		st.UIMode = UIModeAuto
	}
	if st.BrowseView != BrowseViewCards && st.BrowseView != BrowseViewList {
		st.BrowseView = BrowseViewAuto
	}
	return st, nil
}

func (s *Store) save(st Settings) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
