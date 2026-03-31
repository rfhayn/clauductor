# PRD: Lifecycle Automation

**Author**: Rich
**Status**: READY
**Created**: 2026-03-30
**Milestone**: LIFE-1
**Prefix**: LIFE

---

## Problem Statement

Clauductor requires 10-15 manual steps to go from "I want to work" to "ready to code." Engineers must manually register workers, claim files, load context, and track status. There are no automatic safety checks — editing a locked file, forgetting to update the journal, or losing a heartbeat all happen silently. Multi-worker team startup requires manually opening terminals and launching each session.

This milestone adds three layers of automation: compound lifecycle skills, Claude Code hooks for real-time safety, and enhanced team startup.

## Scope

### In Scope
- 3 new lifecycle skills: `/start-project`, `/start-work`, `/done`
- 5 hook scripts in `.claude/hooks/` with settings.json configuration
- 2 new CLI commands: `clauductor check-lock`, `clauductor context`
- Enhanced `clauductor start`: HUD + supervisor + N worker panes
- `orchestration/config.json` for team configuration
- Enhanced `/spawn` with orchestration state in initial prompt
- Hook design documentation (hooks/README.md)
- Doc freshness enforcement: pre-commit hook + enhanced `/commit` skill
- Merge `roadmap.md` into `current-story.md`

### Out of Scope
- Changes to the HUD dashboard UI itself
- New orchestration DB schema changes
- Project-specific hooks (those go in project PRDs)

---

## Layer 1: Lifecycle Skills

### `/start-project` — Guided first-time setup wizard

**When**: First time using clauductor on a repo, or onboarding a new developer.

Interactive wizard that walks through each setup step, skipping items already configured:

1. Check CLAUDE.md Setup Checklist for uncompleted items
2. Define PREFIX registry (prompt for epic names/prefixes)
3. Configure build command in `/build` SKILL.md
4. Configure test command (if applicable)
5. Describe project architecture in CLAUDE.md
6. Configure deployment pipeline in `/release-prep` SKILL.md
7. Define architecture audit rules in `/architecture-audit` SKILL.md
8. Create first milestone via `/new-milestone`

**File**: `template/.claude/skills/start-project/SKILL.md`

### `/start-work [PREFIX-#.#]` — Pick up a piece of work

**When**: Every time a human or agent sits down to work on a specific milestone.

**Chain**: `/session-start` → `/claim PREFIX-#.#` → set status → report ready

- If no milestone specified, offer top of priority queue from `current-story.md`
- Default session type: `build`
- Degrades to just `/session-start` if no orchestration
- Logs single `start_work` event

**File**: `template/.claude/skills/start-work/SKILL.md`

### `/pane` — Open a new tmux pane

**When**: Need another terminal/claude session alongside the current one.

**Usage**:
- `/pane` — new shell pane in current project directory
- `/pane claude` — new pane that auto-launches claude
- `/pane claude /start-work LIFE-1.2` — new pane, launches claude with an initial command

**Implementation**: Detects current tmux session, runs `tmux split-window`, optionally sends `claude` and initial command via `tmux send-keys`.

**File**: `template/.claude/skills/pane/SKILL.md`

### `/done` — Wrap up work

**When**: Finished with a milestone or session.

**Chain**: `/review` → `/dev-journal` → `/commit` → `/pr` → `/milestone-complete` → `/release`

- Interactive: each step presents output, asks before proceeding
- Review failures stop the chain (user must fix before continuing)
- User can skip `/milestone-complete` if only finishing a sub-task
- Degrades gracefully without orchestration

**File**: `template/.claude/skills/done/SKILL.md`

---

## Layer 2: Hooks

**Directory**: `template/.claude/hooks/`

All hooks gracefully no-op when `orchestration/` doesn't exist. Every hook starts with `test -d "$PROJECT_DIR/orchestration" || exit 0`.

### Hook Design Principles (documented in hooks/README.md)

1. **Graceful degradation**: No orchestration = no-op. No crash, no error.
2. **Fast path**: <200ms execution. File stat before SQLite queries. Throttle where needed.
3. **Warn, don't block** (by default): Humans decide, hooks inform. Projects can override to exit 2.
4. **No side effects in PreToolUse**: Only check state, never write to DB from blocking hooks.
5. **JSON protocol**: Read JSON from stdin, optionally write JSON to stdout. Exit 0 = allow, exit 2 = block.
6. **Self-contained**: Each hook is a standalone shell script. No shared libraries or dependencies beyond `jq` and `clauductor`.

### Hook 1: `session-register.sh`
- **Event**: SessionStart (async)
- **Purpose**: Auto-register worker in orchestration DB on session start/resume
- **Behavior**: Parse branch name for milestone, run `clauductor register` (or `clauductor heartbeat` if already registered)
- **Why async**: Registration is not blocking. Session proceeds immediately.

### Hook 2: `heartbeat.sh`
- **Event**: PostToolUse (async, matcher: `Bash|Edit|Write`)
- **Purpose**: Keep worker alive in orchestration DB so supervisor can detect dead sessions
- **Behavior**: Send heartbeat via `clauductor heartbeat --worker-id [name]`
- **Throttle**: Only fires if last heartbeat was >60s ago. Uses timestamp file at `orchestration/.last-heartbeat` to avoid querying SQLite on every tool call.
- **Why async**: Must never slow down tool execution.

### Hook 3: `lock-guard.sh`
- **Event**: PreToolUse (sync, matcher: `Edit|Write`)
- **Purpose**: Warn before editing a file locked by another worker. Most important hook for human UX.
- **Behavior**:
  - Extract file path from tool input JSON
  - Run `clauductor check-lock --file <path>` (new fast CLI command)
  - If locked by current worker: exit 0 (allow silently)
  - If locked by another worker: output warning message, exit 0 (warn but allow)
  - If not locked: exit 0 (allow silently)
- **Default is warn, not block**: Exit 0 with a message. Projects that want hard enforcement change to exit 2.
- **Why sync**: Must run before the edit happens so the user sees the warning.

### Hook 4: `journal-check.sh` *(Removed in LIFE-1.8)*
- **Superseded by**: `doc-freshness.sh` — journal date check is one of six checks in the new hook
- **Original event**: PostToolUse (async, matcher: `Bash`, if: `Bash(git commit *)`)
- **Migration**: Removed in LIFE-1.8d. All functionality absorbed into doc-freshness.sh.

### Hook 5: `status-sync.sh`
- **Event**: PostToolUse (async, matcher: `Write`)
- **Purpose**: Sync milestone status to orchestration DB when current-story.md changes
- **Behavior**: Check if the written file is `current-story.md`. If so, parse for milestone status changes and update orchestration via `clauductor milestone update`.

### Hook 6: `doc-freshness.sh`
- **Event**: PreToolUse (sync, matcher: `Bash`, if: `Bash(git commit *)`)
- **Purpose**: Warn about stale or missing documentation before commits
- **Behavior**:
  - Parse branch name for current milestone (supports both PREFIX-#.# and legacy M#.# patterns)
  - Check each core doc exists — create from template if missing
  - Check `current-story.md` has current milestone as ACTIVE
  - Check `next-prompt.md` has pointer for current milestone
  - Check `next-prompt-PREFIX-#.md` exists for branch's milestone
  - Check `development-journal.md` has today's date in first 20 lines
  - Check `insights-log.md` exists
  - Check `requirements.md` has section for current milestone
  - Report what's stale/missing as WARNING (exit 0 with message), never blocks
- **Why sync + PreToolUse**: Runs before `git commit` executes, giving the user a chance to see warnings. Fast (file stat checks only, no DB queries).
- **Relationship to /commit skill**: The `/commit` skill runs the same checks proactively and can auto-fix issues. The hook catches commits made outside the skill (e.g., raw `git commit`).

### Hook Configuration

Added to `template/.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "bash .claude/hooks/session-register.sh",
            "async": true
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Bash|Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "bash .claude/hooks/heartbeat.sh",
            "async": true
          }
        ]
      },
    ],
    "PreToolUse": [
      {
        "matcher": "Edit|Write",
        "hooks": [
          {
            "type": "command",
            "command": "bash .claude/hooks/lock-guard.sh"
          }
        ]
      },
      {
        "matcher": "Bash",
        "if": "Bash(git commit *)",
        "hooks": [
          {
            "type": "command",
            "command": "bash .claude/hooks/doc-freshness.sh"
          }
        ]
      }
    ]
  }
}
```

---

## Layer 3: Team Startup & Agent Awareness

### Enhanced `clauductor start [-n COUNT]`

**Current**: Creates tmux session with one HUD pane. That's it.

**New**: Creates a full team workspace.

| Pane | Content |
|------|---------|
| Window 0 | HUD — `clauductor watch` (real-time dashboard) |
| Window 1 | Supervisor — auto-launches `claude` with `/supervisor` prompt |
| Windows 2-N | Worker terminals — auto-launch `claude` (configurable) |

**Terminal count**:
- `-n` flag overrides (e.g., `clauductor start -n 5`)
- Default from `orchestration/config.json` field `default_workers`
- Fallback: 3

**Auto-claude config**: `orchestration/config.json` field `auto_claude` (default: `true`). When `true`, worker panes auto-launch claude. When `false`, worker panes open as plain shells.

### New file: `orchestration/config.json`

```json
{
  "default_workers": 3,
  "auto_claude": true
}
```

Created by `clauductor init` and `clauductor install`. Projects customize as needed.

### Changes to `framework/internal/cmd/start.go`:
- Read `orchestration/config.json` for `default_workers` and `auto_claude`
- Accept `-n` flag (overrides config default)
- Create HUD in window 0 (existing behavior)
- Create supervisor in window 1 (new): `tmux new-window`, send `claude '/supervisor'`
- Create N worker windows (new): `tmux new-window`, optionally send `claude` based on `auto_claude`

### Enhanced `/spawn` initial prompt

Spawned agents get full orchestration context:

```
You are a build worker in a Clauductor-managed project.

Your milestone: [PREFIX-#.#] - [description]
Your branch: feature/[PREFIX-#.#]-[description]

## Startup
Run /start-work [PREFIX-#.#] to register, claim files, and load context.

## Orchestration State
Current workers: [output of clauductor query workers]
Current locks: [output of clauductor query locks]

## Rules
- Focus only on files in your claimed manifest
- If you need a locked file, run /blocked [file]
- Commit every 15-30 minutes using /commit
- Run /status periodically to check orchestration state
- When done, run /done
```

**Changes to `template/.claude/skills/spawn/SKILL.md`**: Step 3 (Prepare Initial Prompt) queries `clauductor context` and embeds the output.

### New CLI: `clauductor context [--json]`

Returns complete orchestration snapshot: workers, locks, recent events, milestones. Used by `/spawn` to embed state in initial prompts.

**File**: `framework/internal/cmd/context.go`

### New CLI: `clauductor check-lock --file <path>`

Fast JSON response for the lock-guard hook:
```json
{"locked": false}
```
or:
```json
{"locked": true, "worker_id": "rich-M18-build", "milestone": "M18.1", "locked_at": "2026-03-30T10:30:00Z"}
```

**File**: `framework/internal/cmd/check_lock.go`

---

## Milestones

**IMPORTANT: Commit after every milestone step.**

### LIFE-1.1: Hook Infrastructure (2h)
1. Create `template/.claude/hooks/` directory
2. Write `hooks/README.md` documenting hook protocol and design principles
3. Write `session-register.sh` (SessionStart async hook)
4. Write `heartbeat.sh` (PostToolUse async hook with 60s throttle)
5. Update `template/.claude/settings.json` with SessionStart and PostToolUse hooks config
6. Update `framework/internal/cmd/install.go`: add `.claude/hooks/` to `tierFramework` classification
7. Test: `clauductor install --dry-run` shows hooks as FRAMEWORK files
8. **Commit**: `LIFE-1.1: Add hook infrastructure with session-register and heartbeat`

### LIFE-1.2: Lock Guard (1.5h)
1. Implement `clauductor check-lock --file <path>` in Go
   - New file: `framework/internal/cmd/check_lock.go`
   - Register command in `root.go`
   - Returns JSON: `{"locked": bool, "worker_id": str, "milestone": str, "locked_at": str}`
2. Write `lock-guard.sh` (PreToolUse sync hook)
   - Extract file path from stdin JSON
   - Call `clauductor check-lock`
   - Warn if locked by another worker, allow if locked by self or unlocked
3. Update `settings.json` with PreToolUse hook config
4. Test: Lock a file via CLI, verify hook warns on edit attempt
5. **Commit**: `LIFE-1.2: Add lock-guard hook with check-lock CLI command`

### LIFE-1.3: Journal & Status Hooks (1h)
1. Write `journal-check.sh` (PostToolUse async, fires on git commit)
2. Write `status-sync.sh` (PostToolUse async, fires on Write to current-story.md)
3. Update `settings.json` with journal-check hook config
4. Test: Commit without journal update, verify reminder appears
5. **Commit**: `LIFE-1.3: Add journal-check and status-sync hooks`

> **Note**: `journal-check.sh` is superseded by LIFE-1.8's `doc-freshness.sh`. The journal date check moves into doc-freshness along with five additional checks. `journal-check.sh` is removed in LIFE-1.8d.

### LIFE-1.4: Lifecycle Skills (2h)
1. Write `template/.claude/skills/start-project/SKILL.md` (guided wizard)
2. Write `template/.claude/skills/start-work/SKILL.md` (session-start + claim chain)
3. Write `template/.claude/skills/done/SKILL.md` (review + journal + commit + pr + milestone-complete + release chain)
4. Write `template/.claude/skills/pane/SKILL.md` (tmux pane management: detect session, split-window, optionally send claude + initial command)
5. Test: Invoke each skill, verify they chain correctly
6. **Commit**: `LIFE-1.4: Add start-project, start-work, done, and pane lifecycle skills`

### LIFE-1.5: Context Command & Spawn Enhancement (1.5h)
1. Implement `clauductor context [--json]` in Go
   - New file: `framework/internal/cmd/context.go`
   - Composes data from `ListWorkers`, `ListLocks`, `ListRecentEvents`
   - Register in `root.go`
2. Update `template/.claude/skills/spawn/SKILL.md`:
   - Step 3 queries `clauductor context` and embeds in initial prompt
   - Initial prompt references `/start-work` instead of separate commands
   - Lock state and worker state included
3. Test: `clauductor context --json` returns valid JSON. Spawned session gets orchestration state.
4. **Commit**: `LIFE-1.5: Add context command and enhance spawn with orchestration awareness`

### LIFE-1.6: Team Startup Enhancement (2h)
1. Create `template/orchestration/config.json` with defaults
2. Update `framework/internal/cmd/start.go`:
   - Read `orchestration/config.json` for `default_workers` and `auto_claude`
   - Add `-n` flag for worker count override
   - Create window 0: HUD (`clauductor watch`)
   - Create window 1: supervisor (`claude '/supervisor'`)
   - Create windows 2-N: workers (optionally auto-claude based on config)
3. Update `framework/internal/cmd/install.go`: include `orchestration/config.json` in install
4. Test: `clauductor start` creates HUD + supervisor + 3 workers. `clauductor start -n 5` creates 5 workers.
5. **Commit**: `LIFE-1.6: Enhance clauductor start with team workspace (HUD + supervisor + N workers)`

### LIFE-1.7a: HUD Display Fixes (30m)
1. **Timestamp bug**: Fix `00:00` timestamps in Recent Activity panel — events show `00:00` instead of actual time. HUD is not reading `created_at` field from SQLite correctly.
2. **Activity text wrapping**: When activity text wraps to the next line, the continuation should align with where the text starts (after the timestamp and worker name), not wrap to column 0. Example:
   ```
   07:50  rich-LIFE-    worker_registered: type=build
   1.2-build            milestone=LIFE-1.2 owner=rich   ← BAD (wraps to col 0)

   07:50  rich-LIFE-1.2-build  worker_registered: type=build
                                milestone=LIFE-1.2       ← GOOD (aligned)
   ```
3. Fix in `framework/internal/hud/panels.go` or wherever events are rendered
4. Verify both fixes display correctly
5. **Commit**: `LIFE-1.7a: Fix HUD event timestamps and activity text wrapping`

### LIFE-1.7: Documentation & Polish (1h)
1. Update `template/CLAUDE.md`: add lifecycle skills table, hooks section
2. Update `template/.claude/skills/skills/SKILL.md`: add start-project, start-work, done
3. Update `docs/QUICKSTART.md` with lifecycle workflow
4. Update `docs/onboarding.md` with hooks section
5. End-to-end test: fresh `clauductor init`, `/start-project`, `/start-work`, work, `/done`
6. Note: `roadmap.md` references in template docs are cleaned up in LIFE-1.8a (the roadmap merge)
7. **Commit**: `LIFE-1.7: Documentation and end-to-end validation`

### LIFE-1.8: Doc Freshness Enforcement (2.5h)

**Summary**: Merge `roadmap.md` into `current-story.md`, add commit-triggered doc freshness checks via a new hook and enhanced `/commit` skill.

#### Phase A: Merge roadmap.md into current-story.md (30m)
1. Add "Planning Accuracy" table to `template/docs/current-story.md` (from roadmap.md)
2. Remove `template/docs/roadmap.md` from the template
3. Update `template/docs/project-index.md`: remove roadmap.md row from Core Documentation table
4. Update `template/.claude/skills/milestone-complete/SKILL.md`: remove roadmap.md references
5. **Commit**: `LIFE-1.8a: Merge roadmap.md into current-story.md, remove roadmap template`

#### Phase B: doc-freshness.sh hook (1h)
1. Create `template/.claude/hooks/doc-freshness.sh` (PreToolUse sync hook, matcher: `Bash`, if: `Bash(git commit *)`)
2. Hook checks before commit (all WARNING only, exit 0):
   - Each core doc exists — creates from template if missing (fast file copy)
   - `current-story.md` has current milestone (parsed from branch) listed as ACTIVE
   - `next-prompt.md` has a pointer for the current milestone
   - `next-prompt-PREFIX-#.md` exists for the branch's milestone
   - `development-journal.md` has today's date in first 20 lines
   - `insights-log.md` exists
   - `requirements.md` has a section for the current milestone
3. Milestone parsing supports both `PREFIX-#.#` (e.g., `AUTH-1.3`) and legacy `M#.#` (e.g., `M18.1`) branch naming
4. Update `template/.claude/settings.json`: add PreToolUse entry for doc-freshness.sh
5. Test: commit on a branch with stale docs, verify warnings appear but commit proceeds
6. **Commit**: `LIFE-1.8b: Add doc-freshness.sh pre-commit hook`

#### Phase C: Enhanced /commit skill (1h)
1. Update `template/.claude/skills/commit/SKILL.md`:
   - Add "Step 0: Doc Freshness" before staging — runs the same checks as doc-freshness.sh
   - If docs need updating: update `current-story.md` (milestone status, progress)
   - Generate journal entry via `/dev-journal` agent if stale
   - Scaffold missing `next-prompt-PREFIX-#.md` if needed
   - Prompt about insights (optional, not blocking)
   - Doc updates are staged and included in the same commit as code changes
   - Remove the existing "Post-Commit" journal/insights check (moved to pre-commit freshness)
2. Test: `/commit` with stale docs triggers doc updates before staging
3. **Commit**: `LIFE-1.8c: Enhance /commit skill with pre-commit doc freshness`

#### Phase D: journal-check.sh deprecation (15m)
1. Remove `template/.claude/hooks/journal-check.sh` (superseded by doc-freshness.sh)
2. Remove the journal-check.sh entry from `template/.claude/settings.json` PostToolUse hooks
3. Update `template/.claude/hooks/README.md`: document doc-freshness.sh, remove journal-check.sh
4. **Commit**: `LIFE-1.8d: Remove journal-check.sh, superseded by doc-freshness.sh`

#### Core Docs Enforced by doc-freshness.sh

| Document | Cadence | What "current" means |
|----------|---------|---------------------|
| `current-story.md` | Per session | Active milestone matches branch, progress reflects recent commits |
| `next-prompt.md` | Per milestone | Hub has pointer for active milestone |
| `next-prompt-PREFIX-#.md` | Per milestone | File exists for branch's milestone |
| `development-journal.md` | Per session | Has entry with today's date + current milestone |
| `insights-log.md` | Per session | File exists (content optional per commit) |
| `requirements.md` | Per milestone | Current milestone has a section |

`roadmap.md` is removed (merged into current-story.md). `project-naming-standards.md` is setup-time only, not enforced on commit.

---

## Files Created

| File | Purpose |
|------|---------|
| `template/.claude/hooks/README.md` | Hook protocol documentation |
| `template/.claude/hooks/session-register.sh` | Auto-register worker on session start |
| `template/.claude/hooks/heartbeat.sh` | Auto-heartbeat with 60s throttle |
| `template/.claude/hooks/lock-guard.sh` | Warn before editing locked files |
| `template/.claude/hooks/journal-check.sh` | Remind about stale journal after commits |
| `template/.claude/hooks/status-sync.sh` | Sync milestone status to orchestration |
| `template/.claude/hooks/doc-freshness.sh` | Pre-commit doc freshness warnings |
| `template/.claude/skills/start-project/SKILL.md` | Guided first-time setup wizard |
| `template/.claude/skills/start-work/SKILL.md` | Pick up a piece of work |
| `template/.claude/skills/done/SKILL.md` | Wrap up work |
| `template/.claude/skills/pane/SKILL.md` | Open new tmux pane |
| `template/orchestration/config.json` | Team config (workers, auto-claude) |
| `framework/internal/cmd/check_lock.go` | Fast lock check CLI |
| `framework/internal/cmd/context.go` | Orchestration snapshot CLI |

## Files Modified

| File | Change |
|------|--------|
| `template/.claude/settings.json` | Add hooks section |
| `template/.claude/skills/spawn/SKILL.md` | Enhanced initial prompt with orchestration state |
| `template/.claude/skills/skills/SKILL.md` | Add 3 new lifecycle skills |
| `template/CLAUDE.md` | Add lifecycle skills and hooks documentation |
| `framework/internal/cmd/root.go` | Register check-lock and context commands |
| `framework/internal/cmd/install.go` | Handle `.claude/hooks/` as framework tier |
| `framework/internal/cmd/start.go` | Multi-pane startup, config reading, -n flag |
| `template/.claude/skills/commit/SKILL.md` | Add pre-commit doc freshness checks, remove post-commit journal check |
| `template/.claude/skills/milestone-complete/SKILL.md` | Remove roadmap.md references |
| `template/docs/current-story.md` | Add Planning Accuracy table (from roadmap.md) |
| `template/docs/project-index.md` | Remove roadmap.md row |

## Files Removed

| File | Reason |
|------|--------|
| `template/docs/roadmap.md` | Merged into `current-story.md` (LIFE-1.8a) |
| `template/.claude/hooks/journal-check.sh` | Superseded by `doc-freshness.sh` (LIFE-1.8d) |

---

## Rollback Plan

```bash
# Revert to pre-LIFE-1 state
git checkout main~N -- template/ framework/
```

Hooks are self-contained scripts — removing `.claude/hooks/` and the hooks section from `settings.json` fully disables all hook behavior.

## Success Criteria

1. 5 hook scripts in `.claude/hooks/`, all no-op when orchestration absent
2. Heartbeat fires automatically with 60s throttling
3. Lock guard warns when editing another worker's file
4. Journal reminder fires after commits when journal is stale
5. `/start-project` walks through setup interactively
6. `/start-work` chains session-start + claim in one command
7. `/done` chains review → journal → commit → PR → milestone-complete → release
8. `clauductor check-lock` returns JSON in <50ms
9. `clauductor context` returns complete orchestration snapshot
10. `clauductor start` creates HUD + supervisor + N worker panes
11. `clauductor start -n 5` respects the count flag
12. Spawned agents receive lock/worker state in initial prompt
13. `clauductor install` and `clauductor update` handle `.claude/hooks/` correctly
14. `doc-freshness.sh` warns about stale docs before commits (exit 0, non-blocking)
15. `/commit` skill auto-updates stale docs before staging
16. `roadmap.md` removed from template; Planning Accuracy table lives in `current-story.md`
17. `journal-check.sh` removed; its functionality absorbed into `doc-freshness.sh`
