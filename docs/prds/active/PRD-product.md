# PRD: The product module: an idea carries its bet, is shaped into rows, and its actual cost and outcome roll back up to it

**Status:** awaiting the owner's approval (PROD-1, session 18). Phase 5's rows (PROD-2 to PROD-5, ST-20) stay
unproposed until the owner approves this PRD; each capability row is then proposed just in time and approved on
its own `design.md`. This PRD gives direction; it does not approve a design. The questions under *Open* are the
owner's; nothing here decides them.

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
| An actual cost | `archive-change` step 2: `- <date> archived: actual cost $X of budget $N` (or `unknown (<reason>)`) in the archived `tasks.md`; rule 11 requires the line | a reader that sums it anywhere |
| An expected and observed return | a proposal's `## How we'll know`, queued by `archive-change` step 3 as an `o.<n>` row in the roadmap's `## Outcome checks` | a place to record what the check found, against what was expected |
| A shaping step | rows written by hand; PRDs in `docs/prds/active/` | a path from an idea to them |

The ideas module is **off by default** (`MODULES=""` in `template/.claude/project.conf`) and needs the artifacts
module. So does every screen this PRD describes. ST-20 is what switches them on.

## What it builds on, and does not replace

- **The ideas module stays the one place an idea is written** (its skill: "That database is the ONE place an
  idea is written"). The product module adds fields to an idea; it does not add a second queue.
- **archive-change stays the one writer of actual cost and the outcome row.** The roll-up reads what it writes.
- **The roadmap grammar stays the one queue of work** (`roadmap_queue`, `lib/conf.sh`). Shaping writes rows in
  it; it does not keep its own list.
- **`change-cost.sh` stays the one cost reading.** No second price table.

## Requirements

### 1. The idea record carries its bet (PROD-2)

- An idea gains four fields: **investment** (Claude spend in dollars and the owner's hours), **expected return**
  (in words, with a number where one exists), **confidence**, and a **kill condition** (what, observed, would stop
  it).
- They are written with Claude through `/ideate`, which asks for each and never invents one; `/ideas <text>`
  keeps working unchanged, and an idea without a bet is valid (a bet is what moves it past `discussed`).
- `render.sh` renders and checks the new fields as it does the old: a field it cannot represent is refused,
  naming the file, never skipped. A renderer change that changes the body of an idea with no bet would break
  `--check` on every committed `IDEAS_FILE`, so it is a change to avoid, or to make with a migration.
- The renderer's Standing Tee compatibility (the README's byte-for-byte claim) is either kept or its loss is
  named in the module's *deliberate differences*.

### 2. Shaping: idea to PRD to phase to rows (PROD-3)

- `/shape` takes an idea the owner has approved and drafts, as one PR for the owner: a PRD under
  `docs/prds/active/`, a phase in `docs/roadmap.md`, and rows in the roadmap grammar, each with its lane
  (change, fix or ops), its scope and its budget. The roadmap's parser accepts every row it writes
  (`roadmap-queue.sh --check`).
- Every row it writes names the idea it came from, in a form a script reads, so the roll-up needs no guessing.
- The idea's status moves to `roadmap` when the shaping PR merges, not when it is drafted.
- Shaping proposes; the owner approves the PRD, as today. Whether delegated approval (`APPROVAL_DELEGATED`)
  may extend to a shaped PRD is open question 9.

### 3. The ROI roll-up (PROD-4)

- For each idea: the estimated investment against the actual (the sum of its rows' `archived: actual cost`
  lines), and the expected return against the observed (its rows' outcome checks, once checked).
- An `unknown` cost is shown as unknown and counted, never read as zero: a sum that drops it is a smaller,
  plausible number. The same for a row with no outcome check yet.
- Only the files are read: archived `tasks.md`, the roadmap, the idea record. No transcript is re-read at
  roll-up time; `change-cost.sh` already did that at archive.

### 4. Which modules it switches on (ST-20)

- On by default, in lite and in full: **artifacts, living-visuals, review-page and ideas**, as Standing Tee runs
  them: the shared claude.ai pages, their registry and currency, and a session close refused while a page is
  behind (the artifacts module's `guard.d/currency.sh`, which blocks a close PR while any artifact is not `OK`).
- ST-20 first checks whether that refusal came across whole with P2.11 (Standing Tee's
  `infra/artifact-currency.mjs` and its guard's rule 8(a)); this PRD does not assume it did.
- A project not signed in to claude.ai still works: the modules say `CANNOT CHECK — no Artifact tool` (as the
  ideas module does) and the repository's records stay the source.

### 5. Its lite footprint

- Lite (OPS-92a, OPS-92b) gets the idea record, `/ideate`, `/shape` and the roll-up. Lite's close step writes
  `actual cost unknown (lite: not tracked)` (OPS-92a), so in lite the roll-up shows investment as estimated
  only, and says so; it never reads lite's `unknown` as zero.
- No new process check joins lite's check set (OPS-92a's list) unless the owner adds it; the renderer's
  `ideas:queue` check is the candidate.
- Everything stays sh and jq, as the ideas module is (ADR-0006): no Node, no binary.

### 6. Its views (PROD-5, PANEL-63)

- **A claude.ai artifact page**, the portfolio: ideas ranked by return, confidence and cost, each with its rows
  and actuals. Shareable and usable from a phone; registered and kept current through the artifacts module like
  any other page.
- **The terminal UI's read-only summary** (PANEL-63, from the terminal UI's PRD, which PANEL-60 is writing): ideas and their rows, with estimated and
  actual cost, read from the same files. It writes nothing.
- Both read the repository's files; neither is a second source of the numbers.

## Out of scope

- Billing, pricing or revenue tracking: the direction shift dropped licensing (ADR-0011).
- A second idea store, or importing ideas from outside the ideas page and `/ideas`.
- Writing to the roadmap from the terminal UI.

## Open (the owner's)

1. **The return's unit.** Is expected return free text, a number in a unit the idea names, or one fixed unit
   (dollars, hours saved)? The ranking in PROD-5 depends on it.
2. **Confidence's scale.** A percentage, or a few named levels?
3. **What counts toward an idea's actual cost.** `archive-change` records cost only for a change (`changes/<id>/`);
   ops and fix rows, the PRD's own row and the shaping session have no archived cost line. Count only changes,
   or add a cost line for the others (and where)?
4. **The kill condition.** Is it only recorded, or does a session raise it (as a DUE outcome check is raised)
   when its signal is observed?
5. **Whether an idea needs a bet before `/shape`.** Requirement 2 assumes yes.
6. **The roll-up's home.** Rendered into `IDEAS_FILE` beside each idea, or a file of its own?
7. **ST-20's defaults in lite.** Four modules on by default makes a claude.ai sign-in part of lite's normal
   path. On in both, as the roadmap says, or on in full and offered in lite?
8. **The lite questions still open** (owner queue, 2026-10-28): review in lite (R50, PRD Q2) and scenario checks
   in lite (R52, PRD Q5) decide how much of a shaped row's process lite runs; this PRD follows whatever they
   decide.
9. **Delegated approval and a shaped PRD.** May `APPROVAL_DELEGATED` cover a PRD that `/shape` drafts, or does
   the owner always approve it, as this PRD proposes?
