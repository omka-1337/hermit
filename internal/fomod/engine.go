package fomod

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Environment tells the installer the state of a file a condition names,
// a plugin mostly: active, there but not active, or missing.
type Environment func(file string) FileState

// Choices are the options picked: for each step of Config.Steps, for each of
// its groups, the indices of the plugins picked. A group without an entry
// gets its defaults: what the installer requires and recommends.
type Choices [][][]int

// View is the installer with some options picked: the steps that show, what
// is picked in them once rules and defaults are applied, and what that
// installs.
type View struct {
	Steps   []StepView        `json:"steps"`
	Choices Choices           `json:"choices"`
	Flags   map[string]string `json:"flags"`
	Files   []File            `json:"-"`
	// Ready is false while a group still needs an option picked.
	Ready bool `json:"ready"`
}

type StepView struct {
	// Index is the step's place in Config.Steps, what Choices are keyed by.
	Index  int         `json:"index"`
	Name   string      `json:"name"`
	Groups []GroupView `json:"groups"`
}

type GroupView struct {
	Name    string       `json:"name"`
	Type    GroupType    `json:"type"`
	Plugins []PluginView `json:"plugins"`
	// Problem says what the group still needs, "" when it is fine.
	Problem string `json:"problem"`
}

type PluginView struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Image       string     `json:"image"`
	Type        PluginType `json:"type"`
	Selected    bool       `json:"selected"`
}

// Evaluate goes through the installer step by step: a step shows when the
// flags set so far allow it, each option takes the type its conditions give
// it, and the options picked set flags for the steps after.
func (c *Config) Evaluate(choices Choices, env Environment) View {
	v := View{Flags: map[string]string{}, Choices: make(Choices, len(c.Steps)), Ready: true}
	files := append([]File(nil), c.Required...)
	for s, step := range c.Steps {
		if step.Visible != nil && !step.Visible.holds(v.Flags, env) {
			continue
		}
		sv := StepView{Index: s, Name: step.Name}
		v.Choices[s] = make([][]int, len(step.Groups))
		for g, group := range step.Groups {
			types := make([]PluginType, len(group.Plugins))
			for i, p := range group.Plugins {
				types[i] = p.Type.typeFor(v.Flags, env)
			}
			var given []int
			if s < len(choices) && g < len(choices[s]) {
				given = choices[s][g]
			}
			picked := pick(group.Type, types, given)
			v.Choices[s][g] = picked
			gv := GroupView{Name: group.Name, Type: group.Type, Problem: problem(group.Type, picked)}
			if gv.Problem != "" {
				v.Ready = false
			}
			for i, p := range group.Plugins {
				selected := contains(picked, i)
				gv.Plugins = append(gv.Plugins, PluginView{
					Name: p.Name, Description: p.Description, Image: p.Image, Type: types[i], Selected: selected,
				})
				if selected {
					for _, f := range p.Flags {
						v.Flags[f.Name] = f.Value
					}
					files = append(files, p.Files...)
				}
			}
			sv.Groups = append(sv.Groups, gv)
		}
		v.Steps = append(v.Steps, sv)
	}
	for _, cond := range c.Conditional {
		if cond.Dependencies.holds(v.Flags, env) {
			files = append(files, cond.Files...)
		}
	}
	// Higher priority installs later and wins; equal ones keep their order.
	sort.SliceStable(files, func(i, j int) bool { return files[i].Priority < files[j].Priority })
	v.Files = files
	return v
}

// pick applies a group's rules to the options given, or picks its
// defaults when none were given yet.
func pick(t GroupType, types []PluginType, given []int) []int {
	usable := func(i int) bool { return i >= 0 && i < len(types) && types[i] != NotUsable }
	var picked []int
	switch {
	case t == SelectAll:
		for i := range types {
			if usable(i) {
				picked = append(picked, i)
			}
		}
		return picked
	case given == nil:
		for i, typ := range types {
			if typ == Required || typ == Recommended {
				picked = append(picked, i)
			}
		}
		if len(picked) == 0 && (t == SelectExactlyOne || t == SelectAtLeastOne) {
			for i := range types {
				if usable(i) {
					picked = append(picked, i)
					break
				}
			}
		}
	default:
		for _, i := range given {
			if usable(i) && !contains(picked, i) {
				picked = append(picked, i)
			}
		}
		for i, typ := range types {
			if typ == Required && !contains(picked, i) {
				picked = append(picked, i)
			}
		}
	}
	if (t == SelectExactlyOne || t == SelectAtMostOne) && len(picked) > 1 {
		picked = picked[:1]
	}
	sort.Ints(picked)
	if picked == nil {
		picked = []int{}
	}
	return picked
}

func problem(t GroupType, picked []int) string {
	if len(picked) == 0 && (t == SelectExactlyOne || t == SelectAtLeastOne) {
		return "Pick an option"
	}
	return ""
}

func (t TypeDescriptor) typeFor(flags map[string]string, env Environment) PluginType {
	for _, p := range t.Patterns {
		if p.Dependencies.holds(flags, env) {
			return p.Type
		}
	}
	if t.Default == "" {
		return Optional
	}
	return t.Default
}

func (d Dependencies) holds(flags map[string]string, env Environment) bool {
	var results []bool
	for _, f := range d.Flags {
		results = append(results, flags[f.Name] == f.Value)
	}
	for _, f := range d.Files {
		state := Missing
		if env != nil {
			state = env(f.File)
		}
		results = append(results, state == f.State)
	}
	for _, inner := range d.Inner {
		results = append(results, inner.holds(flags, env))
	}
	if len(results) == 0 {
		return true
	}
	for _, r := range results {
		if r == d.Or {
			return d.Or
		}
	}
	return !d.Or
}

func contains(list []int, i int) bool {
	for _, x := range list {
		if x == i {
			return true
		}
	}
	return false
}

// Install puts the files of a view into dest, the mod's folder, laid out as
// in Data. Sources are found in root, the folder holding the installer, in
// any letter case: installers are written on Windows, where case does not
// matter. Files are hard-linked where root and dest share a file system.
func Install(root string, files []File, dest string) error {
	for _, f := range files {
		src, err := resolve(root, f.Source)
		if err != nil {
			return err
		}
		to := cleanDest(f.Destination)
		if f.Folder {
			if err := copyTree(src, dest, to); err != nil {
				return err
			}
			continue
		}
		if to == "" || strings.HasSuffix(strings.ReplaceAll(f.Destination, `\`, "/"), "/") {
			to = path.Join(to, path.Base(strings.ReplaceAll(f.Source, `\`, "/")))
		}
		if err := place(src, dest, to); err != nil {
			return err
		}
	}
	return nil
}

// cleanDest makes a destination a path inside the mod's folder.
func cleanDest(dest string) string {
	dest = path.Clean("/" + strings.ReplaceAll(dest, `\`, "/"))
	return strings.TrimPrefix(dest, "/")
}

// resolve finds a path of the installer in root whatever its letter case.
func resolve(root, rel string) (string, error) {
	dir := root
	for _, part := range strings.Split(strings.Trim(strings.ReplaceAll(rel, `\`, "/"), "/"), "/") {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", fmt.Errorf("%q leaves the installer's folder", rel)
		}
		found, err := findFold(dir, part)
		if err != nil {
			return "", err
		}
		if found == "" {
			return "", fmt.Errorf("the installer names %q, which the archive does not have", rel)
		}
		dir = found
	}
	return dir, nil
}

// copyTree puts the files of the folder src at to in dest.
func copyTree(src, dest, to string) error {
	return filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		return place(p, dest, path.Join(to, filepath.ToSlash(rel)))
	})
}

// place puts a file at to in dest, replacing what an earlier file put there.
// Folders and files already there are used in whatever letter case they
// have: options of one installer spell the same folder differently.
func place(src, dest, to string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	target := dest
	parts := strings.Split(to, "/")
	for i, part := range parts {
		if found, _ := findFold(target, part); found != "" {
			target = found
		} else {
			target = filepath.Join(target, part)
		}
		if i < len(parts)-1 {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		}
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Link(src, target); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
