package deploy

import "path/filepath"

// Conflicts finds the mods of a profile that have files in common: for each
// mod, the other mods it shares files with and how many. Which of two mods
// wins is their order, so the answer holds however they are moved; it
// changes only when mods are added or removed.
func Conflicts(profileDir string, mods []string, t Target) (map[string]map[string]int, error) {
	owners := map[string][]int{}
	for i, id := range mods {
		err := walkMod(filepath.Join(profileDir, ModsDir, id), t, func(key string, _ source) {
			owners[key] = append(owners[key], i)
		})
		if err != nil {
			return nil, err
		}
	}
	out := map[string]map[string]int{}
	for _, idx := range owners {
		for _, a := range idx {
			for _, b := range idx {
				if a == b {
					continue
				}
				if out[mods[a]] == nil {
					out[mods[a]] = map[string]int{}
				}
				out[mods[a]][mods[b]]++
			}
		}
	}
	return out, nil
}
