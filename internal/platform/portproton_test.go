package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPortProtonPrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	data := filepath.Join(home, ".var/app/ru.linux_gaming.PortProton")
	os.MkdirAll(filepath.Join(data, "data/prefixes/DEFAULT/drive_c"), 0o755)
	os.MkdirAll(filepath.Join(data, "data/prefixes/SIMS/drive_c"), 0o755)
	// ~/PortProton/prefixes, as PortProton links it.
	os.Symlink(data, filepath.Join(home, "PortProton"))
	os.Symlink("data/prefixes", filepath.Join(data, "prefixes"))
	prefixes := filepath.Join(home, "PortProton/prefixes")
	exe := filepath.Join(t.TempDir(), "TS4_x64.exe")
	if got := PortProtonPrefix(exe); got != filepath.Join(prefixes, "DEFAULT") {
		t.Errorf("without settings: %q", got)
	}
	os.WriteFile(exe+".ppdb", []byte("#!/usr/bin/env bash\nexport PW_PREFIX_NAME=\"SIMS\"\nexport PW_VULKAN_USE=2\n"), 0o644)
	if got := PortProtonPrefix(exe); got != filepath.Join(prefixes, "SIMS") {
		t.Errorf("named: %q", got)
	}
	os.WriteFile(exe+".ppdb", []byte("export PW_PREFIX_NAME=\"NONE\"\n"), 0o644)
	if got := PortProtonPrefix(exe); got != "" {
		t.Errorf("a prefix that is not there: %q", got)
	}
}
