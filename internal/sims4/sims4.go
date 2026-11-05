// Package sims4 knows The Sims 4, whose mods do not go into the game's
// folder but into its folder of Documents, Electronic Arts/The Sims 4/Mods,
// which on Linux is in the Wine prefix the game runs in. Its mods are
// .package files, custom content, and .ts4script files, scripts.
package sims4

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"hermit/internal/deploy"
)

// Exe is the game's executable, slash-separated from the game folder; it
// identifies the game.
const Exe = "Game/Bin/TS4_x64.exe"

// Nexus is the game's part of nexusmods.com.
const Nexus = "thesims4"

// Target is where the mods go: deployed into the game's folder of Documents,
// custom content and scripts into its Mods folder, laid out as they are, and
// what a mod's Root folder holds into the folder of Documents itself, as
// Sims made ready to place, which go into Tray. The player's own files stay.
var Target = deploy.Target{Data: "Mods"}

// trayFiles are the extensions of what goes into Tray: households, lots and
// rooms saved in the gallery's format.
var trayFiles = []string{".trayitem", ".householdbinary", ".hhi", ".sgi", ".blueprint", ".bpi", ".room", ".rmi"}

// Shape is how mod archives are laid out: custom content and scripts, maybe
// in a folder of their own or in a Mods folder, and Sims for Tray beside
// them.
var Shape = func() deploy.Shape {
	loose := map[string]string{".package": "", ".ts4script": ""}
	for _, ext := range trayFiles {
		loose[ext] = "Root/Tray"
	}
	return deploy.Shape{
		Loose:  loose,
		Within: map[string]string{"Mods": "", "Tray": "Root/Tray"},
	}
}()

// documents is the game's folder in Documents.
var documents = []string{"Electronic Arts", "The Sims 4"}

// DocumentsDir is the game's folder of Documents in a Wine prefix, with Mods
// and Tray in it: a folder with drive_c in it, or Proton's compatibility
// data folder with its pfx.
func DocumentsDir(prefix string) (string, error) {
	if prefix == "" {
		return "", errors.New("Hermit does not know the Wine prefix The Sims 4 runs in; choose it in the game's settings")
	}
	driveC := filepath.Join(prefix, "drive_c")
	if fi, err := os.Stat(filepath.Join(prefix, "pfx", "drive_c")); err == nil && fi.IsDir() {
		driveC = filepath.Join(prefix, "pfx", "drive_c")
	}
	users := filepath.Join(driveC, "users")
	entries, err := os.ReadDir(users)
	if err != nil {
		return "", errors.New(prefix + " is not a Wine prefix: it has no drive_c/users")
	}
	// Proton's user is steamuser; Wine names it after the user on Linux.
	user := ""
	for _, e := range entries {
		if !e.IsDir() || strings.EqualFold(e.Name(), "Public") {
			continue
		}
		if user == "" || e.Name() == "steamuser" {
			user = e.Name()
		}
	}
	if user == "" {
		return "", errors.New(prefix + " has no user yet; start the game in it once")
	}
	dir := filepath.Join(users, user)
	docs, _ := deploy.FindFold(dir, "Documents")
	if docs == "" {
		docs = "Documents"
	}
	dir = filepath.Join(dir, docs)
	for _, part := range documents {
		if name, _ := deploy.FindFold(dir, part); name != "" {
			part = name
		}
		dir = filepath.Join(dir, part)
	}
	return dir, nil
}

// IsPrefix reports whether dir is a Wine prefix DocumentsDir can use.
func IsPrefix(dir string) bool {
	for _, d := range []string{filepath.Join(dir, "drive_c"), filepath.Join(dir, "pfx", "drive_c")} {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			return true
		}
	}
	return false
}
