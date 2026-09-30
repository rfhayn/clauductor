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

`install` sorts the template's files into three tiers:

- **Framework** (skills, hooks, checks, the workflow, the model's scripts such as
  `scripts/ci/run-local.sh`, `settings.json`): always installed, so re-running `install` brings
  them up to date.
- **Project-owned** (`AGENTS.md`, `.claude/project.conf`, `.claude/model-roles.json`,
  `.clauductor/panel.json`, `scripts/ci/steps.sh`, agents, docs, the configure-first skills):
  created only when missing, never overwritten.
- **Merged**: `CLAUDE.md` gains an `@AGENTS.md` import line; `.gitignore` gains
  `.claude/worktrees/`.

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
3. `scripts/ci/steps.sh`: your lint, typecheck and test commands; the gate runs them.
4. The formatter hook, AGENTS.md's *Essentials*, the commit trailer in `model-roles.json`.
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
| Fix an issue | `fix/<n>-<slug>` | fix from the code as built, gate, `/merge-pr` |
| Ops task | `ops/<name>` | waits for the task |

Sessions merge their own PRs through `/merge-pr` once the gate has a receipt for the head commit
and review has converged; `pr-merge-guard` blocks anything else. You are asked only for decisions
that are yours (proposals, designs, ADRs, deploys) by notification, and what needs you at the
computer waits in the owner queue. End with `/session-close`.

## Updating

```bash
cd ~/clauductor && git pull && ./install.sh
cd ~/Development/my-app && clauductor install   # refreshes the framework tier only
```

## Troubleshooting

- **A hook blocks everything with "jq is not installed"**: install jq; the hooks fail closed
  without it for the commands they police.
- **The merge guard says there is no evidence**: run `scripts/ci/gate.sh` with no flags on the
  committed head; `--quick` and a dirty tree write no receipt it accepts.
- **A worktree agent is refused by worktree-hook-drift**: put the main checkout back on `main`
  and pull.
- **The panel's cards and suggestions show nothing**: `clauductor panel trust`, and see
  `docs/panel.md`, *Troubleshooting*.
