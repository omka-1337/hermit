package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"hermit/internal/bethesda"
	"hermit/internal/launch"
	"hermit/internal/library"
	"hermit/internal/plugininfo"
)

// BethesdaService manages the profiles of Creation Engine games, whose mods
// each keep a folder of their own and are deployed into Data in order.
type BethesdaService struct {
	lib *library.Library
}

func NewBethesdaService(lib *library.Library) *BethesdaService {
	return &BethesdaService{lib: lib}
}

// InspectMO2 reads a Mod Organizer 2 instance, to show what importing it
// would bring.
func (s *BethesdaService) InspectMO2(dir string) (bethesda.MO2Instance, error) {
	return bethesda.ReadMO2(dir)
}

// ImportMO2 fills an empty profile with the mods, their order and the plugin
// list of a profile of an MO2 instance, together with MO2's overwrite folder.
func (s *BethesdaService) ImportMO2(gameID, profileID, dir, mo2Profile string) (library.Profile, error) {
	game, bg, err := s.creationGame(gameID)
	if err != nil {
		return library.Profile{}, err
	}
	if err := s.notRunning(gameID); err != nil {
		return library.Profile{}, err
	}
	profile, err := s.lib.GetProfile(gameID, profileID)
	if err != nil {
		return library.Profile{}, err
	}
	if len(profile.Mods) > 0 {
		return library.Profile{}, errors.New("import into an empty profile: this one already has mods")
	}
	in, err := bethesda.ReadMO2(dir)
	if err != nil {
		return library.Profile{}, err
	}
	if in.GameName != bg.MO2Name {
		return library.Profile{}, fmt.Errorf("this MO2 instance is set up for %s, not %s", in.GameName, game.Name)
	}
	p, err := in.ReadProfile(mo2Profile)
	if err != nil {
		return library.Profile{}, err
	}
	profileDir, err := s.lib.ProfileDir(gameID, profileID)
	if err != nil {
		return library.Profile{}, err
	}
	data, err := gameData(game.Path)
	if err != nil {
		return library.Profile{}, err
	}

	ids := make([]string, len(p.Mods))
	mods := make([]library.Mod, len(p.Mods))
	for i, m := range p.Mods {
		ids[i] = uniqueModID(m.Folder, ids[:i])
		if m.Separator {
			mods[i] = separator(ids[i], m.SeparatorName())
			continue
		}
		source := library.ModSource{Type: library.SourceLocal, URL: m.File}
		if m.NexusID > 0 {
			source = library.ModSource{Type: library.SourceNexus, URL: bg.NexusURL(m.NexusID), Asset: m.File}
		}
		mods[i] = library.Mod{
			ID:                ids[i],
			Name:              m.Folder,
			Version:           m.Version,
			Enabled:           m.Enabled,
			Active:            m.Enabled,
			UnmetDependencies: []string{},
			Source:            source,
			Dependencies:      []string{},
			Files:             []string{},
			Plugins:           []plugininfo.Plugin{},
			Notes:             m.Comments,
		}
	}
	if err := in.ImportMO2(p, ids, profileDir, data); err != nil {
		// Take back what was brought over, so the import can be tried again.
		os.RemoveAll(filepath.Join(profileDir, bethesda.ModsDir))
		os.RemoveAll(filepath.Join(profileDir, bethesda.OverwriteDir))
		os.MkdirAll(filepath.Join(profileDir, bethesda.ModsDir), 0o755)
		return library.Profile{}, err
	}
	return s.lib.UpdateProfile(gameID, profileID, func(p *library.Profile) error {
		p.Mods = mods
		return nil
	})
}

// AddSeparator puts a new separator, a heading over the mods after it, at
// index in the profile's order.
func (s *BethesdaService) AddSeparator(gameID, profileID, name string, index int) (library.Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return library.Profile{}, library.ErrEmptyName
	}
	return s.lib.UpdateProfile(gameID, profileID, func(p *library.Profile) error {
		taken := make([]string, len(p.Mods))
		for i, m := range p.Mods {
			taken[i] = m.ID
		}
		index = max(0, min(index, len(p.Mods)))
		p.Mods = slices.Insert(p.Mods, index, separator(uniqueModID(name+" separator", taken), name))
		return nil
	})
}

// RenameSeparator changes the heading of a separator.
func (s *BethesdaService) RenameSeparator(gameID, profileID, id, name string) (library.Profile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return library.Profile{}, library.ErrEmptyName
	}
	return s.lib.UpdateProfile(gameID, profileID, func(p *library.Profile) error {
		i := slices.IndexFunc(p.Mods, func(m library.Mod) bool { return m.ID == id && m.Separator })
		if i < 0 {
			return fmt.Errorf("separator %q: %w", id, library.ErrNotFound)
		}
		p.Mods[i].Name = name
		return nil
	})
}

func separator(id, name string) library.Mod {
	return library.Mod{
		ID:                id,
		Name:              name,
		Separator:         true,
		UnmetDependencies: []string{},
		Dependencies:      []string{},
		Files:             []string{},
		Plugins:           []plugininfo.Plugin{},
	}
}

// MoveMod puts a mod at index in the profile's order, where later mods win
// over earlier ones.
func (s *BethesdaService) MoveMod(gameID, profileID, modID string, index int) (library.Profile, error) {
	return s.lib.UpdateProfile(gameID, profileID, func(p *library.Profile) error {
		from := slices.IndexFunc(p.Mods, func(m library.Mod) bool { return m.ID == modID })
		if from < 0 {
			return fmt.Errorf("mod %q: %w", modID, library.ErrNotFound)
		}
		m := p.Mods[from]
		p.Mods = slices.Delete(p.Mods, from, from+1)
		index = max(0, min(index, len(p.Mods)))
		p.Mods = slices.Insert(p.Mods, index, m)
		return nil
	})
}

// RemoveMod deletes a mod and its folder from the profile, or a separator
// from its list.
func (s *BethesdaService) RemoveMod(gameID, profileID, modID string) (library.Profile, error) {
	if err := s.notRunning(gameID); err != nil {
		return library.Profile{}, err
	}
	profileDir, err := s.lib.ProfileDir(gameID, profileID)
	if err != nil {
		return library.Profile{}, err
	}
	return s.lib.UpdateProfile(gameID, profileID, func(p *library.Profile) error {
		i := slices.IndexFunc(p.Mods, func(m library.Mod) bool { return m.ID == modID })
		if i < 0 {
			return fmt.Errorf("mod %q: %w", modID, library.ErrNotFound)
		}
		if err := os.RemoveAll(filepath.Join(profileDir, bethesda.ModsDir, modID)); err != nil {
			return err
		}
		p.Mods = slices.Delete(p.Mods, i, i+1)
		return nil
	})
}

func (s *BethesdaService) creationGame(gameID string) (library.Game, bethesda.Game, error) {
	game, err := s.lib.GetGame(gameID)
	if err != nil {
		return library.Game{}, bethesda.Game{}, err
	}
	bg, ok := bethesda.Find(game.Executable)
	if game.Engine != library.EngineCreation || !ok {
		return library.Game{}, bethesda.Game{}, fmt.Errorf("%s is not a Creation Engine game", game.Name)
	}
	return game, bg, nil
}

// notRunning refuses changes to mod folders while the game has them linked in.
func (s *BethesdaService) notRunning(gameID string) error {
	dataDir, err := s.lib.GameDataDir(gameID)
	if err != nil {
		return err
	}
	if session, err := launch.ReadSession(dataDir); err == nil && session.Running() {
		return errors.New("the game is running; change its mods once it has exited")
	}
	return nil
}

// gameData returns the game's Data folder, in whatever letter case it has.
func gameData(gamePath string) (string, error) {
	entries, err := os.ReadDir(gamePath)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IsDir() && strings.EqualFold(e.Name(), "Data") {
			return filepath.Join(gamePath, e.Name()), nil
		}
	}
	return "", errors.New("the game folder has no Data folder")
}

var nonModID = regexp.MustCompile(`[^a-z0-9]+`)

// uniqueModID makes a folder name for a mod from its name, unlike the ones
// in taken.
func uniqueModID(name string, taken []string) string {
	base := strings.Trim(nonModID.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if base == "" {
		base = "mod"
	}
	id := base
	for n := 2; slices.Contains(taken, id); n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}
