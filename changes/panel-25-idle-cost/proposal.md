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

Two projects make about 35, plus a few a minute from the machine (`claude --version`, `claude
auth status`, every 10 min) and from the projects' own interval cards and suggest commands.

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

- **No page in view, no GitHub call per minute.** `gh pr list` runs every 60 s while a page is in
  view, as today. With no page in view it runs only for a project that has a lane auto-close
  watches, every 3 minutes, so that lane still closes when its pull request merges. A project
  with no such lane calls GitHub not at all until a page comes back.
- **No lane and no page in view: the polls back off to 5 minutes.** `claude agents` (once no hook
  has arrived for 5 minutes, today's quiet rule), `git worktree list` and tmux `list-panes` each
  run once every 5 minutes for that project.
- **Anything that needs the panel wakes it at once**: a hook polls `claude agents`; a lane start
  polls tmux, the worktrees and `claude agents`; a worktree added or removed is read within 2 s;
  a page coming into view polls every backed-off source, the pull requests included.
- **The panel counts what it spawns**, by command, and shows the rate in the observability footer
  and `/api/state`. The login agent writes it to its log once an hour.
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
  owned: the spawn count this change adds (D5) is what would show whether a row is worth it.
- **The project's own interval commands**: cards, suggest commands and the metrics command run on
  `panel.json`'s refresh rules (the template's defaults: the fix suggestions every 5 min, the
  metrics command and one card every 10 min, about 0.4 a minute per project). The project chose
  them. Not owned, for the same reason.
- **`claude --version` and `claude auth status`**, the machine's, every 10 minutes: unchanged.
- **The panel's `claude` calls racing a session's login refresh**: PANEL-28. Fewer `claude
  agents` calls make that race rarer; they do not settle it.

## How we'll know

- **Signal:** with two projects registered (the template's `panel.json`), no lane and no page in
  view, the panel spawns **at most 3 processes a minute, down from about 35**, read from its own
  count: the login agent's hourly log line (`~/.clauductor/panel/logs/panel.log`, "spawns in the
  last hour") or `/api/state`'s `obs`, neither of which makes a page "in view". And no lane with
  `lanes_auto_close` stayed open more than 5 minutes after its pull request merged.
- **Check after:** 7 days
