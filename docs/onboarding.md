# Clauductor Onboarding Guide

## What is Clauductor?

Clauductor is a multi-worker orchestration framework for Claude Code. It coordinates multiple AI agents and human developers working simultaneously on the same codebase — with file locking, real-time HUD, session management, and orchestration logging.

## Quick Setup (5 minutes)

### 1. Install

```bash
# Clone the framework
git clone https://github.com/rfhayn/clauductor.git ~/clauductor
cd ~/clauductor

# Install (builds binary, installs Go + tmux if needed)
./install.sh

# Restart your shell to pick up PATH changes
source ~/.zshrc  # or ~/.bashrc for bash
```

### 2. Create Your First Project

```bash
clauductor init ~/Development/my-app
cd ~/Development/my-app
claude
```

Once in Claude Code, run:
```
/start-project
```

This walks you through the full setup: prefix registry, build command, architecture description, and first milestone.

### 3. Daily Workflow

```
/start-work AUTH-1.3    # Registers, claims files, loads context
# ... work ...
/done                   # Review → journal → commit → PR → release
```

`/start-work` chains `/session-start` + `/claim` into a single command.
`/done` chains `/review` → `/dev-journal` → `/commit` → `/pr` → `/milestone-complete` → `/release`.

## Multi-Worker Orchestration

### Starting a Team Workspace

From the terminal:

```bash
clauductor start           # HUD + supervisor + 3 workers
clauductor start -n 5      # Override to 5 workers
```

This creates:
- **Window 0**: HUD dashboard (`clauductor watch`)
- **Window 1**: Supervisor (auto-dispatches work via `/supervisor`)
- **Windows 2-N**: Worker terminals (auto-launch claude)

Configure in `orchestration/config.json`:
```json
{
  "default_workers": 3,
  "auto_claude": true
}
```

### Spawning Agents

From the supervisor session:

```
/spawn build AUTH-1 Login flow
/spawn research CACHE-1 Caching strategies
/spawn spike DB-1 Database schema exploration
```

Each agent runs in its own Claude Code session with isolated file locks.

### Monitoring Progress

From any session:
```
/status
```

From the terminal:
```bash
clauductor watch
clauductor status
clauductor query workers
clauductor query events --limit 20
```

### Handling Conflicts

**File locking** prevents two workers from editing the same file. If a worker tries to claim a file that is already locked:

1. The claim is rejected with a conflict message showing who holds the lock
2. Use `/blocked AUTH-1` to report that you are blocked
3. A 15-minute escalation cycle notifies the lock holder
4. The supervisor can use `/spawn` to reassign work or force-release locks

### Auto-Dispatching Work

Supervisors can automatically assign unclaimed milestones to new agents:

```
/assign 3
```

This reads the priority queue, finds unclaimed READY/PLANNED milestones, and spawns up to 3 workers.

## Session Types

| Type | Purpose | File Access |
|------|---------|-------------|
| **research** | Learning, investigation | Read anything, write docs only |
| **spike** | Architecture exploration | Read anything, write docs only |
| **build** | Feature implementation | Full access, locks claimed files |
| **test** | Writing/running tests | Full access, locks claimed files |

## Hooks (Automatic Safety Checks)

Hooks run automatically via `.claude/settings.json` — no manual invocation needed.

| Hook | When | What |
|------|------|------|
| `session-register.sh` | Session start | Registers worker in orchestration DB |
| `heartbeat.sh` | After edits | Keeps worker alive (60s throttle) |
| `lock-guard.sh` | Before edits | Warns if file is locked by another worker |
| `status-sync.sh` | After writes | Syncs milestone status when current-story.md changes |

**Design principles**:
- All hooks no-op gracefully when `orchestration/` doesn't exist
- Hooks warn but don't block by default (exit 0)
- <200ms execution — file stat before SQLite queries
- See `.claude/hooks/README.md` for the full protocol

## Skill Reference

Run `/skills` in any Claude Code session to see the full list of available skills.

Key skills:

| Skill | Purpose |
|-------|---------|
| `/start-project` | Guided first-time setup wizard |
| `/start-work [PREFIX-#.#]` | Pick up work: register + claim + load context |
| `/done` | Wrap up: review → journal → commit → PR → release |
| `/pane` | Open new tmux pane (optionally with claude) |
| `/session-start` | Register worker, load context |
| `/new-milestone` | Create branch + per-milestone docs |
| `/commit` | Commit with PREFIX-#.# conventions |
| `/spawn` | Launch new Claude Code sessions |
| `/supervisor` | Orchestration loop |
| `/status` | Quick orchestration status |

## Exporting Data

Use the `export` command for reporting and auditing:

```bash
# Export events as JSON
clauductor export events --limit 100

# Export events as markdown table
clauductor export events --format markdown --since 2025-01-01

# Export workers
clauductor export workers --format markdown

# High-level summary
clauductor export summary
```

## Troubleshooting

**`clauductor: command not found`** — Run `source ~/.zshrc  # or ~/.bashrc for bash` or add `~/.local/bin` to your PATH.

**`CLAUDUCTOR_FRAMEWORK not set`** — Run `./install.sh` again, or set manually: `export CLAUDUCTOR_FRAMEWORK=~/clauductor`

**Status line shows wrong milestone** — Run `/session-start` to update `orchestration/.session-status`.

**`clauductor install` shows conflicts** — Use `--dry-run` first. Framework files are safe to overwrite. Doc files with existing content are preserved.

**Worker not showing in HUD** — Ensure you ran `/session-start` or `/claim`. Check `clauductor query workers` to verify registration.

**Lock stuck after crash** — Use `clauductor unlock --worker <id>` to release locks from a crashed session. The supervisor can also force-release locks.
