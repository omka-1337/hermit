package steam

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// sampleShortcuts builds a file the way Steam writes one, with a single entry
// that carries a key Hermit does not know about.
func sampleShortcuts() []byte {
	var buf bytes.Buffer
	buf.WriteByte(typeMap)
	buf.WriteString("shortcuts")
	buf.WriteByte(0)

	buf.WriteByte(typeMap)
	buf.WriteString("0")
	buf.WriteByte(0)

	writeString := func(key, value string) {
		buf.WriteByte(typeString)
		buf.WriteString(key)
		buf.WriteByte(0)
		buf.WriteString(value)
		buf.WriteByte(0)
	}
	writeInt := func(key string, value int32) {
		buf.WriteByte(typeInt32)
		buf.WriteString(key)
		buf.WriteByte(0)
		binary.Write(&buf, binary.LittleEndian, value)
	}
	writeInt("appid", -1234567)
	writeString("AppName", "Some Other Game")
	writeString("Exe", `"/games/other.sh"`)
	writeString("StartDir", `"/games/"`)
	writeString("FlatpakAppID", "com.example.Other") // a key we never write
	writeInt("AllowOverlay", 1)

	buf.WriteByte(typeMap) // tags
	buf.WriteString("tags")
	buf.WriteByte(0)
	writeString("0", "favourite")
	buf.WriteByte(typeEnd)

	buf.WriteByte(typeEnd) // entry 0
	buf.WriteByte(typeEnd) // shortcuts
	buf.WriteByte(typeEnd) // document
	return buf.Bytes()
}

func TestBinaryVDFRoundTrip(t *testing.T) {
	data := sampleShortcuts()
	root, err := parseBinaryVDF(data)
	if err != nil {
		t.Fatalf("parseBinaryVDF: %v", err)
	}
	if got := encodeShortcutsFile(root); !bytes.Equal(got, data) {
		t.Errorf("re-encoding changed the file:\n got %q\nwant %q", got, data)
	}
}

func TestParseBinaryVDFRejectsGarbage(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":        {},
		"truncated":    {typeMap, 's', 0},
		"unknown type": {typeMap, 's', 0, 0x05, 'x', 0, typeEnd, typeEnd},
		"no key end":   {typeString, 'k'},
	} {
		if _, err := parseBinaryVDF(data); err == nil {
			t.Errorf("%s: parsed without an error", name)
		}
	}
}

func TestAddShortcutKeepsExistingEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shortcuts.vdf")
	if err := os.WriteFile(path, sampleShortcuts(), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := AddShortcut(path, Shortcut{
		AppName:       "Hermit",
		Exe:           "/home/deck/Applications/Hermit.AppImage",
		StartDir:      "/home/deck/Applications",
		LaunchOptions: "",
	})
	if err != nil || !added {
		t.Fatalf("AddShortcut = %v, %v", added, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	root, err := parseBinaryVDF(data)
	if err != nil {
		t.Fatalf("the written file does not parse: %v", err)
	}
	shortcuts := root.get("shortcuts").sub
	if len(shortcuts.entries) != 2 {
		t.Fatalf("got %d shortcuts, want the old one and the new one", len(shortcuts.entries))
	}

	old := shortcuts.entries[0].sub
	if got := old.get("AppName").str; got != "Some Other Game" {
		t.Errorf("existing shortcut renamed to %q", got)
	}
	if got := old.get("FlatpakAppID"); got == nil || got.str != "com.example.Other" {
		t.Error("an unknown key of the existing shortcut was dropped")
	}
	if got := old.get("tags"); got == nil || got.sub == nil || len(got.sub.entries) != 1 {
		t.Error("the tags of the existing shortcut were lost")
	}

	added2 := shortcuts.entries[1]
	if added2.key != "1" {
		t.Errorf("new entry numbered %q, want \"1\"", added2.key)
	}
	if got := added2.sub.get("Exe").str; got != `"/home/deck/Applications/Hermit.AppImage"` {
		t.Errorf("Exe = %s, want it quoted", got)
	}
	if got := added2.sub.get("appid").num; got != int32(ShortcutAppID("/home/deck/Applications/Hermit.AppImage", "Hermit")) {
		t.Errorf("appid = %d, want the derived id", got)
	}

	// The previous file is kept, in case Steam and Hermit disagree.
	if backup, err := os.ReadFile(path + ".bak"); err != nil || !bytes.Equal(backup, sampleShortcuts()) {
		t.Error("the original file was not backed up")
	}
}

func TestAddShortcutTwice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shortcuts.vdf")
	s := Shortcut{AppName: "Hermit", Exe: "/apps/Hermit.AppImage", StartDir: "/apps"}

	// The file does not exist yet: it is created.
	if added, err := AddShortcut(path, s); err != nil || !added {
		t.Fatalf("first AddShortcut = %v, %v", added, err)
	}
	if has, err := HasShortcut(path, s.Exe); err != nil || !has {
		t.Fatalf("HasShortcut = %v, %v", has, err)
	}
	// Adding the same command again changes nothing.
	if added, err := AddShortcut(path, s); err != nil || added {
		t.Fatalf("second AddShortcut = %v, %v, want no change", added, err)
	}
	data, _ := os.ReadFile(path)
	root, err := parseBinaryVDF(data)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(root.get("shortcuts").sub.entries); n != 1 {
		t.Errorf("got %d shortcuts, want 1", n)
	}
}

func TestShortcutsFiles(t *testing.T) {
	root := t.TempDir()
	for _, user := range []string{"1043796041", "9999"} {
		if err := os.MkdirAll(filepath.Join(root, "userdata", user, "config"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := ShortcutsFiles([]string{root, filepath.Join(root, "missing")})
	if len(files) != 2 {
		t.Fatalf("got %d files, want one per user: %q", len(files), files)
	}
	for _, f := range files {
		if filepath.Base(f) != "shortcuts.vdf" {
			t.Errorf("unexpected path %q", f)
		}
	}
}
