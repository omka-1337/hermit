package deploy

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMergeText(t *testing.T) {
	// The game's localisation is long; a mod's, its few lines.
	gameText := "[Showcase]\r\n" + strings.Repeat("Batman=BATMAN\r\n", 30)
	game := encode(gameText, utf16LE)
	harley := encode("[Showcase]\r\nHarley=HARLEY QUINN\r\n", utf16LE)
	got := MergeText(game, [][]byte{harley})
	if detect(got) != utf16LE {
		t.Fatal("not UTF-16 any more")
	}
	if text := decode(got); text != gameText+"[Showcase]\r\nHarley=HARLEY QUINN\r\n" {
		t.Errorf("merged %q", text)
	}
	// A whole file in the place of the game's.
	whole := []byte("[Engine]\r\nA=1\r\nB=2\r\nC=3\r\n")
	if got := string(MergeText([]byte("[Engine]\r\nA=0\r\nB=0\r\n"), [][]byte{whole})); got != string(whole) {
		t.Errorf("a whole file was merged: %q", got)
	}
	// Two options of one archive, the same size, both stay.
	a, b := []byte("[G]\n+X=\"A\"\n"), []byte("[G]\n+X=\"B\"\n")
	if got := string(AppendText(a, b)); got != "[G]\n+X=\"A\"\n[G]\n+X=\"B\"\n" {
		t.Errorf("appended %q", got)
	}
}

// A skin pack for Arkham Knight: a folder for each skin, each downloadable
// content of its own, which goes into a folder of the game's DLC.
func TestSkinPack(t *testing.T) {
	game := t.TempDir()
	write(t, game, map[string]string{
		"Binaries/Win64/BatmanAK.exe":                     "",
		"DLC/313100/Content/BmGame/Config/BmGame.ini":     "[BmGame.RGameInfo]\r\n+DLCBundlesInstalled=\"HarleyPolice\"\r\n",
		"DLC/313101/Content/BmGame/CookedPCConsole/x.upk": "",
	})
	shape := Shape{
		Folders: []string{"BmGame", "Binaries", "DLC"},
		Within:  map[string]string{"CookedPCConsole": "BmGame/CookedPCConsole"},
		Bundles: map[string][]string{"DLC/*": {"CookedPCConsole", "Config/BmGame.ini"}},
		Game:    game,
	}
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"two skins side by side", map[string]string{
			"AsylumHarley/CookedPCConsole/Playable_AA.upk": "aa",
			"AsylumHarley/Config/BmGame.ini":               "+DLCBundlesInstalled=\"ASYLUMHARLEY\"",
			"CityHarley/CookedPCConsole/Playable_AC.upk":   "ac",
			"CityHarley/Config/BmGame.ini":                 "+DLCBundlesInstalled=\"CITYHARLEY\"",
			"readme.txt":                                   "",
		}, []string{
			"DLC/313100/AsylumHarley/Config/BmGame.ini", "DLC/313100/AsylumHarley/CookedPCConsole/Playable_AA.upk",
			"DLC/313100/CityHarley/Config/BmGame.ini", "DLC/313100/CityHarley/CookedPCConsole/Playable_AC.upk",
		}},
		{"one skin at the top, named as the mod", map[string]string{
			"CookedPCConsole/Suit.upk": "",
			"Config/BmGame.ini":        "+DLCBundlesInstalled=\"SUIT\"",
		}, []string{"DLC/313100/Blue Suit/Config/BmGame.ini", "DLC/313100/Blue Suit/CookedPCConsole/Suit.upk"}},
		{"packages of the game itself", map[string]string{
			"CookedPCConsole/Playable_Batman.upk": "",
		}, []string{"BmGame/CookedPCConsole/Playable_Batman.upk"}},
	} {
		staged, err := Stage(makeZip(t, tc.files), t.TempDir(), func(dir, _ string) ([]Part, bool, error) { return shape.Layout(dir, "Blue Suit") }, nil)
		if err != nil {
			t.Fatal(err)
		}
		mod := filepath.Join(t.TempDir(), "mod")
		if err := staged.Install(mod, nil, Target{}); err != nil {
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
	}
}

// A mod that adds to the game's own configuration: for the session the game
// reads its file with the mod's lines, and gets it back after.
func TestDeployMergesConfiguration(t *testing.T) {
	game, profile := t.TempDir(), t.TempDir()
	original := "[BmGame.RGameInfo]\r\n" + strings.Repeat("ShowcaseItems=(Item=\"Batman\")\r\n", 20)
	write(t, game, map[string]string{"BmGame/Config/BmGame.ini": original})
	write(t, profile, map[string]string{
		"mods/a/BmGame/Config/BmGame.ini": "[BmGame.RGameInfo]\r\n.ShowcaseItems=(Item=\"A\")\r\n",
		"mods/b/BmGame/Config/BmGame.ini": "[BmGame.RGameInfo]\r\n.ShowcaseItems=(Item=\"B\")\r\n",
	})
	target := Target{Replace: true, Merge: []string{".ini"}}
	s, err := Deploy(game, profile, []string{"a", "b"}, target)
	if err != nil {
		t.Fatal(err)
	}
	played := read(t, filepath.Join(game, "BmGame/Config/BmGame.ini"))
	if !strings.HasPrefix(played, original) || !strings.Contains(played, `Item="A"`) || strings.Index(played, `Item="A"`) > strings.Index(played, `Item="B"`) {
		t.Errorf("the game reads %q", played)
	}
	if _, err := Undeploy(game, target, s); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(game, "BmGame/Config/BmGame.ini")); got != original {
		t.Errorf("after the session the game has %q", got)
	}
}

// A mod TFC Installer writes into the game's own packages is refused, and
// nothing of it is left unpacked.
func TestForeignModRefused(t *testing.T) {
	shape := Shape{Folders: []string{"BmGame"}, Foreign: []string{"GameProfile.xml", ".PackagePatch"}, Refusal: "TFC Installer's"}
	staging := t.TempDir()
	_, err := Stage(makeZip(t, map[string]string{
		"Community Patch/GameProfile.xml":                                     "",
		"Community Patch/Game/BmGame/CookedPCConsole/BmGame.upk.PackagePatch": "",
	}), staging, shape.Layout, nil)
	if err == nil || err.Error() != "TFC Installer's" {
		t.Errorf("staged: %v", err)
	}
	if entries, _ := os.ReadDir(staging); len(entries) != 0 {
		t.Errorf("left unpacked: %v", entries)
	}
}
