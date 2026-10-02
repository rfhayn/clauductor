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
docs/prds/          ← the product requirements the rows come from
```

## How a row becomes a change

- **A row is the scoping unit until its turn.** Capturing an idea for later is a ROW, not a
  change directory. A row makes no design claims, so it cannot go stale the way a design can.
- **At most one change is proposed ahead** of the one being built (`docs/principles.md`, *Propose
  just in time*).
- **Every change ships its slice**: its `tasks.md` ends with a `Slice:` line.
- **Split before proposing**: a row that names more than about three write surfaces, or more than
  one screen, is several rows.

## The grammar (the parser refuses anything else)

- `## Phase <N> — <title>` starts a phase. `### <section>` groups rows inside it.
- `**Owner:** <name>` under a phase heading names the phase's owner.
- A queue table has exactly this header: `| # | Change | Scope | Deps | Status |`.
- `#` is the row id, unique in the file, never changed. **Here it is the milestone id**
  (`PANEL-19`, `OPS-9`, `REL-1`, `ST-0`): commits and PR titles start with it (`PANEL-19:`).
- `Change` starts with the change id in backticks: a capability change (`` `panel-19-metrics` ``,
  branch `change/panel-19-metrics`), or `` `fix/<issue>-<slug>` `` / `` `ops/<name>` `` for the
  other lanes. Then a dash and what a user can now do.
- `Status` leads with one of: `⬜ queued` · `⬜ in flight (#N)` · `✅ merged (#N)` ·
  `❌ cancelled — <why>`.
- The current phase is derived: the first phase with a queued or in-flight row.
- `Budget: $N` anywhere in a row is the change's cost appetite; `(due YYYY-MM-DD)` dates a row.

Before OPS-8 this repo's branches were `feature/PREFIX-N-…`; the merged rows below keep those
names as their ids. Earlier history (M1–M7, LIFE-1) is in `docs/prds/` and the journal.

## Phase 1 — Clauductor runs its own operating model
**Owner:** Rich

Exit criteria: this repo's sessions run session-start, the gate, merge-pr and session-close as a
project does; the old lock-and-supervisor model is gone from the code; what the rehearsal found is
fixed in the template before Standing Tee's Phase 0.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| OPS-8 | `ops/ops-8-own-model` — this repo installs and runs the model it ships; ADRs 0001–0008; the no-clauductor check | `.claude/`, `AGENTS.md`, `scripts/ci/`, `docs/`, `template/.claude/checks/no-clauductor.sh` | — | ⬜ in flight (#27) |
| OPS-13 | `ops/ops-13-remove-old-model` — the CLI no longer carries the lock-and-supervisor model: no SQLite registry, HUD, claim/spawn/assign, and install stops creating `orchestration/` | `framework/internal/{state,hud}`, `framework/internal/cmd`, `docs/onboarding.md` | OPS-8 | ✅ merged (#37) |
| OPS-14 | `ops/ops-14-rehearsal-fixes` — the OPS-8 rehearsal findings too big to fix inline (journal Session 1): old-model detection on a fresh clone, stale old-model files after install, the ambient template path, update adding new doc-tier files, settings and branch prefixes a project changes living in framework files | `framework/internal/cmd/{ownguard,install,update}.go`, `template/.claude/workflows/build-change.js` | OPS-8 | ⬜ in flight (#36) |
| OPS-24 | `ops/ops-24-design-docs-lanes` — a project can run `design/` and `docs/` lanes: `BRANCH_DESIGN` and `BRANCH_DOCS` keys, `checks/branch-prefixes.sh` accepting their panel lanes, the `--tsv` kind for their rows, and a model-roles lanes row (Standing Tee's swap, ST-3.3 and ST-3.12, needs it) | `template/.claude/{project.conf,lib/conf.sh,checks/branch-prefixes.sh,model-roles.json}`, `framework/internal/panel` | — | ⬜ queued |
| OPS-25 | `ops/ops-25-reviewer-hand-back` — a worker that spawns a reviewer gets its result: today a `reviewer` has no SendMessage, so its hand-back reaches the top session and the worker waits silently (Standing Tee #425 lost about 4h); build-change, apply-change and merge-pr route it, or the worker reads it back | `template/.claude/{skills/apply-change,skills/merge-pr,workflows/build-change.js,agents/reviewer.md}` | — | ⬜ queued |

## Phase 2 — The panel
**Owner:** Rich

Exit criteria: the panel serves every repository of the owner's from one process, and shows the
flow, cost and quality metrics the model computes.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| PANEL-14 | `feature/PANEL-14-terminal-links-selection` — open terminal links on ⌘-click, and keep a drag's selection | `framework/internal/panel/web` | — | ⬜ in flight (#16) |
| PANEL-15 | `feature/PANEL-15-plan-aware-quota` — plan-aware quota, a neutral tmux server argv, and image drop | `framework/internal/panel` | — | ⬜ in flight (#17) |
| PANEL-16 | `feature/PANEL-16-multi-repo` — one panel for every repository, with a project menu | `framework/internal/panel` | — | ⬜ in flight (#18) |
| PANEL-17 | `feature/PANEL-17-close-lane-and-help` — close a lane with its worktree and branch, and Help | `framework/internal/panel` | — | ⬜ in flight (#20) |
| PANEL-18 | `feature/PANEL-18-lane-actions-and-cleanup` — lane actions from the lists, leftover worktrees removed, init detects the model | `framework/internal/panel` | — | ⬜ in flight (#21) |
| PANEL-19 | `panel-19-metrics` — a Metrics view (Flow, Cost, Quality, Outcomes), a Flow card, budget and economy badges | `framework/internal/panel` (server + web) | OPS-9 | ⬜ queued |
| PANEL-20 | `panel-20-lane-lifecycle` — lanes auto-archive on merge, show merge readiness, auto-resume at the quota reset, and run per-lane setup and teardown | `framework/internal/panel` | PANEL-17 | ⬜ queued |
| PANEL-21 | `panel-21-binding-corrections` — a session's events bind to the right lane after a worktree moves, and session ends close their lane state | `framework/internal/panel/state` | — | ⬜ queued |
| OPS-15 | `ops/ops-15-clauductor-in-panel` — the owner adds this repo as the panel's second project (`panel trust`, `panel add`) | `.clauductor/panel.json` | PANEL-16 | ⬜ queued |
| PANEL-25 | `panel-25-idle-github` — a panel with no page in view stops calling GitHub every minute per project (`gh pr list`), and a lane still auto-closes when its PR merges | `framework/internal/panel` | — | ⬜ queued |
| PANEL-26 | `panel-26-idle-backoff` — a panel with no lane and no page in view backs its `claude agents`, `git worktree list` and tmux polls off to minutes, and a hook or a lane start still wakes it at once | `framework/internal/panel` | — | ⬜ queued |
| PANEL-27 | `panel-27-install-no-open` — `panel install --no-open` keeps the login agent from opening a browser tab at every login | `framework/internal/panel/install`, `framework/internal/cmd` | — | ⬜ queued |
| PANEL-28 | `panel-28-login-refresh` — the panel's own `claude` calls (`auth status` at start, `agents`) never race a session's login refresh ("another Claude Code process is refreshing it"), or the row records that they do not | `framework/internal/panel` | — | ⬜ queued |

## Phase 3 — Measuring the model
**Owner:** Rich

Exit criteria: flow and DORA metrics, and the reviewer agent's recall, are computed from the repo
and reach the owner through session-start and the panel.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| OPS-9 | `ops/ops-9-metrics` — `.claude/metrics.sh`: lead and cycle time, approval wait, review rounds, aging WIP, merge frequency, change-fail rate; a health line and a card | `template/.claude/metrics.sh`, `template/.claude/health/` | — | ✅ merged (#26) |
| OPS-10 | `ops/ops-10-evals` — a seeded-defect suite measures the reviewer's recall; the merge guard wants an eval receipt for agent, workflow and role changes | `template/.claude/evals/`, `template/.claude/hooks/pr-merge-guard.sh` | — | ✅ merged (#28) |
| OPS-16 | `ops/OPS-16-narrow-rule-13` — rule 13 fires only on what makes the reviewer what it is (its model, its agent, the marked review prompt), declared per role in `evals.triggers`; the reviewer's opus/high baseline measured once | `template/.claude/{lib/evals.sh,evals/run.sh,hooks/}`, `.claude/model-roles.json` | OPS-10 | ⬜ in flight (#40) |
| OPS-23 | `ops/ops-23-freeze-role-tables` — `build-change.js` deep-freezes ROLES, TIERS and ECONOMY, so no step can change a role's model or effort mid-run; it lands in the next PR that runs a reviewer eval anyway (the file is a rule-13 input) | `template/.claude/workflows/build-change.js` | — | ⬜ queued |
| OPS-18 | `ops/OPS-18-clean-controls` — fix the two reviewer clean controls that hold real defects (`sh-clean-coverage-fails-closed`: a non-numeric floor passes; `py-clean-token-expiry`: verify() raises on a non-ASCII mac or a Unicode digit), then #40 re-runs the opus/high eval on its rebased head and records the receipt (rule 13 blocks #40 until a receipt passes) | `template/.claude/evals/reviewer/cases/` | — | ⬜ in flight (#40) |

## Phase 4 — Release
**Owner:** Rich

Exit criteria: a stranger installs clauductor from a release download or Homebrew, not a source
build.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| REL-1 | `feature/REL-1-release` — v0.1.0: public README, changelog, release builds and install paths | `README.md`, `CHANGELOG.md`, `.github/workflows/release*.yml`, `install.sh` | — | ⬜ in flight (#25) |
| REL-2 | `ops/rel-2-install-kit` — what REL-1 (#25) leaves out: release builds cross-compile without cgo (SQLite left with OPS-13), install prints a prerequisites report, and a project starts with a `.gitleaks.toml` | `scripts/release/`, `.github/workflows/release*.yml`, `install.sh`, `template/.gitleaks.toml` | REL-1 | ⬜ queued |

## Phase 5 — Standing Tee converges on the template
**Owner:** Rich

Exit criteria: `clauductor update` keeps Standing Tee current, and its own parts live in config,
modules and the local layer (PRD-change-process, "Adopting it in Standing Tee"). The work lands in
Standing Tee's repo; the clauductor side of each phase is a row here.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| ST-0 | `ops/st-0-convergence-map` — the convergence map: every shared file classified, every record format decided, the onboarding sections listed (read-only) | Standing Tee docs | OPS-8 | ⬜ queued |
| ST-1 | `ops/st-1-extension-points` — optional modules and a project-local layer (the settings.json merge and `clauductor diff` landed in #29) | `framework/internal/cmd`, `template/.claude/modules/` | ST-0 | ⬜ queued |
| ST-2 | `ops/st-2-upstream` — what the map marks generic and better in Standing Tee moves into the template | `template/` | ST-0 | ⬜ queued |
| ST-3 | `ops/st-3-converge` — Standing Tee converges file by file, with the no-clauductor check in its gate | Standing Tee | ST-1, ST-2, OPS-24 | ⬜ queued |
| ST-4 | `ops/st-4-handover` — `.claude/clauductor-template` written; `clauductor update` from then on | Standing Tee | ST-3 | ⬜ queued |
| ST-5 | `ops/st-5-swap-plan` — the swap plan: Standing Tee's convergence as an ordered list of PRs, each with its owner decisions, its checks and its rollback | Standing Tee docs | ST-0 | ⬜ queued |
| ST-6 | `ops/st-6-designer-onboarding` — the designer's onboarding PR: Standing Tee's designer onboarding and welcome page describe the converged model (the panel, who decides), so Damian works from them | Standing Tee docs | ST-3 | ⬜ queued |

### Phase 2 upstream follow-ups (the convergence map's P2 rows not yet owned)

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| P2.15b | `ops/roadmap-boundary-tasks` — the roadmap parser reads Standing Tee's `**Gate X boundary-task status:**` lines, and `--text` warns on an open boundary task (left over from P2.15, #45) | `template/.claude/roadmap-queue.sh`, `template/.claude/checks/roadmap.sh` | P2.15 (#45) | ⬜ queued |
| P2.11 | `ops/module-living-visuals` — derived pages kept current: a registry `refresh:` field, a close step that refreshes or stamps every page that is behind, and a generated-block check (Phase 2 wave 2) | `template/.claude/modules/living-visuals/` | P2.7 (#43) | ⬜ queued |
| P2.12b | `ops/module-ideas` — the ideas queue as a module: the skill, a session-start count, the close-time render, and a check that the rendered page is never hand-edited (Phase 2 wave 2; the risk register half of P2.12 is #41) | `template/.claude/modules/ideas/` | P2.7 (#43) | ⬜ queued |

## Done before the model

Merged before this repo ran its own roadmap; listed so the ids stay taken.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| PANEL-1 | `feature/PANEL-1-web-panel-v0` — the web panel, v0 | `framework/internal/panel` | — | ✅ merged (#2) |
| PANEL-2 | `feature/PANEL-2-terminals-and-lanes` — terminals and lanes | `framework/internal/panel` | — | ✅ merged (#4) |
| PANEL-3 | `feature/PANEL-3-orchestration` — orchestration | `framework/internal/panel` | — | ✅ merged (#5) |
| PANEL-4 | `feature/PANEL-4-themes` — themes | `framework/internal/panel/web` | — | ✅ merged (#6) |
| PANEL-5 | `feature/PANEL-5-state-correctness` — state correctness | `framework/internal/panel/state` | — | ✅ merged (#7) |
| PANEL-6 | `feature/PANEL-6-ui-refinement` — UI refinement | `framework/internal/panel/web` | — | ✅ merged (#8) |
| PANEL-7 | `feature/PANEL-7-cost` — cost | `framework/internal/panel` | — | ✅ merged (#9) |
| PANEL-8 | `feature/PANEL-8-structure` — structure | `framework/internal/panel` | — | ✅ merged (#10) |
| PANEL-9 | `feature/PANEL-9-tests` — deterministic tests, CI, the lock-run signal fix | `framework/` | — | ✅ merged (#11) |
| PANEL-10 | `feature/PANEL-10-adoption` — adoption by a second project | `framework/internal/panel` | — | ✅ merged (#12) |
| PANEL-11 | `feature/PANEL-11-lane-layout` — the lane-centric layout | `framework/internal/panel/web` | — | ✅ merged (#13) |
| PANEL-12 | `feature/PANEL-12-up-next-and-pinned-cards` — Up next, pinned cards | `framework/internal/panel` | — | ✅ merged (#14) |
| PANEL-13 | `feature/PANEL-13-self-verify` — re-verify a new Claude Code from live hooks | `framework/internal/panel` | — | ✅ merged (#15) |
| OPS-1 | `feature/OPS-1-generic-operating-model` — a generic operating model in the template, with an install guard | `template/` | — | ✅ merged (#19) |
| OPS-7 | `feature/OPS-7-change-process` — the change process: OpenSpec-compatible records, scenario tracing | `template/` | — | ✅ merged (#22) |
| OPS-12 | `feature/OPS-12-no-clauductor-invariant` — plan the no-clauductor invariant | `docs/prds/` | — | ✅ merged (#23) |
| OPS-11 | `feature/OPS-11-plugin` — the operating model as a Claude Code plugin | `plugin/`, `framework/internal/plugin` | — | ✅ merged (#24) |

## Outcome checks

Each archived change's "How we'll know", queued for the day to look (`archive-change` step 3).

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
