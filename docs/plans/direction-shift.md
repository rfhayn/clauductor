# The direction shift of 2026-10-27 (OPS-144)

**Status:** decided by the owner on 2026-10-27, in a cloud session. The decision is ADR-0011;
`docs/roadmap.md` carries the rows and the session plan from S13. This file records why, and
which old rows became which new ones. Nothing was built to write it.

## What prompted it

The owner considered making Clauductor an open-source project rather than a paid product. Two
inputs shaped the decision.

**Andrew Baker's Ghostty Dock** (andrewbaker.ninja, 2026-10-21). It is three small, free tools:
a Ghostty build with a sidebar of Claude Code sessions, a gateway (Claude Burst), and a cost panel.
They answer three questions: *which session needs me, what is it costing, and is it about to fall
over?* What we take from it:
- Each session row shows four facts: its state (needs you, running, finished unread, idle), how
  long it has been in that state, its context size and its cost so far.
- A **polite raise**: a session that needs you comes forward only after 10 seconds without input,
  and by default only once nothing else is running. A key jumps to the next session that needs you.
- A session's colour follows it into its terminal.
- **Spending now** in dollars per hour over the last ten minutes, and "16× your average session".
- Context broken down by part (messages, tool results, tools, memory files) against the point
  where compaction starts.
- Subagents made visible: what each was asked, its model, tokens, cost and last words.

What we do not take:
- Forking a terminal. It is macOS only, needs Xcode, and means following every upstream release.
- Burst's routing between subscription and metered billing. It is the risk OPS-113's M1 exists for.
- Gateway-level compaction. That is a different product.
- Its session coordination, where the first session to edit a file "owns" it. One change per
  branch per worktree avoids the conflict instead.

**Standing Tee's operating model**, read at `rfhayn/standingtee` (`e58b085`, session 121, with
standingtee.com live). It runs the same shape as the template: worktrees under
`.claude/worktrees/`, `change/`, `fix/` and `ops/` branches, an orchestrator in the main checkout,
build-change, the gate lease and the merge guard. Most of its practices already came across as
modules (ideas, artifacts, living-visuals, review-page, people, premise-check, risk-register,
openspec, ci-status, write-surfaces), but those modules are off by default. What it has that the
template does not became Phase 4.

## The decisions (the owner, 2026-10-27)

1. **Open source, MIT.** The licensing rows are cancelled.
2. **The terminal UI is the only local face.** The browser panel is frozen, then removed
   (PANEL-64). PANEL-46's overhaul and the browser rows built on it are cancelled, not paused. A
   desktop app comes after launch (PANEL-65).
3. **The product module lives in "both" places.** Its screens are claude.ai artifacts (shareable
   and usable from a phone, proven in Standing Tee), and the terminal UI shows a read-only summary
   of the same files.
4. **The plugin is the only install path.** Lite is the default and includes the product module;
   full is opt-in. The binary is the terminal UI and its engine only. This repository moves to the
   plugin with `--full`.
5. **One version number**, for solo builders and small teams.
6. **The rename stays, with no fallback:** v0.1.0 ships only under the new name (owner-queue item
   O-R2 closes).
7. **Port everything Standing Tee has that the template lacks** (Phase 4), reversing OPS-71.
8. **S13 starts with the cost ports**, which save credits at once.

## What Standing Tee teaches, and where each lesson went

| Lesson | Standing Tee source | Row |
|---|---|---|
| A session's cost, broken down by what it re-reads, is in each journal entry | `infra/usage-report.mjs --contributors` | ST-8 |
| No whole read of a file over 400 lines | `.claude/hooks/read-by-range.sh` | ST-9 |
| Worktree sessions compact at a set ceiling | the run file, `_contextCeilingWhy` | ST-10 |
| session-start's block fits a budget | `session-start/digests.sh` | ST-11 |
| Delete the mechanism when fixes breed bugs; score a round by its worst finding | PRAC-0017 and its amendments | ST-12 |
| A classifier stop is a park signal, retried only on an agreed rule | PRAC-0044 | ST-13 |
| No forced worktree removal; no `pkill` or `killall` | `no-worktree-force-remove.sh`, `no-pattern-kill.sh`, `.claude/bin/` | ST-14 |
| The orchestrator opens worktree sessions as terminal tabs | `infra/open-worktree-session.sh` | ST-15 |
| Every session reports to whoever opened it | the report clause (#593) | ST-16 |
| A session that dies still reports | `infra/session-watch.sh`, `session-outcome-wake.sh` | ST-17 |
| The status line counts the sessions | `statusline-sessions.sh` | ST-18 |
| Session close only in the orchestrator, with checkpoint and relaunch | `session-close/where.sh`, ADR-0039's amendments | ST-19 |
| The shared pages are on and kept current | `docs/artifacts.json`, `artifact-currency.mjs` | ST-20 |
| New lessons keep arriving | (new) | ST-21 |
| The merge guard judges the argv `gh` receives | `.claude/bin/gh`, PRAC-0041 | OPS-86 weighs it |

Clauductor already has things Standing Tee lacks, and they stay: the cd guard, the catch-up guard,
no-silent-wait, each row's records in its own fragment, the review cap read from a change's Risk,
and delegated approval.

## Old rows to new

| Row | Was | Now |
|---|---|---|
| PANEL-46, PANEL-53, PANEL-55 | the browser overhaul and its parts | cancelled |
| PANEL-37, PANEL-38, PANEL-43, PANEL-30, PANEL-31, PANEL-39, OPS-62, OPS-84, OPS-94 | browser views, buttons and chips | cancelled; PRD-tui decides what returns |
| PANEL-54, OPS-112, OPS-116 | licensing | cancelled |
| ST-7, ST-6 | Standing Tee served through the browser panel | cancelled |
| o.4 | PANEL-45's outcome check | cancelled |
| PANEL-25, PANEL-33, PANEL-34, PANEL-50 | engine rows waiting on PANEL-46 | kept, no longer waiting on it |
| OPS-92b | `clauductor install --profile lite` | lite through the plugin |
| OPS-113 | the launch safeguards M1–M5 | M1 and M2 only (the subscription's terms) |
| OPS-115 | a local Mac release script | possibly a GitHub Actions release, once the repository is public |
| OPS-117 | a site with pricing and checkout | a docs site only |
| REL-1 | v0.1.0 | v0.1.0 open source under MIT, under the new name |
| OPS-104 | the rename, with a fallback | the rename, no fallback, cut before PANEL-61 |
| (new) | — | OPS-144 (this plan), OPS-145 (plugin-only install), ST-8 to ST-21, PROD-1 to PROD-5, PANEL-60 to PANEL-65 |

Phases are now:

| Phase | Name |
|---|---|
| 1 | Clauductor runs its own operating model |
| 2 | The panel |
| 3 | Measuring the model |
| 4 | Learn from Standing Tee |
| 5 | The product module |
| 6 | The terminal UI |
| 7 | Open-source release (formerly Phase 4, Release) |
| 8 | Standing Tee attaches (formerly Phase 5; history) |

## What is still open

- **The new name's status.** It gates S15 and the launch (owner queue).
- **The two PRDs** (PROD-1, PANEL-60) set the rows Phases 5 and 6 build. Until then, those rows are
  `planned` and make no design claims.
- **Q11:** which of M1–M5 OPS-113 still builds.
- **The repository goes public at REL-1**, the owner's step.
