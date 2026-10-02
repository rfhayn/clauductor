## Purpose
The panel re-checks the undocumented subagent pairing on each new Claude Code version, says so quietly while it checks, settles it on demand at a stated cost, and alerts only when the pairing has actually changed.

## ADDED Requirements

### Requirement: An unverified version is a quiet status-bar field
WHILE the running Claude Code version is being checked THE SYSTEM SHALL show it as a status-bar field with its progress, and SHALL NOT show a warning bar for it.

#### Scenario: [VERCHECK-1-S1] A new version shows the field, not a bar
- **GIVEN** the panel reads Claude Code 2.1.288 and has verified only 2.1.284
- **WHEN** the page renders
- **THEN** the status bar shows "Claude Code" with "2.1.288 · checking, 0 of 3"
- **AND** no warning bar names the version

#### Scenario: [VERCHECK-1-S2] Confirmations clear the field without a restart
- **GIVEN** Claude Code 2.1.288 is being checked
- **WHEN** three Agent launches are each matched by their SubagentStart, with none unmatched
- **THEN** the version is verified and the field is no longer shown
- **AND** a lane's Agents tab no longer says its list is approximate

#### Scenario: [VERCHECK-1-S3] A verified version shows no field
- **GIVEN** the running Claude Code is 2.1.284, or the version recorded in verified.json
- **WHEN** the page renders
- **THEN** no Claude Code field and no version warning bar is shown

#### Scenario: [VERCHECK-1-S4] One unmatched launch is shown, not counted past three
- **GIVEN** Claude Code 2.1.288 is being checked
- **AND** one Agent launch named an agent no SubagentStart announced within 10 s
- **WHEN** three more launches are matched
- **THEN** the field reads "2.1.288 · checking, 1 unmatched"
- **AND** the version dialog says Verify now or a restart settles it

### Requirement: A break still raises the warning bar
WHEN the check finds that the pairing changed on the running version THE SYSTEM SHALL show a warning bar that says what broke, and the field SHALL read "changed".

#### Scenario: [VERCHECK-2-S1] Two unmatched launches raise the bar
- **GIVEN** Claude Code 2.1.288 is being checked
- **WHEN** two Agent launches each name an agent no SubagentStart announced
- **THEN** a warning bar says Claude Code 2.1.288 changed what the subagent pairing relies on, and what broke
- **AND** the field reads "2.1.288 · changed"

#### Scenario: [VERCHECK-2-S2] An unreadable version still raises the bar
- **GIVEN** `claude --version` fails
- **WHEN** the page renders
- **THEN** a warning bar says the version cannot be read and subagent lists are approximate

### Requirement: A version verified anywhere holds everywhere, and is kept
WHEN any served project verifies the running Claude Code version THE SYSTEM SHALL treat it as verified in every project and SHALL record it in verified.json, replacing an older verified version.

#### Scenario: [VERCHECK-3-S1] The second project's verification clears the first
- **GIVEN** two projects that both hold 2.1.287 as verified, restored from verified.json
- **AND** Claude Code 2.1.288 is running
- **WHEN** the second project verifies 2.1.288
- **THEN** the first project shows no Claude Code field
- **AND** verified.json records 2.1.288

#### Scenario: [VERCHECK-3-S2] The newer verification survives a restart
- **GIVEN** verified.json records 2.1.288 after the second project verified it
- **WHEN** the panel restarts with Claude Code 2.1.288 running
- **THEN** no project shows the Claude Code field

### Requirement: Verify now settles the check on demand
WHEN the owner presses Verify now THE SYSTEM SHALL run one short throwaway Claude Code session that launches three tool-less subagents, judge the running version from that session's hooks alone, and report the result with its duration and cost; THE SYSTEM SHALL NOT start such a session by any other route.

#### Scenario: [VERCHECK-4-S1] A clean probe verifies the version for every project
- **GIVEN** Claude Code 2.1.288 is being checked
- **WHEN** Verify now runs and its session's three Agent launches are each matched by their SubagentStart
- **THEN** 2.1.288 is verified in every project and recorded in verified.json
- **AND** the result reads "Verified: Claude Code 2.1.288 matched 3 of 3 subagent launches" with its seconds and its cost at list price

#### Scenario: [VERCHECK-4-S2] The probe runs the smallest session that can launch an agent
- **GIVEN** Claude Code 2.1.288 is being checked
- **WHEN** Verify now starts its session
- **THEN** it runs `claude -p` on Haiku, with only the Agent tool, the panel's own hooks for this panel's port, no session persistence, a $0.25 budget cap and its own session id
- **AND** it runs without the API-key and parent-session variables, in a new empty folder under the panel's directory that is removed afterwards

#### Scenario: [VERCHECK-4-S3] The probe's hooks stay out of every project
- **GIVEN** Verify now is running with its own session id
- **WHEN** that session's hooks arrive
- **THEN** no project's sessions, feed or "other projects" count changes

#### Scenario: [VERCHECK-4-S4] A probe with too few confirmations changes nothing
- **GIVEN** Claude Code 2.1.288 is being checked with 1 of 3 confirmed
- **WHEN** Verify now's session ends with only 2 of its own launches matched
- **THEN** the result reads "Not settled" with the reason
- **AND** the field still reads "2.1.288 · checking, 1 of 3"

#### Scenario: [VERCHECK-4-S5] A probe that runs too long is interrupted before it is killed
- **GIVEN** Verify now's session is still running after 90 s
- **WHEN** the limit passes
- **THEN** the session gets an interrupt, and is killed only if it is still running 10 s later
- **AND** the result reads "Stopped after 90 s" with the count it reached

#### Scenario: [VERCHECK-4-S6] Nothing starts a probe but the button
- **GIVEN** a panel that reads a new Claude Code version, restarts, and receives hooks
- **WHEN** no Verify now request is made
- **THEN** no `claude -p` session is started

### Requirement: Verify now refuses when it would be unsafe or pointless
WHEN Verify now is requested while the login is refreshing, a probe is running, the quota guard is on, lanes cannot start, or the version needs no check THE SYSTEM SHALL refuse it with a plain sentence and start nothing.

#### Scenario: [VERCHECK-5-S1] A login refresh in progress refuses it
- **GIVEN** `~/.claude/.oauth_refresh.lock` exists
- **WHEN** Verify now is requested
- **THEN** it is refused with "Claude Code is refreshing its login. Try again in a minute"
- **AND** no session is started

#### Scenario: [VERCHECK-5-S2] A second request while one runs is refused
- **GIVEN** Verify now is running
- **WHEN** Verify now is requested again
- **THEN** it is refused, saying it is already running and since when

#### Scenario: [VERCHECK-5-S3] The quota guard refuses it
- **GIVEN** the 5-hour quota is at or over the guard
- **WHEN** Verify now is requested
- **THEN** it is refused with the guard's sentence

#### Scenario: [VERCHECK-5-S4] A version that needs no check refuses it
- **GIVEN** the running Claude Code is verified
- **WHEN** Verify now is requested
- **THEN** it is refused with "needs no check"

### Requirement: Help says what is approximate meanwhile
THE SYSTEM SHALL explain in Help that only the subagent lists and counts are approximate while a version is checked, that the check clears by itself without a restart, and what Verify now runs and costs.

#### Scenario: [VERCHECK-6-S1] Help has the Claude Code versions section
- **GIVEN** the panel page
- **WHEN** Help opens
- **THEN** a "Claude Code versions" section says only the subagent lists and counts are approximate, that nothing needs a restart, and that Verify now runs one short session on Haiku at about 5 cents at list price
