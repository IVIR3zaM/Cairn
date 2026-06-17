package version

import (
	"path"
	"strings"

	"github.com/IVIR3zaM/Cairn/internal/config"
)

// Target is the version a detected unit must carry: the version literal and the
// versioning scheme that governs how it advances. It is what the Resolver answers —
// "the version for *this* package" — so callers (bump, verify) never re-derive the
// canonical-vs-per-package precedence.
type Target struct {
	Version    string
	Versioning string
}

// Resolver maps a detected unit (by directory) to the version it must carry. It honors
// project.packages for a monorepo whose packages version independently and falls back to
// project.canonical_version when no package entry covers the unit — lockstep is just the
// degenerate case where every unit resolves to the one canonical version. Build it once
// from project config and query it per unit; a workspace member is resolved by its own
// directory too, so 6g-iii can turn detected members into a name→version map by running
// each member's dir through ForDir.
type Resolver struct {
	canonical string
	scheme    string
	packages  []config.PackageVersion
}

// NewResolver builds a Resolver from project config, cleaning each package path so a
// trailing slash or "./" prefix in cairn.yaml matches a detected unit dir.
func NewResolver(p config.Project) *Resolver {
	pkgs := make([]config.PackageVersion, len(p.Packages))
	copy(pkgs, p.Packages)
	for i := range pkgs {
		pkgs[i].Path = path.Clean(pkgs[i].Path)
	}
	return &Resolver{
		canonical: p.CanonicalVersion,
		scheme:    p.Versioning,
		packages:  pkgs,
	}
}

// ForDir returns the Target for a detected unit directory. The most specific (longest
// path) project.packages entry whose path covers dir wins, so a nested package overrides
// an ancestor one; with no match it falls back to the repo-wide canonical version and
// scheme. A matching package's scheme is its own override or the inherited project scheme.
func (r *Resolver) ForDir(dir string) Target {
	dir = path.Clean(dir)
	best := -1
	for i := range r.packages {
		if !covers(r.packages[i].Path, dir) {
			continue
		}
		if best == -1 || len(r.packages[best].Path) < len(r.packages[i].Path) {
			best = i
		}
	}
	if best == -1 {
		return Target{Version: r.canonical, Versioning: r.scheme}
	}
	p := r.packages[best]
	return Target{Version: p.Version, Versioning: p.VersioningFor(r.scheme)}
}

// covers reports whether a package rooted at pkgPath governs unit dir: an exact match or
// dir nested beneath pkgPath. "." covers the whole repo. The trailing-slash check stops a
// sibling like "pkg/foobar" from being treated as nested under "pkg/foo".
func covers(pkgPath, dir string) bool {
	if pkgPath == dir || pkgPath == "." {
		return true
	}
	return strings.HasPrefix(dir, pkgPath+"/")
}
