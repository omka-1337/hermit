package creation

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"hermit/internal/deploy"
)

// INISwap is an INI file of the game a profile's took the place of.
type INISwap struct {
	// Profile is the file in the profile's ini folder.
	Profile string `json:"profile"`
	// Game is the game's file, which held Original, nil when there was none.
	Game     string  `json:"game"`
	Original *string `json:"original,omitempty"`
}

// swapINIs puts the profile's INI files in the game's My Games folder for the
// session, keeping the game's own in s. A profile without INI files of its
// own plays with the game's.
func (s *State) swapINIs(profileDir, myGames string) error {
	entries, err := os.ReadDir(filepath.Join(profileDir, INIDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(myGames, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		mine := filepath.Join(profileDir, INIDir, e.Name())
		data, err := os.ReadFile(mine)
		if err != nil {
			return err
		}
		// The game's file in whatever letter case it has.
		name, err := deploy.FindFold(myGames, e.Name())
		if err != nil {
			return err
		}
		if name == "" {
			name = e.Name()
		}
		swap := INISwap{Profile: mine, Game: filepath.Join(myGames, name)}
		original, err := os.ReadFile(swap.Game)
		switch {
		case err == nil:
			text := string(original)
			swap.Original = &text
		case !errors.Is(err, fs.ErrNotExist):
			return err
		}
		if err := os.WriteFile(swap.Game, data, 0o644); err != nil {
			return err
		}
		s.INIs = append(s.INIs, swap)
	}
	return nil
}

// restoreINIs takes back what the game changed in the profile's INI files,
// settings changed in its menus, and puts the game's own files back.
func (s *State) restoreINIs() error {
	var errs []error
	for _, swap := range s.INIs {
		if data, err := os.ReadFile(swap.Game); err == nil {
			errs = append(errs, os.WriteFile(swap.Profile, data, 0o644))
		}
		if swap.Original != nil {
			errs = append(errs, os.WriteFile(swap.Game, []byte(*swap.Original), 0o644))
		} else if err := os.Remove(swap.Game); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
