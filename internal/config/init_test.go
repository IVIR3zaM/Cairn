package config

import (
	"strings"
	"testing"
	"testing/fstest"
)

// TestInitConfigRoundTrips pins the init contract: InitConfig renders a minimal schema-2
// cairn.yaml that LoadTree parses back to the chosen baseline version, while every omitted key
// still resolves to the in-code default (hooks/CI/versioning come from the seeded baseline, not
// the file). A drift between the written bytes and what LoadTree accepts would silently produce
// a config `cairn init` can't reload.
func TestInitConfigRoundTrips(t *testing.T) {
	data, err := InitConfig(Directory{Version: strptr("0.1.0")})
	if err != nil {
		t.Fatal(err)
	}

	// The minimal file carries the version but NOT default-only blocks (no commits/languages/
	// versioning/hooks/ci lines) — init never enshrines a default as a discovered fact.
	for _, def := range []string{"commits:", "languages:", "versioning:", "hooks:", "ci:", "verify:"} {
		if strings.Contains(string(data), def) {
			t.Errorf("minimal init config should omit default block %q:\n%s", def, data)
		}
	}

	// Yet the written bytes parse as schema-2 and resolve to the chosen baseline, with the
	// omitted keys still supplying their defaults from the seeded baseline.
	tree, err := LoadTree(fstest.MapFS{"cairn.yaml": {Data: data}})
	if err != nil {
		t.Fatalf("written init config does not load: %v", err)
	}
	root, ok := tree.Resolve(".")
	if !ok {
		t.Fatal("root resolved as pruned")
	}
	if root.Version == nil || *root.Version != "0.1.0" {
		t.Errorf("baseline version = %v, want 0.1.0", root.Version)
	}
	if root.Versioning == nil || *root.Versioning != "semver" {
		t.Errorf("baseline scheme = %v, want default semver", root.Versioning)
	}
	if root.Hooks == nil || len(root.Hooks.PreCommit) == 0 {
		t.Errorf("resolved config missing default hooks")
	}
	if root.CI == nil || root.CI.Provider != "github" {
		t.Errorf("resolved config missing default CI provider")
	}
}

// TestInitConfigWritesDetectedCommits: when init determines a non-default commit policy (DCO
// sign-off), it records a complete, reloadable commits block — convention included, so the
// wholesale block resolution keeps validating messages.
func TestInitConfigWritesDetectedCommits(t *testing.T) {
	data, err := InitConfig(Directory{
		Version: strptr("0.1.0"),
		Commits: &Commits{Convention: "conventional", Signoff: true, ValidateHook: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := LoadTree(fstest.MapFS{"cairn.yaml": {Data: data}})
	if err != nil {
		t.Fatalf("written init config does not load: %v", err)
	}
	root, _ := tree.Resolve(".")
	if root.Commits == nil || !root.Commits.Signoff || root.Commits.Convention != "conventional" {
		t.Errorf("detected commits not recorded/reloaded: %+v", root.Commits)
	}
}
