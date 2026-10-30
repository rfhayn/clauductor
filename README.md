# Stridal

An operating model for [Claude Code](https://claude.ai/code), and a local panel that runs it. The model is a set of skills, hooks, checks, agents and records a repository carries; the panel is one local web page for the lanes (Claude Code sessions in git worktrees) of every project on your machine.

## The Problem

Claude Code is powerful, but scaling beyond one session is chaos. Run two agents on the same repo and they step on each other's work. Run three and you lose track of who is doing what, what was approved, and what is safe to merge.

Stridal answers that with a process the repository enforces on itself (a roadmap queue, approved proposals, review per task group, a merge guard, a gate) and a panel that shows every lane and what it needs from you.

## What You Need

- **[Claude Code](https://claude.ai/code)** — Anthropic's CLI tool
- **tmux** — terminal multiplexer, for the panel's lanes (installed automatically)
- **Go 1.24+** — for building the CLI (installed automatically)
- **Git**, **GitHub CLI (`gh`)** and **jq** — for the git workflow, the hooks and the checks

## Quick Start

```bash
# Install Stridal
git clone https://github.com/rfhayn/stridal.git ~/stridal
cd ~/stridal && ./install.sh

# Create a new project
stridal init ~/Development/my-app
cd ~/Development/my-app
claude
/session-start

# Or install into an existing project
cd ~/Development/existing-project
stridal install
```

Or, with Claude Code alone, as a plugin (one way per repository; see [docs/plugin.md](docs/plugin.md)):

```
/plugin marketplace add rfhayn/stridal
/plugin install stridal@stridal
/stridal:init
```

## Architecture

```
stridal/                     ← This repo (framework source)
├── framework/                  ← Go source (the CLI and the panel)
├── template/                   ← What projects receive
│   ├── AGENTS.md, CLAUDE.md    ← Project-level instructions
│   ├── .claude/                ← Skills, agents, hooks, checks, workflows
│   ├── scripts/ci/             ← The gate
│   └── docs/                   ← Project record templates
├── plugin/                     ← The operating model as a plugin (generated from template/)
├── install.sh                  ← Build + install script
└── docs/                       ← This repo's records and PRDs
```

Projects get only the `template/` contents — no Go source, no framework code. The model is explained, lane by lane, in the project's own `docs/playbook.md`.

### CLI Commands

| Command | What it does |
|---------|-------------|
| `stridal init <path>` | Create new project |
| `stridal install` | Add to existing project |
| `stridal install --dry-run` | Preview install changes |
| `stridal update` | Upgrade the operating model to the latest template |
| `stridal diff` | Compare this repository with the template, file by file and key by key |
| `stridal doctor` | Check this machine's tools, the template, `gh`, the project and (on macOS) the panel, changing nothing; each problem comes with the command that fixes it |
| `stridal plugin build` / `check` | Package the operating model as a Claude Code plugin, or check the committed one is current |
| `stridal lock-run` | Run a command while holding the shared gate lease ([docs](docs/panel.md)) |
| `stridal panel` | Local read-only web dashboard of a project's Claude sessions; standalone, needs no install ([docs](docs/panel.md)) |
| `stridal panel init` | Write a starter `.stridal/panel.json` for the project, from what the repository already says |
| `stridal panel --uninstall-hooks` | Remove the panel's hooks from `~/.claude/settings.json` |
| `stridal lane close <id> [--yes]` | Close a finished lane of the running panel from the project's checkout: the plan first, the close with `--yes`; a lane still working is refused, and so is one on unmerged work unless its claude has exited (that lane closes and keeps its branch) ([docs](docs/panel.md#close-a-lane-from-the-command-line)) |

## Documentation

- **[Quickstart Guide](docs/QUICKSTART.md)** — Installation and first project setup
- **[Using the web panel](docs/guide.md)** — a short how-to for the panel's page: lanes, the terminal, stop vs close, restore, a second repository
- **[Web panel](docs/panel.md)** — `stridal panel`: quick start, configuration reference and JSON Schema, the gate lock protocol, security model
- **[Claude Code plugin](docs/plugin.md)** — the operating model as a plugin: install, what it carries, what stays in the repository, running it beside `stridal install`
- **[PRD](docs/prds/active/PRD-orchestration-framework.md)** — Full product requirements and architecture

## Origin

Built over 87+ sessions and 274+ hours developing [forager](https://github.com/rfhayn/forager), an iOS app. The framework evolved from real pain — naming drift, lost context, coordination chaos. Stridal extends it from single-session methodology to many lanes at once.

## License

MIT
