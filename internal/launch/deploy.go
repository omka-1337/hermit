package launch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"hermit/internal/creation"
	"hermit/internal/deploy"
	"hermit/internal/library"
)

// deployFile keeps what deploying a game's mods changed for its session in
// its data directory: a deployed profile can have tens of thousands of links,
// more than launch.json should carry. It holds a creation.State, which is a
// deploy.State with what a Creation Engine game is given besides; for the
// games of other engines only the deploy.State is set.
const deployFile = "deploy.json"

// deployMods links the enabled mods of a profile into a game whose mods are
// deployed, and gives a Creation Engine game the profile's plugin list and
// INI files. It returns the command and environment to run: Wine loads the
// DLLs mods put beside the game's executable instead of its own, and a
// Creation Engine game starts with its script extender instead of its
// launcher once the game folder has one, unless the user picked the file
// the game starts from or a tool runs instead of the game.
func deployMods(game library.Game, profile library.Profile, profileDir, dataDir string, command, env []string, tool bool) ([]string, []string, error) {
	modding, ok := game.Modding()
	if !ok {
		return nil, nil, fmt.Errorf("Hermit does not know how to put mods into %s (%s)", game.Name, game.Executable)
	}
	var mods []string
	for _, m := range profile.Mods {
		if m.Enabled && !m.Separator {
			mods = append(mods, m.ID)
		}
	}
	// Left by a session that could not be finished.
	if err := undeployMods(game, dataDir); err != nil {
		return nil, nil, fmt.Errorf("removing the mods of the last session: %w", err)
	}
	var state creation.State
	var err error
	cg, isCreation := creation.Find(game.Executable)
	isCreation = isCreation && game.Engine == library.EngineCreation
	if isCreation {
		var prefix creation.Prefix
		if compatData := compatDataDir(game, command, env); compatData != "" {
			prefix = creation.PrefixFor(compatData, cg)
		}
		state, err = creation.Deploy(game.Path, profileDir, mods, prefix)
	} else {
		state.State, err = deploy.Deploy(game.Path, profileDir, mods, modding.Target)
	}
	if err != nil {
		return nil, nil, err
	}
	if err := writeDeployState(dataDir, state); err != nil {
		undeployState(game, modding, state)
		return nil, nil, err
	}
	env = withNativeDLLs(env, game, state.Links)
	if !isCreation || game.LaunchExecutable != "" || tool {
		return command, env, nil
	}
	return withLoader(command, game.Path, cg), env, nil
}

// withNativeDLLs has Wine load the DLLs linked beside the game's executable
// from there rather than its own builtin ones, as the loaders of mods need:
// Cyber Engine Tweaks comes as version.dll, RED4ext as winmm.dll.
func withNativeDLLs(env []string, game library.Game, links []string) []string {
	exeDir := filepath.Dir(filepath.FromSlash(game.Executable))
	for _, link := range links {
		name := filepath.Base(link)
		if strings.EqualFold(filepath.Dir(link), exeDir) && strings.EqualFold(filepath.Ext(name), ".dll") {
			env = withDLLOverride(env, strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name))), "n,b")
		}
	}
	return env
}

// undeployMods finishes what deployMods started. deploy.json stays until
// that succeeds, so a later session tries again.
func undeployMods(game library.Game, dataDir string) error {
	data, err := os.ReadFile(filepath.Join(dataDir, deployFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var state creation.State
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	modding, ok := game.Modding()
	if !ok {
		return fmt.Errorf("Hermit does not know how to take the mods out of %s (%s)", game.Name, game.Executable)
	}
	if err := undeployState(game, modding, state); err != nil {
		return err
	}
	return os.Remove(filepath.Join(dataDir, deployFile))
}

func undeployState(game library.Game, modding library.Modding, state creation.State) error {
	var err error
	if game.Engine == library.EngineCreation {
		_, err = creation.Undeploy(game.Path, state)
	} else {
		_, err = deploy.Undeploy(game.Path, modding.Target, state.State)
	}
	return err
}

func writeDeployState(dataDir string, s creation.State) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, deployFile), data, 0o644)
}

// compatDataDir finds the Proton prefix the game runs in: Steam says where
// in the environment, and for a game started some other way it is the one
// Steam keeps next to the game, unless PortProton, with prefixes of its
// own, runs it.
func compatDataDir(game library.Game, command, env []string) string {
	for _, kv := range env {
		if dir, ok := strings.CutPrefix(kv, "STEAM_COMPAT_DATA_PATH="); ok && dir != "" {
			return dir
		}
	}
	if _, ok := portProtonExe(command); ok {
		return ""
	}
	return creation.SteamCompatData(game.Path, game.SteamAppID)
}

// withLoader replaces the game's launcher in command with its script
// extender's loader when the game folder has one, from a mod or put there by
// hand.
func withLoader(command []string, gamePath string, g creation.Game) []string {
	entries, err := os.ReadDir(gamePath)
	if err != nil {
		return command
	}
	loader := ""
	for _, e := range entries {
		if strings.EqualFold(e.Name(), g.Loader) {
			loader = e.Name()
		}
	}
	if loader == "" {
		return command
	}
	return withExecutable(command, gamePath, filepath.Join(gamePath, loader), g.Launchers...)
}
