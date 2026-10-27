# ST-9 — read-by-range PreToolUse hook (sh + awk)

- **PR**: #294
- **Session**: S14, started 2026-10-27 15:59 ET
- **Review**: 0 rounds, peak none, cap 2 (Risk low)
- **Gate**: CANNOT CHECK — full gate not run, lease held by ST-8
- **Cost**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID in this shell
- **Session start**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID

## What happened

Ported Standing Tee's read-by-range guard into the template as POSIX `sh` plus `lib/view-reader.awk` (no Node): PreToolUse on Read and Bash, 400-line cap, fail-open on unreadable payloads and unparsed shell. Registered in `settings.json`, documented in `hooks/README.md`, exercised in `checks/hooks.sh` (block and allow rows plus registration table). Synced `.claude/` and rebuilt `plugin/`.

## Decisions

Used a separate `view-reader.awk` forked from the gate tokenizer rather than extending `gate-reader.awk`, so gate and catch-up guards cannot drift.

## Surprises

BSD awk treats `index(line, "\0")` like an empty-string search and falsely marked every text line binary until that check was removed from the awk line counter.

## Insights

None.
