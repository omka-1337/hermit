// Package modinstall installs and removes Thunderstore packages in a profile.
package modinstall

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"hermit/internal/thunderstore"
)

type entry struct {
	file   *zip.File
	dest   string
	config bool
}

// planEntries maps archive entries to their destination in the profile.
//
// A mod loader package (listed in rules.LoaderPackages, or recognised by a
// BepInEx/core folder) is unpacked into the profile root, since it carries the
// doorstop files that belong next to the game executable. Other packages are
// placed by the install rules the way r2modman does it:
//
//   - a folder named like the last segment of a route (plugins, config, ...),
//     at any depth, is installed into that route with its structure kept;
//     when several routes share the name, the one whose path matches the
//     folder's path best wins;
//   - other folders are descended into;
//   - loose files go to the route whose extension matches (the longest match
//     wins, so .mm.dll beats .dll), else to the default route, flattened to
//     their file name.
func planEntries(zr *zip.Reader, modID string, rules Rules) ([]entry, error) {
	files := map[string]*zip.File{}
	var names []string
	for _, f := range zr.File {
		if isDirEntry(f) {
			continue
		}
		name, err := cleanEntryName(f.Name)
		if err != nil {
			return nil, err
		}
		if _, dup := files[name]; !dup {
			names = append(names, name)
		}
		files[name] = f
	}

	if rules.Into != "" {
		return placeInto(files, names, rules.Into), nil
	}
	slices.Sort(names)

	if root, ok := loaderRoot(names, modID, rules); ok {
		var entries []entry
		for _, name := range names {
			rel, ok := strings.CutPrefix(name, root)
			if !ok || (root == "" && slices.Contains(loaderMetadata, strings.ToLower(rel))) {
				continue // manifest, icon, readme next to the loader folder
			}
			config := strings.HasPrefix(strings.ToLower(rel), "bepinex/config/")
			entries = append(entries, entry{file: files[name], dest: rel, config: config})
		}
		return entries, nil
	}

	var entries []entry
	add := func(name string, rule thunderstore.InstallRule, rel string) {
		e := entry{file: files[name]}
		switch rule.TrackingMethod {
		case thunderstore.TrackingNone:
			e.dest, e.config = path.Join(rule.Route, rel), true
		case thunderstore.TrackingState:
			if slices.ContainsFunc(rules.RelativeFileExclusions, func(x string) bool { return strings.EqualFold(x, rel) }) {
				return
			}
			e.dest = path.Join(rule.Route, rel)
		default: // subdir and anything unknown
			e.dest = path.Join(rule.Route, modID, rel)
		}
		entries = append(entries, e)
	}

	var walk func(dir string)
	walk = func(dir string) {
		subdirs := map[string]bool{}
		for _, name := range names {
			rest, ok := strings.CutPrefix(name, dir)
			if !ok {
				continue
			}
			if sub, _, nested := strings.Cut(rest, "/"); nested {
				subdirs[sub] = true
				continue
			}
			if rule, ok := ruleForFile(rules.Routes, rest); ok {
				add(name, rule, rest)
			}
		}
		for _, sub := range slices.Sorted(maps.Keys(subdirs)) {
			full := dir + sub + "/"
			rule, ok := ruleForDir(rules.Routes, strings.TrimSuffix(full, "/"))
			if !ok {
				walk(full)
				continue
			}
			for _, name := range names {
				if rel, ok := strings.CutPrefix(name, full); ok {
					add(name, rule, rel)
				}
			}
		}
	}
	walk("")

	// Later entries overwrite earlier ones on disk; keep the last per destination.
	seen := map[string]bool{}
	var unique []entry
	for i := len(entries) - 1; i >= 0; i-- {
		if !seen[entries[i].dest] {
			seen[entries[i].dest] = true
			unique = append(unique, entries[i])
		}
	}
	slices.Reverse(unique)
	return unique, nil
}

var loaderMetadata = []string{"manifest.json", "icon.png", "readme.md"}

// loaderRoot reports whether the package is a mod loader and returns the
// archive prefix that maps to the profile root.
//
// Loaders are identified by the schema's modloaderPackages list, like r2modman
// does. The fallback for packages not on the list must be strict: regular mods
// also ship files in BepInEx/core (e.g. MonoMod assemblies), so a loader needs
// BepInEx's own assemblies plus a launcher (doorstop files or a start script)
// next to the BepInEx folder.
func loaderRoot(names []string, modID string, rules Rules) (string, bool) {
	if folder, ok := rules.LoaderPackages[strings.ToLower(modID)]; ok {
		if folder == "" {
			return "", true
		}
		return folder + "/", true
	}
	lower := make(map[string]bool, len(names))
	for _, n := range names {
		lower[strings.ToLower(n)] = true
	}
	for _, name := range names {
		l := strings.ToLower(name)
		i := strings.Index(l, "bepinex/core/")
		if i < 0 {
			continue
		}
		prefix := l[:i]
		if prefix != "" && (strings.Count(prefix, "/") != 1 || !strings.HasSuffix(prefix, "/")) {
			continue
		}
		hasBepInEx := slices.ContainsFunc(loaderAssemblies, func(a string) bool { return lower[prefix+"bepinex/core/"+a] })
		hasLauncher := slices.ContainsFunc(loaderLaunchers, func(f string) bool { return lower[prefix+f] })
		if hasBepInEx && hasLauncher {
			return name[:i], true
		}
	}
	return "", false
}

var (
	loaderAssemblies = []string{"bepinex.preloader.dll", "bepinex.dll", "bepinex.core.dll", "bepinex.unity.il2cpp.dll"}
	loaderLaunchers  = []string{"winhttp.dll", "doorstop_config.ini", "run_bepinex.sh", "start_game_bepinex.sh"}
)

func ruleForFile(routes []thunderstore.InstallRule, name string) (thunderstore.InstallRule, bool) {
	lower := strings.ToLower(name)
	best, bestLen := thunderstore.InstallRule{}, 0
	for _, r := range routes {
		for _, ext := range r.DefaultFileExtensions {
			if strings.HasSuffix(lower, strings.ToLower(ext)) && len(ext) > bestLen {
				best, bestLen = r, len(ext)
			}
		}
	}
	if bestLen > 0 {
		return best, true
	}
	for _, r := range routes {
		if r.IsDefaultLocation {
			return r, true
		}
	}
	return thunderstore.InstallRule{}, false
}

// ruleForDir matches a folder by its name against the last route segment.
func ruleForDir(routes []thunderstore.InstallRule, dir string) (thunderstore.InstallRule, bool) {
	dirParts := strings.Split(dir, "/")
	name := dirParts[len(dirParts)-1]
	best, bestScore, found := thunderstore.InstallRule{}, -1, false
	for _, r := range routes {
		if !strings.EqualFold(path.Base(r.Route), name) {
			continue
		}
		routeParts := strings.Split(r.Route, "/")
		score := 0
		for i := 0; i < len(dirParts) && i < len(routeParts); i++ {
			if dirParts[len(dirParts)-1-i] == routeParts[len(routeParts)-1-i] {
				score++
			}
		}
		if score > bestScore {
			best, bestScore, found = r, score, true
		}
	}
	return best, found
}

// PlanFiles returns the tracked files Extract would install, without writing anything.
func PlanFiles(zipPath, modID string, rules Rules) ([]string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	entries, err := planEntries(&zr.Reader, modID, rules)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.config && !slices.Contains(files, e.dest) {
			files = append(files, e.dest)
		}
	}
	slices.Sort(files)
	return files, nil
}

// IsLoader reports whether installed files belong to a mod loader package.
// Only loader packages place files in the profile root (winhttp.dll and
// friends); install rules always put other files under a route folder.
func IsLoader(files []string) bool {
	return slices.ContainsFunc(files, func(f string) bool { return !strings.Contains(f, "/") })
}

// Extract unpacks a package archive into profileDir and returns the installed
// files as slash-separated paths relative to profileDir. Config files are not
// returned and, unless overwriteConfigs is set, only written if absent, so
// user edits survive reinstalls and uninstalls.
func Extract(zipPath, profileDir, modID string, rules Rules, overwriteConfigs bool) ([]string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	entries, err := planEntries(&zr.Reader, modID, rules)
	if err != nil {
		return nil, err
	}

	var installed []string
	for _, e := range entries {
		target := filepath.Join(profileDir, filepath.FromSlash(e.dest))
		if e.config && !overwriteConfigs {
			if _, err := os.Stat(target); err == nil {
				continue
			}
		}
		if err := writeEntry(e.file, target); err != nil {
			_ = Remove(profileDir, installed)
			return nil, fmt.Errorf("extract %s: %w", e.file.Name, err)
		}
		if !e.config && !slices.Contains(installed, e.dest) {
			installed = append(installed, e.dest)
		}
	}
	slices.Sort(installed)
	return installed, nil
}

// isDirEntry reports whether an archive entry is a folder. Archives zipped on
// Windows can name folders with backslashes ("plugins\Translations\") and
// leave out the folder flag, so the trailing separator is what tells.
func isDirEntry(f *zip.File) bool {
	return f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") || strings.HasSuffix(f.Name, `\`)
}

func cleanEntryName(name string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	clean := path.Clean("/" + name)[1:]
	if clean == "" || clean != strings.TrimPrefix(name, "./") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("unsafe path in archive: %q", name)
	}
	return clean, nil
}

func writeEntry(f *zip.File, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	mode := fs.FileMode(0o644)
	if f.Mode()&0o111 != 0 {
		mode = 0o755
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// keptDirs are never pruned, so a profile always has the basic BepInEx layout.
var keptDirs = []string{"BepInEx", "BepInEx/plugins", "BepInEx/patchers", "BepInEx/config"}

// Remove deletes installed files and prunes directories left empty.
func Remove(profileDir string, files []string) error {
	var errs []error
	var removed []string
	for _, rel := range files {
		clean, err := cleanEntryName(rel)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := os.Remove(filepath.Join(profileDir, filepath.FromSlash(clean))); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
		removed = append(removed, clean)
	}
	pruneDirs(profileDir, removed)
	return errors.Join(errs...)
}

// intoSkipped are package metadata, not files the mod reads.
var intoSkipped = []string{"manifest.json", "icon.png"}

// placeInto puts every file of a package under one folder, keeping the paths
// inside the archive. An archive that holds a single folder with everything in
// it is unwrapped first: that folder is only how the files were zipped.
func placeInto(files map[string]*zip.File, names []string, into string) []entry {
	prefix := commonTopFolder(names)
	var entries []entry
	for _, name := range names {
		rel := strings.TrimPrefix(name, prefix)
		if !strings.Contains(rel, "/") && slices.Contains(intoSkipped, strings.ToLower(rel)) {
			continue
		}
		entries = append(entries, entry{file: files[name], dest: path.Join(into, rel)})
	}
	return entries
}

// commonTopFolder returns "dir/" when every name lies in that one folder.
func commonTopFolder(names []string) string {
	if len(names) == 0 {
		return ""
	}
	top, _, nested := strings.Cut(names[0], "/")
	if !nested {
		return ""
	}
	for _, name := range names[1:] {
		dir, _, nested := strings.Cut(name, "/")
		if !nested || dir != top {
			return ""
		}
	}
	return top + "/"
}
