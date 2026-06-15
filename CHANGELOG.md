# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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
