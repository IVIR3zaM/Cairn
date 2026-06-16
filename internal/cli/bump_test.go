package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/IVIR3zaM/Cairn/internal/config"
	versioning "github.com/IVIR3zaM/Cairn/internal/version"
)

// writeFile is a tiny helper so each test reads as a flat list of fixtures.
func writeFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestRunBumpUpdatesAllSurfaces is the happy path: a level bump must advance the manifest
// in a language dir, rewrite a version-sync doc, advance canonical in cairn.yaml, and print
// a suggested commit/tag — proving the whole bump flow wires together.
func TestRunBumpUpdatesAllSurfaces(t *testing.T) {
	dir := t.TempDir()
	cairn := writeFile(t, dir, "cairn.yaml", "project:\n  canonical_version: \"0.1.0\"\n")
	pkg := writeFile(t, dir, "web/package.json", `{"name":"x","version":"0.1.0"}`)
	readme := writeFile(t, dir, "README.md", "Install x@0.1.0 today.\n")

	cfg := config.Default()
	cfg.Project.CanonicalVersion = "0.1.0"
	cfg.Languages = map[string]config.Language{"javascript": {Dir: "web", Enabled: true}}
	cfg.VersionSync.Files = []config.VersionSyncFile{{Path: "README.md", Patterns: []string{"x@{VERSION}"}}}

	var out bytes.Buffer
	if err := runBump(dir, cfg, "minor", time.Now(), &out, false); err != nil {
		t.Fatalf("runBump: %v", err)
	}

	if got := read(t, pkg); !strings.Contains(got, `"version":"0.2.0"`) {
		t.Errorf("manifest not bumped: %s", got)
	}
	if got := read(t, readme); !strings.Contains(got, "x@0.2.0") {
		t.Errorf("version-sync doc not rewritten: %s", got)
	}
	if got := read(t, cairn); !strings.Contains(got, `canonical_version: "0.2.0"`) {
		t.Errorf("canonical not advanced: %s", got)
	}
	s := out.String()
	for _, want := range []string{"0.1.0 → 0.2.0", `chore(release): 0.2.0`, "git tag v0.2.0"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
}

// TestRunBumpExplicitVersion confirms an explicit X.Y.Z is honored over level math.
func TestRunBumpExplicitVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cairn.yaml", "project:\n  canonical_version: 1.2.3\n")
	cfg := config.Default()
	cfg.Project.CanonicalVersion = "1.2.3"

	var out bytes.Buffer
	if err := runBump(dir, cfg, "2.0.0", time.Now(), &out, false); err != nil {
		t.Fatalf("runBump: %v", err)
	}
	if got := read(t, filepath.Join(dir, "cairn.yaml")); !strings.Contains(got, "canonical_version: 2.0.0") {
		t.Errorf("unquoted canonical not advanced: %s", got)
	}
}

// TestRunBumpGuards covers the two refusals: an unset canonical and a non-increasing bump
// (here an explicit downgrade). Neither should touch the filesystem.
func TestRunBumpGuards(t *testing.T) {
	t.Run("empty canonical", func(t *testing.T) {
		cfg := config.Default() // CanonicalVersion is ""
		if err := runBump(t.TempDir(), cfg, "patch", time.Now(), &bytes.Buffer{}, false); err == nil {
			t.Fatal("want error on unset canonical")
		}
	})
	t.Run("downgrade", func(t *testing.T) {
		cfg := config.Default()
		cfg.Project.CanonicalVersion = "2.0.0"
		err := runBump(t.TempDir(), cfg, "1.0.0", time.Now(), &bytes.Buffer{}, false)
		if err == nil || !strings.Contains(err.Error(), "not greater") {
			t.Fatalf("want downgrade guard, got %v", err)
		}
	})
}

// TestReleaseCommitMessage pins the suggested release subject to the configured commit
// convention so bump's hint never contradicts the style the repo enforces.
func TestReleaseCommitMessage(t *testing.T) {
	cases := map[string]string{
		"conventional": "chore(release): 1.2.3",
		"gitmoji":      "🔖 Release 1.2.3",
		"none":         "Release 1.2.3",
		"":             "chore(release): 1.2.3", // unset → safe conventional default
	}
	for conv, want := range cases {
		if got := releaseCommitMessage(conv, "1.2.3"); got != want {
			t.Errorf("releaseCommitMessage(%q) = %q, want %q", conv, got, want)
		}
	}
}

// TestJumpKind pins the classification that drives the wizard's explanation and its
// downgrade safeguard — ordering wins over component math, so a lower version is a
// downgrade even when a component happens to be larger.
func TestJumpKind(t *testing.T) {
	v := func(s string) versioning.Version {
		ver, err := versioning.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return ver
	}
	cases := []struct{ cur, next, want string }{
		{"1.2.3", "2.0.0", "major"},
		{"1.2.3", "1.3.0", "minor"},
		{"1.2.3", "1.2.4", "patch"},
		{"1.2.3", "1.2.3", "same"},
		{"1.2.3", "1.0.9", "downgrade"},
	}
	for _, c := range cases {
		if got := jumpKind(v(c.cur), v(c.next)); got != c.want {
			t.Errorf("jumpKind(%s→%s) = %q, want %q", c.cur, c.next, got, c.want)
		}
	}
}

// TestWizardAppliesOnConfirm walks the happy path: pick "minor", confirm, and the chosen
// version lands in cairn.yaml with the wizard's "Bumped" banner — proving the interactive
// front-end reaches the same applyBump core as the direct path.
func TestWizardAppliesOnConfirm(t *testing.T) {
	dir := t.TempDir()
	cairn := writeFile(t, dir, "cairn.yaml", "project:\n  canonical_version: \"1.2.0\"\n")
	cfg := config.Default()
	cfg.Project.CanonicalVersion = "1.2.0"

	var out bytes.Buffer
	if err := runBumpWizard(dir, cfg, strings.NewReader("2\ny\n"), &out, false); err != nil {
		t.Fatalf("wizard: %v", err)
	}
	if got := read(t, cairn); !strings.Contains(got, `canonical_version: "1.3.0"`) {
		t.Errorf("canonical not advanced: %s", got)
	}
	if s := out.String(); !strings.Contains(s, "Bumped 1.2.0 → 1.3.0") {
		t.Errorf("missing banner:\n%s", s)
	}
}

// TestWizardQuitLeavesFilesUntouched confirms 'q' aborts cleanly without writing.
func TestWizardQuitLeavesFilesUntouched(t *testing.T) {
	dir := t.TempDir()
	cairn := writeFile(t, dir, "cairn.yaml", "project:\n  canonical_version: \"1.2.0\"\n")
	cfg := config.Default()
	cfg.Project.CanonicalVersion = "1.2.0"

	var out bytes.Buffer
	if err := runBumpWizard(dir, cfg, strings.NewReader("q\n"), &out, false); err != nil {
		t.Fatalf("wizard: %v", err)
	}
	if got := read(t, cairn); !strings.Contains(got, "1.2.0") || strings.Contains(got, "1.3.0") {
		t.Errorf("quit should not change the file: %s", got)
	}
	if !strings.Contains(out.String(), "aborted") {
		t.Errorf("expected an abort message")
	}
}

// TestWizardDowngradeNeedsDoubleConfirm proves a downgrade is allowed in the wizard (unlike
// the direct path's guard) but only after both confirmations are given — feeding a custom
// lower version and two yeses applies it.
func TestWizardDowngradeNeedsDoubleConfirm(t *testing.T) {
	dir := t.TempDir()
	cairn := writeFile(t, dir, "cairn.yaml", "project:\n  canonical_version: \"2.0.0\"\n")
	cfg := config.Default()
	cfg.Project.CanonicalVersion = "2.0.0"

	var out bytes.Buffer
	// choice 4 (custom) → 1.0.0 → downgrade confirm → second confirm → apply confirm.
	if err := runBumpWizard(dir, cfg, strings.NewReader("4\n1.0.0\ny\ny\ny\n"), &out, false); err != nil {
		t.Fatalf("wizard: %v", err)
	}
	if got := read(t, cairn); !strings.Contains(got, `canonical_version: "1.0.0"`) {
		t.Errorf("confirmed downgrade not applied: %s", got)
	}
	if !strings.Contains(out.String(), "DOWNGRADE") {
		t.Errorf("expected a loud downgrade warning")
	}
}

// TestComputeNextCalVer checks that for the calver scheme a level argument yields the
// date-based next version rather than semver math.
func TestComputeNextCalVer(t *testing.T) {
	cfg := config.Default()
	cfg.Project.Versioning = "calver"
	cfg.Project.CanonicalVersion = "2024.1.0"
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	_, next, err := computeNext(cfg, "patch", now)
	if err != nil {
		t.Fatal(err)
	}
	if next.String() != "2026.6.0" {
		t.Errorf("calver next = %s, want 2026.6.0", next)
	}
}
