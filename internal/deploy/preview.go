package deploy

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// OverwriteSource is the source of a file in the profile's overwrite folder.
const OverwriteSource = "\x00overwrite"

// Preview is the game folder as a profile's mods make it while it runs,
// without deploying anything: which file is where, and which mod it comes
// from.
type Preview struct {
	root *previewNode
	// Files is how many files the game folder will have.
	Files int
}

type previewNode struct {
	name     string
	dir      bool
	children map[string]*previewNode
	files    int
	// source is the mod whose file wins, "" for the game's own;
	// others are the mods whose file by that name loses, in the order of
	// the mods, "" for the game's own file when a mod's takes its place.
	source string
	others []string
	// fromMod is set once a mod's file is there.
	fromMod bool
}

// PreviewEntry is a file or folder of a Preview.
type PreviewEntry struct {
	Name string `json:"name"`
	// Path is slash-separated from the game folder.
	Path string `json:"path"`
	Dir  bool   `json:"dir"`
	// Files is how many files a folder holds, in its folders as well.
	Files int `json:"files"`
	// Source is the id of the mod a file comes from, "" for the game's own
	// and OverwriteSource for the profile's overwrite folder.
	Source string `json:"source"`
	// Others are the mods that have a file by that name too and lose it,
	// "" standing for the game's own file a mod's takes the place of.
	Others []string `json:"others"`
}

// NewPreview works out the game folder at gamePath as it will be with the
// files of mods, the ids of the profile's enabled mods from lowest to
// highest priority, and of the profile's overwrite folder deployed, as
// Deploy does it: a later mod's file wins over an earlier one's, and the
// game's own file over both unless the target lets mods replace it. Names
// match in any letter case, spelled as the game, or else the first mod,
// spells them.
func NewPreview(gamePath, profileDir string, mods []string, t Target) (*Preview, error) {
	p := &Preview{root: &previewNode{dir: true, children: map[string]*previewNode{}}}
	place := func(rel string) *previewNode {
		n := p.root
		parts := strings.Split(rel, "/")
		for i, part := range parts {
			key := strings.ToLower(part)
			child := n.children[key]
			if child == nil {
				child = &previewNode{name: part, dir: i < len(parts)-1}
				if child.dir {
					child.children = map[string]*previewNode{}
				}
				n.children[key] = child
			}
			n = child
		}
		return n
	}

	// The game's own files; links into a profile are a session's, and files
	// set aside are put back after it.
	err := filepath.WalkDir(gamePath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(gamePath, path)
		if rel == "." || d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel = strings.TrimSuffix(filepath.ToSlash(rel), OriginalSuffix)
		place(rel).source = ""
		return nil
	})
	if err != nil {
		return nil, err
	}
	game := map[*previewNode]bool{}
	var walk func(*previewNode)
	walk = func(n *previewNode) {
		for _, c := range n.children {
			if c.dir {
				walk(c)
			} else {
				game[c] = true
			}
		}
	}
	walk(p.root)

	dataName := ""
	if t.Data != "" {
		if dataName, err = FindFold(gamePath, t.Data); err != nil {
			return nil, err
		}
		if dataName == "" {
			dataName = t.Data
		}
	}
	folders := Folders(profileDir, mods)
	for i, folder := range folders {
		from := OverwriteSource
		if i < len(mods) {
			from = mods[i]
		}
		err := walkMod(folder, t, func(_ string, src source) {
			rel := src.rel
			if t.Data != "" && !src.root {
				rel = dataName + "/" + rel
			}
			n := place(rel)
			switch {
			case game[n] && !t.Replace:
				n.others = append(n.others, from) // hidden by the game's file
			case n.fromMod:
				n.others = append(n.others, n.source)
				n.source = from
			default:
				if game[n] {
					n.others = append(n.others, "")
				}
				n.source, n.fromMod = from, true
			}
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	var count func(*previewNode) int
	count = func(n *previewNode) int {
		if !n.dir {
			return 1
		}
		n.files = 0
		for _, c := range n.children {
			n.files += count(c)
		}
		return n.files
	}
	p.Files = count(p.root)
	return p, nil
}

// Folder lists a folder of the preview, slash-separated from the game
// folder, "" for the game folder itself: folders first, then files, by name.
func (p *Preview) Folder(path string) ([]PreviewEntry, error) {
	n := p.root
	if path != "" {
		for _, part := range strings.Split(path, "/") {
			n = n.children[strings.ToLower(part)]
			if n == nil || !n.dir {
				return nil, fs.ErrNotExist
			}
		}
	}
	out := make([]PreviewEntry, 0, len(n.children))
	for _, c := range n.children {
		e := PreviewEntry{Name: c.name, Dir: c.dir, Files: c.files, Source: c.source, Others: c.others}
		if path == "" {
			e.Path = c.name
		} else {
			e.Path = path + "/" + c.name
		}
		if e.Others == nil {
			e.Others = []string{}
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}
