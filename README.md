# Clauductor

A local panel and an operating model for running many interactive
[Claude Code](https://claude.ai/code) sessions on one machine.

Clauductor is a local, open-source cockpit for running many Claude Code sessions on one Mac, plus
an operating model you install into each repository. Each **lane** is a real, interactive Claude
Code session in its own git worktree and tmux session, shown in a browser panel with live status
from Claude Code's own hooks. The panel shows which lanes are busy, waiting or need you, along
with context, subagents, and your 5-hour and 7-day quota. Watching costs no model tokens, and
nothing leaves your machine. A shared gate queue runs test runs one at a time, so parallel lanes
don't collide. Lanes survive a reboot, and closing one cleans up its worktree and branch. The
**operating model** turns that speed into a process you can trust: change proposals you approve,
build and review loops that run until they converge, hooks that refuse a merge without CI
evidence, and a journal and ADR trail. It all runs on the Claude subscription you already pay for.

## Why use it next to Claude Desktop?

Claude Code Desktop, agent view and Remote Control cover much of the same ground, and they come
with your subscription. If they are enough for you, use them. Clauductor is for people who want
the following:

- **Terminal-first and local.** Lanes are plain `claude` sessions in tmux. They keep running when
  you close the page, and you can attach to them from any terminal.
- **Zero-token status.** The panel reads hooks, the status line, git and `gh`. It never
  summarises a session with a model.
- **One panel for many repositories**, with a gate queue shared across every lane of a repository.
- **Rules that are enforced, not just suggested.** The operating model's hooks block a merge
  that has no CI evidence, and a blind `sed` over the tree. It is plain files in your repository:
  you can read and change every rule.
- **Open source (MIT)** and Claude-only by design.

It is also a young project with one maintainer. Read [Status](#status) before you depend on it.

## Contents

- [What's in it](#whats-in-it)
- [Requirements](#requirements)
- [Install](#install)
- [Quick start: the panel](#quick-start-the-panel)
- [Quick start: the operating model](#quick-start-the-operating-model)
- [A repository works without clauductor](#a-repository-works-without-clauductor)
- [Screenshots](#screenshots)
- [Documentation](#documentation)
- [Status](#status)
- [Contributing](#contributing)
- [License](#license)

## What's in it

| | What it is | Needs |
|---|---|---|
| **The panel** (`clauductor panel`) | A loopback web page over your Claude Code sessions. Start, stop, restart and resume lanes with an embedded terminal, lane templates, the gate queue, merge readiness, metrics, alerts, a quota guard, and restore after a reboot. It is standalone: it works in any git repository. | the `clauductor` binary |
| **The operating model** (`template/`) | Skills, agents, hooks, checks, a CI gate and a change process that a repository adopts. It is installed as files (`clauductor install`) or as a Claude Code plugin. | Claude Code (the plugin), or the binary (`install`) |

Each half works without the other.

## Requirements

| | Needed for | Notes |
|---|---|---|
| **macOS** 12 or later (Apple silicon or Intel) | everything | The primary platform. The login agent, the Dock app, notifications and "Open in Terminal" are macOS only. |
| **Linux** (amd64, arm64; glibc 2.35 or later for release binaries) | the panel and the model | Run `clauductor panel` yourself (there is no login agent), and open the printed URL. There are no desktop notifications. The panel is tested in CI on Ubuntu. |
| **Windows** | through **WSL** only | The binary does not build for native Windows. In WSL it behaves as on Linux. The operating model's plain-shell hooks are not tested on native Windows. |
| **Claude Code** | everything | A recent version: the panel reads `claude agents --json`, and was developed against 2.1.284. |
| **A Claude subscription login** | panel lanes | Lanes run on your subscription, not an API key: the panel refuses to start a lane while `ANTHROPIC_API_KEY` or `ANTHROPIC_AUTH_TOKEN` is set. |
| **tmux** | panel lanes | Each lane is a tmux session. |
| **git** 2.31+ and **gh** (signed in) | worktrees, PRs, merge readiness, metrics | |
| **jq** | the operating model | The hooks and checks read JSON with it. |
| **Go** 1.24+ | building from source only | `install.sh` installs it with Homebrew on macOS if missing. |

## Install

Pick one. [docs/install.md](docs/install.md) has the details: checksums, upgrading and
uninstalling.

**1. A release binary.** The release workflow is ready but nothing has been published yet, so
until v0.1.0 is out, use option 2.

```bash
curl -fsSL https://raw.githubusercontent.com/rfhayn/clauductor/main/install.sh | bash
```

This downloads the binary for your platform, checks it against the release's `checksums.txt`, and
installs it to `~/.local/bin`. It unpacks `template/` beside it, because `init`, `install` and
`update` copy from it. A Homebrew tap (`brew install rfhayn/clauductor/clauductor`) is prepared
but not switched on yet.

`go install <module>@latest` does not work: the module path (`github.com/clauductor/clauductor`)
is not the repository's, and the binary needs `template/`. From a clone,
`cd framework && go install ./cmd/clauductor` works if you point `CLAUDUCTOR_FRAMEWORK` at the
clone.

**2. From source.**

```bash
git clone https://github.com/rfhayn/clauductor.git ~/clauductor
cd ~/clauductor && ./install.sh     # builds with Go; installs Go and tmux with Homebrew if missing
```

**3. The operating model as a Claude Code plugin.** This needs no binary. See
[docs/plugin.md](docs/plugin.md).

```
/plugin marketplace add rfhayn/clauductor
/plugin install clauductor@clauductor
```

## Quick start: the panel

In any git repository:

```bash
clauductor panel init       # writes .clauductor/panel.json from what the repo already says; review it
clauductor panel trust      # the panel runs none of the config's commands until you trust it
clauductor panel            # serves the page and opens your browser
```

Then click **New lane**. To keep the panel running with no terminal open, on macOS:

```bash
clauductor panel install --project . --app   # a login agent, plus a Dock/Spotlight app
clauductor panel open                        # open the installed panel
```

`panel install` asks once where Claude Code's **Remote Control** should be on, so you can drive a
lane from claude.ai/code or the Claude app: every Claude session on this Mac, only the panel's
lanes, or not now. `--remote-control=all|lanes|off` answers without asking. Anyone signed in to
your account can then drive those sessions, so keep your devices locked.

For another repository, run `clauductor panel init`, then `trust`, then `clauductor panel add` in
it, and restart the panel. One panel serves them all. [docs/guide.md](docs/guide.md) is the
how-to for the page. [docs/panel.md](docs/panel.md) covers every setting and the security model.

## Quick start: the operating model

With the binary:

```bash
clauductor init ~/Development/my-app        # a new project
cd existing-repo && clauductor install      # an existing repository (--dry-run to preview)
```

Or with the plugin, inside Claude Code in the repository:

```
/clauductor:init            # scaffolds the project's own files; never overwrites one
```

Then configure it once, from Claude Code in the repository: `/start-project` (or
`/clauductor:start-project` with the plugin). It sets up the project's name, who decides, the
gate's lint and test steps, and the first roadmap rows. Use one way per repository, not both.
A repository that already runs its own skills and hooks is refused rather than overwritten. See
[docs/QUICKSTART.md](docs/QUICKSTART.md) for the first change, end to end.

## A repository works without clauductor

This is a design rule. Everything a contributor needs lives in the repository and runs in
Claude Code alone: the skills, hooks, checks, the gate and its queue lock, the merge guard, and
the records. The panel, the `clauductor` binary and the plugin are conveniences on top, never
dependencies. Anything that talks to the panel treats a missing panel as normal. A teammate who
never installs clauductor can still clone the repository and work in it. A template check that
runs the gate, the checks and the status line with no `clauductor` on `PATH` and no panel
running is planned (`no-clauductor.sh`, in
[the change-process PRD](docs/prds/active/PRD-change-process.md), D10).

## Screenshots

Screenshots are coming. They will live in [docs/README-screens/](docs/README-screens/):

- `panel-lanes.png`: the lane rail, a lane's terminal and its side panel
- `panel-needs-you.png`: Needs you, the quota and the gate queue
- `panel-metrics.png`: the Metrics view
- `panel-themes.png`: the six themes

## Documentation

- [docs/install.md](docs/install.md): install, upgrade, uninstall
- [docs/guide.md](docs/guide.md): using the panel's page
- [docs/panel.md](docs/panel.md): the panel reference: configuration, lanes, the gate lock protocol, the security model, troubleshooting
- [docs/plugin.md](docs/plugin.md): the operating model as a Claude Code plugin
- [docs/QUICKSTART.md](docs/QUICKSTART.md): adopting the operating model, from nothing to a first change
- [docs/onboarding.md](docs/onboarding.md): the older tmux HUD and orchestration commands (`clauductor start`, `watch`, `lock`)
- [CHANGELOG.md](CHANGELOG.md) and [docs/release.md](docs/release.md): what changed, and how a release is cut

## Status

This is pre-1.0 software (`0.x`), written by one person, and used daily on macOS to build
clauductor itself. Before 1.0, expect config versions to move forward, with
the old ones still read. The panel is the most complete part. The operating model is newer and
still settling. Planned work is tracked in [docs/roadmap.md](docs/roadmap.md) and in
[open issues](https://github.com/rfhayn/clauductor/issues).

## Contributing

Issues and pull requests are welcome. Please open an issue before starting large changes.

```bash
cd framework && go test -short ./...    # seconds; CI runs the full -race suite on macOS and Ubuntu
gofmt -l . && go vet ./...
```

- Branches are `feature/PREFIX-#.#-short-name`, and commits start with `PREFIX-#.#:` in the
  imperative mood (see [CLAUDE.md](CLAUDE.md)).
- `template/` is what every adopting repository receives. After changing it, run
  `scripts/build-plugin.sh`: `plugin/` is generated from it, and a test fails until it is rebuilt.
- User-visible changes go into [CHANGELOG.md](CHANGELOG.md). `scripts/changelog.sh` drafts the
  entries from merged PR titles.

## License

[MIT](LICENSE) © 2026 Rich Hayn
