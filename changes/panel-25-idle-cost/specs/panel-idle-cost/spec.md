## Purpose
An idle panel costs little: with no lane and no page in view it starts almost no processes and calls GitHub only for a lane it may close, while anything that needs it wakes it at once, and the login agent can start without opening a browser tab.

## ADDED Requirements

### Requirement: A dormant project backs off its timed polls
WHEN a project has no lane and no page has said it is in view for 90 s THE SYSTEM SHALL wait 5 minutes between timed polls of that project's `git worktree list` and tmux `list-panes`, after an error too, and WHEN in addition no hook has reached the project for 5 minutes THE SYSTEM SHALL wait 5 minutes between timed polls of its `claude agents`, cross-checking the `--cwd` filter only at its first poll. A wake (the next requirement) still polls at once.

#### Scenario: [IDLE-1-S1] An idle project polls once every 5 minutes
- **GIVEN** a project with no lane, no page in view and no hook for 5 minutes
- **WHEN** 30 minutes pass on the panel's clock with no wake
- **THEN** `claude agents`, `git worktree list` and tmux `list-panes` each poll at most 7 times
- **AND** after the first poll, no `claude agents --json` without `--cwd` runs beside a filtered one

#### Scenario: [IDLE-1-S2] A lane in one project does not keep another awake
- **GIVEN** two projects, a lane in the first and none in the second, and no page in view
- **WHEN** 10 minutes pass on the panel's clock with no wake
- **THEN** the first project polls `claude agents` and tmux at today's lane cadence
- **AND** the second project polls each at most 3 times

#### Scenario: [IDLE-1-S3] A page in view keeps today's cadence
- **GIVEN** a project with no lane and no hook for 5 minutes
- **AND** a page that says it is in view every minute
- **WHEN** a minute passes on the panel's clock
- **THEN** `claude agents` polls 4 times, `git worktree list` 6 times and tmux `list-panes` 6 times, as before this change

#### Scenario: [IDLE-1-S4] A tmux error in a dormant project waits 5 minutes too
- **GIVEN** a dormant project whose tmux `list-panes` fails with an error other than "no server"
- **WHEN** the tmux poll returns
- **THEN** its next timed poll is 5 minutes later, not 2 seconds

### Requirement: What needs the panel wakes it at once
WHEN a hook reaches a dormant project, a lane starts in it, a worktree is added or removed, or a page comes into view THE SYSTEM SHALL poll the sources that event concerns at once, without waiting for the 5-minute tick; and WHEN the panel's `claude` slot skips a `claude agents` poll THE SYSTEM SHALL retry it every 2 seconds until one runs, never waiting the 5-minute tick after a skipped poll.

#### Scenario: [IDLE-2-S1] A hook polls claude agents at once
- **GIVEN** an idle project whose `claude agents` loop waits its 5 minutes
- **WHEN** a hook from a session in the project arrives
- **THEN** `claude agents` runs at once, before the panel's clock moves
- **AND** its next interval is the one for hooks flowing, not 5 minutes

#### Scenario: [IDLE-2-S2] A lane start ends dormancy
- **GIVEN** a dormant project
- **WHEN** a lane starts in it
- **THEN** tmux `list-panes`, `git worktree list` and `claude agents` run at once
- **AND** they keep the cadence for a project with a lane afterwards

#### Scenario: [IDLE-2-S3] A page coming into view polls everything that backed off
- **GIVEN** a dormant project that has not read its open pull requests for 10 minutes
- **WHEN** a page says it is in view, and none was
- **THEN** `claude agents`, `git worktree list`, tmux `list-panes` and `gh pr list` each run at once

#### Scenario: [IDLE-2-S4] A worktree added by hand is read within the watch tick
- **GIVEN** a dormant project
- **WHEN** a worktree is added to it with `git worktree add`
- **THEN** `git worktree list` runs within one watch tick (2 s), not 5 minutes later

#### Scenario: [IDLE-2-S5] A hook-kicked poll that the claude slot skips retries in seconds
- **GIVEN** an idle project whose `claude agents` loop waits its 5 minutes
- **AND** the panel's `claude` slot held by another call
- **WHEN** a hook from a session in the project arrives
- **THEN** the poll is skipped without starting a process, and tried again every 2 seconds
- **AND** `claude agents` runs within 2 seconds of the slot coming free, not 5 minutes later

### Requirement: GitHub is called only when something reads the answer
WHEN no page is in view THE SYSTEM SHALL NOT read a project's open pull requests, and WHEN a page is in view SHALL read them every 60 s as before; WHILE a project has a lane that auto-close watches THE SYSTEM SHALL read its merged pull requests at most every 3 minutes, in view or not, and SHALL close that lane, or ask, after its pull request merges, including one opened and merged between two reads; each close and each ask SHALL be logged with the merge time and its own time.

#### Scenario: [IDLE-3-S1] No page and no lane to close means no GitHub call
- **GIVEN** a project with lanes but none with `lanes_auto_close` on, and no page in view
- **WHEN** an hour passes on the panel's clock
- **THEN** no `gh` command runs for the project

#### Scenario: [IDLE-3-S2] A lane still auto-closes with no page in view
- **GIVEN** `lanes_auto_close` "on_merge" and a lane on `fix/done` whose pull request is open
- **AND** no page in view
- **WHEN** the pull request merges
- **THEN** no open-list `gh pr list` runs, and `gh pr list --state merged` runs at most once every 3 minutes
- **AND** the lane is closed, or asks "PR merged: close lane?", within 4 minutes of the merge

#### Scenario: [IDLE-3-S3] A page in view reads the open pull requests every minute
- **GIVEN** a project with no lane
- **AND** a page that says it is in view every minute
- **WHEN** 3 minutes pass on the panel's clock
- **THEN** the open-list `gh pr list` runs once a minute, as before this change

#### Scenario: [IDLE-3-S4] A pull request opened and merged between two reads still closes its lane
- **GIVEN** `lanes_auto_close` "on_merge", a lane on `fix/quick` already seen with no pull request, and a page in view
- **WHEN** a pull request for `fix/quick` is opened and merged between two reads of the open list
- **THEN** the lane is closed, or asks "PR merged: close lane?", within 4 minutes of the merge

#### Scenario: [IDLE-3-S5] An auto-close is logged with its times
- **GIVEN** a lane auto-close closes after its pull request #12 merged
- **WHEN** the close is done
- **THEN** one line naming the lane, the pull request, its merge time and the close time is written to the panel's output

### Requirement: The panel reports what it spawns
THE SYSTEM SHALL count every process it starts through its command runner or tmux, once each, per project (the machine's own calls apart) and by command, with each project's idle minutes, and SHALL report the last 10 minutes in each project's observability counters without that report marking a page in view; WHEN it runs as the login agent THE SYSTEM SHALL also write each project's last hour, and the machine's, to its log once an hour.

#### Scenario: [IDLE-4-S1] The spawn rate is in the observability counters
- **GIVEN** a project whose sources ran `claude agents` twice and `git worktree list` once in the last 10 minutes
- **WHEN** `/api/state` is read for the project
- **THEN** its `obs` names each command with its count, the rate a minute and the idle rate a minute
- **AND** the read does not make a page in view

#### Scenario: [IDLE-4-S2] The login agent logs each project's hour
- **GIVEN** the panel running under `--launchd` with two projects, one idle for the whole hour
- **WHEN** an hour passes on the panel's clock
- **THEN** one line per project and one for the machine are written to its output
- **AND** the idle project's line gives 60 idle minutes and the spawns made in them

#### Scenario: [IDLE-4-S3] The machine's calls are counted once, as the machine's
- **GIVEN** a panel with a default project
- **WHEN** the machine runs `claude --version` once
- **THEN** the machine's count of `claude --version` is 1
- **AND** no project's count includes it

#### Scenario: [IDLE-4-S4] A call the claude slot skips is not counted
- **GIVEN** the panel's `claude` slot held by another call
- **WHEN** a project's `claude agents` poll is skipped for it
- **THEN** the project's count of `claude agents` does not change
- **AND** it rises by one when the retried poll runs

### Requirement: The login agent can start without a browser tab
WHEN `clauductor panel install` is given `--no-open` THE SYSTEM SHALL install a login agent that never opens a browser, and without it SHALL keep opening the page once per login; either way the install SHALL say which it installed.

#### Scenario: [IDLE-5-S1] An agent installed with --no-open opens nothing
- **GIVEN** `clauductor panel install --no-open`
- **WHEN** the agent starts at login, and again after a crash
- **THEN** its plist runs `panel … --launchd --no-open`
- **AND** no browser opens and no `browser-opened` file is written

#### Scenario: [IDLE-5-S2] An agent installed without it opens once per login, and says so
- **GIVEN** `clauductor panel install` with no `--no-open`
- **WHEN** the install finishes
- **THEN** its output says the agent opens the page once per login and names `--no-open`
- **AND** the plist has no `--no-open`
