package library

import (
	"path/filepath"
	"strings"

	"hermit/internal/creation"
	"hermit/internal/deploy"
	"hermit/internal/platform"
	"hermit/internal/redengine"
	"hermit/internal/sims4"
	"hermit/internal/unreal"
)

// Deployed reports whether the mods of the engine's games are files laid
// out as the game's, linked into it for each session as package deploy
// does, rather than BepInEx plugins. Their profiles are an ordered list of
// mods, each in a folder of its own.
func (e Engine) Deployed() bool {
	switch e {
	case EngineCreation, EngineRED, EngineUnreal, EngineSims4:
		return true
	}
	return false
}

// Modding is how the mods of a game whose mods are deployed go into it.
type Modding struct {
	Target deploy.Target
	// Layout says where the files of a mod archive go in a mod's folder.
	Layout deploy.Layout
	// Nexus is the game's part of nexusmods.com, as in its addresses.
	Nexus string
	// Launchers are the files Steam starts the game from.
	Launchers []string
}

// Modding tells how the mods of the game go into it; ok is false for a game
// modded through BepInEx, or one of an engine Hermit deploys mods for that
// it does not know.
func (g Game) Modding() (m Modding, ok bool) {
	switch g.Engine {
	case EngineCreation:
		if cg, ok := creation.Find(g.Executable); ok {
			return Modding{Target: creation.Target, Layout: creation.Layout, Nexus: cg.Nexus, Launchers: cg.Launchers}, true
		}
	case EngineRED:
		if rg, ok := redengine.Find(g.Executable); ok {
			shape := rg.Shape
			shape.Game = g.Path
			return Modding{Target: redengine.Target, Layout: shape.Layout, Nexus: rg.Nexus, Launchers: rg.Launchers}, true
		}
	case EngineSims4:
		return Modding{Target: sims4.Target, Layout: sims4.Shape.Layout, Nexus: sims4.Nexus}, true
	case EngineUnreal:
		if ug, ok := unreal.Find(g.Executable); ok {
			shape := ug.Shape
			shape.Game = g.Path
			return Modding{Target: unreal.Target, Layout: shape.Layout, Nexus: ug.Nexus, Launchers: ug.Launchers}, true
		}
	}
	return Modding{}, false
}

// NexusGame is the name of the game whose part of nexusmods.com is domain,
// among the games Hermit deploys mods for.
func NexusGame(domain string) (string, bool) {
	for _, g := range creation.Games {
		if strings.EqualFold(g.Nexus, domain) {
			return g.MO2Name, true
		}
	}
	for _, g := range redengine.Games {
		if strings.EqualFold(g.Nexus, domain) {
			return g.Name, true
		}
	}
	for _, g := range unreal.Games {
		if strings.EqualFold(g.Nexus, domain) {
			return g.Name, true
		}
	}
	if strings.EqualFold(sims4.Nexus, domain) {
		return "The Sims 4", true
	}
	return "", false
}

// ModsRoot is the folder the game's mods are deployed into: the game's own
// folder, but for The Sims 4 its Mods folder in the Wine prefix it runs in,
// prefix when one is given, as the environment of a session says, or the
// one chosen for the game, or Steam's, or the one PortProton runs it in.
func (g Game) ModsRoot(prefix string) (string, error) {
	if g.Engine != EngineSims4 {
		return g.Path, nil
	}
	if prefix == "" {
		prefix = g.Prefix
	}
	if prefix == "" {
		prefix = creation.SteamCompatData(g.Path, g.SteamAppID)
	}
	if prefix == "" {
		exe := g.LaunchExecutable
		if exe == "" {
			exe = g.Executable
		}
		prefix = platform.PortProtonPrefix(filepath.Join(g.Path, filepath.FromSlash(exe)))
	}
	return sims4.ModsDir(prefix)
}
