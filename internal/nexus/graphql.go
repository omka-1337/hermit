package nexus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// The browsing part of the API, version 2, answers GraphQL queries and
// needs no key: searching, describing and listing the files of mods.

// SearchOptions say which mods of a game Search finds.
type SearchOptions struct {
	// Game is the game's part of nexusmods.com, as in its addresses.
	Game string `json:"game"`
	// Query matches words of the mods' names; empty finds every mod.
	Query string `json:"query"`
	// Sort is one of the Sort constants.
	Sort   string `json:"sort"`
	Offset int    `json:"offset"`
	Count  int    `json:"count"`
	// Adult includes mods with adult content.
	Adult bool `json:"adult"`
}

// The orders Search lists mods in, the most of each first.
const (
	SortRelevance    = "relevance"
	SortEndorsements = "endorsements"
	SortDownloads    = "downloads"
	SortUpdated      = "updatedAt"
	SortNewest       = "createdAt"
)

// ModSummary is a mod as a list of mods shows it.
type ModSummary struct {
	ModID        int    `json:"modId"`
	Name         string `json:"name"`
	Summary      string `json:"summary"`
	Author       string `json:"author"`
	Version      string `json:"version"`
	Endorsements int    `json:"endorsements"`
	Downloads    int    `json:"downloads"`
	PictureURL   string `json:"pictureUrl"`
	ThumbnailURL string `json:"thumbnailUrl"`
	UpdatedAt    string `json:"updatedAt"`
	Adult        bool   `json:"adultContent"`
}

// SearchResult is a page of mods Search found, and how many there are.
type SearchResult struct {
	Total int          `json:"total"`
	Mods  []ModSummary `json:"mods"`
}

// ModPage is a mod with its description, in Nexus Mods' BBCode, and its
// files.
type ModPage struct {
	ModSummary
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Files       []ModFile `json:"files"`
}

// ModFile is a file of a mod a user can download.
type ModFile struct {
	FileID int    `json:"fileId"`
	Name   string `json:"name"`
	// Version is the file's own, which can differ from the mod's.
	Version string `json:"version"`
	// Category is MAIN, UPDATE, OPTIONAL, OLD_VERSION, MISCELLANEOUS or
	// ARCHIVED.
	Category string `json:"category"`
	// Size comes as a string: GraphQL's integers stop at 2 GB.
	Size        int64  `json:"sizeInBytes,string"`
	Date        int64  `json:"date"`
	Description string `json:"description"`
	// Primary is 1 for the file the mod's page offers first.
	Primary int `json:"primary"`
}

const summaryFields = "modId name summary author version endorsements downloads pictureUrl thumbnailUrl updatedAt adultContent"

// Search finds mods of a game.
func (c *Client) Search(ctx context.Context, o SearchOptions) (SearchResult, error) {
	filter := map[string]any{
		"gameDomainName": []any{map[string]any{"value": o.Game}},
		"status":         []any{map[string]any{"value": "published"}},
	}
	if q := strings.TrimSpace(o.Query); q != "" {
		// The words of the query, or the query run together as part of a
		// name, so that "sky ui" finds SkyUI; short ones would find too
		// much inside other words.
		words := []any{map[string]any{"nameStemmed": []any{map[string]any{"value": q, "op": "MATCHES"}}}}
		if compact := strings.ReplaceAll(q, " ", ""); len(compact) >= 4 {
			words = append(words, map[string]any{"name": []any{map[string]any{"value": compact, "op": "WILDCARD"}}})
		}
		filter["filter"] = []any{map[string]any{"op": "OR", "filter": words}}
	}
	if !o.Adult {
		filter["adultContent"] = []any{map[string]any{"value": false}}
	}
	sort := o.Sort
	switch sort {
	case SortRelevance, SortEndorsements, SortDownloads, SortUpdated, SortNewest:
	default:
		sort = SortEndorsements
	}
	if sort == SortRelevance && strings.TrimSpace(o.Query) == "" {
		sort = SortEndorsements // relevance to nothing orders nothing
	}
	count := o.Count
	if count <= 0 || count > 50 {
		count = 20
	}
	var out struct {
		Mods struct {
			TotalCount int          `json:"totalCount"`
			Nodes      []ModSummary `json:"nodes"`
		} `json:"mods"`
	}
	err := c.graphql(ctx, `query Search($filter: ModsFilter, $sort: [ModsSort!], $offset: Int, $count: Int) {
		mods(filter: $filter, sort: $sort, offset: $offset, count: $count) { totalCount nodes { `+summaryFields+` } }
	}`, map[string]any{
		"filter": filter,
		"sort":   []any{map[string]any{sort: map[string]any{"direction": "DESC"}}},
		"offset": max(0, o.Offset),
		"count":  count,
	}, &out)
	if err != nil {
		return SearchResult{}, err
	}
	if out.Mods.Nodes == nil {
		out.Mods.Nodes = []ModSummary{}
	}
	return SearchResult{Total: out.Mods.TotalCount, Mods: out.Mods.Nodes}, nil
}

// Page describes a mod of a game with its files, newest first.
func (c *Client) Page(ctx context.Context, game string, modID int) (ModPage, error) {
	gameID, err := c.gameID(ctx, game)
	if err != nil {
		return ModPage{}, err
	}
	var out struct {
		Mod struct {
			ModSummary
			Description string `json:"description"`
			ModCategory *struct {
				Name string `json:"name"`
			} `json:"modCategory"`
		} `json:"mod"`
		ModFiles []ModFile `json:"modFiles"`
	}
	err = c.graphql(ctx, `query Page($modId: ID!, $gameId: ID!) {
		mod(modId: $modId, gameId: $gameId) { `+summaryFields+` description modCategory { name } }
		modFiles(modId: $modId, gameId: $gameId) { fileId name version category sizeInBytes date description primary }
	}`, map[string]any{"modId": modID, "gameId": gameID}, &out)
	if err != nil {
		return ModPage{}, err
	}
	p := ModPage{ModSummary: out.Mod.ModSummary, Description: out.Mod.Description, Files: out.ModFiles}
	if out.Mod.ModCategory != nil {
		p.Category = out.Mod.ModCategory.Name
	}
	if p.Files == nil {
		p.Files = []ModFile{}
	}
	return p, nil
}

// gameIDs are the numbers Nexus Mods gives games, by their part of its
// addresses, found once.
var gameIDs sync.Map

func (c *Client) gameID(ctx context.Context, game string) (int, error) {
	if id, ok := gameIDs.Load(game); ok {
		return id.(int), nil
	}
	var out struct {
		Game *struct {
			ID int `json:"id"`
		} `json:"game"`
	}
	if err := c.graphql(ctx, `query Game($domain: String) { game(domainName: $domain) { id } }`, map[string]any{"domain": game}, &out); err != nil {
		return 0, err
	}
	if out.Game == nil {
		return 0, fmt.Errorf("Nexus Mods has no game %q", game)
	}
	gameIDs.Store(game, out.Game.ID)
	return out.Game.ID, nil
}

// graphql runs a query of the API's version 2 and reads its data into v.
func (c *Client) graphql(ctx context.Context, query string, variables map[string]any, v any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	base := c.BaseURL
	if base == "" {
		base = DefaultAPIURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v2/graphql", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
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
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	var out struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Nexus Mods answered %s", resp.Status)
		}
		return err
	}
	if len(out.Errors) > 0 {
		var msgs []string
		for _, e := range out.Errors {
			msgs = append(msgs, e.Message)
		}
		return errors.New("Nexus Mods: " + strings.Join(msgs, "; "))
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Nexus Mods answered %s", resp.Status)
	}
	return json.Unmarshal(out.Data, v)
}
