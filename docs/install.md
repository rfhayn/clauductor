# Installing clauductor

There are three ways to install. Pick one:

| | You get | Needs |
|---|---|---|
| [A release binary](#a-release-binary) | the `clauductor` binary and `template/` | curl |
| [From source](#from-source) | the same, built from a clone | Go 1.24+ |
| [The plugin](#the-claude-code-plugin) | the operating model only, with no binary | Claude Code |

The panel and `clauductor init`/`install` need the binary. The operating model alone does not
(see [plugin.md](plugin.md)). The platform notes and what else to install are in the
[README](../README.md#requirements).

## A release binary

> Nothing has been published yet. Until v0.1.0 is released, build [from source](#from-source).

```bash
curl -fsSL https://raw.githubusercontent.com/rfhayn/clauductor/main/install.sh | bash
# or, from a clone:
./install.sh --release                # the latest release
./install.sh --version v0.1.0         # a given release
```

`install.sh` works out your platform (`darwin` or `linux`, `arm64` or `amd64`) and downloads
`clauductor-<os>-<arch>.tar.gz` and the release's `checksums.txt`. It refuses to install if the
SHA-256 does not match. Then it:

1. unpacks the archive to `~/.local/share/clauductor/<version>/` and points
   `~/.local/share/clauductor/current` at it;
2. copies the binary to `~/.local/bin/clauductor`;
3. adds `export CLAUDUCTOR_FRAMEWORK=~/.local/share/clauductor/current` to `~/.zshrc` (or
   `~/.bashrc`), unless the file already sets it. If it sets a different value, it tells you what
   the line should be, and does not change it.

`CLAUDUCTOR_FRAMEWORK` matters because `clauductor init`, `install` and `update` copy the operating
model from `$CLAUDUCTOR_FRAMEWORK/template`. The binary does not contain it.

You can change where things go with `INSTALL_DIR` (the binary) and `CLAUDUCTOR_SHARE` (the
unpacked releases). `CLAUDUCTOR_RELEASE_BASE` downloads from a mirror, or from a `file://`
directory, instead of GitHub.

### By hand

Download the archive for your platform and `checksums.txt` from the
[releases page](https://github.com/rfhayn/clauductor/releases), then:

```bash
sha256sum -c --ignore-missing checksums.txt    # macOS: shasum -a 256 -c --ignore-missing checksums.txt
tar -xzf clauductor-darwin-arm64.tar.gz
mv clauductor-darwin-arm64 ~/.local/share/clauductor-release
ln -s ~/.local/share/clauductor-release/clauductor ~/.local/bin/clauductor
export CLAUDUCTOR_FRAMEWORK=~/.local/share/clauductor-release   # and add it to your shell profile
```

The binaries are not signed or notarised. `curl` does not mark a download as quarantined, but a
browser does. If macOS says the binary "cannot be opened", run
`xattr -d com.apple.quarantine clauductor`.

### Homebrew

> The tap is prepared but not switched on yet ([release.md](release.md#homebrew)).

```bash
brew install rfhayn/clauductor/clauductor
```

The formula installs `tmux`, `gh` and `jq` too, and sets `CLAUDUCTOR_FRAMEWORK` for you.

## From source

```bash
git clone https://github.com/rfhayn/clauductor.git ~/clauductor
cd ~/clauductor && ./install.sh
source ~/.zshrc
```

In a clone, `install.sh` builds with Go when Go is installed, which was its only mode before
REL-1. On macOS it installs Go and tmux with Homebrew if they are missing. If Go is not installed,
it tries a release binary first, and builds only if no release is available. `--source` always
builds.

`CLAUDUCTOR_FRAMEWORK` then names the clone, so `git pull && ./install.sh` updates the binary and
the template together.

`go install` does not work from the module path. The module is `github.com/clauductor/clauductor`,
in `framework/`, and that is not where the repository lives. From a clone, this works:

```bash
cd framework && go install ./cmd/clauductor     # then: export CLAUDUCTOR_FRAMEWORK=<the clone>
```

## The Claude Code plugin

```
/plugin marketplace add rfhayn/clauductor
/plugin install clauductor@clauductor
/clauductor:init
```

See [plugin.md](plugin.md). Use the plugin or `clauductor install` in a repository, not both.

## Upgrading

| Installed with | Upgrade with |
|---|---|
| a release | run `install.sh` again (`--release` in a clone) |
| source | `git pull && ./install.sh` |
| Homebrew | `brew upgrade clauductor` |

Then:

- If you use the login agent, run `clauductor panel install` again: it runs its own copy of the
  binary in `~/.clauductor/panel/bin/`.
- In each repository that uses the operating model, run `clauductor update` (or
  `clauductor install`, which refreshes the framework files only).

## Uninstalling

```bash
clauductor panel uninstall          # the login agent, its token, its copy of the binary, the app
clauductor panel --uninstall-hooks  # the panel's hooks in ~/.claude/settings.json
rm ~/.local/bin/clauductor
rm -rf ~/.local/share/clauductor    # release installs
```

Then remove the `CLAUDUCTOR_FRAMEWORK` line from your shell profile. `~/.clauductor/` keeps the
panel's registered projects and each project's lane list. Delete it once no lane you care about
is still running. Repositories keep what `clauductor install` or `/clauductor:init` put in them.
