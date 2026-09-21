package launch

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"hermit/internal/library"
	"hermit/internal/modinstall"
	"hermit/internal/plugininfo"
)

func mkfile(t *testing.T, path string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLinkProfile(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	profile := filepath.Join(dataRoot, "games/g/profiles/default")
	game := filepath.Join(root, "game")
	mkfile(t, filepath.Join(profile, "BepInEx/core/BepInEx.dll"))
	mkfile(t, filepath.Join(profile, "winhttp.dll"))
	mkfile(t, filepath.Join(profile, "doorstop_config.ini"))
	mkfile(t, filepath.Join(profile, "profile.json"))
	mkfile(t, filepath.Join(profile, "disabled/x/y.dll"))
	mkfile(t, filepath.Join(game, "Game.exe"))

	links, err := LinkProfile(game, profile, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(links, []string{"BepInEx", "doorstop_config.ini", "winhttp.dll"}) {
		t.Fatalf("links: %v", links)
	}
	if _, err := os.Stat(filepath.Join(game, "BepInEx/core/BepInEx.dll")); err != nil {
		t.Error("BepInEx not reachable through link")
	}

	// Stale links from a crashed session are replaced.
	if _, err := LinkProfile(game, profile, dataRoot); err != nil {
		t.Errorf("relink: %v", err)
	}

	if err := Unlink(game, links, dataRoot); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(game)
	if len(entries) != 1 {
		t.Errorf("game dir not clean: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(profile, "winhttp.dll")); err != nil {
		t.Error("unlink removed profile files")
	}
}

func TestLinkProfileConflictsAndVanilla(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "data")
	profile := filepath.Join(dataRoot, "p")
	game := filepath.Join(root, "game")
	mkfile(t, filepath.Join(profile, "BepInEx/plugins/a.dll"))

	// No active loader: nothing is linked.
	if links, err := LinkProfile(game, profile, dataRoot); err != nil || len(links) != 0 {
		t.Fatalf("vanilla: %v %v", links, err)
	}

	mkfile(t, filepath.Join(profile, "winhttp.dll"))
	mkfile(t, filepath.Join(game, "BepInEx/manual.dll")) // manual BepInEx install
	_, err := LinkProfile(game, profile, dataRoot)
	if !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "BepInEx") {
		t.Fatalf("conflict: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(game, "winhttp.dll")); err == nil {
		t.Error("nothing must be linked on conflict")
	}
	// Unlink never removes real files.
	Unlink(game, []string{"BepInEx"}, dataRoot)
	if _, err := os.Stat(filepath.Join(game, "BepInEx/manual.dll")); err != nil {
		t.Error("real files removed")
	}
}

func TestWithDLLOverride(t *testing.T) {
	cases := map[string]string{
		"":                                "WINEDLLOVERRIDES=winhttp=n,b",
		"WINEDLLOVERRIDES=d3d11=n":        "WINEDLLOVERRIDES=d3d11=n;winhttp=n,b",
		"WINEDLLOVERRIDES=winhttp=b":      "WINEDLLOVERRIDES=winhttp=b",
		"WINEDLLOVERRIDES=dxgi,WinHTTP=n": "WINEDLLOVERRIDES=dxgi,WinHTTP=n",
	}
	for in, want := range cases {
		env := []string{"HOME=/h"}
		if in != "" {
			env = append(env, in)
		}
		got := withDLLOverride(env, "winhttp", "n,b")
		if got[len(got)-1] != want {
			t.Errorf("%q: got %q", in, got[len(got)-1])
		}
	}
}

func TestWrapperRun(t *testing.T) {
	tmp := t.TempDir()
	lib, _ := library.New(filepath.Join(tmp, "lib"), nil)
	gamePath := filepath.Join(tmp, "game")
	mkfile(t, filepath.Join(gamePath, "UnityPlayer.dll"))
	mkfile(t, filepath.Join(gamePath, "Game.exe"))
	os.MkdirAll(filepath.Join(gamePath, "Game_Data/Managed"), 0o755)
	game, err := lib.AddGame("Game", gamePath)
	if err != nil || game.Runtime != library.RuntimeProton {
		t.Fatalf("add game: %+v %v", game, err)
	}
	profileDir, _ := lib.ProfileDir(game.ID, game.ActiveProfile)
	mkfile(t, filepath.Join(profileDir, "winhttp.dll"))
	mkfile(t, filepath.Join(profileDir, "doorstop_config.ini"))

	// The "game" records what it sees: links in place and the DLL override.
	report := filepath.Join(tmp, "report")
	logFile := filepath.Join(profileDir, "BepInEx", "LogOutput.log")
	os.MkdirAll(filepath.Dir(logFile), 0o755)
	script := `test -L winhttp.dll && echo linked >> ` + report + `; echo "$WINEDLLOVERRIDES" >> ` + report +
		`; printf '[Info   :   BepInEx] Loading [A 1.0]\n' > '` + logFile + `'; exit 3`
	w := &Wrapper{Lib: lib, Installer: modinstall.NewInstaller(lib, nil, nil)}
	var notified string
	w.Notify = func(s, b string) { notified = s + ": " + b }

	t.Chdir(gamePath)
	code := w.Run([]string{"--game", game.ID, "--", "sh", "-c", script})
	if code != 3 {
		t.Errorf("exit code %d", code)
	}
	data, _ := os.ReadFile(report)
	if string(data) != "linked\nwinhttp=n,b\n" || notified != "" {
		t.Errorf("game saw %q, notified %q", data, notified)
	}
	if _, err := os.Lstat(filepath.Join(gamePath, "winhttp.dll")); err == nil {
		t.Error("links not cleaned up")
	}
	dataDir, _ := lib.GameDataDir(game.ID)
	if s, _ := ReadSession(dataDir); s != nil {
		t.Error("session not removed")
	}
	rep, err := ReadReport(dataDir, game.ActiveProfile)
	if err != nil || rep == nil || !rep.BepInExStarted || !slices.Equal(rep.Loaded, []string{"A 1.0"}) {
		t.Errorf("report: %+v %v", rep, err)
	}

	// A log left from an earlier session means BepInEx did not start this time.
	old := time.Now().Add(-time.Hour)
	os.Chtimes(logFile, old, old)
	w.Run([]string{"--game", game.ID, "--", "true"})
	if rep, _ := ReadReport(dataDir, game.ActiveProfile); rep == nil || rep.BepInExStarted {
		t.Errorf("stale log: %+v", rep)
	}

	// Unknown game: the command still runs, without mods, and the user is told.
	t.Setenv("SteamAppId", "999")
	if code := w.Run([]string{"--", "true"}); code != 0 || notified == "" {
		t.Errorf("unknown game: code %d notified %q", code, notified)
	}
}

const sampleLog = "[Message:   BepInEx] BepInEx 5.4.23.5 - Lethal Company (9/17/2026 11:46:42 AM)\r\n" +
	`[Info   :   BepInEx] Running under Unity v2022.3.9.8303977
[Message:   BepInEx] Chainloader started
[Info   :   BepInEx] 5 plugins to load
[Info   :   BepInEx] Loading [MoreCompany 1.14.0]
[Warning:   BepInEx] Skipping [Old Thing 1.0.0] because a newer version exists (Old Thing 1.2.0)
[Error  :   BepInEx] Could not load [Yippee tbh mod 1.2.4] because it is incompatible with: com.other.mod
[Error  :   BepInEx] Could not load [Needs Stuff 2.0.0] because it has missing dependencies: com.lib.core (v1.0.0 or newer)
[Error  :   BepInEx] Skipping [Chained 1.0.0] because it has a dependency that was not loaded. See previous errors for details.
[Error  :   BepInEx] Error loading [Crashy 0.1.0] : System.NullReferenceException: Object reference not set
  at Crashy.Plugin.Awake ()
[Info   :   MoreCompany] Loading [Not BepInEx 1.0]
[Message:   BepInEx] Chainloader startup complete
`

func TestParseLog(t *testing.T) {
	rep := ParseLog(strings.NewReader(sampleLog))
	if rep.BepInExVersion != "5.4.23.5" || !slices.Equal(rep.Loaded, []string{"MoreCompany 1.14.0"}) {
		t.Errorf("version/loaded: %q %v", rep.BepInExVersion, rep.Loaded)
	}
	want := []Issue{
		{Kind: IssueNewerVersionExists, Plugin: "Old Thing 1.0.0", Detail: "Old Thing 1.2.0"},
		{Kind: IssueIncompatible, Plugin: "Yippee tbh mod 1.2.4", Detail: "com.other.mod"},
		{Kind: IssueMissingDependencies, Plugin: "Needs Stuff 2.0.0", Detail: "com.lib.core (v1.0.0 or newer)"},
		{Kind: IssueDependencyNotLoaded, Plugin: "Chained 1.0.0"},
		{Kind: IssueLoadError, Plugin: "Crashy 0.1.0", Detail: "System.NullReferenceException: Object reference not set"},
	}
	if !slices.Equal(rep.Issues, want) {
		t.Errorf("issues:\n got  %+v\n want %+v", rep.Issues, want)
	}
}

func TestMatchMod(t *testing.T) {
	mods := []library.Mod{
		{ID: "notnotnotswipez-MoreCompany", Name: "MoreCompany", Files: []string{"BepInEx/plugins/notnotnotswipez-MoreCompany/MoreCompany.dll"}},
		{ID: "a-Suits", Name: "More_Suits", Files: []string{"BepInEx/plugins/a-Suits/moresuits/MoreSuits.dll"}},
	}
	mods = append(mods, library.Mod{ID: "sunnobunno-YippeeMod", Name: "YippeeMod", Plugins: []plugininfo.Plugin{{GUID: "sunnobunno.YippeeMod", Name: "Yippee tbh mod", Version: "1.2.4"}}})
	cases := map[string]string{
		"MoreCompany 1.14.0":   "notnotnotswipez-MoreCompany",
		"More Suits 1.5.2":     "a-Suits",
		"Yippee tbh mod 1.2.4": "sunnobunno-YippeeMod", // only the plugin name matches
		"Unknown 1.0":          "",
	}
	for plugin, want := range cases {
		if got := matchMod(mods, plugin); got != want {
			t.Errorf("%q: got %q want %q", plugin, got, want)
		}
	}
}

func TestWrapperRunNative(t *testing.T) {
	tmp := t.TempDir()
	lib, _ := library.New(filepath.Join(tmp, "lib"), nil)
	gamePath := filepath.Join(tmp, "game")
	mkfile(t, filepath.Join(gamePath, "UnityPlayer.so"))
	mkfile(t, filepath.Join(gamePath, "valheim.x86_64"))
	os.MkdirAll(filepath.Join(gamePath, "valheim_Data/Managed"), 0o755)
	game, err := lib.AddGame("Valheim", gamePath)
	if err != nil || game.Runtime != library.RuntimeNative {
		t.Fatalf("add game: %+v %v", game, err)
	}
	profileDir, _ := lib.ProfileDir(game.ID, game.ActiveProfile)
	mkfile(t, filepath.Join(profileDir, "BepInEx/core/BepInEx.Preloader.dll"))
	report := filepath.Join(tmp, "report")
	// Written without the executable bit, as unpacked from a zip.
	script := "#!/bin/sh\necho \"$@\" > '" + report + "'\nprintf '[Info   :   BepInEx] Loading [A 1.0]\\n' > \"$(dirname \"$0\")/BepInEx/LogOutput.log\"\n"
	if err := os.WriteFile(filepath.Join(profileDir, "start_game_bepinex.sh"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	w := &Wrapper{Lib: lib, Installer: modinstall.NewInstaller(lib, nil, nil)}
	code := w.Run([]string{"--game", game.ID, "--", "reaper", "SteamLaunch", "AppId=892970", "--", "./valheim.x86_64", "-console"})
	if code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if data, _ := os.ReadFile(report); string(data) != "reaper SteamLaunch AppId=892970 -- ./valheim.x86_64 -console\n" {
		t.Errorf("launcher got %q", data)
	}
	entries, _ := os.ReadDir(gamePath)
	if len(entries) != 3 {
		t.Errorf("native launch must not touch the game folder: %v", entries)
	}
	dataDir, _ := lib.GameDataDir(game.ID)
	if rep, _ := ReadReport(dataDir, game.ActiveProfile); rep == nil || !rep.BepInExStarted {
		t.Errorf("report: %+v", rep)
	}

	// Without an active loader the game starts vanilla.
	os.RemoveAll(filepath.Join(profileDir, "BepInEx/core"))
	os.Remove(report)
	if code := w.Run([]string{"--game", game.ID, "--", "true"}); code != 0 {
		t.Errorf("vanilla exit code %d", code)
	}
	if _, err := os.Stat(report); err == nil {
		t.Error("launcher must not run without BepInEx")
	}
}

func TestParseLogCompleteAndErrors(t *testing.T) {
	rep := ParseLog(strings.NewReader(sampleLog))
	if !rep.Complete {
		t.Error("a log that reaches the end of loading is not marked complete")
	}
	// Four error lines from BepInEx itself; the warning does not count.
	if rep.Errors != 4 {
		t.Errorf("errors = %d, want 4", rep.Errors)
	}
}

// Modpacks often turn on timestamps; every line then starts with the time.
func TestParseLogWithTimestamps(t *testing.T) {
	const log = "[Message:   BepInEx] BepInEx 5.4.21.0 - REPO (9/21/2026 9:53:20 PM)\n" +
		"[21:53:31.6107579] [Info   :   BepInEx] Loading [REPOLib 2.1.0]\n" +
		"[21:53:31.6547580] [Info   :   BepInEx] Loading [Valuables 1.0.0]\n" +
		"[21:55:13.4688472] [Error  : Unity Log] MissingMethodException: Method not found: void .PlayerAvatar.ChatMessageSend(string,bool)\n" +
		"[21:55:13.4778472] [Error  :   BepInEx] Error loading [Valuables 1.0.0] : System.TypeLoadException\n"
	rep := ParseLog(strings.NewReader(log))
	if rep.BepInExVersion != "5.4.21.0" {
		t.Errorf("version = %q", rep.BepInExVersion)
	}
	if !slices.Equal(rep.Loaded, []string{"REPOLib 2.1.0", "Valuables 1.0.0"}) {
		t.Errorf("loaded = %q", rep.Loaded)
	}
	if len(rep.Issues) != 1 || rep.Issues[0].Kind != IssueLoadError {
		t.Errorf("issues = %+v", rep.Issues)
	}
	if rep.Errors != 2 {
		t.Errorf("errors = %d, want 2 (the game's own and BepInEx's)", rep.Errors)
	}
	// It never reached "Chainloader startup complete": the game stopped there.
	if rep.Complete {
		t.Error("an unfinished log is marked complete")
	}
}

func TestNotStarted(t *testing.T) {
	mods := []library.Mod{
		{ID: "A-Loaded", Active: true, Plugins: []plugininfo.Plugin{{Name: "Loaded One", Version: "1.0.0"}}},
		{ID: "B-Stuck", Active: true, Plugins: []plugininfo.Plugin{{Name: "Stuck", Version: "2.0.0"}}},
		{ID: "C-Disabled", Active: false, Plugins: []plugininfo.Plugin{{Name: "Off", Version: "1.0.0"}}},
		{ID: "D-Library", Active: true}, // no plugins read: not counted
	}
	count, missing := notStarted(mods, []string{"Loaded One 1.0.0"})
	if count != 2 {
		t.Errorf("plugin mods = %d, want 2 (the disabled one and the one without plugins do not count)", count)
	}
	if !slices.Equal(missing, []string{"B-Stuck"}) {
		t.Errorf("not started = %q", missing)
	}
}
