---
name: log-insight
model: opus
effort: medium
description: "Add a technical insight to the insights log (INSIGHTS in .claude/project.conf) immediately, then run the promotion check. Don't defer: sessions clear. TRIGGER when the user says 'log this insight', 'that's worth noting', 'remember this', 'note this gotcha', 'TIL', 'important discovery', 'capture this learning', or asks to record a technical observation."
argument-hint: <topic> <insight text>
---

# Log a technical insight

Add a row to the insights log now (the intake tier; `docs/insights-log.md` by default). Do not
defer it.

## Context: the state, computed
!`sh ${CLAUDE_PLUGIN_ROOT}/skills/log-insight/context.sh`

If the line above shows as literal text instead of output, run
`clauductor-model skills/log-insight/context.sh` yourself and read the result before step 1.

## What counts
Non-obvious observations found while working: a platform or library quirk, a trade-off, a
debugging technique that saved real time, a guard that failed silently, a premise that turned out
false. Not a changelog entry, and not something the code or git history already says.

## Steps
1. Add ONE row at the **top** of the table body:

   `| <today> | <Area> | <Topic> | <the observation> | <how to verify it> | Raw |`

   - **Area** is a coarse bucket from `INSIGHT_AREAS` in `.claude/project.conf` (or, if unset,
     one already used in the log).
   - **Topic** is a hierarchical tag, `<thing>/<aspect>`: `hooks/channel`, `db/pooling`.
   - **How to verify** is a command, a file or a test someone can run later. An observation that
     cannot be re-checked cannot be promoted.
2. Keep the observation specific enough to act on without re-deriving it.

## Then run the promotion check
Routing follows *What this tier holds* in `docs/adr/README.md`.
- **Already covered by an existing ADR or an `AGENTS.md` rule**: mark the row
  `Instance of ADR-NNNN (check N)` (or `Instance of AGENTS.md rule N`), or quote the covering
  sentence: `Instance of ADR-NNNN ("…")`. Find that sentence first; if you cannot, it is not
  covered.
- **A decision with trade-offs, or a lesson with a mechanism**: offer `/clauductor:new-adr`, which writes a
  new ADR or an amendment inside the ADR that already owns the principle.
- **3+ rows share a Topic**: suggest a promotion. Topic tags are typed by hand, so look for rows of
  the same SHAPE under other tags too.
- **A recurring gotcha or convention**: **not `AGENTS.md`**, which changes only to add a mechanism
  or delete something. Amend the ADR that owns the rule, propose a mechanism (a hook, a check), or
  keep the row as `Technique — no mechanism`.

Report which promotion, if any, applies.

## Project steps

What this project's enabled modules and its local layer (`.claude/local/skills/log-insight/`) add to this
skill. Follow them as part of the steps above:

!`sh ${CLAUDE_PLUGIN_ROOT}/extensions.sh fragments log-insight`
