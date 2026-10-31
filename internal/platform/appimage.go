package platform

import (
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
)

// originalEnvVar is set by Hermit's first AppRun hook in the AppImage to the
// environment before the AppImage changed it (NUL-separated, base64).
const originalEnvVar = "HERMIT_ORIGINAL_ENV"

// OriginalEnv returns the environment Hermit was started with, before the
// AppImage's AppRun added library paths, GTK settings and such. Outside an
// AppImage it is simply the current environment.
func OriginalEnv() []string {
	encoded := os.Getenv(originalEnvVar)
	if encoded == "" {
		return os.Environ()
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return os.Environ()
	}
	var env []string
	for _, kv := range bytes.Split(data, []byte{0}) {
		name, _, _ := bytes.Cut(kv, []byte("="))
		if len(kv) > 0 && !appImageVars[string(name)] {
			env = append(env, string(kv))
		}
	}
	return env
}

// appImageVars are set by the AppImage runtime before AppRun runs; they
// describe Hermit's image, not the environment of a game it launches.
var appImageVars = map[string]bool{
	originalEnvVar: true, "APPDIR": true, "APPIMAGE": true, "ARGV0": true, "OWD": true,
}

// OriginalWorkingDir is the directory Hermit was started from. The AppImage
// changes into its own directory so the bundled WebKit finds its helper
// processes; the AppImage runtime keeps the original one in $OWD.
func OriginalWorkingDir() string {
	if owd := os.Getenv("OWD"); owd != "" && os.Getenv("APPIMAGE") != "" {
		return owd
	}
	dir, _ := os.Getwd()
	return dir
}

// Command prepares an external program (xdg-open, notify-send, ...) with the
// original environment, so it does not load the AppImage's bundled libraries.
func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Env = OriginalEnv()
	return cmd
}

// ExecutablePath is the path a user would run Hermit by. Inside an AppImage
// the running binary lives in a temporary mount, so the image itself is the
// answer: that is what goes into a Steam shortcut or launch options.
func ExecutablePath() (string, error) {
	if image := os.Getenv("APPIMAGE"); image != "" {
		return image, nil
	}
	return os.Executable()
}
