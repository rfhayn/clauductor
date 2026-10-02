## Purpose
The panel's own `claude` calls (`agents`, `auth status`, `--version`) never strand or contend for the login refresh every Claude Code session on the machine shares, and the owner sees a refresh that is stuck.

## ADDED Requirements

### Requirement: A claude call is stopped, never killed outright
WHEN a `claude` call the panel started passes its timeout, or the panel shuts down while one runs, THE SYSTEM SHALL send it SIGTERM and SHALL send SIGKILL only after a grace period.

#### Scenario: [CLAUDECALLS-1-S1] A timed-out claude call gets SIGTERM first
- **GIVEN** a `claude` program that traps SIGTERM, records it and exits
- **WHEN** a panel call to it passes its timeout
- **THEN** the program receives SIGTERM, not SIGKILL
- **AND** the caller gets its timeout error without waiting for the program to exit

#### Scenario: [CLAUDECALLS-1-S2] A claude call that ignores SIGTERM is killed after the grace
- **GIVEN** a `claude` program that ignores SIGTERM
- **WHEN** a panel call to it passes its timeout
- **THEN** the program is killed once the grace period has passed, and not before

#### Scenario: [CLAUDECALLS-1-S3] Other commands keep their behaviour
- **GIVEN** a `git` or `gh` command past its timeout
- **WHEN** the panel stops it
- **THEN** it is stopped as it is today

### Requirement: The panel runs one claude call at a time
THE SYSTEM SHALL run at most one `claude` process of its own at a time across every project it serves; WHEN the slot is taken a poll SHALL skip its turn and keep its last value, and a lane action SHALL wait for the slot within its own timeout.

#### Scenario: [CLAUDECALLS-2-S1] A poll skips while another claude call runs
- **GIVEN** a `claude agents` poll in flight for one project
- **WHEN** another project's `claude agents` poll comes due
- **THEN** no second `claude` process starts
- **AND** that project's agents source keeps its last value and says why it skipped

#### Scenario: [CLAUDECALLS-2-S2] A lane action waits for the slot
- **GIVEN** a `claude agents` poll in flight
- **WHEN** a lane action needs `claude agents`
- **THEN** it runs as soon as the poll's call ends, within its own timeout

### Requirement: No claude call while a login refresh holds the lock
WHEN the login refresh lock in Claude Code's config directory was touched in the last 60 seconds THE SYSTEM SHALL start no `claude` call for a poll, and a lane action SHALL wait for the lock to clear within its own timeout.

#### Scenario: [CLAUDECALLS-3-S1] A live lock pauses the polls
- **GIVEN** `.oauth_refresh.lock` in the config directory, touched 3 seconds ago
- **WHEN** a `claude agents` poll comes due
- **THEN** no `claude` process starts
- **AND** the agents source says a login refresh is in progress

#### Scenario: [CLAUDECALLS-3-S2] A stale lock does not stop calls
- **GIVEN** `.oauth_refresh.lock` in the config directory, last touched 5 minutes ago
- **WHEN** a `claude agents` poll comes due
- **THEN** the call runs

#### Scenario: [CLAUDECALLS-3-S3] The config directory follows CLAUDE_CONFIG_DIR
- **GIVEN** the panel's environment sets `CLAUDE_CONFIG_DIR` to a directory holding a live lock
- **AND** `~/.claude` holds none
- **WHEN** a `claude agents` poll comes due
- **THEN** no `claude` process starts

### Requirement: Every refresh lock the panel sees is logged
THE SYSTEM SHALL record each refresh lock it observes, with its holder and whether the holder is one of the panel's own `claude` calls, and each `claude` call of its own that it had to stop.

#### Scenario: [CLAUDECALLS-4-S1] A lock held by another process is logged
- **GIVEN** a lock whose owner record names a live process that is not the panel's
- **WHEN** the lock appears and later goes
- **THEN** the log has one line naming the holder's pid and command, not the panel's own, when it was first seen, when it was last touched and how it ended

#### Scenario: [CLAUDECALLS-4-S2] A lock held by the panel's own call is marked so
- **GIVEN** a lock whose owner record names the pid of a `claude` call the panel started
- **WHEN** the lock is logged
- **THEN** its line says the holder is the panel's own call and names the call

#### Scenario: [CLAUDECALLS-4-S3] A stopped claude call is logged
- **WHEN** the panel stops one of its `claude` calls at its timeout
- **THEN** the log has a line naming the call, the signal it sent and whether a lock existed then

### Requirement: A stuck or stranded lock is shown to the owner and never removed
WHEN the same refresh lock has been touched within 60 seconds continuously for more than 2 minutes THE SYSTEM SHALL raise an alert naming its holder and what to do; WHEN a lock untouched for 60 seconds or more is still present 2 minutes later THE SYSTEM SHALL raise a warning naming the lock, its last holder and the remedy; and THE SYSTEM SHALL never remove or write the lock or its owner record.

#### Scenario: [CLAUDECALLS-5-S1] A lock held live for over 2 minutes is an alert
- **GIVEN** a lock whose directory is touched every 5 seconds for 3 minutes, held by pid 4242
- **WHEN** the panel derives its alerts
- **THEN** an alert names pid 4242, its command and the lock's age, and says to run /login in any session and to end that process if it comes back

#### Scenario: [CLAUDECALLS-5-S2] A stranded lock is a warning with the remedy
- **GIVEN** a lock last touched 5 minutes ago whose owner record names a pid that no longer runs
- **WHEN** the panel derives its alerts
- **THEN** a warning names the lock's path and says its holder is gone
- **AND** it says that if sessions fail to refresh, remove that path and run /login

#### Scenario: [CLAUDECALLS-5-S3] The panel never touches the lock
- **GIVEN** a lock and its owner record, live or stale
- **WHEN** the panel watches, logs and alerts on them
- **THEN** both still exist with the same contents and modification times

#### Scenario: [CLAUDECALLS-5-S4] A lock that has only just gone stale is not yet a warning
- **GIVEN** a lock last touched 90 seconds ago
- **WHEN** the panel derives its alerts
- **THEN** no warning is raised

### Requirement: The account is read on demand, not on a timer
THE SYSTEM SHALL run `claude auth status` only at start when no account reading under 24 hours old is saved, and when the owner asks for a refresh; it SHALL use the saved reading otherwise.

#### Scenario: [CLAUDECALLS-6-S1] A recent saved reading is used at start
- **GIVEN** an account reading saved 3 hours ago
- **WHEN** the panel starts
- **THEN** no `claude auth status` runs
- **AND** the status bar shows the saved reading's mode and plan

#### Scenario: [CLAUDECALLS-6-S2] No timer reads the account
- **GIVEN** a panel that read the account at start
- **WHEN** an hour passes with no refresh asked for
- **THEN** no further `claude auth status` runs

#### Scenario: [CLAUDECALLS-6-S3] Refresh reads the account
- **WHEN** the owner presses Refresh
- **THEN** `claude auth status` runs once, under the same guards as every panel `claude` call, and its reading is saved
