package modinstall

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"hermit/internal/library"
	"hermit/internal/plugininfo"
	"hermit/internal/thunderstore"
)

// LocalPackage describes a mod file before installing it.
type LocalPackage struct {
	// Path is the file on disk: a .zip package or a single .dll.
	Path string `json:"path"`
	// Author, Name and Version are suggestions the user may change; together
	// they form the mod id, like a Thunderstore package.
	Author  string `json:"author"`
	Name    string `json:"name"`
	Version string `json:"version"`
	// HasManifest is set for zips that are Thunderstore packages.
	HasManifest  bool                `json:"hasManifest"`
	Description  string              `json:"description"`
	Dependencies []string            `json:"dependencies"`
	Plugins      []plugininfo.Plugin `json:"plugins"`
	// TargetDir, when set, is the folder of the game (a path relative to its
	// root, like "BepInEx/plugins/models/all") that every file of the package
	// goes into, instead of where the install rules would put it. The folders
	// are created, so nothing has to exist in the game first.
	TargetDir string `json:"targetDir"`
}

var ErrUnsupportedFile = errors.New("choose a .zip or .dll file")

// InspectFile reads a mod file and suggests how to name it.
func InspectFile(filePath string) (LocalPackage, error) {
	pkg := LocalPackage{Path: filePath, Dependencies: []string{}, Plugins: []plugininfo.Plugin{}}
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".dll":
		plugins, err := plugininfo.Read(filePath)
		if err != nil {
			return LocalPackage{}, fmt.Errorf("%s: %w", filepath.Base(filePath), err)
		}
		pkg.Plugins = plugins
	case ".zip":
		zr, err := zip.OpenReader(filePath)
		if err != nil {
			return LocalPackage{}, fmt.Errorf("%s: %w", filepath.Base(filePath), err)
		}
		defer zr.Close()
		for _, f := range zr.File {
			name := strings.ToLower(f.Name)
			switch {
			case name == "manifest.json":
				if m, err := readZipManifest(f); err == nil {
					pkg.HasManifest = true
					pkg.Name, pkg.Version, pkg.Description = m.Name, m.VersionNumber, m.Description
					pkg.Author = m.Namespace
					if m.Dependencies != nil {
						pkg.Dependencies = m.Dependencies
					}
				}
			case strings.HasSuffix(name, ".dll") && f.UncompressedSize64 < 64<<20:
				if data, err := readZipFile(f); err == nil {
					if plugins, err := plugininfo.Parse(data); err == nil {
						pkg.Plugins = append(pkg.Plugins, plugins...)
					}
				}
			}
		}
	default:
		return LocalPackage{}, ErrUnsupportedFile
	}

	base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
	if len(pkg.Plugins) > 0 {
		p := pkg.Plugins[0]
		if pkg.Name == "" {
			pkg.Name = p.Name
		}
		if pkg.Version == "" {
			pkg.Version = p.Version
		}
		if pkg.Author == "" {
			// GUIDs are often "author.mod" or "com.author.mod".
			parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(p.GUID, "com."), "io.github."), ".")
			if len(parts) > 1 {
				pkg.Author = parts[0]
			}
		}
	}
	if pkg.Name == "" {
		pkg.Name = base
	}
	pkg.Author = SanitizeName(pkg.Author, "Local")
	pkg.Name = SanitizeName(pkg.Name, "Mod")
	pkg.Version = NormalizeVersion(pkg.Version)
	return pkg, nil
}

var nonNameChars = regexp.MustCompile(`[^A-Za-z0-9_]+`)

// CleanTargetDir checks a folder a package is installed into: it has to be a
// path inside the game folder, and not one of the files Hermit keeps in a
// profile for itself.
func CleanTargetDir(dir string) (string, error) {
	dir = strings.TrimSpace(strings.ReplaceAll(dir, "\\", "/"))
	clean := path.Clean(strings.Trim(dir, "/"))
	if strings.HasPrefix(dir, "/") || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%q is not a folder inside the game; use a path like BepInEx/plugins/models", dir)
	}
	top, _, _ := strings.Cut(clean, "/")
	for _, reserved := range []string{"profile.json", "disabled"} {
		if strings.EqualFold(top, reserved) {
			return "", fmt.Errorf("%q is used by Hermit itself", top)
		}
	}
	return clean, nil
}

// SanitizeName turns text into a valid Thunderstore-style name.
func SanitizeName(s, fallback string) string {
	s = strings.Trim(nonNameChars.ReplaceAllString(strings.TrimSpace(s), "_"), "_")
	if s == "" {
		return fallback
	}
	return s
}

var versionNumbers = regexp.MustCompile(`\d+`)

// NormalizeVersion makes "v1.2", "1.2.3.4" or "2.0-beta" a three-part version.
func NormalizeVersion(s string) string {
	nums := versionNumbers.FindAllString(s, 3)
	for len(nums) < 3 {
		nums = append(nums, "0")
	}
	for i, n := range nums {
		nums[i] = strings.TrimLeft(n, "0")
		if nums[i] == "" {
			nums[i] = "0"
		}
	}
	return strings.Join(nums, ".")
}

// PlanFile reports what installing a mod file would change.
func (in *Installer) PlanFile(ctx context.Context, gameID, profileID string, pkg LocalPackage) (InstallPlan, error) {
	profile, err := in.lib.GetProfile(gameID, profileID)
	if err != nil {
		return InstallPlan{}, err
	}
	planned, cleanup, err := in.planFile(ctx, gameID, pkg, nil)
	if err != nil {
		return InstallPlan{}, err
	}
	defer cleanup()
	return describePlan(profile, []plannedPackage{planned}), nil
}

// InstallFile installs a local .zip or .dll as a mod, then installs its
// manifest dependencies that the profile lacks from Thunderstore.
func (in *Installer) InstallFile(ctx context.Context, gameID, profileID string, pkg LocalPackage, replaceConflicts bool, onProgress func(Progress)) (library.Profile, error) {
	source := library.ModSource{Type: library.SourceLocal, URL: filepath.Base(pkg.Path)}
	return in.installArchive(ctx, gameID, profileID, pkg, source, replaceConflicts, onProgress)
}

// installArchive installs a local package with the given source and then its dependencies.
func (in *Installer) installArchive(ctx context.Context, gameID, profileID string, pkg LocalPackage, source library.ModSource, replaceConflicts bool, onProgress func(Progress)) (library.Profile, error) {
	planned, cleanup, err := in.planFile(ctx, gameID, pkg, &source)
	if err != nil {
		return library.Profile{}, err
	}
	defer cleanup()

	profile, err := in.installPlanned(gameID, profileID, planned, replaceConflicts)
	if err != nil {
		return library.Profile{}, err
	}

	// Dependencies are installed one by one after the lock above is released.
	var errs []error
	for _, dep := range planned.manifest.Dependencies {
		ref, err := thunderstore.ParseDependency(dep)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if slices.ContainsFunc(profile.Mods, func(m library.Mod) bool {
			return m.ID == ref.ID() && CompareVersions(m.Version, ref.Version) >= 0
		}) || (planned.rules.IsLoaderPackage(ref.ID()) && slices.ContainsFunc(profile.Mods, func(m library.Mod) bool {
			return planned.rules.IsLoaderPackage(m.ID) || IsLoader(m.Files)
		})) {
			continue
		}
		if profile, err = in.Install(ctx, gameID, profileID, ref, Options{}, onProgress); err != nil {
			errs = append(errs, fmt.Errorf("dependency %s: %w", dep, err))
			if profile, err = in.lib.GetProfile(gameID, profileID); err != nil {
				return library.Profile{}, err
			}
		}
	}
	return profile, errors.Join(errs...)
}

// installPlanned installs one prepared package under the profile lock.
func (in *Installer) installPlanned(gameID, profileID string, planned plannedPackage, replaceConflicts bool) (library.Profile, error) {
	lock := in.profileLock(gameID, profileID)
	lock.Lock()
	defer lock.Unlock()

	profile, err := in.lib.GetProfile(gameID, profileID)
	if err != nil {
		return library.Profile{}, err
	}
	profileDir, err := in.lib.ProfileDir(gameID, profileID)
	if err != nil {
		return library.Profile{}, err
	}
	var replace []string
	for _, c := range findConflicts(profile, []plannedPackage{planned}) {
		if !replaceConflicts {
			return library.Profile{}, fmt.Errorf("%s: %w: %s", c.Package, ErrConflicts, c.ModName)
		}
		replace = append(replace, c.ModID)
	}
	if len(replace) > 0 {
		if _, err := in.updateProfile(gameID, profileID, func(p *library.Profile) (error, error) {
			var errs []error
			for _, id := range replace {
				if i := slices.IndexFunc(p.Mods, func(m library.Mod) bool { return m.ID == id }); i >= 0 {
					errs = append(errs, removeModFiles(profileDir, p.Mods[i], p.Mods))
					p.Mods = slices.Delete(p.Mods, i, i+1)
				}
			}
			return errors.Join(append(errs, Sync(profileDir, p, planned.rules))...), nil
		}); err != nil {
			return library.Profile{}, err
		}
	}
	return in.installOne(gameID, profileID, profileDir, planned)
}

// planFile turns a local file into a package ready to install. A single DLL
// is wrapped into a temporary zip with a generated manifest.json, the way
// r2modman gives every mod a manifest.
func (in *Installer) planFile(ctx context.Context, gameID string, pkg LocalPackage, source *library.ModSource) (plannedPackage, func(), error) {
	noop := func() {}
	ref := thunderstore.PackageRef{Namespace: pkg.Author, Name: pkg.Name, Version: pkg.Version}
	if err := ref.Validate(); err != nil {
		return plannedPackage{}, noop, errors.New("author and name may only contain letters, digits and _, and the version must look like 1.2.3")
	}
	rules, err := in.rulesFor(ctx, gameID)
	if err != nil {
		return plannedPackage{}, noop, err
	}
	if pkg.TargetDir != "" {
		into, err := CleanTargetDir(pkg.TargetDir)
		if err != nil {
			return plannedPackage{}, noop, err
		}
		rules.Into = into
	}

	archivePath, cleanup := pkg.Path, noop
	manifest := thunderstore.Manifest{Name: pkg.Name, VersionNumber: pkg.Version, Description: pkg.Description, Dependencies: []string{}}
	switch strings.ToLower(filepath.Ext(pkg.Path)) {
	case ".dll":
		tmp, err := wrapDLL(pkg, manifest)
		if err != nil {
			return plannedPackage{}, noop, err
		}
		archivePath, cleanup = tmp, func() { os.Remove(tmp) }
	case ".zip":
		if m, err := thunderstore.ReadManifest(pkg.Path); err == nil {
			manifest.Dependencies = m.Dependencies
		}
	default:
		return plannedPackage{}, noop, ErrUnsupportedFile
	}

	sum, err := fileSHA256(pkg.Path)
	if err != nil {
		cleanup()
		return plannedPackage{}, noop, err
	}
	files, err := PlanFiles(archivePath, ref.ID(), rules)
	if err != nil {
		cleanup()
		return plannedPackage{}, noop, err
	}
	return plannedPackage{
		ref:      ref,
		archive:  thunderstore.Archive{Path: archivePath, SHA256: sum},
		manifest: manifest,
		rules:    rules,
		files:    files,
		source:   source,
	}, cleanup, nil
}

func wrapDLL(pkg LocalPackage, manifest thunderstore.Manifest) (string, error) {
	tmp, err := os.CreateTemp("", "hermit-*.zip")
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	zw := zip.NewWriter(tmp)
	manifestJSON, _ := json.MarshalIndent(map[string]any{
		"name": manifest.Name, "version_number": manifest.VersionNumber,
		"description": manifest.Description, "website_url": "", "dependencies": manifest.Dependencies,
	}, "", "    ")
	w, err := zw.Create("manifest.json")
	if err == nil {
		_, err = w.Write(manifestJSON)
	}
	if err == nil {
		err = addFileToZip(zw, path.Base(filepath.ToSlash(pkg.Path)), pkg.Path)
	}
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func addFileToZip(zw *zip.Writer, name, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, 64<<20))
}

func readZipManifest(f *zip.File) (thunderstore.Manifest, error) {
	data, err := readZipFile(f)
	if err != nil {
		return thunderstore.Manifest{}, err
	}
	var m thunderstore.Manifest
	err = json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &m)
	return m, err
}
