package app

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"hermit/internal/platform"
	"hermit/internal/steam"
)

// icon is written next to the library so Steam has something to show for the
// shortcut: inside an AppImage every path is gone once Hermit exits.
//
//go:embed icon.png
var icon []byte

// shutdownWait is how long Steam is given to exit. It writes its own files on
// the way out, so the shortcut has to be added after that, not during.
const shutdownWait = 30 * time.Second

// SteamService adds Hermit itself to Steam as a non-Steam game, which is the
// only way to start it from Game Mode on a Steam Deck.
type SteamService struct {
	steamRoots []string
	dataDir    string
}

func NewSteamService(steamRoots []string, dataDir string) *SteamService {
	return &SteamService{steamRoots: steamRoots, dataDir: dataDir}
}

// SteamSetup is what the interface needs to know to offer the shortcut.
type SteamSetup struct {
	// Supported is false when there is no Steam to add anything to.
	Supported bool `json:"supported"`
	// Reason says why it is not supported.
	Reason string `json:"reason"`
	// Added is true once Steam lists a shortcut to this very build.
	Added bool `json:"added"`
	// Running reports whether Steam is running: adding the shortcut has to
	// restart it, because Steam rewrites its shortcuts when it exits.
	Running bool `json:"running"`
	// Exe is the path Steam would start.
	Exe string `json:"exe"`
}

func (s *SteamService) GetSetup() SteamSetup {
	setup := SteamSetup{Running: steam.Running()}
	exe, err := platform.ExecutablePath()
	if err != nil {
		setup.Reason = "Could not tell where Hermit is installed."
		return setup
	}
	setup.Exe = exe

	files := steam.ShortcutsFiles(s.steamRoots)
	if len(files) == 0 {
		setup.Reason = "No Steam user was found on this machine."
		return setup
	}
	setup.Supported = true
	for _, file := range files {
		has, err := steam.HasShortcut(file, exe)
		if err != nil {
			setup.Supported, setup.Reason = false, fmt.Sprintf("Steam's shortcuts could not be read: %v", err)
			return setup
		}
		if has {
			setup.Added = true
		}
	}
	return setup
}

// AddToSteam adds Hermit to every Steam user's shortcuts. Steam is closed
// first if it runs, and started again afterwards, since it keeps the list in
// memory and writes it back on exit.
func (s *SteamService) AddToSteam() (SteamSetup, error) {
	setup := s.GetSetup()
	if !setup.Supported {
		return setup, errors.New(setup.Reason)
	}
	if setup.Added {
		return setup, nil
	}

	wasRunning := steam.Running()
	if wasRunning {
		if err := steam.Shutdown(shutdownWait); err != nil {
			return setup, err
		}
	}

	shortcut := steam.Shortcut{
		AppName:  Name,
		Exe:      setup.Exe,
		StartDir: filepath.Dir(setup.Exe),
		Icon:     s.iconPath(),
	}
	var addErr error
	for _, file := range steam.ShortcutsFiles(s.steamRoots) {
		if _, err := steam.AddShortcut(file, shortcut); err != nil {
			addErr = err
			break
		}
	}

	// Steam goes back up even if writing failed: leaving the user without
	// their client would be worse than not having the shortcut.
	if wasRunning {
		if err := steam.Start(); err != nil {
			addErr = errors.Join(addErr, err)
		}
	}
	return s.GetSetup(), addErr
}

// iconPath writes Hermit's icon into the library directory and returns it, or
// "" when it cannot be written; Steam then shows its own placeholder.
func (s *SteamService) iconPath() string {
	if s.dataDir == "" || len(icon) == 0 {
		return ""
	}
	path := filepath.Join(s.dataDir, "hermit.png")
	if data, err := os.ReadFile(path); err == nil && len(data) == len(icon) {
		return path
	}
	if err := os.WriteFile(path, icon, 0o644); err != nil {
		return ""
	}
	return path
}
