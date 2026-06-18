package version

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/IVIR3zaM/Cairn/internal/config"
)

func TestCheck(t *testing.T) {
	fsys := fstest.MapFS{
		"README.md": {Data: []byte("install mylib:0.1.0\nbadge version-0.0.9 here")},
	}
	files := []config.VersionSyncFile{
		{Path: "README.md", Patterns: []string{"mylib:{VERSION}", "version-{VERSION}"}},
	}

	drifts, err := Check(fsys, lockstep("0.1.0"), files)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(drifts) != 1 {
		t.Fatalf("want 1 drift, got %d: %v", len(drifts), drifts)
	}
	d := drifts[0]
	if d.Found != "0.0.9" || d.Want != "0.1.0" || d.Pattern != "version-{VERSION}" {
		t.Errorf("unexpected drift: %+v", d)
	}
}

// lockstep is a Resolver where every file/unit resolves to one version — the degenerate,
// baseline-only case the version_sync tests exercise. Built from a schema-2 Tree carrying just
// the repo baseline version (omitted when empty, so lockstep("") models "no version set").
func lockstep(canonical string) *Resolver {
	body := "schema: \"2\"\nversioning: semver\n"
	if canonical != "" {
		body += "version: \"" + canonical + "\"\n"
	}
	tree, err := config.LoadTree(fstest.MapFS{
		"cairn.yaml": &fstest.MapFile{Data: []byte(body)},
	})
	if err != nil {
		panic(err)
	}
	return NewResolverFromTree(tree)
}

// A version_sync doc under an independently-versioned package is checked against *that*
// package's version, not the repo-wide canonical: the resolver maps the file by its dir.
func TestCheckPerPackage(t *testing.T) {
	fsys := fstest.MapFS{
		"README.md":              {Data: []byte("repo at 1.0.0")},
		"packages/api/README.md": {Data: []byte("api at 1.0.0")}, // drift: api is on 2.5.0
	}
	files := []config.VersionSyncFile{
		{Path: "README.md", Patterns: []string{"repo at {VERSION}"}},
		{Path: "packages/api/README.md", Patterns: []string{"api at {VERSION}"}},
	}
	res := treeResolver(t, `schema: "2"
version: "1.0.0"
versioning: semver
directories:
  packages/api: { version: "2.5.0" }
`)
	drifts, err := Check(fsys, res, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(drifts) != 1 || drifts[0].Path != "packages/api/README.md" || drifts[0].Want != "2.5.0" {
		t.Fatalf("want only the api doc to drift against its own 2.5.0, got %+v", drifts)
	}
}

func TestCheckPatternNotFound(t *testing.T) {
	fsys := fstest.MapFS{"README.md": {Data: []byte("no version here")}}
	files := []config.VersionSyncFile{
		{Path: "README.md", Patterns: []string{"mylib:{VERSION}"}},
	}

	drifts, err := Check(fsys, lockstep("1.0.0"), files)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(drifts) != 1 || drifts[0].Found != "" {
		t.Fatalf("want one not-found drift, got %v", drifts)
	}
}

func TestCheckHonestIsNoDrift(t *testing.T) {
	fsys := fstest.MapFS{"README.md": {Data: []byte("v1.2.3 and again 1.2.3")}}
	files := []config.VersionSyncFile{
		{Path: "README.md", Patterns: []string{"{VERSION}"}},
	}

	drifts, err := Check(fsys, lockstep("1.2.3"), files)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(drifts) != 0 {
		t.Errorf("honest docs should not drift, got %v", drifts)
	}
}

func TestCheckNoConfigIsNoop(t *testing.T) {
	// No resolver or no files: the check costs nothing and reads nothing.
	if d, err := Check(nil, nil, nil); err != nil || d != nil {
		t.Errorf("nil resolver: got %v, %v", d, err)
	}
	if d, err := Check(nil, lockstep("1.0.0"), nil); err != nil || d != nil {
		t.Errorf("no files: got %v, %v", d, err)
	}
}

func TestRewriteMakesDocHonest(t *testing.T) {
	dir := t.TempDir()
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("install mylib:0.0.9\nbadge version-0.0.9 ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []config.VersionSyncFile{
		{Path: "README.md", Patterns: []string{"mylib:{VERSION}", "version-{VERSION}"}},
	}

	changed, err := Rewrite(dir, lockstep("0.1.0"), files)
	if err != nil {
		t.Fatalf("Rewrite: %v", err)
	}
	if len(changed) != 1 || changed[0] != "README.md" {
		t.Fatalf("changed = %v, want [README.md]", changed)
	}
	// The rewritten doc is now honest, and a re-run is a no-op (idempotent).
	if drifts, _ := Check(os.DirFS(dir), lockstep("0.1.0"), files); len(drifts) != 0 {
		t.Errorf("doc still drifts after rewrite: %v", drifts)
	}
	if again, _ := Rewrite(dir, lockstep("0.1.0"), files); len(again) != 0 {
		t.Errorf("second rewrite should change nothing, got %v", again)
	}
}

func TestCheckBadCanonical(t *testing.T) {
	files := []config.VersionSyncFile{{Path: "README.md", Patterns: []string{"{VERSION}"}}}
	if _, err := Check(fstest.MapFS{}, lockstep("nope"), files); err == nil {
		t.Error("malformed canonical version should error")
	}
}
