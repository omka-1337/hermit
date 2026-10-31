package creation

import (
	"bufio"
	"errors"
	"fmt"
	"hermit/internal/deploy"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

// MO2Instance is a Mod Organizer 2 instance: the folder with
// ModOrganizer.ini, its mods and its profiles.
type MO2Instance struct {
	Dir string `json:"dir"`
	// GameName is how MO2 names the game, e.g. "Skyrim Special Edition".
	GameName string `json:"gameName"`
	// GamePath is the game folder MO2 is set up for, as a Linux path.
	GamePath string   `json:"gamePath"`
	Profiles []string `json:"profiles"`
	// Selected is the profile MO2 was last using.
	Selected string `json:"selected"`

	modsDir, profilesDir, overwriteDir string
}

// MO2Mod is a mod of an MO2 profile.
type MO2Mod struct {
	// Folder is the mod's folder in MO2's mods folder, which is also its name.
	Folder  string `json:"folder"`
	Enabled bool   `json:"enabled"`
	// NexusID is the mod's id on Nexus Mods, 0 when it did not come from there.
	NexusID int    `json:"nexusId"`
	Version string `json:"version"`
	// File is the archive the mod was installed from.
	File string `json:"file"`
	// Comments are MO2's notes on the mod, often the options chosen in its
	// installer.
	Comments string `json:"comments"`
	// Separator is set for MO2's separators, the headings it groups the
	// list with; their folders hold nothing but MO2's own settings.
	Separator bool `json:"separator"`
}

// SeparatorName is the heading of an MO2 separator, whose folder is named
// "<heading>_separator".
func (m MO2Mod) SeparatorName() string {
	return strings.TrimSuffix(m.Folder, mo2SeparatorSuffix)
}

const mo2SeparatorSuffix = "_separator"

// MO2Profile is what Hermit takes over from an MO2 profile.
type MO2Profile struct {
	// Mods are lowest priority first, the order Hermit keeps them in; MO2's
	// modlist.txt lists them the other way round. A separator comes before
	// the mods it heads, as MO2 shows it.
	Mods    []MO2Mod `json:"mods"`
	Plugins []Plugin `json:"plugins"`
	// INIs are the profile's own INI files, by lower-case name, when MO2
	// played the game with them instead of the game's (LocalSettings).
	INIs map[string]string `json:"inis"`
}

// ReadMO2 reads the instance in dir.
func ReadMO2(dir string) (MO2Instance, error) {
	ini, err := readINI(filepath.Join(dir, "ModOrganizer.ini"))
	if errors.Is(err, fs.ErrNotExist) {
		return MO2Instance{}, errors.New("this folder is not a Mod Organizer 2 instance: it has no ModOrganizer.ini")
	}
	if err != nil {
		return MO2Instance{}, err
	}
	in := MO2Instance{
		Dir:      dir,
		GameName: ini["General"]["gameName"],
		GamePath: windowsPath(ini["General"]["gamePath"]),
		Selected: qtValue(ini["General"]["selected_profile"]),
	}
	// Folders MO2 was told to keep elsewhere; %BASE_DIR% is the instance's
	// own base folder.
	base := dir
	if b := ini["Settings"]["base_directory"]; b != "" {
		base = windowsPath(b)
	}
	folder := func(key, name string) string {
		if v := ini["Settings"][key]; v != "" {
			return windowsPath(strings.ReplaceAll(v, "%BASE_DIR%", base))
		}
		return filepath.Join(base, name)
	}
	in.modsDir = folder("mod_directory", "mods")
	in.profilesDir = folder("profiles_directory", "profiles")
	in.overwriteDir = folder("overwrite_directory", "overwrite")

	entries, err := os.ReadDir(in.profilesDir)
	if err != nil {
		return MO2Instance{}, fmt.Errorf("reading MO2's profiles: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			in.Profiles = append(in.Profiles, e.Name())
		}
	}
	return in, nil
}

// ReadProfile reads one of the instance's profiles.
func (in MO2Instance) ReadProfile(name string) (MO2Profile, error) {
	if !slices.Contains(in.Profiles, name) {
		return MO2Profile{}, fmt.Errorf("MO2 has no profile %q", name)
	}
	dir := filepath.Join(in.profilesDir, name)
	lines, err := readLines(filepath.Join(dir, "modlist.txt"))
	if err != nil {
		return MO2Profile{}, err
	}
	var p MO2Profile
	for _, line := range slices.Backward(lines) {
		// "+" enabled, "-" disabled, "*" the game's own content MO2 lists
		// but does not manage.
		mark, folder := line[:1], line[1:]
		if mark != "+" && mark != "-" {
			continue
		}
		if strings.HasSuffix(folder, mo2SeparatorSuffix) {
			p.Mods = append(p.Mods, MO2Mod{Folder: folder, Separator: true})
			continue
		}
		m := MO2Mod{Folder: folder, Enabled: mark == "+"}
		meta, err := readINI(filepath.Join(in.modsDir, folder, "meta.ini"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return MO2Profile{}, err
		}
		general := meta["General"]
		m.NexusID, _ = strconv.Atoi(general["modid"])
		// MO2 records the install date, "d2026.10.26", when the version is unknown.
		if v := general["version"]; !strings.HasPrefix(v, "d") {
			m.Version = v
		}
		if f := general["installationFile"]; f != "" {
			m.File = filepath.Base(windowsPath(f))
		}
		m.Comments = general["comments"]
		if id, version, ok := deploy.ParseNexusFile(m.File); ok {
			if m.NexusID == 0 {
				m.NexusID = id
			}
			if m.Version == "" {
				m.Version = version
			}
		}
		p.Mods = append(p.Mods, m)
	}

	settings, err := readINI(filepath.Join(dir, "settings.ini"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return MO2Profile{}, err
	}
	if settings["General"]["LocalSettings"] == "true" {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return MO2Profile{}, err
		}
		p.INIs = map[string]string{}
		for _, e := range entries {
			if name := strings.ToLower(e.Name()); !e.IsDir() && strings.HasSuffix(name, ".ini") {
				p.INIs[name] = filepath.Join(dir, e.Name())
			}
		}
	}

	// loadorder.txt has every plugin in order, plugins.txt marks the active
	// ones; an older profile may have only plugins.txt.
	active, err := ReadPlugins(filepath.Join(dir, "plugins.txt"))
	if err != nil {
		return MO2Profile{}, err
	}
	order, err := readLines(filepath.Join(dir, "loadorder.txt"))
	if errors.Is(err, fs.ErrNotExist) {
		p.Plugins = active
		return p, nil
	}
	if err != nil {
		return MO2Profile{}, err
	}
	for _, name := range order {
		on := slices.ContainsFunc(active, func(a Plugin) bool { return a.Active && strings.EqualFold(a.Name, name) })
		p.Plugins = append(p.Plugins, Plugin{Name: name, Active: on})
	}
	return p, nil
}

// ImportMO2 brings the mods of an MO2 profile into the profile at
// profileDir, under ids, the Hermit ids of p.Mods in the same order, and
// MO2's overwrite folder into its overwrite folder. The game's own plugins,
// files in gameData, are left out of the plugin list: the game loads them by
// itself. The profile's own INI files, those among inis, the game's, that are
// not empty, go into the profile's ini folder under the game's names. Files
// are hard links where the profile is on the same file system as MO2, which
// takes no time and no space and leaves MO2 as it was; elsewhere they are
// copied, once it is clear they fit.
func (in MO2Instance) ImportMO2(p MO2Profile, ids []string, profileDir, gameData string, inis []string) error {
	type copyJob struct{ from, to string }
	jobs := make([]copyJob, 0, len(p.Mods)+1)
	for i, m := range p.Mods {
		if !m.Separator {
			jobs = append(jobs, copyJob{filepath.Join(in.modsDir, m.Folder), filepath.Join(profileDir, deploy.ModsDir, ids[i])})
		}
	}
	jobs = append(jobs, copyJob{in.overwriteDir, filepath.Join(profileDir, deploy.OverwriteDir)})

	if !sameFileSystem(in.modsDir, profileDir) {
		var need uint64
		for _, j := range jobs {
			size, err := treeSize(j.from)
			if err != nil {
				return err
			}
			need += size
		}
		var st syscall.Statfs_t
		if err := syscall.Statfs(profileDir, &st); err != nil {
			return err
		}
		// Filling the disk to the brim would leave the system without room
		// to work in.
		const spare = 2 << 30
		if free := st.Bavail * uint64(st.Bsize); need+spare > free {
			return fmt.Errorf("the mods take %s and the profile's disk has %s free; move the profile to the disk MO2 is on first, so its files can be linked instead of copied",
				byteSize(need), byteSize(free))
		}
	}
	for _, j := range jobs {
		if err := linkTree(j.from, j.to); err != nil {
			return err
		}
	}

	for _, name := range inis {
		from, ok := p.INIs[strings.ToLower(name)]
		if fi, err := os.Stat(from); !ok || err != nil || fi.Size() == 0 {
			continue
		}
		if err := os.MkdirAll(filepath.Join(profileDir, INIDir), 0o755); err != nil {
			return err
		}
		// Copied rather than linked: the game writes its settings into them.
		if err := copyFile(from, filepath.Join(profileDir, INIDir, name)); err != nil {
			return err
		}
	}

	var plugins []Plugin
	for _, pl := range p.Plugins {
		if name, _ := deploy.FindFold(gameData, pl.Name); name == "" {
			plugins = append(plugins, pl)
		}
	}
	return WritePlugins(filepath.Join(profileDir, PluginsFile), plugins)
}

// NexusURL is the page of a mod on Nexus Mods.
func (g Game) NexusURL(id int) string {
	return fmt.Sprintf("https://www.nexusmods.com/%s/mods/%d", g.Nexus, id)
}

// FindByMO2Name returns the game MO2 calls name.
func FindByMO2Name(name string) (Game, bool) {
	for _, g := range Games {
		if g.MO2Name == name {
			return g, true
		}
	}
	return Game{}, false
}

// linkTree recreates the folder from at to, hard-linking its files where it
// can and copying them where it cannot. A missing from has nothing to bring.
func linkTree(from, to string) error {
	err := filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		target := filepath.Join(to, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type()&fs.ModeSymlink != 0:
			dest, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(dest, target)
		}
		if err := os.Link(p, target); err == nil {
			return nil
		}
		return copyFile(p, target)
	})
	if errors.Is(err, fs.ErrNotExist) {
		if _, statErr := os.Stat(from); errors.Is(statErr, fs.ErrNotExist) {
			return nil
		}
	}
	return err
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

func treeSize(dir string) (uint64, error) {
	var size uint64
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			fi, err := d.Info()
			if err != nil {
				return err
			}
			size += uint64(fi.Size())
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return size, nil
	}
	return size, err
}

func sameFileSystem(a, b string) bool {
	var sa, sb syscall.Stat_t
	return syscall.Stat(a, &sa) == nil && syscall.Stat(b, &sb) == nil && sa.Dev == sb.Dev
}

func byteSize(n uint64) string {
	return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
}

// qtValue unwraps a value Qt wrote as @ByteArray(...), with backslashes
// doubled.
func qtValue(v string) string {
	if inner, ok := strings.CutPrefix(v, "@ByteArray("); ok {
		return strings.ReplaceAll(strings.TrimSuffix(inner, ")"), `\\`, `\`)
	}
	return v
}

// windowsPath turns a path MO2 wrote, from inside Wine, into a Linux one:
// Z: is the root of the Linux file system there.
func windowsPath(v string) string {
	v = strings.ReplaceAll(qtValue(v), `\`, "/")
	if len(v) >= 2 && (v[0] == 'Z' || v[0] == 'z') && v[1] == ':' {
		v = v[2:]
	}
	return filepath.Clean(v)
}

var iniSection = regexp.MustCompile(`^\[(.+)\]$`)

// readINI reads the sections and keys of an INI file as Qt writes them.
func readINI(file string) (map[string]map[string]string, error) {
	lines, err := readLines(file)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]string{}
	section := ""
	for _, line := range lines {
		if m := iniSection.FindStringSubmatch(line); m != nil {
			section = m[1]
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if out[section] == nil {
			out[section] = map[string]string{}
		}
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		out[section][strings.TrimSpace(key)] = value
	}
	return out, nil
}

// readLines returns the lines of a text file that are not empty or comments.
func readLines(file string) ([]string, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), byteOrderMark))
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines, sc.Err()
}
