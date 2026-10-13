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
| OPS-33 | `fix/78-flaky-tests` — the two flaky tests behind #78 are fixed at their cause, not retried (`TestRegistryIsWrittenBeforeTheAction` on Ubuntu, `TestLaunchdStartWaitsForTheRunningPanel` on macOS), and the metrics check's fixture names a failed git step instead of a missing branch (#62 made diagnosable; its cause is OPS-35's) | `framework/internal/panel`, `framework/internal/panel/lanes`, `framework/internal/panel/install`, `framework/internal/testbin`, `template/.claude/checks` | — | ⬜ queued |
| OPS-51 | `fix/95-isolate-plugin-validate` — the plugin tests that run the real `claude` (`TestClaudePluginValidate`, `TestClaudePluginInstall`) run it in a throwaway HOME and config dir, and refuse to run it otherwise (#95, the owner's decision of 2026-10-13) | `framework/internal/plugin` | — | ⬜ in flight (#98) |
| OPS-36 | `fix/84-race-fork-child` — the local gate's race step no longer fails on macOS's race detector itself (#84): on Darwin 27 with Go 1.26.1 `-race`, a forked child of a race-built test binary crashes (`ThreadSanitizer: CHECK failed: tsan_rtl.cpp:94`, exit 66) or spins before exec under fork pressure, on `main` as well; pin it with a minimal reproducer checked against golang/go, then fix or work around it at its cause (never by retrying the step) | `framework/internal/panel`, `template/scripts/ci` | — | ⬜ queued |
| PANEL-29 | `panel-29-lane-dialog` — New lane knows the work: "New lane here" on a change's worktree pre-selects that change's template, an Up next change whose branch or worktree already exists starts in it instead of failing on `git worktree add`, the dialog warns before a build of a change with no Approved line before Start, and a plain "just a Claude session" option is offered (#67, gaps 2–4) | `framework/internal/panel/web`, `framework/internal/panel/lanes` | — | ✅ merged (#94) |
| PANEL-28 | `panel-28-login-refresh` — the panel's own `claude` calls (`agents`, `auth status`, `--version`) never strand or crowd a session's login refresh ("another Claude Code process is refreshing it"): they run one at a time, pause while a refresh holds the lock, are stopped with SIGTERM rather than killed, and `auth status` leaves its timer; a stuck, stranded or future-dated lock shows in the panel. Whether the calls take part in a refresh at all is PANEL-33's to find out | `framework/internal/panel` | — | ⬜ queued — proposed (#76) |

### Attach

By the owner's decision of 2026-10-12, a repository can use the panel without Clauductor installed in it, and Standing
Tee stays independent rather than converging on the template (`docs/prds/active/PRD-attach.md`). These rows come
before MAPS.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| OPS-49 | `ops/ops-49-attach` — the attach direction is on the roadmap with its PRD: Phase 5 becomes "Standing Tee attaches" (ST-0 to ST-4, OPS-24 and P2.15b cancelled, ST-6 rewritten, ST-7 added), PANEL-42 queued before MAPS, each MAPS row says how it works in an attached repository, and PRD-change-process's Standing Tee adoption is marked superseded; OPS-31 keeps the `/curtain` skill and drops its mod | `docs/roadmap.md`, `docs/prds/active/{PRD-attach,PRD-maps,PRD-change-process}.md` | OPS-37 | ✅ merged (#87) |
| PANEL-42 | `panel-42-attach` — `clauductor panel attach <repo>` (and **+ Add a project…** → Attach) serves a repository without writing to it: the panel config is generated into `~/.clauductor/projects/<id>/panel.json` and registered through `projects.json`'s `config`, trusted as any config is, its commands run in the repository; a test proves the repository's files are untouched; the plugin's per-repository opt-out (`enabledPlugins` in the uncommitted `.claude/settings.local.json`) is documented, so its guards never double a repository's own | `framework/internal/cmd`, `framework/internal/panel/install`, `framework/internal/panel/config`, `framework/internal/panel/web`, `docs/panel.md`, `docs/plugin.md` | — | ⬜ queued |

### MAPS phase 1

By the owner's decision of 2026-10-12, these rows come after the rows above and ahead of every row below. How each works in an
attached repository is in PRD-maps, *In an attached repository*; the rows whose design it changes say so. What
Clauductor adopts from Pav Rusovs' "AI OS" (a map of the model, routines, Today, a skills deck) and the Standing
Tee practices since the 2026-10-01 convergence map. Direction, decisions and the standing answer to `/propose` step
0a: `docs/prds/active/PRD-maps.md`. Rollback point: the tag `checkpoint/2026-10-12-pre-maps`.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| OPS-37 | `ops/ops-37-queue-maps` — the MAPS program is on the roadmap in the owner's order, with its PRD; OPS-25 folds into OPS-43 and OPS-23 into OPS-44 | `docs/roadmap.md`, `docs/prds/active/PRD-maps.md` | — | ✅ merged (#86) |
| OPS-38 | `ops/ops-38-roadmap-deps` — `roadmap_queue --tsv` carries Deps as an optional 15th column, validated (an unknown id, a cycle or a cancelled dependency is an error); freeform Deps cells are migrated (`P2.15 (#45)` → `#45`); `currency.sh`'s second roadmap parser reads through `roadmap_queue` | `template/.claude/{roadmap-queue.sh,lib/conf.sh,checks/roadmap.sh}`, `template/.claude/modules/artifacts/bin/currency.sh`, `template/docs/roadmap.md`, `docs/roadmap.md` | OPS-37 | ⬜ queued |
| OPS-39 | `ops/ops-39-roadmap-order` — `--text` derives the order from Deps, as Standing Tee's parser does (#601): each open row reads next, waits on X, `blocked — <what> (<owner>)` or `held — <why>`, and the critical path is marked; `panel-suggest.sh` offers only rows whose dependencies are met | `template/.claude/{roadmap-queue.sh,panel-suggest.sh,checks/roadmap.sh,checks/panel-suggest.sh}` | OPS-38 | ⬜ queued |
| OPS-40 | `ops-40-map-data` — `sh .claude/map.sh --json` prints the operating model as a graph (rules, controls, mechanisms, skills, agents, records, rows; executes, names, registers, enforces, models, extends, deps and builds links), and `checks/map.sh` fails the gate on a reference that does not resolve; an attached repository has no `map.sh`, so the schema allows a map the panel builds from what it sees (lanes, worktrees, the repository's skills, agents and hooks) | `template/.claude/{map.sh,checks/map.sh}` | OPS-38 | ⬜ queued |
| PANEL-35 | `panel-35-map-view` — the Map: a full view (button and `m`) drawing the map in rings and force layouts on a canvas, colour by area, shape by kind, size by links, hover to isolate a node's neighbours, a card per node, search; the one view where colour is decoration (ADR-0009, held by tests); in an attached repository it draws the panel-built map, never an empty page | `framework/internal/panel`, `framework/internal/panel/web`, `docs/panel.md`, `docs/adr/` | OPS-40 | ⬜ queued |
| OPS-41 | `ops/ops-41-gate-state` — the gate keeps an append-only run log and a per-worktree state file; `gate-wait.sh` waits for a gate in the foreground; the `no-background-gate` hook refuses a subagent's background gate | `template/scripts/ci/{run-local.sh,gate-wait.sh}`, `template/.claude/{hooks/no-background-gate.sh,settings.json,checks/gate.sh,checks/hooks.sh}` | — | ⬜ queued |
| OPS-42 | `ops-42-routines` — scheduled work runs while the owner is away: a `.claude/routines.json` registry, `routines.sh` with `tick`, `run`, `list` and `log`, daily and rolling caps, a run log with cost, and a health line; the registry may also live beside an attached repository's sidecar config (`~/.clauductor/projects/<id>/routines.json`), with its run log there too | `template/.claude/{routines.sh,routines.json,health/routines.sh,checks/routines.sh}` | — | ⬜ queued |
| PANEL-36 | `panel-36-map-layouts` — the Map's areas and timeline layouts, animated changes between layouts (snapped under reduced motion), a legend that isolates an area or kind, and live dots for the stream, the runner and the data's age | `framework/internal/panel/web`, `docs/panel.md` | PANEL-35 | ⬜ queued |
| PANEL-37 | `panel-37-routines-view` — the panel ticks each trusted project's routines every 5 minutes, and the Map shows the routines table, ▶ run now, and today's and yesterday's runs, failures and cost; an attached repository's sidecar registry is ticked the same way and covered by the same trust | `framework/internal/panel`, `framework/internal/panel/web`, `docs/panel.md` | OPS-42, PANEL-35 | ⬜ queued |
| PANEL-38 | `panel-38-today` — the Map's Today column: a clock, a quarterly day heatmap (activity or cost), the next 14 days, activity bars and the backlog | `framework/internal/panel`, `framework/internal/panel/web` | PANEL-35, OPS-41, OPS-42 | ⬜ queued |
| PANEL-39 | `panel-39-skills-deck` — a deck of ▶ buttons that start a lane from a deck template (session-start and session-close at the root, propose, build, fix and ops through New lane); the browser still sends no prompt text | `framework/internal/panel/config`, `framework/internal/panel/lanes`, `framework/internal/panel/web`, `.clauductor/panel.json` | PANEL-29, PANEL-35 | ⬜ queued |
| OPS-43 | `ops/ops-43-report-back` — build-change, apply-change and merge-pr report their outcome to a `reportTo` session, and a worker reads its reviewer's result back (folds in OPS-25; `agents/reviewer.md` untouched) | `template/.claude/{workflows/build-change.js,skills/apply-change,skills/merge-pr}` | — | ⬜ queued |
| OPS-44 | `ops/ops-44-review-soft-cap` — review rounds default to 5, and at 5 the run stops to ask the owner whether to continue (a raised `maxRounds` only on their yes); the role tables are deep-frozen (folds in OPS-23) | `template/.claude/{workflows/build-change.js,model-roles.json,checks/model-roles.sh,skills/apply-change}` | OPS-43 | ⬜ queued |
| OPS-45 | `ops/ops-45-statusline-sessions` — the main checkout's status line shows `N running · N gating · N open · N merged today`, and `?` for a number it cannot read | `template/.claude/{statusline.sh,statusline-sessions.sh}` | OPS-41 | ⬜ queued |
| OPS-46 | `ops/ops-46-practice-records` — practice records (`PRAC-NNNN`, `docs/practices/`) beside ADRs, with `new-adr` asking which; a request is priced before it is relayed to a running lane | `template/docs/practices/`, `template/.claude/{skills/new-adr,checks/adr-numbering.sh,skills/session-start}` | — | ⬜ queued |
| OPS-47 | `ops-47-gate-takes-main` — the gate merges `origin/main` in under its lease ("PUSH BEFORE MERGING") and refuses, changing nothing, on a conflict, a dirty tree or a detached HEAD; merge-guard rule 14: every PR's head contains `origin/main` | `template/scripts/ci/run-local.sh`, `template/.claude/{hooks/pr-merge-guard.sh,checks/merge-guard.sh}` | OPS-41 | ⬜ queued |
| PANEL-40 | `panel-40-lane-outcomes` — when a lane's claude ends, is killed or waits, the panel records the outcome and a hook wakes the orchestrator to push it | `framework/internal/panel/lanes`, `template/.claude/{hooks/session-outcome-wake.sh,settings.json}` | OPS-43 | ⬜ queued |

### After MAPS

Queued before 2026-10-12. The approved proposals among them keep their approval and wait (PRD-maps, *The standing
answer to `/propose` step 0a*).

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| OPS-34 | `fix/82-copy-mode-recheck` — typing while scrolled back reaches the lane on a loaded machine too (#82): the copy-mode recheck runs after the wheel is written and polls `pane_in_mode` within a bound, stopping at once when `alternate_on` or `mouse_any_flag` says the program took the wheel (#82's option a; b, a tmux `wait-for` channel, if a bounded poll proves unreliable), and the test's wait goes through `testwait` | `framework/internal/panel/web` | — | ⬜ queued |
| PANEL-30 | `panel-30-remove-worktree-guidance` — Remove worktree says in plain words what goes (the folder) and what stays (the branch and every commit), and warns before removing the worktree of a change in flight (#67, gap 1) | `framework/internal/panel/lanes`, `framework/internal/panel/web` | PANEL-29 (the way back that names a template only) | ⬜ queued — proposed (#74) |
| PANEL-31 | `panel-31-version-check` — a new Claude Code version shows as a quiet status-bar field with its progress rather than a page-wide alert, a version verified in any project clears it in every project and is kept, and Help says what is approximate meanwhile; a real break or an unreadable version still alerts (#68) | `framework/internal/panel`, `framework/internal/panel/state`, `framework/internal/panel/web` | — | ⬜ queued — proposed (#75) |
| PANEL-25 | `panel-25-idle-cost` — an idle panel costs little: with no page in view it stops calling GitHub every minute per project (`gh pr list`) while a lane still auto-closes when its PR merges; with no lane and no page in view it backs its `claude agents`, `git worktree list` and tmux polls off to minutes, and a hook or a lane start still wakes it at once; `panel install --no-open` keeps the login agent from opening a browser tab at every login (folds in PANEL-26 and PANEL-27) | `framework/internal/panel`, `framework/internal/panel/install`, `framework/internal/cmd` | PANEL-28 | ⬜ queued — proposed (#73) |
| PANEL-34 | `panel-34-close-lane` — a session closes its own lane: a `close-lane` skill shows Close's plan (what goes, what stays) and asks the panel to close the lane once the turn ends, through a `clauductor lane close` the panel authenticates (a session cannot call the page's API today); the worktree goes only when clean and merged, as with the page's Close; with no panel the skill says to close it from the panel | `framework/internal/cmd`, `framework/internal/panel`, `template/.claude/skills/close-lane` | — | ⬜ queued |
| OPS-31 | `ops-31-curtain` — `/curtain` wraps a lane up in one command: merge-pr, session-close, then close-lane, stopping at the first step that stops and saying which; it starts only on a PR that is ready (the gate passed on its head, CI green, review converged), so merge-pr reuses that gate receipt rather than running the gate again, and it ends by telling the owner to type `/exit` (a skill cannot send it). No mod: by the owner's decision of 2026-10-12 the terminal animation (`curtain-mod`, `/curtain-mod-demo`) is dropped, and its prototype stays only as history on `ops/curtain-mod` | `template/.claude/skills/curtain`, `template/.claude/skills/merge-pr` | PANEL-34 | ⬜ queued |
| PANEL-32 | `panel-32-verify-now` — Verify now: the panel runs a throwaway session to confirm a new Claude Code version on demand, at a stated cost (#68, split from PANEL-31). From PANEL-31's review (#75): guard on the refresh lock's freshness (touched in the last 60 s, `$CLAUDE_CONFIG_DIR` before `~/.claude`), not its existence; run through PANEL-28's `claude` gate (one stop policy, one slot); the owner approves flag invariants (only Agent executes, nothing waits on a prompt, no settings but the passed hooks), with `.claude/evals/run.sh` as prior art; synchronous probe hooks, so they post before exit; the feasibility spike runs before approval; name whose quota guard applies (the default project's thresholds, as `pollQuotaAlert` reads them); remove leftover probe folders at start and at shutdown; make the 90 s and 10 s limits injectable for tests; and revisit "Accept this version" (PANEL-31 D4) if the check proves too slow to clear. From PANEL-28's review: state the cost of the probe holding PANEL-28's one-call slot, or propose an exception; while it holds it (~90 s plus a 15 s grace) the `claude agents` poll pauses, and every lane action that needs `claude agents` (the first-prompt check, removeVerdict, liveSessions) fails at its 10 s timeout | `framework/internal/panel`, `framework/internal/panel/web` | PANEL-28, PANEL-31 | ⬜ queued |
| PANEL-33 | `panel-33-login-sandbox` — find out whether the panel's `claude agents` and `auth status` calls take or spend a login refresh, in a scratch login under its own CLAUDE_CONFIG_DIR, and act on it (PANEL-28 design D1/D6: outcomes A, B, C, inconclusive). Carry from PANEL-28's review: word tasks plainly (verify-change reads backticked slash commands such as login and status, and dotted Go names, as paths); every stop path names its owner; the 2 s lock watch can miss a fast refresh, so use a fast watcher or an after-the-fact signal (the config dir's mtime), and give the outcome table an "anything else" row; the status command reads local state, so it is a weak check of the main login | `framework/internal/panel` | PANEL-28 | ⬜ queued |
| PANEL-26 | `panel-26-idle-backoff` — folded into PANEL-25 | `framework/internal/panel` | — | ❌ cancelled — folded into PANEL-25, which touches the same polling code |
| PANEL-27 | `panel-27-install-no-open` — folded into PANEL-25 | `framework/internal/panel/install`, `framework/internal/cmd` | — | ❌ cancelled — folded into PANEL-25 |

### Process follow-ups (after the panel batch, by the owner's decision of 2026-10-02)

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| OPS-32 | `ops/ops-32-mod-guard` — a loaded mod cannot quietly undo the blocking hooks: a mod can approve a tool call a `PreToolUse` hook refused (the mods docs), so `pr-merge-guard.sh` and `no-blind-source-rewrite.sh` hold only while no mod does; session-start names the active mods and flags one that handles `tool.call` (`claude plugin validate` lists its hooks and calls), and `docs/conventions.md` says what the guards no longer guarantee with one loaded | `template/.claude/{lib/context.sh,health/,checks/}`, `docs/conventions.md` | — | ⬜ queued |
| OPS-24 | `ops/ops-24-design-docs-lanes` — a project can run `design/` and `docs/` lanes: `BRANCH_DESIGN` and `BRANCH_DOCS` keys, `checks/branch-prefixes.sh` accepting their panel lanes, the `--tsv` kind for their rows, and a model-roles lanes row (Standing Tee's swap, ST-3.3 and ST-3.12, needs it) | `template/.claude/{project.conf,lib/conf.sh,checks/branch-prefixes.sh,model-roles.json}`, `framework/internal/panel` | — | ❌ cancelled — the swap it served is cancelled; an attached Standing Tee gets its design/ lanes from its own sidecar config (owner, 2026-10-12) |
| OPS-25 | `ops/ops-25-reviewer-hand-back` — a worker that spawns a reviewer gets its result: today a `reviewer` has no SendMessage, so its hand-back reaches the top session and the worker waits silently (Standing Tee #425 lost about 4h); build-change, apply-change and merge-pr route it, or the worker reads it back | `template/.claude/{skills/apply-change,skills/merge-pr,workflows/build-change.js,agents/reviewer.md}` | — | ❌ cancelled — folded into OPS-43 (MAPS, 2026-10-12) |
| OPS-29 | `ops/ops-29-train-lows` — the train's resolution-review lows: `gitattributesPlan.apply` prints "Merged .gitattributes…" to stdout rather than update's writer (`apply(out, targetDir)`, and the comment at `gitattributes_test.go:19`) (L1); update names a missing guidance doc twice, in the Docs notice and under the new project files (`update.go`, `projectfiles.go`) (L2); start-project step 9 omits the `people` module (L3); with `GH_DEBUG` on, `pr-merge-guard.sh` (around line 520) shows gh's first stderr line, not its real (last) error, where `tail -n 1` would fix it (L4); `checks/openspec.sh` has no committed negative case for its five required strings, such as a copy of the skill with each removed, asserting FAIL (L5); its `` `/(clauductor:)?propose` `` regex rejects an un-backticked `/propose` while its message says only "does not state '/propose'" (L6) | `framework/internal/cmd/{update,projectfiles}.go`, `template/.claude/skills/start-project/SKILL.md`, `template/.claude/hooks/pr-merge-guard.sh`, `template/.claude/checks/openspec.sh` | OPS-27 | ⬜ queued |
| OPS-35 | `fix/62-metrics-fixture` — the metrics check's flake (#62) is fixed at its cause: when it next fails, OPS-33's `fx` line names the git step that failed, and this row fixes that step. Until then it waits on that observation (64 runs and 300 checkout cycles reproduced nothing) | `template/.claude/checks/metrics.sh` | OPS-33 | ⬜ queued |

### Found on 2026-10-12

Issues found while running the backlog overnight, queued at the bottom by the owner's decision of 2026-10-13. Each is
a small fix or ops lane with no proposal; they run alongside the queue above as the gate allows.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| OPS-52 | `fix/100-guard-multibyte` — the merge guard reads a command that holds "merge" and a non-ASCII character such as `✅` instead of refusing it as unreadable (#100: `merge-reader.awk` fails with `towc: multibyte conversion failure`), while a merge it truly cannot read is still refused | `template/.claude/hooks/lib/merge-reader.awk`, `template/.claude/hooks/pr-merge-guard.sh`, `template/.claude/checks/hooks.sh` | — | ⬜ queued |
| OPS-53 | `fix/92-daemon-lease-flake` — `TestDaemonLeftByTheGateDoesNotHoldTheLease/shell` no longer waits out its 10 s under a loaded full gate (#92): find in `lease.sh` why the shell waiter keeps seeing the killed gate's lease, and fix it at that cause, never by raising the wait | `template/scripts/ci/lease.sh`, `framework/internal/panel/lease` | — | ⬜ queued |
| OPS-54 | `fix/88-rewrite-script-file` — `no-blind-source-rewrite.sh` refuses a find-and-replace run from a script file (`python3 rewrite.py`) as it refuses one given inline (#88; reproduce first) | `template/.claude/hooks/no-blind-source-rewrite.sh`, `template/.claude/checks/hooks.sh` | — | ⬜ queued |
| OPS-55 | `fix/91-race-toolchain-guard` — the race step fails, naming golang/go#79804, on a Go toolchain that crashes a forked child (go1.26.0 to go1.26.4 on darwin), so `GOTOOLCHAIN=local` cannot bring #84 back (#91) | `framework/internal/testwait` | — | ⬜ queued |
| OPS-56 | `ops/ops-56-overnight-lows` — the lows found on 2026-10-12: `archive-change.sh` puts a blank line after a new capability's Purpose; the New lane dialog clears the checked "Where it runs" choice when the plan allows no start; the UX harness's `fake/claude` gains a role that posts no startup hooks, so a template lane's first prompt is not skipped | `template/.claude/archive-change.sh`, `framework/internal/panel/web/static/panel.js`, `framework/internal/panel/testdata/ux/fake` | — | ⬜ queued |
| OPS-57 | `fix/guard-pr-number` — the merge guard reads a merge's PR number exactly, or refuses the merge as one whose PR it cannot tell; it never judges a different number than the shell will merge (found by OPS-52's review; details held privately, as the repository is public) | `template/.claude/hooks/pr-merge-guard.sh`, `template/.claude/checks/merge-guard.sh` | OPS-52 | ⬜ queued |
| OPS-58 | `fix/guard-api-word` — the merge guard reads a `gh api` merge whatever form its command word takes, as it already does for `gh pr merge`; and its KNOWN LIMITS name, by class only, the shell-expansion forms the owner accepted as limits on 2026-10-12 (the guard's purpose is accidental merges without evidence, not a determined adversary) | `template/.claude/hooks/lib/merge-reader.awk`, `template/.claude/hooks/pr-merge-guard.sh`, `template/.claude/checks/merge-guard.sh` | OPS-52 | ⬜ queued |

### MAPS phase 2

The $15 Builder tier. It starts when the owner buys it, so it sits last and nothing waits on the purchase.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| OPS-48 | `ops/ops-48-maps-study` — a behaviour-only spec of the Builder tier's dashboard (layouts, force tuning, the orbit's projection, canvas performance, runner caps), written under the clean-room rule: the paid source stays outside every repo and plugin input, and no code, identifier or value is copied (PRD-maps, Phase 2) | `docs/prds/active/PRD-maps-phase2.md` | PANEL-40 | ⬜ queued |
| PANEL-41 | `panel-41-map-orbit` — the Map's circle and 3D-orbit layouts and what else the study finds worth having, built by a session that never saw the paid source | `framework/internal/panel/web`, `docs/panel.md` | OPS-48 | ⬜ queued |

## Phase 3 — Measuring the model
**Owner:** Rich

Exit criteria: flow and DORA metrics, and the reviewer agent's recall, are computed from the repo
and reach the owner through session-start and the panel.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| OPS-9 | `ops/ops-9-metrics` — `.claude/metrics.sh`: lead and cycle time, approval wait, review rounds, aging WIP, merge frequency, change-fail rate; a health line and a card | `template/.claude/metrics.sh`, `template/.claude/health/` | — | ✅ merged (#26) |
| OPS-10 | `ops/ops-10-evals` — a seeded-defect suite measures the reviewer's recall; the merge guard wants an eval receipt for agent, workflow and role changes | `template/.claude/evals/`, `template/.claude/hooks/pr-merge-guard.sh` | — | ✅ merged (#28) |
| OPS-16 | `ops/OPS-16-narrow-rule-13` — rule 13 fires only on what makes the reviewer what it is (its model, its agent, the marked review prompt), declared per role in `evals.triggers`; the reviewer's opus/high baseline measured once | `template/.claude/{lib/evals.sh,evals/run.sh,hooks/}`, `.claude/model-roles.json` | OPS-10 | ✅ merged (#40) |
| OPS-23 | `ops/ops-23-freeze-role-tables` — `build-change.js` deep-freezes ROLES, TIERS and ECONOMY, so no step can change a role's model or effort mid-run; it lands in the next PR that runs a reviewer eval anyway (the file is a rule-13 input) | `template/.claude/workflows/build-change.js` | — | ❌ cancelled — folded into OPS-44 (MAPS, 2026-10-12) |
| OPS-18 | `ops/OPS-18-clean-controls` — fix the two reviewer clean controls that hold real defects (`sh-clean-coverage-fails-closed`: a non-numeric floor passes; `py-clean-token-expiry`: verify() raises on a non-ASCII mac or a Unicode digit), then #40 re-runs the opus/high eval on its rebased head and records the receipt (rule 13 blocks #40 until a receipt passes) | `template/.claude/evals/reviewer/cases/` | — | ✅ merged (#40) |

## Phase 4 — Release
**Owner:** Rich

Exit criteria: a stranger installs clauductor from a release download or Homebrew, not a source
build.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| REL-1 | `feature/REL-1-release` — v0.1.0: public README, changelog, release builds and install paths | `README.md`, `CHANGELOG.md`, `.github/workflows/release*.yml`, `install.sh` | — | ⬜ in flight (#25) |
| REL-2 | `ops/rel-2-install-kit` — release builds cross-compile with CGO_ENABLED=0 (and CI proves it per PR); install, init and the plugin's init report missing tools with per-OS hints; a starter `.gitleaks.toml` the project owns; install.sh offers, never installs unasked | `scripts/build-release.sh`, `.github/workflows/release-build.yml`, `template/.claude/prereqs.sh`, `template/.gitleaks.toml`, `install.sh` | REL-1 | ✅ merged (#60) |
| OPS-28 | `ops/ops-28-restore-macos-pr-ci` — roll OPS-26 back: a pull request's CI runs macOS and Ubuntu again (keep the superseded-run cancel). OPS-26 was a temporary speed-up for getting the first release out; start this once REL-1 has merged and the owner says the release is settled | `.github/workflows/test.yml`, `CLAUDE.md`, `docs/panel.md`, `scripts/ci/steps.sh` | OPS-26, REL-1 | ⬜ queued |

## Phase 5 — Standing Tee attaches
**Owner:** Rich

Exit criteria: the panel serves Standing Tee in attach mode, with its cards running Standing Tee's own scripts and
nothing of Clauductor's written into its repository; Damian's onboarding names the panel as optional. Standing Tee
keeps its own operating model: by the owner's decision of 2026-10-12 it does not converge on the template
(`docs/prds/active/PRD-attach.md`). Practices it proves flow one way, into the template, as rows in Phase 2.

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| ST-0 | `ops/st-0-convergence-map` — the convergence map: every shared file classified, every record format decided, the onboarding sections listed (read-only) | Standing Tee docs | OPS-8 | ❌ cancelled — Standing Tee does not converge (owner, 2026-10-12); the 2026-10-01 map already fed the template's upstream rows |
| ST-1 | `ops/st-1-extension-points` — optional modules and a project-local layer (the settings.json merge and `clauductor diff` landed in #29) | `framework/internal/cmd`, `template/.claude/modules/` | ST-0 | ❌ cancelled — replaced by attach mode, PANEL-42 (owner, 2026-10-12) |
| ST-2 | `ops/st-2-upstream` — what the map marks generic and better in Standing Tee moves into the template | `template/` | ST-0 | ❌ cancelled — replaced by one-way porting, row by row (the MAPS rows are the first; owner, 2026-10-12) |
| ST-3 | `ops/st-3-converge` — Standing Tee converges file by file, with the no-clauductor check in its gate | Standing Tee | ST-1, ST-2, OPS-24 | ❌ cancelled — replaced by attach mode, PANEL-42 (owner, 2026-10-12) |
| ST-4 | `ops/st-4-handover` — `.claude/clauductor-template` written; `clauductor update` from then on | Standing Tee | ST-3 | ❌ cancelled — replaced by attach mode, PANEL-42 (owner, 2026-10-12) |
| ST-5 | `ops/st-5-swap-plan` — the swap plan: Standing Tee's convergence as an ordered list of PRs, each with its owner decisions, its checks and its rollback | Standing Tee docs | ST-0 | ✅ merged (#60) |
| ST-7 | `ops/st-7-attach-standing-tee` — the panel serves Standing Tee in attach mode: a sidecar config whose cards run Standing Tee's own `infra/roadmap-queue.mjs`, founder queue and health readers, and whose templates match its kinds of session; `docs/panel.md` uses it as the worked example of a repository with a model of its own. Applying it to the installed panel waits for the owner's go | `docs/panel.md`, the owner's `~/.clauductor/projects/` | PANEL-42 | ⬜ queued |
| ST-6 | `ops/st-6-designer-onboarding` — Damian's onboarding and welcome page gain an optional "the panel, in attach mode (recommended on a Mac)" section; the rest of Standing Tee's model is unchanged. A change in Standing Tee's repository, made only on the owner's go | Standing Tee docs | ST-7 | ⬜ queued |

### Phase 2 upstream follow-ups (the convergence map's P2 rows not yet owned)

| # | Change | Scope | Deps | Status |
|---|--------|-------|------|--------|
| P2.15b | `ops/roadmap-boundary-tasks` — the roadmap parser reads Standing Tee's `**Gate X boundary-task status:**` lines, and `--text` warns on an open boundary task (left over from P2.15, #45) | `template/.claude/roadmap-queue.sh`, `template/.claude/checks/roadmap.sh` | P2.15 (#45) | ❌ cancelled — only a converged Standing Tee would run the template's parser; attached, its cards run its own (owner, 2026-10-12) |
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
| o.1 | `ops/check-outcome-panel-29-lane-dialog` — check the outcome of panel-29-lane-dialog (due 2026-10-19) | the owner starts a build lane on an in-flight change's existing worktree from "New lane here", in one click and with no git error, and the dialog's warnings match what the change actually needs | — | ⬜ queued |
