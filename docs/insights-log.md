# Insights Log

**Purpose**: Lightweight capture of technical insights discovered during development sessions. Acts as a triage inbox — when 3+ insights cluster around a topic, promote to a Learning Note or ADR.

**Promotion rules**:
- **3+ related insights** → Learning Note (implementation journey)
- **Architectural decision with trade-offs** → ADR
- **Recurring pattern or gotcha** → Add to CLAUDE.md

**Statuses**: Raw | Promoted (to LN/ADR) | Archived

---

## Active Insights

| Date | Milestone | Topic | Insight | Status |
|------|-----------|-------|---------|--------|
| 2026-03-30 | LIFE-1.1 | Hook throttling | Heartbeat uses timestamp file (`orchestration/.last-heartbeat`) with `find -mmin -1` for 60s throttle — avoids SQLite on every tool call | Raw |
| 2026-03-30 | LIFE-1.2 | Path normalization | Lock-guard must normalize absolute paths (from Claude Code stdin) to project-relative paths before DB lookup — locks stored as relative | Raw |
| 2026-03-30 | LIFE-1.3 | Hook `if` field | Claude Code hooks support `if` field (e.g., `Bash(git commit *)`) for argument-level filtering — prevents unnecessary process spawns | Raw |
| 2026-03-30 | LIFE-1.7a | Silent parse failures | `time.Parse` with discarded error (`_`) causes zero-value timestamps (00:00). SQLite drivers may return timestamps in varying formats — always try multiple | Raw |
| 2026-03-30 | LIFE-1.7a | TUI text handling | Truncation with ellipsis is better than wrapping for dashboard panels — wrapping shifts all content below, making layout unstable | Raw |

---

## Framework Bugs

_Track when the framework itself fails to prevent a problem it's designed to prevent._

| Date | Issue | What Failed | Fix Applied |
|------|-------|-------------|-------------|

---

## Promotion Log

| Cluster | Count | Promoted To | Date |
|---------|-------|-------------|------|
