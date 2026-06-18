# Iterations

The ordered plan from empty repo to a dogfooded MVP. **Do one iteration per run.** Each
entry is self-contained: only read the files in its **Read:** line. Tick the box when
its **Acceptance** is met. Keep entries small — if one grows, split it.

Legend: `[ ]` todo · `[~]` in progress · `[x]` done.

> Conventions for every iteration: add meaningful tests, update `CHANGELOG.md`
> `[Unreleased]`, propose a Conventional Commit message. (See [AGENTS.md](../AGENTS.md).)

---

## [x] 0 — Scaffolding
**Goal:** A buildable Go CLI skeleton with `cairn --version` and `cairn doctor` stubs.
**Read:** AGENTS.md
**Steps:** `go mod init`; add cobra; root command + `version`; empty `doctor` printing
"not implemented"; Makefile or `tool/verify.sh` placeholder; `.gitignore`; GitHub Actions
running `go build` + `go test ./...` + `golangci-lint`.
**Acceptance:** `go build ./...` and `go test ./...` pass; `cairn --version` prints.

## [x] 1 — Config domain
**Goal:** Load, validate, and default-merge `cairn.yaml` into a typed aggregate.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Config + schema sections)
**Steps:** Define config structs matching the schema; YAML load; validation with clear
errors; in-code defaults so a minimal file works; a `LoadOrDefault(path)`.
**Acceptance:** Tests cover: full file, minimal file (defaults fill in), invalid file
(actionable error). No I/O outside the loader.

## [x] 2 — Detection + `doctor`
**Goal:** Detect languages, dirs, package managers, and which standard tools are installed.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Detection) · internal/config
**Steps:** A pluggable registry where each language self-registers from its own
`internal/detect/lang_<name>.go` (marker files + default tools + skip dirs). Scan the
repo; resolve installed tools via `exec.LookPath`. Implement `cairn doctor` to print a
per-language installed/missing table with install hints.
**Acceptance:** On fixture repos (one per language) detection is correct; `doctor` lists
present vs missing tools. Tests use a fake filesystem/lookup.

## [x] 3 — ToolRunner + Reporter ports
**Goal:** The two cross-cutting ports, with real + fake implementations.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Ports, UX/Reporter)
**Steps:** `ToolRunner` (cwd, timeout, captured output, exit code). `Reporter` port with
a TTY impl (color, glyphs ✓✗⊘!, spinner, compact summary) and a plain CI impl; honor
`NO_COLOR`/`--quiet`/`--verbose`/non-TTY. Add a fake ToolRunner for tests.
**Acceptance:** Reporter renders a stable, compact summary in tests; plain mode has no
ANSI; ToolRunner captures exit/output correctly.

## [x] 4 — QualityGate + Go adapter (`verify` end-to-end)
**Goal:** `cairn verify` works fully for one language (Go), proving the whole spine.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (QualityGate) · internal/{config,detect,runner,report}
**Steps:** Step ports (Formatter/Linter/Tester/…); a Go adapter
(`internal/quality/lang_go.go`) wrapping gofumpt/golangci-lint/go test; ordered plan
builder; missing-tool ⇒ required vs warn+skip with hint; wire `cairn verify`.
**Acceptance:** Green Go fixture ⇒ pass; fixture with a lint/format/test error ⇒ non-zero
with a compact failing summary; missing tool degrades per `required`.

## 5 — Remaining language adapters (split)
**Goal:** `verify` supports Rust, Python, TS/JS, Java, Dart — one self-registering file
each, `internal/quality/lang_<name>.go`, mirroring the existing `lang_go.go`/`lang_rust.go`.
Each `init()` calls `register(name, ctor)` (no `adapters` map; `verify` resolves via
`quality.AdapterFor`). Respect per-language `standard` (ruff vs black+flake8, eslint vs
biome) as a branch inside the file. Tested with the fake ToolRunner. Split below.

### [x] 5a — Rust adapter
**Read:** AGENTS.md · docs/ARCHITECTURE.md (tool matrix, extension points) · internal/quality/lang_go.go (template) · internal/cli/verify.go
**Steps:** `internal/quality/lang_rust.go` wrapping `cargo fmt` (format), `cargo clippy
-D warnings` (lint), `cargo test` (test); gate tools rustfmt/clippy-driver/cargo (matching
detection); self-register via `register("rust", …)`.
**Acceptance:** Fake-ToolRunner tests cover format check/fix, lint & test exit-code mapping,
and the stage→tool gating; `rust` is selectable in `verify`. *(Also refactored quality
adapters to one self-registering file per language in the `quality` package — see ADR-006.)*

### [x] 5b — Python adapter (ruff; black+flake8)
**Read:** AGENTS.md · docs/ARCHITECTURE.md (tool matrix) · internal/quality/lang_rust.go (template) · internal/config
**Steps:** `internal/quality/lang_python.go` honoring `standard` (ruff default; black+flake8).
**Acceptance:** Green + failing fixtures pass/fail; standard switch picks the right tools.

### [x] 5c — TS/JS adapter (eslint; biome)
**Read:** AGENTS.md · docs/ARCHITECTURE.md (tool matrix) · internal/quality/lang_rust.go (template) · internal/config
**Steps:** `internal/quality/lang_javascript.go` honoring `standard` (eslint/prettier; biome) via npx.
**Acceptance:** Green + failing fixtures pass/fail; standard switch picks the right tools.

### [x] 5d — Java adapter
**Read:** AGENTS.md · docs/ARCHITECTURE.md (tool matrix) · internal/quality/lang_rust.go (template) · internal/quality/lang_python.go (standard branching) · internal/detect/lang_java.go
**Steps:** `internal/quality/lang_java.go` delegating to the build tool's verification
lifecycle (`mvn -B verify` / `gradle check`), wrapper-aware (`mvnw`/`gradlew`) and
non-interactive — no fabricated plugin goals (an early `spotless:check` hung). Gated on the JDK.
**Acceptance:** Green + failing fixtures pass/fail.

### [x] 5e — Dart adapter
**Read:** AGENTS.md · docs/ARCHITECTURE.md (tool matrix) · internal/quality/lang_rust.go (template)
**Steps:** `internal/quality/lang_dart.go` wrapping `dart format`/`dart analyze`/`dart test`.
**Acceptance:** Green + failing fixtures pass/fail.

## 6 — Versioning + doc honesty + `bump` (split)
**Goal:** `cairn bump` and the version_sync honesty check (Cairn's signature). Split below
so each slice lands cleanly: 6a is the read-only honesty check end-to-end; 6b adds the
manifest writers and the mutating `bump`.

### [x] 6a — SemVer/CalVer math + version_sync honesty check (wired into `verify`)
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Versioning, data flow) · internal/{config,report} · internal/cli/verify.go
**Steps:** `internal/version/version.go` — parse/compare a version, `Next(level)` for
major/minor/patch (SemVer) and a CalVer next-date; guard empty/downgrade at the math layer.
`internal/version/sync.go` — compile each `{VERSION}` pattern to a regex, and a
non-mutating `Check(fsys, canonical, files)` reporting per-file drift/missing. Wire the
honesty check into `cairn verify` as extra report steps after the language stages, so a
drifted doc fails verify.
**Acceptance:** version parse/compare/`Next` + sync drift are tested; `verify` fails on a
drifted doc and passes when honest; no `version_sync` / empty canonical ⇒ no-op (no steps).

### [x] 6b — VersionManager registry + manifests + version_sync mutating rewrite
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Versioning, extension points) · internal/version · internal/config · internal/quality/registry.go (registry template)
**Steps:** per-manifest `VersionManager` as a **self-registering registry**
(`internal/version/manifest_<name>.go`, `register`/`ManagerFor`/`Managers`, panic on dup),
each a pure byte-transform setting the version via safe regex (native tooling is a later
opt-in, documented not stubbed) — no central manifest map; add `version.Rewrite` (the
mutating sibling of `Check`) that fixes every `{VERSION}` pattern in the version_sync docs.
**Acceptance:** `ManagerFor`/`Managers` resolve via self-registration (dup panics);
`SetVersion` rewrites npm/cargo/pyproject versions (and is a no-op when already correct);
`Rewrite` turns a drifted doc honest and reports changed files; adding a manifest is one
self-registering file (no engine edits).

### [x] 6c — `cairn bump` command (compute next, update manifests + docs, suggest commit/tag)
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Versioning, data flow) · internal/version · internal/{config,report} · internal/cli/{root,verify}.go
**Steps:** `cairn bump <level|version>` computes next (level via `Next`/`NextCalVer`, or an
explicit version) from `project.canonical_version`; updates registered manifests in the
repo + each language dir, rewrites version_sync docs, and updates `canonical_version` in
`cairn.yaml` so verify stays green; prints suggested commit/tag; never commits; downgrade
and empty-version rejected.
**Acceptance:** `bump` updates manifests + doc patterns + canonical; downgrade and
empty-version cases are guarded; prints a suggested commit/tag and does not commit.

> 6d (the original "language-owned version locations" slice) grew past one clean run, so
> per its own split note it became **6d (discovery + bump auto-discovery)** and **6e
> (pubspec-workspace manager + verify honesty coverage)**.

### [x] 6d — Language-owned manifest discovery + bump auto-discovery (version_sync as fallback)
**Goal:** Each language declares where its version manifest sits, so `bump` finds and updates
it **from detection** (root + every detected unit dir, incl. pub-workspace members) via
`ManagerFor(filename)` instead of scanning config dirs. `version_sync` regex patterns become a
**fallback** for custom spots (READMEs, generated snippets). Adding a manifest location is a
one-file detect-spec change.
**Read:** AGENTS.md · internal/detect/{detect,registry,lang_rust,lang_python,lang_javascript,lang_dart}.go ·
internal/version/manifest.go · internal/cli/bump.go
**Steps:**
- Add `versionManifests []string` to the detect `langSpec` and expose `VersionManifests` on
  `detect.Language`; declare each language's manifest (rust→`Cargo.toml`, python→`pyproject.toml`,
  javascript→`package.json`, dart→`pubspec.yaml` — its writer lands in 6e).
- Rework `bump`'s `updateManifests` to walk detected units and update each declared manifest
  via `ManagerFor(filename)`; a declared filename without a writer is skipped. `version_sync.Rewrite`
  stays as the fallback for custom files.
**Acceptance:** `bump` auto-updates a detected language's manifest with **no** `cfg.Languages`
entry; a custom README still updates via `version_sync`; a declared manifest without a writer is
skipped; adding a manifest location is a one-file change.

### [x] 6e — pubspec-workspace manager + verify honesty language-owned coverage
**Goal:** Add the Dart `pubspec.yaml` writer (workspace-aware: rewrites sibling `^{VERSION}`
interdependency constraints in lockstep), and give `verify`'s honesty check the same
language-owned manifest coverage so drift is caught without per-file `version_sync` config.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Versioning) · internal/version/{manifest,sync}.go ·
internal/detect/lang_dart.go · internal/cli/verify.go
**Steps:** `internal/version/manifest_pubspec.go` (`register("pubspec.yaml")`, workspace `^`
interdep rewrite); a non-mutating `version.CheckManifests` wired into `verify` alongside the
`version_sync` check.
**Acceptance:** In a Dart-workspace fixture, `bump` updates every member `pubspec.yaml`
`version:` and sibling `^` constraint with no per-file config; `verify` fails on a drifted
manifest and passes when honest. *(Generalized in 6f: the workspace interdependency rewrite/
check became the language-agnostic `version.Workspace` capability + `RewriteWorkspace`/
`CheckWorkspace` engine, so any multi-package format — npm/Cargo workspace, Maven/Gradle
reactor — participates by self-registering, with no language named in `verify.go`/`bump.go`.)*

### [x] 6f — Language-agnostic multi-package workspaces (`version.Workspace` capability)
**Read:** AGENTS.md · internal/version/{manifest,manifest_pubspec}.go · internal/cli/{verify,bump}.go
**Steps:** Lift the Dart-specific sibling pass out of the engine into a self-registering
`version.Workspace` optional capability (`PackageID`/`SetSiblings`/`CheckSiblings`); the engine
(`RewriteWorkspace`/`CheckWorkspace`) groups manifests by format, gathers member identities, and
reconciles intra-repo constraints by member **name** — `verify.go`/`bump.go` pass the generic
`[]ManifestUnit` and never name a language. `pubspec` is the first participant.
**Acceptance:** `verify` flags / `bump` repairs a member-to-member constraint pinned at a stale
version in *any* Workspace-capable format; core engine files contain no format/language names;
adding workspace support to another language is implementing `version.Workspace` on its manager.

## 6g — Independent per-package versions (monorepo) declared in `cairn.yaml`
**Goal:** Support a monorepo where packages version **independently** (each its own SemVer/
CalVer line), not one repo-wide `project.canonical_version`. Versions (or version *scopes*) are
declared in `cairn.yaml`; `bump`, `verify` (manifest + workspace + `version_sync` honesty), and
the workspace interdependency reconciliation all resolve "the version for *this* package" from
that config instead of assuming a single canonical. Mixed-language monorepos (a Java module and
a Dart package each on their own version) must work. The original single-run slice grew past one
clean run, so it is split below: 6g-i is the config schema, 6g-ii the resolver, 6g-iii the
bump/verify wiring.

### [x] 6g-i — Config schema: per-package version map (backward-compatible)
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Config schema) · internal/config/config.go
**Steps:** Add `project.packages` (`[]PackageVersion` of `{path, version, versioning?}`) to the
config aggregate, defaulting to empty (whole repo follows `canonical_version`). Validate each
entry (non-empty path + version; scheme override, if set, one of semver/calver) in `Validate`
alongside the other actionable errors; `PackageVersion.VersioningFor(projectScheme)` resolves the
inherit-vs-override scheme (mirrors `StrictFor`). Document the field in the ARCHITECTURE schema.
**Acceptance:** a monorepo `cairn.yaml` with two `packages` parses (per-package version + scheme
override resolved); invalid entries (empty path/version, unknown scheme) yield one actionable
error; an absent `packages` is unchanged from today (single canonical).

### [x] 6g-ii — `version.Resolver`: map a detected unit/member to its target version
**Read:** AGENTS.md · internal/config/config.go · internal/version/{version,sync,manifest}.go ·
internal/detect/detect.go
**Steps:** A `version.Resolver` built from `project` config that maps a detected unit dir (and a
workspace member name) to its target version + scheme, falling back to `canonical_version` when
no `packages` entry matches (lockstep = the degenerate all-equal case). Longest-path-prefix wins
for nested packages.
**Acceptance:** resolver returns the per-package version for a matching unit, canonical for an
unmatched one, and the most-specific entry for nested paths; tested in isolation.

## 6g-iii — Thread the resolver through `bump` + `verify` (split)
**Goal:** Make the honesty engine and CLI per-package aware via `version.Resolver`. The original
single-run slice grew past one clean run (it migrates four engine signatures + the `Workspace`
interface *and* adds new `bump` ergonomics), so it is split: 6g-iii-a migrates the engine to the
resolver and makes `verify` per-package; 6g-iii-b adds per-package `bump` ergonomics.

### [x] 6g-iii-a — Resolver-threaded honesty engine + per-package `verify`
**Read:** AGENTS.md · internal/version/{sync,manifest,manifest_pubspec,resolver}.go ·
internal/cli/{bump,verify}.go · internal/config/config.go
**Steps:** Replace the lone `canonical string` parameter on `Check`/`Rewrite`/`CheckManifests`/
`CheckWorkspace`/`RewriteWorkspace` with a `*version.Resolver`, resolving each unit (manifest by
its dir, version_sync file by `path.Dir`) to *its* target version; change the `Workspace`
interface's `members` to name→version (`map[string]Version`, drop the single `v`). Wire `verify`
to build the resolver from `cfg.Project` (per-package honesty). Keep `bump` repo-wide for now by
passing a lockstep resolver built from the computed `next` — behavior unchanged.
**Acceptance:** `CheckManifests`/`CheckWorkspace`/`Check` flag drift against each unit's resolved
version (a monorepo where two packages hold different versions passes when each manifest/doc/
interdependency matches *its* version, fails on drift of any one); a repo with no `packages`
behaves exactly as today (lockstep). Tested in the version package.

### [x] 6g-iii-b — Per-package `bump` ergonomics
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Versioning, data flow) · internal/version/resolver.go ·
internal/cli/bump.go
**Steps:** `cairn bump <pkg> <level|version>` advances a single declared package: compute its
next from its `project.packages[].version`, update only that package's manifests + its dependents'
constraints + its `cairn.yaml` packages entry, leaving the others. Repo-wide `cairn bump <level>`
stays for the canonical/lockstep case. Decide wizard behavior for monorepos.
**Acceptance:** in the mixed-language monorepo fixture, `bump <pkg>` advances one package (and its
dependents' constraints) without touching the others; a repo with only `canonical_version` behaves
exactly as today.
> Implemented: a two-arg form `bump <pkg> <level|version>` routes to `runPackageBump`, which builds
> a `version.Resolver` from the post-bump project (target package at `next`, all others unchanged)
> and reuses the shared `applyBump` engine — so honest manifests/docs are skipped and only the
> bumped package + its stale dependents are written. `cairn.yaml` is updated via a scoped
> `version:`-line edit on the matching `packages` entry (canonical untouched). **Wizard decision:**
> the no-argument wizard stays repo-wide (canonical/lockstep); per-package advances are explicit
> (`bump <pkg> <level|version>`), keeping the interactive flow simple. A package-scoped bump banners
> and tags the package (`<pkg>-v<next>`).

## [x] 7 — Changelog (Keep a Changelog)
**Goal:** Promote `[Unreleased]` → version+date with refreshed compare links on `bump`.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Changelog, extension points) · internal/version
**Steps:** Stand up the changelog **standard registry** (`internal/changelog`,
`register`/`WriterFor`) and add the keepachangelog writer as `std_keepachangelog.go`
(self-registered, not a special case); integrate into `bump`; warn on empty
`[Unreleased]`. `git-cliff`/`conventional-changelog` are future `std_<name>.go` files —
documented, not stubbed.
**Acceptance:** A sample CHANGELOG is promoted correctly (idempotent, links updated);
empty-Unreleased warns; `WriterFor("keepachangelog")` resolves via self-registration.

## 8 — Commit conventions + commit-msg hook + bump inference (split)
**Goal:** Validate commit messages and infer the SemVer bump from history. The original
single-run slice grew past one clean run (a new `commit` context *and* threading inference
through the 30KB `bump.go`), so it is split: 8a stands up the `CommitValidator` registry +
conventional convention (validate + classify); 8b wires history-based bump inference using it
(repo-wide, against `canonical_version`); 8c extends inference to be per-package in a monorepo.

### [x] 8a — CommitValidator registry + conventional convention
**Read:** AGENTS.md · docs/ARCHITECTURE.md (CommitConventions, extension points) · internal/changelog/changelog.go (registry template) · internal/version
**Steps:** `CommitValidator` as a **convention registry** (`internal/commit`,
`register`/`ValidatorFor`/`Conventions`, panic on dup); add `conv_conventional.go`
(self-registered), leaving gitmoji/none as future `conv_<name>.go` files; classify
feat/fix/breaking → minor/patch/major; validate header shape + optional sign-off (DCO).
**Acceptance:** Valid/invalid messages classified correctly (feat→minor, fix→patch,
`!`/`BREAKING CHANGE`→major, other→none); sign-off required vs absent enforced; the
convention resolves via `ValidatorFor` per config (`Conventions` lists registered keys).

### [x] 8b — `cairn bump` (no level) infers the next bump from commit history
**Read:** AGENTS.md · docs/ARCHITECTURE.md (CommitConventions, data flow) · internal/commit · internal/version · internal/cli/bump.go
**Steps:** Read commits since the last tag (shell out to `git`), classify each via the
configured `commit.Validator`, take the highest bump, and use it as the default level when
`cairn bump` is run without a level (wizard preselects it / direct no-arg applies it).
**Acceptance:** Inference picks the right level on a fixture history; no commits / no tag
degrades sensibly; the convention resolves via `ValidatorFor` per config.
> Landed repo-wide only: `inferLevel` runs a path-blind `git log <lastTag>..HEAD` and feeds the
> highest level to the canonical/lockstep path; the wizard uses it purely as a preselect hint.
> Per-package inference (mapping commits to a `project.packages` entry) is 8c.

### [x] 8c — Per-package bump inference (monorepo, path-scoped history)
**Goal:** Make inference package-aware so a monorepo with independent `project.packages` versions
gets the right level *per package* from the commits that actually touched it — closing the gap
left by 8b, where inference is repo-wide and only ever steps `canonical_version`. Each declared
package infers its own level from its own history since *its* last tag, and that flows through the
existing per-package `bump` path (`runPackageBump`) and a monorepo-aware wizard.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (CommitConventions, data flow, Versioning) ·
internal/commit · internal/version/resolver.go · internal/cli/bump.go
**Steps:**
- Path-scoped history: a `commitHistory` variant that takes a package dir and runs
  `git log <pkgLastTag>..HEAD -- <pkg.path>`, where `pkgLastTag` matches the package's tag scheme
  (`<pkg>-v*`, the form `applyBump` already prints for a package-scoped bump) and falls back to
  whole history when the package has no tag yet.
- `inferPackageLevel(wd, cfg, pkg)` resolving the convention via `ValidatorFor` and classifying
  that package's commits; reuse `commit.InferBump`.
- Wire it: `cairn bump <pkg>` (one arg, a declared package, no level) infers that package's level
  and applies it via `runPackageBump`; the no-argument flow in a `project.packages` monorepo offers
  a per-package summary (each package + its inferred level) rather than a single repo-wide choice.
  Keep the repo-wide canonical/lockstep path unchanged when no `packages` are declared.
**Acceptance:** in the mixed-language monorepo fixture, two packages with different commit
histories each infer their own level (a `feat:` touching only pkg_a infers `minor` for pkg_a and
`none` for pkg_b); a package with no tag yet degrades to whole-history; a repo with only
`canonical_version` behaves exactly as 8b (repo-wide inference unchanged).

## [x] 9 — Wiring: hooks + CI generation
**Goal:** `init`'s outputs — install git hooks and generate a CI workflow calling `verify`.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Wiring, extension points) · internal/config
**Steps:** Install pre-commit (`cairn verify`) and optional commit-msg hooks via a tracked
hooks dir + `core.hooksPath`; make CI providers a **self-registering registry**
(`internal/wiring/ci_<name>.go`, `register`/`ProviderFor`) and add `ci_github.go` as the
first entry — other providers are later one-file additions, not a `switch`.
**Acceptance:** Hooks installed and runnable; generated GitHub workflow is valid and runs
`cairn verify`; re-running is idempotent; `ProviderFor("github")` resolves via self-registration.

## 10 — Per-directory config + onboarding wizard (split)
**Goal:** Collapse the four overlapping path-shapes (`languages.*.dir`, `project.packages`,
`changelog.packages`, `version_sync.files`) into **one** `directories:` map of override blocks,
let any directory carry its **own `cairn.yaml`** (same override block), and make *all* cascade,
precedence, and disable resolution a concern of the **`config` module** — then build `cairn init`
on top. Split: **10a** is the config refactor + new schema + ARCHITECTURE update; **10b** is the
`init` wizard (the original iteration 10).

### [ ] 10a — Per-directory config model (config owns the cascade)
**Goal:** One unified per-directory override model, resolved entirely inside `config`. The CLI
contexts stop reading `cairn.yaml`/walking dirs themselves and instead ask `config` for the
resolved settings of a directory (and whether it is active). This is the schema and the resolver;
no `init` wizard yet.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Config + schema, Versioning, Detection) ·
internal/config/config.go · internal/version/resolver.go · internal/cli/{verify,bump}.go · internal/detect/detect.go
**Decisions to encode (agreed):**
- **Repo baseline = root top-level keys.** Whole-repo settings live as plain top-level keys in the
  root `cairn.yaml` (`version`, `versioning`, `languages` tool knobs, `verify`, `commits`,
  `changelog`, `version_sync`, `hooks`, `ci`, `addons`). There is **no `.` entry** — the top level
  *is* the repo.
- **`languages` holds tool/standard knobs only — never locations** (`python: { standard: ruff }`,
  `dart: { strict: true }`). Detection owns "where languages are." `languages.*.dir` is removed.
- **One `directories:` map**, keyed by repo-relative path, each value an **override block**. This
  single map replaces `project.packages`, `changelog.packages`, and `version_sync.files`. An empty/
  absent `directories:` ⇒ detect everything, repo follows the single top-level `version` (lockstep).
- **The override block is one type, reused in three forms:** (1) the root file = override block
  (baseline) + optional `directories:`; (2) a root `directories.<path>` entry = override block;
  (3) a `<path>/cairn.yaml` = override block. Root-inline and nested-file are two serializations of
  the *identical* type — design it once.
- **Overrides cover everything**, not just version: any key valid at the repo baseline can be
  overridden per directory (versioning, version, languages knobs, verify toggles/strict, commits,
  changelog, version_sync).
- **Independent vs lockstep is a consequence, not a mode:** a directory with its own `version:` is
  independently versioned (own tag `<dir>-v<version>`, own changelog); without one it inherits the
  repo `version` (lockstep). This subsumes today's `project.packages`/canonical split.
- **Precedence is field-level, layered low → high:**
  1. repo baseline (root top-level keys);
  2. the directory's own `cairn.yaml` — and any ancestor's own file, outer → inner, nearest wins;
  3. root `directories.<path>` entries — and ancestors, outer → inner, nearest wins — **highest
     authority.**
  So an explicit root per-directory override **beats** that directory's own `cairn.yaml`; where the
  root is silent for a directory, the directory's own file governs. Worked examples:
  - root `directories.somerepo` sets `dart.strict: true`, `somerepo/cairn.yaml` says not-strict ⇒
    **strict** (layer 3 over layer 2).
  - root has strict only at the top level, **no** `directories.somerepo` entry, `somerepo/cairn.yaml`
    says not-strict ⇒ **not-strict** (layer 2 over layer 1; layer 3 empty).
- **Absolute disable gate, evaluated before any merge or file read:** a root
  `directories.<path>.enabled: false` (or any disabled ancestor) prunes the whole subtree — its own
  `cairn.yaml` is never read, no detection, no verify, nothing under it runs.
- **`config` owns the complexity. CLI does not.** `config` discovers nested `cairn.yaml` files,
  applies the disable gate + cascade, and exposes a resolved-view API (e.g. `Resolve(dir)` →
  effective settings, plus enumeration of active/pruned directories). `verify`/`bump`/`detect` ask
  `config` for resolved settings instead of re-deriving precedence or reading YAML themselves.
**Steps:**
- Define the `Directory` override block type (the repo-level keys, all optional/pointer where
  "unset means inherit") and a top-level `Directories map[string]Directory`; keep `version: "1"`
  as the only mandatory key. Treat this as a **breaking schema change** — bump the config schema to
  `version: "2"`, accept-and-translate `"1"` where cheap or fail with an actionable migration note
  (decide in-slice; do not silently misread an old file).
- Move `version`/`versioning` to top-level repo baseline; drop `project.canonical_version`,
  `project.packages`, `changelog.packages`, `version_sync.files`, `languages.*.dir`.
- Implement the resolver inside `config`: nested-file discovery, the disable gate, and the
  field-level low→high cascade above; provide the resolved-view + active-directory enumeration API.
- Refit existing callers: `version.Resolver`, `verify`, `bump`, and detection consume the new
  `config` API; remove their direct precedence/`packages` logic. Keep behavior equivalent for a
  single-package repo (no `directories:` ⇒ exactly today's lockstep).
- Update **docs/ARCHITECTURE.md**: rewrite the config-schema block to the new shape, add a
  "per-directory config & precedence" subsection (the layered model, absolute disable, own-`version`
  ⇒ independent, config-owns-cascade), and reconcile the Versioning/Detection sections + ADRs.
**Acceptance:** the new schema parses (root baseline + `directories:` map + a directory's own
`cairn.yaml`); the two precedence examples resolve as specified; a root-disabled directory is pruned
and its own `cairn.yaml` is never read; a single-package repo with no `directories:` behaves exactly
as before; the cascade lives in `config` (CLI contexts contain no YAML-reading or precedence logic);
ARCHITECTURE matches. Tested in the `config` package.

### [ ] 10b — Onboarding wizard (`init`)
**Goal:** The headline UX: a fast, friendly `cairn init`, on top of the 10a config model.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Onboarding, extension points) · internal/{detect,config,wiring,report}
**Steps:** Detect → show findings → multiselect features + standards (smart defaults from
detection) → write `cairn.yaml` → run Wiring → print next steps. `--yes` non-interactive.
The choosable standards/providers come from the registries (changelog/commit/CI), so a
newly registered standard appears in the wizard without touching it.
**Acceptance:** `cairn init --yes` produces a valid config + hook + CI in a sample repo in
seconds; interactive run is navigable and concise.

## [ ] 11 — Dogfood + docs polish
**Goal:** Cairn uses Cairn; docs match reality.
**Read:** AGENTS.md · README.md · docs/ARCHITECTURE.md
**Steps:** Add Cairn's own `cairn.yaml`; wire its hook + CI to `cairn verify`; update
README/ARCHITECTURE for any drift; verify the "add a language/standard" guide is accurate.
**Acceptance:** `cairn verify` is green on Cairn itself in CI and via the hook.
