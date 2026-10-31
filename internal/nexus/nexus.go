// Package nexus talks to Nexus Mods: it reads the nxm:// links its "Mod
// Manager Download" buttons open and asks its API where the files of those
// links are.
package nexus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultAPIURL = "https://api.nexusmods.com"

var ErrNoKey = errors.New("no Nexus Mods API key is set; add yours in Settings (Nexus Mods → your account → API Keys)")

// Link is an nxm:// link: a file of a mod, and the key and expiry Nexus
// Mods gives with it, which let an account without Premium download it.
type Link struct {
	// Game is the game's part of nexusmods.com, as in its addresses.
	Game    string
	ModID   int
	FileID  int
	Key     string
	Expires string
}

// ParseLink reads an nxm:// link of a mod's file,
// nxm://<game>/mods/<mod>/files/<file>?key=…&expires=….
func ParseLink(link string) (Link, error) {
	u, err := url.Parse(link)
	if err != nil || !strings.EqualFold(u.Scheme, "nxm") {
		return Link{}, fmt.Errorf("%q is not an nxm:// link", link)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 4 || parts[0] != "mods" || parts[2] != "files" {
		if len(parts) > 0 && parts[0] == "collections" {
			return Link{}, errors.New("collections from Nexus Mods are not supported; download their mods one by one")
		}
		return Link{}, fmt.Errorf("%q is not a link to a mod's file", link)
	}
	l := Link{Game: strings.ToLower(u.Host), Key: u.Query().Get("key"), Expires: u.Query().Get("expires")}
	if l.ModID, err = strconv.Atoi(parts[1]); err != nil {
		return Link{}, fmt.Errorf("%q has no mod id", link)
	}
	if l.FileID, err = strconv.Atoi(parts[3]); err != nil {
		return Link{}, fmt.Errorf("%q has no file id", link)
	}
	return l, nil
}

// Client asks the Nexus Mods API, with an account's personal API key.
type Client struct {
	BaseURL   string
	Key       string
	UserAgent string
	HTTP      *http.Client
}

// Account is who an API key belongs to.
type Account struct {
	Name    string `json:"name"`
	Premium bool   `json:"premium"`
}

// File is a file of a mod as Nexus Mods describes it.
type File struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	FileName string `json:"file_name"`
	Size     int64  `json:"size_in_bytes"`
}

// Mod is a mod as Nexus Mods describes it.
type Mod struct {
	Name    string `json:"name"`
	Author  string `json:"author"`
	Version string `json:"version"`
}

// Validate checks the key and says whose it is.
func (c *Client) Validate(ctx context.Context) (Account, error) {
	var v struct {
		Name      string `json:"name"`
		IsPremium bool   `json:"is_premium"`
	}
	if err := c.get(ctx, "/v1/users/validate.json", &v); err != nil {
		return Account{}, err
	}
	return Account{Name: v.Name, Premium: v.IsPremium}, nil
}

// File describes the file a link is to.
func (c *Client) File(ctx context.Context, l Link) (File, error) {
	var f File
	err := c.get(ctx, fmt.Sprintf("/v1/games/%s/mods/%d/files/%d.json", l.Game, l.ModID, l.FileID), &f)
	return f, err
}

// Mod describes the mod a link's file belongs to.
func (c *Client) Mod(ctx context.Context, l Link) (Mod, error) {
	var m Mod
	err := c.get(ctx, fmt.Sprintf("/v1/games/%s/mods/%d.json", l.Game, l.ModID), &m)
	return m, err
}

// DownloadURL is where the file of a link can be downloaded from, the first
// of the servers Nexus Mods offers.
func (c *Client) DownloadURL(ctx context.Context, l Link) (string, error) {
	path := fmt.Sprintf("/v1/games/%s/mods/%d/files/%d/download_link.json", l.Game, l.ModID, l.FileID)
	if l.Key != "" {
		path += "?" + url.Values{"key": {l.Key}, "expires": {l.Expires}}.Encode()
	}
	var servers []struct {
		Name string `json:"name"`
		URI  string `json:"URI"`
	}
	if err := c.get(ctx, path, &servers); err != nil {
		return "", err
	}
	if len(servers) == 0 {
		return "", errors.New("Nexus Mods offered no server to download the file from")
	}
	return servers[0].URI, nil
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	if c.Key == "" {
		return ErrNoKey
	}
	base := c.BaseURL
	if base == "" {
		base = DefaultAPIURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", c.Key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Application-Name", "Hermit")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Message string `json:"message"`
		}
		json.Unmarshal(body, &e)
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			return errors.New("Nexus Mods did not accept the API key")
		case resp.StatusCode == http.StatusForbidden && e.Message != "":
			return fmt.Errorf("Nexus Mods refused: %s", e.Message)
		case e.Message != "":
			return fmt.Errorf("Nexus Mods: %s", e.Message)
		}
		return fmt.Errorf("Nexus Mods answered %s", resp.Status)
	}
	return json.Unmarshal(body, v)
}

// ModURL is the page of a mod on Nexus Mods, of the game whose part of
// nexusmods.com is game.
func ModURL(game string, id int) string {
	return fmt.Sprintf("https://www.nexusmods.com/%s/mods/%d", game, id)
}
