package deploy

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// An Unreal Engine 3 game, whose mods take the place of its packages.
func TestDeployReplacesGameFiles(t *testing.T) {
	game, profile := t.TempDir(), t.TempDir()
	write(t, game, map[string]string{
		"Binaries/Win64/BatmanAK.exe":            "game",
		"BmGame/CookedPCConsole/Batman_Skin.upk": "original skin",
		"BmGame/CookedPCConsole/Other.upk":       "untouched",
	})
	write(t, profile, map[string]string{
		"mods/skin/BmGame/CookedPCConsole/batman_skin.upk": "modded skin",
		"mods/loader/Binaries/Win64/dinput8.dll":           "loader",
	})
	target := Target{Replace: true}
	s, err := Deploy(game, profile, []string{"skin", "loader"}, target)
	if err != nil {
		t.Fatal(err)
	}
	skin := filepath.Join(game, "BmGame/CookedPCConsole/Batman_Skin.upk")
	if got := read(t, skin); got != "modded skin" {
		t.Errorf("the game reads %q", got)
	}
	if got := read(t, skin+OriginalSuffix); got != "original skin" {
		t.Errorf("set aside %q", got)
	}
	if len(s.Replaced) != 1 || len(s.Hidden) != 0 {
		t.Errorf("replaced %v, hidden %v", s.Replaced, s.Hidden)
	}

	// The game writes a log during the session.
	write(t, game, map[string]string{"Binaries/Win64/loader.log": "log"})
	captured, err := Undeploy(game, target, s)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, skin); got != "original skin" {
		t.Errorf("after the session the game has %q", got)
	}
	if _, err := os.Lstat(skin + OriginalSuffix); !os.IsNotExist(err) {
		t.Errorf("the set aside file is still there: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(game, "Binaries/Win64/dinput8.dll")); !os.IsNotExist(err) {
		t.Errorf("the loader is still in the game: %v", err)
	}
	if !slices.Equal(captured, []string{"Binaries/Win64/loader.log"}) || read(t, filepath.Join(profile, "overwrite/Binaries/Win64/loader.log")) != "log" {
		t.Errorf("captured %v", captured)
	}
}

// Without Replace the game's files stay, and with a Data folder a Root
// folder means the game folder; without one it is a folder like any other.
func TestDeployKeepsGameFilesAndRoot(t *testing.T) {
	game, profile := t.TempDir(), t.TempDir()
	write(t, game, map[string]string{"bin/x64/Game.exe": "game", "engine/config/base.ini": "game's"})
	write(t, profile, map[string]string{
		"mods/a/engine/config/base.ini": "mod's",
		"mods/a/root/notes.txt":         "a folder of the mod",
	})
	s, err := Deploy(game, profile, []string{"a"}, Target{})
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(game, "engine/config/base.ini")); got != "game's" {
		t.Errorf("the game's file was replaced: %q", got)
	}
	if !slices.Equal(s.Hidden, []string{"engine/config/base.ini"}) {
		t.Errorf("hidden %v", s.Hidden)
	}
	if got := read(t, filepath.Join(game, "root/notes.txt")); got != "a folder of the mod" {
		t.Errorf("root/notes.txt = %q", got)
	}
	if _, err := Undeploy(game, Target{}, s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(game, "root")); !os.IsNotExist(err) {
		t.Errorf("the mod's folder stayed in the game: %v", err)
	}
}

func makeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "mod.zip")
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

func TestShapeLayout(t *testing.T) {
	cyberpunk := Shape{
		Folders: []string{"archive", "bin", "r6"},
		Loose:   map[string]string{".archive": "archive/pc/mod", ".reds": "r6/scripts"},
		Within:  map[string]string{"cyber_engine_tweaks": "bin/x64/plugins/cyber_engine_tweaks"},
	}
	arkham := Shape{
		Folders: []string{"BmGame", "Binaries"},
		Loose:   map[string]string{".upk": "BmGame/CookedPCConsole"},
		Within:  map[string]string{"CookedPCConsole": "BmGame/CookedPCConsole"},
	}
	for _, tc := range []struct {
		name   string
		shape  Shape
		files  map[string]string
		want   []string
		unsure bool
	}{
		{"laid out as the game folder, wrapped", cyberpunk,
			map[string]string{"Mod/archive/pc/mod/a.archive": "", "Mod/r6/scripts/b.reds": "", "readme.txt": ""},
			[]string{"archive/pc/mod/a.archive", "r6/scripts/b.reds"}, false},
		{"loose archives", cyberpunk,
			map[string]string{"a.archive": "", "b.archive": "", "readme.md": ""},
			[]string{"archive/pc/mod/a.archive", "archive/pc/mod/b.archive"}, false},
		{"a Cyber Engine Tweaks folder", cyberpunk,
			map[string]string{"cyber_engine_tweaks/mods/x/init.lua": ""},
			[]string{"bin/x64/plugins/cyber_engine_tweaks/mods/x/init.lua"}, false},
		{"loose packages", arkham,
			map[string]string{"Skin/Batman_Skin.upk": ""},
			[]string{"BmGame/CookedPCConsole/Batman_Skin.upk"}, false},
		{"from CookedPCConsole", arkham,
			map[string]string{"CookedPCConsole/Batman_Skin.upk": ""},
			[]string{"BmGame/CookedPCConsole/Batman_Skin.upk"}, false},
		{"nothing known", arkham,
			map[string]string{"stuff/thing.bin": ""},
			// What it is wrapped in goes, as for the Creation Engine.
			[]string{"thing.bin"}, true},
	} {
		staged, err := Stage(makeZip(t, tc.files), t.TempDir(), tc.shape.Layout, nil)
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
		if !slices.Equal(got, tc.want) || staged.Unsure != tc.unsure {
			t.Errorf("%s: installed %v (unsure %v), want %v (unsure %v)", tc.name, got, staged.Unsure, tc.want, tc.unsure)
		}
		if strings.Contains(strings.Join(got, " "), "readme") {
			t.Errorf("%s: installed the readme", tc.name)
		}
		staged.Remove()
	}
}
