# `clauductor panel` — a local web panel over your Claude sessions

`clauductor panel` serves a live dashboard of every Claude Code session working in one project:
which lanes (worktrees) have a session, whether each is busy, waiting or idle, its context %,
its running subagents, what needs you, the account quota, open PRs, and any cards the project
defines.

From v1 it also **runs lanes**: each lane is an interactive `claude` in its own tmux session,
with a terminal embedded in the page. You start, stop, interrupt, restart and resume lanes from
the browser, and a launchd login agent keeps the panel running with no terminal open. You can
work from the browser alone.

It is **standalone**. It does not need `clauductor install`, the template, the skills, the
SQLite database or file locks. It reads only Claude Code's own signals, plus git and `gh`:

| Source | How | Gives |
|---|---|---|
| HTTP hooks | pushed to `POST /hook` | prompt submitted, turn stopped, subagent start/stop, notifications, session end |
| Status line | the project's status-line script copies its stdin to `POST /status` | context %, 5-hour and 7-day quota, est. cost |
| `claude agents --json [--cwd <dir>]` | polled every 2 s, every 5 s while hooks flow | which sessions exist, busy / waiting / idle |
| `claude --version` | at start, then every 10 min | whether the version-pinned heuristics apply |
| queue leases | read every 1 s from the git common dir | who holds the gate, who waits |
| `git worktree list --porcelain` | polled every 10 s, and within ~2 s of a worktree being added or removed | lanes, and the branch of each |
| `gh pr list` | polled every 60 s | open PRs and their checks |
| project cards | per card: on a file change or an interval | anything the project prints |
| `tmux -L <socket> list-panes -a` | polled every 2 s, and right after a lane action | which lanes run, and whether their program exited |
| the lane registry | in memory, re-read from disk every 30 s | which lane owns which Claude session id, where, as which type |

Keeping the panel current costs **no model tokens**. It never reads transcripts.

## Run it

```bash
clauductor panel                              # project = git toplevel of the current directory
clauductor panel --project ~/Development/app  # or name it
clauductor panel --config /tmp/panel.json     # use a config outside the repo
clauductor panel --port 4393 --no-open        # print the URL instead of opening a browser
clauductor panel --uninstall-hooks            # remove the panel's hooks and exit
clauductor panel --trust-config               # trust panel.json as it is now, then run
clauductor panel trust [--project p]          # trust panel.json as it is now (a running panel follows)
clauductor lock-run [--lane id] [--ttl 10m] <lockdir> -- <cmd…>   # run a command through a queue

clauductor panel install --project ~/Development/app [--config <file>] [--port 4393] [--app]
clauductor panel open                         # open the installed panel in the browser
clauductor panel rotate-token                 # replace the installed panel's token
clauductor panel uninstall                    # stop and remove the login agent
```

The panel watches one project per run. Stop a hand-started panel with Ctrl-C. Stopping the
panel never stops a lane: lanes belong to tmux.

## Configuration: `.clauductor/panel.json`

The project keeps its config at `<project>/.clauductor/panel.json` (or pass `--config`). Unknown
keys are an error, so a misspelt key fails loudly.

```json
{
  "name": "My Project",
  "lanes": {
    "change/": "build",
    "fix/": "fix",
    "ops/": "ops",
    "main": "orchestrator"
  },
  "cards": [
    {
      "id": "founder-queue",
      "title": "Founder queue",
      "command": ["bash", ".claude/founder-queue.sh"],
      "refresh": "watch:docs/founder-queue.md"
    },
    {
      "id": "roadmap-queue",
      "title": "Roadmap queue",
      "command": ["sh", "-c", "node infra/roadmap-queue.mjs --json | jq '[.[] | select(.open) | {title: .change, status, owner}]'"],
      "refresh": "interval:300"
    }
  ]
}
```

| Key | Type | Meaning |
|---|---|---|
| `name` | string, required | Shown in the top bar. |
| `lanes` | object: branch rule → lane type | A rule ending in `/` is a prefix (`"change/"` matches `change/add-x`, shown as `add-x`). A rule ending in `*` is a prefix without the star (`"change/propose-*"`). Any other rule matches one branch exactly (`"main"`). The longest matching rule wins. An unmatched branch is `other`; a detached HEAD is `detached`. |
| `cards` | array | Commands whose output renders as a card in the right column. |
| `cards[].id` | string, required | `[a-z0-9][a-z0-9_-]*`, unique. |
| `cards[].title` | string | Card heading. |
| `cards[].command` | array of strings, required | argv, run in the project root **without a shell**. Use `["sh", "-c", "..."]` if you want one. 30-second timeout. |
| `tmux_socket` | string, default `clauductor` | The panel's own tmux server (`tmux -L <name>`). Lanes never mix with your own tmux sessions. `[A-Za-z0-9_-]{1,64}`. |
| `worktree_dir` | string, default `.claude/worktrees` | Where a new lane's worktree is created: relative to the project root and inside it, or absolute. |
| `base` | string, default `origin/main` | What a new lane's branch starts from. `git fetch` runs first; if it fails, the lane still starts and the page says so. |
| `lane_types` | object: lane type → `{model, effort}` | Launch options per lane type, passed as `claude --model <m> --effort <e>`. Each value is one argv element, `[A-Za-z0-9][A-Za-z0-9._[\]-]*`. |
| `version` | number, optional | The config schema version: 1 or 2. Anything else is refused. |
| `templates` | array | Lane recipes offered by **+ LANE** (see *Lane templates*). |
| `templates[].id` | string, required | `[a-z0-9][a-z0-9_-]*`, unique. |
| `templates[].title` | string | Shown in the dialog. |
| `templates[].lane_type` | string, required | One of the config's lane types. |
| `templates[].branch_pattern` | string | The new branch, with `{name}` (required) and `{issue}`, e.g. `"change/{name}"`. Empty means the lane type's prefix + `{name}`. |
| `templates[].first_prompt` | string, required | ONE line typed into claude once it is ready. `{name}` and `{issue}` only; no newline or control character; at most 4000 characters. |
| `templates[].model`, `.effort` | string | Override the lane type's launch options. |
| `queues` | array | Shared resources held as a lease (see *The gate queue*). |
| `queues[].id` | string, required | `[a-z0-9][a-z0-9_-]*`, unique. |
| `queues[].title` | string | Shown on the queue card. |
| `queues[].lock` | string, required | The lease directory, relative to the **git common dir** (so every worktree agrees), e.g. `"clauductor/gate.lock"`. No `..`, not absolute. |
| `queues[].command` | array of strings | Optional argv that **RUN** starts through `lock-run` in the selected lane's worktree. |
| `alerts` | object | Thresholds; a missing key takes the default, `0` turns that alert off. `idle_minutes` (30), `context_pct` (85), `five_hour_pct` (90), `waiting_seconds` (120), `notify` (true: macOS notifications), `min_interval_seconds` (300: at most one notification per lane per interval). |
| `quota_guard` | object | `five_hour_pct` (95): refuse to start or restore a lane at or above this 5-hour quota, unless the dialog's override is ticked. `0` turns it off. |
| `host_names` | array of strings | Extra names the panel answers to, each `<label>.localhost` in lower case (for example `"myproject.localhost"`). `clauductor.localhost` always works. No wildcards. |
| `cards[].refresh` | string, required | `"watch:<relpath>"`: re-run when that file (or a direct entry of that directory) changes; the path must stay inside the project. `"interval:<seconds>"`: re-run on a timer (minimum 5 s). Every card also runs at start and on ↻ REFRESH. |

**Card output.** If stdout parses as JSON, it renders as JSON: an array becomes a list (for
objects, `title`/`name`/`text`/`summary`/`id` is the main line and other scalar fields are shown
dimmed), and an object becomes key/value rows. Otherwise each non-empty line is a list item,
with a leading markdown bullet (`-`, `*`, `1.`) removed. A failing command shows
"cannot read: …", never an empty card. An example config is in
`framework/internal/panel/testdata/panel.json`.

## What each part of the page means

- **Top bar.** LIVE / DISCONNECTED (the page reconnects on its own; if the panel was restarted
  it says so, because the new launch has a new token). The 5-hour and 7-day quota gauges are the
  real subscription budget (labelled **5 h** and **7 d**). **est. $ (list price)** (with
  "· tracked sessions" on wide screens) is the sum of the status
  line's `total_cost_usd` over the sessions the panel tracks now: live ones, and ones heard from
  in the last 30 minutes. A session forgotten after that drops out of the sum. It is a
  list-price estimate, not a bill.
  `hooks` counts hook events accepted, status-line posts, and events dropped as outside the
  project; below 1440 px it is left to the footer, which has the same counters, so the bar
  stays on one row from 1280 px. **+ LANE** opens the Start dialog; it is disabled, with the reason on hover, while
  lanes cannot start (see *Subscription only*).
- **Lanes (left).** One per worktree with a live session or recent activity. The stripe is green
  for busy, amber for waiting, grey for idle, and red when the lane is busy but no hook has
  arrived from it for 60 s ("no hooks"). The status line repeats the state as a shape (see
  *Themes*). The chip is the lane type from `lanes`. Worktrees with no session are
  listed underneath.
- **Terminals (centre).** One tab per lane, with a status dot. The selected tab is that lane's
  live terminal: type into it as you would in Terminal.app. Under it are **ATTACH IN
  TERMINAL.APP**, **INTERRUPT (ESC)**, **RESTART** and **STOP LANE**. Stop and restart ask for
  confirmation in the page. An orphaned lane has **RESUME** and **FORGET** instead of a terminal.
- **Selected lane (centre, below the terminal).** Its sessions (pid, status, context %, model,
  est. $), running subagents with their age, and the lane's own event feed. A lane card marked
  `· tmux` has a terminal; clicking it opens that tab. Workflow agents stop under a different
  `agent_id` and `agent_type` (`workflow-subagent`) than they started with, so a
  `workflow-subagent` stop with an unknown id retires the oldest running agent of any type. Any
  other typed stop with an unknown id retires the oldest agent of its own type. An unknown id
  with an empty type is an internal agent that never sent a start, and retires nothing. A session
  that `claude agents` reports idle for 10 s, or gone, has its running list cleared. The hook `Stop` clears nothing, because
  background agents outlive the turn.
- **Right column.** *Needs you*: permission and idle-prompt notifications, and sessions that
  `claude agents` reports as waiting, followed by the project's cards. *Open PRs*: from `gh`, with
  "cannot read" on failure (never an empty list). *Feed*: the last events across all lanes.
- **Red banner.** A lane is busy per `claude agents` and no hook has come from it since it went
  busy, for 60 s. Usually the session never loaded the hooks. Restart it. Also shown when `claude
  agents` or `git worktree list` cannot be read.

## Themes

The **theme** button at the right of the top bar picks one of six designs and a mode: **System**
(follows the OS light or dark setting), **Light** or **Dark**. The menu works from the keyboard:
Down or Enter opens it, the arrow keys, Home and End move, Enter picks, Escape closes it and
returns focus to the button. Each theme shows a swatch drawn from its own tokens. The choice is
kept per browser in `localStorage`. Without storage the page shows Console and the picker still
works for the visit. A small script, `static/theme.js`, loads first and sets the theme before
the first paint, so a stored theme never flashes the default. Changing theme re-colours the
open terminals at once. Every theme's terminal is dark, in light mode too: only a dark
background lets each ANSI colour read as text and also carry a label in another ANSI colour.

| Theme | Idea | Faces |
|---|---|---|
| **Console** (default) | An instrument panel at night: navy, a signal-blue readout, a plotting grid, uppercase telemetry labels. | Chakra Petch, IBM Plex Sans, JetBrains Mono |
| **Chart room** | A nautical chart: white water, chart magenta, a latitude-scale border, italic names, sentence case, square corners. Dark is a dimmed night palette. | Newsreader italic, Public Sans, DM Mono |
| **Ward monitor** | A ward's central monitoring station: rounded bed tiles, soft shadows, big condensed figures, surgical teal. The roomiest. | Barlow Semi Condensed, Barlow, Red Hat Mono |
| **Duplicator** | A dispatch office: forms typed in duplicator violet, dashed carbon-form rules, a tractor-feed edge. The densest. | Courier Prime |
| **High contrast** | For low vision and glare: black and white, 7:1 page text and terminal colours (see below for its three exceptions), 2 px rules, a 3 px focus ring, the largest type, no translucent fills. | Atkinson Hyperlegible Next, Atkinson Hyperlegible Mono |
| **Shop floor** | Safety signage: concrete and asphalt, stencil lettering, a hazard-stripe edge, heavy borders, wide state stripes. | Big Shoulders Stencil, Archivo, Martian Mono |

A lane's state is never shown by colour alone. Busy is a filled circle, waiting a diamond,
idle a hollow circle, and blocked or stale a square, and each also has its word ("busy",
"waiting: …", "no hooks", "blocking"). Lane cards and terminal tabs take keyboard focus, and
Enter opens them. A long lane name wraps to two lines; hovering the card shows it whole.

### Adding a theme: the token contract

A theme is only tokens. `web/static/panel.css` references tokens and never a colour or a face
of its own, so a theme adds no component CSS. To add one:

1. Add `{ id, name, note }` to `THEMES` in `web/static/theme.js`, with `aaa: true` if it must
   meet 7:1 text contrast.
2. In `web/static/themes.css`, add three blocks:
   - `[data-theme="<id>"]` holds the shape and type tokens, shared by both modes: the faces
     (`--font-display`, `--font-body`, `--font-mono`, `--font-label`), the type scale and case
     (`--fs-root`, `--display-*`, `--label-*`, `--btn-size`, `--caps`), and the
     shape (`--r`, `--r-btn`, `--r-chip`, `--bw`, `--line-style`, `--stripe`, `--pad`,
     `--gap`, `--col-pad`, `--shadow`, `--focus-w`, `--term-size`, `--term-min-contrast`, `--band`,
     `--band-h`, `--backdrop`, `--backdrop-size`).
   - `[data-theme="<id>"][data-mode="light"]` and `…[data-mode="dark"]` each hold every colour:
     `--surface`, `--panel`, `--panel-2`, `--line`, `--line-strong`, `--text`, `--text-dim`,
     `--accent`, `--accent-ink`, `--accent-soft`, `--go`, `--hold`, `--stop` and their `-soft`
     tints, `--idle`, `--focus`, `--grid`, `--scrim`, and the terminal's `--term-bg`,
     `--term-fg`, `--term-cursor`, `--term-selection` and `--ansi-0` to `--ansi-15`.
3. Put any new font in `web/static/fonts/` with its `OFL-<family>.txt`, and add an `@font-face`.

`themes_test.go` then checks the theme. It fails if a theme × mode lacks any token that another
theme defines or that `panel.css` or `panel.js` uses, if `theme.js` and `themes.css` disagree on
the list, or if a theme block sits inside `@media` or another conditional rule. It fails if
`panel.css` fades anything with `opacity` except a disabled control, since a fade would undo
every ratio below. Quiet rows step back with `--panel-2` instead. It also checks contrast,
measured on `--surface`, `--panel` and `--panel-2`:

- text, dim text, accent text and the state colours as text must reach 4.5:1 (7:1 for an `aaa`
  theme), and so must text, dim text and the state's own colour on each `-soft` tint, laid over
  both `--panel` (cards) and `--surface` (banners), and text on the primary button;
- the focus ring and the idle marker must reach 3:1;
- the terminal foreground must reach 7:1 on its background, and each ANSI colour 3:1 (7:1 for
  colours 1–15 in an `aaa` theme);
- the label pairs a TUI draws (black on green, yellow and cyan; white on red, blue and magenta;
  and the bright variants) must reach 4.5:1. Black on red is not required: with red also at
  3:1 on the background, no red carries both black and white text at 4.5:1;
- in an `aaa` theme every colour is light (7:1 on black), and two light colours differ by at
  most 3:1, so white text on a coloured label cannot be legible there. Black carries every
  label instead, at 4.5:1 on each colour, and ANSI black itself stays at 3:1 on the
  background. Those two figures, and white-on-colour labels, are the High contrast theme's
  three exceptions to 7:1. What the palette cannot promise, xterm enforces:
  `--term-min-contrast` sets its `minimumContrastRatio` (4.5 in every theme, 7 in High
  contrast), so it lightens or darkens any text a program draws, on any ANSI or truecolor
  background, to that ratio. White on red therefore renders at 7:1 there as another colour;
- busy, waiting, blocked and idle must differ by at least ΔE 20.

It fails, too, on a font file that no theme loads or that has no licence, and on more than 700
KB of fonts.

## The status line: sending the panel a copy

Hooks carry no cost or context data; only the status line's stdin does. A project that wants
quota, context % and est. $ on the panel adds this to its status-line script. It posts only
when the panel's marker file exists, never waits (background, 0.5 s cap), and prints nothing, so
the status line is unaffected on a machine that has never run the panel:

`~/.clauductor/panel/port` holds the port and nothing else, because scripts read it as digits.
The panel's PID is in `~/.clauductor/panel/pid` beside it. Both are removed on a clean stop; a
`pid` naming a process that is not running means the panel was killed and both files are stale.

```bash
input=$(cat)
if [ -f "$HOME/.clauductor/panel/port" ]; then
  port=$(cat "$HOME/.clauductor/panel/port")
  printf '%s' "$input" | curl -s --max-time 0.5 -X POST -H 'Content-Type: application/json' \
    --data-binary @- "http://127.0.0.1:$port/status" >/dev/null 2>&1 &
fi
# ...the script's existing output, reading "$input" instead of stdin...
```

## Hooks

On every start, the panel merges one `type: "http"` hook per event into the **running user's**
`~/.claude/settings.json`, and nowhere else:

```json
{ "type": "http", "url": "http://127.0.0.1:4393/hook?src=clauductor-panel", "timeout": 1 }
```

for `UserPromptSubmit`, `Stop`, `SubagentStart`, `SubagentStop`, `Notification`,
`SessionEnd`, and from v2 `StopFailure`, `PermissionRequest` (observed only, never answered),
`PreCompact`, `PostCompact` and `CwdChanged`. `SessionStart` is left out because HTTP hooks do not fire for it (Claude Code
2.1.284); new sessions are found through `claude agents --json`.

- The panel's entries are recognised by the `src=clauductor-panel` query parameter, because Claude
  Code documents no free-form key for ownership. Only tagged entries are replaced or removed;
  every other key and hook is kept, in order. The panel's current entry stays where it is, even if
  a user hook follows it.
- The install is idempotent: a start that would change nothing does not rewrite the file.
- A file that is not exactly one JSON object (invalid, or with trailing data) is refused and not
  touched.
- Before the first write, the file is copied to `settings.json.clauductor-panel.bak`. That backup
  is never overwritten, so it keeps the file as it was before the panel first touched it. Writes
  are atomic (temp file + rename, in the same directory).
- A symlinked `settings.json` (for example, one managed by a dotfiles repo) is followed: the
  panel edits the file it points at, and the link stays a link.
- The hooks stay installed when the panel stops. While it is down, the connection is refused
  at once and the session is never blocked. `clauductor panel --uninstall-hooks` removes them.

## Lanes

A **lane** is one interactive `claude` in its own tmux session, on the panel's own tmux server
(`tmux -L clauductor`, or `tmux_socket`). tmux owns the process, not the panel, so:

- closing the browser, or restarting or upgrading the panel, leaves every lane running;
- several viewers can share a lane: browser tabs, and a Terminal.app window.

Lanes are interactive sessions, never `claude --bg`: at a usage limit an interactive session
pauses, while a workflow in a background session fails.

### Starting a lane

**+ LANE** asks for a lane type (from `lanes` and `lane_types`), a lane name, and where it runs:

- **New branch and worktree.** The server runs `git fetch`, then
  `git worktree add -b <prefix><name> <worktree_dir>/<name> <base>`. The prefix is the type's
  prefix rule in `lanes` (`fix/` for `fix`). A type with only exact rules (such as `main` →
  `orchestrator`) has no prefix and cannot start a new branch.
- **An existing worktree.** Any worktree that `git worktree list` reports.

A lane is refused in a directory where another lane is already running, registered or not: two
sessions in one checkout would edit the same files.
- **The project root.** Use this for the orchestrator.

Then it runs, as an argv list with no shell:

```
tmux -L <socket> -f /dev/null new-session -d -s <name> -c <dir> -x 200 -y 50 \
     -e PATH=… -e HOME=… -e LANG=… \
     /usr/bin/env -u ANTHROPIC_API_KEY … claude [--model m] [--effort e] -n <name> --session-id <uuid>
```

- The lane name is the tmux session name and the worktree directory name. It must match
  `[a-z0-9][a-z0-9-]{0,40}`, so it is safe in a tmux target and in a shell command.
- The session id is a UUID that the panel generates, so the lane is bound to its Claude session
  from its first second. The panel never discovers sessions by directory.
- `-e` gives the session an explicit PATH (the directories of `claude`, `tmux`, `git`, `gh` and
  `node`, then the panel's own PATH), HOME and LANG. It does not depend on whichever process
  happened to start the tmux server.
- `/usr/bin/env -u` removes the API-key variables. It also removes the variables a parent Claude
  session sets for its children, so a panel started from inside Claude cannot make its lanes look
  nested.
- `remain-on-exit` keeps a lane's last screen after `claude` exits. A lane that dies at start
  shows why, instead of vanishing. The tab's dot turns red.

**A new directory shows Claude's workspace-trust dialog.** In Claude Code 2.1.284 it defaults
to **No, exit**. Press ↓, then Enter, in the lane's terminal. If you press Enter first, claude
exits and the lane shows a dead pane; STOP it and start it again.

### The lane registry

`~/.clauductor/panel/<hash of the project path>/lanes.json` (0600, in a 0700 directory) records
each lane: its id, session id, directory, type, branch, and its last action. The intent is
written **before** each action and marked done after it, so a panel that crashes mid-action
finds the half-done action when it restarts. The file is written atomically.

The registry is never trusted on its own. Every 2 s the panel compares it with the tmux socket,
`claude agents --json` (matched by session id) and the worktree list, and it re-reads the file
every 30 s. Anything that does not add up is shown as an **orphan**, never hidden:

| What | Shown as | What you can do |
|---|---|---|
| registered, tmux session gone (a reboot, or tmux ended) | orphaned | **RESUME**, or **FORGET** |
| registered, the panel stopped during an action | orphaned, with the action | **RESUME**, or **FORGET** |
| a tmux session on the socket that the registry does not know | running, "not in the lane registry" | terminal and **STOP** only; without a session id it cannot be restarted |
| registered, its directory no longer a worktree | the reason is added | **FORGET** |
| a record that fails validation on load (session id not a UUID, relative path, unknown mode…) | "corrupt registry record" | **STOP**, **FORGET**; it is never launched |
| a record with an invalid lane id | a banner | edit or delete the file |

### Controls

| Button | What it does |
|---|---|
| **INTERRUPT (ESC)** | `tmux send-keys Escape`, which is claude's interrupt. |
| **STOP LANE** | If `claude agents` reports the lane's session **idle**, sends `C-u` (clearing any unsent text), types `/exit`, checks that the session is **still** idle, then presses Enter as a separate write and waits up to 10 s. If it stopped being idle, it presses Escape instead. In any other case (busy, waiting on a permission or dialog, or unknown), it presses **Escape only**, never Enter: an Enter would confirm whatever default the dialog has focused. Then `kill-session`. The lane leaves the registry. **The worktree is never removed**; the panel offers no way to remove one. |
| **RESTART** | Stops the lane, then starts its **own** session again in the same directory: `claude --resume <session id>`. If the session never had a prompt, it uses `--session-id <same id>` instead, because `--resume` refuses an empty session. The panel marks a session as having a conversation when a `UserPromptSubmit` or `Stop` hook arrives from it, or when `claude agents` shows it busy. Hooks can be dropped, so the mark can be wrong. If claude then exits non-zero within 3 s, the panel retries once with the other flag. It judges by the exit status alone and never reads the screen. In Claude Code 2.1.284, both wrong flags exit 1 at once. If both attempts fail, the dead pane shows claude's message. It **never** uses `--continue`, which picks the directory's most recent conversation, whoever's it is. |
| **RESUME** (orphans) | The same resume, for a lane whose tmux session is gone. It is refused while `claude agents` shows another process on that session id, or cannot be read. Two processes on one session would interleave its transcript. |
| **FORGET** (orphans) | Drops the registry record. The worktree and the conversation stay. |
| **ATTACH IN TERMINAL.APP** | Runs `osascript` to open a Terminal window with `exec tmux -u -L <socket> attach-session -t =<name>`. The command reaches AppleScript as an argument and is never spliced into the script, and every part of it is single-quoted. The first time, macOS asks whether the panel may control Terminal. |

Text that the panel types into a lane (`/exit`) goes as the text first, then Enter 400 ms later.
Sent together, a long line can sit in claude's input box unsubmitted.

### Window size: the latest client wins

Each browser viewer gets its own PTY and its own tmux client. The lane's window uses tmux's
`window-size latest`, set explicitly on each lane because `~/.tmux.conf` might change it. The
window takes the size of whichever client last typed or resized. When you type in the browser,
the lane fits the browser. When you type in Terminal.app, it fits that window, and the browser
shows the same screen, clipped or padded, until you type there again.

We chose this over the alternatives:

- `attach -f ignore-size` for the browser would leave a browser-only lane stuck at the detached
  size.
- A grouped session per viewer would share one window size anyway, and add sessions to clean up.

### Subscription only

The panel refuses to start, restart or resume a lane while `ANTHROPIC_API_KEY` or
`ANTHROPIC_AUTH_TOKEN` is set, in the panel's own environment or in the tmux server's global
environment (`tmux -L <socket> show-environment -g`), which every lane inherits. Either key
outranks the subscription login. The page shows the reason and disables **+ LANE**. If the tmux
environment cannot be read, the panel refuses too: unknown is not "no key". As a second layer,
the lane command unsets both variables.

## Run it with no terminal: the launchd agent

```bash
clauductor panel install --project ~/Development/app          # add --app for a Dock/Spotlight launcher
```

`install`:

1. Loads the config and refuses if it is missing or invalid, rather than crash-looping later.
2. Copies the running binary to `~/.clauductor/panel/bin/clauductor`. The agent never runs from
   a build directory or a worktree that may disappear. Re-run `install` after upgrading
   clauductor.
3. Writes a new persistent token to `~/.clauductor/panel/token` (0600, directory 0700). Every
   install rotates it, so reinstalling also revokes the old one.
4. Writes `~/Library/LaunchAgents/com.clauductor.panel.plist` and checks it with `plutil -lint`.
   The plist sets:
   - `RunAtLoad`;
   - `KeepAlive` with `SuccessfulExit = false`, so a crash restarts the panel, throttled to once
     every 30 s, and `launchctl bootout` stops it for good;
   - `LimitLoadToSessionType = Aqua`;
   - logs in `~/.clauductor/panel/logs/`;
   - `LANG=en_US.UTF-8`;
   - a PATH made of `/opt/homebrew/bin`, `~/.local/bin`, the directories where `claude`,
     `tmux`, `git`, `gh`, `node` and `jq` were found at install time, and the system
     directories. launchd's default PATH has none of these.
5. Replaces any loaded copy (`launchctl bootout`), then runs `launchctl bootstrap gui/$UID`.

Under launchd, the panel runs with `--launchd`:

- It uses the persistent token, and the cookie lasts 30 days.
- The token never goes to stdout or stderr, which are the log files. The log names the token
  file instead.
- At each start it opens the browser once, with the token. Another start within 5 minutes does
  not open it again, so a crash loop cannot fill the browser with tabs.

`--app` also builds `~/Applications/Clauductor Panel.app`, an AppleScript applet made by
`osacompile` that runs `clauductor panel open`. Put it in the Dock, or find it with Spotlight.

```bash
clauductor panel open         # checks /healthz on both loopbacks, then opens http://clauductor.localhost:4393/?t=<token>
clauductor panel uninstall    # bootout, then remove everything the agent owns
```

`open` sends the token only to the panel it expects. `/healthz` answers `ok pid=<pid>`, and
`open` compares that with `~/.clauductor/panel/pid` on **both** `127.0.0.1` and `[::1]`, because
the browser resolves `clauductor.localhost` to `::1` first. On a mismatch at either address it
refuses rather than hand the token to whatever holds the port. If nothing listens on `[::1]`, it
opens `http://127.0.0.1:<port>/` instead.

`uninstall` removes:

- the plist, the token and the copied binary;
- the logs, `pid`, `port` and the browser-opened stamp;
- the app, but only if `install --app` made it;
- every lane registry that lists no lanes.

A registry that still lists lanes is kept, and `uninstall` says so: those lanes may still run in
tmux, and RESUME needs their session ids. It never touches lanes. The panel's hooks stay in
`~/.claude/settings.json`; `clauductor panel --uninstall-hooks` removes them.

### If the panel is down

The lanes are not affected: they run in tmux whether or not the panel is up.

1. `clauductor panel open` tells you whether the panel answers on its port.
2. Restart it with `launchctl kickstart -k gui/$(id -u)/com.clauductor.panel`. See its state with
   `launchctl print gui/$(id -u)/com.clauductor.panel`.
3. Read `~/.clauductor/panel/logs/panel.err.log`. The usual causes are a port another process
   holds, or a config that moved. In both cases the log says which.
4. To reach a lane with no panel, run `tmux -L clauductor ls`, then
   `tmux -L clauductor attach -t '=<lane>'`. Quote the target, because zsh expands a bare
   `=word`. The panel's socket has no prefix key (see *Security model*), so close the window to
   detach; the lane keeps running.
5. If the config file moved (for example, `--config` pointed into a worktree that was removed),
   run `install` again with the new path.

## Security model

A dashboard of your sessions is private, and a browser terminal is a shell, so the page is
locked down even on loopback. Loopback is not a trust boundary: any web page you open can
send requests to `127.0.0.1`.

- **Loopback only.** The server binds `127.0.0.1:<port>` and `[::1]:<port>` (one server, one
  state) and refuses to run if either bound address is not loopback. If the port is taken on
  either address, it **exits with an error** and never falls back to another port (the hooks
  post to a fixed URL, and `clauductor.localhost` would reach whatever holds `[::1]`). A machine
  with no IPv6 loopback at all is served on `127.0.0.1` only, and the log says so. A refused
  start does not touch `settings.json` or the marker.
- **The address is `http://clauductor.localhost:<port>`.** macOS and every current browser
  resolve `*.localhost` to loopback with no system change (no `/etc/hosts` entry, no port 80).
  Cookies are per host, so the token exchange happens at that name.
- **Per-launch token.** Each start makes 32 random bytes and opens
  `http://clauductor.localhost:<port>/?t=<token>`. The server swaps the token for an `HttpOnly;
  SameSite=Strict` cookie and redirects to `/`, so the token leaves the address bar. Every route
  except `/hook`, `/status` and `/healthz` needs the cookie (401 otherwise). `/healthz` answers
  only `ok` and the panel's PID. Under launchd, the token persists in a 0600 file instead (see above).
- **DNS rebinding and cross-site requests.** `Host` must be exactly one of `127.0.0.1:<port>`,
  `localhost:<port>`, `[::1]:<port>` and `clauductor.localhost:<port>`, plus any name in the
  config's `host_names` (each one lower-case label followed by `.localhost`), on every route. There
  are no wildcards: `evil.localhost` and `clauductor.localhost.evil.com` are refused. Names compare
  case-insensitively (RFC 9110), so `CLAUDUCTOR.localhost` is accepted. A `*.localhost` name
  cannot be an attacker's DNS name, because browsers never ask DNS for it. Every state-changing
  request (every `POST`) must carry an `Origin` equal to `http://` + the `Host` of that same
  request, so a page on `localhost` cannot drive the panel at `clauductor.localhost`, or the
  reverse. No CORS headers are sent. The page is served with
  `X-Frame-Options: DENY` and `Referrer-Policy: no-referrer`.
- **Content Security Policy: this origin only.** `script-src 'self'`, `font-src 'self'`,
  `connect-src 'self' ws://<host>`, and no `'unsafe-inline'` anywhere. The page's JS and CSS are
  files embedded in the binary. So are xterm.js and the themes' fonts. The page loads nothing from
  a CDN or Google Fonts. xterm.js creates `<style>` elements at run time, so `style-src` allows
  one per-response nonce as well; `panel.js` stamps it on those elements. xterm's renderer also
  colours cells through a `<span>`'s `style` attribute (truecolor, and colours lifted to
  `--term-min-contrast`), which the CSP blocks; `panel.js` routes a span's `style` attribute
  through CSSOM, which the CSP does not govern, so those colours render.
- **The terminal endpoint** (`GET /ws/term?lane=<id>`) is a shell into a lane, and it is the most
  guarded route. It needs all of the following:
  - the Host check;
  - the cookie;
  - an `Origin` exactly equal to `http://<the Host>`, meaning scheme, host and port;
  - **a single-use ticket**. The page gets one from `POST /api/lanes/<id>/ticket`, which checks
    `Origin`. A ticket is valid for 30 s, for that one lane, and is sent in the
    `Sec-WebSocket-Protocol` header, never in the URL. The cookie alone is not enough, because
    cookies are not isolated by port (RFC 6265 §8.5). A page on another loopback port, such as a
    dev server on `:3100`, is same-site, and the browser sends it the panel's cookie. So is a page
    on another `*.localhost` name, such as `evil.localhost`. Only the
    panel's own page can read a ticket. A test checks that a request from `:3100` cannot open a
    terminal.
  - a valid lane id that names a running lane.

  **A lane's terminal is a shell.** Claude runs `!` commands, and anyone who can type into the
  terminal can do what the lane's user can. Everything above exists so that only you can type
  into it. On the panel's own tmux socket the server never loads `~/.tmux.conf` (`-f
  /dev/null`), and every lane start sets `prefix None`, `prefix2 None` and unbinds the prefix
  and root tables. `-f` only applies when the panel starts the server, so the same settings
  are applied again every time the panel finds a server on its socket (each 2 s poll) and before
  every viewer attaches. A server someone else started there, with their `~/.tmux.conf`
  bindings, is stripped too. A lane's viewer therefore cannot use tmux keys to switch to another lane or reach tmux's
  command prompt and `run-shell`.

  After the upgrade, the browser may send only `{"type":"input","data":…}`,
  `{"type":"resize","cols":…,"rows":…}`, `{"type":"alive"}` and `{"type":"focus","focused":…}`
  (v2: silences that lane's notifications); anything else closes the connection.
  - **Idle pages lose their terminals.** While the page is visible it sends `alive` once a
    minute. A terminal that hears nothing for 5 minutes (the page is hidden, asleep or gone) is
    closed with code 4000. When the page is back in view it reopens each terminal with a fresh
    ticket.
  - **Token rotation.** `clauductor panel rotate-token` writes a new token file; `install`
    does too. The running agent notices within 2 s. At once, every cookie for the old token
    gets 401, every terminal closes with code 4001, every event stream ends, and every ticket
    not yet used is dropped. A terminal whose upgrade passed the cookie check just before the
    rotation is closed the moment it registers. Then
    `clauductor panel open` opens the page with the new token. The browser never
  sends a command. The server runs one fixed argv per viewer: `tmux -u -L <socket>
  attach-session -t =<id>`, where `=` makes the match exact. Stopping a lane closes its viewers.
  Closing a viewer only detaches its tmux client.
- **Terminal output is untrusted.** xterm.js renders it to its own DOM, and the page never passes
  it to `innerHTML`. A link that a lane prints (OSC 8) opens only after an in-page confirmation,
  and only for `http`/`https`. Title escapes are ignored. There is no automatic linkifier.
- **Lane control is fixed verbs on validated ids.** start, stop, interrupt, restart, resume,
  forget, terminal-app, and in v2 restore-all, queue cancel and queue run (a queue id and a
  worktree from `git worktree list`; the command comes from the trusted config, never the
  browser). A start names a lane type (checked against the config), a mode, a lane
  name, and for "existing" a path, which must be one of `git worktree list`'s. Unknown JSON fields
  are refused.
- **The ingest endpoints** (`/hook`, `/status`) take no token, since a session cannot know it.
  They accept `POST` from a loopback peer only, refuse any request carrying `Origin` or
  `Sec-Fetch-Site` (Claude Code sends neither; a browser always does), cap the body at 256 KB,
  answer `204` before processing, and never execute anything. The worst a local process can do
  is post fake lane events.
- **Events from other projects are dropped.** An event counts only if its `cwd` is inside one
  of the project's worktrees, as `git worktree list --porcelain` reports them. That list is the
  authority, never a hand-kept list. It is re-read every 10 s, and early when a worktree is
  added or removed or when an event arrives from an unknown `cwd`.
- **What the panel writes to disk:**
  - the marker `~/.clauductor/panel/port` and `pid`, removed on SIGINT/SIGTERM;
  - the hook install;
  - the lane registry;
  - the trusted config hash, and the logs of queue RUNs;
  - under launchd, the token, the logs, the copied binary and a browser-opened timestamp.

  Hook bodies include prompt text. The panel keeps only a short one-line summary per event, in a
  200-event in-memory ring buffer. It never reads transcript files; a test fails if any panel
  source mentions one.

## Dependencies

| What | Version | Licence | Why |
|---|---|---|---|
| `github.com/coder/websocket` | v1.8.15 | ISC | WebSocket server. It is the maintained successor of nhooyr.io/websocket, with no dependencies of its own and a `context`-based API that fits the panel's shutdown. It negotiates subprotocols, which carry the terminal ticket, and it re-checks `Origin` itself as a second layer. gorilla/websocket was the alternative; it was archived for a time, and its API predates `context`. |
| `github.com/creack/pty` | v1.1.24 | MIT | One PTY per viewer's `tmux attach`. This is why the panel needs no node-pty. |
| `@xterm/xterm` | 6.0.0 | MIT | The terminal in the page. It is vendored as `web/vendor/xterm/xterm.js`, `xterm.css` and `LICENSE`, and embedded with `go:embed`. |
| `@xterm/addon-fit` | 0.11.0 | MIT | Fits the terminal to its box. Vendored the same way. |
| Chakra Petch, IBM Plex Sans, JetBrains Mono, Newsreader, Public Sans, DM Mono, Barlow, Barlow Semi Condensed, Red Hat Mono, Courier Prime, Atkinson Hyperlegible Next, Atkinson Hyperlegible Mono, Big Shoulders Stencil, Archivo, Martian Mono | fontsource 5.3.0, latin | SIL OFL 1.1 | The themes' fonts, in `web/static/fonts/` with their licences. About 470 KB in all; a face downloads only when the active theme uses it. |

To update a vendored file, download it with `npm pack <package>@<version>`, copy the file from
`lib/` (or `files/` for fonts) together with its `LICENSE`, and update this table.

## v2: orchestration

v2 adds lane templates, a queue for the shared gate, alerts with macOS notifications, a quota
guard, and restoring every lane after a reboot. It also hardens v0's reading of Claude Code's
signals. Everything below still costs no model tokens: the panel types a template's first prompt
once, and never reads a screen or a transcript.

### An example v2 config

```json
{
  "name": "My Project",
  "version": 2,
  "lanes": { "change/": "build", "fix/": "fix", "ops/": "ops", "main": "orchestrator" },
  "lane_types": { "build": { "model": "opus", "effort": "high" } },
  "templates": [
    { "id": "build", "title": "Build a change", "lane_type": "build",
      "branch_pattern": "change/{name}", "first_prompt": "/build-change {name}" },
    { "id": "propose", "title": "Propose the next row", "lane_type": "build",
      "branch_pattern": "change/propose-{name}", "first_prompt": "/openspec-propose {name}" },
    { "id": "fix", "title": "Fix an issue", "lane_type": "fix",
      "first_prompt": "Fix GitHub issue {issue}. Start from the code as built." }
  ],
  "queues": [
    { "id": "gate", "title": "Full gate (port 3100)", "lock": "clauductor/gate.lock",
      "command": ["infra/ci/run-local.sh"] }
  ],
  "alerts": { "idle_minutes": 20, "context_pct": 80, "five_hour_pct": 90, "waiting_seconds": 120 },
  "quota_guard": { "five_hour_pct": 95 }
}
```

### Lane templates

**+ LANE** offers the templates. Pick one, give the lane a name (and an issue if the template
uses `{issue}`), and START:

1. The server validates every value: the name is a lane id; the issue is one line of plain text
   (no newline, no control character, no invisible format character such as a bidi override, at
   most 200 characters), and must also make a valid branch name if the pattern uses it. The
   dialog previews the branch and the exact prompt.
2. The lane starts as any new-branch lane does, with the template's model and effort. The
   rendered prompt is written into the lane registry with state `pending` **before** the lane
   starts.
3. The panel types the prompt **once claude is ready**, and it decides "ready" from structured
   signals only: `claude agents --json` lists the lane's own session id as `idle` with no
   `waitingFor`, no hook says the session waits (a permission prompt, an elicitation), and that
   poll succeeded within the last two poll intervals. Right before the first keystroke, under
   the lane lock, it reads `claude agents` once more and checks again. It never scrapes the
   screen. A session held at the workspace-trust dialog is not listed at all
   (verified on 2.1.284), so the prompt can never land in that dialog. The text goes first
   (`send-keys -l -- <text>`, so text starting with `-` stays text), then Enter as a separate
   write, 400 ms later.
4. The state goes `pending` → `typing` → `sent` → `delivered`. `typing` is written before the
   first keystroke, so a panel that dies mid-typing never types the prompt twice. `delivered`
   needs proof: the session's `UserPromptSubmit` hook, or `claude agents` showing it busy.

"Needs you" says so when it is stuck: claude not listed as idle after 15 s ("answer any dialog in
its terminal"), claude exited, the panel stopped mid-typing, or the prompt was typed but not
submitted after 30 s. If you type into the lane first, the prompt is skipped.

### The gate queue: a lease on disk

Two lanes that both run the full gate on port 3100 collide. A queue serialises them. The lease
lives entirely on disk, so it survives a panel restart, and it works when the panel is not
running at all: the gate script takes it itself, through `clauductor lock-run`.

macOS has no `flock(1)`, so the lock is a **directory**, because `mkdir` is atomic everywhere.
The protocol is plain files, so a shell script can honour it with no clauductor at all (below).

| Path | Meaning |
|---|---|
| `<lock>/` | Held while it exists. `mkdir` either creates it or fails with `EEXIST`. |
| `<lock>/owner.json` | The holder: `{v, nonce, pid, pstart, child_pid, child_pstart, host, lane, cmd, started, renewed, ttl}`, written atomically (temp file + `mv`). |
| `<lock>.waiters/<arrival>-<nonce>.json` | One per waiter, same fields. The numeric arrival (unix ns, or unix s followed by nine zeros) orders the queue. |
| `<lock>.waiters/<nonce>.cancel` | Asks that waiter to give up (the panel's **CANCEL WAIT**). |
| `<lock>.reclaim/` | A short mutex, taken only to remove a stale holder. |

`pstart` is the holder's process start time exactly as `LC_ALL=C ps -o lstart= -p <pid>`
prints it, with runs of whitespace collapsed to one space (for example
`Mon Sep 28 23:10:17 2026`), or `proc:<field 22 of /proc/<pid>/stat>` where there is no `ps`.
It is what tells a live holder from a reused pid. `child_pid` and `child_pstart` name the
holder's command the same way.

**When a holder is stale** (only a stale holder may be removed):

- **Same host, pid gone** (`ps -p <pid>` finds nothing): stale.
- **Same host, pid alive, start time equals `pstart`: never stale**, however long it has been
  silent. A holder stopped with `SIGSTOP`, or on a laptop that slept, is still the holder.
  Expiring it would run two gates at once.
- **Same host, pid alive, another start time:** the pid was reused. Stale.
- **Same host, pid alive, start time unverifiable** (none recorded, none readable, or one from
  `ps` and one from `/proc`): live. Missing data never removes somebody else's lease or waiter
  file. A lease stuck this way (a reused pid, no start time) is removed by hand.
- **The command counts too.** `lock-run` records its command as `child_pid` / `child_pstart`.
  On the same host a record is stale only when the holder **and** the command are both dead. A
  `lock-run` killed with `SIGKILL` leaves its gate running, and the gate still holds the lease.
- **Another host:** its pids mean nothing here, so the TTL applies: stale once `renewed + ttl`
  has passed. `lock-run` renews `renewed` every ttl/3 (default ttl 10 min). A shell holder
  writes `ttl: 0` (no expiry).
- **Where there is no `ps`,** liveness is `kill -0` (an `EPERM` answer still means alive) and the
  start time is `proc:` + field 22 of `/proc/<pid>/stat`.
- A directory with no readable `owner.json` for 10 s is stale (its holder died between `mkdir`
  and the write). Never write `owner.json` in place; write a temp file and `mv` it.

**Removing a stale holder.** Only the first live waiter does it, under the reclaim mutex, after
re-reading `owner.json` and checking it is still the same stale holder (same nonce). **Nothing
ever signals or removes a live holder**, including the panel.

**Defence in depth in `lock-run`:**

- It holds `flock(2)` on the lease directory while it runs. The flock **never decides
  liveness**: that is the holder's and the command's pid and start time, the same rule the shell
  applies, so Go and shell waiters always agree. The command does not inherit the flock (a
  daemon the gate leaves behind would hold it long after the gate ended). The panel only
  mentions a flock still held on a stale lease.
- Once a second it checks `owner.json` still carries its nonce. If the lease was taken away, it
  stops the command's whole process group (`TERM`, then `KILL` after 5 s) and exits **70**,
  rather than let two gates finish.

**FIFO.** Only the first live waiter tries `mkdir`. A waiter is dead, and its file removed, by
the same rule as a holder, except that its TTL is 60 s.

**`lock-run` itself:**

- **Exit status.** The command's status (128+n if a signal ended it), 75 if its wait was
  cancelled, 70 if its lease was taken away, and 130 if it was interrupted while waiting. It
  releases the lease only while `owner.json` still carries its own nonce.
- **Signals and the terminal.** The command runs in a process group of its own, and `lock-run`
  passes `SIGINT`, `SIGTERM` or `SIGHUP` it receives on to that group **once**. On a terminal
  (when `lock-run` is in the foreground), the command's group becomes the terminal's foreground
  group, so it can use the terminal (`stty`, a prompt) and a Ctrl-C reaches it once, straight
  from the terminal. `lock-run` takes the terminal back when the command ends.
- **Re-entry.** The command runs with `CLAUDUCTOR_LOCK_HELD=<lock>`. A `lock-run` on the same
  lock inside it runs the command directly, so a script can wrap itself.

The page shows each queue: the holder (lane, pid, age, TTL, and "stale" if it is), then the
waiters in order, each with **CANCEL WAIT**. **RUN** starts the queue's `command` through
`lock-run` in the selected lane's worktree, detached, with its output in
`~/.clauductor/panel/<project hash>/queue-logs/`.

**In a project's gate script.** Put this at the top of `run-local.sh`. It needs git 2.5 or later
(worktrees) and no `--path-format` (git 2.31). It re-runs the script through the queue with
`clauductor lock-run` when clauductor is installed, and otherwise uses the plain-shell
implementation below (paste it into the script, or keep it beside it as `lease.sh`). A failure
to find the git directory runs the gate unqueued with a note, rather than aborting under
`set -e`:

```bash
#!/usr/bin/env bash
set -euo pipefail
# Serialise the full gate (port 3100) across every worktree of this repo.
lock=""
if common=$(git rev-parse --git-common-dir 2>/dev/null); then
  case $common in /*) ;; *) common="$PWD/$common" ;; esac
  lock="$common/clauductor/gate.lock"
fi
lane="${CLAUDUCTOR_LANE:-$(basename "$PWD")}"
if [ -z "$lock" ]; then
  echo "run-local: not in a git checkout; running without the gate queue" >&2
elif [ "${CLAUDUCTOR_LOCK_HELD:-}" != "$lock" ]; then
  if command -v clauductor >/dev/null 2>&1; then
    # TERM=dumb skips a terminal query at clauductor's startup (up to 5 s on a
    # pty that does not answer); lock-run gives the gate the real TERM back, or
    # leaves it unset if it was unset. Ask the environment, not the shell: bash
    # sets an unexported TERM=dumb of its own when TERM is unset.
    term_set="" term_val=""
    if printenv TERM >/dev/null 2>&1; then term_set=1 term_val=$(printenv TERM); fi
    CLAUDUCTOR_TERM="$term_val" CLAUDUCTOR_TERM_SET="$term_set" TERM=dumb \
      exec clauductor lock-run --lane "$lane" "$lock" -- bash "$0" "$@"
  fi
  if [ -f "$(dirname "$0")/lease.sh" ]; then
    . "$(dirname "$0")/lease.sh"        # the plain-shell protocol below
    lease_run "$lock" "$lane" bash "$0" "$@" && exit 0 || exit $?
  fi
  echo "run-local: neither clauductor nor lease.sh found; running without the gate queue" >&2
fi
# ...the gate itself...
```

`lock-run` prints `waiting for gate.lock (held by lane add-x (pid 4242) since 14:02:11)` to
stderr while it waits, so an agent reading the output knows why the gate is slow.

**The protocol in plain shell** (`lease.sh`). It interoperates with `lock-run` in both
directions: a test extracts this block from this page and runs it against `lock-run`. It sets an
`EXIT` trap while it waits and while it holds the lease.

<!-- lease.sh begin -->
```sh
# clauductor lease protocol v1 in plain POSIX shell: interoperates with
# `clauductor lock-run`. Usage: lease_run <lockdir> <lane> <command> [args...]
# Exit status: the command's; 75 if the wait was cancelled from the panel.
# Liveness and start time need ps; without it, kill -0 (EPERM still means alive)
# and /proc/<pid>/stat field 22. A start time from one source is never compared
# with one from the other, and an alive pid that cannot be verified is live.
lease_alive() {
  if command -v ps >/dev/null 2>&1; then [ -n "$(ps -o pid= -p "$1" 2>/dev/null || true)" ]; return; fi
  _e=$(kill -0 "$1" 2>&1) && return 0
  case $_e in *ermitted*) return 0 ;; esac
  return 1
}
lease_pstart() {
  _v=""
  if command -v ps >/dev/null 2>&1; then _v=$(LC_ALL=C ps -o lstart= -p "$1" 2>/dev/null | awk '{$1=$1; print}' || true); fi
  if [ -z "$_v" ] && [ -r "/proc/$1/stat" ]; then _v="proc:$(sed 's/.*) //' "/proc/$1/stat" | awk '{print $20}')"; fi
  printf '%s\n' "$_v"
}
# lease_proc_dead PID RECORDED_START: 0 (true) only when the pid is gone, or was
# reused (a start time from the same source that differs).
lease_proc_dead() {
  lease_alive "$1" || return 0
  _n=$(lease_pstart "$1")
  { [ -n "$2" ] && [ -n "$_n" ]; } || return 1
  _a=${2%%:*} _b=${_n%%:*}
  if { [ "$_a" = proc ] && [ "$_b" = proc ]; } || { [ "$_a" != proc ] && [ "$_b" != proc ]; }; then
    [ "$_n" != "$2" ]; return
  fi
  return 1
}
lease_get() { sed -n "s/.*\"$2\":\"\{0,1\}\([^\",}]*\).*/\1/p" "$1" 2>/dev/null | head -n 1 || true; }
lease_mtime() { stat -f %m "$1" 2>/dev/null || stat -c %Y "$1" 2>/dev/null || echo 0; }
# lease_dead FILE WAITER_TTL: 0 (true) when the record can be removed: on this host
# only when the holder AND its command (child_pid, written by lock-run) are dead.
lease_dead() {
  _p=$(lease_get "$1" pid); _s=$(lease_get "$1" pstart); _h=$(lease_get "$1" host)
  _cp=$(lease_get "$1" child_pid); _cs=$(lease_get "$1" child_pstart)
  _r=$(lease_get "$1" renewed); _t=$(lease_get "$1" ttl); [ -n "$2" ] && _t=$2
  if [ -n "$_p" ] && [ "$_h" = "$(hostname)" ]; then
    lease_proc_dead "$_p" "$_s" || return 1
    if [ -n "$_cp" ] && ! lease_proc_dead "$_cp" "$_cs"; then return 1; fi
    return 0
  fi
  [ "${_t:-0}" -gt 0 ] && [ "$(date +%s)" -gt $(( ${_r:-0} + _t )) ]   # another host: TTL
}
lease_holder_stale() {
  if [ ! -f "$1/owner.json" ]; then [ $(( $(date +%s) - $(lease_mtime "$1") )) -ge 10 ]; return; fi
  lease_dead "$1/owner.json" ""
}
lease_run() {
  # A quote or backslash would break owner.json, which readers then judge stale.
  _lock=$1 _lane=$(printf %s "$2" | tr -d '"\\'); shift 2
  _cmd=$(printf %s "$1" | tr -d '"\\')
  _w="$_lock.waiters" _nonce=$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')
  mkdir -p "$_w"
  _rec="{\"v\":1,\"nonce\":\"$_nonce\",\"pid\":$$,\"pstart\":\"$(lease_pstart $$)\",\"host\":\"$(hostname)\",\"lane\":\"$_lane\",\"cmd\":\"$_cmd\",\"started\":$(date +%s),\"renewed\":$(date +%s),\"ttl\":0}"
  _me="$_w/$(date +%s)000000000-$_nonce.json"
  printf '%s\n' "$_rec" > "$_me.tmp" && mv "$_me.tmp" "$_me"
  trap 'rm -f "$_me" "$_w/$_nonce.cancel"' EXIT
  _said=""
  while :; do
    if [ -e "$_w/$_nonce.cancel" ]; then echo "lease: wait cancelled from the panel" >&2; return 75; fi
    _first=""
    for _f in $(ls "$_w" 2>/dev/null | grep '\.json$' | sort -t- -k1,1n -k2); do
      if lease_dead "$_w/$_f" 60; then rm -f "$_w/$_f"; continue; fi
      _first=$_f; break
    done
    if [ "$_first" = "${_me##*/}" ] && mkdir "$_lock" 2>/dev/null; then
      printf '%s\n' "$_rec" > "$_lock/.owner.tmp" && mv "$_lock/.owner.tmp" "$_lock/owner.json"
      rm -f "$_me"
      trap 'if [ "$(lease_get "$_lock/owner.json" nonce)" = "$_nonce" ]; then rm -rf "$_lock"; fi' EXIT
      CLAUDUCTOR_LOCK_HELD=$_lock "$@" && _rc=0 || _rc=$?
      if [ "$(lease_get "$_lock/owner.json" nonce)" = "$_nonce" ]; then rm -rf "$_lock"; fi
      trap - EXIT
      return "$_rc"
    fi
    if [ "$_first" = "${_me##*/}" ] && [ -d "$_lock" ] && lease_holder_stale "$_lock"; then
      _judged=$(lease_get "$_lock/owner.json" nonce)
      if mkdir "$_lock.reclaim" 2>/dev/null; then
        if [ "$(lease_get "$_lock/owner.json" nonce)" = "$_judged" ] && lease_holder_stale "$_lock"; then
          echo "lease: reclaiming $_lock from a dead holder" >&2; rm -rf "$_lock"
        fi
        rmdir "$_lock.reclaim"; continue
      elif [ $(( $(date +%s) - $(lease_mtime "$_lock.reclaim") )) -ge 30 ]; then rmdir "$_lock.reclaim" 2>/dev/null || true
      fi
    fi
    [ -n "$_said" ] || { echo "lease: waiting for $_lock ($(lease_get "$_lock/owner.json" lane))" >&2; _said=1; }
    sleep 1
  done
}
```
<!-- lease.sh end -->

### Alerts and notifications

Alerts are derived from the state, never stored, against the `alerts` thresholds:

| Alert | When | Severity |
|---|---|---|
| waiting | a permission prompt, MCP elicitation or input request older than `waiting_seconds` | block |
| rate_limit | `StopFailure` with `error_type: rate_limit` | block |
| stop_failure | any other `StopFailure` | warn |
| no_auto_resume | `quota_auto_resume_stale` or `_disabled`: the lane will not continue by itself | warn |
| context | `context_window.used_percentage` ≥ `context_pct` | warn |
| idle | a live session idle longer than `idle_minutes` | info |
| quota | the 5-hour quota ≥ `five_hour_pct` (block at 100%) | warn |

Each shows in the **Alerts** panel. Only what blocks you **interrupts**: a macOS notification
goes out for a new `block` alert (waiting, rate_limit, quota at 100%) and for `no_auto_resume`.
Idle, context, quota and stop-failure alerts stay on the page. A notification goes out:

- once per stretch: an alert that clears and comes back notifies again. What was notified, and
  when each lane last was, is saved in `~/.clauductor/panel/<project hash>/notifier.json`, so a
  panel restart never re-notifies an alert that is still active, and the daily count survives it;
- grouped: the new alerts of one lane make one notification;
- rate-limited: at most one notification per lane per `min_interval_seconds`; later ones wait and
  go out when the interval ends, if still active;
- suppressed while that lane's terminal has keyboard focus in a visible page (the page reports
  focus over the terminal's WebSocket). A suppressed alert counts as seen.

The Alerts heading shows **interruptions today**. Notifications run
`/usr/bin/osascript -e 'on run argv' -e 'display notification (item 2 of argv) with title (item 1 of argv)' -e 'end run' -- <title> <text>`:
the text arrives as an argument and is never spliced into AppleScript source, so a quote in a lane
name cannot run `do shell script`. The `--` matters: osascript keeps parsing options among its
arguments, so without it a title starting with `-e` would be read as more script. The title is
the config's `name` only while the config is trusted; `name` must be one line of plain text that
does not start with `-`. The first one may make macOS ask whether the panel may send
notifications.

### Quota guard

At or above `quota_guard.five_hour_pct`, **+ LANE** and **RESTORE ALL** refuse, and the dialog
offers an override checkbox. An expired window (past its `resets_at`) or an unknown one never
blocks: the guard acts only on a number it has.

### Restore after a reboot

tmux lanes do not survive a reboot. When the panel starts, every registered lane whose tmux
session is gone is **restorable**, and a banner offers **RESTORE ALL** (each lane also keeps its
own **RESUME**). A restore:

- runs `claude --resume <the lane's own session id>` in its worktree, or `--session-id <id>` for
  a session that never had a prompt. It **never** uses `--continue`;
- never resumes a session twice. A lane is skipped if `claude agents` shows its session id in a
  running process, if a second lane carries the same session id, or if `claude agents` cannot be
  read (then nothing is restored). Its worktree must still exist;
- types nothing into the lane.

**The resume dialog.** A session that was idle for more than an hour and holds more than 100k
tokens makes claude ask, before the first message, whether to resume from a summary. The panel
does not answer it for you. Every lane restored on a conversation shows in **Needs you** ("Restored
lane") until you prompt it or it goes busy. Open its terminal, answer the dialog if it is there,
and continue.

### Config trust

`panel.json` is in the repository, and it names commands the panel runs (cards, queue RUN) and
prompts it types (templates). The panel records the file's SHA-256 under
`~/.clauductor/panel/<project hash>/trusted-config.json` the first time it runs on it, and logs
the hash at every start. When the file changes (a pull, say), the panel still starts, but its
cards, queue RUN and templates stay **off** until you run `clauductor panel trust` (a running
panel follows within 5 s) or start with `--trust-config`. A red banner names both hashes.
`clauductor panel install` trusts the config it installs.

### What v2 changed in how signals are read

- **Notifications.** Each of the 12 documented `notification_type` values maps to what it does
  to the session, whether it goes in **Needs you**, and how severe it is:

  | Type | Effect | Needs you | Severity |
  |---|---|---|---|
  | `permission_prompt` | waiting | yes | block |
  | `elicitation_dialog`, `elicitation_url_dialog` | waiting | yes | block |
  | `agent_needs_input` | waiting | yes | block |
  | `quota_auto_resume_stale`, `quota_auto_resume_disabled` | none | yes | warn |
  | `idle_prompt` | idle: your move | no: **Done** | info |
  | `agent_completed` | none | no: **Done** once the turn is over | info |
  | `elicitation_complete`, `elicitation_response`, `quota_auto_resume_fired` | answers a waiting note | no | info |
  | `auth_success` | none | no | info |
  | anything else | none; shown on the session, counted | **never** | unknown |

  **Needs you** holds blocking states only. **Done · your move** lists finished turns apart.
- **New hook events.** `StopFailure` (its `error_type`), `PermissionRequest`, `PreCompact` and
  `PostCompact` (shown as "compacting"), and `CwdChanged` (recorded in the feed). The panel only
  **observes** `PermissionRequest`: `/hook` answers `204` with an empty body, which Claude Code
  documents as "no decision", so the permission dialog proceeds as usual. `/hook` takes no token,
  so it must never answer. Only subscribed event names are applied; others are counted.
- **Binding.** A session is bound to its lane once. A lane the panel started is bound by the
  session id it assigned (`--session-id`), whatever the event's `cwd`. Any other session is bound
  by its `cwd` at first sight, and a later `cd` does not move it.
- **Quota.** A window whose `resets_at` has passed is dropped, and its gauge says "reset".
- **`claude agents`.** `id`, `state` and the `waitingFor` enum (permission prompt, input needed,
  sandbox request, worker request, dialog open) are decoded. The poll passes `--cwd <the
  deepest directory holding every worktree>`, but only after a cross-check: every 5 minutes it
  also runs unfiltered, and if the filter drops any session of the project, it polls unfiltered.
  One poll measured 93–103 ms wall (p50 98 ms), about 105 ms CPU and 148 MB peak RSS on
  2.1.284; at 2 s that is about 5% of a core. So it backs off to 5 s while hooks are flowing (a
  hook in the last 30 s), and a kick still polls at once.
- **Overflow.** A hook body dropped because the panel fell behind is counted apart from foreign
  drops, and raises a banner.
- **`settings.json`.** The hook install re-reads the file immediately before its rename, and redoes
  the edit if another writer changed it meanwhile (up to 5 times, then it refuses).
- **Version pinning.** The subagent pairing, the missing `SessionStart` HTTP hook and the recorded
  fixtures were verified on Claude Code **2.1.284**. The panel reads `claude --version`; on any
  other version the subagent list says "approximate" and a warning bar says why.

### Observability

The footer shows the panel's own counters: hook events, status posts, drops by cause (foreign
`cwd`, overflow, malformed, unknown event name), unknown notification types, the last, mean and
worst `claude agents` poll latency and its current interval, the filter in use and why, the
Claude Code version against the one the heuristics were verified on, and notifications sent or
failed.

## Not yet

- No removing a worktree from the page.
- One project per panel.
- No remote access; the panel is loopback only.
- The panel never answers a permission request. Doing it from the browser would need a
  token-carrying HTTP hook (`headers` plus `allowedEnvVars`), and is not planned.
