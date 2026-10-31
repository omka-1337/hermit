package bethesda

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"hermit/internal/archive"
	"hermit/internal/fomod"
)

// StagingDir is the folder in a profile archives are unpacked into before
// they are installed: on the profile's disk, so their files move into the
// mod's folder without being copied.
const StagingDir = ".staging"

// Staged is a mod archive unpacked and looked at, ready to be installed.
type Staged struct {
	Dir string
	// File is the archive's name.
	File string
	// Fomod is the archive's installer, nil when it has none; FomodRoot is
	// the folder holding its fomod folder, the one its paths start from.
	Fomod     *fomod.Config
	FomodRoot string
	// DataRoot is the folder of a plain archive laid out as Data is, and
	// GameRoot one whose files go into the game folder itself, if any.
	DataRoot string
	GameRoot []string
	// Unsure is set when nothing in the archive looked like Data: its files
	// are installed as they are.
	Unsure bool
	// Name, Version and Author come from the installer or the file name.
	Name, Version, Author string
	NexusID               int
}

// Stage unpacks an archive into a new folder of stagingParent and works out
// how it is laid out.
func Stage(file, stagingParent string, progress func(done, total int64)) (*Staged, error) {
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
	s.DataRoot, s.GameRoot, s.Unsure = layout(dir)
	return s, nil
}

// Remove deletes the unpacked archive.
func (s *Staged) Remove() error { return os.RemoveAll(s.Dir) }

// Install puts the mod's files into modDir: those a FOMOD installer picked
// with the options in files, or a plain archive's, its game folder files
// under Root.
func (s *Staged) Install(modDir string, files []fomod.File) error {
	if s.Fomod != nil {
		return fomod.Install(s.FomodRoot, files, modDir)
	}
	if err := moveContents(s.DataRoot, modDir); err != nil {
		return err
	}
	for _, name := range s.GameRoot {
		if err := moveFile(filepath.Join(filepath.Dir(s.DataRoot), name), filepath.Join(modDir, "Root", name)); err != nil {
			return err
		}
	}
	return nil
}

// fomodRoot finds the folder holding an archive's fomod folder, at its top
// or in the one folder it is wrapped in.
func fomodRoot(dir string) string {
	if hasFomod(dir) {
		return dir
	}
	if only := onlyFolder(dir); only != "" && hasFomod(only) {
		return only
	}
	return ""
}

func hasFomod(dir string) bool {
	name, _ := findFold(dir, "fomod")
	if name == "" {
		return false
	}
	config, _ := findFold(filepath.Join(dir, name), "ModuleConfig.xml")
	return config != ""
}

// dataFolders are folders found at the top of Data, by which a folder of an
// archive is known to be laid out as Data is.
var dataFolders = []string{
	"meshes", "textures", "scripts", "interface", "sound", "music", "skse", "f4se", "sfse", "seq", "strings",
	"video", "materials", "lodsettings", "grass", "shadersfx", "calientetools", "nemesis_engine", "netscriptframework",
	"dyndolod", "source", "lightplacer", "pbrnifpatcher", "pbrtexturesets", "terrain", "vis", "facegen", "distantlod",
	"lod", "shaders", "platform", "tools",
}

// layout works out where an archive's Data starts. An explicit Data folder
// has it, and what is beside it goes into the game folder, as with SKSE;
// otherwise the folder with Data's folders or with plugins is it, looked
// for through folders an archive is wrapped in.
func layout(dir string) (dataRoot string, gameRoot []string, unsure bool) {
	for {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return dir, nil, true
		}
		for _, e := range entries {
			if e.IsDir() && strings.EqualFold(e.Name(), "Data") {
				for _, other := range entries {
					if other.Name() != e.Name() && !strings.EqualFold(other.Name(), "src") {
						gameRoot = append(gameRoot, other.Name())
					}
				}
				return filepath.Join(dir, e.Name()), gameRoot, false
			}
		}
		for _, e := range entries {
			name := strings.ToLower(e.Name())
			if (e.IsDir() && slices.Contains(dataFolders, name)) || (!e.IsDir() && isDataFile(name)) {
				return dir, nil, false
			}
		}
		only := onlyFolder(dir)
		if only == "" {
			return dir, nil, true
		}
		dir = only
	}
}

func isDataFile(name string) bool {
	switch filepath.Ext(name) {
	case ".esp", ".esm", ".esl", ".bsa", ".ba2":
		return true
	}
	return false
}

// onlyFolder is the one folder dir has, when everything else in it is
// documentation; "" otherwise.
func onlyFolder(dir string) string {
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
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".txt", ".md", ".pdf", ".url", ".html", ".htm", ".rtf", ".jpg", ".jpeg", ".png":
		default:
			return ""
		}
	}
	return only
}

// moveContents moves what is in from into to.
func moveContents(from, to string) error {
	if err := os.MkdirAll(to, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := moveFile(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
			return err
		}
	}
	return nil
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
