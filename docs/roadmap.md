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
| OPS-8 | `ops/ops-8-own-model` — this repo installs and runs the model it ships; ADRs 0001–0008; the no-clauductor check | `.claude/`, `AGENTS.md`, `scripts/ci/`, `docs/`, `template/.claude/checks/no-clauductor.sh` | — | ✅ merged (#27) |
| OPS-13 | `ops/ops-13-remove-old-model` — the CLI no longer carries the lock-and-supervisor model: no SQLite registry, HUD, claim/spawn/assign, and install stops creating `orchestration/` | `framework/internal/{state,hud}`, `framework/internal/cmd`, `docs/onboarding.md` | OPS-8 | ✅ merged (#37) |
| OPS-26 | `ops/ops-26-ci-linux-prs` — a pull request's CI runs Ubuntu only and a newer push cancels the running suite; `main` still runs macOS and Ubuntu (the local gate covers macOS per PR) | `.github/workflows/test.yml`, `CLAUDE.md`, `docs/panel.md`, `scripts/ci/steps.sh` | — | ✅ merged (#58) |
| OPS-14 | `ops/ops-14-rehearsal-fixes` — the OPS-8 rehearsal findings too big to fix inline (journal Session 1): old-model detection on a fresh clone, stale old-model files after install, the ambient template path, update adding new doc-tier files, settings and branch prefixes a project changes living in framework files | `framework/internal/cmd/{ownguard,install,update}.go`, `template/.claude/workflows/build-change.js` | OPS-8 | ✅ merged (#36) |
| OPS-27 | `ops/train-2026-10-02` — the 2026-10-02 merge train: #59, #44, #46, #41, #42, #49, #55, #56, #47 and #32 landed as one squash, with their conflict resolutions reviewed on their own | `docs/roadmap.md`, the included PRs' files | — | ✅ merged (#60) |
| OPS-30 | `ops/train-2-2026-10-02` — the second 2026-10-02 merge train: #54 and #57 landed as one squash, with their conflict resolutions reviewed on their own | `docs/roadmap.md`, the included PRs' files | OPS-27 | ✅ merged (#61) |

## Phase 2 — The panel
**Owner:** Rich

Exit criteria: the panel serves every repository of the owner's from one process, and shows the
flow, cost and quality metrics the model computes.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| PANEL-14 | `feature/PANEL-14-terminal-links-selection` — open terminal links on ⌘-click, and keep a drag's selection | `framework/internal/panel/web` | — | ✅ merged (#34) |
| PANEL-15 | `feature/PANEL-15-plan-aware-quota` — plan-aware quota, a neutral tmux server argv, and image drop | `framework/internal/panel` | — | ✅ merged (#34) |
| PANEL-16 | `feature/PANEL-16-multi-repo` — one panel for every repository, with a project menu | `framework/internal/panel` | — | ✅ merged (#34) |
| PANEL-17 | `feature/PANEL-17-close-lane-and-help` — close a lane with its worktree and branch, and Help | `framework/internal/panel` | — | ✅ merged (#34) |
| PANEL-18 | `feature/PANEL-18-lane-actions-and-cleanup` — lane actions from the lists, leftover worktrees removed, init detects the model | `framework/internal/panel` | — | ✅ merged (#34) |
| PANEL-19 | `panel-19-metrics` — a Metrics view (Flow, Cost, Quality, Outcomes), a Flow card, budget and economy badges | `framework/internal/panel` (server + web) | OPS-9 | ✅ merged (#34) |
| PANEL-20 | `panel-20-lane-lifecycle` — lanes auto-archive on merge, show merge readiness, auto-resume at the quota reset, and run per-lane setup and teardown | `framework/internal/panel` | PANEL-17 | ✅ merged (#34) |
| PANEL-21 | `panel-21-binding-corrections` — a session's events bind to the right lane after a worktree moves, and session ends close their lane state | `framework/internal/panel/state` | — | ✅ merged (#34) |
| OPS-15 | `ops/ops-15-clauductor-in-panel` — the owner adds this repo as the panel's second project (`panel trust`, `panel add`) | `.clauductor/panel.json` | PANEL-16 | ❌ cancelled — no PR needed: the owner added this repo by hand on 2026-10-01 (`~/.clauductor/panel/projects.json`) |
| OPS-33 | `fix/78-flaky-tests` — `main`'s CI is green every run: the three flaky tests are fixed at their cause, not retried (#78: `TestRegistryIsWrittenBeforeTheAction` on Ubuntu, `TestLaunchdStartWaitsForTheRunningPanel` on macOS; #62: the metrics check dropping an unmerged branch from aging WIP under a full gate) | `framework/internal/panel`, `framework/internal/panel/lanes`, `framework/internal/panel/install`, `template/.claude/checks` | — | ⬜ queued |
| PANEL-29 | `panel-29-lane-dialog` — New lane knows the work: "New lane here" on a change's worktree pre-selects that change's template, an Up next change whose branch or worktree already exists starts in it instead of failing on `git worktree add`, the dialog warns before a build of a change with no Approved line before Start, and a plain "just a Claude session" option is offered (#67, gaps 2–4) | `framework/internal/panel/web`, `framework/internal/panel/lanes` | — | ⬜ queued — proposed (#72) |
| PANEL-28 | `panel-28-login-refresh` — the panel's own `claude` calls (`agents`, `auth status`, `--version`) never strand or crowd a session's login refresh ("another Claude Code process is refreshing it"): they run one at a time, pause while a refresh holds the lock, are stopped with SIGTERM rather than killed, and `auth status` leaves its timer; a stuck, stranded or future-dated lock shows in the panel. Whether the calls take part in a refresh at all is PANEL-33's to find out | `framework/internal/panel` | — | ⬜ queued — proposed (#76) |
| PANEL-30 | `panel-30-remove-worktree-guidance` — Remove worktree says in plain words what goes (the folder) and what stays (the branch and every commit), and warns before removing the worktree of a change in flight (#67, gap 1) | `framework/internal/panel/lanes`, `framework/internal/panel/web` | PANEL-29 (the way back that names a template only) | ⬜ queued — proposed (#74) |
| PANEL-31 | `panel-31-version-check` — a new Claude Code version shows as a quiet status-bar field with its progress rather than a page-wide alert, a version verified in any project clears it in every project and is kept, and Help says what is approximate meanwhile; a real break or an unreadable version still alerts (#68) | `framework/internal/panel`, `framework/internal/panel/state`, `framework/internal/panel/web` | — | ⬜ queued — proposed (#75) |
| PANEL-25 | `panel-25-idle-cost` — an idle panel costs little: with no page in view it stops calling GitHub every minute per project (`gh pr list`) while a lane still auto-closes when its PR merges; with no lane and no page in view it backs its `claude agents`, `git worktree list` and tmux polls off to minutes, and a hook or a lane start still wakes it at once; `panel install --no-open` keeps the login agent from opening a browser tab at every login (folds in PANEL-26 and PANEL-27) | `framework/internal/panel`, `framework/internal/panel/install`, `framework/internal/cmd` | PANEL-28 | ⬜ queued — proposed (#73) |
| PANEL-34 | `panel-34-close-lane` — a session closes its own lane: a `close-lane` skill shows Close's plan (what goes, what stays) and asks the panel to close the lane once the turn ends, through a `clauductor lane close` the panel authenticates (a session cannot call the page's API today); the worktree goes only when clean and merged, as with the page's Close; with no panel the skill says to close it from the panel | `framework/internal/cmd`, `framework/internal/panel`, `template/.claude/skills/close-lane` | — | ⬜ queued |
| OPS-31 | `ops-31-curtain` — `/curtain` wraps a lane up in one command: merge-pr, session-close, close-lane, then the exit, stopping at the first step that stops; the `curtain-mod` mod plays it in the terminal (Clawd sweeps the stage while the curtain falls, paced by each step's learned duration, halting at an intermission when a step stops) and sends the `/exit` a skill cannot; `/curtain-mod-demo` plays it with simulated steps. A prototype is on `ops/curtain-mod` | `template/.claude/skills/curtain`, a mod in the plugin | PANEL-34 | ⬜ queued |
| PANEL-32 | `panel-32-verify-now` — Verify now: the panel runs a throwaway session to confirm a new Claude Code version on demand, at a stated cost (#68, split from PANEL-31). From PANEL-31's review (#75): guard on the refresh lock's freshness (touched in the last 60 s, `$CLAUDE_CONFIG_DIR` before `~/.claude`), not its existence; run through PANEL-28's `claude` gate (one stop policy, one slot); the owner approves flag invariants (only Agent executes, nothing waits on a prompt, no settings but the passed hooks), with `.claude/evals/run.sh` as prior art; synchronous probe hooks, so they post before exit; the feasibility spike runs before approval; name whose quota guard applies (the default project's thresholds, as `pollQuotaAlert` reads them); remove leftover probe folders at start and at shutdown; make the 90 s and 10 s limits injectable for tests; and revisit "Accept this version" (PANEL-31 D4) if the check proves too slow to clear. From PANEL-28's review: state the cost of the probe holding PANEL-28's one-call slot, or propose an exception; while it holds it (~90 s plus a 15 s grace) the `claude agents` poll pauses, and every lane action that needs `claude agents` (the first-prompt check, removeVerdict, liveSessions) fails at its 10 s timeout | `framework/internal/panel`, `framework/internal/panel/web` | PANEL-28, PANEL-31 | ⬜ queued |
| PANEL-33 | `panel-33-login-sandbox` — find out whether the panel's `claude agents` and `auth status` calls take or spend a login refresh, in a scratch login under its own CLAUDE_CONFIG_DIR, and act on it (PANEL-28 design D1/D6: outcomes A, B, C, inconclusive). Carry from PANEL-28's review: word tasks plainly (verify-change reads backticked slash commands such as login and status, and dotted Go names, as paths); every stop path names its owner; the 2 s lock watch can miss a fast refresh, so use a fast watcher or an after-the-fact signal (the config dir's mtime), and give the outcome table an "anything else" row; the status command reads local state, so it is a weak check of the main login | `framework/internal/panel` | PANEL-28 | ⬜ queued |
| PANEL-26 | `panel-26-idle-backoff` — folded into PANEL-25 | `framework/internal/panel` | — | ❌ cancelled — folded into PANEL-25, which touches the same polling code |
| PANEL-27 | `panel-27-install-no-open` — folded into PANEL-25 | `framework/internal/panel/install`, `framework/internal/cmd` | — | ❌ cancelled — folded into PANEL-25 |

### Process follow-ups (after the panel batch, by the owner's decision of 2026-10-02)

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| OPS-32 | `ops/ops-32-mod-guard` — a loaded mod cannot quietly undo the blocking hooks: a mod can approve a tool call a `PreToolUse` hook refused (the mods docs), so `pr-merge-guard.sh` and `no-blind-source-rewrite.sh` hold only while no mod does; session-start names the active mods and flags one that handles `tool.call` (`claude plugin validate` lists its hooks and calls), and `docs/conventions.md` says what the guards no longer guarantee with one loaded | `template/.claude/{lib/context.sh,health/,checks/}`, `docs/conventions.md` | — | ⬜ queued |
| OPS-24 | `ops/ops-24-design-docs-lanes` — a project can run `design/` and `docs/` lanes: `BRANCH_DESIGN` and `BRANCH_DOCS` keys, `checks/branch-prefixes.sh` accepting their panel lanes, the `--tsv` kind for their rows, and a model-roles lanes row (Standing Tee's swap, ST-3.3 and ST-3.12, needs it) | `template/.claude/{project.conf,lib/conf.sh,checks/branch-prefixes.sh,model-roles.json}`, `framework/internal/panel` | — | ⬜ queued |
| OPS-25 | `ops/ops-25-reviewer-hand-back` — a worker that spawns a reviewer gets its result: today a `reviewer` has no SendMessage, so its hand-back reaches the top session and the worker waits silently (Standing Tee #425 lost about 4h); build-change, apply-change and merge-pr route it, or the worker reads it back | `template/.claude/{skills/apply-change,skills/merge-pr,workflows/build-change.js,agents/reviewer.md}` | — | ⬜ queued |
| OPS-29 | `ops/ops-29-train-lows` — the train's resolution-review lows: `gitattributesPlan.apply` prints "Merged .gitattributes…" to stdout rather than update's writer (`apply(out, targetDir)`, and the comment at `gitattributes_test.go:19`) (L1); update names a missing guidance doc twice, in the Docs notice and under the new project files (`update.go`, `projectfiles.go`) (L2); start-project step 9 omits the `people` module (L3); with `GH_DEBUG` on, `pr-merge-guard.sh` (around line 520) shows gh's first stderr line, not its real (last) error, where `tail -n 1` would fix it (L4); `checks/openspec.sh` has no committed negative case for its five required strings, such as a copy of the skill with each removed, asserting FAIL (L5); its `` `/(clauductor:)?propose` `` regex rejects an un-backticked `/propose` while its message says only "does not state '/propose'" (L6) | `framework/internal/cmd/{update,projectfiles}.go`, `template/.claude/skills/start-project/SKILL.md`, `template/.claude/hooks/pr-merge-guard.sh`, `template/.claude/checks/openspec.sh` | OPS-27 | ⬜ queued |

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
| REL-2 | `ops/rel-2-install-kit` — release builds cross-compile with CGO_ENABLED=0 (and CI proves it per PR); install, init and the plugin's init report missing tools with per-OS hints; a starter `.gitleaks.toml` the project owns; install.sh offers, never installs unasked | `scripts/build-release.sh`, `.github/workflows/release-build.yml`, `template/.claude/prereqs.sh`, `template/.gitleaks.toml`, `install.sh` | REL-1 | ✅ merged (#60) |
| OPS-28 | `ops/ops-28-restore-macos-pr-ci` — roll OPS-26 back: a pull request's CI runs macOS and Ubuntu again (keep the superseded-run cancel). OPS-26 was a temporary speed-up for getting the first release out; start this once REL-1 has merged and the owner says the release is settled | `.github/workflows/test.yml`, `CLAUDE.md`, `docs/panel.md`, `scripts/ci/steps.sh` | OPS-26, REL-1 | ⬜ queued |

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
| ST-5 | `ops/st-5-swap-plan` — the swap plan: Standing Tee's convergence as an ordered list of PRs, each with its owner decisions, its checks and its rollback | Standing Tee docs | ST-0 | ✅ merged (#60) |
| ST-6 | `ops/st-6-designer-onboarding` — the designer's onboarding PR: Standing Tee's designer onboarding and welcome page describe the converged model (the panel, who decides), so Damian works from them | Standing Tee docs | ST-3 | ⬜ queued |

### Phase 2 upstream follow-ups (the convergence map's P2 rows not yet owned)

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| P2.15b | `ops/roadmap-boundary-tasks` — the roadmap parser reads Standing Tee's `**Gate X boundary-task status:**` lines, and `--text` warns on an open boundary task (left over from P2.15, #45) | `template/.claude/roadmap-queue.sh`, `template/.claude/checks/roadmap.sh` | P2.15 (#45) | ⬜ queued |
| P2.11 | `ops/module-living-visuals` — derived pages kept current: a registry `refresh:` field, a close step that refreshes or stamps every page that is behind, and a generated-block check (Phase 2 wave 2) | `template/.claude/modules/living-visuals/` | P2.7 (#43) | ✅ merged (#61) |
| P2.12b | `ops/module-ideas` — the ideas queue as a module: the skill, a session-start count, the close-time render, and a check that the rendered page is never hand-edited (Phase 2 wave 2; the risk register half of P2.12 is #41) | `template/.claude/modules/ideas/` | P2.7 (#43) | ✅ merged (#61) |
| P2.11c | `ops/living-visuals-lows` — #57's round-2 lows in `living.sh`: a claim value is read only up to the first `<`, so `<b data-claim="owner">Rich <i>and Damian</i></b>` passes against `echo Rich`, and the README's "markup inside a value fails" is untrue for that shape (L1); `data-claim` is counted inside scripts, styles and comments, so a `[data-claim=owner]` selector fails a valid page with a misleading message (L2); `--regen`'s temp-file-and-rename drops the page's mode, replaces a symlinked page and leaves `*.living-regen.<pid>` behind when interrupted (L3); jq 1.7's `(at file:6)` defeats the `jq: error` prefix strip (L4); and from #57's last review, the ideas module's `render.sh` (around lines 101-103): its comment names "the one false pass" of the undated count, but there are two (also a forged none over ideas with `updatedAt` and no `createdAt`), and its list of free-text fields omits the roadmap row (L5) | `template/.claude/modules/{living-visuals,ideas}/` | P2.11 | ⬜ queued |

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
