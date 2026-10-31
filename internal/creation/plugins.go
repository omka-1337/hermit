package creation

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// byteOrderMark starts files saved by some Windows editors.
const byteOrderMark = string(rune(0xFEFF))

// Plugin is a line of plugins.txt.
type Plugin struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// ReadPlugins reads a plugin list in the format of the game's plugins.txt:
// one file name a line, active ones marked with "*", "#" starting comments.
// A missing file is an empty list.
func ReadPlugins(file string) ([]Plugin, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Plugin
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, byteOrderMark))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, active := strings.CutPrefix(line, "*")
		list = append(list, Plugin{Name: strings.TrimSpace(name), Active: active})
	}
	return list, nil
}

// WritePlugins writes a plugin list as ReadPlugins reads it, with the line
// endings the game writes itself.
func WritePlugins(file string, list []Plugin) error {
	var b strings.Builder
	b.WriteString("# This file is used by the game to keep track of your downloaded content.\r\n")
	b.WriteString("# Written by Hermit for the profile being played.\r\n")
	for _, p := range list {
		if p.Active {
			b.WriteByte('*')
		}
		b.WriteString(p.Name)
		b.WriteString("\r\n")
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, []byte(b.String()), 0o644)
}

// PrefixPluginsPath is where a game running in the Wine prefix of a Proton
// compatibility data folder (compatdata/<appid>) reads plugins.txt from.
func PrefixPluginsPath(compatData string, g Game) string {
	dir := filepath.Join(prefixUser(compatData), "AppData", "Local", g.AppData)
	if name, err := findFold(dir, PluginsFile); err == nil && name != "" {
		return filepath.Join(dir, name)
	}
	return filepath.Join(dir, "Plugins.txt")
}

// swapPlugins adds the deployed plugins the profile's list does not have
// yet, active, at its end, and writes the list to pluginsPath for the
// session, keeping what was there in s. Plugins the list has but nothing
// provides stay in the profile's list, so a mod disabled for a while keeps
// its place, but the game is not given them.
func (s *State) swapPlugins(profileDir, dataDir string, deployed []string, pluginsPath string) error {
	listFile := filepath.Join(profileDir, PluginsFile)
	list, err := ReadPlugins(listFile)
	if err != nil {
		return err
	}
	has := func(name string) bool {
		return slices.ContainsFunc(list, func(p Plugin) bool { return strings.EqualFold(p.Name, name) })
	}
	changed := false
	for _, name := range deployed {
		if !has(name) {
			list = append(list, Plugin{Name: name, Active: true})
			changed = true
		}
	}
	if changed {
		if err := WritePlugins(listFile, list); err != nil {
			return err
		}
	}

	// The game's own plugins are in Data as files of their own.
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return err
	}
	present := func(name string) bool {
		return slices.ContainsFunc(deployed, func(d string) bool { return strings.EqualFold(d, name) }) ||
			slices.ContainsFunc(entries, func(e fs.DirEntry) bool { return strings.EqualFold(e.Name(), name) })
	}
	var given []Plugin
	for _, p := range list {
		if present(p.Name) {
			given = append(given, p)
		}
	}

	original, err := os.ReadFile(pluginsPath)
	switch {
	case err == nil:
		text := string(original)
		s.OriginalPlugins = &text
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}
	s.PluginsPath = pluginsPath
	return WritePlugins(pluginsPath, given)
}
