# Architecture

Cairn is a small **domain core** wrapped in **thin adapters** that shell out to
in-market tools. This file is the single source of truth for the design; iterations
([ITERATIONS.md](ITERATIONS.md)) reference it rather than restating it.

## Guiding constraints

1. **Never reimplement a tool.** Adapters shell out. The core knows *what* must happen
   (format, lint, test, bump…); adapters know *how* to ask a specific tool.
2. **Keep it simple.** Introduce an abstraction only when a second real case earns it.
3. **Hexagonal / DDD-light.** A pure domain core defines ports; the outside world
   (tools, filesystem, git, terminal) lives behind adapters.
4. **UX is a first-class concern**, not an afterthought — it has its own context.

## Bounded contexts

Each context is a package with a clear responsibility and a small public surface.

| Context              | Responsibility                                                        |
| -------------------- | --------------------------------------------------------------------- |
| **Config**           | Load/validate/merge `cairn.yaml`; the aggregate root for everything.  |
| **Detection**        | Find languages, their dirs, package managers, and installed tools.    |
| **QualityGate**      | Orchestrate `verify`: order steps, run them, collect results.         |
| **Versioning**       | SemVer/CalVer math; bump manifests; version/doc honesty checks.       |
| **Changelog**        | Promote/generate the changelog per chosen standard.                   |
| **CommitConventions**| Validate commit messages; infer the version bump from history.        |
| **Onboarding**       | The `init` wizard: detect → choose → write config + wiring.           |
| **Wiring**           | Generate/install git hooks and the CI workflow.                       |
| **UX/Reporter**      | Colorful, concise rendering: summaries, glyphs, spinners, verbosity.  |

## Ports (core interfaces)

The domain depends only on these. Adapters implement them; nothing in the core imports
a concrete tool.

- `ToolRunner` — run an external command, capture output/exit, respect timeout & cwd.
- `Formatter` / `Linter` / `TypeChecker` / `Tester` / `Builder` — one method each:
  `Run(ctx, LangUnit, Mode) StepResult` (Mode = check | fix).
- `VersionManager` — read current version, write a new one for a language manifest.
- `ChangelogWriter` — promote `[Unreleased]` → version, or generate from commits.
- `CommitValidator` — validate one message against the chosen convention; classify it
  (feat/fix/breaking) for bump inference.
- `Reporter` — the UX port (start/step/summary/error); has a TTY and a plain impl.

Adapters are named `<context>/<lang>` or `<context>/<tool>`, e.g.
`quality/python_ruff`, `version/cargo`, `changelog/keepachangelog`. Each is thin and
individually testable with a fake `ToolRunner`.

## The config aggregate — `cairn.yaml`

```yaml
version: "1"

project:
  canonical_version: "0.1.0"     # source of truth for version-sync
  versioning: semver             # semver | calver

languages:                        # auto-detected; user-editable
  go:     { dir: ".",  enabled: true }
  python: { dir: "py", enabled: true, standard: ruff }   # ruff | black+flake8

verify:                           # global toggles; per-language override allowed
  format:    { enabled: true,  required: true,  mode: check }
  lint:      { enabled: true,  required: true }
  typecheck: { enabled: true,  required: false }
  test:      { enabled: true,  required: true }
  build:     { enabled: false }

commits:
  convention: conventional        # conventional | gitmoji | none
  signoff: false                  # DCO
  validate_hook: true             # install commit-msg hook

changelog:
  standard: keepachangelog        # keepachangelog | git-cliff | conventional-changelog
  file: CHANGELOG.md

version_sync:                     # the doc-honesty check (Cairn's signature feature)
  files:
    - { path: README.md, patterns: ["mylib:{VERSION}", "version-{VERSION}"] }

hooks: { pre_commit: [verify], commit_msg: [commit-lint], pre_push: [] }
ci:    { provider: github, jobs: [verify] }

addons: { editorconfig: false, license_header: false, branch_name: false }
```

Config is loaded once, validated, and passed (read-only) into every context. Defaults
live in code so a minimal `cairn.yaml` still works.

## Data flow

**`cairn verify`**
`Config → Detection (which langs/tools are present) → QualityGate builds an ordered
plan (format→lint→typecheck→test→build per language, + version_sync honesty) →
runs each via its adapter through ToolRunner → Reporter renders a compact summary →
exit code = worst result.` Missing tool ⇒ behave per `required` (fail vs warn+skip with
install hint).

**`cairn bump <level|version>`**
`Config → Versioning computes next version (explicit, level, or inferred from commits) →
VersionManager updates each language manifest → version_sync rewrites doc patterns →
Changelog promotes [Unreleased] → Reporter prints diff summary + suggested commit/tag.
Never commits automatically.`

**`cairn init`**
`Detection → wizard (multiselect features + standards, smart defaults) → write
cairn.yaml → Wiring installs hook + generates CI → Reporter prints next steps.`

## Adding a language or standard (extension point)

A new language = one adapter file per port it needs (usually format/lint/test) plus a
detection entry (the marker files). A new standard = one adapter implementing the
relevant port (e.g. a `changelog/gitcliff`). No core changes. Document the markers and
the default tool in a single registry file so detection and docs stay in sync.

## Implementation notes

- **Language:** Go — single static cross-platform binary, no runtime dependency in the
  user's repo, cobra for commands. (ADR-001)
- Run independent steps **concurrently** where safe; render results in stable order.
- Honor `NO_COLOR`, non-TTY (CI plain mode), `--quiet`, `--verbose`.

## ADRs (one-liners; expand only if reversed)

- **ADR-001 — Go as the implementation language.** Single binary, zero user-repo runtime
  dep, strong CLI ecosystem (cobra, charm).
- **ADR-002 — Shell out, never reimplement.** Cairn wraps tools; it owns glue, not logic.
- **ADR-003 — One config aggregate (`cairn.yaml`).** Single source of truth; code holds
  defaults so minimal configs work.
- **ADR-004 — Hexagonal ports & adapters.** Keeps the core tool-agnostic and testable
  with fakes; makes new languages/standards additive.
- **ADR-005 — `verify` is the shared contract.** Hook and CI both call it, so local and
  CI never drift.
