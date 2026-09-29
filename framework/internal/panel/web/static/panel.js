"use strict";
// xterm.js adds <style> elements at run time. The CSP allows no inline style except
// one carrying this response's nonce, so stamp it on every <style> the page creates.
// This must run before xterm creates its first terminal.
const STYLE_NONCE = document.querySelector('meta[name="csp-style-nonce"]').content;
{
  const create = Document.prototype.createElement;
  Document.prototype.createElement = function (tag, opts) {
    const e = create.call(this, tag, opts);
    if (String(tag).toLowerCase() === "style") e.nonce = STYLE_NONCE;
    return e;
  };
}
// Every piece of text from the server goes through textContent: prompts, card output
// and PR titles are data, never markup.
let S = null, offset = 0, es = null;
// The selected lane, by key: "t:<lane id>" for a lane with a terminal, "w:<worktree>"
// for a session started outside the panel (PANEL-11). Kept per browser.
let selKey = null;
try {
  selKey = localStorage.getItem("clauductor-panel-sel");
  if (!selKey && localStorage.getItem("clauductor-panel-term")) selKey = "t:" + localStorage.getItem("clauductor-panel-term");
} catch (e) {}

function el(tag, cls, text, kids) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  if (text != null) e.textContent = text;
  for (const k of kids || []) if (k) e.appendChild(k);
  return e;
}
const $ = (id) => document.getElementById(id);

// ---- Patching in place (PANEL-6) ------------------------------------------------
// The page is described by building fresh elements, then patched into the live DOM:
// an element whose key (data-k) or position matches is kept and only its differences
// are applied, so focus, selection and scroll survive an update. Every control has a
// stable key. While text is selected, the keyed element at each end of the selection
// (a card, a row) is held as it is, so what you selected stays put; everything else,
// including a new card next to it and the heading counts, still updates. Headings
// are never held.
function key(e, k) { e.dataset.k = k; return e; }
// Handlers live on the element as data, so a patch can swap them without re-adding
// listeners: a kept element runs the handler of the newest description of it.
function dispatch(ev) { const f = this._h && this._h[ev.type]; if (f) f.call(this, ev); }
function on(e, type, fn) {
  (e._h || (e._h = {}))[type] = fn;
  e.addEventListener(type, dispatch);
  return e;
}
let selHeld = new Set();
function heldBySelection() {
  const held = new Set();
  const s = window.getSelection();
  if (!s || s.isCollapsed || !s.rangeCount) return held;
  for (let n of [s.anchorNode, s.focusNode]) {
    if (n && n.nodeType !== 1) n = n.parentElement;
    const k = n && !n.closest(".xterm") ? n.closest("[data-k]") : null;
    if (k && k.tagName !== "H2") held.add(k);
  }
  return held;
}
function sameKind(o, n) { return o.nodeType === n.nodeType && (o.nodeType !== 1 || o.tagName === n.tagName); }
function keyOf(n) { return n.nodeType === 1 && n.dataset.k != null ? n.dataset.k : null; }
function syncAttrs(o, n) {
  for (const a of Array.from(o.attributes)) if (a.name !== "style" && !n.hasAttribute(a.name)) o.removeAttribute(a.name);
  for (const a of Array.from(n.attributes)) if (a.name !== "style" && o.getAttribute(a.name) !== a.value) o.setAttribute(a.name, a.value);
  // Styles go through CSSOM: the CSP refuses style attributes.
  if (o.style.cssText !== n.style.cssText) o.style.cssText = n.style.cssText;
  if (n._h) { o._h = n._h; for (const t of Object.keys(n._h)) o.addEventListener(t, dispatch); } else o._h = null;
}
function morph(o, n) {
  if (selHeld.has(o)) return;
  syncAttrs(o, n);
  patch(o, n.childNodes);
}
// patch makes parent's children match kids, keeping every node it can.
function patch(parent, kids) {
  if (selHeld.has(parent)) return;
  kids = Array.from(kids);
  const olds = Array.from(parent.childNodes);
  const byKey = new Map(), loose = [];
  for (const o of olds) { const k = keyOf(o); if (k != null && !byKey.has(k)) byKey.set(k, o); else if (k == null) loose.push(o); }
  const used = new Set();
  let li = 0;
  const plan = kids.map((n) => {
    const k = keyOf(n);
    let o = null;
    if (k != null) { o = byKey.get(k) || null; if (o && (!sameKind(o, n) || used.has(o))) o = null; }
    else while (li < loose.length) { const c = loose[li++]; if (sameKind(c, n)) { o = c; break; } }
    if (o) used.add(o);
    return [o, n];
  });
  for (const o of olds) if (!used.has(o)) o.remove();
  plan.forEach(([o, n], i) => {
    const node = o || n;
    if (o) {
      if (o.nodeType === 1) morph(o, n);
      else if (o.nodeValue !== n.nodeValue) { o.nodeValue = n.nodeValue; flash(o.parentElement); }
    }
    const at = parent.childNodes[i];
    if (at !== node) parent.insertBefore(node, at || null);
  });
}
function patchInto(id, kids) { patch($(id), kids.filter(Boolean)); }
// The one motion on the page: a figure whose value changed flashes for 150 ms. Ages
// and countdowns tick every second and do not.
function flash(e) {
  if (!e || !e.classList || !e.classList.contains("num") || e.classList.contains("age") || e.classList.contains("cd")) return;
  e.classList.add("flash");
  setTimeout(() => e.classList.remove("flash"), 150);
}
function setText(e, t) { if (e.textContent !== t) e.textContent = t; }

// ---- Time --------------------------------------------------------------------------
// Every age is computed here from a timestamp: the server sends none, and pushes a
// view only when it changed. While the page has lost the panel, the clock stops at
// the last word from it, so nothing looks fresher than it is.
let frozenAt = 0;
const now = () => frozenAt || Date.now() - offset;
function ageText(ms) {
  if (!ms) return "—";
  const s = Math.max(0, Math.round((now() - ms) / 1000));
  if (s < 60) return s + "s";
  if (s < 3600) return Math.floor(s / 60) + "m";
  if (s < 86400) return Math.floor(s / 3600) + "h" + String(Math.floor((s % 3600) / 60)).padStart(2, "0");
  return Math.floor(s / 86400) + "d";
}
// An age that keeps moving: tickAges rewrites it once a second, in place.
function age(ms, before, after) {
  const e = el("span", "age", (before || "") + ageText(ms) + (after || ""));
  e.dataset.at = ms || 0;
  if (before) e.dataset.pre = before;
  if (after) e.dataset.post = after;
  return e;
}
function tickAges() {
  if (frozenAt) return;
  const held = [...heldBySelection()];
  for (const e of document.querySelectorAll("span.age[data-at]")) {
    if (held.some((h) => h.contains(e))) continue;
    setText(e, (e.dataset.pre || "") + ageText(+e.dataset.at) + (e.dataset.post || ""));
  }
  for (const e of document.querySelectorAll("span.cd[data-until]")) if (!held.some((h) => h.contains(e))) paintUntil(e);
}
function hhmm(ms) { const d = new Date(ms); return String(d.getHours()).padStart(2, "0") + ":" + String(d.getMinutes()).padStart(2, "0") + ":" + String(d.getSeconds()).padStart(2, "0"); }
function hm(ms) { const d = new Date(ms); return String(d.getHours()).padStart(2, "0") + ":" + String(d.getMinutes()).padStart(2, "0"); }
function pct(v) { return v == null ? "—" : Math.round(v) + "%"; }

// ---- The connection -------------------------------------------------------------
// LIVE while state or a heartbeat (every 5 s) arrives. Three missed beats, or a
// dropped stream, and the page says so everywhere: a banner with the reconnect
// status, the title, dimmed columns, ages frozen at the last word, actions off.
const BEAT_MISS_MS = 15000;
let lastBeat = 0;       // Date.now() of the last message from the panel
let lastServer = 0;     // the panel's clock at that message
const conn = { state: "connecting", attempt: 0, nextAt: 0, timer: null, expired: false };
const offline = () => conn.state !== "live" && S !== null;

function heard(serverNow) {
  lastBeat = Date.now();
  if (serverNow) { lastServer = serverNow; offset = Date.now() - serverNow; }
}
function connect() {
  if (es) es.close();
  es = new EventSource("/events");
  es.addEventListener("state", (e) => {
    const first = !S;
    S = JSON.parse(e.data);
    heard(S.now);
    if (first) setTimeout(seen, 0);
    goLive();
    render();
  });
  es.addEventListener("hb", (e) => {
    try { heard(JSON.parse(e.data).now); } catch (x) { heard(0); }
    if (S) goLive();
  });
  es.onerror = () => lost();
}
function goLive() {
  if (conn.state === "live") return;
  const was = conn.state;
  conn.state = "live"; conn.attempt = 0; conn.expired = false; frozenAt = 0;
  clearTimeout(conn.timer);
  if (was !== "connecting") render();
}
function lost(expired) {
  if (es) { es.close(); es = null; }
  if (conn.state === "live" || conn.state === "connecting") frozenAt = lastServer || 0;
  conn.state = "lost"; conn.expired = !!expired;
  const delay = Math.min(1000 * 2 ** conn.attempt, 10000);
  conn.attempt++;
  conn.nextAt = Date.now() + delay;
  clearTimeout(conn.timer);
  conn.timer = setTimeout(probe, delay);
  render();
}
// EventSource gives up silently on a 401 (e.g. the panel restarted with a new token),
// so probe with fetch to tell "server down" from "session expired".
// A check that hangs counts as failed after PROBE_MS, and so does a stream that
// opens but says nothing.
const PROBE_MS = 5000;
async function probe() {
  clearTimeout(conn.timer);
  conn.nextAt = 0;
  renderConn();
  const ac = new AbortController();
  const limit = setTimeout(() => ac.abort(), PROBE_MS);
  try {
    const r = await fetch("/api/state", { cache: "no-store", signal: ac.signal });
    if (r.status === 401) { lost(true); return; }
    if (!r.ok) throw new Error(r.status);
  } catch (e) { lost(conn.expired); return; } finally { clearTimeout(limit); }
  connect();
  conn.timer = setTimeout(() => { if (conn.state !== "live") lost(); }, PROBE_MS);
}
setInterval(() => {
  if (conn.state === "live" && Date.now() - lastBeat > BEAT_MISS_MS) lost();
  if (conn.state === "lost") renderConn();
}, 1000);

function renderConn() {
  const bar = $("connbar"), live = $("live");
  const off = offline();
  document.body.classList.toggle("offline", off);
  if (!S) {
    setText($("livetext"), conn.state === "lost" ? "Disconnected" : "Connecting");
    live.className = "live dead";
  } else {
    setText($("livetext"), off ? "Disconnected" : "Live");
    live.className = off ? "live dead" : "live";
  }
  bar.hidden = !off;
  if (!off) return;
  const asOf = frozenAt ? hhmm(frozenAt) : "—";
  const wait = conn.nextAt ? Math.max(0, Math.ceil((conn.nextAt - Date.now()) / 1000)) : 0;
  const status = conn.expired
    ? "The panel restarted with a new token: open the URL clauductor panel printed, or run clauductor panel open."
    : !conn.nextAt ? "Checking whether the panel is back…"
    : conn.attempt > 1 ? "Reconnect failed (attempt " + (conn.attempt - 1) + "), retrying in " + wait + " s."
    : "Retrying in " + wait + " s.";
  const retry = key(el("button", "btn small", "Retry now"), "retry");
  retry.type = "button";
  on(retry, "click", () => probe());
  patch(bar, [
    el("b", null, "Disconnected"),
    el("span", null, "The panel is not answering. Everything below is as of " + asOf + ", and actions are off until it is back. "),
    key(el("span", "sub", status), "status"),
    conn.expired ? null : retry,
  ].filter(Boolean));
}


// APPROX marks a status that is not a current `claude agents` reading (a failed or
// old poll, or hooks alone): shown, never trusted as current.
const APPROX = "≈ ";
const APPROX_TITLE = "approximate: not a current `claude agents` reading (the poll failed or is old, or only hooks know)";

// How old an approximate status is: the age of the last good `claude agents` read.
function staleTag(approx) {
  if (!approx) return null;
  if (!S.agentsReadAt) return el("span", "dim", "stale, hooks only");
  return age(S.agentsReadAt, "stale, read ", " ago");
}

// ---- Small graphics: bullet bars, sparklines, the state glyph (PANEL-11) ---------------
// A bullet bar, 64 x 6: fill to v (0-100), a tick at `tick`, a projection at `proj`.
function bullet(v, o) {
  o = o || {};
  const b = el("span", "bullet" + (o.cls ? " " + o.cls : ""), null, [el("i")]);
  b.firstChild.style.width = Math.max(0, Math.min(100, v || 0)) + "%";
  if (o.tick != null) { const t = el("b"); t.style.left = Math.max(0, Math.min(100, o.tick)) + "%"; b.appendChild(t); }
  if (o.proj != null) { const p = el("u"); p.style.left = Math.max(0, Math.min(100, o.proj)) + "%"; b.appendChild(p); }
  b.setAttribute("aria-hidden", "true");
  return b;
}
// A sparkline, 48 x 12, of a Spark (one value a minute). TUI draws it in braille.
const SVGNS = "http://www.w3.org/2000/svg";
function spark(sp, o) {
  o = o || {};
  if (!sp || !sp.v || sp.v.length < 2) return null;
  const v = sp.v.slice(-48);
  let lo = o.lo != null ? o.lo : Math.min(...v), hi = o.hi != null ? o.hi : Math.max(...v);
  if (hi - lo < 1e-9) { hi = lo + 1; }
  const style = getComputedStyle(document.documentElement).getPropertyValue("--spark").replace(/["' ]/g, "");
  if (style === "braille") {
    // Two columns a character, four levels each: 8 characters for up to 16 points.
    const pts = [];
    for (let i = 0; i < 16; i++) pts.push(v[Math.min(v.length - 1, Math.round((i * (v.length - 1)) / 15))]);
    const lvl = (x) => Math.max(0, Math.min(4, Math.round(((x - lo) / (hi - lo)) * 4)));
    const L = [0, 0x40, 0x44, 0x46, 0x47], R = [0, 0x80, 0xa0, 0xb0, 0xb8];
    let s = "";
    for (let i = 0; i < 16; i += 2) s += String.fromCharCode(0x2800 + L[lvl(pts[i])] + R[lvl(pts[i + 1])]);
    const e = el("span", "brl", s);
    e.setAttribute("aria-hidden", "true");
    return e;
  }
  const svg = document.createElementNS(SVGNS, "svg");
  svg.setAttribute("class", "spark");
  svg.setAttribute("viewBox", "0 0 48 12");
  svg.setAttribute("preserveAspectRatio", "none");
  svg.setAttribute("aria-hidden", "true");
  const x = (i) => ((i / (v.length - 1)) * 47 + 0.5).toFixed(1), y = (a) => (11 - ((a - lo) / (hi - lo)) * 10).toFixed(1);
  const path = document.createElementNS(SVGNS, "path");
  path.setAttribute("d", v.map((a, i) => (i ? "L" : "M") + x(i) + " " + y(a)).join(""));
  const dot = document.createElementNS(SVGNS, "circle");
  dot.setAttribute("cx", x(v.length - 1)); dot.setAttribute("cy", y(v[v.length - 1])); dot.setAttribute("r", "1.2");
  svg.append(path, dot);
  return svg;
}
// A figure with its unit, in the data face.
function num(text, unit, cls) {
  const e = el("span", "num" + (cls ? " " + cls : ""), text);
  return unit ? el("span", "nu", null, [e, el("span", "unit", unit)]) : e;
}
function money(v) { return v == null ? "—" : "$" + (v >= 100 ? v.toFixed(0) : v.toFixed(2)); }
function kilo(n) { return n == null ? "—" : n >= 1e6 ? (n / 1e6).toFixed(1) + "M" : n >= 1e3 ? (n / 1e3).toFixed(1) + "k" : String(n); }
function dur(ms) {
  const s = Math.max(0, Math.round(ms / 1000));
  if (s < 60) return s + "s";
  if (s < 3600) return Math.floor(s / 60) + "m";
  return Math.floor(s / 3600) + "h" + String(Math.floor((s % 3600) / 60)).padStart(2, "0");
}
// A countdown that ticks in place: "cold in 1:40", amber under two minutes.
function until(ms, pre, warnUnder) {
  const e = el("span", "cd num", "");
  e.dataset.until = ms;
  e.dataset.pre = pre || "";
  if (warnUnder) e.dataset.warn = warnUnder;
  paintUntil(e);
  return e;
}
function paintUntil(e) {
  const left = Math.max(0, +e.dataset.until - now());
  const s = Math.round(left / 1000);
  const t = left <= 0 ? "now" : s >= 3600 ? Math.floor(s / 3600) + "h" + String(Math.floor((s % 3600) / 60)).padStart(2, "0") : Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
  setText(e, e.dataset.pre + t);
  e.classList.toggle("warn", !!e.dataset.warn && left > 0 && left < +e.dataset.warn);
}

// ---- Lanes (PANEL-11) ---------------------------------------------------------------
// A lane is what you select: a tmux lane the panel started (with its terminal), or a
// worktree with a claude session started elsewhere (no terminal). Everything the page
// says about a lane is derived here from the view the panel sent; the page keeps no
// state of its own but what is selected and how the layout is sized.
function lanesOf() {
  const all = S.lanes.concat(S.quietWorktrees);
  const byPath = new Map(all.map((l) => [l.path, l]));
  const out = [];
  for (const t of S.terminals || []) {
    let lv = t.worktree ? byPath.get(t.worktree) || null : null;
    if (lv && lv.terminal && lv.terminal !== t.id) lv = null; // a second lane in one worktree
    out.push({ key: "t:" + t.id, id: t.id, name: t.id, t, lv, path: t.path, wt: t.worktree || "", branch: t.branch || (lv ? lv.branch : ""), type: t.type || (lv ? lv.type : "") });
  }
  for (const l of S.lanes) {
    if (!l.terminal) out.push({ key: "w:" + l.path, id: l.id, name: l.name, t: null, lv: l, path: l.path, wt: l.path, branch: l.branch, type: l.type });
  }
  return out;
}
// One of busy | waiting | idle | none | dead | orphaned | stale.
function laneState(x) {
  if (x.t && x.t.dead) return "dead";
  if (x.t && !x.t.running) return "orphaned";
  if (x.lv && x.lv.stale) return "stale";
  const st = x.t ? x.t.status : x.lv.status;
  return st === "running" ? "none" : st;
}
function laneApprox(x) { return !!((x.t && x.t.approx) || (!x.t && x.lv && x.lv.approx)); }
function laneWaitingFor(x) { return (x.t && x.t.waitingFor) || (x.lv && x.lv.waitingFor) || ""; }
const STATE_WORD = { busy: "working", waiting: "needs you", idle: "idle", none: "starting", dead: "exited", orphaned: "orphaned", stale: "no hooks" };
const STATE_ORDER = { waiting: 0, stale: 1, dead: 1, orphaned: 1, busy: 2, none: 3, idle: 4 };
function stateWord(x) {
  const s = laneState(x);
  if (s === "none" && !x.t) return "no session";
  return STATE_WORD[s] || s;
}
function stateMark(x) {
  const s = laneState(x);
  const w = el("span", "state-word " + s, (laneApprox(x) ? APPROX : "") + stateWord(x));
  if (laneApprox(x)) w.title = APPROX_TITLE;
  return el("span", "st", null, [el("span", "sq " + s), w]);
}
function laneStatusText(x) {
  const a = laneApprox(x) ? APPROX : "";
  const s = laneState(x);
  if (s === "waiting") return a + "needs you" + (laneWaitingFor(x) ? ": " + laneWaitingFor(x) : "");
  let t = a + stateWord(x);
  const subs = x.lv ? x.lv.subagents.length : 0;
  if (subs) t += ", " + subs + " subagent" + (subs > 1 ? "s" : "");
  return t;
}
function laneM(x) { return (x.lv && x.lv.metrics) || {}; }
// When the lane came up: its tmux session, else its oldest claude process.
function laneSince(x) {
  if (x.t && x.t.created) return x.t.created * 1000;
  const starts = x.lv ? x.lv.sessions.map((s) => s.startedAt).filter(Boolean) : [];
  return starts.length ? Math.min(...starts) : 0;
}
function laneCtx(x) { return x.lv && x.lv.ctxPct != null ? x.lv.ctxPct : x.t ? x.t.ctxPct : null; }
function laneCost(x) { const m = laneM(x); return m.costUsd != null ? m.costUsd : null; }
function perHour(n, m) { return m.durationMs >= 5 * 60000 && n != null ? n / (m.durationMs / 3.6e6) : null; }
// The model is what the status line reports; the effort is the lane's launch option
// (its template's, else its lane type's), since nothing reports it back.
function laneModel(x) {
  const s = x.lv && x.lv.sessions.find((v) => v.model);
  const tpl = x.t && x.t.template ? (S.templates || []).find((v) => v.id === x.t.template) : null;
  const ty = (S.laneTypes || []).find((v) => v.name === x.type);
  return { model: (s && s.model) || (tpl && tpl.model) || (ty && ty.model) || "", effort: (tpl && tpl.effort) || (ty && ty.effort) || "" };
}
function laneNeeds(x) { return S.needsYou.filter((n) => (x.t ? n.terminal === x.id : !n.terminal && n.lane === x.path)); }
function laneAlerts(x) {
  const asked = new Set(laneNeeds(x).map((n) => n.session));
  return (S.alerts || []).filter((a) => (x.t ? a.terminal === x.id : !a.terminal && a.lane === x.path) && !(a.kind === "waiting" && asked.has(a.session)));
}
function laneFeed(x) { return x.wt ? S.feed.filter((e) => e.lane === x.wt) : []; }
function lanePR(x) { return x.branch ? (S.prs || []).find((p) => p.headRefName === x.branch) : null; }
function laneGate(x) {
  for (const q of S.queues || []) {
    if (q.holder && q.holder.lane === x.id) return { q, pos: 0, since: q.holder.started };
    const i = q.waiters.findIndex((w) => w.lane === x.id);
    if (i >= 0) return { q, pos: i + 1, since: q.waiters[i].started };
  }
  return null;
}
function laneFailure(x) { const s = x.lv && x.lv.sessions.find((v) => v.failure); return s ? s.failure : ""; }
function abnormal(x) {
  const s = laneState(x);
  if (s === "dead" || s === "orphaned" || s === "stale" || laneFailure(x)) return "crit";
  return s === "waiting" ? "warn" : "";
}
function tail(p, n) { n = n || 44; return p && p.length > n ? "…" + p.slice(-(n - 1)) : p || ""; }
function relPath(p) {
  if (!p || !S.root) return p || "";
  if (p === S.root) return "project root";
  return p.startsWith(S.root + "/") ? p.slice(S.root.length + 1) : p;
}

function selectLane(k, how) {
  selKey = k;
  try { localStorage.setItem("clauductor-panel-sel", k); } catch (e) {}
  confirmAct = null;
  render();
  if (how === "tab") focusKey("tab:" + k);
}

// A clickable row or node is reachable and operable from the keyboard too.
function pressable(e, label) {
  e.tabIndex = 0;
  if (label) e.setAttribute("aria-label", label);
  on(e, "keydown", (ev) => {
    if (ev.target === e && (ev.key === "Enter" || ev.key === " ")) { ev.preventDefault(); e.click(); }
  });
}
function heading(text, k, count, tag) {
  const h = el(tag || "h2", null, text);
  if (count != null) h.appendChild(el("span", "cnt", " " + count));
  return key(h, k);
}
function paneHead(k, text, count, extra) {
  return key(el("div", "pane-h", null, [heading(text, k + ":h", count), el("span", "sp"), ...(extra || [])]), k);
}

// ---- The lane table: sortable, its columns chosen per viewer ------------------------------
// Every column is a figure the panel already has; none costs a model token.
const COLS = [
  { id: "state", label: "State", def: true, sort: (x) => STATE_ORDER[laneState(x)] ?? 9, cell: (x) => stateMark(x) },
  { id: "time", label: "For", def: true, num: true, sort: (x) => -(laneM(x).stateSince || 0), cell: (x) => (laneM(x).stateSince ? el("span", "num", null, [age(laneM(x).stateSince)]) : el("span", "dim", "—")) },
  { id: "ctx", label: "Ctx", def: true, num: true, sort: (x) => laneCtx(x) ?? -1, cell: (x) => num(pct(laneCtx(x))) },
  { id: "cache", label: "Cache", def: true, num: true, sort: (x) => laneM(x).cacheHitRatio ?? -1, cell: cacheCell },
  { id: "costh", label: "$/h", def: true, num: true, sort: (x) => (x.lv && x.lv.costPerH) ?? -1, cell: (x) => num(x.lv && x.lv.costPerH != null ? money(x.lv.costPerH) : "—") },
  { id: "cost", label: "Cost", num: true, sort: (x) => laneCost(x) ?? -1, cell: (x) => num(money(laneCost(x))) },
  { id: "model", label: "Model", sort: (x) => laneModel(x).model, cell: (x) => el("span", null, laneModel(x).model || "—") },
  { id: "busy", label: "Busy", num: true, sort: (x) => busyRatio(laneM(x)) ?? -1, cell: (x) => num(busyRatio(laneM(x)) == null ? "—" : Math.round(busyRatio(laneM(x)) * 100) + "%") },
  { id: "lines", label: "Lines", num: true, sort: (x) => (laneM(x).linesAdded || 0) + (laneM(x).linesRemoved || 0), cell: (x) => num("+" + (laneM(x).linesAdded || 0) + " −" + (laneM(x).linesRemoved || 0)) },
  { id: "linesh", label: "Lines/h", num: true, sort: (x) => perHour((laneM(x).linesAdded || 0) + (laneM(x).linesRemoved || 0), laneM(x)) ?? -1, cell: (x) => rateCell(perHour((laneM(x).linesAdded || 0) + (laneM(x).linesRemoved || 0), laneM(x))) },
  { id: "turnsh", label: "Turns/h", num: true, sort: (x) => perHour(laneM(x).turns || 0, laneM(x)) ?? -1, cell: (x) => rateCell(perHour(laneM(x).turns || 0, laneM(x)), 1) },
  { id: "asksh", label: "Asks/h", num: true, sort: (x) => perHour(laneM(x).asks || 0, laneM(x)) ?? -1, cell: (x) => rateCell(perHour(laneM(x).asks || 0, laneM(x)), 1) },
  { id: "compact", label: "Compactions", num: true, sort: (x) => laneM(x).compactions || 0, cell: (x) => num(String(laneM(x).compactions || 0)) },
  { id: "fail", label: "Last failure", sort: (x) => laneFailure(x), cell: (x) => (laneFailure(x) ? el("span", "crit", laneFailure(x)) : el("span", "dim", "—")) },
  { id: "agents", label: "Agents", num: true, sort: (x) => laneM(x).subagentsRunning || 0, cell: (x) => num(String(laneM(x).subagentsRunning || 0)) },
  { id: "git", label: "Git", sort: (x) => (x.lv && x.lv.git ? x.lv.git.dirty : -1), cell: gitCell },
  { id: "pr", label: "PR", sort: (x) => (lanePR(x) ? lanePR(x).number : 0), cell: (x) => (lanePR(x) ? prCell(lanePR(x), true) : el("span", "dim", "—")) },
  { id: "gate", label: "Gate", sort: (x) => { const g = laneGate(x); return g ? g.pos : 99; }, cell: gateCell },
  { id: "cpu", label: "CPU", num: true, sort: (x) => laneM(x).cpu ?? -1, cell: (x) => num(laneM(x).cpu == null ? "—" : Math.round(laneM(x).cpu) + "%") },
];
function busyRatio(m) { return m.durationMs >= 60000 && m.apiDurationMs != null ? Math.min(1, (m.apiDurationMs || 0) / m.durationMs) : null; }
function rateCell(v, dp) { return num(v == null ? "—" : v.toFixed(dp == null ? 0 : dp)); }
function cacheCell(x) {
  const m = laneM(x);
  const kids = [num(m.cacheHitRatio == null ? "—" : Math.round(m.cacheHitRatio * 100) + "%")];
  if (m.cacheExpiresAt) {
    const left = m.cacheExpiresAt - now();
    if (left <= 0) kids.push(el("span", "dim", " cold"));
    else if (left < 120000) kids.push(document.createTextNode(" "), until(m.cacheExpiresAt, "cold in ", 120000));
  }
  return el("span", null, null, kids);
}
function gitCell(x) {
  const g = x.lv && x.lv.git;
  if (!g) return el("span", "dim", "—");
  if (g.error && !g.at) return el("span", "crit", "cannot read");
  const bits = [];
  if (g.hasUpstream) bits.push("↑" + g.ahead + " ↓" + g.behind);
  bits.push(g.dirty ? g.dirty + " changed" : "clean");
  return num(bits.join(" "));
}
function gateCell(x) {
  const g = laneGate(x);
  if (!g) return el("span", "dim", "—");
  return g.pos === 0 ? el("span", null, "holds") : el("span", null, null, [document.createTextNode("waits " + g.pos + " "), el("span", "num", null, [age(g.since)])]);
}
function prCell(p, short) {
  const checks = p.checksFail ? el("span", "crit", p.checksFail + " failing") : p.checksPending ? el("span", null, p.checksPending + " pending")
    : p.checksPass ? el("span", null, p.checksPass + " passing") : el("span", "dim", "no checks");
  const kids = [num("#" + p.number)];
  if (!short) kids.push(document.createTextNode(p.isDraft ? " draft " : " "), checks);
  else if (p.checksFail) kids.push(document.createTextNode(" "), el("span", "crit", "failing"));
  if (!short && p.review) kids.push(el("span", p.review === "CHANGES_REQUESTED" ? "warn" : null, " " + p.review.toLowerCase().replace(/_/g, " ")));
  return el("span", null, null, kids);
}

let cols = COLS.filter((c) => c.def).map((c) => c.id), sortBy = { col: "state", dir: 1 }, colMenu = false;
try {
  const c = JSON.parse(localStorage.getItem("clauductor-panel-cols") || "null");
  if (Array.isArray(c) && c.every((id) => COLS.some((x) => x.id === id))) cols = c;
  const s = JSON.parse(localStorage.getItem("clauductor-panel-sort") || "null");
  if (s && COLS.some((x) => x.id === s.col) && (s.dir === 1 || s.dir === -1)) sortBy = s;
} catch (e) {}
function saveCols() { try { localStorage.setItem("clauductor-panel-cols", JSON.stringify(cols)); localStorage.setItem("clauductor-panel-sort", JSON.stringify(sortBy)); } catch (e) {} }

function laneTable(ls, cur) {
  const shown = COLS.filter((c) => cols.includes(c.id));
  const byCol = COLS.find((c) => c.id === sortBy.col) || COLS[0];
  const rows = ls.slice().sort((a, b) => {
    const x = byCol.sort(a), y = byCol.sort(b);
    return (x < y ? -1 : x > y ? 1 : a.name < b.name ? -1 : 1) * sortBy.dir;
  });
  const th = (c, label, isNum) => {
    const h = el("th", isNum ? "num" : null);
    const b = el("button", null, label);
    b.type = "button";
    b.title = "Sort by " + label.toLowerCase();
    on(b, "click", () => { sortBy = sortBy.col === c ? { col: c, dir: -sortBy.dir } : { col: c, dir: 1 }; saveCols(); render(); });
    h.appendChild(key(b, "sort:" + c));
    h.setAttribute("aria-sort", sortBy.col === c ? (sortBy.dir > 0 ? "ascending" : "descending") : "none");
    h.scope = "col";
    return key(h, "th:" + c);
  };
  const head = el("tr", null, null, [th("state", "State"), th("lane", "Lane"), ...shown.filter((c) => c.id !== "state").map((c) => th(c.id, c.label, c.num))]);
  if (!cols.includes("state")) head.removeChild(head.firstChild);
  const body = el("tbody");
  for (const x of rows) {
    const ab = abnormal(x), sel = cur && x.key === cur.key;
    const tr = el("tr", (ab ? "abn-" + ab : "") + (sel ? " sel" : ""));
    if (cols.includes("state")) tr.appendChild(el("td", null, null, [stateMark(x)]));
    const nm = el("td", "lane-nm", x.name);
    nm.title = x.name + ", " + (x.branch || "detached") + ", " + x.path;
    tr.appendChild(nm);
    for (const c of shown) if (c.id !== "state") tr.appendChild(el("td", c.num ? "num" : null, null, [c.cell(x)]));
    on(tr, "click", () => selectLane(x.key));
    pressable(tr, x.name + ", " + laneStatusText(x));
    if (sel) tr.setAttribute("aria-selected", "true");
    body.appendChild(key(tr, "row:" + x.key));
  }
  const tbl = el("table", "tbl", null, [el("thead", null, null, [head]), body]);
  tbl.setAttribute("aria-label", "Lanes, sorted by " + byCol.label.toLowerCase());
  return key(el("div", "tblwrap", null, [tbl]), "lanetbl");
}
function colPicker() {
  const b = button("Columns", "small", () => { colMenu = !colMenu; render(); }, "Choose the lane table's columns", "cols");
  b.setAttribute("aria-expanded", String(colMenu));
  const kids = [b];
  if (colMenu) {
    const m = el("div", "colmenu");
    m.setAttribute("role", "group");
    m.setAttribute("aria-label", "Lane table columns");
    for (const c of COLS) {
      const cb = el("input");
      cb.type = "checkbox";
      cb.checked = cols.includes(c.id);
      on(cb, "change", () => {
        cols = cb.checked ? COLS.filter((x) => x.id === c.id || cols.includes(x.id)).map((x) => x.id) : cols.filter((id) => id !== c.id);
        saveCols(); render();
      });
      m.appendChild(key(el("label", null, null, [key(cb, "cb"), document.createTextNode(c.label)]), "col:" + c.id));
    }
    on(m, "keydown", (ev) => { if (ev.key === "Escape") { colMenu = false; render(); focusKey("cols"); } });
    kids.push(key(m, "colmenu"));
  }
  return key(el("div", "colpick", null, kids), "colpick");
}

// The lanes' states over the last two hours, one line each.
function timeline(ls) {
  const t1 = now(), t0 = t1 - 2 * 3600e3;
  const rows = [];
  for (const x of ls) {
    const segs = (x.lv && x.lv.timeline) || [];
    if (!segs.length) continue;
    const bar = el("span", "tl");
    segs.forEach((s, i) => {
      const from = Math.max(t0, s.from), to = i + 1 < segs.length ? segs[i + 1].from : t1;
      if (to <= t0) return;
      const seg = el("i", s.state);
      seg.style.left = (((from - t0) / (t1 - t0)) * 100).toFixed(2) + "%";
      seg.style.width = Math.max(0.3, ((to - from) / (t1 - t0)) * 100).toFixed(2) + "%";
      seg.title = (STATE_WORD[s.state] || s.state) + " from " + hm(s.from);
      bar.appendChild(seg);
    });
    rows.push(key(el("div", "tl-row", null, [el("span", null, x.name), bar]), "tl:" + x.key));
  }
  if (!rows.length) return null;
  rows.push(key(el("div", "tl-axis", null, [el("span", null, "2 h ago"), el("span", null, "1 h"), el("span", null, "now")]), "tl:axis"));
  return key(el("div", "timeline", null, rows), "timeline");
}

// ---- The rail: the lane table, then the worktree tree ------------------------------------
function renderRail(ls, cur) {
  const kids = [];
  const nb = button("New lane", "primary small", () => openStart(), "Start a lane: an interactive claude in its own tmux session", "rail:new", true);
  if (S.startBlocked) { nb.disabled = true; nb.title = S.startBlocked; }
  kids.push(paneHead("lanes", "Lanes" + asOf(), ls.length, [colPicker(), nb]));
  kids.push(sourceNote(S.sources.agents, "claude agents", "src:agents"));
  if (!ls.length) kids.push(key(el("div", "empty", "No lane yet. Sessions started in this project's worktrees show here too."), "lanes:none"));
  else { kids.push(timeline(ls)); kids.push(laneTable(ls, cur)); }
  patchInto("lanelist", kids);
  patchInto("tree", renderTree(ls, cur));
}

// The worktree tree, as the old control room's topology had it: the project, each
// worktree (with or without a lane), each lane's claude session (state, uptime,
// context), and the agents under it, nested by which agent started which (see
// state/nesting.go): exact once the starting call returns, ≈ until then; workflow
// agents under their run, always ≈. Finished agents fold under a count.
let treeOpen = {};
try { treeOpen = JSON.parse(localStorage.getItem("clauductor-panel-tree") || "{}") || {}; } catch (e) {}
function foldable(k, summaryText, kids) {
  const d = el("details", null, null, [el("summary", null, summaryText), ...kids]);
  if (treeOpen[k]) d.open = true;
  on(d, "toggle", () => {
    if (d.open) treeOpen[k] = 1; else delete treeOpen[k];
    try { localStorage.setItem("clauductor-panel-tree", JSON.stringify(treeOpen)); } catch (e) {}
  });
  return key(d, k);
}
// A node is two lines: what it is (a mark, a tag, a name), then its figures.
function nodeEl(tag, cls, first, second) {
  const n = el(tag, "node " + cls, null, [el("span", "nl", null, first.filter(Boolean))]);
  const bits = (second || []).filter(Boolean);
  if (bits.length) {
    const t = el("span", "nt");
    bits.forEach((b, i) => { if (i) t.appendChild(document.createTextNode(", ")); t.appendChild(typeof b === "string" ? document.createTextNode(b) : b); });
    n.appendChild(t);
  }
  return n;
}
// agentForest arranges a session's agents: roots (started by the session, or whose
// starter is unknown), workflow runs as groups, children under their starter.
function agentForest(list) {
  const ids = new Set(list.map((a) => a.id));
  const kids = new Map(), runs = new Map(), roots = [];
  for (const a of list) {
    if (a.run) { if (!runs.has(a.run)) runs.set(a.run, { id: a.run, name: a.runName, agents: [] }); runs.get(a.run).agents.push(a); }
    else if (a.parent && ids.has(a.parent)) { if (!kids.has(a.parent)) kids.set(a.parent, []); kids.get(a.parent).push(a); }
    else roots.push(a);
  }
  return { roots, kids, runs: [...runs.values()] };
}
function agentLabel(a) { return (a.type || "untyped") + (a.description ? ": " + a.description : ""); }
function agentNodes(list, prefix) {
  const f = agentForest(list);
  const one = (a, depth) => {
    const done = !!a.ended;
    const n = nodeEl("span", "agent", [el("span", "sq " + (done ? "idle" : "busy")), el("span", "nn", agentLabel(a))],
      [el("span", "num", a.id.slice(0, 7)), done ? "ran " + dur(a.ended - a.since) : age(a.since, "running "), a.parentApprox ? "≈ starter" : null]);
    const li = el("li", null, null, [n]);
    const ch = f.kids.get(a.id);
    if (ch && depth < 4) li.appendChild(el("ul", null, null, ch.map((c) => one(c, depth + 1))));
    return key(li, prefix + a.id + (a.ended || ""));
  };
  const out = f.roots.map((a) => one(a, 0));
  for (const r of f.runs) {
    const n = nodeEl("span", "run", [el("span", "nn", "Workflow " + (r.name || r.id))], [el("span", "num", r.id), "≈ grouped by timing"]);
    out.push(key(el("li", null, null, [n, el("ul", null, null, r.agents.map((a) => one(a, 1)))]), prefix + "run:" + r.id));
  }
  return out;
}
function sessionNode(x, s, lv) {
  const st = s ? s.status : laneState(x);
  const since = s && s.startedAt ? s.startedAt : laneSince(x);
  const ctx = s ? s.ctxPct : laneCtx(x);
  const n = nodeEl("span", "sess", [el("span", "sq " + st), el("span", "nn", (s && s.name) || "claude")],
    [(s && s.approx ? APPROX : "") + (STATE_WORD[st] || st), since ? age(since, "up ") : null, "context " + pct(ctx)]);
  const li = el("li", null, null, [n]);
  const running = lv ? lv.subagents.filter((a) => !s || a.session === s.id) : [];
  const fin = lv ? (lv.finishedSubagents || []).filter((a) => !s || a.session === s.id) : [];
  const ul = el("ul", null, null, agentNodes(running, "ta:"));
  if (fin.length) ul.appendChild(key(el("li", null, null, [foldable("tf:" + x.key + ":" + (s ? s.id : ""), fin.length + " finished", [el("ul", null, null, agentNodes(fin, "tfa:"))])]), "tfin"));
  if (ul.childNodes.length) li.appendChild(ul);
  return key(li, "ts:" + x.key + ":" + (s ? s.id : "none"));
}
function renderTree(ls, cur) {
  const wts = S.lanes.concat(S.quietWorktrees).slice().sort((a, b) => (a.path === S.root ? -1 : b.path === S.root ? 1 : a.path < b.path ? -1 : 1));
  const kids = [paneHead("treeh", "Worktrees", wts.length)];
  kids.push(sourceNote(S.sources.worktrees, "git worktree list", "src:wt"));
  const root = nodeEl("span", "root", [el("span", "nn", S.name)], [ls.length + " lane" + (ls.length === 1 ? "" : "s"), wts.length + " worktree" + (wts.length === 1 ? "" : "s")]);
  root.title = S.root;
  const top = el("ul");
  const placed = new Set();
  for (const w of wts) {
    const mine = ls.filter((x) => x.wt === w.path);
    mine.forEach((x) => placed.add(x.key));
    const sel = cur && mine.some((x) => x.key === cur.key);
    const g = w.git;
    const first = [el("span", "tag", w.type), el("span", "nn", w.branch || "detached")];
    const second = [w.path === S.root ? "project root" : relPath(w.path)];
    if (g && !g.error) {
      if (g.hasUpstream) second.push(el("span", "num", "↑" + g.ahead + " ↓" + g.behind));
      second.push(g.dirty ? g.dirty + " changed" : "clean");
    }
    let node;
    if (mine.length) {
      node = nodeEl("button", "wt" + (sel ? " sel" : ""), first, second);
      node.type = "button";
      node.title = "Show lane " + mine[0].name + ", " + w.path;
      on(node, "click", () => selectLane(mine[0].key));
      if (sel) node.setAttribute("aria-current", "true");
    } else {
      node = nodeEl("span", "wt", first, ["no lane"].concat(second));
      node.title = w.path;
    }
    const li = el("li", null, null, [key(node, "wtn")]);
    if (!mine.length) {
      const b = button("Start lane here", "small", () => openStart({ worktree: w.path }), "Start a lane in " + w.path, "wt:start", true);
      if (S.startBlocked) b.disabled = true;
      li.appendChild(b);
    } else {
      const ul = el("ul");
      for (const x of mine) {
        const ss = x.lv ? x.lv.sessions : [];
        if (ss.length) for (const s of ss) ul.appendChild(sessionNode(x, s, x.lv));
        else ul.appendChild(sessionNode(x, null, x.lv));
      }
      li.appendChild(ul);
    }
    top.appendChild(key(li, "tw:" + w.path));
  }
  // A lane whose directory is in none of the project's worktrees still shows.
  for (const x of ls) {
    if (placed.has(x.key)) continue;
    const node = nodeEl("button", "wt" + (cur && cur.key === x.key ? " sel" : ""), [el("span", "tag", x.type || "lane"), el("span", "nn", x.name)], ["outside the worktrees"]);
    node.type = "button";
    on(node, "click", () => selectLane(x.key));
    top.appendChild(key(el("li", null, null, [key(node, "wtn"), el("ul", null, null, [sessionNode(x, null, x.lv)])]), "tx:" + x.key));
  }
  kids.push(key(el("div", "tree", null, [el("div", null, null, [root]), top]), "treebody"));
  return kids;
}

// ---- Terminal tabs: one per lane, then "+" -------------------------------------------------
// A tablist with a roving tabindex: Tab reaches the selected tab only; Left/Right,
// Home and End move and select at once. Selecting never enters the terminal.
function renderTabs(ls, cur) {
  const tabs = ls.map((x) => {
    const selected = cur && x.key === cur.key;
    const s = laneState(x);
    const kids = [el("span", "sq " + s), el("span", null, x.name)];
    if (x.type && x.type !== x.name) kids.push(el("span", "ty", x.type));
    const tab = el("div", "tab", null, kids);
    tab.setAttribute("role", "tab");
    tab.setAttribute("aria-selected", String(!!selected));
    tab.setAttribute("aria-controls", "wsbody");
    tab.id = "tab-" + x.key.replace(/[^A-Za-z0-9_-]/g, "_");
    tab.tabIndex = selected ? 0 : -1;
    tab.setAttribute("aria-label", x.name + ", " + (laneApprox(x) ? APPROX : "") + stateWord(x) + (x.t ? "" : ", no terminal here"));
    tab.title = (x.branch || "") + ", " + x.path + ", " + laneStatusText(x) + (x.t && x.t.orphan ? ", " + x.t.orphan : "") + (laneApprox(x) ? ", " + APPROX_TITLE : "");
    on(tab, "click", () => selectLane(x.key));
    on(tab, "keydown", (ev) => {
      const i = ls.findIndex((y) => y.key === x.key);
      let j = -1;
      if (ev.key === "ArrowRight") j = (i + 1) % ls.length;
      else if (ev.key === "ArrowLeft") j = (i - 1 + ls.length) % ls.length;
      else if (ev.key === "Home") j = 0;
      else if (ev.key === "End") j = ls.length - 1;
      else if (ev.key === "Enter" || ev.key === " ") { ev.preventDefault(); selectLane(x.key, "tab"); return; }
      if (j < 0) return;
      ev.preventDefault();
      selectLane(ls[j].key, "tab");
    });
    return key(tab, "tab:" + x.key);
  });
  patchInto("tabs", tabs);
  if (cur) $("wsbody").setAttribute("aria-labelledby", "tab-" + cur.key.replace(/[^A-Za-z0-9_-]/g, "_"));
  else $("wsbody").removeAttribute("aria-labelledby");
  const add = $("addtab");
  add.disabled = !!S.startBlocked || offline();
  add.title = offline() ? "Disconnected from the panel" : S.startBlocked || "Start a new lane";
}

// ---- The lane's header ----------------------------------------------------------------------
let sideOpen = true;
try { sideOpen = localStorage.getItem("clauductor-panel-side") !== "closed"; } catch (e) {}
function kv1(k, v) { return el("span", "kv1", null, [el("span", "k", k), v]); }
function renderLaneHead(x) {
  const kids = [];
  if (x) {
    kids.push(key(el("h2", null, x.name), "name"));
    kids.push(key(stateMark(x), "state"));
    const where = el("span", "where", null, [document.createTextNode((x.branch || "detached") + " in " + relPath(x.path))]);
    where.title = x.path;
    kids.push(key(where, "where"));
    const m = laneModel(x), lm = laneM(x), since = laneSince(x), ctx = laneCtx(x);
    const mode = [m.effort ? "effort " + m.effort : "", lm.thinking ? "thinking" : "", lm.fastMode ? "fast" : ""].filter(Boolean).join(", ");
    if (m.model || mode) kids.push(key(kv1("Model", el("span", null, (m.model || "unknown") + (mode ? ", " + mode : ""))), "model"));
    if (since) kids.push(key(kv1("Up", el("span", "num", null, [age(since)])), "up"));
    kids.push(key(kv1("Context", el("span", null, null, [bullet(ctx, { tick: S.trends.autocompactPct, cls: ctx >= 85 ? "w" : "" }), document.createTextNode(" "), num(pct(ctx))])), "ctx"));
    kids.push(key(kv1("Cost", el("span", null, null, [num(money(laneCost(x))), x.lv && x.lv.costPerH != null ? num(" " + money(x.lv.costPerH), "/h") : null])), "cost"));
    if (laneApprox(x)) kids.push(key(el("span", "dim", null, [staleTag(true)]), "stale"));
  } else kids.push(key(el("h2", null, "No lane selected"), "name"));
  const tog = button(sideOpen ? "Hide details" : "Show details", "small", () => {
    sideOpen = !sideOpen;
    try { localStorage.setItem("clauductor-panel-side", sideOpen ? "open" : "closed"); } catch (e) {}
    render();
  }, "Show or hide this lane's agents, figures, git, gate, alerts and activity", "sidetog");
  tog.setAttribute("aria-expanded", String(sideOpen));
  tog.setAttribute("aria-controls", "side");
  kids.push(key(el("span", "sp"), "sp"), tog);
  patchInto("lanehead", kids);
  $("wsgrid").classList.toggle("noside", !sideOpen);
}

// ---- The lane's side panel: tabs between metric families ------------------------------------
const SIDE_TABS = [["agents", "Agents"], ["figures", "Figures"], ["git", "Git"], ["gate", "Gate"], ["alerts", "Alerts"], ["activity", "Activity"]];
let sideTab = "agents";
try { const t = localStorage.getItem("clauductor-panel-sidetab"); if (SIDE_TABS.some((x) => x[0] === t)) sideTab = t; } catch (e) {}
function kvRows(rows) {
  const box = el("div", "kv");
  for (const [k, v] of rows) {
    if (v == null) continue;
    box.appendChild(el("span", "k", k));
    box.appendChild(el("span", "v", null, (Array.isArray(v) ? v : [v]).filter(Boolean).map((n) => (typeof n === "string" ? document.createTextNode(n) : n))));
  }
  return box;
}
function renderSide(x) {
  if (!x) { patchInto("side", [key(el("div", "empty", "Select a lane, or start one."), "side:none")]); return; }
  const needs = laneNeeds(x), als = laneAlerts(x);
  const nAl = needs.length + als.length;
  const counts = { agents: laneM(x).subagentsRunning || 0, alerts: nAl };
  const tl = el("div", "sidetabs");
  tl.setAttribute("role", "tablist");
  tl.setAttribute("aria-label", "This lane");
  SIDE_TABS.forEach(([id, label], i) => {
    const b = el("button", null, label);
    b.type = "button";
    b.setAttribute("role", "tab");
    b.setAttribute("aria-selected", String(id === sideTab));
    b.tabIndex = id === sideTab ? 0 : -1;
    if (counts[id]) b.appendChild(el("span", "cnt" + (id === "alerts" ? (needs.some((n) => n.severity === "block") ? " c" : " w") : ""), String(counts[id])));
    on(b, "click", () => { sideTab = id; try { localStorage.setItem("clauductor-panel-sidetab", id); } catch (e) {} render(); });
    on(b, "keydown", (ev) => {
      let j = -1;
      if (ev.key === "ArrowRight") j = (i + 1) % SIDE_TABS.length; else if (ev.key === "ArrowLeft") j = (i - 1 + SIDE_TABS.length) % SIDE_TABS.length;
      if (j < 0) return;
      ev.preventDefault();
      sideTab = SIDE_TABS[j][0];
      render();
      focusKey("stab:" + sideTab);
    });
    tl.appendChild(key(b, "stab:" + id));
  });
  const body = [];
  if (sideTab === "agents") body.push(...sideAgents(x));
  else if (sideTab === "figures") body.push(sideFigures(x));
  else if (sideTab === "git") body.push(...sideGit(x));
  else if (sideTab === "gate") body.push(...sideGate(x));
  else if (sideTab === "alerts") body.push(...sideAlerts(x, needs, als));
  else body.push(feedList(laneFeed(x).slice(0, 60), false, "lanefeed"));
  const panel = el("div", "sidebody", null, body);
  panel.setAttribute("role", "tabpanel");
  patchInto("side", [key(tl, "sidetabs"), key(panel, "sidebody:" + sideTab)]);
}
// Agents: the sessions, then a Gantt of the agents over the lane's window, nested by
// which agent started which.
function sideAgents(x) {
  const lv = x.lv, out = [];
  if (!lv) return [key(el("div", "empty", "Its directory is in none of the project's worktrees, so no session is matched to it."), "ag:nolv")];
  for (const s of lv.sessions) {
    const m = s.metrics || {};
    out.push(key(kvRows([
      ["Session", [el("span", "num", s.name || s.id.slice(0, 8)), s.pid ? el("span", "num dim", "pid " + s.pid) : null, el("span", null, s.kind || "hooks only")]],
      ["State", [el("span", "state-word " + s.status, (s.approx ? APPROX : "") + (STATE_WORD[s.status] || s.status)), s.waitingFor ? el("span", null, s.waitingFor) : null,
        m.stateSince ? el("span", "num dim", null, [age(m.stateSince, "for ")]) : null]],
      s.compacting ? ["Compacting", s.compacting] : [null, null],
      s.failure ? ["Last failure", el("span", "crit", s.failure)] : [null, null],
      s.unknownNotification ? ["Unknown notification", s.unknownNotification] : [null, null],
      s.agentState ? ["Agent state", s.agentState] : [null, null],
    ].filter((r) => r[0])), "sess:" + s.id));
  }
  if (!lv.sessions.length) out.push(key(el("div", "empty", x.t ? "No claude session reported yet." : "No session."), "ag:nosess"));
  if (lv.subagentsApprox) out.push(key(el("div", "approx", "Approximate: agent pairing was verified on Claude Code " + S.observe.verifiedOn + ", and " + (S.observe.claudeVersion || "an unknown version") + " is running."), "ag:approx"));
  const all = lv.subagents.concat(lv.finishedSubagents || []);
  if (!all.length) { out.push(key(el("div", "empty", "No subagent has run in this lane yet."), "ag:none")); return out; }
  out.push(key(el("h3", null, lv.subagents.length + " running, " + (lv.finishedSubagents || []).length + " finished"), "ag:h"));
  const t1 = now(), t0 = Math.max(Math.min(...all.map((a) => a.since)), t1 - 2 * 3600e3), span = Math.max(60000, t1 - t0);
  const rows = [];
  const row = (a, depth, run) => {
    const done = !!a.ended;
    const bar = el("span", "g-bar", null, [el("i")]);
    const from = Math.max(t0, a.since), to = done ? a.ended : t1;
    bar.firstChild.style.left = (((from - t0) / span) * 100).toFixed(2) + "%";
    bar.firstChild.style.width = Math.max(0.5, ((to - from) / span) * 100).toFixed(2) + "%";
    const nm = el("span", "g-nm", null, [document.createTextNode(" ".repeat(depth * 2) + agentLabel(a) + " "), el("span", "id num", a.id.slice(0, 7))]);
    nm.title = agentLabel(a) + ", " + a.id + (a.parentApprox ? ", starter paired by timing" : "") + (run ? ", workflow run " + run : "");
    rows.push(key(el("div", "g-row" + (done ? " done" : ""), null, [nm, bar, el("span", "num", done ? dur(a.ended - a.since) : null, done ? [] : [age(a.since)])]), "g:" + a.id + (a.ended || "")));
  };
  const walk = (list, depth, run) => {
    const f = agentForest(list);
    const one = (a, d) => { row(a, d, run); for (const c of f.kids.get(a.id) || []) one(c, d + 1); };
    f.roots.forEach((a) => one(a, depth));
    for (const r of f.runs) {
      rows.push(key(el("div", "g-row run", null, [el("span", "g-nm", " ".repeat(depth * 2) + "Workflow " + (r.name || r.id) + " ≈"), el("span"), el("span", "num", String(r.agents.length))]), "g:run:" + r.id));
      walk(r.agents.map((a) => Object.assign({}, a, { run: "" })), depth + 1, r.id);
    }
  };
  walk(all.slice().sort((a, b) => a.since - b.since), 0, "");
  out.push(key(el("div", "gantt", null, rows), "gantt"));
  return out;
}
// Figures: the lane's numbers, each with its unit.
function sideFigures(x) {
  const m = laneM(x), lv = x.lv || {};
  const hitSp = spark(lv.hitSpark, { lo: 0, hi: 1 }), costSp = spark(lv.costSpark);
  const cold = m.cacheExpiresAt ? (m.cacheExpiresAt - now() > 0 ? until(m.cacheExpiresAt, "cold in ", 120000) : el("span", null, "cold")) : null;
  const rows = [
    ["State", [stateMark(x), m.stateSince ? el("span", "num dim", null, [age(m.stateSince, "for ")]) : null]],
    ["Context", [bullet(m.ctxPct, { tick: S.trends.autocompactPct, cls: m.ctxPct >= 85 ? "w" : "" }), num(pct(m.ctxPct)), m.inputTokens != null ? num(kilo(m.inputTokens), "in") : null,
      m.outputTokens != null ? num(kilo(m.outputTokens), "out") : null, m.ctxWindow ? num(kilo(m.ctxWindow), "window") : null]],
    ["Cache", [num(m.cacheHitRatio == null ? "—" : Math.round(m.cacheHitRatio * 100) + "%", "hits"), hitSp, cold]],
    m.lastMissCause ? ["Last miss", m.lastMissCause] : null,
    m.recacheTokens ? ["If cold", num(kilo(m.recacheTokens), "tokens to rebuild")] : null,
    ["Cost", [num(money(m.costUsd)), lv.costPerH != null ? num(money(lv.costPerH), "/h") : null, costSp]],
    ["Busy", busyRatio(m) == null ? "—" : [num(Math.round(busyRatio(m) * 100) + "%"), el("span", "dim", "of the time in the API")]],
    ["Time", m.durationMs ? [num(dur(m.durationMs), "wall"), num(dur(m.apiDurationMs || 0), "API")] : "—"],
    ["Lines", [num("+" + (m.linesAdded || 0) + " −" + (m.linesRemoved || 0)), rateCell(perHour((m.linesAdded || 0) + (m.linesRemoved || 0), m)), el("span", "unit", "/h")]],
    ["Turns", [num(String(m.turns || 0)), rateCell(perHour(m.turns || 0, m), 1), el("span", "unit", "/h")]],
    ["Asked you", [num(String(m.asks || 0)), rateCell(perHour(m.asks || 0, m), 1), el("span", "unit", "/h")]],
    ["Compactions", num(String(m.compactions || 0))],
    laneFailure(x) ? ["Last failure", el("span", "crit", laneFailure(x))] : null,
    ["Subagents", [num(String(m.subagentsRunning || 0), "running"), num(String(m.subagentsFinished || 0), "finished")]],
    ["Process", m.cpu == null ? el("span", "dim", S.trends.procsError ? "cannot read: " + S.trends.procsError : "read while this page is in view") : [num(Math.round(m.cpu) + "%", "CPU"), num(Math.round(m.rssMb || 0), "MB")]],
    ["Mode", [m.thinking ? "thinking" : "", m.fastMode ? "fast" : "", m.outputStyle || ""].filter(Boolean).join(", ") || "—"],
  ].filter(Boolean);
  return key(kvRows(rows), "figs");
}
function sideGit(x) {
  const lv = x.lv || {}, g = lv.git, out = [];
  const rows = [["Branch", x.branch || "detached"], ["HEAD", el("span", "num", lv.head ? lv.head.slice(0, 10) : "—")], ["Path", el("span", null, relPath(x.path))]];
  if (!g) rows.push(["Status", el("span", "dim", "read every 30 s while this page is in view")]);
  else if (g.error && !g.at) rows.push(["Status", el("span", "crit", "cannot read: " + g.error)]);
  else {
    if (g.error) rows.push(["Status", el("span", "crit", "last read failed: " + g.error)]);
    rows.push(["Upstream", g.hasUpstream ? [el("span", null, g.upstream), num("↑" + g.ahead, "ahead"), num("↓" + g.behind, "behind")] : el("span", "dim", "none")]);
    rows.push(["Changes", g.dirty ? [num(String(g.changed), "changed"), num(String(g.untracked), "untracked"), g.conflicts ? el("span", "crit", g.conflicts + " conflicted") : null] : "clean"]);
    if (g.files) rows.push(["Diff", [num(String(g.files), "files"), num("+" + g.insertions), num("−" + g.deletions)]]);
    if (g.lastCommitAt) rows.push(["Last commit", el("span", "num", null, [age(g.lastCommitAt, "", " ago")])]);
    rows.push(["Read", el("span", "num dim", null, [age(g.at, "", " ago")])]);
  }
  if (x.t && x.t.template) rows.push(["Template", x.t.template + ", first prompt " + (x.t.promptState || "—") + (x.t.promptNote ? ": " + x.t.promptNote : "")]);
  out.push(key(kvRows(rows), "git:kv"));
  out.push(key(el("h3", null, "Pull request"), "git:prh"));
  const pr = lanePR(x);
  if (!S.sources.prs.ok && !S.sources.prs.pending) out.push(sourceNote(S.sources.prs, "gh pr list", "wt:prsrc"));
  else if (pr) out.push(key(kvRows([["Number", prCell(pr)], ["Title", pr.title], ["Author", pr.author]]), "git:pr"));
  else out.push(key(el("div", "empty", x.branch ? "No open pull request for this branch." : "No branch."), "git:nopr"));
  return out;
}
function sideGate(x) {
  const qs = S.queues || [];
  if (!qs.length && S.queuesSource.ok) return [key(el("div", "empty", "No queue is configured."), "q:none")];
  return [sourceNote(S.queuesSource, "the queues", "q:src"), ...qs.map((q) => laneQueue(q, x))];
}
function sideAlerts(x, needs, als) {
  const out = [];
  for (const n of needs) out.push(key(el("div", "alert", null, [el("span", n.severity === "block" ? "crit" : "warn", n.severity === "block" ? "Blocking" : "Needs you"),
    el("span", null, (n.label || n.kind) + (n.text ? ": " + n.text : ""), n.at ? [el("span", "num dim", null, [age(n.at, " ", " waiting")])] : [])]), "la:n:" + needKey(n)));
  for (const a of als) {
    const text = el("span", null, a.text);
    if (AGED[a.kind] && a.since) text.appendChild(el("span", "num dim", null, [age(a.since, " ")]));
    out.push(key(el("div", "alert", null, [el("span", a.severity === "block" ? "crit" : "warn", a.kind.replace("_", " ")), text]), "la:" + a.key));
  }
  if (!out.length) out.push(key(el("div", "empty", "Nothing for this lane."), "la:none"));
  return out;
}

// ---- Queues ----------------------------------------------------------------------------
let queueMsg = null;
async function queuePost(q, verb, body) {
  queueMsg = { q, text: verb + "…" }; render();
  try {
    const r = await fetch("/api/queues/" + encodeURIComponent(q) + "/" + verb, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    const j = await r.json().catch(() => ({}));
    queueMsg = { q, text: r.ok ? verb + ": done" + (j.run ? ", log " + j.run.log : "") : (j.error || "HTTP " + r.status), err: !r.ok };
  } catch (e) { queueMsg = { q, text: String(e), err: true }; }
  render();
}
function leaseLine(l) {
  return el("span", null, (l.lane || "pid " + l.pid) + " ", [el("span", "num", null, [age(l.started)]), document.createTextNode(l.cmd ? " " + l.cmd : "")]);
}
function holderLine(q) {
  if (!q.held) return el("div", null, "Free");
  if (!q.holder) return el("div", null, "Held");
  const who = el("span", q.holder.stale ? "crit" : null, "Held by ", [leaseLine(q.holder)]);
  if (q.holder.stale) who.appendChild(document.createTextNode(", stale" + (q.holder.staleWhy ? ": " + q.holder.staleWhy : "")));
  // The lease's time to live, in words: a holder that stops renewing is taken for dead after it.
  return el("div", "who", null, [who, el("span", "dim", q.holder.ttl ? "renews within " + Math.round(q.holder.ttl / 60) + " min" : "")]);
}
function waiterList(q, onlyLane) {
  const ol = el("ol");
  for (const w of q.waiters) {
    const li = el("li", "who", null, [el("span", null, null, [leaseLine(w), document.createTextNode(w.cancelling ? ", cancelling" : "")])]);
    if (!w.cancelling && (!onlyLane || w.lane === onlyLane)) li.appendChild(button("Cancel wait", "small", () => queuePost(q.id, "cancel", { waiter: w.nonce }), "Ask this waiter to give up. The holder is never stopped.", "cancel:" + w.nonce, true));
    ol.appendChild(key(li, "w:" + w.nonce));
  }
  return ol;
}
// Where lane x stands in queue q, then the queue itself, and Run in this lane.
function laneQueue(q, x) {
  const holds = q.holder && q.holder.lane === x.id;
  const wi = q.waiters.findIndex((w) => w.lane === x.id);
  const role = holds ? el("div", null, "This lane holds it" + (q.holder.stale ? " (stale)" : "") + ".")
    : wi >= 0 ? el("div", null, "Waiting, " + (wi + 1) + " of " + q.waiters.length + " in line.")
    : el("div", "dim", "Not in this queue.");
  const c = el("div", "queue", null, [el("h3", null, q.title || q.id), role, holderLine(q)]);
  if (q.holderNote) c.appendChild(el("div", "warn", q.holderNote));
  if (q.waiters.length) { c.appendChild(el("div", "dim", "Waiting, in order:")); c.appendChild(waiterList(q, x.id)); }
  if (q.hasCommand) {
    const b = button("Run in " + x.name, "small", () => queuePost(q.id, "run", { worktree: x.path }),
      "Run this queue's command through lock-run in this lane's worktree; it waits its turn.", "run", true);
    if (!x.path || (S.trust && S.trust.hash && !S.trust.trusted)) b.disabled = true;
    c.appendChild(b);
  }
  if (q.run) c.appendChild(el("div", "dim", "Last run: pid " + q.run.pid + (q.run.exit != null ? ", exit " + q.run.exit : ", running") + ", log " + q.run.log));
  if (queueMsg && queueMsg.q === q.id) c.appendChild(el("div", queueMsg.err ? "crit" : "dim", queueMsg.text));
  return key(c, "lq:" + q.id);
}

// ---- Needs you: rows under the status bar, only when something needs you ------------------
function jumpTo(n) {
  const ls = lanesOf();
  const x = (n.terminal && ls.find((y) => y.key === "t:" + n.terminal)) || ls.find((y) => y.wt && y.wt === n.lane) || null;
  if (!x) return;
  selectLane(x.key);
  // To the terminal's door, not inside it: Enter there types into claude.
  if (x.t && x.t.running) requestAnimationFrame(() => { const h = $("termhost"); h.focus({ preventScroll: true }); h.scrollIntoView({ block: "nearest" }); });
}
function needKey(n) { return "need:" + n.lane + ":" + n.session + ":" + n.kind; }
function asOf() { return offline() && frozenAt ? ", as of " + hm(frozenAt) : ""; }
const AGED = { waiting: true, idle: true };
// Alerts shown in the strip: every alert that belongs to no lane, and blocking ones of
// any lane (a waiting alert already in Needs you is not repeated).
function stripAlerts() {
  const asked = new Set(S.needsYou.map((n) => n.session));
  return (S.alerts || []).filter((a) => !(a.kind === "waiting" && asked.has(a.session)) && (!(a.terminal || a.lane) || a.severity === "block"));
}
function needRow(n, cls, k, done) {
  const sev = done ? "" : n.severity === "block" ? "crit" : "warn";
  const tr = el("tr", sev ? "ask abn-" + sev : "done", null, [
    el("td", null, null, [el("span", "sq " + (done ? "idle" : sev === "crit" ? "block" : "waiting")), el("span", "who-waits", done ? "Your move" : sev === "crit" ? "Blocking" : "Needs you")]),
    el("td", null, n.name),
    el("td", "ask", (n.label || n.kind) + (n.text ? ": " + n.text : "") + (n.approx ? " (≈ not a current reading)" : "")),
    el("td", "num", null, n.at ? [age(n.at, "", done ? " ago" : "")] : []),
    el("td", null, null, [button(n.terminal ? "Open terminal" : "Show lane", "small jump", (ev) => { ev.stopPropagation(); jumpTo(n); }, null, "jump")]),
  ]);
  tr.children[2].title = tr.children[2].textContent;
  on(tr, "click", () => jumpTo(n));
  return key(tr, k);
}
function renderNeeds() {
  const al = stripAlerts(), done = S.done || [];
  const box = $("needs");
  box.hidden = !S.needsYou.length && !done.length && !al.length;
  const kids = [];
  if (S.needsYou.length) {
    kids.push(heading("Needs you" + asOf(), "h:needs", S.needsYou.length));
    kids.push(key(el("table", "tbl", null, [el("tbody", null, null, S.needsYou.map((n) => needRow(n, "", needKey(n))))]), "needs:tbl"));
  }
  if (al.length) {
    kids.push(heading("Alerts", "h:alerts", al.length));
    kids.push(key(el("table", "tbl", null, [el("tbody", null, null, al.map((a) => {
      const sev = a.severity === "block" ? "crit" : "warn";
      const tr = el("tr", "abn-" + sev, null, [el("td", null, null, [el("span", "sq " + (sev === "crit" ? "block" : "waiting")), el("span", null, a.kind.replace("_", " "))]),
        el("td", null, a.name || "All lanes"), el("td", "ask", a.text), el("td", "num", null, AGED[a.kind] && a.since ? [age(a.since)] : []),
        el("td", null, null, a.terminal || a.lane ? [button(a.terminal ? "Open terminal" : "Show lane", "small jump", (ev) => { ev.stopPropagation(); jumpTo(a); }, null, "jump")] : [])]);
      if (a.terminal || a.lane) on(tr, "click", () => jumpTo(a));
      return key(tr, "alert:" + a.key);
    }))]), "alerts:tbl"));
  }
  if (done.length) {
    kids.push(heading("Your move", "h:done", done.length));
    kids.push(key(el("table", "tbl", null, [el("tbody", null, null, done.map((n) => needRow(n, "", "done:" + needKey(n), true)))]), "done:tbl"));
  }
  patchInto("needs", kids);
}

// ---- The status bar ------------------------------------------------------------------------
function quotaField(id, v, expired, resets, proj) {
  const g = $(id).querySelector(".v");
  const kids = [bullet(expired ? 0 : v, { proj }), num(expired ? "reset" : pct(v))];
  if (resets && !expired) kids.push(el("span", "more", null, [until(resets * 1000, "resets in ")]));
  patch(g, kids);
}
function renderStatus(ls) {
  const q = S.quota || {}, tr = S.trends || {};
  // The 5-hour window: where it lands at the reset at the current burn, in --info.
  let proj = null;
  if (tr.burnPerH != null && q.fiveHourResetsAt && q.fiveHour != null) proj = q.fiveHour + tr.burnPerH * Math.max(0, (q.fiveHourResetsAt * 1000 - now()) / 3.6e6);
  quotaField("g5", q.fiveHour, q.fiveHourExpired, q.fiveHourResetsAt, proj);
  quotaField("g7", q.sevenDay, q.sevenDayExpired, q.sevenDayResetsAt, null);
  patch($("f-burn").querySelector(".v"), tr.burnPerH == null ? [el("span", "num", "—")] : [
    num((tr.burnPerH >= 0 ? "+" : "") + tr.burnPerH.toFixed(1), "%/h"),
    tr.exhaustAt ? el("span", tr.beforeReset ? "warn" : "more", "full at " + hm(tr.exhaustAt) + (tr.beforeReset ? ", before the reset" : "")) : null,
    spark(tr.quota5, { lo: 0, hi: 100 }),
  ].filter(Boolean));
  patch($("costmore"), [tr.costToday != null ? num(money(tr.costToday), "today") : null, tr.costPerH != null ? num(money(tr.costPerH), "/h") : null, spark(tr.cost)].filter(Boolean));
  const n = { busy: 0, waiting: 0, idle: 0 };
  for (const x of ls) {
    const s = laneState(x);
    if (s === "busy") n.busy++; else if (s === "waiting") n.waiting++; else n.idle++;
  }
  const stack = el("span", "stack");
  for (const k of ["busy", "waiting", "idle"]) if (n[k]) { const i = el("i", k); i.style.width = ((n[k] / ls.length) * 100).toFixed(1) + "%"; stack.appendChild(i); }
  stack.setAttribute("aria-hidden", "true");
  patch($("lanecounts"), ls.length ? [stack, num(String(n.busy)), document.createTextNode("working"), el("span", n.waiting ? "warn" : null, null, [num(String(n.waiting)), document.createTextNode(" need you")]), num(String(n.idle)), document.createTextNode("idle")] : [document.createTextNode("none")]);
  // Needs you: how many, and how long the oldest has waited.
  const oldest = S.needsYou.reduce((m, x) => (x.at && (!m || x.at < m) ? x.at : m), 0);
  patch($("attn"), [el("span", S.needsYou.length ? "warn" : null, null, [num(String(S.needsYou.length))]), oldest ? el("span", "more", null, [age(oldest, "oldest ")]) : null].filter(Boolean));
  // The gate: the first queue, which is the one a project usually has.
  const qs = S.queues || [];
  const gate = $("tm-gate"), g = qs[0];
  gate.hidden = !g && S.queuesSource.ok;
  if (!g) patch($("gatestate"), [el("span", S.queuesSource.ok ? null : "crit", S.queuesSource.ok ? "none" : "cannot read")]);
  else {
    setText(gate.querySelector(".k"), g.title || g.id);
    const w = g.waiters.length;
    const oldestW = g.waiters.reduce((m, x) => (x.started && (!m || x.started < m) ? x.started : m), 0);
    patch($("gatestate"), [
      el("span", g.holder && g.holder.stale ? "crit" : null, !g.held ? "free" : g.holder ? "held by " + (g.holder.lane || "pid " + g.holder.pid) + (g.holder.stale ? ", stale" : "") : "held"),
      w ? el("span", "more", null, [num(String(w)), document.createTextNode(" waiting, oldest "), age(oldestW)]) : null,
    ].filter(Boolean));
    gate.title = "The " + (g.title || g.id) + " queue. " + (g.holder && g.holder.lane ? "Click to show the lane that holds it." : "Click for every queue.");
  }
  patch($("procs"), tr.cpu == null ? [el("span", "more", tr.procsError ? "cannot read" : "read while in view")] :
    [num(Math.round(tr.cpu) + "%", "CPU"), num(Math.round(tr.memMb || 0), "MB"), spark(tr.cpuSpark, { lo: 0 })].filter(Boolean));
  setText($("intr"), String((S.observe.notifier || {}).interrupts || 0));
}
$("tm-gate").addEventListener("click", () => {
  const q = S && (S.queues || [])[0];
  const x = q && q.holder && lanesOf().find((y) => y.id === q.holder.lane);
  if (x) selectLane(x.key); else openDrawer();
});

// ---- The Activity drawer: every lane's events, the PRs, the queues, the cards -----------------
function projectCard(c) {
  const card = el("section", "pane pc", null, [paneHead("pch", c.title || c.id, null, c.source.at ? [el("span", "num dim", null, [age(c.source.at, "", " ago")])] : [])]);
  if (c.source.pending) card.appendChild(el("div", "empty", "running…"));
  else if (!c.source.ok) card.appendChild(el("div", "srcerr", "cannot read: " + c.source.error));
  else if (c.output.kind === "json") card.appendChild(renderJSON(c.output.json));
  else if (!c.output.lines || !c.output.lines.length) card.appendChild(el("div", "empty", "(no output)"));
  else {
    const ul = el("ul");
    for (const line of c.output.lines) ul.appendChild(el("li", null, line.replace(/^\s*(?:[-*+]|\d+\.)\s+/, "")));
    card.appendChild(ul);
  }
  return key(card, "card:" + c.id);
}
function renderJSON(v) {
  if (Array.isArray(v)) {
    if (!v.length) return el("div", "empty", "(empty)");
    const ul = el("ul");
    for (const it of v) {
      if (it !== null && typeof it === "object" && !Array.isArray(it)) {
        const main = it.title ?? it.name ?? it.text ?? it.summary ?? it.id;
        const rest = Object.entries(it).filter(([k, x]) => x !== main && (x === null || typeof x !== "object")).map(([k, x]) => k + ": " + x);
        const li = el("li", null, main != null ? String(main) : "");
        if (rest.length) li.appendChild(el("div", "dim", rest.join(", ")));
        ul.appendChild(li);
      } else ul.appendChild(el("li", null, typeof it === "string" ? it : JSON.stringify(it)));
    }
    return ul;
  }
  if (v !== null && typeof v === "object") {
    const kv = el("div", "kv");
    for (const [k, x] of Object.entries(v)) {
      kv.appendChild(el("span", "k", k));
      kv.appendChild(x !== null && typeof x === "object" ? renderJSON(x) : el("span", "v", String(x)));
    }
    return kv;
  }
  return el("div", null, String(v));
}
function feedList(items, withLane, k) {
  if (!items.length) return key(el("div", "empty", "No events yet."), k + ":empty");
  const f = key(el("div", "feed"), k);
  for (const e of items) {
    const d = el("span", "d", (withLane ? e.name + ": " : "") + e.event);
    if (e.detail) d.appendChild(el("span", "dim", e.detail));
    d.title = (e.name || "") + ", " + e.event + (e.detail ? ", " + e.detail : "");
    f.appendChild(key(el("div", "ev", null, [el("span", "t num", hhmm(e.at)), d]), "ev:" + e.at + ":" + (e.lane || "") + ":" + e.event + ":" + (e.detail || "")));
  }
  return f;
}
function sourceNote(src, what, k) {
  if (!src) return null;
  if (src.pending) return key(el("div", "empty", "reading " + what + "…"), k);
  if (!src.ok) return key(el("div", "srcerr", "cannot read " + what + ": " + src.error), k);
  return null;
}
function renderDrawer() {
  if ($("drawer").hidden) return;
  const kids = [];
  const qs = S.queues || [];
  if (qs.length || !S.queuesSource.ok) {
    const q = [paneHead("d:qh", "Queues", qs.length), sourceNote(S.queuesSource, "the queues", "d:qsrc")];
    for (const k of qs) {
      const c = el("div", "queue", null, [el("h3", null, k.title || k.id), holderLine(k)]);
      if (k.holderNote) c.appendChild(el("div", "warn", k.holderNote));
      if (k.waiters.length) { c.appendChild(el("div", "dim", "Waiting, in order:")); c.appendChild(waiterList(k, null)); } else c.appendChild(el("div", "dim", "Nobody waiting."));
      q.push(key(c, "dq:" + k.id));
    }
    kids.push(key(el("section", "pane", null, q), "d:queues"));
  }
  const prs = [paneHead("d:prh", "Open pull requests", S.sources.prs.ok ? S.prs.length : "?"), sourceNote(S.sources.prs, "gh pr list", "d:prsrc")];
  if (S.sources.prs.ok && !S.prs.length) prs.push(key(el("div", "empty", "None open."), "d:prs:none"));
  if (S.prs.length) {
    const tb = el("tbody");
    for (const p of S.prs) tb.appendChild(key(el("tr", null, null, [el("td", null, null, [prCell(p)]), el("td", null, p.headRefName), el("td", null, p.title)]), "pr:" + p.number));
    prs.push(key(el("div", "tblwrap", null, [el("table", "tbl", null, [tb])]), "d:prtbl"));
  }
  kids.push(key(el("section", "pane", null, prs), "d:prs"));
  for (const c of S.cards) kids.push(projectCard(c));
  // Every lane's events, grouped by lane, the lane with the newest event first.
  const groups = new Map();
  for (const e of S.feed.slice(0, 120)) {
    const k = e.name || e.lane || "?";
    if (!groups.has(k)) groups.set(k, []);
    groups.get(k).push(e);
  }
  const g = [paneHead("d:fh", "By lane")];
  if (!S.thresholds.notify) g.push(key(el("div", "dim", "OS notifications are off (alerts.notify)."), "d:notify"));
  for (const [name, evs] of groups) g.push(key(el("div", "feedgrp", null, [el("h4", null, name), feedList(evs.slice(0, 15), false, "dfeed:" + name)]), "dg:" + name));
  if (!groups.size) g.push(key(el("div", "empty", "No events yet."), "d:feed:none"));
  kids.push(key(el("section", "pane", null, g), "d:feed"));
  patchInto("drawerbody", kids);
}
let drawerReturn = null;
function openDrawer() {
  drawerReturn = document.activeElement;
  $("drawer").hidden = false;
  $("activitybtn").setAttribute("aria-expanded", "true");
  if (S) renderDrawer();
  $("drawerclose").focus();
}
function closeDrawer() {
  $("drawer").hidden = true;
  $("activitybtn").setAttribute("aria-expanded", "false");
  const r = drawerReturn; drawerReturn = null;
  if (r && r.isConnected && r.focus) r.focus();
}
$("activitybtn").addEventListener("click", () => ($("drawer").hidden ? openDrawer() : closeDrawer()));
$("drawerclose").addEventListener("click", closeDrawer);
$("drawer").addEventListener("keydown", (e) => { if (e.key === "Escape") { e.preventDefault(); closeDrawer(); } });

// ---- v1: lane terminals -------------------------------------------------------
// Each tab is one lane (PANEL-11: a lane started outside the panel has a tab too, and
// no terminal). A lane's xterm and its WebSocket live as long as the
// lane does, so switching tabs keeps scrollback and costs no reconnect. The page
// sends only {type: "input", data} and {type: "resize", cols, rows}.
//
// The keyboard (PANEL-6). Nothing ever moves focus into a terminal by itself: not
// loading the page, not picking a lane, not OPEN TERMINAL. The terminal is one stop
// in the Tab order (#termhost); Enter there, or a click, enters it. Inside, every key
// is claude's, Tab, Shift+Tab and Escape included, because that is what a terminal is
// for once you chose it. Ctrl+] leaves, back to the lane's tab, as in telnet. Picking
// a tab, with the mouse or the arrow keys, never enters the terminal either. A
// double Escape does not leave: claude uses Esc Esc itself (to edit an earlier
// message), so taking it would break claude.
const terms = {};              // lane id → {id, host, term, fit, ws, retry, delay}
let selTerm = null, shownTerm = null;
let confirmAct = null;         // {id, action} awaiting the in-page confirmation
let actMsg = null, busyAct = null;
let pendingTerm = null;        // a lane just started: selected as soon as a poll shows it
const IS_MAC = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

function enterTerm() {
  const t = terms[selTerm];
  if (t && !t.host.hidden) t.term.focus();
}
function leaveTerm(id) {
  const tab = document.querySelector('#tabs [data-k="tab:t:' + CSS.escape(id) + '"]');
  if (tab) tab.focus(); else $("termhost").focus();
}
let overTerm = false;
function renderHint() {
  const host = $("termhost"), a = document.activeElement, t = terms[selTerm];
  const inTerm = a && a.classList && a.classList.contains("xterm-helper-textarea") && host.contains(a);
  const copy = IS_MAC ? "⌘C copies" : "Ctrl+Shift+C copies";
  const hint = $("termhint");
  // The line is always there, so entering the terminal never resizes it.
  if (t && t.scrolled) setText(hint, "Scrolled back in history. Any key returns to the live screen and is typed; Esc only returns.");
  else if (inTerm) setText(hint, "Typing into " + selTerm + ". Ctrl+] leaves the terminal. Drag selects, " + copy + ", the wheel scrolls its history.");
  else if (a === host && t) setText(hint, "Press Enter to type into the terminal");
  else if (overTerm && t) setText(hint, "Click to type into the terminal. Drag selects text, " + copy + ", the wheel scrolls its history.");
  else if (t) setText(hint, "Click the terminal, or Tab to it and press Enter, to type into it");
  else setText(hint, "");
  hint.classList.toggle("on", !!(inTerm || a === host || (t && t.scrolled)));
}
document.addEventListener("focusin", renderHint);
document.addEventListener("focusout", () => setTimeout(renderHint, 0));
{
  const host = $("termhost");
  host.tabIndex = 0;
  host.setAttribute("role", "group");
  host.addEventListener("keydown", (ev) => {
    if (ev.target === host && ev.key === "Enter") { ev.preventDefault(); enterTerm(); }
  });
  host.addEventListener("mouseenter", () => { overTerm = true; renderHint(); });
  host.addEventListener("mouseleave", () => { overTerm = false; renderHint(); });
  // A click on the frame around the terminal enters it too; xterm handles its own.
  host.addEventListener("mousedown", (ev) => { if (ev.target === host) { ev.preventDefault(); enterTerm(); } });
}

// The terminal's colours are the active theme's --term-* and --ansi-* tokens, read from
// <html>, so themes.css stays the one place a colour is defined.
const ANSI = ["black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"];
function termTheme() {
  const cs = getComputedStyle(document.documentElement);
  const v = (k) => cs.getPropertyValue("--" + k).trim();
  const th = { background: v("term-bg"), foreground: v("term-fg"), cursor: v("term-cursor"), cursorAccent: v("term-bg"), selectionBackground: v("term-selection") };
  ANSI.forEach((n, i) => {
    th[n] = v("ansi-" + i);
    th["bright" + n[0].toUpperCase() + n.slice(1)] = v("ansi-" + (i + 8));
  });
  return th;
}
// The terminal's face is the theme's --font-term. xterm measures its cell from the face
// it has when the option is set, so a face still loading is set only once it has
// loaded (termFace), and Menlo stands in until then.
const TERM_FALLBACK = "Menlo, ui-monospace, SFMono-Regular, monospace";
function termFamily() {
  return getComputedStyle(document.documentElement).getPropertyValue("--font-term").trim() || TERM_FALLBACK;
}
let termFaceReady = "";
function termFace() {
  const fam = termFamily();
  return termFaceReady === fam ? fam : TERM_FALLBACK;
}
function loadTermFace() {
  const fam = termFamily(), size = termFontSize();
  const first = fam.split(",")[0].trim();
  const set = () => { termFaceReady = fam; retheme(); };
  if (!document.fonts || !document.fonts.load) { set(); return; }
  Promise.all([document.fonts.load(size + "px " + first), document.fonts.load("bold " + size + "px " + first)]).then(set, set);
}
// The terminal's text size: your own (A− / A+ under the terminal, kept per browser),
// or the theme's --term-size times the page's scale.
const TERM_KEY = "clauductor-panel-term-size";
let termSizeOwn = null;
try { const v = parseFloat(localStorage.getItem(TERM_KEY)); if (v >= 9 && v <= 32) termSizeOwn = v; } catch (e) {}
function termAutoSize() {
  const cs = getComputedStyle(document.documentElement);
  const base = parseFloat(cs.getPropertyValue("--term-size")) || 13;
  const scale = parseFloat(cs.getPropertyValue("--ui-scale")) || 1;
  return Math.round(base * scale * 2) / 2;
}
function termFontSize() { return termSizeOwn || termAutoSize(); }
function setTermSize(v) {
  termSizeOwn = v == null ? null : Math.max(9, Math.min(32, v));
  try { if (termSizeOwn == null) localStorage.removeItem(TERM_KEY); else localStorage.setItem(TERM_KEY, String(termSizeOwn)); } catch (e) {}
  retheme();
  if (S) render();
  renderPicker();
}
// The theme's floor for text a program draws on any colour, ANSI or truecolor.
function termMinContrast() {
  return parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--term-min-contrast")) || 1;
}
function retheme() {
  const theme = termTheme(), size = termFontSize(), minC = termMinContrast(), face = termFace();
  for (const t of Object.values(terms)) {
    t.term.options.theme = theme;
    t.term.options.minimumContrastRatio = minC;
    let refit = false;
    if (t.term.options.fontFamily !== face) { t.term.options.fontFamily = face; refit = true; }
    if (t.term.options.fontSize !== size) { t.term.options.fontSize = size; refit = true; }
    if (refit) fitTerm(t);
  }
  // A Tuned slider being dragged must not be rebuilt under the pointer.
  const a = document.activeElement;
  if (!(a && a.type === "range" && $("thememenu").contains(a))) renderPicker();
  renderFavicon();
}
document.addEventListener("panel-theme", () => { retheme(); loadTermFace(); });
document.addEventListener("panel-scale", () => { retheme(); if (S) render(); });

function termSend(t, msg) {
  if (t.ws && t.ws.readyState === WebSocket.OPEN) t.ws.send(JSON.stringify(msg));
}

function ensureTerm(id) {
  if (terms[id]) return terms[id];
  const host = el("div", "term");
  host.hidden = true;
  $("termhost").appendChild(host);
  const term = new Terminal({
    fontFamily: termFace(), fontSize: termFontSize(),
    // No blinking cursor: the page's one motion is a changed figure's flash.
    cursorBlink: false, scrollback: 2000, macOptionIsMeta: true,
    // tmux asks for mouse reports (so the wheel scrolls its history), which would
    // take every drag too: an Option-press forces a selection (see the mousedown
    // handler below, which turns a plain press into one).
    macOptionClickForcesSelection: true,
    // An Option-click would otherwise move claude's cursor by sending it arrow keys.
    altClickMovesCursor: false,
    theme: termTheme(), minimumContrastRatio: termMinContrast(),
    // Terminal output is untrusted. A link (OSC 8) opens only after an in-page
    // confirmation, and only http(s). Title escapes are ignored: nothing subscribes
    // to onTitleChange, so they never reach the DOM.
    linkHandler: { activate: (ev, uri) => askOpenLink(uri), allowNonHttpProtocols: false },
  });
  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.open(host);
  const t = { id, host, term, fit, ws: null, retry: null, delay: 1000, gone: false, focused: false, scrolled: false };
  // tmux has the mouse, so that the wheel scrolls its history, and it takes no
  // clicks. A plain press would reach tmux and do nothing, so it becomes a text
  // selection instead, exactly as an Option-press (Shift off a Mac) would: drag
  // selects, and ⌘C copies through xterm. Nothing is taken from claude, which never
  // saw clicks here.
  host.addEventListener("mousedown", (ev) => {
    if (ev.panelForced || ev.button !== 0) return;
    // The frame around the terminal (#termhost) is focusable, and a press would
    // otherwise focus it after xterm has focused its input: keep the input, so ⌘C
    // reaches xterm's copy and typing reaches claude.
    ev.preventDefault();
    setTimeout(() => term.focus(), 0);
    if (ev.altKey || ev.shiftKey || ev.ctrlKey || ev.metaKey) return;
    ev.stopImmediatePropagation();
    if (!IS_MAC) term.clearSelection(); // Shift extends a selection; start a fresh one
    const e2 = new MouseEvent("mousedown", { bubbles: true, cancelable: true, composed: true, view: window, detail: ev.detail,
      screenX: ev.screenX, screenY: ev.screenY, clientX: ev.clientX, clientY: ev.clientY, button: 0, buttons: ev.buttons,
      altKey: IS_MAC, shiftKey: !IS_MAC });
    e2.panelForced = true;
    ev.target.dispatchEvent(e2);
  }, true);
  // Ctrl+] is the way out; nothing else is taken from claude.
  term.attachCustomKeyEventHandler((ev) => {
    if (ev.ctrlKey && !ev.altKey && !ev.metaKey && (ev.code === "BracketRight" || ev.key === "]")) {
      if (ev.type === "keydown") { ev.preventDefault(); leaveTerm(id); }
      return false;
    }
    return true;
  });
  term.onData((d) => termSend(t, { type: "input", data: d }));
  // Tell the server which terminal has keyboard focus: alerts for that lane send no
  // OS notification, because you are looking at it.
  if (term.textarea) {
    // Out of the Tab order: #termhost is the terminal's one stop, and Enter enters.
    term.textarea.tabIndex = -1;
    term.textarea.addEventListener("focus", () => { t.focused = true; sendFocus(t); });
    term.textarea.addEventListener("blur", () => { t.focused = false; sendFocus(t); });
  }
  term.onResize(({ cols, rows }) => termSend(t, { type: "resize", cols, rows }));
  terms[id] = t;
  connectTerm(t);
  return t;
}

// A terminal WebSocket needs a single-use ticket, fetched by a POST the server checks
// for this page's Origin. The cookie alone would not do: other loopback ports share it.
async function connectTerm(t) {
  if (t.gone) return;
  let ticket;
  try {
    const r = await fetch("/api/lanes/" + encodeURIComponent(t.id) + "/ticket", { method: "POST" });
    if (!r.ok) throw new Error("ticket: HTTP " + r.status);
    ticket = (await r.json()).ticket;
  } catch (e) {
    if (t.gone) return;
    t.retry = setTimeout(() => connectTerm(t), t.delay);
    t.delay = Math.min(t.delay * 2, 10000);
    return;
  }
  if (t.gone) return;
  const q = "lane=" + encodeURIComponent(t.id) + "&cols=" + t.term.cols + "&rows=" + t.term.rows;
  const ws = new WebSocket("ws://" + location.host + "/ws/term?" + q, ["clauductor.term.v1", "ticket." + ticket]);
  ws.binaryType = "arraybuffer";
  ws.onopen = () => { t.delay = 1000; fitTerm(t); termSend(t, { type: "resize", cols: t.term.cols, rows: t.term.rows }); sendFocus(t); };
  ws.onmessage = (e) => {
    if (typeof e.data === "string") {
      // {"type":"scroll","back":…}: the lane is (or no longer is) scrolled back in
      // tmux's copy mode. {"type":"exit"}: onclose follows.
      try { const m = JSON.parse(e.data); if (m.type === "scroll") { t.scrolled = !!m.back; renderHint(); } } catch (x) {}
      return;
    }
    t.term.write(new Uint8Array(e.data));
  };
  ws.onclose = (e) => {
    if (t.gone || t.ws !== ws) return;
    if (e.code === 4000) { // closed while the page was out of view: reopen when it is back
      t.idle = true;
      t.term.write("\r\n\x1b[2m[panel] closed while this page was idle; it reopens when you come back\x1b[0m\r\n");
      if (document.visibilityState === "visible") wakeTerms();
      return;
    }
    if (e.code === 4001) { // the token was rotated: this page's cookie is dead too
      t.term.write("\r\n\x1b[2m[panel] the panel's token was rotated; run clauductor panel open\x1b[0m\r\n");
      return;
    }
    t.term.write("\r\n\x1b[2m[panel] detached; reconnecting while the lane exists…\x1b[0m\r\n");
    t.retry = setTimeout(() => connectTerm(t), t.delay);
    t.delay = Math.min(t.delay * 2, 10000);
  };
  t.ws = ws;
}

function sendFocus(t) {
  const on = t.focused && document.visibilityState === "visible" && document.hasFocus();
  termSend(t, { type: "focus", focused: on });
}
function refocusAll() { for (const t of Object.values(terms)) sendFocus(t); }
document.addEventListener("visibilitychange", refocusAll);
window.addEventListener("blur", refocusAll);
window.addEventListener("focus", refocusAll);

function disposeTerm(id) {
  const t = terms[id];
  if (!t) return;
  t.gone = true;
  clearTimeout(t.retry);
  if (t.ws) t.ws.close();
  t.term.dispose();
  t.host.remove();
  delete terms[id];
}

function fitTerm(t) {
  if (!t || t.host.hidden) return;
  try { t.fit.fit(); } catch (e) {}
}

// The terminal's frame takes the space the workspace leaves (a flex item, panel.css),
// so the controls under it stay on screen whatever banners show; below 1180 px it is
// a fixed share of the window. xterm is refitted whenever the frame changes size.
function sizeTerm() { fitTerm(terms[selTerm]); }
new ResizeObserver(() => sizeTerm()).observe($("termhost"));

let linkAsk = null;
function askOpenLink(uri) {
  let u;
  try { u = new URL(uri); } catch (e) { return; }
  if (u.protocol !== "http:" && u.protocol !== "https:") return;
  linkAsk = u.href;
  render();
}

async function laneAction(id, action) {
  busyAct = id + ":" + action; actMsg = null; render();
  try {
    const r = await fetch("/api/lanes/" + encodeURIComponent(id) + "/" + action, { method: "POST" });
    const j = await r.json().catch(() => ({}));
    actMsg = r.ok ? { id, text: action + ": done" } : { id, text: j.error || "HTTP " + r.status, err: true };
  } catch (e) { actMsg = { id, text: String(e), err: true }; }
  busyAct = null;
  render();
}

// A button. An action (act) reaches the panel, so it is off while the page has lost it.
function button(label, cls, onClick, title, k, act) {
  const b = el("button", "btn" + (cls ? " " + cls : ""), label);
  b.type = "button";
  if (title) b.title = title;
  on(b, "click", onClick);
  if (act && offline()) { b.disabled = true; b.title = "Disconnected from the panel"; }
  return key(b, k || "b:" + label);
}
// Focus a control a render just placed. The terminal is sized first, so a control
// under it is where it will stay before the column scrolls to show it.
function focusKey(k) {
  requestAnimationFrame(() => {
    const e = document.querySelector('[data-k="' + CSS.escape(k) + '"]');
    if (!e) return;
    sizeTerm();
    e.focus({ preventScroll: true });
    e.scrollIntoView({ block: "nearest" });
  });
}

// The terminal of the selected lane: created on first show, kept while the lane runs.
function renderTerminals(cur) {
  const ts = S.terminals || [];
  for (const id of Object.keys(terms)) if (!ts.find((x) => x.id === id && x.running)) disposeTerm(id);
  const t = cur && cur.t;
  selTerm = t ? t.id : null;
  if (t && t.running) ensureTerm(selTerm);
  for (const [id, x] of Object.entries(terms)) x.host.hidden = id !== selTerm;

  const empty = $("termempty");
  empty.hidden = !!(t && t.running) || !!(t && !t.running);
  const kids = [];
  if (!cur) {
    kids.push(el("div", null, "No lane is running yet."));
    kids.push(el("div", "dim", "A lane is an interactive claude in its own tmux session and worktree. Sessions started in a terminal of your own still show their status here, without a terminal."));
    const b = button("+ New lane", "primary big", () => openStart(), null, "empty:new", true);
    if (S.startBlocked) b.disabled = true;
    kids.push(b);
  } else if (!t) {
    kids.push(el("div", null, cur.name + " was started outside the panel, so it has no terminal here."));
    kids.push(el("div", "dim", "Its status, agents, alerts and activity still show. Attach to it in the terminal it runs in."));
  }
  if (S.startBlocked && !(t && t.running)) kids.push(el("div", "card err", S.startBlocked));
  patch(empty, kids);
  const orphan = $("termorphan");
  orphan.hidden = !(t && !t.running);
  if (t && !t.running) setText(orphan, "Lane " + t.id + " is orphaned: " + (t.orphan || "no tmux session") +
    ". RESUME restarts claude --resume " + t.sessionId + " in " + t.path + "; FORGET drops the record.");
  const host = $("termhost");
  host.setAttribute("aria-label", t && t.running ? "Terminal of lane " + t.id + ". Enter types into it; Ctrl+] leaves." : "Terminal");
  if (selTerm !== shownTerm) {
    shownTerm = selTerm;
    const x = terms[selTerm];
    if (x) requestAnimationFrame(() => fitTerm(x));
  }
  renderTermBar(t);
  renderHint();
}

// What STOP or RESTART will do to this lane, from its state now: the server sends
// /exit only to a lane `claude agents` says is idle, and Escape to any other.
function stopWords(t, restart) {
  const lane = S.lanes.find((l) => l.terminal === t.id);
  const subs = lane ? lane.subagents.length : 0;
  const subTxt = subs ? subs + " subagent" + (subs > 1 ? "s" : "") : "";
  const ap = t.approx ? " (≈ not a current reading; the panel checks again before it acts)" : "";
  let how;
  if (t.dead) how = "claude has already exited, so its tmux session just ends.";
  else if (t.status === "idle") how = "claude is idle" + ap + ": it gets /exit, then its tmux session ends.";
  else if (t.status === "busy") how = "claude is busy" + (subTxt ? " with " + subTxt : "") + ap + ": it gets Escape, which interrupts the turn" +
    (subTxt ? " and stops the " + subTxt : "") + ", then its tmux session is killed.";
  else if (t.status === "waiting") how = "claude is waiting on you" + (t.waitingFor ? " (" + t.waitingFor + ")" : "") + ap +
    ": it gets Escape, which dismisses the question unanswered, then its tmux session is killed.";
  else how = "its state is not known yet: it gets /exit if claude agents says idle, Escape otherwise, then its tmux session ends.";
  let q = "";
  for (const x of S.queues || []) {
    if (x.holder && x.holder.lane === t.id) q += " It holds the " + (x.title || x.id) + " queue" +
      (x.waiters.length ? ", with " + x.waiters.length + " waiting behind it" : "") + ".";
    else if (x.waiters.some((w) => w.lane === t.id)) q += " It is waiting in the " + (x.title || x.id) + " queue.";
  }
  return (restart ? "Restart lane " + t.id + "? " : "Stop lane " + t.id + "? ") + how + q +
    (restart ? " Then claude resumes its own session " + t.sessionId + " in the same directory." : " The worktree stays.");
}

function renderTermBar(t) {
  const kids = [];
  if (t) {
    const busy = busyAct && busyAct.startsWith(t.id + ":");
    if (linkAsk) {
      const href = linkAsk;
      kids.push(key(el("span", "confirm", "The lane printed a link. Open " + href + " in a new tab?"), "linkask"),
        button("Open link", "", () => { linkAsk = null; window.open(href, "_blank", "noopener,noreferrer"); render(); }),
        button("Cancel", "", () => { linkAsk = null; render(); }, null, "b:link-cancel"));
    } else if (!t.running) {
      if (confirmAct && confirmAct.id === t.id) {
        kids.push(key(el("span", "confirm", "Forget lane " + t.id + "? It leaves the registry; its worktree and conversation stay."), "confirm"),
          button("Confirm forget", "danger", () => { confirmAct = null; laneAction(t.id, "forget"); }, null, null, true),
          button("Cancel", "", () => { confirmAct = null; render(); focusKey("b:Forget"); }, null, "b:cancel"));
      } else {
        kids.push(
          button("Resume", "primary", () => laneAction(t.id, "resume"), "claude --resume " + (t.sessionId || "") + " in " + t.path, null, true),
          button("Forget", "danger", () => { confirmAct = { id: t.id, action: "forget" }; render(); focusKey("b:cancel"); },
            "Remove it from the lane registry. The worktree and the conversation stay.", null, true));
      }
    } else if (confirmAct && confirmAct.id === t.id) {
      const stop = confirmAct.action === "stop";
      const back = stop ? "b:Stop lane" : "b:Restart";
      kids.push(
        key(el("span", "confirm", stopWords(t, !stop)), "confirm"),
        button(stop ? "Confirm stop" : "Confirm restart", "danger", () => { const a = confirmAct; confirmAct = null; laneAction(t.id, a.action); }, null, null, true),
        button("Cancel", "", () => { confirmAct = null; render(); focusKey(back); }, null, "b:cancel"));
    } else {
      const b = [
        button("Attach in Terminal.app", "", () => laneAction(t.id, "terminal-app"), "Open a Terminal.app window on this lane (tmux attach)", null, true),
        button("Interrupt (Esc)", "", () => laneAction(t.id, "interrupt"), "Press Escape in the lane", null, true),
        t.registered ? button("Restart", "", () => { confirmAct = { id: t.id, action: "restart" }; render(); focusKey("b:cancel"); }, null, null, true) : null,
        button("Stop lane", "danger", () => { confirmAct = { id: t.id, action: "stop" }; render(); focusKey("b:cancel"); }, null, null, true),
      ].filter(Boolean);
      for (const x of b) { if (busy) x.disabled = true; kids.push(x); }
    }
    if (t.orphan && t.running) kids.push(key(el("span", "hold sub", t.orphan), "orphan"));
    if (t.template) kids.push(key(el("span", "note", "Template " + t.template + ", first prompt " + (t.promptState || "—") + (t.promptNote ? ": " + t.promptNote : "")), "tpl"));
    if (t.dead) kids.push(key(el("span", "stop sub", "claude exited" + (t.deadStatus ? " (status " + t.deadStatus + ")" : "") + "; RESTART or STOP"), "dead"));
    if (busy) kids.push(key(el("span", "sub", busyAct.split(":")[1] + "…"), "busy"));
    if (actMsg && actMsg.id === t.id) kids.push(key(el("span", actMsg.err ? "stop sub" : "sub", actMsg.text), "msg"));
  }
  patchInto("termbar", kids);
}

// Terminals stay open only while the page is in view: while visible the page says
// "alive" once a minute, and the server closes a terminal after 5 minutes without a
// message (code 4000). Coming back reopens each one with a fresh ticket.
setInterval(() => {
  if (document.visibilityState !== "visible") return;
  for (const t of Object.values(terms)) termSend(t, { type: "alive" });
}, 60000);
function wakeTerms() {
  if (document.visibilityState !== "visible") return;
  for (const t of Object.values(terms)) if (t.idle) { t.idle = false; t.delay = 1000; connectTerm(t); }
}
document.addEventListener("visibilitychange", wakeTerms);
window.addEventListener("focus", wakeTerms);

// ---- v1: the Start lane dialog ------------------------------------------------
function slug(s) { return s.toLowerCase().replace(/[^a-z0-9-]+/g, "-").replace(/^-+/, "").slice(0, 41).replace(/-+$/, ""); }
function stMode() { return document.querySelector('input[name="st-mode"]:checked').value; }

function stTpl() { return (S.templates || []).find((x) => x.id === $("st-tpl").value); }
function fillTpl(s, name, issue) { return s.replace(/\{name\}/g, name || "<name>").replace(/\{issue\}/g, issue || "<issue>"); }

function stUpdate() {
  const tpl = stTpl();
  const untrusted = S.trust && S.trust.hash && !S.trust.trusted;
  $("st-type").disabled = !!tpl;
  for (const r of document.querySelectorAll('input[name="st-mode"]')) r.disabled = !!tpl;
  $("st-issue-row").hidden = !(tpl && tpl.needsIssue);
  $("st-prompt").hidden = !tpl;
  if (tpl) {
    $("st-type").value = tpl.laneType;
    document.querySelector('input[name="st-mode"][value="new"]').checked = true;
  }
  $("st-quota-row").hidden = !S.quotaGuard;
  $("st-quota-why").textContent = S.quotaGuard ? S.quotaGuard + ". Start anyway." : "";
  const type = (S.laneTypes || []).find((x) => x.name === $("st-type").value);
  const mode = stMode();
  $("st-wt-row").hidden = mode !== "existing";
  const name = $("st-name").value;
  let p = "";
  if (mode === "new") {
    p = type && type.prefix
      ? "Creates branch " + type.prefix + (name || "<name>") + " from " + S.laneBase + " in " + S.worktreeRoot + "/" + (name || "<name>")
      : "Lane type " + (type ? type.name : "?") + " has no branch prefix; pick an existing worktree or the project root.";
  } else if (mode === "existing") p = "Runs in " + ($("st-wt").value || "?");
  else p = "Runs in " + S.root;
  if (tpl) {
    const name = $("st-name").value, issue = $("st-issue").value.trim();
    p = "Creates branch " + fillTpl(tpl.branchPattern, name, issue) + " from " + S.laneBase + " in " + S.worktreeRoot + "/" + (name || "<name>");
    const model = tpl.model || (type && type.model), effort = tpl.effort || (type && type.effort);
    if (model || effort) p += ", then claude" + (model ? " --model " + model : "") + (effort ? " --effort " + effort : "");
    $("st-prompt").replaceChildren(el("div", null, "Once claude is idle, the panel types this, then Enter:"),
      el("div", "tpl-prompt", fillTpl(tpl.firstPrompt, name, issue)));
    if (untrusted) $("st-prompt").appendChild(el("div", "stop", "Templates are off: panel.json changed since you trusted it (clauductor panel trust)."));
  } else if (type && (type.model || type.effort)) p += ", then claude" + (type.model ? " --model " + type.model : "") + (type.effort ? " --effort " + type.effort : "");
  $("st-preview").textContent = p;
  $("st-block").hidden = !S.startBlocked;
  $("st-block").textContent = S.startBlocked || "";
  $("st-go").disabled = !!S.startBlocked || offline();
}

// opts.worktree: "Start lane here" on a worktree in the tree picks that worktree.
let startReturn = null;
function openStart(opts) {
  if (!S || offline()) return;
  startReturn = document.activeElement;
  const tsel = $("st-tpl"), tprev = tsel.value;
  const none = el("option", null, "(none: an empty lane)"); none.value = "";
  tsel.replaceChildren(none, ...(S.templates || []).map((x) => { const o = el("option", null, (x.title || x.id) + " (" + x.laneType + ")"); o.value = x.id; return o; }));
  tsel.value = tprev && (S.templates || []).find((x) => x.id === tprev) ? tprev : "";
  $("st-quota").checked = false;
  const sel = $("st-type"), prev = sel.value;
  sel.replaceChildren(...(S.laneTypes || []).map((x) => { const o = el("option", null, x.name); o.value = x.name; return o; }));
  if (prev) sel.value = prev;
  const wt = $("st-wt");
  // A worktree path can be longer than the dialog: it is cut from the left, where
  // every path is the same, and the whole path is the option's title.
  const tail = (p) => p.length > 44 ? "…" + p.slice(-43) : p;
  wt.replaceChildren(...S.lanes.concat(S.quietWorktrees).map((l) => { const o = el("option", null, l.name + ": " + tail(l.path)); o.value = l.path; o.title = l.path; return o; }));
  $("st-err").textContent = "";
  $("startdlg").hidden = false;
  stTypeChanged();
  if (opts && opts.worktree) {
    $("st-tpl").value = "";
    wt.value = opts.worktree;
    wt.title = opts.worktree;
    document.querySelector('input[name="st-mode"][value="existing"]').checked = true;
    if (!$("st-name").value) $("st-name").value = slug(opts.worktree.split("/").pop() || "");
    stUpdate();
  }
  $("st-name").focus();
}

function stTypeChanged() {
  const type = (S.laneTypes || []).find((x) => x.name === $("st-type").value);
  const mode = type && type.prefix ? "new" : "root";
  document.querySelector('input[name="st-mode"][value="' + mode + '"]').checked = true;
  if (mode === "root" && !$("st-name").value) $("st-name").value = slug(type ? type.name : "orchestrator");
  stUpdate();
}

// Closing returns focus to what opened the dialog, if it is still on the page.
function closeStart() {
  $("startdlg").hidden = true;
  const r = startReturn; startReturn = null;
  if (r && r.isConnected && r.focus) r.focus();
}

$("addlane").addEventListener("click", () => openStart());
$("addtab").addEventListener("click", () => openStart());
$("st-cancel").addEventListener("click", closeStart);
// A modal dialog: Escape closes it wherever focus is inside, and Tab stays inside.
$("startdlg").addEventListener("keydown", (e) => {
  if (e.key === "Escape") { e.preventDefault(); closeStart(); return; }
  if (e.key !== "Tab") return;
  const f = Array.from($("startform").querySelectorAll("select, input, button")).filter((x) => !x.disabled && x.offsetParent !== null);
  if (!f.length) return;
  const first = f[0], last = f[f.length - 1];
  if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); }
  else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
});
// After a failed start, focus stays in the dialog (the audit found it fell to <body>).
$("startdlg").addEventListener("focusout", () => setTimeout(() => {
  if (!$("startdlg").hidden && !$("startdlg").contains(document.activeElement)) $("st-name").focus();
}, 0));
$("st-type").addEventListener("change", stTypeChanged);
$("st-wt").addEventListener("change", () => { $("st-wt").title = $("st-wt").value; if (!$("st-name").value) $("st-name").value = slug($("st-wt").value.split("/").pop()); stUpdate(); });
$("st-name").addEventListener("input", stUpdate);
$("st-issue").addEventListener("input", stUpdate);
$("st-tpl").addEventListener("change", stUpdate);
for (const r of document.querySelectorAll('input[name="st-mode"]')) r.addEventListener("change", () => {
  if (stMode() === "existing" && !$("st-name").value) $("st-name").value = slug($("st-wt").value.split("/").pop() || "");
  stUpdate();
});
$("startform").addEventListener("submit", async (e) => {
  e.preventDefault();
  const tpl = stTpl();
  const body = tpl ? { template: tpl.id, name: $("st-name").value.trim() } : { type: $("st-type").value, mode: stMode(), name: $("st-name").value.trim() };
  if (tpl && tpl.needsIssue) body.issue = $("st-issue").value.trim();
  if (!tpl && body.mode === "existing") body.worktree = $("st-wt").value;
  if (S.quotaGuard && $("st-quota").checked) body.overrideQuota = true;
  $("st-go").disabled = true;
  $("st-err").textContent = "starting…";
  try {
    const r = await fetch("/api/lanes", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    const j = await r.json().catch(() => ({}));
    if (!r.ok) { $("st-err").textContent = j.error || "HTTP " + r.status; return; }
    closeStart();
    $("st-name").value = ""; $("st-issue").value = "";
    if (j.lane && j.lane.notes) actMsg = { id: j.lane.id, text: j.lane.notes.join(" ") };
    pendingTerm = j.lane.id;
    fetch("/api/refresh", { method: "POST" }).catch(() => {});
  } catch (err) { $("st-err").textContent = String(err); }
  finally { $("st-go").disabled = !!(S && S.startBlocked) || offline(); }
});



// ---- v2: restore, banners, observability ------------------------------------------
let restoreMsg = null, restoreBusy = false;
function renderRestore() {
  const bar = $("restorebar");
  const ids = S.restorable || [];
  bar.hidden = !ids.length && !restoreMsg;
  const row = el("div", "restore");
  // The one place a lost tmux session is announced, with the one control for it.
  row.appendChild(el("b", null, "Restore"));
  if (ids.length) {
    row.appendChild(el("span", null, ids.length + " lane" + (ids.length > 1 ? "s" : "") + " lost " + (ids.length > 1 ? "their" : "its") +
      " tmux session (a reboot, or the tmux server ended): " + ids.join(", ") + ". Restore all resumes each on its own session id."));
    if (S.quotaGuard) {
      const over = key(el("input"), "over");
      over.type = "checkbox"; over.id = "restore-over";
      row.appendChild(key(el("label", null, null, [over, document.createTextNode(S.quotaGuard + ". Restore anyway.")]), "overl"));
    }
    const b = button("Restore all", "primary", async () => {
      restoreBusy = true; restoreMsg = null; render();
      const over = $("restore-over");
      try {
        const r = await fetch("/api/lanes/restore-all", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ overrideQuota: !!(over && over.checked) }) });
        const j = await r.json().catch(() => ({}));
        if (!r.ok) restoreMsg = { text: j.error || "HTTP " + r.status, err: true };
        else {
          const res = j.result || {};
          restoreMsg = { text: "restored " + (res.restored || []).join(", ") + ((res.skipped || []).length ? "; skipped " + res.skipped.map((s) => s.id + " (" + s.reason + ")").join("; ") : "") };
        }
      } catch (e) { restoreMsg = { text: String(e), err: true }; }
      restoreBusy = false;
      render();
    }, "claude --resume <its own session id> in each lane's worktree; never --continue, never twice", null, true);
    if (restoreBusy || S.startBlocked) b.disabled = true;
    row.appendChild(b);
  }
  if (restoreMsg) {
    row.appendChild(el("span", restoreMsg.err ? "stop sub" : "sub", restoreMsg.text));
    row.appendChild(button("Dismiss", "", () => { restoreMsg = null; render(); }));
  }
  patch(bar, [key(row, "restore")]);
}

// Each banner says what it is. The lost-tmux banner is left to the restore bar.
const BANNER_LABEL = { no_hooks: "No hooks", dropped: "Events dropped", untrusted: "Config changed", hooks: "Hooks", registry: "Lane registry" };
const SOURCE_NAME = { worktrees: "git worktree list", agents: "claude agents", tmux: "the panel's tmux server" };
function renderBanners() {
  const kids = [];
  const items = S.bannerItems || S.banners.map((t) => ({ kind: "", text: t }));
  items.forEach((b, i) => {
    if (b.kind === "restore") return;
    const warn = b.kind === "untrusted" || b.kind === "dropped";
    kids.push(key(el("div", "banner" + (warn ? " warn" : ""), null, [el("b", null, BANNER_LABEL[b.kind] || "Notice"), document.createTextNode(b.text)]), "banner:" + b.kind + ":" + i));
  });
  for (const [k, src] of Object.entries(S.sources)) {
    if (k !== "prs" && !src.pending && !src.ok) kids.push(key(el("div", "banner", null, [el("b", null, "Cannot read"), document.createTextNode((SOURCE_NAME[k] || k) + ": " + src.error)]), "banner:src:" + k));
  }
  (S.warnings || []).forEach((w, i) => kids.push(key(el("div", "warnbar", w), "warn:" + i)));
  patchInto("banners", kids);
}

// The footer: one line of the counters that matter, the rest one click away.
let obsOpen = false;
try { obsOpen = localStorage.getItem("clauductor-panel-obs") === "open"; } catch (e) {}
function renderObs() {
  const o = S.observe;
  const kv = (k, v, cls) => el("span", cls || null, null, [document.createTextNode(k + " "), el("b", null, String(v))]);
  const toggle = button(obsOpen ? "Fewer counters" : "All counters", "obstoggle", () => {
    obsOpen = !obsOpen;
    try { localStorage.setItem("clauductor-panel-obs", obsOpen ? "open" : "closed"); } catch (e) {}
    render();
  }, obsOpen ? "Show the main counters only" : "Show every counter the panel keeps", "obstoggle");
  toggle.setAttribute("aria-expanded", String(obsOpen));
  toggle.setAttribute("aria-controls", "obs");
  const drops = o.droppedForeign + o.overflowDrops + o.malformedDrops + o.droppedUnknownEvent;
  const kids = [toggle, kv("events", o.hookEvents), kv("status posts", o.statusPosts), kv("dropped", drops),
    kv("notifications", o.notifySent + (o.notifyFailed ? ", failed " + o.notifyFailed : ""))];
  if (obsOpen) kids.push(
    kv("dropped: foreign cwd", o.droppedForeign), kv("overflow", o.overflowDrops), kv("malformed", o.malformedDrops),
    kv("unknown event", o.droppedUnknownEvent), kv("unknown notification", o.unknownNotifications),
    kv("claude agents", (o.agentsPolls ? o.agentsPollMs + " ms (avg " + o.agentsPollAvgMs + ", max " + o.agentsPollMaxMs + ")" : "—") +
      " every " + (o.agentsIntervalMs ? o.agentsIntervalMs / 1000 + " s" : "—")),
    kv("filter", o.agentsFilter || "—", "wrap"),
    kv("Claude Code", (o.claudeVersion || "?") + (o.claudeVersion && o.claudeVersion !== o.verifiedOn ? " (verified on " + o.verifiedOn + ")" : "")));
  const f = $("obs");
  f.classList.toggle("open", obsOpen);
  patch(f, kids.map((x, i) => (x.dataset.k ? x : key(x, "o:" + i))));
  f.title = o.notifyError || "";
}

// The tab title and the favicon carry the Needs-you count, so it shows from any tab.
let favSig = "";
function tok(name) {
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return /^(#[0-9a-fA-F]{3,8}|rgba?\([0-9., ]+\))$/.test(v) ? v : "currentColor";
}
function renderFavicon() {
  if (!S) return;
  const n = S.needsYou.length, off = offline();
  const sig = [n, off, document.documentElement.dataset.theme, document.documentElement.dataset.mode].join("|");
  if (sig === favSig) return;
  favSig = sig;
  const bg = tok(off ? "--text-3" : "--text-1"), ink = tok("--ground");
  let svg = "<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'><rect x='1' y='1' width='30' height='30' rx='7' fill='" + bg + "'/>";
  if (off) svg += "<text x='16' y='23' font-family='sans-serif' font-size='20' font-weight='700' text-anchor='middle' fill='" + ink + "'>!</text>";
  else if (n) svg += "<circle cx='21' cy='11' r='10' fill='" + tok("--crit") + "' stroke='" + ink + "' stroke-width='2'/>" +
    "<text x='21' y='15.5' font-family='sans-serif' font-size='13' font-weight='700' text-anchor='middle' fill='" + ink + "'>" + (n > 9 ? "9+" : n) + "</text>";
  svg += "</svg>";
  $("favicon").href = "data:image/svg+xml," + encodeURIComponent(svg);
}

// New blocking items are read out once, politely.
let seenNeeds = null;
function announceNeeds() {
  const keys = S.needsYou.map(needKey);
  if (seenNeeds) {
    const fresh = S.needsYou.filter((n) => !seenNeeds.has(needKey(n)));
    if (fresh.length) setText($("announce"), "Needs you: " + fresh.map((n) => n.name + ", " + (n.label || n.kind) + (n.text ? ": " + n.text : "")).join(". "));
  }
  seenNeeds = new Set(keys);
}

function render() {
  if (!S) { renderConn(); return; }
  selHeld = heldBySelection();
  const off = offline();
  const n = S.needsYou.length;
  document.title = (off ? "⚠ Disconnected: " : "") + (n ? "(" + n + ") " : "") + S.name + " panel";
  setText($("pname"), S.name);
  setText($("cost"), S.estCostUsd == null ? "—" : money(S.estCostUsd));
  setText($("hookn"), S.hookEvents + " / " + S.statusPosts + (S.dropped ? " / " + S.dropped + " dropped" : ""));
  const add = $("addlane");
  add.disabled = !!S.startBlocked || off;
  add.title = off ? "Disconnected from the panel" : S.startBlocked || "Start a lane: an interactive claude in its own tmux session";
  $("refresh").disabled = off;
  renderConn();
  renderBanners();
  renderRestore();
  renderObs();
  renderFavicon();
  if (!off) announceNeeds();

  const ls = lanesOf();
  // A lane just started is selected as soon as a poll shows it.
  if (pendingTerm && ls.find((x) => x.key === "t:" + pendingTerm)) {
    selKey = "t:" + pendingTerm; pendingTerm = null;
    try { localStorage.setItem("clauductor-panel-sel", selKey); } catch (e) {}
  }
  const cur = ls.find((x) => x.key === selKey) || ls[0] || null;
  renderStatus(ls);
  renderNeeds();
  renderRail(ls, cur);
  renderTabs(ls, cur);
  renderLaneHead(cur);
  renderTerminals(cur);
  renderSide(cur);
  renderDrawer();
  renderHint();
}

$("refresh").addEventListener("click", () => { if (!offline()) fetch("/api/refresh", { method: "POST" }).catch(() => {}); });

// ---- The page's text size: Ctrl+Alt+= / − / 0, and the Size group of Appearance ------
// theme.js owns the value (PanelScale); every size in panel.css is in rem. The keys need
// Alt, so the browser's own zoom (⌘ or Ctrl with = and −) is never taken, and inside a
// terminal every key stays claude's.
document.addEventListener("keydown", (ev) => {
  if (!ev.ctrlKey || !ev.altKey || ev.metaKey) return;
  if (ev.target && ev.target.closest && ev.target.closest(".xterm")) return;
  if (ev.code === "Equal" || ev.code === "NumpadAdd") { ev.preventDefault(); window.PanelScale.step(1); }
  else if (ev.code === "Minus" || ev.code === "NumpadSubtract") { ev.preventDefault(); window.PanelScale.step(-1); }
  else if (ev.code === "Digit0" || ev.code === "Numpad0") { ev.preventDefault(); window.PanelScale.reset(); }
});

// ---- The rail: hide it, or drag (or arrow-key) its edge. Kept per browser. ------------------
const RAIL_KEY = "clauductor-panel-rail";
// On a phone the rail starts folded (the tabs switch lanes), so the terminal is near
// the top; it is kept apart from the desktop's choice.
const NARROW = matchMedia("(max-width: 900px)");
let rail = { open: true, openNarrow: false, w: 0 };
try { rail = Object.assign(rail, JSON.parse(localStorage.getItem(RAIL_KEY) || "{}")); } catch (e) {}
function saveRail() { try { localStorage.setItem(RAIL_KEY, JSON.stringify(rail)); } catch (e) {} }
function applyRail() {
  const shell = $("shell"), sp = $("splitter");
  const open = NARROW.matches ? rail.openNarrow : rail.open;
  shell.classList.toggle("norail", !open);
  $("railbtn").setAttribute("aria-expanded", String(open));
  if (rail.w) shell.style.setProperty("--rail-w", rail.w + "px"); else shell.style.removeProperty("--rail-w");
  const w = Math.round($("rail").getBoundingClientRect().width);
  sp.setAttribute("aria-valuenow", String(w));
  sp.setAttribute("aria-valuemin", "160");
  sp.setAttribute("aria-valuemax", String(railMax()));
  sp.setAttribute("aria-valuetext", w + " pixels");
}
function railMax() { return Math.max(200, Math.round(window.innerWidth * 0.45)); }
function setRailW(px) { rail.w = Math.max(160, Math.min(railMax(), Math.round(px))); saveRail(); applyRail(); }
$("railbtn").addEventListener("click", () => {
  if (NARROW.matches) rail.openNarrow = !rail.openNarrow; else rail.open = !rail.open;
  saveRail(); applyRail();
});
NARROW.addEventListener("change", applyRail);
{
  const sp = $("splitter");
  sp.addEventListener("pointerdown", (ev) => {
    if (ev.button !== 0) return;
    ev.preventDefault();
    sp.setPointerCapture(ev.pointerId);
    sp.classList.add("drag");
    document.body.classList.add("resizing");
    const left = $("shell").getBoundingClientRect().left;
    const move = (e) => setRailW(e.clientX - left);
    const up = () => {
      sp.classList.remove("drag");
      document.body.classList.remove("resizing");
      sp.removeEventListener("pointermove", move);
      sp.removeEventListener("pointerup", up);
      sp.removeEventListener("pointercancel", up);
    };
    sp.addEventListener("pointermove", move);
    sp.addEventListener("pointerup", up);
    sp.addEventListener("pointercancel", up);
  });
  // The keyboard: Left and Right move the edge 16 px, Home and End to either limit,
  // and Enter hides the rail. A double-click, or Escape, returns to the theme's width.
  sp.addEventListener("keydown", (ev) => {
    const w = $("rail").getBoundingClientRect().width;
    if (ev.key === "ArrowLeft") setRailW(w - 16);
    else if (ev.key === "ArrowRight") setRailW(w + 16);
    else if (ev.key === "Home") setRailW(160);
    else if (ev.key === "End") setRailW(railMax());
    else if (ev.key === "Escape") { rail.w = 0; saveRail(); applyRail(); }
    else if (ev.key === "Enter") { rail.open = false; saveRail(); applyRail(); $("railbtn").focus(); }
    else return;
    ev.preventDefault();
  });
  sp.addEventListener("dblclick", () => { rail.w = 0; saveRail(); applyRail(); });
}
applyRail();

// ---- Appearance: theme, mode, type and size ----------------------------------------
// A menu button (WAI-ARIA APG): Enter, Space or Down opens it on the checked theme, Up on
// the last item; Up/Down/Home/End move; Enter or Space picks; Escape closes and returns
// focus to the button; Tab closes. A theme, mode or type applies at once, persists
// (theme.js) and closes the menu; a size step applies and keeps the menu open, so it
// can be pressed again.
const MODE_LABEL = { system: "System", light: "Light", dark: "Dark" };
function menuItems() { return Array.from($("thememenu").querySelectorAll('[role="menuitemradio"], [role="menuitem"], input[type="range"]')); }
function renderPicker() {
  const P = window.PanelTheme, cur = P.get(), menu = $("thememenu");
  const th = P.themes.find((x) => x.id === cur.theme), ty = P.types.find((x) => x.id === cur.type);
  $("themebtn").setAttribute("aria-label", "Appearance: theme " + th.name + ", mode " + MODE_LABEL[cur.mode] + ", type " + ty.name + ", text " + window.PanelScale.get() + "%");
  const focusedKey = menu.contains(document.activeElement) ? document.activeElement.dataset.key : null;
  const item = (key, checked, kids, onPick, stay, cls) => {
    const e = el("div", "mi" + (cls ? " " + cls : ""), null, kids);
    e.setAttribute("role", checked == null ? "menuitem" : "menuitemradio");
    if (checked != null) e.setAttribute("aria-checked", String(checked));
    e.tabIndex = -1;
    e.dataset.key = key;
    e.addEventListener("click", () => { onPick(); if (stay) renderPicker(); else closePicker(true); });
    return e;
  };
  // The headings are visual only: each group carries its own aria-label.
  const heading = (t) => { const h = el("div", "mh", t); h.setAttribute("aria-hidden", "true"); return h; };
  const group = (label, cls, kids) => { const g = el("div", cls, null, kids); g.setAttribute("role", "group"); g.setAttribute("aria-label", label); return g; };
  const themes = group("Theme", null, [heading("Theme")]);
  for (const x of P.themes) {
    const sw = el("span", "sw", null, [el("i"), el("i"), el("i"), el("i")]);
    sw.setAttribute("data-theme", x.id);
    sw.setAttribute("data-mode", cur.resolved);
    sw.setAttribute("aria-hidden", "true");
    themes.appendChild(item("t:" + x.id, x.id === cur.theme, [sw, el("span", null, x.name, [el("small", null, x.note)]), el("span", "tick", "✓")], () => P.setTheme(x.id), false, "theme"));
  }
  const modes = group("Mode", "modes", P.modes.map((m) => item("m:" + m, m === cur.mode, [el("span", null, MODE_LABEL[m])], () => P.setMode(m))));
  const col1 = [themes, heading("Mode"), modes];
  // Tuned's three inputs: sliders that apply as they move and keep the menu open.
  if (th.tuned) {
    const slider = (k, label, min, max, stepv, val, fmt) => {
      const r = el("input");
      r.type = "range"; r.min = min; r.max = max; r.step = stepv; r.value = val;
      r.setAttribute("aria-label", label);
      r.dataset.key = "tu:" + k;
      const out = el("span", "num", fmt(val));
      r.addEventListener("input", () => { P.setTuned(k, +r.value); out.textContent = fmt(+r.value); });
      return el("div", "tunedrow", null, [el("span", null, label), r, out]);
    };
    col1.push(group("Tuned", null, [heading("Tuned"),
      slider("hue", "Base hue", 0, 359, 1, cur.tuned.hue, (v) => String(Math.round(v))),
      slider("accent", "Accent hue", 0, 359, 1, cur.tuned.accent, (v) => String(Math.round(v))),
      slider("contrast", "Contrast", 0, 1, 0.05, cur.tuned.contrast, (v) => (v >= 1 ? "AAA" : Math.round(v * 100) + "%"))]));
  }
  const types = group("Type", null, [heading("Type")]);
  const sugg = P.types.find((x) => x.id === th.type);
  types.appendChild(item("y:", cur.typeChoice === null, [el("span", null, "Theme's choice", [el("small", null, sugg.name)]), el("span", "tick", "✓")], () => P.setType(null)));
  for (const x of P.types) {
    const face = el("span", "face", x.name, [el("small", null, x.note)]);
    face.setAttribute("data-type", x.id);
    types.appendChild(item("y:" + x.id, cur.typeChoice === x.id, [face, el("span", "tick", "✓")], () => P.setType(x.id)));
  }
  // Size: the page's text, and the terminal's apart from it.
  const S2 = window.PanelScale, pctv = S2.get(), tsz = termFontSize();
  const step = (key, label, aria, fn) => { const e = item(key, null, [el("span", null, label)], fn, true); e.setAttribute("aria-label", aria); return e; };
  const size = group("Size", "sizes", [heading("Size"),
    el("div", "sizerow", null, [el("span", null, "Page text"), step("s:pd", "A−", "Page text smaller (Ctrl+Alt+−)", () => S2.step(-1)),
      el("span", "val num", pctv + "%" + (S2.own() ? "" : " auto")), step("s:pu", "A+", "Page text larger (Ctrl+Alt+=)", () => S2.step(1))]),
    el("div", "sizerow", null, [el("span", null, "Terminal text"), step("s:td", "A−", "Terminal text smaller", () => setTermSize(termFontSize() - 1)),
      el("span", "val num", tsz + " px" + (termSizeOwn ? "" : " auto")), step("s:tu", "A+", "Terminal text larger", () => setTermSize(termFontSize() + 1))]),
    step("s:reset", "Reset both sizes", "Reset the page and terminal text sizes (Ctrl+Alt+0 resets the page)", () => { S2.reset(); setTermSize(null); }),
  ]);
  menu.replaceChildren(el("div", "mcol", null, col1), el("div", "mcol", null, [types, size]));
  if (focusedKey) { const f = menu.querySelector('[data-key="' + focusedKey + '"]'); if (f) f.focus(); }
}
function openPicker(which) {
  const menu = $("thememenu");
  renderPicker();
  menu.hidden = false;
  menu.classList.remove("flip");
  if (menu.getBoundingClientRect().left < 8) menu.classList.add("flip");
  $("themebtn").setAttribute("aria-expanded", "true");
  const items = menuItems();
  const target = which === "last" ? items[items.length - 1] : (items.find((x) => x.getAttribute("aria-checked") === "true") || items[0]);
  target.focus();
}
function closePicker(refocus) {
  $("thememenu").hidden = true;
  $("themebtn").setAttribute("aria-expanded", "false");
  if (refocus) $("themebtn").focus();
}
$("themebtn").addEventListener("click", () => { if ($("thememenu").hidden) openPicker(); else closePicker(false); });
$("themebtn").addEventListener("keydown", (e) => {
  if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); openPicker(e.key === "ArrowUp" ? "last" : "checked"); }
});
$("thememenu").addEventListener("keydown", (e) => {
  const items = menuItems(), i = items.indexOf(document.activeElement);
  const go = (n) => { e.preventDefault(); items[(n + items.length) % items.length].focus(); };
  if (e.key === "ArrowDown") go(i + 1);
  else if (e.key === "ArrowUp") go(i - 1);
  else if (e.key === "Home") go(0);
  else if (e.key === "End") go(items.length - 1);
  else if (e.key === "Escape") { e.preventDefault(); closePicker(true); }
  else if (e.key === "Tab") closePicker(false);
  else if ((e.key === "Enter" || e.key === " ") && i >= 0) { e.preventDefault(); items[i].click(); }
});
document.addEventListener("pointerdown", (e) => {
  if (!$("thememenu").hidden && !e.target.closest(".picker")) closePicker(false);
});
renderPicker();
loadTermFace();
setInterval(tickAges, 1000);
// While this page is in view it says so, once a minute: the panel reads ps and git for
// the dashboard only then (POST /api/seen, PANEL-11).
function seen() {
  if (document.visibilityState === "visible" && S && !offline()) fetch("/api/seen", { method: "POST" }).catch(() => {});
}
setInterval(seen, 60000);
document.addEventListener("visibilitychange", seen); // ages move in place; the page is patched only when the panel says something changed
connect();
