---
name: start-project
model: opus
effort: high
description: "First-time setup of this project's operating model: fill .claude/project.conf and the matching panel config, the gate's steps, the formatter, AGENTS.md's essentials, the commit trailer, the first roadmap rows, and the optional modules; then run the checks. Skips what is already configured. TRIGGER when the user says 'start project', 'set up the project', 'configure the operating model', 'first time setup', 'onboard'."
---

> **Installed as the clauductor plugin.** If `.claude/project.conf` is missing, run
> `/clauductor:init` first: it scaffolds this repository's own files. The scripts, hooks, skills and
> agents named below are the plugin's (`${CLAUDE_PLUGIN_ROOT}`, read-only); the files under `.claude/`
> that this setup edits (project.conf, model-roles.json, settings.json) are the repository's.

# Start a project: configure the operating model

Walk through each step with the user, skipping what is already done. Read `docs/playbook.md` first
if you have not: it explains what each piece is for. Work on an `ops/setup` branch; land it with
`merge-pr` at the end.

## Context
- Config: !`grep -E '^[A-Z_]+=' .claude/project.conf`
- Gate steps configured: !`grep -cE '^[[:space:]]*step ' scripts/ci/steps.sh`
- Roadmap: !`sh ${CLAUDE_PLUGIN_ROOT}/roadmap-queue.sh --check 2>&1 | head -3`

If those lines show as literal text, run the commands yourself.

## Steps
1. **Identity and who decides** (`.claude/project.conf`): `PROJECT_NAME`, `PROJECT_SLUG`
   (`[a-z0-9-]`), `STATUS_MARK` (one character that tells this repo's status line apart from
   others), `OWNER_ROLE` (the word for whoever approves designs: "owner", "founder", "lead") and
   `OWNER_NAME`, `MAIN_BRANCH`, `INSIGHT_AREAS` (5–10 coarse buckets for the insights log).
2. **The panel config** (`.clauductor/panel.json`): set `name` to `PROJECT_NAME` and
   `tmux_socket` to `PROJECT_SLUG`; make `lanes` use the `BRANCH_*` prefixes and `MAIN_BRANCH`, and
   `base` `origin/<MAIN_BRANCH>`. Then tell the user to run `clauductor panel trust` themselves
   (a config's commands run only once its owner has trusted it; never trust on their behalf).
3. **The gate** (`scripts/ci/steps.sh`): ask for the lint, typecheck and test commands, and any
   slow suite for the full gate only; write each as a `step`. Keep the "process checks" step.
   Ask whether to use the clean room (`GATE_CLEAN_ROOM="archive"`: stronger, but the steps must
   install dependencies) and whether a remote CI workflow counts as evidence
   (`GATE_REMOTE_WORKFLOW`). Set `TEST_GLOBS` in `.claude/project.conf` if the tests are not
   in one of the default layouts (it tells the scenario trace where tests live), and suggest
   installing `gitleaks` (the gate's secret scan skips without it locally, and fails under CI).
   If the project already has a gate runner of its own, set `GATE_RUN` (and `GATE`) to it instead,
   and have it source `scripts/ci/lib/steps.sh` and call `model_steps` (the scenario trace and the
   secret scan); set `GATE_QUICK_FLAGS` to its quick-run flags. A lease of its own is tested with
   `sh scripts/ci/lease-conformance/run.sh <impl>`.
   Then run `scripts/ci/gate.sh --quick` and show the result.
4. **The formatter** (`FORMAT_CMD`, `FORMAT_EXT`), if the project has one.
4b. **Least privilege** (`.claude/settings.json`, kept current by `clauductor install`): Claude may
   not read `.env` files, keys, `~/.ssh` or `~/.aws`, force-push, or kill a tmux server, and Bash
   runs in the sandbox with a network allowlist (git, gh, tmux, clauductor, docker and the gate run
   outside it). Tell the user. If one of the project's tools fails in the sandbox, or needs a host,
   loosen it in **`.claude/settings.local.json`** (`sandbox.excludedCommands`,
   `sandbox.network.allowedDomains`: these lists merge with the project's, and an install never
   overwrites the local file), not in `settings.json`.
5. **AGENTS.md "Essentials"**: replace the examples with this project's few load-bearing rules
   (two lines each), and fill the *(yours)* sections of `docs/conventions.md`. Keep AGENTS.md under
   its byte budget (`clauductor-model checks/run.sh agents-md-budget`; the ceilings are
   `AGENTS_MD_MAX_BYTES` and `AGENTS_MD_MAX_ROW`). If the playbook lives elsewhere (or is an HTML
   page), set `PLAYBOOK`. If the project's change records carry sections of their own, list them in
   `CHANGE_RECORD_EXTRA`.
6. **The commit trailers** (`.claude/model-roles.json`): `attribution` names the model you run in
   `trailer`, or set `enabled` to false if this project keeps commits unattributed; `provenance`
   (the `Change:`, `Agent-Role:`, `Model:`, `Session:` trailers `pr-merge-guard` requires on a
   squash) is on by default, off with `enabled: false`. Mirror both in
   `${CLAUDE_PLUGIN_ROOT}/workflows/build-change.js` (`ATTRIBUTION_DEFAULT`, `PROVENANCE`; the check compares
   them). Update `.prices` if the list prices have changed. Change a
   role's model or effort only in `model-roles.json`, then fix what `checks/model-roles.sh` names.
7. **The first roadmap rows** (`docs/roadmap.md`): replace the example Phase 1 with the real first
   phase: its exit criterion, its owner, and 2–5 rows, each a change id and what a user can then
   do. Then `clauductor-model roadmap-queue.sh --check`.
8. **Health lines** (`.claude/health/`): delete the GitHub ones if the project does not use
   GitHub Actions; add any the project needs (a migration ledger, a backup's age). Uncomment the
   ecosystems this project uses in `.github/dependabot.yml` (each keeps its `cooldown`).
9. **Optional modules and the local layer**: OpenSpec, the claude.ai review page, the premise
   check and the write-surface advisory (each `${CLAUDE_PLUGIN_ROOT}/modules/<name>/README.md`) are off by
   default; turn one on by naming it in `MODULES` (`${CLAUDE_PLUGIN_ROOT}/modules/README.md`). What the project adds of its own
   (an extra guard rule, a context section, a health line, a skill step, a conflict row) goes in
   `.claude/local/` (its README has each contract), never in a framework file the next
   `clauductor update` overwrites. `clauductor-model extensions.sh list` shows what is on.
10. **Verify**: `clauductor-model checks/run.sh` must pass. Then commit, open the PR, and land it with
    `merge-pr` (the gate must pass in full first: `scripts/ci/gate.sh`).

## Rules
- Ask; do not invent. A placeholder left in place is better than a guessed command.
- Never trust the panel config, grant permissions or change `~/.claude` for the user.

## Project steps

What this project's enabled modules and its local layer (`.claude/local/skills/start-project/`) add to this
skill. Follow them as part of the steps above:

!`sh ${CLAUDE_PLUGIN_ROOT}/extensions.sh fragments start-project`
