package fomod

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/text/encoding/unicode"
)

const sample = `<?xml version="1.0" encoding="UTF-16"?>
<config>
  <moduleName>Sample</moduleName>
  <requiredInstallFiles>
    <folder source="Core" destination="" />
  </requiredInstallFiles>
  <installSteps order="Explicit">
    <installStep name="Body">
      <optionalFileGroups>
        <group name="Body" type="SelectExactlyOne">
          <plugins order="Explicit">
            <plugin name="Slim">
              <description>Thin</description>
              <files><folder source="Bodies\Slim" destination="meshes" /></files>
              <conditionFlags><flag name="body">slim</flag></conditionFlags>
              <typeDescriptor><type name="Optional" /></typeDescriptor>
            </plugin>
            <plugin name="Athletic">
              <files><folder source="Bodies\Athletic" destination="meshes" /></files>
              <conditionFlags><flag name="body">athletic</flag></conditionFlags>
              <typeDescriptor><type name="Recommended" /></typeDescriptor>
            </plugin>
          </plugins>
        </group>
        <group name="Patches" type="SelectAny">
          <plugins>
            <plugin name="Zeta patch">
              <files><file source="Patches\Zeta.esp" destination="Zeta.esp" /></files>
              <typeDescriptor>
                <dependencyType>
                  <defaultType name="NotUsable" />
                  <patterns>
                    <pattern>
                      <dependencies operator="Or"><fileDependency file="Zeta.esm" state="Active" /></dependencies>
                      <type name="Recommended" />
                    </pattern>
                  </patterns>
                </dependencyType>
              </typeDescriptor>
            </plugin>
            <plugin name="Alpha patch">
              <files><file source="patches\alpha.esp" /></files>
              <typeDescriptor><type name="Optional" /></typeDescriptor>
            </plugin>
          </plugins>
        </group>
      </optionalFileGroups>
    </installStep>
    <installStep name="Slim extras">
      <visible><flagDependency flag="body" value="slim" /></visible>
      <optionalFileGroups>
        <group name="Extras" type="SelectAll">
          <plugins>
            <plugin name="Extra"><files><file source="Extra.txt" destination="docs\" /></files>
              <typeDescriptor><type name="Optional" /></typeDescriptor></plugin>
          </plugins>
        </group>
      </optionalFileGroups>
    </installStep>
  </installSteps>
  <conditionalFileInstalls>
    <patterns>
      <pattern>
        <dependencies><flagDependency flag="body" value="athletic" /></dependencies>
        <files><file source="Bodies\Athletic\body.nif" destination="meshes\override.nif" priority="5" /></files>
      </pattern>
    </patterns>
  </conditionalFileInstalls>
</config>`

func utf16(t *testing.T, s string) []byte {
	t.Helper()
	data, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func names(v View) []string {
	var out []string
	for _, s := range v.Steps {
		for _, g := range s.Groups {
			for _, p := range g.Plugins {
				if p.Selected {
					out = append(out, s.Name+"/"+g.Name+"/"+p.Name)
				}
			}
		}
	}
	return out
}

func TestEvaluate(t *testing.T) {
	cfg, err := Parse(utf16(t, sample))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "Sample" || len(cfg.Steps) != 2 {
		t.Fatalf("parsed %+v", cfg)
	}
	// Plugins without an order attribute are sorted by name.
	if p := cfg.Steps[0].Groups[1].Plugins; p[0].Name != "Alpha patch" {
		t.Errorf("patches in order %q, %q", p[0].Name, p[1].Name)
	}

	missing := func(string) FileState { return Missing }
	v := cfg.Evaluate(nil, missing)
	// Defaults: the recommended body; the Zeta patch cannot be used.
	if got := names(v); !slices.Equal(got, []string{"Body/Body/Athletic"}) {
		t.Errorf("defaults %v", got)
	}
	if v.Steps[0].Groups[1].Plugins[1].Type != NotUsable || len(v.Steps) != 1 || !v.Ready {
		t.Errorf("view %+v", v)
	}

	// With Zeta active, its patch is recommended; slim shows the extras step.
	active := func(f string) FileState {
		if f == "Zeta.esm" {
			return Active
		}
		return Missing
	}
	v = cfg.Evaluate(Choices{{{0}, nil}}, active)
	want := []string{"Body/Body/Slim", "Body/Patches/Zeta patch", "Slim extras/Extras/Extra"}
	if got := names(v); !slices.Equal(got, want) {
		t.Errorf("picked %v, want %v", got, want)
	}

	// Picking two in a group of exactly one keeps one.
	if v = cfg.Evaluate(Choices{{{0, 1}}}, missing); len(v.Choices[0][0]) != 1 {
		t.Errorf("exactly one: %v", v.Choices[0][0])
	}
	// A group of exactly one with nothing picked is not ready.
	if v = cfg.Evaluate(Choices{{{}}}, missing); v.Ready || v.Steps[0].Groups[0].Problem == "" {
		t.Errorf("nothing picked: %+v", v.Steps[0].Groups[0])
	}
}

func TestInstall(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"fomod/ModuleConfig.xml", "core/core.esp", "Bodies/athletic/body.nif", "Bodies/Slim/body.nif", "Patches/Alpha.esp", "Patches/Zeta.esp", "extra.txt"} {
		p := filepath.Join(root, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(f), 0o644)
	}
	if err := os.WriteFile(filepath.Join(root, "fomod", "ModuleConfig.xml"), utf16(t, sample), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	v := cfg.Evaluate(Choices{{{1}, {0}}}, nil)
	dest := t.TempDir()
	if err := Install(root, v.Files, dest); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{
		"core.esp":            "core/core.esp",
		"meshes/body.nif":     "Bodies/athletic/body.nif",
		"meshes/override.nif": "Bodies/athletic/body.nif",
		"patches/alpha.esp":   "Patches/Alpha.esp",
	} {
		data, err := os.ReadFile(filepath.Join(dest, rel))
		if err != nil || string(data) != want {
			t.Errorf("%s: %q, %v", rel, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "Zeta.esp")); err == nil {
		t.Error("installed a patch that was not picked")
	}

	// The extras go into a folder given with a trailing separator.
	v = cfg.Evaluate(Choices{{{0}, {}}}, nil)
	dest = t.TempDir()
	if err := Install(root, v.Files, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "docs", "Extra.txt")); err != nil {
		t.Errorf("extra: %v", err)
	}
}
