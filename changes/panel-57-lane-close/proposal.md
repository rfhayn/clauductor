**Status:** awaiting approval
**Roadmap row:** PANEL-57
**Risk:** normal

## Why
Nothing but the owner can close a panel lane. Two things close one today:
- **Close lane** on the page (`POST /api/p/<project>/lanes/<id>/close`, PANEL-17), behind the
  page's cookie and its `Origin`.
- **Auto-close on merge** (PANEL-20, `lanes_auto_close: "on_merge"`). It acts only when claude is
  idle and the plan removes both the worktree and the branch; otherwise it asks in Needs you. It
  is off unless a project's config turns it on, and the template's default is off.

So when a lane's work is done and auto-close is off or holds back, no session or skill can finish
the job. session-close's own rule says to close a lane once its work has landed, been handed off
or stopped, and the only way to follow it is to ask the owner to click (#242, the owner's request
of 2026-10-24).

## What changes
`clauductor lane close <id>`, run in any checkout of a project the panel serves, asks the running
panel to close that lane through Close lane's own plan and guards. Nothing about what Close removes
changes: the worktree goes only when it is clean, the branch only when it is merged, and nothing is
forced.
- **The plan first** (D3). With no flag the command prints what Close would remove and keep, and
  changes nothing. `--yes` closes the lane with the plan's offers as consent, as the page's
  **Confirm** does, and prints what was removed and kept.
- **Only a lane whose work has ended, never the caller's own** (D2). The panel refuses unless the
  lane's claude has exited, or is idle with no background work of its turn still in flight (a
  subagent, a workflow, a background shell, a teammate) and no gate lease held or waited for. Auto-close reads the same verdict, so it too stops
  closing a lane whose turn ended on background work: a fix to PANEL-20 the owner approves with
  D2. A lane cannot close itself.
- **Reached over a socket only the owner's user can open** (D1). The panel serves the close on
  `~/.clauductor/panel/cli.sock` (mode 0600, in the 0700 panel directory). No browser can reach it,
  and the CLI holds no token.
- **An interrupted command leaves the close finished** (D4). Once the panel starts a close, it
  finishes it.
- **Its caller in production** (D5). session-close's *Close the lanes* rule runs the command for
  each idle lane whose pull request merged and that the panel left open, including one whose plan
  keeps the worktree or branch, and names what stayed in its final message.
- **Allowed without a prompt** (D7). The template's settings allow `Bash(clauductor lane close *)`
  and nothing wider, so a session close does not wait on the owner twice per lane.
- **Beyond the row's scope column** (`framework/internal/cmd`, `framework/internal/panel/web`,
  `docs/panel.md`, `template/.claude/skills/`, `plugin/`): `framework/internal/panel/run.go` (the
  socket's listener and the owner record), `framework/internal/panel/live.go` (its rewrites of the
  owner record keep the socket), `framework/internal/panel/install/singleton.go` (the owner record
  names the socket), `framework/internal/panel/install/candidate.go` (`MainWorktree`, which
  `mainRoot` then calls), `framework/internal/panel/autoclose.go` and
  `framework/internal/panel/state/model.go` (the shared verdict), `template/.claude/settings.json`
  (D7), this repository's own `.claude/skills/session-close/SKILL.md` and `.claude/settings.json`
  (synced), and `README.md`'s command table.

## What the existing specs already guarantee
None of the seven living specs covers closing a lane. PANEL-17 and PANEL-20 predate the specs:
their behaviour is in `docs/panel.md` (*Close lane*, *Close a lane when its PR merges*) and their
tests (`lanes/close_test.go`, `close_integration_test.go`, `autoclose_integration_test.go`). This
change modifies no requirement:
- **ATTACH-2** (attaching and serving write nothing in the repository): the command closes a lane
  exactly as the page's Close lane does, so an attached project gets nothing the page does not
  already do to it.
- **STALL-1** (a session whose turn ended on a subagent or workflow still running is *working*,
  not stalled): the new verdict reads the stall's own rule, `sessionWorking`, and adds to it the
  open tasks STALL-1 lists (a shell, a monitor, a teammate) and the leases STALL-1-S3 reads. It
  changes neither STALL-1's reading nor the stall's.
- **NEWLANE-1 to NEWLANE-5** cover starting a lane. Starting one from the command line is out of
  scope (D6).

This change adds a new capability, `panel-lane-close` (LANECLOSE-1 to LANECLOSE-4).

## Out of scope
- **Starting a lane from the command line** (`POST /lanes`): D6. No row is opened unless the owner
  asks for one.
- **Stop, interrupt, restart, resume or remove a worktree from the command line:** the socket
  serves the close and nothing else (D1). This is a refusal, not a deferral.
- **A lane that closes itself when its turn ends:** D2 refuses a self-close. For a lane on a
  merged branch, auto-close (`on_merge`) already closes it once its work ends. This is a refusal.
- **`--json`, and a `lane list` command:** no production caller reads either (rule 3). An unknown
  id is answered with the project's lanes and their branches (LANECLOSE-1-S4), and with the
  worktree's folder name (D5) that is enough for a session to find the lane it means.
- **Any change to what Close removes or keeps,** or to the page's route and its guards.

## How we'll know
- **Signal:** in the 14 days after it merges, no idle lane whose pull request merged survives a
  session close with a Needs-you ask (`PR merged: close lane <id>?`, `autoclose.go:97`) still standing: at the next session
  start, the page shows none. Each lane a session closed has its `closed by clauductor lane close`
  line in the panel's log. A lane closed that the owner did not mean to lose (a working lane, or
  one whose worktree or branch held unmerged work) is a miss and gets a fix row. If no session
  close in the 14 days met such a lane, the signal is "not exercised" and the check is re-queued
  for 14 days more.
- **Check after:** 14 days
