// Package library manages games and their profiles on disk.
//
// Layout under the library root:
//
//	cache/<mod-id>/<version>.zip
//	games/<game-id>/game.json
//	games/<game-id>/profiles/<profile-id>/profile.json
//	games/<game-id>/profiles/<profile-id>/BepInEx/...
//
// A profile can also be kept in a folder of its own outside the root, listed
// in its game's game.json; nothing else about it changes.
//
// Directory names are stable ids; display names live in the JSON files, so
// renaming never moves anything on disk.
package library

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"

	"hermit/internal/bethesda"
	"hermit/internal/steam"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrLastProfile   = errors.New("cannot remove the last profile of a game")
	ErrEmptyName     = errors.New("name must not be empty")
	ErrInvalidPath   = errors.New("game path must be an existing directory")
	ErrAlreadyExists = errors.New("already added")
	ErrGameRunning   = errors.New("the game is running or its last session was not finished")
)

const DefaultProfileName = "Default"

// SessionFile is written to a game's data directory by package launch for as
// long as a profile is linked into the game.
const SessionFile = "launch.json"

type Library struct {
	root       string
	steamRoots []string
	mu         sync.Mutex
}

// New opens the library at root. steamRoots are Steam installations used for
// game discovery (see steam.DefaultRoots).
func New(root string, steamRoots []string) (*Library, error) {
	for _, dir := range []string{root, filepath.Join(root, "games"), filepath.Join(root, "cache")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create library dir: %w", err)
		}
	}
	return &Library{root: root, steamRoots: steamRoots}, nil
}

func (l *Library) gamesDir() string         { return filepath.Join(l.root, "games") }
func (l *Library) gameDir(id string) string { return filepath.Join(l.gamesDir(), id) }
func (l *Library) profilesDir(gameID string) string {
	return filepath.Join(l.gameDir(gameID), "profiles")
}
func (l *Library) profileDir(gameID, profileID string) string {
	if g, err := l.loadGame(gameID); err == nil {
		if dir, ok := g.ProfileDirs[profileID]; ok {
			return dir
		}
	}
	return filepath.Join(l.profilesDir(gameID), profileID)
}

// profileIDs lists the ids of a game's profiles, in the library and outside it.
func (l *Library) profileIDs(gameID string) ([]string, error) {
	ids, err := listDirs(l.profilesDir(gameID))
	if err != nil {
		return nil, err
	}
	g, err := l.loadGame(gameID)
	if err != nil {
		return ids, nil
	}
	for id := range g.ProfileDirs {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// ListGames returns all games sorted by name.
func (l *Library) ListGames() ([]Game, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	ids, err := listDirs(l.gamesDir())
	if err != nil {
		return nil, err
	}
	games := make([]Game, 0, len(ids))
	for _, id := range ids {
		g, err := l.loadGame(id)
		if errors.Is(err, ErrNotFound) {
			continue // stray directory without game.json
		}
		if err != nil {
			return nil, err
		}
		// The engine is read again each time the games are listed: an
		// update can move a game to a newer Unity, and games added before
		// engines were recognised have none recorded. A game whose folder is
		// gone keeps what it had.
		if d := DetectGame(g.Path); d.Engine != EngineUnknown && (d.Engine != g.Engine || d.EngineVersion != g.EngineVersion) {
			g.Engine, g.EngineVersion = d.Engine, d.EngineVersion
			if err := l.saveGame(g); err != nil {
				return nil, err
			}
		}
		games = append(games, g)
	}
	sort.Slice(games, func(i, j int) bool { return games[i].Name < games[j].Name })
	return games, nil
}

// GameDataDir returns the manager's data directory of an existing game.
func (l *Library) GameDataDir(id string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := l.loadGame(id); err != nil {
		return "", err
	}
	return l.gameDir(id), nil
}

// FindGameBySteamAppID returns the added game with the given Steam app id.
func (l *Library) FindGameBySteamAppID(appID string) (Game, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	ids, err := listDirs(l.gamesDir())
	if err != nil {
		return Game{}, err
	}
	for _, id := range ids {
		if g, err := l.loadGame(id); err == nil && appID != "" && g.SteamAppID == appID {
			return g, nil
		}
	}
	return Game{}, fmt.Errorf("steam app %s: %w", appID, ErrNotFound)
}

// Root is the library root directory.
func (l *Library) Root() string { return l.root }

func (l *Library) GetGame(id string) (Game, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loadGame(id)
}

// DiscoverGames lists the games installed via Steam that Hermit can mod:
// Unity games, through BepInEx, and Creation Engine games.
func (l *Library) DiscoverGames() ([]GameCandidate, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var result []GameCandidate
	for _, app := range steam.InstalledApps(l.steamRoots) {
		d := DetectGame(app.InstallPath)
		if d.Engine == EngineUnknown {
			continue
		}
		c := GameCandidate{Path: app.InstallPath, Name: app.Name, SteamAppID: app.AppID, Detection: d}
		_, err := l.findGameByPath(app.InstallPath)
		c.AlreadyAdded = err == nil
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// InspectGamePath describes a manually picked folder without adding it.
func (l *Library) InspectGamePath(path string) (GameCandidate, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.inspect(path)
}

func (l *Library) inspect(path string) (GameCandidate, error) {
	path, err := canonicalDir(path)
	if err != nil {
		return GameCandidate{}, err
	}
	c := GameCandidate{Path: path, Detection: DetectGame(path)}
	if app, ok := steam.FindAppByPath(l.steamRoots, path); ok {
		c.Name = app.Name
		c.SteamAppID = app.AppID
	} else if exe := c.Detection.Executable; exe != "" {
		c.Name = strings.TrimSuffix(exe, filepath.Ext(exe))
	} else {
		c.Name = filepath.Base(path)
	}
	_, err = l.findGameByPath(path)
	c.AlreadyAdded = err == nil
	return c, nil
}

// AddGame registers a game installed at path and creates its default profile.
// The game directory itself is only inspected, never modified.
func (l *Library) AddGame(name, path string) (Game, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if name == "" {
		return Game{}, ErrEmptyName
	}
	c, err := l.inspect(path)
	if err != nil {
		return Game{}, err
	}
	if c.AlreadyAdded {
		return Game{}, fmt.Errorf("game at %s: %w", c.Path, ErrAlreadyExists)
	}
	ids, err := listDirs(l.gamesDir())
	if err != nil {
		return Game{}, err
	}

	g := Game{
		ID:            uniqueID(slugify(name, "game"), ids),
		Name:          name,
		Path:          c.Path,
		Runtime:       c.Detection.Runtime,
		Backend:       c.Detection.Backend,
		Engine:        c.Detection.Engine,
		EngineVersion: c.Detection.EngineVersion,
		Executable:    c.Detection.Executable,
		SteamAppID:    c.SteamAppID,
	}
	p, err := l.createProfile(g.ID, g.Engine, DefaultProfileName)
	if err != nil {
		_ = os.RemoveAll(l.gameDir(g.ID))
		return Game{}, err
	}
	g.ActiveProfile = p.ID
	if err := l.saveGame(g); err != nil {
		_ = os.RemoveAll(l.gameDir(g.ID))
		return Game{}, err
	}
	return g, nil
}

func (l *Library) RenameGame(id, name string) (Game, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if name == "" {
		return Game{}, ErrEmptyName
	}
	g, err := l.loadGame(id)
	if err != nil {
		return Game{}, err
	}
	g.Name = name
	return g, l.saveGame(g)
}

// SetLaunchExecutable chooses the file a game is started from; "" goes back to
// the game's own executable, or for a Creation Engine game to its script
// extender when it has one. The path is relative to the game folder and must
// name a file inside it, or one a mod of the active profile puts there.
func (l *Library) SetLaunchExecutable(id, exe string) (Game, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	g, err := l.loadGame(id)
	if err != nil {
		return Game{}, err
	}
	// Picking a Creation Engine game's own executable is a choice of its
	// own: starting it without the script extender.
	if exe == g.Executable && g.Engine != EngineCreation {
		exe = ""
	}
	if exe != "" {
		clean := filepath.Clean(filepath.FromSlash(exe))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return Game{}, fmt.Errorf("%q is not inside the game folder", exe)
		}
		fi, err := os.Stat(filepath.Join(g.Path, clean))
		if (err != nil || fi.IsDir()) && !slices.Contains(l.profileRootFiles(g), filepath.ToSlash(clean)) {
			return Game{}, fmt.Errorf("%q was not found in the game folder", exe)
		}
		exe = filepath.ToSlash(clean)
	}
	g.LaunchExecutable = exe
	return g, l.saveGame(g)
}

// ProfileRootFiles lists the files the enabled mods of a Creation Engine
// game's active profile put into the game folder while it runs, such as the
// script extender's loader, as paths relative to the game folder.
func (l *Library) ProfileRootFiles(gameID string) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	g, err := l.loadGame(gameID)
	if err != nil {
		return nil, err
	}
	return l.profileRootFiles(g), nil
}

func (l *Library) profileRootFiles(g Game) []string {
	if g.Engine != EngineCreation {
		return nil
	}
	p, err := l.loadProfile(g.ID, g.ActiveProfile)
	if err != nil {
		return nil
	}
	dir := l.profileDir(g.ID, g.ActiveProfile)
	var files []string
	for _, m := range p.Mods {
		if !m.Enabled || m.Separator {
			continue
		}
		modDir := filepath.Join(dir, bethesda.ModsDir, m.ID)
		entries, _ := os.ReadDir(modDir)
		for _, e := range entries {
			if !e.IsDir() || !strings.EqualFold(e.Name(), bethesda.RootDir) {
				continue
			}
			root := filepath.Join(modDir, e.Name())
			filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					rel, _ := filepath.Rel(root, p)
					if rel = filepath.ToSlash(rel); !slices.Contains(files, rel) {
						files = append(files, rel)
					}
				}
				return nil
			})
		}
	}
	return files
}

// RemoveGame deletes the game entry together with all its profiles and installed mods.
func (l *Library) RemoveGame(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	g, err := l.loadGame(id)
	if err != nil {
		return err
	}
	for _, dir := range g.ProfileDirs {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return os.RemoveAll(l.gameDir(id))
}

func (l *Library) SetActiveProfile(gameID, profileID string) (Game, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	g, err := l.loadGame(gameID)
	if err != nil {
		return Game{}, err
	}
	if _, err := l.loadProfile(gameID, profileID); err != nil {
		return Game{}, err
	}
	g.ActiveProfile = profileID
	return g, l.saveGame(g)
}

// ListProfiles returns the profiles of a game sorted by name.
func (l *Library) ListProfiles(gameID string) ([]Profile, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := l.loadGame(gameID); err != nil {
		return nil, err
	}
	return l.listProfiles(gameID)
}

func (l *Library) GetProfile(gameID, profileID string) (Profile, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loadProfile(gameID, profileID)
}

// ProfileDir returns the directory of an existing profile.
func (l *Library) ProfileDir(gameID, profileID string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := l.loadProfile(gameID, profileID); err != nil {
		return "", err
	}
	return l.profileDir(gameID, profileID), nil
}

// UpdateProfile loads a profile, applies fn and saves the result atomically
// with respect to other library operations.
func (l *Library) UpdateProfile(gameID, profileID string, fn func(*Profile) error) (Profile, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	p, err := l.loadProfile(gameID, profileID)
	if err != nil {
		return Profile{}, err
	}
	if err := fn(&p); err != nil {
		return Profile{}, err
	}
	p.ID = profileID
	return p, l.saveProfile(gameID, p)
}

func (l *Library) CreateProfile(gameID, name string) (Profile, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if name == "" {
		return Profile{}, ErrEmptyName
	}
	g, err := l.loadGame(gameID)
	if err != nil {
		return Profile{}, err
	}
	return l.createProfile(gameID, g.Engine, name)
}

func (l *Library) RenameProfile(gameID, profileID, name string) (Profile, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if name == "" {
		return Profile{}, ErrEmptyName
	}
	p, err := l.loadProfile(gameID, profileID)
	if err != nil {
		return Profile{}, err
	}
	p.Name = name
	return p, l.saveProfile(gameID, p)
}

// CopyProfile duplicates a profile under a new name: the same mods, the same
// configs, its own folder. What it is not is the modpack link — a copy is a
// profile assembled by hand from that moment on, free to be changed without
// the copy and the original drifting apart silently.
func (l *Library) CopyProfile(gameID, profileID, name string) (Profile, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if name == "" {
		return Profile{}, ErrEmptyName
	}
	g, err := l.loadGame(gameID)
	if err != nil {
		return Profile{}, err
	}
	source, err := l.loadProfile(gameID, profileID)
	if err != nil {
		return Profile{}, err
	}
	ids, err := l.profileIDs(gameID)
	if err != nil {
		return Profile{}, err
	}
	p := source
	p.ID = uniqueID(slugify(name, "profile"), ids)
	p.Name = name
	// The copy goes where the original is: a profile was moved out of the
	// library because its mods are large, and a copy of it is just as large.
	dir := filepath.Join(l.profilesDir(gameID), p.ID)
	if sourceDir, ok := g.ProfileDirs[profileID]; ok {
		if dir, err = freeFolder(filepath.Dir(sourceDir), gameID+"-"+p.ID); err != nil {
			return Profile{}, err
		}
		if g.ProfileDirs == nil {
			g.ProfileDirs = map[string]string{}
		}
		g.ProfileDirs[p.ID] = dir
	}
	if err := os.CopyFS(dir, os.DirFS(l.profileDir(gameID, profileID))); err != nil {
		_ = os.RemoveAll(dir)
		return Profile{}, err
	}
	if g.ProfileDirs[p.ID] != "" {
		if err := l.saveGame(g); err != nil {
			_ = os.RemoveAll(dir)
			return Profile{}, err
		}
	}
	if err := l.saveProfile(gameID, p); err != nil {
		_ = os.RemoveAll(dir)
		return Profile{}, err
	}
	return p, nil
}

// RemoveProfile deletes a profile with all its mods. The last profile of a game
// cannot be removed; if the active profile is removed, another one becomes active.
func (l *Library) RemoveProfile(gameID, profileID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	g, err := l.loadGame(gameID)
	if err != nil {
		return err
	}
	if _, err := l.loadProfile(gameID, profileID); err != nil {
		return err
	}
	profiles, err := l.listProfiles(gameID)
	if err != nil {
		return err
	}
	if len(profiles) <= 1 {
		return ErrLastProfile
	}
	if err := os.RemoveAll(l.profileDir(gameID, profileID)); err != nil {
		return err
	}
	_, outside := g.ProfileDirs[profileID]
	delete(g.ProfileDirs, profileID)
	if g.ActiveProfile == profileID {
		for _, p := range profiles {
			if p.ID != profileID {
				g.ActiveProfile = p.ID
				break
			}
		}
		return l.saveGame(g)
	}
	if outside {
		return l.saveGame(g)
	}
	return nil
}

// MoveProfile moves a profile with all its mods into a folder of its own
// inside location, named after the game and the profile, and returns that
// folder. An empty location moves it back into the library. Moving to
// another disk copies everything, which takes a while for large profiles.
func (l *Library) MoveProfile(gameID, profileID, location string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	g, err := l.loadGame(gameID)
	if err != nil {
		return "", err
	}
	if _, err := l.loadProfile(gameID, profileID); err != nil {
		return "", err
	}
	// Links in the game folder point into the profile while it runs.
	if _, err := os.Stat(filepath.Join(l.gameDir(gameID), SessionFile)); err == nil {
		return "", ErrGameRunning
	}
	current := l.profileDir(gameID, profileID)

	target := filepath.Join(l.profilesDir(gameID), profileID)
	if location != "" {
		if location, err = canonicalDir(location); err != nil {
			return "", err
		}
		switch {
		case within(location, g.Path):
			return "", errors.New("profiles cannot be kept in the game folder: Hermit does not change anything there")
		case within(location, current):
			return "", errors.New("a profile cannot be moved into its own folder")
		case within(location, l.root):
			return "", errors.New("this folder is inside Hermit's own data; choose a folder outside it")
		case filepath.Dir(current) == location:
			return current, nil
		}
		if dir := l.profileDirContaining(location); dir != "" {
			return "", fmt.Errorf("this folder is inside another profile (%s); choose a folder outside it", dir)
		}
		if target, err = freeFolder(location, gameID+"-"+profileID); err != nil {
			return "", err
		}
	}
	if target == current {
		return current, nil
	}
	if err := moveDir(current, target); err != nil {
		return "", err
	}

	if location == "" {
		delete(g.ProfileDirs, profileID)
	} else {
		if g.ProfileDirs == nil {
			g.ProfileDirs = map[string]string{}
		}
		g.ProfileDirs[profileID] = target
	}
	if err := l.saveGame(g); err != nil {
		// Put it back where game.json still says it is.
		if moveErr := moveDir(target, current); moveErr != nil {
			return "", errors.Join(err, moveErr)
		}
		return "", err
	}
	return target, nil
}

// profileDirContaining returns the folder of a profile kept outside the
// library that path is in, of any game, or "".
func (l *Library) profileDirContaining(path string) string {
	ids, _ := listDirs(l.gamesDir())
	for _, id := range ids {
		g, err := l.loadGame(id)
		if err != nil {
			continue
		}
		for _, dir := range g.ProfileDirs {
			if within(path, dir) {
				return dir
			}
		}
	}
	return ""
}

// freeFolder returns a path for a new folder in dir named base, or base-2 and
// so on when that name is taken.
func freeFolder(dir, base string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	taken := make([]string, len(entries))
	for i, e := range entries {
		taken[i] = e.Name()
	}
	return filepath.Join(dir, uniqueID(base, taken)), nil
}

// moveDir renames a folder, or copies it and removes the original when the
// two places are on different file systems.
func moveDir(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	err := os.Rename(from, to)
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	if err := os.CopyFS(to, os.DirFS(from)); err != nil {
		_ = os.RemoveAll(to)
		return err
	}
	return os.RemoveAll(from)
}

// within reports whether path is dir or inside it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// createProfile makes an empty profile laid out for the game's engine: the
// BepInEx folders for Unity games, a folder for mods for Creation Engine ones.
func (l *Library) createProfile(gameID string, engine Engine, name string) (Profile, error) {
	ids, err := l.profileIDs(gameID)
	if err != nil {
		return Profile{}, err
	}
	p := Profile{
		SchemaVersion: ProfileSchemaVersion,
		ID:            uniqueID(slugify(name, "profile"), ids),
		Name:          name,
		Mods:          []Mod{},
	}
	dir := l.profileDir(gameID, p.ID)
	subs := []string{"BepInEx/plugins", "BepInEx/patchers", "BepInEx/config"}
	if engine == EngineCreation {
		subs = []string{bethesda.ModsDir}
	}
	for _, sub := range subs {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(sub)), 0o755); err != nil {
			return Profile{}, err
		}
	}
	if err := l.saveProfile(gameID, p); err != nil {
		_ = os.RemoveAll(dir)
		return Profile{}, err
	}
	return p, nil
}

func (l *Library) listProfiles(gameID string) ([]Profile, error) {
	ids, err := l.profileIDs(gameID)
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(ids))
	for _, id := range ids {
		p, err := l.loadProfile(gameID, id)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, nil
}

func (l *Library) findGameByPath(path string) (Game, error) {
	ids, err := listDirs(l.gamesDir())
	if err != nil {
		return Game{}, err
	}
	for _, id := range ids {
		if g, err := l.loadGame(id); err == nil && g.Path == path {
			return g, nil
		}
	}
	return Game{}, ErrNotFound
}

func (l *Library) loadGame(id string) (Game, error) {
	if !validID(id) {
		return Game{}, fmt.Errorf("game %q: %w", id, ErrNotFound)
	}
	var g Game
	if err := readJSON(filepath.Join(l.gameDir(id), "game.json"), &g); err != nil {
		return Game{}, fmt.Errorf("game %q: %w", id, err)
	}
	g.ID = id
	return g, nil
}

func (l *Library) saveGame(g Game) error {
	return writeJSON(filepath.Join(l.gameDir(g.ID), "game.json"), g)
}

func (l *Library) loadProfile(gameID, id string) (Profile, error) {
	if !validID(gameID) || !validID(id) {
		return Profile{}, fmt.Errorf("profile %q: %w", id, ErrNotFound)
	}
	var p Profile
	if err := readJSON(filepath.Join(l.profileDir(gameID, id), "profile.json"), &p); err != nil {
		return Profile{}, fmt.Errorf("profile %q: %w", id, err)
	}
	p.ID = id
	if p.Mods == nil {
		p.Mods = []Mod{}
	}
	if p.SchemaVersion < 2 {
		for i := range p.Mods {
			p.Mods[i].Active = p.Mods[i].Enabled
		}
	}
	p.SchemaVersion = ProfileSchemaVersion
	return p, nil
}

func (l *Library) saveProfile(gameID string, p Profile) error {
	return writeJSON(filepath.Join(l.profileDir(gameID, p.ID), "profile.json"), p)
}
