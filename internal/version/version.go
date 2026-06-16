// Package version is the Versioning bounded context: SemVer/CalVer math and the
// version_sync doc-honesty check. It owns *what* the next version is and whether docs
// stay honest with the canonical version; writing the version into language manifests
// (the *how*) lives behind self-registering VersionManager adapters added in 6b.
package version

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Version is a parsed semantic version (major.minor.patch). A leading "v" is accepted on
// parse and dropped; String never emits it. Prerelease/build metadata are out of scope
// until a real case earns them (keep it simple).
type Version struct {
	Major, Minor, Patch int
}

// Parse reads "X.Y.Z" (optionally "vX.Y.Z"). It errors on anything malformed or empty so
// a missing project.canonical_version is caught at the boundary rather than silently
// treated as 0.0.0.
func Parse(s string) (Version, error) {
	raw := strings.TrimPrefix(strings.TrimSpace(s), "v")
	if raw == "" {
		return Version{}, fmt.Errorf("empty version")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("version %q must be major.minor.patch", s)
	}
	var v Version
	for i, dst := range []*int{&v.Major, &v.Minor, &v.Patch} {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("version %q has a non-numeric component %q", s, parts[i])
		}
		*dst = n
	}
	return v, nil
}

// String renders the canonical "major.minor.patch" form, no leading "v".
func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

// Compare returns -1, 0, or +1 as v sorts before, equal to, or after o.
func (v Version) Compare(o Version) int {
	for _, d := range []int{v.Major - o.Major, v.Minor - o.Minor, v.Patch - o.Patch} {
		switch {
		case d < 0:
			return -1
		case d > 0:
			return 1
		}
	}
	return 0
}

// Next returns v incremented by level: "major" (X+1.0.0), "minor" (X.Y+1.0), or
// "patch" (X.Y.Z+1). An unknown level is an error so a typo never silently no-ops a bump.
func (v Version) Next(level string) (Version, error) {
	switch level {
	case "major":
		return Version{v.Major + 1, 0, 0}, nil
	case "minor":
		return Version{v.Major, v.Minor + 1, 0}, nil
	case "patch":
		return Version{v.Major, v.Minor, v.Patch + 1}, nil
	default:
		return Version{}, fmt.Errorf("unknown bump level %q (want major|minor|patch)", level)
	}
}

// NextCalVer returns the CalVer (YYYY.MM.MICRO) that follows prev for now: the micro
// resets to 0 in a new year-month and increments within the same one. prev may be the
// zero Version for a first release. CalVer keeps the same Version shape so comparison and
// version_sync work unchanged.
func NextCalVer(prev Version, now time.Time) Version {
	year, month := now.Year(), int(now.Month())
	if prev.Major == year && prev.Minor == month {
		return Version{year, month, prev.Patch + 1}
	}
	return Version{year, month, 0}
}
