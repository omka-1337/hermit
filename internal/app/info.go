// Package app holds application-level metadata and services exposed to the frontend.
package app

import (
	"context"
	"runtime"
	"strings"

	"hermit/internal/github"
	"hermit/internal/modinstall"
	"hermit/internal/platform"
)

const (
	Name = "Hermit"
	// ID is used for on-disk directories (e.g. ~/.local/share/<ID>).
	ID = "hermit"
	// The repository Hermit itself is released from.
	RepoOwner = "omka-1337"
	RepoName  = "hermit"
	RepoURL   = "https://github.com/" + RepoOwner + "/" + RepoName
)

// Version is overridden at build time via -ldflags "-X hermit/internal/app.Version=...".
var Version = "0.2.0"

type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	// SteamDeck reports whether Hermit runs on a Steam Deck, which decides
	// the interface layout when the UI mode is set to auto.
	SteamDeck  bool   `json:"steamDeck"`
	Repository string `json:"repository"`
}

// Update is the answer to "is there a newer Hermit?". Hermit does not install
// itself: the user downloads the release.
type Update struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	// URL is the release page, empty when nothing was found.
	URL string `json:"url"`
}

// releaseSource is the part of the GitHub client InfoService needs.
type releaseSource interface {
	LatestRelease(ctx context.Context, owner, repo string) (github.Release, error)
}

type InfoService struct {
	releases releaseSource
}

func NewInfoService(releases releaseSource) *InfoService {
	return &InfoService{releases: releases}
}

func (s *InfoService) GetInfo() AppInfo {
	return AppInfo{
		Name:       Name,
		Version:    Version,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		SteamDeck:  platform.IsSteamDeck(),
		Repository: RepoURL,
	}
}

// CheckUpdate asks GitHub for the newest release of Hermit itself. A build
// without a version (a development build) is never out of date.
func (s *InfoService) CheckUpdate(ctx context.Context) (Update, error) {
	update := Update{Current: Version}
	release, err := s.releases.LatestRelease(ctx, RepoOwner, RepoName)
	if err != nil {
		return Update{}, err
	}
	update.Latest = strings.TrimPrefix(release.Tag, "v")
	update.URL = RepoURL + "/releases/tag/" + release.Tag
	update.Available = modinstall.CompareVersions(update.Latest, Version) > 0
	return update, nil
}
