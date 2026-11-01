package creation

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// plugin makes the start of a plugin file: its TES4 record with the flags
// and masters given.
func plugin(flags uint32, masters ...string) string {
	var fields bytes.Buffer
	field := func(kind string, data []byte) {
		fields.WriteString(kind)
		binary.Write(&fields, binary.LittleEndian, uint16(len(data)))
		fields.Write(data)
	}
	field("HEDR", make([]byte, 12))
	for _, m := range masters {
		field("MAST", append([]byte(m), 0))
		field("DATA", make([]byte, 8))
	}
	var b bytes.Buffer
	b.WriteString("TES4")
	binary.Write(&b, binary.LittleEndian, uint32(fields.Len()))
	binary.Write(&b, binary.LittleEndian, flags)
	b.Write(make([]byte, 12))
	b.Write(fields.Bytes())
	b.WriteString("GRUP")
	return b.String()
}

func names(list []LoadOrderEntry) []string {
	var out []string
	for _, e := range list {
		out = append(out, e.Name)
	}
	return out
}

func TestLoadOrder(t *testing.T) {
	game, profile := t.TempDir(), t.TempDir()
	skyrim := Games[0]
	write(t, game, map[string]string{
		"SkyrimSE.exe":              "",
		"Skyrim.ccc":                "ccBGSSSE001-Fish.esm\r\nccNotInstalled.esl\r\n",
		"Data/Skyrim.esm":           plugin(1),
		"Data/Update.esm":           plugin(1, "Skyrim.esm"),
		"Data/ccBGSSSE001-Fish.esm": plugin(1, "Skyrim.esm"),
		"Data/Dawnguard.esm":        plugin(1, "Skyrim.esm"),
		"Data/HandPlaced.esp":       plugin(0, "Skyrim.esm"),
		"Data/A folder.esp/x":       "",
	})
	write(t, profile, map[string]string{
		"mods/ussep/Unofficial Skyrim Special Edition Patch.esp": plugin(1, "Skyrim.esm", "Dawnguard.esm"),
		"mods/ui/SkyUI_SE.esp":     plugin(0, "Skyrim.esm"),
		"mods/light/Small.esl":     plugin(0, "Skyrim.esm"),
		"mods/flagged/Flagged.esp": plugin(0x200, "Skyrim.esm"),
		"mods/needy/Needy.esp":     plugin(0, "Skyrim.esm", "Absent.esm", "SkyUI_SE.esp"),
		"mods/newer/SkyUI_SE.esp":  plugin(0, "Skyrim.esm"),
		// The list has SkyUI before the patch, a plugin of a mod that is off,
		// and HandPlaced switched off.
		PluginsFile: "*SkyUI_SE.esp\r\n*Off.esp\r\nHandPlaced.esp\r\n*Needy.esp\r\n",
	})
	mods := []string{"ussep", "ui", "light", "flagged", "needy", "newer"}
	list, err := LoadOrder(skyrim, game, profile, mods)
	if err != nil {
		t.Fatal(err)
	}
	// The implicit ones first, Dawnguard among them; then the masters, the
	// patch flagged as one, before SkyUI; then the list's order; plugins new
	// to the list last.
	want := []string{
		"Skyrim.esm", "Update.esm", "Dawnguard.esm", "ccBGSSSE001-Fish.esm",
		"Unofficial Skyrim Special Edition Patch.esp", "Small.esl",
		"SkyUI_SE.esp", "HandPlaced.esp", "Needy.esp", "Flagged.esp",
	}
	if got := names(list); !slices.Equal(got, want) {
		t.Fatalf("order\n got  %q\n want %q", got, want)
	}
	byName := map[string]LoadOrderEntry{}
	for _, e := range list {
		byName[e.Name] = e
	}
	if e := byName["Dawnguard.esm"]; !e.Implicit || !e.Active || e.Index != "02" || e.Mod != "" {
		t.Errorf("Dawnguard: %+v", e)
	}
	if e := byName["SkyUI_SE.esp"]; e.Mod != "newer" || e.Index != "05" {
		t.Errorf("SkyUI comes from the later mod: %+v", e)
	}
	if e := byName["Small.esl"]; !e.Light || !e.Master || e.Index != "FE:000" {
		t.Errorf("Small.esl: %+v", e)
	}
	if e := byName["Flagged.esp"]; !e.Light || e.Master || e.Index != "FE:001" {
		t.Errorf("Flagged.esp: %+v", e)
	}
	if e := byName["HandPlaced.esp"]; e.Active || e.Index != "" || e.Mod != "" {
		t.Errorf("HandPlaced.esp: %+v", e)
	}
	if e := byName["Needy.esp"]; !slices.Equal(e.Missing, []string{"Absent.esm"}) {
		t.Errorf("Needy.esp misses %v", e.Missing)
	}

	// Moving Needy to the top of what can move, and switching SkyUI off.
	if err := MovePlugins(skyrim, game, profile, mods, []string{"Needy.esp"}, "Skyrim.esm"); err != nil {
		t.Fatal(err)
	}
	if err := SetPluginActive(skyrim, game, profile, mods, "SkyUI_SE.esp", false); err != nil {
		t.Fatal(err)
	}
	if err := SetPluginActive(skyrim, game, profile, mods, "Update.esm", false); err != ErrImplicit {
		t.Errorf("switched off an implicit plugin: %v", err)
	}
	stored, _ := ReadPlugins(filepath.Join(profile, PluginsFile))
	wantStored := []Plugin{
		{"Needy.esp", true}, {"SkyUI_SE.esp", false}, {"Off.esp", true}, {"HandPlaced.esp", false},
		{"Unofficial Skyrim Special Edition Patch.esp", true}, {"Small.esl", true}, {"Flagged.esp", true},
	}
	if !slices.Equal(stored, wantStored) {
		t.Errorf("plugins.txt\n got  %v\n want %v", stored, wantStored)
	}
	list, _ = LoadOrder(skyrim, game, profile, mods)
	if e := list[slices.IndexFunc(list, func(e LoadOrderEntry) bool { return e.Name == "Needy.esp" })]; !slices.Equal(e.Missing, []string{"Absent.esm", "SkyUI_SE.esp"}) {
		t.Errorf("with SkyUI off and after it, Needy.esp misses %v", e.Missing)
	}
}

func TestReadHeaderRefusesOtherFiles(t *testing.T) {
	file := filepath.Join(t.TempDir(), "x.esp")
	os.WriteFile(file, []byte("PK\x03\x04 not a plugin at all"), 0o644)
	if _, err := ReadHeader(file, Games[0]); err == nil {
		t.Error("read a header from a zip file")
	}
}
