# Evidence: how the panel shapes the context of the sessions it drives

This is the owner's context review of 2026-10-15. It covers what Clauductor, as the application
that launches and wakes sessions, decides about their context and cost in a repository it attaches
to. It does not cover Clauductor's own development sessions.

- **Data:** Standing Tee's Claude Code transcripts on the owner's machine, 2026-10-08 to 10-14.
  That is 76 sessions, 393 subagents and 18,378 calls, **$1,208.28 at list price**.
  - The figures are list-price equivalents, not the bill.
  - Haiku 5.5 is unpriced in the tool (42 calls, about $0.13 at its documented rate) and is
    reported as UNPRICED, never $0.
- **Tool:** the audit tool from Standing Tee's own transcript audit (`usage-report.mjs`,
  `audit.mjs`), extended with `driver.mjs` for the driver-decided measures. It is committed under
  `tool/` by task 1.5. It reproduced the earlier audit's headline exactly.
- **Prices**, in $ per million tokens:

  | Model | Input | Output | Cache read |
  |---|---|---|---|
  | Opus 5.5 | 4 | 20 | 0.20 |
  | Sonnet 5.5 | 2 | 10 | 0.20 |

  A cache write costs 1.25× input for 5 minutes and 2× for an hour.

## 1. Where the panel touches a session (the code at main 43df973)

- **One launch site.** `LaneManager.LaneCommand` (`framework/internal/panel/lanes/lanes.go`) builds
  the argv for Start, Restart, Resume and Restore. It sets:
  - `--model` and `--effort`, only when `lane_types` names them; the sidecar `panel attach` writes
    has no `lane_types`;
  - `--remote-control`, if chosen at install;
  - `-n <id>`;
  - and `--session-id` or `--resume`.

  It sets no `--settings`, no MCP flags and no context variable.
- **Typed text.** The panel types four things into a lane:
  - a template's first prompt, once;
  - the quota auto-resume line, at most once per rate-limit stop;
  - `/remote-control`;
  - `/exit`.

  It wakes no session and sends no reportTo; that is the repository's own process.
- **Resume.** Restart, Resume and Restore all use `--resume`, which reloads the whole conversation.
- **Measures.** The context bar is Claude Code's `used_percentage`, from a status line that a
  repository file posts. An attached repository's lanes show no context, cost or alert unless its
  own status line posts (PANEL-49). The page hard-codes an 85 warning colour, and the bar's
  auto-compact tick is an inferred constant, 95.

## 2. The control surface, from the Claude Code docs (code.claude.com/docs, 2026-10-15)

| Control | Set at launch | Reaches subagents | Can the repository override it? | Source |
|---|---|---|---|---|
| `CLAUDE_CODE_AUTO_COMPACT_WINDOW` (100000–1000000, a plain integer) | yes | yes; it beats a subagent's own `autoCompactWindow` | as an exported variable, yes: a settings file's `env` "applies in most sessions". Passed in `--settings`, no: `--settings` ranks above project and local settings | env-vars, model-config, sub-agents |
| `--settings <json>` | yes; it lasts one session and writes no file | its hooks and permissions do | managed settings > `--settings` > local > project > user | settings, cli-reference |
| `ENABLE_CLAUDEAI_MCP_SERVERS=false` | yes | n/a | the docs say a `true` from any source wins, which reads as: a repository's own `true` turns connectors back on. Unverified; probe P3 tests it | env-vars, mcp |
| `CLAUDE_CODE_SUBAGENT_MODEL` (`_FORCE=1` overrides agent definitions) | yes | yes | an agent definition's own model wins unless forced | model-config, sub-agents |
| `--resume` | — | — | — | sessions: `--settings`, `--mcp-config` and `--plugin-dir` are NOT restored and must be passed again |
| The status line's `used_percentage` | — | — | — | statusline: measured against the model's whole window, input tokens only. There is no field for the compaction threshold |

**Unverified in the docs:**
- whether `--strict-mcp-config` also drops claude.ai connectors and plugin servers;
- what `--setting-sources` does to CLAUDE.md;
- the per-call cost of `--append-system-prompt`.

`CLAUDE_CODE_AUTO_COMPACT_WINDOW` is documented but absent from `claude --help` on 2.1.300. Probe P1
settles it.

## 3. The measurement

**What a cap on lifetime is worth.** The simulation compacts a thread to its startup plus 30k once
it passes the cap. Each compaction costs a full re-read and 10k of output. Repository content is held
fixed, and quality is not modelled.

| Cap | Orchestrator | Ops | Fix | Propose | Subagents | All, net per week |
|---|---|---|---|---|---|---|
| 150k | $26.35 | $36.58 | $5.53 | $7.23 | $58.75 | **$133.20** |
| 200k | $30.74 | $51.26 | $13.89 | $11.02 | $56.42 | **$163.33** |
| 300k | $27.07 | $45.26 | $12.31 | $11.28 | $38.26 | **$134.17** |

**The other levers, each alone, per week:**
- **A lean startup at 25k: $63.36.** Startup costs $212 a week. A lane's first call is about 50k:
  - system prompt and tools, 24k;
  - CLAUDE.md and AGENTS.md, 11k, which are the repository's own;
  - the skill listing, 7k;
  - the agent listing, 2k.

  So about $2 a week per 1k removed from every main-thread startup. The connectors' share is not
  measured (probe P3).
- **Subagents on Sonnet 5.5: up to $151.78**, on price alone. Opus 5.5 and Sonnet 5.5 cost the same
  per cache read, so only writes and output fall. The effect on quality is not measured.
- **Keep-alive pings through idle gaps: $71.28.** But 50 of the 101 cold subagent calls ($44.84)
  sat inside a foreground gate run, where nothing can ping, and those are the repository's own
  process.
- **Subagents on the 1-hour cache: −$88.24** (writes at 2×).
- **Batched orchestrator wakes:** $8.98 at 2 per wake, $11.97 at 3, $14.36 at 5. A 60k routing
  session would save $40.89, plus $25.80 if the orchestrator got a 1k digest. The panel sees none of
  these wakes: they are the repository's own lanes messaging its orchestrator.
- **Restart with a handoff instead of `--resume`: about $1.** No resumes happened in the week.

**Together:** the cap at 200k, the lean startup and keep-alive, simulated together per thread, plus
batching 3 wakes, come to **$286.24 (23.7%)**. They overlap, so it is not the sum of the parts.

**What the repository's own content decides:** $529 of the $971 traceable to content. The largest
items are Read $122, Bash sed $85, grep $65, CLAUDE.md/AGENTS.md $65, and git $50.

## 4. The levers the panel can pull, ranked

| # | Change | Saving per week | In attach mode | Row |
|---|---|---|---|---|
| 1 | A context window per lane type, set at launch (default 200k) | $153.79, since a 200k window compacts at about 167k (*The cap re-run*; $163.33 was a cap at 200k) | yes, through `--settings` | PANEL-48 |
| 1b | The bar and the alert measured against that window | none directly; without it the alert never fires | where a status line posts | PANEL-48 |
| 2 | Attached lanes' context and cost read from their transcripts | none directly; it makes 1 visible | yes | PANEL-49 |
| 3 | claude.ai connectors off at launch | about $2.80: P3 measured 1.4k at $2 per 1k (first estimated $4–10) | yes | PANEL-48 |
| 4 | A subagent model per lane type (owner's choice, off by default) | up to $151.78, price only | yes, but it overrides the repository's choice | PANEL-48 |
| 5 | Keep-alive pings into an idle orchestrator | at most $18.67 | yes, but every ping is a turn in the owner's conversation | not proposed |
| 6 | Batched or routed wakes | $11.97–$66.69 | no: the panel sees none of them | not proposed |
| 7 | Restart with a handoff | about $1 | — | not proposed |
| 8 | Hooks passed at launch to trim tool output | not measured | yes | not now: Standing Tee trims in its own repository |
| 9 | The per-lane top context contributors | none directly | after PANEL-49 | PANEL-50 |

## 5. What could not be measured
- **Who typed a lane's prompt:** the transcript records the panel and the owner alike.
- **Startup:** it cannot be split into system prompt and tool schemas; that part is a remainder.
- **Any lever's effect on the quality of the work.**
- **Whether a keep-alive could reach a session blocked in a foreground tool call.**
- **Why 10 caches went cold inside their TTL.**
- **Scheduled wakes:** none appeared.
- **Re-reads after the 5 real compactions:** these are slightly overcounted.
- **A cache prefix shared between threads.**
- **Haiku 5.5's tokenizer.**
- **The actual bill.**

## Probe results
Run on 2026-10-17 by group 1 (`tasks.md` 1.1–1.4). The setup:
- **Claude Code:** the installed version was 2.1.302, not the 2.1.300 the design names. It ran on
  Sonnet 5.5, on the owner's login (Claude Max).
- **Where:** a throwaway git repository in the session's scratchpad, on tmux socket `p48-probe`.
- **The launch:** each session started with `env -i` (HOME, USER, PATH, TERM and LANG only) and
  `claude --model sonnet --settings '<json>'`.
- **The settings:** the JSON's `env` held the variables under test. Its `statusLine` and its
  `PreCompact`, `Stop` and `SessionStart` hooks appended their stdin to log files.
- **The fill:** the repository held 30 filler files of about 11k tokens each, read one per turn.
- **The spend:** $0.25 at list price (the status line's `total_cost_usd`; pricing the transcript
  gives $0.254). One session made all the API calls; the others only ran `/context` and `/mcp`.

**P1: compaction fires, but at a fixed 33k below the window, not at a share of it.**
- **At 100000:** `/context` read "Auto-compact window: 100k tokens" and "Autocompact buffer: 33k
  tokens (33.0%)". The last call before compaction held 64,912 tokens. The next turn's tool result
  crossed the line, and compaction ran mid-turn: `PreCompact` with `"trigger":"auto"`, and the
  transcript's `compact_boundary` recorded `preTokens` 67,216 and `postTokens` 6,669. So it fired
  at 67.2% of the window.
- **At other windows:** `/context` showed the same 33k buffer at 200k (16.5%), 300k (11.0%) and
  1m (3.3%). The buffer is absolute, so compaction fires at the window minus 33k. That is 83.5%
  of a 200k window, 89% of 300k, and 96.7% of 1m (today's inferred 95 is this case).
- **Consequence for D7, LANECTX-2-S5 and D4:** a single "P1 share" places the tick right only for
  a 100k window. The default 85% alert (`DefaultContextPct`) sits above a 200k lane's compaction
  point, 83.5%, so it would never fire. A "200k window" acts as a cap of about 167k, while D4's
  simulation compacted at 200k. These are the owner's decisions: see the Decision log.

**P4: the status line does not report against the set window.**
- **What it reported:** under the 100000 window, every post read `context_window_size` 1000000.
  At 64,912 tokens it read `used_percentage` 6, which is a share of the model's 1M window.
- **The figure D7 uses works:** `current_usage` carried `input_tokens`,
  `cache_creation_input_tokens` and `cache_read_input_tokens`. Their sum, 64,912, over the
  recorded window gives 65%.
- **So:** D7's calibration is needed. The branch "if P4 shows Claude Code already reports against
  the window" does not apply.

**P2: `--settings` beats the repository's own `env`.**

| The repository's `.claude/settings.json` `env` window | The `--settings` window | `/context` showed |
|---|---|---|
| 1000000 | 100000 | 100k |
| 300000 (control) | none | 300k |
| 300000 (control) | 100000 | 100k |

The control shows that the repository's value is read, and that `--settings` outranks it.

**P3: the connectors cost 1.4k at startup, and a repository cannot turn them back on.**
- **The totals:** `/context` at startup read 32.9k with the connectors on and 31.5k with
  `ENABLE_CLAUDEAI_MCP_SERVERS=false` in `--settings`' `env`: 1.4k less.
  - "MCP tools" fell from 612 tokens to none, as the deferred tool listing went from 89 tools to
    26.
  - "MCP server instructions" fell from 1.1k to 361.
  - `/mcp` went from 9 servers to 3. The six claude.ai connectors (Claude Docs, Gmail, Google
    Calendar, Google Drive, Indeed and Uber; 63 tools) were gone, and the user's own
    `writing-library` and the built-in servers stayed.
- **What it is worth:** at section 3's $2 a week per 1k, about $2.80 a week, below the $4–10
  estimated in section 4.
- **A caveat on the totals:** `/context` estimates low. The same startup's first API call held
  50,375 tokens against `/context`'s 32.9k, so the true saving may be larger.
- **Can a repository undo it?**

  | The repository's `env` | `--settings`' `env` | Connectors |
  |---|---|---|
  | `ENABLE_CLAUDEAI_MCP_SERVERS=true` | `false` | **off**: 3 servers, none from claude.ai |
  | `ENABLE_CLAUDEAI_MCP_SERVERS=false` (control) | none | off |

  The control shows that the repository's value for this key is read. With `--settings` at
  `false`, its `true` does not bring the connectors back. On 2.1.302, the docs' "a `true` from any
  source wins" does not hold against `--settings`.

**P5: `--resume` keeps the window only when `--settings` is passed again.**
- The P1 session, resumed with `--resume <id>` and the same `--settings`, read "Auto-compact
  window: 100k tokens".
- Resumed with `--settings '{}'`, it read "1m tokens".
- This confirms D1: the policy has to go on every launch.

**P1 on Opus: the same 33k buffer.** P1 ran on Sonnet, but this repository's lanes run Opus
(`.clauductor/panel.json`'s `lane_types`). So on 2026-10-17, after the amendment's review, the
buffer was read again under `--model opus`.
- **How:** in a fresh throwaway git repository in the session's scratchpad, with
  `env -i … claude -p --model opus --settings '<json>' "/context"`, once per window, on Claude
  Code 2.1.302 (`claude --version`). It used
  print mode, not tmux, because the sandbox refused the tmux socket. `/context` is a local
  command: the runs wrote no transcript, so no API call is on record.
- **The readings** (`/context`'s "Tokens" and "Autocompact buffer" lines; the model read
  `claude-opus-5-5`):

  | Model | Window | Buffer | Share |
  |---|---|---|---|
  | Opus 5.5 | 100000 | 33k | 33.0% |
  | Opus 5.5 | 200000 | 33k | 16.5% |
  | Opus 5.5 | 300000 | 33k | 11.0% |
  | Opus 5.5 | none (1m) | 33k | 3.3% |
  | Sonnet 5.5 (control) | 200000 | 33k | 16.5% |

- **So:** the buffer `/context` reports is the same on Opus as on Sonnet. The Sonnet control
  matches P1's own reading, which shows that print mode's `/context` reports the same figure as
  the interactive one.
- **Not observed:** no Opus session was filled to compaction. That Opus compacts at the window
  less 33k is inferred from the reported buffer, as P1 showed the buffer predicting compaction on
  Sonnet. The outcome check's reading of `compact_boundary`'s `preTokens` is the backstop.

## The cap re-run at the compaction point
Run on 2026-10-17, after P1, for design.md D4. Section 3 simulated compaction AT the cap. P1 shows
that a window compacts at the window less 33k, so a 200k window acts as a cap of about 167k.
- **How:** `audit.mjs` over the baseline week, 2026-10-08..14 (arguments `2026-10-08 2026-10-15`,
  the end exclusive), into a scratchpad, then a
  scratchpad copy of `driver.mjs` with two constants made settable: the cap list (`CAPS`) and the
  combined figure's cap (`COMB_CAP`, 167000). The committed `tool/` is unchanged.
- **Check:** the run reproduced section 3's 150k, 200k and 300k rows to the cent. Its headline read
  $1,208.24 over 18,377 calls, one call fewer than on 2026-10-15, so one transcript has changed
  since.

| Window | Cap (window − 33k) | Orchestrator | Ops | Fix | Propose | Subagents | All, net per week |
|---|---|---|---|---|---|---|---|
| 150k | 117k | −$10.05 | −$2.75 | −$37.06 | −$5.96 | $27.03 | **−$31.73** |
| 183k | 150k | $26.35 | $36.58 | $5.53 | $7.23 | $58.75 | **$133.20** |
| 200k | 167k | $30.75 | $44.23 | $10.62 | $9.22 | $59.72 | **$153.79** |
| 233k | 200k | $30.74 | $51.26 | $13.89 | $11.02 | $56.42 | **$163.33** |
| 300k | 267k | $30.53 | $49.40 | $12.99 | $10.74 | $41.82 | **$145.48** |
| 333k | 300k | $27.07 | $45.26 | $12.31 | $11.28 | $38.26 | **$134.17** |

- **Compactions at 167k:** 186 a week: orchestrator 27, ops 43, fix 43, propose 16, subagents 56.
- **Together** (lean startup 25k, the cap at 167k on every class, keep-alive, and batching 3
  wakes): **$286.41 (23.7%)**, against $286.24 at a 200k cap. The levers overlap, so a lower cap
  barely moves the combined figure.
