package archive

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtract7z(t *testing.T) {
	dir := t.TempDir()
	if err := Extract("testdata/mod.7z", dir, nil); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{"Wrap/Data/meshes/a.nif": "mesh\n", "Wrap/fomod/ModuleConfig.xml": "<config/>\n"} {
		if data, err := os.ReadFile(filepath.Join(dir, rel)); err != nil || string(data) != want {
			t.Errorf("%s: %q, %v", rel, data, err)
		}
	}
}

func TestExtractZipWithWindowsNames(t *testing.T) {
	file := filepath.Join(t.TempDir(), "mod.zip")
	f, _ := os.Create(file)
	w := zip.NewWriter(f)
	for _, name := range []string{`Data\textures\a.dds`, "readme.txt", "data/Textures/b.dds"} {
		fw, _ := w.Create(name)
		fw.Write([]byte(name))
	}
	w.Close()
	f.Close()

	dir := t.TempDir()
	var done, total int64
	if err := Extract(file, dir, func(d, t int64) { done, total = d, t }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Data", "textures", "a.dds")); err != nil {
		t.Errorf("backslash names: %v", err)
	}
	// One folder, however each entry spells it.
	if _, err := os.Stat(filepath.Join(dir, "Data", "textures", "b.dds")); err != nil {
		t.Errorf("folder spelled another way: %v", err)
	}
	if done != total || total == 0 {
		t.Errorf("progress %d of %d", done, total)
	}
}

func TestExtractRefusesEscapes(t *testing.T) {
	file := filepath.Join(t.TempDir(), "evil.zip")
	f, _ := os.Create(file)
	w := zip.NewWriter(f)
	fw, _ := w.Create("../outside.txt")
	fw.Write([]byte("x"))
	w.Close()
	f.Close()
	if err := Extract(file, t.TempDir(), nil); err == nil {
		t.Error("unpacked a file outside the folder")
	}
}
