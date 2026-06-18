package changelog

import (
	"strings"
	"testing"
	"time"

	version "github.com/IVIR3zaM/Cairn/internal/version"
)

func mustParse(t *testing.T, s string) version.Version {
	t.Helper()
	v, err := version.Parse(s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v
}

// Detect recognises a Keep a Changelog file by its bracketed headings (both an `[Unreleased]`
// section and a released `[x.y.z] - date` heading), and reports nothing for a foreign or empty
// file — so `cairn init` records the changelog block only when it is confident of the format.
func TestDetectKeepAChangelog(t *testing.T) {
	for _, body := range []string{
		"# Changelog\n\n## [Unreleased]\n\n### Added\n- thing\n",
		"# Changelog\n\n## [1.2.3] - 2026-06-09\n\n### Fixed\n- bug\n",
	} {
		if got, ok := Detect([]byte(body)); !ok || got != "keepachangelog" {
			t.Errorf("Detect(%q) = %q, %v; want keepachangelog, true", body, got, ok)
		}
	}
	for _, body := range []string{
		"",
		"# Changelog\n\n## Unreleased\n\n- plain heading, no brackets\n",
		"just some prose with no changelog headings at all\n",
	} {
		if got, ok := Detect([]byte(body)); ok {
			t.Errorf("Detect(%q) = %q, true; want no match", body, got)
		}
	}
}

// WriterFor resolves a registered standard via self-registration and reports the absence of
// an unregistered one (so bump can skip promotion for a future-only standard).
func TestWriterForResolvesRegistered(t *testing.T) {
	if _, ok := WriterFor("keepachangelog"); !ok {
		t.Fatal("keepachangelog should be registered")
	}
	if _, ok := WriterFor("git-cliff"); ok {
		t.Fatal("git-cliff has no writer yet; WriterFor should report false")
	}
}

const sample = `# Changelog

## [Unreleased]

### Added
- A shiny new thing.

### Fixed
- A pesky bug.

## [1.0.0] - 2024-01-01

### Added
- First release.

[Unreleased]: https://github.com/o/r/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/o/r/releases/tag/v1.0.0
`

// Promote moves the Unreleased entries under a dated release, refreshes the compare links
// (Unreleased now starts at the new tag; a new version link spans the previous tag), leaves a
// fresh empty Unreleased, and is idempotent: a second run finds nothing to promote.
func TestPromoteReleasesAndUpdatesLinks(t *testing.T) {
	w, _ := WriterFor("keepachangelog")
	date := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	res, err := w.Promote([]byte(sample), mustParse(t, "1.1.0"), date)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.Empty {
		t.Fatalf("expected a change with non-empty Unreleased, got changed=%v empty=%v", res.Changed, res.Empty)
	}
	got := string(res.Content)

	if !strings.Contains(got, "## [1.1.0] - 2024-06-01") {
		t.Errorf("missing dated release heading:\n%s", got)
	}
	// The moved entries land under the new release, above the previous one.
	rel := strings.Index(got, "## [1.1.0]")
	if a := strings.Index(got, "A shiny new thing."); a < rel {
		t.Errorf("Unreleased entry not moved under the new release")
	}
	if !strings.Contains(got, "[Unreleased]: https://github.com/o/r/compare/v1.1.0...HEAD") {
		t.Errorf("Unreleased link not advanced to v1.1.0:\n%s", got)
	}
	if !strings.Contains(got, "[1.1.0]: https://github.com/o/r/compare/v1.0.0...v1.1.0") {
		t.Errorf("missing new version compare link:\n%s", got)
	}
	// Unreleased is left empty, so promoting the result again is a no-op.
	again, err := w.Promote(res.Content, mustParse(t, "1.2.0"), date)
	if err != nil {
		t.Fatal(err)
	}
	if again.Changed || !again.Empty {
		t.Errorf("re-promote should be a no-op (empty Unreleased), got changed=%v empty=%v", again.Changed, again.Empty)
	}
}

// The dart standard promotes the plain pub.dev style — `## Unreleased` → `## X.Y.Z - DATE`
// with no brackets and no compare links — which is what each package in a Dart workspace keeps.
func TestDartStylePromote(t *testing.T) {
	const pkg = `# Changelog

## Unreleased

- A package change.

## 0.1.0 - 2024-01-01

- First.
`
	w, ok := WriterFor("dart")
	if !ok {
		t.Fatal("dart should be registered")
	}
	res, err := w.Promote([]byte(pkg), mustParse(t, "0.2.0"), time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	got := string(res.Content)
	if !strings.Contains(got, "## 0.2.0 - 2024-06-01") {
		t.Errorf("missing plain released heading:\n%s", got)
	}
	if strings.Contains(got, "## [0.2.0]") {
		t.Errorf("dart style must not bracket the released heading:\n%s", got)
	}
	if strings.Index(got, "A package change.") < strings.Index(got, "## 0.2.0") {
		t.Errorf("unreleased entry not moved under the new release:\n%s", got)
	}
}

// An Unreleased section with only blank lines and stale group headers reports Empty and is
// left untouched, so bump warns instead of cutting an empty release.
func TestPromoteEmptyUnreleasedWarns(t *testing.T) {
	const empty = `# Changelog

## [Unreleased]

### Added

## [1.0.0] - 2024-01-01

### Added
- First release.
`
	w, _ := WriterFor("keepachangelog")
	res, err := w.Promote([]byte(empty), mustParse(t, "1.1.0"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed || !res.Empty {
		t.Errorf("empty Unreleased should report Empty and no change, got changed=%v empty=%v", res.Changed, res.Empty)
	}
}
