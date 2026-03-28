# Clauductor

A multi-worker orchestration framework for [Claude Code](https://claude.ai/code). Coordinate multiple AI agents and human developers working simultaneously on the same codebase — with file locking, real-time HUD, session management, and orchestration logging.

## The Problem

Claude Code is powerful, but scaling beyond one session is chaos. Run two agents on the same repo and they step on each other's files. Run three and you lose track of who's doing what. There's no coordination, no locking, no shared awareness.

**claude-squad** solved the process management part (spawning agents in tmux), but has no inter-agent coordination, no file locking, and no intelligence in the orchestration layer.

Clauductor fixes this. It's a coordination protocol — not just a process manager — built on Claude Code's skills system, with a real-time TUI dashboard and SQLite-backed state.

## What You Need

- **[Claude Code](https://claude.ai/code)** — Anthropic's CLI tool
- **tmux** — terminal multiplexer (installed automatically)
- **Go 1.22+** — for building the CLI (installed automatically)
- **Git** + **GitHub CLI (`gh`)** — for git workflow

## Quick Start

```bash
# Install Clauductor
git clone https://github.com/your-org/claude-dev-framework.git ~/clauductor
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
│   ├── .claude/skills/         ← Orchestration skills
│   ├── .claude/statusline.sh   ← Dynamic status line
│   └── docs/                   ← Project doc templates
├── install.sh                  ← Build + install script
└── docs/prds/                  ← Framework PRDs
```

Projects get only the `template/` contents — no Go source, no framework code.

## Core Concepts

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
│  ● rich      M1.2  BUILD   src/auth/*       12m       │
│  ● agent-1   M1.3  BUILD   src/api/routes   4m        │
│  ● agent-2   M2.1  SPIKE   (no locks)       8m        │
│                                                        │
│  MILESTONES                                            │
│  M1.2  AUTH PROVIDER     ██████████░░  65%  rich       │
│  M1.3  API ROUTES        ████░░░░░░░  30%  agent-1    │
└────────────────────────────────────────────────────────┘
```

### Skills

| Skill | Purpose |
|-------|---------|
| `/session-start` | Register worker, load context |
| `/new-milestone` | Create branch + per-milestone docs |
| `/claim` | Declare file manifest, lock files |
| `/release` | Release locks, deregister |
| `/supervisor` | Orchestration loop — dispatch, monitor |
| `/spawn` | Launch new Claude Code sessions |
| `/commit` | Commit with M#.#.# conventions |
| `/status` | Quick orchestration status |

## Origin

Built over 87+ sessions and 274+ hours developing [Forager](https://github.com/rfhayn/forager), an iOS app. The framework evolved from real pain — naming drift, lost context, coordination chaos. Clauductor extends it from single-session methodology to multi-worker orchestration.

## License

MIT
