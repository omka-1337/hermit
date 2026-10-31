package launch

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"hermit/internal/bethesda"
	"hermit/internal/library"
)

// deployFile keeps the bethesda.State of a Creation Engine game's session in
// its data directory: a deployed profile can have tens of thousands of links,
// more than launch.json should carry.
const deployFile = "deploy.json"

// deployCreation links the enabled mods of a profile into the Data folder of
// a Creation Engine game and gives the game the profile's plugin list. It
// returns the command to run, which starts the script extender instead of
// the game's launcher once the game folder has one.
func deployCreation(game library.Game, profile library.Profile, profileDir, dataDir string, command, env []string) ([]string, error) {
	g, ok := bethesda.Find(game.Executable)
	if !ok {
		return nil, fmt.Errorf("%s is not a Creation Engine game Hermit knows", game.Executable)
	}
	var mods []string
	for _, m := range profile.Mods {
		if m.Enabled {
			mods = append(mods, m.ID)
		}
	}
	// Left by a session that could not be finished.
	if err := undeployCreation(game, dataDir); err != nil {
		return nil, fmt.Errorf("removing the mods of the last session: %w", err)
	}
	pluginsPath := ""
	if compatData := compatDataDir(game, command, env); compatData != "" {
		pluginsPath = bethesda.PrefixPluginsPath(compatData, g)
	}
	state, err := bethesda.Deploy(game.Path, profileDir, mods, pluginsPath)
	if err != nil {
		return nil, err
	}
	if err := writeDeployState(dataDir, state); err != nil {
		bethesda.Undeploy(game.Path, state)
		return nil, err
	}
	return withLoader(command, game.Path, g), nil
}

// undeployCreation finishes what deployCreation started. deploy.json stays
// until that succeeds, so a later session tries again.
func undeployCreation(game library.Game, dataDir string) error {
	data, err := os.ReadFile(filepath.Join(dataDir, deployFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var state bethesda.State
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	_, err = bethesda.Undeploy(game.Path, state)
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(dataDir, deployFile))
}

func writeDeployState(dataDir string, s bethesda.State) error {
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
	if _, ok := portProtonExe(command); ok || game.SteamAppID == "" {
		return ""
	}
	// <library>/steamapps/common/<game> → <library>/steamapps/compatdata/<appid>
	dir := filepath.Join(filepath.Dir(filepath.Dir(game.Path)), "compatdata", game.SteamAppID)
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		return dir
	}
	return ""
}

// withLoader replaces the game's launcher in command with its script
// extender's loader when the game folder has one, from a mod or put there by
// hand.
func withLoader(command []string, gamePath string, g bethesda.Game) []string {
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
	for i, arg := range command {
		for _, launcher := range g.Launchers {
			if strings.EqualFold(filepath.Base(arg), launcher) {
				out := append([]string(nil), command...)
				out[i] = filepath.Join(gamePath, loader)
				return out
			}
		}
	}
	return command
}
