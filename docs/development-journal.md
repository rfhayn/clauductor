# Development Journal

**Purpose**: A narrative chronicle of building this project — capturing decisions, learning moments, AI tooling evolution, and the story behind the code. Unlike the insights log (quick-reference table) or learning notes (milestone summaries), this journal tells the *why* behind the *what*.

**Format**: Session-level entries in reverse chronological order.

---

## 2026-03-30 — LIFE-1: Lifecycle Automation (Session 1)

**Milestone**: LIFE-1.1 through LIFE-1.4, plus LIFE-1.7a
**Branch**: `feature/LIFE-1-lifecycle-automation`

### What Happened

Built the entire hook infrastructure and lifecycle skills layer for Clauductor in a single session. Started with LIFE-1.1 (hook directory, session-register, heartbeat) and progressed through LIFE-1.2 (lock-guard + check-lock CLI), LIFE-1.3 (journal-check + status-sync hooks), and LIFE-1.4 (start-project, start-work, done, pane skills). Also fixed two HUD bugs as LIFE-1.7a (timestamp display and activity text wrapping).

### Key Decisions

- **Timestamp file throttle over SQLite queries**: The heartbeat hook uses `find -mmin -1` on a file instead of querying the DB. This keeps the fast path under 200ms even on slow filesystems. The trade-off is slight imprecision (file mtime has ~1s resolution), but for a 60s throttle this is irrelevant.

- **Warn, don't block for lock-guard**: The hook exits 0 with a warning message rather than exit 2 (block). This respects human autonomy — the developer sees the warning but can proceed. Projects wanting hard enforcement just change one line.

- **`parseSQLiteTime` multi-format fallback**: Rather than guessing which format the SQLite driver returns, the fix tries 5 common formats in order. This is defensive but correct — different go-sqlite3 versions behave differently.

- **Truncation over wrapping in HUD**: For the activity panel, truncating long lines with `…` keeps layout stable. Wrapping would cause panel height to fluctuate as data changes, breaking the dashboard aesthetic.

- **Added `/pane` skill**: The PRD was updated to include a `/pane` skill for quick tmux pane management. Simpler than `/spawn` — no orchestration, just "give me another terminal."

### AI Tooling Observations

- Claude Code's hook `if` field enables argument-level filtering (e.g., `Bash(git commit *)`) — this prevents the hook process from spawning at all for non-matching commands. Significant performance win vs. filtering inside the script.

- Skills-as-markdown is a powerful pattern: the 4 lifecycle skills are pure instructions, no executable code. Claude reads and follows them, using its own tools. This makes skills composable (chaining via `/skill-name`) without any runtime dependency.

### What's Next

LIFE-1.5 (context CLI + spawn enhancement), LIFE-1.6 (team startup), LIFE-1.7 (docs + polish).
