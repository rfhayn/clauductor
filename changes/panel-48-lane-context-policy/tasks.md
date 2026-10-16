# Tasks: panel-48-lane-context-policy

## Progress
- 2026-10-16 proposed; nothing built yet

## Decision log
- (none yet)

## 1. The probes (D8)
The orchestrator runs these outside the sandbox. It uses a throwaway git repository in the
session's scratchpad, tmux socket `p48-probe`, Sonnet, a 100000 window, and at most $5. It
records the results in `changes/panel-48-lane-context-policy/evidence/README.md` under
*Probe results*, and in the Decision log.
- [ ] 1.1 P1 and P4: start claude in the probe repository with `--settings '{"env":{"CLAUDE_CODE_AUTO_COMPACT_WINDOW":"100000"}}'`, and a `statusLine` that appends its stdin to a file.
  - Fill the context past 100k, then record where compaction fires (the `PreCompact` moment against the last status post's tokens), and what `context_window_size` and `used_percentage` read.
  - If no compaction fires near the window, stop here and report to the owner (Refusals).
  - (manual: needs the owner's login and real spend)
- [ ] 1.2 P2: put `"env": {"CLAUDE_CODE_AUTO_COMPACT_WINDOW": "1000000"}` in the probe repository's `.claude/settings.json`, launch with the `--settings` of 1.1, and confirm the settings' 100000 applies. (manual: as 1.1)
- [ ] 1.3 P3: `/context` at startup with connectors on, then with `ENABLE_CLAUDEAI_MCP_SERVERS=false` in the settings `env`; record both totals and the difference. (manual: as 1.1)
- [ ] 1.4 P5: exit the 1.1 session, relaunch it with `--resume <id>` and the same `--settings`, and confirm the window still applies. (manual: as 1.1)
- [ ] 1.5 Commit the audit scripts the measurement ran into `changes/panel-48-lane-context-policy/evidence/tool/` (D12): `usage-report.mjs`, `audit.mjs` and `driver.mjs`.
  - The transcript folder becomes an argument; no home path is written in.
  - No transcript data or CSV is committed.
  - A README says how the outcome check re-runs them.
  - (manual: an evidence tool for the outcome check, not shipped code)

## 2. The config and the launch (D1, D2, D6, D11)
- [ ] 2.1 Config version 6 in `framework/internal/panel/config/fields.go` and `config.go`.
  - New keys: `launch.context_window`, `launch.claudeai_connectors`, `lane_types.*.context_window` and `lane_types.*.subagent_model`, with their validation.
  - Tested in `framework/internal/panel/config/config_test.go`, citing [LANECTX-1-S7].
  - Version gating tested in `config/version_test.go`, citing [LANECTX-3-S2].
  - Regenerate `docs/panel.schema.json` and the reference table in `docs/panel.md`, as `config/generated_test.go` requires.
- [ ] 2.2 `LaneCommand` in `framework/internal/panel/lanes/lanes.go` builds `--settings` from the lane's effective policy, with `json.Marshal` over the validated values, on every launch.
  - It records the effective window in the lane's registry record at each launch.
  - Tested in `framework/internal/panel/lanes/lanes_test.go`, citing [LANECTX-1-S1], [LANECTX-1-S2], [LANECTX-1-S3], [LANECTX-1-S4], [LANECTX-1-S5] and [LANECTX-1-S6].

## 3. Init, attach and the presets (D3, D4)
- [ ] 3.1 `framework/internal/panel/install/init.go`'s starter writes `"launch": {"context_window": 200000, "claudeai_connectors": false}` and version 6.
  - Tested in `framework/internal/panel/install/attach_test.go`, citing [LANECTX-3-S1].
  - `install/init_test.go`'s expected configs and ATTACH-1-S2's test still pass.
- [ ] 3.2 Add the same `launch` object, and version 6, to `.clauductor/panel.json` and `template/.clauductor/panel.json`.
  - Rebuild the plugin with `scripts/build-plugin.sh`.
  - Run `framework/clauductor update` from the repository root with the checkout's binary.

## 4. The bar and the alert (D7)
- [ ] 4.1 `framework/internal/panel/state/model.go` computes a lane's context against its recorded window: `current_usage`'s input, cache creation and cache read tokens over the window.
  - Lanes with no recorded window keep `used_percentage`.
  - The alert in `state/alerts.go` compares the new figure, and the tick in `state/trends.go` sits where P1 saw compaction fire.
  - Tested in `framework/internal/panel/state/alerts_test.go` and `state/metrics_test.go`, citing [LANECTX-2-S1], [LANECTX-2-S2] and [LANECTX-2-S3].
- [ ] 4.2 `framework/internal/panel/web/static/panel.js` draws the context warning colour from the alert threshold the state carries, not a hard-coded 85.
  - Tested in `framework/internal/panel/web/metrics_test.go`, citing [LANECTX-2-S4].
- [ ] 4.3 `docs/panel.md` describes the launch policy (the keys, `--settings`, and what a repository cannot override) and the calibrated bar, and names the limits: managed settings win, and an attached lane with no status line shows no bar until PANEL-49.

- [ ] Slice: an owner can start a lane in an attached repository from the panel at http://127.0.0.1:<port> and see it launched with a 200k context window and connectors off
