package launch

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PortProton is a Flatpak launcher for Windows games outside Steam. It has no
// field for a command prefix, so Hermit starts it itself, wrapped in "run".
//
// One more thing is needed. PortProton drops WINEDLLOVERRIDES from the
// environment when it starts and takes it from its settings file for the
// game, "<game>.exe.ppdb", which it keeps next to the executable. BepInEx only
// loads if Wine prefers the winhttp.dll linked into the game folder, so for
// the length of a session that file carries winhttp=n,b as well: added in
// Prepare, taken out again in Cleanup, and nothing else in it is touched.
const PortProtonApp = "ru.linux_gaming.PortProton"

// portProtonInstalls are where Flatpak puts PortProton, system-wide and for
// the user.
var portProtonInstalls = []string{
	"/var/lib/flatpak/app/" + PortProtonApp,
	"~/.local/share/flatpak/app/" + PortProtonApp,
}

// PortProtonInstalled reports whether the PortProton Flatpak is installed.
func PortProtonInstalled() bool {
	home, _ := os.UserHomeDir()
	for _, dir := range portProtonInstalls {
		if strings.HasPrefix(dir, "~/") {
			if home == "" {
				continue
			}
			dir = filepath.Join(home, dir[2:])
		}
		if _, err := os.Stat(dir); err == nil {
			return true
		}
	}
	return false
}

// PortProtonCommand is how PortProton starts a game: the executable is all it
// needs, the rest comes from its settings file.
func PortProtonCommand(exe string) []string {
	return []string{"flatpak", "run", PortProtonApp, exe}
}

// PortProtonSettings returns the settings file PortProton keeps for a game.
func PortProtonSettings(exe string) string { return exe + ".ppdb" }

// portProtonExe recognises a command built by PortProtonCommand and returns
// the game executable it starts.
func portProtonExe(command []string) (string, bool) {
	for i := 0; i+1 < len(command); i++ {
		if command[i] == PortProtonApp {
			return command[len(command)-1], true
		}
	}
	return "", false
}

var overridesLine = regexp.MustCompile(`^(\s*export\s+WINEDLLOVERRIDES=)(.*)$`)

// addPPDBOverride makes the settings file load dll with the given mode and
// reports whether it had to change anything. An override for dll that is
// already there is the user's and is left alone. A missing file is not an
// error: PortProton writes it on the first launch of a game.
func addPPDBOverride(path, dll, mode string) (added bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	lines := strings.Split(string(data), "\n")
	token := dll + "=" + mode
	for i, line := range lines {
		m := overridesLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		value := unquoteShell(m[2])
		if overridesDLL(value, dll) {
			return false, nil
		}
		if value == "" {
			value = token
		} else {
			value += ";" + token
		}
		lines[i] = m[1] + `"` + value + `"`
		return true, writeKeepingMode(path, strings.Join(lines, "\n"))
	}
	// No overrides line at all: add one.
	text := strings.TrimRight(string(data), "\n") + "\nexport WINEDLLOVERRIDES=\"" + token + "\"\n"
	return true, writeKeepingMode(path, text)
}

// removePPDBOverride takes the entry addPPDBOverride added back out.
func removePPDBOverride(path, dll, mode string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	token := dll + "=" + mode
	for i, line := range lines {
		m := overridesLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var kept []string
		for _, entry := range strings.Split(unquoteShell(m[2]), ";") {
			if entry != "" && !strings.EqualFold(entry, token) {
				kept = append(kept, entry)
			}
		}
		lines[i] = m[1] + `"` + strings.Join(kept, ";") + `"`
		return writeKeepingMode(path, strings.Join(lines, "\n"))
	}
	return nil
}

// overridesDLL reports whether a WINEDLLOVERRIDES value mentions dll: entries
// are separated by ';' and name one or more DLLs, comma separated, before '='.
func overridesDLL(value, dll string) bool {
	for _, entry := range strings.Split(value, ";") {
		names, _, _ := strings.Cut(entry, "=")
		for _, name := range strings.Split(names, ",") {
			if strings.EqualFold(strings.TrimSpace(name), dll) {
				return true
			}
		}
	}
	return false
}

func unquoteShell(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
		return value[1 : len(value)-1]
	}
	return value
}

// writeKeepingMode replaces a file atomically without changing its
// permissions; PortProton rewrites the same file with sed.
func writeKeepingMode(path, text string) error {
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp := path + ".hermit-tmp"
	if err := os.WriteFile(tmp, []byte(text), mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
