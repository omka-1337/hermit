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
}

func NewInstallService(lib *library.Library, installer *modinstall.Installer, gh *github.Client) *InstallService {
	return &InstallService{lib: lib, installer: installer, github: gh}
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
	return s.installer.PlanInstall(ctx, gameID, profileID, ref, opts, emitProgress)
}

// InstallPackage installs a Thunderstore package version with its dependencies
// into a profile, emitting InstallProgressEvent along the way. Conflicting
// installed mods are uninstalled only if opts.ReplaceConflicts is set.
func (s *InstallService) InstallPackage(ctx context.Context, gameID, profileID, namespace, name, version string, opts modinstall.Options) (library.Profile, error) {
	ref := thunderstore.PackageRef{Namespace: namespace, Name: name, Version: version}
	return s.installer.Install(ctx, gameID, profileID, ref, opts, emitProgress)
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
	return s.installer.UpdateAll(ctx, gameID, profileID, emitProgress)
}

// InstallModpack creates a new profile from a Thunderstore modpack.
func (s *InstallService) InstallModpack(ctx context.Context, gameID, namespace, name, version, profileName string) (library.Profile, error) {
	ref := thunderstore.PackageRef{Namespace: namespace, Name: name, Version: version}
	return s.installer.InstallAsNewProfile(ctx, gameID, profileName, ref, emitProgress)
}

// UpdateMod updates one Thunderstore or GitHub mod to its latest version.
func (s *InstallService) UpdateMod(ctx context.Context, gameID, profileID, modID string) (library.Profile, error) {
	return s.installer.UpdateMod(ctx, gameID, profileID, modID, emitProgress)
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
