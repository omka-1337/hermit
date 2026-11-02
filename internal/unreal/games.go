// Package unreal knows the Unreal Engine games whose mods Hermit deploys.
// The mods of Unreal Engine 3 games are packages laid out as the game
// folder, most of them taking the place of the game's own packages.
package unreal

import (
	"strings"

	"hermit/internal/deploy"
)

// Target is where the mods of these games go: into the game folder, in
// place of the game's own files, but for configuration and localisation,
// whose mods hold only the lines they add to the game's.
var Target = deploy.Target{
	Replace: true,
	Merge: []string{
		".ini",
		// Localisation, a file for each language.
		".int", ".fra", ".deu", ".ita", ".esn", ".esm", ".pol", ".rus", ".ptb", ".jpn", ".kor", ".chn", ".cht", ".cze", ".hun",
	},
}

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
			// Skins and suits come as downloadable content of their own, a
			// folder each, which goes into any of the game's DLC folders.
			Bundles: map[string][]string{
				"DLC/*": {"CookedPCConsole", "Config/BmGame.ini"},
			},
			// TFC Installer writes these mods into the game's own packages.
			Foreign: []string{"GameProfile.xml", ".PackagePatch", ".TFCMapping", ".IniPatch", ".descriptor"},
			Refusal: "this mod is installed with TFC Installer, which writes it into the game's own files; Hermit keeps the game as it is and links mods in only while it runs, so it does not install mods of this kind",
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
