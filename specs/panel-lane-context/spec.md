# panel-lane-context Specification

## Purpose
The panel launches each lane with the context policy its config sets: a context window at which Claude Code compacts, claude.ai's connectors on or off, and optionally a model for the lane's subagents. It passes the policy at launch, so nothing is written in the repository. A lane's context bar and alert then measure against the window that lane was launched with.

## Requirements
### Requirement: A lane launches with its config's context policy
WHEN the panel launches a lane (Start, Restart, Resume or Restore) and its config sets a context policy for the lane's type THE SYSTEM SHALL pass `--settings` with an `env` object holding exactly the policy's variables, built from validated config values; and WHEN the config sets no policy THE SYSTEM SHALL launch the lane with no `--settings`.

#### Scenario: [LANECTX-1-S1] A new lane carries the window as settings
- **GIVEN** a config with `"launch": {"context_window": 200000}`, and a lane type `fix` with no `context_window` of its own
- **WHEN** a `fix` lane starts
- **THEN** claude's argv holds `--settings` followed by one element, the JSON `{"env":{"CLAUDE_CODE_AUTO_COMPACT_WINDOW":"200000"}}`
- **AND** the argv ends in `--session-id <uuid>`, as today

#### Scenario: [LANECTX-1-S2] A resumed lane carries the policy again
- **GIVEN** the config of LANECTX-1-S1, and a lane whose conversation exists
- **WHEN** the lane is restarted, resumed or restored
- **THEN** claude's argv holds the same `--settings` element and ends in `--resume <uuid>`

#### Scenario: [LANECTX-1-S3] A lane type's window overrides the default
- **GIVEN** `"launch": {"context_window": 200000}` and `"lane_types": {"propose": {"context_window": 300000}}`
- **WHEN** a `propose` lane and a `fix` lane start
- **THEN** the `propose` lane's settings set `CLAUDE_CODE_AUTO_COMPACT_WINDOW` to `"300000"`, and the `fix` lane's to `"200000"`

#### Scenario: [LANECTX-1-S4] A config without a policy launches as today
- **GIVEN** a config with no `launch` key and no `context_window` or `subagent_model` in `lane_types`
- **WHEN** a lane starts
- **THEN** claude's argv holds no `--settings`, and equals today's argv for that lane

#### Scenario: [LANECTX-1-S5] Connectors off is one more variable in the same settings
- **GIVEN** `"launch": {"context_window": 200000, "claudeai_connectors": false}`
- **WHEN** a lane starts
- **THEN** its settings `env` holds `"ENABLE_CLAUDEAI_MCP_SERVERS": "false"` and `"CLAUDE_CODE_AUTO_COMPACT_WINDOW": "200000"`, and no other key

#### Scenario: [LANECTX-1-S6] A subagent model sets the default for subagents, not a forced one
- **GIVEN** `"lane_types": {"fix": {"subagent_model": "sonnet"}}`
- **WHEN** a `fix` lane starts
- **THEN** its settings `env` holds `"CLAUDE_CODE_SUBAGENT_MODEL": "sonnet"`
- **AND** it holds no `CLAUDE_CODE_SUBAGENT_MODEL_FORCE`

#### Scenario: [LANECTX-1-S7] A window outside Claude Code's range does not load
- **GIVEN** a config with `"launch": {"context_window": 99999}`, or a lane type with `"context_window": 1000001`
- **WHEN** the panel loads the config
- **THEN** the load fails, and the error names the key and the range 100000–1000000

### Requirement: A lane's context is measured against the window it was launched with
WHEN a lane was launched with a context window THE SYSTEM SHALL record that window for the lane, report it as the lane's window, and compute the lane's context percentage, its context alert and its bar's auto-compact tick against it: the tick at the window less Claude Code's 33000-token autocompact buffer, as a share of the window, and the alert at the lower of `alerts.context_pct` and the tick less 5 points, unless `alerts.context_pct` is 0, which keeps the alert off. THE SYSTEM SHALL take a session's window from its status post's `auto_compact_window` when present, else from the lane record carrying the session's id, and never from a lane whose worktree the session merely runs in; and WHEN a lane was launched with none THE SYSTEM SHALL keep Claude Code's `used_percentage`, the configured alert threshold and today's tick.

#### Scenario: [LANECTX-2-S1] A 200k window on a 1M-token model reads against 200k
- **GIVEN** a lane launched with a 200000 window, and `alerts.context_pct` 85
- **WHEN** its status line posts `context_window_size` 1000000, `used_percentage` 18, and a `current_usage` whose input, cache-creation and cache-read tokens sum to 180000
- **THEN** the lane's context reads 90%, both as the session's figure and as the lane's metrics' figure
- **AND** the lane's metrics read a 200000 window, not 1000000
- **AND** the context alert fires for the lane

#### Scenario: [LANECTX-2-S2] A lane launched without a window keeps Claude Code's figure
- **GIVEN** a lane launched with no window
- **WHEN** its status line posts `used_percentage` 42
- **THEN** the lane's context reads 42%

#### Scenario: [LANECTX-2-S3] A config edit does not move a running lane's measure
- **GIVEN** a lane launched with a 200000 window
- **WHEN** the config's window becomes 300000, and the lane posts 180000 tokens of context before it is relaunched
- **THEN** the lane's context still reads 90%
- **AND** after a restart it reads against 300000

#### Scenario: [LANECTX-2-S4] The page's warning colour follows the alert threshold
- **GIVEN** `alerts.context_pct` 80, and a lane launched with no window
- **WHEN** the page draws the lane while its context reads 82%
- **THEN** the context figure is drawn in the warning colour, though it is under 85

#### Scenario: [LANECTX-2-S5] The auto-compact tick sits where Claude Code compacts, per lane
- **GIVEN** three lanes: one launched with a 200000 window, one with a 100000 window, and one with none
- **WHEN** the state is computed
- **THEN** the 200000 lane's tick sits at 83.5, that is (200000 − 33000) / 200000 of its bar
- **AND** the 100000 lane's tick sits at 67
- **AND** the lane with no window keeps today's 95

#### Scenario: [LANECTX-2-S6] A windowed lane's alert fires before it compacts
- **GIVEN** `alerts.context_pct` 85, and a lane launched with a 200000 window
- **WHEN** its status line posts a `current_usage` whose three token counts sum to 160000
- **THEN** the lane's context reads 80%, and its alert threshold is 78.5, its tick less 5 points
- **AND** the context alert fires for the lane, and the page draws its figure in the warning colour
- **AND** with `alerts.context_pct` 80 and a lane launched with a 300000 window, the lane's alert threshold is 80

#### Scenario: [LANECTX-2-S7] An alert turned off stays off for a windowed lane
- **GIVEN** `alerts.context_pct` 0, and a lane launched with a 200000 window
- **WHEN** its status line posts a `current_usage` whose three token counts sum to 160000
- **THEN** the lane's context reads 80%
- **AND** no context alert fires for the lane, and the page does not draw its figure in the warning colour

#### Scenario: [LANECTX-2-S8] A lane keeps its window after /clear
- **GIVEN** a lane launched with a 200000 window, whose status line posts `"auto_compact_window": 200000`
- **WHEN** the owner runs `/clear` in it, and its status line then posts under a session id that no lane record holds, with `"auto_compact_window": 200000` and a `current_usage` whose three token counts sum to 180000
- **THEN** the session's context reads 90%

#### Scenario: [LANECTX-2-S9] A session started by hand in a windowed lane's folder keeps Claude Code's figure
- **GIVEN** a lane launched with a 200000 window in a worktree
- **WHEN** a session the owner started by hand in that worktree posts, under a session id no lane record holds, with no `auto_compact_window`, `used_percentage` 42 and a `current_usage` whose three token counts sum to 420000
- **THEN** that session's context reads 42%
- **AND** no context alert fires for it

### Requirement: Init and attach write the default policy
WHEN `panel init` or `panel attach` writes a config THE SYSTEM SHALL write `"launch": {"context_window": 200000, "claudeai_connectors": false}` and declare the latest config version; and a config that declares a version before 6 SHALL NOT use the policy's keys.

#### Scenario: [LANECTX-3-S1] An attached repository's lanes launch with the policy
- **GIVEN** a git repository that is not registered
- **WHEN** the owner attaches it and starts a lane in it
- **THEN** the sidecar declares the latest config version and holds the default `launch` object
- **AND** the lane's argv holds the `--settings` of LANECTX-1-S5

#### Scenario: [LANECTX-3-S2] A version-5 config cannot use the policy's keys
- **GIVEN** a config declaring `"version": 5` that sets `launch.context_window`
- **WHEN** the panel loads it
- **THEN** the load fails, and the error names the key and version 6

