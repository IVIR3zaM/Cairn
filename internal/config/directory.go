package config

// Directory is the per-directory override block: the repo-baseline settings, each
// optional (a nil pointer — or an absent map entry — means "unset, inherit the lower
// layer"). One type serves three forms (the root file's baseline, a root
// directories.<path> entry, and a directory's own cairn.yaml); 10a-ii layers them with
// cascade. See docs/ARCHITECTURE.md "per-directory config & precedence".
type Directory struct {
	// Enabled is the absolute disable gate: a false at any layer prunes the subtree
	// (10a-ii reads it before any merge or file read). Nil inherits.
	Enabled *bool `yaml:"enabled,omitempty"`
	// Version, when set, makes this directory independently versioned (own tag/changelog);
	// nil inherits the repo baseline version (lockstep).
	Version     *string             `yaml:"version,omitempty"`
	Versioning  *string             `yaml:"versioning,omitempty"`
	Languages   map[string]Language `yaml:"languages,omitempty"`
	Verify      *Verify             `yaml:"verify,omitempty"`
	Commits     *Commits            `yaml:"commits,omitempty"`
	Changelog   *Changelog          `yaml:"changelog,omitempty"`
	VersionSync *VersionSync        `yaml:"version_sync,omitempty"`
	Hooks       *Hooks              `yaml:"hooks,omitempty"`
	CI          *CI                 `yaml:"ci,omitempty"`
	Addons      *Addons             `yaml:"addons,omitempty"`
}

// overlay folds over onto base field-by-field: a field set in over wins, an unset one
// inherits base. Languages merge by name (an over entry replaces base's for that name,
// other names survive) so a directory can tweak one language's knobs without restating
// the rest. The single low→high step the cascade is built from.
func overlay(base, over Directory) Directory {
	out := base
	if over.Enabled != nil {
		out.Enabled = over.Enabled
	}
	if over.Version != nil {
		out.Version = over.Version
	}
	if over.Versioning != nil {
		out.Versioning = over.Versioning
	}
	if len(over.Languages) > 0 {
		merged := make(map[string]Language, len(base.Languages)+len(over.Languages))
		for k, v := range base.Languages {
			merged[k] = v
		}
		for k, v := range over.Languages {
			merged[k] = v
		}
		out.Languages = merged
	}
	if over.Verify != nil {
		out.Verify = over.Verify
	}
	if over.Commits != nil {
		out.Commits = over.Commits
	}
	if over.Changelog != nil {
		out.Changelog = over.Changelog
	}
	if over.VersionSync != nil {
		out.VersionSync = over.VersionSync
	}
	if over.Hooks != nil {
		out.Hooks = over.Hooks
	}
	if over.CI != nil {
		out.CI = over.CI
	}
	if over.Addons != nil {
		out.Addons = over.Addons
	}
	return out
}

// VerifyOrDefault returns this resolved directory's verify stage config, falling back to
// the in-code defaults when no layer set it. The CLI reads it instead of re-deriving the
// cascade, so verify carries no precedence logic of its own.
func (d Directory) VerifyOrDefault() Verify {
	if d.Verify != nil {
		return *d.Verify
	}
	return Default().Verify
}

// StrictFor reports the effective strictness for a language in this resolved directory:
// the per-language override (languages.<name>.strict) when set, otherwise the resolved
// verify.strict default. The Tree-resolved mirror of Config.StrictFor — a single
// resolution point so the CLI never re-derives the inherit-vs-override precedence.
func (d Directory) StrictFor(lang string) bool {
	if l, ok := d.Languages[lang]; ok && l.Strict != nil {
		return *l.Strict
	}
	return d.VerifyOrDefault().Strict
}

// cascade folds layers low → high so the nearest (last) layer that sets a field wins and
// any field no layer sets stays unset. Callers pass the precedence order (repo baseline,
// then own-file ancestors outer→inner, then root directories.<path> ancestors) and read
// the single resolved block.
func cascade(layers ...Directory) Directory {
	var out Directory
	for _, l := range layers {
		out = overlay(out, l)
	}
	return out
}
