package launch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"hermit/internal/steam"
)

var requireTool = regexp.MustCompile(`"require_tool_appid"\s+"(\d+)"`)

// SteamProtonCommand runs a Windows program in a game's Proton prefix the way
// Steam runs the game: with the Proton Steam last ran the prefix with, inside
// the Steam Linux Runtime that Proton asks for. compatData is the game's
// compatdata/<appid> folder. The variables Proton needs are part of the
// command, so they reach it whatever environment runs it.
func SteamProtonCommand(steamRoots []string, compatData, appID, exe string) ([]string, error) {
	proton, err := prefixProton(compatData)
	if err != nil {
		return nil, err
	}
	if len(steamRoots) == 0 {
		return nil, errors.New("Steam was not found")
	}
	command := []string{
		"env",
		"STEAM_COMPAT_DATA_PATH=" + compatData,
		"STEAM_COMPAT_CLIENT_INSTALL_PATH=" + steamRoots[0],
		"SteamAppId=" + appID,
		"SteamGameId=" + appID,
	}
	manifest, err := os.ReadFile(filepath.Join(proton, "toolmanifest.vdf"))
	if err != nil {
		return nil, fmt.Errorf("reading Proton's tool manifest: %w", err)
	}
	if m := requireTool.FindSubmatch(manifest); m != nil {
		runtime := ""
		for _, app := range steam.InstalledApps(steamRoots) {
			if app.AppID == string(m[1]) {
				runtime = filepath.Join(app.InstallPath, "_v2-entry-point")
			}
		}
		if _, err := os.Stat(runtime); runtime == "" || err != nil {
			return nil, fmt.Errorf("the Steam Linux Runtime %s needs (app %s) is not installed", filepath.Base(proton), m[1])
		}
		command = append(command, runtime, "--verb=waitforexitandrun", "--")
	}
	return append(command, filepath.Join(proton, "proton"), "waitforexitandrun", exe), nil
}

// prefixProton is the Proton folder Steam last ran a prefix with, as it
// notes in config_info: the paths there lead into Proton's files folder.
func prefixProton(compatData string) (string, error) {
	data, err := os.ReadFile(filepath.Join(compatData, "config_info"))
	if err != nil {
		return "", errors.New("Steam has not run this game through Proton yet; start it from Steam once")
	}
	for _, line := range strings.Split(string(data), "\n") {
		if dir, _, ok := strings.Cut(strings.TrimSpace(line), "/files/"); ok {
			if _, err := os.Stat(filepath.Join(dir, "proton")); err == nil {
				return dir, nil
			}
		}
	}
	return "", errors.New("the Proton Steam runs this game with was not found; start the game from Steam once")
}
