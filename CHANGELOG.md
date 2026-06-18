# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `cairn init --yes` (10b-i): non-interactive onboarding that detects languages, writes a valid
  schema-2 `cairn.yaml` with the detected languages enabled (`config.InitConfig`, round-trips
  through `LoadTree`), installs the configured git hooks, generates the CI workflow, and prints
  next steps. Non-destructive — an existing `cairn.yaml` is kept. The interactive wizard is 10b-ii.
- `cairn init` now seeds the new `cairn.yaml`'s `version:` from the project's real version instead
  of a `0.1.0` placeholder: each manifest Manager gained a `ReadVersion` mirror of `SetVersion`,
  and `version.DetectVersion` reads the first detected language manifest that declares a version
  (a pom's `<version>`, a `package.json` `"version"`, Cargo/pyproject/pubspec, …), dropping a
  Maven qualifier like `-SNAPSHOT`. So `cairn verify` agrees out of the box; it falls back to
  `0.1.0` only when nothing declares a version.
- `cairn init` detects the commit policy from git history instead of writing a blind default: it
  enables `commits.signoff` only when DCO `Signed-off-by:` trailers are the norm across history
  (new `commit.IsSignedOff` reuses the validator's trailer definition; `detectCommits` applies a
  majority threshold). When sign-off is enabled it records a complete `commits` block (convention
  included, since config resolves the block as a unit); otherwise the block is omitted.
- `cairn init` auto-wires `version_sync` by scanning the README for the project's real version: a
  new `version.DetectSyncPatterns` records the distinctive surrounding tokens (a badge, a
  dependency coordinate like `group:artifact:{VERSION}`, an XML `<version>` snippet, an install
  command) as `{VERSION}` patterns, while rejecting a bare or merely `v`-prefixed number in prose.
  So Cairn's signature doc-honesty check runs from the very first `cairn verify`, no hand-written
  patterns required.
- Maven version manifest (`internal/version/manifest_pom.go`): a self-registering `pom.xml`
  manager so `cairn verify` asserts the reactor root pom's own `<version>` against the configured
  version and `cairn bump` writes it, preserving a trailing qualifier such as `-SNAPSHOT`. It
  scans only the project-version region (after any `</parent>` block, before the first
  dependencies/build/etc. section) so inherited and pinned versions are never touched; Java
  detection now declares `versionManifests: ["pom.xml"]`. Multi-module child `<parent>`
  reconciliation is a follow-up because Java detection is single-root.

### Changed
- `cairn init` now writes a **smart, discovered** `cairn.yaml` rather than a wall of defaults: it
  records only the facts it positively determined — the detected `version`, the languages present
  (each enabled, as an editable scaffold), the `version_sync` patterns found in the README, and a
  `commits` block when history justifies one. Settings it cannot discover are omitted and ride the
  in-code defaults via the resolved baseline, so an omitted key behaves identically without
  cluttering the file. `InitConfig`'s signature changed to `InitConfig(base Directory) ([]byte,
  error)` (the caller now assembles the discovered baseline; the unused `*Config` return was
  dropped).
- Rewrote `cairn.example.yaml` for schema 2 (top-level `version:`/`schema:`, the `languages`
  block as tool/standard knobs, a `directories:` per-directory override section) and documented
  that `init` writes a smart discovered file, not this fully-annotated reference.

### Removed
- Dropped the legacy schema-1 fields from the `config` aggregate (10a-iii-c-iii): `project.*`
  (`Project`/`PackageVersion`), `changelog.packages` (`Changelog.Packages`/`PackageChangelog`),
  and `languages.*.dir`. The per-directory `Tree` (schema 2) is now the sole version/location
  model — independence comes from a directory's own `version:`, per-package changelogs from a
  directory's own `changelog:` override block, and locations from detection. The legacy
  `version: "1"` / `project:` shape is still accepted and translated in `tree.go`. Removed the
  legacy `version.NewResolver(config.Project)` constructor; `NewResolverFromTree` is the only one.

### Fixed
- The generated `commit-msg` hook called `cairn commit-lint "$@"`, but no such command existed,
  so every commit aborted with "unknown command". Added the `commit-lint <message-file>` command:
  it resolves the convention and sign-off requirement from the repo's `cairn.yaml` (per-directory
  `Tree`), strips git's comment lines, and validates the message via `commit.ValidatorFor` — a
  non-conforming message exits non-zero, an unregistered convention (e.g. `none`) never blocks.
- Locked in that git actually fires the tracked-directory hooks: `TestInstalledHookFiresOnCommit`
  installs hooks via `wiring.InstallHooks`, stubs `cairn` on `PATH`, and asserts a real `git
  commit` runs `cairn verify` on the pass path and is blocked when the hook exits non-zero.
- Schema-2 `cairn.yaml` baseline now starts from the in-code defaults, so a *partial* top-level
  block (e.g. `verify: { strict: true }`) merges field-by-field instead of wiping the stages it
  omits. Previously `parseRootTree` parsed the baseline from a zero value, silently disabling
  `format`/`lint`/`test` and leaving `verify` to run only the version checks; it now seeds the
  baseline via `baselineFromConfig(Default())` before unmarshalling (mirroring the legacy
  `Default()`+unmarshal merge and the no-file `defaultTree()`). Directory override entries stay
  nil ⇒ "inherit", so the cascade is unchanged.

### Changed
- `cairn bump` now resolves everything through the per-directory config `Tree` (10a-iii-c-ii):
  it loads via `config.LoadTree`, reads/writes the repo baseline top-level `version:` for repo-wide
  bumps, enumerates `Tree.Independent()` (writing back to `directories.<path>.version`) for
  per-package bumps, and drives manifests/version_sync/changelog/workspace through
  `version.NewResolverFromTree` (post-bump target via the new `config.Tree.WithVersion`) — so the
  CLI carries no YAML-reading or precedence logic beyond its own cairn.yaml write-back. Commits,
  changelog (incl. per-package), and version_sync come from the resolved baseline `Directory`;
  bump tests migrated to schema-2 fixtures. Single-package repos behave exactly as before.
- `cairn verify` now resolves each detected unit's settings through the per-directory config
  `Tree` (10a-iii-b): languages standard/strict, verify stage toggles, version_sync, the absolute
  disable gate, and the target version all come from `Tree.Resolve` + `version.NewResolverFromTree`,
  so the CLI carries no precedence logic of its own (`Config.LoadOrDefault`/`StrictFor`/`cfg.Project`
  reads dropped). New `Directory.VerifyOrDefault`/`StrictFor` mirror the cascade; single-package
  repos behave exactly as before.
- ARCHITECTURE.md now documents the per-directory config model (10a-iii-doc): the `cairn.yaml`
  schema block was rewritten to the schema-`"2"` shape (repo baseline as plain top-level keys +
  a `directories:` map of override blocks, `languages` tool-knobs-only, the `schema:` format
  marker), a "Per-directory config & precedence" subsection captures the field-level low→high
  cascade (baseline < own-file < root `directories.<path>`), the absolute disable gate, and
  own-`version`⇒independent versioning, and the data-flow + ADR-003 were reconciled so the
  source-of-truth doc matches the implemented `config.Directory`/`Tree` model. Docs-only.

### Added
- `config.Tree.Independent()` (10a-iii-c-i): lists the directories that declare their own
  `version` (a root `directories.<path>` entry or the directory's own `cairn.yaml`), sorted and
  excluding pruned subtrees — the schema-2 successor to `project.packages` that `bump` will
  enumerate release units from. Purely additive; no caller changes.
- `version.NewResolverFromTree` (10a-iii-a): build a `version.Resolver` over the resolved
  per-directory config `Tree` so `ForDir` answers each unit's target version + scheme from config's
  cascade (a pruned directory yields an empty Target). The legacy `NewResolver(project)` constructor
  stays until `verify`/`bump` migrate — purely additive.
- Per-directory config loader (10a-ii): `config.LoadTree` reads the repo `cairn.yaml` (schema `"2"`,
  with the format marker moved to a `schema:` key and the top-level `version:` repurposed as the repo
  baseline version), discovers nested `<path>/cairn.yaml` override blocks, and resolves a directory's
  effective settings via `Tree.Resolve(dir)` — folding repo baseline < own-file ancestors < root
  `directories.<path>` overrides (nearest wins). An absolute disable gate (`enabled: false`) prunes a
  subtree before its own file is ever read; `Active`/`Pruned` enumerate the tree. A legacy
  `version: "1"`/`project:` file is accepted and translated, never silently misread.
- Per-directory config core (10a-i): a `config.Directory` override block (the repo-baseline keys, each
  optional ⇒ "inherit") plus the field-level `overlay`/`cascade` merge — set field wins, unset
  inherits, `languages` merges by name. The reusable "design it once" foundation for the upcoming
  unified per-directory config model; loader/discovery/cascade wiring lands in 10a-ii/iii.
- Wiring context (`internal/wiring`): install Cairn into a repo so the local hook and CI share one
  `cairn verify` (ADR-005). `InstallHooks` writes the configured git hooks (`hooks.pre_commit`/
  `commit_msg`/`pre_push`) as executable POSIX-sh scripts into a tracked `.cairn/hooks` dir and
  points git at it via `core.hooksPath` (commit-msg forwards the message file path); CI providers
  are a self-registering registry (`wiring.ProviderFor`/`Providers`) with `github` as the first
  entry, generating `.github/workflows/cairn.yml` that runs each `ci.jobs` step. Both installs are
  idempotent; other CI providers are future one-file additions.
- Per-package bump inference (monorepo): `cairn bump <pkg>` with no level infers that package's
  level from the commits that touched its directory since its own `<pkg>-v*` tag (degrading to the
  package's whole history when untagged), then advances only it. The no-argument flow in a
  `project.packages` monorepo now prints a per-package inferred summary instead of a single
  repo-wide choice; repos with only `canonical_version` keep the repo-wide behavior unchanged.
- `cairn bump` with no level now infers the next bump from commit history: it classifies every
  commit since the last tag via the configured `commit.Validator` (`commit.InferBump` takes the
  highest implied level), then the wizard preselects it (bare Enter accepts) and a non-interactive
  run applies it directly — degrading sensibly when no tag/commits/release-worthy change is found.
- CommitConventions context (`internal/commit`): a self-registering `CommitValidator` registry
  (`commit.ValidatorFor`/`Conventions`) with `conventional` as the first convention — it validates
  a Conventional Commits header (`type(scope)?!?: description`, known-type set, optional DCO
  sign-off) and classifies a message into the SemVer bump it implies (`feat`→minor, `fix`→patch,
  `!`/`BREAKING CHANGE`→major, else none) for upcoming history-based bump inference. `gitmoji`/
  `none` are future one-file additions.
- Changelog context (`internal/changelog`): `cairn bump` now promotes the configured
  CHANGELOG's unreleased section into a dated release, leaving a fresh empty section and (for
  Keep a Changelog) refreshing the compare links — the `[Unreleased]` link advances to the new
  tag and a `[X.Y.Z]` link is inserted, with the `v`-prefix style derived from the file so Cairn
  neither invents a repo URL nor imposes a tag convention. Promotion is idempotent. The changelog
  *standard* is a self-registering registry (`changelog.WriterFor`): `keepachangelog`
  (`## [Unreleased]` → `## [X.Y.Z] - DATE`) and `dart` (plain pub.dev style `## Unreleased` →
  `## X.Y.Z - DATE`, no links) ship as the first two writers over a shared promotion engine;
  `git-cliff`/`conventional-changelog` are future one-file additions.
  - **Multi-package changelogs:** `changelog.packages` (`{standard, file}`) gives a monorepo a
    second style for each package's own changelog, auto-discovered as `<package-dir>/<file>` per
    detected package — so a bump promotes the root changelog *and* every package's in one pass,
    each to its resolved version (a pub.dev workspace keeps a root Keep a Changelog file plus a
    plain per-package one). A repo-wide bump covers every detected package; `bump <pkg>` covers
    only that package (and leaves the root changelog alone).
  - **Empty unreleased fails the bump:** a release whose targeted unreleased section is empty is
    refused up front (nothing is written) listing every offending file, so a notes-less release
    can't be cut. A changelog file that doesn't use the convention is skipped, not failed.
  - Bump now honors `languages.<name>.enabled: false` in its detection-based manifest and
    changelog discovery (like `verify` does), so a vendored/disabled tree (e.g. `reference/`) is
    never bumped or promoted (7).
- `cairn bump <pkg> <level|version>` advances a single declared `project.packages` entry from
  its own version line — updating only that package's manifests, its dependents' interdependency
  constraints, and its `cairn.yaml` entry, leaving the other packages and `canonical_version`
  untouched. Repo-wide `cairn bump <level>` and the no-argument wizard are unchanged (6g-iii-b).
- `cairn verify` honesty checks are now per-package: the manifest, workspace-interdependency,
  and `version_sync` doc checks each resolve a unit (or version_sync file) to *its own* target
  version via `version.Resolver`, so a monorepo whose packages version independently passes
  when each manifest/doc/interdependency matches its declared version and fails on drift of any
  one. A repo with only `canonical_version` is unchanged (lockstep). The honesty engine
  (`Check`/`Rewrite`/`CheckManifests`/`CheckWorkspace`/`RewriteWorkspace`) now takes a
  `*version.Resolver` and the `Workspace` member map is name→version (6g-iii-a).
- `version.Resolver` maps a detected unit directory to its target version + scheme,
  resolving `project.packages` (most-specific path prefix wins, nested overrides ancestor)
  and falling back to `canonical_version` when no package matches — the foundation for
  per-package bump/verify (6g-ii).
- Config schema now accepts `project.packages` — a per-package version map (`{path, version,
  versioning?}`) for monorepos that version independently; absent/empty keeps the single
  `canonical_version` behavior, and each entry is validated with an actionable error (6g-i).
- `cairn verify` now tells you how to fix a failing stage and can fix it for you, with
  wording that never over-promises. Each language stage declares its auto-fix command, so
  a failure prints a hint beneath the tool output: a **formatter** fully resolves its
  stage, so it reads `↳ auto-fixable: run \`<cmd>\` (or \`cairn verify --fix\`)`; a
  **linter** only covers a subset (staticcheck `SA*`, type errors, etc. have no autofix),
  so it reads `↳ some findings may be auto-fixable: run \`<cmd>\` …; the rest need a manual
  fix`. The new `--fix` flag re-runs every fixable stage in write mode before reporting
  whatever could not be repaired, and a failure that **survives** a `--fix` run reads
  `↳ auto-fix already ran — the findings above need a manual fix` instead of re-suggesting
  the command that just failed to resolve it. Wired per language: Go (`gofumpt -w`,
  `golangci-lint run --fix`), Rust (`cargo fmt`, `cargo clippy --fix`), Python (ruff
  `format`/`check --fix`, or `black`), JS/TS (prettier/eslint `--fix` or biome `--write`),
  and Dart (`dart format`, `dart fix --apply`). Version drift gets its own accurate hint
  pointing at `cairn bump <canonical>` rather than `--fix`. Java stays build-tool-owned
  (no fix). A stage advertises its fix via a `fix:` field in its `stepSpec`, so adding one
  for a new language stays a one-file change.
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
