package config

import (
	"os"
	"path/filepath"
	"testing"
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

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
