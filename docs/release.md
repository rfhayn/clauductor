# Cutting a release

A release is a tag `vX.Y.Z` on `main`. Pushing the tag runs `.github/workflows/release.yml`,
which:

1. checks the tag against `Version` in `framework/internal/cmd/root.go`, and checks that
   `CHANGELOG.md` has a dated section for it (`scripts/release-check.sh vX.Y.Z`);
2. builds `darwin/arm64`, `darwin/amd64`, `linux/amd64` and `linux/arm64`, each on its own
   runner (`scripts/release/build.sh`). cgo is on for SQLite, so the builds cannot be
   cross-compiled from one machine;
3. smoke-tests each archive (`clauductor version` must print the tag; darwin/amd64 is only
   checked with `file`, because it is built on arm64);
4. creates a **draft** GitHub Release with the four archives and `checksums.txt`, and uses the
   CHANGELOG section as its notes.

Nothing is public until someone publishes the draft. Publishing is also what triggers
`release-homebrew.yml`, once the tap is switched on ([Homebrew](#homebrew)).

A pull request that touches the release path (the workflow, `scripts/release/`,
`scripts/release-check.sh`, `CHANGELOG.md`, `root.go`, `install.sh`) runs the same builds with a
`-dev` version and publishes nothing. That way a broken build shows up on the PR, not on the tag.

## Cut v0.1.0

The first release goes out once the panel stack (PANEL-14 to PANEL-20) and the operating-model
work in flight (OPS-8 to OPS-10) have merged, because the README describes them.

1. **Sync `main`, and branch.**
   ```bash
   git switch main && git pull
   git switch -c release/v0.1.0
   ```
2. **Version.** `framework/internal/cmd/root.go` already says `var Version = "0.1.0"`. For a
   later release, bump it there. This is the only place the version lives.
3. **Plugin.** The plugin's version comes from `Version`, and `TestCommittedPluginIsCurrent`
   fails until `plugin/` is rebuilt:
   ```bash
   scripts/build-plugin.sh
   ```
4. **Changelog.**
   ```bash
   scripts/changelog.sh          # lists every merged PR that no released section cites yet under [Unreleased]
   ```
   - Sort the new entries into `### Added`, `### Changed` and `### Fixed` under `## [0.1.0]`.
   - Change that header to `## [0.1.0] - YYYY-MM-DD`.
   - Put `Nothing merged since the last release.` back under `[Unreleased]`.
   - For a later release, add its compare link at the bottom of the file.
5. **Check.**
   ```bash
   scripts/release-check.sh v0.1.0             # prints 0.1.0
   cd framework && go test -short ./... && cd ..
   ```
6. **Land it.** Commit (`REL-1: Prepare v0.1.0`), open the PR, and merge it once CI is green.
   The release workflow's PR builds must pass too.
7. **Tag the merge commit and push the tag.**
   ```bash
   git switch main && git pull
   git tag -a v0.1.0 -m "clauductor v0.1.0"
   git push origin v0.1.0
   ```
8. **Review the draft.** Open it from the Actions run or the
   [releases page](https://github.com/rfhayn/clauductor/releases). Check the notes and the five
   assets, then try one:
   ```bash
   CLAUDUCTOR_RELEASE_BASE=<a directory with the downloaded assets, as file://...> ./install.sh --release
   ```
   Then publish the draft. After that, `install.sh` with no clone and
   `releases/latest/download/...` both resolve to v0.1.0.

**If the tag was wrong**, delete it before you publish anything:
`git push origin :refs/tags/v0.1.0 && git tag -d v0.1.0`. Then delete the draft, fix the
problem, and tag again. Never move a published tag.

## Homebrew

The tap is off by default. To switch it on:

1. Create the tap repository `rfhayn/homebrew-clauductor` (public, empty, with a `Formula/`
   directory).
2. Create a fine-grained token with **Contents: read and write** on that repository only. Add it
   to this repository as the secret `HOMEBREW_TAP_TOKEN`.
3. Set the repository variable `HOMEBREW_TAP_REPO` to `rfhayn/homebrew-clauductor`.

From then on, publishing a release renders `packaging/homebrew/clauductor.rb.tmpl` with that
release's checksums (`scripts/release/formula.sh`) and pushes `Formula/clauductor.rb` to the tap.
To render one for a release that is already out, run `release-homebrew` from the Actions tab
with its tag. To check a formula locally:

```bash
scripts/release/formula.sh v0.1.0 checksums.txt > clauductor.rb && ruby -c clauductor.rb
brew install --formula ./clauductor.rb && brew test clauductor
```

## Not done yet

- **Code signing and notarisation.** The macOS binaries are unsigned. `curl` downloads run, and
  browser downloads need `xattr -d com.apple.quarantine` ([install.md](install.md#by-hand)).
- **A universal macOS binary.** There are two archives, one per architecture, instead.
- **Native Windows.** The binary does not build for native Windows (it uses `flock` and a PTY).
  Use WSL.
- **SBOMs and provenance attestations.**
