package deploy

import (
	"slices"
	"testing"
)

func TestPreview(t *testing.T) {
	game, profile := t.TempDir(), t.TempDir()
	write(t, game, map[string]string{
		"SkyrimSE.exe":           "",
		"Data/Skyrim.esm":        "",
		"Data/Scripts/actor.pex": "game's",
	})
	write(t, profile, map[string]string{
		"mods/low/textures/sky.dds":     "",
		"mods/low/scripts/actor.pex":    "hidden by the game's",
		"mods/high/Textures/SKY.dds":    "",
		"mods/high/Root/loader.exe":     "",
		"overwrite/SKSE/written.log":    "",
		"mods/off/textures/ignored.dds": "",
	})
	p, err := NewPreview(game, profile, []string{"low", "high"}, Target{Data: "Data"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Files != 6 {
		t.Errorf("%d files", p.Files)
	}
	top, _ := p.Folder("")
	if got := entryNames(top); !slices.Equal(got, []string{"Data", "loader.exe", "SkyrimSE.exe"}) {
		t.Errorf("game folder: %v", got)
	}
	if e := find(top, "loader.exe"); e.Source != "high" {
		t.Errorf("loader.exe: %+v", e)
	}
	data, _ := p.Folder("Data")
	if got := entryNames(data); !slices.Equal(got, []string{"Scripts", "SKSE", "textures", "Skyrim.esm"}) {
		t.Errorf("Data: %v", got)
	}
	if e := find(data, "textures"); !e.Dir || e.Files != 1 || e.Path != "Data/textures" {
		t.Errorf("textures: %+v", e)
	}
	sky, _ := p.Folder("Data/TEXTURES")
	if e := find(sky, "sky.dds"); e.Source != "high" || !slices.Equal(e.Others, []string{"low"}) {
		t.Errorf("sky.dds: %+v", e)
	}
	scripts, _ := p.Folder("Data/Scripts")
	if e := find(scripts, "actor.pex"); e.Source != "" || !slices.Equal(e.Others, []string{"low"}) {
		t.Errorf("actor.pex, the game's: %+v", e)
	}
	skse, _ := p.Folder("Data/SKSE")
	if e := find(skse, "written.log"); e.Source != OverwriteSource {
		t.Errorf("written.log: %+v", e)
	}

	// Where mods take the place of the game's files.
	p, _ = NewPreview(game, profile, []string{"low"}, Target{Data: "Data", Replace: true})
	scripts, _ = p.Folder("Data/Scripts")
	if e := find(scripts, "actor.pex"); e.Source != "low" || !slices.Equal(e.Others, []string{""}) {
		t.Errorf("actor.pex, replaced: %+v", e)
	}
	if _, err := p.Folder("Data/nowhere"); err == nil {
		t.Error("listed a folder that is not there")
	}
}

func entryNames(list []PreviewEntry) []string {
	var out []string
	for _, e := range list {
		out = append(out, e.Name)
	}
	return out
}

func find(list []PreviewEntry, name string) PreviewEntry {
	for _, e := range list {
		if e.Name == name {
			return e
		}
	}
	return PreviewEntry{}
}
