package detect

import (
	"os/exec"
	"strings"
	"testing"
	"testing/fstest"
)

// allMissing is a LookupFunc that reports every tool as absent.
func allMissing(string) (string, error) { return "", exec.ErrNotFound }

func unit(r *Result, name string) (Language, bool) {
	for _, l := range r.Languages {
		if l.Name == name {
			return l, true
		}
	}
	return Language{}, false
}

// One marker per language across nested dirs, plus a marker buried in node_modules
// that must be ignored — covers language, dir, and package-manager detection at once.
func TestDetect_LanguagesDirsAndPackageManagers(t *testing.T) {
	fsys := fstest.MapFS{
		"go.mod":                          {},
		"py/pyproject.toml":               {},
		"rs/Cargo.toml":                   {},
		"web/package.json":                {},
		"svc/pom.xml":                     {},
		"app/pubspec.yaml":                {},
		"web/node_modules/dep/go.mod":     {}, // must be skipped
		"web/node_modules/dep/Cargo.toml": {}, // must be skipped
	}

	res, err := Detect(fsys, allMissing)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}

	want := []struct{ name, dir, pm string }{
		{"dart", "app", "pub"},
		{"go", ".", "go modules"},
		{"java", "svc", "maven"},
		{"javascript", "web", "npm"},
		{"python", "py", "pip"},
		{"rust", "rs", "cargo"},
	}
	if len(res.Languages) != len(want) {
		t.Fatalf("got %d languages, want %d: %+v", len(res.Languages), len(want), res.Languages)
	}
	for i, w := range want {
		got := res.Languages[i] // Result is sorted by (name, dir)
		if got.Name != w.name || got.Dir != w.dir || got.PackageManager != w.pm {
			t.Errorf("language %d = {%s %s %s}, want {%s %s %s}",
				i, got.Name, got.Dir, got.PackageManager, w.name, w.dir, w.pm)
		}
	}
}

// Tool resolution reflects what the lookup reports, and each tool is looked up once.
func TestDetect_ToolInstalledStatus(t *testing.T) {
	var calls int
	look := func(name string) (string, error) {
		calls++
		if name == "go" || name == "gofumpt" {
			return "/usr/bin/" + name, nil
		}
		return "", exec.ErrNotFound
	}

	res, err := Detect(fstest.MapFS{"go.mod": {}}, look)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	g, ok := unit(res, "go")
	if !ok {
		t.Fatal("go not detected")
	}

	got := map[string]bool{}
	for _, ts := range g.Tools {
		got[ts.Tool.Name] = ts.Installed
	}
	for name, want := range map[string]bool{"go": true, "gofumpt": true, "golangci-lint": false} {
		if got[name] != want {
			t.Errorf("%s installed = %v, want %v", name, got[name], want)
		}
	}
	if calls != len(g.Tools) {
		t.Errorf("lookup called %d times, want %d (one per distinct tool)", calls, len(g.Tools))
	}
}

func TestDetect_NoLanguages(t *testing.T) {
	res, err := Detect(fstest.MapFS{"README.md": {}}, allMissing)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(res.Languages) != 0 {
		t.Fatalf("want no languages, got %+v", res.Languages)
	}
}

// The registry is assembled from each lang_<name>.go file's init(); a language
// contributing skipDirs (here rust's "target") must have them honored by a scan, and
// re-registering an existing name must fail loudly rather than shadow it. This pins
// the "add a file to add a language" contract.
func TestRegistry_SelfRegistrationAndSkipDirs(t *testing.T) {
	// Every language file registered exactly once, and its skipDirs were merged.
	if len(registry) == 0 {
		t.Fatal("registry is empty: no language self-registered")
	}
	if !skipDirs["target"] {
		t.Error(`rust's "target" skipDir was not merged into skipDirs`)
	}

	// A marker buried under a language-contributed skip dir must be ignored.
	res, err := Detect(fstest.MapFS{
		"rs/Cargo.toml":                {},
		"rs/target/dep/pyproject.toml": {}, // inside rust's skipDir → ignored
	}, allMissing)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if _, ok := unit(res, "python"); ok {
		t.Error("python detected inside rust's target/ skipDir; should be skipped")
	}

	// Duplicate registration is a programming error and must panic.
	defer func() {
		if recover() == nil {
			t.Error("registering a duplicate language name did not panic")
		}
	}()
	register(langSpec{name: "go"})
}

func TestRender(t *testing.T) {
	r := &Result{Languages: []Language{{
		Name: "go", Dir: ".", PackageManager: "go modules",
		Tools: []ToolStatus{
			{Tool: Tool{Name: "go", Hint: "https://go.dev/dl/"}, Installed: true},
			{Tool: Tool{Name: "golangci-lint", Hint: "see-install"}, Installed: false},
		},
	}}}

	var sb strings.Builder
	Render(&sb, r)
	out := sb.String()

	for _, want := range []string{
		"go  (dir: ., pkg: go modules)",
		glyphPresent + " go",
		glyphMissing + " golangci-lint  → see-install",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Render output missing %q\n---\n%s", want, out)
		}
	}
}

func TestRender_Empty(t *testing.T) {
	var sb strings.Builder
	Render(&sb, &Result{})
	if !strings.Contains(sb.String(), "No supported languages") {
		t.Errorf("empty Render = %q", sb.String())
	}
}
