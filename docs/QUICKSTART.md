# Clauductor Quickstart Guide

Clauductor gives a project an **operating model** for Claude Code (the files in `template/`) and
a **local panel** over the sessions that run it (`clauductor panel`, `docs/panel.md`). This guide
takes a project from nothing to its first change. The model itself is explained, lane by lane, in
the project's own `docs/playbook.md` (`template/docs/playbook.md` here).

## Prerequisites

- **macOS or Linux**, **git** 2.31+, **Claude Code**
- **jq** (the hooks and checks read JSON with it), **gh** (PRs, merges, issues), **tmux** (the
  panel's lanes), **Go** (to build clauductor; `install.sh` installs Go and tmux if missing)
- Optional: **python3** (machine-quiet's live-session check; without it, it removes no worktree)

## Install clauductor

```bash
git clone https://github.com/rfhayn/clauductor.git ~/clauductor
cd ~/clauductor && ./install.sh
source ~/.zshrc   # or ~/.bashrc
```

## Adopt the operating model

**A new project:**

```bash
clauductor init ~/Development/my-app
cd ~/Development/my-app
```

**An existing repository:**

```bash
cd ~/Development/existing-project
clauductor install --dry-run   # preview
clauductor install
```

**A repository that already runs its own operating model** (its own skills, hooks or
`AGENTS.md`, grown in place) is refused: `install` and `update` list the files they would
overwrite or add, and change nothing. Such a repository needs none of the template to use the
panel. Run `clauductor panel init`, `trust` and `add` there instead, and copy any single piece
of the template by hand. `--force` installs anyway, overwriting. `install` and `init` leave
`.claude/clauductor-template` behind, and that marker lets later installs and updates act.
`clauductor diff` (`--json`, `--path`, `--exit-code`) compares any repository with the template
file by file, and settings.json key by key, without the guard: it only reads.

**A repository that runs clauductor's old model** (the lock-based skills such as `claim`,
`spawn` and `supervisor`, the hooks that call the binary, or settings registering them) is
clauductor's, and is installed over without `--force`. It is recognised from tracked files, so a
fresh clone counts too (the old runtime state, `orchestration/`, is gitignored). The old model's
files that the current one replaced are listed under OLD MODEL and offered for removal; `--prune`
removes them without asking. Only those go: a project's own skill, a file the old model did not
ship, or an old agent the project edited is never touched. `settings.json` loses every hook
registration that runs a `.claude/hooks/` script that will not exist, and the old model's hooks.

**Which template.** `init`, `install`, `update` and `diff` print the template they read on their
first line. It comes from `--template-dir`, else `CLAUDUCTOR_FRAMEWORK`, else next to the binary
(a release ships its `template/` beside it, unpacked to `~/.local/share/clauductor/<version>/`),
else the checkout the binary was built from. Its `.template-version` must match `clauductor
version`; a mismatch is refused (`--template-dir` uses it anyway, with a warning). There is no
guessing from `~/Development/clauductor` any more.

`install` sorts the template's files into three tiers:

- **Framework** (skills, hooks, checks, the workflow, the model's scripts such as
  `scripts/ci/run-local.sh`): always installed, so re-running `install` brings them up to date.
  The gate scripts go where `GATE_RUN`, `GATE` and `GATE_STEPS` in `project.conf` say (a
  `GATE_RUN` with another file name is your own runner, and the template's is left out).
- **Settings** (`.claude/settings.json`): merged key by key. The model's hooks, status line, deny
  list and sandbox entries are brought up to date (the gate's paths from `project.conf`); your
  model, effort, env, skill overrides, plugins, and your own permissions and hooks are kept.
  Every disagreement is reported, and `--dry-run` prints the diff.
- **Project-owned** (`AGENTS.md`, `.claude/project.conf`, `.claude/model-roles.json`,
  `.clauductor/panel.json`, `scripts/ci/steps.sh`, agents, docs, the configure-first skills):
  created only when missing, never overwritten. `.clauductor/panel.json` is written with the
  branch prefixes `project.conf` sets (`BRANCH_CHANGE`, `BRANCH_FIX`, `BRANCH_OPS`).
  `model-roles.json` gains the keys a newer template added, without a value changing.
- **Merged**: `CLAUDE.md` gains an `@AGENTS.md` import line; `.gitignore` gains the template's
  lines (`.claude/worktrees/`, `.claude/settings.local.json`, ...).

## Configure it (once)

```bash
claude
/start-project
```

`/start-project` walks through, and skips what is done:

1. `.claude/project.conf`: the project's name and slug, its status-line mark, **who decides**
   (`OWNER_ROLE`, `OWNER_NAME`), the main branch, the insight areas.
2. `.clauductor/panel.json`: the name and tmux socket to match. Then **you** run
   `clauductor panel trust` (a config's commands run only once you trust it).
3. `scripts/ci/steps.sh`: your lint, typecheck and test commands; the gate runs them, after its
   own two steps: the scenario trace (`TEST_GLOBS` in `project.conf` says where your tests are)
   and a secret scan (install `gitleaks`; locally the step says SKIPPED without it, under CI it
   fails).
4. The formatter hook, AGENTS.md's *Essentials*, the commit and provenance trailers in
   `model-roles.json`, and the ecosystems in `.github/dependabot.yml`.
5. The first real rows of `docs/roadmap.md`.
6. The optional modules: OpenSpec, and the claude.ai review page.

Then `sh .claude/checks/run.sh` must pass; it is also the gate's first step.

## Daily use

```bash
clauductor panel        # the local panel: lanes, the gate queue, the owner queue
claude                  # in the main checkout (it stays on main)
/session-start          # orient; the session then runs hands-off
```

From the panel's **New lane**, the templates offer what the roadmap allows next:

| Template | Branch | First prompt |
|---|---|---|
| Propose the next roadmap row | `change/<id>` | `/propose <id>`: drafts the change and stops for your approval |
| Build an approved change | `change/<id>` | `/build-change {"change": "<id>"}`: per task group, build, gate, independent review, commit |
| Fix an issue | `fix/<n>-<slug>` | fix from the code as built, gate, `/merge-pr` (the fast path: no proposal) |
| Ops task | `ops/<name>` | waits for the task (the fast path too) |

A diff that fits in one sentence takes the fast path, with no proposal. A change to what a user can
do is proposed: with a risk tier (it picks the build's models), a budget if its roadmap row has
one, how you will know it worked, and scenarios whose IDs (`[AUTH-2-S1]`) its tests cite. You
approve it; `build-change` builds it; `/verify-change` and the merge guard hold it to all of that;
`/archive-change` records its cost and queues the check of its outcome. `changes/README.md` has the
format, and `.claude/examples/` a complete example.

Sessions merge their own PRs through `/merge-pr` once the gate has a receipt for the head commit
and review has converged; `pr-merge-guard` blocks anything else. You are asked only for decisions
that are yours (proposals, designs, ADRs, deploys) by notification, and what needs you at the
computer waits in the owner queue. End with `/session-close`.

## Updating

```bash
cd ~/clauductor && git pull && ./install.sh
cd ~/Development/my-app && clauductor update --dry-run   # what would change, settings diff included
cd ~/Development/my-app && clauductor install   # refreshes the framework tier, merges settings.json
```

`update` also offers what the template added to project-owned files, and overwrites none of
them: new files (a health line, `.claude/evals/`, a doc) are listed and created only if missing,
after a yes or with `--create-missing`; `model-roles.json` gains the template's new keys (no
existing value changes); the rows the template's AGENTS.md table gained are printed as a
suggestion (AGENTS.md is yours and has a byte budget, so nothing is applied); and
`.clauductor/panel.json` is checked against the branch keys. Project settings never live in a
framework file: attribution and provenance are `model-roles.json`'s and the branch prefixes are
`project.conf`'s, and build-change reads both at run time (`.claude/project-config.sh`), so
`update` sees `build-change.js` as the template's.

## Troubleshooting

- **A hook blocks everything with "jq is not installed"**: install jq; the hooks fail closed
  without it for the commands they police.
- **The merge guard says there is no evidence**: run `scripts/ci/gate.sh` with no flags on the
  committed head; `--quick` and a dirty tree write no receipt it accepts.
- **The merge guard wants provenance trailers**: write the squash body to a file ending in
  `Change:`, `Agent-Role:`, `Model:` and `Session:` lines and merge with `--body-file`
  (`merge-pr` step 4), or set `provenance.enabled` to false in `model-roles.json`.
- **The gate fails at "scenario trace"**: a scenario is cited by no test. Name its ID in the test
  that asserts it, or add a `(manual: <reason>)` line naming it to the change's `tasks.md`.
- **A command fails inside the Bash sandbox**: add the host to `sandbox.network.allowedDomains`,
  or the command to `sandbox.excludedCommands`, in `.claude/settings.local.json`: these lists
  merge with the project's, and an install never overwrites the local file.
- **A worktree agent is refused by worktree-hook-drift**: put the main checkout back on `main`
  and pull.
- **The panel's cards and suggestions show nothing**: `clauductor panel trust`, and see
  `docs/panel.md`, *Troubleshooting*.
