# Releasing Cairn

Cairn ships from **git tags**. Pushing a `vX.Y.Z` tag triggers
[`.github/workflows/release.yml`](../.github/workflows/release.yml), which runs
[GoReleaser](https://goreleaser.com) ([`.goreleaser.yaml`](../.goreleaser.yaml)) to build the
cross-platform binaries and publish a GitHub Release. No binaries live in git.

## One-time setup

The repo must exist on GitHub at **`IVIR3zaM/Cairn`** (matching the `go.mod` module path).

```bash
# if the remote isn't set yet:
git remote add origin git@github.com:IVIR3zaM/Cairn.git
git push -u origin main
```

GoReleaser authenticates with the workflow's built-in `GITHUB_TOKEN` — no extra secret is
needed for a same-repo GitHub Release.

## Cutting a release (e.g. `v0.1.0`)

1. **Finalize the changelog.** Ensure `CHANGELOG.md` has the version section ready (Cairn can
   do this for you — `cairn bump` promotes `[Unreleased]` → the dated version and refreshes the
   compare links). For `0.1.0` this is already done.
2. **Bump the version in the repo.** `cairn.yaml`'s `version:` and any manifests must match the
   tag. Either run `cairn bump 0.1.0` (updates manifests + docs + changelog, prints the commit)
   or set it by hand. Then verify it's honest:
   ```bash
   cairn verify        # must be green — the honesty check fails on version drift
   ```
3. **Commit** the release prep:
   ```bash
   git add -A && git commit -m "chore(release): 0.1.0"
   git push origin main
   ```
4. **Tag and push** — this is what fires the release workflow:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```
5. **Watch the workflow** under the repo's *Actions → Release*. On success a GitHub Release
   appears with the binaries, `checksums.txt`, and your `CHANGELOG.md` notes.

## Verifying / dry-running locally

```bash
goreleaser check                          # validate .goreleaser.yaml
goreleaser release --snapshot --clean     # full build, no publish (artifacts in ./dist)
```

## Notes

- The tag drives `cairn --version` via ldflags (`-X …/internal/cli.version={{.Version}}`).
- Pre-release tags (e.g. `v0.2.0-rc.1`) are auto-marked as pre-releases (`prerelease: auto`).
- `cairn bump` never tags or pushes — those two steps are always yours, on purpose.
