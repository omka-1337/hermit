// Package library manages games and their profiles on disk.
//
// Layout under the library root:
//
//	cache/<mod-id>/<version>.zip
//	games/<game-id>/game.json
//	games/<game-id>/profiles/<profile-id>/profile.json
//	games/<game-id>/profiles/<profile-id>/BepInEx/...
//
// Directory names are stable ids; display names live in the JSON files, so
// renaming never moves anything on disk.
package library

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"hermit/internal/steam"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrLastProfile   = errors.New("cannot remove the last profile of a game")
	ErrEmptyName     = errors.New("name must not be empty")
	ErrInvalidPath   = errors.New("game path must be an existing directory")
	ErrAlreadyExists = errors.New("already added")
)

const DefaultProfileName = "Default"

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
	return filepath.Join(l.profilesDir(gameID), profileID)
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

// DiscoverGames lists BepInEx-compatible (Unity) games installed via Steam.
func (l *Library) DiscoverGames() ([]GameCandidate, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var result []GameCandidate
	for _, app := range steam.InstalledApps(l.steamRoots) {
		d := DetectGame(app.InstallPath)
		if !d.Unity {
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
		ID:         uniqueID(slugify(name, "game"), ids),
		Name:       name,
		Path:       c.Path,
		Runtime:    c.Detection.Runtime,
		Backend:    c.Detection.Backend,
		Executable: c.Detection.Executable,
		SteamAppID: c.SteamAppID,
	}
	p, err := l.createProfile(g.ID, DefaultProfileName)
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
// the game's own executable. The path is relative to the game folder and must
// name a file inside it.
func (l *Library) SetLaunchExecutable(id, exe string) (Game, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	g, err := l.loadGame(id)
	if err != nil {
		return Game{}, err
	}
	if exe == g.Executable {
		exe = ""
	}
	if exe != "" {
		clean := filepath.Clean(filepath.FromSlash(exe))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return Game{}, fmt.Errorf("%q is not inside the game folder", exe)
		}
		if fi, err := os.Stat(filepath.Join(g.Path, clean)); err != nil || fi.IsDir() {
			return Game{}, fmt.Errorf("%q was not found in the game folder", exe)
		}
		exe = filepath.ToSlash(clean)
	}
	g.LaunchExecutable = exe
	return g, l.saveGame(g)
}

// RemoveGame deletes the game entry together with all its profiles and installed mods.
func (l *Library) RemoveGame(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, err := l.loadGame(id); err != nil {
		return err
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
	if _, err := l.loadGame(gameID); err != nil {
		return Profile{}, err
	}
	return l.createProfile(gameID, name)
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
	if g.ActiveProfile == profileID {
		for _, p := range profiles {
			if p.ID != profileID {
				g.ActiveProfile = p.ID
				break
			}
		}
		return l.saveGame(g)
	}
	return nil
}

func (l *Library) createProfile(gameID, name string) (Profile, error) {
	ids, err := listDirs(l.profilesDir(gameID))
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
	for _, sub := range []string{"plugins", "patchers", "config"} {
		if err := os.MkdirAll(filepath.Join(dir, "BepInEx", sub), 0o755); err != nil {
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
	ids, err := listDirs(l.profilesDir(gameID))
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
