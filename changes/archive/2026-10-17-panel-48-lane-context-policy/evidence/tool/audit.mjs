// Driver: walk every transcript of the repo for a window and write calls.jsonl, items.jsonl,
// subject.json into the output dir. driver.mjs reads them.
//   node audit.mjs <outDir> <since> <until> <mainTranscriptDir>
// <mainTranscriptDir> is the repository's main checkout's folder under ~/.claude/projects (its
// path with every non-alphanumeric character turned into "-"). Its worktrees' folders are found
// beside it. It is an argument so that no one's home path is written into the tool (PANEL-48 D12).
import { existsSync, readdirSync, statSync, writeFileSync, createWriteStream } from "node:fs";
import { basename, dirname, join, resolve } from "node:path";
import {
  agentIdOf, attributeItems, attributeThread, CATEGORY, labelOf, normBash, normPath, priceFor,
  readMeta, readTranscript, repoProjectDirs, roleOf, transcriptsUnder,
} from "./usage-report.mjs";

const [out, since, until, mainArg] = process.argv.slice(2);
if (!out || !since || !until || !mainArg) {
  process.stderr.write("usage: node audit.mjs <outDir> <since> <until> <mainTranscriptDir>\n");
  process.exit(2);
}
const mainDir = resolve(mainArg);
if (!existsSync(mainDir)) {
  process.stderr.write(`audit: no transcript directory at ${mainDir}\n`);
  process.exit(1);
}
const root = dirname(mainDir);
const mainBase = basename(mainDir);
const listing = readdirSync(root);
const dirs = repoProjectDirs(mainDir, listing).filter((d) => existsSync(d));
// Scratchpad sessions that ran in a copy of the repository are listed in subject.json, not
// counted. They are matched by the folder name's last segment (the repository's name when it has
// no "-" in it); the list is a record only, so a looser match costs nothing.
const repoName = mainBase.split("-").filter(Boolean).pop();
const excluded = listing.filter((e) => e.startsWith("-private-tmp-") && e.includes(repoName));
const inWindow = (ts) => !!ts && ts >= since && ts < until;
const priceOf = (c) => priceFor(c.model, c.speed);

const callsOut = createWriteStream(join(out, "calls.jsonl"));
const itemsOut = createWriteStream(join(out, "items.jsonl"));
const subject = { since, until, dirs: [], excluded, sessions: 0, agents: 0, parseFailures: [], missingMeta: 0, resets: 0, calib: [], crossCheck: { mine: 0, base: 0 } };

function kindOf(dirName, thread) {
  const d = dirName.startsWith(mainBase) ? dirName.slice(mainBase.length).replace(/^--claude-worktrees-/, "") : dirName;
  if (!d) return "orchestrator";
  if (/^agent-a[0-9a-f]+$/.test(d)) return "worktree:agent-isolation";
  const tus = thread.calls.flatMap((c) => c.toolUses);
  const skills = new Set(tus.filter((t) => t.name === "Skill").map((t) => t.input?.skill));
  const fp = thread.firstPrompt ?? "";
  if (d.startsWith("build-") || skills.has("build-change") || tus.some((t) => t.name === "Workflow") || /\/build-change|build-change/.test(fp)) return "worktree:build";
  if (d.startsWith("propose-") || skills.has("openspec-propose") || /openspec-propose|\bpropose\b/i.test(fp.slice(0, 200))) return "worktree:propose";
  const br = [...thread.branches.entries()].sort((a, b) => b[1] - a[1])[0]?.[0] ?? "";
  if (br.startsWith("fix/")) return "worktree:fix";
  if (br.startsWith("ops/")) return "worktree:ops";
  if (br.startsWith("change/")) return "worktree:propose";
  return "worktree:other";
}

// Lines worth searching for later: long, distinctive, not boilerplate.
function samples(text = "") {
  const seen = new Set();
  return text
    .split("\n")
    .map((l) => l.replace(/^\s*\d+[\t→]/, "").trim())
    .filter((l) => l.length >= 30 && l.length <= 160 && /[a-z]{4}/i.test(l) && !seen.has(l) && seen.add(l))
    .sort((a, b) => b.length - a.length)
    .slice(0, 6)
    .map((l) => l.slice(0, 60));
}

function processThread(file, ctx) {
  const thread = attributeThread(file, { agentTypes: ctx.agentTypes });
  if (thread.parseFailures) subject.parseFailures.push({ file: file.replace(root, "~/.claude/projects"), lines: thread.parseFailures, reason: "JSON.parse threw (truncated or partial line)" });
  const callsIn = thread.calls.filter((c) => inWindow(c.ts));
  if (!callsIn.length) return { thread, n: 0 };
  // cross-check against the base readTranscript (same dedupe), same window
  const base = readTranscript(file, ctx.role, since).filter((r) => r.ts < until);
  subject.crossCheck.mine += callsIn.length;
  subject.crossCheck.base += base.length;
  const { items, resets, calibration } = attributeItems(thread, inWindow, priceOf);
  subject.resets += resets;
  if (calibration) subject.calib.push(calibration);
  const kind = ctx.kind ?? kindOf(ctx.dirName, thread);
  // output text index for "was it used later"
  let outStr = "";
  const offs = [];
  const tuIndex = [];
  thread.calls.forEach((c, i) => {
    offs.push(outStr.length);
    outStr += `${c.out.join("\n")}\n`;
    for (const t of c.toolUses) tuIndex.push({ i, name: t.name, path: normPath(t.input?.file_path ?? ""), key: t.name === "Bash" ? normBash(t.input?.command) : t.name });
  });
  thread.calls.forEach((c, i) => {
    if (!inWindow(c.ts)) return;
    const u = c.usage;
    const ctxT = (u.input_tokens ?? 0) + (u.cache_read_input_tokens ?? 0) + (u.cache_creation_input_tokens ?? 0);
    const prev = thread.calls[i - 1];
    const prevCtx = prev ? (prev.usage.input_tokens ?? 0) + (prev.usage.cache_read_input_tokens ?? 0) + (prev.usage.cache_creation_input_tokens ?? 0) : 0;
    const p = priceOf(c);
    const w = u.cache_creation_input_tokens ?? 0;
    const w1 = u.cache_creation?.ephemeral_1h_input_tokens ?? 0;
    const lastTu = prev?.toolUses.at(-1);
    const wait = !prev ? "start" : lastTu ? (lastTu.name === "Bash" ? `Bash ${normBash(lastTu.input?.command)}` : lastTu.name === "Agent" || lastTu.name === "Task" ? `Agent ${lastTu.input?.subagent_type ?? "general-purpose"}${lastTu.input?.run_in_background ? " (bg)" : ""}` : lastTu.name) : `turn end → ${c.items.find((x) => /^(User|Message|Agent|Hook|Bash background)/.test(x.key))?.key ?? c.items[0]?.key ?? "?"}`;
    callsOut.write(`${JSON.stringify({
      sid: ctx.sid, dir: ctx.dirName, kind, role: ctx.role, thread: ctx.agentId ?? "main", i, ts: c.ts,
      model: c.speed === "fast" ? `${c.model} (fast)` : c.model, priced: !!p,
      input: u.input_tokens ?? 0, cw5: w - w1, cw1: w1, cr: u.cache_read_input_tokens ?? 0, out: u.output_tokens ?? 0, ctx: ctxT, prevCtx,
      cost: p ? { input: ((u.input_tokens ?? 0) * p.input) / 1e6, cacheWrite: ((w - w1) * p.input * 1.25 + w1 * p.input * 2) / 1e6, cacheRead: ((u.cache_read_input_tokens ?? 0) * p.cacheRead) / 1e6, output: ((u.output_tokens ?? 0) * p.output) / 1e6 } : null,
      nTools: c.toolUses.length,
      gapSec: prev && prev.ts && c.ts ? (Date.parse(c.ts) - Date.parse(prev.ts)) / 1000 : null,
      wait,
    })}\n`);
  });
  for (const it of items) {
    const rec = { sid: ctx.sid, kind, role: ctx.role, thread: ctx.agentId ?? "main", call: it.call, origin: it.origin ?? undefined, src: it.src ?? undefined, meta: it.meta || undefined, head: !it.tu && it.text ? it.text.slice(0, 40) : undefined, key: it.key, cat: CATEGORY(it.key), tokens: Math.round(it.tokens), later: it.later, downstream: it.downstream, write: it.write, chars: it.chars, ts: it.ts };
    if (it.text && (it.key.startsWith("Read") || it.key.startsWith("Bash") || it.key.startsWith("Agent"))) {
      const s = samples(it.text);
      rec.samples = s.length;
      rec.quoted = s.filter((l) => outStr.indexOf(l, offs[it.call]) >= 0).length;
    }
    if (it.key.startsWith("Read ")) {
      const path = normPath(it.tu?.input?.file_path ?? "");
      const after = tuIndex.filter((t) => t.i > it.call && t.path === path);
      rec.path = path;
      rec.reReads = after.filter((t) => t.name === "Read").length;
      rec.edited = after.some((t) => t.name === "Edit" || t.name === "Write");
      rec.republished = after.some((t) => t.name === "Artifact");
      rec.lines = (it.text ?? "").split("\n").length;
    }
    itemsOut.write(`${JSON.stringify(rec)}\n`);
  }
  return { thread, n: callsIn.length };
}

for (const dir of dirs) {
  const dirName = basename(dir);
  const mains = readdirSync(dir).filter((f) => f.endsWith(".jsonl")).map((f) => join(dir, f));
  let used = 0;
  for (const main of mains) {
    const sid = basename(main, ".jsonl");
    const subDir = join(dir, sid, "subagents");
    const files = existsSync(subDir) ? transcriptsUnder(subDir) : [];
    const recent = statSync(main).mtime.toISOString() >= since || files.some((f) => statSync(f).mtime.toISOString() >= since);
    if (!recent) continue;
    const metas = new Map();
    for (const f of files) {
      const meta = readMeta(f.replace(/\.jsonl$/, ".meta.json"));
      if (meta) metas.set(agentIdOf(f), meta);
      else subject.missingMeta++;
    }
    const agentTypes = new Map([...metas].map(([id, m]) => [id, labelOf(m)]));
    const r = processThread(main, { sid, dirName, role: "session", agentTypes });
    const kind = kindOf(dirName, r.thread);
    let agentsIn = 0;
    for (const f of files) {
      const id = agentIdOf(f);
      const a = processThread(f, { sid, dirName, role: roleOf(id, metas), agentId: id, kind, agentTypes });
      if (a.n) agentsIn++;
    }
    if (r.n || agentsIn) {
      subject.sessions++;
      used++;
      subject.agents += agentsIn;
    }
  }
  if (used) subject.dirs.push({ dir: dirName, sessions: used });
}
callsOut.end();
itemsOut.end();
subject.allDirs = dirs.map((d) => basename(d));
writeFileSync(join(out, "subject.json"), JSON.stringify(subject, null, 1));
console.log(JSON.stringify({ sessions: subject.sessions, agents: subject.agents, dirsUsed: subject.dirs.length, dirsTotal: dirs.length, parseFailFiles: subject.parseFailures.length, resets: subject.resets, cross: subject.crossCheck, missingMeta: subject.missingMeta }));
