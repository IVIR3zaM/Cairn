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

// Resolver maps a detected unit (by directory) to the version it must carry. Built from the
// resolved per-directory config Tree (schema 2), it answers ForDir via Tree.Resolve, so
// per-package vs lockstep is entirely the cascade's concern — config owns the precedence, not
// the resolver. The legacy project-config constructor (NewResolver) is kept until verify/bump
// migrate to the Tree; it honors project.packages for an independently-versioned monorepo and
// falls back to project.canonical_version, lockstep being the degenerate all-equal case. Query
// it once per unit; a workspace member is resolved by its own directory too.
type Resolver struct {
	tree *config.Tree // schema-2 per-directory model; when set, ForDir delegates to it

	// legacy project-config fallback (used when tree is nil)
	canonical string
	scheme    string
	packages  []config.PackageVersion
}

// NewResolverFromTree builds a Resolver over the resolved per-directory config Tree: each unit
// dir resolves to its target version + scheme via Tree.Resolve, so the canonical-vs-per-package
// precedence lives in config's cascade, not here. A dir pruned by the disable gate yields an
// empty Target (no version to assert). This is the config-owns-the-cascade path the CLI adopts.
func NewResolverFromTree(t *config.Tree) *Resolver {
	return &Resolver{tree: t}
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
	if r.tree != nil {
		d, ok := r.tree.Resolve(dir)
		if !ok {
			return Target{} // pruned by the disable gate — no version to assert
		}
		t := Target{}
		if d.Version != nil {
			t.Version = *d.Version
		}
		if d.Versioning != nil {
			t.Versioning = *d.Versioning
		}
		return t
	}
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

// targetVersion resolves dir to its parsed target Version. ok is false when no version is
// configured for the unit (an empty literal — e.g. a per-package repo with no canonical and
// no covering entry), so the caller simply skips it; err is non-nil only for a present but
// malformed version literal (a config mistake worth surfacing). It is the engine's single
// entry point for "the version this unit must carry", so Check/CheckManifests/CheckWorkspace
// share one resolution and one skip/error policy.
func (r *Resolver) targetVersion(dir string) (Version, bool, error) {
	lit := r.ForDir(dir).Version
	if lit == "" {
		return Version{}, false, nil
	}
	v, err := Parse(lit)
	if err != nil {
		return Version{}, false, err
	}
	return v, true, nil
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
