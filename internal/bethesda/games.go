// Package bethesda mods games on Bethesda's Creation Engine: it links the
// enabled mods of a profile into the game's Data folder for the length of a
// session, in the order the profile gives them, and puts the profile's
// plugin list where the game reads it.
package bethesda

import "strings"

// Game is what differs between the Creation Engine games.
type Game struct {
	// Exe is the game's executable, which also identifies it.
	Exe string
	// Master is the file every install of the game has in Data.
	Master string
	// EngineVersion is "2" for Creation Engine 2 and empty for the first one.
	EngineVersion string
	// AppData is the folder in the user's AppData\Local where the game reads
	// plugins.txt from.
	AppData string
	// Loader starts the game with its script extender (SKSE, F4SE, SFSE).
	Loader string
	// Launchers are the files Steam starts the game from; with the script
	// extender installed they are replaced by Loader.
	Launchers []string
	// MO2Name is how Mod Organizer 2 names the game in its instance settings.
	MO2Name string
	// Nexus is the game's part of nexusmods.com, as in its addresses.
	Nexus string
}

var Games = []Game{
	{
		Exe: "SkyrimSE.exe", Master: "Skyrim.esm", AppData: "Skyrim Special Edition",
		Loader: "skse64_loader.exe", Launchers: []string{"SkyrimSELauncher.exe", "SkyrimSE.exe"},
		MO2Name: "Skyrim Special Edition", Nexus: "skyrimspecialedition",
	},
	{
		Exe: "SkyrimVR.exe", Master: "Skyrim.esm", AppData: "Skyrim VR",
		Loader: "sksevr_loader.exe", Launchers: []string{"SkyrimVR.exe"},
		MO2Name: "Skyrim VR", Nexus: "skyrimspecialedition",
	},
	{
		Exe: "TESV.exe", Master: "Skyrim.esm", AppData: "Skyrim",
		Loader: "skse_loader.exe", Launchers: []string{"SkyrimLauncher.exe", "TESV.exe"},
		MO2Name: "Skyrim", Nexus: "skyrim",
	},
	{
		Exe: "Fallout4.exe", Master: "Fallout4.esm", AppData: "Fallout4",
		Loader: "f4se_loader.exe", Launchers: []string{"Fallout4Launcher.exe", "Fallout4.exe"},
		MO2Name: "Fallout 4", Nexus: "fallout4",
	},
	{
		Exe: "Fallout4VR.exe", Master: "Fallout4.esm", AppData: "Fallout4VR",
		Loader: "f4sevr_loader.exe", Launchers: []string{"Fallout4VR.exe"},
		MO2Name: "Fallout 4 VR", Nexus: "fallout4",
	},
	{
		Exe: "Starfield.exe", Master: "Starfield.esm", EngineVersion: "2", AppData: "Starfield",
		Loader: "sfse_loader.exe", Launchers: []string{"Starfield.exe"},
		MO2Name: "Starfield", Nexus: "starfield",
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
