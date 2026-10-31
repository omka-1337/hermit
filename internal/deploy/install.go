package deploy

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"hermit/internal/archive"
	"hermit/internal/fomod"
)

// Part is a file or folder of an unpacked archive and where it goes in a
// mod's folder, a slash-separated path, "" for the folder itself.
type Part struct {
	From string
	To   string
}

// Layout works out where the files of an archive unpacked into dir go in a
// mod's folder. unsure is set when nothing in it looked like the game's
// files: it is then installed as it is.
type Layout func(dir string) (parts []Part, unsure bool)

// Staged is a mod archive unpacked and looked at, ready to be installed.
type Staged struct {
	Dir string
	// File is the archive's name.
	File string
	// Fomod is the archive's installer, nil when it has none; FomodRoot is
	// the folder holding its fomod folder, the one its paths start from.
	Fomod     *fomod.Config
	FomodRoot string
	// Parts are where the files of a plain archive go.
	Parts  []Part
	Unsure bool
	// Name, Version and Author come from the installer or the file name.
	Name, Version, Author string
	NexusID               int
}

// Stage unpacks an archive into a new folder of stagingParent and works out
// with layout how it is laid out.
func Stage(file, stagingParent string, layout Layout, progress func(done, total int64)) (*Staged, error) {
	id := make([]byte, 6)
	rand.Read(id)
	dir := filepath.Join(stagingParent, hex.EncodeToString(id))
	if err := archive.Extract(file, dir, progress); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	s := &Staged{Dir: dir, File: filepath.Base(file)}
	s.Name = NameFromFile(s.File)
	s.NexusID, s.Version, _ = ParseNexusFile(s.File)

	if root := fomodRoot(dir); root != "" {
		cfg, err := fomod.Load(root)
		if err != nil {
			os.RemoveAll(dir)
			return nil, err
		}
		s.Fomod, s.FomodRoot = cfg, root
		if name := firstNonEmpty(cfg.Info.Name, cfg.Name); name != "" {
			s.Name = name
		}
		s.Version = firstNonEmpty(cfg.Info.Version, s.Version)
		s.Author = cfg.Info.Author
		return s, nil
	}
	s.Parts, s.Unsure = layout(dir)
	return s, nil
}

// Remove deletes the unpacked archive.
func (s *Staged) Remove() error { return os.RemoveAll(s.Dir) }

// Install puts the mod's files into modDir: those a FOMOD installer picked
// with the options in files, or a plain archive's where its layout puts
// them. Files modDir has already are replaced and the rest kept, so
// installing into a mod's folder merges the two.
func (s *Staged) Install(modDir string, files []fomod.File) error {
	if s.Fomod != nil {
		return fomod.Install(s.FomodRoot, files, modDir)
	}
	for _, p := range s.Parts {
		to := modDir
		if p.To != "" {
			to = foldJoin(modDir, p.To)
		}
		if err := mergeTree(p.From, to); err != nil {
			return err
		}
	}
	return nil
}

// HasFomod reports whether an archive comes with a FOMOD installer, where
// Stage looks for one, without unpacking it.
func HasFomod(file string) (bool, error) {
	names, err := archive.List(file)
	if err != nil {
		return false, err
	}
	for _, name := range names {
		parts := strings.Split(strings.Trim(strings.ToLower(name), "/"), "/")
		if n := len(parts); n >= 2 && n <= 3 && parts[n-1] == "moduleconfig.xml" && parts[n-2] == "fomod" {
			return true, nil
		}
	}
	return false, nil
}

// fomodRoot finds the folder holding an archive's fomod folder, at its top
// or in the one folder it is wrapped in.
func fomodRoot(dir string) string {
	if hasFomod(dir) {
		return dir
	}
	if only := OnlyFolder(dir); only != "" && hasFomod(only) {
		return only
	}
	return ""
}

func hasFomod(dir string) bool {
	name, _ := FindFold(dir, "fomod")
	if name == "" {
		return false
	}
	config, _ := FindFold(filepath.Join(dir, name), "ModuleConfig.xml")
	return config != ""
}

// Shape is how the archives of a game's mods are laid out, for the games
// whose archives start from the game folder.
type Shape struct {
	// Folders are folders at the top of the game folder: an archive's
	// folder holding one of them is laid out as the game folder is.
	Folders []string
	// Loose are the folders files are put in that an archive has at its
	// top, or at the top of the folders it is wrapped in, by extension.
	Loose map[string]string
	// Within are folders deeper in the game folder an archive can start
	// from, such as CookedPCConsole for BmGame/CookedPCConsole.
	Within map[string]string
}

// Layout is a Layout for archives of this shape. An archive is laid out as
// the game folder when it, or the folders it is wrapped in, holds one of
// the shape's Folders; otherwise its Within folders and Loose files go
// where they belong.
func (sh Shape) Layout(dir string) ([]Part, bool) {
	for {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return []Part{{From: dir}}, true
		}
		for _, e := range entries {
			if e.IsDir() && slices.ContainsFunc(sh.Folders, func(f string) bool { return strings.EqualFold(f, e.Name()) }) {
				return []Part{{From: dir}}, false
			}
		}
		var parts []Part
		known := false
		for _, e := range entries {
			from := filepath.Join(dir, e.Name())
			if e.IsDir() {
				if to, ok := foldLookup(sh.Within, e.Name()); ok {
					parts = append(parts, Part{From: from, To: to})
					known = true
					continue
				}
			} else if to, ok := foldLookup(sh.Loose, filepath.Ext(e.Name())); ok {
				parts = append(parts, Part{From: from, To: path(to, e.Name())})
				known = true
				continue
			} else if isDocument(e.Name()) {
				continue
			}
			parts = append(parts, Part{From: from, To: e.Name()})
		}
		if known {
			return parts, false
		}
		only := OnlyFolder(dir)
		if only == "" {
			return []Part{{From: dir}}, true
		}
		dir = only
	}
}

func foldLookup(m map[string]string, key string) (string, bool) {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
}

func path(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

// OnlyFolder is the one folder dir has, when everything else in it is
// documentation; "" otherwise.
func OnlyFolder(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	only := ""
	for _, e := range entries {
		if e.IsDir() {
			if only != "" {
				return ""
			}
			only = filepath.Join(dir, e.Name())
			continue
		}
		if !isDocument(e.Name()) {
			return ""
		}
	}
	return only
}

// isDocument reports whether a file of an archive is there to be read, not
// to be installed.
func isDocument(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".txt", ".md", ".pdf", ".url", ".html", ".htm", ".rtf", ".jpg", ".jpeg", ".png":
		return true
	}
	return false
}

// mergeTree moves the files of from, a folder or a file, to the same places
// in to, into the folders already there in whatever letter case they have,
// replacing the files that are there.
func mergeTree(from, to string) error {
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		target := to
		if rel != "." {
			target = foldJoin(to, filepath.ToSlash(rel))
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return MoveFile(p, target)
	})
}

// foldJoin is the path of rel under base, spelled as the folders and files
// already there are.
func foldJoin(base, rel string) string {
	target := base
	for _, part := range strings.Split(rel, "/") {
		if name, _ := FindFold(target, part); name != "" {
			part = name
		}
		target = filepath.Join(target, part)
	}
	return target
}

var (
	// "SkyUI-12604-6-11-1778020881.zip": name, mod id, version with dashes
	// for dots, upload time.
	nexusFile = regexp.MustCompile(`^.+?-(\d+)-([0-9a-zA-Z][0-9a-zA-Z-]*?)-\d{9,11}\.\w+$`)
	// "SkyUI 12604 6.11 2026-08-27T16-52Z s6Og0dG94.7z", as the Nexus Mods
	// app names its downloads.
	nexusAppFile = regexp.MustCompile(`^.+ (\d+) (\S+) \d{4}-\d{2}-\d{2}T\d{2}-\d{2}Z \w+\.\w+$`)
)

// ParseNexusFile reads the mod id and version from the name of an archive
// downloaded from Nexus Mods.
func ParseNexusFile(name string) (id int, version string, ok bool) {
	if m := nexusAppFile.FindStringSubmatch(name); m != nil {
		id, _ = strconv.Atoi(m[1])
		return id, m[2], true
	}
	if m := nexusFile.FindStringSubmatch(name); m != nil {
		id, _ = strconv.Atoi(m[1])
		return id, strings.ReplaceAll(m[2], "-", "."), true
	}
	return 0, "", false
}

// NameFromFile is a mod's name as the name of the archive it came in gives
// it, without what Nexus Mods adds: the mod id, version and upload time.
func NameFromFile(file string) string {
	name := strings.TrimSuffix(file, filepath.Ext(file))
	if m := nexusAppFile.FindStringSubmatchIndex(file); m != nil {
		return strings.TrimSpace(file[:m[2]])
	}
	if m := nexusFile.FindStringSubmatchIndex(file); m != nil {
		return strings.TrimRight(file[:m[2]], "-")
	}
	return name
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
