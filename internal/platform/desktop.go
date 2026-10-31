package platform

import (
	"bytes"
	"os"
	"path/filepath"
)

// DesktopFile is Hermit's own desktop entry. A Wayland compositor shows a
// window's icon from the desktop entry named after the window's app id,
// which for Hermit is the name of its executable, hermit; packages install
// this entry, an AppImage or a build of one's own does not.
const DesktopFile = "hermit.desktop"

// IconName is the name of Hermit's icon in the icon theme.
const IconName = "hermit"

// InstallDesktopEntry puts Hermit's desktop entry, starting exe, and its
// icon into the user's data folder dataHome, unless one of the system's
// data folders dataDirs has the entry already, as a package installs it.
// Files that are already as they should be are left alone, so it costs
// nothing after the first start; it reports whether it wrote anything.
func InstallDesktopEntry(dataHome string, dataDirs []string, exe string, icon []byte) (bool, error) {
	if os.Getenv("FLATPAK_ID") != "" {
		return false, nil
	}
	for _, dir := range dataDirs {
		if _, err := os.Stat(filepath.Join(dir, "applications", DesktopFile)); err == nil {
			return false, nil
		}
	}
	entry := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=Hermit\n" +
		"Comment=Mod manager for Linux\n" +
		"Exec=" + desktopQuote(exe) + "\n" +
		"Icon=" + IconName + "\n" +
		"Categories=Game;Utility;\n" +
		"StartupWMClass=hermit\n" +
		"Terminal=false\n"
	wrote := false
	for _, f := range []struct {
		path string
		data []byte
	}{
		{filepath.Join(dataHome, "icons", "hicolor", "512x512", "apps", IconName+".png"), icon},
		{filepath.Join(dataHome, "applications", DesktopFile), []byte(entry)},
	} {
		if old, err := os.ReadFile(f.path); err == nil && bytes.Equal(old, f.data) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			return wrote, err
		}
		if err := os.WriteFile(f.path, f.data, 0o644); err != nil {
			return wrote, err
		}
		wrote = true
	}
	if wrote {
		// KDE finds desktop entries through a cache of its own.
		Command("update-desktop-database", filepath.Join(dataHome, "applications")).Run()
		for _, tool := range []string{"kbuildsycoca6", "kbuildsycoca5"} {
			if Command(tool).Run() == nil {
				break
			}
		}
		// Plasma remembers that there was no icon named hermit from the
		// windows of Hermit it saw before; this has KDE look icons up again.
		Command("dbus-send", "--session", "--type=signal", "/KIconLoader", "org.kde.KIconLoader.iconChanged", "int32:0").Run()
	}
	return wrote, nil
}
