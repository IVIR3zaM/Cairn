# Agent & Contributor Guide

This is how we work on Cairn — for humans and AI agents alike. Read this once per
session, then follow the per-iteration protocol below.

## What Cairn is (in one line)

A polyglot orchestrator that wraps the best in-market tools for quality, versioning,
changelog, and commit hygiene behind one config, one hook, and one CI job. See
[README](README.md) for the pitch and [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for
the design.

## Principles

1. **Never reinvent a wheel.** If a tool already does the job, shell out to it. Cairn
   owns glue, not linting/formatting/testing logic.
2. **Keep it simple.** No speculative abstraction. Add a pattern only when a *second*
   real case earns it. Optimize for "easy to navigate" over clever.
3. **DDD-light / hexagonal.** Respect the bounded contexts and ports in ARCHITECTURE.
   The core never imports a concrete tool; adapters do.
4. **Meaningful tests only.** Test detection, config merge/validation, version-sync,
   the bump math, the report summarizer, and command exit codes. Do **not** pad
   coverage with trivial struct/getter tests.
5. **UX is a feature.** Output is colorful but *concise*: a tight summary with status
   glyphs (✓ ✗ ⊘ !), not walls of logs. Full logs only behind `--verbose`.

## Token-frugal working protocol (important)

Each iteration is a small vertical slice. To keep runs cheap:

- Read **AGENTS.md** once. Then read **only** the current iteration entry in
  [docs/ITERATIONS.md](docs/ITERATIONS.md).
- Read **only** the files that iteration's **Read:** list names. Do not scan the repo,
  re-read whole docs, or open files "just in case".
- Prefer targeted edits over re-reading large files. Don't re-read a file you just
  wrote — the editor confirms success.
- If you discover the iteration needs a file not in its Read-list, add it to that
  entry's Read-list (so the next agent benefits), then proceed.
- Keep new docs/code skimmable: one source of truth per fact, no copy-paste across
  files, cross-link instead.

## Definition of done (every iteration)

1. Code compiles and the iteration's **Acceptance** criteria pass.
2. Meaningful tests added/updated and green.
3. `CHANGELOG.md` `[Unreleased]` updated with a one-line entry.
4. The iteration's status ticked in `docs/ITERATIONS.md`.
5. A suggested **Conventional Commit** message proposed (do not commit unless asked).

## Conventions

- **Commits:** [Conventional Commits](https://www.conventionalcommits.org/) — `feat:`,
  `fix:`, `docs:`, `refactor:`, `test:`, `chore:`. Breaking → `feat!:` / `BREAKING
  CHANGE:`. (Cairn dogfoods this.)
- **Versioning:** [SemVer](https://semver.org/).
- **Changelog:** [Keep a Changelog](https://keepachangelog.com/) — every change lands
  under `[Unreleased]` in the right `### Added/Changed/Fixed/Removed` group.
- **Go style:** standard `gofmt`/`gofumpt`; `golangci-lint` clean; small packages
  aligned to the bounded contexts.

## Repo map

```
README.md              # pitch + quickstart
AGENTS.md              # this file
CHANGELOG.md           # Keep a Changelog
cairn.yaml             # Cairn dogfooding its own config (added in an early iteration)
docs/
  ARCHITECTURE.md      # design, config schema, ADRs
  ITERATIONS.md        # the ordered build plan (read one entry at a time)
  PROMPT.md            # the reusable "do the next iteration" prompt
```
(Source layout under the bounded contexts appears as iterations add it.)

## How to work on this project

1. Skim ARCHITECTURE only if the iteration touches a context you haven't seen.
2. Run the next-iteration prompt in [docs/PROMPT.md](docs/PROMPT.md).
3. One iteration per run. Stop when its Definition of Done is met.
