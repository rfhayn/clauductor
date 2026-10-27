# Row fragments

One file per roadmap row, written by the row's own PR: `<row id>.md`. A row's PR never edits the
development journal, the insights log or any row's Status, so rows built in parallel never touch one
shared line (`pr-merge-guard` rule 7 refuses its session heading; the rest is the row's to keep). A
daily close folds every fragment here into one `## Session N` entry
(`.claude/skills/session-close/fold.sh`), moves the insight rows to the log, sets each row's Status
and deletes the fragments. This README stays.

`.claude/checks/journal.sh` holds the shape at the row's gate (`.claude/lib/fragments.sh`), and the
fold refuses a malformed fragment whole, so a record is never dropped silently.

## The shape

```markdown
# OPS-12 — what the row did, in a few words

- **PR**: #123
- **Session**: S10, started 2026-10-23 09:15 ET
- **Review**: 2 rounds, peak low, cap 3
- **Gate**: 14 min
- **Cost**: $4.20
- **Session start**: 180000 tokens, 6 min

## What happened

Two or three sentences and the facts: what changed, what landed.

## Decisions

Each with its reason; whose decision it was if it was the owner's.

## Surprises

What did not go as expected, or `None.`

## Insights

| 2026-10-23 | Tooling | a topic | the observation | how to verify it | Raw |
```

- The file name is the row's id, and line 1 starts `# <row id> — `.
- **Session** is the marker's (`session-start/marker.sh`, the row PR body's first line): the
  session plan's number and the time the session started, in `TIME_ZONE_LABEL`.
- **Review**: the rounds run, the highest severity found (`critical`, `high`, `medium`, `low` or
  `none`), and the cap (`sh .claude/review-cap.sh <row id>`). Past the cap, KNOWN LIMITS in the PR
  names the owner.
- **Gate**: the minutes of the last full gate before the fragment's commit. Review and Gate describe
  the runs BEFORE that commit: writing the fragment makes a new head, which the final gate's receipt
  covers. **Cost**: `usage-report.sh` over this row's sessions (`$<amount>`), optionally followed by
  `; top: <contributor> (~$<re-read>)` from `usage-report.sh --contributors` (one short line; the
  fold still totals only the leading `$<amount>`). **Session start**: the marker's `session-start
  cost:` line. A reading that failed is `CANNOT CHECK — <why>`, never `0`.
- **Also closes** (optional, after the fields above): `<row id>, <row id>`, the other rows a batched
  PR closes; the fold marks each `✅ merged` by this fragment's PR.
- **Insights**: rows of the log's table, six cells, date first, no header; or `None.`
- No `## Session` heading: only the close numbers a session.
