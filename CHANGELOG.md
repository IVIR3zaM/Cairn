# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Quality adapters are now self-registering, one file per language
  (`internal/quality/lang_<name>.go`) inside the `quality` package, mirroring detection.
  Each calls `register(name, ctor)` in `init()` and `cairn verify` resolves them via
  `quality.AdapterFor` — adding a language no longer edits the CLI's `adapters` map (now
  removed). Shared step plumbing and exit-code helpers moved to `adapter.go` (ADR-006).
- Detection languages are now pluggable, one self-registering file per language
  (`internal/detect/lang_<name>.go`) instead of a hardcoded central list. Each file
  owns its markers, tools, and skip dirs and calls `register(...)` from `init()`;
  adding a language is adding a file, with no edits to the detection engine.

### Added
- Rust quality adapter (`internal/quality/lang_rust.go`) wrapping `cargo fmt`, `cargo
  clippy` (warnings as errors), and `cargo test`; self-registered into `cairn verify`.
- QualityGate (`internal/quality`) and a Go adapter (`internal/quality/go`): `cairn
  verify` builds an ordered per-language plan (format → lint → typecheck → test →
  build), runs each stage's tool through the `ToolRunner`, and renders a compact
  summary (exit non-zero on failure). Disabled stages are omitted; a missing tool
  fails a `required` stage or warns-and-skips an optional one with an install hint.
  The Go adapter wraps gofumpt, golangci-lint, and `go test`.
- Cross-cutting ports: `ToolRunner` (`internal/runner`) with an `Exec` adapter that
  captures stdout/stderr/exit-code and honors cwd + timeout, plus a `Fake` for tests;
  and the `Reporter` UX port (`internal/report`) rendering glyph steps and a compact
  summary, honoring color/`NO_COLOR`/`--quiet`/`--verbose`/non-TTY.
- Detection domain (`internal/detect`): a language registry scanning the repo for
  marker files to find languages, dirs, and package managers, resolving each
  language's standard tools via lookup; `cairn doctor` renders the installed/missing
  table with install hints.
- Config domain (`internal/config`): typed `cairn.yaml` aggregate with in-code
  defaults, default-merge load, normalization, validation, and `LoadOrDefault`.
- Buildable Go CLI skeleton (cobra): `cairn --version` and a `doctor` stub, plus
  `tool/verify.sh` and a GitHub Actions CI running build/test/lint.
- Project scaffold: vision README, DDD/hexagonal `docs/ARCHITECTURE.md`, contributor
  guide `AGENTS.md`, the iterative build plan `docs/ITERATIONS.md`, the reusable
  next-iteration prompt `docs/PROMPT.md`, an example `cairn.yaml`, and licensing.
