// Package config is the aggregate root: it loads, validates, and default-merges
// cairn.yaml into a typed Config that every other context reads (read-only).
// Defaults live here so a minimal cairn.yaml — or none at all — still works.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the whole cairn.yaml aggregate. See docs/ARCHITECTURE.md for the schema.
type Config struct {
	Version     string              `yaml:"version"`
	Project     Project             `yaml:"project"`
	Languages   map[string]Language `yaml:"languages"`
	Verify      Verify              `yaml:"verify"`
	Commits     Commits             `yaml:"commits"`
	Changelog   Changelog           `yaml:"changelog"`
	VersionSync VersionSync         `yaml:"version_sync"`
	Hooks       Hooks               `yaml:"hooks"`
	CI          CI                  `yaml:"ci"`
	Addons      Addons              `yaml:"addons"`
}

// Project carries the canonical version (source of truth for version-sync) and scheme.
type Project struct {
	CanonicalVersion string `yaml:"canonical_version"`
	Versioning       string `yaml:"versioning"`
}

// Language is one detected/enabled language unit.
type Language struct {
	Dir      string `yaml:"dir"`
	Enabled  bool   `yaml:"enabled"`
	Standard string `yaml:"standard,omitempty"`
}

// Step is a toggleable verify stage (format/lint/typecheck/test/build).
type Step struct {
	Enabled  bool   `yaml:"enabled"`
	Required bool   `yaml:"required"`
	Mode     string `yaml:"mode,omitempty"`
}

// Verify holds the global stage toggles and the per-stage timeout.
type Verify struct {
	Format    Step   `yaml:"format"`
	Lint      Step   `yaml:"lint"`
	Typecheck Step   `yaml:"typecheck"`
	Test      Step   `yaml:"test"`
	Build     Step   `yaml:"build"`
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

// Commits configures convention validation.
type Commits struct {
	Convention   string `yaml:"convention"`
	Signoff      bool   `yaml:"signoff"`
	ValidateHook bool   `yaml:"validate_hook"`
}

// Changelog selects the changelog standard and file.
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
		Project:   Project{Versioning: "semver"},
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
	cfg.normalize()
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

// normalize fills small structural defaults that depend on user input, e.g. an
// enabled language with no dir defaults to ".".
func (c *Config) normalize() {
	for name, lang := range c.Languages {
		if lang.Enabled && lang.Dir == "" {
			lang.Dir = "."
			c.Languages[name] = lang
		}
	}
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
	if !oneOf(c.Project.Versioning, "semver", "calver") {
		add("project.versioning: %q is not one of [semver calver]", c.Project.Versioning)
	}
	if !oneOf(c.Commits.Convention, "conventional", "gitmoji", "none") {
		add("commits.convention: %q is not one of [conventional gitmoji none]", c.Commits.Convention)
	}
	if !oneOf(c.Changelog.Standard, "keepachangelog", "git-cliff", "conventional-changelog") {
		add("changelog.standard: %q is not one of [keepachangelog git-cliff conventional-changelog]", c.Changelog.Standard)
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
	for _, name := range sortedKeys(c.Languages) {
		if c.Languages[name].Enabled && c.Languages[name].Dir == "" {
			add("languages.%s.dir: must not be empty when enabled", name)
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

func sortedKeys(m map[string]Language) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
