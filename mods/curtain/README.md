# Curtain

The full wrap-up of a lane in one command, with a closing show.

A prototype (OPS-31). It lives here as a standalone plugin so the owner can try it. It reaches
`template/` later, through a proposal.

| Piece | What it is | What it does |
|---|---|---|
| `/curtain` | the skill, `skills/curtain/SKILL.md` | The real work, in one turn: `/merge-pr`, then `/session-close`, then close the lane, then exit. It stops at the first step that stops. |
| the mod | `hooks/register.ts` | Watches the skill and draws the show above the prompt. On the skill's done signal, with the PR confirmed merged, it closes the curtain, plays the bow and **submits `/exit`**, because a skill can't type `/exit`. |
| `/curtain-mod` | a mod command | Plays the closing show alone: the sweep, the curtain falling, the bow. **It doesn't exit.** `/curtain-mod cancel` holds a running show at INTERMISSION, and a second `cancel` clears it. |
| `/curtain-mod-demo [small\|medium\|full] [halt]` | a mod command | Plays the whole show with pretend steps of 26 s, 9 s and 4 s, about 50 s in all. It runs nothing, submits nothing, records nothing and **never exits**. A size word overrides the setting for that run; `halt` shows the intermission. |

**Without the mod** (`claude -p`, the VS Code chat panel, a Desktop WSL session, or a session that
didn't load this plugin's hooks), `/curtain` still runs its three steps and ends with
`Type /exit to leave the session.`

## Try it

```bash
claude plugin validate /Users/rich/Development/clauductor/.claude/worktrees/curtain-mod/mods/curtain
claude --plugin-dir /Users/rich/Development/clauductor/.claude/worktrees/curtain-mod/mods/curtain
```

Then, in that session:

- `/curtain-mod-demo`: the whole show in about 50 s, at your size setting
- `/curtain-mod-demo full` (or `medium`): the same at another size, for this run only
- `/curtain-mod-demo halt`: the intermission, 9 s in
- `/curtain-mod`: the closing beat alone

**Don't type `/curtain` to try it.** It merges the branch's PR, closes the session and exits. A
plugin's skill may be listed as `/curtain:curtain`, because plugin skills are namespaced.

The tests run without a session: `claude plugin test` from this directory.

## The show

The show plays in the band above the prompt, in two parts.

**While the work runs: a two-row strip, by default.** Across the top row, a velvet-red valance with
a gold hem fills the bar as the work goes on. Gold ticks on the rail mark where each step ends. On
the second row are the step, its number, and its time against the estimate. Beside them, a small
Clawd sweeps his lane with a puff or two of dust. At 80 columns:

```
▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔▔
1/3 merge-pr 3:12 / ~8m      ·  ∘ ▓╱▗▛█▜▖
```

When the status needs more room (`2/3 session-close 3:00 / ~4m`), it takes it. When the band is too
narrow for Clawd's lane, the status keeps the row to itself.

**The finale: the full stage.** When the done signal arrives, the strip expands to the full
stage for about 9 s. The curtain falls the rest of the way over Clawd as he sweeps the boards,
Clawd steps out and bows, then `~ fin ~` and `Thank you, goodnight`. `/curtain-mod` gets the
same finale.

**The size setting** chooses how much room the show takes while the work runs. The finale is
always the full stage.

| `size` | While the work runs |
|---|---|
| `small` (the default) | the two-row strip |
| `medium` | half the stage: the valance, three rows of stage, the floor and the status (6 rows) |
| `full` | the whole stage: six rows of curtain over Clawd (9 rows) |

Set it in `/config`. The **Curtain size** row is a picker, and a change reloads the mod. Or set
it in `~/.claude/settings.json`; for a plugin loaded with `--plugin-dir`, the key is
`curtain@inline`:

```json
{ "pluginConfigs": { "curtain@inline": { "options": { "size": "medium" } } } }
```

It's the plugin's `userConfig` option `size` (in `plugin.json`), which the mod reads as
`register(on, options)`. A missing or unknown value means `small`. A band too small for the size
asked for gets the next one down: a stage needs 28 columns, and its rows plus one.

On the terminal the drawing is one `Raster`, repainted with `$.ui.blit` at about 6.7 frames a
second (150 ms), so a frame costs no render pass. Other surfaces get the same frame as coloured
`Text`, redrawn about twice a second.

**The timing follows the real work:**

- The curtain's travel is split into three stretches, one per step. Each is weighted by that
  step's expected duration.
- Within a stretch, the curtain eases toward its end as `0.97 × (1 − e^(−2t/expected))`. It's
  about 86% of the way at the expected time and creeps after that, but it **never crosses the
  boundary until the step completes**. Then it glides to the boundary.
- When a step completes, its real duration goes into the mod's own store (`$.store`, the plugin's
  JSON file under `~/.claude/plugins/store/`). The keys are `samples:merge-pr`,
  `samples:session-close` and `samples:lane`, each holding the last 5 runs. The expected duration
  is their mean. First-run defaults: merge-pr 8 min, session-close 4 min, lane 20 s.

**The end:** after the finale, the mod submits `/exit`, but only for a real `/curtain` whose done
signal arrived and whose turn has ended.

**INTERMISSION:** the show freezes where it is, in the layout it was in. A halt never expands
the strip. The frame timer stops, and the strip's second row gives the reason
(`INTERMISSION · merge-pr stopped: PR #42 is OPEN, not MERGED`), as do the status line under the
prompt and a transcript line. Nothing further runs, and no `/exit` is sent.

## How the mod knows how the skill is going

| Moment | The event the mod watches | What it checks |
|---|---|---|
| The curtain rises | `skill.prompt` for `curtain` (typed, or through the Skill tool). Matched as `curtain` or `curtain:curtain`. | Records the branch's PR: `gh pr view --json number,state` |
| merge-pr → session-close | `skill.prompt` for `session-close` | `gh pr view <n> --json state` reads `MERGED` |
| session-close → lane | the skill calls the mod's tool `mcp__curtain__curtain_call` with `{"cue": "lane"}` | A `…session-<N>-close` PR merged after the curtain rose (`gh pr list --state merged`). session-close's last act is landing that PR. |
| done | the same tool with `{"cue": "exit"}` | The PR is still `MERGED`, and every earlier check has passed |
| the exit | `turn.complete` on the main loop, after the done signal | Then the curtain closes, the bow plays, and the mod calls `$.command.run({ command: 'exit' })` |

The tool's result tells Claude to go on or to stop (`Curtain: INTERMISSION. …`), so the mod's
checks bind the skill too. If the turn ends without the done signal (a step stopped, the turn was
interrupted, Claude ended the turn to wait on something), the show holds at INTERMISSION.

## Where the API was uncertain

Everything here uses calls and events the mods docs or this build's type declarations
(Claude Code 2.1.286) show. These are the places where the docs don't settle the behaviour:

- **`$.command.run({ command: 'exit' })`.** It's documented as running "a slash command as if
  the person typed `/command args`", and it's what the mod uses rather than `$.prompt.submit`. A
  submitted prompt is documented as text the model reads, not as a slash command. Nothing says
  whether a built-in that ends the process, like `/exit`, runs this way. If the call rejects,
  the status line and transcript say `type /exit`. The tests stub it, so only a live session
  shows it working.
- **The skill's name in `skill.prompt`.** It's matched as `curtain` or `curtain:curtain`, because
  a plugin skill's namespacing isn't documented for this event.
- **The band's `requestId`.** The blit uses the `requestId` the render hook was given. If a blit
  is refused, the mod falls back to a redraw.
- **Text colours off the terminal.** Only named colours are used (`red`, `yellow`, `gray`).
- **The `pluginConfigs` shape for `size`.** The types say values sit under
  `pluginConfigs[<plugin>].options`, keyed `curtain@inline` for `--plugin-dir`. The settings page
  shows them directly under the plugin's id. The `/config` picker avoids the question.

## Requirements

Claude Code 2.1.287 or later, where mods load by default. Validated and tested with the installed
2.1.288.
