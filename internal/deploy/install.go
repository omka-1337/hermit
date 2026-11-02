package deploy

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
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
// mod's folder; name is the mod's, from the archive. unsure is set when
// nothing in it looked like the game's files: it is then installed as it is.
// An error says why the archive is not one Hermit installs.
type Layout func(dir, name string) (parts []Part, unsure bool, err error)

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
	// A file that is no archive is a mod of one file, as The Sims 4's
	// custom content comes, put as it is in the folder unpacked.
	var err error
	if archive.Supported(file) {
		err = archive.Extract(file, dir, progress)
	} else if err = os.MkdirAll(dir, 0o755); err == nil {
		err = copyFile(file, filepath.Join(dir, filepath.Base(file)))
	}
	if err != nil {
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
	if s.Parts, s.Unsure, err = layout(dir, s.Name); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return s, nil
}

// Remove deletes the unpacked archive.
func (s *Staged) Remove() error { return os.RemoveAll(s.Dir) }

// Install puts the mod's files into modDir: those a FOMOD installer picked
// with the options in files, or a plain archive's where its layout puts
// them. Files modDir has already are replaced and the rest kept, so
// installing into a mod's folder merges the two; text files of the target's
// Merge, though, are added to, as the options of one archive each bring
// their lines of the same configuration.
func (s *Staged) Install(modDir string, files []fomod.File, t Target) error {
	if s.Fomod != nil {
		return fomod.Install(s.FomodRoot, files, modDir)
	}
	for _, p := range s.Parts {
		to := modDir
		if p.To != "" {
			to = foldJoin(modDir, p.To)
		}
		if err := mergeTree(p.From, to, t); err != nil {
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
	// Bundles are folders of an archive that are whole bundles of the
	// game's own kind, recognised by the paths all found in them, and where
	// they go, under their own name: the downloadable content of Unreal
	// Engine games, a skin or a suit each, is a folder with CookedPCConsole
	// and Config/BmGame.ini, put into a folder of DLC. A "*" in where they go
	// stands for the first folder the game has there; an archive can hold
	// several bundles side by side.
	Bundles map[string][]string
	// Game is the game's folder, which a "*" of Bundles is looked up in.
	Game string
	// Foreign are the names and extensions of files by which an archive is
	// known to be a mod another tool installs by changing the game's own
	// files; Hermit, keeping the game as it is, does not install those, and
	// Refusal says so.
	Foreign []string
	Refusal string
}

// foreign refuses an archive with files of Foreign in it.
func (sh Shape) foreign(dir string) error {
	if len(sh.Foreign) == 0 {
		return nil
	}
	found := false
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		name := strings.ToLower(d.Name())
		found = slices.ContainsFunc(sh.Foreign, func(f string) bool {
			f = strings.ToLower(f)
			return name == f || strings.HasPrefix(f, ".") && strings.HasSuffix(name, f)
		})
		if found {
			return fs.SkipAll
		}
		return nil
	})
	if found {
		return errors.New(sh.Refusal)
	}
	return nil
}

// bundle finds where dir goes when it is a bundle, named name.
func (sh Shape) bundle(dir, name string) (string, bool) {
	for target, paths := range sh.Bundles {
		if !slices.ContainsFunc(paths, func(p string) bool { return !existsFold(dir, p) }) {
			return path(sh.resolve(target), safeName(name)), true
		}
	}
	return "", false
}

// resolve puts the first folder the game has in place of a "*" of target.
func (sh Shape) resolve(target string) string {
	parts := strings.Split(target, "/")
	dir := sh.Game
	for i, part := range parts {
		if part == "*" {
			entries, _ := os.ReadDir(dir)
			first := ""
			for _, e := range entries {
				if e.IsDir() {
					first = e.Name()
					break
				}
			}
			if first == "" {
				first = "Hermit"
			}
			parts[i] = first
		}
		if name, _ := FindFold(dir, parts[i]); name != "" {
			parts[i] = name
		}
		dir = filepath.Join(dir, parts[i])
	}
	return strings.Join(parts, "/")
}

// existsFold reports whether rel, slash-separated, is under dir, in any
// letter case.
func existsFold(dir, rel string) bool {
	for _, part := range strings.Split(rel, "/") {
		name, _ := FindFold(dir, part)
		if name == "" {
			return false
		}
		dir = filepath.Join(dir, name)
	}
	return true
}

// safeName is a name fit for a folder of the game.
func safeName(name string) string {
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r < ' ' {
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "Mod"
	}
	return name
}

// Layout is a Layout for archives of this shape. An archive is laid out as
// the game folder when it, or the folders it is wrapped in, holds one of
// the shape's Folders; otherwise its Bundles, its Within folders and its
// Loose files go where they belong.
func (sh Shape) Layout(dir, name string) ([]Part, bool, error) {
	if err := sh.foreign(dir); err != nil {
		return nil, false, err
	}
	top := dir
	for {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return []Part{{From: dir}}, true, nil
		}
		for _, e := range entries {
			if e.IsDir() && slices.ContainsFunc(sh.Folders, func(f string) bool { return strings.EqualFold(f, e.Name()) }) {
				return []Part{{From: dir}}, false, nil
			}
		}
		// The archive is one bundle: named as the folder it is wrapped in,
		// or as the mod.
		own := name
		if dir != top {
			own = filepath.Base(dir)
		}
		if to, ok := sh.bundle(dir, own); ok {
			return []Part{{From: dir, To: to}}, false, nil
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
				if to, ok := sh.bundle(from, e.Name()); ok {
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
			return parts, false, nil
		}
		only := OnlyFolder(dir)
		if only == "" {
			return []Part{{From: dir}}, true, nil
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
// replacing the files that are there, or adding to those t merges.
func mergeTree(from, to string, t Target) error {
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
		if t.merges(target) {
			if had, err := os.ReadFile(target); err == nil {
				add, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				return os.WriteFile(target, AppendText(had, add), 0o644)
			}
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

func copyFile(from, to string) error {
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
		return err
	}
	return out.Close()
}
