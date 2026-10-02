**Status:** awaiting approval
**Roadmap row:** PANEL-25
**Risk:** normal

## Why

The panel is meant to idle for days under the login agent, and it mostly does: measured on
2026-10-02 on the owner's Mac with two projects registered, the panel process sits at about 27 MB
RSS and 0% CPU. What it costs is the processes it starts: **about 35 a minute with nothing
happening**, and closer to 100 with lanes running. The cadences account for all of it
(`DefaultTicks()` in `framework/internal/panel/run.go`, per project):

| Poll | Today, no lane, no page, no hook | A minute |
|---|---|---|
| `git worktree list` | every 10 s (`Ticks.Worktrees`) | 6 |
| tmux `list-panes` | every 10 s with no lane (`TmuxIdle`) | 6 |
| `claude agents` | every 15 s (`AgentsQuiet`), plus two more every 5 min for the `--cwd` cross-check | 4.4 |
| `gh pr list` | every 60 s (`Ticks.PRs`), page or no page | 1 |
| **per project** | | **17.4** |

Two projects make about 35, plus about 1 a minute from the machine (`claude --version` and
`claude auth status`, each every 10 min) and the projects' own interval cards and suggest
commands.

None of these reads has a reader while nothing is happening:

- **`gh pr list` feeds the page and auto-close only.** No alert or notification reads the open
  pull requests (`state/model.go` `ApplyPRs`; readers: `readiness_source.go`, already gated on a
  page in view, and `autoclose.go`). Yet it calls GitHub every minute per project, with no page
  open and no lane to close.
- **With no lane, the tmux and worktree polls watch for things the panel is told about anyway.**
  A lane start kicks tmux, the worktrees and `claude agents` at once (`live.go`,
  `lanes.Changed`). A worktree added or removed kicks the worktree read within 2 s, by a `stat` of
  git's worktree registry, not a spawn (`runtime.go`, the `worktrees` source's `watch`). A hook in
  a quiet stretch kicks `claude agents` at once (`hookSeen`).
- **The page already gates the dashboard's reads** (`ps`, `git status`, merge readiness, merged
  PRs) on a page having said it is in view in the last 90 s (`web.Server.PageVisible`, PANEL-11).
  The four polls above never learned to.

Separately, the login agent opens a browser tab at every login (`run.go`: under `--launchd` it
opens once per login, held to once per 5 minutes by `browser-opened`). `clauductor panel` has a
`--no-open` flag (`internal/cmd/panel.go`), but `panel install` never writes it into the plist
(`install/launchd.go` `renderPlist`), and `Run` checks `Launchd` before `NoOpen`, so the flag
would be ignored under the agent even if it were there.

## What changes

An idle panel spawns almost nothing, and nothing that needs it waits:

- **No page in view, no open-PR read every minute.** The open pull requests (`gh pr list`) are
  read every 60 s while a page is in view, as today, and not at all otherwise.
- **Auto-close reads the merged pull requests itself.** For a project with a lane auto-close
  watches, one `gh pr list --state merged`, bounded by merge time, every 3 minutes, page or not,
  so a lane closes even when its pull request was opened and merged between two reads (a gap
  that exists today), and after a merge train. A project with no such lane makes no call.
  Auto-close is off by default, so for most projects the panel's own polls make no GitHub call
  while no page is in view. The project's own commands may still: with the template's
  `panel.json`, the fix suggestions run `gh issue list` every 5 minutes and the metrics command
  runs two `gh` reads every 10.
- **The metrics command waits for a page** (D10, the owner's call): its only reader is the
  Metrics view, so it runs while a page is in view, and at once when one comes back.
- **No lane and no page in view: the polls back off to 5 minutes.** `claude agents` (once no hook
  has arrived for 5 minutes, today's quiet rule), `git worktree list` and tmux `list-panes` each
  wait 5 minutes between timed polls for that project.
- **Anything that needs the panel wakes it at once**: a hook polls `claude agents`; a lane start
  polls tmux, the worktrees and `claude agents`; a worktree added or removed is read within 2 s;
  a page coming into view polls every backed-off source, the open pull requests included.
- **The panel counts what it spawns**, per project and by command, and tells idle time from busy
  time. The rate shows in the observability footer and `/api/state`, and the login agent writes
  each project's last hour to its log once an hour. Each auto-close writes a log line with the
  merge time and the close time.
- **`clauductor panel install --no-open`** installs a login agent that never opens a browser tab.
  `clauductor panel open`, the app and a bookmark still open the page.

## What the existing specs already guarantee

No living spec covers the panel's polling, auto-close or the login agent (`specs/` holds only its
README; PANEL-29's `panel-new-lane` delta is not yet merged and covers the New lane dialog only).
Nothing here modifies or contradicts a requirement. This change adds the capability spec
`panel-idle-cost`.

The behaviour it alters is documented, not specified, in `docs/panel.md`: the sources table at
the top (the cadence of `claude agents`, `git worktree list`, `gh pr list` and tmux), *What the
panel reads, and when*, *Close a lane when its PR merges* ("when the branch's pull request leaves
the open list the panel already polls"), *The lane registry* ("every 10 s with none") and *The
launchd agent*. Each is updated to match.

## Out of scope

- **The cadence while lanes run or sessions are active.** `claude agents` and tmux every 2 s with
  a lane, and `claude agents` every 2 s for the 4.5 minutes after a session outside a lane goes
  quiet, stay as they are: first prompts, waiting notifications ("a prompt answered in the
  terminal fires no hook", `docs/panel.md`, *Current or stale*) and auto-resume read them. Not
  owned: the spawn count this change adds (D5) is what would show whether a row is worth it,
  since its hourly lines give the busy minutes' rate too.
- **The project's own interval cards and suggest commands** keep `panel.json`'s refresh rules
  (the template's: the fix suggestions every 5 min, which call `gh issue list`, and the health
  card every 10 min; about 0.3 a minute per project). The project chose them. Holding them too
  while no page is in view is D10's alternative. Not owned otherwise, for the same reason. The
  metrics command is D10.
- **The machine's `claude` calls.** `claude --version` stays every 10 minutes. PANEL-28 takes
  `claude auth status` off its timer (start and **Refresh** only); this change does not touch
  either.
- **The panel's `claude` calls racing a session's login refresh**: PANEL-28, which builds first
  (this row's Deps). Fewer `claude agents` calls make that race rarer; they do not settle it.
  This change builds on PANEL-28's machine-wide `claude` slot: the spawn count sits inside it
  (D6), and a poll the slot skips retries in seconds, never after the dormant 5 minutes (D9).
- **Processes the panel starts on a person's action**, not on a timer: an OS notification, a
  terminal attach, a queue RUN, Terminal.app, the browser, a lease holder's start time. The
  spawn count leaves them out and says so (D6).

## How we'll know

- **Signal:** while a project is **idle** (no lane, no page in view, no hook for 5 minutes), it
  spawns **at most 1.2 processes a minute, down from about 17.4**, and the machine's own calls stay
  under 0.2 a minute (`claude --version` every 10 minutes, once PANEL-28 has taken `auth status`
  off its timer): two idle projects together **under 3 a minute, down from about 35**. Read
  from the login agent's hourly lines in `~/.clauductor/panel/logs/panel.log` ("spawns, last hour,
  <project>: idle N min, M spawns …"), over the hours with at least 30 idle minutes. Neither the
  log nor `/api/state` makes a page "in view". Where a project has turned `lanes_auto_close` on
  (neither registered project has today; it is off by default), each auto-close log line shows the
  close within 5 minutes of the merge.
- **Check after:** 7 days
