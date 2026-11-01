package creation

// A profile of a Creation Engine game is laid out as deploy has it, with
// mods laid out as Data is and the game folder's files in their Root
// folders, and with these beside its mods:
//
//	plugins.txt  the plugin list, "*" marking active plugins
//	ini/         the game's INI files, when the profile has its own
//	saves/       the profile's saves, when it keeps them
const (
	PluginsFile = "plugins.txt"
	INIDir      = "ini"
	SavesDir    = "saves"
)
