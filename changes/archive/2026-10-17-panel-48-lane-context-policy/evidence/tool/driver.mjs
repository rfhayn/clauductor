// Driver-decided cost: what the app that launches and wakes sessions decides, separately from what
// the repo's own content decides. Reads calls.jsonl + items.jsonl written by audit.mjs (extended
// with call index, origin, nTools, head). Writes driver.json + CSVs into <dir>, prints markdown.
//   node driver.mjs <dir> [capClasses]
import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { priceFor } from "./usage-report.mjs";

const dir = process.argv[2];
const load = (f) => readFileSync(join(dir, f), "utf8").trim().split("\n").filter(Boolean).map((l) => JSON.parse(l));
const C = load("calls.jsonl");
const I = load("items.jsonl");
const subject = JSON.parse(readFileSync(join(dir, "subject.json"), "utf8"));
const out = [];
const p = (s = "") => out.push(s);
const $ = (n) => (n == null ? "UNPRICED" : `$${n.toFixed(2)}`);
const K = (n) => `${Math.round(n / 1000)}k`;
const pct = (a, b) => `${((100 * a) / b).toFixed(1)}%`;
const sum = (a, f) => a.reduce((s, x) => s + (f(x) ?? 0), 0);
const q = (arr, f) => { const s = [...arr].sort((a, b) => a - b); return s.length ? s[Math.min(s.length - 1, Math.floor(s.length * f))] : 0; };
const raw = {};

// ---------- classes ----------
const cls = (c) => (c.role === "session" ? (c.kind === "orchestrator" ? "orchestrator" : `lane:${c.kind.replace("worktree:", "")}`) : "subagent");
const top = (c) => (cls(c).startsWith("lane") ? "lane main" : cls(c));
function subGroup(role) {
  if (role.startsWith("workflow:")) return "workflow steps";
  if (role.startsWith("agent:builder")) return "builder";
  if (role === "agent:reviewer") return "reviewer";
  if (role.startsWith("agent:reviewer ›")) return "reviewer's finders";
  if (role === "agent:code-review" || role.endsWith("› code-review")) return "code-review (session-spawned)";
  return role.replace("agent:", "").split(" › ")[0];
}
const tkey = (c) => `${c.sid}|${c.thread}`;
const costT = (c) => (c.cost ? c.cost.input + c.cost.cacheWrite + c.cost.cacheRead + c.cost.output : 0);
const pr = (c) => priceFor(c.model.replace(/ \(fast\)$/, ""), c.model.endsWith("(fast)") ? "fast" : undefined);
const crP = (c) => (pr(c)?.cacheRead ?? 0) / 1e6;
const wRate = (c) => { const p0 = pr(c); if (!p0) return 0; const w = c.cw5 + c.cw1; return w ? c.cost.cacheWrite / w : (p0.input * (c.cw1 ? 2 : 1.25)) / 1e6; };
const isRewrite = (c) => c.i > 0 && c.ctx >= c.prevCtx && c.cw5 + c.cw1 > 0.5 * c.ctx && c.ctx > 20000;
// tokens written that were already context before this call (a cold cache), beyond the new delta
const rewriteTok = (c) => (c.i > 0 && c.ctx >= c.prevCtx ? Math.max(0, c.cw5 + c.cw1 - (c.ctx - c.prevCtx)) : 0);

const threads = new Map();
for (const c of C) (threads.get(tkey(c)) ?? threads.set(tkey(c), []).get(tkey(c))).push(c);
for (const a of threads.values()) a.sort((x, y) => x.i - y.i);
const itemsBy = new Map();
for (const it of I) { const k = `${it.sid}|${it.thread}|${it.call}`; (itemsBy.get(k) ?? itemsBy.set(k, []).get(k)).push(it); }
const itemsOf = (c) => itemsBy.get(`${c.sid}|${c.thread}|${c.i}`) ?? [];
// suffix sum of cache-read $/token over later in-window calls of the same thread
const suffix = new Map();
for (const [k, a] of threads) { const s = new Array(a.length + 1).fill(0); for (let j = a.length - 1; j >= 0; j--) s[j] = s[j + 1] + crP(a[j]); suffix.set(k, s); }

const priced = C.filter((c) => c.cost);
const TOTAL = sum(priced, costT);

// ---------- 0. headline ----------
p("## 0. Headline reproduction");
p();
const parts = { cacheRead: sum(priced, (c) => c.cost.cacheRead), cacheWrite: sum(priced, (c) => c.cost.cacheWrite), output: sum(priced, (c) => c.cost.output), input: sum(priced, (c) => c.cost.input) };
const unpricedCalls = C.filter((c) => !c.cost);
const sessions = new Set(C.map((c) => c.sid)).size;
p(`| measure | prototype (reported) | this run |`);
p(`|---|---|---|`);
p(`| sessions | 76 | ${sessions} (subject.json: ${subject.sessions}) |`);
p(`| subagents | 393 | ${subject.agents} |`);
p(`| calls | 18,378 | ${C.length.toLocaleString()} |`);
p(`| list price | $1,208 | ${$(TOTAL)} |`);
p(`| cache reads / writes / output / input | 49.6% / 36.4% / 14% / – | ${pct(parts.cacheRead, TOTAL)} / ${pct(parts.cacheWrite, TOTAL)} / ${pct(parts.output, TOTAL)} / ${pct(parts.input, TOTAL)} |`);
const byTop = {};
for (const c of priced) { const k = top(c) + (top(c) === "subagent" ? ` of ${c.kind === "orchestrator" ? "orchestrator" : "lanes"}` : ""); byTop[k] = (byTop[k] ?? 0) + costT(c); }
p(`| lane main threads | $543 | ${$(byTop["lane main"])} |`);
p(`| orchestrator + its subagents | $154 + $38 | ${$(byTop.orchestrator)} + ${$(byTop["subagent of orchestrator"])} |`);
const ownOut = I.filter((i) => i.key === "Own output (previous call)");
p(`| self re-read (own output, downstream) | $141/wk | ${$(sum(ownOut, (i) => i.downstream))} |`);
const rew = priced.filter(isRewrite);
p(`| cache rewrites (calls writing >50% of ctx) | $109 | ${$(sum(rew, (c) => c.cost.cacheWrite))} (${rew.length} calls) |`);
const orchStarts = C.filter((c) => c.kind === "orchestrator" && c.role === "session" && c.i === 0).map((c) => c.ctx);
p(`| orchestrator startup (first-call ctx, median) | ~61k | ${K(q(orchStarts, 0.5))} (n=${orchStarts.length}) |`);
p();
const unp = {};
for (const c of unpricedCalls) { const u = (unp[c.model] ??= { calls: 0, input: 0, cw5: 0, cw1: 0, cr: 0, out: 0 }); u.calls++; u.input += c.input; u.cw5 += c.cw5; u.cw1 += c.cw1; u.cr += c.cr; u.out += c.out; }
// Haiku 5.5 at the claude-api skill's documented rate (not in the tool's table): ≤100k prompt $0.10/$0.50, above $0.50/$2.50; cache read 0.1×, writes 1.25×/2×
const haikuP = (ctx) => (ctx <= 100000 ? { input: 0.1, output: 0.5, cacheRead: 0.01 } : { input: 0.5, output: 2.5, cacheRead: 0.05 });
const reprice = (c, P) => (c.input * P.input + c.cw5 * P.input * 1.25 + c.cw1 * P.input * 2 + c.cr * P.cacheRead + c.out * P.output) / 1e6;
for (const [m, u] of Object.entries(unp)) p(`UNPRICED in the headline: ${m}: ${u.calls} calls, input ${u.input}, cache write 5m ${u.cw5.toLocaleString()}, 1h ${u.cw1.toLocaleString()}, cache read ${u.cr.toLocaleString()}, output ${u.out.toLocaleString()}${m === "claude-haiku-5-5" ? ` — at the skill doc's Haiku 5.5 rate it would be ${$(sum(unpricedCalls.filter((c) => c.model === m), (c) => reprice(c, haikuP(c.ctx))))} (not added to any total)` : ""}.`);
raw.headline = { sessions, subagents: subject.agents, calls: C.length, total: TOTAL, parts, byTop, unpriced: unp };

// ---------- 1. lifetime ----------
p();
p("## 1. Lifetime: calls per thread, context per call, re-reading accumulated context");
p();
p("`accum re-read` = Σ calls (cache-read tokens − the thread's first-call context)⁺ × cache-read price: the re-reading of what accumulated after startup. A thread carried in from before the window uses its class's median first-call context as the base.");
p();
const classes = ["orchestrator", "lane:ops", "lane:fix", "lane:propose", "lane:build", "subagent"];
p("| class | threads | calls | calls/thread p50 / p90 / max | ctx/call mean / p50 / p90 | read $ | write $ | output $ | total $ | accum re-read $ |");
p("|---|---|---|---|---|---|---|---|---|---|");
raw.lifetime = {};
const startCtx = new Map();
const firstByCls = {};
for (const a of threads.values()) if (a[0].i === 0) (firstByCls[cls(a[0])] ??= []).push(a[0].ctx);
for (const [k, a] of threads) startCtx.set(k, a[0].i === 0 ? a[0].ctx : q(firstByCls[cls(a[0])] ?? [60000], 0.5));
for (const cl of [...classes, "ALL"]) {
  const cs = priced.filter((c) => cl === "ALL" || cls(c) === cl);
  const ts = [...new Set(cs.map(tkey))];
  const n = ts.map((k) => threads.get(k).length);
  const ctxs = cs.map((c) => c.ctx);
  const acc = sum(cs, (c) => Math.max(0, c.cr - startCtx.get(tkey(c))) * crP(c));
  const r = { threads: ts.length, calls: cs.length, read: sum(cs, (c) => c.cost.cacheRead), write: sum(cs, (c) => c.cost.cacheWrite), output: sum(cs, (c) => c.cost.output), total: sum(cs, costT), accum: acc };
  raw.lifetime[cl] = r;
  p(`| ${cl} | ${ts.length} | ${cs.length} | ${q(n, 0.5)} / ${q(n, 0.9)} / ${Math.max(...n)} | ${K(sum(ctxs, (x) => x) / ctxs.length)} / ${K(q(ctxs, 0.5))} / ${K(q(ctxs, 0.9))} | ${$(r.read)} | ${$(r.write)} | ${$(r.output)} | ${$(r.total)} | ${$(acc)} |`);
}
p();
p("**Counterfactual: auto-compaction cap.** When a thread's simulated context passes X, it compacts to S = (that thread's first-call context) + 30k. Assumptions, stated: the compaction call re-reads the whole context once (cache read) and emits 10k output tokens; the post-compaction context S is written fresh at the thread's own write rate (conservative: in practice the system+tools prefix stays cached); every later call reads (actual − removed) instead of actual; a later cache rewrite rewrites (actual − removed); a real compaction in the transcript resets the simulation. Quality loss from compaction (re-reading files it dropped) is NOT modelled.");
p();
p("| cap X | class | compactions | re-read saved | rewrite saved | compaction calls cost | net saving/wk | new read+write $ |");
p("|---|---|---|---|---|---|---|---|");
raw.caps = [];
function simCap(a, X, deltaStart = 0) {
  // returns { n, readSave, writeSave, cost } for one thread; deltaStart lowers every call's context first (lean startup)
  const S0 = Math.max(0, startCtx.get(tkey(a[0])) - deltaStart) + 30000;
  let removed = 0, n = 0, readSave = 0, writeSave = 0, cost = 0;
  for (let j = 0; j < a.length; j++) {
    const c = a[j];
    if (!c.cost) continue;
    if (j > 0 && c.ctx < a[j - 1].ctx) removed = 0;
    const sim = c.ctx - deltaStart - removed;
    readSave += Math.min(removed, c.cr) * crP(c);
    if (isRewrite(c)) writeSave += Math.min(removed, rewriteTok(c)) * wRate(c);
    if (sim > X) { n++; cost += sim * crP(c) + 10000 * (pr(c).output / 1e6) + S0 * wRate(c); removed += sim - S0; }
  }
  return { n, readSave, writeSave, cost };
}
for (const X of [150000, 200000, 300000]) {
  for (const cl of [...classes, "ALL"]) {
    const ts = [...threads.entries()].filter(([, a]) => a[0].cost && (cl === "ALL" || cls(a[0]) === cl));
    let n = 0, rs = 0, ws = 0, cc = 0;
    for (const [, a] of ts) { const r = simCap(a, X); n += r.n; rs += r.readSave; ws += r.writeSave; cc += r.cost; }
    const base = raw.lifetime[cl].read + raw.lifetime[cl].write;
    raw.caps.push({ X, cl, n, readSave: rs, writeSave: ws, cost: cc, net: rs + ws - cc });
    if (cl === "lane:build" || (n === 0 && cl !== "ALL")) continue;
    p(`| ${K(X)} | ${cl} | ${n} | ${$(rs)} | ${$(ws)} | ${$(cc)} | **${$(rs + ws - cc)}** | ${$(base)} → ${$(base - rs - ws + cc)} |`);
  }
}

// ---------- 2. wakes ----------
p();
p("## 2. Wakes: what starts a turn, and what each wake costs");
p();
const HUMAN_SRC = new Set(["typed", "queued", "suggestion_accepted"]);
function trigger(c) {
  if (c.i === 0) return "session start";
  const its = itemsOf(c).filter((i) => i.cat !== "own output");
  const has = (f) => its.some(f);
  if (has((i) => i.origin === "human" || HUMAN_SRC.has(i.src) || (i.key === "User prompt" && !i.origin && !i.meta && !/^This session is being continued/.test(i.head ?? "")) || /^<bash-input>/.test(i.head ?? ""))) return "human";
  if (has((i) => i.origin === "peer" || /^Another Claude session/.test(i.head ?? "") || i.key.startsWith("Message cross-session"))) return "peer session message";
  if (has((i) => i.key === "Message teammate")) return "teammate message";
  if (has((i) => /^\[Cross-session idle/.test(i.head ?? ""))) return "idle notice";
  if (has((i) => i.key === "Hook Stop feedback")) return "Stop hook";
  if (has((i) => i.key === "Bash background-task notice")) return "background Bash done";
  if (has((i) => /^Agent .*result/.test(i.key) && (i.origin === "task-notification" || /\(async/.test(i.key)))) return "subagent result";
  if (has((i) => i.origin === "task-notification")) return "other task notification";
  if (has((i) => i.origin === "coordinator")) return "coordinator";
  if (has((i) => i.key === "Post-compaction summary" || /^This session is being continued/.test(i.head ?? ""))) return "compaction continue";
  if (has((i) => i.key.startsWith("Hook SessionStart"))) return "restart (SessionStart)";
  if (has((i) => i.key.startsWith("Hook"))) return `hook (${its.find((i) => i.key.startsWith("Hook")).key})`;
  return `other (${its[0]?.key ?? "none"})`;
}
const wakeRows = [];
for (const [k, a] of threads) {
  if (a[0].role !== "session") continue;
  const starts = a.map((c, j) => (c.i === 0 || c.wait === "start" || c.wait.startsWith("turn end") ? j : -1)).filter((j) => j >= 0);
  for (let s = 0; s < starts.length; s++) {
    const j = starts[s], end = (starts[s + 1] ?? a.length) - 1, c = a[j];
    if (!c.cost) continue;
    const delta = Math.max(0, c.ctx - c.prevCtx);
    const fixed = c.i === 0 ? 0 : c.cr * crP(c) + Math.max(0, c.cw5 + c.cw1 - delta) * wRate(c);
    const turn = a.slice(j, end + 1);
    // content brought in during the turn, re-read by calls AFTER the turn ends
    const sfx = suffix.get(k);
    const turnItemsTok = sum(turn, (tc) => sum(itemsOf(tc), (i) => i.tokens));
    const knock = turnItemsTok * sfx[end + 1];
    wakeRows.push({ sid: c.sid, cls: cls(c), i: c.i, ts: c.ts, trig: trigger(c), ctx: c.ctx, gapSec: c.gapSec, cold: isRewrite(c), first: costT(c), fixed, turnCalls: turn.length, turnCost: sum(turn, costT), turnItemsTok, knock, j, end, k });
  }
}
raw.wakes = wakeRows;
const orchW = wakeRows.filter((w) => w.cls === "orchestrator");
const trigs = [...new Set(wakeRows.map((w) => w.trig))];
const tb = (rows) => {
  p("| trigger | wakes | ctx at wake p50 / p90 | cold (rewrite) | first-call $ | fixed $ (re-read + rewrite of carried ctx) | whole-turn $ | turn content re-read after the turn $ |");
  p("|---|---|---|---|---|---|---|---|");
  const g = trigs.map((t) => [t, rows.filter((w) => w.trig === t)]).filter(([, r]) => r.length).sort((x, y) => sum(y[1], (w) => w.first) - sum(x[1], (w) => w.first));
  for (const [t, r] of [...g, ["ALL", rows]]) {
    const cx = r.map((w) => w.ctx);
    p(`| ${t} | ${r.length} | ${K(q(cx, 0.5))} / ${K(q(cx, 0.9))} | ${r.filter((w) => w.cold).length} | ${$(sum(r, (w) => w.first))} | ${$(sum(r, (w) => w.fixed))} | ${$(sum(r, (w) => w.turnCost))} | ${$(sum(r, (w) => w.knock))} |`);
  }
};
p("### Orchestrator main threads");
p();
tb(orchW);
p();
const orchHuman = orchW.filter((w) => w.trig === "human"), orchMachine = orchW.filter((w) => !["human", "session start"].includes(w.trig));
const statusQs = I.filter((i) => i.kind === "orchestrator" && i.role === "session" && i.origin === "human" && /status update|status\?/i.test(i.head ?? "")).length;
p(`Human-typed turn starts ${orchHuman.length}; machine turn starts (not typed, not session start) ${orchMachine.length} (${pct(orchMachine.length, orchHuman.length + orchMachine.length)}). Of the human prompts, ${statusQs} begin "can I get a status update?" or similar: the owner asking for what nothing showed them.`);
p();
p("### Lane main threads (for comparison; a lane's first prompt is typed by the panel, which the transcript records as `human`)");
p();
tb(wakeRows.filter((w) => w.cls.startsWith("lane")));
p();
p("**Counterfactuals (orchestrator, machine wakes only).** `fixed` = the cost a wake pays just to resume: cache-read of the context it already had, plus the rewrite of that context when the cache had gone cold. New content's write and the output are paid in every scenario.");
p();
const fixedM = sum(orchMachine, (w) => w.fixed);
p("| scenario | arithmetic | saving/wk |");
p("|---|---|---|");
raw.wakeCF = {};
for (const N of [2, 3, 5]) { const s = (1 - 1 / N) * fixedM; raw.wakeCF[`batch${N}`] = s; p(`| batch ${N} outcomes per wake | (1 − 1/${N}) × ${$(fixedM)} fixed over ${orchMachine.length} machine wakes | **${$(s)}** |`); }
// light routing session: machine wakes land in a ~60k session; the orchestrator gets a 1k digest per wake instead of the turn's content
const R = 60000;
let rFirst = 0, rTurn = 0, rKnock = 0;
for (const w of orchMachine) {
  const a = threads.get(w.k), c = a[w.j], shift = Math.max(0, c.ctx - R);
  rFirst += Math.min(shift, c.cr) * crP(c) + (w.cold ? Math.min(shift, rewriteTok(c)) * wRate(c) : 0);
  for (let j = w.j; j <= w.end; j++) { const tc = a[j]; if (!tc.cost) continue; rTurn += Math.min(shift, tc.cr) * crP(tc) + (isRewrite(tc) ? Math.min(shift, rewriteTok(tc)) * wRate(tc) : 0); }
  rKnock += Math.max(0, w.turnItemsTok - 1000) * suffix.get(w.k)[w.end + 1];
}
raw.wakeCF.router = { rFirst, rTurn, rKnock };
p(`| a ~60k routing session takes machine wakes: first call only | Σ (ctx at wake − 60k)⁺ × read price (+ the same off any cold rewrite) | ${$(rFirst)} |`);
p(`| … the whole machine-woken turn runs in the router | every call of the turn shifted down by (ctx at wake − 60k)⁺ | **${$(rTurn)}** |`);
p(`| … and the orchestrator receives a 1k digest instead of the turn's content | (turn content tokens − 1k) × read price of every later orchestrator call | +${$(rKnock)} |`);

// ---------- 3. startup ----------
p();
p("## 3. Startup: the first call of every thread");
p();
p("Split of the first call's context by what the transcript attributes to it (estimated from characters at 2.44 chars/token, scaled to the measured context; `system prompt + tools` is the remainder). The task prompt (the user's or the parent's prompt) is the work, not startup, and is excluded from the lean-profile arithmetic.");
p();
const TASK = (i) => ["user", "peer/agent messages"].includes(i.cat) || /^Slash|^Skill .* body|Reminder meta-user text/.test(i.key);
const sKey = (i) => (TASK(i) ? "task prompt" : i.key.startsWith("Hook") ? "hooks (repo)" : i.key.startsWith("Reminder") ? i.key.replace("Reminder ", "") : i.key.replace("Startup ", ""));
const st = [];
for (const [k, a] of threads) {
  const c = a[0];
  if (c.i !== 0 || !c.cost) continue;
  const its = itemsOf(c);
  const split = {};
  for (const i of its) split[sKey(i)] = (split[sKey(i)] ?? 0) + i.tokens;
  const task = split["task prompt"] ?? 0;
  const startupTok = c.ctx - task;
  const inCost = c.cost.cacheRead + c.cost.cacheWrite + c.cost.input;
  const firstShare = (startupTok / c.ctx) * inCost;
  const reRead = startupTok * suffix.get(k)[1];
  const rewr = sum(a.slice(1).filter(isRewrite), (x) => Math.min(startupTok, rewriteTok(x)) * wRate(x));
  st.push({ k, cls: cls(c), sub: c.role === "session" ? cls(c) : subGroup(c.role), ctx: c.ctx, startupTok, split, firstShare, reRead, rewr, crFirst: c.cr, a });
}
raw.startup = st.map(({ a, ...r }) => r);
const keysAll = {};
for (const s of st) for (const [kk, v] of Object.entries(s.split)) keysAll[kk] = (keysAll[kk] ?? 0) + v;
const keyList = Object.entries(keysAll).sort((x, y) => y[1] - x[1]).map(([kk]) => kk).slice(0, 9);
p(`| class | threads | first-call ctx p50 | startup tok p50 | ${keyList.map((x) => `${x} (mean)`).join(" | ")} |`);
p(`|---|---|---|---|${keyList.map(() => "---").join("|")}|`);
for (const cl of [...classes, "ALL"]) {
  const r = st.filter((s) => cl === "ALL" || s.cls === cl);
  if (!r.length) continue;
  p(`| ${cl} | ${r.length} | ${K(q(r.map((s) => s.ctx), 0.5))} | ${K(q(r.map((s) => s.startupTok), 0.5))} | ${keyList.map((kk) => K(sum(r, (s) => s.split[kk] ?? 0) / r.length)).join(" | ")} |`);
}
p();
p("Startup cost = the first call's input-side cost × (startup ÷ context) + startup tokens × the cache-read price of every later in-window call of that thread + the startup part of each later cold rewrite. Lean profile: startup capped at 25k, saving = (startup − 25k)⁺ in each of those three terms.");
p();
p("| class | threads | startup tok total | first call $ | re-read $ | in rewrites $ | startup total $ | lean 25k saving/wk |");
p("|---|---|---|---|---|---|---|---|");
raw.lean = {};
for (const cl of [...classes, "ALL"]) {
  const r = st.filter((s) => cl === "ALL" || s.cls === cl);
  if (!r.length) continue;
  const tot = sum(r, (s) => s.firstShare + s.reRead + s.rewr);
  const lean = sum(r, (s) => { const d = Math.max(0, s.startupTok - 25000); return s.startupTok ? (d / s.startupTok) * (s.firstShare + s.reRead + s.rewr) : 0; });
  raw.lean[cl] = { tot, lean };
  p(`| ${cl} | ${r.length} | ${K(sum(r, (s) => s.startupTok))} | ${$(sum(r, (s) => s.firstShare))} | ${$(sum(r, (s) => s.reRead))} | ${$(sum(r, (s) => s.rewr))} | ${$(tot)} | **${$(lean)}** |`);
}
p();
p("Subagent startup by role:");
p();
p("| role | threads | startup p50 | startup total $ | lean 25k saving |");
p("|---|---|---|---|---|");
const subs = [...new Set(st.filter((s) => s.cls === "subagent").map((s) => s.sub))];
for (const g of subs.map((g) => [g, st.filter((s) => s.sub === g)]).sort((x, y) => sum(y[1], (s) => s.firstShare + s.reRead + s.rewr) - sum(x[1], (s) => s.firstShare + s.reRead + s.rewr)).slice(0, 10)) {
  const [name, r] = g;
  p(`| ${name} | ${r.length} | ${K(q(r.map((s) => s.startupTok), 0.5))} | ${$(sum(r, (s) => s.firstShare + s.reRead + s.rewr))} | ${$(sum(r, (s) => { const d = Math.max(0, s.startupTok - 25000); return s.startupTok ? (d / s.startupTok) * (s.firstShare + s.reRead + s.rewr) : 0; }))} |`);
}
p();
p(`First calls that were mostly a cache READ (a shared prefix warm from another thread): ${st.filter((s) => s.crFirst > 0.5 * s.ctx).length} of ${st.length}.`);

// ---------- 4. idle gaps vs TTL ----------
p();
p("## 4. Idle gaps against the cache TTL");
p();
p("A thread's TTL is 1h if any of its calls wrote `ephemeral_1h` tokens, else 5m. A gap is the time from the previous call's response to this call. Cold = the call rewrote over half its context (the prototype's rewrite rule). `paid` = rewritten-carried tokens × the call's write rate; `if warm` = the same tokens × cache-read price.");
p();
const ttlOf = new Map();
for (const [k, a] of threads) ttlOf.set(k, a.some((c) => c.cw1 > 0) ? 3600 : 300);
const gapRows = [];
for (const [k, a] of threads) for (const c of a) if (c.i > 0 && c.cost && c.gapSec != null) gapRows.push({ c, ttl: ttlOf.get(k), over: c.gapSec > ttlOf.get(k), cold: isRewrite(c) });
p("| class | TTL | gaps | gaps > TTL | of those cold | cold with gap ≤ TTL | rewrite paid $ | same tokens if warm $ | cost of going cold $ |");
p("|---|---|---|---|---|---|---|---|---|");
raw.gaps = {};
for (const cl of [...classes, "ALL"]) for (const ttl of [300, 3600]) {
  const r = gapRows.filter((g) => (cl === "ALL" || cls(g.c) === cl) && g.ttl === ttl);
  if (!r.length) continue;
  const cold = r.filter((g) => g.cold);
  const paid = sum(cold, (g) => rewriteTok(g.c) * wRate(g.c)), warm = sum(cold, (g) => rewriteTok(g.c) * crP(g.c));
  raw.gaps[`${cl}|${ttl}`] = { gaps: r.length, over: r.filter((g) => g.over).length, paid, warm };
  p(`| ${cl} | ${ttl === 300 ? "5m" : "1h"} | ${r.length} | ${r.filter((g) => g.over).length} | ${r.filter((g) => g.over && g.cold).length} | ${r.filter((g) => !g.over && g.cold).length} | ${$(paid)} | ${$(warm)} | **${$(paid - warm)}** |`);
}
p();
const norm = (w) => {
  const t = w.match(/^turn end → (Message|Agent|Hook|User|Reminder|Bash background)/);
  if (t) return `turn end, woken by: ${t[1]}`;
  if (/^Bash (\S+\/)?(gate-wait|gate|run-local|gate-lock)\.sh/.test(w)) return "foreground Bash: the gate (gate.sh / gate-wait.sh)";
  if (/^Bash (pnpm|npx) /.test(w)) return "foreground Bash: tests/format (pnpm, npx)";
  if (w.startsWith("Bash ")) return "foreground Bash: other";
  if (w.startsWith("Agent ")) return "foreground Agent";
  return w;
};
const coldAll = gapRows.filter((g) => g.cold);
const byWait = {};
for (const g of coldAll) { const w = norm(g.c.wait); const x = (byWait[w] ??= { n: 0, paid: 0, warm: 0 }); x.n++; x.paid += rewriteTok(g.c) * wRate(g.c); x.warm += rewriteTok(g.c) * crP(g.c); }
p("Cold calls by what the thread was waiting on (the driver decides most of these waits):");
p();
p("| waiting on | cold calls | paid $ | if warm $ |");
p("|---|---|---|---|");
for (const [w, x] of Object.entries(byWait).sort((x, y) => y[1].paid - x[1].paid)) p(`| ${w} | ${x.n} | ${$(x.paid)} | ${$(x.warm)} |`);
p();
// counterfactual A: 5m threads switch to 1h TTL (writes 2× instead of 1.25×; cold calls with gap ≤ 1h become warm)
const g5 = gapRows.filter((g) => g.ttl === 300);
const t5 = [...threads.entries()].filter(([k]) => ttlOf.get(k) === 300).flatMap(([, a]) => a).filter((c) => c.cost);
const extra1h = sum(t5, (c) => (c.cw5 * (pr(c).input * 0.75)) / 1e6);
const saved1h = sum(g5.filter((g) => g.cold && g.c.gapSec <= 3600), (g) => rewriteTok(g.c) * (wRate(g.c) - crP(g.c)));
// counterfactual B: keep-alive — a max_tokens:0 request every 4.5 min (5m threads) or 55 min (1h threads) through gaps up to 2h, read at ctx
let kaCost = 0, kaSave = 0, kaN = 0;
for (const g of gapRows.filter((g) => g.cold && g.over && g.c.gapSec <= 7200)) {
  const period = g.ttl === 300 ? 270 : 3300, pings = Math.floor(g.c.gapSec / period);
  kaN += pings; kaCost += pings * g.c.prevCtx * crP(g.c); kaSave += rewriteTok(g.c) * (wRate(g.c) - crP(g.c));
}
raw.ttlCF = { extra1h, saved1h, kaCost, kaSave, kaN };
p("| scenario | arithmetic | saving/wk |");
p("|---|---|---|");
p(`| 5m threads (subagents) on the 1h TTL | cold calls with gap ≤ 1h become reads: ${$(saved1h)}; minus every 5m write now at 2× not 1.25× input: ${$(extra1h)} | **${$(saved1h - extra1h)}** |`);
p(`| keep-alive pings through gaps ≤ 2h | ${kaN} pings at the prior context × read price: ${$(kaCost)}; cold rewrites avoided: ${$(kaSave)} | **${$(kaSave - kaCost)}** |`);

// ---------- 5. model per role ----------
p();
p("## 5. Model per class: tokens and cost; price-only counterfactuals");
p();
const mc = {};
for (const c of C) { const key = `${top(c) === "subagent" ? "subagent: " + subGroup(c.role) : cls(c)}|${c.model}`; const x = (mc[key] ??= { calls: 0, ctxTok: 0, out: 0, cost: 0, priced: true }); x.calls++; x.ctxTok += c.ctx; x.out += c.out; x.cost += costT(c); if (!c.cost) x.priced = false; }
p("| class | model | calls | input-side tokens | output tokens | $ |");
p("|---|---|---|---|---|---|");
for (const [key, x] of Object.entries(mc).sort((a, b) => b[1].cost - a[1].cost)) { if (x.cost < 1 && x.priced) continue; const [a, m] = key.split("|"); p(`| ${a} | ${m} | ${x.calls} | ${(x.ctxTok / 1e6).toFixed(1)}M | ${(x.out / 1e6).toFixed(2)}M | ${x.priced ? $(x.cost) : "UNPRICED"} |`); }
p("(rows under $1 omitted; UNPRICED rows always shown)");
p();
// Sonnet 5.5's cache read is 0.05x input, $0.10 (pricing page footnote 2, read 2026-10-18; PANEL-49
// task 1.3). The baseline was measured at $0.20.
const SON = { input: 2, output: 10, cacheRead: 0.1 };
p("Repriced at Sonnet 5.5 list ($2 in / $10 out / $0.10 cache read / writes 1.25× and 2× input) and at Haiku 5.5's documented rate (≤100k prompt $0.10/$0.50, above $0.50/$2.50; reads 0.1×, writes 1.25×/2×). Same tokens: no tokenizer change (Haiku 5.5's tokenizer may count more), no change in turns, no quality claim. Note Sonnet 5.5's cache read ($0.10) is half Opus 5.5's ($0.20), so a Sonnet swap saves on reads as well as on writes and output.");
p();
p("| class | actual $ | at Sonnet 5.5 | saving | at Haiku 5.5 | saving |");
p("|---|---|---|---|---|---|");
raw.modelCF = {};
const cfGroups = [["lane:ops", (c) => cls(c) === "lane:ops"], ["lane:fix", (c) => cls(c) === "lane:fix"], ["lane:propose", (c) => cls(c) === "lane:propose"], ["orchestrator", (c) => cls(c) === "orchestrator"], ["subagent: builder", (c) => top(c) === "subagent" && subGroup(c.role) === "builder"], ["subagent: reviewer", (c) => top(c) === "subagent" && subGroup(c.role) === "reviewer"], ["subagent: reviewer's finders + code-review", (c) => top(c) === "subagent" && /finders|code-review/.test(subGroup(c.role))], ["subagent: all others", (c) => top(c) === "subagent" && !/^(builder|reviewer|reviewer's finders|code-review \(session-spawned\))$/.test(subGroup(c.role))], ["all subagents", (c) => top(c) === "subagent"]];
for (const [name, f] of cfGroups) {
  const r = priced.filter(f);
  const act = sum(r, costT), son = sum(r, (c) => reprice(c, SON)), hai = sum(r, (c) => reprice(c, haikuP(c.ctx)));
  raw.modelCF[name] = { act, son, hai };
  p(`| ${name} | ${$(act)} | ${$(son)} | ${$(act - son)} | ${$(hai)} | ${$(act - hai)} |`);
}

// ---------- 6. restarts / resumes ----------
p();
p("## 6. Restarts and resumes");
p();
const mains = [...threads.entries()].filter(([, a]) => a[0].role === "session");
const startedBefore = mains.filter(([, a]) => a[0].i > 0);
const bigFirst = mains.filter(([, a]) => a[0].i === 0 && a[0].ctx > 100000);
const mid = [];
for (const [k, a] of mains) for (const c of a) if (c.i > 0 && itemsOf(c).some((i) => i.key.startsWith("Hook SessionStart"))) mid.push({ k, c, compact: itemsOf(c).some((i) => /compaction/.test(i.cat)), a });
p(`- Main threads whose first in-window call is not their first call (started before 2026-10-08, carried in): ${startedBefore.length}; their first in-window context: ${startedBefore.map(([, a]) => K(a[0].ctx)).join(", ") || "none"}.`);
p(`- Main threads whose FIRST call already carries > 100k (a resume or fork into a new session id): ${bigFirst.length}${bigFirst.length ? ": " + bigFirst.map(([k, a]) => `${k.slice(0, 8)} ${K(a[0].ctx)}`).join(", ") : ""}.`);
p(`- Mid-thread restarts (a SessionStart hook after call 0, same session file): ${mid.length}; of which with a compaction summary: ${mid.filter((m) => m.compact).length}.`);
const resumeCost = [];
for (const m of mid) {
  const j = m.a.indexOf(m.c), carried = Math.max(0, m.c.ctx - (startCtx.get(m.k) ?? 0));
  resumeCost.push({ sid: m.k.slice(0, 8), ts: m.c.ts, ctx: m.c.ctx, prevCtx: m.c.prevCtx, carried, gapSec: m.c.gapSec, compact: m.compact, firstCost: costT(m.c), reRead: carried * suffix.get(m.k)[j + 1] });
}
raw.resumes = resumeCost;
if (resumeCost.length) {
  p();
  p("| session | at | ctx before → after | gap | compaction | carried above startup | restart call $ | carried re-read after $ |");
  p("|---|---|---|---|---|---|---|---|");
  for (const r of resumeCost) p(`| ${r.sid} | ${r.ts.slice(0, 16)} | ${K(r.prevCtx)} → ${K(r.ctx)} | ${r.gapSec == null ? "?" : Math.round(r.gapSec / 60) + "m"} | ${r.compact ? "yes" : "no"} | ${K(r.carried)} | ${$(r.firstCost)} | ${$(r.reRead)} |`);
}
const realComp = C.filter((c) => c.i > 0 && c.ctx < c.prevCtx && c.cost);
p();
p(`Real compactions/resets in the window (context fell between calls): ${realComp.length}, by class: ${Object.entries(realComp.reduce((m, c) => ((m[cls(c)] = (m[cls(c)] ?? 0) + 1), m), {})).map(([a, b]) => `${a} ${b}`).join(", ")}; context before → after: ${realComp.map((c) => `${K(c.prevCtx)}→${K(c.ctx)}`).join(", ")}.`);

// ---------- 7. repo-content share ----------
p();
p("## 7. What the repo's content decides: tool results and repo-authored text");
p();
p("Each item's cost = its first cache write + its re-reads by every later in-window call of its thread (the prototype's `downstream`). Grouped by who decides the tokens.");
p();
const DEC = (i) => {
  if (["Read", "Bash", "Skill", "Artifact", "web", "MCP"].includes(i.cat) || i.key === "Startup CLAUDE.md/AGENTS.md" || i.key.startsWith("Hook")) return "repo (tool results, CLAUDE.md/AGENTS.md, skill bodies, repo hooks)";
  if (i.cat === "own output") return "model (its own output, re-read)";
  if (i.cat === "user") return "user (prompts)";
  if (i.cat === "Agent" || i.cat === "peer/agent messages") return "orchestration (agent results, peer and teammate messages)";
  if (i.key === "Startup system prompt + tools" || i.cat === "reminders/hooks") return "harness/driver (system prompt + tools, reminders, listings)";
  if (i.cat === "compaction") return "harness/driver (compaction)";
  return "other";
};
const dec = {};
for (const i of I) { const x = (dec[DEC(i)] ??= { tok: 0, ds: 0, wr: 0 }); x.tok += i.tokens; x.ds += i.downstream ?? 0; x.wr += i.write ?? 0; }
const decTot = sum(Object.values(dec), (x) => x.ds + x.wr);
p("| decided by | tokens | re-read $ | first write $ | total $ | share of item $ |");
p("|---|---|---|---|---|---|");
for (const [d, x] of Object.entries(dec).sort((a, b) => b[1].ds + b[1].wr - a[1].ds - a[1].wr)) p(`| ${d} | ${K(x.tok)} | ${$(x.ds)} | ${$(x.wr)} | ${$(x.ds + x.wr)} | ${pct(x.ds + x.wr, decTot)} |`);
p(`| items total | | | | ${$(decTot)} | of ${$(TOTAL)} (the rest: output ${$(parts.output)}, cold rewrites, unattributed) |`);
p();
p("Tool results by tool:");
p();
p("| tool | n | tokens | mean | re-read $ | write $ | total $ |");
p("|---|---|---|---|---|---|---|");
const byTool = {};
for (const i of I) { if (!["Read", "Bash", "Skill", "Artifact", "web", "MCP", "Agent"].includes(i.cat)) continue; const t = i.cat === "Bash" ? `Bash ${i.key.split(" ")[1] ?? ""}` : i.cat; const x = (byTool[t] ??= { n: 0, tok: 0, ds: 0, wr: 0 }); x.n++; x.tok += i.tokens; x.ds += i.downstream ?? 0; x.wr += i.write ?? 0; }
for (const [t, x] of Object.entries(byTool).sort((a, b) => b[1].ds + b[1].wr - a[1].ds - a[1].wr).slice(0, 18)) p(`| ${t} | ${x.n} | ${K(x.tok)} | ${Math.round(x.tok / x.n)} | ${$(x.ds)} | ${$(x.wr)} | ${$(x.ds + x.wr)} |`);
p();
p("Largest single repo-content contributors (by key, summed over the week):");
p();
p("| key | n | tokens | re-read + write $ |");
p("|---|---|---|---|");
const byKey = {};
for (const i of I) { if (!DEC(i).startsWith("repo")) continue; const x = (byKey[i.key] ??= { n: 0, tok: 0, d: 0 }); x.n++; x.tok += i.tokens; x.d += (i.downstream ?? 0) + (i.write ?? 0); }
const topKeys = Object.entries(byKey).sort((a, b) => b[1].d - a[1].d).slice(0, 20);
for (const [kk, x] of topKeys) p(`| ${kk.slice(0, 80).replace(/\|/g, "\\|")} | ${x.n} | ${K(x.tok)} | ${$(x.d)} |`);
raw.decided = dec;
raw.byTool = byTool;
writeFileSync(join(dir, "repo-contributors.csv"), "key,n,tokens,cost\n" + Object.entries(byKey).sort((a, b) => b[1].d - a[1].d).map(([kk, x]) => `"${kk.replace(/"/g, "'")}",${x.n},${Math.round(x.tok)},${x.d.toFixed(4)}`).join("\n") + "\n");

// ---------- combined driver levers ----------
// One simulation per thread, holding repo content fixed: lean startup 25k + compaction cap 200k + cold calls that were
// waiting ≤ 1h priced as warm reads on the 1h TTL (5m threads pay 2× on their writes) + orchestrator machine wakes batched 3.
// the cap is applied only to the classes where it pays on its own (section 1, cap 200k)
// (argv[3] = a comma list overrides it, e.g. to apply the week's choice to one day)
const CAP_CLASSES = new Set(process.argv[3] ? process.argv[3].split(",") : raw.caps.filter((r) => r.X === 200000 && r.cl !== "ALL" && r.net > 0).map((r) => r.cl));
let comb = 0, wakePost = 0;
const machineSet = new Set(orchMachine.map((w) => `${w.k}|${w.j}`));
for (const [k, a] of threads) {
  const s0 = a[0].i === 0 ? st.find((s) => s.k === k) : null;
  const dS = s0 ? Math.max(0, s0.startupTok - 25000) : 0;
  let removed = 0;
  const S0 = Math.max(0, startCtx.get(k) - dS) + 30000;
  const capOn = CAP_CLASSES.has(cls(a[0]));
  for (let j = 0; j < a.length; j++) {
    const c = a[j];
    if (!c.cost) continue;
    if (j > 0 && c.ctx < a[j - 1].ctx) removed = 0;
    const cut = dS + removed;
    comb += Math.min(cut, c.cr) * crP(c);
    if (j === 0 && s0) comb += s0.firstShare * (s0.startupTok ? dS / s0.startupTok : 0);
    if (isRewrite(c)) {
      const rt = rewriteTok(c), rest = Math.max(0, rt - cut);
      comb += Math.min(cut, rt) * wRate(c);
      const ttl = ttlOf.get(k);
      if (c.gapSec != null && c.gapSec > ttl && c.gapSec <= 7200) {
        const pings = Math.floor(c.gapSec / (ttl === 300 ? 270 : 3300));
        comb += rest * (wRate(c) - crP(c)) - pings * Math.max(0, c.prevCtx - cut) * crP(c);
      }
    }
    if (machineSet.has(`${k}|${j}`)) wakePost += Math.max(0, c.cr - cut) * crP(c) + (isRewrite(c) && !(c.gapSec > ttlOf.get(k) && c.gapSec <= 7200) ? Math.max(0, rewriteTok(c) - cut) * wRate(c) : 0);
    const sim = c.ctx - cut;
    if (capOn && sim > 200000) { comb -= sim * crP(c) + 10000 * (pr(c).output / 1e6) + S0 * wRate(c); removed += sim - S0; }
  }
}
const wakeExtra = (2 / 3) * wakePost; // batching 3: the fixed cost each machine wake still pays after the other levers
raw.combined = { comb, wakeExtra };

p();
p("## Driver-decided share");
p();
p(`Combined driver levers, simulated together per thread so they do not double count (lean startup 25k + compaction cap 200k on ${[...CAP_CLASSES].join(", ")} + keep-alive pings through idle gaps up to 2h): **${$(comb)}** of ${$(TOTAL)} (${pct(comb, TOTAL)}). Adding orchestrator machine wakes batched 3 per wake, on the fixed cost each wake still pays after those levers: +${$(wakeExtra)} → **${$(comb + wakeExtra)} (${pct(comb + wakeExtra, TOTAL)})**. A model swap is excluded (a price lever with a quality question, section 5).`);

writeFileSync(join(dir, "driver.json"), JSON.stringify(raw, (k, v) => (typeof v === "number" ? +v.toFixed(4) : v), 1));
writeFileSync(join(dir, "wakes.csv"), "sid,class,i,ts,trigger,ctx,gapSec,cold,firstCost,fixed,turnCalls,turnCost,turnItemsTok,knockOn\n" + wakeRows.map((w) => [w.sid, w.cls, w.i, w.ts, JSON.stringify(w.trig), w.ctx, w.gapSec, w.cold, w.first.toFixed(4), w.fixed.toFixed(4), w.turnCalls, w.turnCost.toFixed(4), Math.round(w.turnItemsTok), w.knock.toFixed(4)].join(",")).join("\n") + "\n");
writeFileSync(join(dir, "startup.csv"), "thread,class,role,firstCtx,startupTok,firstShare,reRead,rewrites\n" + st.map((s) => [s.k, s.cls, JSON.stringify(s.sub), s.ctx, Math.round(s.startupTok), s.firstShare.toFixed(4), s.reRead.toFixed(4), s.rewr.toFixed(4)].join(",")).join("\n") + "\n");
console.log(out.join("\n"));
