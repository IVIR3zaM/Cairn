package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// write a cairn.yaml in a temp dir and return its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cairn.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// A full file is parsed faithfully across every section.
func TestLoad_FullFile(t *testing.T) {
	path := writeConfig(t, `
version: "1"
project:
  canonical_version: "0.4.2"
  versioning: calver
languages:
  go:     { dir: ".",  enabled: true }
  python: { dir: "py", enabled: true, standard: ruff }
verify:
  build: { enabled: true, required: false }
version_sync:
  files:
    - { path: README.md, patterns: ["mylib:{VERSION}", "version-{VERSION}"] }
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Project.CanonicalVersion != "0.4.2" || cfg.Project.Versioning != "calver" {
		t.Errorf("project = %+v", cfg.Project)
	}
	if cfg.Languages["python"].Standard != "ruff" {
		t.Errorf("python standard = %q", cfg.Languages["python"].Standard)
	}
	if !cfg.Verify.Build.Enabled {
		t.Errorf("build should be enabled by file override")
	}
	files := cfg.VersionSync.Files
	if len(files) != 1 || files[0].Path != "README.md" || len(files[0].Patterns) != 2 {
		t.Errorf("version_sync = %+v", files)
	}
}

// A minimal file gets every absent value filled from in-code defaults, and an
// enabled language with no dir is normalized to ".".
func TestLoad_MinimalFile_FillsDefaults(t *testing.T) {
	path := writeConfig(t, `
version: "1"
languages:
  go: { enabled: true }
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Languages["go"].Dir; got != "." {
		t.Errorf("enabled go dir = %q, want \".\"", got)
	}
	if !cfg.Verify.Format.Enabled || cfg.Verify.Format.Mode != "check" {
		t.Errorf("format defaults missing: %+v", cfg.Verify.Format)
	}
	if !cfg.Verify.Test.Required {
		t.Errorf("test.required default should be true")
	}
	if cfg.Verify.Build.Enabled {
		t.Errorf("build should default to disabled")
	}
	if cfg.Changelog.File != "CHANGELOG.md" || cfg.Commits.Convention != "conventional" {
		t.Errorf("defaults not applied: changelog=%+v commits=%+v", cfg.Changelog, cfg.Commits)
	}
}

// An invalid value yields one actionable error naming the field and the choices.
func TestLoad_InvalidFile_ActionableError(t *testing.T) {
	path := writeConfig(t, `
version: "1"
project:
  versioning: weekly
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for bad versioning")
	}
	msg := err.Error()
	for _, want := range []string{"project.versioning", "weekly", "semver", "calver"} {
		if !contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

// verify.timeout must parse as a Go duration; a bad value is an actionable error, and a
// good one (or the default) is exposed as a time.Duration the gate can bound stages with.
func TestVerifyTimeout(t *testing.T) {
	bad := writeConfig(t, "version: \"1\"\nverify:\n  timeout: 5minutes\n")
	if _, err := Load(bad); err == nil || !contains(err.Error(), "verify.timeout") {
		t.Fatalf("bad timeout: want actionable error, got %v", err)
	}

	good := writeConfig(t, "version: \"1\"\nverify:\n  timeout: 90s\n")
	cfg, err := Load(good)
	if err != nil {
		t.Fatalf("good timeout: %v", err)
	}
	if cfg.Verify.StepTimeout() != 90*time.Second {
		t.Errorf("StepTimeout = %v, want 90s", cfg.Verify.StepTimeout())
	}
	if Default().Verify.StepTimeout() != 5*time.Minute {
		t.Errorf("default StepTimeout = %v, want 5m", Default().Verify.StepTimeout())
	}
}

// StrictFor resolves the per-language override against the repo-wide default:
// an explicit languages.<name>.strict wins (either direction), absent inherits.
func TestStrictFor(t *testing.T) {
	body := "version: \"1\"\n" +
		"verify:\n  strict: true\n" +
		"languages:\n" +
		"  go:\n    enabled: true\n" + // inherits verify.strict (true)
		"  dart:\n    enabled: true\n    strict: false\n" + // overrides down to false
		"  rust:\n    enabled: true\n    strict: true\n"
	cfg, err := Load(writeConfig(t, body))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	for _, tc := range []struct {
		lang string
		want bool
	}{
		{"go", true},     // no per-language key → inherits verify.strict=true
		{"dart", false},  // explicit override beats the true default
		{"rust", true},   // explicit override agreeing with the default
		{"python", true}, // undeclared language still inherits verify.strict
	} {
		if got := cfg.StrictFor(tc.lang); got != tc.want {
			t.Errorf("StrictFor(%q) = %v, want %v", tc.lang, got, tc.want)
		}
	}

	// The repo-wide default is false, so an undeclared language is relaxed.
	if Default().StrictFor("go") {
		t.Error("default StrictFor should be false")
	}
}

// LoadOrDefault returns the in-code defaults (no error) when the file is absent.
func TestLoadOrDefault_MissingFile(t *testing.T) {
	cfg, err := LoadOrDefault(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("LoadOrDefault: %v", err)
	}
	if cfg.Version != "1" || cfg.Changelog.Standard != "keepachangelog" {
		t.Errorf("defaults not returned: %+v", cfg)
	}
}

// A monorepo declares per-package versions: each entry parses, and VersioningFor resolves
// the per-package scheme override against the project-wide default.
func TestLoad_Packages_PerPackageVersions(t *testing.T) {
	path := writeConfig(t, `
version: "1"
project:
  canonical_version: "0.4.2"
  versioning: semver
  packages:
    - { path: services/api, version: "2.1.0" }
    - { path: pkgs/cli, version: "2025.6.0", versioning: calver }
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	pkgs := cfg.Project.Packages
	if len(pkgs) != 2 {
		t.Fatalf("packages = %+v", pkgs)
	}
	if pkgs[0].Path != "services/api" || pkgs[0].Version != "2.1.0" {
		t.Errorf("packages[0] = %+v", pkgs[0])
	}
	// absent override inherits project.versioning; explicit override wins.
	if got := pkgs[0].VersioningFor(cfg.Project.Versioning); got != "semver" {
		t.Errorf("packages[0] scheme = %q, want semver (inherited)", got)
	}
	if got := pkgs[1].VersioningFor(cfg.Project.Versioning); got != "calver" {
		t.Errorf("packages[1] scheme = %q, want calver (override)", got)
	}
}

// Each package entry must carry a non-empty path and version, and a scheme override (if
// given) must be a known one — all reported in one actionable error.
func TestLoad_Packages_InvalidEntries(t *testing.T) {
	path := writeConfig(t, `
version: "1"
project:
  packages:
    - { path: "", version: "" }
    - { path: pkgs/cli, version: "1.0.0", versioning: weekly }
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid package entries")
	}
	msg := err.Error()
	for _, want := range []string{
		"project.packages[0].path", "project.packages[0].version",
		"project.packages[1].versioning", "weekly",
	} {
		if !contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
