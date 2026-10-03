# Curtain

The full wrap-up of a lane in one command, with a closing show.

A prototype (OPS-31). It lives here as a standalone plugin so the owner can try it. It reaches
`template/` later, through a proposal.

| Piece | What it is | What it does |
|---|---|---|
| `/curtain` | the skill, `skills/curtain/SKILL.md` | The real work, in one turn: `/merge-pr`, then `/session-close`, then close the lane, then exit. It stops at the first step that stops. |
| the mod | `hooks/register.ts` | Watches the skill and draws the show above the prompt. On the skill's done signal, with the PR confirmed merged, it closes the curtain, plays the bow and **submits `/exit`**, because a skill can't type `/exit`. |
| `/curtain-mod` | a mod command | Plays the closing show alone: the last of the crew, the curtain closing, the bow. **It doesn't exit.** `/curtain-mod cancel` holds a running show at INTERMISSION, and a second `cancel` clears it. |
| `/curtain-mod-demo [off\|small\|medium\|full] [halt]` | a mod command | Plays the whole production with pretend steps of 26 s, 9 s and 4 s, about 50 s in all. It runs nothing, submits nothing, records nothing and **never exits**. A size word overrides the setting for that run; `halt` shows the intermission. |

**Without the mod** (`claude -p`, the VS Code chat panel, a Desktop WSL session, or a session that
didn't load this plugin's hooks), `/curtain` still runs its three steps and ends with
`Type /exit to leave the session.`

## Try it

```bash
claude plugin validate /Users/rich/Development/clauductor/.claude/worktrees/curtain-mod/mods/curtain
claude --plugin-dir /Users/rich/Development/clauductor/.claude/worktrees/curtain-mod/mods/curtain
```

Then, in that session:

- `/curtain-mod-demo`: the whole production in about 50 s, at your size setting
- `/curtain-mod-demo medium` (or `full`, `small`): the same at another size, for this run only
- `/curtain-mod-demo halt`: the intermission, 9 s in
- `/curtain-mod`: the closing beat alone

**Don't type `/curtain` to try it.** It merges the branch's PR, closes the session and exits. A
plugin's skill may be listed as `/curtain:curtain`, because plugin skills are namespaced.

The tests run without a session: `claude plugin test` from this directory.

## The show

The show plays in the band above the prompt: a stage crew of small Clawds strikes the set while
the work runs.

**At the start** the stage is full. At 80 columns there are five crew and six props: a crate, a
chair, a potted plant, a ladder, a spotlight, and a painted flat on wheels. Fewer fit on a narrow
terminal: 4 and 4 at 60 columns, 3 and 3 at 40.

**As the work goes on** each mover takes its props into the nearest wing: it pushes the flat out
in front of it, or walks out ahead of a carried prop. It leaves with its last one. The curtain
comes down over the sky above them. **Near the end** one Clawd is left, sweeping the last spot.
None of this runs ahead of the work: the scene follows the same weighted progress as before,
which never passes a step's boundary until the step completes.

**The finale** (the done signal, or `/curtain-mod`): the curtain closes. One Clawd runs out in
front of it and bows, then `~ fin ~` and `Thank you, goodnight`. About 9 s in all.

**INTERMISSION:** the scene freezes where it is and the frame timer stops. The status row gives the
reason (`INTERMISSION - merge-pr stopped: PR #42 is OPEN, not MERGED`), as do the status line under
the prompt and a transcript line. Nobody bows, nothing further runs, and no `/exit` is sent.

At 80 columns, `small`, halfway through merge-pr:

```
▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
█                   ▐▛▜▌  ▞▚         ▐▛▜▌\   '  ▐▛▜▌  ▛▀▀▜  ▟█   ▐▛▜▌          █
█                   ▘▝▘▝  ▜▛         ▝▘▝▘▒▓.    ▘▝▘▝  ▙▄▄▟  ▐    ▘▝▘▝          █
▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀▀
1/3 merge-pr 4:05 / ~8m
```

**The size setting:**

| `size` | Rows | What it is |
|---|---|---|
| `off` | 0 | No show. `/curtain` still works, and the mod still submits `/exit` on its done signal once the turn ends. `/curtain-mod` and `/curtain-mod-demo` say the show is off. |
| `small` (the default) | 6 | The valance, 3 rows of stage, the floor, and the status |
| `medium` | 9 | The same with 6 rows of stage: more sky for the curtain, and carried props held up |
| `full` | 12 | 9 rows of stage |

Set it in `/config`. The **Curtain size** row is a picker, and a change reloads the mod. Or set
it in `~/.claude/settings.json`; for a plugin loaded with `--plugin-dir`, the key is
`curtain@inline`, and for the local install it's `curtain@curtain-proto`:

```json
{ "pluginConfigs": { "curtain@inline": { "options": { "size": "medium" } } } }
```

It's the plugin's `userConfig` option `size` (in `plugin.json`), which the mod reads as
`register(on, options)`. A missing or unknown value means `small`.

**Nothing is clipped.** A band too short for the size asked for steps down a size: a stage needs
its rows plus one. Below `small`, or narrower than 30 columns, the show is a two-row bar (the
progress in velvet, and the status) with no sprites. The tests check every sprite stays inside
its grid at 80, 60 and 40 columns, at every size and phase, and that no Clawd walks through a
prop or another Clawd.

**What renders in a panel lane** (tmux inside xterm.js):

- **Characters:** only printable ASCII, box drawing and block elements (U+2500 to U+259F).
  xterm.js draws that range itself, cell for cell. I avoided emoji and symbols such as `·`, `°`,
  `✓` and `▸`, whose width a font or tmux may count as two cells and so shift the row. The dust is
  plain `.`, `:`, `'` and a backtick, the broom handle is `/` or `\`, and the separators are ` - `.
- **Colours:** every colour is one of the xterm 256-colour palette's, from the 6×6×6 cube or the
  grey ramp. The panel's tmux gives `xterm-256color` no RGB feature (`hardenArgs` adds only
  `hyperlinks`), so tmux maps truecolour down to 256 colours. Colours already on the palette pass
  through unchanged; the earlier velvet's in-between reds collapsed into one flat red.

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
