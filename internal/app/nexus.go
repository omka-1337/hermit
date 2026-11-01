package app

import (
	"context"
	"fmt"
	"slices"

	"hermit/internal/library"
	"hermit/internal/nexus"
)

// NexusService browses the mods Nexus Mods has for a game: searching them,
// describing them and listing their files, which needs no account.
type NexusService struct {
	lib    *library.Library
	client *nexus.Client
}

func NewNexusService(lib *library.Library, agent string) *NexusService {
	return &NexusService{lib: lib, client: &nexus.Client{UserAgent: agent}}
}

// Search finds mods of a game on Nexus Mods; opts.Game is filled in from
// the game.
func (s *NexusService) Search(ctx context.Context, gameID string, opts nexus.SearchOptions) (nexus.SearchResult, error) {
	domain, err := s.domain(gameID)
	if err != nil {
		return nexus.SearchResult{}, err
	}
	opts.Game = domain
	return s.client.Search(ctx, opts)
}

// Page describes a mod of a game with the files that can be downloaded,
// main files first and the newest first among them; archived files are left
// out, as Nexus Mods no longer gives them.
func (s *NexusService) Page(ctx context.Context, gameID string, modID int) (nexus.ModPage, error) {
	domain, err := s.domain(gameID)
	if err != nil {
		return nexus.ModPage{}, err
	}
	p, err := s.client.Page(ctx, domain, modID)
	if err != nil {
		return nexus.ModPage{}, err
	}
	p.Files = slices.DeleteFunc(p.Files, func(f nexus.ModFile) bool { return f.Category == "ARCHIVED" })
	slices.SortStableFunc(p.Files, func(a, b nexus.ModFile) int {
		if a, b := fileRank(a.Category), fileRank(b.Category); a != b {
			return a - b
		}
		return int(b.Date - a.Date)
	})
	return p, nil
}

// FilesURL is the page of a mod's file on Nexus Mods whose "Mod Manager
// Download" hands the file to Hermit; with no file, the mod's files.
func (s *NexusService) FilesURL(gameID string, modID, fileID int) (string, error) {
	domain, err := s.domain(gameID)
	if err != nil {
		return "", err
	}
	if fileID == 0 {
		return fmt.Sprintf("https://www.nexusmods.com/%s/mods/%d?tab=files", domain, modID), nil
	}
	return fmt.Sprintf("https://www.nexusmods.com/%s/mods/%d?tab=files&file_id=%d&nmm=1", domain, modID, fileID), nil
}

// fileRank orders the categories of files as a mod's page lists them.
func fileRank(category string) int {
	switch category {
	case "MAIN":
		return 0
	case "UPDATE":
		return 1
	case "OPTIONAL":
		return 2
	case "MISCELLANEOUS":
		return 3
	case "OLD_VERSION":
		return 4
	}
	return 5
}

func (s *NexusService) domain(gameID string) (string, error) {
	game, err := s.lib.GetGame(gameID)
	if err != nil {
		return "", err
	}
	modding, ok := game.Modding()
	if !ok || modding.Nexus == "" {
		return "", fmt.Errorf("%s has no mods on Nexus Mods Hermit can install", game.Name)
	}
	return modding.Nexus, nil
}
