package library

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestLibrary(t *testing.T) *Library {
	t.Helper()
	lib, err := New(filepath.Join(t.TempDir(), "lib"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func fakeGame(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		p := filepath.Join(dir, f)
		if strings.HasSuffix(f, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAddGameCreatesDefaultProfile(t *testing.T) {
	lib := newTestLibrary(t)
	path := fakeGame(t, "Valheim_Data/", "Valheim.exe", "UnityPlayer.dll")

	g, err := lib.AddGame("Valheim", path)
	if err != nil {
		t.Fatal(err)
	}
	if g.ID != "valheim" || g.Runtime != RuntimeProton {
		t.Fatalf("unexpected game: %+v", g)
	}

	profiles, err := lib.ListProfiles(g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0].ID != g.ActiveProfile || profiles[0].Name != DefaultProfileName {
		t.Fatalf("unexpected profiles: %+v (active %q)", profiles, g.ActiveProfile)
	}
	for _, sub := range []string{"plugins", "patchers", "config"} {
		if _, err := os.Stat(filepath.Join(lib.profileDir(g.ID, g.ActiveProfile), "BepInEx", sub)); err != nil {
			t.Errorf("missing BepInEx/%s: %v", sub, err)
		}
	}

	entries, _ := os.ReadDir(path)
	if len(entries) != 3 {
		t.Errorf("game directory was modified: %v", entries)
	}
}

func TestAddGameValidation(t *testing.T) {
	lib := newTestLibrary(t)
	path := fakeGame(t)

	if _, err := lib.AddGame("", path); !errors.Is(err, ErrEmptyName) {
		t.Errorf("empty name: got %v", err)
	}
	if _, err := lib.AddGame("X", filepath.Join(path, "missing")); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("missing path: got %v", err)
	}
	if _, err := lib.AddGame("X", path); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.AddGame("Y", path); !errors.Is(err, ErrAlreadyExists) {
		t.Errorf("duplicate path: got %v", err)
	}
}

func TestIDsAreUniqueAndStableAcrossRename(t *testing.T) {
	lib := newTestLibrary(t)
	a, _ := lib.AddGame("My Game", fakeGame(t))
	b, _ := lib.AddGame("My Game", fakeGame(t))
	c, _ := lib.AddGame("Гра", fakeGame(t))
	if a.ID != "my-game" || b.ID != "my-game-2" || c.ID != "game" {
		t.Fatalf("ids: %q %q %q", a.ID, b.ID, c.ID)
	}

	renamed, err := lib.RenameGame(a.ID, "Other")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := lib.GetGame(a.ID)
	if renamed.ID != a.ID || got.Name != "Other" {
		t.Fatalf("rename: %+v", got)
	}

	games, _ := lib.ListGames()
	if len(games) != 3 || games[0].Name != "My Game" || games[2].Name != "Гра" {
		t.Fatalf("list: %+v", games)
	}
}

func TestRemoveProfile(t *testing.T) {
	lib := newTestLibrary(t)
	g, _ := lib.AddGame("Game", fakeGame(t))

	if err := lib.RemoveProfile(g.ID, g.ActiveProfile); !errors.Is(err, ErrLastProfile) {
		t.Fatalf("last profile: got %v", err)
	}

	second, err := lib.CreateProfile(g.ID, "Modded")
	if err != nil {
		t.Fatal(err)
	}
	if err := lib.RemoveProfile(g.ID, g.ActiveProfile); err != nil {
		t.Fatal(err)
	}
	g, _ = lib.GetGame(g.ID)
	if g.ActiveProfile != second.ID {
		t.Fatalf("active profile not switched: %q", g.ActiveProfile)
	}
}

func TestSetActiveProfileAndRemoveGame(t *testing.T) {
	lib := newTestLibrary(t)
	g, _ := lib.AddGame("Game", fakeGame(t))

	if _, err := lib.SetActiveProfile(g.ID, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown profile: got %v", err)
	}
	p, _ := lib.CreateProfile(g.ID, "Second")
	if g, _ = lib.SetActiveProfile(g.ID, p.ID); g.ActiveProfile != p.ID {
		t.Errorf("active: %q", g.ActiveProfile)
	}

	if err := lib.RemoveGame(g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.GetGame(g.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("after remove: got %v", err)
	}
}

func TestRejectsPathTraversalIDs(t *testing.T) {
	lib := newTestLibrary(t)
	for _, id := range []string{"", ".", "..", "../x", `a\b`} {
		if _, err := lib.GetGame(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetGame(%q): got %v", id, err)
		}
	}
}

func TestInspectGamePath(t *testing.T) {
	lib := newTestLibrary(t)
	path := fakeGame(t, "Lethal Company_Data/Managed/", "Lethal Company.exe", "UnityCrashHandler64.exe")

	c, err := lib.InspectGamePath(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Lethal Company" || !c.Detection.Unity || c.AlreadyAdded {
		t.Fatalf("candidate: %+v", c)
	}

	if _, err := lib.AddGame(c.Name, path); err != nil {
		t.Fatal(err)
	}
	if c, _ = lib.InspectGamePath(path); !c.AlreadyAdded {
		t.Error("expected AlreadyAdded")
	}

	other := fakeGame(t)
	if c, _ = lib.InspectGamePath(other); c.Name != filepath.Base(other) || c.Detection.Unity {
		t.Errorf("non-game folder: %+v", c)
	}
}

func TestDetectGame(t *testing.T) {
	cases := []struct {
		files []string
		want  Detection
	}{
		{
			[]string{"Game_Data/Managed/", "Game.exe", "UnityPlayer.dll", "UnityCrashHandler64.exe"},
			Detection{Unity: true, Runtime: RuntimeProton, Backend: BackendMono, Executable: "Game.exe"},
		},
		{
			[]string{"Game_Data/il2cpp_data/", "Game.exe", "GameAssembly.dll", "UnityPlayer.dll"},
			Detection{Unity: true, Runtime: RuntimeProton, Backend: BackendIL2CPP, Executable: "Game.exe"},
		},
		{
			[]string{"Game_Data/Managed/", "Game.x86_64", "UnityPlayer.so"},
			Detection{Unity: true, Runtime: RuntimeNative, Backend: BackendMono, Executable: "Game.x86_64"},
		},
		{
			// Old Unity without UnityPlayer.dll.
			[]string{"Old_Data/Managed/", "Old.exe"},
			Detection{Unity: true, Runtime: RuntimeProton, Backend: BackendMono, Executable: "Old.exe"},
		},
		{
			// A _Data folder alone is not enough.
			[]string{"Other_Data/", "Other.exe"},
			Detection{Runtime: RuntimeUnknown, Backend: BackendUnknown},
		},
		{
			[]string{"readme.txt"},
			Detection{Runtime: RuntimeUnknown, Backend: BackendUnknown},
		},
	}
	for _, tc := range cases {
		if got := DetectGame(fakeGame(t, tc.files...)); got != tc.want {
			t.Errorf("%v:\n got  %+v\n want %+v", tc.files, got, tc.want)
		}
	}
}

func TestDiscoverGamesFromSteam(t *testing.T) {
	steamRoot := t.TempDir()
	apps := filepath.Join(steamRoot, "steamapps")
	writeFile := func(rel, content string) {
		p := filepath.Join(apps, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := func(id, name, dir string) string {
		return `"AppState" { "appid" "` + id + `" "name" "` + name + `" "installdir" "` + dir + `" }`
	}
	writeFile("appmanifest_1966720.acf", manifest("1966720", "Lethal Company", "Lethal Company"))
	writeFile("common/Lethal Company/Lethal Company.exe", "")
	writeFile("common/Lethal Company/UnityPlayer.dll", "")
	writeFile("common/Lethal Company/Lethal Company_Data/Managed/Assembly-CSharp.dll", "")
	writeFile("appmanifest_1070560.acf", manifest("1070560", "Steam Linux Runtime", "SteamLinuxRuntime"))
	writeFile("common/SteamLinuxRuntime/run.sh", "")

	lib, err := New(filepath.Join(t.TempDir(), "lib"), []string{steamRoot})
	if err != nil {
		t.Fatal(err)
	}
	found, err := lib.DiscoverGames()
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].SteamAppID != "1966720" || found[0].Detection.Backend != BackendMono {
		t.Fatalf("discovered: %+v", found)
	}

	g, err := lib.AddGame(found[0].Name, found[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if g.SteamAppID != "1966720" || g.Runtime != RuntimeProton || g.Executable != "Lethal Company.exe" {
		t.Fatalf("added: %+v", g)
	}
	if found, _ = lib.DiscoverGames(); !found[0].AlreadyAdded {
		t.Error("expected AlreadyAdded after adding")
	}
}

func TestProfileSchemaMigration(t *testing.T) {
	lib := newTestLibrary(t)
	g, _ := lib.AddGame("Game", fakeGame(t))
	legacy := `{"schemaVersion": 1, "name": "Default", "mods": [
		{"id": "a-On", "enabled": true}, {"id": "b-Off", "enabled": false}]}`
	os.WriteFile(filepath.Join(lib.profileDir(g.ID, g.ActiveProfile), "profile.json"), []byte(legacy), 0o644)

	p, err := lib.GetProfile(g.ID, g.ActiveProfile)
	if err != nil {
		t.Fatal(err)
	}
	if p.SchemaVersion != ProfileSchemaVersion || !p.Mods[0].Active || p.Mods[1].Active {
		t.Fatalf("migrated: %+v", p)
	}
}

func TestSetLaunchExecutable(t *testing.T) {
	lib := newTestLibrary(t)
	path := fakeGame(t, "Game.exe", "Game_Data/", "UnityPlayer.dll", "Tools/", "Tools/Launcher.exe")
	g, err := lib.AddGame("Game", path)
	if err != nil {
		t.Fatal(err)
	}

	g, err = lib.SetLaunchExecutable(g.ID, "Tools/Launcher.exe")
	if err != nil {
		t.Fatalf("SetLaunchExecutable: %v", err)
	}
	if g.LaunchExecutable != "Tools/Launcher.exe" {
		t.Errorf("LaunchExecutable = %q", g.LaunchExecutable)
	}
	// Stored, not only returned.
	if saved, _ := lib.GetGame(g.ID); saved.LaunchExecutable != "Tools/Launcher.exe" {
		t.Errorf("saved LaunchExecutable = %q", saved.LaunchExecutable)
	}

	// Choosing the game's own executable goes back to the default.
	g, err = lib.SetLaunchExecutable(g.ID, g.Executable)
	if err != nil || g.LaunchExecutable != "" {
		t.Errorf("back to default: %q, %v", g.LaunchExecutable, err)
	}
}

func TestSetLaunchExecutableStaysInsideTheGame(t *testing.T) {
	lib := newTestLibrary(t)
	path := fakeGame(t, "Game.exe", "Game_Data/", "UnityPlayer.dll")
	g, err := lib.AddGame("Game", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, exe := range []string{"../outside.exe", "/usr/bin/true", "Missing.exe", "Game_Data"} {
		if _, err := lib.SetLaunchExecutable(g.ID, exe); err == nil {
			t.Errorf("SetLaunchExecutable(%q) was accepted", exe)
		}
	}
}

func TestCopyProfile(t *testing.T) {
	lib := newTestLibrary(t)
	g, _ := lib.AddGame("Game", fakeGame(t))
	source, err := lib.UpdateProfile(g.ID, g.ActiveProfile, func(p *Profile) error {
		p.Mods = []Mod{{ID: "A-Mod", Name: "Mod", Author: "A", Version: "1.0.0", Enabled: true, Active: true}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := lib.ProfileDir(g.ID, source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "BepInEx", "config", "Mod.cfg"), []byte("Volume = 3"), 0o644); err != nil {
		t.Fatal(err)
	}

	copied, err := lib.CopyProfile(g.ID, source.ID, "Experiment")
	if err != nil {
		t.Fatal(err)
	}
	if copied.ID == source.ID || copied.Name != "Experiment" {
		t.Fatalf("copy has id %q name %q", copied.ID, copied.Name)
	}
	if len(copied.Mods) != 1 || copied.Mods[0].ID != "A-Mod" {
		t.Fatalf("mods not copied: %+v", copied.Mods)
	}
	copyDir, err := lib.ProfileDir(g.ID, copied.ID)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := os.ReadFile(filepath.Join(copyDir, "BepInEx", "config", "Mod.cfg"))
	if err != nil || string(cfg) != "Volume = 3" {
		t.Fatalf("config not copied: %q %v", cfg, err)
	}

	// The original keeps its own files: changing the copy leaves it alone.
	if _, err := lib.RenameProfile(g.ID, copied.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	if again, _ := lib.GetProfile(g.ID, source.ID); again.Name != source.Name {
		t.Fatalf("source renamed to %q", again.Name)
	}
}
