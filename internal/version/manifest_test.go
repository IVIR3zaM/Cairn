package version

import (
	"strings"
	"testing"
)

func TestSetVersion(t *testing.T) {
	v := Version{2, 0, 0}
	cases := []struct {
		file    string
		in      string
		want    string // substring the result must contain
		changed bool
	}{
		{"package.json", `{"name":"x","version":"1.2.3"}`, `"version":"2.0.0"`, true},
		{"package.json", `{"version": "2.0.0"}`, `"version": "2.0.0"`, false}, // already correct
		{"Cargo.toml", "[package]\nname = \"x\"\nversion = \"0.9.0\"\n", "version = \"2.0.0\"", true},
		{"pyproject.toml", "[project]\nversion = \"1.0.0\"\n", "version = \"2.0.0\"", true},
	}
	for _, c := range cases {
		m, ok := ManagerFor(c.file)
		if !ok {
			t.Fatalf("ManagerFor(%q) not registered", c.file)
		}
		out, changed, err := m.SetVersion([]byte(c.in), v)
		if err != nil {
			t.Fatalf("%s: SetVersion: %v", c.file, err)
		}
		if changed != c.changed {
			t.Errorf("%s: changed = %v, want %v", c.file, changed, c.changed)
		}
		if !strings.Contains(string(out), c.want) {
			t.Errorf("%s: result %q missing %q", c.file, out, c.want)
		}
	}
}

// A dependency pin (not a line-leading version) must not be mistaken for the package
// version: setVia takes the first ^version match only.
func TestSetVersionTomlIgnoresDependencyPin(t *testing.T) {
	in := "[package]\nversion = \"1.0.0\"\n\n[dependencies]\nserde = { version = \"1.2.3\" }\n"
	m, _ := ManagerFor("Cargo.toml")
	out, _, err := m.SetVersion([]byte(in), Version{2, 0, 0})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "version = \"2.0.0\"") || !strings.Contains(string(out), "serde = { version = \"1.2.3\" }") {
		t.Errorf("only the package version should change, got:\n%s", out)
	}
}

func TestSetVersionNoVersionErrors(t *testing.T) {
	m, _ := ManagerFor("package.json")
	if _, _, err := m.SetVersion([]byte(`{"name":"x"}`), Version{1, 0, 0}); err == nil {
		t.Error("a manifest without a version should error, not silently no-op")
	}
}

func TestManagersListedAndRegistered(t *testing.T) {
	got := map[string]bool{}
	for _, m := range Managers() {
		got[m.Filename()] = true
	}
	for _, want := range []string{"package.json", "Cargo.toml", "pyproject.toml"} {
		if !got[want] {
			t.Errorf("Managers() missing %q", want)
		}
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("registering a duplicate filename should panic")
		}
	}()
	register(npm{}) // package.json already registered in init
}
