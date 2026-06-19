package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IVIR3zaM/Cairn/internal/changelog"
	"github.com/IVIR3zaM/Cairn/internal/commit"
	"github.com/IVIR3zaM/Cairn/internal/wiring"
)

// TestWizardAppliesChoicesOverDetection: the guided run overlays the user's decisions on the
// discovered baseline — here enabling DCO sign-off on a history that doesn't sign off — and the
// standard menus are sourced from the registries (so a newly registered standard appears with no
// edit to the wizard). One run proves both the apply-over-default and the registry-driven menus.
func TestWizardAppliesChoicesOverDetection(t *testing.T) {
	dir := t.TempDir()
	gitInitWithUser(t, dir)
	writeFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.23\n")

	// keep version default, keep convention/changelog/CI defaults, but turn sign-off ON, then
	// accept both wiring steps.
	in := strings.NewReader("\n\ny\n\n\n\n\n")
	var out bytes.Buffer
	if err := runInitWizard(dir, in, &out); err != nil {
		t.Fatalf("runInitWizard: %v", err)
	}

	root, ok := loadTree(t, dir).Resolve(".")
	if !ok {
		t.Fatal("root resolved as pruned")
	}
	if root.Commits == nil || !root.Commits.Signoff {
		t.Errorf("wizard did not apply the sign-off choice over detection: %+v", root.Commits)
	}

	// Every registry entry must be offered — the proof that menus are registry-driven, not a
	// hand-maintained list the wizard would have to edit when a standard is added.
	s := out.String()
	for _, want := range commit.Conventions() {
		assertOffered(t, s, want)
	}
	for _, want := range changelog.Standards() {
		assertOffered(t, s, want)
	}
	for _, want := range wiring.Providers() {
		assertOffered(t, s, want)
	}
}

// TestWizardDeclineWiring: answering "no" to the wiring questions skips InstallHooks/GenerateCI
// entirely — no hook or workflow files — while still writing the config, so a user can adopt Cairn
// without touching their hooks/CI.
func TestWizardDeclineWiring(t *testing.T) {
	dir := t.TempDir()
	gitInitWithUser(t, dir)
	writeFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.23\n")

	// keep version + all standards (incl. repo-strict default); decline both hooks and CI.
	in := strings.NewReader("\n\n\n\n\n\nn\nn\n")
	var out bytes.Buffer
	if err := runInitWizard(dir, in, &out); err != nil {
		t.Fatalf("runInitWizard: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "cairn.yaml")); err != nil {
		t.Fatalf("wizard did not write cairn.yaml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".cairn/hooks/pre-commit")); !os.IsNotExist(err) {
		t.Errorf("declined hooks were still installed (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".github/workflows/cairn.yml")); !os.IsNotExist(err) {
		t.Errorf("declined CI workflow was still generated (err=%v)", err)
	}
}

// TestWizardConfiguresFullDirectoryRuleSet: the wizard offers a sub-unit the *full* per-directory
// override set, not just version/disable. Here pkgs/web is made independently versioned on a calver
// scheme with its own pub.dev-style (dart) changelog — every facet resolving through the Tree —
// proving the wizard exposes all the possibilities the schema allows.
func TestWizardConfiguresFullDirectoryRuleSet(t *testing.T) {
	dir := t.TempDir()
	gitInitWithUser(t, dir)
	writeFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.23\n")
	writeFile(t, dir, "CHANGELOG.md", "# Changelog\n\n## [Unreleased]\n\n### Added\n- x\n")
	writeFile(t, dir, "pkgs/web/package.json", `{"name":"web","version":"2.0.0"}`)

	// version (blank), standards (4 blank + repo-strict blank), then configure pkgs/web:
	// enable(default), version independently=y, scheme=calver, changelog=dart, strict js (default
	// no), then wiring (2 blank).
	answers := []string{"", "", "", "", "", "", "y", "", "y", "calver", "dart", "", "", ""}
	in := strings.NewReader(strings.Join(answers, "\n") + "\n")
	var out bytes.Buffer
	if err := runInitWizard(dir, in, &out); err != nil {
		t.Fatalf("runInitWizard: %v", err)
	}

	tree := loadTree(t, dir)
	web, ok := tree.Resolve("pkgs/web")
	if !ok {
		t.Fatal("pkgs/web resolved as pruned")
	}
	if web.Version == nil || *web.Version != "2.0.0" {
		t.Errorf("pkgs/web version = %v, want 2.0.0", web.Version)
	}
	if web.Versioning == nil || *web.Versioning != "calver" {
		t.Errorf("pkgs/web versioning = %v, want calver", web.Versioning)
	}
	if web.Changelog == nil || web.Changelog.Standard != "dart" {
		t.Errorf("pkgs/web changelog = %v, want dart", web.Changelog)
	}
	if !strings.Contains(strings.Join(tree.Independent(), ","), "pkgs/web") {
		t.Errorf("pkgs/web not recorded as independently versioned: %v", tree.Independent())
	}
	if !strings.Contains(out.String(), "pkgs/web") {
		t.Errorf("wizard did not surface the detected sub-directory:\n%s", out.String())
	}
}

// TestYesDetectsPerDirectoryChangelog: the non-interactive path records per-directory facts too —
// a sub-package whose CHANGELOG follows a different (pub.dev/dart) format than the keepachangelog
// root gets a `directories.<path>.changelog` override, with no version override when its manifest
// declares none. This is the gap the earlier wizard missed: `--yes` now emits the same per-directory
// rule sets it can detect.
func TestYesDetectsPerDirectoryChangelog(t *testing.T) {
	dir := t.TempDir()
	gitInitWithUser(t, dir)
	writeFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.23\n")
	writeFile(t, dir, "CHANGELOG.md", "# Changelog\n\n## [Unreleased]\n\n### Added\n- x\n")
	writeFile(t, dir, "packages/foo/package.json", `{"name":"foo"}`)
	writeFile(t, dir, "packages/foo/CHANGELOG.md", "# Changelog\n\n## Unreleased\n\n- y\n")

	var out bytes.Buffer
	if err := runInit(dir, &out); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	tree := loadTree(t, dir)
	foo, ok := tree.Resolve("packages/foo")
	if !ok {
		t.Fatal("packages/foo resolved as pruned")
	}
	if foo.Changelog == nil || foo.Changelog.Standard != "dart" {
		t.Errorf("packages/foo changelog override = %v, want dart", foo.Changelog)
	}
	// No version override: the manifest declares none, so packages/foo stays lockstep on the
	// baseline (Independent lists only dirs whose own layer sets a version).
	if got := strings.Join(tree.Independent(), ","); strings.Contains(got, "packages/foo") {
		t.Errorf("packages/foo should not be independently versioned: %q", got)
	}
}

// TestWizardRepoWideStrictInheritsAndOverrides: the wizard asks for a repo-wide strict default
// before the per-directory pass. Enabling it records verify.strict on the baseline; a sub-directory
// then inherits that default, so pressing Enter on its strict question writes no override (it keeps
// the repo setting), while a deliberate "no" records a per-language strict=false override.
func TestWizardRepoWideStrictInheritsAndOverrides(t *testing.T) {
	dir := t.TempDir()
	gitInitWithUser(t, dir)
	writeFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.23\n")
	writeFile(t, dir, "pkgs/web/package.json", `{"name":"web","version":"2.0.0"}`)

	// version (blank), standards (4 blank), repo-strict=y, then configure pkgs/web: enable(default),
	// version independently (default y, detected), scheme blank, changelog blank, strict js: inherit
	// (blank keeps the repo default → no override), then wiring (2 blank).
	answers := []string{"", "", "", "", "", "y", "", "", "", "", "", "", ""}
	in := strings.NewReader(strings.Join(answers, "\n") + "\n")
	var out bytes.Buffer
	if err := runInitWizard(dir, in, &out); err != nil {
		t.Fatalf("runInitWizard: %v", err)
	}

	tree := loadTree(t, dir)
	root, _ := tree.Resolve(".")
	if !root.VerifyOrDefault().Strict {
		t.Errorf("wizard did not record the repo-wide strict choice: %+v", root.Verify)
	}
	// pkgs/web inherits the repo default through resolution.
	web, ok := tree.Resolve("pkgs/web")
	if !ok {
		t.Fatal("pkgs/web resolved as pruned")
	}
	if !web.VerifyOrDefault().Strict {
		t.Errorf("pkgs/web should inherit repo-wide strict, got verify.strict=false")
	}
	// Pressing Enter on the per-directory strict question wrote no override: the only strict in the
	// file is the repo-wide one (no per-language strict: false leaked into the directory block).
	if cfg := read(t, filepath.Join(dir, "cairn.yaml")); strings.Contains(cfg, "strict: false") {
		t.Errorf("inheriting directory should not write a strict override:\n%s", cfg)
	}
}

// TestWizardDisablingSoleDirectoryPrunesLanguage: when the only home of a language is a directory
// the user disables, that language is not enabled repo-wide. Here dart lives at the root and java
// only under reference/didwebvh-java; disabling that directory leaves dart enabled and java absent
// from the baseline language list, instead of blindly enabling a language nothing reachable uses.
func TestWizardDisablingSoleDirectoryPrunesLanguage(t *testing.T) {
	dir := t.TempDir()
	gitInitWithUser(t, dir)
	writeFile(t, dir, "pubspec.yaml", "name: app\nversion: 0.1.0\n")
	writeFile(t, dir, "reference/didwebvh-java/pom.xml", "<project><artifactId>x</artifactId></project>\n")

	// version (blank), standards (4 blank + repo-strict blank), then for reference/didwebvh-java:
	// configure=y, enable=n (disable it), then wiring (2 blank).
	answers := []string{"", "", "", "", "", "", "y", "n", "", ""}
	in := strings.NewReader(strings.Join(answers, "\n") + "\n")
	var out bytes.Buffer
	if err := runInitWizard(dir, in, &out); err != nil {
		t.Fatalf("runInitWizard: %v", err)
	}

	root, _ := loadTree(t, dir).Resolve(".")
	if _, ok := root.Languages["dart"]; !ok {
		t.Errorf("dart (present at root) should stay enabled repo-wide: %+v", root.Languages)
	}
	if _, ok := root.Languages["java"]; ok {
		t.Errorf("java (only in the disabled directory) should not be enabled repo-wide: %+v", root.Languages)
	}
}

// TestWizardAsksForVersionWhenUnsure: when detection can't determine the repo version confidently
// (a go.mod-only repo declares none), the wizard asks rather than silently committing the 0.1.0
// placeholder, and the user's typed answer is what lands in the config.
func TestWizardAsksForVersionWhenUnsure(t *testing.T) {
	dir := t.TempDir()
	gitInitWithUser(t, dir)
	writeFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.23\n")

	// version=2.0.0 (first prompt), everything else default via EOF.
	in := strings.NewReader("2.0.0\n")
	var out bytes.Buffer
	if err := runInitWizard(dir, in, &out); err != nil {
		t.Fatalf("runInitWizard: %v", err)
	}

	if !strings.Contains(out.String(), "couldn't detect") {
		t.Errorf("wizard should flag that it couldn't detect a version:\n%s", out.String())
	}
	root, _ := loadTree(t, dir).Resolve(".")
	if root.Version == nil || *root.Version != "2.0.0" {
		t.Errorf("wizard did not apply the typed version: got %v, want 2.0.0", root.Version)
	}
}

// TestWizardFindingsShowVersionsAndScansReadme: the findings list each sub-directory's detected
// version, the baseline defaults to the dominant one, and accepting it wires the README's version
// mentions as version_sync patterns (proving the README is scanned against the chosen version).
func TestWizardFindingsShowVersionsAndScansReadme(t *testing.T) {
	dir := t.TempDir()
	gitInitWithUser(t, dir)
	writeFile(t, dir, "pubspec.yaml", "name: _ws\nworkspace:\n")
	writeFile(t, dir, "packages/a/pubspec.yaml", "name: a\nversion: 1.2.3\n")
	writeFile(t, dir, "packages/b/pubspec.yaml", "name: b\nversion: 1.2.3\n")
	writeFile(t, dir, "README.md", "# proj\n\n![ver](https://img.shields.io/badge/version-1.2.3-blue)\n")

	// Accept every default (dominant version, standards, no per-dir rules, wiring) via EOF.
	var out bytes.Buffer
	if err := runInitWizard(dir, strings.NewReader(""), &out); err != nil {
		t.Fatalf("runInitWizard: %v", err)
	}

	s := out.String()
	if !strings.Contains(s, "packages/a (dart) — version 1.2.3") {
		t.Errorf("findings did not show the sub-directory version:\n%s", s)
	}
	if !strings.Contains(s, "[1.2.3]") {
		t.Errorf("version prompt should default to the dominant version:\n%s", s)
	}

	root, _ := loadTree(t, dir).Resolve(".")
	if root.Version == nil || *root.Version != "1.2.3" {
		t.Errorf("baseline = %v, want dominant 1.2.3", root.Version)
	}
	if root.VersionSync == nil || len(root.VersionSync.Files) != 1 {
		t.Fatalf("README was not scanned for version_sync: %+v", root.VersionSync)
	}
}

func assertOffered(t *testing.T, output, option string) {
	t.Helper()
	if !strings.Contains(output, option) {
		t.Errorf("registry option %q was not offered in the wizard:\n%s", option, output)
	}
}
