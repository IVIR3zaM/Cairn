package version

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
)

// Manager writes the canonical version into one language manifest format. Each format
// lives in its own manifest_<name>.go and self-registers in init(); bump resolves it via
// ManagerFor. Implementations are pure byte-transforms (safe regex) so they are trivially
// testable and never shell out — native bumpers (e.g. `npm version`) are a later opt-in,
// documented not stubbed (ADR-006: a registry is earned, entries are added, never engines).
type Manager interface {
	// Filename is the manifest this manager owns, e.g. "package.json".
	Filename() string
	// SetVersion returns content with its version set to v and whether anything changed
	// (false when already correct, so bump can skip an untouched file). It errors only
	// when the manifest declares no version this manager can locate.
	SetVersion(content []byte, v Version) ([]byte, bool, error)
	// ReadVersion returns the project's own declared version (its numeric X.Y.Z core, any
	// qualifier such as -SNAPSHOT dropped) and true, or false when the manifest declares no
	// version this manager can locate (e.g. an inheriting child pom). It is the read mirror
	// of SetVersion, used by `cairn init` to seed cairn.yaml from the real project version
	// instead of a placeholder.
	ReadVersion(content []byte) (Version, bool)
}

// managers maps a manifest filename to its writer, populated by each manifest_<name>.go
// init(). Adding a manifest format is dropping one self-registering file — no edits here.
var managers = map[string]Manager{}

// register wires a manifest manager. Call it from a manifest_<name>.go init(). It panics
// on a duplicate filename to catch a copy-paste mistake at startup.
func register(m Manager) {
	if _, dup := managers[m.Filename()]; dup {
		panic("version: duplicate manager registered for " + m.Filename())
	}
	managers[m.Filename()] = m
}

// ManagerFor returns the manager owning filename, or false when none is registered.
func ManagerFor(filename string) (Manager, bool) {
	m, ok := managers[filename]
	return m, ok
}

// Managers lists every registered manager, sorted by filename for a stable scan/output.
func Managers() []Manager {
	out := make([]Manager, 0, len(managers))
	for _, m := range managers {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Filename() < out[j].Filename() })
	return out
}

// ManifestUnit is one detected unit's version-bearing manifests: its directory and the
// manifest filenames its language owns. CheckManifests walks these, keeping the version
// package decoupled from detect — verify maps each detect.Language to a ManifestUnit.
type ManifestUnit struct {
	Dir       string
	Manifests []string
}

// ManifestDrift is one language-owned manifest whose stated version disagrees with the
// canonical one. It is the manifest analogue of Drift (version_sync's custom-file drift),
// surfaced by CheckManifests so verify catches drift in the very files bump writes — no
// per-file version_sync config required.
type ManifestDrift struct {
	Path   string
	Want   string
	Detail string // optional context (e.g. which sibling constraint drifted); empty for the version: field
}

// Reason renders a one-line, actionable description for the reporter. A Detail (a workspace
// sibling drift) already carries its own "want", since each sibling may target a different
// per-package version; the plain case states the unit's own resolved target.
func (d ManifestDrift) Reason() string {
	if d.Detail != "" {
		return fmt.Sprintf("%s: %s", d.Path, d.Detail)
	}
	return fmt.Sprintf("%s: version disagrees with declared %s", d.Path, d.Want)
}

// CheckManifests is the non-mutating honesty assertion over language-owned manifests: for
// each unit's declared manifest it checks the file's version against canonical and reports
// any that drift. It reuses each Manager without writing: SetVersion(content, canonical)
// reports changed=false for an honest manifest and changed=true for a drifted one. A
// declared manifest with no registered writer, a missing file, or a present file that
// states no locatable version is skipped (none is a *lie* about a version). Each unit is
// asserted against *its own* resolved target version (res maps a unit dir to it), so a
// monorepo whose packages version independently is checked per package; lockstep is the
// degenerate case where every unit resolves to the one canonical version. It returns the
// drifts and how many manifests were actually examined, so the caller can omit the check
// entirely when nothing version-bearing was found. A nil resolver or no units is a no-op,
// as is a unit with no configured version.
func CheckManifests(fsys fs.FS, res *Resolver, units []ManifestUnit) ([]ManifestDrift, int, error) {
	if res == nil || len(units) == 0 {
		return nil, 0, nil
	}
	var drifts []ManifestDrift
	checked := 0
	seen := map[string]bool{} // a manifest path is checked at most once
	for _, u := range units {
		want, ok, err := res.targetVersion(u.Dir)
		if err != nil {
			return nil, 0, fmt.Errorf("version for %s: %w", u.Dir, err)
		}
		if !ok {
			continue // no version configured for this unit
		}
		for _, fname := range u.Manifests {
			m, ok := ManagerFor(fname)
			if !ok {
				continue // declared location with no writer yet
			}
			rel := path.Join(u.Dir, fname)
			if seen[rel] {
				continue
			}
			seen[rel] = true
			data, err := fs.ReadFile(fsys, rel)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, checked, fmt.Errorf("%s: %w", rel, err)
			}
			_, changed, err := m.SetVersion(data, want)
			if err != nil {
				continue // states no version this manager can locate — nothing to contradict
			}
			checked++
			if changed {
				drifts = append(drifts, ManifestDrift{Path: rel, Want: want.String()})
			}
		}
	}
	return drifts, checked, nil
}

// Workspace is the optional capability a Manager implements when its manifest format can
// depend on sibling packages in the same repo — a Dart pub workspace, a Cargo/npm workspace,
// a Maven/Gradle reactor. The engine (CheckWorkspace/RewriteWorkspace) gathers package
// identities across every manifest of that format and asks each one to reconcile its
// intra-repo dependency constraints, so multi-package version lockstep works for any such
// format without the engine — or the CLI — naming a language. A format without
// interdependencies (a lone package.json, Cargo.toml) simply doesn't implement it.
type Workspace interface {
	// PackageID returns the package name this manifest declares, used to recognize a sibling
	// dependency by name; false when it declares none.
	PackageID(content []byte) (string, bool)
	// SetSiblings rewrites every dependency on a member to that member's target version,
	// returning the result and whether it changed. members maps each member name to the
	// version it must carry, so independently-versioned packages reconcile per package.
	SetSiblings(content []byte, members map[string]Version) ([]byte, bool)
	// CheckSiblings reports a one-line reason for each dependency on a member that pins a
	// version other than that member's target (in members).
	CheckSiblings(content []byte, members map[string]Version) []string
}

// wsGroup collects, per manifest format, its Workspace manager, each member name mapped to
// the version it must carry (resolved per package), and each manifest's content keyed by
// repo-relative path.
type wsGroup struct {
	ws      Workspace
	members map[string]Version
	files   map[string][]byte
}

// gatherWorkspaces walks units and, for every manifest whose Manager is Workspace-capable,
// reads it through read, records its package identity mapped to its resolved target version,
// and groups it by format. read returns (content, found, err); a not-found manifest is
// skipped. Grouping by format keeps each language's members separate — a Cargo member never
// reconciles against a pubspec member. A member whose unit has no configured version is left
// out of the member map (nothing to reconcile it to).
func gatherWorkspaces(units []ManifestUnit, res *Resolver, read func(rel string) ([]byte, bool, error)) (map[string]*wsGroup, error) {
	groups := map[string]*wsGroup{}
	for _, u := range units {
		for _, fname := range u.Manifests {
			m, ok := ManagerFor(fname)
			if !ok {
				continue
			}
			ws, ok := m.(Workspace)
			if !ok {
				continue // this format has no sibling-dependency concept
			}
			rel := path.Join(u.Dir, fname)
			g := groups[fname]
			if g == nil {
				g = &wsGroup{ws: ws, members: map[string]Version{}, files: map[string][]byte{}}
				groups[fname] = g
			}
			if _, seen := g.files[rel]; seen {
				continue
			}
			data, found, err := read(rel)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", rel, err)
			}
			if !found {
				continue
			}
			g.files[rel] = data
			if id, ok := ws.PackageID(data); ok {
				want, vok, err := res.targetVersion(u.Dir)
				if err != nil {
					return nil, fmt.Errorf("version for %s: %w", u.Dir, err)
				}
				if vok {
					g.members[id] = want
				}
			}
		}
	}
	return groups, nil
}

// sortedKeys returns a map's keys sorted, for stable iteration/output.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// CheckWorkspace is the language-agnostic multi-package honesty assertion: for every detected
// manifest whose Manager is Workspace-capable, it gathers the package identities of all
// manifests of that format, then reports each intra-repo dependency constraint that disagrees
// with its target. It complements CheckManifests (each manifest's own version): a stale sibling
// pin looks individually honest. Each member is reconciled to *its own* resolved version, so
// independently-versioned packages are honored. A nil resolver or no units is a no-op.
func CheckWorkspace(fsys fs.FS, res *Resolver, units []ManifestUnit) ([]ManifestDrift, error) {
	if res == nil || len(units) == 0 {
		return nil, nil
	}
	groups, err := gatherWorkspaces(units, res, func(rel string) ([]byte, bool, error) {
		data, err := fs.ReadFile(fsys, rel)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return data, err == nil, err
	})
	if err != nil {
		return nil, err
	}
	var drifts []ManifestDrift
	for _, fname := range sortedKeys(groups) {
		g := groups[fname]
		for _, rel := range sortedKeys(g.files) {
			for _, reason := range g.ws.CheckSiblings(g.files[rel], g.members) {
				drifts = append(drifts, ManifestDrift{Path: rel, Detail: reason})
			}
		}
	}
	return drifts, nil
}

// RewriteWorkspace is the mutating sibling of CheckWorkspace: across every Workspace-capable
// manifest format it sets each intra-repo dependency constraint to the depended member's
// resolved version, writing only changed files and returning their repo-relative paths. It
// assumes each manifest's own version: was already set by the generic manifest pass; it
// touches only sibling constraints.
func RewriteWorkspace(root string, res *Resolver, units []ManifestUnit) ([]string, error) {
	groups, err := gatherWorkspaces(units, res, func(rel string) ([]byte, bool, error) {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return data, err == nil, err
	})
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, fname := range sortedKeys(groups) {
		g := groups[fname]
		for _, rel := range sortedKeys(g.files) {
			out, did := g.ws.SetSiblings(g.files[rel], g.members)
			if !did {
				continue
			}
			if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), out, 0o644); err != nil {
				return changed, err
			}
			changed = append(changed, rel)
		}
	}
	return changed, nil
}

// setVia rewrites the first capture group of re (which must capture the version literal)
// to v across content, reporting whether it changed. It is the shared core of every regex
// manager: the manager only supplies the locating regex. A no-match is an error so a bump
// never silently leaves a manifest behind.
func setVia(content []byte, re *regexp.Regexp, v Version, what string) ([]byte, bool, error) {
	loc := re.FindSubmatchIndex(content)
	if loc == nil {
		return nil, false, fmt.Errorf("no %s found", what)
	}
	// loc[2]:loc[3] is the first submatch — the version literal to replace.
	start, end := loc[2], loc[3]
	if string(content[start:end]) == v.String() {
		return content, false, nil
	}
	out := make([]byte, 0, len(content)+len(v.String()))
	out = append(out, content[:start]...)
	out = append(out, v.String()...)
	out = append(out, content[end:]...)
	return out, true, nil
}

// readVia reads the first capture group of re (the version literal) from content and parses
// it, the read mirror of setVia. It reports false when re does not match or the literal is
// not a clean X.Y.Z (e.g. a "v"-prefixed tag is accepted via Parse). It is the shared core
// of every regex manager's ReadVersion: the manager supplies only its locating regex.
func readVia(content []byte, re *regexp.Regexp) (Version, bool) {
	loc := re.FindSubmatchIndex(content)
	if loc == nil {
		return Version{}, false
	}
	v, err := Parse(string(content[loc[2]:loc[3]]))
	if err != nil {
		return Version{}, false
	}
	return v, true
}

// DetectVersion returns the project's own declared version, as a string, read from the first
// of the given units whose manifest a registered Manager can locate — so `cairn init` seeds
// cairn.yaml from the real project version (a pom's <version>, a package.json's "version", …)
// rather than a placeholder. Units are tried in order and, within a unit, manifests in their
// declared order; the first locatable version wins. A missing file, an unregistered manifest,
// or a manifest stating no own version is skipped. It reports false when nothing declares a
// version, leaving the caller to fall back to its default.
func DetectVersion(fsys fs.FS, units []ManifestUnit) (string, bool) {
	for _, u := range units {
		for _, fname := range u.Manifests {
			m, ok := ManagerFor(fname)
			if !ok {
				continue
			}
			data, err := fs.ReadFile(fsys, path.Join(u.Dir, fname))
			if err != nil {
				continue
			}
			if v, ok := m.ReadVersion(data); ok {
				return v.String(), true
			}
		}
	}
	return "", false
}
