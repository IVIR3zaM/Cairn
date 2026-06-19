<div align="center">

# 🪨 Cairn

**One marker for a healthy repo.**

Unified quality gates, versioning, changelog, and commit hygiene across every language —
built from the tools you already trust.

[![CI](https://github.com/IVIR3zaM/Cairn/actions/workflows/ci.yml/badge.svg)](https://github.com/IVIR3zaM/Cairn/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/IVIR3zaM/Cairn.svg)](https://pkg.go.dev/github.com/IVIR3zaM/Cairn)
[![Go Report Card](https://goreportcard.com/badge/github.com/IVIR3zaM/Cairn)](https://goreportcard.com/report/github.com/IVIR3zaM/Cairn)
[![Latest release](https://img.shields.io/github/v/release/IVIR3zaM/Cairn?sort=semver)](https://github.com/IVIR3zaM/Cairn/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/IVIR3zaM/Cairn)](go.mod)
[![License](https://img.shields.io/github/license/IVIR3zaM/Cairn)](LICENSE)

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

## Install

> **Status: `v0.1.0` (first release).** Pre-built binaries and a Homebrew tap are planned;
> until they land, install from source with Go ≥ 1.23.

```bash
# from source (always works)
go install github.com/IVIR3zaM/Cairn/cmd/cairn@v0.1.0   # pin the release
go install github.com/IVIR3zaM/Cairn/cmd/cairn@latest   # or track the latest tag

# <!-- INSTALL-GUIDE: 0.1.0 — fill in once release artifacts exist -->
# download a pre-built binary
#   curl -sSL https://github.com/IVIR3zaM/Cairn/releases/download/v0.1.0/cairn_<os>_<arch>.tar.gz | tar xz
# or Homebrew (planned)
#   brew install IVIR3zaM/tap/cairn
```

`cairn` shells out to each language's standard tools, so install the ones you use
(`cairn doctor` tells you what's present vs missing, with install hints).

## Quickstart

```bash
cairn init       # detect languages + tools, pick features/standards, write cairn.yaml,
                 # install the git hook, generate the CI workflow  (~30s wizard, or --yes)
cairn verify     # the unified gate: format · lint · typecheck · test · doc-honesty
cairn bump minor # bump version across manifests + docs, promote the changelog
cairn doctor     # what's installed vs missing per language, with install hints
```

## Supported languages

Java · TypeScript/JavaScript · Go · Rust · Dart/Flutter · Python
*(adding another is two self-registering files, no central edits — see
[ARCHITECTURE](docs/ARCHITECTURE.md) "Extension points".)*

## Standards — opinionated defaults you can swap

Cairn ships sensible defaults and lets you choose alternatives at `cairn init`:

| Concern        | Default                | Alternatives / add-ons                              |
| -------------- | ---------------------- | --------------------------------------------------- |
| Versioning     | **SemVer**             | CalVer                                              |
| Commit messages| **Conventional Commits** | *(gitmoji / none are future one-file additions)*  |
| Changelog      | **Keep a Changelog**   | pub.dev (`dart`) *(git-cliff / others planned)*     |
| Lint/format/test | language standard tool | per-language presets (e.g. ruff vs black+flake8)  |

Every concern and every step is individually **enable/disable** and **required/optional**.

## How it works

```
            ┌────────────── cairn.yaml ──────────────┐
            │  languages · features · standards       │
            │  + per-directory overrides              │
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

1. **Detect.** Cairn scans for marker files (`go.mod`, `Cargo.toml`, `package.json`,
   `pyproject.toml`, `pom.xml`/`build.gradle`, `pubspec.yaml`) to learn which languages live
   where, then resolves each language's standard tools on your `PATH`.
2. **Configure.** One `cairn.yaml` (schema `"2"`) holds the repo baseline plus a `directories:`
   map of overrides; any directory may also carry its own `cairn.yaml`. `config` resolves the
   effective settings for any directory via a field-level cascade (baseline < a directory's own
   file < a root `directories.<path>` entry), with an absolute `enabled: false` disable gate. A
   directory with its own `version:` is **independently versioned** (its own tag + changelog);
   otherwise it moves in **lockstep** with the repo version.
3. **Verify.** `cairn verify` builds an ordered plan per detected unit and shells out to each
   tool, then runs the **honesty checks**: every version manifest, multi-package workspace
   interdependency, and configured `version_sync` doc pattern must quote the right version.
4. **Bump.** `cairn bump` computes the next version (a SemVer level, an explicit version, a
   CalVer date-step, or one **inferred from commit history**), updates every manifest, rewrites
   `version_sync` docs, reconciles workspace interdependencies, promotes the changelog, and
   prints a suggested release commit/tag — it **never commits for you**.
5. **Wire.** `cairn init` installs the git hooks (into a tracked `.cairn/hooks` dir via
   `core.hooksPath`) and generates the CI workflow, both running `cairn verify`.

Everything multi-implementation is a **self-registering registry** — languages, manifest
managers, changelog/commit/CI standards — so adding one is dropping a file, never editing an
engine or a `switch`.

## Limitations

Cairn is at `v0.1.0`; known boundaries today:

- **You must install the underlying tools.** Cairn orchestrates; it does not bundle gofumpt,
  ruff, clippy, etc. `cairn doctor` shows what's missing. A missing **required** tool fails
  that stage; an optional one warns and is skipped.
- **CI generation targets GitHub Actions only** (the registry makes GitLab/others a one-file
  addition, not yet shipped). The generated workflow installs Cairn via `go install …@latest`
  and runs `cairn verify` — you still provide the language toolchains in CI.
- **Commit convention = Conventional Commits**, changelog = Keep a Changelog or pub.dev.
  gitmoji, git-cliff, and conventional-changelog are designed-for but not yet implemented.
- **Java/Gradle is single-root**: detection collapses a multi-module build to its reactor root
  and verifies once; per-submodule `<parent>` version reconciliation is a follow-up.
- **No pre-built binaries or Homebrew tap yet** — install from source (see [Install](#install)).
- **`cairn bump` never commits, tags, or pushes** — by design. It mutates files and prints the
  commands; you run them.
- **Not a hosted service** — Cairn is a local/CI CLI, with no daemon or server component.

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — design (DDD / ports & adapters), config
  schema, extension points, ADRs.
- [AGENTS.md](AGENTS.md) — contributor & AI-agent guide; how we work.
- [docs/ITERATIONS.md](docs/ITERATIONS.md) — the iterative build plan.
- [docs/PROMPT.md](docs/PROMPT.md) — the reusable "do the next iteration" prompt.
- [CHANGELOG.md](CHANGELOG.md) — release notes (Keep a Changelog).

## Status

🐢 Built iteratively and now **dogfooding itself**: Cairn's own [cairn.yaml](cairn.yaml)
drives its pre-commit hook and [CI](.github/workflows/ci.yml), both running the same
`cairn verify`. The CLI grows one small iteration at a time — see
[docs/ITERATIONS.md](docs/ITERATIONS.md).

## Releasing

Releases are cut by pushing a `vX.Y.Z` tag; a GitHub Actions workflow
([release.yml](.github/workflows/release.yml)) builds the cross-platform binaries with
[GoReleaser](https://goreleaser.com) and publishes a GitHub Release. See
[docs/RELEASING.md](docs/RELEASING.md).

## License

[Apache-2.0](LICENSE).
