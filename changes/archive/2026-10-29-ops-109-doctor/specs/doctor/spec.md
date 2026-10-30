## Purpose
`clauductor doctor` checks a machine's setup and a project's, and names every missing or broken
piece with what to do about it, changing nothing, so a user can tell why Clauductor does not work
here without reading three troubleshooting lists.

## ADDED Requirements

### Requirement: Report the machine's setup
WHEN a user runs `clauductor doctor` THE SYSTEM SHALL report every tool the prerequisites list
names, whether the binary finds a template it can use, whether `gh` is signed in, and on macOS
the panel's state, each problem with a fix that works where it is printed.

#### Scenario: [DOCTOR-1-S1] A missing required tool fails, with its install hint
- **GIVEN** a `PATH` without `jq`
- **WHEN** the user runs `clauductor doctor`
- **THEN** a failing line names `jq`, why the model needs it, and this OS's install hint from
  `prereqs.sh`
- **AND** doctor exits 1

#### Scenario: [DOCTOR-1-S2] A missing recommended tool warns, and a missing optional tool is a note
- **GIVEN** a `PATH` with every required tool, without `gitleaks` (recommended) and without `tmux`
  (optional)
- **WHEN** the user runs `clauductor doctor`
- **THEN** `gitleaks` is a warning with its install hint
- **AND** `tmux` is a note, counted as neither a failure nor a warning
- **AND** doctor exits 0
- **AND** when the template's `prereqs.sh` prints no line doctor can read as a row, a note says
  the tools could not be read and names the script, rather than no tool lines

#### Scenario: [DOCTOR-1-S3] A template that is not the binary's fails, saying what to do
- **GIVEN** the only template the binary finds has a `.template-version` other than the binary's
- **WHEN** the user runs `clauductor doctor`
- **THEN** the template line fails with `template.Resolve`'s reason, naming the matching release,
  `CLAUDUCTOR_FRAMEWORK` and `--template-dir`
- **AND** the tools line is a note that it could not run

#### Scenario: [DOCTOR-1-S4] A signed-out gh warns, and no token is printed
- **GIVEN** `gh` on the `PATH`, and `gh auth token` exits non-zero
- **WHEN** the user runs `clauductor doctor`
- **THEN** the gh line warns and names `gh auth login`
- **AND** when `gh auth token` succeeds instead, the output contains nothing it printed

#### Scenario: [DOCTOR-1-S5] A login agent whose panel is not running warns, with the restart and the log
- **GIVEN** macOS, the panel's login agent installed, and no live panel
- **WHEN** the user runs `clauductor doctor`
- **THEN** the panel line warns and names `launchctl kickstart -k
  gui/$(id -u)/com.clauductor.panel` and `~/.clauductor/panel/logs/panel.err.log`
- **AND** with no login agent, a registered project and no panel running, the line is a note

#### Scenario: [DOCTOR-1-S6] A panel binary of another build is a note that says which is which
- **GIVEN** macOS, and the login agent's binary has a build revision other than the running
  `clauductor`'s
- **WHEN** the user runs `clauductor doctor`
- **THEN** the panel build line is a note naming both short revisions and both binaries' paths
- **AND** it names `clauductor panel install` only as the way to put this binary in the panel
  when this one is the newer, saying it rotates the page's token
- **AND** it counts as neither a failure nor a warning

#### Scenario: [DOCTOR-1-S7] No panel installed is a note
- **GIVEN** macOS, no login agent and no registered project, and a repository whose
  `panel.json` loads
- **WHEN** the user runs `clauductor doctor` in it
- **THEN** one note says the panel is not installed, that it is optional, and names `clauductor
  panel install --project <main checkout>`, which then installs the panel
- **AND** no panel line warns or fails
- **AND** in a repository with no `panel.json`, the note names `clauductor panel init --project
  <main checkout>` before `panel install`

### Requirement: Report the project's setup
WHEN `clauductor doctor` runs inside a git repository THE SYSTEM SHALL report whose operating
model the repository runs, whether Clauductor's model matches the template, and on macOS whether
the panel loads, serves and trusts the project's config, reading the panel's state at the
repository's main checkout.

#### Scenario: [DOCTOR-2-S1] Outside a git repository only the machine is checked
- **GIVEN** a current directory that is in no git repository
- **WHEN** the user runs `clauductor doctor`
- **THEN** the machine lines are printed
- **AND** one note says the project checks did not run

#### Scenario: [DOCTOR-2-S2] A repository with no model is a note, not a problem
- **GIVEN** a git repository with no install marker, no plugin marker and none of `AGENTS.md`,
  `.claude/skills`, `.claude/hooks` or `.claude/settings.json`
- **WHEN** the user runs `clauductor doctor` in it
- **THEN** a note says it has no operating model and names `clauductor install`
- **AND** there is no drift line

#### Scenario: [DOCTOR-2-S3] A repository running its own model or the plugin's is not checked for drift
- **GIVEN** a git repository with `.claude/skills` and no install marker
- **WHEN** the user runs `clauductor doctor` in it
- **THEN** a note says it runs an operating model of its own, which `install` and `update`
  refuse, and names `clauductor diff`
- **AND** there is no drift line
- **AND** with the plugin marker instead, the note says it runs the plugin's model

#### Scenario: [DOCTOR-2-S4] A drifted framework warns, with the update that fixes it
- **GIVEN** a repository holding Clauductor's model, one framework file edited and one deleted
- **WHEN** the user runs `clauductor doctor` from a subdirectory of it
- **THEN** the drift line warns with 1 file that differs and 1 missing
- **AND** names `cd <toplevel> && clauductor update --dry-run` and `cd <toplevel> && clauductor
  update`, which run from the subdirectory and clear the warning on a re-run

#### Scenario: [DOCTOR-2-S5] A framework that matches the template is ok
- **GIVEN** a repository just installed by `clauductor install`
- **WHEN** the user runs `clauductor doctor` in it
- **THEN** the drift line is ok and names the template's version
- **AND** the last line names `sh .claude/checks/run.sh`

#### Scenario: [DOCTOR-2-S6] A served config not trusted as it is now warns
- **GIVEN** macOS, the panel installed, the project registered, and its `panel.json` changed
  since it was trusted
- **WHEN** the user runs `clauductor doctor` in the project
- **THEN** the trust line warns that cards, Run and templates do nothing, and names `clauductor
  panel trust --project <main checkout>`

#### Scenario: [DOCTOR-2-S7] A project the panel does not serve is a note
- **GIVEN** macOS, the panel installed and serving another project, and this project's main
  checkout, holding a `panel.json` never trusted that names its own `tmux_socket`, not in the
  registry
- **WHEN** the user runs `clauductor doctor` in the project
- **THEN** a note says the panel does not serve it, and names `clauductor panel add --project
  <main checkout>`, which then registers it
- **AND** there is no panel config line and no trust line
- **AND** doctor exits 0
- **AND** when its `panel.json` names the socket the served project holds, the note gives
  `panel add`'s refusal (its own `tmux_socket`) instead of the `panel add` command

#### Scenario: [DOCTOR-2-S8] A config that does not load fails
- **GIVEN** macOS, the panel installed, the project registered, and its `panel.json` holds an
  unknown field
- **WHEN** the user runs `clauductor doctor` in the project
- **THEN** the panel config line fails with the loader's error
- **AND** doctor exits 1

#### Scenario: [DOCTOR-2-S9] From a lane worktree the panel's lines read the main checkout
- **GIVEN** macOS, the panel installed, the main checkout registered and its config trusted, and
  the current directory in a linked worktree of that repository
- **WHEN** the user runs `clauductor doctor` there
- **THEN** the panel serves it line and the trust line are both ok
- **AND** the drift line reads the worktree's own files

### Requirement: Change nothing, and exit by the worst finding
THE SYSTEM SHALL write nothing while `clauductor doctor` runs, and SHALL exit 1 when any line
fails and 0 otherwise, ending with a count of failures and warnings.

#### Scenario: [DOCTOR-3-S1] Doctor leaves the repository and the panel's files as they were
- **GIVEN** a repository holding Clauductor's model with drift, and a home directory with the
  panel installed and an untrusted config
- **WHEN** the user runs `clauductor doctor` in it
- **THEN** every file in the repository and under the home directory is unchanged afterwards

#### Scenario: [DOCTOR-3-S2] The last line counts what was found, and the exit status follows the failures
- **GIVEN** a machine and project with one failing line and two warnings
- **WHEN** the user runs `clauductor doctor`
- **THEN** the last line says 1 failed and 2 warnings
- **AND** doctor exits 1
- **AND** with the failure fixed and the warnings left, it exits 0
- **AND** with nothing found, the last line says nothing is missing or broken
