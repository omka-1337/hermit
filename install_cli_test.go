package main

import (
	"archive/zip"
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hermit/internal/app"
	"hermit/internal/library"
)

const cliFomod = `<config><moduleName>Bodies</moduleName><installSteps order="Explicit">
  <installStep name="Body"><optionalFileGroups>
    <group name="Body type" type="SelectExactlyOne"><plugins order="Explicit">
      <plugin name="Slim"><files><file source="slim.nif" destination="meshes/body.nif"/></files>
        <conditionFlags><flag name="extras">yes</flag></conditionFlags>
        <typeDescriptor><type name="Optional"/></typeDescriptor></plugin>
      <plugin name="Athletic"><files><file source="athletic.nif" destination="meshes/body.nif"/></files>
        <typeDescriptor><type name="Recommended"/></typeDescriptor></plugin>
    </plugins></group></optionalFileGroups></installStep>
  <installStep name="Extras"><visible><flagDependency flag="extras" value="yes"/></visible><optionalFileGroups>
    <group name="Extra bits" type="SelectAny"><plugins>
      <plugin name="Freckles"><files><file source="freckles.dds" destination="textures/freckles.dds"/></files>
        <typeDescriptor><type name="Optional"/></typeDescriptor></plugin>
    </plugins></group></optionalFileGroups></installStep>
</installSteps></config>`

func prepareCLI(t *testing.T) (*app.ModListService, app.PreparedMod) {
	t.Helper()
	tmp := t.TempDir()
	lib, err := library.New(filepath.Join(tmp, "lib"), nil)
	if err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(tmp, "Skyrim Special Edition")
	os.MkdirAll(filepath.Join(game, "Data"), 0o755)
	os.WriteFile(filepath.Join(game, "SkyrimSE.exe"), nil, 0o644)
	os.WriteFile(filepath.Join(game, "Data", "Skyrim.esm"), nil, 0o644)
	g, err := lib.AddGame("Skyrim", game)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(tmp, "Bodies.zip")
	f, _ := os.Create(file)
	w := zip.NewWriter(f)
	for name, content := range map[string]string{"fomod/ModuleConfig.xml": cliFomod, "slim.nif": "s", "athletic.nif": "a", "freckles.dds": "f"} {
		fw, _ := w.Create(name)
		fw.Write([]byte(content))
	}
	w.Close()
	f.Close()
	svc := app.NewModListService(lib, nil)
	prepared, err := svc.PrepareInstall(g.ID, g.ActiveProfile, file)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.CancelInstall(prepared.Session) })
	return svc, prepared
}

func picked(t *testing.T, svc *app.ModListService, prepared app.PreparedMod, choose []string, input string) string {
	t.Helper()
	var out strings.Builder
	choices, err := pickOptions(svc, prepared, choose, input != "", strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	view, _ := svc.FomodView(prepared.Session, choices)
	var names []string
	for _, s := range view.Steps {
		for _, g := range s.Groups {
			for _, p := range g.Plugins {
				if p.Selected {
					names = append(names, p.Name)
				}
			}
		}
	}
	return strings.Join(names, ",")
}

func TestPickOptions(t *testing.T) {
	svc, prepared := prepareCLI(t)
	if got := picked(t, svc, prepared, nil, ""); got != "Athletic" {
		t.Errorf("defaults: %s", got)
	}
	// The option picked brings up the step the second choice is in.
	if got := picked(t, svc, prepared, []string{"Body type=slim", "Extras/Extra bits=Freckles"}, ""); got != "Slim,Freckles" {
		t.Errorf("chosen: %s", got)
	}
	// Asked step by step: Slim, then the freckles in the step it brings up.
	if got := picked(t, svc, prepared, nil, "1\n1\n"); got != "Slim,Freckles" {
		t.Errorf("asked: %s", got)
	}
	if _, err := pickOptions(svc, prepared, []string{"Body type=Curvy"}, false, nil, nil); err == nil {
		t.Error("picked an option the installer does not have")
	}
}

func TestParseInstallArgs(t *testing.T) {
	a, err := parseInstallArgs([]string{"--game", "skyrim", "--choose", "A=B", "--choose", "C/D=E,F", "mod.7z", "other.zip"})
	if err != nil || a.game != "skyrim" || len(a.choose) != 2 || len(a.files) != 2 {
		t.Errorf("%+v %v", a, err)
	}
	if _, err := parseInstallArgs([]string{"--defaults"}); err == nil {
		t.Error("no archive given, no error")
	}
}

func TestExistingTarget(t *testing.T) {
	prepared := app.PreparedMod{Name: "Lux", Existing: "Lux"}
	ask := func(input string) (string, string) {
		target, name, err := existingTarget(prepared, installArgs{}, true, bufio.NewReader(strings.NewReader(input)), &strings.Builder{})
		if err != nil {
			t.Fatal(err)
		}
		return target, name
	}
	if target, _ := ask("x\nm\n"); target != app.TargetMerge {
		t.Errorf("merge: %s", target)
	}
	if target, name := ask("n\nLux 2\n"); target != app.TargetRename || name != "Lux 2" {
		t.Errorf("rename: %s %q", target, name)
	}
	if _, _, err := existingTarget(prepared, installArgs{}, false, nil, nil); err == nil {
		t.Error("went on without being told what to do")
	}
	if target, _, _ := existingTarget(prepared, installArgs{existing: "replace"}, false, nil, nil); target != app.TargetReplace {
		t.Errorf("flag: %s", target)
	}
	if _, err := parseInstallArgs([]string{"--existing", "rename", "mod.7z"}); err == nil {
		t.Error("rename without a name")
	}
}
