package bethesda

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
)

// notDeployed are entries at the top of a mod folder that belong to the mod
// manager that installed it, not to the game.
var notDeployed = []string{"meta.ini", "fomod"}

// State is what Deploy changed, kept until Undeploy puts things back.
type State struct {
	// ProfileDir is the profile the links point into.
	ProfileDir string `json:"profileDir"`
	// Links are the symlinks made, relative to the game folder.
	Links []string `json:"links"`
	// Existing are the lower-case paths, relative to Data, of what Data held
	// before the session, folders with a trailing slash. Anything else found
	// there afterwards was written during the session.
	Existing []string `json:"existing"`
	// Hidden are the files of mods not linked because the game folder has a
	// file of its own by that name, relative to the game folder.
	Hidden []string `json:"hidden,omitempty"`
	// PluginsPath is the plugins.txt the game reads, replaced for the
	// session; OriginalPlugins is what it held, nil when there was none.
	PluginsPath     string  `json:"pluginsPath,omitempty"`
	OriginalPlugins *string `json:"originalPlugins,omitempty"`
	// INIs are the game's INI files the profile's replaced for the session.
	INIs []INISwap `json:"inis,omitempty"`
}

// source is the file of a mod that wins a path.
type source struct {
	file string // absolute path of the mod's file
	rel  string // slash-separated path from Data or the game folder, as the mod spells it
	root bool   // goes into the game folder rather than Data
}

// Deploy links the files of mods, the ids of the profile's enabled mods from
// lowest to highest priority, into the game at gamePath, followed by the
// profile's overwrite folder. Where two mods have the same file, the later
// one wins; where the game has a file of its own, the game's stays. The
// profile's plugin list is brought up to date with the plugins deployed and
// given to the game, and its INI files, if it has them, take the place of
// the game's, in the places prefix names.
func Deploy(gamePath, profileDir string, mods []string, prefix Prefix) (State, error) {
	dataName, err := findFold(gamePath, "Data")
	if err != nil {
		return State{}, err
	}
	if dataName == "" {
		return State{}, errors.New("the game folder has no Data folder")
	}
	dataDir := filepath.Join(gamePath, dataName)

	winners := map[string]source{}
	var plugins []string // as the mods bring them, lowest priority first
	folders := make([]string, 0, len(mods)+1)
	for _, id := range mods {
		folders = append(folders, filepath.Join(profileDir, ModsDir, id))
	}
	folders = append(folders, filepath.Join(profileDir, OverwriteDir))
	for _, folder := range folders {
		if err := collect(folder, winners, &plugins); err != nil {
			return State{}, err
		}
	}

	s := State{ProfileDir: profileDir}
	if s.Existing, err = snapshot(dataDir); err != nil {
		return State{}, err
	}
	trees := map[bool]*tree{false: newTree(dataDir), true: newTree(gamePath)}
	keys := slices.Sorted(maps.Keys(winners))
	for _, key := range keys {
		src := winners[key]
		t := trees[src.root]
		target, exists, err := t.place(src.rel)
		if err != nil {
			Undeploy(gamePath, s)
			return State{}, err
		}
		rel, _ := filepath.Rel(gamePath, target)
		if exists {
			if !isLinkInto(target, profileDir) {
				s.Hidden = append(s.Hidden, filepath.ToSlash(rel))
				continue
			}
			// Left by a session that did not finish.
			if err := os.Remove(target); err != nil {
				Undeploy(gamePath, s)
				return State{}, err
			}
		}
		if err := os.Symlink(src.file, target); err != nil {
			Undeploy(gamePath, s)
			return State{}, err
		}
		s.Links = append(s.Links, rel)
	}

	if prefix.Plugins != "" {
		if err := s.swapPlugins(profileDir, dataDir, plugins, prefix.Plugins); err != nil {
			Undeploy(gamePath, s)
			return State{}, err
		}
	}
	if prefix.MyGames != "" {
		if err := s.swapINIs(profileDir, prefix.MyGames); err != nil {
			Undeploy(gamePath, s)
			return State{}, err
		}
	}
	return s, nil
}

// Undeploy removes the links Deploy made, moves the files written into Data
// during the session into the profile's overwrite folder, removes folders
// that were not there before and puts the original plugins.txt back, and the
// game's INI files, keeping what the game changed in the profile's. It
// returns the moved files, relative to Data.
func Undeploy(gamePath string, s State) ([]string, error) {
	profileDir := s.ProfileDir
	var errs []error
	for _, rel := range s.Links {
		target := filepath.Join(gamePath, rel)
		if !isLinkInto(target, profileDir) {
			continue // replaced during the session; captured below if in Data
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	var captured []string
	dataName, err := findFold(gamePath, "Data")
	if err == nil && dataName != "" {
		captured, err = capture(filepath.Join(gamePath, dataName), filepath.Join(profileDir, OverwriteDir), s.Existing)
	}
	errs = append(errs, err)

	errs = append(errs, s.restoreINIs())
	if s.PluginsPath != "" {
		if s.OriginalPlugins != nil {
			errs = append(errs, os.WriteFile(s.PluginsPath, []byte(*s.OriginalPlugins), 0o644))
		} else if err := os.Remove(s.PluginsPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return captured, errors.Join(errs...)
}

// collect records the files of a mod folder in winners, replacing those of
// mods before it, and adds the plugins it has that are not in plugins yet. A
// missing folder has nothing to add.
func collect(folder string, winners map[string]source, plugins *[]string) error {
	return walkMod(folder, func(key string, src source) {
		winners[key] = src
		if !src.root && isPlugin(src.rel) && !slices.ContainsFunc(*plugins, func(p string) bool { return strings.EqualFold(p, src.rel) }) {
			*plugins = append(*plugins, src.rel)
		}
	})
}

// walkMod calls fn for each file of a mod folder that is deployed, with the
// key two mods having the same file share: its lower-case path in Data, or
// in the game folder after a "/". A missing folder has no files.
func walkMod(folder string, fn func(key string, src source)) error {
	err := filepath.WalkDir(folder, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(folder, p)
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if !strings.Contains(rel, "/") && slices.Contains(notDeployed, strings.ToLower(rel)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		src := source{file: p, rel: rel}
		if first, rest, ok := strings.Cut(rel, "/"); ok && strings.EqualFold(first, rootDir) {
			src.rel, src.root = rest, true
		}
		key := strings.ToLower(src.rel)
		if src.root {
			key = "/" + key // never the same as a Data path
		}
		fn(key, src)
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// snapshot lists what Data holds, as State.Existing.
func snapshot(dataDir string) ([]string, error) {
	var existing []string
	err := filepath.WalkDir(dataDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dataDir, p)
		if rel == "." {
			return nil
		}
		rel = strings.ToLower(filepath.ToSlash(rel))
		if d.IsDir() {
			rel += "/"
		}
		existing = append(existing, rel)
		return nil
	})
	sort.Strings(existing)
	return existing, err
}

// capture moves the files in dataDir that are not in existing into
// overwrite, and removes the folders not in existing that are left empty.
func capture(dataDir, overwrite string, existing []string) ([]string, error) {
	var files, dirs []string
	err := filepath.WalkDir(dataDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dataDir, p)
		if rel == "." {
			return nil
		}
		key := strings.ToLower(filepath.ToSlash(rel))
		if d.IsDir() {
			if _, found := slices.BinarySearch(existing, key+"/"); !found {
				dirs = append(dirs, p)
			}
			return nil
		}
		if _, found := slices.BinarySearch(existing, key); found || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	var errs []error
	errs = append(errs, err)
	var moved []string
	for _, rel := range files {
		if err := moveFile(filepath.Join(dataDir, rel), filepath.Join(overwrite, rel)); err != nil {
			errs = append(errs, err)
			continue
		}
		moved = append(moved, filepath.ToSlash(rel))
	}
	// Deepest first, so a folder's subfolders are gone before it.
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i]) // fails, as it should, while anything is left in it
	}
	return moved, errors.Join(errs...)
}

// tree places files under a folder, matching the names of the folders that
// are already there in any letter case: Wine looks names up regardless of
// case, and two folders differing only in case would split a mod's files.
type tree struct {
	base     string
	listings map[string]map[string]string // folder → lower-case name → name
}

func newTree(base string) *tree {
	return &tree{base: base, listings: map[string]map[string]string{}}
}

// place returns where the file rel goes, creating the folders on the way,
// and whether something is already there.
func (t *tree) place(rel string) (string, bool, error) {
	parts := strings.Split(rel, "/")
	dir := t.base
	for _, part := range parts[:len(parts)-1] {
		name, err := t.lookup(dir, part)
		if err != nil {
			return "", false, err
		}
		if name == "" {
			if err := os.Mkdir(filepath.Join(dir, part), 0o755); err != nil {
				return "", false, err
			}
			t.listings[dir][strings.ToLower(part)] = part
			t.listings[filepath.Join(dir, part)] = map[string]string{}
			name = part
		}
		dir = filepath.Join(dir, name)
	}
	file := parts[len(parts)-1]
	name, err := t.lookup(dir, file)
	if err != nil {
		return "", false, err
	}
	if name != "" {
		return filepath.Join(dir, name), true, nil
	}
	t.listings[dir][strings.ToLower(file)] = file
	return filepath.Join(dir, file), false, nil
}

func (t *tree) lookup(dir, name string) (string, error) {
	listing, ok := t.listings[dir]
	if !ok {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", err
		}
		listing = make(map[string]string, len(entries))
		for _, e := range entries {
			listing[strings.ToLower(e.Name())] = e.Name()
		}
		t.listings[dir] = listing
	}
	return listing[strings.ToLower(name)], nil
}

// findFold returns the name of the entry of dir called name in any letter
// case, or "" when there is none.
func findFold(dir, name string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			return e.Name(), nil
		}
	}
	return "", nil
}

// isLinkInto reports whether p is a symlink pointing into dir.
func isLinkInto(p, dir string) bool {
	dest, err := os.Readlink(p)
	return err == nil && strings.HasPrefix(dest, filepath.Clean(dir)+string(filepath.Separator))
}

// moveFile renames a file, or copies and removes it when the two places are
// on different file systems; what is at to already is replaced.
func moveFile(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	err := os.Rename(from, to)
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copying %s: %w", from, err)
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(from)
}

// isPlugin reports whether a Data path is a plugin the game loads.
func isPlugin(rel string) bool {
	if strings.Contains(rel, "/") {
		return false
	}
	switch strings.ToLower(path.Ext(rel)) {
	case ".esp", ".esm", ".esl":
		return true
	}
	return false
}
