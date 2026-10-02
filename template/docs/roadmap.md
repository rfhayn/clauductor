# Roadmap

The program plan: phases, their order, and the change queue. The queue tables below are the
**authority** for what is next; every script reads them through one parser,
`.claude/roadmap-queue.sh` (`sh .claude/roadmap-queue.sh --check` validates this file), or the
project's own named by `ROADMAP_PARSER` (*Plugging in your own parser*, below).

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
- **An open row in its own word**: `⬜ <word>`, one lowercase word (`⬜ planned`,
  `⬜ deferred — <trigger>`, `⬜ placeholder — <what it waits for>`). It is open, so it keeps its
  phase current and `--text` lists it under its word, but it is never NEXT, never `--queued` and
  never offered by the panel: it waits for something a queued row does not.
- **A section's start**: `**<section> started:** YYYY-MM-DD`, optionally `— <why>`, anywhere in a
  `###` section whose heading begins with `<section>` (`**Gate 2A started:** 2026-07-21` under
  `### Gate 2A — "the model is correct"`). Its rows carry the date, and `--text` shows the days
  since. One per section; a misspelled, misplaced, doubled or impossible one is an error.
- The current phase is derived: the first phase with a queued, in-flight or open row.
- **A budget**: `Budget: $N` anywhere in a row (the Scope cell, say) is the change's cost
  appetite. Its `proposal.md` repeats it, `build-change` stops past it, archive records the actual.
- **A date**: `(due YYYY-MM-DD)` in the Change cell. `archive-change` queues each change's outcome
  check that way in `## Outcome checks` below, and the queue lists it as DUE from that date.

## Plugging in your own parser

A project whose roadmap already has its own grammar keeps it, and its own parser: set
`ROADMAP_PARSER` in `.claude/project.conf` to the command (run from the repo root, the mode
appended, an explicit roadmap path after it when given; `ROOT` and `ROADMAP` are in its
environment). Every reader goes through `roadmap_queue` in `.claude/lib/conf.sh`, and
`sh .claude/roadmap-queue.sh` hands every call to it, so the skills, the panel and the checks all
follow the one key. `checks/roadmap.sh` fails a script that reads the queue any other way.

The contract a parser meets:

| Mode | Prints | Exit |
|---|---|---|
| `--text` | the current phase's open rows, for people (any layout) | 0 read; non-zero: the queue is UNKNOWN |
| `--check` | one summary line, or one `ERROR` line per line it cannot parse | 0 valid; 1 not |
| `--tsv` | one row per line, tab-separated, the 12 columns below and optionally the 13th; more are ignored | 0 read; non-zero: UNKNOWN |

`--queued [change\|fix\|ops]` is derived from `--tsv` by the helper: a parser need not implement it.

| # | Column | Values |
|---|---|---|
| 1 | line | the row's line number in the file |
| 2 | phase | the phase it belongs to, `-` outside every phase (an outcome check) |
| 3 | section | the group inside the phase (a gate, a track); may be empty |
| 4 | id | the row id, `[A-Za-z0-9._-]`, unique |
| 5 | change | the change id: `add-x`, `fix/<n>-<slug>`, `ops/<name>` |
| 6 | kind | `change`, `fix` or `ops` |
| 7 | state | `queued`, `inflight`, `open` (open, waiting in its own word), `merged` or `cancelled` |
| 8 | pr | the PR number, or empty |
| 9 | owner | who owns the row, or empty |
| 10 | summary | what a user can now do |
| 11 | budget | dollars (`40`, `12.50`), or empty |
| 12 | due | `YYYY-MM-DD`, or empty |
| 13 | started | optional: the row's section's start date, `YYYY-MM-DD`, or empty |
| 14 | status (optional) | the row's raw status text (`⬜ deferred — trigger: …`), for a parser whose statuses say more than column 7; the `people` module skips a `⬜ deferred` row for **Next**. Column 13 is the builtin parser's own. |

Every `--tsv` row is held to this table before any reader sees it: a row that breaks it makes the
queue UNKNOWN, never a shorter queue. A parser whose own words differ (a status such as
`⬜ planned` or `❌ retired`, a richer phase id) does not have to change: `ROADMAP_NORMALIZER` is a
filter command from its `--tsv` to the contract's (`awk -F'\t' -v OFS='\t' '$7 == "planned" { $7 =
"queued" } { print }'`, say).

## Phase 1 — First slice
**Owner:** <owner name>

Exit criteria: <what is true when this phase is done, observably>.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| 1.1 | `add-first-capability` — <a user can now …> | <the write surfaces, the screen or entry point> · Budget: $20 | — | ⬜ queued |
| 1.2 | `ops/gate` — the gate runs the project's real lint and tests | `scripts/ci/steps.sh` | — | ⬜ queued |

## Outcome checks

Each archived change's "How we'll know", queued for the day to look (`archive-change` step 3). A
row is outside every phase, so it never becomes the current phase; the queue lists it as DUE.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
