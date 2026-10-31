package bethesda

import (
	"os"
	"path/filepath"
)

// SteamCompatData is the Proton compatibility data folder, compatdata/<appid>,
// that Steam keeps for a game in the library the game is installed in, or ""
// when there is none.
func SteamCompatData(gamePath, appID string) string {
	if appID == "" {
		return ""
	}
	// <library>/steamapps/common/<game> → <library>/steamapps/compatdata/<appid>
	dir := filepath.Join(filepath.Dir(filepath.Dir(gamePath)), "compatdata", appID)
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		return dir
	}
	return ""
}

// prefixUser is the Windows user's folder inside a compatibility data folder.
func prefixUser(compatData string) string {
	return filepath.Join(compatData, "pfx", "drive_c", "users", "steamuser")
}

// ExtenderLogs is the folder the script extender and its plugins write their
// logs to, Documents\My Games\<game>\SKSE for Skyrim.
func ExtenderLogs(compatData string, g Game) string {
	return filepath.Join(prefixUser(compatData), "Documents", "My Games", g.MyGames, g.Extender)
}
