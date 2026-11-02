package sims4

import (
	"os"
	"path/filepath"
	"testing"

	"hermit/internal/deploy"
)

func TestModsDir(t *testing.T) {
	// Proton: compatdata with pfx, the user steamuser.
	proton := t.TempDir()
	os.MkdirAll(filepath.Join(proton, "pfx/drive_c/users/Public"), 0o755)
	os.MkdirAll(filepath.Join(proton, "pfx/drive_c/users/steamuser/Documents/Electronic Arts/The Sims 4"), 0o755)
	if got, err := ModsDir(proton); err != nil || got != filepath.Join(proton, "pfx/drive_c/users/steamuser/Documents/Electronic Arts/The Sims 4/Mods") {
		t.Errorf("proton: %q %v", got, err)
	}
	// Wine: drive_c straight in the prefix, the user named after the user.
	wine := t.TempDir()
	os.MkdirAll(filepath.Join(wine, "drive_c/users/omka"), 0o755)
	if got, err := ModsDir(wine); err != nil || got != filepath.Join(wine, "drive_c/users/omka/Documents/Electronic Arts/The Sims 4/Mods") {
		t.Errorf("wine: %q %v", got, err)
	}
	if !IsPrefix(proton) || !IsPrefix(wine) || IsPrefix(t.TempDir()) {
		t.Error("IsPrefix")
	}
	if _, err := ModsDir(""); err == nil {
		t.Error("no prefix")
	}
}

func TestModsGoIntoMods(t *testing.T) {
	tmp := t.TempDir()
	// A single piece of custom content, as it is downloaded.
	cc := filepath.Join(tmp, "HairCC.package")
	os.WriteFile(cc, []byte("cc"), 0o644)
	staged, err := deploy.Stage(cc, filepath.Join(tmp, "staging"), Shape.Layout, nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(tmp, "profile")
	if err := staged.Install(filepath.Join(profile, deploy.ModsDir, "hair"), nil, Target); err != nil {
		t.Fatal(err)
	}
	// The player's own mod stays where it is.
	mods := filepath.Join(tmp, "Mods")
	os.MkdirAll(mods, 0o755)
	os.WriteFile(filepath.Join(mods, "Own.package"), []byte("own"), 0o644)
	s, err := deploy.Deploy(mods, profile, []string{"hair"}, Target)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(mods, "HairCC.package")); err != nil || string(data) != "cc" {
		t.Errorf("the game sees %q %v", data, err)
	}
	if _, err := deploy.Undeploy(mods, Target, s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(mods, "HairCC.package")); err == nil {
		t.Error("the mod stayed in Mods")
	}
	if _, err := os.Stat(filepath.Join(mods, "Own.package")); err != nil {
		t.Errorf("the player's own mod is gone: %v", err)
	}
}
