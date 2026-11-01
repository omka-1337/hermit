package creation

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"hermit/internal/deploy"
)

// Target is where the mods of a Creation Engine game go: into Data, and
// what their Root folders hold into the game folder.
var Target = deploy.Target{Data: "Data"}

// State is what Deploy changed, kept until Undeploy puts things back.
type State struct {
	deploy.State
	// PluginsPath is the plugins.txt the game reads, replaced for the
	// session; OriginalPlugins is what it held, nil when there was none.
	PluginsPath     string  `json:"pluginsPath,omitempty"`
	OriginalPlugins *string `json:"originalPlugins,omitempty"`
	// INIs are the game's INI files the profile's replaced for the session.
	INIs []INISwap `json:"inis,omitempty"`
	// Saves is the game's Saves folder the profile's replaced.
	Saves *SavesSwap `json:"saves,omitempty"`
}

// Deploy links the files of mods into the game's Data folder, as
// deploy.Deploy does. The profile's plugin list is brought up to date with
// the plugins deployed and given to the game, its INI files, if it has
// them, take the place of the game's, and its saves folder the game's, in
// the places prefix names.
func Deploy(gamePath, profileDir string, mods []string, prefix Prefix) (State, error) {
	ds, err := deploy.Deploy(gamePath, profileDir, mods, Target)
	if err != nil {
		return State{}, err
	}
	s := State{State: ds}
	if prefix.Plugins != "" {
		dataDir, err := deploy.DataDir(gamePath, Target)
		if err == nil {
			err = s.swapPlugins(profileDir, dataDir, deployedPlugins(profileDir, mods), prefix.Plugins)
		}
		if err != nil {
			Undeploy(gamePath, s)
			return State{}, err
		}
	}
	if prefix.MyGames != "" {
		if err := s.swapINIs(profileDir, prefix.MyGames); err != nil {
			Undeploy(gamePath, s)
			return State{}, err
		}
	}
	if prefix.Saves != "" {
		if err := s.swapSaves(profileDir, prefix.Saves); err != nil {
			Undeploy(gamePath, s)
			return State{}, err
		}
	}
	return s, nil
}

// Undeploy takes the mods out of Data as deploy.Undeploy does and puts the
// original plugins.txt back, the game's INI files, keeping what the game
// changed in the profile's, and the game's Saves folder. It returns the files written into Data
// during the session, now in the profile's overwrite folder.
func Undeploy(gamePath string, s State) ([]string, error) {
	captured, err := deploy.Undeploy(gamePath, Target, s.State)
	errs := []error{err, s.restoreINIs(), s.restoreSaves()}
	if s.PluginsPath != "" {
		if s.OriginalPlugins != nil {
			errs = append(errs, os.WriteFile(s.PluginsPath, []byte(*s.OriginalPlugins), 0o644))
		} else if err := os.Remove(s.PluginsPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return captured, errors.Join(errs...)
}

// deployedPlugins are the plugins the mods and the overwrite folder bring,
// in the order they bring them, lowest priority first.
func deployedPlugins(profileDir string, mods []string) []string {
	var plugins []string
	for _, folder := range deploy.Folders(profileDir, mods) {
		entries, _ := os.ReadDir(folder)
		for _, e := range entries {
			if !e.IsDir() && isPlugin(e.Name()) && !slices.ContainsFunc(plugins, func(p string) bool { return strings.EqualFold(p, e.Name()) }) {
				plugins = append(plugins, e.Name())
			}
		}
	}
	return plugins
}

// isPlugin reports whether a Data path is a plugin the game loads.
func isPlugin(rel string) bool {
	if strings.Contains(rel, "/") {
		return false
	}
	switch strings.ToLower(path.Ext(rel)) {
	case ".esp", ".esm", ".esl":
		return true
	}
	return false
}

// Layout is where the files of a mod archive go in a Creation Engine mod's
// folder. An explicit Data folder has Data's, and what is beside it goes
// into the game folder, as with SKSE; otherwise the folder with Data's
// folders or with plugins is Data, looked for through folders an archive is
// wrapped in.
func Layout(dir string) ([]deploy.Part, bool) {
	for {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return []deploy.Part{{From: dir}}, true
		}
		for _, e := range entries {
			if e.IsDir() && strings.EqualFold(e.Name(), "Data") {
				parts := []deploy.Part{{From: filepath.Join(dir, e.Name())}}
				for _, other := range entries {
					if other.Name() != e.Name() && !strings.EqualFold(other.Name(), "src") {
						parts = append(parts, deploy.Part{From: filepath.Join(dir, other.Name()), To: "Root/" + other.Name()})
					}
				}
				return parts, false
			}
		}
		for _, e := range entries {
			name := strings.ToLower(e.Name())
			if (e.IsDir() && slices.Contains(dataFolders, name)) || (!e.IsDir() && isDataFile(name)) {
				return []deploy.Part{{From: dir}}, false
			}
		}
		only := deploy.OnlyFolder(dir)
		if only == "" {
			return []deploy.Part{{From: dir}}, true
		}
		dir = only
	}
}

// dataFolders are folders found at the top of Data, by which a folder of an
// archive is known to be laid out as Data is.
var dataFolders = []string{
	"meshes", "textures", "scripts", "interface", "sound", "music", "skse", "f4se", "sfse", "seq", "strings",
	"video", "materials", "lodsettings", "grass", "shadersfx", "calientetools", "nemesis_engine", "netscriptframework",
	"dyndolod", "source", "lightplacer", "pbrnifpatcher", "pbrtexturesets", "terrain", "vis", "facegen", "distantlod",
	"lod", "shaders", "platform", "tools",
}

func isDataFile(name string) bool {
	switch filepath.Ext(name) {
	case ".esp", ".esm", ".esl", ".bsa", ".ba2":
		return true
	}
	return false
}
