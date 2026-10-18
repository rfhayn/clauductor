#!/usr/bin/env node
// Token usage and API-equivalent cost for Claude Code sessions in this repo, by role and model.
//
// WHY THIS EXISTS (2026-09-23). The model-per-role choices (`.claude/model-roles.json`) were a
// hypothesis with no measurement behind them: `docs/playbook.html` carried "token cost per group —
// not yet captured" from the first `build-change` run onward, and the evaluation memory asked for
// cost to be recorded next to defects found. A note asking for a number is not a mechanism (rule
// 4); this prints the number, and `session-close` runs it.
//
// The first run overturned the premise the roles were chosen on. Cost here is dominated by CACHE
// READS — context re-read on every turn — not by output or thinking: over 2026-09-18..23 (~$1,943
// list-price) the builder and reviewer, counting the reviewer's /code-review agents, re-read ~1.8B
// cached tokens and generated ~8.6M, and every agent started at ~55k context. So the levers, in
// order, are startup context × turns, then price per cached token, then effort — which is why
// this reports `avg start ctx` per role. Since 2026-09-27 a second table splits each top-level
// role's dollars into cache read / cache write / output and gives its average context PER CALL:
// startup is ~15-20% of a call's context, and the rest (whole-file reads, gate logs) is what the
// agent-context-diet changes target, so that column is the one to watch week over week.
//
// Every way this report can be wrong degrades to a PLAUSIBLE SMALLER OR LARGER NUMBER, never to an
// error, so each is handled explicitly and pinned by `usage-report.test.ts`:
//   - workflow agents nest one directory deeper (`subagents/workflows/<run>/`); the first version
//     skipped them and reported ~$968 for the ~$1,943 week;
//   - agents spawned BY an agent (a reviewer's `/code-review` finders, `spawnDepth` 2+) carry only
//     `general-purpose` + `parentAgentId`; they are charged to their ROOT's role, grouped under
//     the root's direct child (`agent:reviewer › code-review` holds that run's finders too), and
//     the rollup totals each root with everything under it. Before this, 69 such agents made
//     review look cheaper than it is. A chain that cannot reach a root (a parent-id cycle, a
//     missing parent meta) is pooled under `agent:?` and shows in the rollup, never guessed;
//   - a streamed response appears on several lines with one message id; records dedupe by id;
//   - a model or speed with no known price is listed as UNPRICED, never costed at zero.
//
// SUBJECT FIRST. The report opens by naming exactly which session(s) and window it summed
// (AGENTS.md: a green result must name its subject). By default that is the CALLING session
// (`CLAUDE_CODE_SESSION_ID`), not the most recently written transcript — with two sessions open,
// "most recent" is someone else's.
//
//   node infra/usage-report.mjs                      # the calling session (main + its agents)
//   node infra/usage-report.mjs --session <id>       # one session by id
//   node infra/usage-report.mjs --since 2026-09-18   # every session with activity since a date
//   node infra/usage-report.mjs --dir <path> ...     # a different transcript directory (tests);
//                                                    # repeatable: the day's cost across the main
//                                                    # checkout's and every worktree's (#388)
//
// Dollars are list-price API equivalents. On a subscription they are a proxy for how fast the
// weekly limit drains, not a bill.

import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { homedir } from "node:os";
import { basename, dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

// $ per million tokens, standard speed. cacheRead is explicit rather than 0.1 × input because Opus
// 5.5 and Fable 5.1 break that ratio. Cache writes: 5-minute TTL = 1.25 × input, 1-hour = 2 ×.
// A key matches its EXACT version only — optionally followed by a date snapshot (`-20251001`) or a
// context tag (`[1m]`). Family-prefix matching priced Opus 4.1 at a third of its real rate, and
// would price any future version (`claude-opus-5-6`) at its family's rate with no warning; an
// unknown version is now UNPRICED, which is visible.
const PRICES = {
  "claude-opus-5-5": { input: 4, output: 20, cacheRead: 0.2 },
  "claude-opus-5": { input: 5, output: 25, cacheRead: 0.5 },
  "claude-opus-4-8": { input: 5, output: 25, cacheRead: 0.5 },
  "claude-opus-4-7": { input: 5, output: 25, cacheRead: 0.5 },
  "claude-opus-4-6": { input: 5, output: 25, cacheRead: 0.5 },
  "claude-opus-4-5": { input: 5, output: 25, cacheRead: 0.5 },
  "claude-opus-4-1": { input: 15, output: 75, cacheRead: 1.5 },
  "claude-opus-4-0": { input: 15, output: 75, cacheRead: 1.5 },
  "claude-opus-4": { input: 15, output: 75, cacheRead: 1.5 },
  "claude-fable-5-1": { input: 10, output: 50, cacheRead: 0.25 },
  "claude-fable-5": { input: 10, output: 50, cacheRead: 1 },
  // Sonnet 5.5: the published pricing page, read 2026-10-07 (#390) — not inferred from Sonnet 5.
  "claude-sonnet-5-5": { input: 2, output: 10, cacheRead: 0.2 },
  "claude-sonnet-5": { input: 2, output: 10, cacheRead: 0.2 },
  "claude-sonnet-4-6": { input: 3, output: 15, cacheRead: 0.3 },
  "claude-sonnet-4-5": { input: 3, output: 15, cacheRead: 0.3 },
  "claude-sonnet-4-0": { input: 3, output: 15, cacheRead: 0.3 },
  "claude-sonnet-4": { input: 3, output: 15, cacheRead: 0.3 },
  "claude-haiku-4-5": { input: 1, output: 5, cacheRead: 0.1 },
};
// Fast mode (`usage.speed === "fast"`): the published pricing page (*Fast mode pricing*, read
// 2026-10-07) prices it at $8/$40 on Opus 5.5 and $10/$50 on Opus 5 and Opus 4.8 — 2× standard,
// and offers it on no other model. The same page says prompt-caching multipliers apply ON TOP of
// fast-mode pricing, so a cache read doubles with the input rate. Any other model's fast mode is
// UNPRICED until its rate is sourced here.
const FAST_MULTIPLIER = { "claude-opus-5-5": 2, "claude-opus-5": 2, "claude-opus-4-8": 2 };

function keyFor(model) {
  const base = model.replace(/\[[^\]]*\]$/, "").replace(/-\d{8}$/, "");
  return Object.hasOwn(PRICES, base) ? base : undefined;
}

export function priceFor(model, speed) {
  const key = keyFor(model);
  if (!key) return null;
  if (speed !== "fast") return PRICES[key];
  const x = FAST_MULTIPLIER[key];
  if (!x) return null;
  const p = PRICES[key];
  return { input: p.input * x, output: p.output * x, cacheRead: p.cacheRead * x };
}

// The cost of a usage record by COMPONENT. Cache writes cost 25-40x a cache read per token on Opus
// 5.5 ($5 or $8 vs $0.20 per M), so "context is expensive" splits into two different levers — fewer
// tokens re-read (reads) versus less NEW context per turn (writes) — and the split says which.
export function partsOf(u, p) {
  if (!p) return null;
  return {
    input: (u.input * p.input) / 1e6,
    cacheWrite: (u.cacheWrite5m * p.input * 1.25 + u.cacheWrite1h * p.input * 2) / 1e6,
    cacheRead: (u.cacheRead * p.cacheRead) / 1e6,
    output: (u.output * p.output) / 1e6,
  };
}

function costOf(u, p) {
  const parts = partsOf(u, p);
  return parts && parts.input + parts.cacheWrite + parts.cacheRead + parts.output;
}

// One record per API response. Streaming writes the same message id on several lines, so records
// are deduped by id — summing lines instead of ids double-counts every streamed turn.
export function readTranscript(file, role, since) {
  const seen = new Map();
  for (const line of readFileSync(file, "utf8").split("\n")) {
    if (!line.includes('"usage"')) continue;
    let e;
    try {
      e = JSON.parse(line);
    } catch {
      continue;
    }
    const m = e.message;
    if (!m?.usage || !m.model || m.model === "<synthetic>") continue;
    if (since && e.timestamp && e.timestamp < since) continue;
    const u = m.usage;
    const total = u.cache_creation_input_tokens ?? 0;
    const w1 = u.cache_creation?.ephemeral_1h_input_tokens ?? 0;
    seen.set(m.id ?? `${file}:${seen.size}`, {
      role,
      model: u.speed === "fast" ? `${m.model} (fast)` : m.model,
      speed: u.speed,
      ts: e.timestamp ?? "",
      input: u.input_tokens ?? 0,
      cacheWrite1h: w1,
      cacheWrite5m: Math.max(0, total - w1),
      cacheRead: u.cache_read_input_tokens ?? 0,
      output: u.output_tokens ?? 0,
    });
  }
  return [...seen.values()];
}

// The label an agent's own metadata gives it, before parent attribution. Shapes seen in real
// transcripts (2026-09-23): a custom agent (`agentType: "builder"`); a workflow step
// (`workflow-subagent` + `workflowPhase`); a named teammate (`customAgentType: "builder"`, or
// `agentType` equal to a run-specific `name`); and a generic spawn (`general-purpose`/`fork` with
// or without a `name` such as `code-review`). Run-specific suffixes (`angle-reuse-2-2`,
// `builder-151-152`) are folded so there is one row per role, not per spawn.
const GENERIC = new Set(["general-purpose", "fork"]);
export function labelOf(meta) {
  if (!meta) return "?";
  if (meta.customAgentType) return meta.customAgentType;
  // Every real workflow step records its phase (surveyed 2026-09-23: 0 of 123 without one); a step
  // without one shows as `workflow:?` rather than being guessed from free-form description text.
  if (meta.agentType === "workflow-subagent") return `workflow:${meta.workflowPhase || "?"}`;
  const raw = GENERIC.has(meta.agentType) && meta.name ? meta.name : (meta.agentType ?? "?");
  return raw.replace(/(-\d+)+$/, "");
}

// An agent spawned by another agent is charged to its ROOT, grouped under the root's direct child:
// a reviewer's `/code-review` run and every finder that run spawned share one row,
// `agent:reviewer › code-review`. Two levels, deliberately — the full chain gave each spawn path
// its own single-agent row (one real `/code-review` run became nine), and root-plus-leaf merged a
// child that shared its root's label into the root's own row. A child sharing the root's label
// still gets its own row (`agent:builder › builder`), so root-row averages stay per-root.
//
// A chain that cannot reach a root goes to ONE bucket, `agent:?`, rather than being guessed: parent
// ids can CYCLE in real metadata (observed: a spawnDepth-3 agent whose parent names it as its own
// parent), and a parent's meta.json could be absent. Charging those to whichever agent the walk
// stopped at under-counts review, the thing this attribution exists to prevent. (0 of 170 real
// parent ids were unresolvable on 2026-09-23, so today the bucket is empty; it exists so it would
// SHOW if that changed.)
export function roleOf(id, metas) {
  const chain = [];
  const seen = new Set();
  let cur = id;
  for (; cur && metas.has(cur) && !seen.has(cur); cur = metas.get(cur).parentAgentId) {
    seen.add(cur);
    chain.unshift(labelOf(metas.get(cur)));
  }
  if (!chain.length || cur) return "agent:?";
  const [root, child] = chain;
  const head = root.startsWith("workflow:") ? root : `agent:${root}`;
  return child === undefined ? head : `${head} › ${child}`;
}

export function readMeta(file) {
  try {
    return JSON.parse(readFileSync(file, "utf8"));
  } catch {
    return null;
  }
}

export function transcriptsUnder(dir) {
  const out = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const p = join(dir, entry.name);
    if (entry.isDirectory()) out.push(...transcriptsUnder(p));
    else if (entry.name.endsWith(".jsonl")) out.push(p);
  }
  return out;
}

export const agentIdOf = (file) => basename(file, ".jsonl").replace(/^agent-/, "");

export function collect(dir, { session, since, exclude = [] } = {}) {
  const mains = readdirSync(dir)
    .filter((f) => f.endsWith(".jsonl") && !exclude.includes(basename(f, ".jsonl")))
    .map((f) => join(dir, f));
  const picked = session
    ? mains.filter((f) => basename(f, ".jsonl") === session)
    : since
      ? mains.filter((f) => statSync(f).mtime.toISOString() >= since)
      : [];
  const agents = [];
  const records = [];
  for (const main of picked) {
    records.push(...readTranscript(main, "session", since));
    const subDir = join(dir, basename(main, ".jsonl"), "subagents");
    if (!existsSync(subDir)) continue;
    const files = transcriptsUnder(subDir);
    const metas = new Map();
    for (const f of files) {
      const meta = readMeta(f.replace(/\.jsonl$/, ".meta.json"));
      if (meta) metas.set(agentIdOf(f), meta);
    }
    for (const f of files) {
      const role = roleOf(agentIdOf(f), metas);
      const recs = readTranscript(f, role, since).sort((a, b) => a.ts.localeCompare(b.ts));
      if (!recs.length) continue;
      agents.push({ role, model: recs[0].model, turns: recs.length, startCtx: ctx(recs[0]) });
      records.push(...recs);
    }
  }
  return { sessions: picked.map((f) => basename(f, ".jsonl")), records, agents };
}

export const ctx = (r) => r.input + r.cacheWrite1h + r.cacheWrite5m + r.cacheRead;
const FIELDS = ["input", "cacheWrite1h", "cacheWrite5m", "cacheRead", "output"];

export function summarize({ records, agents }) {
  const groups = new Map();
  for (const r of records) {
    const k = `${r.role}\t${r.model}`;
    const g = groups.get(k) ?? { role: r.role, model: r.model, speed: r.speed, calls: 0 };
    g.calls++;
    for (const f of FIELDS) g[f] = (g[f] ?? 0) + r[f];
    groups.set(k, g);
  }
  const rows = [...groups.values()].map((g) => {
    const mine = agents.filter((a) => a.role === g.role && a.model === g.model);
    const avg = (f) =>
      mine.length ? Math.round(mine.reduce((s, a) => s + a[f], 0) / mine.length) : null;
    const base = g.speed === "fast" ? g.model.replace(/ \(fast\)$/, "") : g.model;
    const price = priceFor(base, g.speed);
    return {
      ...g,
      agents: mine.length,
      avgTurns: avg("turns"),
      avgStartCtx: avg("startCtx"),
      parts: partsOf(g, price),
      cost: costOf(g, price),
    };
  });
  // Unpriced rows sort last, and compare as equal to each other: `Infinity - Infinity` is NaN,
  // which made the comparator inconsistent when two models were unpriced.
  rows.sort((a, b) =>
    a.cost === null || b.cost === null ? (a.cost === null) - (b.cost === null) : b.cost - a.cost,
  );
  const unpriced = [...new Set(rows.filter((r) => r.cost === null).map((r) => r.model))];
  const total = rows.reduce((s, r) => s + (r.cost ?? 0), 0);
  const byRoot = new Map();
  for (const r of rows) {
    if (r.cost === null) continue;
    const root = r.role.split(" › ")[0];
    byRoot.set(root, (byRoot.get(root) ?? 0) + r.cost);
  }
  const rollup = [...byRoot.entries()].sort((a, b) => b[1] - a[1]);
  // Per top-level role: the dollar split and the average context per CALL (every token the call
  // sent: input + cache writes + cache reads). Startup context is one call's worth; this is the
  // whole life of the agent, which is what accumulated reads and gate logs inflate. Priced rows only,
  // so the split and the rollup above sum to the same total.
  const splitMap = new Map();
  for (const r of rows) {
    if (r.cost === null) continue;
    const root = r.role.split(" › ")[0];
    const s = splitMap.get(root) ?? {
      role: root,
      calls: 0,
      ctx: 0,
      input: 0,
      cacheWrite: 0,
      cacheRead: 0,
      output: 0,
      cost: 0,
    };
    s.calls += r.calls;
    s.ctx += r.input + r.cacheWrite1h + r.cacheWrite5m + r.cacheRead;
    for (const f of ["input", "cacheWrite", "cacheRead", "output"]) s[f] += r.parts[f];
    s.cost += r.cost;
    splitMap.set(root, s);
  }
  const split = [...splitMap.values()]
    .map((s) => ({ ...s, avgCtx: Math.round(s.ctx / s.calls) }))
    .sort((a, b) => b.cost - a.cost);
  return { rows, unpriced, total, rollup, split };
}

const M = (n) => (n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : `${Math.round(n / 1e3)}k`);

export function render(subject, { rows, unpriced, total, rollup, split = [] }) {
  const out = [`Usage — ${subject}`];
  if (!rows.length) return `${out[0]}\nNo API calls recorded in this window.`;
  out.push(
    "",
    "| role | model | calls | agents | avg turns | avg start ctx | cache read | cache write | output | ~$ |",
    "|---|---|---|---|---|---|---|---|---|---|",
  );
  for (const r of rows) {
    const cells = [
      r.role,
      r.model,
      r.calls,
      r.agents || "—",
      r.avgTurns ?? "—",
      r.avgStartCtx === null ? "—" : M(r.avgStartCtx),
      M(r.cacheRead),
      M(r.cacheWrite1h + r.cacheWrite5m),
      M(r.output),
      r.cost === null ? "?" : r.cost.toFixed(2),
    ];
    out.push(`| ${cells.join(" | ")} |`);
  }
  out.push("", "By top-level role, children included:", "");
  for (const [root, cost] of rollup) out.push(`- ${root}: ~$${cost.toFixed(2)}`);
  out.push(
    "",
    "Where each top-level role's dollars go (priced rows; context = input + cache write + cache read):",
    "",
    "| role | calls | avg ctx/call | cache read $ | cache write $ | output $ | input $ | ~$ |",
    "|---|---|---|---|---|---|---|---|",
  );
  const $ = (n) => n.toFixed(2);
  for (const s of split) {
    const cells = [s.role, s.calls, M(s.avgCtx), $(s.cacheRead), $(s.cacheWrite), $(s.output)];
    out.push(`| ${[...cells, $(s.input), $(s.cost)].join(" | ")} |`);
  }
  out.push("", `Total ~$${total.toFixed(2)} (list-price API equivalent)`);
  if (unpriced.length) out.push(`UNPRICED (excluded from total): ${unpriced.join(", ")}`);
  return out.join("\n");
}

/** The transcript directory Claude Code keeps for a checkout. */
//
// Claude Code keys the directory by the directory a session was STARTED in. Two candidates, and
// neither is right every time (review of #373):
//   - `here`, this checkout. Right for a session started in it, including one started INSIDE a
//     worktree. Wrong for a subagent running in a worktree the main session made
//     (`.claude/worktrees/agent-…`): that directory does not exist, and a subagent read
//     today's cost as "not read" (2026-09-27).
//   - `main`, the main checkout (the parent of the git COMMON dir). Right for that subagent, whose
//     session was started in the main checkout. Wrong for a session started inside a worktree.
// So chooseProjectDir decides by EVIDENCE: with a session id, the candidate that holds
// `<id>.jsonl`; without one, `here` when it exists, else `main`. `here` comes first in both, so a
// session started inside a worktree keeps its own directory even when the main one also exists.
const slugDir = (p) =>
  join(homedir(), ".claude", "projects", resolve(p).replace(/[^A-Za-z0-9]/g, "-"));

//
// A THIRD CASE (#388, 2026-09-28): a session that ran `EnterWorktree` has its WHOLE transcript
// moved to that worktree's directory (`<main>--claude-worktrees-<name>`), so a subagent in another
// worktree found it in neither candidate. `others` is every worktree directory of this repo
// (repoProjectDirs), searched for the session id after `here` and `main`, never as a fallback.
/** Pure: which candidate directory to read. `exists` is injected so the rule is testable. */
export function chooseProjectDir({ here, main, others = [], sessionId, exists }) {
  const candidates = [here, main].filter(Boolean);
  if (sessionId) {
    const holder = [...candidates, ...others].find((d) => exists(join(d, `${sessionId}.jsonl`)));
    if (holder) return holder;
  }
  return candidates.find((d) => exists(d)) ?? here;
}

/**
 * Pure: this repo's transcript directories, from a listing of `~/.claude/projects`: the main
 * checkout's (`mainDir`) and every `<its name>--claude-worktrees-<name>` one. A directory whose
 * name merely STARTS with the main one (`-repo-other`) is another repo's and is left out.
 */
export function repoProjectDirs(mainDir, entries) {
  const base = basename(mainDir);
  const wt = `${base}--claude-worktrees-`;
  return [
    mainDir,
    ...entries
      .filter((e) => e.startsWith(wt))
      .sort()
      .map((e) => join(dirname(mainDir), e)),
  ];
}

/** The main checkout's transcript directory, or null outside a git checkout (the CI container). */
function mainProjectDir() {
  const repo = fileURLToPath(new URL("..", import.meta.url));
  try {
    const common = execFileSync(
      "git",
      ["rev-parse", "--path-format=absolute", "--git-common-dir"],
      { cwd: repo, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] },
    ).trim();
    return slugDir(dirname(common));
  } catch {
    return null;
  }
}

/** Every existing transcript directory of this repo (repoProjectDirs over the real listing). */
function allProjectDirs() {
  const main = mainProjectDir();
  if (!main) return [];
  let entries = [];
  try {
    entries = readdirSync(dirname(main));
  } catch {
    return [];
  }
  return repoProjectDirs(main, entries).filter((d) => existsSync(d));
}

function projectDir(sessionId) {
  const repo = fileURLToPath(new URL("..", import.meta.url));
  const main = mainProjectDir();
  const others = main ? allProjectDirs().filter((d) => d !== main) : [];
  return chooseProjectDir({ here: slugDir(repo), main, others, sessionId, exists: existsSync });
}

// ---------------------------------------------------------------------------------------------
// CONTEXT ATTRIBUTION (prototype of `--contributors`, 2026-10-15). WHERE does the context come from?
//
// Each API call reports its whole context (input + cache read + cache write). Between two calls of
// one thread, the context grows by what arrived in between: tool results, skill bodies, reminders,
// user and peer messages, and the previous call's own output. The growth (the DELTA) is measured,
// not estimated; it is split across what arrived by character length (an image counts as ~1,600
// tokens, the previous call's output by its output_tokens), so every attributed token is a real
// token the API counted. An item then costs its cache write once and a cache READ on every later
// call of the same thread: downstream $ = tokens × Σ cacheRead price of each later call.
//
// Degrades to a plausible number, never an error, in these ways — each is surfaced, not hidden:
//   - a negative delta (compaction, cleared tool results) resets the baseline: the items in that gap
//     get their char estimate and the rest is "Post-compaction base"; the count is reported;
//   - thinking dropped between turns makes the previous output over-claim a share (calibration
//     ratio estimated/measured is reported);
//   - a model with no price makes downstream $ null (UNPRICED), never 0.
// ---------------------------------------------------------------------------------------------

// Fitted 2026-10-15 over 17,905 in-window deltas: delta ≈ 0.86·prev output + 0.41·chars + ~400/call
// (least squares), i.e. ~2.44 chars per token for this repo's code, logs and prose — not the 3.6
// a prose rule of thumb gives. A delta after a call with no items is ≈1.05× its output_tokens, so
// the previous call's output (thinking included) is carried whole.
export const CHARS_PER_TOKEN = 2.44;
const IMAGE_CHARS = 1600 * CHARS_PER_TOKEN;

export function normPath(p = "") {
  return String(p)
    .replace(/^\/Users\/[^/]+\/Development\/standingtee\/(\.claude\/worktrees\/[^/]+\/)?/, "")
    .replace(/^\/(private\/)?tmp\/claude-\d+\/[^/]+\/[^/]+\/scratchpad\//, "<scratchpad>/")
    .replace(/^\/Users\/[^/]+\/\.claude\//, "~/.claude/")
    .replace(/^\/Users\/[^/]+\//, "~/");
}

const FILE_VIEWERS = new Set(["cat", "sed", "head", "tail", "less", "nl", "wc"]);
// One key per COMMAND KIND: the script for a script, `gh pr view`, `git diff`, `pnpm test`. A file
// viewer keeps its file, since "which file was cat'd" is the actionable part.
export function normBash(cmd = "") {
  let s = String(cmd)
    .split("\n")
    .map((l) => l.trim())
    .filter((l) => l && !l.startsWith("#"))
    .join(" ; ");
  for (let i = 0; i < 6; i++) {
    const before = s;
    s = s
      .replace(/^cd\s+\S+\s*(&&|;)\s*/, "")
      .replace(/^[A-Z_][A-Z0-9_]*=("[^"]*"|'[^']*'|\S*)\s+/, "")
      .replace(/^(export\s+[A-Z_][A-Z0-9_]*=\S*\s*(&&|;)\s*)/, "")
      .replace(/^(timeout|time|env|command)\s+(\d+[sm]?\s+)?/, "")
      .replace(/^(set\s+-\S+\s*;\s*)/, "")
      .replace(/^\(\s*/, "");
    if (s === before) break;
  }
  const tok = s.split(/\s+/).filter(Boolean);
  let [a, ...rest] = tok;
  if (!a) return "(empty)";
  if (["bash", "sh", "zsh", "node"].includes(a) && rest[0] && !rest[0].startsWith("-"))
    return normPath(rest[0].replace(/^["']|["']$/g, "")).replace(/^\.\//, "");
  if (/[/.](sh|mjs|js|py)$|\/bin\//.test(a) || a.startsWith("./") || a.includes("/"))
    return normPath(a.replace(/^["']|["']$/g, "")).replace(/^\.\//, "");
  const words = rest.filter((w) => !w.startsWith("-"));
  if (a === "gh") return `gh ${words.slice(0, a === "gh" && words[0] === "api" ? 1 : 2).join(" ")}`;
  if (a === "git") {
    const r = rest.slice();
    while (r[0] === "-C" || r[0] === "-c") r.splice(0, 2);
    return `git ${r.find((w) => !w.startsWith("-")) ?? ""}`.trim();
  }
  if (a === "pnpm" || a === "npx") {
    const r = rest.slice();
    const out = [];
    for (let i = 0; i < r.length && out.length < 1; i++) {
      if (["-C", "--filter", "--dir", "-F"].includes(r[i])) i++;
      else if (r[i].startsWith("-") || r[i] === "exec" || r[i] === "run" || r[i] === "-r") continue;
      else out.push(r[i]);
    }
    return `${a} ${out.join(" ")}`.trim();
  }
  if (FILE_VIEWERS.has(a)) {
    const file = [...rest].reverse().find((w) => /[/.]\w/.test(w) && !w.startsWith("-") && !/^['"]?\d/.test(w));
    return `${a} ${file ? normPath(file.replace(/^["']|["']$/g, "")) : "(stdin)"}`;
  }
  return a;
}

function lenOf(content) {
  if (content == null) return 0;
  if (typeof content === "string") return content.length;
  if (Array.isArray(content))
    return content.reduce((s, b) => s + (b?.type === "image" ? IMAGE_CHARS : lenOf(b?.text ?? b?.content ?? "")), 0);
  return JSON.stringify(content).length;
}
function textOf(content) {
  if (typeof content === "string") return content;
  if (Array.isArray(content)) return content.map((b) => (typeof b?.text === "string" ? b.text : typeof b?.content === "string" ? b.content : "")).join("\n");
  return "";
}

export const CATEGORY = (key) => {
  const k = key.split(" ")[0];
  if (k === "Read") return "Read";
  if (k === "Bash") return "Bash";
  if (k === "Agent") return "Agent";
  if (k === "Skill") return "Skill";
  if (["WebFetch", "WebSearch"].includes(k)) return "web";
  if (k.startsWith("Artifact")) return "Artifact";
  if (k === "Reminder" || k === "Hook") return "reminders/hooks";
  if (k === "User" || k === "Slash") return "user";
  if (k === "Message") return "peer/agent messages";
  if (k === "Own") return "own output";
  if (k === "Startup") return "startup";
  if (k === "Post-compaction") return "compaction";
  if (k === "MCP") return "MCP";
  return "other";
};

// The key a tool_result is filed under, from the tool_use that asked for it.
function keyForTool(tu, resultText) {
  if (!tu) return "Other tool (unmatched)";
  const n = tu.name;
  const i = tu.input ?? {};
  if (n === "Read") return `Read ${normPath(i.file_path)}, ${i.offset != null || i.limit != null ? "range" : "whole"}`;
  if (n === "Bash") return `Bash ${normBash(i.command)}`;
  if (n === "Agent" || n === "Task") {
    if (/^Async agent launched|agentId: \w+ \(internal|(launched|Spawned) successfully/.test(resultText)) return "Agent launch ack";
    return `Agent ${i.subagent_type || "general-purpose"} result`;
  }
  if (n === "Skill") return "Skill launch ack";
  if (n.startsWith("mcp__")) return `MCP ${n.split("__")[1]}`;
  if (n === "Artifact") return `Artifact ${i.action ?? "publish"}`;
  return n;
}

function userKey(e, text, toolUses, agentTypes) {
  if (e.isCompactSummary) return "Post-compaction summary";
  if (/^Base directory for this skill: (\S+)/.test(text)) return `Skill ${text.match(/skills\/([^\s/]+)/)?.[1] ?? "?"} body`;
  if (e.sourceToolUseID && toolUses.get(e.sourceToolUseID)?.name === "Skill")
    return `Skill ${toolUses.get(e.sourceToolUseID).input?.skill ?? "?"} body`;
  if (/^<command-name>|<command-message>/.test(text)) return `Slash ${text.match(/<command-name>\/?([^<]+)/)?.[1] ?? "?"}`;
  if (/<local-command-stdout>/.test(text)) return "User local-command output";
  if (/cross-session-message/.test(text)) return "Message cross-session";
  if (/<teammate-message/.test(text)) return "Message teammate";
  if (/<task-notification>/.test(text)) {
    if (/Stop hook/.test(text)) return "Hook Stop feedback";
    const id = text.match(/<task-id>([\w-]+)<\/task-id>/)?.[1];
    if (id && agentTypes.has(id)) return `Agent ${agentTypes.get(id)} result (async)`;
    if (/<tool-use-id>|shell|Background command|bash/i.test(text) && !/agent/i.test(text.slice(0, 300))) return "Bash background-task notice";
    return "Agent result (async, type unknown)";
  }
  if (e.isMeta) return "Reminder meta-user text";
  return "User prompt";
}

const SKIP_ATTACH = new Set(["prompt_snapshot", "deferred_tools_record", "command_permissions", "credential_org", "thinking_drop"]);

/**
 * Walk ONE thread (a main session or one agent) in file order and attribute each call's context
 * growth to what arrived since the previous call. `agentTypes` maps async agent ids to a role label.
 */
export function attributeThread(file, { agentTypes = new Map() } = {}) {
  const lines = readFileSync(file, "utf8").split("\n");
  let parseFailures = 0;
  const calls = []; // { id, ts, model, speed, usage(last), items, outText, toolUses: [] }
  const byId = new Map();
  const toolUses = new Map();
  let pending = [];
  const branches = new Map();
  let firstPrompt = null;
  for (const line of lines) {
    if (!line.trim()) continue;
    let e;
    try {
      e = JSON.parse(line);
    } catch {
      parseFailures++;
      continue;
    }
    if (e.gitBranch) branches.set(e.gitBranch, (branches.get(e.gitBranch) ?? 0) + 1);
    const m = e.message;
    if (e.type === "assistant" && m?.model && m.model !== "<synthetic>" && m.usage) {
      const id = m.id ?? `${file}:${calls.length}`;
      let c = byId.get(id);
      if (!c) {
        c = { id, ts: e.timestamp ?? "", model: m.model, speed: m.usage.speed, items: pending, out: [], toolUses: [] };
        pending = [];
        byId.set(id, c);
        calls.push(c);
      }
      c.usage = m.usage; // last line wins, as in readTranscript
      for (const b of Array.isArray(m.content) ? m.content : []) {
        if (b.type === "text") c.out.push(b.text);
        if (b.type === "tool_use") {
          toolUses.set(b.id, b);
          c.toolUses.push(b);
          for (const v of Object.values(b.input ?? {})) if (typeof v === "string") c.out.push(v);
        }
      }
      continue;
    }
    if (e.type === "user" && m) {
      const content = m.content;
      if (Array.isArray(content) && content.some((b) => b.type === "tool_result")) {
        for (const b of content) {
          if (b.type !== "tool_result") continue;
          const tu = toolUses.get(b.tool_use_id);
          const text = textOf(b.content);
          pending.push({ key: keyForTool(tu, text), chars: lenOf(b.content), text, tu, ts: e.timestamp });
        }
        continue;
      }
      const text = textOf(content);
      const key = userKey(e, text, toolUses, agentTypes);
      if (key === "User prompt" && !firstPrompt) firstPrompt = text.slice(0, 400);
      // origin/promptSource: who typed it (human, task-notification, peer); driver-context audit (3-measure)
      pending.push({ key, chars: lenOf(content), text, ts: e.timestamp, origin: e.origin?.kind ?? null, src: e.promptSource ?? null, meta: !!e.isMeta });
      continue;
    }
    if (e.type === "attachment" && e.attachment && !SKIP_ATTACH.has(e.attachment.type) && e.rendered !== undefined) {
      const t = e.attachment.type;
      const key = t.startsWith("hook_") ? `Hook ${e.attachment.hookName ?? t}` : t === "instructions" ? "Startup CLAUDE.md/AGENTS.md" : `Reminder ${t}`;
      pending.push({ key, chars: JSON.stringify(e.rendered).length, ts: e.timestamp });
    }
  }
  return { calls, parseFailures, branches, firstPrompt, trailing: pending };
}

const tok = (u) =>
  (u.input_tokens ?? 0) + (u.cache_read_input_tokens ?? 0) + (u.cache_creation_input_tokens ?? 0);

/**
 * Turn a walked thread into attributed items. `inWindow(ts)` bounds both the items counted and the
 * later calls that re-read them. Returns items with tokens, later-call count and $.
 */
export function attributeItems(thread, inWindow, priceOf) {
  const { calls } = thread;
  // suffix sum of cacheRead $/token over in-window calls
  const suffix = new Array(calls.length + 1).fill(0);
  const later = new Array(calls.length + 1).fill(0);
  let unpricedLater = false;
  for (let i = calls.length - 1; i >= 0; i--) {
    const p = priceOf(calls[i]);
    const w = inWindow(calls[i].ts);
    suffix[i] = suffix[i + 1] + (w && p ? p.cacheRead / 1e6 : 0);
    later[i] = later[i + 1] + (w ? 1 : 0);
    if (w && !p) unpricedLater = true;
  }
  const items = [];
  let resets = 0;
  let estSum = 0;
  let measSum = 0;
  for (let i = 0; i < calls.length; i++) {
    const c = calls[i];
    const ctxNow = tok(c.usage);
    const prev = calls[i - 1];
    const delta = prev ? ctxNow - tok(prev.usage) : ctxNow;
    const parts = c.items.map((it) => ({ ...it, est: it.chars / CHARS_PER_TOKEN }));
    let reset = false;
    if (prev) parts.unshift({ key: "Own output (previous call)", est: prev.usage.output_tokens ?? 0 });
    const est = parts.reduce((s, p) => s + p.est, 0);
    let scale;
    let remainderKey = null;
    if (!prev || delta < 0) {
      // first call or a reset: items keep their estimate (capped at ctx), the rest is the base
      reset = !!prev;
      if (reset) {
        resets++;
        parts.shift(); // own output does not survive a compaction as itself
      }
      const e2 = parts.reduce((s, p) => s + p.est, 0);
      scale = e2 > ctxNow ? ctxNow / e2 : 1;
      remainderKey = reset ? "Post-compaction base" : "Startup system prompt + tools";
    } else {
      scale = est > 0 ? delta / est : 0;
      estSum += est;
      measSum += delta;
    }
    if (!inWindow(c.ts)) continue;
    const p = priceOf(c);
    const writeRate = p
      ? (() => {
          const u = c.usage;
          const w = u.cache_creation_input_tokens ?? 0;
          const w1 = u.cache_creation?.ephemeral_1h_input_tokens ?? 0;
          return w ? (p.input * (1.25 * (w - w1) + 2 * w1)) / w / 1e6 : (p.input * 1.25) / 1e6;
        })()
      : null;
    let assigned = 0;
    const push = (key, tokens, it = {}) => {
      if (tokens <= 0) return;
      assigned += tokens;
      items.push({
        call: i,
        key,
        tokens,
        later: later[i + 1],
        downstream: unpricedLater ? null : tokens * suffix[i + 1],
        write: writeRate == null ? null : tokens * writeRate,
        chars: it.chars ?? 0,
        origin: it.origin, src: it.src, meta: it.meta,
        text: it.text,
        tu: it.tu,
        ts: c.ts,
      });
    };
    for (const pt of parts) push(pt.key, pt.est * scale, pt);
    if (remainderKey) push(remainderKey, ctxNow - assigned);
    else if (est === 0 && delta > 0) push("Other unattributed growth", delta);
    // (a reset with no items left is still the base)
  }
  return { items, resets, calibration: measSum ? estSum / measSum : null };
}

/**
 * `--contributors`: the top context contributors across the given threads, by normalized key, plus
 * the cache rewrites (a call whose cache write is over half its context, after the first call: the
 * cache expired while the thread waited). Every number names its subject in the header.
 */
export function contributors(files, { since, until } = {}) {
  const inWindow = (ts) => !!ts && (!since || ts >= since) && (!until || ts < until);
  const priceOf = (c) => priceFor(c.model, c.speed);
  const byKey = new Map();
  let threads = 0;
  let resets = 0;
  let parseFailures = 0;
  const rewrites = { n: 0, cost: 0, unpriced: 0, waits: new Map() };
  for (const { file } of files) {
    const t = attributeThread(file);
    parseFailures += t.parseFailures;
    if (!t.calls.some((c) => inWindow(c.ts))) continue;
    threads++;
    const { items, resets: r } = attributeItems(t, inWindow, priceOf);
    resets += r;
    for (const it of items) {
      const g = byKey.get(it.key) ?? { key: it.key, n: 0, tokens: 0, downstream: 0, unpriced: false };
      g.n++;
      g.tokens += it.tokens;
      if (it.downstream == null) g.unpriced = true;
      else g.downstream += it.downstream;
      byKey.set(it.key, g);
    }
    t.calls.forEach((c, i) => {
      const prev = t.calls[i - 1];
      if (!prev || !inWindow(c.ts)) return;
      const total = tok(c.usage);
      const w = c.usage.cache_creation_input_tokens ?? 0;
      if (total < tok(prev.usage) || w <= 0.5 * total) return;
      rewrites.n++;
      const p = priceOf(c);
      const w1 = c.usage.cache_creation?.ephemeral_1h_input_tokens ?? 0;
      if (p) rewrites.cost += ((w - w1) * p.input * 1.25 + w1 * p.input * 2) / 1e6;
      else rewrites.unpriced++;
      const last = prev.toolUses.at(-1);
      const wait = last ? (last.name === "Bash" ? `Bash ${normBash(last.input?.command)}` : last.name) : "turn end";
      rewrites.waits.set(wait, (rewrites.waits.get(wait) ?? 0) + 1);
    });
  }
  const rows = [...byKey.values()].sort((a, b) => b.downstream - a.downstream);
  return { threads, files: files.length, resets, parseFailures, rows, rewrites };
}

export function renderContributors(subject, { threads, files, resets, parseFailures, rows, rewrites }, top = 10) {
  const out = [`Top context contributors — ${subject} (${threads} of ${files} threads active)`];
  if (!rows.length) return `${out[0]}\nNo attributed context in this window.`;
  out.push("", "| contributor | times | tokens added | mean | re-read ~$ |", "|---|---|---|---|---|");
  for (const r of rows.slice(0, top))
    out.push(`| ${r.key} | ${r.n} | ${M(r.tokens)} | ${r.tokens / r.n < 1000 ? Math.round(r.tokens / r.n) : M(r.tokens / r.n)} | ${r.unpriced ? "UNPRICED" : `$${r.downstream.toFixed(2)}`} |`);
  const waits = [...rewrites.waits.entries()].sort((a, b) => b[1] - a[1]).slice(0, 3).map(([k, v]) => `${k} ×${v}`);
  out.push(
    "",
    `Cache rewrites (write > 50% of context after an idle gap): ${rewrites.n}, ~$${rewrites.cost.toFixed(2)}${rewrites.unpriced ? ` + ${rewrites.unpriced} UNPRICED` : ""}${waits.length ? `; waiting on ${waits.join(", ")}` : ""}.`,
  );
  // Blind spots are stated, never shown as zero.
  if (resets) out.push(`${resets} context reset(s) (compaction): their base is one row, not attributed.`);
  if (parseFailures) out.push(`${parseFailures} transcript line(s) failed to parse and are not counted.`);
  return out.join("\n");
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  const opt = (name) => {
    const i = args.indexOf(name);
    return i >= 0 ? args[i + 1] : undefined;
  };
  const since = opt("--since");
  const session = opt("--session") ?? (since ? undefined : process.env.CLAUDE_CODE_SESSION_ID);
  // --dir may be given more than once, to sum several worktrees' transcript directories (#388);
  // each directory is read once however often it is named.
  const named = [
    ...new Set(
      args.flatMap((a, i) => (a === "--dir" && args[i + 1] ? [resolve(args[i + 1])] : [])),
    ),
  ];
  const dirs = named.length ? named : [projectDir(session)];
  for (const dir of dirs)
    if (!existsSync(dir)) {
      process.stderr.write(`usage-report: no transcript directory at ${dir}\n`);
      process.exit(1);
    }
  if (!session && !since) {
    process.stderr.write(
      "usage-report: no session to report — pass --session <id> or --since <date>, or run inside Claude Code (CLAUDE_CODE_SESSION_ID)\n",
    );
    process.exit(1);
  }
  // A session is read from the first directory that holds it: EnterWorktree MOVES a transcript,
  // so one id in two directories is not expected, and counting it twice would double its cost.
  const data = { sessions: [], records: [], agents: [] };
  for (const dir of dirs) {
    const d = collect(dir, { session, since, exclude: data.sessions });
    data.sessions.push(...d.sessions);
    data.records.push(...d.records);
    data.agents.push(...d.agents);
  }
  if (!data.sessions.length) {
    process.stderr.write(`usage-report: no session matched in ${dirs.join(", ")}\n`);
    process.exit(1);
  }
  const window = since ? `since ${since}` : "whole session";
  const who =
    data.sessions.length === 1 ? `session ${data.sessions[0]}` : `${data.sessions.length} sessions`;
  const where = dirs.length > 1 ? `, across ${dirs.length} transcript directories` : "";
  process.stdout.write(`${render(`${who}, ${window}${where}`, summarize(data))}\n`);
  if (args.includes("--contributors")) {
    const files = [];
    for (const dir of dirs)
      for (const s of data.sessions) {
        const main = join(dir, `${s}.jsonl`);
        if (!existsSync(main)) continue;
        files.push({ file: main, role: "session" });
        const sub = join(dir, s, "subagents");
        if (!existsSync(sub)) continue;
        const subFiles = transcriptsUnder(sub);
        const metas = new Map();
        for (const f of subFiles) {
          const meta = readMeta(f.replace(/\.jsonl$/, ".meta.json"));
          if (meta) metas.set(agentIdOf(f), meta);
        }
        for (const f of subFiles) files.push({ file: f, role: roleOf(agentIdOf(f), metas) });
      }
    const top = Number(opt("--top") ?? 10);
    process.stdout.write(`\n${renderContributors(`${who}, ${window}${where}`, contributors(files, { since }), top)}\n`);
  }
}
