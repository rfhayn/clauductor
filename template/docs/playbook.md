# Playbook: how work runs here

This project runs on an operating model for Claude Code: **hands-off sessions** that orchestrate
**lanes** (one git worktree each, visible on the local panel), a **roadmap queue** of rows,
**proposals** the owner approves just in time, changes **built and reviewed per task group**, and
**merges** that a guard, not a person, holds to the rules. This page is the manual. The rules
themselves are in `AGENTS.md`; the reasons behind them in `docs/principles.md`.

## The session, start to close

1. **`/session-start`** computes where things stand (the owner queue first, the panel, open PRs
   and whose they are, the change queue, the journal, insights, ADRs, health lines) and sets the
   status-line focus.
2. The session then **runs hands-off**: it starts lanes, builds, reviews, fixes, and merges
   through `/merge-pr`, without asking, until something only the owner can decide comes up.
3. **`/session-close`** lands what is open, archives finished changes, updates the roadmap,
   insights, owner queue and journal, lands its own `ops/session-<N>-close` PR, quiets the machine
   (`machine-quiet.sh`), and sends one **Done** notification.

A check that could not run says `CANNOT CHECK`, never "none": absence must not read as health.

## Who decides

| The owner decides | Everything else runs without asking |
|---|---|
| a change's proposal and every design decision | building, gating, reviewing, fixing |
| ADRs | commits, pushes, PRs, issues on this repo |
| deploys and anything irreversible | **merging**, through `merge-pr`, once the guard passes and review converged |
| anything outward beyond this repo | archiving, the records, closing the session |

The owner is reached by `PushNotification` only: when **Blocked** (an owner decision, an
environment fault, a gate that stays red, review that does not converge) and once when **Done**.
Work that needs them at the computer goes in the **owner queue** (`docs/owner-queue.md`), which
`session-start` and the panel show until it is ticked. The owner's role name is `OWNER_ROLE` in
`.claude/project.conf`.

## Lanes

A lane is one interactive Claude session in its own git worktree (`.claude/worktrees/<lane>`),
started from the panel's **New lane** (or by hand), on one branch:

| Lane | Branch | How many | Starts with |
|---|---|---|---|
| build | `change/<id>` | one at a time | `/build-change {"change": "<id>"}` on an approved proposal |
| propose | `change/<id>` | at most one ahead of the build | `/propose <id>`, which stops for the owner |
| fix | `fix/<issue>-<slug>` | any that share no files | the issue, the code as built, the gate, `/merge-pr` |
| ops | `ops/<name>` | any that share no files | a tooling, docs or process task |
| orchestrator | `main` (the main checkout) | one | `/session-start` |

- **The main checkout stays on `main`.** Every hook runs from it; `worktree-hook-drift.sh` refuses
  worktree agents while its hooks lag `origin/main`.
- **One gate at a time.** All lanes share one gate lease (`scripts/ci/run-local.sh`), so a lane's
  gate may queue behind another's; the panel shows who holds it.
- The panel (`clauductor panel`) shows each lane's focus, context, quota and state. Its preset
  config is `.clauductor/panel.json`: the lanes above, lane templates whose **Up next** rows come
  from the roadmap (`panel-suggest.sh`), the gate queue, and pinned owner-queue and change-queue
  cards. Trust it once with `clauductor panel trust`.

## Several people

Each person has their own Claude and **merges only their own PRs**. `session-start` stars
another person's PR (★) and names every file their open PR shares with your branch (OVERLAP).
Shared records conflict at close by design, and resolve one way each (`session-close`'s table): the
journal renumbers, the insights log keeps both rows, ADRs take the next number. Claude's memory is
per machine, so what the other person needs goes in the repo.

## The change lifecycle

```
roadmap row ──► /propose (just in time) ──► owner approves ──► proposal lands on main
     ▲                                                              │
     │                                                              ▼
  archive ◄── merge-pr ◄── full gate receipt ◄── /build-change, one task group at a time
  (specs/ promoted, row ✅)
```

0. **The fast path.** A diff that fits in one sentence (a typo, a bump, a one-line fix) gets no
   proposal: it goes to a `fix/` or `ops/` lane, the gate and `/merge-pr`. Everything below is for
   a change to what a user can do.
1. **A roadmap row** (`docs/roadmap.md`) is the scoping unit until its turn. Capturing an idea for
   later is a row, not a change directory. A row may carry a cost appetite, `Budget: $N`.
2. **Propose just in time, at most one ahead** of the change being built: `/propose` checks for
   another proposal (in the tree and on branches), reads the specs the change touches, and drafts
   `changes/<id>/{proposal,design,tasks}.md` (`changes/README.md`): a risk tier, the budget, how
   we'll know it worked, and scenarios with IDs (`[CAP-n-Sn]`) the tests will cite.
3. **The owner approves** the proposal, every design decision and the risk tier: in the PR by
   default, or on a claude.ai review page (the optional module). The approval is recorded in
   `proposal.md` with a hash of the design (`change-approval.sh`), so a later edit to `design.md`
   voids it until the owner approves again. The proposal lands on `main` alone.
4. **Build per task group** (`/build-change`, or `/apply-change` by hand): the loop below.
5. **Verify** (`/verify-change`): every task ticked, every scenario the change adds or modifies
   cited by a test (or escaped with a reason), and the diff touching what `tasks.md` claims.
6. **Merge** through `/merge-pr`: gate evidence for the head commit, converged review, a squash
   body carrying the provenance trailers (`Change:`, `Agent-Role:`, `Model:`, `Session:`).
7. **Archive** (`/archive-change`): spec deltas promoted into `specs/`, the actual cost recorded,
   the outcome check queued for the day the proposal named, the directory moved to
   `changes/archive/`, the row marked `✅ merged (#N)`.

A `fix/` or `ops/` change skips 2–3, 5 and 7: it is not a capability change (the test: *does it
change what a user can do?*).

**Scenarios and tests.** Every scenario a change adds or modifies is cited, by its ID, in a test
file (`TEST_GLOBS` in `.claude/project.conf` says where tests live), or escaped in `tasks.md` with
`(manual: <reason>)`. `.claude/scenario-trace.sh` lists each ID and its tests. The gate and the merge
guard enforce it once a change's tasks are all ticked, and for the living specs always, so deleting
the last test of a merged scenario turns the gate red. The reviewer checks each citing test asserts
the THEN; the trace only proves the test names it.

## Inside build-change

For each task group with open tasks: the **builder** implements only that group, leaving it
uncommitted → the **quick gate** runs (the builder fixes a red gate at its source, twice at most) →
the **reviewer**, told only the change and the group, reviews `git diff HEAD` → findings medium or
worse go back to the builder, who fixes them at their source or disputes them → gate and review
again until a round finds nothing medium or worse → **commit** the group. Then the **full gate** on
HEAD writes the receipt `merge-pr` needs, and the **verify** step runs last. A finding whose source
is an earlier group lifts the group boundary for exactly that fix.

- **Models follow the risk tier** the owner approved: a role with a variant for it in
  `.claude/model-roles.json` runs on it (the builder on sonnet for low risk, opus/xhigh for high;
  the reviewer on opus/xhigh for high). In **economy mode** (the panel writes
  `~/.clauductor/panel/economy.json` past its quota threshold) the roles in `.economy.roles` drop a
  tier; the reviewer and the planner never do.
- **The budget**: after each committed group it reads `.claude/change-cost.sh` and stops once the
  change's spend passes the proposal's `**Budget:**`.
- **The log**: the builder writes each decision `design.md` does not settle into `tasks.md`'s
  `## Decision log`, and each committed group adds a `## Progress` line, so a resumed run (or
  lane) continues from the file.

## When it stops, and your move

| Stop | What it means | Your move |
|---|---|---|
| preflight | wrong branch, dirty tree, in the main checkout, nothing to build | fix the named precondition, re-run (a mid-group stop: `{"resume": <n>}`) |
| design-issue | building would change a decided design or a spec | decide; the proposal or design is updated; re-run |
| blocked / environment fault | a service down, a credential missing | fix the environment, re-run with `resume` |
| gate still red | two fix attempts failed | read the failure; fix or split the group |
| severity rose | fixes introduced worse defects than they removed | read the rounds; usually the group is too big: split it |
| not converged | three rounds without a clean one | rule on the builder's disputes, or split the group |
| agent returned nothing | new agent types register at session start | restart Claude Code, re-run with `resume` |
| receipt | the full gate failed on HEAD | fix, re-run the full gate |
| budget | the change's spend passed its budget | raise the budget (row and proposal) or cut scope, then re-run |
| verify | a task open, a scenario uncited, the approval void, or a task's paths untouched | fix what it names; `/verify-change <id>` |

## Three layers of agents

1. **The session** (the orchestrator lane): talks to the owner, runs skills, starts lanes.
2. **The workflow script** (`.claude/workflows/build-change.js`): the loop, as code, so its stops
   and limits are not a matter of prose.
3. **The agents** (`.claude/agents/`): `builder` (edits), `reviewer` and `reviewer-docs` (never
   edit: no Edit or Write in their `tools:` line), `researcher` (read-only; use it instead of a
   fork for research). Their reach is set by their tools, not by their prompts.

Models and efforts are chosen in ONE place, `.claude/model-roles.json`: each role, its risk-tier
variants, its economy-mode drop, the provenance trailers and the list prices change-cost reads.
Everything else restates it and `checks/model-roles.sh` fails on a disagreement.

## Skills

<!-- skills-table begin -->
| Skill | When | Role |
|---|---|---|
| `/session-start` | every session, first | orient |
| `/session-close` | every session, last | scribe |
| `/propose` | the next roadmap row, just in time | planner |
| `/apply-change` | build a change by hand, group by group | thinker |
| `/archive-change` | after a change's PR merges | scribe |
| `/verify-change` | before a change merges: tasks, scenarios, diff | mechanic |
| `/merge-pr` | land a PR: evidence, converged review, squash | scribe |
| `/dev-journal` | the session's narrative (session-close runs it) | scribe |
| `/log-insight` | a non-obvious lesson, now | scribe |
| `/new-adr` | promote a decision or a lesson with a mechanism | planner |
| `/start-project` | first-time setup of this model | thinker |
| `/architecture-audit` | check the project's architecture rules | thinker |
| `/release-prep` | take main to an environment (owner's go) | thinker |
<!-- skills-table end -->

`/build-change` is a workflow, not a skill: `.claude/workflows/build-change.js`.

## What runs without being asked

| Guard | When | What it does |
|---|---|---|
| `pr-merge-guard.sh` | every Bash call | blocks `--auto`/`--admin`, a merge without gate evidence for its head, a change PR with no slice line, a duplicate journal session, a build with open tasks, an uncited scenario, an archive of an unfinished change, a squash body without provenance trailers |
| `no-blind-source-rewrite.sh` | every Bash call | blocks `sed -i`/`perl -pi`/python read-modify-write on tracked source: use the Edit tool |
| `worktree-hook-drift.sh` | every agent spawn | blocks a worktree agent while the main checkout's hooks lag `origin/main` |
| `focus-staleness.sh` | every prompt | nudges when the status-line focus is stale |
| `format.sh` | every Write/Edit | runs the project's formatter, when configured |
| `run-local.sh` | every gate | takes the gate lease; runs the scenario trace and the secret scan (gitleaks) before your steps; writes the receipt only for a complete, clean run |
| `settings.json` | every tool call | denies reads of `.env` files and keys and a few commands (force-push, `tmux kill-server`); runs Bash in the sandbox with a network allowlist |
| `.claude/checks/run.sh` | every gate (a step) | the process checks: hook tables, the roadmap parser, ADR numbering, model roles, the budget |

AGENTS.md's *What executes each rule* table is the full list, including the honest last row: what
nothing executes.

## The record

| Tier | Where | Holds | Promotes to |
|---|---|---|---|
| raw | `docs/insights-log.md` | lessons as they happen, with a status | an ADR, or `Instance of`, or `Technique` |
| decisions | `docs/adr/` | decisions and lessons with a mechanism, each naming its enforcement | AGENTS.md only as a mechanism |
| rules | `AGENTS.md` | the four rules and what executes each (a byte budget) | never grows by prose |
| narrative | `docs/development-journal.md` | each session: what, why, what next | — |

Plus the living state: `docs/roadmap.md` (the queue), `changes/` (in flight), `specs/` (what the
system does), `docs/owner-queue.md` (what needs the owner).
