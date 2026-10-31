package bethesda

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func makeZip(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), name)
	f, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	for n, content := range files {
		fw, _ := w.Create(n)
		fw.Write([]byte(content))
	}
	w.Close()
	f.Close()
	return file
}

func TestStageAndInstallPlainArchives(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			// SKSE: a Data folder, and files for the game folder beside it.
			"Skyrim Script Extender (SKSE64) Steam 30379 2.3.1 2026-08-27T16-52Z s6Og0dG94.7z.zip",
			map[string]string{"skse64_2_03_01/Data/Scripts/skse.pex": "", "skse64_2_03_01/skse64_loader.exe": "", "skse64_2_03_01/src/x.cpp": ""},
			[]string{"Root/skse64_loader.exe", "Scripts/skse.pex"},
		},
		{
			// Wrapped in a folder, with a readme beside it.
			"SkyUI-12604-6-11-1778020881.zip",
			map[string]string{"SkyUI/SkyUI_SE.esp": "", "SkyUI/interface/skyui.swf": "", "readme.txt": ""},
			[]string{"SkyUI_SE.esp", "interface/skyui.swf"},
		},
		{
			"Address Library.zip",
			map[string]string{"SKSE/Plugins/versionlib.bin": ""},
			[]string{"SKSE/Plugins/versionlib.bin"},
		},
	} {
		staged, err := Stage(makeZip(t, tc.name, tc.files), t.TempDir(), nil)
		if err != nil {
			t.Fatal(err)
		}
		mod := filepath.Join(t.TempDir(), "mod")
		if err := staged.Install(mod, nil); err != nil {
			t.Fatal(err)
		}
		var got []string
		filepath.WalkDir(mod, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				rel, _ := filepath.Rel(mod, p)
				got = append(got, filepath.ToSlash(rel))
			}
			return nil
		})
		slices.Sort(got)
		if !slices.Equal(got, tc.want) || staged.Unsure {
			t.Errorf("%s: installed %v (unsure %v), want %v", tc.name, got, staged.Unsure, tc.want)
		}
		staged.Remove()
	}
}

func TestStageFomodAndNames(t *testing.T) {
	config := `<config><moduleName>Fancy Mod</moduleName><requiredInstallFiles><folder source="core" destination=""/></requiredInstallFiles></config>`
	file := makeZip(t, "Fancy-123-1-2-1700000000.zip", map[string]string{
		"Fancy/fomod/ModuleConfig.xml": config,
		"Fancy/fomod/info.xml":         `<fomod><Name>Fancy</Name><Author>Someone</Author><Version>1.2.0</Version></fomod>`,
		"Fancy/core/Fancy.esp":         "",
	})
	staged, err := Stage(file, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Remove()
	if staged.Fomod == nil || staged.Name != "Fancy" || staged.Version != "1.2.0" || staged.Author != "Someone" || staged.NexusID != 123 {
		t.Fatalf("staged %+v", staged)
	}
	mod := filepath.Join(t.TempDir(), "mod")
	if err := staged.Install(mod, staged.Fomod.Evaluate(nil, nil).Files); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mod, "Fancy.esp")); err != nil {
		t.Error(err)
	}

	for file, want := range map[string]string{
		"SkyUI-12604-6-11-1778020881.zip":                               "SkyUI",
		"Lux (main)-43158-7-0-1753427726.rar":                           "Lux (main)",
		"Community Shaders 86492 1.9.1 2026-09-24T12-49Z lNkz9LIZ8.zip": "Community Shaders",
		"OStim Standalone 7.0.7z":                                       "OStim Standalone 7.0",
	} {
		if got := NameFromFile(file); got != want {
			t.Errorf("%s: %q, want %q", file, got, want)
		}
	}
}
