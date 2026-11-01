package app

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"

	"hermit/internal/github"
	"hermit/internal/library"
	"hermit/internal/modinstall"
	"hermit/internal/thunderstore"
)

// InstallProgressEvent carries modinstall.Progress to the frontend.
const InstallProgressEvent = "install:progress"

type InstallService struct {
	lib       *library.Library
	installer *modinstall.Installer
	github    *github.Client
	// downloads lists what is downloaded from Thunderstore with the files
	// from Nexus Mods, for the Downloads page and the status bar.
	downloads *DownloadService
}

func NewInstallService(lib *library.Library, installer *modinstall.Installer, gh *github.Client, downloads *DownloadService) *InstallService {
	return &InstallService{lib: lib, installer: installer, github: gh, downloads: downloads}
}

// follow runs an install from Thunderstore as an entry of the downloads, one
// however many packages it takes, which can cancel it from there. key names
// the package, so that its plan and its install share the entry.
func follow[T any](s *InstallService, ctx context.Context, gameID, profileID, key, name string, run func(ctx context.Context, progress func(modinstall.Progress)) (T, error)) (T, error) {
	if s.downloads == nil {
		return run(ctx, emitProgress)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	t := s.downloads.track(gameID, profileID, gameID+"/"+profileID+"/"+key, name, cancel)
	out, err := run(ctx, func(p modinstall.Progress) {
		emitProgress(p)
		t.progress(p)
	})
	t.finish(err)
	return out, err
}

func emitProgress(p modinstall.Progress) {
	if app := application.Get(); app != nil {
		app.Event.Emit(InstallProgressEvent, p)
	}
}

// PlanInstall downloads a package with its dependencies and reports what
// installing it would change, including conflicts with installed mods.
func (s *InstallService) PlanInstall(ctx context.Context, gameID, profileID, namespace, name, version string, opts modinstall.Options) (modinstall.InstallPlan, error) {
	ref := thunderstore.PackageRef{Namespace: namespace, Name: name, Version: version}
	return follow(s, ctx, gameID, profileID, namespace+"-"+name, name, func(ctx context.Context, progress func(modinstall.Progress)) (modinstall.InstallPlan, error) {
		return s.installer.PlanInstall(ctx, gameID, profileID, ref, opts, progress)
	})
}

// InstallPackage installs a Thunderstore package version with its dependencies
// into a profile, emitting InstallProgressEvent along the way. Conflicting
// installed mods are uninstalled only if opts.ReplaceConflicts is set.
func (s *InstallService) InstallPackage(ctx context.Context, gameID, profileID, namespace, name, version string, opts modinstall.Options) (library.Profile, error) {
	ref := thunderstore.PackageRef{Namespace: namespace, Name: name, Version: version}
	return follow(s, ctx, gameID, profileID, namespace+"-"+name, name, func(ctx context.Context, progress func(modinstall.Progress)) (library.Profile, error) {
		return s.installer.Install(ctx, gameID, profileID, ref, opts, progress)
	})
}

func (s *InstallService) UninstallMod(gameID, profileID, modID string) (library.Profile, error) {
	return s.installer.Uninstall(gameID, profileID, modID)
}

// SetModEnabled enables or disables a mod; dependants follow automatically.
func (s *InstallService) SetModEnabled(gameID, profileID, modID string, enabled bool) (library.Profile, error) {
	return s.installer.SetEnabled(gameID, profileID, modID, enabled)
}

// OpenProfile returns a profile with mod states brought up to date.
func (s *InstallService) OpenProfile(gameID, profileID string) (library.Profile, error) {
	return s.installer.Refresh(gameID, profileID)
}

// CheckUpdates lists installed Thunderstore mods with a newer version.
func (s *InstallService) CheckUpdates(ctx context.Context, gameID, profileID string) ([]modinstall.Update, error) {
	return s.installer.CheckUpdates(ctx, gameID, profileID)
}

// UpdateAll updates every outdated mod, emitting InstallProgressEvent.
func (s *InstallService) UpdateAll(ctx context.Context, gameID, profileID string) (modinstall.UpdateResult, error) {
	return follow(s, ctx, gameID, profileID, "updates", "Updates", func(ctx context.Context, progress func(modinstall.Progress)) (modinstall.UpdateResult, error) {
		return s.installer.UpdateAll(ctx, gameID, profileID, progress)
	})
}

// InstallModpack creates a new profile from a Thunderstore modpack.
func (s *InstallService) InstallModpack(ctx context.Context, gameID, namespace, name, version, profileName string) (library.Profile, error) {
	ref := thunderstore.PackageRef{Namespace: namespace, Name: name, Version: version}
	// A modpack is one entry: its mods are downloaded first and installed
	// once they are all there.
	return follow(s, ctx, gameID, "", "modpack/"+namespace+"-"+name, name, func(ctx context.Context, progress func(modinstall.Progress)) (library.Profile, error) {
		return s.installer.InstallAsNewProfile(ctx, gameID, profileName, ref, progress)
	})
}

// UpdateMod updates one Thunderstore or GitHub mod to its latest version.
func (s *InstallService) UpdateMod(ctx context.Context, gameID, profileID, modID string) (library.Profile, error) {
	return follow(s, ctx, gameID, profileID, modID, modID, func(ctx context.Context, progress func(modinstall.Progress)) (library.Profile, error) {
		return s.installer.UpdateMod(ctx, gameID, profileID, modID, progress)
	})
}

// InspectFile reads a .zip or .dll mod file and suggests its author, name and version.
func (s *InstallService) InspectFile(path string) (modinstall.LocalPackage, error) {
	return modinstall.InspectFile(path)
}

// PlanFile reports conflicts of installing a local or downloaded mod file.
func (s *InstallService) PlanFile(ctx context.Context, gameID, profileID string, pkg modinstall.LocalPackage) (modinstall.InstallPlan, error) {
	return s.installer.PlanFile(ctx, gameID, profileID, pkg)
}

// InstallFile installs a local mod file.
func (s *InstallService) InstallFile(ctx context.Context, gameID, profileID string, pkg modinstall.LocalPackage, replaceConflicts bool) (library.Profile, error) {
	return s.installer.InstallFile(ctx, gameID, profileID, pkg, replaceConflicts, emitProgress)
}

type GitHubRepo struct {
	Owner    string           `json:"owner"`
	Repo     string           `json:"repo"`
	Releases []github.Release `json:"releases"`
}

// GitHubReleases lists the releases of a repository given as owner/repo or URL.
func (s *InstallService) GitHubReleases(ctx context.Context, input string) (GitHubRepo, error) {
	owner, repo, err := github.ParseRepo(input)
	if err != nil {
		return GitHubRepo{}, err
	}
	releases, err := s.github.Releases(ctx, owner, repo)
	if err != nil {
		return GitHubRepo{}, err
	}
	return GitHubRepo{Owner: owner, Repo: repo, Releases: releases}, nil
}

// DownloadGitHubAsset downloads a release file and suggests how to name the
// mod: from its manifest if it has one, otherwise after the repository and tag.
func (s *InstallService) DownloadGitHubAsset(ctx context.Context, owner, repo, tag string, asset github.Asset) (modinstall.LocalPackage, error) {
	path, err := s.github.Download(ctx, owner, repo, tag, asset, nil)
	if err != nil {
		return modinstall.LocalPackage{}, err
	}
	pkg, err := modinstall.InspectFile(path)
	if err != nil {
		return modinstall.LocalPackage{}, err
	}
	if !pkg.HasManifest {
		pkg.Author = modinstall.SanitizeName(owner, "GitHub")
		pkg.Name = modinstall.SanitizeName(repo, pkg.Name)
		pkg.Version = modinstall.NormalizeVersion(tag)
	}
	return pkg, nil
}

// InstallGitHub installs a downloaded release file as a mod that can be updated from GitHub.
func (s *InstallService) InstallGitHub(ctx context.Context, gameID, profileID string, pkg modinstall.LocalPackage, owner, repo, tag, asset string, replaceConflicts bool) (library.Profile, error) {
	return s.installer.InstallGitHub(ctx, gameID, profileID, pkg, owner, repo, tag, asset, replaceConflicts, emitProgress)
}
