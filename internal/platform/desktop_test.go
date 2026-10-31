package platform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallDesktopEntry(t *testing.T) {
	// No update-desktop-database or kbuildsycoca6 of the system's.
	t.Setenv("PATH", "")
	t.Setenv("FLATPAK_ID", "")
	home, system := t.TempDir(), t.TempDir()
	wrote, err := InstallDesktopEntry(home, []string{system}, "/opt/My Games/Hermit.AppImage", []byte("png"))
	if err != nil || !wrote {
		t.Fatalf("wrote %v, %v", wrote, err)
	}
	entry, _ := os.ReadFile(filepath.Join(home, "applications", DesktopFile))
	if !strings.Contains(string(entry), "Exec=\"/opt/My Games/Hermit.AppImage\"\n") || !strings.Contains(string(entry), "Icon=hermit\n") {
		t.Errorf("entry:\n%s", entry)
	}
	if icon, _ := os.ReadFile(filepath.Join(home, "icons/hicolor/512x512/apps/hermit.png")); string(icon) != "png" {
		t.Errorf("icon %q", icon)
	}
	if wrote, err := InstallDesktopEntry(home, []string{system}, "/opt/My Games/Hermit.AppImage", []byte("png")); wrote || err != nil {
		t.Errorf("wrote again: %v, %v", wrote, err)
	}

	// A package's entry is left to do the job.
	other := t.TempDir()
	os.MkdirAll(filepath.Join(system, "applications"), 0o755)
	os.WriteFile(filepath.Join(system, "applications", DesktopFile), nil, 0o644)
	if wrote, err := InstallDesktopEntry(other, []string{system}, "/usr/bin/hermit", []byte("png")); wrote || err != nil {
		t.Errorf("wrote beside the system's entry: %v, %v", wrote, err)
	}
}
