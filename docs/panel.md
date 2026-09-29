# `clauductor panel` — a local web panel over your Claude sessions

`clauductor panel` serves a read-only, live dashboard of every Claude Code session working in
one project: which lanes (worktrees) have a session, whether each is busy, waiting or idle, its
context %, its running subagents, what needs you, the account quota, open PRs, and any cards
the project defines.

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

Keeping the panel current costs **no model tokens**. It never reads transcripts.

## Run it

```bash
clauductor panel                              # project = git toplevel of the current directory
clauductor panel --project ~/Development/app  # or name it
clauductor panel --config /tmp/panel.json     # use a config outside the repo
clauductor panel --port 4393 --no-open        # print the URL instead of opening a browser
clauductor panel --uninstall-hooks            # remove the panel's hooks and exit
```

v0 watches one project per run. Stop it with Ctrl-C.

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
  project.
- **Lanes (left).** One per worktree with a live session or recent activity. The stripe is green
  for busy, amber for waiting, grey for idle, and red when the lane is busy but no hook has
  arrived from it for 60 s. The chip is the lane type from `lanes`. Worktrees with no session are
  listed underneath.
- **Selected lane (centre).** Its sessions (pid, status, context %, model, est. $), running
  subagents with their age, and the lane's own event feed. v0 has no terminal. Workflow agents
  can stop under a different `agent_id` than they started with, so a stop with an unknown id
  retires the oldest running agent of the same type. A session that `claude agents` reports idle
  for 10 s, or gone, has its running list cleared. The hook `Stop` clears nothing, because
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

## Security model

A dashboard of your sessions is private, and later versions will add terminals, so the page is
locked down even on loopback.

- **Loopback only.** The server binds `127.0.0.1` and refuses to run if the bound address is not
  loopback. If the port is taken, it **exits with an error** and never falls back to another port
  (the hooks post to a fixed URL). A refused start does not touch `settings.json` or the marker.
- **Per-launch token.** Each start makes 32 random bytes and opens
  `http://127.0.0.1:<port>/?t=<token>`. The server swaps the token for an `HttpOnly;
  SameSite=Strict` cookie and redirects to `/`, so the token leaves the address bar. Every route
  except `/hook` and `/status` needs the cookie (401 otherwise).
- **DNS rebinding and cross-site requests.** `Host` must be `127.0.0.1:<port>` or
  `localhost:<port>` on every route. Every state-changing request (today only `POST
  /api/refresh`) must carry an `Origin` of the panel itself. No CORS headers are sent. The page
  is served with a restrictive CSP, `X-Frame-Options: DENY` and `Referrer-Policy: no-referrer`.
- **The ingest endpoints** (`/hook`, `/status`) take no token, since a session cannot know it.
  They accept `POST` from a loopback peer only, refuse any request carrying `Origin` or
  `Sec-Fetch-Site` (Claude Code sends neither; a browser always does), cap the body at 256 KB,
  answer `204` before processing, and never execute anything. The worst a local process can do
  is post fake lane events.
- **Events from other projects are dropped.** An event counts only if its `cwd` is inside one
  of the project's worktrees, as `git worktree list --porcelain` reports them. That list is the
  authority, never a hand-kept list. It is re-read every 10 s, and early when a worktree is
  added or removed or when an event arrives from an unknown `cwd`.
- **Nothing is written to disk** except the marker file `~/.clauductor/panel/port` (removed on
  SIGINT/SIGTERM) and the hook install. Hook bodies include prompt text. The panel keeps only a
  short one-line summary per event, in a 200-event in-memory ring buffer. It never reads
  transcript files; a test fails if any panel source mentions one.

## Not in v0

v0 only watches. It has no terminals, no lane start/stop, no gate queue and no alerts; those are
v1 and v2. It runs one project at a time.
