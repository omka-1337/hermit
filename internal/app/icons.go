package app

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"

	"hermit/internal/launch"
	"hermit/internal/steam"
)

// IconService provides game icons as data URLs, so the webview can show local
// Steam cache files.
type IconService struct {
	steamRoots []string
}

func NewIconService(steamRoots []string) *IconService {
	return &IconService{steamRoots: steamRoots}
}

var iconTypes = map[string]string{".ico": "image/x-icon", ".jpg": "image/jpeg", ".png": "image/png", ".svg": "image/svg+xml"}

// flatpakIconDirs are where Flatpak exports the icons of installed apps.
var flatpakIconDirs = []string{
	"/var/lib/flatpak/exports/share/icons/hicolor",
	"~/.local/share/flatpak/exports/share/icons/hicolor",
}

// GetPortProtonIcon returns PortProton's own icon as a data URL, or "" when it
// is not installed; the launch button shows it next to its name.
func (s *IconService) GetPortProtonIcon() string {
	home, _ := os.UserHomeDir()
	for _, dir := range flatpakIconDirs {
		if strings.HasPrefix(dir, "~/") {
			if home == "" {
				continue
			}
			dir = filepath.Join(home, dir[2:])
		}
		// Scalable first, then the largest bitmap there is.
		for _, size := range []string{"scalable", "512x512", "256x256", "128x128", "64x64"} {
			matches, _ := filepath.Glob(filepath.Join(dir, size, "apps", launch.PortProtonApp+".*"))
			for _, path := range matches {
				mime := iconTypes[strings.ToLower(filepath.Ext(path))]
				if mime == "" {
					continue
				}
				data, err := os.ReadFile(path)
				if err != nil || len(data) > 2<<20 {
					continue
				}
				return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
			}
		}
	}
	return ""
}

// GetSteamIcon returns the icon of a Steam app as a data URL, or "" if none is cached.
func (s *IconService) GetSteamIcon(appID string) string {
	path := steam.IconPath(s.steamRoots, appID)
	mime := iconTypes[strings.ToLower(filepath.Ext(path))]
	if path == "" || mime == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 2<<20 {
		return ""
	}
	if mime == "image/x-icon" {
		if png := largestPNGInICO(data); png != nil {
			data, mime = png, "image/png"
		}
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// largestPNGInICO returns the largest PNG-encoded image of an .ico file, or
// nil if it has none (older icons store bitmaps).
func largestPNGInICO(ico []byte) []byte {
	if len(ico) < 6 || binary.LittleEndian.Uint16(ico[2:4]) != 1 {
		return nil
	}
	count := int(binary.LittleEndian.Uint16(ico[4:6]))
	var best []byte
	bestSize := 0
	for i := 0; i < count; i++ {
		entry := 6 + 16*i
		if entry+16 > len(ico) {
			break
		}
		width := int(ico[entry])
		if width == 0 {
			width = 256
		}
		size := int(binary.LittleEndian.Uint32(ico[entry+8:]))
		offset := int(binary.LittleEndian.Uint32(ico[entry+12:]))
		if offset < 0 || size <= 0 || offset+size > len(ico) {
			continue
		}
		img := ico[offset : offset+size]
		if width > bestSize && bytes.HasPrefix(img, []byte("\x89PNG\r\n\x1a\n")) {
			best, bestSize = img, width
		}
	}
	return best
}
