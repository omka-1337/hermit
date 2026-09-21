package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"hermit/internal/launch"
	"hermit/internal/library"
	"hermit/internal/platform"
	"hermit/internal/steam"
)

type LaunchInfo struct {
	// Supported is false when the manager cannot launch the game with mods;
	// Reason explains why.
	Supported bool   `json:"supported"`
	Reason    string `json:"reason"`
	// LaunchOptions is the value to put into the game's Steam Launch Options.
	LaunchOptions string `json:"launchOptions"`
	// External is true for games Steam does not start: Hermit can neither
	// start them itself nor read their launcher's settings, so the user
	// puts LaunchPrefix into whatever launcher runs the game.
	External bool `json:"external"`
	// LaunchPrefix is the wrapper to put in front of the game command in
	// launchers that take a command prefix, such as Lutris or Heroic.
	LaunchPrefix string `json:"launchPrefix"`
	// PortProton is true when an external Windows game can be started through
	// the PortProton Flatpak, which has nowhere to put a prefix, so Hermit
	// starts it itself.
	PortProton bool `json:"portProton"`
	// Configured reports whether Steam's saved config has these launch options.
	// Steam writes its config lazily, so false may just mean "not saved yet".
	Configured bool `json:"configured"`
	Running    bool `json:"running"`
	// RunningProfile is the profile of the running session, if any.
	RunningProfile string `json:"runningProfile"`
}

type LaunchService struct {
	lib        *library.Library
	steamRoots []string
}

func NewLaunchService(lib *library.Library, steamRoots []string) *LaunchService {
	return &LaunchService{lib: lib, steamRoots: steamRoots}
}

func (s *LaunchService) GetLaunchInfo(gameID string) (LaunchInfo, error) {
	game, err := s.lib.GetGame(gameID)
	if err != nil {
		return LaunchInfo{}, err
	}
	info := LaunchInfo{LaunchOptions: RecommendedLaunchOptions()}
	switch {
	case game.Runtime == library.RuntimeUnknown:
		info.Reason = "Could not tell whether the game runs natively or through Proton."
	case game.SteamAppID == "":
		// Without Steam there is no SteamAppId to find the game by, so the
		// wrapper is told which game it is.
		info.Supported, info.External = true, true
		info.LaunchPrefix = wrapperPrefix(game.ID)
		info.LaunchOptions = info.LaunchPrefix + " %command%"
		info.PortProton = game.Runtime == library.RuntimeProton && launch.PortProtonInstalled()
	default:
		info.Supported = true
	}
	if !info.External {
		for _, opts := range steam.LaunchOptions(s.steamRoots, game.SteamAppID) {
			if isOurLaunchOptions(opts) {
				info.Configured = true
			}
		}
	}
	if dataDir, err := s.lib.GameDataDir(gameID); err == nil {
		if session, err := launch.ReadSession(dataDir); err == nil && session.Running() {
			info.Running = true
			info.RunningProfile = session.ProfileID
		}
	}
	return info, nil
}

// Play makes the profile active and starts the game through Steam. Mods load
// only if the game's launch options run it through the wrapper.
func (s *LaunchService) Play(gameID, profileID string) error {
	info, err := s.GetLaunchInfo(gameID)
	if err != nil {
		return err
	}
	if !info.Supported {
		return errors.New(info.Reason)
	}
	if info.Running {
		return errors.New("the game is already running")
	}
	if info.External {
		return errors.New("this game is not started by Steam; start it from the launcher that runs it")
	}
	game, err := s.lib.SetActiveProfile(gameID, profileID)
	if err != nil {
		return err
	}
	cmd := platform.Command("xdg-open", "steam://rungameid/"+game.SteamAppID)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // xdg-open hands over to Steam and exits; reap it
	return nil
}

// PlayViaPortProton makes the profile active and starts the game through
// PortProton, wrapped in "run" so the profile is linked in for the session.
// The wrapper is a process of its own: the game outlives Hermit's window, and
// the links are still removed when it exits.
func (s *LaunchService) PlayViaPortProton(gameID, profileID string) error {
	info, err := s.GetLaunchInfo(gameID)
	if err != nil {
		return err
	}
	if !info.PortProton {
		return errors.New("PortProton is not installed, or this game does not run through Wine")
	}
	if info.Running {
		return errors.New("the game is already running")
	}
	game, err := s.lib.SetActiveProfile(gameID, profileID)
	if err != nil {
		return err
	}
	exe := game.Executable
	if game.LaunchExecutable != "" {
		exe = filepath.FromSlash(game.LaunchExecutable)
	}
	if !filepath.IsAbs(exe) {
		exe = filepath.Join(game.Path, exe)
	}
	args := append([]string{"run", "--game", game.ID, "--"}, launch.PortProtonCommand(exe)...)
	cmd := platform.Command(executablePath(), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting PortProton: %w", err)
	}
	return cmd.Process.Release()
}

// notLaunchable are executables that ship next to games but never start
// them: Unity's crash reporter, installers and uninstallers, redistributables.
var notLaunchable = []string{"unitycrashhandler", "crashhandler", "crashreport", "unins", "setup", "redist", "vc_redist", "dxsetup", "dotnet"}

// ListExecutables returns the .exe files a game could be started from,
// relative to its folder: the game's own executable first, then any other
// found a few levels deep, such as a launcher a mod asks to start the game
// with.
func (s *LaunchService) ListExecutables(gameID string) ([]string, error) {
	game, err := s.lib.GetGame(gameID)
	if err != nil {
		return nil, err
	}
	const maxDepth = 3
	var found []string
	err = filepath.WalkDir(game.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable parts of the folder are skipped
		}
		rel, _ := filepath.Rel(game.Path, path)
		if d.IsDir() {
			if rel != "." && (strings.Count(rel, string(filepath.Separator)) >= maxDepth-1 || isHiddenOrData(d.Name())) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(d.Name()), ".exe") {
			return nil
		}
		lower := strings.ToLower(d.Name())
		for _, skip := range notLaunchable {
			if strings.Contains(lower, skip) {
				return nil
			}
		}
		found = append(found, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	own := filepath.ToSlash(game.Executable)
	sort.SliceStable(found, func(i, j int) bool {
		if (found[i] == own) != (found[j] == own) {
			return found[i] == own
		}
		return strings.ToLower(found[i]) < strings.ToLower(found[j])
	})
	return found, nil
}

// isHiddenOrData skips folders that hold no launchers: dot folders, Unity's
// *_Data folders and the parts of BepInEx a mod manager put there.
func isHiddenOrData(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, ".") || strings.HasSuffix(lower, "_data") || lower == "bepinex" || lower == "dotnet"
}

// RecommendedLaunchOptions is the Steam Launch Options value that runs games
// through this executable.
func RecommendedLaunchOptions() string {
	return `"` + executablePath() + `" run -- %command%`
}

// wrapperPrefix is what goes in front of a game's command in any launcher;
// the game is named because only Steam says which game it is starting.
func wrapperPrefix(gameID string) string {
	return `"` + executablePath() + `" run --game ` + gameID + ` --`
}

func isOurLaunchOptions(opts string) bool {
	exe := filepath.Base(executablePath())
	return strings.Contains(opts, "%command%") &&
		(strings.Contains(opts, exe+`" run `) || strings.Contains(opts, exe+" run "))
}

func executablePath() string {
	if appImage := os.Getenv("APPIMAGE"); appImage != "" {
		return appImage
	}
	exe, err := os.Executable()
	if err != nil {
		return ID
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

// GetLaunchReport returns what BepInEx loaded during the last session of a
// profile, or nil if the profile was not launched with mods yet.
func (s *LaunchService) GetLaunchReport(gameID, profileID string) (*launch.Report, error) {
	dataDir, err := s.lib.GameDataDir(gameID)
	if err != nil {
		return nil, err
	}
	return launch.ReadReport(dataDir, profileID)
}
