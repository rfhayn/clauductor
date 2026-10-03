---
name: curtain
description: "The full wrap-up of a lane in one command: /merge-pr, then /session-close, then close the lane, then exit the session. Stops at the first step that stops. With the Curtain mod loaded, a closing show plays above the prompt and the mod submits /exit after the bow; without it, the skill ends with 'type /exit'. TRIGGER only when the user types /curtain."
disable-model-invocation: true
---

# Curtain: the full wrap-up of a lane

Four steps, in order, **in this one turn**. Each starts only once the one before it has
**succeeded**, checked by what you can observe rather than by what a skill reported. At the first
step that stops, stop: report which step stopped and why, and leave the session open.

**Stay in this turn from start to finish.** If a step runs something in the background (the
gate, a reviewer), wait for it inside the turn. Do not end your turn to wait. The Curtain mod reads
a turn that ends without the done signal as a stop, and holds the show at INTERMISSION.

## The done signal

The Curtain mod (this plugin's hooks module) registers a tool, `mcp__curtain__curtain_call`, with
one argument, `cue`. It is the mod's only way to learn how the skill is going:

- `cue: "lane"`: session-close has succeeded. The mod confirms merge-pr and session-close itself.
- `cue: "exit"`: every step has succeeded. The mod confirms the PR is merged, closes the curtain,
  plays the bow, and then **submits `/exit` for you**. A skill cannot type `/exit`, so the exit is
  the mod's to perform.

**Read the tool's result each time.** If it starts `Curtain: INTERMISSION`, stop right there and
report what it says. Do not call the tool again.

**Without the mod**, the tool is not in your tool list: `claude -p`, the VS Code chat panel, a
Desktop WSL session, or any session that did not load the plugin's hooks module. Then skip both
calls, and end step 4 with the line `Type /exit to leave the session.`

## Steps

### 0. Know the PR

`gh pr view --json number,state,url,headRefName`. Note the number: it is what step 1 has to merge.
No PR for this branch: **stop**. There is nothing for `/curtain` to land.

### 1. `/merge-pr`

Run the `merge-pr` skill (the Skill tool, `skill: "merge-pr"`) and follow it all the way.

**Succeeded when** `gh pr view <number> --json state` reads `MERGED`. Use the number from step 0:
merge-pr may have left the branch. Anything else stops `/curtain`: a guard blocked, review did not
converge, the gate stayed red, merge-pr stopped to notify the owner.

### 2. `/session-close`

Run the `session-close` skill (the Skill tool, `skill: "session-close"`) and follow it all the
way, including landing its own close PR.

**Succeeded when** its close PR (the branch `ops/session-<N>-close`, `BRANCH_SESSION_CLOSE` in
`.claude/project.conf`) reads `MERGED`:
`gh pr list --state merged --limit 30 --json number,headRefName,mergedAt`. If session-close found
nothing to record and opened no close PR, that is a stop: say so, and leave the exit to the owner.

Then, if `mcp__curtain__curtain_call` is in your tool list, call it with `{"cue": "lane"}` and read
its result.

### 3. Close the lane

`printenv CLAUDUCTOR_LANE`:
- **Set**: this session is a panel lane. Say: `Close lane <name> from the panel.` There is no
  command for it yet: the roadmap row PANEL-34 adds a panel-authenticated `clauductor lane close`,
  and this step becomes that command when it lands.
- **Unset**: say `No panel lane to close.`

### 4. Exit

- **With the mod** (`mcp__curtain__curtain_call` is in your tool list): call it with
  `{"cue": "exit"}`. If the result confirms, end your turn at once with a one-line summary
  (`Curtain: #<n> merged, session closed (#<m>), lane <name> to close from the panel.`). The mod
  then closes the curtain, plays the bow, and submits `/exit`.
- **Without the mod**, or if the result does not confirm: end with the summary line and then
  `Type /exit to leave the session.`

## Rules

- **Never skip a step, and never reorder them.** `/session-close` runs only after the PR merged.
- **Never call `cue: "exit"` unless steps 1 to 3 all succeeded.**
- **Never force a step through**: no `--admin`, no `--auto`, no weakening a check. A step that
  stops is the result, and the owner reads it.
- **Never type or suggest `/exit` while a step has stopped.** The session stays open for the owner.
