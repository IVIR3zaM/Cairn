# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `cairn bump <version> --force` (`-f`) allows a deliberate **downgrade** on the direct,
  non-interactive path — the equivalent of the wizard's double-confirm. Without it, a direct
  bump still refuses to go backwards, and the refusal now points at `--force`. A no-op (the
  current version) is refused even with `--force`, since there is nothing to apply.
- Language-agnostic **multi-package workspace** support for `bump`/`verify`, plus the Dart
  `pubspec.yaml` writer as its first participant. A manifest manager may now opt into the
  self-registering `version.Workspace` capability (`PackageID`/`SetSiblings`/`CheckSiblings`);
  the engine (`version.RewriteWorkspace`/`CheckWorkspace`) gathers package identities across
  every manifest of that format and reconciles member-to-member dependency constraints by
  **member name**, so a sibling pinned at any stale version is repaired/flagged while an
  external dependency is left alone — for any workspace/reactor format, with no language named
  in the CLI or engine. `cairn bump` moves each member's `version:` and its sibling `^`
  constraints in lockstep; `cairn verify` adds a language-owned manifest honesty check
  (`version.CheckManifests`) and the workspace check alongside `version_sync`, so drift in the
  files `bump` writes fails verify with no per-file config.
- `cairn bump` now finds version manifests by **language-owned auto-discovery** instead of
  scanning configured dirs: each language declares its manifest filename(s) in its detect spec
  (rust→`Cargo.toml`, python→`pyproject.toml`, javascript→`package.json`, dart→`pubspec.yaml`),
  and bump rewrites every detected unit's declared manifest via `version.ManagerFor` — so a
  package is bumped because the language owns it, not because it appears in `cairn.yaml`. A
  declared location with no writer yet (Dart's pubspec, until 6e) is skipped; `version_sync`
  remains the fallback for custom files. Adding a manifest location is a one-file change.
- `cairn bump [level|version]` computes the next version from `project.canonical_version`
  (semver level or explicit `X.Y.Z`; CalVer date-step), updates every registered manifest in
  the repo and each language dir, rewrites `version_sync` docs, advances `canonical_version`
  in `cairn.yaml`, and prints a suggested release commit/tag — never committing. Unset
  canonical and non-increasing bumps are refused. Run with no argument for a colorful,
  interactive wizard (patch/minor/major/custom with each target version shown, a color-coded
  jump explanation, and a loud double-confirm safeguard for downgrades); honors
  `NO_COLOR`/non-TTY and falls back to requiring an explicit level/version when not run
  interactively. The suggested release commit now follows `commits.convention`
  (`chore(release): X` / `🔖 Release X` / `Release X`) and adds `-s` when `commits.signoff`
  is set, instead of always proposing a Conventional Commits subject.
- Self-registering `VersionManager` registry (`version.ManagerFor`/`Managers`) with
  npm/cargo/pyproject manifest writers, plus the mutating `version.Rewrite` that fixes
  drifted `version_sync` docs — the library layer behind the upcoming `cairn bump`.
- Strict mode for the quality gate: `verify.strict` (repo-wide) and per-language
  `languages.<name>.strict` (which overrides it) promote a linter's most lenient
  diagnostics to hard failures wherever the toolchain has such a tier — `dart analyze
  --fatal-infos`, eslint `--max-warnings=0`, biome `--error-on-warnings`. Linters that
  already fail on every finding (go, rust, python, java) are unaffected. Defaults to off.
- Versioning context (`internal/version`): SemVer parse/compare and `Next(major|minor|patch)`,
  plus a CalVer next-date helper. `cairn verify` now runs the non-mutating `version_sync`
  doc-honesty check — every configured `{VERSION}` pattern must quote
  `project.canonical_version`, and a drifted or missing pattern fails verify with a compact
  per-file reason.

### Fixed
- CLI errors are now printed to stderr instead of being swallowed: the root command
  silences cobra's own error printing (so `verify` renders its own summary), but `main`
  previously exited non-zero without a message — so a failed `bump` (unset
  `canonical_version`, a downgrade guard) or an unknown command produced no output at all.
  `Execute` now surfaces every non-already-reported error.
- Dart pub workspaces (Dart 3.6+) are now verified per member package instead of once at
  the aggregator root: detection recognizes a `workspace:` pubspec as an aggregator that
  owns no code and defers to the member packages nested beneath it, so `verify` runs
  format/lint/test in each member's own directory (with the dir shown in each step label)
  rather than recursing from the root — which duplicated work and ran `dart test` at a
  level with no tests. Generalized as a `workspace` predicate on a language spec (the
  mirror of `singleRoot`), so any language can opt in by dropping one self-registered file.
- `dart · test` no longer fails a Dart package that simply has no tests: `dart test`
  treats a missing default `test/` directory as a usage error (non-zero exit), so the
  adapter now skips the stage (⊘) when `test/` is absent instead of reporting a failure.
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
- `cairn verify --verbose` now preserves each tool's own colors: on a color TTY it forces
  the tool to emit ANSI (which it otherwise auto-disables when its output is captured)
  via each language's native knob — `CARGO_TERM_COLOR`, `FORCE_COLOR`/`PY_COLORS`,
  `CLICOLOR_FORCE`, Maven `-Dstyle.color`, Gradle `--console=rich`, `dart test --color`.
  Piped/`NO_COLOR` runs stay escape-code free. The knobs live in each `lang_<name>.go`.
- Dart quality adapter (`internal/quality/lang_dart.go`) wrapping the single `dart`
  toolchain — `dart format` (check via `--set-exit-if-changed`), `dart analyze`, and
  `dart test`; every stage gates on `dart` and it self-registers into `cairn verify`.
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
