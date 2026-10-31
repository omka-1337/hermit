package library

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"hermit/internal/creation"
	"hermit/internal/deploy"
	"hermit/internal/redengine"
	"hermit/internal/unreal"
)

// Engine is what a game is built on; it decides how the game is modded.
type Engine string

const (
	EngineUnknown  Engine = "unknown"
	EngineUnity    Engine = "unity"
	EngineCreation Engine = "creation"  // the Creation Engine
	EngineRED      Engine = "redengine" // CD Projekt Red's REDengine
	EngineUnreal   Engine = "unreal"
)

// Detection is what can be learned about a game from its install directory.
type Detection struct {
	Unity  bool   `json:"unity"`
	Engine Engine `json:"engine"`
	// EngineVersion is the Unity version, e.g. "2022.3.62f2", "2" for
	// Creation Engine 2, "4" for REDengine 4 or "3" for Unreal Engine 3;
	// empty when it is not known.
	EngineVersion string  `json:"engineVersion"`
	Runtime       Runtime `json:"runtime"`
	Backend       Backend `json:"backend"`
	// Executable is the game executable file name, e.g. "Valheim.exe", or
	// its slash-separated path for a game that keeps it in a folder, as
	// "bin/x64/Cyberpunk2077.exe".
	Executable string `json:"executable"`
}

// DetectGame inspects a game directory without modifying it.
//
// Unity is recognised by UnityPlayer.dll / UnityCrashHandler*.exe (Windows
// builds), UnityPlayer.so (Linux builds), or, for old Unity versions without
// those files, by a <Name>_Data folder with managed or IL2CPP data next to a
// matching executable. Creation Engine games are recognised by their
// executable and the master file in Data, REDengine and Unreal Engine games
// by their executable.
func DetectGame(dir string) Detection {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Detection{Engine: EngineUnknown, Runtime: RuntimeUnknown, Backend: BackendUnknown}
	}
	files := map[string]bool{}
	// names and folders map lower-case names to how they are spelled on disk.
	names, folders := map[string]string{}, map[string]string{}
	var dataDirs []string
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if e.IsDir() {
			if base, ok := strings.CutSuffix(e.Name(), "_Data"); ok {
				dataDirs = append(dataDirs, base)
			}
			folders[name] = e.Name()
			continue
		}
		files[name] = true
		names[name] = e.Name()
	}

	windowsUnity := files["unityplayer.dll"] || files["unitycrashhandler64.exe"] || files["unitycrashhandler32.exe"]
	linuxUnity := files["unityplayer.so"]

	d := Detection{Engine: EngineUnknown, Runtime: RuntimeUnknown, Backend: BackendUnknown}
	for _, base := range dataDirs {
		exe, native := unityExecutable(files, base)
		if exe == "" {
			continue
		}
		dataDir := filepath.Join(dir, base+"_Data")
		if !windowsUnity && !linuxUnity && !isDir(filepath.Join(dataDir, "Managed")) && !isDir(filepath.Join(dataDir, "il2cpp_data")) {
			continue
		}
		d.Unity = true
		d.Engine = EngineUnity
		d.EngineVersion = unityVersion(dataDir)
		d.Executable = exe
		d.Runtime = RuntimeProton
		if native {
			d.Runtime = RuntimeNative
		}
		d.Backend = detectBackend(files, dataDir)
		return d
	}
	if data, ok := folders["data"]; ok {
		for _, g := range creation.Games {
			exe := strings.ToLower(g.Exe)
			if files[exe] && hasFileFold(filepath.Join(dir, data), g.Master) {
				d.Engine = EngineCreation
				d.EngineVersion = g.EngineVersion
				d.Executable = names[exe]
				// Bethesda ships these games for Windows only.
				d.Runtime = RuntimeProton
				return d
			}
		}
	}
	for _, g := range redengine.Games {
		if hasPathFold(dir, g.Exe) {
			d.Engine, d.EngineVersion, d.Executable, d.Runtime = EngineRED, g.EngineVersion, g.Exe, RuntimeProton
			return d
		}
	}
	for _, g := range unreal.Games {
		if hasPathFold(dir, g.Exe) {
			d.Engine, d.EngineVersion, d.Executable, d.Runtime = EngineUnreal, g.EngineVersion, g.Exe, RuntimeProton
			return d
		}
	}
	return d
}

// hasPathFold reports whether dir has a file at the slash-separated path
// rel, its folders and name in any letter case.
func hasPathFold(dir, rel string) bool {
	parts := strings.Split(rel, "/")
	for _, part := range parts[:len(parts)-1] {
		name, err := deploy.FindFold(dir, part)
		if err != nil || name == "" {
			return false
		}
		dir = filepath.Join(dir, name)
	}
	return hasFileFold(dir, parts[len(parts)-1])
}

// unityVersionPattern matches a Unity version such as "2022.3.62f2" or
// "6000.0.67f1".
var unityVersionPattern = regexp.MustCompile(`\d{1,4}\.\d+\.\d+[abfpx]\d+`)

// unityVersion reads the Unity version a game was built with. It is written
// in plain text near the start of the game's first serialized file, or of
// the bundle holding it in builds that pack their data into one file.
func unityVersion(dataDir string) string {
	for _, name := range []string{"globalgamemanagers", "mainData", "data.unity3d"} {
		f, err := os.Open(filepath.Join(dataDir, name))
		if err != nil {
			continue
		}
		head := make([]byte, 256)
		n, _ := io.ReadFull(f, head)
		f.Close()
		if v := unityVersionPattern.Find(head[:n]); v != nil {
			return string(v)
		}
	}
	return ""
}

// hasFileFold reports whether dir has a file named name in any letter case.
func hasFileFold(dir, name string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(e.Name(), name) {
			return true
		}
	}
	return false
}

// unityExecutable finds the executable matching a <base>_Data folder,
// preferring a native Linux binary over a Windows one.
func unityExecutable(files map[string]bool, base string) (name string, native bool) {
	lower := strings.ToLower(base)
	for _, ext := range []string{".x86_64", ".x86"} {
		if files[lower+ext] {
			return base + ext, true
		}
	}
	if files[lower+".exe"] {
		return base + ".exe", false
	}
	return "", false
}

func detectBackend(files map[string]bool, dataDir string) Backend {
	if files["gameassembly.dll"] || files["gameassembly.so"] || isDir(filepath.Join(dataDir, "il2cpp_data")) {
		return BackendIL2CPP
	}
	if isDir(filepath.Join(dataDir, "Managed")) {
		return BackendMono
	}
	return BackendUnknown
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
