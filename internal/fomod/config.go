// Package fomod reads FOMOD installers, the fomod/ModuleConfig.xml many mods
// on Nexus Mods come with, and works out what to install from the options
// picked in them.
package fomod

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// Config is a FOMOD installer.
type Config struct {
	Name  string
	Image string
	// Info is what fomod/info.xml says about the mod, when there is one.
	Info     Info
	Required []File
	// Steps are in the order they are shown.
	Steps       []Step
	Conditional []ConditionalFiles
}

// Info is fomod/info.xml.
type Info struct {
	Name    string `xml:"Name" json:"name"`
	Author  string `xml:"Author" json:"author"`
	Version string `xml:"Version" json:"version"`
	Website string `xml:"Website" json:"website"`
}

type Step struct {
	Name    string
	Visible *Dependencies
	Groups  []Group
}

type GroupType string

const (
	SelectAny        GroupType = "SelectAny"
	SelectAll        GroupType = "SelectAll"
	SelectExactlyOne GroupType = "SelectExactlyOne"
	SelectAtMostOne  GroupType = "SelectAtMostOne"
	SelectAtLeastOne GroupType = "SelectAtLeastOne"
)

type Group struct {
	Name    string
	Type    GroupType
	Plugins []Plugin
}

// Plugin is an option of a group.
type Plugin struct {
	Name        string
	Description string
	Image       string
	Files       []File
	Flags       []Flag
	Type        TypeDescriptor
}

type PluginType string

const (
	Required      PluginType = "Required"
	Recommended   PluginType = "Recommended"
	Optional      PluginType = "Optional"
	NotUsable     PluginType = "NotUsable"
	CouldBeUsable PluginType = "CouldBeUsable"
)

// TypeDescriptor gives a plugin its type, fixed or depending on flags and
// files.
type TypeDescriptor struct {
	Default  PluginType
	Patterns []TypePattern
}

type TypePattern struct {
	Dependencies Dependencies
	Type         PluginType
}

type Flag struct{ Name, Value string }

// File is a file or folder to install, its paths as the installer gives
// them: Source from the folder holding fomod, Destination in Data.
type File struct {
	Source      string
	Destination string
	Folder      bool
	Priority    int
}

type ConditionalFiles struct {
	Dependencies Dependencies
	Files        []File
}

// Dependencies is a condition: on flags set by options picked earlier, and
// on files, plugins mostly, the game has.
type Dependencies struct {
	Or    bool
	Flags []Flag
	Files []FileDependency
	Inner []Dependencies
}

type FileState string

const (
	Active   FileState = "Active"
	Inactive FileState = "Inactive"
	Missing  FileState = "Missing"
)

type FileDependency struct {
	File  string
	State FileState
}

// Load reads the installer in dir, the folder holding the fomod folder.
func Load(dir string) (*Config, error) {
	fomodDir, err := findFold(dir, "fomod")
	if err != nil {
		return nil, err
	}
	if fomodDir == "" {
		return nil, errors.New("no fomod folder")
	}
	moduleConfig, err := findFold(fomodDir, "ModuleConfig.xml")
	if err != nil || moduleConfig == "" {
		return nil, errors.New("the fomod folder has no ModuleConfig.xml")
	}
	data, err := os.ReadFile(moduleConfig)
	if err != nil {
		return nil, err
	}
	cfg, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("ModuleConfig.xml: %w", err)
	}
	if info, _ := findFold(fomodDir, "info.xml"); info != "" {
		if data, err := os.ReadFile(info); err == nil {
			// A broken info.xml costs only the name it would give.
			decode(data, &cfg.Info)
		}
	}
	return cfg, nil
}

// Parse reads a ModuleConfig.xml.
func Parse(data []byte) (*Config, error) {
	var raw xmlConfig
	if err := decode(data, &raw); err != nil {
		return nil, err
	}
	cfg := &Config{Name: strings.TrimSpace(raw.ModuleName), Image: raw.ModuleImage.Path, Required: raw.Required.files()}
	steps := raw.InstallSteps.Steps
	sortByName(raw.InstallSteps.Order, len(steps), func(i int) string { return steps[i].Name }, func(i, j int) { steps[i], steps[j] = steps[j], steps[i] })
	for _, s := range steps {
		step := Step{Name: s.Name}
		if s.Visible != nil {
			deps := s.Visible.dependencies()
			step.Visible = &deps
		}
		groups := s.Groups.Groups
		sortByName(s.Groups.Order, len(groups), func(i int) string { return groups[i].Name }, func(i, j int) { groups[i], groups[j] = groups[j], groups[i] })
		for _, g := range groups {
			group := Group{Name: g.Name, Type: GroupType(g.Type)}
			plugins := g.Plugins.Plugins
			sortByName(g.Plugins.Order, len(plugins), func(i int) string { return plugins[i].Name }, func(i, j int) { plugins[i], plugins[j] = plugins[j], plugins[i] })
			for _, p := range plugins {
				plugin := Plugin{
					Name:        p.Name,
					Description: strings.TrimSpace(p.Description),
					Image:       p.Image.Path,
					Files:       p.Files.files(),
					Type:        p.TypeDescriptor.descriptor(),
				}
				for _, f := range p.ConditionFlags {
					plugin.Flags = append(plugin.Flags, Flag{Name: f.Name, Value: strings.TrimSpace(f.Value)})
				}
				group.Plugins = append(group.Plugins, plugin)
			}
			step.Groups = append(step.Groups, group)
		}
		cfg.Steps = append(cfg.Steps, step)
	}
	for _, p := range raw.Conditional {
		cfg.Conditional = append(cfg.Conditional, ConditionalFiles{Dependencies: p.Dependencies.dependencies(), Files: p.Files.files()})
	}
	return cfg, nil
}

// decode reads XML in whatever encoding FOMOD authors saved it in: UTF-8
// with or without a byte order mark, or UTF-16 with one, as the tools on
// Windows write it.
func decode(data []byte, v any) error {
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) || bytes.HasPrefix(data, []byte{0xFE, 0xFF}) ||
		(len(data) > 1 && data[0] == '<' && data[1] == 0) {
		utf16 := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
		converted, _, err := transform.Bytes(utf16.NewDecoder(), data)
		if err != nil {
			return err
		}
		data = converted
	}
	data = bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))
	d := xml.NewDecoder(bytes.NewReader(data))
	// The text is UTF-8 by now, whatever the declaration says.
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	d.Strict = false
	return d.Decode(v)
}

// sortByName puts items in the order an installer asks for: as written for
// Explicit, by name otherwise, Ascending being the default.
func sortByName(order string, n int, name func(int) string, swap func(i, j int)) {
	if order == "Explicit" {
		return
	}
	desc := order == "Descending"
	sort.Stable(byName{n, name, swap, desc})
}

type byName struct {
	n    int
	name func(int) string
	swap func(i, j int)
	desc bool
}

func (b byName) Len() int      { return b.n }
func (b byName) Swap(i, j int) { b.swap(i, j) }
func (b byName) Less(i, j int) bool {
	a, c := strings.ToLower(b.name(i)), strings.ToLower(b.name(j))
	if b.desc {
		return a > c
	}
	return a < c
}

// findFold finds the entry of dir called name in any letter case.
func findFold(dir, name string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", nil
}

// The XML as it is written.

type xmlConfig struct {
	ModuleName   string       `xml:"moduleName"`
	ModuleImage  xmlImage     `xml:"moduleImage"`
	Required     xmlFiles     `xml:"requiredInstallFiles"`
	InstallSteps xmlSteps     `xml:"installSteps"`
	Conditional  []xmlPattern `xml:"conditionalFileInstalls>patterns>pattern"`
}

type xmlImage struct {
	Path string `xml:"path,attr"`
}

type xmlSteps struct {
	Order string    `xml:"order,attr"`
	Steps []xmlStep `xml:"installStep"`
}

type xmlStep struct {
	Name    string           `xml:"name,attr"`
	Visible *xmlDependencies `xml:"visible"`
	Groups  struct {
		Order  string     `xml:"order,attr"`
		Groups []xmlGroup `xml:"group"`
	} `xml:"optionalFileGroups"`
}

type xmlGroup struct {
	Name    string `xml:"name,attr"`
	Type    string `xml:"type,attr"`
	Plugins struct {
		Order   string      `xml:"order,attr"`
		Plugins []xmlPlugin `xml:"plugin"`
	} `xml:"plugins"`
}

type xmlPlugin struct {
	Name           string   `xml:"name,attr"`
	Description    string   `xml:"description"`
	Image          xmlImage `xml:"image"`
	Files          xmlFiles `xml:"files"`
	ConditionFlags []struct {
		Name  string `xml:"name,attr"`
		Value string `xml:",chardata"`
	} `xml:"conditionFlags>flag"`
	TypeDescriptor xmlTypeDescriptor `xml:"typeDescriptor"`
}

type xmlTypeDescriptor struct {
	Type *struct {
		Name string `xml:"name,attr"`
	} `xml:"type"`
	DependencyType *struct {
		Default struct {
			Name string `xml:"name,attr"`
		} `xml:"defaultType"`
		Patterns []struct {
			Dependencies xmlDependencies `xml:"dependencies"`
			Type         struct {
				Name string `xml:"name,attr"`
			} `xml:"type"`
		} `xml:"patterns>pattern"`
	} `xml:"dependencyType"`
}

func (t xmlTypeDescriptor) descriptor() TypeDescriptor {
	switch {
	case t.Type != nil:
		return TypeDescriptor{Default: PluginType(t.Type.Name)}
	case t.DependencyType != nil:
		d := TypeDescriptor{Default: PluginType(t.DependencyType.Default.Name)}
		for _, p := range t.DependencyType.Patterns {
			d.Patterns = append(d.Patterns, TypePattern{Dependencies: p.Dependencies.dependencies(), Type: PluginType(p.Type.Name)})
		}
		return d
	}
	return TypeDescriptor{Default: Optional}
}

type xmlFiles struct {
	Items []xmlFile `xml:",any"`
}

type xmlFile struct {
	XMLName     xml.Name
	Source      string  `xml:"source,attr"`
	Destination *string `xml:"destination,attr"`
	Priority    string  `xml:"priority,attr"`
}

func (f xmlFiles) files() []File {
	var files []File
	for _, item := range f.Items {
		folder := item.XMLName.Local == "folder"
		if !folder && item.XMLName.Local != "file" {
			continue
		}
		file := File{Source: item.Source, Folder: folder}
		file.Priority, _ = strconv.Atoi(item.Priority)
		// Without a destination a file goes where it is in the installer;
		// an empty one means the top of Data.
		if item.Destination != nil {
			file.Destination = *item.Destination
		} else {
			file.Destination = item.Source
		}
		files = append(files, file)
	}
	return files
}

type xmlPattern struct {
	Dependencies xmlDependencies `xml:"dependencies"`
	Files        xmlFiles        `xml:"files"`
}

type xmlDependencies struct {
	Operator string `xml:"operator,attr"`
	Flags    []struct {
		Flag  string `xml:"flag,attr"`
		Value string `xml:"value,attr"`
	} `xml:"flagDependency"`
	Files []struct {
		File  string `xml:"file,attr"`
		State string `xml:"state,attr"`
	} `xml:"fileDependency"`
	Inner []xmlDependencies `xml:"dependencies"`
}

func (d xmlDependencies) dependencies() Dependencies {
	deps := Dependencies{Or: d.Operator == "Or"}
	for _, f := range d.Flags {
		deps.Flags = append(deps.Flags, Flag{Name: f.Flag, Value: f.Value})
	}
	for _, f := range d.Files {
		deps.Files = append(deps.Files, FileDependency{File: f.File, State: FileState(f.State)})
	}
	for _, inner := range d.Inner {
		deps.Inner = append(deps.Inner, inner.dependencies())
	}
	return deps
}
