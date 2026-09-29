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
| `claude agents --json` | polled every 2 s | which sessions exist, busy / waiting / idle |
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
  "name": "Standing Tee",
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
  real subscription budget. **est. $ · tracked sessions (list price)** is the sum of the status
  line's `total_cost_usd` over the sessions the panel tracks now: live ones, and ones heard from
  in the last 30 minutes. A session forgotten after that drops out of the sum. It is a
  list-price estimate, not a bill.
  `hooks` counts hook events accepted, status-line posts, and events dropped as outside the
  project. **+ LANE** opens the Start dialog; it is disabled, with the reason on hover, while
  lanes cannot start (see *Subscription only*).
- **Lanes (left).** One per worktree with a live session or recent activity. The stripe is green
  for busy, amber for waiting, grey for idle, and red when the lane is busy but no hook has
  arrived from it for 60 s. The chip is the lane type from `lanes`. Worktrees with no session are
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

for `UserPromptSubmit`, `Stop`, `SubagentStart`, `SubagentStop`, `Notification` and
`SessionEnd`. `SessionStart` is left out because HTTP hooks do not fire for it (Claude Code
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
clauductor panel open         # checks /healthz, then opens http://127.0.0.1:4393/?t=<token>
clauductor panel uninstall    # bootout, then remove everything the agent owns
```

`open` sends the token only to the panel it expects. `/healthz` answers `ok pid=<pid>`, and
`open` compares that with `~/.clauductor/panel/pid`; on a mismatch it refuses rather than hand
the token to whatever holds the port.

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

- **Loopback only.** The server binds `127.0.0.1` and refuses to run if the bound address is not
  loopback. If the port is taken, it **exits with an error** and never falls back to another port
  (the hooks post to a fixed URL). A refused start does not touch `settings.json` or the marker.
- **Per-launch token.** Each start makes 32 random bytes and opens
  `http://127.0.0.1:<port>/?t=<token>`. The server swaps the token for an `HttpOnly;
  SameSite=Strict` cookie and redirects to `/`, so the token leaves the address bar. Every route
  except `/hook`, `/status` and `/healthz` needs the cookie (401 otherwise). `/healthz` answers
  only `ok` and the panel's PID. Under launchd, the token persists in a 0600 file instead (see above).
- **DNS rebinding and cross-site requests.** `Host` must be `127.0.0.1:<port>` or
  `localhost:<port>` on every route. Every state-changing request (every `POST`) must carry an
  `Origin` of the panel itself. No CORS headers are sent. The page is served with
  `X-Frame-Options: DENY` and `Referrer-Policy: no-referrer`.
- **Content Security Policy: this origin only.** `script-src 'self'`, `font-src 'self'`,
  `connect-src 'self' ws://<host>`, and no `'unsafe-inline'` anywhere. The page's JS and CSS are
  files embedded in the binary. So are xterm.js and the three fonts. The page loads nothing from
  a CDN or Google Fonts. xterm.js creates `<style>` elements at run time, so `style-src` allows
  one per-response nonce as well; `panel.js` stamps it on those elements.
- **The terminal endpoint** (`GET /ws/term?lane=<id>`) is a shell into a lane, and it is the most
  guarded route. It needs all of the following:
  - the Host check;
  - the cookie;
  - an `Origin` exactly equal to `http://<the Host>`, meaning scheme, host and port;
  - **a single-use ticket**. The page gets one from `POST /api/lanes/<id>/ticket`, which checks
    `Origin`. A ticket is valid for 30 s, for that one lane, and is sent in the
    `Sec-WebSocket-Protocol` header, never in the URL. The cookie alone is not enough, because
    cookies are not isolated by port (RFC 6265 §8.5). A page on another loopback port, such as a
    dev server on `:3100`, is same-site, and the browser sends it the panel's cookie. Only the
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
  `{"type":"resize","cols":…,"rows":…}` and `{"type":"alive"}`; anything else closes the
  connection.
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
  forget, terminal-app. A start names a lane type (checked against the config), a mode, a lane
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
| Chakra Petch, IBM Plex Sans, JetBrains Mono | fontsource 5.3.0, latin | SIL OFL 1.1 | The page's fonts, in `web/static/fonts/` with their licences. |

To update a vendored file, download it with `npm pack <package>@<version>`, copy the file from
`lib/` (or `files/` for fonts) together with its `LICENSE`, and update this table.

## Not in v1

- No gate queue, lane templates or alerts. Those are v2.
- No automatic restore after a reboot. The registry shows the lanes a reboot killed as orphans,
  each with **RESUME**; v2 can resume them all at once.
- No removing a worktree from the page.
- One project per panel.
- No remote access; the panel is loopback only.
