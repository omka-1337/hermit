package app

import (
	"testing"

	"hermit/internal/github"
	"hermit/internal/library"
)

// releases as GitHub returns them: newest first, BepInEx 6 published before
// the last BepInEx 5 releases.
var bepinexReleases = []github.Release{
	{Tag: "v5.4.23.5", Assets: []github.Asset{
		{Name: "BepInEx_linux_x64_5.4.23.5.zip"},
		{Name: "BepInEx_macos_universal_5.4.23.5.zip"},
		{Name: "BepInEx_Patcher_5.4.23.5.zip"},
		{Name: "BepInEx_win_x64_5.4.23.5.zip"},
		{Name: "BepInEx_win_x86_5.4.23.5.zip"},
	}},
	{Tag: "v6.0.0-pre.2", Prerelease: true, Assets: []github.Asset{
		{Name: "BepInEx-NET.CoreCLR-net6.0-win-x64-6.0.0-pre.2.zip"},
		{Name: "BepInEx-Unity.IL2CPP-linux-x64-6.0.0-pre.2.zip"},
		{Name: "BepInEx-Unity.IL2CPP-win-x64-6.0.0-pre.2.zip"},
		{Name: "BepInEx-Unity.Mono-win-x64-6.0.0-pre.2.zip"},
	}},
	{Tag: "v5.4.23.2", Assets: []github.Asset{
		{Name: "BepInEx_win_x64_5.4.23.2.zip"},
	}},
}

func TestPickBepInEx(t *testing.T) {
	tests := []struct {
		name      string
		runtime   library.Runtime
		backend   library.Backend
		wantTag   string
		wantAsset string
	}{
		{"mono under proton takes the newest BepInEx 5 for windows",
			library.RuntimeProton, library.BackendMono, "v5.4.23.5", "BepInEx_win_x64_5.4.23.5.zip"},
		{"mono native build takes the linux files",
			library.RuntimeNative, library.BackendMono, "v5.4.23.5", "BepInEx_linux_x64_5.4.23.5.zip"},
		{"il2cpp takes the BepInEx 6 pre-release",
			library.RuntimeProton, library.BackendIL2CPP, "v6.0.0-pre.2", "BepInEx-Unity.IL2CPP-win-x64-6.0.0-pre.2.zip"},
		{"il2cpp native build takes the linux pre-release",
			library.RuntimeNative, library.BackendIL2CPP, "v6.0.0-pre.2", "BepInEx-Unity.IL2CPP-linux-x64-6.0.0-pre.2.zip"},
		{"unknown backend is treated as mono",
			library.RuntimeProton, library.BackendUnknown, "v5.4.23.5", "BepInEx_win_x64_5.4.23.5.zip"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			game := library.Game{Runtime: tc.runtime, Backend: tc.backend}
			release, asset, err := pickBepInEx(bepinexReleases, game)
			if err != nil {
				t.Fatal(err)
			}
			if release.Tag != tc.wantTag || asset.Name != tc.wantAsset {
				t.Errorf("got %s %s, want %s %s", release.Tag, asset.Name, tc.wantTag, tc.wantAsset)
			}
		})
	}
}

// A Mono game must not be given a BepInEx 6 pre-release even when it is the
// only thing left, so a repository without BepInEx 5 files is an error.
func TestPickBepInExNoStableBuild(t *testing.T) {
	only6 := bepinexReleases[1:2]
	game := library.Game{Runtime: library.RuntimeProton, Backend: library.BackendMono}
	if _, _, err := pickBepInEx(only6, game); err == nil {
		t.Fatal("expected an error, got a build")
	}
}
