# PRAC 0015: A classifier stop is a park signal, by a rule agreed before the retry

- **Status**: Proposed
- **Date**: 2026-10-30
- **Tags**: process, lanes, safety, classifiers
- **Source**: Standing Tee's PRAC-0044 (Accepted there 2026-10-17), ported as roadmap row ST-13
  (S18)

## Context
Sessions here run in auto mode. Two kinds of classifier can refuse a step: the safety classifier
on what a session writes or does, and the auto-mode permission classifier on a tool call. A
refusal is not a fault in the session's code, and it does not always mean the step is wrong.

Standing Tee met it three times before writing its PRAC-0044: two worktree sessions stopped in a
row while designing one refusal, where a rule agreed before the second try (a second stop parks)
meant the second stop needed no decision; a deliberate break of a security control, refused as
"Security Test Removal", which the founder then ran outside auto mode; and an `rm` and a secrets
scan on a server, left for the founder and written down as not done.

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
   record says so.

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
  clause, carries the same sentence for a lane reporting to the session that opened it.
- **Known limits.** Both are instructions to a model: nothing checks that a session obeys them,
  or detects a retry it made anyway. A session started without either brief gets nothing.
  Decision 2 has no executor beyond the session that opened the lane and the owner.

## Related
- Standing Tee PRAC-0044, which this ports.
- PRAC-0010 (close the class): a reworded step that passes closes an instance, not the class.
