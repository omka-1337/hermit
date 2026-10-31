// Package deploy puts the mods of a profile into a game for the length of a
// session, for games whose mods are files laid out as the game's own: a
// profile's mods are folders, in an order that decides which of two mods
// with the same file wins, Deploy links their files into the game folder and
// Undeploy takes them out again, keeping what the game wrote meanwhile in
// the profile's overwrite folder.
package deploy

// Layout of such a profile:
//
//	mods/<mod-id>/...       a mod's files, laid out as the game's
//	mods/<mod-id>/Root/...  with a Target's Data, files that go into the
//	                        game folder itself
//	overwrite/...           files the game and tools wrote during sessions
//	downloads/              the archives of mods downloaded from Nexus Mods
//	.staging/               archives unpacked to be installed
//
// The Root folder follows MO2's Root Builder, so mods set up there come over
// unchanged.
const (
	ModsDir      = "mods"
	OverwriteDir = "overwrite"
	DownloadsDir = "downloads"
	RootDir      = "root"
	// StagingDir is on the profile's disk, so the files of an unpacked
	// archive move into the mod's folder without being copied.
	StagingDir = ".staging"
)

// Target is where in a game the mods of its profiles go.
type Target struct {
	// Data is the folder of the game mods are laid out as, Data for the
	// Creation Engine; empty for the game folder itself. With one, a mod's
	// Root folder holds what goes into the game folder itself.
	Data string
	// Replace lets the file of a mod take the place of the game's own file
	// by that name for the session, as the mods of Unreal Engine 3 games
	// do with the game's packages; otherwise the game's file stays.
	Replace bool
}
