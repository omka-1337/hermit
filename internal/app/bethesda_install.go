package app

import (
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"hermit/internal/bethesda"
	"hermit/internal/fomod"
	"hermit/internal/library"
	"hermit/internal/plugininfo"
)

const (
	// UnpackProgressEvent carries how far unpacking an archive got.
	UnpackProgressEvent = "bethesda:unpack"
	// ModInstalledEvent says a mod was installed into a profile, so that the
	// main window shows it whichever window installed it.
	ModInstalledEvent = "bethesda:installed"
)

// UnpackProgress is the payload of UnpackProgressEvent.
type UnpackProgress struct {
	File  string `json:"file"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
}

// PreparedMod is an archive unpacked for installing.
type PreparedMod struct {
	Session string `json:"session"`
	Name    string `json:"name"`
	Version string `json:"version"`
	Author  string `json:"author"`
	// Fomod is set when the archive has an installer with options to pick.
	Fomod bool   `json:"fomod"`
	Image string `json:"image"`
	// Unsure is set when nothing in a plain archive looked like Data.
	Unsure bool `json:"unsure"`
	// Existing is the name of the mod of the profile the archive has the
	// name of, if one does: ChooseTarget says what to do about it.
	Existing string `json:"existing"`
}

// What an archive does to a mod of the profile with its name, as MO2 asks.
const (
	// TargetReplace installs it in place of the mod: what the mod had goes.
	TargetReplace = "replace"
	// TargetMerge installs it over the mod: its files replace the mod's,
	// the mod's other files stay.
	TargetMerge = "merge"
	// TargetRename installs it as a mod of its own under another name.
	TargetRename = "rename"
)

type installSession struct {
	gameID, profileID string
	staged            *bethesda.Staged
	env               fomod.Environment
	// existing is the id of the mod with the archive's name, target what to
	// do about it, and name the name the mod is installed under.
	existing string
	target   string
	name     string
}

func emit(name string, data any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, data)
	}
}

// PrepareInstall unpacks a mod archive for a profile and looks at it. What
// it unpacked stays until the mod is installed or the install is cancelled.
func (s *BethesdaService) PrepareInstall(gameID, profileID, file string) (PreparedMod, error) {
	game, _, err := s.creationGame(gameID)
	if err != nil {
		return PreparedMod{}, err
	}
	profile, err := s.lib.GetProfile(gameID, profileID)
	if err != nil {
		return PreparedMod{}, err
	}
	profileDir, err := s.lib.ProfileDir(gameID, profileID)
	if err != nil {
		return PreparedMod{}, err
	}
	name := filepath.Base(file)
	staging := filepath.Join(profileDir, bethesda.StagingDir)
	s.removeStale(staging)
	staged, err := bethesda.Stage(file, staging, func(done, total int64) {
		progress := UnpackProgress{File: name, Done: done, Total: total}
		emit(UnpackProgressEvent, progress)
		if s.OnUnpack != nil {
			s.OnUnpack(progress)
		}
	})
	if err != nil {
		return PreparedMod{}, fmt.Errorf("unpacking %s: %w", name, err)
	}
	session := filepath.Base(staged.Dir)
	s.mu.Lock()
	if s.sessions == nil {
		s.sessions = map[string]*installSession{}
	}
	is := &installSession{
		gameID: gameID, profileID: profileID, staged: staged, name: staged.Name,
		env: pluginStates(game.Path, profileDir, profile),
	}
	if m, ok := modNamed(profile, staged.Name); ok {
		is.existing = m.ID
	}
	s.sessions[session] = is
	s.mu.Unlock()
	p := PreparedMod{
		Session: session, Name: staged.Name, Version: staged.Version, Author: staged.Author,
		Fomod: staged.Fomod != nil, Unsure: staged.Unsure,
	}
	if is.existing != "" {
		p.Existing = staged.Name
	}
	if staged.Fomod != nil {
		p.Image = staged.Fomod.Image
	}
	return p, nil
}

// ChooseTarget says what installing a prepared archive does to the mod with
// its name: TargetReplace, TargetMerge, or TargetRename with name the name
// to install it under, which no mod of the profile may have.
func (s *BethesdaService) ChooseTarget(session, target, name string) error {
	is, err := s.session(session)
	if err != nil {
		return err
	}
	switch target {
	case TargetReplace, TargetMerge:
		if is.existing == "" {
			return errors.New("no mod of the profile has this name")
		}
		is.target, is.name = target, is.staged.Name
	case TargetRename:
		name = strings.TrimSpace(name)
		if name == "" {
			return library.ErrEmptyName
		}
		profile, err := s.lib.GetProfile(is.gameID, is.profileID)
		if err != nil {
			return err
		}
		if m, ok := modNamed(profile, name); ok {
			return fmt.Errorf("the profile has a mod named %s already", m.Name)
		}
		is.target, is.name = target, name
	default:
		return fmt.Errorf("%q is not replace, merge or rename", target)
	}
	return nil
}

// modNamed finds the mod of a profile with a name, in any letter case.
func modNamed(p library.Profile, name string) (library.Mod, bool) {
	i := slices.IndexFunc(p.Mods, func(m library.Mod) bool { return !m.Separator && strings.EqualFold(m.Name, name) })
	if i < 0 {
		return library.Mod{}, false
	}
	return p.Mods[i], true
}

// OpenFomod shows the installer of a prepared archive in a window of its own.
func (s *BethesdaService) OpenFomod(session string) error {
	is, err := s.session(session)
	if err != nil {
		return err
	}
	if is.staged.Fomod == nil {
		return errors.New("this archive has no installer")
	}
	if s.OpenWindow == nil {
		return errors.New("no window can be opened from here")
	}
	s.OpenWindow("/#fomod/"+session, "Install "+is.staged.Name)
	return nil
}

// PreparedInfo describes a prepared archive to the installer's window.
func (s *BethesdaService) PreparedInfo(session string) (PreparedMod, error) {
	is, err := s.session(session)
	if err != nil {
		return PreparedMod{}, err
	}
	p := PreparedMod{Session: session, Name: is.staged.Name, Version: is.staged.Version, Author: is.staged.Author, Fomod: is.staged.Fomod != nil}
	if is.staged.Fomod != nil {
		p.Image = is.staged.Fomod.Image
	}
	return p, nil
}

// FomodView is the installer of a prepared archive with the options picked
// so far.
func (s *BethesdaService) FomodView(session string, choices fomod.Choices) (fomod.View, error) {
	is, err := s.session(session)
	if err != nil {
		return fomod.View{}, err
	}
	if is.staged.Fomod == nil {
		return fomod.View{}, errors.New("this archive has no installer")
	}
	return is.staged.Fomod.Evaluate(choices, is.env), nil
}

// FomodImage returns a picture of the installer as a data URL.
func (s *BethesdaService) FomodImage(session, path string) (string, error) {
	is, err := s.session(session)
	if err != nil {
		return "", err
	}
	file, err := resolveFold(is.staged.FomodRoot, path)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(file)
	if err != nil {
		return "", err
	}
	if fi.Size() > 16<<20 {
		return "", errors.New("the picture is too large")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	kind := mime.TypeByExtension(strings.ToLower(filepath.Ext(file)))
	if kind == "" {
		kind = "image/png"
	}
	return "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// FinishInstall installs a prepared archive as a new mod at the end of the
// profile's order, where it wins over the mods before it; a FOMOD with the
// options picked in choices. An archive with the name of a mod of the
// profile replaces or merges with that mod, keeping its place and whether it
// is on, or becomes a mod of its own, as ChooseTarget said.
func (s *BethesdaService) FinishInstall(session string, choices fomod.Choices) (library.Profile, error) {
	is, err := s.session(session)
	if err != nil {
		return library.Profile{}, err
	}
	if is.existing != "" && is.target == "" {
		return library.Profile{}, fmt.Errorf("a mod named %s is installed already; replace it, merge with it or install under another name", is.staged.Name)
	}
	if is.target == TargetReplace || is.target == TargetMerge {
		// The mod's files are linked into the game while it runs.
		if err := s.notRunning(is.gameID); err != nil {
			return library.Profile{}, err
		}
	}
	profileDir, err := s.lib.ProfileDir(is.gameID, is.profileID)
	if err != nil {
		return library.Profile{}, err
	}
	var files []fomod.File
	notes := ""
	if cfg := is.staged.Fomod; cfg != nil {
		view := cfg.Evaluate(choices, is.env)
		if !view.Ready {
			return library.Profile{}, errors.New("some groups of the installer still need an option picked")
		}
		if len(view.Files) == 0 {
			return library.Profile{}, errors.New("the options picked install no files")
		}
		files = view.Files
		notes = fomodNotes(view)
	}

	source := library.ModSource{Type: library.SourceLocal, URL: is.staged.File}
	if is.staged.NexusID > 0 {
		if _, bg, err := s.creationGame(is.gameID); err == nil {
			source = library.ModSource{Type: library.SourceNexus, URL: bg.NexusURL(is.staged.NexusID), Asset: is.staged.File}
		}
	}
	p, err := s.lib.UpdateProfile(is.gameID, is.profileID, func(p *library.Profile) error {
		if is.target == TargetReplace || is.target == TargetMerge {
			i := slices.IndexFunc(p.Mods, func(m library.Mod) bool { return m.ID == is.existing })
			if i < 0 {
				return fmt.Errorf("mod %q: %w", is.existing, library.ErrNotFound)
			}
			modDir := filepath.Join(profileDir, bethesda.ModsDir, is.existing)
			if is.target == TargetReplace {
				if err := os.RemoveAll(modDir); err != nil {
					return err
				}
			}
			if err := is.staged.Install(modDir, files); err != nil {
				return err
			}
			m := &p.Mods[i]
			m.Version, m.Source = is.staged.Version, source
			if is.staged.Author != "" {
				m.Author = is.staged.Author
			}
			if notes != "" || is.target == TargetReplace {
				m.Notes = notes
			}
			return nil
		}
		taken := make([]string, 0, len(p.Mods))
		for _, m := range p.Mods {
			taken = append(taken, m.ID)
		}
		// Folders left by mods removed outside Hermit are taken too.
		entries, _ := os.ReadDir(filepath.Join(profileDir, bethesda.ModsDir))
		for _, e := range entries {
			taken = append(taken, e.Name())
		}
		id := uniqueModID(is.name, taken)
		modDir := filepath.Join(profileDir, bethesda.ModsDir, id)
		if err := is.staged.Install(modDir, files); err != nil {
			os.RemoveAll(modDir)
			return err
		}
		p.Mods = append(p.Mods, library.Mod{
			ID:                id,
			Name:              is.name,
			Author:            is.staged.Author,
			Version:           is.staged.Version,
			Enabled:           true,
			Active:            true,
			UnmetDependencies: []string{},
			Source:            source,
			Dependencies:      []string{},
			Files:             []string{},
			Plugins:           []plugininfo.Plugin{},
			Notes:             notes,
		})
		return nil
	})
	if err != nil {
		return library.Profile{}, err
	}
	s.CancelInstall(session)
	emit(ModInstalledEvent, map[string]string{"gameId": is.gameID, "profileId": is.profileID})
	return p, nil
}

// CancelInstall throws away what PrepareInstall unpacked.
func (s *BethesdaService) CancelInstall(session string) error {
	s.mu.Lock()
	is := s.sessions[session]
	delete(s.sessions, session)
	s.mu.Unlock()
	if is == nil {
		return nil
	}
	return is.staged.Remove()
}

// removeStale deletes what installs that never finished, cut short by a
// closed window or Hermit quitting, left in a profile's staging folder.
func (s *BethesdaService) removeStale(staging string) {
	entries, _ := os.ReadDir(staging)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range entries {
		if s.sessions[e.Name()] == nil {
			os.RemoveAll(filepath.Join(staging, e.Name()))
		}
	}
}

func (s *BethesdaService) session(id string) (*installSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	is := s.sessions[id]
	if is == nil {
		return nil, errors.New("this install was finished or cancelled")
	}
	return is, nil
}

// fomodNotes records the options picked, the way MO2 keeps them in a mod's
// notes, so the mod can be set up the same way again.
func fomodNotes(v fomod.View) string {
	var picked []string
	for _, step := range v.Steps {
		for _, group := range step.Groups {
			for _, p := range group.Plugins {
				if p.Selected && p.Type != fomod.Required {
					picked = append(picked, p.Name)
				}
			}
		}
	}
	if len(picked) == 0 {
		return ""
	}
	return "FOMOD: " + strings.Join(picked, ", ")
}

// pluginStates tells an installer which plugins the game has: the game's own
// and those of the profile's enabled mods are there, and active unless the
// profile's plugin list has them switched off. A plugin the list does not
// have yet is switched on when the game next starts.
func pluginStates(gamePath, profileDir string, profile library.Profile) fomod.Environment {
	official := map[string]bool{}
	if data, err := gameData(gamePath); err == nil {
		entries, _ := os.ReadDir(data)
		for _, e := range entries {
			official[strings.ToLower(e.Name())] = true
		}
	}
	fromMods := map[string]bool{}
	for _, m := range profile.Mods {
		if !m.Enabled || m.Separator {
			continue
		}
		entries, _ := os.ReadDir(filepath.Join(profileDir, bethesda.ModsDir, m.ID))
		for _, e := range entries {
			fromMods[strings.ToLower(e.Name())] = true
		}
	}
	list, _ := bethesda.ReadPlugins(filepath.Join(profileDir, bethesda.PluginsFile))
	return func(file string) fomod.FileState {
		name := strings.ToLower(strings.ReplaceAll(file, `\`, "/"))
		switch {
		case official[name]:
			return fomod.Active
		case !fromMods[name]:
			return fomod.Missing
		}
		i := slices.IndexFunc(list, func(p bethesda.Plugin) bool { return strings.EqualFold(p.Name, name) })
		if i >= 0 && !list[i].Active {
			return fomod.Inactive
		}
		return fomod.Active
	}
}

// resolveFold finds a path given by an installer under root in any letter
// case; paths leaving root are refused.
func resolveFold(root, rel string) (string, error) {
	dir := root
	for _, part := range strings.Split(strings.Trim(strings.ReplaceAll(rel, `\`, "/"), "/"), "/") {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", fmt.Errorf("%q leaves the archive", rel)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return "", err
		}
		i := slices.IndexFunc(entries, func(e os.DirEntry) bool { return strings.EqualFold(e.Name(), part) })
		if i < 0 {
			return "", fmt.Errorf("the archive has no %q", rel)
		}
		dir = filepath.Join(dir, entries[i].Name())
	}
	return dir, nil
}
