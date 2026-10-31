package app

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// CacheService measures and empties Hermit's download cache: mod archives
// kept after installing them, GitHub release files and Thunderstore data.
// Everything there is fetched again when it is needed, so clearing it only
// costs downloads; installed mods live in the profiles and are not touched.
type CacheService struct {
	dir string
}

func NewCacheService(dir string) *CacheService {
	return &CacheService{dir: dir}
}

// Size returns the bytes the cache takes up.
func (s *CacheService) Size() (int64, error) {
	var total int64
	err := filepath.WalkDir(s.dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, err
}

// Clear deletes everything in the cache and returns how many bytes it freed.
// The folder itself stays, since the clients write into it without creating
// it first.
func (s *CacheService) Clear() (int64, error) {
	before, err := s.Size()
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var errs []error
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(s.dir, e.Name())); err != nil {
			errs = append(errs, err)
		}
	}
	after, _ := s.Size()
	return before - after, errors.Join(errs...)
}
