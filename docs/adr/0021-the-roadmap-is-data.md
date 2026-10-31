# ADR 0021: The roadmap is data that the engine owns; the markdown and the owner's doc are views of it

- **Status**: Accepted (the owner, 2026-10-31)
- **Date**: 2026-10-31
- **Tags**: roadmap, records, mcp, panel
- **Source**: S19. The owner asked for the roadmap to be redesigned as part of the re-architecture,
  with Claude Docs as its display in place of the Artifact tracker (the owner, 2026-10-31). The
  S19 classification, section E, mapped every reader and writer.

## Context
`docs/roadmap.md` is a markdown table read by two awk grammars: `roadmap-queue.sh` behind
`roadmap_queue`, and `roadmap-plan.sh` for the Session plan.
- **Readers:** about ten scripts read it, including `panel-suggest.sh`, `review-cap.sh`, budgets,
  metrics, `marker.sh`, team-queue `--next-id` and the tracker. The Go panel never parses it; it
  only runs those scripts.
- **Writers:** the writes are text edits of Status cells, by `fold.sh`, session-close and
  archive-change.
- **Why it is fragile:** most of `roadmap-queue.sh`'s error handling exists because the input is
  free text. That covers near-miss headers, misspelt states, byte against glyph widths and the
  16-column TSV contract. Facts also hide in prose: `Risk:`, `Budget:`, `(due …)` and
  `(team-queue #N)` live inside the Change cells.
- **The owner's dashboard** re-derives the states by itself.

## Decision
1. **Rows are files.** Each row is `docs/roadmap/rows/<ID>.json`, with typed fields: `id`, `phase`,
   `section`, `lane` (`change`, `fix` or `ops`), `name`, `what`, `scope[]`, `deps[]`, `risk`,
   `budget`, `due`, `status` (`queued`, `in-flight`, `merged` or `cancelled`), `pr`, `why`, and
   `sources[]`. The phases and sections are `docs/roadmap/phases.json`, and the Session plan is
   `docs/roadmap/plan.json`. Every file is plain JSON that `jq` can read.
2. **The engine owns the schema and the transitions.** A row's status changes only through an
   action (`roadmap.set_status`, `roadmap.add_row`), which checks the transition (`queued` to
   `in-flight` to `merged`) and its evidence (a PR number, a merge commit). Ids come from an engine
   counter that also sees open PRs. Every action has a CLI twin.
3. **Readers query, they never parse.** The queries are `roadmap.rows`, `roadmap.plan`,
   `roadmap.next` and `roadmap.validate`, plus `lanes.candidates` for the panel. Each answers from
   `origin/<main>` unless it is asked about the working tree. The hard limits on what may be
   offered next are filters inside these queries. The ranking among the allowed rows is judgment
   (ADR-0016).
4. **`docs/roadmap.md` is rendered and read-only.** The engine renders it from the data. A check
   fails when the render differs from the data, so an edit of the render is caught. The rendering
   keeps today's layout, so a reader on GitHub sees what they see today.
5. **Extensions keep a TSV export.** `stridal roadmap rows --tsv` emits today's 16 fields, so
   `roadmap.d/*.sh` (the owners rule, the local history rule) keep working until they are ported.
6. **The owner's display is a Claude Docs doc.** The roadmap-tracker step (local layer) refreshes it
   at session start and close from `roadmap.rows` and `roadmap.plan`. It shows a drawn timeline of
   the sessions, the rows table and the owner's decisions. Its address stays in
   `<git common dir>/stridal/tracker-url`, out of the public repository, as the tracker's does
   today. Panel lanes run with `ENABLE_CLAUDEAI_MCP_SERVERS=false` (`docs/panel.md`), so only the
   main session refreshes the doc; a lane's close says "doc not refreshed" and leaves it to the
   main session. The Artifact tracker retires.

## Consequences
### Positive
- `roadmap-queue.sh`'s free-text error handling, `ROADMAP_PARSER`, and `fold.sh`'s "Status cell not
  found" failure all go away.
- One file per row means two PRs that touch different rows no longer conflict on one table.
- Risk, budget, due date and the team-queue link become fields, not prose to grep.

### Negative / trade-offs
- A one-time migration of every row, including the prose facts hidden in Change cells. Rows that
  merged before the 2026-10-27 reset stay in `docs/roadmap-history.md` as text.
- The fixtures of `checks/roadmap.sh` and `checks/panel-suggest.sh` are rewritten.
- Merge-guard rules 7 and 8 ("lands on top of `origin/main`") must cover a directory, not one file.
- The panel's Change-queue card watches the data directory, not `docs/roadmap.md`.
- Editing a row by hand means editing JSON, or calling the action. The render is no longer the
  place to type.

## Enforcement
- **The schema and the transitions:** engine tests, and `roadmap.validate` run by `checks/roadmap.sh`.
  Built in OPS-195.
- **The render matches the data:** a check in the gate. OPS-195.
- **No writer edits a row file except through an action:** the rewrite guard's records rule
  (OPS-68), extended to `docs/roadmap/`. Until OPS-68 lands, this is held by review: **Nothing. You.**
- **The owner's doc is current:** the tracker step's STALE line in session-start's context, as today.

## Rests on
- Every reader and writer of the roadmap is known: the S19 classification (section E) and the
  round 3 review (team-queue `--next-id`, `roadmap-tracker.sh`). A reader found later is added to
  OPS-195 before it merges; `checks/roadmap.sh` fails a reader that bypasses the engine.
- The Claude Docs tools are reachable from the main session: **passed** (S19 built the doc).

## Related
- ADR-0016, ADR-0017. Replaces the roadmap's grammar section and `roadmap-queue.sh`. OPS-68.
