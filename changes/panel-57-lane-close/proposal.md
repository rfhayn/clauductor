**Approved:** 2026-10-24 by Rich · design 0d8677f3b540
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
- **Only a lane whose work has ended and landed, never the caller's own** (D2). The panel refuses
  a lane that holds or waits for a gate lease. Otherwise it refuses unless the lane's claude has
  exited, or is idle with no background work of its own session's turn still in flight (a
  subagent, a workflow, a background shell, a teammate) and its branch is merged: an idle lane is
  not a finished one. A lane whose tmux session is gone, which the page offers to restore, closes
  only when its branch is merged. Auto-close reads the same verdict for the work, so it too stops closing a
  lane whose turn ended on background work, that holds a lease, whose claude has not yet been
  read by `claude agents`, or whose session ended while claude still runs (after a `/clear`,
  which the owner then closes from the page): a fix to PANEL-20 the owner approves with D2. Both
  make their final check under the lock the close takes, which narrows the window in which a lane
  that starts working is stopped, but cannot close it: the panel reads claude every few seconds. A lane cannot close itself.
- **Reached over a socket only the owner's user can open** (D1). The panel serves the close on
  `~/.clauductor/panel/cli.sock` (mode 0600, in the 0700 panel directory). No browser can reach it,
  and the CLI holds no token.
- **An interrupted command leaves the close finished** (D4). Once the panel starts a close, it
  finishes it.
- **Its caller in production** (D5). session-close's *Close the lanes* rule runs the command, from
  the repository, for each lane whose pull request merged and that the panel left open, including
  one whose plan keeps a dirty worktree (and with it the branch). It confirms with `--yes` only
  when the plan's first line names that merged branch, and names what stayed, and any refusal, in
  its final message.
- **Allowed without a prompt** (D7). The template's settings allow `Bash(clauductor lane close *)`
  and nothing wider, so a session close does not wait on the owner twice per lane. The command
  takes its project from the current directory alone (no `--project`), so an allowed call reaches
  only the project it runs in.
- **Beyond the row's scope column** (`framework/internal/cmd`, `framework/internal/panel/web`,
  `docs/panel.md`, `template/.claude/skills/`, `plugin/`): `framework/internal/panel/run.go` (the
  socket's listener and the owner record), `framework/internal/panel/live.go` (its rewrites of the
  owner record keep the socket), `framework/internal/panel/install/singleton.go` (the owner record
  names the socket), `framework/internal/panel/install/candidate.go` (`MainWorktree`, which
  `mainRoot` then calls), `framework/internal/panel/autoclose.go`,
  `framework/internal/panel/state/model.go` and `framework/internal/panel/state/stall.go` (the
  shared verdict, and the leases it reads), `framework/internal/panel/lanes/close.go` (the plan
  says whether the branch is merged), `framework/internal/panel/config/fields.go` (auto-close's
  reference line) and the `docs/panel.schema.json` it generates, `template/.claude/settings.json`
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
- **`--project`, closing a lane of another project:** no production caller needs it, and with D7's
  allow rule it would let a session close another project's lanes. This is a refusal.
- **`--json`, and a `lane list` command:** no production caller reads either (rule 3). An unknown
  id is answered with the project's lanes and their branches (LANECLOSE-1-S4), and with the
  worktree's folder name (D5) that is enough for a session to find the lane it means.
- **Any change to what Close removes or keeps,** or to the page's route and its guards.

## How we'll know
- **Signal:** in the 14 days after it merges, no lane whose pull request merged and whose work
  has ended (its `CloseHeld` empty) survives a session close with a Needs-you ask (`PR merged:
  close lane <id>?`, `autoclose.go:97`) still standing: at the next session start, the page shows
  none. A lane still asked about because D2 holds it (a lease, background work, claude busy) is
  not a miss; its ask names why. One held because its session ended after a `/clear` is not a
  miss either, but is counted and named in the check, so a hold that is common shows. Each lane a session closed has its `closed by clauductor lane close`
  line in the panel's log. A lane closed that the owner did not mean to lose (a working lane, or
  one whose worktree or branch held unmerged work) is a miss and gets a fix row. If no session
  close in the 14 days met such a lane, the signal is "not exercised" and the check is re-queued
  for 14 days more.
- **Check after:** 14 days
