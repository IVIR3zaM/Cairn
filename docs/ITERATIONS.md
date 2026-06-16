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

## [ ] 6 — Versioning + doc honesty + `bump`
**Goal:** `cairn bump` and the version_sync honesty check (Cairn's signature).
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Versioning, data flow, extension points) · internal/{config,report}
**Steps:** SemVer (+CalVer) math; per-manifest `VersionManager` as a **self-registering
registry** (`internal/version/manifest_<name>.go`, `register`/`ManagerFor`, panic on dup),
each delegating to native tooling where it exists else safe regex — no central manifest
map; version_sync rewrite + a non-mutating honesty assertion wired into `verify`; `cairn
bump` prints suggested commit/tag, never commits.
**Acceptance:** `bump` updates manifests + doc patterns; `verify` fails on drifted docs;
downgrade and empty-version cases are guarded. Tests cover the math + sync; adding a
manifest is one self-registering file (no engine edits).

## [ ] 7 — Changelog (Keep a Changelog)
**Goal:** Promote `[Unreleased]` → version+date with refreshed compare links on `bump`.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Changelog, extension points) · internal/version
**Steps:** Stand up the changelog **standard registry** (`internal/changelog`,
`register`/`WriterFor`) and add the keepachangelog writer as `std_keepachangelog.go`
(self-registered, not a special case); integrate into `bump`; warn on empty
`[Unreleased]`. `git-cliff`/`conventional-changelog` are future `std_<name>.go` files —
documented, not stubbed.
**Acceptance:** A sample CHANGELOG is promoted correctly (idempotent, links updated);
empty-Unreleased warns; `WriterFor("keepachangelog")` resolves via self-registration.

## [ ] 8 — Commit conventions + commit-msg hook + bump inference
**Goal:** Validate commit messages and infer the SemVer bump from history.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (CommitConventions, extension points) · internal/version
**Steps:** `CommitValidator` as a **convention registry** (`internal/commit`,
`register`/`ValidatorFor`); add `conv_conventional.go` (self-registered), leaving
gitmoji/none as future `conv_<name>.go` files; classify feat/fix/breaking; optional
sign-off; `cairn bump` (no level) infers the next version from commits since the last tag.
**Acceptance:** Valid/invalid messages classified correctly; inference picks the right
level on a fixture history; the convention resolves via `ValidatorFor` per config.

## [ ] 9 — Wiring: hooks + CI generation
**Goal:** `init`'s outputs — install git hooks and generate a CI workflow calling `verify`.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Wiring, extension points) · internal/config
**Steps:** Install pre-commit (`cairn verify`) and optional commit-msg hooks via a tracked
hooks dir + `core.hooksPath`; make CI providers a **self-registering registry**
(`internal/wiring/ci_<name>.go`, `register`/`ProviderFor`) and add `ci_github.go` as the
first entry — other providers are later one-file additions, not a `switch`.
**Acceptance:** Hooks installed and runnable; generated GitHub workflow is valid and runs
`cairn verify`; re-running is idempotent; `ProviderFor("github")` resolves via self-registration.

## [ ] 10 — Onboarding wizard (`init`)
**Goal:** The headline UX: a fast, friendly `cairn init`.
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
