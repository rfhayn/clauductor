# PRD: The product module: an idea carries its bet, is shaped into rows, and its actual cost and outcome roll back up to it

**Status:** Approved by the owner, 2026-10-30 (PROD-1, session 18), with the nine decisions below. Phase 5's rows
(PROD-2 to PROD-5, ST-20) are each still proposed just in time and approved on their own `design.md`. This PRD
gives direction; it does not approve a design.

## Why

The direction shift (the owner, 2026-10-27; ADR-0011, `docs/plans/direction-shift.md` decisions 3 and 4) makes
the product module part of what every project gets: lite installs through the plugin with the product module by
default, and its screens live in "both" places, claude.ai artifacts and a read-only summary in the terminal UI.
Phase 5's exit criteria in `docs/roadmap.md` say what it is for: a solo builder or a small team takes an idea to
built work inside the repository, and the idea's actual cost and outcome roll back up to it.

Most of the pieces exist already, in separate places, and none of them points at another:

| Piece | Where it lives today | What it lacks for this |
|---|---|---|
| An idea | the ideas module (P2.12b): a claude.ai page whose database is the queue, `/ideas`, rendered into `IDEAS_FILE` (default `docs/ideas.md`) by `bin/render.sh` | any field beyond `text`, `note`, `authorId`, the two times and `status` (the skill writes "No other fields"; `render.sh` refuses a status outside `not-touched`, `discussed`, `roadmap`, `parked`) |
| An estimate | a change's `**Budget:** $N` (hard or `soft`), read by `.claude/change-cost.sh` and enforced by `build-change.js` | any link to the idea it serves |
| An actual cost | `archive-change` step 2: `- <date> archived: actual cost $X of budget $N` (or `unknown (<reason>)`) in the archived `tasks.md`; rule 11 requires the line. An ops or fix row has no archive; its session's cost is on its journal fragment's Cost line (ST-8) | a reader that sums it anywhere |
| An expected and observed return | a proposal's `## How we'll know`, queued by `archive-change` step 3 as an `o.<n>` row in the roadmap's `## Outcome checks` | a place to record what the check found, against what was expected |
| A shaping step | rows written by hand; PRDs in `docs/prds/active/` | a path from an idea to them |

The ideas module is **off by default** (`MODULES=""` in `template/.claude/project.conf`) and needs the artifacts
module. So does every screen this PRD describes. ST-20 switches them on in full (decision 7).

## What it builds on, and does not replace

- **The ideas module stays the one place an idea is written** where it is on (its skill: "That database is the
  ONE place an idea is written"). The product module adds fields to an idea; it does not add a second queue.
  Where it is off (lite by default, decision 7), the idea record is a repository file, and that file is the one
  place.
- **archive-change stays the one writer of a change's actual cost and its outcome row**, and the journal fragment
  the one record of an ops or fix row's session cost. The roll-up reads what they write.
- **The roadmap grammar stays the one queue of work** (`roadmap_queue`, `lib/conf.sh`). Shaping writes rows in
  it; it does not keep its own list.
- **The existing cost readings stay the only ones** (`change-cost.sh`, and the session cost a fragment records).
  No second price table.

## Requirements

### 1. The idea record carries its bet (PROD-2)

- An idea gains four fields:
  - **investment**: Claude spend in dollars and the owner's hours;
  - **expected return**: words, plus an optional number in a unit the idea names (hours saved a week, $ a month);
    there is no one fixed unit (decision 1);
  - **confidence**: one of three named levels, `low`, `medium`, `high` (decision 2);
  - **kill condition**: what, observed, would stop it.
- They are written with Claude through `/ideate`, which asks for each and never invents one. `/ideas <text>`
  keeps working unchanged, and an idea without a bet is valid; a bet, even a cheap one, is what `/shape` requires
  (decision 5).
- `render.sh` renders and checks the new fields as it does the old: a field it cannot represent is refused,
  naming the file, never skipped. An idea with neither a bet nor rows renders byte-identically to today, so every
  committed `IDEAS_FILE` keeps passing `--check` (decision 6).
- The renderer's Standing Tee compatibility (the README's byte-for-byte claim) is either kept or its loss is
  named in the module's *deliberate differences*.

### 2. Shaping: idea to PRD to phase to rows (PROD-3)

- `/shape` takes an idea the owner has approved, and that carries a bet (decision 5), and drafts, as one PR for
  the owner: a PRD under `docs/prds/active/`, a phase in `docs/roadmap.md`, and rows in the roadmap grammar, each
  with its lane (change, fix or ops), its scope and its budget. The roadmap's parser accepts every row it writes
  (`roadmap-queue.sh --check`). It refuses an idea with no bet, and says so.
- Every row it writes names the idea it came from, in a form a script reads, so the roll-up needs no guessing.
- The idea's status moves to `roadmap` when the shaping PR merges, not when it is drafted.
- **The owner always approves a PRD; `APPROVAL_DELEGATED` never covers one** (decision 9). The rows proposed from
  it can be delegated under `APPROVAL_DELEGATED`, as today.

### 3. The ROI roll-up (PROD-4)

- For each idea: the estimated investment against the actual, and the expected return against the observed (its
  rows' outcome checks, once checked).
- **The actual counts every row the idea produced** (decision 3): a change from its `archived: actual cost` line;
  an ops or fix row from the session cost its journal fragment records; plus the shaping session and the PRD's
  own row.
- A missing or `unknown` cost is shown and counted as unknown, never read as zero: a sum that drops it is a
  smaller, plausible number. The same for a row with no outcome check yet.
- Only the files are read: archived `tasks.md`, the journal fragments, the roadmap, the idea record. No
  transcript is re-read at roll-up time.
- **It renders into `IDEAS_FILE`, beside each idea** (decision 6), and only for an idea with a bet or rows; every
  other idea stays byte-identical.
- **The kill condition is raised the way a DUE outcome check is** (decision 4): one line at session start when
  its signal is observed. It never blocks; stopping the idea stays the owner's call.

### 4. Which modules it switches on (ST-20)

- **On by default in full; offered in lite as a one-line opt-in** (decision 7): **artifacts, living-visuals,
  review-page and ideas**, as Standing Tee runs them: the shared claude.ai pages, their registry and currency,
  and a session close refused while a page is behind (the artifacts module's `guard.d/currency.sh`, which blocks a
  close PR while any artifact is not `OK`).
- ST-20 first checks whether that refusal came across whole with P2.11 (Standing Tee's
  `infra/artifact-currency.mjs` and its guard's rule 8(a)); this PRD does not assume it did.
- A project not signed in to claude.ai still works: the modules say `CANNOT CHECK — no Artifact tool` (as the
  ideas module does) and the repository's records stay the source.

### 5. Its lite footprint

- Lite (OPS-92a, OPS-92b) keeps the idea record, `/ideate`, `/shape` and the roll-up **as repository files**, with
  no claude.ai sign-in needed (decision 7); the four modules are its opt-in.
- Lite's close step writes `actual cost unknown (lite: not tracked)` (OPS-92a), so in lite a change's actual shows
  as unknown and is counted so, never as zero.
- No new process check joins lite's check set (OPS-92a's list) unless the owner adds it; the renderer's
  `ideas:queue` check is the candidate.
- How much of a shaped row's process lite runs follows the owner's separate answers on review in lite (R50, PRD
  Q2) and scenario checks in lite (R52, PRD Q5), due before OPS-92a (decision 8).
- Everything stays sh and jq, as the ideas module is (ADR-0006): no Node, no binary.

### 6. Its views (PROD-5, PANEL-63)

- **A claude.ai artifact page**, the portfolio: ideas ranked by return within each unit, then confidence and cost,
  each with its rows and actuals. Ideas with no number, or in a unit of their own, are listed apart; there is no
  exchange rate between units (decision 1). Shareable and usable from a phone; registered and kept current
  through the artifacts module like any other page.
- **The terminal UI's read-only summary** (PANEL-63, from the terminal UI's PRD, which PANEL-60 is writing):
  ideas and their rows, with estimated and actual cost, read from the same files. It writes nothing.
- Both read the repository's files; neither is a second source of the numbers.

## Out of scope

- Billing, pricing or revenue tracking: the direction shift dropped licensing (ADR-0011).
- A second idea store, or importing ideas from outside the ideas page and `/ideas` (or, in lite, the
  repository's idea record).
- Writing to the roadmap from the terminal UI.

## Decisions (the owner, 2026-10-30)

1. **Expected return** is words plus an optional number in a unit the idea names (hours saved a week, $ a month),
   not one fixed unit. PROD-5 ranks within a unit and lists unitless ideas apart; there is no exchange rate
   between units.
2. **Confidence** has three named levels: `low`, `medium`, `high`.
3. **Actual cost counts every row the idea produced** (change, ops, fix), plus the shaping session and the PRD's
   row. Ops and fix rows take their cost from the session cost their journal fragment records. A missing line is
   shown and counted as unknown, never as zero.
4. **The kill condition** is raised the way a DUE outcome check is: one line at session start when its signal is
   observed. Never a block; stopping the idea stays the owner's call.
5. **An idea needs a bet before `/shape`** (a cheap one is fine).
6. **The roll-up renders into `IDEAS_FILE` beside each idea**, only for ideas with a bet or rows. Ideas with
   neither stay byte-identical, so committed files keep passing `--check`.
7. **ST-20's four modules are on by default in full, and offered in lite** as a one-line opt-in. Lite keeps the
   idea record, `/ideate`, `/shape` and the roll-up as repository files.
8. **R50 and R52 stay separate** (owner queue, before OPS-92a); this PRD follows them.
9. **The owner always approves a PRD**; `APPROVAL_DELEGATED` never covers one. Row proposals under it can be
   delegated as today.
