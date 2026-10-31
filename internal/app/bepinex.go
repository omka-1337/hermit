package app

import (
	"context"
	"fmt"
	"strings"

	"hermit/internal/github"
	"hermit/internal/library"
	"hermit/internal/modinstall"
)

// BepInEx is a package like any other, and for games with a Thunderstore
// community it arrives as a dependency of the first mod. Games without one —
// anything not on Thunderstore, or modded from Nexus Mods — have to get it
// from where it is released: BepInEx's own GitHub releases. Which build a game
// needs follows from what Hermit already detected: the scripting backend picks
// the major version (5 for Mono, 6 for IL2CPP), the runtime picks the platform
// files (Windows ones for games run through Proton).
const (
	bepinexOwner = "BepInEx"
	bepinexRepo  = "BepInEx"
	// BepInExModID is how the loader installed from GitHub is named in a
	// profile, "<author>-<name>" like every other mod.
	BepInExModID = "BepInEx-BepInEx"
)

// BepInExBuild is the release file a game needs.
type BepInExBuild struct {
	Tag     string `json:"tag"`
	Version string `json:"version"`
	Asset   string `json:"asset"`
	Size    int64  `json:"size"`
	// Prerelease is true for BepInEx 6: IL2CPP support has no stable release
	// yet, so IL2CPP games get a pre-release build.
	Prerelease bool   `json:"prerelease"`
	URL        string `json:"url"`
}

// LoaderStatus describes the mod loader of a profile, if it has one.
type LoaderStatus struct {
	Installed bool   `json:"installed"`
	ModID     string `json:"modId,omitempty"`
	Name      string `json:"name,omitempty"`
	Version   string `json:"version,omitempty"`
	// Enabled is the user's choice; a disabled loader leaves the game vanilla.
	Enabled bool `json:"enabled"`
}

// assetPrefix is the start of the release file name for a game, lowercased.
func assetPrefix(game library.Game) (prefix string, prerelease bool) {
	windows := game.Runtime != library.RuntimeNative
	if game.Backend == library.BackendIL2CPP {
		if windows {
			return "bepinex-unity.il2cpp-win-x64-", true
		}
		return "bepinex-unity.il2cpp-linux-x64-", true
	}
	if windows {
		return "bepinex_win_x64_", false
	}
	return "bepinex_linux_x64_", false
}

// pickBepInEx takes the newest release carrying the file the game needs.
// Pre-releases are skipped unless the game needs BepInEx 6, which has none
// other.
func pickBepInEx(releases []github.Release, game library.Game) (github.Release, github.Asset, error) {
	prefix, prerelease := assetPrefix(game)
	for _, r := range releases {
		if r.Prerelease && !prerelease {
			continue
		}
		for _, a := range r.Assets {
			name := strings.ToLower(a.Name)
			if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".zip") {
				return r, a, nil
			}
		}
	}
	return github.Release{}, github.Asset{}, fmt.Errorf("no BepInEx release with a %s… build", prefix)
}

// BepInExBuild reports which BepInEx release a game would get from GitHub.
func (s *InstallService) BepInExBuild(ctx context.Context, gameID string) (BepInExBuild, error) {
	game, err := s.lib.GetGame(gameID)
	if err != nil {
		return BepInExBuild{}, err
	}
	releases, err := s.github.Releases(ctx, bepinexOwner, bepinexRepo)
	if err != nil {
		return BepInExBuild{}, err
	}
	release, asset, err := pickBepInEx(releases, game)
	if err != nil {
		return BepInExBuild{}, err
	}
	return BepInExBuild{
		Tag:        release.Tag,
		Version:    strings.TrimPrefix(release.Tag, "v"),
		Asset:      asset.Name,
		Size:       asset.Size,
		Prerelease: release.Prerelease,
		URL:        github.RepoURL(bepinexOwner, bepinexRepo) + "/releases/tag/" + release.Tag,
	}, nil
}

// InstallBepInEx installs that release into a profile as a mod, so it is
// listed, disabled and updated like the Thunderstore packs are.
func (s *InstallService) InstallBepInEx(ctx context.Context, gameID, profileID string) (library.Profile, error) {
	game, err := s.lib.GetGame(gameID)
	if err != nil {
		return library.Profile{}, err
	}
	releases, err := s.github.Releases(ctx, bepinexOwner, bepinexRepo)
	if err != nil {
		return library.Profile{}, err
	}
	release, asset, err := pickBepInEx(releases, game)
	if err != nil {
		return library.Profile{}, err
	}
	// The IL2CPP build carries a .NET runtime and is over 30 MB, long enough
	// to need a progress bar.
	path, err := s.github.Download(ctx, bepinexOwner, bepinexRepo, release.Tag, asset, func(done, total int64) {
		emitProgress(modinstall.Progress{
			GameID: gameID, ProfileID: profileID,
			Target: BepInExModID, Package: BepInExModID,
			Stage: modinstall.StageDownload,
			Done:  done, Total: total, Step: 0, Steps: 1,
		})
	})
	if err != nil {
		return library.Profile{}, err
	}
	pkg, err := modinstall.InspectFile(path)
	if err != nil {
		return library.Profile{}, err
	}
	pkg.Author, pkg.Name = bepinexOwner, bepinexRepo
	pkg.Version = modinstall.NormalizeVersion(release.Tag)
	return s.installer.InstallGitHub(ctx, gameID, profileID, pkg, bepinexOwner, bepinexRepo, release.Tag, asset.Name, false, emitProgress)
}

// LoaderStatus reports whether a profile has a mod loader, whichever way it
// got there: a Thunderstore pack, a GitHub release or a local archive. Without
// one Hermit links nothing into the game and it starts vanilla.
func (s *InstallService) LoaderStatus(gameID, profileID string) (LoaderStatus, error) {
	profile, err := s.lib.GetProfile(gameID, profileID)
	if err != nil {
		return LoaderStatus{}, err
	}
	for _, m := range profile.Mods {
		if !modinstall.IsLoader(m.Files) {
			continue
		}
		return LoaderStatus{
			Installed: true,
			ModID:     m.ID,
			Name:      m.Name,
			Version:   m.Version,
			Enabled:   m.Enabled,
		}, nil
	}
	return LoaderStatus{}, nil
}
