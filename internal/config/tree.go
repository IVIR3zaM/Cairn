package config

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is the current config format marker. The repo baseline version now lives
// in the top-level `version:` key (a project version like "1.2.3"), so the *format* version
// moved to its own `schema:` key — default "2" when absent. A legacy `version: "1"` / `project:`
// file is still accepted and translated (see loadLegacyTree); it is never silently misread.
const SchemaVersion = "2"

// Tree is the resolved per-directory config model (schema 2). It holds the repo baseline
// (root top-level keys), the root `directories.<path>` override entries (highest authority),
// and the discovered per-directory `<path>/cairn.yaml` blocks, and answers Resolve(dir) by
// folding them with the field-level cascade. config owns this complexity so verify/bump/detect
// ask for resolved settings instead of re-deriving precedence or reading YAML themselves.
type Tree struct {
	baseline Directory            // repo baseline: root top-level keys
	rootDirs map[string]Directory // root directories.<path> entries (layer 3, highest)
	ownFiles map[string]Directory // discovered <path>/cairn.yaml blocks (layer 2)
	pruned   map[string]bool      // paths cut by the absolute disable gate
}

// rootDoc is the schema-2 root cairn.yaml: the inline baseline override block plus the
// `directories:` map of per-path overrides. A nested `<path>/cairn.yaml` unmarshals into the
// same shape and contributes only its inline block (its `directories:` is ignored — a nested
// file is just an override block). The inline baseline is declared before Directories so it
// marshals first — the repo-wide settings read at the top of the file, the per-path overrides
// at the end.
type rootDoc struct {
	Schema      string `yaml:"schema,omitempty"`
	Directory   `yaml:",inline"`
	Directories map[string]Directory `yaml:"directories,omitempty"`
}

// LoadTree reads the repo's cairn.yaml from fsys (rooted at the repo root), discovers nested
// `<path>/cairn.yaml` files — pruning any subtree cut by the absolute disable gate before its
// own file is ever read — and returns the resolved Tree. Pass os.DirFS(repoRoot) in production.
func LoadTree(fsys fs.FS) (*Tree, error) {
	data, err := fs.ReadFile(fsys, "cairn.yaml")
	if errors.Is(err, fs.ErrNotExist) {
		return defaultTree(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read cairn.yaml: %w", err)
	}

	t, err := parseRootTree(data)
	if err != nil {
		return nil, err
	}
	if err := t.discover(fsys); err != nil {
		return nil, err
	}
	return t, nil
}

// defaultTree is the no-file case: the in-code defaults as the repo baseline, no overrides.
func defaultTree() *Tree {
	return &Tree{
		baseline: baselineFromConfig(Default()),
		rootDirs: map[string]Directory{},
		ownFiles: map[string]Directory{},
		pruned:   map[string]bool{},
	}
}

// parseRootTree decides schema-2 vs legacy and builds the Tree's baseline + root directory
// entries. A legacy file is translated (not misread); a schema-2 file is parsed directly.
func parseRootTree(data []byte) (*Tree, error) {
	var probe struct {
		Schema  string    `yaml:"schema"`
		Version string    `yaml:"version"`
		Project yaml.Node `yaml:"project"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("parse cairn.yaml: %w", err)
	}
	if probe.Schema == "" && (probe.Version == "1" || !probe.Project.IsZero()) {
		return loadLegacyTree(data)
	}

	var doc rootDoc
	// Seed the baseline with the in-code defaults so a *partial* top-level block (e.g.
	// `verify: { strict: true }`) merges onto them field-by-field rather than wiping the
	// stages it doesn't mention — mirroring the legacy Default()+unmarshal merge and the
	// no-file defaultTree(). Only the baseline is seeded; directories.<path> override
	// entries stay nil ⇒ "inherit", so the cascade semantics are unchanged.
	doc.Directory = baselineFromConfig(Default())
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse cairn.yaml: %w", err)
	}
	if doc.Schema != "" && doc.Schema != SchemaVersion {
		return nil, fmt.Errorf("invalid cairn.yaml: schema: unsupported %q (expected %q)", doc.Schema, SchemaVersion)
	}
	if err := validateDirectories(doc.Directories); err != nil {
		return nil, fmt.Errorf("invalid cairn.yaml: %w", err)
	}
	t := &Tree{
		baseline: doc.Directory,
		rootDirs: cleanDirKeys(doc.Directories),
		ownFiles: map[string]Directory{},
		pruned:   map[string]bool{},
	}
	return t, nil
}

// legacyProject is the dropped schema-1 `project:` block. It lives here, not on the Config
// aggregate (10a-iii-c-iii removed it), so the legacy-translation path can still read a
// `version: "1"` file's canonical version, scheme, and packages and translate them into the
// schema-2 Tree — never silently misreading an old file.
type legacyProject struct {
	CanonicalVersion string          `yaml:"canonical_version"`
	Versioning       string          `yaml:"versioning"`
	Packages         []legacyPackage `yaml:"packages"`
}

type legacyPackage struct {
	Path       string `yaml:"path"`
	Version    string `yaml:"version"`
	Versioning string `yaml:"versioning"`
}

// loadLegacyTree translates a `version: "1"` / `project:` config into the schema-2 Tree:
// the legacy top-level keys (validated via the Config aggregate) become the baseline, the
// dropped project block is parsed on the side, and each project.packages entry becomes a
// root directories.<path> override carrying its own version.
func loadLegacyTree(data []byte) (*Tree, error) {
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse cairn.yaml: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid cairn.yaml: %w", err)
	}
	var legacy struct {
		Project legacyProject `yaml:"project"`
	}
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("parse cairn.yaml: %w", err)
	}
	proj := legacy.Project
	if proj.Versioning != "" && !oneOf(proj.Versioning, "semver", "calver") {
		return nil, fmt.Errorf("invalid cairn.yaml: project.versioning: %q is not one of [semver calver]", proj.Versioning)
	}

	baseline := baselineFromConfig(cfg)
	if proj.CanonicalVersion != "" {
		baseline.Version = strPtr(proj.CanonicalVersion)
	}
	if proj.Versioning != "" {
		baseline.Versioning = strPtr(proj.Versioning)
	}
	rootDirs := map[string]Directory{}
	for i, p := range proj.Packages {
		if strings.TrimSpace(p.Path) == "" || strings.TrimSpace(p.Version) == "" {
			return nil, fmt.Errorf("invalid cairn.yaml: project.packages[%d]: path and version must not be empty", i)
		}
		if p.Versioning != "" && !oneOf(p.Versioning, "semver", "calver") {
			return nil, fmt.Errorf("invalid cairn.yaml: project.packages[%d].versioning: %q is not one of [semver calver]", i, p.Versioning)
		}
		d := Directory{Version: strPtr(p.Version)}
		if p.Versioning != "" {
			d.Versioning = strPtr(p.Versioning)
		}
		rootDirs[path.Clean(p.Path)] = d
	}
	return &Tree{
		baseline: baseline,
		rootDirs: rootDirs,
		ownFiles: map[string]Directory{},
		pruned:   map[string]bool{},
	}, nil
}

// baselineFromConfig lifts a Config's tool/standard knobs into a Directory baseline block
// (every section a pointer so "unset" inherits — though the baseline sets all of them). The
// baseline versioning defaults to semver; the legacy-translation path overrides version and
// versioning from the dropped project block.
func baselineFromConfig(cfg *Config) Directory {
	return Directory{
		Languages:   cfg.Languages,
		Verify:      &cfg.Verify,
		Commits:     &cfg.Commits,
		Changelog:   &cfg.Changelog,
		VersionSync: &cfg.VersionSync,
		Hooks:       &cfg.Hooks,
		CI:          &cfg.CI,
		Addons:      &cfg.Addons,
		Versioning:  strPtr("semver"),
	}
}

// discover walks fsys, recording each `<path>/cairn.yaml` override block. The absolute disable
// gate is honored first: a gated directory (a root directories entry with enabled:false, or any
// gated ancestor) is pruned and never descended into, so its own cairn.yaml is never read.
func (t *Tree) discover(fsys fs.FS) error {
	// Seed pruned with declared-but-absent gated directories so enumeration is complete.
	for p, d := range t.rootDirs {
		if d.Enabled != nil && !*d.Enabled {
			t.pruned[p] = true
		}
	}
	return fs.WalkDir(fsys, ".", func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !de.IsDir() || p == "." {
			return nil
		}
		if t.gated(p) {
			t.pruned[p] = true
			return fs.SkipDir
		}
		data, err := fs.ReadFile(fsys, path.Join(p, "cairn.yaml"))
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		var doc rootDoc
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parse %s/cairn.yaml: %w", p, err)
		}
		t.ownFiles[p] = doc.Directory
		return nil
	})
}

// Resolve folds the layers governing dir low → high — repo baseline, then own-file ancestors
// outer → inner, then root directories.<path> ancestors outer → inner (highest authority) — so
// the nearest layer that sets a field wins. The bool is false when dir is pruned by the disable
// gate, in which case nothing under it runs.
func (t *Tree) Resolve(dir string) (Directory, bool) {
	dir = path.Clean(dir)
	if t.gated(dir) {
		return Directory{}, false
	}
	layers := []Directory{t.baseline}
	chain := ancestors(dir)
	for _, a := range chain {
		if d, ok := t.ownFiles[a]; ok {
			layers = append(layers, d)
		}
	}
	for _, a := range chain {
		if d, ok := t.rootDirs[a]; ok {
			layers = append(layers, d)
		}
	}
	return cascade(layers...), true
}

// Active lists every directory carrying config (a root entry or its own file) that survives the
// disable gate, sorted. Pruned lists the gated directories. Together they enumerate the tree.
func (t *Tree) Active() []string {
	seen := map[string]bool{}
	for p := range t.rootDirs {
		seen[p] = true
	}
	for p := range t.ownFiles {
		seen[p] = true
	}
	var out []string
	for p := range seen {
		if !t.pruned[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// Independent lists the directories that declare their own version: ones whose own override
// layer (a root directories.<path> entry or that directory's own cairn.yaml) sets `version`,
// making them independently versioned (own tag/changelog) rather than inheriting the repo
// baseline (lockstep). It is the schema-2 successor to project.packages — bump enumerates the
// release units from it and reads each unit's target version/scheme via Resolve(dir), instead
// of walking a config list. Sorted, and excluding any directory pruned by the disable gate.
func (t *Tree) Independent() []string {
	seen := map[string]bool{}
	for p, d := range t.rootDirs {
		if d.Version != nil {
			seen[path.Clean(p)] = true
		}
	}
	for p, d := range t.ownFiles {
		if d.Version != nil {
			seen[path.Clean(p)] = true
		}
	}
	var out []string
	for p := range seen {
		if !t.pruned[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// WithVersion returns a shallow copy of the Tree with the target version for dir set to v,
// representing the post-bump state a bump resolves manifests/docs/workspace against. dir "."
// sets the repo baseline (so lockstep units resolve to v); any other dir sets that directory's
// own override (so only it — and its workspace dependents — move, while every other unit keeps
// its current version and is a no-op). The original Tree is left unchanged; ownFiles/pruned are
// shared read-only and rootDirs is cloned before a per-directory set.
func (t *Tree) WithVersion(dir, v string) *Tree {
	dir = path.Clean(dir)
	nt := &Tree{
		baseline: t.baseline,
		rootDirs: make(map[string]Directory, len(t.rootDirs)),
		ownFiles: t.ownFiles,
		pruned:   t.pruned,
	}
	for k, d := range t.rootDirs {
		nt.rootDirs[k] = d
	}
	if dir == "." {
		nt.baseline.Version = &v
	} else {
		d := nt.rootDirs[dir]
		d.Version = &v
		nt.rootDirs[dir] = d
	}
	return nt
}

// Pruned lists the directories cut by the absolute disable gate, sorted.
func (t *Tree) Pruned() []string {
	out := make([]string, 0, len(t.pruned))
	for p := range t.pruned {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// gated reports whether dir, or any ancestor, is cut by a root directories.<path>.enabled:false.
// The gate lives in the root file alone, so it is knowable before any directory's own file is read.
func (t *Tree) gated(dir string) bool {
	for _, a := range ancestors(dir) {
		if d, ok := t.rootDirs[a]; ok && d.Enabled != nil && !*d.Enabled {
			return true
		}
	}
	return false
}

// ancestors returns dir and each of its path prefixes, outer → inner (excluding the repo root
// ".", whose settings are the baseline). E.g. "a/b/c" → ["a", "a/b", "a/b/c"].
func ancestors(dir string) []string {
	dir = path.Clean(dir)
	if dir == "." || dir == "" {
		return nil
	}
	parts := strings.Split(dir, "/")
	out := make([]string, 0, len(parts))
	for i := range parts {
		out = append(out, path.Join(parts[:i+1]...))
	}
	return out
}

// validateDirectories reports actionable errors for the root directories map: a per-path
// version override must be non-empty, and a versioning override must be a known scheme.
func validateDirectories(dirs map[string]Directory) error {
	var problems []string
	for _, p := range sortedDirKeys(dirs) {
		d := dirs[p]
		if strings.TrimSpace(p) == "" {
			problems = append(problems, "directories: path key must not be empty")
		}
		if d.Version != nil && strings.TrimSpace(*d.Version) == "" {
			problems = append(problems, fmt.Sprintf("directories.%s.version: must not be empty when set", p))
		}
		if d.Versioning != nil && !oneOf(*d.Versioning, "semver", "calver") {
			problems = append(problems, fmt.Sprintf("directories.%s.versioning: %q is not one of [semver calver]", p, *d.Versioning))
		}
	}
	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}

// cleanDirKeys normalizes path keys (path.Clean) so lookups match ancestors() output.
func cleanDirKeys(dirs map[string]Directory) map[string]Directory {
	out := make(map[string]Directory, len(dirs))
	for p, d := range dirs {
		out[path.Clean(p)] = d
	}
	return out
}

func sortedDirKeys(m map[string]Directory) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func strPtr(s string) *string { return &s }
