# Next Prompt: LIFE-1.1 — Hook Infrastructure

**Branch**: `feature/LIFE-1.1-hook-infrastructure`
**Status**: ACTIVE
**PRD**: `docs/prds/active/LIFE-1-lifecycle-automation.md` (Section: Layer 2: Hooks, Milestone LIFE-1.1)

---

## What to Build

Create the hook infrastructure that all subsequent LIFE-1 milestones depend on. This includes the hook directory, design documentation, and two async hooks (session-register, heartbeat).

## Tasks

1. **Create `template/.claude/hooks/README.md`** — Document hook protocol and design principles:
   - Graceful degradation (no orchestration = no-op)
   - Fast path (<200ms, file stat before SQLite)
   - Warn, don't block (default behavior)
   - No side effects in PreToolUse
   - JSON protocol (stdin/stdout, exit codes)
   - Self-contained scripts

2. **Write `template/.claude/hooks/session-register.sh`** — SessionStart async hook:
   - Parse branch name for milestone prefix
   - Run `clauductor register` (or `clauductor heartbeat` if already registered)
   - Starts with `test -d "$PROJECT_DIR/orchestration" || exit 0`

3. **Write `template/.claude/hooks/heartbeat.sh`** — PostToolUse async hook:
   - Fires on `Bash|Edit|Write` tool use
   - Throttle: only fire if last heartbeat >60s ago
   - Uses timestamp file at `orchestration/.last-heartbeat`
   - Runs `clauductor heartbeat --worker-id [name]`

4. **Update `template/.claude/settings.json`** — Add hooks configuration:
   - SessionStart: session-register.sh (async)
   - PostToolUse (matcher: `Bash|Edit|Write`): heartbeat.sh (async)

5. **Update `framework/internal/cmd/install.go`** — Add `.claude/hooks/` to `tierFramework` classification

6. **Verify**: `clauductor install --dry-run` shows hooks as FRAMEWORK files

## Key Files

- `template/.claude/settings.json` — existing, needs hooks section added
- `framework/internal/cmd/install.go` — existing, needs tier update
- `template/.claude/hooks/` — new directory (3 new files)

## Dependencies

- Existing `clauductor register` and `clauductor heartbeat` CLI commands (already implemented)
- Existing `orchestration/` directory structure

## Commit Message

```
LIFE-1.1: Add hook infrastructure with session-register and heartbeat
```
