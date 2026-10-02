## Purpose
An idle panel costs little: with no lane and no page in view it starts almost no processes and calls GitHub only for a lane it may close, while anything that needs it wakes it at once, and the login agent can start without opening a browser tab.

## ADDED Requirements

### Requirement: A dormant project backs off its polls
WHEN a project has no lane and no page has said it is in view for 90 s THE SYSTEM SHALL run that project's `git worktree list` and tmux `list-panes` at most once every 5 minutes, and WHEN in addition no hook has reached the project for 5 minutes THE SYSTEM SHALL run its `claude agents` at most once every 5 minutes, with no `--cwd` cross-check.

#### Scenario: [IDLE-1-S1] A dormant project polls once every 5 minutes
- **GIVEN** a project with no lane, no page in view and no hook for 5 minutes
- **WHEN** 30 minutes pass on the panel's clock
- **THEN** `claude agents`, `git worktree list` and tmux `list-panes` each run at most 7 times
- **AND** no `claude agents --json` without `--cwd` runs beside a filtered one

#### Scenario: [IDLE-1-S2] A lane in one project does not keep another awake
- **GIVEN** two projects, a lane in the first and none in the second, and no page in view
- **WHEN** 10 minutes pass on the panel's clock
- **THEN** the first project polls `claude agents` and tmux at today's lane cadence
- **AND** the second project polls each at most 3 times

#### Scenario: [IDLE-1-S3] A page in view keeps today's cadence
- **GIVEN** a project with no lane and no hook for 5 minutes
- **AND** a page that says it is in view every minute
- **WHEN** a minute passes on the panel's clock
- **THEN** `claude agents` runs 4 times, `git worktree list` 6 times and tmux `list-panes` 6 times, as before this change

### Requirement: What needs the panel wakes it at once
WHEN a hook reaches a dormant project, a lane starts in it, a worktree is added or removed, or a page comes into view THE SYSTEM SHALL poll the sources that event concerns at once, without waiting for the 5-minute tick.

#### Scenario: [IDLE-2-S1] A hook polls claude agents at once
- **GIVEN** a dormant project whose `claude agents` loop waits its 5 minutes
- **WHEN** a hook from a session in the project arrives
- **THEN** `claude agents` runs at once, before the panel's clock moves
- **AND** its next interval is the one for hooks flowing, not 5 minutes

#### Scenario: [IDLE-2-S2] A lane start ends dormancy
- **GIVEN** a dormant project
- **WHEN** a lane starts in it
- **THEN** tmux `list-panes`, `git worktree list` and `claude agents` run at once
- **AND** they keep the cadence for a project with a lane afterwards

#### Scenario: [IDLE-2-S3] A page coming into view polls everything that backed off
- **GIVEN** a dormant project that has not read its pull requests for 10 minutes
- **WHEN** a page says it is in view, and none was
- **THEN** `claude agents`, `git worktree list`, tmux `list-panes` and `gh pr list` each run at once

#### Scenario: [IDLE-2-S4] A worktree added by hand is read within the watch tick
- **GIVEN** a dormant project
- **WHEN** a worktree is added to it with `git worktree add`
- **THEN** `git worktree list` runs within one watch tick (2 s), not 5 minutes later

### Requirement: GitHub is called only when something reads the answer
WHEN no page is in view THE SYSTEM SHALL run a project's `gh pr list` only while the project has a lane that auto-close watches, at most once every 3 minutes, and WHEN a page is in view THE SYSTEM SHALL run it every 60 s as before; a lane auto-close watches SHALL still close, or ask, after its pull request merges.

#### Scenario: [IDLE-3-S1] No page and no lane to close means no GitHub call
- **GIVEN** a project with lanes but none with `lanes_auto_close` on, and no page in view
- **WHEN** an hour passes on the panel's clock
- **THEN** no `gh` command runs for the project

#### Scenario: [IDLE-3-S2] A lane still auto-closes with no page in view
- **GIVEN** `lanes_auto_close` "on_merge" and a lane on `fix/done` whose pull request is open
- **AND** no page in view
- **WHEN** the pull request merges and leaves the open list
- **THEN** `gh pr list` has run at most once every 3 minutes meanwhile
- **AND** the lane is closed, or asks "PR merged: close lane?", within 4 minutes of the merge

#### Scenario: [IDLE-3-S3] A page in view reads the pull requests every minute
- **GIVEN** a project with no lane
- **AND** a page that says it is in view every minute
- **WHEN** 3 minutes pass on the panel's clock
- **THEN** `gh pr list` runs once a minute, as before this change

### Requirement: The panel reports what it spawns
THE SYSTEM SHALL count every process the panel starts, by command, and SHALL report the rate over the last 10 minutes in each project's observability counters without that report marking a page in view; WHEN it runs as the login agent THE SYSTEM SHALL also write the last hour's count to its log once an hour.

#### Scenario: [IDLE-4-S1] The spawn rate is in the observability counters
- **GIVEN** a project whose sources ran `claude agents` twice and `git worktree list` once in the last 10 minutes
- **WHEN** `/api/state` is read for the project
- **THEN** its `obs` names each command with its count and the rate a minute
- **AND** the read does not make a page in view

#### Scenario: [IDLE-4-S2] The login agent logs the hour's spawns
- **GIVEN** the panel running under `--launchd`
- **WHEN** an hour passes on the panel's clock
- **THEN** one line "spawns in the last hour: N (…)" is written to its output, naming the count of each command

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
