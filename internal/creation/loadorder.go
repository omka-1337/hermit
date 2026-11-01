package creation

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"hermit/internal/deploy"
)

// Header is what a plugin's file header says about it.
type Header struct {
	// Master is set for a master file, ESM, which the game loads before the
	// plugins that are not, whatever the order the list gives them.
	Master bool
	// Light is set for a light plugin, ESL, which takes no index of its own
	// but a slot in the shared FE index.
	Light bool
	// Masters are the plugins it needs loaded before it.
	Masters []string
}

// recordHeader is the size of the header of a record from Skyrim Special
// Edition on: type, size, flags, form id and version control.
const recordHeader = 24

// ReadHeader reads the TES4 record a plugin starts with. A plugin named .esm
// is a master and one named .esl light and a master, as the game takes
// them, whatever their header says.
func ReadHeader(file string, g Game) (Header, error) {
	f, err := os.Open(file)
	if err != nil {
		return Header{}, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	head := make([]byte, recordHeader)
	if _, err := io.ReadFull(r, head); err != nil {
		return Header{}, fmt.Errorf("%s: %w", filepath.Base(file), err)
	}
	if string(head[:4]) != "TES4" {
		return Header{}, fmt.Errorf("%s is not a plugin of this game", filepath.Base(file))
	}
	size := binary.LittleEndian.Uint32(head[4:8])
	flags := binary.LittleEndian.Uint32(head[8:12])
	if size > 1<<20 {
		return Header{}, fmt.Errorf("%s has a header of %d bytes", filepath.Base(file), size)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return Header{}, fmt.Errorf("%s: %w", filepath.Base(file), err)
	}

	ext := strings.ToLower(filepath.Ext(file))
	h := Header{Master: flags&0x1 != 0 || ext == ".esm" || ext == ".esl", Light: flags&lightFlag(g) != 0 || ext == ".esl"}
	// Fields: a type, a 16-bit size and the data, MAST holding the name of a
	// master as a zero-terminated string.
	for len(data) >= 6 {
		kind, n := string(data[:4]), int(binary.LittleEndian.Uint16(data[4:6]))
		if 6+n > len(data) {
			break
		}
		if kind == "MAST" {
			if name, _, _ := bytes.Cut(data[6:6+n], []byte{0}); len(name) > 0 {
				h.Masters = append(h.Masters, string(name))
			}
		}
		data = data[6+n:]
	}
	return h, nil
}

// lightFlag is the header flag of a light plugin: Starfield moved it.
func lightFlag(g Game) uint32 {
	if g.EngineVersion == "2" {
		return 0x100
	}
	return 0x200
}

// LoadOrderEntry is a plugin in the order the game loads it.
type LoadOrderEntry struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
	// Implicit is set for the game's own plugins it loads whatever the list
	// says; they cannot be switched off or moved.
	Implicit bool `json:"implicit"`
	Master   bool `json:"master"`
	Light    bool `json:"light"`
	// Mod is the id of the mod the plugin comes from, "" for the game's own
	// and OverwriteMod for the profile's overwrite folder.
	Mod string `json:"mod"`
	// Index is how the game numbers the plugin in form ids, "0A" or "FE:003"
	// for a light one; empty while it is off.
	Index   string   `json:"index"`
	Masters []string `json:"masters"`
	// Missing are the masters not loaded before it, without which the game
	// will not start.
	Missing []string `json:"missing"`
	// Error says why the plugin's header could not be read.
	Error string `json:"error,omitempty"`
}

// OverwriteMod is LoadOrderEntry.Mod for a plugin in the overwrite folder.
const OverwriteMod = deploy.OverwriteSource

// available is a plugin that will be in Data while the profile is played,
// with where it comes from.
type available struct {
	name, file, mod string
}

// LoadOrder lists the plugins the game will load with a profile, mods being
// the ids of its enabled mods from lowest to highest priority: the game's
// implicit plugins first, then those of the game and the mods in the order
// of the profile's plugin list, masters before the rest as the game sorts
// them. A plugin the list does not have yet comes last, on, as Deploy adds
// it.
func LoadOrder(g Game, gamePath, profileDir string, mods []string) ([]LoadOrderEntry, error) {
	implicit, plugins, err := present(g, gamePath, profileDir, mods)
	if err != nil {
		return nil, err
	}
	stored, err := ReadPlugins(filepath.Join(profileDir, PluginsFile))
	if err != nil {
		return nil, err
	}
	list := merge(stored, plugins)

	var out []LoadOrderEntry
	for _, p := range implicit {
		out = append(out, entry(g, p, true, true))
	}
	var masters, rest []LoadOrderEntry
	for _, p := range list {
		i := slices.IndexFunc(plugins, func(a available) bool { return strings.EqualFold(a.name, p.Name) })
		if i < 0 {
			continue // its mod is off or gone; the list keeps its place
		}
		e := entry(g, plugins[i], p.Active, false)
		if e.Master {
			masters = append(masters, e)
		} else {
			rest = append(rest, e)
		}
	}
	out = slices.Concat(out, masters, rest)
	number(out)
	return out, nil
}

func entry(g Game, p available, active, implicit bool) LoadOrderEntry {
	e := LoadOrderEntry{Name: p.name, Active: active, Implicit: implicit, Mod: p.mod, Masters: []string{}, Missing: []string{}}
	h, err := ReadHeader(p.file, g)
	if err != nil {
		e.Error = err.Error()
		return e
	}
	e.Master, e.Light = h.Master, h.Light
	if h.Masters != nil {
		e.Masters = h.Masters
	}
	return e
}

// number gives the active plugins their index and finds the masters each
// misses.
func number(list []LoadOrderEntry) {
	full, light := 0, 0
	loaded := map[string]bool{}
	for i := range list {
		e := &list[i]
		if !e.Active {
			continue
		}
		for _, m := range e.Masters {
			if !loaded[strings.ToLower(m)] {
				e.Missing = append(e.Missing, m)
			}
		}
		loaded[strings.ToLower(e.Name)] = true
		if e.Light {
			e.Index = fmt.Sprintf("FE:%03X", light)
			light++
		} else {
			e.Index = fmt.Sprintf("%02X", full)
			full++
		}
	}
}

// present finds the plugins that will be in Data: the game's implicit ones
// in the order it loads them, and the others of the game folder, of the
// mods and of the overwrite folder, a later one taking the place of an
// earlier one by the same name.
func present(g Game, gamePath, profileDir string, mods []string) (implicit, plugins []available, err error) {
	dataDir, err := deploy.DataDir(gamePath, Target)
	if err != nil {
		return nil, nil, err
	}
	add := func(dir, mod string) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() || !isPlugin(e.Name()) {
				continue
			}
			p := available{name: e.Name(), file: filepath.Join(dir, e.Name()), mod: mod}
			if i := slices.IndexFunc(plugins, func(a available) bool { return strings.EqualFold(a.name, p.name) }); i >= 0 {
				plugins[i] = p
			} else {
				plugins = append(plugins, p)
			}
		}
	}
	add(dataDir, "")
	for _, id := range mods {
		add(filepath.Join(profileDir, deploy.ModsDir, id), id)
	}
	add(filepath.Join(profileDir, deploy.OverwriteDir), OverwriteMod)

	for _, name := range implicitNames(g, gamePath) {
		if i := slices.IndexFunc(plugins, func(a available) bool { return strings.EqualFold(a.name, name) }); i >= 0 {
			implicit = append(implicit, plugins[i])
			plugins = slices.Delete(plugins, i, i+1)
		}
	}
	return implicit, plugins, nil
}

// implicitNames are the plugins the game loads first, its own and those the
// Creation Club list in its folder names.
func implicitNames(g Game, gamePath string) []string {
	names := slices.Clone(g.Implicit)
	if g.CCC == "" {
		return names
	}
	file, _ := deploy.FindFold(gamePath, g.CCC)
	if file == "" {
		return names
	}
	ccc, err := ReadPlugins(filepath.Join(gamePath, file))
	if err != nil {
		return names
	}
	for _, p := range ccc {
		names = append(names, p.Name)
	}
	return names
}

// merge is the profile's plugin list with the plugins it does not have yet
// at its end, on.
func merge(stored []Plugin, plugins []available) []Plugin {
	list := slices.Clone(stored)
	for _, p := range plugins {
		if !slices.ContainsFunc(list, func(s Plugin) bool { return strings.EqualFold(s.Name, p.name) }) {
			list = append(list, Plugin{Name: p.name, Active: true})
		}
	}
	return list
}

// ErrImplicit refuses a change to a plugin the game loads by itself.
var ErrImplicit = errors.New("the game loads this plugin by itself; it cannot be switched off or moved")

// SetPluginActive switches a plugin of the profile's list on or off.
func SetPluginActive(g Game, gamePath, profileDir string, mods []string, name string, active bool) error {
	list, err := editable(g, gamePath, profileDir, mods, name)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(list, func(p Plugin) bool { return strings.EqualFold(p.Name, name) })
	if i < 0 {
		return fmt.Errorf("plugin %s is not in the profile", name)
	}
	list[i].Active = active
	return WritePlugins(filepath.Join(profileDir, PluginsFile), list)
}

// MovePlugins moves plugins, in the order they have among themselves, to
// just before the plugin before, or to the end when before is empty; before
// one of the game's implicit plugins means to the start. Plugins the list
// keeps for mods that are off keep their places among the rest.
func MovePlugins(g Game, gamePath, profileDir string, mods []string, names []string, before string) error {
	list, err := editable(g, gamePath, profileDir, mods, names...)
	if err != nil {
		return err
	}
	is := func(p Plugin, names ...string) bool {
		return slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, p.Name) })
	}
	var picked, rest []Plugin
	for _, p := range list {
		if is(p, names...) {
			picked = append(picked, p)
		} else {
			rest = append(rest, p)
		}
	}
	if len(picked) != len(names) {
		return fmt.Errorf("plugins %v are not all in the profile", names)
	}
	at := len(rest)
	if before != "" {
		if at = slices.IndexFunc(rest, func(p Plugin) bool { return is(p, before) }); at < 0 {
			at = 0
		}
	}
	return WritePlugins(filepath.Join(profileDir, PluginsFile), slices.Concat(rest[:at], picked, rest[at:]))
}

// editable is the profile's plugin list with the plugins present it does
// not have yet, refusing changes to the game's implicit plugins.
func editable(g Game, gamePath, profileDir string, mods []string, names ...string) ([]Plugin, error) {
	implicit, plugins, err := present(g, gamePath, profileDir, mods)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if slices.ContainsFunc(implicit, func(a available) bool { return strings.EqualFold(a.name, name) }) {
			return nil, ErrImplicit
		}
	}
	stored, err := ReadPlugins(filepath.Join(profileDir, PluginsFile))
	if err != nil {
		return nil, err
	}
	return merge(stored, plugins), nil
}
