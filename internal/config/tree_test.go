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

// A partial top-level block must merge onto the in-code defaults, not wipe the stages it
// omits: writing only `verify: { strict: true }` keeps format/lint/test enabled (regression —
// the schema-2 baseline used to start from zero, silently disabling every language stage).
func TestResolve_PartialVerifyKeepsDefaultStages(t *testing.T) {
	fsys := fstest.MapFS{
		"cairn.yaml": &fstest.MapFile{Data: []byte(`
schema: "2"
version: "1.0.0"
verify: { strict: true }
`)},
		"packages/lib/pubspec.yaml": &fstest.MapFile{Data: []byte("name: lib\n")},
	}
	tr, err := LoadTree(fsys)
	if err != nil {
		t.Fatalf("LoadTree: %v", err)
	}
	d, _ := tr.Resolve("packages/lib")
	v := d.VerifyOrDefault()
	if !v.Strict {
		t.Error("strict = false, want true (the one field the file set)")
	}
	if !v.Format.Enabled || !v.Lint.Enabled || !v.Test.Enabled {
		t.Errorf("default stages disabled by a partial verify block: format=%v lint=%v test=%v",
			v.Format.Enabled, v.Lint.Enabled, v.Test.Enabled)
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

// StrictFor resolves per-directory strictness through the cascade so the CLI never re-derives
// precedence: the repo baseline default applies where nothing overrides, a directory override
// raises it, and a per-language strict override beats the directory's verify.strict default.
func TestResolve_StrictForCascade(t *testing.T) {
	fsys := fstest.MapFS{
		"cairn.yaml": &fstest.MapFile{Data: []byte(`
schema: "2"
version: "1.0.0"
verify: { strict: false }
directories:
  pkg_a: { verify: { strict: true } }
  pkg_b:
    verify: { strict: true }
    languages: { go: { strict: false } }
`)},
	}
	tr, err := LoadTree(fsys)
	if err != nil {
		t.Fatalf("LoadTree: %v", err)
	}
	root, _ := tr.Resolve(".")
	if root.StrictFor("go") {
		t.Error("root StrictFor(go) = true, want false (baseline default)")
	}
	a, _ := tr.Resolve("pkg_a")
	if !a.StrictFor("go") {
		t.Error("pkg_a StrictFor(go) = false, want true (directory verify.strict override)")
	}
	b, _ := tr.Resolve("pkg_b")
	if b.StrictFor("go") {
		t.Error("pkg_b StrictFor(go) = true, want false (per-language strict beats directory default)")
	}
	if !b.StrictFor("rust") {
		t.Error("pkg_b StrictFor(rust) = false, want true (no per-language override → directory default)")
	}
}

// Independent lists exactly the directories that declare their own version — whether via a root
// directories.<path> entry or via the directory's own cairn.yaml — and excludes ones that only
// inherit the baseline and any pruned subtree, sorted. It is bump's schema-2 replacement for
// project.packages, so it must reflect the cascade's "own version ⇒ independent" rule.
func TestTree_Independent(t *testing.T) {
	fsys := fstest.MapFS{
		"cairn.yaml": &fstest.MapFile{Data: []byte(`
schema: "2"
version: "1.0.0"
directories:
  pkg_b: { version: "2.0.0" }
  pkg_c: { verify: { strict: true } }
  vendored: { enabled: false }
`)},
		// pkg_a is independent via its own file; pkg_c only overrides verify (inherits version).
		"pkg_a/cairn.yaml": &fstest.MapFile{Data: []byte("version: \"0.5.0\"\n")},
		"pkg_c/cairn.yaml": &fstest.MapFile{Data: []byte("commits: { signoff: true }\n")},
		// A disabled subtree declaring a version must still be excluded (never read / pruned).
		"vendored/cairn.yaml": &fstest.MapFile{Data: []byte("version: \"9.9.9\"\n")},
	}
	tr, err := LoadTree(fsys)
	if err != nil {
		t.Fatalf("LoadTree: %v", err)
	}
	got := tr.Independent()
	want := []string{"pkg_a", "pkg_b"}
	if len(got) != len(want) {
		t.Fatalf("Independent() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Independent() = %v, want %v", got, want)
		}
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
