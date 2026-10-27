# ST-8 — port usage-report --contributors into Clauductor

- **PR**: #293
- **Session**: S14, started 2026-10-27 15:01 ET
- **Review**: 0 rounds, peak none, cap 3
- **Gate**: CANNOT CHECK — scoped checks only (metrics, journal), not full gate
- **Cost**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID in this Cursor lane
- **Session start**: CANNOT CHECK — no CLAUDE_CODE_SESSION_ID

## What happened

Standing Tee's context attribution (`usage-report.mjs --contributors`) is now `template/.claude/lib/usage-contributors.jq` plus `usage-report.sh --contributors`, sharing transcript reading with `lib/usage.sh`. `metrics.sh` falsifies a fixture where a `Read docs/roadmap.md` dominates; row fragments may append `; top: …` on **Cost** per `docs/journal.d/README.md`.

## Decisions

None beyond the approved row scope.

## Surprises

jq's `=` assignment returns the rhs, not the thread state — attribution needed explicit `callIds` and `| .` after every `+=`.

## Insights

None.
