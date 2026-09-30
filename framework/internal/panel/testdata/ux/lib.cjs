// UX harness (UX-1): what matrix.cjs and flows.cjs share. Synthetic data only.
//
// - run(): the run up.sh described (its JSON line, a file holding it, or $UX_RUN);
// - Findings: one entry per failed assertion or flow step, written to findings.json;
// - openPage(): a page on the panel, with its console errors collected;
// - layoutAudit(): the per-shot layout assertions (see matrix.cjs).
const fs = require("fs");
const path = require("path");

function loadPlaywright() {
  const tries = [process.env.PLAYWRIGHT, "playwright"].filter(Boolean);
  for (const t of tries) { try { return require(t); } catch (e) { /* next */ } }
  throw new Error("Playwright not found: set PLAYWRIGHT to the playwright package directory (or NODE_PATH)");
}

function run(arg) {
  const src = arg || process.env.UX_RUN;
  if (!src) throw new Error("pass up.sh's JSON (or a file holding it) as the first argument, or set UX_RUN");
  const text = src.trim().startsWith("{") ? src : fs.readFileSync(src, "utf8");
  return JSON.parse(text.trim().split("\n").filter((l) => l.startsWith("{")).pop());
}

// Findings: kept in memory, written after every add, so a crash still leaves a file.
class Findings {
  constructor(outDir, shard) {
    this.outDir = outDir;
    this.shard = shard;
    this.items = [];
    this.passes = [];
    fs.mkdirSync(outDir, { recursive: true });
    this.file = path.join(outDir, "findings.json");
    this.write();
  }
  add(f) {
    this.items.push(Object.assign({ shard: this.shard, at: new Date().toISOString() }, f));
    this.write();
  }
  pass(name, extra) { this.passes.push(Object.assign({ name }, extra || {})); this.write(); }
  write() {
    fs.writeFileSync(this.file, JSON.stringify({ shard: this.shard, findings: this.items, passes: this.passes }, null, 1));
  }
}

async function openPage(browser, R, opts = {}) {
  const ctx = await browser.newContext(Object.assign({ viewport: { width: 1440, height: 900 } }, opts.context || {}));
  if (opts.storage) await ctx.addInitScript((kv) => { for (const [k, v] of Object.entries(kv)) localStorage.setItem(k, v); }, opts.storage);
  const page = await ctx.newPage();
  page.setDefaultTimeout(10000); // a missing control fails a view in seconds, not 30
  const errors = [];
  page.on("pageerror", (e) => errors.push("pageerror: " + e.message));
  page.on("console", (m) => { if (m.type() === "error") errors.push("console: " + m.text()); });
  const q = opts.project ? "&p=" + encodeURIComponent(opts.project) : "";
  await page.goto(R.base + "/?t=" + R.token + q);
  await page.waitForSelector("#tabs", { timeout: 15000 });
  await page.waitForFunction(() => document.getElementById("livetext") && /Live/.test(document.getElementById("livetext").textContent), null, { timeout: 15000 }).catch(() => {});
  return { ctx, page, errors };
}

// The appearance, set the way the page itself keeps it (theme.js reads these keys).
function appearanceStorage({ theme, mode, type, scale }) {
  const kv = {};
  if (theme) kv["clauductor-panel-theme"] = theme;
  if (mode) kv["clauductor-panel-mode"] = mode;
  if (type) kv["clauductor-panel-type"] = type;
  if (scale) kv["clauductor-panel-scale"] = String(scale);
  return kv;
}

// layoutAudit runs in the page and returns the failed assertions, each with a CSS
// selector-ish path to what failed. The rules:
// - hscroll: the page scrolls sideways (documentElement.scrollWidth > innerWidth);
// - clip: an element whose own text overflows its box (scrollWidth > clientWidth + 1)
//   while it hides the overflow (overflow hidden/clip without an ellipsis, which is
//   a deliberate truncation), or spills past a parent that hides it; only visible
//   elements with their own text;
// - overlap: two visible interactive elements (button, a, input, select, textarea,
//   [role=tab], [role=menuitem], [role=option]) whose boxes intersect by more than
//   2 px each way, neither containing the other, and the one on top really covers
//   the other (elementFromPoint at the intersection's centre);
// - offscreen: an interactive element partly outside the viewport sideways;
// - xterm: the visible terminal does not fill its host within 2 px, or its cols/rows
//   differ from what a fit would give now.
async function layoutAudit(page) {
  return page.evaluate(() => {
    const out = [];
    const vis = (e) => {
      const r = e.getBoundingClientRect();
      if (r.width < 2 || r.height < 2) return false; // screen-reader-only text is 1 px
      for (let x = e; x && x !== document.body; x = x.parentElement) {
        const cs = getComputedStyle(x);
        if (cs.display === "none" || cs.visibility === "hidden" || +cs.opacity === 0) return false;
        if (x.hidden) return false;
      }
      return true;
    };
    const sel = (e) => {
      const parts = [];
      for (let x = e; x && x !== document.body && parts.length < 5; x = x.parentElement) {
        let s = x.tagName.toLowerCase();
        if (x.id) { s += "#" + x.id; parts.unshift(s); break; }
        if (x.dataset && x.dataset.k) s += '[data-k="' + x.dataset.k + '"]';
        else if (x.classList.length) s += "." + Array.from(x.classList).slice(0, 2).join(".");
        parts.unshift(s);
      }
      return parts.join(" > ");
    };
    const text = (e) => (e.innerText || e.value || "").trim().replace(/\s+/g, " ").slice(0, 60);
    const de = document.documentElement;
    if (de.scrollWidth > innerWidth + 1) {
      // Name the widest culprit.
      let worst = null, w = innerWidth;
      for (const e of document.querySelectorAll("body *")) {
        const r = e.getBoundingClientRect();
        if (r.right > w + 1 && vis(e)) { w = r.right; worst = e; }
      }
      out.push({ rule: "hscroll", selector: worst ? sel(worst) : "html", detail: "page scrollWidth " + de.scrollWidth + " > " + innerWidth });
    }
    // Clipped text.
    const inTerm = (e) => !!e.closest(".xterm");
    for (const e of document.querySelectorAll("body *")) {
      if (inTerm(e) || !vis(e)) continue;
      const own = Array.from(e.childNodes).some((n) => n.nodeType === 3 && n.textContent.trim());
      if (!own) continue;
      const cs = getComputedStyle(e);
      if (cs.textOverflow === "ellipsis") continue;
      const hides = /hidden|clip/.test(cs.overflowX) || /hidden|clip/.test(cs.overflow);
      if (hides && e.scrollWidth > e.clientWidth + 1 && e.clientWidth > 0) {
        out.push({ rule: "clip", selector: sel(e), detail: "text \"" + text(e) + "\" needs " + e.scrollWidth + " px, has " + e.clientWidth });
        continue;
      }
      // Spills out of an ancestor that hides overflow.
      const r = e.getBoundingClientRect();
      for (let p = e.parentElement, n = 0; p && p !== document.body && n < 6; p = p.parentElement, n++) {
        const pcs = getComputedStyle(p);
        if (!/hidden|clip/.test(pcs.overflowX)) continue;
        if (pcs.textOverflow === "ellipsis") break; // a deliberate truncation, drawn as one
        const pr = p.getBoundingClientRect();
        if (r.right > pr.right + 2 && r.left < pr.right) out.push({ rule: "clip", selector: sel(e), detail: "text \"" + text(e) + "\" runs " + Math.round(r.right - pr.right) + " px past " + sel(p) });
        break;
      }
    }
    // Interactive overlap.
    const I = Array.from(document.querySelectorAll('button, a[href], input:not([type=hidden]), select, textarea, [role="tab"], [role="menuitem"], [role="option"]'))
      .filter((e) => !inTerm(e) && vis(e) && !e.disabled);
    // The part of each box that shows: cut by every ancestor that clips (a scrolled
    // rail hides what runs past its foot). A floating layer (a menu, a dialog, the
    // drawer) sits over the page by design, so a control in one never "overlaps" a
    // control outside it.
    const clipRect = (e) => {
      const r = e.getBoundingClientRect();
      let l = r.left, t = r.top, rt = r.right, b = r.bottom;
      for (let p = e.parentElement; p && p !== document.documentElement; p = p.parentElement) {
        const cs = getComputedStyle(p);
        if (cs.overflowX === "visible" && cs.overflowY === "visible") continue;
        const pr = p.getBoundingClientRect();
        l = Math.max(l, pr.left); t = Math.max(t, pr.top); rt = Math.min(rt, pr.right); b = Math.min(b, pr.bottom);
      }
      return { left: l, top: t, right: rt, bottom: b };
    };
    const layer = (e) => e.closest('.menu, [role="menu"], [role="listbox"], .modal, [role="dialog"], .drawer, .colmenu');
    const modalOpen = document.querySelector(".modal:not([hidden])");
    const boxes = I.map(clipRect);
    for (let i = 0; i < I.length; i++) {
      const a = boxes[i];
      if (a.right > a.left && (a.right > innerWidth + 1 || a.left < -1)) out.push({ rule: "offscreen", selector: sel(I[i]), detail: "\"" + text(I[i]) + "\" spans x " + Math.round(a.left) + "–" + Math.round(a.right) + " in a " + innerWidth + " px window" });
      for (let j = i + 1; j < I.length; j++) {
        const b = boxes[j];
        if (I[i].contains(I[j]) || I[j].contains(I[i])) continue;
        if (layer(I[i]) !== layer(I[j])) continue;
        if (modalOpen && !modalOpen.contains(I[i])) continue; // inert behind the dialog
        const ix = Math.min(a.right, b.right) - Math.max(a.left, b.left), iy = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
        if (ix <= 2 || iy <= 2) continue;
        const cx = Math.max(a.left, b.left) + ix / 2, cy = Math.max(a.top, b.top) + iy / 2;
        if (cx < 0 || cy < 0 || cx >= innerWidth || cy >= innerHeight) continue;
        const top = document.elementFromPoint(cx, cy);
        if (!top) continue;
        const onA = I[i].contains(top), onB = I[j].contains(top);
        if (!onA && !onB) continue; // something else (a menu, a dialog) covers both
        out.push({ rule: "overlap", selector: sel(I[i]) + " ⟂ " + sel(I[j]), detail: "\"" + text(I[i]) + "\" and \"" + text(I[j]) + "\" overlap " + Math.round(ix) + "×" + Math.round(iy) + " px" });
      }
    }
    // Covered: something that is not a control (a label, "More below") sits over the
    // middle of a control's visible part, so a click there misses it.
    for (let i = 0; i < I.length; i++) {
      const c = boxes[i];
      if (c.right - c.left < 2 || c.bottom - c.top < 2) continue;
      if (modalOpen && !modalOpen.contains(I[i])) continue;
      const cx = (c.left + c.right) / 2, cy = (c.top + c.bottom) / 2;
      if (cx < 0 || cy < 0 || cx >= innerWidth || cy >= innerHeight) continue;
      const top = document.elementFromPoint(cx, cy);
      if (!top || I[i].contains(top) || top.contains(I[i])) continue;
      if (layer(top) && layer(top) !== layer(I[i])) continue; // an open menu or the drawer, over the page by design
      if (I.some((o) => o !== I[i] && o.contains(top))) continue; // another control: the overlap rule's
      out.push({ rule: "covered", selector: sel(I[i]), detail: "\"" + text(I[i]) + "\" is covered at its centre by " + sel(top) + (text(top) ? " (\"" + text(top) + "\")" : "") });
    }
    // A selected tab is in view: a tab strip that scrolls must keep its selected tab
    // shown whole.
    for (const e of document.querySelectorAll('[role="tab"][aria-selected="true"]')) {
      if (!vis(e)) continue;
      const r = e.getBoundingClientRect(), c = clipRect(e);
      if (c.right - c.left < r.width - 2) out.push({ rule: "selected-hidden", selector: sel(e), detail: "the selected tab \"" + text(e) + "\" shows " + Math.max(0, Math.round(c.right - c.left)) + " of its " + Math.round(r.width) + " px" });
    }
    // The terminal fills its host, at the size a fit gives.
    const t = typeof terms !== "undefined" && typeof selTerm !== "undefined" ? terms[selTerm] : null;
    if (t && t.term && t.term.element && vis(t.term.element)) {
      const host = t.host.getBoundingClientRect();
      const screen = t.term.element.querySelector(".xterm-screen").getBoundingClientRect();
      const cell = t.term._core._renderService.dimensions.css.cell;
      const slackX = host.width - screen.width, slackY = host.height - screen.height;
      // Within one cell (the fit rounds down) plus the 2 px the rule allows.
      if (slackX < -2 || slackX > cell.width + 2 + 16 || slackY < -2 || slackY > cell.height + 2)
        out.push({ rule: "xterm-fill", selector: sel(t.host), detail: "screen " + Math.round(screen.width) + "×" + Math.round(screen.height) + " in host " + Math.round(host.width) + "×" + Math.round(host.height) + " (cell " + cell.width.toFixed(1) + "×" + cell.height.toFixed(1) + ")" });
      try {
        const d = t.fit.proposeDimensions();
        if (d && (d.cols !== t.term.cols || d.rows !== t.term.rows))
          out.push({ rule: "xterm-fit", selector: sel(t.host), detail: "term " + t.term.cols + "×" + t.term.rows + ", fit proposes " + d.cols + "×" + d.rows });
      } catch (e) { /* no fit addon on this build */ }
    }
    return out;
  });
}

// stableAudit: layoutAudit, but a terminal size finding must hold for 1.5 s more.
// The page refits a terminal a moment after its box changes (a bar comes or goes, a
// live update adds a row), so one reading can catch it mid-change.
async function stableAudit(page) {
  const a = await layoutAudit(page);
  if (!a.some((f) => f.rule.startsWith("xterm"))) return a;
  await page.waitForTimeout(1500);
  const b = await layoutAudit(page);
  const again = new Set(b.filter((f) => f.rule.startsWith("xterm")).map((f) => f.rule));
  return a.filter((f) => !f.rule.startsWith("xterm") || again.has(f.rule));
}

// focusRingAudit: Tab through the first n stops; each focused control must draw a
// ring (an outline, or a box-shadow) that differs from its unfocused look.
async function focusRingAudit(page, n = 14) {
  await page.evaluate(() => { document.activeElement && document.activeElement.blur(); window.scrollTo(0, 0); });
  await page.mouse.click(2, 2).catch(() => {});
  const out = [];
  for (let i = 0; i < n; i++) {
    await page.keyboard.press("Tab");
    const r = await page.evaluate(() => {
      const e = document.activeElement;
      if (!e || e === document.body) return null;
      if (e.closest(".xterm")) return { skip: true };
      const cs = getComputedStyle(e);
      const ring = (cs.outlineStyle !== "none" && parseFloat(cs.outlineWidth) > 0) || (cs.boxShadow && cs.boxShadow !== "none");
      const r = e.getBoundingClientRect();
      let s = e.tagName.toLowerCase() + (e.id ? "#" + e.id : "") + (e.dataset.k ? '[data-k="' + e.dataset.k + '"]' : "");
      return { ring, sel: s, text: (e.innerText || e.getAttribute("aria-label") || "").trim().slice(0, 40), fv: e.matches(":focus-visible"), onscreen: r.bottom > 0 && r.top < innerHeight && r.width > 0 };
    });
    if (!r || r.skip) continue;
    if (!r.ring) out.push({ rule: "focus-ring", selector: r.sel, detail: "Tab stop " + (i + 1) + " \"" + r.text + "\" draws no outline or box-shadow when focused" + (r.fv ? "" : " (not :focus-visible)") });
    else if (!r.onscreen) out.push({ rule: "focus-offscreen", selector: r.sel, detail: "Tab stop " + (i + 1) + " \"" + r.text + "\" is focused off screen" });
  }
  return out;
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

module.exports = { loadPlaywright, run, Findings, openPage, appearanceStorage, layoutAudit, stableAudit, focusRingAudit, sleep };
