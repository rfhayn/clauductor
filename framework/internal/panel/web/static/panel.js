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
let S = null, offset = 0, es = null, selected = null;
try { selected = localStorage.getItem("clauductor-panel-lane"); } catch (e) {}

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
// stable key. A region (the nearest keyed element) that holds the active text
// selection is not touched until the selection is gone.
function key(e, k) { e.dataset.k = k; return e; }
// Handlers live on the element as data, so a patch can swap them without re-adding
// listeners: a kept element runs the handler of the newest description of it.
function dispatch(ev) { const f = this._h && this._h[ev.type]; if (f) f.call(this, ev); }
function on(e, type, fn) {
  (e._h || (e._h = {}))[type] = fn;
  e.addEventListener(type, dispatch);
  return e;
}
let selRegion = null;
function selectionRegion() {
  const s = window.getSelection();
  if (!s || s.isCollapsed || !s.rangeCount) return null;
  let n = s.getRangeAt(0).commonAncestorContainer;
  if (n.nodeType !== 1) n = n.parentElement;
  if (!n || n.closest(".xterm")) return null;
  return n.closest("[data-k]") || n;
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
  if (o === selRegion) return;
  syncAttrs(o, n);
  patch(o, n.childNodes);
}
// patch makes parent's children match kids, keeping every node it can.
function patch(parent, kids) {
  if (parent === selRegion) return;
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
      else if (o.nodeValue !== n.nodeValue) o.nodeValue = n.nodeValue;
    }
    const at = parent.childNodes[i];
    if (at !== node) parent.insertBefore(node, at || null);
  });
}
function patchInto(id, kids) { patch($(id), kids.filter(Boolean)); }
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
  const hold = selectionRegion();
  for (const e of document.querySelectorAll("span.age[data-at]")) {
    if (hold && hold.contains(e)) continue;
    setText(e, (e.dataset.pre || "") + ageText(+e.dataset.at) + (e.dataset.post || ""));
  }
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
    S = JSON.parse(e.data);
    heard(S.now);
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
async function probe() {
  clearTimeout(conn.timer);
  conn.nextAt = 0;
  renderConn();
  try {
    const r = await fetch("/api/state", { cache: "no-store" });
    if (r.status === 401) { lost(true); return; }
    if (!r.ok) throw new Error(r.status);
  } catch (e) { lost(conn.expired); return; }
  connect();
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
    setText(live, conn.state === "lost" ? "DISCONNECTED" : "CONNECTING");
    live.className = "live dead";
  } else {
    setText(live, off ? "DISCONNECTED" : "LIVE");
    live.className = off ? "live dead" : "live";
  }
  bar.hidden = !off;
  if (!off) return;
  const asOf = frozenAt ? hhmm(frozenAt) : "—";
  const wait = conn.nextAt ? Math.max(0, Math.ceil((conn.nextAt - Date.now()) / 1000)) : 0;
  const status = conn.expired
    ? "The panel restarted with a new token: open the URL clauductor panel printed, or run clauductor panel open."
    : conn.nextAt ? "Reconnecting: attempt " + conn.attempt + ", next try in " + wait + " s." : "Reconnecting now…";
  const retry = key(el("button", "btn", "RETRY NOW"), "retry");
  retry.type = "button";
  on(retry, "click", () => probe());
  patch(bar, [
    el("b", null, "⚠ DISCONNECTED"),
    el("span", null, "The panel is not answering. Everything below is as of " + asOf + ", and actions are off until it is back. "),
    key(el("span", "sub", status), "status"),
    conn.expired ? null : retry,
  ].filter(Boolean));
}

function gauge(id, v, expired) {
  const g = $(id), f = g.querySelector(".fill");
  // A window past its resets_at is dropped by the server: say "reset", not a number.
  setText(g.querySelector("b"), expired ? "reset" : pct(v));
  f.style.width = (v == null ? 0 : Math.min(100, v)) + "%";
  f.className = "fill" + (v >= 90 ? " s" : v >= 70 ? " h" : "");
}

// APPROX marks a status that is not a current `claude agents` reading (a failed or
// old poll, or hooks alone): shown, never trusted as current.
const APPROX = "≈ ";
const APPROX_TITLE = "approximate: not a current `claude agents` reading (the poll failed or is old, or only hooks know)";

function laneStatusText(l) {
  const a = l.approx ? APPROX : "";
  if (l.status === "waiting") return a + "waiting" + (l.waitingFor ? ": " + l.waitingFor : "");
  let t = a + (l.status === "none" ? "no session" : l.status);
  if (l.subagents.length) t += " · " + l.subagents.length + " subagent" + (l.subagents.length > 1 ? "s" : "");
  if (l.sessions.length > 1) t += " · " + l.sessions.length + " sessions";
  return t;
}
// How old an approximate status is: the age of the last good `claude agents` read.
function staleTag(approx) {
  if (!approx) return null;
  if (!S.agentsReadAt) return el("span", "stale-tag", "stale · hooks only");
  return age(S.agentsReadAt, "stale · read ", " ago");
}

function laneCard(l, quiet) {
  const c = el("div", "lane " + l.status + (l.stale ? " stale" : "") + (l.approx ? " old-reading" : "") + (l.id === selected ? " sel" : "") + (quiet ? " quiet" : ""), null, [
    el("div", "row", null, [el("span", "nm", l.name), Object.assign(el("span", "kind", l.terminal ? l.type + " · tmux" : l.type), { title: l.terminal ? l.type + " lane, with a terminal" : l.type })]),
    el("div", "sub", l.branch || "(detached)"),
  ]);
  if (!quiet) {
    const st = el("span", "sub state" + (l.status === "waiting" ? " hold" : ""), laneStatusText(l) + (l.stale ? " · no hooks" : ""));
    if (l.approx) st.title = APPROX_TITLE;
    c.appendChild(el("div", "row", null, [st, el("span", "sub", "ctx " + pct(l.ctxPct))]));
    const tag = staleTag(l.approx);
    if (tag) c.appendChild(el("div", "sub", null, [tag]));
    const bar = el("div", "ctx", null, [el("i")]);
    bar.firstChild.style.width = (l.ctxPct || 0) + "%";
    c.appendChild(bar);
    c.appendChild(l.lastEventAt ? el("div", "sub", "last: " + l.lastEvent + " · ", [age(l.lastEventAt, "", " ago")]) : el("div", "sub", "no hook event yet"));
  }
  on(c, "click", () => {
    selected = l.id; try { localStorage.setItem("clauductor-panel-lane", l.id); } catch (e) {}
    if (l.terminal) selectTerm(l.terminal); else render();
  });
  pressable(c, l.name + ", " + laneStatusText(l) + (l.approx ? ", stale" : ""));
  c.title = l.name + " · " + (l.branch || "(detached)");
  if (l.id === selected) c.setAttribute("aria-current", "true");
  return key(c, "lane:" + l.id);
}

// A clickable card or tab is reachable and operable from the keyboard too.
function pressable(e, label) {
  e.tabIndex = 0;
  e.setAttribute("role", "button");
  if (label) e.setAttribute("aria-label", label);
  on(e, "keydown", (ev) => {
    if (ev.target === e && (ev.key === "Enter" || ev.key === " ")) { ev.preventDefault(); e.click(); }
  });
}

function feedList(items, withLane, k) {
  if (!items.length) return key(el("div", "empty", "no events yet"), k + ":empty");
  const f = key(el("div", "feed"), k);
  for (const e of items) {
    const d = el("span", "d", (withLane ? e.name + " · " : "") + e.event);
    if (e.detail) d.appendChild(el("span", "dim", e.detail));
    d.title = (e.name || "") + " · " + e.event + (e.detail ? " · " + e.detail : "");
    f.appendChild(key(el("div", "ev", null, [el("span", "t", hhmm(e.at)), d]), "ev:" + e.at + ":" + (e.lane || "") + ":" + e.event + ":" + (e.detail || "")));
  }
  return f;
}

function sourceNote(src, what, k) {
  if (!src) return null;
  if (src.pending) return key(el("div", "empty", "reading " + what + "…"), k);
  if (!src.ok) return key(el("div", "card err", "cannot read " + what + ": " + src.error), k);
  return null;
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
        if (rest.length) li.appendChild(el("div", "dim", rest.join(" · ")));
        ul.appendChild(li);
      } else ul.appendChild(el("li", null, typeof it === "string" ? it : JSON.stringify(it)));
    }
    return ul;
  }
  if (v !== null && typeof v === "object") {
    const kv = el("div", "kv");
    for (const [k, x] of Object.entries(v)) {
      kv.appendChild(el("span", "dim", k));
      kv.appendChild(x !== null && typeof x === "object" ? renderJSON(x) : el("span", null, String(x)));
    }
    return kv;
  }
  return el("div", null, String(v));
}

function projectCard(c) {
  const card = el("div", "card" + (!c.source.pending && !c.source.ok ? " err" : ""), null, [
    el("div", "row", null, [el("b", null, c.title || c.id), c.source.at ? el("span", "sub", null, [age(c.source.at, "", " ago")]) : el("span", "sub")]),
  ]);
  if (c.source.pending) card.appendChild(el("div", "empty", "running…"));
  else if (!c.source.ok) card.appendChild(el("div", "stop", "cannot read: " + c.source.error));
  else if (c.output.kind === "json") card.appendChild(renderJSON(c.output.json));
  else if (!c.output.lines || !c.output.lines.length) card.appendChild(el("div", "empty", "(no output)"));
  else {
    const ul = el("ul");
    for (const line of c.output.lines) ul.appendChild(el("li", null, line.replace(/^\s*(?:[-*+]|\d+\.)\s+/, "")));
    card.appendChild(ul);
  }
  return key(card, "card:" + c.id);
}

// ---- v1: lane terminals -------------------------------------------------------
// Each tab is one tmux lane. A lane's xterm and its WebSocket live as long as the
// lane does, so switching tabs keeps scrollback and costs no reconnect. The page
// sends only {type: "input", data} and {type: "resize", cols, rows}.
//
// The keyboard (PANEL-6). Nothing ever moves focus into a terminal by itself: not
// loading the page, not picking a lane, not OPEN TERMINAL. The terminal is one stop
// in the Tab order (#termhost); Enter there, or a click, enters it. Inside, every key
// is claude's, Tab, Shift+Tab and Escape included, because that is what a terminal is
// for once you chose it. Ctrl+] leaves, back to the lane's tab, as in telnet. A
// double Escape does not leave: claude uses Esc Esc itself (to edit an earlier
// message), so taking it would break claude.
const terms = {};              // lane id → {id, host, term, fit, ws, retry, delay}
let selTerm = null, shownTerm = null;
let confirmAct = null;         // {id, action} awaiting the in-page confirmation
let actMsg = null, busyAct = null;
let pendingTerm = null;        // a lane to select as soon as a poll shows it
try { pendingTerm = localStorage.getItem("clauductor-panel-term"); } catch (e) {}
const IS_MAC = /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

function selectTerm(id) {
  selTerm = id;
  try { localStorage.setItem("clauductor-panel-term", id); } catch (e) {}
  const t = S && S.terminals.find((x) => x.id === id);
  if (t && t.worktree) selected = t.worktree;
  confirmAct = null;
  render();
}
function enterTerm() {
  const t = terms[selTerm];
  if (t && !t.host.hidden) t.term.focus();
}
function leaveTerm(id) {
  const tab = document.querySelector('#tabs [data-k="tab:' + CSS.escape(id) + '"]');
  if (tab) tab.focus(); else $("termhost").focus();
}
function renderHint() {
  const host = $("termhost"), a = document.activeElement;
  const inTerm = a && a.classList && a.classList.contains("xterm-helper-textarea") && host.contains(a);
  const sel = IS_MAC ? "⌥-drag selects text" : "Shift-drag selects text";
  const hint = $("termhint");
  // The line is always there, so entering the terminal never resizes it.
  if (inTerm) setText(hint, "Ctrl+] leaves the terminal · typing into " + selTerm + " · " + sel + " · the wheel scrolls its history");
  else if (a === host && terms[selTerm]) setText(hint, "Press Enter to type into the terminal");
  else if (terms[selTerm]) setText(hint, "Click the terminal, or Tab to it and press Enter, to type into it");
  else setText(hint, "");
  hint.classList.toggle("on", !!(inTerm || a === host));
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
function termFontSize() {
  return parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--term-size")) || 13;
}
// The theme's floor for text a program draws on any colour, ANSI or truecolor.
function termMinContrast() {
  return parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--term-min-contrast")) || 1;
}
function retheme() {
  const theme = termTheme(), size = termFontSize(), minC = termMinContrast();
  for (const t of Object.values(terms)) {
    t.term.options.theme = theme;
    t.term.options.minimumContrastRatio = minC;
    if (t.term.options.fontSize !== size) { t.term.options.fontSize = size; fitTerm(t); }
  }
  renderPicker();
  renderFavicon();
}
document.addEventListener("panel-theme", retheme);

function termSend(t, msg) {
  if (t.ws && t.ws.readyState === WebSocket.OPEN) t.ws.send(JSON.stringify(msg));
}

function ensureTerm(id) {
  if (terms[id]) return terms[id];
  const host = el("div", "term");
  host.hidden = true;
  $("termhost").appendChild(host);
  const term = new Terminal({
    fontFamily: 'Menlo, "JetBrains Mono", ui-monospace, SFMono-Regular, monospace', fontSize: termFontSize(),
    cursorBlink: !matchMedia("(prefers-reduced-motion: reduce)").matches, scrollback: 2000, macOptionIsMeta: true,
    // tmux asks for mouse reports (so the wheel scrolls its history), which would
    // take every drag too: Option-drag (Shift-drag elsewhere) still selects text.
    macOptionClickForcesSelection: true,
    theme: termTheme(), minimumContrastRatio: termMinContrast(),
    // Terminal output is untrusted. A link (OSC 8) opens only after an in-page
    // confirmation, and only http(s). Title escapes are ignored: nothing subscribes
    // to onTitleChange, so they never reach the DOM.
    linkHandler: { activate: (ev, uri) => askOpenLink(uri), allowNonHttpProtocols: false },
  });
  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.open(host);
  const t = { id, host, term, fit, ws: null, retry: null, delay: 1000, gone: false, focused: false };
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
    if (typeof e.data === "string") return; // {"type":"exit"}: onclose follows
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

// The terminal takes the height the centre column has left once the banners, the
// tabs and the controls under it are placed, so STOP and INTERRUPT are always on
// screen. On a narrow screen the page scrolls as a whole, and the terminal takes
// most of the window.
const NARROW = matchMedia("(max-width: 980px)");
function sizeTerm() {
  const host = $("termhost"), col = $("centre");
  let h;
  if (NARROW.matches) h = Math.max(320, Math.round(innerHeight * 0.7));
  else {
    const top = host.getBoundingClientRect().top - col.getBoundingClientRect().top + col.scrollTop;
    const pad = parseFloat(getComputedStyle(col).paddingBottom) || 0;
    const gap = parseFloat(getComputedStyle(col).rowGap) || 0;
    h = Math.max(220, Math.floor(col.clientHeight - top - $("termbar").offsetHeight - gap - pad));
  }
  if (host.style.height !== h + "px") host.style.height = h + "px";
  fitTerm(terms[selTerm]);
}
{
  const ro = new ResizeObserver(() => sizeTerm());
  for (const id of ["centre", "banners", "restorebar", "tabrow", "termbar"]) ro.observe($(id));
  NARROW.addEventListener("change", sizeTerm);
}

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

function tabLabel(x) {
  // One name per lane: the tab says the lane's name, and its type only when that adds something.
  return x.type && x.type !== x.id ? [el("span", null, x.id), el("span", "dim", x.type)] : [el("span", null, x.id)];
}

function renderTerminals(lane) {
  const ts = S.terminals || [];
  for (const id of Object.keys(terms)) if (!ts.find((x) => x.id === id)) disposeTerm(id);
  if (pendingTerm && ts.find((x) => x.id === pendingTerm)) { selTerm = pendingTerm; pendingTerm = null; }
  if (selTerm && !ts.find((x) => x.id === selTerm)) selTerm = null;
  if (!selTerm && ts.length) selTerm = (lane && lane.terminal) || ts[0].id;

  const add = $("addlane");
  add.disabled = !!S.startBlocked || offline();
  add.title = offline() ? "Disconnected from the panel" : S.startBlocked || "Start a lane: an interactive claude in its own tmux session";
  $("refresh").disabled = offline();

  const tabs = ts.map((x) => {
    const tab = el("div", "tab" + (x.id === selTerm ? " sel" : ""), null, [el("span", "dot " + x.status), ...tabLabel(x)]);
    tab.title = (x.branch || "") + " · " + x.path + " · " + (x.approx ? APPROX : "") + x.status + (x.orphan ? " · " + x.orphan : "") +
      (x.approx ? " · " + APPROX_TITLE : "");
    on(tab, "click", () => selectTerm(x.id));
    pressable(tab, "terminal " + x.id + ", " + (x.approx ? APPROX : "") + x.status);
    if (x.id === selTerm) tab.setAttribute("aria-current", "true");
    return key(tab, "tab:" + x.id);
  });
  if (!ts.length) tabs.push(key(el("div", "mh", "Terminals"), "tabs:none"));
  patchInto("tabs", tabs);

  const empty = $("termempty");
  empty.hidden = ts.length > 0;
  patch(empty, [
    el("div", null, "No lane is running yet."),
    el("div", "dim", "+ LANE starts one. Sessions started in a terminal of your own still show their status, but have no terminal here."),
    S.startBlocked ? el("div", "card err", S.startBlocked) : null,
  ].filter(Boolean));

  const cur = ts.find((x) => x.id === selTerm);
  for (const id of Object.keys(terms)) if (!ts.find((x) => x.id === id && x.running)) disposeTerm(id);
  if (cur && cur.running) ensureTerm(selTerm);
  for (const [id, t] of Object.entries(terms)) t.host.hidden = id !== selTerm;
  const orphan = $("termorphan");
  orphan.hidden = !(cur && !cur.running);
  if (cur && !cur.running) setText(orphan, "Lane " + cur.id + " is orphaned: " + (cur.orphan || "no tmux session") +
    ". RESUME restarts claude --resume " + cur.sessionId + " in " + cur.path + "; FORGET drops the record.");
  const host = $("termhost");
  host.setAttribute("aria-label", cur && cur.running ? "Terminal of lane " + cur.id + ". Enter types into it; Ctrl+] leaves." : "Terminal");
  if (selTerm !== shownTerm) {
    shownTerm = selTerm;
    const t = terms[selTerm];
    if (t) requestAnimationFrame(() => fitTerm(t));
  }
  renderTermBar(cur);
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
        button("OPEN LINK", "", () => { linkAsk = null; window.open(href, "_blank", "noopener,noreferrer"); render(); }),
        button("CANCEL", "", () => { linkAsk = null; render(); }, null, "b:link-cancel"));
    } else if (!t.running) {
      if (confirmAct && confirmAct.id === t.id) {
        kids.push(key(el("span", "confirm", "Forget lane " + t.id + "? It leaves the registry; its worktree and conversation stay."), "confirm"),
          button("CONFIRM FORGET", "danger", () => { confirmAct = null; laneAction(t.id, "forget"); }, null, null, true),
          button("CANCEL", "", () => { confirmAct = null; render(); focusKey("b:FORGET"); }, null, "b:cancel"));
      } else {
        kids.push(
          button("RESUME", "primary", () => laneAction(t.id, "resume"), "claude --resume " + (t.sessionId || "") + " in " + t.path, null, true),
          button("FORGET", "danger", () => { confirmAct = { id: t.id, action: "forget" }; render(); focusKey("b:cancel"); },
            "Remove it from the lane registry. The worktree and the conversation stay.", null, true));
      }
    } else if (confirmAct && confirmAct.id === t.id) {
      const stop = confirmAct.action === "stop";
      const back = stop ? "b:STOP LANE" : "b:RESTART";
      kids.push(
        key(el("span", "confirm", stopWords(t, !stop)), "confirm"),
        button(stop ? "CONFIRM STOP" : "CONFIRM RESTART", "danger", () => { const a = confirmAct; confirmAct = null; laneAction(t.id, a.action); }, null, null, true),
        button("CANCEL", "", () => { confirmAct = null; render(); focusKey(back); }, null, "b:cancel"));
    } else {
      const b = [
        button("ATTACH IN TERMINAL.APP", "", () => laneAction(t.id, "terminal-app"), "Open a Terminal.app window on this lane (tmux attach)", null, true),
        button("INTERRUPT (ESC)", "", () => laneAction(t.id, "interrupt"), "Press Escape in the lane", null, true),
        t.registered ? button("RESTART", "", () => { confirmAct = { id: t.id, action: "restart" }; render(); focusKey("b:cancel"); }, null, null, true) : null,
        button("STOP LANE", "danger", () => { confirmAct = { id: t.id, action: "stop" }; render(); focusKey("b:cancel"); }, null, null, true),
      ].filter(Boolean);
      for (const x of b) { if (busy) x.disabled = true; kids.push(x); }
    }
    if (t.orphan && t.running) kids.push(key(el("span", "hold sub", t.orphan), "orphan"));
    if (t.template) kids.push(key(el("span", "sub", "template " + t.template + " · first prompt " + (t.promptState || "—") + (t.promptNote ? ": " + t.promptNote : "")), "tpl"));
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
    if (model || effort) p += " · claude" + (model ? " --model " + model : "") + (effort ? " --effort " + effort : "");
    $("st-prompt").replaceChildren(el("div", null, "Once claude is idle, the panel types this, then Enter:"),
      el("div", "tpl-prompt", fillTpl(tpl.firstPrompt, name, issue)));
    if (untrusted) $("st-prompt").appendChild(el("div", "stop", "Templates are off: panel.json changed since you trusted it (clauductor panel trust)."));
  } else if (type && (type.model || type.effort)) p += " · claude" + (type.model ? " --model " + type.model : "") + (type.effort ? " --effort " + type.effort : "");
  $("st-preview").textContent = p;
  $("st-block").hidden = !S.startBlocked;
  $("st-block").textContent = S.startBlocked || "";
  $("st-go").disabled = !!S.startBlocked || offline();
}

function openStart() {
  if (!S) return;
  const tsel = $("st-tpl"), tprev = tsel.value;
  const none = el("option", null, "(none: an empty lane)"); none.value = "";
  tsel.replaceChildren(none, ...(S.templates || []).map((x) => { const o = el("option", null, (x.title || x.id) + " · " + x.laneType); o.value = x.id; return o; }));
  tsel.value = tprev && (S.templates || []).find((x) => x.id === tprev) ? tprev : "";
  $("st-quota").checked = false;
  const sel = $("st-type"), prev = sel.value;
  sel.replaceChildren(...(S.laneTypes || []).map((x) => { const o = el("option", null, x.name); o.value = x.name; return o; }));
  if (prev) sel.value = prev;
  const wt = $("st-wt");
  wt.replaceChildren(...S.lanes.concat(S.quietWorktrees).map((l) => { const o = el("option", null, l.name + " · " + l.path); o.value = l.path; return o; }));
  $("st-err").textContent = "";
  $("startdlg").hidden = false;
  stTypeChanged();
  $("st-name").focus();
}

function stTypeChanged() {
  const type = (S.laneTypes || []).find((x) => x.name === $("st-type").value);
  const mode = type && type.prefix ? "new" : "root";
  document.querySelector('input[name="st-mode"][value="' + mode + '"]').checked = true;
  if (mode === "root" && !$("st-name").value) $("st-name").value = slug(type ? type.name : "orchestrator");
  stUpdate();
}

function closeStart() { $("startdlg").hidden = true; }

$("addlane").addEventListener("click", openStart);
$("st-cancel").addEventListener("click", closeStart);
$("startdlg").addEventListener("keydown", (e) => { if (e.key === "Escape") closeStart(); });
$("st-type").addEventListener("change", stTypeChanged);
$("st-wt").addEventListener("change", () => { if (!$("st-name").value) $("st-name").value = slug($("st-wt").value.split("/").pop()); stUpdate(); });
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


// ---- v2: needs you, alerts, queues, restore, observability ----------------------
function jumpTo(n) {
  if (n.terminal && S.terminals.find((x) => x.id === n.terminal)) {
    selectTerm(n.terminal);
    // To the terminal's door, not inside it: Enter there types into claude.
    requestAnimationFrame(() => { const h = $("termhost"); h.focus({ preventScroll: true }); h.scrollIntoView({ block: "nearest" }); });
  } else { selected = n.lane; render(); }
}
function needKey(n) { return "need:" + n.lane + ":" + n.session + ":" + n.kind; }
function needCard(n, cls, k) {
  const done = cls.includes("done");
  const c = el("div", cls, null, [
    el("div", "row", null, [el("b", null, n.name), n.at ? el("span", "sub", null, [age(n.at, "", done ? " ago" : " waiting")]) : el("span", "sub")]),
    cls.includes("ask") ? el("span", "who-waits", n.severity === "block" ? "blocking" : "needs you") : null,
    el("div", null, (n.label || n.kind) + (n.text ? ": " + n.text : "")),
  ]);
  c.appendChild(button(n.terminal ? "OPEN TERMINAL" : "SHOW LANE", "jump", () => jumpTo(n), null, "jump"));
  return key(c, k);
}
function heading(text, k) { return key(el("h2", "mh", text), k); }
function asOf() { return offline() && frozenAt ? " · as of " + hm(frozenAt) : ""; }

function renderLeft() {
  const kids = [];
  // Needs you first, at every width: blocking states only. A finished turn is
  // "done", shown right under it.
  kids.push(heading("Needs you · " + S.needsYou.length + asOf(), "h:needs"));
  if (!S.needsYou.length) kids.push(key(el("div", "empty", "nothing blocked on you"), "needs:none"));
  for (const n of S.needsYou) kids.push(needCard(n, "card ask " + (n.severity || ""), needKey(n)));
  if ((S.done || []).length) {
    kids.push(heading("Done · your move · " + S.done.length, "h:done"));
    for (const n of S.done) kids.push(needCard(n, "card done", "done:" + needKey(n)));
  }
  kids.push(heading("Lanes · " + S.lanes.length + asOf(), "h:lanes"));
  kids.push(sourceNote(S.sources.agents, "claude agents", "src:agents"));
  if (!S.lanes.length) kids.push(key(el("div", "empty", "no Claude session in this project's worktrees"), "lanes:none"));
  for (const l of S.lanes) kids.push(laneCard(l, false));
  if (S.quietWorktrees.length) {
    kids.push(heading("Worktrees · no session · " + S.quietWorktrees.length, "h:quiet"));
    for (const l of S.quietWorktrees) kids.push(laneCard(l, true));
  }
  patchInto("left", kids);
}

// Waiting alerts repeat what Needs you already shows; the alert list keeps the rest.
function shownAlerts() {
  const asked = new Set(S.needsYou.map((n) => n.session));
  return (S.alerts || []).filter((a) => !(a.kind === "waiting" && asked.has(a.session)));
}
const AGED = { waiting: true, idle: true };
function renderAlerts(kids) {
  const all = S.alerts || [], al = shownAlerts();
  const ns = S.observe.notifier || {};
  const hidden = all.length - al.length;
  kids.push(heading("Alerts · " + al.length + (hidden ? " (+" + hidden + " in Needs you)" : "") + " · interruptions today " + (ns.interrupts || 0), "h:alerts"));
  if (!al.length) { kids.push(key(el("div", "empty", "no alert"), "alerts:none")); return; }
  const box = key(el("div", "card"), "alerts");
  for (const a of al) {
    const text = el("span", null, (a.name ? a.name + ": " : "") + a.text);
    if (AGED[a.kind] && a.since) text.appendChild(age(a.since, " · "));
    const kids2 = [el("span", "sev " + a.severity, a.kind.replace("_", " ")), text];
    // A row that leads somewhere is a button, so the keyboard reaches it too.
    const line = a.terminal || a.lane ? el("button", "alert jumpable", null, kids2) : el("div", "alert", null, kids2);
    if (line.tagName === "BUTTON") {
      line.type = "button";
      line.title = a.terminal ? "Open the terminal of " + (a.name || a.terminal) : "Show lane " + (a.name || "");
      on(line, "click", () => jumpTo(a));
    }
    box.appendChild(key(line, "alert:" + a.key));
  }
  if (!S.thresholds.notify) box.appendChild(key(el("div", "sub", "OS notifications are off (alerts.notify)"), "alerts:off"));
  kids.push(box);
}

let queueMsg = null;
async function queuePost(q, verb, body) {
  queueMsg = { q, text: verb + "…" }; render();
  try {
    const r = await fetch("/api/queues/" + encodeURIComponent(q) + "/" + verb, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
    const j = await r.json().catch(() => ({}));
    queueMsg = { q, text: r.ok ? verb + ": done" + (j.run ? " · log " + j.run.log : "") : (j.error || "HTTP " + r.status), err: !r.ok };
  } catch (e) { queueMsg = { q, text: String(e), err: true }; }
  render();
}

function leaseLine(l) {
  return el("span", null, (l.lane || "pid " + l.pid) + " · ", [age(l.started), document.createTextNode(l.cmd ? " · " + l.cmd : "")]);
}

function renderQueues(kids) {
  const qs = S.queues || [];
  if (!qs.length && S.queuesSource.ok) return;
  kids.push(heading("Queues · " + qs.length, "h:queues"));
  kids.push(sourceNote(S.queuesSource, "the queues", "src:queues"));
  for (const q of qs) {
    const c = el("div", "card queue", null, [el("b", null, q.title || q.id)]);
    if (!q.held) c.appendChild(el("div", "sub go", "free"));
    else if (q.holder) {
      const who = el("span", q.holder.stale ? "stop" : "go", "held by ", [leaseLine(q.holder)]);
      if (q.holder.stale) who.appendChild(document.createTextNode(" · stale"));
      c.appendChild(el("div", "who", null, [who, el("span", "sub", q.holder.ttl ? "ttl " + q.holder.ttl + "s" : "")]));
    } else c.appendChild(el("div", "sub hold", "held"));
    if (q.holderNote) c.appendChild(el("div", "sub hold", q.holderNote));
    if (q.waiters.length) {
      const ol = el("ol");
      for (const w of q.waiters) {
        const li = el("li", "who", null, [el("span", null, null, [leaseLine(w), document.createTextNode(w.cancelling ? " · cancelling" : "")])]);
        if (!w.cancelling) li.appendChild(button("CANCEL WAIT", "", () => queuePost(q.id, "cancel", { waiter: w.nonce }), "Ask this waiter to give up. The holder is never stopped.", "cancel:" + w.nonce, true));
        ol.appendChild(key(li, "w:" + w.nonce));
      }
      c.appendChild(el("div", "sub", "waiting, in order:"));
      c.appendChild(ol);
    } else c.appendChild(el("div", "sub", "nobody waiting"));
    if (q.hasCommand) {
      const lane = S.lanes.concat(S.quietWorktrees).find((l) => l.id === selected);
      const b = button("RUN IN " + (lane ? lane.name : "?"), "", () => queuePost(q.id, "run", { worktree: lane.path }),
        "Run this queue's command through lock-run in the selected lane's worktree; it waits its turn.", "run", true);
      if (!lane || (S.trust && S.trust.hash && !S.trust.trusted)) b.disabled = true;
      c.appendChild(b);
    }
    if (q.run) c.appendChild(el("div", "sub", "last RUN pid " + q.run.pid + (q.run.exit != null ? " · exit " + q.run.exit : " · running") + " · " + q.run.log));
    if (queueMsg && queueMsg.q === q.id) c.appendChild(el("div", queueMsg.err ? "stop sub" : "sub", queueMsg.text));
    kids.push(key(c, "queue:" + q.id));
  }
}

let restoreMsg = null, restoreBusy = false;
function renderRestore() {
  const bar = $("restorebar");
  const ids = S.restorable || [];
  bar.hidden = !ids.length && !restoreMsg;
  const row = el("div", "restore");
  // The one place a lost tmux session is announced, with the one control for it.
  row.appendChild(el("b", null, "RESTORE"));
  if (ids.length) {
    row.appendChild(el("span", null, ids.length + " lane" + (ids.length > 1 ? "s" : "") + " lost " + (ids.length > 1 ? "their" : "its") +
      " tmux session (a reboot, or the tmux server ended): " + ids.join(", ") + ". RESTORE ALL resumes each on its own session id."));
    if (S.quotaGuard) {
      const over = key(el("input"), "over");
      over.type = "checkbox"; over.id = "restore-over";
      row.appendChild(key(el("label", null, null, [over, document.createTextNode(S.quotaGuard + ". Restore anyway.")]), "overl"));
    }
    const b = button("RESTORE ALL", "primary", async () => {
      restoreBusy = true; restoreMsg = null; render();
      const over = $("restore-over");
      try {
        const r = await fetch("/api/lanes/restore-all", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ overrideQuota: !!(over && over.checked) }) });
        const j = await r.json().catch(() => ({}));
        if (!r.ok) restoreMsg = { text: j.error || "HTTP " + r.status, err: true };
        else {
          const res = j.result || {};
          restoreMsg = { text: "restored " + (res.restored || []).join(", ") + ((res.skipped || []).length ? " · skipped " + res.skipped.map((s) => s.id + " (" + s.reason + ")").join("; ") : "") };
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
    row.appendChild(button("DISMISS", "", () => { restoreMsg = null; render(); }));
  }
  patch(bar, [key(row, "restore")]);
}

// Each banner says what it is. The lost-tmux banner is left to the restore bar.
const BANNER_LABEL = { no_hooks: "NO HOOKS", dropped: "EVENTS DROPPED", untrusted: "CONFIG CHANGED", hooks: "HOOKS", registry: "LANE REGISTRY" };
const SOURCE_NAME = { worktrees: "git worktree list", agents: "claude agents", tmux: "the panel's tmux server" };
function renderBanners() {
  const kids = [];
  const items = S.bannerItems || S.banners.map((t) => ({ kind: "", text: t }));
  items.forEach((b, i) => {
    if (b.kind === "restore") return;
    const warn = b.kind === "untrusted" || b.kind === "dropped";
    kids.push(key(el("div", "banner" + (warn ? " warn" : ""), null, [el("b", null, BANNER_LABEL[b.kind] || "NOTICE"), document.createTextNode(b.text)]), "banner:" + b.kind + ":" + i));
  });
  for (const [k, src] of Object.entries(S.sources)) {
    if (k !== "prs" && !src.pending && !src.ok) kids.push(key(el("div", "banner", null, [el("b", null, "CANNOT READ"), document.createTextNode((SOURCE_NAME[k] || k) + ": " + src.error)]), "banner:src:" + k));
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
  const toggle = button(obsOpen ? "FEWER" : "ALL COUNTERS", "obstoggle", () => {
    obsOpen = !obsOpen;
    try { localStorage.setItem("clauductor-panel-obs", obsOpen ? "open" : "closed"); } catch (e) {}
    render();
  }, obsOpen ? "Show the main counters only" : "Show every counter the panel keeps", "obstoggle");
  toggle.setAttribute("aria-expanded", String(obsOpen));
  toggle.setAttribute("aria-controls", "obs");
  const drops = o.droppedForeign + o.overflowDrops + o.malformedDrops + o.droppedUnknownEvent;
  const kids = [toggle, kv("events", o.hookEvents), kv("status posts", o.statusPosts), kv("dropped", drops),
    kv("notifications", o.notifySent + (o.notifyFailed ? " · failed " + o.notifyFailed : ""))];
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
  const bg = tok(off ? "--idle" : "--accent"), ink = tok("--surface");
  let svg = "<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'><rect x='1' y='1' width='30' height='30' rx='7' fill='" + bg + "'/>";
  if (off) svg += "<text x='16' y='23' font-family='sans-serif' font-size='20' font-weight='700' text-anchor='middle' fill='" + ink + "'>!</text>";
  else if (n) svg += "<circle cx='21' cy='11' r='10' fill='" + tok("--stop") + "' stroke='" + ink + "' stroke-width='2'/>" +
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
  selRegion = selectionRegion();
  const off = offline();
  const n = S.needsYou.length;
  document.title = (off ? "⚠ DISCONNECTED · " : "") + (n ? "(" + n + ") " : "") + S.name + " · Panel";
  setText($("pname"), S.name);
  gauge("g5", S.quota ? S.quota.fiveHour : null, S.quota && S.quota.fiveHourExpired);
  gauge("g7", S.quota ? S.quota.sevenDay : null, S.quota && S.quota.sevenDayExpired);
  setText($("cost"), S.estCostUsd == null ? "—" : S.estCostUsd.toFixed(2));
  setText($("hookn"), S.hookEvents + " · status " + S.statusPosts + (S.dropped ? " · dropped " + S.dropped : ""));
  renderConn();
  renderBanners();
  renderRestore();
  renderObs();
  renderFavicon();
  if (!off) announceNeeds();

  renderLeft();
  // Centre: the selected lane.
  const all = S.lanes.concat(S.quietWorktrees);
  let lane = all.find((l) => l.id === selected) || S.lanes[0] || all[0];
  if (lane && lane.id !== selected) selected = lane.id;
  renderTerminals(lane);
  const centre = [];
  if (lane) {
    const st = el("span", "chip " + ({ busy: "go", waiting: "hold" }[lane.status] || ""), (lane.approx ? APPROX : "") + (lane.status === "none" ? "no session" : lane.status));
    if (lane.approx) st.title = APPROX_TITLE;
    const chips = el("div", "chips", null, [
      el("span", "chip sig", lane.type),
      lane.terminal ? el("span", "chip", "tmux lane") : null,
      st,
      lane.approx ? el("span", "chip hold", null, [staleTag(true)]) : null,
      el("span", "chip", "ctx " + pct(lane.ctxPct)),
      el("span", "chip", null, lane.lastHookAt ? [age(lane.lastHookAt, "last hook ", " ago")] : [document.createTextNode("last hook never")]),
    ]);
    if (lane.stale) chips.appendChild(el("span", "chip stop", "no hooks while busy"));
    const sum = el("div", "summary", null, [
      el("h2", null, lane.name), chips,
      el("div", "sub", lane.branch + " · " + lane.path),
    ]);
    if (lane.sessions.length) {
      const tb = el("tbody");
      for (const s of lane.sessions) {
        tb.appendChild(key(el("tr", null, null, [
          el("td", null, s.name || s.id.slice(0, 8)), el("td", null, s.pid ? String(s.pid) : "—"), el("td", null, s.kind || "hooks only"),
          el("td", { busy: "go", waiting: "hold" }[s.status] || "", (s.approx ? APPROX : "") + s.status + (s.waitingFor ? " · " + s.waitingFor : "") +
            (s.compacting ? " · compacting (" + s.compacting + ")" : "") + (s.failure ? " · failed: " + s.failure : "") +
            (s.unknownNotification ? " · unknown notification " + s.unknownNotification : "") + (s.agentState ? " · " + s.agentState : "")),
          el("td", null, pct(s.ctxPct)), el("td", null, s.model || "—"),
          el("td", null, s.estCostUsd == null ? "—" : "$" + s.estCostUsd.toFixed(2)),
        ]), "s:" + s.id));
      }
      const th = el("tr", null, null, ["session", "pid", "kind", "status", "ctx", "model", "est. $"].map((h) => el("th", null, h)));
      sum.appendChild(el("div", "tablewrap", null, [el("table", null, null, [el("thead", null, null, [th]), tb])]));
    }
    sum.appendChild(el("div", "mh", "Running subagents · " + lane.subagents.length));
    if (lane.subagentsApprox) sum.appendChild(el("div", "approx sub", "approximate: the pairing was verified on Claude Code " + S.observe.verifiedOn + ", and " + (S.observe.claudeVersion || "an unknown version") + " is running"));
    if (lane.subagents.length) {
      const ul = el("div", "feed");
      for (const a of lane.subagents) ul.appendChild(key(el("div", "ev", null, [el("span", "t", null, [age(a.since)]), el("span", "d", (a.type || "(untyped)") + " " + a.id.slice(0, 7))]), "sa:" + a.id));
      sum.appendChild(ul);
    } else sum.appendChild(el("div", "empty", "none"));
    centre.push(key(sum, "sum:" + lane.id));
    centre.push(key(el("div", "mh", "Events · " + lane.name), "h:events"));
    centre.push(feedList(S.feed.filter((e) => e.lane === lane.id), false, "lanefeed"));
  } else centre.push(key(el("div", "empty", "No lane yet: start one with + LANE."), "nolane"));
  patchInto("detail", centre);

  // Right: alerts, queues, the project's cards, PRs, the feed.
  const right = [];
  renderAlerts(right);
  renderQueues(right);
  for (const c of S.cards) right.push(projectCard(c));
  right.push(heading("Open PRs · " + (S.sources.prs.ok ? S.prs.length : "?"), "h:prs"));
  right.push(sourceNote(S.sources.prs, "gh pr list", "src:prs"));
  if (S.sources.prs.ok && !S.prs.length) right.push(key(el("div", "empty", "none open"), "prs:none"));
  for (const p of S.prs) {
    const checks = p.checksFail ? el("span", "stop", p.checksFail + " failing") : p.checksPending ? el("span", "hold", p.checksPending + " pending")
      : p.checksPass ? el("span", "go", p.checksPass + " passing") : el("span", "dim", "no checks");
    right.push(key(el("div", "card" + (S.sources.prs.ok ? "" : " dim"), null, [
      el("div", "mono", "#" + p.number + " " + p.headRefName),
      el("div", null, p.title),
      el("div", "sub", null, [document.createTextNode((p.isDraft ? "draft · " : "") + p.author + " · "), checks]),
    ]), "pr:" + p.number));
  }
  right.push(heading("Feed", "h:feed"));
  right.push(feedList(S.feed.slice(0, 60), true, "feed"));
  patchInto("right", right);
  renderHint();
}

$("refresh").addEventListener("click", () => { if (!offline()) fetch("/api/refresh", { method: "POST" }).catch(() => {}); });

// ---- Themes: the picker ---------------------------------------------------------
// A menu button (WAI-ARIA APG): Enter, Space or Down opens it on the checked item, Up on
// the last; Up/Down/Home/End move; Enter or Space picks; Escape closes and returns
// focus to the button; Tab closes. Picking applies at once and persists (theme.js).
const MODE_LABEL = { system: "System", light: "Light", dark: "Dark" };
function menuItems() { return Array.from($("thememenu").querySelectorAll('[role="menuitemradio"]')); }
function renderPicker() {
  const P = window.PanelTheme, cur = P.get(), menu = $("thememenu");
  const th = P.themes.find((x) => x.id === cur.theme);
  $("themename").textContent = th.name.toUpperCase();
  $("themebtn").setAttribute("aria-label", "Theme: " + th.name + ", mode: " + MODE_LABEL[cur.mode]);
  const focusedKey = menu.contains(document.activeElement) ? document.activeElement.dataset.key : null;
  const item = (key, checked, kids, onPick) => {
    const e = el("div", "mi", null, kids);
    e.setAttribute("role", "menuitemradio");
    e.setAttribute("aria-checked", String(checked));
    e.tabIndex = -1;
    e.dataset.key = key;
    e.addEventListener("click", () => { onPick(); closePicker(true); });
    return e;
  };
  // The headings are visual only: each group carries its own aria-label.
  const heading = (t) => { const h = el("div", "mh", t); h.setAttribute("aria-hidden", "true"); return h; };
  const themes = el("div", null, null, [heading("Theme")]);
  themes.setAttribute("role", "group");
  themes.setAttribute("aria-label", "Theme");
  for (const x of P.themes) {
    const sw = el("span", "sw", null, [el("b", null, "Aa"), el("i"), el("i"), el("i"), el("i")]);
    sw.setAttribute("data-theme", x.id);
    sw.setAttribute("data-mode", cur.resolved);
    sw.setAttribute("aria-hidden", "true");
    themes.appendChild(item("t:" + x.id, x.id === cur.theme,
      [sw, el("span", null, x.name, [el("small", null, x.note)]), el("span", "tick", "✓")],
      () => P.setTheme(x.id)));
  }
  const modes = el("div", "modes");
  modes.setAttribute("role", "group");
  modes.setAttribute("aria-label", "Mode");
  for (const m of P.modes) modes.appendChild(item("m:" + m, m === cur.mode, [el("span", null, MODE_LABEL[m])], () => P.setMode(m)));
  menu.replaceChildren(themes, heading("Mode"), modes);
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
setInterval(tickAges, 1000); // ages move in place; the page is patched only when the panel says something changed
connect();
