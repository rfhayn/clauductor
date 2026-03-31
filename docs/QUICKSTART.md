# Clauductor Quickstart Guide

## Prerequisites

- **macOS or Linux** (macOS recommended for v1)
- **Git** installed
- **Claude Code** installed ([claude.ai/code](https://claude.ai/code))
- **Go** and **tmux** (installed automatically by the installer)

## Installation

```bash
# Clone the framework
git clone https://github.com/rfhayn/clauductor.git ~/clauductor
cd ~/clauductor

# Install (builds binary, installs Go + tmux if needed)
./install.sh

# Restart your shell to pick up PATH changes
source ~/.zshrc  # or ~/.bashrc for bash
```

## Create a New Project

```bash
clauductor init ~/Development/my-app
cd ~/Development/my-app
claude
```

Once in Claude Code:
```
/start-project
```

This walks you through setup: prefix registry, build command, architecture, first milestone.

## Install Into an Existing Project

```bash
cd ~/Development/existing-project

# Preview what will change
clauductor install --dry-run

# Install for real
clauductor install
```

The installer handles files in three tiers:
- **Framework files** (skills, settings, statusline) — always installed
- **Project files** (agents, docs, README) — created only if missing, never overwrites
- **Config files** (CLAUDE.md, .gitignore) — merged with existing

## Your First Milestone

```
/new-milestone AUTH-1 Build user authentication
```

This:
1. Creates branch `feature/AUTH-1-build-user-auth`
2. Updates `docs/current-story.md` with ACTIVE status
3. Creates `docs/next-prompt-AUTH-1.md` with implementation guidance
4. Adds a pointer in `docs/next-prompt.md`

## Daily Workflow

```
# Pick up a milestone (registers, claims files, loads context)
/start-work AUTH-1

# Work... build... test...
/build
/commit

# Milestone done? This chains: review → journal → commit → PR → release
/done
```

## Multi-Worker Orchestration

Start a full team workspace:

```bash
clauductor start           # HUD + supervisor + 3 workers
clauductor start -n 5      # Override to 5 workers
```

This creates:
- **Window 0**: HUD dashboard (`clauductor watch`)
- **Window 1**: Supervisor (auto-dispatches work)
- **Windows 2-N**: Worker terminals (auto-launch claude)

From any session:
```
# Spawn additional workers
/spawn build API-1 API routes
/spawn research CACHE-1 Caching strategies

# Check status
/status
```

## Session Types

| Type | Purpose | File Access |
|------|---------|-------------|
| **research** | Learning, investigation | Read anything, write docs only |
| **spike** | Architecture exploration | Read anything, write docs only |
| **build** | Feature implementation | Full access, locks claimed files |
| **test** | Writing/running tests | Full access, locks claimed files |

## Key Commands

| Command | What it does |
|---------|-------------|
| `clauductor init <path>` | Create new project |
| `clauductor install` | Add to existing project |
| `clauductor install --dry-run` | Preview install changes |
| `clauductor update` | Upgrade skills to latest |
| `clauductor start [-n N]` | Full team workspace (HUD + supervisor + workers) |
| `clauductor watch` | HUD dashboard only |
| `clauductor status` | Quick terminal status |
| `clauductor context [--json]` | Orchestration snapshot |
| `clauductor check-lock --file <path>` | Check file lock status |
| `clauductor query <type>` | Query state (JSON): workers, locks, events, milestones |
| `clauductor export <type>` | Export data (JSON/markdown) |

## File Structure

After `clauductor init`, your project looks like:

```
my-project/
├── CLAUDE.md              ← Claude Code instructions (edit for your project)
├── .claude/
│   ├── skills/            ← Orchestration skills
│   ├── agents/            ← Composite agents
│   ├── hooks/             ← Automated safety checks
│   ├── settings.json      ← Permissions + hooks + status line
│   └── statusline.sh      ← Dynamic status display
├── docs/
│   ├── current-story.md   ← Source of truth for milestones
│   ├── next-prompt.md     ← Hub/index for implementation guidance
│   └── prds/              ← Product requirement documents
└── orchestration/         ← Runtime state (gitignored)
    ├── config.json        ← Team settings (workers, auto-claude)
    └── framework.db       ← SQLite: workers, locks, events
```

## Updating

When the framework gets new skills or improvements:

```bash
cd ~/clauductor && git pull
./install.sh

# Then in your project:
cd ~/Development/my-project
clauductor update
```

## Troubleshooting

**`clauductor: command not found`** — Run `source ~/.zshrc  # or ~/.bashrc for bash` or add `~/.local/bin` to your PATH.

**`CLAUDUCTOR_FRAMEWORK not set`** — Run `./install.sh` again, or set manually: `export CLAUDUCTOR_FRAMEWORK=~/clauductor`

**Status line shows wrong milestone** — Run `/session-start` to update `orchestration/.session-status`.

**`clauductor install` shows conflicts** — Use `--dry-run` first. Framework files are safe to overwrite. Doc files with existing content are preserved.
