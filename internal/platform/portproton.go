package platform

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// PortProtonPrefix is the Wine prefix PortProton runs exe in: the one its
// settings file for the game, "<exe>.ppdb" next to it, names, DEFAULT when it
// names none; "" when PortProton has no such prefix.
func PortProtonPrefix(exe string) string {
	name := "DEFAULT"
	if f, err := os.Open(exe + ".ppdb"); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sc.Text()), "export "))
			if v, ok := strings.CutPrefix(line, "PW_PREFIX_NAME="); ok {
				if v = strings.Trim(v, `"'`); v != "" && !strings.ContainsAny(v, "/\\") {
					name = v
				}
			}
		}
		f.Close()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	// PortProton keeps its prefixes in ~/PortProton/prefixes, a link into
	// its data, the Flatpak's or that of an install of its own.
	for _, prefixes := range []string{
		filepath.Join(home, "PortProton", "prefixes"),
		filepath.Join(home, "PortProton", "data", "prefixes"),
		filepath.Join(home, ".var", "app", "ru.linux_gaming.PortProton", "data", "prefixes"),
	} {
		dir := filepath.Join(prefixes, name)
		if fi, err := os.Stat(filepath.Join(dir, "drive_c")); err == nil && fi.IsDir() {
			return dir
		}
	}
	return ""
}
