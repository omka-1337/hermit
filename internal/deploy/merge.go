package deploy

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

// MergedDir is the folder of a profile the files Deploy merges are written
// to for the session, laid out as the game folder.
const MergedDir = ".merged"

// merges reports whether a file of a mod is one the target merges with the
// game's file by its name rather than putting in its place.
func (t Target) merges(rel string) bool {
	ext := strings.ToLower(filepath.Ext(rel))
	for _, m := range t.Merge {
		if strings.EqualFold(m, ext) {
			return true
		}
	}
	return false
}

// MergeText adds the files of mods, lowest priority first, to the game's
// file of their name, as the mods of Unreal Engine 3 games are meant to be
// installed: a mod's configuration or localisation holds only the lines it
// adds, and the engine reads a section that comes twice as one. A mod's file
// at least half as long as the game's is a whole file instead, and takes
// the place of what is there so far. The result is in the game's encoding:
// UTF-16 for localisation, as the game keeps it.
func MergeText(game []byte, mods [][]byte) []byte {
	encoding := detect(game)
	if len(game) == 0 && len(mods) > 0 {
		encoding = detect(mods[0])
	}
	text := decode(game)
	whole := len(text) / 2
	for _, m := range mods {
		part := decode(m)
		if whole > 0 && len(part) >= whole {
			text = part
			continue
		}
		text = joinText(text, part)
	}
	return encode(text, encoding)
}

// AppendText puts b after a, in a's encoding: two parts of what goes into
// one file, as two options of a mod's archive both bring.
func AppendText(a, b []byte) []byte {
	return encode(joinText(decode(a), decode(b)), detect(a))
}

func joinText(a, b string) string {
	if a == "" {
		return b
	}
	if !strings.HasSuffix(a, "\n") {
		a += "\r\n"
	}
	return a + b
}

type textEncoding int

const (
	utf8Text textEncoding = iota
	utf16LE
	utf16BE
)

func detect(b []byte) textEncoding {
	switch {
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return utf16LE
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return utf16BE
	}
	return utf8Text
}

func decode(b []byte) string {
	var order binary.ByteOrder
	switch detect(b) {
	case utf16LE:
		order = binary.LittleEndian
	case utf16BE:
		order = binary.BigEndian
	default:
		return string(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}))
	}
	b = b[2:]
	units := make([]uint16, len(b)/2)
	for i := range units {
		units[i] = order.Uint16(b[2*i:])
	}
	return string(utf16.Decode(units))
}

func encode(s string, e textEncoding) []byte {
	if e == utf8Text {
		return []byte(s)
	}
	units := utf16.Encode([]rune(s))
	out := make([]byte, 2+2*len(units))
	var order binary.ByteOrder = binary.LittleEndian
	out[0], out[1] = 0xFF, 0xFE
	if e == utf16BE {
		order = binary.BigEndian
		out[0], out[1] = 0xFE, 0xFF
	}
	for i, u := range units {
		order.PutUint16(out[2+2*i:], u)
	}
	return out
}

// writeMerged writes what the game reads at rel for the session: its own
// file, game, with the mods' files of that name merged into it.
func writeMerged(profileDir, rel string, game []byte, files []string) (string, error) {
	var mods [][]byte
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		mods = append(mods, data)
	}
	path := filepath.Join(profileDir, MergedDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, MergeText(game, mods), 0o644)
}
