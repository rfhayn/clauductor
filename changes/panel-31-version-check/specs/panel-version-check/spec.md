## Purpose
The panel re-checks the undocumented subagent pairing on each new Claude Code version, says so quietly in the status bar while it checks, explains what is approximate meanwhile, and alerts only when something is actually wrong.

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

#### Scenario: [VERCHECK-1-S4] One unmatched launch costs one more confirmation
- **GIVEN** Claude Code 2.1.288 is being checked
- **AND** one Agent launch named an agent no SubagentStart announced within 10 s
- **WHEN** three more launches are matched
- **THEN** the version is not yet verified and the field reads "2.1.288 · checking, 3 of 4"
- **AND** the version dialog says one launch was never announced, so it clears by itself after one more

#### Scenario: [VERCHECK-1-S5] The fourth confirmation clears it without a restart
- **GIVEN** Claude Code 2.1.288 is being checked with one unmatched launch and 3 of 4 confirmed
- **WHEN** one more launch is matched
- **THEN** the version is verified and the field is no longer shown

### Requirement: Something actually wrong still raises the warning bar
WHEN the check finds that the pairing changed on the running version, or the latest `claude --version` fails, THE SYSTEM SHALL show a warning bar for each that holds; a failed latest read SHALL take precedence over the check in progress, and SHALL NOT hide a break.

#### Scenario: [VERCHECK-2-S1] Two unmatched launches raise the bar
- **GIVEN** Claude Code 2.1.288 is being checked
- **WHEN** two Agent launches each name an agent no SubagentStart announced
- **THEN** a warning bar says Claude Code 2.1.288 changed what the subagent pairing relies on, and what broke
- **AND** the field reads "2.1.288 · changed"

#### Scenario: [VERCHECK-2-S2] An unreadable version raises the bar
- **GIVEN** `claude --version` fails on the panel's first read
- **WHEN** the page renders
- **THEN** a warning bar says the version cannot be read and subagent lists are approximate

#### Scenario: [VERCHECK-2-S3] A read that fails after one worked raises the bar, not the field
- **GIVEN** the panel read Claude Code 2.1.288 and is checking it
- **WHEN** the next `claude --version` fails
- **THEN** a warning bar says the version cannot be read and subagent lists are approximate
- **AND** no Claude Code field is shown

#### Scenario: [VERCHECK-2-S4] A failed read does not hide a break
- **GIVEN** the check found a break on Claude Code 2.1.288
- **WHEN** the next `claude --version` fails
- **THEN** the break bar and the unreadable-version bar both show
- **AND** the field reads "2.1.288 · changed"

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

#### Scenario: [VERCHECK-3-S3] The first project's verification clears the second
- **GIVEN** two projects that both hold 2.1.287 as verified, restored from verified.json
- **AND** Claude Code 2.1.288 is running
- **WHEN** the first project verifies 2.1.288
- **THEN** the second project shows no Claude Code field

### Requirement: Help says what is approximate meanwhile
THE SYSTEM SHALL explain in Help that only the subagent lists and counts are approximate while a version is checked, and that the check clears by itself in every project without a restart.

#### Scenario: [VERCHECK-6-S1] Help has the Claude Code versions section
- **GIVEN** the panel page
- **WHEN** Help opens
- **THEN** a "Claude Code versions" section says only the subagent lists and counts are approximate, that the check clears by itself after three matched launches plus one more for each launch that didn't match, and that nothing needs a restart
