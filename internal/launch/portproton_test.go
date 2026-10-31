package launch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A trimmed copy of what PortProton writes for a game.
const samplePPDB = `#!/usr/bin/env bash
#Author: stanislav
#MiSideFull.exe
export PW_WINE_USE="PROTON_LG_10-28"
export PW_DLL_INSTALL=""
export WINEDLLOVERRIDES=""
export PW_WINDOWS_VER="10"
`

func writePPDB(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Game.exe.ppdb")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func overridesOf(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if m := overridesLine.FindStringSubmatch(line); m != nil {
			return unquoteShell(m[2])
		}
	}
	t.Fatal("no WINEDLLOVERRIDES line")
	return ""
}

func TestPPDBOverrideRoundTrip(t *testing.T) {
	path := writePPDB(t, samplePPDB)

	added, err := addPPDBOverride(path, "winhttp", "n,b")
	if err != nil || !added {
		t.Fatalf("addPPDBOverride = %v, %v", added, err)
	}
	if got := overridesOf(t, path); got != "winhttp=n,b" {
		t.Errorf("overrides = %q, want winhttp=n,b", got)
	}

	if err := removePPDBOverride(path, "winhttp", "n,b"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != samplePPDB {
		t.Errorf("file not restored:\n%s", data)
	}
}

func TestPPDBOverrideKeepsOthers(t *testing.T) {
	path := writePPDB(t, strings.Replace(samplePPDB, `WINEDLLOVERRIDES=""`, `WINEDLLOVERRIDES="d3d8,mss32=n,b"`, 1))

	if added, err := addPPDBOverride(path, "winhttp", "n,b"); err != nil || !added {
		t.Fatalf("addPPDBOverride = %v, %v", added, err)
	}
	if got := overridesOf(t, path); got != "d3d8,mss32=n,b;winhttp=n,b" {
		t.Errorf("overrides = %q", got)
	}
	if err := removePPDBOverride(path, "winhttp", "n,b"); err != nil {
		t.Fatal(err)
	}
	if got := overridesOf(t, path); got != "d3d8,mss32=n,b" {
		t.Errorf("after removal overrides = %q, want the user's entry only", got)
	}
}

// A winhttp override the user set up is theirs: nothing is added, so
// nothing is removed later either.
func TestPPDBOverrideRespectsUsersEntry(t *testing.T) {
	path := writePPDB(t, strings.Replace(samplePPDB, `WINEDLLOVERRIDES=""`, `WINEDLLOVERRIDES="WinHTTP=b"`, 1))
	if added, err := addPPDBOverride(path, "winhttp", "n,b"); err != nil || added {
		t.Fatalf("addPPDBOverride = %v, %v, want no change", added, err)
	}
	if got := overridesOf(t, path); got != "WinHTTP=b" {
		t.Errorf("overrides = %q", got)
	}
}

// PortProton writes the file on the first launch; before that there is
// nothing to change and that is not an error.
func TestPPDBOverrideMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Game.exe.ppdb")
	if added, err := addPPDBOverride(path, "winhttp", "n,b"); err != nil || added {
		t.Fatalf("addPPDBOverride = %v, %v", added, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a settings file was created")
	}
}

func TestPortProtonExe(t *testing.T) {
	exe, ok := portProtonExe(PortProtonCommand("/games/MiSide/MiSideFull.exe"))
	if !ok || exe != "/games/MiSide/MiSideFull.exe" {
		t.Errorf("portProtonExe = %q, %v", exe, ok)
	}
	if _, ok := portProtonExe([]string{"wine", "Game.exe"}); ok {
		t.Error("a plain wine command was taken for PortProton")
	}
}
