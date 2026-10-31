package app

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"hermit/internal/creation"
	"hermit/internal/deploy"
	"hermit/internal/library"
	"hermit/internal/platform"
	"hermit/internal/plugininfo"
)

// CreationService does for the profiles of Creation Engine games what only
// they have: importing a Mod Organizer 2 instance and reading the logs of
// the script extender. ModListService manages their mods.
type CreationService struct {
	lib *library.Library
}

func NewCreationService(lib *library.Library) *CreationService {
	return &CreationService{lib: lib}
}

// InspectMO2 reads a Mod Organizer 2 instance, to show what importing it
// would bring.
func (s *CreationService) InspectMO2(dir string) (creation.MO2Instance, error) {
	return creation.ReadMO2(dir)
}

// ImportMO2 fills an empty profile with the mods, their order, the plugin
// list and the INI files of a profile of an MO2 instance, together with MO2's
// overwrite folder.
func (s *CreationService) ImportMO2(gameID, profileID, dir, mo2Profile string) (library.Profile, error) {
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
	in, err := creation.ReadMO2(dir)
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
	data, err := deploy.DataDir(game.Path, creation.Target)
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
	if err := in.ImportMO2(p, ids, profileDir, data, bg.INIs); err != nil {
		// Take back what was brought over, so the import can be tried again.
		os.RemoveAll(filepath.Join(profileDir, deploy.ModsDir))
		os.RemoveAll(filepath.Join(profileDir, deploy.OverwriteDir))
		os.RemoveAll(filepath.Join(profileDir, creation.INIDir))
		os.MkdirAll(filepath.Join(profileDir, deploy.ModsDir), 0o755)
		return library.Profile{}, err
	}
	return s.lib.UpdateProfile(gameID, profileID, func(p *library.Profile) error {
		p.Mods = mods
		return nil
	})
}

// ImportMO2AsProfile creates a profile called name, in a folder of its own
// inside location unless that is empty, and imports a profile of an MO2
// instance into it. Nothing is left behind when the import fails.
func (s *CreationService) ImportMO2AsProfile(gameID, dir, mo2Profile, name, location string) (library.Profile, error) {
	if _, _, err := s.creationGame(gameID); err != nil {
		return library.Profile{}, err
	}
	// A folder that is no MO2 instance should not cost an empty profile.
	if _, err := creation.ReadMO2(dir); err != nil {
		return library.Profile{}, err
	}
	created, err := s.lib.CreateProfile(gameID, strings.TrimSpace(name))
	if err != nil {
		return library.Profile{}, err
	}
	if location != "" {
		if _, err := s.lib.MoveProfile(gameID, created.ID, location); err != nil {
			s.lib.RemoveProfile(gameID, created.ID)
			return library.Profile{}, err
		}
	}
	p, err := s.ImportMO2(gameID, created.ID, dir, mo2Profile)
	if err != nil {
		s.lib.RemoveProfile(gameID, created.ID)
		return library.Profile{}, err
	}
	return p, nil
}

// LogFile is a log the script extender or one of its plugins wrote.
type LogFile struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// maxLog is how much of a log ReadLog returns: the end of a longer one, where
// the last session is.
const maxLog = 2 << 20

// Logs lists the logs in the script extender's folder, SKSE for Skyrim,
// newest first. A game not started yet has none.
func (s *CreationService) Logs(gameID string) ([]LogFile, error) {
	dir, err := s.logDir(gameID)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []LogFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	logs := []LogFile{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		logs = append(logs, LogFile{Name: e.Name(), Size: fi.Size(), Modified: fi.ModTime()})
	}
	slices.SortFunc(logs, func(a, b LogFile) int { return b.Modified.Compare(a.Modified) })
	return logs, nil
}

// ReadLog returns a log from the script extender's folder, or its last
// maxLog bytes when it is longer.
func (s *CreationService) ReadLog(gameID, name string) (string, error) {
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		return "", fmt.Errorf("%q is not a log file", name)
	}
	dir, err := s.logDir(gameID)
	if err != nil {
		return "", err
	}
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return "", err
	}
	cut := ""
	if fi.Size() > maxLog {
		if _, err := f.Seek(-maxLog, io.SeekEnd); err != nil {
			return "", err
		}
		cut = fmt.Sprintf("… only the last %d MB of %d MB are shown\n", maxLog>>20, fi.Size()>>20)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return cut + string(data), nil
}

// OpenLogs shows the script extender's log folder in the file manager.
func (s *CreationService) OpenLogs(gameID string) error {
	dir, err := s.logDir(gameID)
	if err != nil {
		return err
	}
	cmd := platform.Command("xdg-open", dir)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening %s: %w", dir, err)
	}
	go cmd.Wait()
	return nil
}

// logDir is where the script extender of a game run by Steam's Proton
// writes its logs, in the game's prefix.
func (s *CreationService) logDir(gameID string) (string, error) {
	game, bg, err := s.creationGame(gameID)
	if err != nil {
		return "", err
	}
	compatData := creation.SteamCompatData(game.Path, game.SteamAppID)
	if compatData == "" {
		return "", errors.New("the game's Proton prefix was not found; Steam makes it the first time it starts the game")
	}
	return creation.ExtenderLogs(compatData, bg), nil
}

func (s *CreationService) creationGame(gameID string) (library.Game, creation.Game, error) {
	game, err := s.lib.GetGame(gameID)
	if err != nil {
		return library.Game{}, creation.Game{}, err
	}
	bg, ok := creation.Find(game.Executable)
	if game.Engine != library.EngineCreation || !ok {
		return library.Game{}, creation.Game{}, fmt.Errorf("%s is not a Creation Engine game", game.Name)
	}
	return game, bg, nil
}

func (s *CreationService) notRunning(gameID string) error { return notRunning(s.lib, gameID) }
