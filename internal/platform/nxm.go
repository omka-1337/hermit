package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// NXMDesktopFile is the desktop entry through which Hermit opens the nxm://
// links of Nexus Mods' "Mod Manager Download" buttons.
const NXMDesktopFile = "hermit-nxm.desktop"

// NXMHandler names the desktop entry that opens nxm:// links now, "" when
// none does.
func NXMHandler() string {
	out, err := Command("xdg-mime", "query", "default", "x-scheme-handler/nxm").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// RegisterNXM makes Hermit open nxm:// links: a desktop entry of its own in
// the user's applications folder, set as the default for the scheme. The
// entry runs exe with the link.
func RegisterNXM(applications, exe string) error {
	if os.Getenv("FLATPAK_ID") != "" {
		return errors.New("Hermit runs as a Flatpak here; the Flatpak sets up opening nxm:// links itself")
	}
	entry := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=Hermit (Nexus Mods downloads)\n" +
		"Exec=" + desktopQuote(exe) + " %u\n" +
		"MimeType=x-scheme-handler/nxm;\n" +
		"NoDisplay=true\n" +
		"Terminal=false\n"
	if err := os.MkdirAll(applications, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(applications, NXMDesktopFile), []byte(entry), 0o644); err != nil {
		return err
	}
	if out, err := Command("xdg-mime", "default", NXMDesktopFile, "x-scheme-handler/nxm").CombinedOutput(); err != nil {
		return errors.New("xdg-mime: " + strings.TrimSpace(string(out)) + " " + err.Error())
	}
	// Some desktops read the scheme from caches these update, KDE from its
	// own; it is fine when the tools are missing.
	Command("update-desktop-database", applications).Run()
	for _, tool := range []string{"kbuildsycoca6", "kbuildsycoca5"} {
		if Command(tool).Run() == nil {
			break
		}
	}
	return nil
}

// desktopQuote quotes a path for the Exec key of a desktop entry.
func desktopQuote(s string) string {
	if !strings.ContainsAny(s, " \t\n\"'\\><~|&;$*?#()`") {
		return s
	}
	r := strings.NewReplacer(`\`, `\\\\`, `"`, `\\"`, "`", "\\\\`", "$", `\\$`)
	return `"` + r.Replace(s) + `"`
}
