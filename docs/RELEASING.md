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

### Homebrew tap (so `brew install IVIR3zaM/tap/cairn` works)

`brew install IVIR3zaM/tap/cairn` resolves to a **separate** repo, `IVIR3zaM/homebrew-tap`,
that holds the formula. GoReleaser publishes the formula there on every non-prerelease tag
(the `brews:` block in [`.goreleaser.yaml`](../.goreleaser.yaml)). Two one-time steps:

1. **Create the tap repo** — a public repo named exactly **`homebrew-tap`** under `IVIR3zaM`
   (empty is fine; GoReleaser commits `Formula/cairn.rb` into it):
   ```bash
   gh repo create IVIR3zaM/homebrew-tap --public \
     --description "Homebrew tap for Cairn" --add-readme
   ```
2. **Add a token secret.** The release runs in `IVIR3zaM/Cairn`, but its built-in
   `GITHUB_TOKEN` can't push to `homebrew-tap`. Create a PAT with write access to the tap
   repo and store it as a secret named `HOMEBREW_TAP_GITHUB_TOKEN` on this repo:
   ```bash
   # fine-grained PAT scoped to the homebrew-tap repo with Contents: read & write,
   # or a classic PAT with the `repo` scope. Then:
   gh secret set HOMEBREW_TAP_GITHUB_TOKEN --repo IVIR3zaM/Cairn
   ```

The formula appears only **after a release runs with this config** — so the `v0.1.0` you
already cut won't have it. Re-cut it (delete the tag/release and push `v0.1.0` again) or ship
the next tag; see "Re-running a release" below.

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

## Re-running a release (e.g. to publish the Homebrew formula for an existing tag)

If a tag was already released without the tap config, delete the release + tag and re-push:

```bash
gh release delete v0.1.0 --repo IVIR3zaM/Cairn --yes   # remove the GitHub Release
git push origin :refs/tags/v0.1.0                       # delete the remote tag
git tag -d v0.1.0                                        # delete the local tag
git tag v0.1.0 && git push origin v0.1.0                # re-tag → re-fires the workflow
```

GoReleaser runs `--clean`, so a re-run rebuilds and re-uploads everything (binaries,
checksums, and now the Homebrew formula).

## Notes

- The tag drives `cairn --version` via ldflags (`-X …/internal/cli.version={{.Version}}`).
- Pre-release tags (e.g. `v0.2.0-rc.1`) are auto-marked as pre-releases (`prerelease: auto`).
- `cairn bump` never tags or pushes — those two steps are always yours, on purpose.
