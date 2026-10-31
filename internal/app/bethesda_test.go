package app

import (
	"archive/zip"
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

	s := NewBethesdaService(lib, nil)
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
	s := NewBethesdaService(lib, nil)

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

func TestTools(t *testing.T) {
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
	profileDir, _ := lib.ProfileDir(game.ID, game.ActiveProfile)
	writeFiles(t, filepath.Join(profileDir, "mods"), map[string]string{
		"bodyslide/CalienteTools/BodySlide/BodySlide.exe":              "",
		"nemesis/Nemesis_Engine/Nemesis Unlimited Behavior Engine.exe": "",
		"nemesis/Nemesis_Engine/hkxcmd.exe":                            "",
		"nemesis/Nemesis_Engine/Lib/distutils/command/wininst-9.0.exe": "",
		"skse/Root/skse64_loader.exe":                                  "",
		"off/Tool.exe":                                                 "",
	})
	if _, err := lib.UpdateProfile(game.ID, game.ActiveProfile, func(p *library.Profile) error {
		p.Mods = []library.Mod{
			{ID: "skse", Name: "SKSE", Enabled: true},
			{ID: "bodyslide", Name: "BodySlide", Enabled: true},
			{ID: "nemesis", Name: "Nemesis", Enabled: true},
			{ID: "off", Name: "Off", Enabled: false},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	tools, err := NewBethesdaService(lib, nil).Tools(game.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []Tool{
		{Path: "CalienteTools/BodySlide/BodySlide.exe", Name: "BodySlide", Mod: "BodySlide"},
		{Path: "Nemesis_Engine/Nemesis Unlimited Behavior Engine.exe", Name: "Nemesis Unlimited Behavior Engine", Mod: "Nemesis"},
	}
	if !slices.Equal(tools, want) {
		t.Errorf("tools:\n got  %+v\n want %+v", tools, want)
	}
}

func zipFile(t *testing.T, name string, files map[string]string) string {
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

func TestInstallArchives(t *testing.T) {
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
	s := NewBethesdaService(lib, nil)

	plain := zipFile(t, "SkyUI-12604-6-11-1778020881.zip", map[string]string{"SkyUI/SkyUI_SE.esp": "ui"})
	prepared, err := s.PrepareInstall(game.ID, game.ActiveProfile, plain)
	if err != nil || prepared.Fomod || prepared.Name != "SkyUI" {
		t.Fatalf("prepared %+v, %v", prepared, err)
	}
	p, err := s.FinishInstall(prepared.Session, nil)
	if err != nil {
		t.Fatal(err)
	}
	ui := p.Mods[len(p.Mods)-1]
	if ui.ID != "skyui" || ui.Version != "6.11" || ui.Source.Type != library.SourceNexus || !ui.Enabled {
		t.Errorf("installed %+v", ui)
	}
	profileDir, _ := lib.ProfileDir(game.ID, game.ActiveProfile)
	if _, err := os.Stat(filepath.Join(profileDir, "mods", "skyui", "SkyUI_SE.esp")); err != nil {
		t.Error(err)
	}

	// A patch offered only when SkyUI's plugin is there.
	config := `<config><moduleName>Patches</moduleName><installSteps><installStep name="Main"><optionalFileGroups>
	  <group name="Patches" type="SelectAny"><plugins order="Explicit">
	    <plugin name="SkyUI patch"><files><file source="skyui.esp" destination="Patch - SkyUI.esp"/></files>
	      <typeDescriptor><dependencyType><defaultType name="NotUsable"/><patterns><pattern>
	        <dependencies><fileDependency file="SkyUI_SE.esp" state="Active"/></dependencies><type name="Recommended"/>
	      </pattern></patterns></dependencyType></typeDescriptor></plugin>
	    <plugin name="Other patch"><files><file source="other.esp" destination="Patch - Other.esp"/></files>
	      <typeDescriptor><type name="Optional"/></typeDescriptor></plugin>
	  </plugins></group></optionalFileGroups></installStep></installSteps></config>`
	withFomod := zipFile(t, "Patches.zip", map[string]string{"fomod/ModuleConfig.xml": config, "skyui.esp": "p", "other.esp": "o"})
	prepared, err = s.PrepareInstall(game.ID, game.ActiveProfile, withFomod)
	if err != nil || !prepared.Fomod {
		t.Fatalf("prepared %+v, %v", prepared, err)
	}
	view, err := s.FomodView(prepared.Session, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := view.Steps[0].Groups[0].Plugins[0]; got.Type != "Recommended" || !got.Selected {
		t.Errorf("SkyUI patch with SkyUI there: %+v", got)
	}
	if p, err = s.FinishInstall(prepared.Session, view.Choices); err != nil {
		t.Fatal(err)
	}
	patches := p.Mods[len(p.Mods)-1]
	if patches.Notes != "FOMOD: SkyUI patch" {
		t.Errorf("notes %q", patches.Notes)
	}
	if _, err := os.Stat(filepath.Join(profileDir, "mods", patches.ID, "Patch - SkyUI.esp")); err != nil {
		t.Error(err)
	}
	if entries, _ := os.ReadDir(filepath.Join(profileDir, ".staging")); len(entries) != 0 {
		t.Errorf("unpacked archives left: %v", entries)
	}
	if _, err := s.FomodView(prepared.Session, nil); err == nil {
		t.Error("a finished install is still there")
	}
}
