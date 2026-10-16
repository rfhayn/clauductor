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
| `ENABLE_CLAUDEAI_MCP_SERVERS=false` | yes | n/a | a `true` from any source wins, so a project cannot turn connectors back on | env-vars, mcp |
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
| 1 | A context window per lane type, set at launch (default 200k) | $163.33 | yes, through `--settings` | PANEL-48 |
| 1b | The bar and the alert measured against that window | none directly; without it the alert never fires | where a status line posts | PANEL-48 |
| 2 | Attached lanes' context and cost read from their transcripts | none directly; it makes 1 visible | yes | PANEL-49 |
| 3 | claude.ai connectors off at launch | about $2 per 1k removed; estimated $4–10 | yes | PANEL-48 |
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
(Filled in by group 1, `tasks.md` 1.1–1.4.)
