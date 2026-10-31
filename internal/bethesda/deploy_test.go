package bethesda

import (
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
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
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

func setup(t *testing.T) (game, profile string) {
	game, profile = t.TempDir(), t.TempDir()
	write(t, game, map[string]string{
		"SkyrimSE.exe":              "game",
		"skse64_loader.exe":         "the user's own",
		"Data/Skyrim.esm":           "master",
		"Data/Textures/vanilla.dds": "vanilla loose file",
		"Data/Scripts/actor.pex":    "game script",
	})
	write(t, profile, map[string]string{
		"mods/low/textures/sky.dds":        "low",
		"mods/low/textures/ground.dds":     "low",
		"mods/low/Low.esp":                 "plugin",
		"mods/low/meta.ini":                "mo2",
		"mods/high/Textures/SKY.dds":       "high",
		"mods/high/scripts/actor.pex":      "mod script",
		"mods/high/High.esm":               "plugin",
		"mods/skse/Root/skse64_loader.exe": "skse",
		"mods/skse/Root/skse64_1_7.dll":    "skse",
		"mods/off/textures/sky.dds":        "disabled",
		"overwrite/textures/ground.dds":    "overwrite",
	})
	return game, profile
}

func TestDeployOrderCaseAndGameFiles(t *testing.T) {
	game, profile := setup(t)
	s, err := Deploy(game, profile, []string{"low", "high", "skse"}, Prefix{})
	if err != nil {
		t.Fatal(err)
	}

	for rel, want := range map[string]string{
		"Data/Textures/SKY.dds":     "high",      // the later mod wins, under the game's own spelling
		"Data/Textures/ground.dds":  "overwrite", // overwrite comes after every mod
		"Data/Textures/vanilla.dds": "vanilla loose file",
		"Data/Scripts/actor.pex":    "game script", // the game's own file stays
		"Data/Low.esp":              "plugin",
		"skse64_1_7.dll":            "skse",
		"skse64_loader.exe":         "the user's own",
	} {
		if got := read(t, filepath.Join(game, rel)); got != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(game, "Data", "textures")); err == nil {
		t.Error("a second textures folder was made next to Textures")
	}
	if _, err := os.Stat(filepath.Join(game, "Data", "meta.ini")); err == nil {
		t.Error("MO2's meta.ini was deployed")
	}
	slices.Sort(s.Hidden)
	if want := []string{"Data/Scripts/actor.pex", "skse64_loader.exe"}; !slices.Equal(s.Hidden, want) {
		t.Errorf("hidden = %v, want %v", s.Hidden, want)
	}

	if _, err := Undeploy(game, s); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"Data/Textures/SKY.dds", "Data/Low.esp", "skse64_1_7.dll"} {
		if _, err := os.Lstat(filepath.Join(game, rel)); !os.IsNotExist(err) {
			t.Errorf("%s is still there after the session", rel)
		}
	}
	if got := read(t, filepath.Join(game, "Data", "Textures", "vanilla.dds")); got != "vanilla loose file" {
		t.Errorf("the game's file changed: %q", got)
	}
}

func TestUndeployCapturesFilesWrittenDuringSession(t *testing.T) {
	game, profile := setup(t)
	s, err := Deploy(game, profile, []string{"low"}, Prefix{})
	if err != nil {
		t.Fatal(err)
	}
	// A shader cache in a new folder, and a tool replacing a linked file.
	write(t, game, map[string]string{"Data/ShaderCache/a.bin": "cache"})
	sky := filepath.Join(game, "Data", "Textures", "sky.dds")
	if err := os.Remove(sky); err != nil {
		t.Fatal(err)
	}
	write(t, game, map[string]string{"Data/Textures/sky.dds": "rebuilt"})

	captured, err := Undeploy(game, s)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(captured)
	if want := []string{"ShaderCache/a.bin", "Textures/sky.dds"}; !slices.Equal(captured, want) {
		t.Fatalf("captured %v, want %v", captured, want)
	}
	if got := read(t, filepath.Join(profile, "overwrite", "ShaderCache", "a.bin")); got != "cache" {
		t.Errorf("overwrite has %q", got)
	}
	if _, err := os.Stat(filepath.Join(game, "Data", "ShaderCache")); !os.IsNotExist(err) {
		t.Error("the folder made during the session is still in Data")
	}
	if got := read(t, filepath.Join(profile, "mods", "low", "textures", "sky.dds")); got != "low" {
		t.Errorf("the mod's own file changed: %q", got)
	}
}

func TestDeployReplacesLinksLeftByUnfinishedSession(t *testing.T) {
	game, profile := setup(t)
	if _, err := Deploy(game, profile, []string{"low"}, Prefix{}); err != nil {
		t.Fatal(err)
	}
	// The session never ended; the next one starts over the old links.
	s, err := Deploy(game, profile, []string{"low", "high"}, Prefix{})
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(game, "Data", "Textures", "sky.dds")); got != "high" {
		t.Errorf("sky.dds = %q, want high", got)
	}
	if len(s.Hidden) != 1 {
		t.Errorf("old links were taken for game files: %v", s.Hidden)
	}
}

func TestDeploySwapsPlugins(t *testing.T) {
	game, profile := setup(t)
	write(t, profile, map[string]string{"plugins.txt": "# comment\r\n*Gone.esp\r\nHigh.esm\r\n"})
	prefixList := filepath.Join(t.TempDir(), "Plugins.txt")
	write(t, filepath.Dir(prefixList), map[string]string{"Plugins.txt": "*Theirs.esp\r\n"})

	s, err := Deploy(game, profile, []string{"low", "high"}, Prefix{Plugins: prefixList})
	if err != nil {
		t.Fatal(err)
	}
	given, _ := ReadPlugins(prefixList)
	if want := []Plugin{{"High.esm", false}, {"Low.esp", true}}; !slices.Equal(given, want) {
		t.Errorf("game is given %v, want %v", given, want)
	}
	kept, _ := ReadPlugins(filepath.Join(profile, "plugins.txt"))
	if want := []Plugin{{"Gone.esp", true}, {"High.esm", false}, {"Low.esp", true}}; !slices.Equal(kept, want) {
		t.Errorf("profile keeps %v, want %v", kept, want)
	}

	if _, err := Undeploy(game, s); err != nil {
		t.Fatal(err)
	}
	if got := read(t, prefixList); got != "*Theirs.esp\r\n" {
		t.Errorf("plugins.txt after the session = %q", got)
	}
}

func TestConflicts(t *testing.T) {
	_, profile := setup(t)
	got, err := Conflicts(profile, []string{"low", "high", "skse", "off", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	// low and high share textures/sky.dds (in different letter case); off
	// has sky.dds too. meta.ini is MO2's, not a file of the mod.
	want := map[string]map[string]int{
		"low":  {"high": 1, "off": 1},
		"high": {"low": 1, "off": 1},
		"off":  {"low": 1, "high": 1},
	}
	if len(got) != len(want) {
		t.Fatalf("conflicts %v, want %v", got, want)
	}
	for a, others := range want {
		for b, n := range others {
			if got[a][b] != n {
				t.Errorf("%s/%s = %d, want %d (all: %v)", a, b, got[a][b], n, got)
			}
		}
	}
}

func TestExtenderLogs(t *testing.T) {
	library := t.TempDir()
	game := filepath.Join(library, "steamapps", "common", "Skyrim Special Edition")
	write(t, library, map[string]string{"steamapps/common/Skyrim Special Edition/": "", "steamapps/compatdata/489830/": ""})
	compatData := SteamCompatData(game, "489830")
	if compatData != filepath.Join(library, "steamapps", "compatdata", "489830") {
		t.Fatalf("compatdata %q", compatData)
	}
	if SteamCompatData(game, "1") != "" || SteamCompatData(game, "") != "" {
		t.Error("found compatdata for another game")
	}
	g, _ := Find("SkyrimSE.exe")
	want := filepath.Join(compatData, "pfx/drive_c/users/steamuser/Documents/My Games/Skyrim Special Edition/SKSE")
	if got := ExtenderLogs(compatData, g); got != want {
		t.Errorf("logs in %s, want %s", got, want)
	}
}

func TestDeploySwapsINIs(t *testing.T) {
	game, profile := setup(t)
	write(t, profile, map[string]string{"ini/Skyrim.ini": "[Papyrus]\r\nbEnableLogging=1\r\n", "ini/SkyrimCustom.ini": "custom"})
	myGames := t.TempDir()
	write(t, myGames, map[string]string{"skyrim.ini": "the game's"})

	s, err := Deploy(game, profile, nil, Prefix{MyGames: myGames})
	if err != nil {
		t.Fatal(err)
	}
	// The game's file keeps its spelling; the one it lacked is added.
	if got := read(t, filepath.Join(myGames, "skyrim.ini")); got != "[Papyrus]\r\nbEnableLogging=1\r\n" {
		t.Errorf("game plays with %q", got)
	}
	if got := read(t, filepath.Join(myGames, "SkyrimCustom.ini")); got != "custom" {
		t.Errorf("custom ini %q", got)
	}

	// Settings changed in the game's menus.
	write(t, myGames, map[string]string{"skyrim.ini": "changed in game"})
	if _, err := Undeploy(game, s); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(profile, "ini", "Skyrim.ini")); got != "changed in game" {
		t.Errorf("profile keeps %q", got)
	}
	if got := read(t, filepath.Join(myGames, "skyrim.ini")); got != "the game's" {
		t.Errorf("game's own ini after the session: %q", got)
	}
	if _, err := os.Stat(filepath.Join(myGames, "SkyrimCustom.ini")); !os.IsNotExist(err) {
		t.Error("the ini the game did not have is still there")
	}
}
