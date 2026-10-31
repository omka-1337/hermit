package app

import (
	"github.com/adrg/xdg"

	"hermit/internal/platform"
)

// InstallDesktopEntry gives Hermit's window its icon on Wayland, where it
// comes from Hermit's desktop entry, by putting the entry and the icon into
// the user's data folder when no package has installed them.
func InstallDesktopEntry() error {
	_, err := platform.InstallDesktopEntry(xdg.DataHome, xdg.DataDirs, executablePath(), icon)
	return err
}
