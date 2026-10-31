package deploy

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
)

// OriginalSuffix is added to the name of a game's file a mod's takes the
// place of, for the length of the session.
const OriginalSuffix = ".hermit-original"

// notDeployed are entries at the top of a mod folder that belong to the mod
// manager that installed it, not to the game.
var notDeployed = []string{"meta.ini", "fomod"}

// State is what Deploy changed, kept until Undeploy puts things back.
type State struct {
	// ProfileDir is the profile the links point into.
	ProfileDir string `json:"profileDir"`
	// Links are the symlinks made, relative to the game folder.
	Links []string `json:"links"`
	// Existing are the lower-case paths, relative to the Target's Data, of
	// what it held before the session, folders with a trailing slash.
	// Anything else found there afterwards was written during the session.
	Existing []string `json:"existing"`
	// Hidden are the files of mods not linked because the game folder has a
	// file of its own by that name, relative to the game folder.
	Hidden []string `json:"hidden,omitempty"`
	// Replaced are the game's own files set aside for the files of mods,
	// relative to the game folder; each waits beside its place under its
	// name with OriginalSuffix added.
	Replaced []string `json:"replaced,omitempty"`
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
// one wins; where the game has a file of its own, the game's stays unless
// the target lets mods replace it.
func Deploy(gamePath, profileDir string, mods []string, t Target) (State, error) {
	dataDir, err := DataDir(gamePath, t)
	if err != nil {
		return State{}, err
	}

	winners := map[string]source{}
	for _, folder := range Folders(profileDir, mods) {
		err := walkMod(folder, t, func(key string, src source) { winners[key] = src })
		if err != nil {
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
		target, exists, err := trees[src.root].place(src.rel)
		if err != nil {
			Undeploy(gamePath, t, s)
			return State{}, err
		}
		rel, _ := filepath.Rel(gamePath, target)
		if exists {
			switch {
			case isLinkInto(target, profileDir):
				// Left by a session that did not finish.
				err = os.Remove(target)
			case !t.Replace:
				s.Hidden = append(s.Hidden, filepath.ToSlash(rel))
				continue
			default:
				if _, err := os.Lstat(target + OriginalSuffix); err == nil {
					Undeploy(gamePath, t, s)
					return State{}, fmt.Errorf("%s is there already; put it back in place of %s if it is the game's own file", target+OriginalSuffix, rel)
				}
				if err = os.Rename(target, target+OriginalSuffix); err == nil {
					s.Replaced = append(s.Replaced, rel)
				}
			}
			if err != nil {
				Undeploy(gamePath, t, s)
				return State{}, err
			}
		}
		if err := os.Symlink(src.file, target); err != nil {
			Undeploy(gamePath, t, s)
			return State{}, err
		}
		s.Links = append(s.Links, rel)
	}
	return s, nil
}

// Undeploy removes the links Deploy made, puts back the game's files the
// links took the place of, moves the files written into the target during
// the session into the profile's overwrite folder and removes folders that
// were not there before. It returns the moved files, relative to the
// target's Data.
func Undeploy(gamePath string, t Target, s State) ([]string, error) {
	profileDir := s.ProfileDir
	overwrite := filepath.Join(profileDir, OverwriteDir)
	var errs []error
	for _, rel := range s.Links {
		target := filepath.Join(gamePath, rel)
		if !isLinkInto(target, profileDir) {
			continue // replaced during the session; captured below
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	dataDir, err := DataDir(gamePath, t)
	if err != nil {
		return nil, errors.Join(append(errs, err)...)
	}
	for _, rel := range s.Replaced {
		target := filepath.Join(gamePath, rel)
		if _, err := os.Lstat(target); err == nil {
			// The game wrote a file of its own in place of the mod's link:
			// it is the mod's file changed, and the profile keeps it.
			if inData, err := filepath.Rel(dataDir, target); err == nil {
				errs = append(errs, MoveFile(target, filepath.Join(overwrite, inData)))
			}
		}
		if err := os.Rename(target+OriginalSuffix, target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}

	captured, err := capture(dataDir, overwrite, s.Existing)
	errs = append(errs, err)
	return captured, errors.Join(errs...)
}

// DataDir is the folder of the game at gamePath the mods of t are laid out
// as, in whatever letter case it has.
func DataDir(gamePath string, t Target) (string, error) {
	if t.Data == "" {
		return gamePath, nil
	}
	name, err := FindFold(gamePath, t.Data)
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", fmt.Errorf("the game folder has no %s folder", t.Data)
	}
	return filepath.Join(gamePath, name), nil
}

// Folders are the folders Deploy takes files from, lowest priority first:
// those of mods, then the profile's overwrite folder.
func Folders(profileDir string, mods []string) []string {
	folders := make([]string, 0, len(mods)+1)
	for _, id := range mods {
		folders = append(folders, filepath.Join(profileDir, ModsDir, id))
	}
	return append(folders, filepath.Join(profileDir, OverwriteDir))
}

// walkMod calls fn for each file of a mod folder that is deployed, with the
// key two mods having the same file share: its lower-case path in the
// target's Data, or in the game folder after a "/". A missing folder has no
// files.
func walkMod(folder string, t Target, fn func(key string, src source)) error {
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
		if first, rest, ok := strings.Cut(rel, "/"); ok && t.Data != "" && strings.EqualFold(first, RootDir) {
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

// snapshot lists what dir holds, as State.Existing.
func snapshot(dir string) ([]string, error) {
	var existing []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
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

// capture moves the files in dir that are not in existing into overwrite,
// and removes the folders not in existing that are left empty.
func capture(dir, overwrite string, existing []string) ([]string, error) {
	var files, dirs []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
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
		if _, found := slices.BinarySearch(existing, key); found || d.Type()&fs.ModeSymlink != 0 || strings.HasSuffix(key, OriginalSuffix) {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	var errs []error
	errs = append(errs, err)
	var moved []string
	for _, rel := range files {
		if err := MoveFile(filepath.Join(dir, rel), filepath.Join(overwrite, rel)); err != nil {
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

// FindFold returns the name of the entry of dir called name in any letter
// case, or "" when there is none.
func FindFold(dir, name string) (string, error) {
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

// MoveFile renames a file, or copies and removes it when the two places are
// on different file systems; what is at to already is replaced.
func MoveFile(from, to string) error {
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
