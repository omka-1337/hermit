package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"hermit/internal/library"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
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

func TestImportMO2AndOrderMods(t *testing.T) {
	tmp := t.TempDir()
	lib, err := library.New(filepath.Join(tmp, "lib"), nil)
	if err != nil {
		t.Fatal(err)
	}
	gamePath := filepath.Join(tmp, "Skyrim Special Edition")
	writeFiles(t, gamePath, map[string]string{"SkyrimSE.exe": "", "Data/Skyrim.esm": ""})
	game, err := lib.AddGame("Skyrim", gamePath)
	if err != nil {
		t.Fatal(err)
	}
	mo2 := filepath.Join(tmp, "MO2")
	writeFiles(t, mo2, map[string]string{
		"ModOrganizer.ini":             "[General]\ngameName=Skyrim Special Edition\nselected_profile=@ByteArray(Default)\n",
		"profiles/Default/modlist.txt": "+Patch\r\n-PATCHES_separator\r\n+Sky UI\r\n",
		"profiles/Default/plugins.txt": "*SkyUI_SE.esp\r\n",
		"mods/Sky UI/SkyUI_SE.esp":     "ui",
		"mods/Sky UI/meta.ini":         "[General]\nmodid=12604\nversion=6.11\ncomments=\"from the FOMOD\"\n",
		"mods/Patch/Patch.esp":         "patch",
	})

	s := NewBethesdaService(lib)
	p, err := s.ImportMO2(game.ID, game.ActiveProfile, mo2, "Default")
	if err != nil {
		t.Fatal(err)
	}
	ids := func(p library.Profile) []string {
		var out []string
		for _, m := range p.Mods {
			out = append(out, m.ID)
		}
		return out
	}
	if !slices.Equal(ids(p), []string{"sky-ui", "patches-separator", "patch"}) {
		t.Fatalf("mods: %v", ids(p))
	}
	if sep := p.Mods[1]; !sep.Separator || sep.Name != "PATCHES" || sep.Enabled {
		t.Errorf("separator: %+v", sep)
	}
	if p, err = s.RemoveMod(game.ID, game.ActiveProfile, "patches-separator"); err != nil {
		t.Fatal(err)
	}
	if p, err = s.AddSeparator(game.ID, game.ActiveProfile, "Patches", 1); err != nil || p.Mods[1].ID != "patches-separator" {
		t.Fatalf("added separator: %v %v", ids(p), err)
	}
	if p, err = s.RenameSeparator(game.ID, game.ActiveProfile, "patches-separator", "Fixes"); err != nil || p.Mods[1].Name != "Fixes" {
		t.Fatalf("renamed separator: %+v %v", p.Mods[1], err)
	}
	if _, err := s.RenameSeparator(game.ID, game.ActiveProfile, "patch", "Not a separator"); err == nil {
		t.Error("renamed a mod as a separator")
	}
	if p, err = s.RemoveMod(game.ID, game.ActiveProfile, "patches-separator"); err != nil {
		t.Fatal(err)
	}
	ui := p.Mods[0]
	if ui.Source.Type != library.SourceNexus || ui.Source.URL != "https://www.nexusmods.com/skyrimspecialedition/mods/12604" ||
		ui.Version != "6.11" || ui.Notes != "from the FOMOD" || !ui.Enabled {
		t.Errorf("SkyUI: %+v", ui)
	}
	if _, err := s.ImportMO2(game.ID, game.ActiveProfile, mo2, "Default"); err == nil {
		t.Error("imported into a profile that has mods")
	}

	if p, err = s.MoveMods(game.ID, game.ActiveProfile, []string{"patch"}, "sky-ui"); err != nil || !slices.Equal(ids(p), []string{"patch", "sky-ui"}) {
		t.Fatalf("after moving: %v %v", ids(p), err)
	}
	if p, err = s.MoveMods(game.ID, game.ActiveProfile, []string{"patch"}, ""); err != nil || !slices.Equal(ids(p), []string{"sky-ui", "patch"}) {
		t.Fatalf("after moving to the end: %v %v", ids(p), err)
	}
	if p, err = s.MoveMods(game.ID, game.ActiveProfile, []string{"patch"}, "sky-ui"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MoveMods(game.ID, game.ActiveProfile, []string{"patch"}, "patch"); err == nil {
		t.Error("moved a mod before itself")
	}
	profileDir, _ := lib.ProfileDir(game.ID, game.ActiveProfile)
	if p, err = s.RemoveMod(game.ID, game.ActiveProfile, "patch"); err != nil || !slices.Equal(ids(p), []string{"sky-ui"}) {
		t.Fatalf("after removing: %v %v", ids(p), err)
	}
	if _, err := os.Stat(filepath.Join(profileDir, "mods", "patch")); !os.IsNotExist(err) {
		t.Error("the removed mod's folder is still there")
	}
}

func TestImportMO2AsProfile(t *testing.T) {
	tmp := t.TempDir()
	lib, err := library.New(filepath.Join(tmp, "lib"), nil)
	if err != nil {
		t.Fatal(err)
	}
	gamePath := filepath.Join(tmp, "Skyrim Special Edition")
	writeFiles(t, gamePath, map[string]string{"SkyrimSE.exe": "", "Data/Skyrim.esm": ""})
	game, err := lib.AddGame("Skyrim", gamePath)
	if err != nil {
		t.Fatal(err)
	}
	mo2 := filepath.Join(tmp, "MO2")
	writeFiles(t, mo2, map[string]string{
		"ModOrganizer.ini":             "[General]\ngameName=Skyrim Special Edition\n",
		"profiles/Default/modlist.txt": "+SkyUI\r\n",
		"mods/SkyUI/SkyUI_SE.esp":      "ui",
	})
	s := NewBethesdaService(lib)

	location := filepath.Join(tmp, "Hermit")
	if err := os.Mkdir(location, 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := s.ImportMO2AsProfile(game.ID, mo2, "Default", "From MO2", location)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := lib.ProfileDir(game.ID, p.ID)
	if p.Name != "From MO2" || len(p.Mods) != 1 || filepath.Dir(dir) != location {
		t.Fatalf("profile %+v in %s", p, dir)
	}

	// A failed import leaves no profile behind.
	if _, err := s.ImportMO2AsProfile(game.ID, mo2, "Missing", "Broken", location); err == nil {
		t.Fatal("imported a profile MO2 does not have")
	}
	if profiles, _ := lib.ListProfiles(game.ID); len(profiles) != 2 {
		t.Errorf("profiles after a failed import: %d", len(profiles))
	}
}
