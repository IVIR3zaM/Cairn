package config

import (
	"testing"
	"testing/fstest"
)

// resolveVersion is a small helper: resolve dir and return its effective version string.
func resolveVersion(t *testing.T, tr *Tree, dir string) string {
	t.Helper()
	d, ok := tr.Resolve(dir)
	if !ok {
		t.Fatalf("Resolve(%q): unexpectedly pruned", dir)
	}
	if d.Version == nil {
		return ""
	}
	return *d.Version
}

// A schema-2 file parses: the baseline lives in top-level keys, per-directory overrides in the
// directories map, and Resolve folds them so an independent directory carries its own version.
func TestLoadTree_SchemaTwoParses(t *testing.T) {
	fsys := fstest.MapFS{
		"cairn.yaml": &fstest.MapFile{Data: []byte(`
schema: "2"
version: "1.0.0"
versioning: semver
directories:
  pkg_a: { version: "2.3.0" }
  pkg_b: { version: "0.9.1", versioning: calver }
`)},
	}
	tr, err := LoadTree(fsys)
	if err != nil {
		t.Fatalf("LoadTree: %v", err)
	}
	if got := resolveVersion(t, tr, "."); got != "1.0.0" {
		t.Errorf("root version = %q, want 1.0.0 (baseline)", got)
	}
	if got := resolveVersion(t, tr, "pkg_a"); got != "2.3.0" {
		t.Errorf("pkg_a version = %q, want 2.3.0 (override)", got)
	}
	d, _ := tr.Resolve("pkg_b")
	if d.Versioning == nil || *d.Versioning != "calver" {
		t.Errorf("pkg_b versioning = %v, want calver", d.Versioning)
	}
}

// Precedence example 1: a root directories override beats the directory's own cairn.yaml
// (layer 3 over layer 2).
func TestResolve_RootOverrideBeatsOwnFile(t *testing.T) {
	fsys := fstest.MapFS{
		"cairn.yaml": &fstest.MapFile{Data: []byte(`
schema: "2"
version: "1.0.0"
verify: { strict: true }
directories:
  somerepo: { verify: { strict: true } }
`)},
		"somerepo/cairn.yaml": &fstest.MapFile{Data: []byte(`
verify: { strict: false }
`)},
	}
	tr, err := LoadTree(fsys)
	if err != nil {
		t.Fatalf("LoadTree: %v", err)
	}
	d, _ := tr.Resolve("somerepo")
	if d.Verify == nil || !d.Verify.Strict {
		t.Errorf("somerepo strict = %v, want true (root override wins over own file)", d.Verify)
	}
}

// Precedence example 2: with no root directories entry, the directory's own cairn.yaml governs
// over the repo baseline (layer 2 over layer 1).
func TestResolve_OwnFileBeatsBaseline(t *testing.T) {
	fsys := fstest.MapFS{
		"cairn.yaml": &fstest.MapFile{Data: []byte(`
schema: "2"
version: "1.0.0"
verify: { strict: true }
`)},
		"somerepo/cairn.yaml": &fstest.MapFile{Data: []byte(`
verify: { strict: false }
`)},
	}
	tr, err := LoadTree(fsys)
	if err != nil {
		t.Fatalf("LoadTree: %v", err)
	}
	d, _ := tr.Resolve("somerepo")
	if d.Verify == nil || d.Verify.Strict {
		t.Errorf("somerepo strict = %v, want false (own file wins over baseline)", d.Verify)
	}
}

// The absolute disable gate prunes a subtree and its own cairn.yaml is never read: a gated
// directory holding invalid YAML must not error, and Resolve reports it pruned.
func TestDisableGate_OwnFileNeverRead(t *testing.T) {
	fsys := fstest.MapFS{
		"cairn.yaml": &fstest.MapFile{Data: []byte(`
schema: "2"
version: "1.0.0"
directories:
  vendored: { enabled: false }
`)},
		// Invalid YAML: if the gate read this, LoadTree would fail.
		"vendored/cairn.yaml":     &fstest.MapFile{Data: []byte(": this is not valid yaml {[")},
		"vendored/sub/cairn.yaml": &fstest.MapFile{Data: []byte("also: [broken")},
	}
	tr, err := LoadTree(fsys)
	if err != nil {
		t.Fatalf("LoadTree should not read pruned files: %v", err)
	}
	if _, ok := tr.Resolve("vendored"); ok {
		t.Error("Resolve(vendored) ok = true, want pruned")
	}
	if _, ok := tr.Resolve("vendored/sub"); ok {
		t.Error("Resolve(vendored/sub) ok = true, want pruned (descendant of disabled)")
	}
	if got := tr.Pruned(); len(got) == 0 || got[0] != "vendored" {
		t.Errorf("Pruned() = %v, want [vendored ...]", got)
	}
}

// A legacy version:"1" / project: file is accepted and translated: the canonical version
// becomes the baseline and each package becomes an independent directory — a single-package
// (no packages) legacy repo resolves exactly to its old settings.
func TestLoadTree_LegacyTranslated(t *testing.T) {
	single := fstest.MapFS{
		"cairn.yaml": &fstest.MapFile{Data: []byte(`
version: "1"
project:
  canonical_version: "1.2.3"
  versioning: calver
`)},
	}
	tr, err := LoadTree(single)
	if err != nil {
		t.Fatalf("LoadTree legacy single: %v", err)
	}
	if got := resolveVersion(t, tr, "."); got != "1.2.3" {
		t.Errorf("legacy root version = %q, want 1.2.3", got)
	}
	if d, _ := tr.Resolve("."); d.Versioning == nil || *d.Versioning != "calver" {
		t.Errorf("legacy versioning not carried: %v", d.Versioning)
	}

	monorepo := fstest.MapFS{
		"cairn.yaml": &fstest.MapFile{Data: []byte(`
version: "1"
project:
  canonical_version: "1.0.0"
  packages:
    - { path: pkg_a, version: "2.0.0" }
`)},
	}
	tr2, err := LoadTree(monorepo)
	if err != nil {
		t.Fatalf("LoadTree legacy monorepo: %v", err)
	}
	if got := resolveVersion(t, tr2, "pkg_a"); got != "2.0.0" {
		t.Errorf("legacy package version = %q, want 2.0.0", got)
	}
}

// An unknown schema is rejected with an actionable error rather than silently misread.
func TestLoadTree_UnknownSchemaRejected(t *testing.T) {
	fsys := fstest.MapFS{
		"cairn.yaml": &fstest.MapFile{Data: []byte(`schema: "9"` + "\n")},
	}
	if _, err := LoadTree(fsys); err == nil {
		t.Fatal("LoadTree accepted unknown schema, want error")
	}
}
