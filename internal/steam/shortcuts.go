package steam

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Steam keeps the games a user added by hand in
// userdata/<user>/config/shortcuts.vdf. It is binary VDF: a tree of named
// entries, each tagged with its type.
//
//	0x00 <key> 0x00   a map, ended by 0x08
//	0x01 <key> 0x00 <value> 0x00   a string
//	0x02 <key> 0x00 <int32 little endian>
//
// The file belongs to the user and usually holds shortcuts Hermit knows
// nothing about, so it is parsed into this generic tree and written back
// unchanged apart from the entry being added. Nothing here invents defaults
// for keys Steam wrote itself.
const (
	typeMap    byte = 0x00
	typeString byte = 0x01
	typeInt32  byte = 0x02
	typeEnd    byte = 0x08
)

type vdfEntry struct {
	kind byte
	key  string
	str  string
	num  int32
	sub  *vdfMap
}

type vdfMap struct {
	entries []vdfEntry
}

func (m *vdfMap) get(key string) *vdfEntry {
	for i := range m.entries {
		if strings.EqualFold(m.entries[i].key, key) {
			return &m.entries[i]
		}
	}
	return nil
}

func (m *vdfMap) setString(key, value string) {
	if e := m.get(key); e != nil {
		e.kind, e.str = typeString, value
		return
	}
	m.entries = append(m.entries, vdfEntry{kind: typeString, key: key, str: value})
}

func (m *vdfMap) setInt(key string, value int32) {
	if e := m.get(key); e != nil {
		e.kind, e.num = typeInt32, value
		return
	}
	m.entries = append(m.entries, vdfEntry{kind: typeInt32, key: key, num: value})
}

var errTruncated = errors.New("steam: shortcuts.vdf ends in the middle of an entry")

func parseBinaryVDF(data []byte) (*vdfMap, error) {
	root, rest, err := parseMap(data)
	if err != nil {
		return nil, err
	}
	// Some writers add further closing bytes; Steam's own files end here.
	for len(rest) > 0 && rest[0] == typeEnd {
		rest = rest[1:]
	}
	if len(rest) > 0 {
		return nil, fmt.Errorf("steam: %d bytes left after the last entry", len(rest))
	}
	return root, nil
}

func parseMap(data []byte) (*vdfMap, []byte, error) {
	m := &vdfMap{}
	for {
		if len(data) == 0 {
			return nil, nil, errTruncated
		}
		kind := data[0]
		data = data[1:]
		if kind == typeEnd {
			return m, data, nil
		}
		key, rest, err := readString(data)
		if err != nil {
			return nil, nil, err
		}
		data = rest
		switch kind {
		case typeMap:
			sub, rest, err := parseMap(data)
			if err != nil {
				return nil, nil, err
			}
			m.entries = append(m.entries, vdfEntry{kind: kind, key: key, sub: sub})
			data = rest
		case typeString:
			value, rest, err := readString(data)
			if err != nil {
				return nil, nil, err
			}
			m.entries = append(m.entries, vdfEntry{kind: kind, key: key, str: value})
			data = rest
		case typeInt32:
			if len(data) < 4 {
				return nil, nil, errTruncated
			}
			m.entries = append(m.entries, vdfEntry{kind: kind, key: key, num: int32(binary.LittleEndian.Uint32(data))})
			data = data[4:]
		default:
			return nil, nil, fmt.Errorf("steam: unknown entry type 0x%02x in shortcuts.vdf", kind)
		}
	}
}

func readString(data []byte) (string, []byte, error) {
	end := bytes.IndexByte(data, 0)
	if end < 0 {
		return "", nil, errTruncated
	}
	return string(data[:end]), data[end+1:], nil
}

func (m *vdfMap) encode(buf *bytes.Buffer) {
	for _, e := range m.entries {
		buf.WriteByte(e.kind)
		buf.WriteString(e.key)
		buf.WriteByte(0)
		switch e.kind {
		case typeMap:
			e.sub.encode(buf)
		case typeString:
			buf.WriteString(e.str)
			buf.WriteByte(0)
		case typeInt32:
			binary.Write(buf, binary.LittleEndian, e.num)
		}
	}
	buf.WriteByte(typeEnd)
}

// encodeShortcutsFile writes the document. The root map's own closing byte is
// the last byte of the file, so nothing is appended after it.
func encodeShortcutsFile(root *vdfMap) []byte {
	var buf bytes.Buffer
	root.encode(&buf)
	return buf.Bytes()
}

// Shortcut is a game added to Steam by hand.
type Shortcut struct {
	AppName       string
	Exe           string
	StartDir      string
	Icon          string
	LaunchOptions string
}

// ShortcutAppID is the id Steam uses for a shortcut's artwork, derived from
// the command and the name the same way the client derives it.
func ShortcutAppID(exe, appName string) uint32 {
	return crc32.ChecksumIEEE([]byte(exe+appName)) | 0x80000000
}

// ShortcutsFiles returns the shortcuts.vdf of every Steam user found in the
// given roots, whether or not the file exists yet.
func ShortcutsFiles(roots []string) []string {
	var files []string
	for _, root := range roots {
		dirs, _ := filepath.Glob(filepath.Join(root, "userdata", "*", "config"))
		for _, dir := range dirs {
			files = append(files, filepath.Join(dir, "shortcuts.vdf"))
		}
	}
	return files
}

// HasShortcut reports whether the file already lists a shortcut with this Exe.
func HasShortcut(path, exe string) (bool, error) {
	root, err := readShortcuts(path)
	if err != nil {
		return false, err
	}
	shortcuts := root.get("shortcuts")
	if shortcuts == nil || shortcuts.sub == nil {
		return false, nil
	}
	for _, entry := range shortcuts.sub.entries {
		if entry.sub == nil {
			continue
		}
		if e := entry.sub.get("Exe"); e != nil && unquote(e.str) == unquote(exe) {
			return true, nil
		}
	}
	return false, nil
}

// AddShortcut adds a shortcut to a user's shortcuts.vdf, keeping every entry
// that is already there, and returns false if an entry with the same Exe
// exists. The previous file is kept next to it as shortcuts.vdf.bak.
//
// Steam reads this file when it starts and writes it back when it exits, so
// it must not be running while this is called: see Running.
func AddShortcut(path string, s Shortcut) (added bool, err error) {
	root, err := readShortcuts(path)
	if err != nil {
		return false, err
	}
	shortcuts := root.get("shortcuts")
	if shortcuts == nil || shortcuts.sub == nil {
		root.entries = append(root.entries, vdfEntry{kind: typeMap, key: "shortcuts", sub: &vdfMap{}})
		shortcuts = root.get("shortcuts")
	}
	for _, entry := range shortcuts.sub.entries {
		if entry.sub == nil {
			continue
		}
		if e := entry.sub.get("Exe"); e != nil && unquote(e.str) == unquote(s.Exe) {
			return false, nil
		}
	}

	// Steam numbers the entries; keep going from the highest one.
	next := 0
	for _, entry := range shortcuts.sub.entries {
		if n, err := strconv.Atoi(entry.key); err == nil && n >= next {
			next = n + 1
		}
	}

	item := &vdfMap{}
	item.setInt("appid", int32(ShortcutAppID(s.Exe, s.AppName)))
	item.setString("AppName", s.AppName)
	item.setString("Exe", quote(s.Exe))
	item.setString("StartDir", quote(s.StartDir))
	item.setString("icon", s.Icon)
	item.setString("ShortcutPath", "")
	item.setString("LaunchOptions", s.LaunchOptions)
	item.setInt("IsHidden", 0)
	item.setInt("AllowDesktopConfig", 1)
	item.setInt("AllowOverlay", 1)
	item.setInt("OpenVR", 0)
	item.setInt("Devkit", 0)
	item.setString("DevkitGameID", "")
	item.setInt("DevkitOverrideAppID", 0)
	item.setInt("LastPlayTime", 0)
	item.entries = append(item.entries, vdfEntry{kind: typeMap, key: "tags", sub: &vdfMap{}})
	shortcuts.sub.entries = append(shortcuts.sub.entries, vdfEntry{
		kind: typeMap, key: strconv.Itoa(next), sub: item,
	})

	if err := backupAndWrite(path, encodeShortcutsFile(root)); err != nil {
		return false, err
	}
	return true, nil
}

func readShortcuts(path string) (*vdfMap, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &vdfMap{entries: []vdfEntry{{kind: typeMap, key: "shortcuts", sub: &vdfMap{}}}}, nil
	}
	if err != nil {
		return nil, err
	}
	return parseBinaryVDF(data)
}

// backupAndWrite keeps the previous file as .bak and replaces it atomically:
// a half-written shortcuts.vdf would cost the user every shortcut they have.
func backupAndWrite(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", old, 0o644); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Steam stores Exe and StartDir quoted.
func quote(value string) string {
	if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		return value
	}
	return `"` + value + `"`
}

func unquote(value string) string { return strings.Trim(value, `"`) }
