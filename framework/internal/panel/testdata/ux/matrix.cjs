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
//               Appearance menu, Metrics if the build has it, each project; PANEL-22: a
//               project's ⋯ actions, Add a project… empty, refused, with the init
//               preview and with the trust report, Remove from panel… refused and
//               allowed, Trust config…)
//   appearance  every theme × mode (light, dark) the Appearance menu offers, with its
//               own type, on overview and New lane; every type system on overview and
//               a streaming lane; --full adds every theme × type × mode
//   sizes       every text size step the page allows (85–175 %) at 1280, 1920 and the
//               narrow 900 px
//   features    (UX-2, PANEL-19/20) at 1280, 1440, 1920 and 900 px, once the signals are
//               up: Needs you with the approval, budget and stale alerts, the economy
//               badge, the Flow card, the Budget bar (over and amber), Port and Remote in
//               the header, the Checks tab (not ready, ready, the root), the stale lane's
//               Alerts tab, the ⋯ menu with Remote control and its confirmations (idle,
//               busy), Metrics opened from the Flow card
//   metrics     (UX-2) the Metrics view: every tab × range × scope, at 1440 and 900 px
//   (features and metrics are not in the default --areas: run-parallel.sh gives them a
//   shard of their own, layout-metrics)
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
    // Feature-detect: the Metrics button (PANEL-19); skip on a build without it.
    if (!(await page.$("#metricsbtn"))) return null;
    await page.click("#metricsbtn");
    await page.waitForSelector("#mview:not([hidden])", { timeout: 3000 });
    await metricsLoaded(page);
    return async () => { await page.click("#mviewclose").catch(() => page.keyboard.press("Escape")); };
  },
};
// The side panel's tabs, on the working lane (its PR, checks, agents and figures).
for (const tab of ["figures", "git", "checks", "gate", "alerts", "activity"]) VIEWS["side-" + tab] = async (page) => {
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

// PANEL-22: the project menu's actions and its dialogs. Read-only like every view: each
// opens, is shot, and is cancelled; nothing is added, trusted or removed.
async function openMenuActs(page, id) {
  await page.click("#projbtn");
  await page.waitForSelector("#projmenu:not([hidden])", { timeout: 3000 });
  await page.click('[data-key="more:' + id + '"]');
  await page.waitForSelector("#projacts:not([hidden])", { timeout: 3000 });
}
async function openAdd(page, p) {
  await page.click("#projbtn");
  await page.click("#addproj");
  await page.waitForSelector("#adddlg:not([hidden])", { timeout: 3000 });
  if (p) { await page.fill("#ad-path", p); await page.press("#ad-path", "Enter"); }
  return async () => { await page.click("#ad-cancel").catch(() => page.keyboard.press("Escape")); };
}
VIEWS["project-actions"] = async (page) => {
  await openMenuActs(page, "zeta");
  return async () => { await page.keyboard.press("Escape"); await page.keyboard.press("Escape"); };
};
VIEWS["project-add"] = async (page) => openAdd(page, "");
VIEWS["project-add-init"] = async (page) => {
  if (!R.candidates) return null;
  const undo = await openAdd(page, R.candidates.delta);
  await page.waitForSelector("#ad-body pre.cfg", { timeout: 8000 });
  return undo;
};
VIEWS["project-add-trust"] = async (page) => {
  if (!R.candidates) return null;
  const undo = await openAdd(page, R.candidates.epsilon);
  await page.waitForSelector("#ad-trust:not([hidden])", { timeout: 8000 });
  return undo;
};
VIEWS["project-add-refused"] = async (page) => {
  const undo = await openAdd(page, R.leftovers[0]); // a linked worktree of Alpha
  await page.waitForFunction(() => /Not addable/.test(document.getElementById("ad-status").textContent), null, { timeout: 8000 });
  return undo;
};
VIEWS["project-remove-refused"] = async (page) => {
  await openMenuActs(page, "alpha");
  await page.click('#projacts [data-act="remove"]');
  await page.waitForSelector("#pd-body ul.lanes", { timeout: 8000 });
  return async () => { await page.click("#pd-cancel").catch(() => page.keyboard.press("Escape")); };
};
VIEWS["project-remove"] = async (page) => {
  if (!(R.projects && R.projects.zeta)) return null;
  await openMenuActs(page, "zeta");
  await page.click('#projacts [data-act="remove"]');
  await page.waitForSelector("#pd-go:not([hidden])", { timeout: 8000 });
  return async () => { await page.click("#pd-cancel").catch(() => page.keyboard.press("Escape")); };
};
VIEWS["project-trust"] = async (page) => {
  if (!(R.projects && R.projects.zeta)) return null;
  await openMenuActs(page, "zeta");
  await page.click('#projacts [data-act="trust"]');
  await page.waitForSelector("#pd-go:not([hidden])", { timeout: 8000 });
  return async () => { await page.click("#pd-cancel").catch(() => page.keyboard.press("Escape")); };
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

// ---- PANEL-19/20 (UX-2) ------------------------------------------------------------
// The Metrics view has read its figures (or its error) once "Reading the metrics…" goes.
async function metricsLoaded(page) {
  await page.waitForFunction(() => { const b = document.getElementById("mvbody"); return b && b.textContent && !/Reading the metrics/.test(b.textContent); }, null, { timeout: 10000 }).catch(() => {});
}
const featureFinding = (area, name, detail, ctx) => F.add(Object.assign({ area, name, rule: "feature-missing", selector: "", detail, screenshot: "" }, ctx || {}));

// metrics: every tab × range × scope of the Metrics view, at a wide and the narrow window.
async function metricsArea(browser) {
  const pick = VIEWPORTS.filter((v) => ["1440x900", "900x800"].includes(vpName(v)));
  for (const vp of pick.length ? pick : VIEWPORTS.slice(0, 1)) {
    const area = "metrics-" + vpName(vp);
    const { ctx, page, errors } = await L.openPage(browser, R, { context: { viewport: vp } });
    try {
      await page.waitForSelector('[data-k="tab:t:working"]', { timeout: 15000 });
      if (!(await page.$("#metricsbtn"))) { F.pass(area + "/all", { skipped: "no Metrics view in this build" }); continue; }
      const c = Object.assign({ viewport: vpName(vp) }, await appearance(page));
      await page.click("#metricsbtn");
      await page.waitForSelector("#mview:not([hidden])", { timeout: 3000 });
      for (const scope of ["project", "all"]) {
        await page.click('[data-k="mscope:' + scope + '"]');
        for (const range of ["7d", "30d", "90d"]) {
          await page.click('[data-k="mrange:' + range + '"]');
          for (const tab of ["flow", "cost", "quality", "outcomes"]) {
            try {
              await page.click("#mtab-" + tab);
              await metricsLoaded(page);
              await settle(page, 250);
              await shot(page, errors, area, scope + "-" + range + "-" + tab, c);
            } catch (e) {
              F.add(Object.assign({ area, name: scope + "-" + range + "-" + tab, rule: "view-failed", selector: "", detail: String(e.message || e).split("\n")[0] }, c));
            }
          }
        }
      }
      // Put the view back the way a viewer finds it first.
      await page.click('[data-k="mscope:project"]').catch(() => {});
      await page.click('[data-k="mrange:30d"]').catch(() => {});
      await page.click("#mtab-flow").catch(() => {});
      await page.click("#mviewclose").catch(() => {});
    } finally { await ctx.close(); }
  }
}

// The signals the feature views need take a minute to appear: the spend ledger (budget)
// is written every minute, the stale lane needs the registry re-read (30 s) and a git read
// while a page is in view (30 s), readiness needs that git read too. Refresh kicks them.
async function waitSignals(page, ms = 110000) {
  const want = { approval: '[data-k="alert:approval_wait:add-group-card"]', budget: '[data-k^="alert:budget:"]', stale: '[data-k="alert:stale:quiet"]', economy: '[data-k="q:eco"]' };
  const t0 = Date.now(), got = {};
  let lastRefresh = 0;
  while (Date.now() - t0 < ms) {
    for (const [k, sel] of Object.entries(want)) if (!got[k] && (await page.$(sel))) got[k] = Math.round((Date.now() - t0) / 1000);
    if (Object.keys(got).length === Object.keys(want).length) break;
    if (Date.now() - lastRefresh > 10000) { await page.click("#refresh").catch(() => {}); lastRefresh = Date.now(); }
    await page.waitForTimeout(1000);
  }
  return { got, missing: Object.keys(want).filter((k) => !(k in got)) };
}
// The side panel is folded at some sizes: a feature view in it shows it, and folds it back.
async function withSide(page) {
  const folded = await page.$eval('[data-k="sidetog"]', (b) => b.getAttribute("aria-expanded") === "false").catch(() => false);
  if (folded) { await page.click('[data-k="sidetog"]'); await page.waitForTimeout(400); }
  return async () => { if (folded) await page.click('[data-k="sidetog"]').catch(() => {}); };
}
const sideTab = async (page, lane, tab) => {
  await page.click('[data-k="tab:t:' + lane + '"]');
  await page.waitForTimeout(300);
  const back = await withSide(page);
  await page.click('[data-k="stab:' + tab + '"]');
  await page.waitForTimeout(400);
  return async () => { await page.click('[data-k="stab:agents"]').catch(() => {}); await back(); };
};
// reach clicks a control, or, when something covers it, presses it as the keyboard would,
// so the view behind it is still shot (the layout rules report the cover).
async function reach(page, sel) {
  try { await page.click(sel, { timeout: 3000 }); } catch (e) { await page.$eval(sel, (b) => b.click()); }
}
// toProject switches to a project from the menu; what it returns switches back to Alpha.
async function toProject(page, id) {
  const name = id[0].toUpperCase() + id.slice(1);
  await page.click("#projbtn");
  await page.click("#proj-" + id);
  await page.waitForFunction((n) => document.getElementById("pname").textContent === n, name, { timeout: 10000 });
  await page.waitForTimeout(600);
  return async () => {
    await page.click("#projbtn");
    await page.click("#proj-alpha");
    await page.waitForFunction(() => document.getElementById("pname").textContent === "Alpha", null, { timeout: 10000 });
    await page.waitForSelector('[data-k="tab:t:working"]', { timeout: 10000 });
  };
}
async function flowCardShort(page, pct) {
  const home = await toProject(page, "beta");
  if (pct) await page.evaluate((v) => window.PanelScale.set(v), pct);
  if (pct) await page.waitForTimeout(700);
  const back = await withSide(page);
  await page.waitForSelector(".flowcard", { timeout: 10000 });
  if (!(await page.$(".flowcard .fsub"))) throw new Error("Beta's Flow card shows no spend span (.fsub): the short-history case is not on the page");
  await page.locator(".flowcard").scrollIntoViewIfNeeded();
  return async () => { await back(); if (pct) await page.evaluate(() => window.PanelScale.reset()); await home(); };
}
const FEATURES = {
  async "needs-alerts"(page) {
    await page.waitForSelector("#needs:not([hidden])", { timeout: 3000 });
    await page.locator("#needs").scrollIntoViewIfNeeded();
    return true;
  },
  async "economy-badge"(page) {
    const q = await page.$('[data-k="q:eco"]');
    if (!q) return null;
    await page.hover('[data-k="q:eco"]');
    return true;
  },
  async "flow-card"(page) {
    await page.click('[data-k="tab:t:' + alphaLanes[0] + '"]');
    const back = await withSide(page);
    const fc = page.locator(".flowcard");
    if (!(await fc.count())) { await back(); return null; }
    await fc.scrollIntoViewIfNeeded();
    return back;
  },
  async "budget-bar-over"(page) {
    if (!alphaLanes.includes("budget")) return null;
    await page.click('[data-k="tab:t:budget"]');
    await page.waitForSelector('#lanehead [data-k="budget"]', { timeout: 5000 });
    return true;
  },
  async "budget-bar-amber"(page) {
    await page.click('[data-k="tab:t:working"]');
    await page.waitForSelector('#lanehead [data-k="budget"]', { timeout: 5000 });
    return true;
  },
  async "port-remote-header"(page) {
    await page.click('[data-k="tab:t:working"]');
    await page.waitForSelector('#lanehead [data-k="port"]', { timeout: 5000 });
    await page.waitForSelector('#lanehead [data-k="remote"]', { timeout: 5000 });
    return true;
  },
  async "checks-working"(page) {
    const back = await sideTab(page, "working", "checks");
    await page.waitForSelector('#side [data-k="ck:v"]', { timeout: 5000 });
    return back;
  },
  async "checks-ready"(page) {
    const back = await sideTab(page, "ready", "checks");
    await page.waitForSelector('#side [data-k="ck:v"]', { timeout: 5000 });
    return back;
  },
  async "checks-root"(page) {
    return sideTab(page, "idle", "checks");
  },
  async "stale-alerts-tab"(page) {
    return sideTab(page, "quiet", "alerts");
  },
  async "row-menu-remote"(page) {
    const back = await withRail(page);
    const k = '[data-k="ra:t:idle"]';
    await page.locator(k).scrollIntoViewIfNeeded({ timeout: 5000 });
    await reach(page, k); // the footer covers it below 1180 px (the covered rule reports it)
    await page.waitForSelector("#rowmenu:not([hidden])", { timeout: 3000 });
    if (!(await page.$('#rowmenu [data-act="remote-control"]'))) throw new Error("the ⋯ menu of a running lane has no Remote control in lanes mode");
    return async () => { await page.keyboard.press("Escape"); await back(); };
  },
  async "remote-confirm"(page) {
    const back = await withRail(page);
    const k = '[data-k="ra:t:idle"]';
    await page.locator(k).scrollIntoViewIfNeeded({ timeout: 5000 });
    await reach(page, k); // the footer covers it below 1180 px (the covered rule reports it)
    await page.click('#rowmenu [data-act="remote-control"]');
    await page.waitForSelector('#termbar [data-k="confirm"]', { timeout: 3000 });
    return async () => { await page.click('#termbar [data-k="b:cancel"]').catch(() => {}); await back(); };
  },
  async "remote-confirm-busy"(page) {
    const back = await withRail(page);
    const k = '[data-k="ra:t:working"]';
    await page.locator(k).scrollIntoViewIfNeeded({ timeout: 5000 });
    await reach(page, k); // the footer covers it below 1180 px (the covered rule reports it)
    await page.click('#rowmenu [data-act="remote-control"]');
    await page.waitForSelector('#termbar [data-k="confirm"]', { timeout: 3000 });
    return async () => { await page.click('#termbar [data-k="b:cancel"]').catch(() => {}); await back(); };
  },
  // PANEL-21: Beta's spend has under a week kept (its ledger starts with the run), so its
  // Flow card shows the span ("since …, 1 day") on its own line; at 1440×900 and 110% it
  // pushed the Spend label out and the sparklines past the side panel. The flowcard rule
  // checks every row's label and the card's edge; this view also checks at 150%.
  async "flow-card-short"(page) { return flowCardShort(page, 0); },
  async "flow-card-short-150"(page) { return flowCardShort(page, 150); },
  async "metrics-short-spend"(page) {
    if (!(await page.$("#metricsbtn"))) return null;
    const home = await toProject(page, "beta");
    await page.click("#metricsbtn");
    await page.waitForSelector("#mview:not([hidden])", { timeout: 3000 });
    await page.click('[data-k="mscope:project"]').catch(() => {});
    await page.click('[data-k="mrange:30d"]').catch(() => {});
    await page.click("#mtab-cost");
    await metricsLoaded(page);
    if (!(await page.$("#mvbody .mrow .msub"))) throw new Error("Beta's Metrics Cost tab shows no spend span (.msub): the short-history case is not on the page");
    await page.locator("#mvbody .mrow .msub").first().scrollIntoViewIfNeeded().catch(() => {});
    return async () => { await page.click("#mtab-flow").catch(() => {}); await page.click("#mviewclose").catch(() => {}); await home(); };
  },
  async "metrics-from-flow-card"(page) {
    await page.click('[data-k="tab:t:' + alphaLanes[0] + '"]');
    const back = await withSide(page);
    const fc = page.locator(".flowcard");
    if (!(await fc.count())) { await back(); return null; }
    await fc.click();
    await page.waitForSelector("#mview:not([hidden])", { timeout: 3000 });
    await metricsLoaded(page);
    return async () => { await page.click("#mviewclose").catch(() => {}); await back(); };
  },
};
async function featuresArea(browser) {
  const pick = VIEWPORTS.filter((v) => ["1280x800", "1440x900", "1920x1080", "900x800"].includes(vpName(v)));
  let first = true;
  for (const vp of pick.length ? pick : VIEWPORTS.slice(0, 1)) {
    const area = "features-" + vpName(vp);
    const { ctx, page, errors } = await L.openPage(browser, R, { context: { viewport: vp } });
    try {
      await page.waitForSelector('[data-k="tab:t:working"]', { timeout: 15000 });
      const c = Object.assign({ viewport: vpName(vp) }, await appearance(page));
      if (first) {
        first = false;
        const w = await waitSignals(page);
        console.log("matrix: signals after " + JSON.stringify(w.got) + " s" + (w.missing.length ? "; missing " + w.missing.join(", ") : ""));
        for (const m of w.missing) featureFinding(area, "signals", "no " + m + " signal on the page after 110 s (Needs you / status bar)", c);
      } else await settle(page, 1500);
      for (const [name, fn] of Object.entries(FEATURES)) {
        await page.evaluate(() => window.scrollTo(0, 0));
        let undo;
        try { undo = await fn(page); } catch (e) {
          F.add(Object.assign({ area, name, rule: "view-failed", selector: "", detail: String(e.message || e).split("\n")[0] }, c));
          await page.keyboard.press("Escape").catch(() => {});
          continue;
        }
        if (undo === null) { F.pass(area + "/" + name, { skipped: "not in this build or state" }); continue; }
        await settle(page);
        await shot(page, errors, area, name, c);
        if (typeof undo === "function") await undo().catch(() => {});
        await settle(page, 200);
      }
    } finally { await ctx.close(); }
  }
}

// railActs (PANEL-21): every lane's ⋯ in the rail, whole and clickable, at the rail's
// narrowest (160 px), its default and its widest; a shot of each width.
async function railActs(page, area, c) {
  const back = await withRail(page);
  try {
    for (const w of [160, 0, "max"]) {
      await page.evaluate((w) => { if (w === 0) { rail.w = 0; saveRail(); applyRail(); } else setRailW(w === "max" ? railMax() : w); }, w);
      await settle(page, 400);
      const name = "rail-" + (w === 0 ? "default" : w);
      const dir = path.join(out, "shots", area);
      fs.mkdirSync(dir, { recursive: true });
      const file = path.join(dir, name + ".png");
      await page.screenshot({ path: file });
      shots++;
      for (const f of await L.rowActAudit(page)) F.add(Object.assign({ area, name, screenshot: path.relative(out, file) }, c, f));
      F.pass(area + "/" + name);
    }
  } finally {
    await page.evaluate(() => { rail.w = 0; saveRail(); applyRail(); }).catch(() => {});
    await back();
  }
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
      await railActs(page, area, c);
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
    // PANEL-22: the dropdown open, and Add a project's trust report, in every theme × mode.
    for (const [th] of offered.themes) for (const mode of ["light", "dark"]) combos.push([th, mode, null, ["overview", "new-lane", "row-menu", "project-menu", "project-add-trust"]]);
    for (const [ty] of offered.types) combos.push(["hmi", "light", ty, ["overview", "lane-stream", "project-menu"]]);
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
        if (pct >= 150 || pct === steps[0]) {
          await runView(page, errors, "sizes", vpName(vp) + "-" + pct + "-project-menu", c);
          await runView(page, errors, "sizes", vpName(vp) + "-" + pct + "-project-add-init", c);
        }
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
    if (areas.includes("features")) await featuresArea(browser);
    if (areas.includes("metrics")) await metricsArea(browser);
  } catch (e) {
    F.add({ area: "matrix", name: "crash", rule: "harness", selector: "", detail: String(e.stack || e) });
  } finally {
    await browser.close();
  }
  console.log("matrix: " + shots + " shots, " + F.items.length + " findings in " + Math.round((Date.now() - t0) / 1000) + " s → " + F.file);
})();
