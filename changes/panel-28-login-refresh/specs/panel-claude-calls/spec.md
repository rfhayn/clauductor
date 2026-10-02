## Purpose
The panel's own `claude` calls (`agents`, `auth status`, `--version`, and any later call through the same gate) never strand or contend for the login refresh every Claude Code session on the machine shares, and the owner sees a refresh lock that is stuck, stranded or dated in the future.

## ADDED Requirements

### Requirement: A claude call is stopped, never killed outright
WHEN a `claude` call the panel started passes its timeout THE SYSTEM SHALL return the timeout to its caller at once, send the process SIGTERM, and send SIGKILL only if it is still running 15 seconds later; WHEN the panel shuts down THE SYSTEM SHALL send SIGTERM to every `claude` call still running and SIGKILL only to one still running 3 seconds later.

#### Scenario: [CLAUDECALLS-1-S1] A timed-out claude call gets SIGTERM, and its caller does not wait
- **GIVEN** a `claude` program that traps SIGTERM, records it, and exits 5 seconds later
- **WHEN** a panel call to it passes its timeout
- **THEN** the caller gets its timeout error at the timeout, while the program still runs
- **AND** the program receives SIGTERM, not SIGKILL

#### Scenario: [CLAUDECALLS-1-S2] A claude call that ignores SIGTERM is killed after the grace
- **GIVEN** a `claude` program that ignores SIGTERM
- **WHEN** a panel call to it passes its timeout
- **THEN** the program is killed once 15 seconds have passed since SIGTERM, and not before

#### Scenario: [CLAUDECALLS-1-S3] Other commands keep their behaviour
- **GIVEN** a `git` or `gh` command past its timeout
- **WHEN** the panel stops it
- **THEN** it is stopped as it is today

#### Scenario: [CLAUDECALLS-1-S4] Shutdown bounds the grace
- **GIVEN** a `claude` program that ignores SIGTERM, running when the panel is asked to stop
- **WHEN** the panel shuts down
- **THEN** the program receives SIGTERM, then SIGKILL 3 seconds later
- **AND** the panel's shutdown waits for it no longer than those 3 seconds

### Requirement: The panel runs one claude call at a time
THE SYSTEM SHALL run at most one `claude` process of its own at a time across every project it serves, the slot held until that process has exited; WHEN the slot is taken a call SHALL wait for it up to a short bound, and a poll that still finds it taken SHALL try again within seconds, applying no update to the model.

#### Scenario: [CLAUDECALLS-2-S1] A colliding poll waits briefly, then retries soon
- **GIVEN** a `claude` call holding the slot for 10 seconds
- **WHEN** a `claude agents` poll comes due
- **THEN** no second `claude` process starts
- **AND** the poll tries again within 2 seconds of giving up, not after its interval
- **AND** the agents source's last reading, its success time and its error stay as they were

#### Scenario: [CLAUDECALLS-2-S2] A lane action waits for the slot
- **GIVEN** a `claude agents` poll in flight
- **WHEN** a lane action needs `claude agents`
- **THEN** it runs as soon as the poll's call ends, within its own timeout

#### Scenario: [CLAUDECALLS-2-S3] A stopped call holds the slot until it exits
- **GIVEN** a `claude` call past its timeout whose program takes 5 seconds to exit after SIGTERM
- **WHEN** another poll comes due 1 second after the timeout
- **THEN** no second `claude` process starts until the first has exited

#### Scenario: [CLAUDECALLS-2-S5] A call with no deadline does not wait without bound
- **GIVEN** a `claude` call holding the slot for a minute
- **WHEN** a lane action calls `claude agents` with a context that has no deadline
- **THEN** it gives up within 10 seconds with a busy error, and no second `claude` process starts

#### Scenario: [CLAUDECALLS-2-S4] Calls that start together all run
- **GIVEN** a panel starting, with the agents, version and account sources due at once
- **WHEN** their calls each take under a second
- **THEN** each runs within a few seconds of the start, one after another

### Requirement: No claude call while a login refresh holds a lock
WHEN the login refresh lock, or the legacy lock beside the config directory, was modified in the last 60 seconds and no more than 5 seconds in the future THE SYSTEM SHALL start no `claude` call; a poll SHALL try again within seconds, applying no update, and a lane action SHALL wait within its own timeout.

#### Scenario: [CLAUDECALLS-3-S1] A live lock pauses the polls without marking them failed
- **GIVEN** `.oauth_refresh.lock` in the config directory, modified 3 seconds ago
- **WHEN** a `claude agents` poll comes due
- **THEN** no `claude` process starts
- **AND** the agents source says a login refresh is in progress, and is neither marked failed nor read again as fresh
- **AND** once the lock is gone, the next poll runs within seconds

#### Scenario: [CLAUDECALLS-3-S2] A stale lock does not stop calls
- **GIVEN** `.oauth_refresh.lock` in the config directory, last modified 5 minutes ago
- **WHEN** a `claude agents` poll comes due
- **THEN** the call runs

#### Scenario: [CLAUDECALLS-3-S3] The config directory follows CLAUDE_CONFIG_DIR
- **GIVEN** the panel's environment sets `CLAUDE_CONFIG_DIR` to a directory holding a live lock
- **AND** `~/.claude` holds none
- **WHEN** a `claude agents` poll comes due
- **THEN** no `claude` process starts

#### Scenario: [CLAUDECALLS-3-S4] A lock dated in the future does not stop calls
- **GIVEN** `.oauth_refresh.lock` modified 2 hours in the future
- **WHEN** a `claude agents` poll comes due
- **THEN** the call runs

#### Scenario: [CLAUDECALLS-3-S5] A live legacy lock pauses the polls too
- **GIVEN** the legacy lock beside the config directory, modified 3 seconds ago, and no `.oauth_refresh.lock`
- **WHEN** a `claude agents` poll comes due
- **THEN** no `claude` process starts

### Requirement: Every refresh lock the panel sees is logged
THE SYSTEM SHALL record each refresh lock it observes, with its holder and whether the holder is one of the panel's own `claude` calls, decided when the lock is first seen from the owner record's pid and the lock's birth time falling within that call's run, and each `claude` call of its own that it had to stop.

#### Scenario: [CLAUDECALLS-4-S1] A lock held by another process is logged
- **GIVEN** a lock whose owner record names a live process that is not the panel's
- **WHEN** the lock appears and later goes
- **THEN** the log has one line naming the lock's path, the holder's pid and command, not the panel's own, when it was first seen, when it was last modified and how it ended

#### Scenario: [CLAUDECALLS-4-S2] A lock held by the panel's own call is marked so
- **GIVEN** a lock whose owner record names the pid of a `claude` call the panel started
- **WHEN** the lock is logged
- **THEN** its line says the holder is the panel's own call and names the call

#### Scenario: [CLAUDECALLS-4-S4] A lock left by a panel call that has already exited is still the panel's
- **GIVEN** a panel `claude` call that started, took a lock, and was killed and reaped
- **AND** the lock is still there, its owner record naming that call's pid and a birth time within the call's start and end
- **WHEN** the lock is logged
- **THEN** its line says the holder was the panel's own call and names the call

#### Scenario: [CLAUDECALLS-4-S5] A lock created after a panel call ended is not the panel's, whatever its pid
- **GIVEN** a panel `claude` call with pid 4242 that ended at 10:00:00
- **AND** a lock whose owner record names pid 4242 and a birth time of 10:00:30, from another process that reused the pid
- **WHEN** the lock is first seen and logged
- **THEN** its line says the holder is not the panel's own call

#### Scenario: [CLAUDECALLS-4-S3] A stopped claude call is logged
- **WHEN** the panel stops one of its `claude` calls at its timeout
- **THEN** the log has a line naming the call, the signals it sent and whether a lock existed then

### Requirement: A stuck, stranded or future-dated lock is shown to the owner and never removed
WHEN a refresh lock has been modified within 60 seconds continuously for more than 2 minutes THE SYSTEM SHALL raise an alert naming its holder and what to do; WHEN a lock last modified 60 seconds or more ago is still present 2 minutes later, or a lock is modified more than 5 seconds in the future, THE SYSTEM SHALL raise a warning naming the lock and the remedy; and THE SYSTEM SHALL never remove or write a lock or its owner record.

#### Scenario: [CLAUDECALLS-5-S1] A lock held live for over 2 minutes is an alert
- **GIVEN** a lock whose directory is modified every 5 seconds for 3 minutes, held by pid 4242
- **WHEN** the panel derives its alerts
- **THEN** an alert names pid 4242, its command and the lock's age, and says to run /login in any session and to end that process if it comes back

#### Scenario: [CLAUDECALLS-5-S2] A stranded lock is a warning with the remedy
- **GIVEN** a lock last modified 5 minutes ago whose owner record names a pid that no longer runs
- **WHEN** the panel derives its alerts
- **THEN** a warning names that lock's path and says its holder is gone
- **AND** it says that if sessions fail to refresh, remove that path and run /login

#### Scenario: [CLAUDECALLS-5-S3] The panel never touches a lock
- **GIVEN** a lock and its owner record, live, stale or future-dated, and a legacy lock
- **WHEN** the panel watches, logs, pauses its calls and derives its alerts
- **THEN** each still exists with the same contents and modification time

#### Scenario: [CLAUDECALLS-5-S4] A lock that has only just gone stale is not yet a warning
- **GIVEN** a lock last modified 90 seconds ago
- **WHEN** the panel derives its alerts
- **THEN** no warning is raised

#### Scenario: [CLAUDECALLS-5-S5] A future-dated lock is a warning, not a live alert
- **GIVEN** a lock modified 2 hours in the future
- **WHEN** the panel derives its alerts
- **THEN** a warning says the lock is dated in the future, that Claude Code waits for real time to pass it, and gives the remedy
- **AND** no live-holder alert is raised

### Requirement: The account is read on demand, not on a timer
THE SYSTEM SHALL run `claude auth status` only at start when no account reading under 24 hours old is saved, and when the owner asks for a refresh, retrying a failed read once a minute later; it SHALL use the saved reading otherwise.

#### Scenario: [CLAUDECALLS-6-S1] A recent saved reading is used at start
- **GIVEN** an account reading saved 3 hours ago
- **WHEN** the panel starts
- **THEN** no `claude auth status` runs
- **AND** the status bar shows the saved reading's mode and plan

#### Scenario: [CLAUDECALLS-6-S4] Adding a project does not read the account
- **GIVEN** an account reading saved 3 hours ago
- **WHEN** a project is added to the running panel
- **THEN** `claude --version` may run, but no `claude auth status` runs

#### Scenario: [CLAUDECALLS-6-S5] A failed account read is retried once
- **GIVEN** no saved account reading, and a `claude auth status` that fails
- **WHEN** the panel starts
- **THEN** it runs once at start and once more a minute later, and no more until Refresh
- **AND** the account source shows the error

#### Scenario: [CLAUDECALLS-6-S2] No timer reads the account
- **GIVEN** a panel that read the account at start
- **WHEN** an hour passes with no refresh asked for
- **THEN** no further `claude auth status` runs

#### Scenario: [CLAUDECALLS-6-S3] Refresh reads the account and the version
- **WHEN** the owner presses Refresh on any project
- **THEN** `claude auth status` and `claude --version` each run once, through the same gate as every panel `claude` call, and the account reading is saved
