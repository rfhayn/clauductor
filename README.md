# Clauductor

A multi-worker orchestration framework for [Claude Code](https://claude.ai/code). Coordinate multiple AI agents and human developers working simultaneously on the same codebase — with file locking, real-time HUD, session management, and orchestration logging.

## The Problem

Claude Code is powerful, but scaling beyond one session is chaos. Run two agents on the same repo and they step on each other's files. Run three and you lose track of who's doing what. There's no coordination, no locking, no shared awareness.

**claude-squad** solved the process management part (spawning agents in tmux), but has no inter-agent coordination, no file locking, and no intelligence in the orchestration layer.

Clauductor fixes this. It's a coordination protocol — not just a process manager — built on Claude Code's skills system, with a real-time TUI dashboard and SQLite-backed state.

## What You Need

- **[Claude Code](https://claude.ai/code)** — Anthropic's CLI tool
- **tmux** — terminal multiplexer (installed automatically)
- **Go 1.24+** — for building the CLI (installed automatically)
- **Git** + **GitHub CLI (`gh`)** — for git workflow

## Quick Start

```bash
# Install Clauductor
git clone https://github.com/rfhayn/clauductor.git ~/clauductor
cd ~/clauductor && ./install.sh

# Create a new project
clauductor init ~/Development/my-app
cd ~/Development/my-app
claude
/session-start

# Or install into an existing project
cd ~/Development/existing-project
clauductor install
```

## Architecture

```
clauductor/                     ← This repo (framework source)
├── framework/                  ← Go source (CLI + HUD)
├── template/                   ← What projects receive
│   ├── .claude/skills/         ← 21 orchestration + workflow skills
│   ├── .claude/statusline.sh   ← Dynamic status line
│   └── docs/                   ← Project doc templates
├── install.sh                  ← Build + install script
└── docs/prds/                  ← Framework PRDs
```

Projects get only the `template/` contents — no Go source, no framework code.

## Core Concepts

### Naming Convention (PREFIX-#.#)
```
AUTH        = Epic (domain area)
AUTH-1      = Feature within that epic
AUTH-1.3    = Task within that feature
```
Define your epics in the Prefix Registry in CLAUDE.md. Legacy M#.#.# format also supported.

### Session Types
- **Research/Spike** — read anything, modify docs only, no file locks
- **Build/Test** — PRD-driven, declares file manifest, locks files

### File Locking
Build sessions claim files before modifying them. Other sessions are blocked from those files until the lock is released. 15-minute escalation cycle with user notification.

### The HUD
Real-time TUI dashboard showing active workers, file locks, activity feed, and milestone progress. Toggle between sessions via tmux panes.

```
┌─ Clauductor ──────────────────────────────────────────┐
│  WORKERS                                               │
│  ● rich      AUTH-1  BUILD   src/auth/*       12m     │
│  ● agent-1   API-1   BUILD   src/api/routes    4m     │
│  ● agent-2   CACHE-1 SPIKE   (no locks)        8m     │
│                                                        │
│  MILESTONES                                            │
│  AUTH-1  LOGIN FLOW      ██████████░░  65%  rich       │
│  API-1   API ROUTES      ████░░░░░░░  30%  agent-1    │
└────────────────────────────────────────────────────────┘
```

### Skills (21 included)

| Skill | Purpose |
|-------|---------|
| `/session-start` | Register worker, load context |
| `/new-milestone` | Create branch + per-milestone docs |
| `/claim` | Declare file manifest, lock files |
| `/release` | Release locks, deregister |
| `/review` | Pre-PR code review (conventions, quality, docs) |
| `/supervisor` | Orchestration loop — dispatch, monitor |
| `/spawn` | Launch new Claude Code sessions |
| `/assign` | Auto-dispatch work to agents |
| `/handoff` | Structured handoff between workers |
| `/blocked` | Report block, start wait/escalation |
| `/status` | Quick orchestration status |
| `/commit` | Commit with PREFIX-#.# conventions |
| `/pr` | Create PR with project format |
| `/build` | Build project |
| `/milestone-complete` | Update docs + clean up |
| `/dev-journal` | Session narrative entry |
| `/log-insight` | Log technical insight |
| `/prd-audit` | Verify PRD against code |
| `/architecture-audit` | Check for violations |
| `/release-prep` | Deployment pipeline |
| `/skills` | List all available skills |

### CLI Commands

| Command | What it does |
|---------|-------------|
| `clauductor init <path>` | Create new project |
| `clauductor install` | Add to existing project |
| `clauductor install --dry-run` | Preview install changes |
| `clauductor update` | Upgrade skills to latest |
| `clauductor start` | Launch tmux + HUD |
| `clauductor watch` | HUD dashboard only |
| `clauductor status` | Quick terminal status |
| `clauductor register` | Register a worker |
| `clauductor deregister` | Remove a worker |
| `clauductor lock` | Lock files |
| `clauductor unlock` | Release locks |
| `clauductor heartbeat` | Update worker heartbeat |
| `clauductor milestone` | Create/update milestones |
| `clauductor event` | Log orchestration event |
| `clauductor query <type>` | Query state (JSON) |
| `clauductor export <type>` | Export data (JSON/markdown) |

## Documentation

- **[Quickstart Guide](docs/QUICKSTART.md)** — Installation and first project setup
- **[Onboarding Guide](docs/onboarding.md)** — Complete tutorial from install to multi-worker orchestration
- **[PRD](docs/prds/active/PRD-orchestration-framework.md)** — Full product requirements and architecture

## Origin

Built over 87+ sessions and 274+ hours developing [forager](https://github.com/rfhayn/forager), an iOS app. The framework evolved from real pain — naming drift, lost context, coordination chaos. Clauductor extends it from single-session methodology to multi-worker orchestration.

## License

MIT
