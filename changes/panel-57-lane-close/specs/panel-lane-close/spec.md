## Purpose
`clauductor lane close <id>` lets an agent or the owner close a finished panel lane from the
command line, through Close lane's own plan and guards, so a session can follow its own rule to
close its lanes without the owner's click, and without any way to close a lane that is working.

## ADDED Requirements

### Requirement: Close a lane from the command line through Close lane's plan and guards
WHEN `clauductor lane close <id>` runs in any checkout of a project the running panel serves, for
a lane the second requirement does not refuse, THE SYSTEM SHALL print the plan Close lane's
confirmation lists and change nothing, and with `--yes` SHALL close the lane through Close lane's
own plan and guards, printing what was removed and what was kept, each with why.

#### Scenario: [LANECLOSE-1-S1] Without --yes, the plan is printed and nothing changes
- **GIVEN** a running panel serving a project with an idle lane on a clean worktree whose branch
  is merged
- **WHEN** `clauductor lane close <id>` runs with no flag
- **THEN** it prints the plan's **Removes** and **Keeps** lines and its notes, as the page's
  confirmation lists them
- **AND** the lane, its tmux session, its worktree and its branch are as they were
- **AND** it exits 0

#### Scenario: [LANECLOSE-1-S2] With --yes, a clean merged lane is closed as the page closes it
- **GIVEN** a running panel serving a project with an idle lane on a clean worktree whose branch
  is merged
- **WHEN** `clauductor lane close <id> --yes` runs
- **THEN** the lane is gone from the registry, its tmux session is gone, its worktree is removed
  and its branch deleted
- **AND** it prints the **Removed** lines and exits 0
- **AND** the panel's output has one line naming the lane, its project and `clauductor lane close`

#### Scenario: [LANECLOSE-1-S3] A dirty worktree stays, and says why
- **GIVEN** an idle lane whose branch is merged and whose worktree holds an uncommitted change
- **WHEN** `clauductor lane close <id> --yes` runs
- **THEN** the lane is stopped and forgotten, as Close lane stops it
- **AND** the worktree and its branch stay, and the **Kept** lines say why
- **AND** it exits 0

#### Scenario: [LANECLOSE-1-S4] An unknown lane is refused with the project's lane ids
- **GIVEN** a running panel serving a project whose view lists lanes `a` and `b`
- **WHEN** `clauductor lane close x --yes` runs
- **THEN** it fails naming `x` and the lanes `a` and `b`, each with its branch
- **AND** nothing changes, and it exits 1

#### Scenario: [LANECLOSE-1-S5] Run inside a lane's worktree, the command addresses the main checkout's project
- **GIVEN** a registered project, and the current directory inside one of its linked worktrees
- **WHEN** `clauductor lane close <id>` runs
- **THEN** it asks the panel about the project whose root is the repository's main checkout
- **AND** run from a directory in no project the panel serves, it is refused naming
  `clauductor panel list`, and the panel is asked nothing

### Requirement: Close only a lane whose work has ended and landed, never the caller's own
WHEN the command asks to close a lane THE SYSTEM SHALL refuse a lane that holds or waits for a
queue's lease, and otherwise SHALL refuse unless that lane's claude has exited, or its branch is
merged and its claude is gone or idle on a current reading with no background work of its own
session's turn still in flight, by the one verdict auto-close also reads for its work, and SHALL
refuse a lane's own id when run inside that lane.

#### Scenario: [LANECLOSE-2-S1] A lane whose claude is working is refused, and keeps running
- **GIVEN** a lane whose claude is working
- **WHEN** `clauductor lane close <id> --yes` runs
- **THEN** it is refused with "claude is working", and exits 1
- **AND** the lane's tmux session, worktree and branch are untouched
- **AND** a lane waiting on the owner, a lane starting, and a lane whose reading is not current
  are refused the same way, each in auto-close's words
- **AND** run without `--yes`, it is refused the same way, and no plan is printed

#### Scenario: [LANECLOSE-2-S2] A lane does not close itself
- **GIVEN** the command runs with `CLAUDUCTOR_LANE=<id>` in its environment
- **WHEN** `clauductor lane close <id>` runs, with or without `--yes`
- **THEN** it is refused before anything is read or the panel is asked, naming the page's Close
  lane, and auto-close where it is on for the lane's type
- **AND** it exits 1

#### Scenario: [LANECLOSE-2-S3] A lane whose turn ended on background work is refused, by the command and by auto-close
- **GIVEN** a lane whose claude reads idle, and whose session's last `Stop` lists a subagent, a
  workflow or a background shell still running, or a teammate
- **WHEN** `clauductor lane close <id> --yes` runs
- **THEN** it is refused naming the background work, and exits 1, and the lane keeps running
- **AND** an idle lane that holds or waits for a queue's lease is refused the same way, naming the
  lease, and so is a lane whose claude has exited while it holds one
- **AND** another session busy in the same worktree does not hold the lane: only the lane's own
  session is read
- **AND** with `lanes_auto_close` `on_merge` and the lane's pull request merged, auto-close does
  not close it either, and asks in Needs you with the same reason
- **AND** once a later `Stop` lists no such task, the lane may close

#### Scenario: [LANECLOSE-2-S4] An idle lane on unmerged work is refused; an exited one closes and keeps its branch
- **GIVEN** a lane whose claude is idle, with no background work, on a branch neither merged into
  the base nor the head of a merged pull request
- **WHEN** `clauductor lane close <id> --yes` runs
- **THEN** it is refused with "its branch is not merged" and the reason, and exits 1
- **AND** the lane's tmux session, its registry record, its worktree and its branch are untouched
- **AND** a lane whose plan names no branch is refused the same way, "its branch is not known"
- **AND** a registered lane whose tmux session is gone, on that branch, is refused the same way,
  and the page still offers to restore it
- **AND** when that lane's claude has exited instead, the close goes ahead: the lane is stopped
  and forgotten, and the **Kept** lines keep the branch, saying it is not merged

### Requirement: Reach the panel only as the owner's user, never from a browser
WHILE the panel runs THE SYSTEM SHALL serve the command's close on a Unix socket only the
owner's user can open, serving nothing else there, and the page's server SHALL keep every guard
it has.

#### Scenario: [LANECLOSE-3-S1] The socket is private, and serves the close alone
- **GIVEN** a running panel
- **THEN** its socket exists with mode 0600, in the 0700 panel directory, and `owner.json` names
  its path
- **AND** a request on it for any other path or method, the page's state and a lane's stop among
  them, is answered 404 or 405
- **AND** the page's close route on the TCP port still refuses a request without the page's cookie
  or `Origin`

#### Scenario: [LANECLOSE-3-S2] With no live panel, the command says so and changes nothing
- **GIVEN** no panel runs, and a socket file a killed panel left behind
- **WHEN** `clauductor lane close <id> --yes` runs
- **THEN** it says no panel runs and names `clauductor panel`
- **AND** it connects to nothing, and exits 1

#### Scenario: [LANECLOSE-3-S3] The panel replaces a stale socket at start, and removes its own at exit
- **GIVEN** a socket file left at the socket's path by a killed panel
- **WHEN** a panel starts
- **THEN** it serves the close on a socket of its own at that path
- **AND** when it exits, the socket file is gone

#### Scenario: [LANECLOSE-3-S4] A panel that cannot make its socket starts anyway, and the command says why
- **GIVEN** a socket path the panel cannot listen on
- **WHEN** the panel starts
- **THEN** it serves the page as before, prints why it has no socket, and `owner.json` names no
  socket
- **AND** `clauductor lane close <id>` then exits 1, saying the running panel has no socket

### Requirement: Finish a close the panel has begun
WHEN the panel has begun a close asked for on its socket THE SYSTEM SHALL finish it even if the
command that asked is interrupted.

#### Scenario: [LANECLOSE-4-S1] An interrupted command leaves the close finished
- **GIVEN** a `--yes` close of an idle lane on a clean, merged worktree, which the panel has begun
- **WHEN** the client's connection closes before the answer
- **THEN** the panel finishes the close: the worktree is removed and the branch deleted
- **AND** the panel's output has the close's line
