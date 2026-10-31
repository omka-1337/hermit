package library

import (
	"strings"

	"hermit/internal/creation"
	"hermit/internal/deploy"
	"hermit/internal/redengine"
	"hermit/internal/unreal"
)

// Deployed reports whether the mods of the engine's games are files laid
// out as the game's, linked into it for each session as package deploy
// does, rather than BepInEx plugins. Their profiles are an ordered list of
// mods, each in a folder of its own.
func (e Engine) Deployed() bool {
	switch e {
	case EngineCreation, EngineRED, EngineUnreal:
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
			return Modding{Target: redengine.Target, Layout: rg.Shape.Layout, Nexus: rg.Nexus, Launchers: rg.Launchers}, true
		}
	case EngineUnreal:
		if ug, ok := unreal.Find(g.Executable); ok {
			return Modding{Target: unreal.Target, Layout: ug.Shape.Layout, Nexus: ug.Nexus, Launchers: ug.Launchers}, true
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
	return "", false
}
