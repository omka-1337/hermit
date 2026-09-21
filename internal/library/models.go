package library

import "hermit/internal/plugininfo"

type Runtime string

const (
	RuntimeUnknown Runtime = "unknown"
	RuntimeNative  Runtime = "native" // Linux build, BepInEx via run_bepinex.sh
	RuntimeProton  Runtime = "proton" // Windows build run through Proton/Wine
)

// Backend is the Unity scripting backend; it decides which BepInEx build is needed
// (BepInEx 5 supports Mono only, BepInEx 6 also IL2CPP).
type Backend string

const (
	BackendUnknown Backend = "unknown"
	BackendMono    Backend = "mono"
	BackendIL2CPP  Backend = "il2cpp"
)

// Game is stored in games/<id>/game.json. It holds everything machine-specific
// (paths, runtime); profiles must stay portable.
type Game struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Path          string  `json:"path"`
	Runtime       Runtime `json:"runtime"`
	Backend       Backend `json:"backend"`
	Executable    string  `json:"executable"`
	SteamAppID    string  `json:"steamAppId,omitempty"`
	ActiveProfile string  `json:"activeProfile"`
	// LaunchExecutable is the file Hermit starts the game from when it
	// starts it itself, relative to Path; empty means Executable. It is kept
	// apart from Executable, which identifies the game (Thunderstore finds
	// its community by that name), because some mods ship their own launcher.
	LaunchExecutable string `json:"launchExecutable,omitempty"`
}

// GameCandidate is a game that can be added: found in Steam or picked manually.
type GameCandidate struct {
	Path         string    `json:"path"`
	Name         string    `json:"name"`
	SteamAppID   string    `json:"steamAppId,omitempty"`
	Detection    Detection `json:"detection"`
	AlreadyAdded bool      `json:"alreadyAdded"`
}

// ProfileSchemaVersion history:
//
//	1: initial
//	2: Mod.Active added; older profiles have all mods active
const ProfileSchemaVersion = 2

// Profile is stored in profiles/<id>/profile.json and is meant to be shareable:
// no absolute paths or machine-specific data.
type Profile struct {
	SchemaVersion int    `json:"schemaVersion"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	// Modpack is the Thunderstore modpack the profile was created from,
	// "<author>-<name>-<version>"; empty for profiles assembled by hand.
	Modpack string `json:"modpack,omitempty"`
	Mods    []Mod  `json:"mods"`
}

type Mod struct {
	// ID is "<author>-<name>", the Thunderstore full name without version.
	ID      string `json:"id"`
	Name    string `json:"name"`
	Author  string `json:"author"`
	Version string `json:"version"`
	// Enabled is the user's choice.
	Enabled bool `json:"enabled"`
	// Active means the mod's files are in place and loaded by BepInEx: it is
	// enabled and all its dependencies are installed and active. Files of an
	// inactive mod are kept under disabled/<mod-id>/ in the profile.
	Active bool `json:"active"`
	// UnmetDependencies lists dependencies that are not installed or not
	// active; while non-empty the mod cannot be active.
	UnmetDependencies []string  `json:"unmetDependencies"`
	Source            ModSource `json:"source"`
	// Dependencies are "<author>-<name>-<version>" strings from the package manifest.
	Dependencies []string `json:"dependencies"`
	// Files are slash-separated paths relative to the profile directory.
	Files []string `json:"files"`
	// Plugins are the BepInEx plugins found in the mod's assemblies; nil
	// until they have been scanned.
	Plugins []plugininfo.Plugin `json:"plugins"`
}

type ModSourceType string

const (
	SourceLocal        ModSourceType = "local"
	SourceThunderstore ModSourceType = "thunderstore"
	SourceGitHub       ModSourceType = "github"
)

// ModSource describes where a mod came from and how to fetch it again.
type ModSource struct {
	Type ModSourceType `json:"type"`
	// URL is the Thunderstore download URL, the GitHub repository URL, or the
	// file name of a local file.
	URL    string `json:"url,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	// Release and Asset identify the GitHub release file the mod came from.
	Release string `json:"release,omitempty"`
	Asset   string `json:"asset,omitempty"`
}
