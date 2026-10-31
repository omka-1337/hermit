// Package redengine knows the games of CD Projekt Red's REDengine whose mods
// Hermit deploys. Their mods are laid out as the game folder: archives in
// archive/pc/mod, the plugins of Cyber Engine Tweaks and RED4ext beside the
// game's executable and in red4ext, scripts in r6.
package redengine

import (
	"strings"

	"hermit/internal/deploy"
)

// Target is where the mods of these games go: into the game folder, where
// they can take the place of the game's own files, as its configuration.
var Target = deploy.Target{Replace: true}

// Game is what differs between the REDengine games.
type Game struct {
	Name string
	// Exe is the game's executable, slash-separated from the game folder,
	// which also identifies it.
	Exe string
	// EngineVersion is the REDengine version, "4" for Cyberpunk 2077.
	EngineVersion string
	// Launchers are the files Steam starts the game from.
	Launchers []string
	// Nexus is the game's part of nexusmods.com, as in its addresses.
	Nexus string
	// Shape is how archives of the game's mods are laid out.
	Shape deploy.Shape
}

var Games = []Game{
	{
		Name: "Cyberpunk 2077",
		Exe:  "bin/x64/Cyberpunk2077.exe", EngineVersion: "4",
		Launchers: []string{"REDprelauncher.exe", "Cyberpunk2077.exe"},
		Nexus:     "cyberpunk2077",
		Shape: deploy.Shape{
			// mods holds REDmod's mods.
			Folders: []string{"archive", "bin", "engine", "r6", "red4ext", "mods"},
			Loose: map[string]string{
				".archive": "archive/pc/mod",
				// ArchiveXL's files beside the archives they extend.
				".xl":   "archive/pc/mod",
				".reds": "r6/scripts",
			},
			Within: map[string]string{
				"cyber_engine_tweaks": "bin/x64/plugins/cyber_engine_tweaks",
			},
		},
	},
}

// Find returns the game whose executable is exe.
func Find(exe string) (Game, bool) {
	for _, g := range Games {
		if strings.EqualFold(g.Exe, exe) {
			return g, true
		}
	}
	return Game{}, false
}
