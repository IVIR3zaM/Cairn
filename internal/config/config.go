// Package config is the aggregate root: it loads, validates, and default-merges
// cairn.yaml into a typed Config that every other context reads (read-only).
// Defaults live here so a minimal cairn.yaml — or none at all — still works.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the legacy (schema-1) flat aggregate. The per-directory model (schema 2) is the
// Tree in tree.go; this type survives as the shape the legacy-translation path unmarshals into
// (its tool/standard knobs become the Tree baseline) and as the in-code Default() source.
// See docs/ARCHITECTURE.md for the schema.
type Config struct {
	Version     string              `yaml:"version"`
	Languages   map[string]Language `yaml:"languages"`
	Verify      Verify              `yaml:"verify"`
	Commits     Commits             `yaml:"commits"`
	Changelog   Changelog           `yaml:"changelog"`
	VersionSync VersionSync         `yaml:"version_sync"`
	Hooks       Hooks               `yaml:"hooks"`
	CI          CI                  `yaml:"ci"`
	Addons      Addons              `yaml:"addons"`
}

// Language is one language's tool/standard knobs (never its location — detection owns where
// languages are). Strict overrides the repo-wide verify.strict default for just this language;
// nil (the absent case) inherits it.
type Language struct {
	Enabled  bool   `yaml:"enabled"`
	Standard string `yaml:"standard,omitempty"`
	Strict   *bool  `yaml:"strict,omitempty"`
}

// Step is a toggleable verify stage (format/lint/typecheck/test/build).
type Step struct {
	Enabled  bool   `yaml:"enabled"`
	Required bool   `yaml:"required"`
	Mode     string `yaml:"mode,omitempty"`
}

// Verify holds the global stage toggles and the per-stage timeout. Strict is the
// repo-wide default for strict mode (see StrictFor), which asks each adapter to
// promote its most lenient diagnostics — analyzer infos, linter warnings — to
// failures instead of leaving them advisory.
type Verify struct {
	Format    Step   `yaml:"format"`
	Lint      Step   `yaml:"lint"`
	Typecheck Step   `yaml:"typecheck"`
	Test      Step   `yaml:"test"`
	Build     Step   `yaml:"build"`
	Strict    bool   `yaml:"strict,omitempty"`
	Timeout   string `yaml:"timeout,omitempty"`
}

// StepTimeout is the per-stage deadline parsed from Timeout; zero (the empty or
// unparseable case) means no deadline. It bounds each tool invocation so a hung
// command — e.g. a build downloading dependencies — fails instead of freezing verify.
func (v Verify) StepTimeout() time.Duration {
	d, err := time.ParseDuration(v.Timeout)
	if err != nil {
		return 0
	}
	return d
}

// StrictFor reports the effective strictness for a language: the per-language
// override (languages.<name>.strict) when set, otherwise the repo-wide default
// (verify.strict). It is the single resolution point so the CLI never has to
// re-derive the inherit-vs-override precedence.
func (c *Config) StrictFor(lang string) bool {
	if l, ok := c.Languages[lang]; ok && l.Strict != nil {
		return *l.Strict
	}
	return c.Verify.Strict
}

// Commits configures convention validation.
type Commits struct {
	Convention   string `yaml:"convention"`
	Signoff      bool   `yaml:"signoff"`
	ValidateHook bool   `yaml:"validate_hook"`
}

// Changelog selects the changelog standard and file. In the per-directory model an
// independently-versioned directory carries its own changelog via its override block, so a
// repo-wide `packages` style is no longer needed here.
type Changelog struct {
	Standard string `yaml:"standard"`
	File     string `yaml:"file"`
}

// VersionSyncFile is one doc whose version patterns must stay honest.
type VersionSyncFile struct {
	Path     string   `yaml:"path"`
	Patterns []string `yaml:"patterns"`
}

// VersionSync is Cairn's signature doc-honesty check configuration.
type VersionSync struct {
	Files []VersionSyncFile `yaml:"files"`
}

// Hooks lists the cairn jobs wired into each git hook.
type Hooks struct {
	PreCommit []string `yaml:"pre_commit"`
	CommitMsg []string `yaml:"commit_msg"`
	PrePush   []string `yaml:"pre_push"`
}

// CI configures generated continuous-integration workflows.
type CI struct {
	Provider string   `yaml:"provider"`
	Jobs     []string `yaml:"jobs"`
}

// Addons are optional convenience features.
type Addons struct {
	EditorConfig  bool `yaml:"editorconfig"`
	LicenseHeader bool `yaml:"license_header"`
	BranchName    bool `yaml:"branch_name"`
}

// Default returns a Config with every in-code default applied. Unmarshalling a
// cairn.yaml on top of this yields the default-merge: keys present in the file
// override; absent keys keep these values.
func Default() *Config {
	return &Config{
		Version:   "1",
		Languages: map[string]Language{},
		Verify: Verify{
			Format:    Step{Enabled: true, Required: true, Mode: "check"},
			Lint:      Step{Enabled: true, Required: true},
			Typecheck: Step{Enabled: true, Required: false},
			Test:      Step{Enabled: true, Required: true},
			Build:     Step{Enabled: false},
			Timeout:   "5m",
		},
		Commits:   Commits{Convention: "conventional", ValidateHook: true},
		Changelog: Changelog{Standard: "keepachangelog", File: "CHANGELOG.md"},
		Hooks:     Hooks{PreCommit: []string{"verify"}, CommitMsg: []string{"commit-lint"}},
		CI:        CI{Provider: "github", Jobs: []string{"verify"}},
	}
}

// Load reads, default-merges, normalizes, and validates a cairn.yaml at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config %s: %w", path, err)
	}
	return cfg, nil
}

// LoadOrDefault loads path, or returns the in-code defaults when no file exists.
func LoadOrDefault(path string) (*Config, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	return Load(path)
}

// Validate reports every problem at once with an actionable message.
func (c *Config) Validate() error {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	if c.Version != "1" {
		add("version: unsupported %q (expected \"1\")", c.Version)
	}
	if !oneOf(c.Commits.Convention, "conventional", "gitmoji", "none") {
		add("commits.convention: %q is not one of [conventional gitmoji none]", c.Commits.Convention)
	}
	if !oneOf(c.Changelog.Standard, "keepachangelog", "dart", "git-cliff", "conventional-changelog") {
		add("changelog.standard: %q is not one of [keepachangelog dart git-cliff conventional-changelog]", c.Changelog.Standard)
	}
	for _, s := range []struct {
		name string
		mode string
	}{{"format", c.Verify.Format.Mode}, {"test", c.Verify.Test.Mode}} {
		if s.mode != "" && !oneOf(s.mode, "check", "fix") {
			add("verify.%s.mode: %q is not one of [check fix]", s.name, s.mode)
		}
	}
	if c.Verify.Timeout != "" {
		if _, err := time.ParseDuration(c.Verify.Timeout); err != nil {
			add("verify.timeout: %q is not a valid duration (e.g. \"90s\", \"5m\")", c.Verify.Timeout)
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, "; "))
}

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}
