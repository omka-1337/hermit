package launch

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"hermit/internal/library"
)

type IssueKind string

const (
	IssueIncompatible        IssueKind = "incompatible"
	IssueMissingDependencies IssueKind = "missing_dependencies"
	IssueDependencyNotLoaded IssueKind = "dependency_not_loaded"
	IssueNewerVersionExists  IssueKind = "newer_version_exists"
	IssueProcessFilter       IssueKind = "process_filter"
	IssueLoadError           IssueKind = "load_error"
)

// Issue is a plugin BepInEx refused or failed to load.
type Issue struct {
	Kind IssueKind `json:"kind"`
	// Plugin is how BepInEx names the plugin: "<name> <version>".
	Plugin string `json:"plugin"`
	// Detail is the rest of the message, e.g. the incompatible plugin GUIDs.
	Detail string `json:"detail"`
	// ModID is the installed mod the plugin most likely belongs to, if known.
	ModID string `json:"modId"`
}

// Report summarises BepInEx's LogOutput.log of the last game session of a profile.
type Report struct {
	ProfileID  string    `json:"profileId"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	// BepInExStarted is false when the log was not written during the session,
	// i.e. BepInEx did not load at all.
	BepInExStarted bool     `json:"bepinexStarted"`
	BepInExVersion string   `json:"bepinexVersion"`
	Loaded         []string `json:"loaded"`
	Issues         []Issue  `json:"issues"`
	// Complete is set once BepInEx says it finished loading plugins. A log
	// that stops before that means the game hung or crashed while a plugin
	// was starting: the last one in Loaded.
	Complete bool `json:"complete"`
	// Errors counts error lines from any source, BepInEx or the mods and the
	// game logging through it. Thousands of them usually mean mods written
	// for another version of the game.
	Errors int `json:"errors"`
	// PluginMods is how many active mods of the profile carry BepInEx
	// plugins, and NotStarted lists those of them BepInEx never began to
	// load: the numbers to compare when a game starts to a black screen.
	PluginMods int      `json:"pluginMods"`
	NotStarted []string `json:"notStarted"`
}

var (
	// logLine is "[Level : Source] message". BepInEx can be set to put the
	// time in front ("[21:53:31.6107579] [Info : BepInEx] ..."), and modpacks
	// often ship such a config.
	logLine = regexp.MustCompile(`^(?:\[[\d:.]+\]\s+)?\[(\w+)\s*:\s*([^\]]*?)\s*\] (.*)$`)
	version = regexp.MustCompile(`^BepInEx (\S+) - `)
	// Messages of BepInEx 5 Chainloader.
	logPatterns = []struct {
		kind IssueKind
		re   *regexp.Regexp
	}{
		{IssueNewerVersionExists, regexp.MustCompile(`^Skipping \[(.+)\] because a newer version exists \((.*)\)$`)},
		{IssueProcessFilter, regexp.MustCompile(`^Skipping \[(.+)\] because of process filters \((.*)\)$`)},
		{IssueIncompatible, regexp.MustCompile(`^Could not load \[(.+)\] because it is incompatible with: (.*)$`)},
		{IssueMissingDependencies, regexp.MustCompile(`^Could not load \[(.+)\] because it has missing dependencies: (.*)$`)},
		{IssueDependencyNotLoaded, regexp.MustCompile(`^Skipping \[(.+)\] because it has a dependency that was not loaded()`)},
		{IssueLoadError, regexp.MustCompile(`^Error loading \[(.+)\] : (.*)$`)},
	}
	loadingLine = regexp.MustCompile(`^Loading \[(.+)\]$`)
	// completeLine ends BepInEx 5's plugin loading; BepInEx 6 logs the same.
	completeLine = regexp.MustCompile(`^Chainloader startup complete`)
)

// ParseLog extracts loaded plugins and load issues from a BepInEx log.
func ParseLog(r io.Reader) Report {
	rep := Report{Loaded: []string{}, Issues: []Issue{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		m := logLine.FindStringSubmatch(strings.TrimRight(sc.Text(), "\r"))
		if m == nil {
			continue
		}
		level, source, msg := m[1], m[2], m[3]
		if level == "Error" || level == "Fatal" {
			rep.Errors++
		}
		if source != "BepInEx" {
			continue
		}
		if completeLine.MatchString(msg) {
			rep.Complete = true
			continue
		}
		if v := version.FindStringSubmatch(msg); v != nil && rep.BepInExVersion == "" {
			rep.BepInExVersion = v[1]
			continue
		}
		if l := loadingLine.FindStringSubmatch(msg); l != nil {
			rep.Loaded = append(rep.Loaded, l[1])
			continue
		}
		for _, p := range logPatterns {
			if s := p.re.FindStringSubmatch(msg); s != nil {
				rep.Issues = append(rep.Issues, Issue{Kind: p.kind, Plugin: s[1], Detail: s[2]})
				break
			}
		}
	}
	return rep
}

// BuildReport reads the profile's BepInEx log after a session.
func BuildReport(profileDir string, session Session, mods []library.Mod) Report {
	logPath := filepath.Join(profileDir, "BepInEx", "LogOutput.log")
	rep := Report{Loaded: []string{}, Issues: []Issue{}}
	if fi, err := os.Stat(logPath); err == nil && !fi.ModTime().Before(session.StartedAt) {
		if f, err := os.Open(logPath); err == nil {
			rep = ParseLog(f)
			f.Close()
			rep.BepInExStarted = true
		}
	}
	rep.ProfileID = session.ProfileID
	rep.StartedAt = session.StartedAt
	rep.FinishedAt = time.Now()
	for i := range rep.Issues {
		rep.Issues[i].ModID = matchMod(mods, rep.Issues[i].Plugin)
	}
	rep.PluginMods, rep.NotStarted = notStarted(mods, rep.Loaded)
	return rep
}

// notStarted counts the active mods that have BepInEx plugins and returns
// those none of whose plugins BepInEx logged as loading. Mods whose plugins
// were never read are left out: there is nothing to compare them by.
func notStarted(mods []library.Mod, loaded []string) (int, []string) {
	seen := map[string]bool{}
	for _, entry := range loaded {
		name := entry
		if i := strings.LastIndexByte(entry, ' '); i > 0 {
			name = entry[:i]
		}
		seen[name] = true
	}
	count, missing := 0, []string{}
	for _, m := range mods {
		if !m.Active || len(m.Plugins) == 0 {
			continue
		}
		count++
		started := false
		for _, p := range m.Plugins {
			if seen[p.Name] {
				started = true
				break
			}
		}
		if !started {
			missing = append(missing, m.ID)
		}
	}
	return count, missing
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]`)

func normalize(s string) string { return nonAlnum.ReplaceAllString(strings.ToLower(s), "") }

// matchMod finds the mod a plugin belongs to. BepInEx logs name plugins as
// "<name> <version>" from [BepInPlugin], which is matched against the plugins
// read from each mod's assemblies; mods without that list fall back to
// comparing names with the mod and its DLL files.
func matchMod(mods []library.Mod, plugin string) string {
	name, version := plugin, ""
	if i := strings.LastIndexByte(plugin, ' '); i > 0 {
		name, version = plugin[:i], plugin[i+1:]
	}
	for _, m := range mods {
		for _, p := range m.Plugins {
			if p.Name == name && (version == "" || p.Version == version) {
				return m.ID
			}
		}
	}
	for _, m := range mods {
		for _, p := range m.Plugins {
			if p.Name == name {
				return m.ID
			}
		}
	}

	want := normalize(name)
	if want == "" {
		return ""
	}
	for _, m := range mods {
		if m.Plugins != nil && len(m.Plugins) > 0 {
			continue
		}
		if normalize(m.Name) == want {
			return m.ID
		}
		for _, f := range m.Files {
			if strings.EqualFold(path.Ext(f), ".dll") && normalize(strings.TrimSuffix(path.Base(f), path.Ext(f))) == want {
				return m.ID
			}
		}
	}
	return ""
}

func reportPath(gameDataDir, profileID string) string {
	return filepath.Join(gameDataDir, "reports", profileID+".json")
}

func SaveReport(gameDataDir string, rep Report) error {
	data, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	p := reportPath(gameDataDir, rep.ProfileID)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// ReadReport returns the last report of a profile, or nil if there is none.
func ReadReport(gameDataDir, profileID string) (*Report, error) {
	data, err := os.ReadFile(reportPath(gameDataDir, profileID))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rep Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, err
	}
	return &rep, nil
}
