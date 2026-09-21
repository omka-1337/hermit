package app

import (
	"fmt"

	"hermit/internal/library"
	"hermit/internal/platform"
)

// FolderService opens Hermit's folders in the desktop's file manager, for
// looking at a profile's files or dropping something in by hand.
type FolderService struct {
	lib *library.Library
}

func NewFolderService(lib *library.Library) *FolderService {
	return &FolderService{lib: lib}
}

// OpenProfile shows a profile's folder: the mods, configs and BepInEx files
// that are linked into the game while it runs.
func (s *FolderService) OpenProfile(gameID, profileID string) error {
	dir, err := s.lib.ProfileDir(gameID, profileID)
	if err != nil {
		return err
	}
	// platform.Command gives xdg-open the desktop's environment, not the one
	// an AppImage sets up for Hermit's own libraries.
	cmd := platform.Command("xdg-open", dir)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening %s: %w", dir, err)
	}
	// Reap it once it hands the folder over, or it lingers as a zombie.
	go cmd.Wait()
	return nil
}
