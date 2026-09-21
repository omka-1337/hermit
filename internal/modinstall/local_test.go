package modinstall

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"hermit/internal/github"
	"hermit/internal/library"
	"hermit/internal/plugininfo/testasm"
)

func TestNormalizeVersionAndSanitizeName(t *testing.T) {
	for in, want := range map[string]string{"v1.2": "1.2.0", "1.2.3.4": "1.2.3", "2.0-beta": "2.0.0", "": "0.0.0", "01.002.0": "1.2.0"} {
		if got := NormalizeVersion(in); got != want {
			t.Errorf("NormalizeVersion(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"Yippee tbh mod": "Yippee_tbh_mod", "  ": "X", "a-b.c": "a_b_c"} {
		if got := SanitizeName(in, "X"); got != want {
			t.Errorf("SanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInstallDLLFile(t *testing.T) {
	tmp := t.TempDir()
	lib, _ := library.New(filepath.Join(tmp, "lib"), nil)
	os.MkdirAll(filepath.Join(tmp, "game"), 0o755)
	game, _ := lib.AddGame("Game", filepath.Join(tmp, "game"))
	pid := game.ActiveProfile
	dir, _ := lib.ProfileDir(game.ID, pid)

	dll := filepath.Join(tmp, "YippeeMod.dll")
	os.WriteFile(dll, testasm.PluginDLL(testasm.Plugin{Class: "Plugin", GUID: "sunnobunno.YippeeMod", Name: "Yippee tbh mod", Version: "1.2.4.0"}), 0o644)

	pkg, err := InspectFile(dll)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Author != "sunnobunno" || pkg.Name != "Yippee_tbh_mod" || pkg.Version != "1.2.4" || len(pkg.Plugins) != 1 {
		t.Fatalf("inspect: %+v", pkg)
	}

	in := NewInstaller(lib, &fakeDownloader{t: t, dir: tmp}, nil)
	p, err := in.InstallFile(context.Background(), game.ID, pid, pkg, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := findMod(t, p, "sunnobunno-Yippee_tbh_mod")
	if m.Source.Type != library.SourceLocal || m.Source.URL != "YippeeMod.dll" || m.Source.SHA256 == "" || !m.Active {
		t.Errorf("mod: %+v", m)
	}
	if !slices.Equal(m.Files, []string{"BepInEx/plugins/sunnobunno-Yippee_tbh_mod/YippeeMod.dll", "BepInEx/plugins/sunnobunno-Yippee_tbh_mod/manifest.json"}) {
		t.Errorf("files: %v", m.Files)
	}
	if len(m.Plugins) != 1 || m.Plugins[0].GUID != "sunnobunno.YippeeMod" {
		t.Errorf("plugins: %+v", m.Plugins)
	}
	if !exists(filepath.Join(dir, "BepInEx/plugins/sunnobunno-Yippee_tbh_mod/YippeeMod.dll")) {
		t.Error("dll not installed")
	}

	bad := pkg
	bad.Name = "no spaces allowed"
	if _, err := in.InstallFile(context.Background(), game.ID, pid, bad, false, nil); err == nil {
		t.Error("invalid name must be rejected")
	}
}

func TestInstallZipFileWithThunderstoreDependency(t *testing.T) {
	tmp := t.TempDir()
	lib, _ := library.New(filepath.Join(tmp, "lib"), nil)
	os.MkdirAll(filepath.Join(tmp, "game"), 0o755)
	game, _ := lib.AddGame("Game", filepath.Join(tmp, "game"))
	pid := game.ActiveProfile

	zipPath := makeZip(t, tmp, map[string]string{
		"manifest.json":         `{"name": "CoolMod", "version_number": "0.3.0", "description": "local build", "dependencies": ["A-Lib-1.0.0"]}`,
		"BepInEx/plugins/C.dll": string(testasm.PluginDLL(testasm.Plugin{Class: "C", GUID: "me.cool", Name: "Cool", Version: "0.3.0"})),
	})
	pkg, err := InspectFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if !pkg.HasManifest || pkg.Name != "CoolMod" || pkg.Version != "0.3.0" || !slices.Equal(pkg.Dependencies, []string{"A-Lib-1.0.0"}) {
		t.Fatalf("inspect: %+v", pkg)
	}
	pkg.Author = "Me"

	in := NewInstaller(lib, &fakeDownloader{t: t, dir: tmp, packages: map[string][]string{"A-Lib-1.0.0": {}}}, nil)
	p, err := in.InstallFile(context.Background(), game.ID, pid, pkg, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ids := modVersions(p); !slices.Equal(ids, []string{"A-Lib@1.0.0", "Me-CoolMod@0.3.0"}) {
		t.Errorf("mods: %v", ids)
	}
	if m := findMod(t, p, "Me-CoolMod"); !m.Active || m.Source.Type != library.SourceLocal {
		t.Errorf("local mod: %+v", m)
	}

	if _, err := InspectFile(filepath.Join(tmp, "notes.txt")); err != ErrUnsupportedFile {
		t.Errorf("unsupported file: %v", err)
	}
}

type fakeGitHub struct {
	t       *testing.T
	dir     string
	release github.Release
	files   map[string][]byte // asset name -> content
}

func (f *fakeGitHub) LatestRelease(context.Context, string, string) (github.Release, error) {
	return f.release, nil
}

func (f *fakeGitHub) Download(_ context.Context, _, _, tag string, asset github.Asset, _ func(int64, int64)) (string, error) {
	p := filepath.Join(f.dir, tag+"-"+asset.Name)
	return p, os.WriteFile(p, f.files[asset.Name], 0o644)
}

func TestGitHubModInstallAndUpdate(t *testing.T) {
	tmp := t.TempDir()
	lib, _ := library.New(filepath.Join(tmp, "lib"), nil)
	os.MkdirAll(filepath.Join(tmp, "game"), 0o755)
	game, _ := lib.AddGame("Game", filepath.Join(tmp, "game"))
	pid := game.ActiveProfile
	ctx := context.Background()

	gh := &fakeGitHub{t: t, dir: tmp, files: map[string][]byte{
		"CoolMod.dll": testasm.PluginDLL(testasm.Plugin{Class: "P", GUID: "me.cool", Name: "Cool", Version: "1.0.0"}),
	}}
	in := NewInstaller(lib, &fakeDownloader{t: t, dir: tmp}, nil)
	in.SetGitHub(gh)

	first := filepath.Join(tmp, "v1-CoolMod.dll")
	os.WriteFile(first, gh.files["CoolMod.dll"], 0o644)
	pkg, _ := InspectFile(first)
	pkg.Author, pkg.Name, pkg.Version = "me", "CoolMod", "1.0.0"
	p, err := in.InstallGitHub(ctx, game.ID, pid, pkg, "me", "cool-mod", "v1.0.0", "CoolMod.dll", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m := findMod(t, p, "me-CoolMod"); m.Source.Type != library.SourceGitHub || m.Source.URL != "https://github.com/me/cool-mod" || m.Source.Release != "v1.0.0" {
		t.Fatalf("source: %+v", m.Source)
	}

	// Same release: no update.
	gh.release = github.Release{Tag: "v1.0.0", Assets: []github.Asset{{Name: "CoolMod.dll"}}}
	if updates, _ := in.CheckUpdates(ctx, game.ID, pid); len(updates) != 0 {
		t.Errorf("unexpected updates: %+v", updates)
	}

	gh.release = github.Release{Tag: "v1.1", Assets: []github.Asset{{Name: "CoolMod.dll"}, {Name: "source.tar.gz"}}}
	updates, _ := in.CheckUpdates(ctx, game.ID, pid)
	if !slices.Equal(updates, []Update{{ModID: "me-CoolMod", Current: "1.0.0", Latest: "1.1.0"}}) {
		t.Fatalf("updates: %+v", updates)
	}
	result, err := in.UpdateAll(ctx, game.ID, pid, nil)
	if err != nil || len(result.Failed) != 0 {
		t.Fatalf("update: %+v %v", result, err)
	}
	if m := findMod(t, result.Profile, "me-CoolMod"); m.Version != "1.1.0" || m.Source.Release != "v1.1" {
		t.Errorf("after update: %+v", m)
	}
}

func TestPickAsset(t *testing.T) {
	r := github.Release{Tag: "v2", Assets: []github.Asset{{Name: "A.zip"}, {Name: "B.dll"}, {Name: "notes.txt"}}}
	if a, err := PickAsset(r, "B.dll"); err != nil || a.Name != "B.dll" {
		t.Errorf("preferred: %v %v", a, err)
	}
	if _, err := PickAsset(r, "Gone.zip"); err == nil {
		t.Error("ambiguous release must fail")
	}
	one := github.Release{Assets: []github.Asset{{Name: "Mod.zip"}, {Name: "readme.md"}}}
	if a, err := PickAsset(one, ""); err != nil || a.Name != "Mod.zip" {
		t.Errorf("single: %v %v", a, err)
	}
}

func TestInstallIntoFolder(t *testing.T) {
	into := DefaultRules()
	into.Into = "BepInEx/plugins/models/all"

	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name:  "loose files, like model packs",
			files: map[string]string{"bfufu": "x", "wfufu": "x"},
			want:  []string{"BepInEx/plugins/models/all/bfufu", "BepInEx/plugins/models/all/wfufu"},
		},
		{
			name:  "one wrapping folder is only how it was zipped",
			files: map[string]string{"Furina/bfufu": "x", "Furina/sub/wfufu": "x"},
			want:  []string{"BepInEx/plugins/models/all/bfufu", "BepInEx/plugins/models/all/sub/wfufu"},
		},
		{
			name:  "layout inside the archive is kept",
			files: map[string]string{"a/one": "x", "b/two": "x", "manifest.json": "{}", "icon.png": "x"},
			want:  []string{"BepInEx/plugins/models/all/a/one", "BepInEx/plugins/models/all/b/two"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			zip := makeZip(t, t.TempDir(), tc.files)
			got, err := PlanFiles(zip, "Someone-Pack", into)
			if err != nil {
				t.Fatal(err)
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("planned %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCleanTargetDir(t *testing.T) {
	good := map[string]string{
		"BepInEx/plugins/models/all":  "BepInEx/plugins/models/all",
		" BepInEx/plugins/models/ ":   "BepInEx/plugins/models",
		`BepInEx\plugins\models`:      "BepInEx/plugins/models",
		"BepInEx/plugins/../config/x": "BepInEx/config/x",
	}
	for in, want := range good {
		if got, err := CleanTargetDir(in); err != nil || got != want {
			t.Errorf("CleanTargetDir(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", ".", "/etc", "../outside", "a/../../outside", "disabled/x", "Profile.json"} {
		if got, err := CleanTargetDir(bad); err == nil {
			t.Errorf("CleanTargetDir(%q) = %q, want an error", bad, got)
		}
	}
}
