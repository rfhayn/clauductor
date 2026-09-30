// UX harness (UX-1): merge the shards' findings into <out>/findings.json and a
// markdown summary, <out>/report.md.
//   node report.cjs <out> [shard …]   (default: every directory in <out> with a findings.json)
// Findings that repeat across shots (the same rule on the same element) are grouped:
// the report lists each once, with how many shots show it, where, and one screenshot.
const fs = require("fs");
const path = require("path");

const [outArg, ...names] = process.argv.slice(2);
const out = path.resolve(outArg || "ux-out");
const shards = names.length ? names : fs.readdirSync(out).filter((d) => fs.existsSync(path.join(out, d, "findings.json")));
const read = (f, d) => { try { return JSON.parse(fs.readFileSync(f, "utf8")); } catch (e) { return d; } };

// A run's temp paths differ each time; the report names them <run>.
// Ids and times inside keys (an agent's id, a timestamp) differ per run too.
const norm = (s) => String(s || "").replace(/\/[^\s"']*?\/(projects|home)\//g, "<run>/$1/").replace(/[0-9a-f]{8}-[0-9a-f-]{27}/g, "<uuid>")
  .replace(/\b[a-z]?[0-9a-f]{7,}\d*\b/g, "<id>");

const all = [], rows = [];
for (const s of shards) {
  const dir = path.join(out, s);
  const f = read(path.join(dir, "findings.json"), { findings: [], passes: [] });
  const t = read(path.join(dir, "time.json"), {});
  const err = fs.existsSync(path.join(dir, "error.txt")) ? fs.readFileSync(path.join(dir, "error.txt"), "utf8").trim() : "";
  for (const x of f.findings) all.push(Object.assign({}, x, { shard: s, screenshot: x.screenshot ? path.join(s, x.screenshot) : "" }));
  const flowFails = f.findings.filter((x) => x.rule === "flow").length;
  rows.push({ shard: s, passes: f.passes.length, skipped: f.passes.filter((p) => p.skipped).map((p) => p.name), findings: f.findings.length, flowFails,
    upSecs: t.upSecs, runSecs: t.runSecs, totalSecs: t.totalSecs, status: t.status, err, passList: f.passes });
}
fs.writeFileSync(path.join(out, "findings.json"), JSON.stringify({ generated: new Date().toISOString(), shards: rows.map(({ passList, ...r }) => r), findings: all }, null, 1));

// Group: rule + normalised selector (+ the flow, for flow failures).
const groups = new Map();
for (const x of all) {
  const k = x.rule + "|" + (x.rule === "flow" || x.rule === "harness" ? x.name : norm(x.selector)) + "|" + (x.rule === "console" ? norm(x.detail) : "");
  if (!groups.has(k)) groups.set(k, { rule: x.rule, selector: norm(x.selector), name: x.name, detail: norm(x.detail), items: [] });
  groups.get(k).items.push(x);
}
const order = ["harness", "flow", "console", "hscroll", "xterm-fill", "xterm-fit", "selected-hidden", "overlap", "offscreen", "clip", "focus-ring", "focus-offscreen", "menu-missing", "view-failed"];
const sorted = Array.from(groups.values()).sort((a, b) => (order.indexOf(a.rule) - order.indexOf(b.rule)) || (b.items.length - a.items.length));
const uniq = (a) => Array.from(new Set(a.filter(Boolean)));
const md = [];
md.push("# UX pass report", "", "Generated " + new Date().toISOString() + " from " + shards.length + " shard(s) in `" + out + "`.", "");
md.push("| Shard | Shots / flows passed | Findings | Flow failures | Up (s) | Run (s) | Total (s) |", "|---|---|---|---|---|---|---|");
for (const r of rows) md.push("| " + r.shard + (r.err ? " (error: " + r.err.replace(/\|/g, "/") + ")" : "") + " | " + r.passes + " | " + r.findings + " | " + r.flowFails + " | " + (r.upSecs ?? "") + " | " + (r.runSecs ?? "") + " | " + (r.totalSecs ?? "") + " |");
md.push("");
const byRule = {};
for (const g of sorted) byRule[g.rule] = (byRule[g.rule] || 0) + 1;
md.push("Distinct findings by rule: " + (Object.keys(byRule).length ? Object.entries(byRule).map(([k, v]) => k + " " + v).join(", ") : "none") + ".", "");
const skipped = rows.flatMap((r) => r.skipped);
if (skipped.length) md.push("Skipped (not in this build or state): " + skipped.join(", ") + ".", "");
const flows = rows.flatMap((r) => r.passList.filter((p) => p.name.startsWith("flows/")).map((p) => p.name.slice(6) + " (" + p.secs + " s)"));
if (flows.length) md.push("Flows passed: " + flows.join(", ") + ".", "");
md.push("## Findings", "");
if (!sorted.length) md.push("None.");
for (const g of sorted) {
  const it = g.items;
  const ctx = (k) => uniq(it.map((x) => x[k]));
  md.push("### " + g.rule + (g.rule === "flow" ? ": " + g.name : g.selector ? ": `" + g.selector.slice(0, 140) + "`" : ""), "");
  md.push("- " + g.detail.slice(0, 400));
  md.push("- Seen in " + it.length + " shot(s)" + (ctx("viewport").length ? "; viewports " + ctx("viewport").join(", ") : "") +
    (ctx("theme").length ? "; themes " + ctx("theme").join(", ") : "") + (ctx("mode").length ? "; modes " + ctx("mode").join(", ") : "") +
    (ctx("scale").length ? "; sizes " + ctx("scale").join(", ") + "%" : "") + "; views " + uniq(it.map((x) => x.area + "/" + x.name)).slice(0, 8).join(", ") + (uniq(it.map((x) => x.name)).length > 8 ? ", …" : ""));
  const shot = it.find((x) => x.screenshot);
  if (shot) md.push("- Screenshot: `" + shot.screenshot + "`");
  md.push("");
}
fs.writeFileSync(path.join(out, "report.md"), md.join("\n") + "\n");
console.log("report: " + all.length + " findings (" + sorted.length + " distinct) → " + path.join(out, "report.md"));
