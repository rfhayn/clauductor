# Clauductor Onboarding Guide

## What is Clauductor?

Clauductor is a multi-worker orchestration framework for Claude Code. It coordinates multiple AI agents and human developers working simultaneously on the same codebase — with file locking, real-time HUD, session management, and orchestration logging.

## Quick Setup (5 minutes)

### 1. Install

```bash
# Clone the framework
git clone https://github.com/your-org/claude-dev-framework.git ~/clauductor
cd ~/clauductor

# Install (builds binary, installs Go + tmux if needed)
./install.sh

# Restart your shell to pick up PATH changes
source ~/.zshrc
```

### 2. Create Your First Project

```bash
clauductor init ~/Development/my-app
cd ~/Development/my-app
claude
```

Once in Claude Code, run:
```
/session-start
```

This loads all context docs, checks git state, registers you as a worker, and reports status.

### 3. Define Your First Epic

Open `CLAUDE.md` and add to the Prefix Registry:

| Prefix | Description | Status |
|--------|-------------|--------|
| AUTH | Authentication system | PLANNED |

### 4. Start Your First Milestone

```
/new-milestone AUTH-1 User login flow
```

This creates a feature branch, updates `docs/current-story.md` with ACTIVE status, and generates implementation guidance in `docs/next-prompt-AUTH-1.md`.

### 5. Claim and Build

```
/claim AUTH-1
```

This locks the files you will be working on. Then build:

```
# Make your changes...
/build
/commit
```

Repeat the build-commit cycle as you make progress.

### 6. Review and PR

When the milestone is ready for review:

```
/pr
```

This creates a pull request with the project format, linking the milestone and summarizing changes.

### 7. Complete the Milestone

```
/milestone-complete AUTH-1
```

This updates `docs/current-story.md`, writes a journal entry, and marks the milestone as complete.

## Multi-Worker Orchestration

### Starting the Supervisor

From a Claude Code session:

```
/supervisor
```

This launches the orchestration loop with the real-time HUD dashboard.

Alternatively, from the terminal:

```bash
clauductor start
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

## Skill Reference

Run `/skills` in any Claude Code session to see the full list of available skills.

Key skills:

| Skill | Purpose |
|-------|---------|
| `/session-start` | Register worker, load context |
| `/new-milestone` | Create branch + per-milestone docs |
| `/claim` | Declare file manifest, lock files |
| `/release` | Release locks, deregister |
| `/supervisor` | Orchestration loop |
| `/spawn` | Launch new Claude Code sessions |
| `/assign` | Auto-dispatch work (supervisor only) |
| `/commit` | Commit with PREFIX-#.# conventions |
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

**`clauductor: command not found`** — Run `source ~/.zshrc` or add `~/.local/bin` to your PATH.

**`CLAUDUCTOR_FRAMEWORK not set`** — Run `./install.sh` again, or set manually: `export CLAUDUCTOR_FRAMEWORK=~/clauductor`

**Status line shows wrong milestone** — Run `/session-start` to update `orchestration/.session-status`.

**`clauductor install` shows conflicts** — Use `--dry-run` first. Framework files are safe to overwrite. Doc files with existing content are preserved.

**Worker not showing in HUD** — Ensure you ran `/session-start` or `/claim`. Check `clauductor query workers` to verify registration.

**Lock stuck after crash** — Use `clauductor unlock --worker <id>` to release locks from a crashed session. The supervisor can also force-release locks.
