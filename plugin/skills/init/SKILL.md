---
name: init
model: opus
effort: high
description: "Set up the clauductor operating model in this repository from the plugin: scaffold the project-owned files (AGENTS.md, .claude/project.conf, model-roles.json, settings.json, docs/, changes/, specs/, scripts/ci/, .clauductor/panel.json) without overwriting any, mark the repository so the plugin's hooks act here, then hand over to start-project. TRIGGER when the user says 'clauductor init', 'set up clauductor here', 'add the operating model to this repo', or start-project finds no .claude/project.conf."
---

# Set up the operating model from the plugin

The plugin carries the framework: skills, agents, hooks, the build-change workflow and the
process checks, all read-only in Claude Code's plugin cache. What the project owns and edits has
to live in the repository; this skill puts it there. It never overwrites a file.

## What init would do here

!`sh ${CLAUDE_PLUGIN_ROOT}/scaffold.sh --dry-run`

If that block is empty or shows an error, run `sh ${CLAUDE_PLUGIN_ROOT}/scaffold.sh --dry-run`
yourself and read the result before step 1.

## Steps

1. **Read the plan above.** On `REFUSED`, stop and show the owner the refusal as it is. Pass
   `--force` only when the owner says, in this session, to add the model beside a model the
   repository already runs. Never work around a refusal because of `clauductor install`
   (`.claude/clauductor-template`): one repository runs the model one way.
2. **Scaffold**: `sh ${CLAUDE_PLUGIN_ROOT}/scaffold.sh`. It creates the missing files, merges
   `.gitignore`, `CLAUDE.md` and `.claude/settings.json`, and writes `.claude/clauductor-plugin`.
3. **Configure**: run `/clauductor:start-project`. It fills `.claude/project.conf`, the gate's
   steps and AGENTS.md's essentials.
4. **CI**: the gate's process-checks step (`scripts/ci/steps.sh`) runs the plugin's checks
   through `scripts/ci/clauductor-model.sh`. In CI (`$CI` set), where no plugin is installed, it
   clones `github.com/rfhayn/clauductor` at tag `v0.1.0` (override with `CLAUDUCTOR_REF`) into
   `~/.cache/clauductor` once; cache that directory between runs. If it cannot, the step fails, on
   purpose.
5. **Commit** the scaffold on a branch, with the owner's go.
