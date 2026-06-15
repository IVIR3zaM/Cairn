# Iterations

The ordered plan from empty repo to a dogfooded MVP. **Do one iteration per run.** Each
entry is self-contained: only read the files in its **Read:** line. Tick the box when
its **Acceptance** is met. Keep entries small — if one grows, split it.

Legend: `[ ]` todo · `[~]` in progress · `[x]` done.

> Conventions for every iteration: add meaningful tests, update `CHANGELOG.md`
> `[Unreleased]`, propose a Conventional Commit message. (See [AGENTS.md](../AGENTS.md).)

---

## [ ] 0 — Scaffolding
**Goal:** A buildable Go CLI skeleton with `cairn --version` and `cairn doctor` stubs.
**Read:** AGENTS.md
**Steps:** `go mod init`; add cobra; root command + `version`; empty `doctor` printing
"not implemented"; Makefile or `tool/verify.sh` placeholder; `.gitignore`; GitHub Actions
running `go build` + `go test ./...` + `golangci-lint`.
**Acceptance:** `go build ./...` and `go test ./...` pass; `cairn --version` prints.

## [ ] 1 — Config domain
**Goal:** Load, validate, and default-merge `cairn.yaml` into a typed aggregate.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Config + schema sections)
**Steps:** Define config structs matching the schema; YAML load; validation with clear
errors; in-code defaults so a minimal file works; a `LoadOrDefault(path)`.
**Acceptance:** Tests cover: full file, minimal file (defaults fill in), invalid file
(actionable error). No I/O outside the loader.

## [ ] 2 — Detection + `doctor`
**Goal:** Detect languages, dirs, package managers, and which standard tools are installed.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Detection) · internal/config
**Steps:** A `registry` mapping each language → marker files + default tools. Scan the
repo; resolve installed tools via `exec.LookPath`. Implement `cairn doctor` to print a
per-language installed/missing table with install hints.
**Acceptance:** On fixture repos (one per language) detection is correct; `doctor` lists
present vs missing tools. Tests use a fake filesystem/lookup.

## [ ] 3 — ToolRunner + Reporter ports
**Goal:** The two cross-cutting ports, with real + fake implementations.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Ports, UX/Reporter)
**Steps:** `ToolRunner` (cwd, timeout, captured output, exit code). `Reporter` port with
a TTY impl (color, glyphs ✓✗⊘!, spinner, compact summary) and a plain CI impl; honor
`NO_COLOR`/`--quiet`/`--verbose`/non-TTY. Add a fake ToolRunner for tests.
**Acceptance:** Reporter renders a stable, compact summary in tests; plain mode has no
ANSI; ToolRunner captures exit/output correctly.

## [ ] 4 — QualityGate + Go adapter (`verify` end-to-end)
**Goal:** `cairn verify` works fully for one language (Go), proving the whole spine.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (QualityGate) · internal/{config,detect,runner,report}
**Steps:** Step ports (Formatter/Linter/Tester/…); a `quality/go` adapter wrapping
gofumpt/golangci-lint/go test; ordered plan builder; missing-tool ⇒ required vs warn+skip
with hint; wire `cairn verify`.
**Acceptance:** Green Go fixture ⇒ pass; fixture with a lint/format/test error ⇒ non-zero
with a compact failing summary; missing tool degrades per `required`.

## [ ] 5 — Remaining language adapters
**Goal:** `verify` supports Java, TS/JS, Rust, Dart, Python.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (tool matrix) · internal/quality/go (as template)
**Steps:** One thin adapter per language wrapping its matrix tools (see ARCHITECTURE);
respect per-language `standard` choice (e.g. ruff vs black+flake8, eslint vs biome).
Split into sub-iterations (5a…5f) if large.
**Acceptance:** Each language has a green + a failing fixture passing/failing correctly;
adapters are thin and tested with the fake ToolRunner.

## [ ] 6 — Versioning + doc honesty + `bump`
**Goal:** `cairn bump` and the version_sync honesty check (Cairn's signature).
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Versioning, data flow) · internal/{config,report}
**Steps:** SemVer (+CalVer) math; per-manifest `VersionManager` (delegate to native
tooling where it exists, else safe regex); version_sync rewrite + a non-mutating honesty
assertion wired into `verify`; `cairn bump` prints suggested commit/tag, never commits.
**Acceptance:** `bump` updates manifests + doc patterns; `verify` fails on drifted docs;
downgrade and empty-version cases are guarded. Tests cover the math + sync.

## [ ] 7 — Changelog (Keep a Changelog)
**Goal:** Promote `[Unreleased]` → version+date with refreshed compare links on `bump`.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Changelog) · internal/version
**Steps:** `changelog/keepachangelog` adapter; integrate into `bump`; warn on empty
`[Unreleased]`. Leave `git-cliff`/`conventional-changelog` as documented future adapters.
**Acceptance:** A sample CHANGELOG is promoted correctly (idempotent, links updated);
empty-Unreleased warns.

## [ ] 8 — Commit conventions + commit-msg hook + bump inference
**Goal:** Validate commit messages and infer the SemVer bump from history.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (CommitConventions) · internal/version
**Steps:** `CommitValidator` for Conventional Commits (+ optional gitmoji/sign-off);
classify feat/fix/breaking; `cairn bump` (no level) infers the next version from commits
since the last tag.
**Acceptance:** Valid/invalid messages classified correctly; inference picks the right
level on a fixture history.

## [ ] 9 — Wiring: hooks + CI generation
**Goal:** `init`'s outputs — install git hooks and generate a CI workflow calling `verify`.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Wiring) · internal/config
**Steps:** Install pre-commit (`cairn verify`) and optional commit-msg hooks via a tracked
hooks dir + `core.hooksPath`; generate a GitHub Actions workflow; structure for other CI
providers later.
**Acceptance:** Hooks installed and runnable; generated workflow is valid and runs
`cairn verify`; re-running is idempotent.

## [ ] 10 — Onboarding wizard (`init`)
**Goal:** The headline UX: a fast, friendly `cairn init`.
**Read:** AGENTS.md · docs/ARCHITECTURE.md (Onboarding) · internal/{detect,config,wiring,report}
**Steps:** Detect → show findings → multiselect features + standards (smart defaults from
detection) → write `cairn.yaml` → run Wiring → print next steps. `--yes` non-interactive.
**Acceptance:** `cairn init --yes` produces a valid config + hook + CI in a sample repo in
seconds; interactive run is navigable and concise.

## [ ] 11 — Dogfood + docs polish
**Goal:** Cairn uses Cairn; docs match reality.
**Read:** AGENTS.md · README.md · docs/ARCHITECTURE.md
**Steps:** Add Cairn's own `cairn.yaml`; wire its hook + CI to `cairn verify`; update
README/ARCHITECTURE for any drift; verify the "add a language/standard" guide is accurate.
**Acceptance:** `cairn verify` is green on Cairn itself in CI and via the hook.
