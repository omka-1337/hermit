package bethesda

// Layout of a profile of a Creation Engine game:
//
//	mods/<mod-id>/...       files that go into Data, as they are laid out there
//	mods/<mod-id>/Root/...  files that go into the game folder itself
//	overwrite/...           files the game and tools wrote into Data
//	plugins.txt             the plugin list, "*" marking active plugins
//
// The Root folder follows MO2's Root Builder, so mods set up there come over
// unchanged.
const (
	ModsDir      = "mods"
	OverwriteDir = "overwrite"
	PluginsFile  = "plugins.txt"
	rootDir      = "root"
)
