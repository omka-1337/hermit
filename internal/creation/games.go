// Package creation mods games on the Creation Engine: it links the
// enabled mods of a profile into the game's Data folder for the length of a
// session, in the order the profile gives them, and puts the profile's
// plugin list where the game reads it.
package creation

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
	// MyGames is the game's folder in Documents\My Games, where its INI files,
	// saves and logs are; Extender is the script extender's folder in it, the
	// one its plugins write their logs to.
	MyGames  string
	Extender string
	// INIs are the game's settings files in MyGames, as the game names them.
	INIs []string
	// Implicit are the plugins the game loads first whether plugins.txt has
	// them or not, in that order, when they are in Data; after them come
	// those the file CCC in the game folder lists, the Creation Club's.
	Implicit []string
	CCC      string
}

var Games = []Game{
	{
		Exe: "SkyrimSE.exe", Master: "Skyrim.esm", AppData: "Skyrim Special Edition",
		Loader: "skse64_loader.exe", Launchers: []string{"SkyrimSELauncher.exe", "SkyrimSE.exe"},
		MO2Name: "Skyrim Special Edition", Nexus: "skyrimspecialedition",
		MyGames: "Skyrim Special Edition", Extender: "SKSE",
		INIs:     []string{"Skyrim.ini", "SkyrimPrefs.ini", "SkyrimCustom.ini"},
		Implicit: []string{"Skyrim.esm", "Update.esm", "Dawnguard.esm", "HearthFires.esm", "Dragonborn.esm"},
		CCC:      "Skyrim.ccc",
	},
	{
		Exe: "SkyrimVR.exe", Master: "Skyrim.esm", AppData: "Skyrim VR",
		Loader: "sksevr_loader.exe", Launchers: []string{"SkyrimVR.exe"},
		MO2Name: "Skyrim VR", Nexus: "skyrimspecialedition",
		MyGames: "Skyrim VR", Extender: "SKSE",
		INIs:     []string{"SkyrimVR.ini", "SkyrimPrefs.ini"},
		Implicit: []string{"Skyrim.esm", "Update.esm", "Dawnguard.esm", "HearthFires.esm", "Dragonborn.esm", "SkyrimVR.esm"},
	},
	{
		Exe: "TESV.exe", Master: "Skyrim.esm", AppData: "Skyrim",
		Loader: "skse_loader.exe", Launchers: []string{"SkyrimLauncher.exe", "TESV.exe"},
		MO2Name: "Skyrim", Nexus: "skyrim",
		MyGames: "Skyrim", Extender: "SKSE",
		INIs: []string{"Skyrim.ini", "SkyrimPrefs.ini"},
		// The first Skyrim loads its DLCs only when plugins.txt has them.
		Implicit: []string{"Skyrim.esm", "Update.esm"},
	},
	{
		Exe: "Fallout4.exe", Master: "Fallout4.esm", AppData: "Fallout4",
		Loader: "f4se_loader.exe", Launchers: []string{"Fallout4Launcher.exe", "Fallout4.exe"},
		MO2Name: "Fallout 4", Nexus: "fallout4",
		MyGames: "Fallout4", Extender: "F4SE",
		INIs: []string{"Fallout4.ini", "Fallout4Prefs.ini", "Fallout4Custom.ini"},
		Implicit: []string{
			"Fallout4.esm", "DLCRobot.esm", "DLCworkshop01.esm", "DLCCoast.esm", "DLCworkshop02.esm",
			"DLCworkshop03.esm", "DLCNukaWorld.esm", "DLCUltraHighResolution.esm",
		},
		CCC: "Fallout4.ccc",
	},
	{
		Exe: "Fallout4VR.exe", Master: "Fallout4.esm", AppData: "Fallout4VR",
		Loader: "f4sevr_loader.exe", Launchers: []string{"Fallout4VR.exe"},
		MO2Name: "Fallout 4 VR", Nexus: "fallout4",
		MyGames: "Fallout4VR", Extender: "F4SE",
		INIs:     []string{"Fallout4Custom.ini", "Fallout4Prefs.ini"},
		Implicit: []string{"Fallout4.esm", "Fallout4_VR.esm"},
	},
	{
		Exe: "Starfield.exe", Master: "Starfield.esm", EngineVersion: "2", AppData: "Starfield",
		Loader: "sfse_loader.exe", Launchers: []string{"Starfield.exe"},
		MO2Name: "Starfield", Nexus: "starfield",
		MyGames: "Starfield", Extender: "SFSE",
		INIs:     []string{"StarfieldCustom.ini", "StarfieldPrefs.ini"},
		Implicit: []string{"Starfield.esm", "Constellation.esm", "OldMars.esm", "BlueprintShips-Starfield.esm"},
		CCC:      "Starfield.ccc",
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
