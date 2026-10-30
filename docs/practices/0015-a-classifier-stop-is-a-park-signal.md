# PRAC 0015: A classifier stop is a park signal, by a rule agreed before the retry

- **Status**: Proposed
- **Date**: 2026-10-30
- **Tags**: process, lanes, safety, classifiers
- **Source**: Standing Tee's PRAC-0044 (Accepted there 2026-10-17), ported as roadmap row ST-13
  (S18); `docs/insights-log.md` rows *classifier blocks the orchestrator* (2026-10-19), *no
  laundered workaround* (2026-10-18) and *classifier inconsistency* (2026-10-17); the owner queue's
  OPS-127 item (2026-10-24)

## Context
Sessions here run in auto mode. Two kinds of classifier can refuse a step: the safety classifier
on what a session writes or does, and the auto-mode permission classifier on a tool call. A
refusal is not a fault in the session's code, and it does not always mean the step is wrong.

Standing Tee met it three times before writing its PRAC-0044: two worktree sessions stopped in a row
while designing a change that refused `$` in a location (its #698), where a rule agreed before the
second try (a second stop parks) meant the second stop needed no decision; a deliberate break of a
security control, refused as "Security Test Removal", which the founder then ran outside auto mode;
and an `rm` and a secrets scan on a server, left for the founder and written down as not done.

This repo has met them too, and each time the session parked rather than routed around:
- **2026-10-17.** The classifier refused the merge of #142 while two PRs merged by the same command
  that day; the owner added a `/permissions` rule and #142 merged.
- **2026-10-18.** A lane's workaround of a sandbox limit was refused, and the main session did not
  run the step for it: relaying it would have laundered the refusal. The step went to the owner.
- **2026-10-19, session 6.** Deletions (worktrees, `git rm`) and anything read as a CI bypass were
  refused to the orchestrating session. The owner ran those steps.
- **2026-10-24, OPS-127.** Claude's own edit turning delegated approval on was refused; it was done
  in #246 on the owner's direct instruction.

The log declined to promote those rows: the classifier is the owner's control doing its job; the
harness already forbids running a step an agent was denied; and the classifier is Claude Code's,
not this repo's, so the owner's `/permissions` rule settled #142's refusal. All three hold (the
last is this record's Practice item 2 at work: the retry was decided outside the session), and none
is what this record adds. They say what to do once a refusal is recognised as one. This record
says what a session does next in its own work (no rewording, no other path, no pieces that pass),
who decides a retry, and that the retry's stop rule is agreed before it runs. None of that was
written down here, and every one of those sessions did it by judgment.

The failure this prevents is the one none of those sessions committed. A session meets a refusal
mid-task, rewords the step or reaches the same result by another path, and reports success. From
outside, a reworded step that passes reads exactly like a step that was fine all along, so a
workaround leaves no trace. And a session that decides for itself whether to retry is deciding
under the pressure of the task it was given.

## Practice
1. **A classifier refusal is a stop.** The session parks the step that was refused and does not
   retry it in another form: no rewording, no other tool, no splitting it into pieces that pass.
   It reports what was refused, and why it was being done, to the session that opened it
   (`reportTo`), or to the owner if none did. Work that does not depend on the step may go on.
2. **A retry is decided outside the session, with its stop rule agreed before it runs.** The
   session that opened the lane takes it to the owner. A retry, if there is one, goes ahead with
   the rule for the next stop already settled. By default that rule is to park for good, with an
   observable unpark trigger on a named change or issue (AGENTS.md rule 1). Then a second stop
   needs no decision.
3. **The owner may run the step instead**, outside auto mode. That is an owner action, and the
   session's report (its PR body, its journal fragment) says the owner ran it.

## Consequences
### Positive
- No session routes around a refusal, so a step that passes is a step no classifier refused.
- The decision to retry is made once, by the person who owns the risk, before the retry rather
  than during it.

### Negative / trade-offs
- A false refusal costs a round-trip to the owner and sometimes a parked PR.
- A refusal inside a `build-change` run lands mid-build as an unplanned owner step.

## Enforcement
- **`template/.claude/agents/builder.md`** carries the rule in the builder's brief: a classifier
  refusal is returned as `blocked`, never retried another way.
- **`template/.claude/skills/apply-change/SKILL.md`**, its *Report to the session that asked*
  clause, carries the same rule for a lane reporting to the session that opened it.
- **`.claude/checks/change-process.sh`** fails if either line is removed.
- **The harness's notice** on every agent message forbids running a step another session was
  denied (permission laundering), which holds rule 1 across sessions.
- **Known limits.** `merge-pr`, `session-close` and a main session get no line of their own: only
  the harness notice and this record reach them. Both lines are instructions to a model: nothing
  checks that a session obeys them, or detects a retry it made anyway. A session started without
  either brief gets nothing. Decision 2 has no executor beyond the session that opened the lane and
  the owner.

## Related
- Standing Tee PRAC-0044, which this ports.
- PRAC-0010 (close the class): a reworded step that passes closes an instance, not the class.
