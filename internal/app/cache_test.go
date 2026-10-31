package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheSizeAndClear(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	files := map[string]int{
		"thunderstore/packages/A-Mod-1.0.0.zip": 1000,
		"thunderstore/schema.json":              200,
		"github/owner/repo/v1/mod.zip":          50,
	}
	for name, size := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := NewCacheService(dir)

	if size, err := s.Size(); err != nil || size != 1250 {
		t.Fatalf("Size = %d, %v; want 1250", size, err)
	}
	freed, err := s.Clear()
	if err != nil || freed != 1250 {
		t.Fatalf("Clear = %d, %v; want 1250 freed", freed, err)
	}
	if size, _ := s.Size(); size != 0 {
		t.Errorf("size after clearing = %d", size)
	}
	// The folder itself stays for the clients to write into.
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Error("the cache folder was removed")
	}
}

func TestCacheMissingFolder(t *testing.T) {
	s := NewCacheService(filepath.Join(t.TempDir(), "never-created"))
	if size, err := s.Size(); err != nil || size != 0 {
		t.Errorf("Size = %d, %v", size, err)
	}
	if freed, err := s.Clear(); err != nil || freed != 0 {
		t.Errorf("Clear = %d, %v", freed, err)
	}
}
