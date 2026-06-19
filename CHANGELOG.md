# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-06-19

First public release. Cairn is a polyglot orchestrator: one config, one git hook, and one
CI job that unify quality, versioning, changelog, and commit hygiene by shelling out to the
best in-market tool for each language. (This release squashes the iterative build history —
the design and ports & adapters/DDD-light architecture are in
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).)

### Added
- **`cairn init`** — onboarding in one command: detects the languages present, writes a
  self-documenting, *discovered* `cairn.yaml` (records only facts it can prove — version,
  languages, README `version_sync` patterns, commit policy from git history), installs the git
  hooks, and generates the CI workflow. Runs an interactive wizard on a TTY (findings →
  multiselect standards/features, choices sourced live from the registries) or
  `--yes` non-interactively; non-destructive (keeps an existing `cairn.yaml`).
- **`cairn verify`** — the unified quality gate. Per detected language it runs an ordered plan
  (format → lint → typecheck → test) by shelling out to that language's standard tools:
  **Go** (gofumpt, golangci-lint, `go test`), **Rust** (cargo fmt/clippy/test), **Python** (ruff
  or black+flake8), **JS/TS** (eslint+prettier or biome via npx), **Java** (`mvn verify`/`gradle
  check`, wrapper-aware), **Dart** (dart format/analyze/test). A missing tool fails a `required`
  stage or warns-and-skips an optional one with an install hint; `--fix` re-runs every fixable
  stage and prints honest, non-over-promising fix hints; `--verbose` streams each tool's own
  (colored) output; a live spinner and a per-stage timeout keep it from ever looking frozen.
- **Version & doc honesty** — Cairn's signature check. `cairn verify` asserts every version
  manifest, multi-package workspace interdependency, and configured `version_sync` doc pattern
  quotes the right version, failing on drift. SemVer + CalVer math underneath.
- **`cairn bump`** — computes the next version (SemVer level / explicit / CalVer date-step, or
  **inferred from commit history** via the commit convention), then updates every manifest,
  rewrites `version_sync` docs, reconciles workspace interdependencies, promotes the changelog,
  and prints a suggested release commit/tag — never committing. Interactive wizard with a
  downgrade safeguard; refuses empty/downgrade bumps (`--force` to override a deliberate
  downgrade).
- **`cairn doctor`** — per-language installed-vs-missing tool table with install hints.
- **Per-directory config model** — one `cairn.yaml` schema (`schema: "2"`): repo-baseline
  top-level keys plus a `directories:` map of override blocks (and any directory may carry its
  own `cairn.yaml`). `config` owns the field-level low→high cascade (baseline < own-file < root
  override), an absolute `enabled: false` disable gate, and `Resolve(dir)`; a directory with its
  own `version:` is independently versioned (own tag/changelog), otherwise it rides the repo
  version in lockstep. Supports mixed-language monorepos with independent per-package versions.
- **Commit conventions** — `conventional` validator classifies messages into the SemVer bump
  they imply and powers bump inference; the generated `commit-msg` hook runs `cairn commit-lint`.
- **Changelog promotion** — `cairn bump` promotes `[Unreleased]` → a dated release (refreshing
  compare links for Keep a Changelog), with `keepachangelog` and `dart` (pub.dev) writers;
  refuses a release with empty notes.
- **Wiring** — installs git hooks into a tracked `.cairn/hooks` dir (via `core.hooksPath`) and
  generates a CI workflow, so the local hook and CI run the same `cairn verify`. Cairn
  **dogfoods itself**: its own `cairn.yaml` drives its pre-commit hook and CI.
- **Pluggable by self-registration** — every multi-implementation choice is a registry whose
  entries self-register in `init()` (languages for detection & quality, version manifest
  managers, multi-package `Workspace` capability, changelog standards, commit conventions, CI
  providers). Adding a language is two files; adding a standard/provider is one — no engine,
  `switch`, or central map edits.

[Unreleased]: https://github.com/IVIR3zaM/Cairn/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/IVIR3zaM/Cairn/releases/tag/v0.1.0
