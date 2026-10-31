# PRAC 0023: A decision is accepted only on the evidence its assumptions name

- **Status**: Accepted (the owner, 2026-10-31)
- **Date**: 2026-10-31
- **Tags**: decisions, planning, adr
- **Source**: the S19 pre-mortem (OPS-182); `docs/plans/s19-spikes.md`; the insights-log row
  `decisions/accepted-before-tested` (2026-10-31)

## Context
Every earlier pivot followed one pattern: a design was accepted in prose and work was committed to
it, and the constraint that bound it was checked only later, when something outside prompted a
fresh read. The evidence was cheap and available each time.
- MAPS: approved 10-12, cut 10-14.
- Standing Tee convergence: 10-01 to 10-12.
- The browser UI and PANEL-46: 10-14 to 10-27.
- ADR-0011's "ADR-0006 unchanged".
- The merge guard in shell: 10-13 to 10-29.
- ADR-0012 in shell: 10-29 to 10-31.

The S19 drafts repeated the pattern: three load-bearing claims were refuted from the docs in
minutes, and four more by spikes in an evening (`docs/plans/s19-spikes.md`).

## Practice
1. **Every ADR names what it rests on.** A `## Rests on` section lists each assumption the decision
   needs to be true: a platform behaviour, an existing implementation elsewhere, the reason behind
   an earlier record, a cost estimate. Each one cites a result (a spike, a measurement, a doc page
   that was opened), or says **untested** and names the gate or row that will test it.
2. **Accepted means tested.** An ADR moves from Proposed to Accepted only when every assumption
   in it cites a passed result. Until then, work may build only what the tested part needs, and
   the owner may accept it with a named assumption explicitly left at risk.
3. **A phase starts on its gate's evidence.** The roadmap's phases end in gates that list evidence,
   not dates. A phase's rows are not offered until the previous gate's evidence is recorded.
4. **A spike comes before a commitment.** When a decision depends on how a platform behaves, the
   cheapest experiment that could refute it runs first, in a throwaway repo, before any row that
   depends on it starts.
5. **New outside inputs queue.** An article, a release or a new tool that suggests a change of
   direction becomes an idea record, read at the next gate, not a pivot mid-phase. A platform change
   that breaks a tested assumption is not an outside input: it trips that assumption, and the ADR is
   reopened.
6. **An estimate that slips past its tripwire is trimmed, not pivoted.** When a phase overruns by
   more than two sessions, the owner chooses what to cut from the remaining scope before anything
   new is added.

## Consequences
### Positive
- A decision's weak point is written next to it, with the row that will test it, so a refuted
  assumption reopens one named ADR instead of surfacing as a pivot.
- ADR-0016 to ADR-0021 were accepted on 2026-10-31 under this record, each untested item marked
  "accepted at risk by the owner" and owned by a row (OPS-199 for the platform spikes).

### Negative / trade-offs
- A spike costs time before any build starts, and some assumptions (a review-round estimate) can
  only be tested by doing the work.
- The phase gates hold rows back that might have run in parallel.

## Enforcement
- **Practices 1 and 2:** a check, OPS-200 (the evidence-gated ADRs row in the roadmap). An Accepted
  ADR dated after this record must have a `## Rests on` section in which no item reads "untested"
  without "accepted at risk by the owner". Until OPS-200 merges, **Nothing. You.**
- **Practice 3:** the phase-gate filter in `roadmap.next` and `lanes.candidates` (OPS-207). It
  offers no row of a phase whose previous gate's evidence is not recorded in `plan.json`. Until
  then, **Nothing. You.**
- **Practice 4:** review judgment: **Nothing. You.**
- **Practice 5:** **Nothing. You.**, until PROD-2's idea record exists; then the idea action, plus
  session-start listing queued ideas at a gate.
- **Practice 6:** session-close's plan line, which shows each phase's planned against actual
  sessions. The choice itself is the owner's.

## Related
- AGENTS.md rules 1 and 4 (the rationale expires; a rule needs an executor). PRAC-0010, PRAC-0014,
  PRAC-0015. ADR-0016 to ADR-0022.
