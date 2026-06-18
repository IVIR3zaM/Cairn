package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/IVIR3zaM/Cairn/internal/config"
	versioning "github.com/IVIR3zaM/Cairn/internal/version"
)

// gitCommit stages the given pathspecs and records a commit with msg in dir, so a test can build
// the path-scoped history per-package inference reads. It configures an isolated identity and
// disables signing so it runs the same everywhere (CI included).
func gitCommit(t *testing.T, dir, msg string, paths ...string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); os.IsNotExist(err) {
		run("init")
		run("config", "commit.gpgsign", "false")
	}
	run(append([]string{"add", "--"}, paths...)...)
	run("commit", "-m", msg)
}

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

// loadTree resolves the schema-2 per-directory config Tree from a temp repo, the same way the
// bump command does — so tests exercise the real config cascade instead of a hand-built Config.
func loadTree(t *testing.T, dir string) *config.Tree {
	t.Helper()
	tree, err := config.LoadTree(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// TestRunBumpUpdatesAllSurfaces is the happy path: a level bump must advance the manifest
// in a language dir, rewrite a version-sync doc, advance the baseline version in cairn.yaml, and
// print a suggested commit/tag — proving the whole bump flow wires together.
func TestRunBumpUpdatesAllSurfaces(t *testing.T) {
	dir := t.TempDir()
	cairn := writeFile(t, dir, "cairn.yaml",
		"version: \"0.1.0\"\nversion_sync:\n  files:\n    - path: README.md\n      patterns:\n        - \"x@{VERSION}\"\n")
	pkg := writeFile(t, dir, "web/package.json", `{"name":"x","version":"0.1.0"}`)
	readme := writeFile(t, dir, "README.md", "Install x@0.1.0 today.\n")

	var out bytes.Buffer
	if err := runBump(dir, loadTree(t, dir), "minor", time.Now(), &out, false, false); err != nil {
		t.Fatalf("runBump: %v", err)
	}

	if got := read(t, pkg); !strings.Contains(got, `"version":"0.2.0"`) {
		t.Errorf("manifest not bumped: %s", got)
	}
	if got := read(t, readme); !strings.Contains(got, "x@0.2.0") {
		t.Errorf("version-sync doc not rewritten: %s", got)
	}
	if got := read(t, cairn); !strings.Contains(got, `version: "0.2.0"`) {
		t.Errorf("baseline version not advanced: %s", got)
	}
	s := out.String()
	for _, want := range []string{"0.1.0 → 0.2.0", `chore(release): 0.2.0`, "git tag v0.2.0"} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
}

// TestRunBumpPromotesChangelog proves bump wires the Changelog context in: a level bump
// promotes the configured CHANGELOG's [Unreleased] entries into a dated release.
func TestRunBumpPromotesChangelog(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cairn.yaml", "version: \"0.1.0\"\n")
	cl := writeFile(t, dir, "CHANGELOG.md", "# Changelog\n\n## [Unreleased]\n\n### Added\n- A new thing.\n")

	date := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	var out bytes.Buffer
	if err := runBump(dir, loadTree(t, dir), "minor", date, &out, false, false); err != nil {
		t.Fatalf("runBump: %v", err)
	}
	if got := read(t, cl); !strings.Contains(got, "## [0.2.0] - 2024-06-01") || !strings.Contains(got, "- A new thing.") {
		t.Errorf("changelog not promoted: %s", got)
	}
}

// TestRunBumpEmptyChangelogFails proves an empty [Unreleased] section blocks the bump entirely:
// it errors and nothing is written (the manifest stays at the old version), so a notes-less
// release can never be cut.
func TestRunBumpEmptyChangelogFails(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cairn.yaml", "version: \"0.1.0\"\n")
	writeFile(t, dir, "CHANGELOG.md", "# Changelog\n\n## [Unreleased]\n\n## [0.1.0] - 2024-01-01\n\n### Added\n- First.\n")
	pkg := writeFile(t, dir, "web/package.json", `{"name":"x","version":"0.1.0"}`)

	var out bytes.Buffer
	err := runBump(dir, loadTree(t, dir), "minor", time.Now(), &out, false, false)
	if err == nil || !strings.Contains(err.Error(), "[Unreleased] section is empty") {
		t.Fatalf("expected empty-changelog refusal, got err=%v", err)
	}
	if got := read(t, pkg); !strings.Contains(got, `"version":"0.1.0"`) {
		t.Errorf("manifest must be untouched when the bump is refused: %s", got)
	}
}

// TestRunBumpPromotesPerPackageChangelogs proves the multi-package edge case: with
// changelog.packages set, a repo-wide bump promotes the root changelog (Keep a Changelog style)
// and each detected package's own changelog (plain dart style), each to the bumped version, and
// an empty [Unreleased] in any one of them fails the whole bump.
func TestRunBumpPromotesPerPackageChangelogs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cairn.yaml",
		"version: \"0.1.0\"\nchangelog:\n  standard: keepachangelog\n  file: CHANGELOG.md\n  packages:\n    standard: dart\n    file: CHANGELOG.md\n")
	// A Dart pub workspace with two members, each keeping its own changelog.
	writeFile(t, dir, "pubspec.yaml", "name: ws\nworkspace:\n  - pkg_a\n  - pkg_b\n")
	writeFile(t, dir, "pkg_a/pubspec.yaml", "name: pkg_a\nversion: 0.1.0\nresolution: workspace\n")
	writeFile(t, dir, "pkg_b/pubspec.yaml", "name: pkg_b\nversion: 0.1.0\nresolution: workspace\n")
	root := writeFile(t, dir, "CHANGELOG.md", "# Changelog\n\n## [Unreleased]\n\n### Added\n- Root note.\n")
	clA := writeFile(t, dir, "pkg_a/CHANGELOG.md", "# Changelog\n\n## Unreleased\n\n- Pkg A note.\n")
	clB := writeFile(t, dir, "pkg_b/CHANGELOG.md", "# Changelog\n\n## Unreleased\n\n- Pkg B note.\n")

	date := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	var out bytes.Buffer
	if err := runBump(dir, loadTree(t, dir), "minor", date, &out, false, false); err != nil {
		t.Fatalf("runBump: %v", err)
	}
	if got := read(t, root); !strings.Contains(got, "## [0.2.0] - 2024-06-01") {
		t.Errorf("root changelog not promoted (Keep a Changelog style): %s", got)
	}
	for name, cl := range map[string]string{"pkg_a": clA, "pkg_b": clB} {
		if got := read(t, cl); !strings.Contains(got, "## 0.2.0 - 2024-06-01") {
			t.Errorf("%s changelog not promoted (dart style): %s", name, got)
		}
	}

	// Emptying one package's Unreleased fails the whole bump.
	writeFile(t, dir, "pkg_b/CHANGELOG.md", "# Changelog\n\n## Unreleased\n\n## 0.2.0 - 2024-06-01\n\n- Pkg B note.\n")
	out.Reset()
	err := runBump(dir, loadTree(t, dir), "minor", date, &out, false, false)
	if err == nil || !strings.Contains(err.Error(), "pkg_b/CHANGELOG.md") {
		t.Fatalf("expected refusal naming pkg_b's empty changelog, got err=%v", err)
	}
}

// TestRunPackageBumpPromotesOnlyItsChangelog proves the "bump one package" case: with
// independently-versioned directories, `bump <pkg>` promotes only that package's changelog (and
// leaves the root changelog and the other package's changelog alone).
func TestRunPackageBumpPromotesOnlyItsChangelog(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cairn.yaml",
		"version: \"0.0.0\"\nchangelog:\n  standard: keepachangelog\n  file: CHANGELOG.md\n  packages:\n    standard: dart\n    file: CHANGELOG.md\ndirectories:\n  pkg_a:\n    version: \"1.0.0\"\n  pkg_b:\n    version: \"2.0.0\"\n")
	writeFile(t, dir, "pkg_a/pubspec.yaml", "name: pkg_a\nversion: 1.0.0\n")
	writeFile(t, dir, "pkg_b/pubspec.yaml", "name: pkg_b\nversion: 2.0.0\n")
	root := writeFile(t, dir, "CHANGELOG.md", "# Changelog\n\n## [Unreleased]\n\n### Added\n- Root note.\n")
	clA := writeFile(t, dir, "pkg_a/CHANGELOG.md", "# Changelog\n\n## Unreleased\n\n- Pkg A note.\n")
	clB := writeFile(t, dir, "pkg_b/CHANGELOG.md", "# Changelog\n\n## Unreleased\n\n- Pkg B note.\n")

	date := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	var out bytes.Buffer
	if err := runPackageBump(dir, loadTree(t, dir), "pkg_a", "minor", date, &out, false, false); err != nil {
		t.Fatalf("runPackageBump: %v", err)
	}
	if got := read(t, clA); !strings.Contains(got, "## 1.1.0 - 2024-06-01") {
		t.Errorf("pkg_a changelog not promoted: %s", got)
	}
	if got := read(t, clB); strings.Contains(got, "## 2") || !strings.Contains(got, "## Unreleased\n\n- Pkg B note.") {
		t.Errorf("pkg_b changelog must be untouched: %s", got)
	}
	if got := read(t, root); !strings.Contains(got, "## [Unreleased]\n\n### Added\n- Root note.") {
		t.Errorf("root changelog must be untouched on a package-scoped bump: %s", got)
	}
}

// TestRunBumpAutoDiscoversManifests proves the language-owned discovery path: with no language
// config at all, bump still finds and bumps each manifest purely from detection — a Rust
// crate in a sub-dir, and the members of a Dart pub workspace whose 6e writer moves each
// member's `version:` and the member-to-member `^` interdependency in lockstep — no config.
func TestRunBumpAutoDiscoversManifests(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cairn.yaml", "version: \"0.1.0\"\n")
	crate := writeFile(t, dir, "engine/Cargo.toml", "[package]\nname = \"x\"\nversion = \"0.1.0\"\n")
	// A Dart pub workspace: the root aggregates two members; pkg_b depends on pkg_a.
	writeFile(t, dir, "pubspec.yaml", "name: ws\nworkspace:\n  - pkg_a\n  - pkg_b\n")
	pkgA := writeFile(t, dir, "pkg_a/pubspec.yaml", "name: pkg_a\nversion: 0.1.0\nresolution: workspace\n")
	pkgB := writeFile(t, dir, "pkg_b/pubspec.yaml", "name: pkg_b\nversion: 0.1.0\nresolution: workspace\n\ndependencies:\n  pkg_a: ^0.1.0\n")

	var out bytes.Buffer
	if err := runBump(dir, loadTree(t, dir), "minor", time.Now(), &out, false, false); err != nil {
		t.Fatalf("runBump: %v", err)
	}
	if got := read(t, crate); !strings.Contains(got, `version = "0.2.0"`) {
		t.Errorf("auto-discovered Cargo.toml not bumped: %s", got)
	}
	if got := read(t, pkgA); !strings.Contains(got, "version: 0.2.0") {
		t.Errorf("workspace member pkg_a version not bumped: %s", got)
	}
	if got := read(t, pkgB); !strings.Contains(got, "version: 0.2.0") || !strings.Contains(got, "pkg_a: ^0.2.0") {
		t.Errorf("workspace member pkg_b version/interdependency not bumped in lockstep: %s", got)
	}
}

// TestRunPackageBump is the per-package acceptance: in a mixed-language monorepo whose
// directories version independently, `bump <pkg>` advances exactly one declared directory — its
// own manifest, its dependents' interdependency constraints, and its directories.<path>.version
// entry — while every other package (a Dart sibling on a different version, a Rust crate) and the
// repo baseline stay put.
func TestRunPackageBump(t *testing.T) {
	dir := t.TempDir()
	cairn := writeFile(t, dir, "cairn.yaml",
		"version: \"9.9.9\"\ndirectories:\n  pkg_a:\n    version: \"1.0.0\"\n  pkg_b:\n    version: \"2.0.0\"\n  rust_pkg:\n    version: \"3.0.0\"\n")
	// A Dart pub workspace: pkg_b depends on its sibling pkg_a. A Rust crate versions on its own.
	writeFile(t, dir, "pubspec.yaml", "name: ws\nworkspace:\n  - pkg_a\n  - pkg_b\n")
	pkgA := writeFile(t, dir, "pkg_a/pubspec.yaml", "name: pkg_a\nversion: 1.0.0\nresolution: workspace\n")
	pkgB := writeFile(t, dir, "pkg_b/pubspec.yaml", "name: pkg_b\nversion: 2.0.0\nresolution: workspace\n\ndependencies:\n  pkg_a: ^1.0.0\n")
	crate := writeFile(t, dir, "rust_pkg/Cargo.toml", "[package]\nname = \"rust_pkg\"\nversion = \"3.0.0\"\n")

	var out bytes.Buffer
	if err := runPackageBump(dir, loadTree(t, dir), "pkg_a", "minor", time.Now(), &out, false, false); err != nil {
		t.Fatalf("runPackageBump: %v", err)
	}

	if got := read(t, pkgA); !strings.Contains(got, "version: 1.1.0") {
		t.Errorf("pkg_a not advanced: %s", got)
	}
	if got := read(t, pkgB); !strings.Contains(got, "pkg_a: ^1.1.0") {
		t.Errorf("dependent constraint not reconciled: %s", got)
	}
	if got := read(t, pkgB); !strings.Contains(got, "version: 2.0.0") {
		t.Errorf("pkg_b's own version should be untouched: %s", got)
	}
	if got := read(t, crate); !strings.Contains(got, `version = "3.0.0"`) {
		t.Errorf("rust_pkg should be untouched: %s", got)
	}
	got := read(t, cairn)
	if !strings.Contains(got, "  pkg_a:\n    version: \"1.1.0\"") {
		t.Errorf("cairn.yaml pkg_a entry not advanced: %s", got)
	}
	if !strings.Contains(got, `version: "2.0.0"`) || !strings.Contains(got, `version: "3.0.0"`) {
		t.Errorf("cairn.yaml other package entries must stay put: %s", got)
	}
	if !strings.Contains(got, `version: "9.9.9"`) {
		t.Errorf("baseline version must not change on a per-package bump: %s", got)
	}
	if s := out.String(); !strings.Contains(s, "Bumped pkg_a: 1.0.0 → 1.1.0") || !strings.Contains(s, "git tag pkg_a-v1.1.0") {
		t.Errorf("per-package banner/tag missing:\n%s", s)
	}
}

// TestRunPackageBumpUnknown confirms an undeclared package name fails fast with the valid
// choices, rather than silently doing nothing.
func TestRunPackageBumpUnknown(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cairn.yaml", "version: \"0.0.0\"\ndirectories:\n  pkg_a:\n    version: \"1.0.0\"\n")
	err := runPackageBump(dir, loadTree(t, dir), "nope", "patch", time.Now(), &bytes.Buffer{}, false, false)
	if err == nil || !strings.Contains(err.Error(), "pkg_a") {
		t.Fatalf("want unknown-package error listing pkg_a, got %v", err)
	}
}

// TestRunBumpExplicitVersion confirms an explicit X.Y.Z is honored over level math.
func TestRunBumpExplicitVersion(t *testing.T) {
	dir := t.TempDir()
	cairn := writeFile(t, dir, "cairn.yaml", "version: 1.2.3\n")

	var out bytes.Buffer
	if err := runBump(dir, loadTree(t, dir), "2.0.0", time.Now(), &out, false, false); err != nil {
		t.Fatalf("runBump: %v", err)
	}
	if got := read(t, cairn); !strings.Contains(got, "version: 2.0.0") {
		t.Errorf("unquoted baseline version not advanced: %s", got)
	}
}

// TestRunBumpGuards covers the two refusals: an unset baseline version and a non-increasing bump
// (here an explicit downgrade). Neither should touch the filesystem.
func TestRunBumpGuards(t *testing.T) {
	t.Run("empty version", func(t *testing.T) {
		dir := t.TempDir() // no cairn.yaml ⇒ defaults, baseline version unset
		if err := runBump(dir, loadTree(t, dir), "patch", time.Now(), &bytes.Buffer{}, false, false); err == nil {
			t.Fatal("want error on unset baseline version")
		}
	})
	t.Run("downgrade", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "cairn.yaml", "version: \"2.0.0\"\n")
		err := runBump(dir, loadTree(t, dir), "1.0.0", time.Now(), &bytes.Buffer{}, false, false)
		if err == nil || !strings.Contains(err.Error(), "not greater") {
			t.Fatalf("want downgrade guard, got %v", err)
		}
		if !strings.Contains(err.Error(), "--force") {
			t.Errorf("downgrade refusal should point at --force, got %v", err)
		}
	})
}

// TestRunBumpForceDowngrade proves --force is the direct-path equivalent of the wizard's
// double-confirm: an explicit lower version that the guard would reject is applied when
// force is set, advancing the baseline backwards on purpose.
func TestRunBumpForceDowngrade(t *testing.T) {
	dir := t.TempDir()
	cairn := writeFile(t, dir, "cairn.yaml", "version: \"2.0.0\"\n")

	var out bytes.Buffer
	if err := runBump(dir, loadTree(t, dir), "1.0.0", time.Now(), &out, false, true); err != nil {
		t.Fatalf("forced downgrade: %v", err)
	}
	if got := read(t, cairn); !strings.Contains(got, `version: "1.0.0"`) {
		t.Errorf("forced downgrade not applied: %s", got)
	}
}

// TestRunBumpForceRejectsSame confirms --force still refuses a no-op: forcing the current
// version has nothing to apply, so it errors rather than silently doing nothing.
func TestRunBumpForceRejectsSame(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cairn.yaml", "version: \"2.0.0\"\n")
	err := runBump(dir, loadTree(t, dir), "2.0.0", time.Now(), &bytes.Buffer{}, false, true)
	if err == nil || !strings.Contains(err.Error(), "same") {
		t.Fatalf("want same-version refusal even with force, got %v", err)
	}
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
	cairn := writeFile(t, dir, "cairn.yaml", "version: \"1.2.0\"\n")

	var out bytes.Buffer
	if err := runBumpWizard(dir, loadTree(t, dir), strings.NewReader("2\ny\n"), &out, false, ""); err != nil {
		t.Fatalf("wizard: %v", err)
	}
	if got := read(t, cairn); !strings.Contains(got, `version: "1.3.0"`) {
		t.Errorf("baseline version not advanced: %s", got)
	}
	if s := out.String(); !strings.Contains(s, "Bumped 1.2.0 → 1.3.0") {
		t.Errorf("missing banner:\n%s", s)
	}
}

// TestWizardPreselectsInferred proves the inferred level is the wizard's default: a bare
// Enter (empty choice) accepts it, so feeding "" then a confirm applies the inferred bump.
func TestWizardPreselectsInferred(t *testing.T) {
	dir := t.TempDir()
	cairn := writeFile(t, dir, "cairn.yaml", "version: \"1.2.0\"\n")

	var out bytes.Buffer
	// Empty choice (Enter) + confirm, with "minor" inferred ⇒ 1.3.0.
	if err := runBumpWizard(dir, loadTree(t, dir), strings.NewReader("\ny\n"), &out, false, "minor"); err != nil {
		t.Fatalf("wizard: %v", err)
	}
	if got := read(t, cairn); !strings.Contains(got, `version: "1.3.0"`) {
		t.Errorf("inferred level not applied on Enter: %s", got)
	}
	if s := out.String(); !strings.Contains(s, "inferred from commits") {
		t.Errorf("menu should mark the inferred level:\n%s", s)
	}
}

// TestWizardQuitLeavesFilesUntouched confirms 'q' aborts cleanly without writing.
func TestWizardQuitLeavesFilesUntouched(t *testing.T) {
	dir := t.TempDir()
	cairn := writeFile(t, dir, "cairn.yaml", "version: \"1.2.0\"\n")

	var out bytes.Buffer
	if err := runBumpWizard(dir, loadTree(t, dir), strings.NewReader("q\n"), &out, false, ""); err != nil {
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
	cairn := writeFile(t, dir, "cairn.yaml", "version: \"2.0.0\"\n")

	var out bytes.Buffer
	// choice 4 (custom) → 1.0.0 → downgrade confirm → second confirm → apply confirm.
	if err := runBumpWizard(dir, loadTree(t, dir), strings.NewReader("4\n1.0.0\ny\ny\ny\n"), &out, false, ""); err != nil {
		t.Fatalf("wizard: %v", err)
	}
	if got := read(t, cairn); !strings.Contains(got, `version: "1.0.0"`) {
		t.Errorf("confirmed downgrade not applied: %s", got)
	}
	if !strings.Contains(out.String(), "DOWNGRADE") {
		t.Errorf("expected a loud downgrade warning")
	}
}

// TestComputeNextCalVer checks that for the calver scheme a level argument yields the
// date-based next version rather than semver math.
func TestComputeNextCalVer(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cairn.yaml", "version: \"2024.1.0\"\nversioning: calver\n")
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	_, next, err := computeNext(loadTree(t, dir), "patch", now, false)
	if err != nil {
		t.Fatal(err)
	}
	if next.String() != "2026.6.0" {
		t.Errorf("calver next = %s, want 2026.6.0", next)
	}
}

// TestInferredPackageBump is the per-package inference acceptance: in a mixed-language monorepo,
// a `feat:` that touched only pkg_a infers `minor` for pkg_a and `none` for pkg_b (each from its
// own scoped history), and `cairn bump <pkg>` with no level applies the inferred level to that
// package alone. Neither package has a tag yet, so inference degrades to each package's whole
// history.
func TestInferredPackageBump(t *testing.T) {
	dir := t.TempDir()
	cairn := writeFile(t, dir, "cairn.yaml",
		"version: \"0.0.0\"\ndirectories:\n  pkg_a:\n    version: \"1.0.0\"\n  pkg_b:\n    version: \"2.0.0\"\n")
	pkgA := writeFile(t, dir, "pkg_a/pubspec.yaml", "name: pkg_a\nversion: 1.0.0\n")
	pkgB := writeFile(t, dir, "pkg_b/pubspec.yaml", "name: pkg_b\nversion: 2.0.0\n")
	gitCommit(t, dir, "chore: scaffold", "cairn.yaml", "pkg_a", "pkg_b")
	writeFile(t, dir, "pkg_a/feature.txt", "new\n")
	gitCommit(t, dir, "feat: add a feature to pkg_a", "pkg_a/feature.txt")

	tree := loadTree(t, dir)
	if got := inferPackageLevel(dir, tree, "pkg_a"); got != "minor" {
		t.Errorf("pkg_a inferred level = %q, want minor", got)
	}
	if got := inferPackageLevel(dir, tree, "pkg_b"); got != "" {
		t.Errorf("pkg_b inferred level = %q, want none", got)
	}

	var out bytes.Buffer
	if err := runInferredPackageBump(dir, tree, "pkg_a", time.Now(), &out, false, false); err != nil {
		t.Fatalf("runInferredPackageBump: %v", err)
	}
	if got := read(t, pkgA); !strings.Contains(got, "version: 1.1.0") {
		t.Errorf("pkg_a not advanced to 1.1.0: %s", got)
	}
	if got := read(t, pkgB); !strings.Contains(got, "version: 2.0.0") {
		t.Errorf("pkg_b must be untouched: %s", got)
	}
	if got := read(t, cairn); !strings.Contains(got, "    version: \"1.1.0\"") || !strings.Contains(got, `version: "0.0.0"`) {
		t.Errorf("cairn.yaml: pkg_a should advance, baseline stay: %s", got)
	}
}

// TestInferredPackageBumpNoChanges proves the fail-fast: a declared package with no
// release-worthy commits since its last tag refuses the inferred bump (nothing written) and
// names the package, rather than silently doing nothing.
func TestInferredPackageBumpNoChanges(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cairn.yaml", "version: \"0.0.0\"\ndirectories:\n  pkg_a:\n    version: \"1.0.0\"\n")
	writeFile(t, dir, "pkg_a/pubspec.yaml", "name: pkg_a\nversion: 1.0.0\n")
	gitCommit(t, dir, "chore: scaffold", "cairn.yaml", "pkg_a")

	err := runInferredPackageBump(dir, loadTree(t, dir), "pkg_a", time.Now(), &bytes.Buffer{}, false, false)
	if err == nil || !strings.Contains(err.Error(), "pkg_a") {
		t.Fatalf("want a no-inference error naming pkg_a, got %v", err)
	}
}
