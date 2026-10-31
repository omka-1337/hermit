package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"syscall"

	"hermit/internal/creation"
	"hermit/internal/deploy"
	"hermit/internal/launch"
	"hermit/internal/library"
	"hermit/internal/platform"
	"hermit/internal/plugininfo"
)

// ModListService manages the profiles of games whose mods are deployed, of
// the Creation Engine, REDengine and Unreal Engine: each mod keeps a folder
// of its own, and the order of the mods says which of them wins a file.
type ModListService struct {
	lib        *library.Library
	steamRoots []string
	// OpenWindow opens a window of the app at url, calling onClosed when it
	// closes; main sets it.
	OpenWindow func(url, title string, onClosed func())
	// OnUnpack, when set, follows unpacking an archive as well, for the
	// command line, where there are no events.
	OnUnpack func(UnpackProgress)

	mu       sync.Mutex
	sessions map[string]*installSession
}

func NewModListService(lib *library.Library, steamRoots []string) *ModListService {
	return &ModListService{lib: lib, steamRoots: steamRoots}
}

// AddSeparator puts a new separator, a heading over the mods after it, at
// index in the profile's order.
func (s *ModListService) AddSeparator(gameID, profileID, name string, index int) (library.Profile, error) {
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
func (s *ModListService) RenameSeparator(gameID, profileID, id, name string) (library.Profile, error) {
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

// Conflicts tells, for each mod of the profile, which other mods have files
// in common with it and how many; the order decides which of them wins.
func (s *ModListService) Conflicts(gameID, profileID string) (map[string]map[string]int, error) {
	profile, err := s.lib.GetProfile(gameID, profileID)
	if err != nil {
		return nil, err
	}
	profileDir, err := s.lib.ProfileDir(gameID, profileID)
	if err != nil {
		return nil, err
	}
	var mods []string
	for _, m := range profile.Mods {
		if !m.Separator {
			mods = append(mods, m.ID)
		}
	}
	_, modding, err := s.deployedGame(gameID)
	if err != nil {
		return nil, err
	}
	return deploy.Conflicts(profileDir, mods, modding.Target)
}

// MoveMods moves mods, in the order they have among themselves, to just
// before the mod before, or to the end when before is empty; later mods win
// over earlier ones. Dragging a folded separator moves the mods under it too.
func (s *ModListService) MoveMods(gameID, profileID string, ids []string, before string) (library.Profile, error) {
	return s.lib.UpdateProfile(gameID, profileID, func(p *library.Profile) error {
		var picked, rest []library.Mod
		for _, m := range p.Mods {
			if slices.Contains(ids, m.ID) {
				picked = append(picked, m)
			} else {
				rest = append(rest, m)
			}
		}
		if len(picked) != len(ids) {
			return fmt.Errorf("mods %v: %w", ids, library.ErrNotFound)
		}
		at := len(rest)
		if before != "" {
			if at = slices.IndexFunc(rest, func(m library.Mod) bool { return m.ID == before }); at < 0 {
				return fmt.Errorf("mod %q to move before: %w", before, library.ErrNotFound)
			}
		}
		p.Mods = slices.Concat(rest[:at], picked, rest[at:])
		return nil
	})
}

// Rescan brings a profile's list of mods in line with its mods folder, as
// MO2 does on a refresh: folders put there by hand become mods at the end of
// the list, switched off, and mods whose folder is gone leave the list.
func (s *ModListService) Rescan(gameID, profileID string) (library.Profile, error) {
	profileDir, err := s.lib.ProfileDir(gameID, profileID)
	if err != nil {
		return library.Profile{}, err
	}
	entries, err := os.ReadDir(filepath.Join(profileDir, deploy.ModsDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return library.Profile{}, err
	}
	var folders []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			folders = append(folders, e.Name())
		}
	}
	return s.lib.UpdateProfile(gameID, profileID, func(p *library.Profile) error {
		p.Mods = slices.DeleteFunc(p.Mods, func(m library.Mod) bool {
			return !m.Separator && !slices.Contains(folders, m.ID)
		})
		for _, folder := range folders {
			if slices.ContainsFunc(p.Mods, func(m library.Mod) bool { return m.ID == folder }) {
				continue
			}
			p.Mods = append(p.Mods, library.Mod{
				ID:                folder,
				Name:              folder,
				Source:            library.ModSource{Type: library.SourceLocal},
				UnmetDependencies: []string{},
				Dependencies:      []string{},
				Files:             []string{},
				Plugins:           []plugininfo.Plugin{},
			})
		}
		return nil
	})
}

// RemoveMod deletes a mod and its folder from the profile, or a separator
// from its list.
func (s *ModListService) RemoveMod(gameID, profileID, modID string) (library.Profile, error) {
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
		if err := os.RemoveAll(filepath.Join(profileDir, deploy.ModsDir, modID)); err != nil {
			return err
		}
		p.Mods = slices.Delete(p.Mods, i, i+1)
		return nil
	})
}

// Tool is a modding tool a mod of the profile brings, such as Pandora or
// BodySlide: a program run with the profile's mods deployed.
type Tool struct {
	// Path is where the tool is while the profile is deployed,
	// slash-separated from the folder the game's mods are laid out as, Data
	// for the Creation Engine.
	Path string `json:"path"`
	Name string `json:"name"`
	// Mod is the name of the mod the tool comes with.
	Mod string `json:"mod"`
}

// notTools are programs mods carry along that tools run themselves, never
// the user: converters, compilers, installers of the libraries they ship.
var notTools = regexp.MustCompile(`(?i)^(hkxcmd|papyrus(compiler|assembler)|wininst-.*|unins.*|vc_?redist.*|.*crash.*report.*)\.exe$`)

// Tools lists the tools the enabled mods of the active profile bring, in
// the order of the mods.
func (s *ModListService) Tools(gameID string) ([]Tool, error) {
	game, modding, err := s.deployedGame(gameID)
	if err != nil {
		return nil, err
	}
	profile, err := s.lib.GetProfile(gameID, game.ActiveProfile)
	if err != nil {
		return nil, err
	}
	profileDir, err := s.lib.ProfileDir(gameID, game.ActiveProfile)
	if err != nil {
		return nil, err
	}
	tools := []Tool{}
	for _, m := range profile.Mods {
		if !m.Enabled || m.Separator {
			continue
		}
		modDir := filepath.Join(profileDir, deploy.ModsDir, m.ID)
		filepath.WalkDir(modDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(modDir, p)
			if d.IsDir() {
				// Root holds what starts the game, Lib a tool's own libraries.
				name := strings.ToLower(d.Name())
				if rel != "." && (modding.Target.Data != "" && name == deploy.RootDir && filepath.Dir(rel) == "." || name == "lib") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.EqualFold(filepath.Ext(p), ".exe") && !notTools.MatchString(d.Name()) {
				tools = append(tools, Tool{Path: filepath.ToSlash(rel), Name: strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())), Mod: m.Name})
			}
			return nil
		})
	}
	return tools, nil
}

// RunTool runs a tool of the profile in the game's Proton prefix, the way
// Steam runs the game, with the profile deployed for as long as it runs;
// what the tool writes into the game is kept in the profile's overwrite
// folder.
// It returns once the tool has started.
func (s *ModListService) RunTool(gameID, path string) error {
	game, modding, err := s.deployedGame(gameID)
	if err != nil {
		return err
	}
	if err := s.notRunning(gameID); err != nil {
		return err
	}
	tools, err := s.Tools(gameID)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(tools, func(t Tool) bool { return t.Path == path }) {
		return fmt.Errorf("no mod of the profile brings %s", path)
	}
	compatData := creation.SteamCompatData(game.Path, game.SteamAppID)
	if compatData == "" {
		return errors.New("the game's Proton prefix was not found; Steam makes it the first time it starts the game")
	}
	data, err := deploy.DataDir(game.Path, modding.Target)
	if err != nil {
		return err
	}
	command, err := launch.SteamProtonCommand(s.steamRoots, compatData, game.SteamAppID, filepath.Join(data, filepath.FromSlash(path)))
	if err != nil {
		return err
	}
	// The wrapper deploys the profile, runs the tool and cleans up after it,
	// as a process of its own: the tool may outlive Hermit's window.
	cmd := platform.Command(executablePath(), append([]string{"run", "--game", game.ID, "--tool", "--"}, command...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", filepath.Base(path), err)
	}
	return cmd.Process.Release()
}

func (s *ModListService) deployedGame(gameID string) (library.Game, library.Modding, error) {
	game, err := s.lib.GetGame(gameID)
	if err != nil {
		return library.Game{}, library.Modding{}, err
	}
	modding, ok := game.Modding()
	if !ok {
		return library.Game{}, library.Modding{}, fmt.Errorf("Hermit does not manage the mods of %s as a list of its own", game.Name)
	}
	return game, modding, nil
}

func (s *ModListService) notRunning(gameID string) error { return notRunning(s.lib, gameID) }

// notRunning refuses changes to mod folders while the game has them linked in.
func notRunning(lib *library.Library, gameID string) error {
	dataDir, err := lib.GameDataDir(gameID)
	if err != nil {
		return err
	}
	if session, err := launch.ReadSession(dataDir); err == nil && session.Running() {
		return errors.New("the game is running; change its mods once it has exited")
	}
	return nil
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
