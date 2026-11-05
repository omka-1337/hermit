package sims4

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hermit/internal/deploy"
)

func TestModsDir(t *testing.T) {
	// Proton: compatdata with pfx, the user steamuser.
	proton := t.TempDir()
	os.MkdirAll(filepath.Join(proton, "pfx/drive_c/users/Public"), 0o755)
	os.MkdirAll(filepath.Join(proton, "pfx/drive_c/users/steamuser/Documents/Electronic Arts/The Sims 4"), 0o755)
	if got, err := DocumentsDir(proton); err != nil || got != filepath.Join(proton, "pfx/drive_c/users/steamuser/Documents/Electronic Arts/The Sims 4") {
		t.Errorf("proton: %q %v", got, err)
	}
	// Wine: drive_c straight in the prefix, the user named after the user.
	wine := t.TempDir()
	os.MkdirAll(filepath.Join(wine, "drive_c/users/omka"), 0o755)
	if got, err := DocumentsDir(wine); err != nil || got != filepath.Join(wine, "drive_c/users/omka/Documents/Electronic Arts/The Sims 4") {
		t.Errorf("wine: %q %v", got, err)
	}
	if !IsPrefix(proton) || !IsPrefix(wine) || IsPrefix(t.TempDir()) {
		t.Error("IsPrefix")
	}
	if _, err := DocumentsDir(""); err == nil {
		t.Error("no prefix")
	}
}

// A Sim made on a Mac: custom content in Mods, the Sim for Tray, and what
// macOS adds to archives, which is no mod.
func TestModsGoIntoMods(t *testing.T) {
	tmp := t.TempDir()
	archive := filepath.Join(tmp, "HarleyQuinn_Base.zip")
	makeZip(t, archive, map[string]string{
		"HarleyQuinn_Base/Mods/Skin.package":            "skin",
		"HarleyQuinn_Base/Mods/.DS_Store":               "",
		"HarleyQuinn_Base/Tray/0x1!0x2.trayitem":        "sim",
		"__MACOSX/HarleyQuinn_Base/Mods/._Skin.package": "resource fork",
	})
	staged, err := deploy.Stage(archive, filepath.Join(tmp, "staging"), Shape.Layout, nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(tmp, "profile")
	if err := staged.Install(filepath.Join(profile, deploy.ModsDir, "harley"), nil, Target); err != nil {
		t.Fatal(err)
	}
	// A single piece of custom content, as it is downloaded.
	cc := filepath.Join(tmp, "HairCC.package")
	os.WriteFile(cc, []byte("cc"), 0o644)
	staged, err = deploy.Stage(cc, filepath.Join(tmp, "staging"), Shape.Layout, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := staged.Install(filepath.Join(profile, deploy.ModsDir, "hair"), nil, Target); err != nil {
		t.Fatal(err)
	}

	// The game's folder of Documents, with the player's own mod.
	docs := filepath.Join(tmp, "The Sims 4")
	os.MkdirAll(filepath.Join(docs, "Mods"), 0o755)
	os.WriteFile(filepath.Join(docs, "Mods", "Own.package"), []byte("own"), 0o644)
	s, err := deploy.Deploy(docs, profile, []string{"harley", "hair"}, Target)
	if err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{
		"Mods/HairCC.package":   "cc",
		"Mods/Skin.package":     "skin",
		"Tray/0x1!0x2.trayitem": "sim",
	} {
		if data, err := os.ReadFile(filepath.Join(docs, rel)); err != nil || string(data) != want {
			t.Errorf("%s: %q %v", rel, data, err)
		}
	}
	var junk []string
	filepath.WalkDir(docs, func(p string, d os.DirEntry, err error) error {
		if err == nil && (d.Name() == "__MACOSX" || d.Name() == ".DS_Store" || strings.HasPrefix(d.Name(), "._")) {
			junk = append(junk, p)
		}
		return nil
	})
	if len(junk) > 0 {
		t.Errorf("macOS's files reached the game: %v", junk)
	}
	if _, err := deploy.Undeploy(docs, Target, s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(docs, "Mods", "HairCC.package")); err == nil {
		t.Error("the mod stayed in Mods")
	}
	if _, err := os.Lstat(filepath.Join(docs, "Tray", "0x1!0x2.trayitem")); err == nil {
		t.Error("the Sim stayed in Tray")
	}
	if _, err := os.Stat(filepath.Join(docs, "Mods", "Own.package")); err != nil {
		t.Errorf("the player's own mod is gone: %v", err)
	}
}

func makeZip(t *testing.T, file string, files map[string]string) {
	t.Helper()
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
}
