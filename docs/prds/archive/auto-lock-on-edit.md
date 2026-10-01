# PRD: Auto-Lock on Edit

**Status**: PLANNED
**Estimated**: 3-5 hours
**Priority**: Framework enhancement
**Created**: April 2, 2026

---

## Problem Statement

Today, clauductor workers must explicitly lock files via `/claim` or `clauductor lock` before editing them. In practice, workers often skip this step — they start editing files without locking, creating race conditions when multiple workers are active. The existing `lock-guard.sh` hook only **warns** when a file is already locked by another worker; it doesn't prevent the edit or auto-lock for the current worker.

**Joe's law of orchestration**: If a worker CAN edit a file without locking it first, it eventually WILL.

### Current Flow (broken)
1. Worker registers via `/start-work`
2. Worker claims files via `/claim` (often forgotten or incomplete)
3. Worker edits files — no lock check unless `lock-guard.sh` fires AND another worker has the lock
4. Two workers edit the same file → conflict

### Desired Flow
1. Worker registers via `/start-work`
2. Worker edits a file → hook auto-locks it for that worker before the edit executes
3. If another worker already has the lock → edit is **blocked** with a clear error message
4. Worker finishes → locks released on deregister (existing behavior)

---

## Solution: Auto-Lock PreToolUse Hook

Upgrade `lock-guard.sh` from a passive warning system to an active lock-on-edit system.

### Design Principles

1. **Zero friction for the writer** — auto-lock is invisible when no conflicts exist
2. **Hard block on conflicts** — if another worker holds the lock, the edit fails with a clear message naming the lock holder
3. **Existing `/claim` still works** — pre-claiming files is still valid for declaring intent; auto-lock is the safety net
4. **No new identity mechanism** — use the existing git branch + $USER derivation (already proven in `session-register.sh` and `heartbeat.sh`)
5. **Idempotent** — re-locking the same file by the same worker is a no-op
6. **PreToolUse only** — sync hook, must be fast (<100ms)

### Identity Resolution

Workers are identified by `{USER}-{MILESTONE}-{SESSION_TYPE}`, derived from:
- `$USER` environment variable
- Git branch name parsed for milestone (e.g., `feature/M18.1.3-store-ui` → `M18.1.3`)
- Session type from `orchestration/.session-status` or default `build`

This is the same derivation used by `session-register.sh` and `heartbeat.sh`. No new mechanism needed.

**Edge case — supervisor on main**: The supervisor runs on `main` with no milestone branch. Its worker ID is `supervisor` (hardcoded in the supervisor skill). The hook should detect this and either:
- Skip auto-locking for supervisor (supervisor doesn't edit code)
- Use `supervisor` as the worker ID

**Edge case — no orchestration**: If `orchestration/` directory doesn't exist, the hook exits silently (existing graceful degradation pattern).

---

## Changes Required

### 1. Upgrade `lock-guard.sh` → `auto-lock.sh`

**File**: `template/.claude/hooks/lock-guard.sh` (rename or replace)

Current behavior:
```
PreToolUse(Edit/Write) → check-lock → warn if locked by other → allow anyway
```

New behavior:
```
PreToolUse(Edit/Write) → 
  1. Derive worker ID (git branch + $USER)
  2. Check if worker is registered (skip if not — non-orchestrated session)
  3. Call `clauductor auto-lock --worker-id <id> --file <path>`
     - File unlocked → lock it, return success
     - File locked by same worker → no-op, return success  
     - File locked by other worker → return BLOCK with error message
  4. Exit 0 (allow) or exit 2 (block)
```

**Hook input parsing**: The hook receives JSON on stdin with the tool input. Extract `file_path` from Edit/Write tool calls:
```bash
FILE_PATH=$(echo "$HOOK_INPUT" | jq -r '.input.file_path // empty')
```

### 2. New CLI Command: `clauductor auto-lock`

**File**: `framework/internal/cmd/auto_lock.go`

```go
// auto-lock combines check + lock in one atomic operation
// Returns exit 0 if lock acquired (or already held), exit 1 if conflict
func autoLockCmd() *cobra.Command {
    // Flags: --worker-id, --file
    // 1. Check if file is locked
    // 2. If unlocked → lock it, log event, return 0
    // 3. If locked by same worker → return 0 (idempotent)
    // 4. If locked by different worker → print conflict JSON, return 1
}
```

**State function**:
```go
func (db *DB) AutoLock(workerID, milestone, filePath string) (*LockConflict, error) {
    // Single transaction:
    // 1. SELECT from locks WHERE file_path = ?
    // 2. If no row → INSERT lock, return nil (success)
    // 3. If row exists AND worker_id = workerID → return nil (idempotent)
    // 4. If row exists AND worker_id != workerID → return LockConflict
}
```

**Output on conflict** (JSON for hook parsing):
```json
{
  "blocked": true,
  "file": "forager/Views/Settings/SettingsView.swift",
  "locked_by": "rich-M18.1.3-build",
  "locked_at": "2026-04-02T19:30:00Z",
  "milestone": "M18.1.3"
}
```

**Output on success**:
```json
{"blocked": false, "action": "locked"}
```
or
```json
{"blocked": false, "action": "already_held"}
```

### 3. Event Logging

Auto-lock events should be logged but throttled to avoid noise:
- `auto_lock` event type: logged on first lock acquisition only (not on idempotent re-locks)
- `auto_lock_blocked` event type: always logged (important for debugging conflicts)

### 4. Unlock Mechanism

Existing unlock paths remain:
- `clauductor deregister --worker-id <id>` — releases all locks (existing)
- `clauductor unlock --worker-id <id>` — releases all locks for worker (existing)
- `clauductor unlock --file <path>` — releases specific file lock (existing)
- `/release` skill — calls deregister (existing)

**New**: Add `clauductor unlock --auto` flag that releases all auto-acquired locks for the current derived worker ID (convenience for sessions that don't use `/release`).

### 5. Settings.json Hook Registration

The hook needs to be registered in the project's `.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit|Write",
        "command": ".claude/hooks/auto-lock.sh",
        "timeout": 5000
      }
    ]
  }
}
```

This replaces the current `lock-guard.sh` registration (if present).

---

## Performance Requirements

- Hook execution: < 100ms (SQLite query + optional insert)
- No network calls
- No blocking on DB contention (WAL mode handles concurrent reads)
- Graceful degradation: if DB is unavailable, allow the edit (don't block work)

---

## Migration Path

1. Add `auto-lock` command to clauductor binary
2. Replace `lock-guard.sh` with `auto-lock.sh` in template
3. Run `clauductor update` in existing projects to sync new hook
4. Existing `/claim` workflows continue to work — auto-lock is additive

---

## Acceptance Criteria

- [ ] `clauductor auto-lock --worker-id X --file Y` acquires lock atomically
- [ ] Same worker re-locking same file is a no-op (exit 0)
- [ ] Different worker locking same file returns conflict (exit 1) with clear JSON
- [ ] PreToolUse hook on Edit/Write calls auto-lock before allowing edit
- [ ] Hook blocks edit when conflict exists (exit 2, error message shown to Claude)
- [ ] Hook allows edit when no conflict (exit 0, transparent)
- [ ] No orchestration directory → hook exits silently (graceful degradation)
- [ ] Unregistered worker → hook exits silently (non-orchestrated session)
- [ ] Auto-lock events logged (first acquisition only, not re-locks)
- [ ] Blocked events always logged
- [ ] Hook execution < 100ms
- [ ] Existing `/claim`, `lock`, `unlock`, `deregister` commands unaffected
- [ ] `clauductor update` syncs new hook to existing projects

---

## Testing Plan

### Unit Tests (Go)
- AutoLock: unlocked file → acquires lock
- AutoLock: same worker → idempotent no-op
- AutoLock: different worker → returns LockConflict
- AutoLock: worker not registered → error
- AutoLock: concurrent auto-locks from different workers → one wins, one conflicts

### Integration Tests (shell)
- Hook receives Edit tool input → extracts file_path → calls auto-lock
- Hook blocks edit when file locked by other worker
- Hook allows edit when file unlocked
- Hook allows edit when file locked by same worker
- Hook exits silently when no orchestration directory
- Hook exits silently when worker not registered

### Manual Tests
- Two Claude Code sessions editing different files → both succeed
- Two Claude Code sessions editing same file → second is blocked with clear message
- Worker deregisters → locks released → other worker can now edit

---

## Future Considerations

- **Directory-level locks**: Lock `forager/Views/Grocery/` instead of individual files — useful for new-file creation where the path isn't known yet
- **Lock timeout**: Auto-expire locks after N hours if worker heartbeat is stale
- **Lock dashboard**: Show lock status in HUD with file paths and holders
- **Optimistic locking**: Allow edits but warn on save if file was modified by another worker since lock acquisition
