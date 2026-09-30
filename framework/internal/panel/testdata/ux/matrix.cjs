// UX harness (UX-1): screenshots and layout assertions over a matrix of viewports,
// appearances and views, against a panel up.sh started. Read-only: it opens menus
// and confirmations and cancels them; it never confirms an action.
//
//   node matrix.cjs <up.json | its JSON line> [--out dir] [--areas views,appearance,sizes]
//        [--viewports 1280x800,1440x900] [--full]
//
// Areas:
//   views       every viewport × every view (overview, each lane, project menu, Help,
//               New lane, Close confirmation, Remove confirmation, ⋯ menu, Activity,
//               Appearance menu, Metrics if the build has it, each project)
//   appearance  every theme × mode (light, dark) the Appearance menu offers, with its
//               own type, on overview and New lane; every type system on overview and
//               a streaming lane; --full adds every theme × type × mode
//   sizes       every text size step the page allows (85–175 %) at 1280, 1920 and the
//               narrow 900 px
// Each shot runs lib.layoutAudit (horizontal scroll, clipped text, overlapping
// controls, controls off screen, the terminal's fill and fit) and collects console
// errors; each viewport and each theme × mode also runs the focus-ring audit.
// Output: <out>/shots/<area>/<name>.png and <out>/findings.json.
const path = require("path");
const fs = require("fs");
const L = require("./lib.cjs");
const pw = L.loadPlaywright();

const args = process.argv.slice(2);
const opt = (name, d) => { const i = args.indexOf("--" + name); return i >= 0 ? args[i + 1] : d; };
const R = L.run(args[0] && !args[0].startsWith("--") ? args[0] : null);
const out = path.resolve(opt("out", path.join(require("os").tmpdir(), "clauductor-ux-out", "matrix-" + R.runid)));
const areas = opt("areas", "views,appearance,sizes").split(",");
const full = args.includes("--full");
const VIEWPORTS = opt("viewports", "1280x800,1440x900,1920x1080,2000x900,2560x1440,900x800").split(",").map((s) => { const [w, h] = s.split("x").map(Number); return { width: w, height: h }; });
const F = new L.Findings(out, "matrix:" + areas.join("+"));
const alphaLanes = R.lanes.alpha;
const vpName = (v) => v.width + "x" + v.height;
let shots = 0;

async function settle(page, ms = 500) {
  await page.waitForTimeout(ms);
  await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
}

// shot: screenshot, then the audits; every failure is a finding with this context.
async function shot(page, errors, area, name, ctx) {
  const dir = path.join(out, "shots", area);
  fs.mkdirSync(dir, { recursive: true });
  const file = path.join(dir, name.replace(/[^a-z0-9._-]+/gi, "_") + ".png");
  await page.screenshot({ path: file });
  shots++;
  const base = Object.assign({ area, name, screenshot: path.relative(out, file) }, ctx);
  for (const f of await L.stableAudit(page)) F.add(Object.assign({}, base, f));
  for (const e of errors.splice(0)) F.add(Object.assign({}, base, { rule: "console", selector: "", detail: e }));
  F.pass(area + "/" + name);
}

async function appearance(page) {
  return page.evaluate(() => {
    const s = window.PanelTheme.get();
    return { theme: s.theme, mode: s.resolved, type: s.type, scale: window.PanelScale.get() };
  });
}

// The rail starts hidden on a narrow window (below 900 px, and when the layout folds
// it at a large size): a view that needs it shows it, and hides it again after.
async function withRail(page) {
  const hidden = await page.$eval("#railbtn", (b) => b.getAttribute("aria-expanded") === "false").catch(() => false);
  if (hidden) { await page.click("#railbtn"); await page.waitForTimeout(400); }
  return async () => { if (hidden) await page.click("#railbtn").catch(() => {}); };
}

// Each view opens something, shoots it, and puts the page back.
const VIEWS = {
  async overview(page) { return true; },
  async "project-menu"(page) {
    await page.click("#projbtn");
    await page.waitForSelector("#projmenu:not([hidden])", { timeout: 3000 });
    return async () => { await page.keyboard.press("Escape"); };
  },
  async help(page) {
    await page.click("#helpbtn");
    await page.waitForSelector("#helpdlg:not([hidden])", { timeout: 3000 });
    return async () => { await page.keyboard.press("Escape"); };
  },
  async "new-lane"(page) {
    await page.click("#addlane");
    await page.waitForSelector("#startdlg:not([hidden])", { timeout: 3000 });
    await page.waitForTimeout(300);
    return async () => { await page.keyboard.press("Escape"); await page.waitForSelector("#startdlg", { state: "hidden", timeout: 2000 }).catch(() => page.click("#st-cancel")); };
  },
  async "appearance-menu"(page) {
    await page.click("#themebtn");
    await page.waitForSelector("#thememenu:not([hidden])", { timeout: 3000 });
    return async () => { await page.keyboard.press("Escape"); };
  },
  async "row-menu"(page) {
    const back = await withRail(page);
    const k = '[data-k="ra:t:working"]';
    await page.locator(k).scrollIntoViewIfNeeded({ timeout: 5000 });
    await page.click(k);
    await page.waitForSelector("#rowmenu:not([hidden])", { timeout: 3000 });
    return async () => { await page.keyboard.press("Escape"); await back(); };
  },
  async "close-confirm"(page) {
    await page.click('[data-k="tab:t:merged"]');
    await settle(page, 400);
    await page.click('#termbar [data-k="b:Close lane"]');
    await page.waitForSelector('#termbar [data-k="closeask"] .closelist, #termbar .stop.sub', { timeout: 15000 });
    return async () => { await page.click('#termbar [data-k="b:close-cancel"]').catch(() => {}); };
  },
  async "remove-confirm"(page) {
    const k = '[data-k="wtrm:' + R.leftovers[1] + '"]';
    if (!(await page.$(k))) return null;
    const back = await withRail(page);
    await page.locator(k).scrollIntoViewIfNeeded({ timeout: 5000 });
    await page.click(k);
    await page.waitForSelector(".tree .wtask .closelist, .tree .wtask .stop", { timeout: 15000 });
    await page.locator(".tree .wtask").scrollIntoViewIfNeeded({ timeout: 5000 });
    return async () => { await page.click('[data-k="wtno:' + R.leftovers[1] + '"]').catch(() => {}); await back(); };
  },
  async activity(page) {
    await page.click("#activitybtn");
    await page.waitForSelector("#drawer:not([hidden])", { timeout: 3000 });
    await page.waitForTimeout(300);
    return async () => { await page.click("#drawerclose").catch(() => page.keyboard.press("Escape")); };
  },
  async metrics(page) {
    // Feature-detect: a control named Metrics (a later build's view); skip otherwise.
    const m = page.locator('button:has-text("Metrics"), [role="tab"]:has-text("Metrics"), a:has-text("Metrics")').first();
    if (!(await m.count())) return null;
    await m.click();
    await page.waitForTimeout(500);
    return async () => { await page.keyboard.press("Escape"); };
  },
};
// The side panel's tabs, on the working lane (its PR, checks, agents and figures).
for (const tab of ["figures", "git", "gate", "alerts", "activity"]) VIEWS["side-" + tab] = async (page) => {
  await page.click('[data-k="tab:t:working"]');
  const k = '[data-k="stab:' + tab + '"]';
  if (!(await page.locator(k).isVisible().catch(() => false))) return null; // the side panel is folded at this size
  await page.click(k);
  await page.waitForTimeout(300);
  return async () => { await page.click('[data-k="stab:agents"]').catch(() => {}); };
};
for (const id of alphaLanes) VIEWS["lane-" + id] = async (page) => {
  const k = '[data-k="tab:t:' + id + '"]';
  if (!(await page.$(k))) return null;
  await page.click(k);
  await page.waitForTimeout(700);
  return true;
};
VIEWS["project-beta"] = async (page) => {
  await page.click("#projbtn");
  await page.click("#proj-beta");
  await page.waitForFunction(() => document.getElementById("pname").textContent === "Beta", null, { timeout: 10000 });
  await page.waitForSelector(".xterm-rows", { timeout: 10000 }).catch(() => {});
  await page.waitForTimeout(800);
  return async () => {
    await page.click("#projbtn");
    await page.click("#proj-alpha");
    await page.waitForFunction(() => document.getElementById("pname").textContent === "Alpha", null, { timeout: 10000 });
    await page.waitForSelector('[data-k="tab:t:working"]', { timeout: 10000 });
  };
};
VIEWS["project-gamma"] = async (page) => {
  // Gamma cannot load: its row in the menu says why, and picking it switches nothing.
  await page.click("#projbtn");
  await page.waitForSelector("#proj-gamma", { timeout: 3000 });
  await page.hover("#proj-gamma");
  return async () => { await page.keyboard.press("Escape"); };
};

// runView runs the view a shot name ends with ("hmi-dark-new-lane" is new-lane).
async function runView(page, errors, area, name, ctx) {
  const view = VIEWS[name] ? name : Object.keys(VIEWS).sort((a, b) => b.length - a.length).find((k) => name.endsWith("-" + k));
  if (!view) throw new Error("no view for " + name);
  // Every shot starts at the top of the page (the focus audit may have scrolled it).
  await page.evaluate(() => window.scrollTo(0, 0));
  let undo;
  try {
    undo = await VIEWS[view](page);
  } catch (e) {
    F.add(Object.assign({ area, name, rule: "view-failed", selector: "", detail: String(e.message || e).split("\n")[0] }, ctx));
    await page.keyboard.press("Escape").catch(() => {});
    return;
  }
  if (undo === null) { F.pass(area + "/" + name, { skipped: "not in this build or state" }); return; }
  await settle(page);
  await shot(page, errors, area, name, ctx);
  if (typeof undo === "function") await undo().catch(() => {});
  await settle(page, 200);
}

async function views(browser) {
  for (const vp of VIEWPORTS) {
    const area = "viewport-" + vpName(vp);
    const { ctx, page, errors } = await L.openPage(browser, R, { context: { viewport: vp } });
    try {
      await page.waitForSelector('[data-k="tab:t:working"]', { timeout: 15000 });
      await settle(page, 1200);
      const c = Object.assign({ viewport: vpName(vp) }, await appearance(page));
      for (const f of await L.focusRingAudit(page)) F.add(Object.assign({ area, name: "focus", screenshot: "" }, c, f));
      await page.click('[data-k="tab:t:' + alphaLanes[0] + '"]').catch(() => {});
      for (const name of Object.keys(VIEWS)) await runView(page, errors, area, name, c);
    } finally { await ctx.close(); }
  }
}

async function appearanceArea(browser) {
  const vp = { width: 1440, height: 900 };
  const { ctx, page, errors } = await L.openPage(browser, R, { context: { viewport: vp } });
  try {
    await page.waitForSelector('[data-k="tab:t:working"]', { timeout: 15000 });
    // What the Appearance menu offers, checked against theme.js's lists.
    const offered = await page.evaluate(() => ({ themes: window.PanelTheme.themes.map((t) => [t.id, t.name]), types: window.PanelTheme.types.map((t) => [t.id, t.name]) }));
    await page.click("#themebtn");
    const menuText = await page.$eval("#thememenu", (m) => m.innerText);
    await page.keyboard.press("Escape");
    for (const [id, name] of offered.themes.concat(offered.types)) if (!menuText.includes(name)) F.add({ area: "appearance", name: "menu", rule: "menu-missing", selector: "#thememenu", detail: name + " (" + id + ") is not in the Appearance menu", viewport: vpName(vp) });
    const set = (theme, mode, type) => page.evaluate(([t, m, y]) => { window.PanelTheme.setTheme(t); window.PanelTheme.setMode(m); window.PanelTheme.setType(y); }, [theme, mode, type]);
    const combos = [];
    for (const [th] of offered.themes) for (const mode of ["light", "dark"]) combos.push([th, mode, null, ["overview", "new-lane", "row-menu"]]);
    for (const [ty] of offered.types) combos.push(["hmi", "light", ty, ["overview", "lane-stream"]]);
    if (full) for (const [th] of offered.themes) for (const [ty] of offered.types) for (const mode of ["light", "dark"]) combos.push([th, mode, ty, ["overview"]]);
    const focusDone = new Set();
    for (const [th, mode, ty, names] of combos) {
      await set(th, mode, ty);
      await settle(page, 900); // a type change loads the face, then refits
      const c = Object.assign({ viewport: vpName(vp) }, await appearance(page));
      const tag = th + "-" + mode + (ty ? "-" + ty : "");
      if (!focusDone.has(th + mode)) {
        focusDone.add(th + mode);
        for (const f of await L.focusRingAudit(page, 10)) F.add(Object.assign({ area: "appearance", name: tag + "-focus", screenshot: "" }, c, f));
      }
      for (const n of names) await runView(page, errors, "appearance", tag + "-" + n, c).catch(() => {});
    }
    await set("hmi", "system", null);
  } finally { await ctx.close(); }
}
async function sizes(browser) {
  for (const vp of [{ width: 1280, height: 800 }, { width: 1920, height: 1080 }, { width: 900, height: 800 }]) {
    const { ctx, page, errors } = await L.openPage(browser, R, { context: { viewport: vp } });
    try {
      await page.waitForSelector('[data-k="tab:t:working"]', { timeout: 15000 });
      const steps = await page.evaluate(() => window.PanelScale.steps);
      const pick = steps.filter((v) => v % 15 === 10 || v === steps[0] || v === steps[steps.length - 1] || v === 100);
      for (const pct of pick) {
        await page.evaluate((v) => window.PanelScale.set(v), pct);
        await settle(page, 700);
        const c = Object.assign({ viewport: vpName(vp) }, await appearance(page));
        await runView(page, errors, "sizes", vpName(vp) + "-" + pct + "-overview", c);
        if (pct >= 150) await runView(page, errors, "sizes", vpName(vp) + "-" + pct + "-new-lane", c);
      }
      await page.evaluate(() => window.PanelScale.reset());
    } finally { await ctx.close(); }
  }
}

(async () => {
  const t0 = Date.now();
  const browser = await pw.chromium.launch();
  try {
    if (areas.includes("views")) await views(browser);
    if (areas.includes("appearance")) await appearanceArea(browser);
    if (areas.includes("sizes")) await sizes(browser);
  } catch (e) {
    F.add({ area: "matrix", name: "crash", rule: "harness", selector: "", detail: String(e.stack || e) });
  } finally {
    await browser.close();
  }
  console.log("matrix: " + shots + " shots, " + F.items.length + " findings in " + Math.round((Date.now() - t0) / 1000) + " s → " + F.file);
})();
