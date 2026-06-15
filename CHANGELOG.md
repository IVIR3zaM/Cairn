# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- Multi-module Maven/Gradle projects no longer run (and hang) once per submodule:
  detection now collapses a single-root build tool's nested manifests to the outermost
  reactor root, so `cairn verify` builds the whole project once where the build tool's
  reactor can resolve inter-module dependencies — instead of building each submodule
  alone (which stalls trying to fetch its siblings) and printing N identical lines.
- Java `cairn verify` no longer hangs and actually verifies the project: the adapter now
  runs the build tool's real lifecycle (`mvn -B verify` / `gradle --console=plain check`)
  instead of a hardcoded `spotless:check`, which froze resolving a Spotless plugin most
  projects never declare. It prefers the committed `mvnw`/`gradlew` wrapper and always
  runs non-interactively, so a missing plugin or a prompt can't stall it.
- `cairn verify` no longer leaves you staring at a silent terminal: each stage renders a
  live spinner with elapsed seconds as it runs (TTY); the step label includes the unit's
  directory when it isn't the repo root; and `--verbose` prints the exact command (with
  its working directory) and streams the tool's own output — the visibility CI runs and
  debugging need. As a safety net, a stalled stage is still bounded by `verify.timeout`
  (default 5m) and fails as "timed out".
- Tool lookup now checks GOPATH/bin and GOBIN in addition to PATH, resolving Go tools
  (like gofumpt) installed via `go install` but not in the shell's PATH.

### Changed
- Quality adapters now accept a per-language `standard` parameter (e.g. "ruff" or
  "black+flake8" for Python) to select between multiple tool choices; the registry
  signature changed to `func(runner.ToolRunner, string) Adapter` and `AdapterFor`
  resolves adapters with the standard from `cairn.yaml`.
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
- Java quality adapter (`internal/quality/lang_java.go`) delegating to the build tool's
  verification lifecycle — `maven` (default) or `gradle` — via the committed wrapper when
  present, gated on the JDK; self-registered into `cairn verify`.
- JavaScript/TypeScript quality adapter (`internal/quality/lang_javascript.go`) supporting
  both `eslint` (prettier + eslint) and `biome` standards via `npx`, with tests run through
  `npm test`; every stage gates on `npx` and it self-registers into `cairn verify`.
- Python quality adapter (`internal/quality/lang_python.go`) supporting both ruff (modern
  single-tool) and black+flake8 (traditional pair) standards via per-language config;
  self-registered into `cairn verify`.
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
