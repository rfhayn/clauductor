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
const now = () => Date.now() - offset;
function age(ms) {
  if (!ms) return "—";
  const s = Math.max(0, Math.round((now() - ms) / 1000));
  if (s < 60) return s + "s";
  if (s < 3600) return Math.floor(s / 60) + "m";
  if (s < 86400) return Math.floor(s / 3600) + "h" + String(Math.floor((s % 3600) / 60)).padStart(2, "0");
  return Math.floor(s / 86400) + "d";
}
function hhmm(ms) { const d = new Date(ms); return String(d.getHours()).padStart(2, "0") + ":" + String(d.getMinutes()).padStart(2, "0") + ":" + String(d.getSeconds()).padStart(2, "0"); }
function pct(v) { return v == null ? "—" : Math.round(v) + "%"; }

function setLive(ok, msg) {
  const l = $("live");
  l.textContent = ok ? "LIVE" : (msg || "DISCONNECTED");
  l.className = ok ? "live" : "live dead";
}

function connect() {
  if (es) es.close();
  es = new EventSource("/events");
  es.addEventListener("state", (e) => {
    S = JSON.parse(e.data);
    offset = Date.now() - S.now;
    setLive(true);
    render();
  });
  es.onerror = () => { setLive(false); es.close(); es = null; setTimeout(probe, 2000); };
}
// EventSource gives up silently on a 401 (e.g. the panel restarted with a new token),
// so probe with fetch to tell "server down" from "session expired".
async function probe() {
  try {
    const r = await fetch("/api/state", { cache: "no-store" });
    if (r.status === 401) { setLive(false, "DISCONNECTED · open the new URL printed by clauductor panel"); setTimeout(probe, 5000); return; }
    if (!r.ok) throw new Error(r.status);
  } catch (e) { setLive(false); setTimeout(probe, 2000); return; }
  connect();
}

function gauge(id, v, expired) {
  const g = $(id), f = g.querySelector(".fill");
  // A window past its resets_at is dropped by the server: say "reset", not a number.
  g.querySelector("b").textContent = expired ? "reset" : pct(v);
  f.style.width = (v == null ? 0 : Math.min(100, v)) + "%";
  f.className = "fill" + (v >= 90 ? " s" : v >= 70 ? " h" : "");
}

function laneStatusText(l) {
  if (l.status === "waiting") return "waiting" + (l.waitingFor ? ": " + l.waitingFor : "");
  let t = l.status === "none" ? "no session" : l.status;
  if (l.subagents.length) t += " · " + l.subagents.length + " subagent" + (l.subagents.length > 1 ? "s" : "");
  if (l.sessions.length > 1) t += " · " + l.sessions.length + " sessions";
  return t;
}

function laneCard(l, quiet) {
  const c = el("div", "lane " + l.status + (l.stale ? " stale" : "") + (l.id === selected ? " sel" : "") + (quiet ? " quiet" : ""), null, [
    el("div", "row", null, [el("span", "nm", l.name), el("span", "kind", l.terminal ? l.type + " · tmux" : l.type)]),
    el("div", "sub", l.branch || "(detached)"),
  ]);
  if (!quiet) {
    c.appendChild(el("div", "row", null, [
      el("span", "sub state" + (l.status === "waiting" ? " hold" : ""), laneStatusText(l) + (l.stale ? " · no hooks" : "")),
      el("span", "sub", "ctx " + pct(l.ctxPct)),
    ]));
    const bar = el("div", "ctx", null, [el("i")]);
    bar.firstChild.style.width = (l.ctxPct || 0) + "%";
    c.appendChild(bar);
    c.appendChild(el("div", "sub", l.lastEventAt ? "last: " + l.lastEvent + " · " + age(l.lastEventAt) + " ago" : "no hook event yet"));
  }
  c.addEventListener("click", () => {
    selected = l.id; try { localStorage.setItem("clauductor-panel-lane", l.id); } catch (e) {}
    if (l.terminal) selectTerm(l.terminal); else render();
  });
  pressable(c, l.name + ", " + laneStatusText(l));
  c.title = l.name + " · " + (l.branch || "(detached)");
  c.dataset.fk = "lane:" + l.id;
  return c;
}

// A clickable card or tab is reachable and operable from the keyboard too.
function pressable(e, label) {
  e.tabIndex = 0;
  e.setAttribute("role", "button");
  if (label) e.setAttribute("aria-label", label);
  e.addEventListener("keydown", (ev) => {
    if (ev.target === e && (ev.key === "Enter" || ev.key === " ")) { ev.preventDefault(); e.click(); }
  });
}

function feedList(items, withLane) {
  if (!items.length) return el("div", "empty", "no events yet");
  const f = el("div", "feed");
  for (const e of items) {
    const d = el("span", "d", (withLane ? e.name + " · " : "") + e.event);
    if (e.detail) d.appendChild(el("span", "dim", e.detail));
    d.title = (e.name || "") + " · " + e.event + (e.detail ? " · " + e.detail : "");
    f.appendChild(el("div", "ev", null, [el("span", "t", hhmm(e.at)), d]));
  }
  return f;
}

function sourceNote(src, what) {
  if (!src) return null;
  if (src.pending) return el("div", "empty", "reading " + what + "…");
  if (!src.ok) return el("div", "card err", "cannot read " + what + ": " + src.error);
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
    el("div", "row", null, [el("b", null, c.title || c.id), el("span", "sub", c.source.at ? age(c.source.at) + " ago" : "")]),
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
  return card;
}

// ---- v1: lane terminals -------------------------------------------------------
// Each tab is one tmux lane. A lane's xterm and its WebSocket live as long as the
// lane does, so switching tabs keeps scrollback and costs no reconnect. The page
// sends only {type: "input", data} and {type: "resize", cols, rows}.
const terms = {};              // lane id → {id, host, term, fit, ws, retry, delay}
let selTerm = null, shownTerm = null, tabSig = "", barSig = "", emptySig = "";
let confirmAct = null;         // {id, action} awaiting the in-page confirmation
let actMsg = null, busyAct = null;
let pendingTerm = null;        // a lane to select as soon as a poll shows it
try { pendingTerm = localStorage.getItem("clauductor-panel-term"); } catch (e) {}

function selectTerm(id) {
  selTerm = id;
  try { localStorage.setItem("clauductor-panel-term", id); } catch (e) {}
  const t = S && S.terminals.find((x) => x.id === id);
  if (t && t.worktree) selected = t.worktree;
  confirmAct = null;
  render();
  if (terms[id]) terms[id].term.focus();
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
function retheme() {
  const theme = termTheme(), size = termFontSize();
  for (const t of Object.values(terms)) {
    t.term.options.theme = theme;
    if (t.term.options.fontSize !== size) { t.term.options.fontSize = size; fitTerm(t); }
  }
  renderPicker();
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
    cursorBlink: true, scrollback: 2000, macOptionIsMeta: true,
    theme: termTheme(),
    // Terminal output is untrusted. A link (OSC 8) opens only after an in-page
    // confirmation, and only http(s). Title escapes are ignored: nothing subscribes
    // to onTitleChange, so they never reach the DOM.
    linkHandler: { activate: (ev, uri) => askOpenLink(uri), allowNonHttpProtocols: false },
  });
  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.open(host);
  const t = { id, host, term, fit, ws: null, retry: null, delay: 1000, gone: false, focused: false };
  term.onData((d) => termSend(t, { type: "input", data: d }));
  // Tell the server which terminal has keyboard focus: alerts for that lane send no
  // OS notification, because you are looking at it.
  if (term.textarea) {
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

let linkAsk = null;
function askOpenLink(uri) {
  let u;
  try { u = new URL(uri); } catch (e) { return; }
  if (u.protocol !== "http:" && u.protocol !== "https:") return;
  linkAsk = u.href;
  barSig = "";
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

function button(label, cls, onClick, title) {
  const b = el("button", "btn" + (cls ? " " + cls : ""), label);
  b.type = "button";
  if (title) b.title = title;
  b.addEventListener("click", onClick);
  return b;
}

function renderTerminals(lane) {
  const ts = S.terminals || [];
  for (const id of Object.keys(terms)) if (!ts.find((x) => x.id === id)) disposeTerm(id);
  if (pendingTerm && ts.find((x) => x.id === pendingTerm)) { selTerm = pendingTerm; pendingTerm = null; }
  if (selTerm && !ts.find((x) => x.id === selTerm)) selTerm = null;
  if (!selTerm && ts.length) selTerm = (lane && lane.terminal) || ts[0].id;

  const add = $("addlane");
  add.disabled = !!S.startBlocked;
  add.title = S.startBlocked || "Start a lane: an interactive claude in its own tmux session";

  const sig = JSON.stringify([ts.map((x) => [x.id, x.status, x.type, x.orphan]), selTerm]);
  if (sig !== tabSig) {
    tabSig = sig;
    const tabs = $("tabs");
    tabs.replaceChildren();
    for (const x of ts) {
      const tab = el("div", "tab" + (x.id === selTerm ? " sel" : ""), null, [el("span", "dot " + x.status), el("span", null, x.id), el("span", "dim", x.type || "")]);
      tab.title = (x.branch || "") + " · " + x.path + " · " + x.status + (x.orphan ? " · " + x.orphan : "");
      tab.addEventListener("click", () => selectTerm(x.id));
      pressable(tab, "terminal " + x.id + ", " + x.status);
      tabs.appendChild(tab);
    }
    if (!ts.length) tabs.appendChild(el("div", "mh", "Terminals"));
  }

  const esig = JSON.stringify([ts.length, S.startBlocked, S.tmuxSocket]);
  if (esig !== emptySig) {
    emptySig = esig;
    const empty = $("termempty");
    empty.hidden = ts.length > 0;
    empty.replaceChildren(
      el("div", null, "No lane is running on tmux socket " + S.tmuxSocket + "."),
      el("div", "dim", "+ LANE starts one. Sessions started in a terminal of your own still show their status below, but have no terminal here."));
    if (S.startBlocked) empty.appendChild(el("div", "card err", S.startBlocked));
  }

  const cur = ts.find((x) => x.id === selTerm);
  for (const id of Object.keys(terms)) if (!ts.find((x) => x.id === id && x.running)) disposeTerm(id);
  if (cur && cur.running) ensureTerm(selTerm);
  for (const [id, t] of Object.entries(terms)) t.host.hidden = id !== selTerm;
  const orphan = $("termorphan");
  orphan.hidden = !(cur && !cur.running);
  if (cur && !cur.running) orphan.textContent = "Lane " + cur.id + " is orphaned: " + (cur.orphan || "no tmux session") +
    ". RESUME restarts claude --resume " + cur.sessionId + " in " + cur.path + "; FORGET drops the record.";
  if (selTerm !== shownTerm) {
    shownTerm = selTerm;
    const t = terms[selTerm];
    if (t) requestAnimationFrame(() => { fitTerm(t); t.term.focus(); });
  }
  renderTermBar(ts.find((x) => x.id === selTerm));
}

function renderTermBar(t) {
  const sig = JSON.stringify([t && t.id, t && t.dead, t && t.running, t && t.orphan, t && t.promptState, t && t.promptNote, confirmAct, actMsg, busyAct, linkAsk]);
  if (sig === barSig) return;
  barSig = sig;
  const bar = $("termbar");
  bar.replaceChildren();
  if (!t) return;
  const busy = busyAct && busyAct.startsWith(t.id + ":");
  if (linkAsk) {
    const href = linkAsk;
    bar.append(el("span", "confirm", "The lane printed a link. Open " + href + " in a new tab?"),
      button("OPEN LINK", "", () => { linkAsk = null; window.open(href, "_blank", "noopener,noreferrer"); render(); }),
      button("CANCEL", "", () => { linkAsk = null; render(); }));
  } else if (!t.running) {
    bar.append(
      button("RESUME", "primary", () => laneAction(t.id, "resume"), "claude --resume " + (t.sessionId || "") + " in " + t.path),
      button("FORGET", "danger", () => { confirmAct = { id: t.id, action: "forget" }; render(); }, "Remove it from the lane registry. The worktree and the conversation stay."));
    if (confirmAct && confirmAct.id === t.id) {
      bar.replaceChildren(el("span", "confirm", "Forget lane " + t.id + "? It leaves the registry; its worktree and conversation stay."),
        button("CONFIRM FORGET", "danger", () => { confirmAct = null; laneAction(t.id, "forget"); }),
        button("CANCEL", "", () => { confirmAct = null; render(); }));
    }
  } else if (confirmAct && confirmAct.id === t.id) {
    const stop = confirmAct.action === "stop";
    bar.append(
      el("span", "confirm", stop
        ? "Stop lane " + t.id + "? Claude gets /exit, then its tmux session ends. The worktree stays."
        : "Restart lane " + t.id + "? It stops, then claude resumes its own session " + t.sessionId + " in the same directory."),
      button(stop ? "CONFIRM STOP" : "CONFIRM RESTART", "danger", () => { const a = confirmAct; confirmAct = null; laneAction(t.id, a.action); }),
      button("CANCEL", "", () => { confirmAct = null; render(); }));
  } else {
    const b = [
      button("ATTACH IN TERMINAL.APP", "", () => laneAction(t.id, "terminal-app"), "Open a Terminal.app window on this lane (tmux attach)"),
      button("INTERRUPT (ESC)", "", () => laneAction(t.id, "interrupt"), "Press Escape in the lane"),
      button("RESTART", "", () => { confirmAct = { id: t.id, action: "restart" }; render(); }),
      button("STOP LANE", "danger", () => { confirmAct = { id: t.id, action: "stop" }; render(); }),
    ];
    if (!t.registered) b.splice(2, 1); // no session id to resume
    for (const x of b) { x.disabled = !!busy; bar.appendChild(x); }
  }
  if (t.orphan) bar.appendChild(el("span", "hold sub", t.orphan));
  if (t.template) bar.appendChild(el("span", "sub", "template " + t.template + " · first prompt " + (t.promptState || "—") + (t.promptNote ? ": " + t.promptNote : "")));
  if (t.dead) bar.appendChild(el("span", "stop sub", "claude exited" + (t.deadStatus ? " (status " + t.deadStatus + ")" : "") + "; RESTART or STOP"));
  if (busy) bar.appendChild(el("span", "sub", busyAct.split(":")[1] + "…"));
  if (actMsg && actMsg.id === t.id) bar.appendChild(el("span", actMsg.err ? "stop sub" : "sub", actMsg.text));
}

new ResizeObserver(() => fitTerm(terms[selTerm])).observe($("termhost"));

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
  $("st-go").disabled = !!S.startBlocked;
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
  finally { $("st-go").disabled = !!(S && S.startBlocked); }
});

// ---- v2: needs you, alerts, queues, restore, observability ----------------------
function jumpTo(n) {
  if (n.terminal && S.terminals.find((x) => x.id === n.terminal)) selectTerm(n.terminal);
  else { selected = n.lane; render(); }
}
function needCard(n, cls) {
  const c = el("div", cls, null, [
    el("div", "row", null, [el("b", null, n.name), el("span", "sub", n.at ? age(n.at) + (cls.includes("done") ? " ago" : " waiting") : "")]),
    cls.includes("ask") ? el("span", "who-waits", n.severity === "block" ? "blocking" : "needs you") : null,
    el("div", null, (n.label || n.kind) + (n.text ? ": " + n.text : "")),
  ]);
  c.appendChild(button(n.terminal ? "OPEN TERMINAL" : "SHOW LANE", "jump", () => jumpTo(n)));
  return c;
}

function renderAlerts(right) {
  const al = S.alerts || [];
  const ns = S.observe.notifier || {};
  right.appendChild(el("div", "mh", "Alerts · " + al.length + " · interruptions today " + (ns.interrupts || 0)));
  if (!al.length) { right.appendChild(el("div", "empty", "no alert")); return; }
  const box = el("div", "card");
  for (const a of al) {
    const line = el("div", "alert", null, [el("span", "sev " + a.severity, a.kind.replace("_", " ")),
      el("span", null, (a.name ? a.name + ": " : "") + a.text)]);
    if (a.terminal || a.lane) { line.style.cursor = "pointer"; line.addEventListener("click", () => jumpTo(a)); }
    box.appendChild(line);
  }
  if (!S.thresholds.notify) box.appendChild(el("div", "sub", "OS notifications are off (alerts.notify)"));
  right.appendChild(box);
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
  return (l.lane || "pid " + l.pid) + " · " + age(l.started) + (l.cmd ? " · " + l.cmd : "");
}

function renderQueues(right) {
  const qs = S.queues || [];
  if (!qs.length && S.queuesSource.ok) return;
  right.appendChild(el("div", "mh", "Queues · " + qs.length));
  const src = sourceNote(S.queuesSource, "the queues");
  if (src) right.appendChild(src);
  for (const q of qs) {
    const c = el("div", "card queue", null, [el("b", null, q.title || q.id)]);
    if (!q.held) c.appendChild(el("div", "sub go", "free"));
    else if (q.holder) {
      c.appendChild(el("div", "who", null, [el("span", q.holder.stale ? "stop" : "go", "held by " + leaseLine(q.holder) + (q.holder.stale ? " · stale" : "")),
        el("span", "sub", q.holder.ttl ? "ttl " + q.holder.ttl + "s" : "")]));
    } else c.appendChild(el("div", "sub hold", "held"));
    if (q.holderNote) c.appendChild(el("div", "sub hold", q.holderNote));
    if (q.waiters.length) {
      const ol = el("ol");
      for (const w of q.waiters) {
        const li = el("li", "who", null, [el("span", null, leaseLine(w) + (w.cancelling ? " · cancelling" : ""))]);
        if (!w.cancelling) li.appendChild(button("CANCEL WAIT", "", () => queuePost(q.id, "cancel", { waiter: w.nonce }), "Ask this waiter to give up. The holder is never stopped."));
        ol.appendChild(li);
      }
      c.appendChild(el("div", "sub", "waiting, in order:"));
      c.appendChild(ol);
    } else c.appendChild(el("div", "sub", "nobody waiting"));
    if (q.hasCommand) {
      const lane = S.lanes.concat(S.quietWorktrees).find((l) => l.id === selected);
      const b = button("RUN IN " + (lane ? lane.name : "?"), "", () => queuePost(q.id, "run", { worktree: lane.path }),
        "Run this queue's command through lock-run in the selected lane's worktree; it waits its turn.");
      b.disabled = !lane || (S.trust && S.trust.hash && !S.trust.trusted);
      c.appendChild(b);
    }
    if (q.run) c.appendChild(el("div", "sub", "last RUN pid " + q.run.pid + (q.run.exit != null ? " · exit " + q.run.exit : " · running") + " · " + q.run.log));
    if (queueMsg && queueMsg.q === q.id) c.appendChild(el("div", queueMsg.err ? "stop sub" : "sub", queueMsg.text));
    right.appendChild(c);
  }
}

let restoreMsg = null, restoreBusy = false;
function renderRestore() {
  const bar = $("restorebar");
  const ids = S.restorable || [];
  const sig = JSON.stringify([ids, S.quotaGuard, restoreMsg, restoreBusy]);
  if (bar.dataset.sig === sig) return;
  bar.dataset.sig = sig;
  bar.hidden = !ids.length && !restoreMsg;
  bar.replaceChildren();
  const row = el("div", "restore");
  if (ids.length) {
    row.appendChild(el("span", null, ids.length + " lane(s) lost their tmux session: " + ids.join(", ") + "."));
    let over = null;
    if (S.quotaGuard) {
      over = el("input"); over.type = "checkbox";
      row.appendChild(el("label", null, null, [over, document.createTextNode(S.quotaGuard + ". Restore anyway.")]));
    }
    const b = button("RESTORE ALL", "primary", async () => {
      restoreBusy = true; restoreMsg = null; render();
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
    }, "claude --resume <its own session id> in each lane's worktree; never --continue, never twice");
    b.disabled = restoreBusy || !!S.startBlocked;
    row.appendChild(b);
  }
  if (restoreMsg) {
    row.appendChild(el("span", restoreMsg.err ? "stop sub" : "sub", restoreMsg.text));
    row.appendChild(button("DISMISS", "", () => { restoreMsg = null; render(); }));
  }
  bar.appendChild(row);
}

function renderObs() {
  const o = S.observe;
  const kv = (k, v) => el("span", null, null, [document.createTextNode(k + " "), el("b", null, String(v))]);
  $("obs").replaceChildren(
    kv("events", o.hookEvents), kv("status posts", o.statusPosts),
    kv("dropped: foreign cwd", o.droppedForeign), kv("overflow", o.overflowDrops), kv("malformed", o.malformedDrops),
    kv("unknown event", o.droppedUnknownEvent), kv("unknown notification", o.unknownNotifications),
    kv("claude agents", (o.agentsPolls ? o.agentsPollMs + " ms (avg " + o.agentsPollAvgMs + ", max " + o.agentsPollMaxMs + ")" : "—") +
      " every " + (o.agentsIntervalMs ? o.agentsIntervalMs / 1000 + " s" : "—")),
    kv("filter", o.agentsFilter || "—"),
    kv("Claude Code", (o.claudeVersion || "?") + (o.claudeVersion && o.claudeVersion !== o.verifiedOn ? " (verified on " + o.verifiedOn + ")" : "")),
    kv("notifications", o.notifySent + (o.notifyFailed ? " · failed " + o.notifyFailed : "")),
  );
  $("obs").title = o.notifyError || "";
}

function render() {
  if (!S) return;
  // The columns are rebuilt on every render; keep keyboard focus on the same card.
  const fk = document.activeElement && document.activeElement.dataset ? document.activeElement.dataset.fk : null;
  renderBody();
  if (fk) {
    const e = document.querySelector('[data-fk="' + CSS.escape(fk) + '"]');
    if (e && e !== document.activeElement) e.focus({ preventScroll: true });
  }
}

function renderBody() {
  document.title = S.name + " · Panel";
  $("pname").textContent = S.name;
  gauge("g5", S.quota ? S.quota.fiveHour : null, S.quota && S.quota.fiveHourExpired);
  gauge("g7", S.quota ? S.quota.sevenDay : null, S.quota && S.quota.sevenDayExpired);
  $("cost").textContent = S.estCostUsd == null ? "—" : S.estCostUsd.toFixed(2);
  $("hookn").textContent = S.hookEvents + " · status " + S.statusPosts + (S.dropped ? " · dropped " + S.dropped : "");

  const banners = $("banners");
  banners.replaceChildren();
  const bannerTexts = [...S.banners];
  for (const [k, src] of Object.entries(S.sources)) if (k !== "prs" && !src.pending && !src.ok) bannerTexts.push("cannot read " + k + ": " + src.error);
  for (const t of bannerTexts) banners.appendChild(el("div", "banner", null, [el("b", null, "NO SIGNAL"), document.createTextNode(t)]));
  for (const w of S.warnings || []) banners.appendChild(el("div", "warnbar", w));
  renderRestore();
  renderObs();

  // Left: lanes.
  const left = $("left");
  left.replaceChildren(el("div", "mh", "Lanes · " + S.lanes.length));
  const aSrc = sourceNote(S.sources.agents, "claude agents");
  if (aSrc) left.appendChild(aSrc);
  if (!S.lanes.length) left.appendChild(el("div", "empty", "no Claude session in this project's worktrees"));
  for (const l of S.lanes) left.appendChild(laneCard(l, false));
  if (S.quietWorktrees.length) {
    left.appendChild(el("div", "mh", "Worktrees · no session · " + S.quietWorktrees.length));
    for (const l of S.quietWorktrees) left.appendChild(laneCard(l, true));
  }

  // Centre: the selected lane.
  const all = S.lanes.concat(S.quietWorktrees);
  let lane = all.find((l) => l.id === selected) || S.lanes[0] || all[0];
  if (lane && lane.id !== selected) selected = lane.id;
  renderTerminals(lane);
  const centre = $("detail");
  centre.replaceChildren();
  if (lane) {
    const chips = el("div", "chips", null, [
      el("span", "chip sig", lane.type),
      lane.terminal ? el("span", "chip", "tmux lane " + lane.terminal) : null,
      el("span", "chip " + ({ busy: "go", waiting: "hold" }[lane.status] || ""), lane.status === "none" ? "no session" : lane.status),
      el("span", "chip", "ctx " + pct(lane.ctxPct)),
      el("span", "chip", "last hook " + (lane.lastHookAt ? age(lane.lastHookAt) + " ago" : "never")),
    ]);
    if (lane.stale) chips.appendChild(el("span", "chip stop", "no hooks while busy"));
    const sum = el("div", "summary", null, [
      el("h2", null, lane.name), chips,
      el("div", "sub", lane.branch + " · " + lane.path),
    ]);
    if (lane.sessions.length) {
      const tb = el("tbody");
      for (const s of lane.sessions) {
        tb.appendChild(el("tr", null, null, [
          el("td", null, s.name || s.id.slice(0, 8)), el("td", null, s.pid ? String(s.pid) : "—"), el("td", null, s.kind || "hooks only"),
          el("td", { busy: "go", waiting: "hold" }[s.status] || "", s.status + (s.waitingFor ? " · " + s.waitingFor : "") +
            (s.compacting ? " · compacting (" + s.compacting + ")" : "") + (s.failure ? " · failed: " + s.failure : "") +
            (s.unknownNotification ? " · unknown notification " + s.unknownNotification : "") + (s.agentState ? " · " + s.agentState : "")),
          el("td", null, pct(s.ctxPct)), el("td", null, s.model || "—"),
          el("td", null, s.estCostUsd == null ? "—" : "$" + s.estCostUsd.toFixed(2)),
        ]));
      }
      const th = el("tr", null, null, ["session", "pid", "kind", "status", "ctx", "model", "est. $"].map((h) => el("th", null, h)));
      sum.appendChild(el("div", "tablewrap", null, [el("table", null, null, [el("thead", null, null, [th]), tb])]));
    }
    sum.appendChild(el("div", "mh", "Running subagents · " + lane.subagents.length));
    if (lane.subagentsApprox) sum.appendChild(el("div", "approx sub", "approximate: the pairing was verified on Claude Code " + S.observe.verifiedOn + ", and " + (S.observe.claudeVersion || "an unknown version") + " is running"));
    if (lane.subagents.length) {
      const ul = el("div", "feed");
      for (const a of lane.subagents) ul.appendChild(el("div", "ev", null, [el("span", "t", age(a.since)), el("span", "d", (a.type || "(untyped)") + " " + a.id.slice(0, 7))]));
      sum.appendChild(ul);
    } else sum.appendChild(el("div", "empty", "none"));
    centre.appendChild(sum);
    centre.appendChild(el("div", "mh", "Events · " + lane.name));
    centre.appendChild(feedList(S.feed.filter((e) => e.lane === lane.id), false));
  } else centre.appendChild(el("div", "empty", "no worktrees"));

  // Right: needs you, cards, PRs, global feed.
  const right = $("right");
  // Needs you holds blocking states only; a finished turn is "done", shown apart.
  right.replaceChildren(el("div", "mh", "Needs you · " + S.needsYou.length));
  if (!S.needsYou.length) right.appendChild(el("div", "empty", "nothing blocked on you"));
  for (const n of S.needsYou) right.appendChild(needCard(n, "card ask " + (n.severity || "")));
  renderAlerts(right);
  renderQueues(right);
  if ((S.done || []).length) {
    right.appendChild(el("div", "mh", "Done · your move · " + S.done.length));
    for (const n of S.done) right.appendChild(needCard(n, "card done"));
  }
  for (const c of S.cards) right.appendChild(projectCard(c));

  right.appendChild(el("div", "mh", "Open PRs · " + (S.sources.prs.ok ? S.prs.length : "?")));
  const pSrc = sourceNote(S.sources.prs, "gh pr list");
  if (pSrc) right.appendChild(pSrc);
  if (S.sources.prs.ok && !S.prs.length) right.appendChild(el("div", "empty", "none open"));
  for (const p of S.prs) {
    const checks = p.checksFail ? el("span", "stop", p.checksFail + " failing") : p.checksPending ? el("span", "hold", p.checksPending + " pending")
      : p.checksPass ? el("span", "go", p.checksPass + " passing") : el("span", "dim", "no checks");
    right.appendChild(el("div", "card" + (S.sources.prs.ok ? "" : " dim"), null, [
      el("div", "mono", "#" + p.number + " " + p.headRefName),
      el("div", null, p.title),
      el("div", "sub", null, [document.createTextNode((p.isDraft ? "draft · " : "") + p.author + " · "), checks]),
    ]));
  }

  right.appendChild(el("div", "mh", "Feed"));
  right.appendChild(feedList(S.feed.slice(0, 60), true));
}

$("refresh").addEventListener("click", () => { fetch("/api/refresh", { method: "POST" }).catch(() => {}); });

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
  const themes = el("div", null, null, [el("div", "mh", "Theme")]);
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
  menu.replaceChildren(themes, el("div", "mh", "Mode"), modes);
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
setInterval(render, 1000); // ages move between server pushes
connect();
