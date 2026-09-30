# Roadmap

The program plan: phases, their order, and the change queue. The queue tables below are the
**authority** for what is next; every script reads them through one parser,
`.claude/roadmap-queue.sh` (`sh .claude/roadmap-queue.sh --check` validates this file).

## How this layers with the other records

```
docs/roadmap.md     ← THIS: phases, order, exit criteria, and the queue of rows
changes/<id>/       ← the ONE unit of work in flight: proposal, design, tasks (proposed just in time)
specs/              ← what the system does, per capability (promoted from a change when it merges)
docs/adr/           ← decisions with trade-offs, alongside
```

A phase is several changes. Decompose as you go, phase by phase.

## How a row becomes a change

- **A row is the scoping unit until its turn.** Capturing an idea for later is a ROW, not a
  change directory. A row makes no design claims, so it cannot go stale the way a design can.
- **At most one change is proposed ahead** of the one being built (`docs/principles.md`, *Propose
  just in time*). There is no "proposed" status: a change is proposed when it is next.
- **Every change ships its slice**: its `tasks.md` ends with a `Slice:` line.
- **Split before proposing**, not after a review says so: a row that names more than about three
  write surfaces, or more than one screen, is several rows.

## The grammar (the parser refuses anything else)

- `## Phase <N> — <title>` starts a phase. `### <section>` groups rows inside it (a gate, a track).
- `**Owner:** <name>` under a phase heading names the phase's owner; under a `###` heading, that
  section's owner, which overrides the phase's for its rows.
- A queue table has exactly this header: `| # | Change | Scope | Deps | Status |`.
- `#` is a row id, unique in the file, and it never changes (code may cite it).
- `Change` starts with the change id in backticks: `` `add-thing` `` for a capability change
  (branch `change/add-thing`), `` `fix/<issue>-<slug>` `` or `` `ops/<name>` `` for the other
  lanes. Then a dash and what a user can now do.
- `Status` leads with one of: `⬜ queued` · `⬜ in flight (#N)` · `✅ merged (#N)` ·
  `❌ cancelled — <why>`. Text may follow. `session-close` moves a row to in flight when its PR
  opens and to merged when it merges.
- The current phase is derived: the first phase with a queued or in-flight row.

## Phase 1 — First slice
**Owner:** <owner name>

Exit criteria: <what is true when this phase is done, observably>.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| 1.1 | `add-first-capability` — <a user can now …> | <the write surfaces, the screen or entry point> | — | ⬜ queued |
| 1.2 | `ops/gate` — the gate runs the project's real lint and tests | `scripts/ci/steps.sh` | — | ⬜ queued |
