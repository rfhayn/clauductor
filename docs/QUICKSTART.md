# Clauductor Quickstart Guide

## Prerequisites

- **macOS or Linux** (macOS recommended for v1)
- **Git** installed
- **Claude Code** installed ([claude.ai/code](https://claude.ai/code))
- **Go** and **tmux** (installed automatically by the installer)

## Installation

```bash
# Clone the framework
git clone https://github.com/your-org/claude-dev-framework.git ~/clauductor
cd ~/clauductor

# Install (builds binary, installs Go + tmux if needed)
./install.sh

# Restart your shell to pick up PATH changes
source ~/.zshrc
```

## Create a New Project

```bash
clauductor init ~/Development/my-app
cd ~/Development/my-app
claude
```

Once in Claude Code:
```
/session-start
```

This loads all context docs, checks git state, and reports status.

## Install Into an Existing Project

```bash
cd ~/Development/existing-project

# Preview what will change
clauductor install --dry-run

# Install for real
clauductor install
```

The installer handles files in three tiers:
- **Framework files** (skills, agents, settings) — always installed
- **Doc templates** (current-story, journal, etc.) — created only if missing
- **Config files** (CLAUDE.md, .gitignore) — merged with existing

## Your First Milestone

```
/new-milestone M1.1 Build user authentication
```

This:
1. Creates branch `feature/M1.1-build-user-auth`
2. Updates `docs/current-story.md` with ACTIVE status
3. Creates `docs/next-prompt-M1.1.md` with implementation guidance
4. Adds a pointer in `docs/next-prompt.md`

## Daily Workflow

```
# Start of session (mandatory)
/session-start

# Claim files for a build session
/claim M1.1

# Work... build... test...
/build
/commit

# End of session
/dev-journal

# Milestone done?
/milestone-complete M1.1
```

## Multi-Worker Orchestration

Start the orchestrator to run multiple sessions:

```bash
clauductor start
```

This creates a tmux session with the HUD dashboard. From there:

```
# Spawn a build agent
/spawn build M1.2 API routes

# Spawn a research agent
/spawn spike M2.1 caching strategies

# Check status
/status

# Toggle between sessions: press 1, 2, 3...
# Back to HUD: press Esc
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
| `clauductor start` | Launch tmux + HUD |
| `clauductor watch` | HUD dashboard only |
| `clauductor status` | Quick terminal status |
| `clauductor register` | Register a worker |
| `clauductor lock` | Lock files |
| `clauductor unlock` | Release locks |
| `clauductor query <type>` | Query state (JSON) |

## File Structure

After `clauductor init`, your project looks like:

```
my-project/
├── CLAUDE.md              ← Claude Code instructions (edit for your project)
├── .claude/
│   ├── skills/            ← Orchestration skills
│   ├── agents/            ← Composite agents
│   ├── settings.json      ← Permissions + status line
│   └── statusline.sh      ← Dynamic status display
├── docs/
│   ├── current-story.md   ← Source of truth for milestones
│   ├── next-prompt.md     ← Hub/index for implementation guidance
│   ├── roadmap.md         ← Milestone sequence
│   └── prds/              ← Product requirement documents
└── orchestration/         ← Runtime state (gitignored)
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

**`clauductor: command not found`** — Run `source ~/.zshrc` or add `~/.local/bin` to your PATH.

**`CLAUDUCTOR_FRAMEWORK not set`** — Run `./install.sh` again, or set manually: `export CLAUDUCTOR_FRAMEWORK=~/clauductor`

**Status line shows wrong milestone** — Run `/session-start` to update `orchestration/.session-status`.

**`clauductor install` shows conflicts** — Use `--dry-run` first. Framework files are safe to overwrite. Doc files with existing content are preserved.
