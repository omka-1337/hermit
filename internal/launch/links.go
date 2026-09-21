// Package launch links a profile into a game directory for the duration of a
// game session and runs the game.
package launch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

// notLinked are profile entries that belong to the manager, not the game.
var notLinked = []string{"profile.json", "disabled"}

// loaderMarker is present in the profile root only when a BepInEx loader pack
// is installed and active; without it nothing is linked and the game runs vanilla.
const loaderMarker = "winhttp.dll"

var ErrConflict = errors.New("game folder already contains files with the same name")

// Session describes an active launch, stored as launch.json in the game's data dir.
type Session struct {
	ProfileID string    `json:"profileId"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
	// Links are names of symlinks created in the game directory.
	Links []string `json:"links"`
	// Modded records whether the game was started with BepInEx, so the
	// session can be finished by a process that did not start it.
	Modded bool `json:"modded"`
	// PPDB is the PortProton settings file the winhttp override was added to
	// for this session; Cleanup takes it out again.
	PPDB string `json:"ppdb,omitempty"`
}

func sessionPath(gameDataDir string) string { return filepath.Join(gameDataDir, "launch.json") }

// ReadSession returns the stored session, or nil if there is none.
func ReadSession(gameDataDir string) (*Session, error) {
	data, err := os.ReadFile(sessionPath(gameDataDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Running reports whether the session's process is still alive.
func (s *Session) Running() bool {
	if s == nil || s.PID <= 0 {
		return false
	}
	err := syscall.Kill(s.PID, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func writeSession(gameDataDir string, s Session) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sessionPath(gameDataDir), data, 0o644)
}

// LinkProfile symlinks the profile's BepInEx folder and loader files into
// gamePath. Existing symlinks into dataRoot (left by a crashed session) are
// replaced; real files or foreign symlinks are never touched.
func LinkProfile(gamePath, profileDir, dataRoot string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(profileDir, loaderMarker)); err != nil {
		return nil, nil
	}
	entries, err := os.ReadDir(profileDir)
	if err != nil {
		return nil, err
	}
	var names, conflicts []string
	for _, e := range entries {
		if slices.Contains(notLinked, e.Name()) {
			continue
		}
		target := filepath.Join(gamePath, e.Name())
		fi, err := os.Lstat(target)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return nil, err
		case fi.Mode()&fs.ModeSymlink != 0 && isOurLink(target, dataRoot):
		default:
			conflicts = append(conflicts, e.Name())
		}
		names = append(names, e.Name())
	}
	if len(conflicts) > 0 {
		return nil, fmt.Errorf("%w: %s", ErrConflict, strings.Join(conflicts, ", "))
	}

	var created []string
	for _, name := range names {
		target := filepath.Join(gamePath, name)
		_ = removeOurLink(target, dataRoot)
		if err := os.Symlink(filepath.Join(profileDir, name), target); err != nil {
			Unlink(gamePath, created, dataRoot)
			return nil, err
		}
		created = append(created, name)
	}
	return created, nil
}

// Unlink removes symlinks created by LinkProfile. Anything that is no longer
// a symlink into dataRoot is left alone.
func Unlink(gamePath string, names []string, dataRoot string) error {
	var errs []error
	for _, name := range names {
		if filepath.Base(name) != name {
			continue
		}
		if err := removeOurLink(filepath.Join(gamePath, name), dataRoot); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func isOurLink(path, dataRoot string) bool {
	dest, err := os.Readlink(path)
	return err == nil && strings.HasPrefix(dest, filepath.Clean(dataRoot)+string(filepath.Separator))
}

func removeOurLink(path, dataRoot string) error {
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if fi.Mode()&fs.ModeSymlink == 0 || !isOurLink(path, dataRoot) {
		return nil
	}
	return os.Remove(path)
}
