package version

import (
	"strings"
	"testing"
	"testing/fstest"
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

// ReadVersion is SetVersion's mirror: it locates the project's own version across every
// manifest format, dropping a Maven qualifier and skipping a manifest that declares none.
func TestReadVersion(t *testing.T) {
	cases := []struct {
		file string
		in   string
		want string // empty means: no version locatable
	}{
		{"package.json", `{"name":"x","version":"1.2.3"}`, "1.2.3"},
		{"Cargo.toml", "[package]\nname = \"x\"\nversion = \"0.9.0\"\n", "0.9.0"},
		{"pyproject.toml", "[project]\nversion = \"1.0.0\"\n", "1.0.0"},
		{"pubspec.yaml", "name: x\nversion: 2.3.4\n", "2.3.4"},
		{"pom.xml", "<project><version>0.3.1-SNAPSHOT</version></project>", "0.3.1"},
		{"package.json", `{"name":"x"}`, ""},                                            // no version
		{"pom.xml", "<project><parent><version>3.2.0</version></parent></project>", ""}, // only a parent ref
	}
	for _, c := range cases {
		m, ok := ManagerFor(c.file)
		if !ok {
			t.Fatalf("ManagerFor(%q) not registered", c.file)
		}
		v, ok := m.ReadVersion([]byte(c.in))
		if c.want == "" {
			if ok {
				t.Errorf("%s: ReadVersion(%q) = %v, want no version", c.file, c.in, v)
			}
			continue
		}
		if !ok || v.String() != c.want {
			t.Errorf("%s: ReadVersion(%q) = (%v, %v), want %s", c.file, c.in, v, ok, c.want)
		}
	}
}

func TestDetectVersion(t *testing.T) {
	fsys := fstest.MapFS{
		"backend/pom.xml":       {Data: []byte("<project><version>0.3.1-SNAPSHOT</version></project>")},
		"frontend/package.json": {Data: []byte(`{"name":"web","version":"4.5.6"}`)},
	}
	// First unit with a locatable version wins, in order.
	got, ok := DetectVersion(fsys, []ManifestUnit{
		{Dir: "backend", Manifests: []string{"pom.xml"}},
		{Dir: "frontend", Manifests: []string{"package.json"}},
	})
	if !ok || got != "0.3.1" {
		t.Errorf("DetectVersion = (%q, %v), want 0.3.1", got, ok)
	}
}

func TestDetectVersionFallsBackWhenNoneFound(t *testing.T) {
	fsys := fstest.MapFS{
		"go.mod":        {Data: []byte("module x\n")},                                                  // no registered manager
		"child/pom.xml": {Data: []byte("<project><parent><version>1.0.0</version></parent></project>")}, // inheriting child, no own version
	}
	if got, ok := DetectVersion(fsys, []ManifestUnit{
		{Dir: ".", Manifests: []string{"go.mod"}},
		{Dir: "child", Manifests: []string{"pom.xml"}},
	}); ok {
		t.Errorf("DetectVersion = (%q, true), want no version found", got)
	}
}
