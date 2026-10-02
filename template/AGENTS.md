# Working conventions

> **A budget, not an archive.** Every session and every subagent loads this file at startup, so
> each line is paid for again every time an agent starts. `.claude/checks/agents-md-budget.sh`
> holds it to a byte ceiling and each table row to a length ceiling. **New lessons go to
> `docs/insights-log.md`**; detail goes to **`docs/conventions.md`**. Change this file only to
> add a mechanism or to delete something, and give the mechanism its row in the table below.

## Four rules that keep the model honest

Rule 4 is the general shape; rules 1–3 are the three instances of it that cost the most. What
unites them is a failure MODE, not a subject: **each degrades to silence rather than to an
error**, so every automated check passes and the work reads as correct. The reasoning behind the
cited principles is in `docs/principles.md`.

**1. Deferring a finding means naming the CHANGE that will close it, not an endpoint.** An
endpoint is a hope; a queued change is a commitment. If no such change exists, creating it (a
roadmap row, an issue) is part of deferring.
- **The owner must EXIST, be OPEN, and contain the work.** `gh issue view N --json state,title,body`.
  A stale issue number has no failure mode: it is a correctly formatted integer forever.
- **The rationale expires independently of the work.** Re-read the reason, not just the item.
- **Prefer a condition you would OBSERVE over a phase label.** A phase slips quietly.

**2. A handoff summary is a claim, not a receipt.** Verify what a summary names by **existence**
(`ls`, `grep`, `git log`, `gh pr list`), never by re-reading the summary. Verify the outcome,
never the exit code, never the report. When you write a test for a control, ask what would have to
be true for it to pass **without** the control, and confirm it fails there first (*A passing test
is evidence of nothing until it has failed*).

**3. For every capability, name the process that invokes it IN PRODUCTION.** If the only answer
is "a test", it is not shipped. The check is one `grep` for the callers and the question *"is any
of these a real process?"*. A package, a script and a config key are capabilities too.

**4. For every rule this repo states, name what EXECUTES it.** If the answer is "whoever reads the
file", it is documentation, not a control. Corollaries:
- **A note instructing a future reader is not a mechanism.** Prefer a hook, a check, a constraint.
- **A delegation target is a NAMED THING: check it exists.** Skills, script paths and config keys
  are named things. A missing function throws, but a missing instruction gets interpreted.
- **A guard must iterate the AUTHORITY, not the list it was handed** (*Enumerate the authority*),
  and run the negative case.
- **Ask WHO RECEIVES the result when it fails**, and whether they already look there (*A control
  needs a named addressee*). **A green result must name its SUBJECT.**

### What executes each rule

Rule 4 applied to this file. Most of what is written here is documentation, and that is stated
rather than hidden. One line per row. Add your project's own controls; keep the last row last.

| Rule / convention | What executes it |
|---|---|
| No `--auto`, no `--admin`; a merge has gate evidence for its exact head commit | **`.claude/hooks/pr-merge-guard.sh`** rules 1–2 (blocking); **`checks/merge-guard.sh`** |
| Only a complete run of a clean tree is evidence; two gates on one machine never overlap | **`scripts/ci/run-local.sh`**: the receipt, and the panel's gate lease (`lock-run` or `lease.sh`); **`checks/gate.sh`** |
| Every change PR states its slice | **`pr-merge-guard.sh`** rule 3 (blocking) |
| A journal `## Session N` is not claimed twice | **`pr-merge-guard.sh`** rule 7 (blocking); **`checks/journal.sh`** |
| A session close lands on top of `origin/main`, never on a picture main has moved past | **`pr-merge-guard.sh`** rule 8 (blocking, `BRANCH_SESSION_CLOSE`); **`checks/merge-guard.sh`** |
| An open change has the shape build-change reads and records the owner's approval | **`checks/changes.sh`**; `/propose` stops for the owner |
| An approval covers the design as written; an edit after it voids it | **`checks/changes.sh`** via `.claude/change-approval.sh` (the design hash); **`checks/change-tools.sh`** |
| Every scenario a change adds or modifies is cited by a test, and a merged one stays cited | **`.claude/scenario-trace.sh`**: a `run-local.sh` step and `pr-merge-guard.sh` rule 10 (blocking); **`checks/scenarios.sh`** |
| A build merges finished; an archive holds only a finished change, its cost and its outcome row | **`pr-merge-guard.sh`** rules 9 and 11 (blocking); `/verify-change`; **`checks/merge-guard.sh`** |
| An archive loses no living scenario, keeps superseded wording out, and skips a `NOT-SYNCED.md` capability | **`.claude/archive-change.sh`** (`archive-change` step 1); **`checks/archive-change.sh`** |
| Every registered hook launches from any directory; the panel's cards run what session-start runs | **`checks/hooks.sh`** (the registration table); **`checks/panel-contract.sh`** (and no panel hook checked in) |
| A squash commit names its change, role, model and session | **`pr-merge-guard.sh`** rule 12 (blocking while `provenance.enabled`) |
| A role's model or a trigger it declares (`evals.triggers`: its agent, the marked review prompt) changes only with a passing eval receipt | **`pr-merge-guard.sh`** rule 13 (blocking); **`checks/model-roles.sh`** (the `eval` evidence); **`checks/evals.sh`** |
| A change stays within its budget | **`build-change.js`** stops (`.claude/change-cost.sh`); by hand, **Nothing. You.** |
| No secret is committed, and agents cannot read the project's | **`run-local.sh`** secrets step (gitleaks; fails under CI without it); **`settings.json`** deny list and sandbox; **`checks/settings.sh`**, **`checks/gate.sh`** |
| A dependency release ages before it is proposed; an advisory reaches a reader | **`.github/dependabot.yml`** cooldown; **`dependency-audit.yml`** and its health line; **`checks/supply-chain.sh`** |
| Review a change PER TASK GROUP, not per PR | **`.claude/workflows/build-change.js`** when built through it; **Nothing. You.** by hand (`/apply-change`) |
| The panel offers only what the roadmap allows next | **`.claude/panel-suggest.sh`** over the one parser; **`checks/panel-suggest.sh`** |
| Claude merges only once review has CONVERGED | **`merge-pr`** step 3: a skill step, **no hook checks it** |
| No scripted find-replace against tracked source | **`.claude/hooks/no-blind-source-rewrite.sh`** (blocking) |
| A worktree agent is guarded by the hooks on `origin/main` | **`.claude/hooks/worktree-hook-drift.sh`** (blocking on drift) |
| The status-line focus says what this lane is doing | **`.claude/hooks/focus-staleness.sh`** (a nudge, not a block) |
| A model or effort is chosen in ONE place | **`.claude/checks/model-roles.sh`**: `model-roles.json` against every skill, agent, settings, workflow and lane type |
| An agent that must not edit cannot | Each agent's **`tools:`** line; **`model-roles.sh`** bars write tools from read-only roles. Bash can still write: check `git status` |
| Every insight gets a decision (promote, an instance, or why not) within a few sessions | **`.claude/compound.sh`** via `session-close` step 3b; **`checks/compound.sh`** |
| A scheduled or post-merge failure reaches a reader, naming its run's commit and age | **`.claude/health/*.sh`** via `session-start` (the directory is the list; `lib/health.sh`); **`checks/health.sh`**, **`health-lines.sh`** |
| Work under way is seen on GitHub: branches with no PR, rows queued while their PR is open | **`lib/context.sh`** via the session-start and session-close context; **`checks/session-context.sh`** |
| A module adds only while `MODULES` names it; a broken extension guard rule blocks, never allows | **`lib/modules.sh`** for every host; **`pr-merge-guard.sh`** extension rules; **`checks/modules.sh`** |
| Orphans and clean lane worktrees do not outlive a session; tmux lanes are never killed | **`.claude/machine-quiet.sh`** via `session-close`; **`checks/machine-quiet.sh`** |
| Work that needs the owner at the computer is seen | **`.claude/owner-queue.sh`** via `session-start` and the panel card |
| The change queue is read ONE way, and a malformed row is an error, not a shorter queue | **`roadmap_queue`** (`lib/conf.sh`) over `.claude/roadmap-queue.sh` or `ROADMAP_PARSER`; **`checks/roadmap.sh`** fails a bypass |
| A project's history is never reformatted; records since adoption meet the format | `RECORDS_BASELINE`, `CHANGES_LEGACY` (`lib/records.sh`); **`checks/journal.sh`**, **`adr-numbering.sh`**, **`changes.sh`** |
| An ADR number is never taken twice; every ADR is indexed and names its enforcement | **`checks/adr-numbering.sh`**; `new-adr` numbers from `origin/main` and open PRs |
| A skill's context script and every `.claude/` path a skill or agent names exists | **`checks/skills.sh`** |
| The repo works without clauductor: nothing needs the binary or the panel | **`checks/no-clauductor.sh`**: the gate, the checks, a context script and the status line, `clauductor` off `PATH` |
| A branch is named by its `BRANCH_*` key, never a literal prefix | **`checks/branch-prefixes.sh`**: the model's files and `panel.json` |
| **This file stays a budget, not an archive** | **`.claude/checks/agents-md-budget.sh`** |
| **Everything else in this file and `docs/conventions.md`** | **Nothing. You.** Including all four rules above. |

Run every check with `sh .claude/checks/run.sh`.

## Who decides

**The owner decides** (the role named `OWNER_ROLE` in `.claude/project.conf`):
- a change's proposal and every `design.md` decision;
- ADRs;
- anything irreversible (a production data migration, a deletion, a release);
- anything outward beyond this repo.

**Everything else runs without asking, merging included**, through `merge-pr` once the merge guard
passes and review has converged. Reach the owner with `PushNotification` (or the project's
equivalent) only when blocked on one of the decisions above, and once when a session is done.
Where several people work, each merges only their own PRs. Claude's memory is per machine, so what
another person needs goes in the repo. Detail: `docs/conventions.md`; how-to: `docs/playbook.md`;
models: `.claude/model-roles.json`.

## Essentials every agent applies

Replace this section with your project's few load-bearing rules: the ones a reviewer grades on and
an agent cannot infer from the code. Keep each to two lines and put the detail in
`docs/conventions.md`. For example:

- **Branching.** One change = one branch = one squash PR: `change/<id>` for a capability,
  `fix/<n>-<slug>`, `ops/<name>` otherwise. No stacked PRs. Commit messages in the imperative.
  The main checkout stays on `main`; work happens in worktrees (`.claude/worktrees/<lane>`).
- **The gate** is `GATE_RUN` in `.claude/project.conf`; agents run it through `GATE`.
- **Reuse before you write.** Grep for the concept before adding a function, a query or a
  dependency.
