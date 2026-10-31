# S19 spikes: what the engine architecture rests on, measured (2026-10-31)

Claude Code 2.1.316 and Go 1.26.1, on the owner's Mac, inside the Seatbelt sandbox. Every run
used `--setting-sources project`. The scratch repos were under `$TMPDIR/spikes`. Landed in
`docs/plans/` with OPS-182; ADR-0016 to ADR-0022 cite these results by number.

| # | Question | Result | Consequence |
|---|---|---|---|
| S1a | A PreToolUse command hook execs a missing binary (exit 126 or 127) | **The call ran.** Logged as an error only | A broken engine fails open by default |
| S1b | The hook exits 2 | **Blocked**, with stderr shown to Claude | Exit 2 is the only default block |
| S1c | The hook hangs past `"timeout": 5` | **The call ran** after the timeout | A hung engine fails open by default |
| S1d | S1c plus `"onFailure": "block"` | **Blocked**: "timed out; blocking because onFailure is "block"" | Every control hook carries `onFailure: "block"` (ADR-0018) |
| S1e | S1a plus `"onFailure": "block"` | **Blocked** | as S1d |
| S2a | Plugin MCP tool, headless, no allow rule | **Denied**, with no prompt: listed in `permission_denials` | The template allows the engine's servers explicitly (ADR-0017) |
| S2b | Allow rule `mcp__plugin_<plugin>_<server>__*` | **Works**; the tool ran in 10 ms; the name is `mcp__plugin_spikeplug_engine-read__ping` | The wildcard per server is the allow rule |
| S2c | The server binary exits at once | **The tool silently disappears**; the run exits 0 | No control may live on MCP (ADR-0016) |
| S2d | An `mcp_tool` hook at SessionStart | **Refused**: "not available for the 'SessionStart' hook event (no MCP client context)" | No `mcp_tool` hooks in the design (ADR-0017) |
| S2e | An `mcp_tool` hook at UserPromptSubmit | It ran, but **its output never reached the model**; a command hook's output did | as S2d; context comes from command hooks |
| S2f | Binary under `engine/`, not `bin/` | **Works**; the server connects in 16–30 ms | `engine/` is the binary directory (claude.ai and Cowork refuse a top-level `bin/`) |
| S3a | `prompt` hook, PreToolUse, ok:false | The call was blocked, and **the whole session ended** (`terminal_reason: hook_stopped`) | No prompt hooks on PreToolUse in hands-off runs (ADR-0020) |
| S3b | `prompt` hook, Stop, ok:false | **The session continued**, with the reason as its next instruction; the second Stop saw `stop_hook_active` and ended | A Stop gate is a self-correction nudge, bounded to one loop |
| S3c | `agent` hook, PreToolUse, ok:false | **Only the call was blocked**; Claude went on; 2.2 s | Tool-call gates are agent hooks (ADR-0020) |
| S4 | Per-call cost: sh launcher exec'ing the Go engine | Median 3.87 ms, against 3.64 ms for a bare `sh -c`; Claude Code's own hook wall time was 19–26 ms | Latency is not a risk; one dispatcher per event is enough |

**Not tested here.** Nested sessions inherit this session's sandbox, so these wait for Gate A on
the Mac, outside the sandbox (OPS-199):
- whether hooks write outside the sandbox (`CLAUDE_PLUGIN_DATA`, a SessionStart command hook's
  session env);
- whether a panel lane reaches plugin MCP tools;
- whether a cloud Claude Code session runs the plugin's engine.

**Out of scope.** Other clients (Cursor): the design covers Claude Code only (the owner,
2026-10-31; ADR-0016's *Rests on*), so OPS-199 does not test them.
