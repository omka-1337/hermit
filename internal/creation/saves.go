package creation

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"hermit/internal/deploy"
)

// SavesSwap is the game's Saves folder a profile's took the place of for a
// session: Link is where the game looks for its saves, a link into the
// profile while the session lasts, and Original where the game's own
// folder waits meanwhile, "" when it had none.
type SavesSwap struct {
	Link     string `json:"link"`
	Original string `json:"original,omitempty"`
}

// swapSaves has the game keep its saves in the profile for the session: the
// game's Saves folder is set aside and a link to the profile's saves folder
// takes its place, as MO2 gives a profile saves of its own.
func (s *State) swapSaves(profileDir, saves string) error {
	mine := filepath.Join(profileDir, SavesDir)
	if err := os.MkdirAll(mine, 0o755); err != nil {
		return err
	}
	swap := &SavesSwap{Link: saves}
	fi, err := os.Lstat(saves)
	switch {
	case err == nil && fi.Mode()&fs.ModeSymlink != 0:
		// Left by a session that did not finish.
		if err := os.Remove(saves); err != nil {
			return err
		}
	case err == nil:
		swap.Original = saves + deploy.OriginalSuffix
		if _, err := os.Lstat(swap.Original); err == nil {
			return fmt.Errorf("%s is there already; put it back as %s if it holds the game's saves", swap.Original, filepath.Base(saves))
		}
		if err := os.Rename(saves, swap.Original); err != nil {
			return err
		}
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	if err := os.MkdirAll(filepath.Dir(saves), 0o755); err != nil {
		return err
	}
	s.Saves = swap
	return os.Symlink(mine, saves)
}

// restoreSaves puts the game's own Saves folder back.
func (s *State) restoreSaves() error {
	if s.Saves == nil {
		return nil
	}
	if fi, err := os.Lstat(s.Saves.Link); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
		if err := os.Remove(s.Saves.Link); err != nil {
			return err
		}
	}
	if s.Saves.Original == "" {
		return nil
	}
	return os.Rename(s.Saves.Original, s.Saves.Link)
}
