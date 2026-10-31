// Package unreal knows the Unreal Engine games whose mods Hermit deploys.
// The mods of Unreal Engine 3 games are packages laid out as the game
// folder, most of them taking the place of the game's own packages.
package unreal

import (
	"strings"

	"hermit/internal/deploy"
)

// Target is where the mods of these games go: into the game folder, in
// place of the game's own files.
var Target = deploy.Target{Replace: true}

// Game is what differs between the Unreal Engine games.
type Game struct {
	Name string
	// Exe is the game's executable, slash-separated from the game folder,
	// which also identifies it.
	Exe string
	// EngineVersion is the Unreal Engine version, "3" for Arkham Knight.
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
		Name: "Batman: Arkham Knight",
		Exe:  "Binaries/Win64/BatmanAK.exe", EngineVersion: "3",
		Launchers: []string{"BatmanAK.exe"},
		Nexus:     "batmanarkhamknight",
		Shape: deploy.Shape{
			Folders: []string{"BmGame", "Binaries", "Engine", "DLC"},
			Loose: map[string]string{
				".upk": "BmGame/CookedPCConsole",
				".tfc": "BmGame/CookedPCConsole",
			},
			Within: map[string]string{
				"CookedPCConsole": "BmGame/CookedPCConsole",
				"Win64":           "Binaries/Win64",
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
