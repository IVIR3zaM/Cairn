package version

import (
	"testing"
	"testing/fstest"

	"github.com/IVIR3zaM/Cairn/internal/config"
)

// treeResolver builds a Tree-backed Resolver from a schema-2 cairn.yaml body — the test
// constructor since the per-directory Tree (not a project list) is the only thing that drives
// resolution now.
func treeResolver(tb testing.TB, body string) *Resolver {
	tb.Helper()
	tree, err := config.LoadTree(fstest.MapFS{
		"cairn.yaml": &fstest.MapFile{Data: []byte(body)},
	})
	if err != nil {
		tb.Fatalf("LoadTree: %v", err)
	}
	return NewResolverFromTree(tree)
}

// A Tree-backed resolver answers ForDir purely from config's cascade: the repo baseline (a
// top-level version, lockstep) governs unmatched units, a directories.<path> override carries
// its own version (scheme inherited), and a nested entry layers its own scheme — proving the
// precedence lives in config, not the resolver.
func TestResolverFromTree(t *testing.T) {
	fsys := fstest.MapFS{
		"cairn.yaml": &fstest.MapFile{Data: []byte(`schema: "2"
version: "1.0.0"
versioning: semver
directories:
  packages/api:
    version: "2.3.0"
  packages/api/internal:
    version: "0.9.0"
    versioning: calver
`)},
	}
	tree, err := config.LoadTree(fsys)
	if err != nil {
		t.Fatalf("LoadTree: %v", err)
	}
	r := NewResolverFromTree(tree)

	tests := []struct {
		name string
		dir  string
		want Target
	}{
		{
			"unmatched unit gets the repo baseline (lockstep)",
			"tools/cli",
			Target{Version: "1.0.0", Versioning: "semver"},
		},
		{
			"directory override carries its version, inherits the baseline scheme",
			"packages/api",
			Target{Version: "2.3.0", Versioning: "semver"},
		},
		{
			"dir under an override inherits that override, not the baseline",
			"packages/api/handlers",
			Target{Version: "2.3.0", Versioning: "semver"},
		},
		{
			"nested override layers its own version and scheme (nearest wins)",
			"packages/api/internal",
			Target{Version: "0.9.0", Versioning: "calver"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.ForDir(tt.dir); got != tt.want {
				t.Errorf("ForDir(%q) = %+v, want %+v", tt.dir, got, tt.want)
			}
		})
	}
}

// With no per-directory override the resolver is the lockstep case: every unit gets the repo
// baseline version and scheme.
func TestResolverNoPackagesIsLockstep(t *testing.T) {
	r := treeResolver(t, "schema: \"2\"\nversion: \"3.1.4\"\nversioning: calver\n")
	want := Target{Version: "3.1.4", Versioning: "calver"}
	for _, dir := range []string{".", "packages/api", "deep/nested/unit"} {
		if got := r.ForDir(dir); got != want {
			t.Errorf("ForDir(%q) = %+v, want %+v", dir, got, want)
		}
	}
}
