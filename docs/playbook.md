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
3. **`/session-close`** lands what is open, archives finished changes, **compounds** the
   insights (each Raw one promoted to a rule, marked an instance, or recorded as not promoted),
   updates the roadmap, insights, owner queue and journal, lands its own `ops/session-<N>-close` PR, quiets the machine
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
- **Nothing needs the panel or the `clauductor` binary.** The skills, hooks, checks, the gate and
  its lease (`lease.sh` when there is no binary), the merge guard and the records all run in Claude
  Code alone; a panel that is not running is a normal state. `checks/no-clauductor.sh` holds this:
  it runs the gate, the process checks, a skill context script and the status line with
  `clauductor` off `PATH` and no panel, and fails on anything that errors or asks for either.

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
- **Each round is graded**: `pass` (gate green, nothing medium or worse), `concern` (the worst
  finding is medium) or `fail` (the gate red, or a high or critical finding); the grades go into the
  group's Progress line. **The stuck-loop breaker** stops the group, as `STUCK`, at the first sign
  the loop cannot converge rather than at the round cap: a finding fixed last round is back
  unchanged (and was not disputed), the peak and the count of findings both did not fall, or the
  reviewed diff is one already reviewed (the fix changed nothing, or reverted).
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
| stuck (notify says STUCK) | the stuck-loop breaker: a fixed finding came back unchanged, the peak and the count both stopped falling, or a round reviewed a diff already reviewed | read the finding it names; usually a design question or a group too big: decide, or split |
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

## Choosing a model by evidence

A role's model and effort are a bet on quality per dollar. `.claude/evals/` turns the bet into a
measurement for every role that has a suite. Today that is the reviewer: 18 seeded-defect cases in
Go, TypeScript, Python and shell, covering wrong money, cross-tenant access, data loss, an untested
scenario, a test that passes without its code, a control that silently does nothing, and
portability. Five of the cases are clean controls, which measure false alarms.

```sh
sh .claude/evals/run.sh --role reviewer --model sonnet --effort high --estimate   # the price, free
EVAL_CLAUDE=.claude/evals/fake-claude.sh sh .claude/evals/run.sh --role reviewer --model opus --effort high --out /tmp/try   # the harness, free
sh .claude/evals/run.sh --role reviewer --model opus --effort high                # the real run
```

Each case runs in a scratch repository. The agent runs through `claude -p` with the same prompt,
tools and findings schema that build-change uses. The run writes
`.claude/evals/receipts/<role>-<model>-<effort>-<date>.json`, which holds:

- the role's **recall** (planted defects caught);
- its **precision** (actionable findings that were real);
- its **fp_rate** (clean cases it flagged);
- its **severity accuracy**;
- the run's **cost**, taken from Claude Code's own usage report;
- the hashes it ran at.

A full run's estimated cost:

| Model and effort | Per run |
|---|---|
| opus/high | about $7.40 |
| opus/medium | about $6.30 |
| sonnet/high | about $4.10 |
| haiku/high | about $2.10 |

Multiply by 2 to 4 if the agent fans out through the `code-review` skill.

**Comparing opus/high, sonnet/high and opus/medium.**

1. Run all three on the same suite on the same day.
2. Tabulate the receipts:

   ```sh
   jq -r '[.model, .effort, .scores.recall, .scores.fp_rate, .scores.severity_accuracy, .cost.usd, .cost.catches_per_usd] | @tsv' .claude/evals/receipts/reviewer-*.json
   ```

3. Drop any receipt below the thresholds in `model-roles.json` `.evals.thresholds`, however cheap it
   is. The thresholds are recall ≥ 0.8, fp_rate ≤ 0.2 and severity accuracy ≥ 0.5.
4. Among the receipts that pass, catches per dollar decides, with two cautions:
   - **Noise.** A run is one sample, and one case moves recall by 0.05. Re-run a close call. Treat a
     recall gap under about 0.1 as noise.
   - **What was missed.** `jq '.cases[] | select(.caught < .planted)'` lists the misses. A cheaper
     model that misses a critical case (cross-tenant access, a silent control) is not cheaper. For
     the reviewer, a missed defect costs far more than the few dollars between runs.
5. Weigh severity accuracy heavily for the reviewer. Severity is what tells build-change to stop,
   so a reviewer that grades a critical as a medium ends rounds early.

**Recording a choice.**

1. Set the role's model and effort in `model-roles.json`.
2. Fix what `checks/model-roles.sh` names. That covers the agent's frontmatter and build-change's
   tables.
3. Commit the receipt.
4. Copy the line the run prints into the role's `eval` field.

The check fails while the role's model differs from its evidence. `pr-merge-guard` rule 13 refuses
any PR that changes the role's model, its agent or the workflows unless it carries a passing
receipt at the head's hashes. Re-run the suite when any of those three changes. A role that has
not yet been measured records `{"baseline": "<model>/<effort>"}`: it can stay as it is, but it
cannot change without a receipt.

**When Haiku makes sense for the mechanic.** The mechanic runs a script and quotes its result
(preflight, gate, commit, receipt). It fails by misquoting, for example by dropping a FAIL line,
not by misjudging. Haiku fits the mechanic when both of these hold:

- the step returns a schema that the workflow checks against evidence (the gate step reports
  `passed` alongside the output it quotes);
- you have seen Haiku do the job, through economy mode, which already drops the mechanic to
  haiku/low.

Before making Haiku the mechanic's default, give the mechanic a suite. Use script outputs with a
planted FAIL line and the expected `passed: false`, under `.claude/evals/mechanic/cases/`, then
compare haiku/low with sonnet/low. Until a suite exists, rule 13 only notes the change. Never use
Haiku for the reviewer or the planner: economy mode never drops them either.

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
| `pr-merge-guard.sh` | every Bash call | blocks `--auto`/`--admin`, a merge without gate evidence for its head, a change PR with no slice line, a duplicate journal session, a build with open tasks, an uncited scenario, an archive of an unfinished change, a squash body without provenance trailers, a changed model, agent or workflow without a passing eval receipt |
| `no-blind-source-rewrite.sh` | every Bash call | blocks `sed -i`/`perl -pi`/python read-modify-write on tracked source: use the Edit tool |
| `worktree-hook-drift.sh` | every agent spawn | blocks a worktree agent while the main checkout's hooks lag `origin/main` |
| `focus-staleness.sh` | every prompt | nudges when the status-line focus is stale |
| `format.sh` | every Write/Edit | runs the project's formatter, when configured |
| `run-local.sh` | every gate | takes the gate lease; runs the scenario trace and the secret scan (gitleaks) before your steps; writes the receipt only for a complete, clean run |
| `settings.json` | every tool call | denies reads of `.env` files and keys and a few commands (force-push, `tmux kill-server`); runs Bash in the sandbox with a network allowlist. Loosen per machine in `.claude/settings.local.json` (`sandbox.excludedCommands`, `sandbox.network.allowedDomains`) |
| `.claude/checks/run.sh` | every gate (a step) | the process checks: hook tables, the roadmap parser, ADR numbering, model roles, the budget |

AGENTS.md's *What executes each rule* table is the full list, including the honest last row: what
nothing executes.

## Metrics: how the work flows and what it costs

DORA 2025 found that AI raises throughput and instability together, so both are measured, from
data that already exists. Nothing new is recorded to feed them.

| Figure | From |
|---|---|
| lead time (first commit to merge), cycle time (PR opened to merge), merges per week | merged PRs (`gh`) |
| change-fail rate, escaped defects | a merged PR later reverted, or fixed by a `fix/` PR that names it (its `#number`, its change id, its title) |
| approval wait | the proposal's first commit to its `**Approved:**` line (to the day where one squash carries both) |
| review rounds | `tasks.md` `## Progress`: "converged in N round(s)" per group |
| aging work in progress | open changes, and `change/`, `fix/`, `ops/` branches not merged |
| outcomes | each proposal's `## How we'll know`, due and checked from its outcome-check roadmap row |
| cost by role, model, change and project | this machine's Claude Code transcripts at the list prices in `model-roles.json` |

- **`.claude/metrics.sh`** prints them as the panel's metrics JSON for 7, 30 and 90 days: the
  panel's **Metrics** view and **Flow** card draw it (`metrics` in `.clauductor/panel.json`,
  panel config version 4). A source it cannot read (no `gh`, offline, no transcripts) leaves its
  figures empty with a note saying why; it never prints a zero for "unknown".
  `checks/metrics.sh` holds it to the panel's contract.
- **`.claude/health/flow.sh`** is the same figures as one line at every `session-start`.
- **`.claude/usage-report.sh`** is cost by role and model, for one session (`session-close` shows
  this session's) or a window (`--days 30`). An agent's spend goes to the role at the root of its
  spawn chain. A model with no price is listed as UNPRICED, never costed at zero. These are list
  prices, so on a subscription they show how fast the quota drains, not a bill.

## The record

| Tier | Where | Holds | Promotes to |
|---|---|---|---|
| raw | `docs/insights-log.md` | lessons as they happen, with a status | an ADR, or `Instance of`, or `Technique` |
| decisions | `docs/adr/` | decisions and lessons with a mechanism, each naming its enforcement | AGENTS.md only as a mechanism |
| rules | `AGENTS.md` | the four rules and what executes each (a byte budget) | never grows by prose |
| narrative | `docs/development-journal.md` | each session: what, why, what next | — |

Plus the living state: `docs/roadmap.md` (the queue), `changes/` (in flight), `specs/` (what the
system does), `docs/owner-queue.md` (what needs the owner).
