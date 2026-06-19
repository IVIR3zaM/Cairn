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
	data, err := InitConfig(Directory{Version: strptr("0.1.0")}, nil, nil)
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

// TestInitConfigDirectoriesComeLast: the per-path `directories:` map is rendered after the repo
// baseline so the file reads top-down — repo-wide settings first, per-directory overrides at the
// end — rather than burying the baseline under the overrides.
func TestInitConfigDirectoriesComeLast(t *testing.T) {
	off := false
	data, err := InitConfig(
		Directory{Version: strptr("0.1.0"), Languages: map[string]Language{"dart": {Enabled: true}}},
		map[string]Directory{"pkgs/x": {Enabled: &off}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	li, di := strings.Index(s, "languages:"), strings.Index(s, "directories:")
	if li < 0 || di < 0 {
		t.Fatalf("expected both languages: and directories: in output:\n%s", s)
	}
	if di < li {
		t.Errorf("directories: should be rendered after the baseline, got:\n%s", s)
	}
}

// TestInitConfigOmitsDefaultChangelogFile: a changelog block whose file is the default CHANGELOG.md
// renders the standard alone (no redundant `file:` row), while a non-default file is kept — and
// either way the written bytes still resolve to a usable changelog file via the cascade.
func TestInitConfigOmitsDefaultChangelogFile(t *testing.T) {
	def, err := InitConfig(Directory{Version: strptr("0.1.0"), Changelog: &Changelog{Standard: "dart", File: "CHANGELOG.md"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(def), "file:") {
		t.Errorf("default changelog file should be omitted:\n%s", def)
	}
	if !strings.Contains(string(def), "standard: dart") {
		t.Errorf("changelog standard should still be written:\n%s", def)
	}

	custom, err := InitConfig(Directory{Version: strptr("0.1.0"), Changelog: &Changelog{Standard: "dart", File: "HISTORY.md"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(custom), "file: HISTORY.md") {
		t.Errorf("non-default changelog file should be kept:\n%s", custom)
	}
}

// TestInitConfigAnnotatesBlocks: a comments map writes an explanatory header above each matching
// top-level block, so the generated file documents why each row exists. The bytes still parse.
func TestInitConfigAnnotatesBlocks(t *testing.T) {
	comments := map[string]string{"version": "the project version"}
	data, err := InitConfig(Directory{Version: strptr("0.1.0")}, nil, comments)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# the project version") {
		t.Errorf("comment not written above version:\n%s", data)
	}
	if _, err := LoadTree(fstest.MapFS{"cairn.yaml": {Data: data}}); err != nil {
		t.Fatalf("annotated config does not load: %v", err)
	}
}

// TestInitConfigWritesDetectedCommits: when init determines a non-default commit policy (DCO
// sign-off), it records a complete, reloadable commits block — convention included, so the
// wholesale block resolution keeps validating messages.
func TestInitConfigWritesDetectedCommits(t *testing.T) {
	data, err := InitConfig(Directory{
		Version: strptr("0.1.0"),
		Commits: &Commits{Convention: "conventional", Signoff: true, ValidateHook: true},
	}, nil, nil)
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
