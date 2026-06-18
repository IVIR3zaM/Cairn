package version

import (
	"path"

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
// the resolver. Query it once per unit; a workspace member is resolved by its own directory too.
type Resolver struct {
	tree *config.Tree // schema-2 per-directory model; ForDir delegates to it
}

// NewResolverFromTree builds a Resolver over the resolved per-directory config Tree: each unit
// dir resolves to its target version + scheme via Tree.Resolve, so the canonical-vs-per-package
// precedence lives in config's cascade, not here. A dir pruned by the disable gate yields an
// empty Target (no version to assert). This is the only constructor — config owns the cascade.
func NewResolverFromTree(t *config.Tree) *Resolver {
	return &Resolver{tree: t}
}

// ForDir returns the Target for a detected unit directory by resolving it through the config
// Tree's cascade. A dir pruned by the disable gate yields an empty Target (no version to assert).
func (r *Resolver) ForDir(dir string) Target {
	dir = path.Clean(dir)
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
