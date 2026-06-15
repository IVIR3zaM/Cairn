<div align="center">

# 🪨 Cairn

**One marker for a healthy repo.**

Unified quality gates, versioning, changelog, and commit hygiene across every language —
built from the tools you already trust.

</div>

---

## What is Cairn?

A cairn is a trail marker built by *stacking stones that are already there*. You don't
carve new rock — you arrange what exists into a marker that shows the right way.

Cairn (the tool) does the same for your repository. It is a polyglot **orchestrator**:
one config file, one git hook, and one CI job that unify the chores every healthy repo
needs — running format, lint, type-check, test, versioning, changelog, and commit-message
checks through the **best in-market tools for each language**.

### The one rule: don't reinvent the wheel

Cairn **never** reimplements a linter, formatter, test runner, version engine, or
changelog generator. It *detects* the languages in your repo and *shells out* to the
standard tool for each. Cairn's only original code is the glue: detection, the unified
config, version/doc honesty checks, the onboarding wizard, and hook/CI generation.

## Why

Most projects re-solve the same problems by hand, slightly differently each time:

- A `bump-version` script that also has to fix the version in the README.
- A `verify` script that runs lint + format + tests so nothing lands broken.
- A pre-commit hook and a CI job that must stay in sync with both.

Cairn turns that scattered, copy-pasted boilerplate into **one tool with one config**,
identical across a Java service, a TypeScript package, a Go binary, a Rust crate, a Dart
library, and a Python project.

## Supported languages

Java · TypeScript/JavaScript · Go · Rust · Dart/Flutter · Python
*(adding another is two self-registering files, no central edits — see
[ARCHITECTURE](docs/ARCHITECTURE.md) "Extension points".)*

## Standards — opinionated defaults you can swap

Cairn ships sensible defaults and lets you choose alternatives at `cairn init`:

| Concern        | Default                | Alternatives / add-ons                              |
| -------------- | ---------------------- | --------------------------------------------------- |
| Versioning     | **SemVer**             | CalVer                                              |
| Commit messages| **Conventional Commits** | commitlint preset · Gitmoji · DCO / GPG-signed   |
| Changelog      | **Keep a Changelog**   | git-cliff · conventional-changelog                  |
| Lint/format/test | language standard tool | per-language presets (e.g. ruff vs black+flake8)  |
| Cross-cutting  | *(off by default)*     | EditorConfig · license-header · branch-name rules   |

Every concern and every step is individually **enable/disable** and **required/optional**.

## Quickstart

```bash
# install (one of)
go install github.com/<owner>/cairn@latest
brew install cairn

# in your repo
cairn init       # detect languages + tools, pick features/standards, write cairn.yaml,
                 # install the git hook, generate the CI workflow  (~30s wizard)
cairn verify     # the unified gate: format · lint · typecheck · test · doc-honesty
cairn bump minor # bump version across manifests + docs, promote the changelog
cairn doctor     # what's installed vs missing per language, with install hints
```

## How it works

```
            ┌────────────── cairn.yaml ──────────────┐
            │  languages · features · standards       │
            └────────────────────┬────────────────────┘
                                 │
   detect ──▶  core (ports) ──▶ thin adapters ──▶ in-market tools
   languages   verify/bump/...   (one per tool)    eslint, ruff, clippy,
   & tools                                         spotless, dart, go test…
                                 │
                ┌────────────────┴────────────────┐
            git hook                            CI job
         (pre-commit / commit-msg)         (GitHub Actions)
```

Both the git hook and CI call the same `cairn verify`, so local and CI never drift.

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — design (DDD / ports & adapters), config
  schema, extension points, ADRs.
- [AGENTS.md](AGENTS.md) — contributor & AI-agent guide; how we work.
- [docs/ITERATIONS.md](docs/ITERATIONS.md) — the iterative build plan.
- [docs/PROMPT.md](docs/PROMPT.md) — the reusable "do the next iteration" prompt.

## Status

🚧 Pre-implementation. This repository currently contains the design and the iteration
plan; the CLI is built one small iteration at a time. See
[docs/ITERATIONS.md](docs/ITERATIONS.md).

## License

[Apache-2.0](LICENSE).
