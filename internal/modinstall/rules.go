package modinstall

import (
	"strings"

	"hermit/internal/thunderstore"
)

// Rules decide where package files go in a profile. They come from the
// Thunderstore ecosystem schema, so packages end up exactly where r2modman
// puts them, which is the layout mod authors test against.
type Rules struct {
	Routes []thunderstore.InstallRule
	// RelativeFileExclusions are skipped by state-tracked routes.
	RelativeFileExclusions []string
	// LoaderPackages maps lower-case full package names of mod loaders to the
	// archive folder that is unpacked into the profile root.
	LoaderPackages map[string]string
	// Into, when set, overrides the routes: every file of the package goes
	// under this folder of the profile, keeping the archive's own layout.
	// It is for files the rules cannot place, like assets another mod reads
	// from a folder of its own.
	Into string
}

// IsLoaderPackage reports whether a package id ("<author>-<name>") is a
// BepInEx loader package. Without the schema list, names like BepInExPack or
// BepInEx_Valheim_Full are recognised.
func (r Rules) IsLoaderPackage(id string) bool {
	if len(r.LoaderPackages) > 0 {
		_, ok := r.LoaderPackages[strings.ToLower(id)]
		return ok
	}
	name := strings.ToLower(id[strings.LastIndexByte(id, '-')+1:])
	return strings.HasPrefix(name, "bepinexpack") || strings.HasPrefix(name, "bepinex_")
}

// DefaultRules are the rules shared by almost all BepInEx games, for games
// that are not in the schema.
func DefaultRules() Rules {
	return Rules{
		Routes: []thunderstore.InstallRule{
			{Route: "BepInEx/plugins", TrackingMethod: thunderstore.TrackingSubdir, DefaultFileExtensions: []string{".dll"}, IsDefaultLocation: true},
			{Route: "BepInEx/core", TrackingMethod: thunderstore.TrackingSubdir},
			{Route: "BepInEx/patchers", TrackingMethod: thunderstore.TrackingSubdir},
			{Route: "BepInEx/monomod", TrackingMethod: thunderstore.TrackingSubdir, DefaultFileExtensions: []string{".mm.dll"}},
			{Route: "BepInEx/config", TrackingMethod: thunderstore.TrackingNone},
		},
		LoaderPackages: map[string]string{},
	}
}

// RulesFromSchema returns the rules of a game, falling back to DefaultRules for
// games the schema does not know.
func RulesFromSchema(schema *thunderstore.Ecosystem, steamAppID, executable string) Rules {
	rules := DefaultRules()
	if schema == nil {
		return rules
	}
	for _, p := range schema.ModloaderPackages {
		if p.Loader == "bepinex" {
			rules.LoaderPackages[strings.ToLower(p.PackageID)] = p.RootFolder
		}
	}
	if _, settings, ok := schema.FindGame(steamAppID, executable); ok && len(settings.InstallRules) > 0 {
		rules.Routes = flattenRoutes(settings.InstallRules, "")
		rules.RelativeFileExclusions = settings.RelativeFileExclusions
	}
	return rules
}

func flattenRoutes(rules []thunderstore.InstallRule, prefix string) []thunderstore.InstallRule {
	var out []thunderstore.InstallRule
	for _, r := range rules {
		if prefix != "" {
			r.Route = prefix + "/" + r.Route
		}
		out = append(out, r)
		out = append(out, flattenRoutes(r.SubRoutes, r.Route)...)
	}
	return out
}
