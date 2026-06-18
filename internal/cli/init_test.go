package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunInitYesWritesConfigHooksAndCI is the headline acceptance: `cairn init --yes` in a fresh
// repo writes a valid schema-2 cairn.yaml listing the detected language, installs runnable git
// hooks, generates a CI workflow, and is non-destructive on re-run (an existing config is kept).
func TestRunInitYesWritesConfigHooksAndCI(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	// a Go marker so detection finds a language to enable.
	writeFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.23\n")

	var out bytes.Buffer
	if err := runInit(dir, &out); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	// cairn.yaml loads via the real config cascade and resolves to the seeded baseline, with the
	// detected language recorded as an enabled, editable scaffold.
	root, ok := loadTree(t, dir).Resolve(".")
	if !ok {
		t.Fatal("root resolved as pruned")
	}
	if root.Version == nil {
		t.Errorf("init did not write a baseline version: %+v", root)
	}
	if l, ok := root.Languages["go"]; !ok || !l.Enabled {
		t.Errorf("init did not record the detected go language: %+v", root.Languages)
	}

	// hooks installed and executable; CI workflow generated.
	hook := filepath.Join(dir, ".cairn/hooks/pre-commit")
	if info, err := os.Stat(hook); err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("pre-commit hook missing or not executable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".github/workflows/cairn.yml")); err != nil {
		t.Fatalf("CI workflow not generated: %v", err)
	}

	s := out.String()
	for _, want := range []string{"wrote cairn.yaml", "installed git hooks", "generated CI", "Next steps"} {
		if !strings.Contains(s, want) {
			t.Errorf("init output missing %q:\n%s", want, s)
		}
	}

	// Non-destructive/idempotent: a second run keeps the existing config byte-for-byte.
	before := read(t, filepath.Join(dir, "cairn.yaml"))
	var out2 bytes.Buffer
	if err := runInit(dir, &out2); err != nil {
		t.Fatalf("second runInit: %v", err)
	}
	if after := read(t, filepath.Join(dir, "cairn.yaml")); after != before {
		t.Errorf("init clobbered existing cairn.yaml")
	}
	if !strings.Contains(out2.String(), "cairn.yaml exists") {
		t.Errorf("second run did not report keeping the existing config:\n%s", out2.String())
	}
}

// TestInitSeedsVersionFromManifest: init must seed cairn.yaml's version from the real project
// manifest (here a package.json's "version"), so `cairn verify` agrees out of the box instead of
// drifting against a placeholder.
func TestInitSeedsVersionFromManifest(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	writeFile(t, dir, "package.json", `{"name":"web","version":"3.4.5"}`)

	var out bytes.Buffer
	if err := runInit(dir, &out); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	root, ok := loadTree(t, dir).Resolve(".")
	if !ok {
		t.Fatal("root resolved as pruned")
	}
	if root.Version == nil || *root.Version != "3.4.5" {
		t.Errorf("init did not seed version from package.json: got %v, want 3.4.5", root.Version)
	}
}

// TestInitDefaultsVersionWhenNoManifest: with no version-bearing manifest, init falls back to the
// 0.1.0 placeholder rather than failing.
func TestInitDefaultsVersionWhenNoManifest(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	writeFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.23\n")

	var out bytes.Buffer
	if err := runInit(dir, &out); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	root, ok := loadTree(t, dir).Resolve(".")
	if !ok {
		t.Fatal("root resolved as pruned")
	}
	if root.Version == nil || *root.Version != "0.1.0" {
		t.Errorf("init fallback version = %v, want 0.1.0", root.Version)
	}
}

// TestInitDetectsVersionSyncFromReadme: init scans the README for the project's real version and
// wires the surrounding tokens up as version_sync patterns, so `cairn verify` keeps the docs
// honest from the first run rather than waiting for the user to hand-write patterns.
func TestInitDetectsVersionSyncFromReadme(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	writeFile(t, dir, "package.json", `{"name":"web","version":"3.4.5"}`)
	writeFile(t, dir, "README.md", "# web\n\n![ver](https://img.shields.io/badge/version-3.4.5-blue)\n\nInstall with `web@3.4.5`. Released 3.4.5 today.\n")

	var out bytes.Buffer
	if err := runInit(dir, &out); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	root, _ := loadTree(t, dir).Resolve(".")
	if root.VersionSync == nil || len(root.VersionSync.Files) != 1 {
		t.Fatalf("init did not wire version_sync from README: %+v", root.VersionSync)
	}
	f := root.VersionSync.Files[0]
	if f.Path != "README.md" {
		t.Errorf("version_sync path = %q, want README.md", f.Path)
	}
	pats := strings.Join(f.Patterns, "\n")
	for _, want := range []string{"version-{VERSION}-blue", "web@{VERSION}"} {
		if !strings.Contains(pats, want) {
			t.Errorf("missing distinctive pattern %q in:\n%s", want, pats)
		}
	}
	// The bare prose "3.4.5" is not distinctive enough to become a pattern.
	for _, p := range f.Patterns {
		if p == "{VERSION}" {
			t.Errorf("init wrote an over-generic bare pattern: %q", p)
		}
	}
	if !strings.Contains(out.String(), "version_sync:") {
		t.Errorf("init output did not report the version_sync detection:\n%s", out.String())
	}
}

// TestInitDetectsSignoffFromHistory: init must decide commits.signoff from the repo's history,
// not write a blind default. A history where every commit is signed off enables sign-off; an
// unsigned history leaves the commits block omitted (rides the default-off policy).
func TestInitDetectsSignoffFromHistory(t *testing.T) {
	signedDir := t.TempDir()
	gitInitWithUser(t, signedDir)
	writeFile(t, signedDir, "go.mod", "module example.com/x\n\ngo 1.23\n")
	for _, msg := range []string{"feat: one", "fix: two"} {
		commitWithSignoff(t, signedDir, msg, true)
	}
	var out bytes.Buffer
	if err := runInit(signedDir, &out); err != nil {
		t.Fatalf("runInit (signed): %v", err)
	}
	root, _ := loadTree(t, signedDir).Resolve(".")
	if root.Commits == nil || !root.Commits.Signoff {
		t.Errorf("signed-off history should enable commits.signoff, got %+v", root.Commits)
	}

	unsignedDir := t.TempDir()
	gitInitWithUser(t, unsignedDir)
	writeFile(t, unsignedDir, "go.mod", "module example.com/x\n\ngo 1.23\n")
	for _, msg := range []string{"feat: one", "fix: two"} {
		commitWithSignoff(t, unsignedDir, msg, false)
	}
	var out2 bytes.Buffer
	if err := runInit(unsignedDir, &out2); err != nil {
		t.Fatalf("runInit (unsigned): %v", err)
	}
	// The default-off policy is not enshrined: no commits block in the written file.
	if strings.Contains(read(t, filepath.Join(unsignedDir, "cairn.yaml")), "commits:") {
		t.Errorf("unsigned history should not write a commits block:\n%s", read(t, filepath.Join(unsignedDir, "cairn.yaml")))
	}
}

// gitInitWithUser initializes a repo and sets a committer identity so commits can be created.
func gitInitWithUser(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "dev@example.com"}, {"config", "user.name", "Dev"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

// commitWithSignoff makes an empty commit, optionally adding a DCO sign-off trailer.
func commitWithSignoff(t *testing.T, dir, msg string, signoff bool) {
	t.Helper()
	args := []string{"-C", dir, "commit", "--allow-empty", "-m", msg}
	if signoff {
		args = append(args, "-s")
	}
	if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
		t.Fatalf("git commit %q: %v: %s", msg, err, out)
	}
}

// TestInitWithoutYesPointsAtFlag: the bare command (no TTY wizard yet) errors actionably toward
// --yes rather than doing nothing — the 10b-i interactive gate until 10b-ii lands.
func TestInitWithoutYesPointsAtFlag(t *testing.T) {
	cmd := newRootCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"init"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error from `init` without --yes")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Errorf("error does not point at --yes: %v", err)
	}
}
