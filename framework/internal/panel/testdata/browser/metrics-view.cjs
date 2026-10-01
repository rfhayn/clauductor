// PANEL-19: the Metrics view in a real browser against a real panel. run.sh gives the
// project the fixture metrics command (metrics/testdata/metrics.sh: 7d and 30d, no 90d)
// and a gh that lists no merged pull request, so:
// - Metrics, next to Activity, opens the view; its tabs are a tablist (←/→, Home, End
//   move and select at once, as the side panel's do);
// - 30d Flow shows the project's cycle time, marked "project"; the panel's own merge
//   frequency fills in where the payload has none, marked "built in";
// - Cost's by-change table marks a change over its budget;
// - 90d is not in the payload: a figure there is "—" with why, never 0;
// - the scope switches to all projects; Escape closes the view and focus returns;
// - PANEL-19 part 3: the Flow card in the side panel opens the view;
// - nothing breaks the CSP, and at 390 px the page does not scroll sideways.
// Screenshots go to $SHOTS (a directory) when it is set, one per theme and one narrow.
// Usage: node metrics-view.cjs <base URL> <token>
const pw = require(process.env.PLAYWRIGHT || "playwright");
const path = require("path");
const [base, token] = process.argv.slice(2);
const shots = process.env.SHOTS || "";
const fail = (msg) => { console.error("FAIL: " + msg); process.exitCode = 1; };

(async () => {
  const b = await pw.chromium.launch();
  try {
    const ctx = await b.newContext({ viewport: { width: 1440, height: 900 } });
    const p = await ctx.newPage();
    const errors = [];
    p.on("pageerror", (e) => errors.push(e.message));
    p.on("console", (m) => { if (m.type() === "error" && /Content Security Policy|Refused/.test(m.text())) errors.push(m.text()); });
    await p.goto(base + "/?t=" + token);
    await p.waitForSelector(".xterm-rows", { timeout: 15000 });
    // The page says it is in view, so the panel reads merged pull requests.
    await p.evaluate(() => fetch("/api/seen", { method: "POST" }));

    const btn = await p.$eval("#metricsbtn", (e) => ({ text: e.textContent, prev: e.previousElementSibling && e.previousElementSibling.id, exp: e.getAttribute("aria-expanded") }));
    if (btn.text !== "Metrics" || btn.prev !== "activitybtn" || btn.exp !== "false") fail("the Metrics button " + JSON.stringify(btn));
    await p.click("#metricsbtn");
    await p.waitForSelector("#mvbody .mrow", { timeout: 15000 });
    // The metrics command reports within a poll of the panel's start.
    await p.waitForFunction(() => mv.data && mv.data.command && mv.data.command.ok, null, { timeout: 15000 }).catch(() => {});
    await p.evaluate(() => loadMetrics());
    await p.waitForTimeout(300);

    const tabs = await p.$$eval('#mvbar [role="tablist"] [role="tab"]', (ts) => ts.map((t) => [t.textContent, t.getAttribute("aria-selected"), t.tabIndex]));
    if (JSON.stringify(tabs.map((t) => t[0])) !== JSON.stringify(["Flow", "Cost", "Quality", "Outcomes"])) fail("tabs " + JSON.stringify(tabs));
    if (tabs[0][1] !== "true" || tabs[0][2] !== 0 || tabs[1][2] !== -1) fail("Flow is not the selected tab, alone in the Tab order: " + JSON.stringify(tabs));
    const focusOnTab = await p.evaluate(() => document.activeElement.id);
    if (focusOnTab !== "mtab-flow") fail("opening the view left focus on " + focusOnTab);
    const panel = await p.$eval("#mvbody", (e) => [e.getAttribute("role"), e.getAttribute("aria-labelledby")]);
    if (panel[0] !== "tabpanel" || panel[1] !== "mtab-flow") fail("the body is not the Flow tab's panel: " + panel);

    const fig = (label) => p.evaluate((label) => {
      const r = Array.from(document.querySelectorAll("#mvbody .mrow")).find((x) => x.querySelector(".ml").textContent === label);
      return r ? { v: r.querySelector(".mv").textContent, src: (r.querySelector(".msrc") || {}).textContent || "", note: (r.querySelector(".mnote") || {}).textContent || "",
        bars: r.querySelectorAll(".mbars rect").length } : null;
    }, label);
    const cyc = await fig("Cycle time");
    if (!cyc || cyc.v !== "7.5 h" || cyc.src !== "project" || cyc.bars < 10) fail("30d cycle time " + JSON.stringify(cyc));
    const cfr = await fig("Change-fail rate");
    if (!cfr || cfr.v !== "8.3%" || !/reverts/.test(cfr.note)) fail("30d change-fail rate " + JSON.stringify(cfr));
    if (shots) await p.screenshot({ path: path.join(shots, "metrics-flow-hmi.png") });

    // → moves to Cost and selects it; a change over its budget is marked.
    await p.keyboard.press("ArrowRight");
    await p.waitForTimeout(150);
    const cost = await p.evaluate(() => ({ focus: document.activeElement.id, sel: document.querySelector('#mvbar [aria-selected="true"]').textContent,
      over: Array.from(document.querySelectorAll("#mvbody tr.abn-warn")).map((r) => r.textContent) }));
    if (cost.focus !== "mtab-cost" || cost.sel !== "Cost") fail("→ did not move to Cost: " + JSON.stringify(cost));
    if (!cost.over.some((t) => /add-score-photo/.test(t) && /over/.test(t))) fail("the change over its budget is not marked: " + JSON.stringify(cost.over));
    if (shots) await p.screenshot({ path: path.join(shots, "metrics-cost-hmi.png") });
    await p.keyboard.press("End");
    await p.waitForTimeout(150);
    const outc = await p.evaluate(() => ({ sel: document.querySelector('#mvbar [aria-selected="true"]').textContent,
      rows: Array.from(document.querySelectorAll("#mvbody tbody tr")).map((r) => r.textContent) }));
    if (outc.sel !== "Outcomes" || outc.rows.length !== 2 || !outc.rows.some((t) => /yes/.test(t))) fail("Outcomes " + JSON.stringify(outc));

    // 90d: the payload has no such window. Quality's figures are "—" with why.
    await p.click('#mvbar [aria-label="Range"] button:has-text("90d")');
    await p.click("#mtab-quality");
    await p.waitForTimeout(150);
    const rr = await fig("Review rounds per change");
    if (!rr || rr.v !== "—" || !/no 90d window/.test(rr.note)) fail("90d review rounds " + JSON.stringify(rr));
    const pressed = await p.$$eval('#mvbar [aria-label="Range"] button', (bs) => bs.map((x) => x.getAttribute("aria-pressed")));
    if (JSON.stringify(pressed) !== JSON.stringify(["false", "false", "true"])) fail("range pressed " + JSON.stringify(pressed));
    if (shots) await p.screenshot({ path: path.join(shots, "metrics-quality-90d-missing.png") });
    // Flow at 90d: the panel's own merges (gh lists none: 0 a week, never "—"). PANEL-21:
    // the page came into view, so gh was read at once, and the view asks again every 3 s
    // while the read is pending: well within the wait, never "Reading…" for a minute.
    await p.click("#mtab-flow");
    await p.waitForFunction(() => mv.data && mv.data.merged && mv.data.merged.ok, null, { timeout: 8000 }).catch(() => fail("merged pull requests still not read 8 s after the page came into view"));
    await p.waitForTimeout(150);
    const mf = await fig("Merge frequency");
    if (!mf || mf.v !== "0 a week" || mf.src !== "built in" || !/No pull request was merged in the last 90d/.test(mf.note)) fail("90d merge frequency " + JSON.stringify(mf));
    // PANEL-21: spend with under a week kept is the amount so far and since when, never a
    // weekly rate spread over the window.
    const spanText = await p.evaluate(() => [mShown({ value: 11.7, span: { since: "2026-09-30", days: 1 } }, "$wk"), mShown({ value: 14 }, "$wk")]);
    if (!/^\$11\.70? \(since 2026-09-30, 1 day\)$/.test(spanText[0]) || !/a week$/.test(spanText[1])) fail("spend with a span " + JSON.stringify(spanText));

    // All projects.
    await p.click('#mvbar [aria-label="Scope"] button:has-text("All projects")');
    await p.waitForFunction(() => mv.data && mv.data.name === "All projects", null, { timeout: 5000 }).catch(() => fail("scope all never loaded"));
    const all = await p.$eval("#mvbody .mvsrc", (e) => e.textContent);
    if (!/^All projects: /.test(all)) fail("scope all says " + all);
    await p.click('#mvbar [aria-label="Scope"] button:has-text("This project")');
    await p.click('#mvbar [aria-label="Range"] button:has-text("30d")');

    // Every theme, light and dark, draws it: screenshots, and no figure is invisible.
    for (const th of ["hmi", "amber", "cockpit", "tuned", "tui", "native"]) {
      for (const mode of ["light", "dark"]) {
        const ok = await p.evaluate(([th, mode]) => {
          PanelTheme.setTheme(th); PanelTheme.setMode(mode); render();
          const v = document.querySelector("#mvbody .mrow .mv"), s = getComputedStyle(v), bg = getComputedStyle(document.getElementById("mview")).backgroundColor;
          return s.color !== bg && s.visibility === "visible";
        }, [th, mode]);
        if (!ok) fail("a figure is not visible in " + th + " " + mode);
        if (shots && (mode === "dark" || th === "tui")) await p.screenshot({ path: path.join(shots, "metrics-" + th + "-" + mode + ".png") });
      }
    }
    await p.evaluate(() => { PanelTheme.setTheme("hmi"); PanelTheme.setMode("system"); render(); });

    // Escape closes it, and focus goes back to the button.
    await p.focus("#mtab-flow");
    await p.keyboard.press("Escape");
    const closed = await p.evaluate(() => ({ hidden: document.getElementById("mview").hidden, focus: document.activeElement.id,
      exp: document.getElementById("metricsbtn").getAttribute("aria-expanded") }));
    if (!closed.hidden || closed.focus !== "metricsbtn" || closed.exp !== "false") fail("Escape: " + JSON.stringify(closed));

    // Narrow: the view fills the window and the page never scrolls sideways.
    await p.setViewportSize({ width: 390, height: 844 });
    await p.click("#metricsbtn");
    await p.waitForSelector("#mvbody .mrow");
    await p.waitForTimeout(300);
    const narrow = await p.evaluate(() => ({ sw: document.documentElement.scrollWidth, w: innerWidth,
      vw: document.getElementById("mview").getBoundingClientRect().width }));
    if (narrow.sw > narrow.w + 1) fail("at 390 px the page scrolls sideways: " + JSON.stringify(narrow));
    if (shots) await p.screenshot({ path: path.join(shots, "metrics-narrow.png") });
    await p.keyboard.press("Escape");
    await p.setViewportSize({ width: 1440, height: 900 });

    // The Flow card: four figures of the last 30 days under the project's box; a click
    // opens Metrics on Flow at 30d.
    await p.evaluate(() => { mv.tab = "cost"; mv.range = "7d"; saveMv(); });
    await p.waitForSelector("#side .flowcard", { timeout: 15000 });
    const card = await p.$eval("#side .flowcard", (c) => ({ head: c.querySelector(".fh").textContent, rows: Array.from(c.querySelectorAll(".frow .fl")).map((x) => x.textContent),
      vals: Array.from(c.querySelectorAll(".frow .fv")).map((x) => x.textContent), label: c.getAttribute("aria-label"),
      after: !!c.closest(".sideflow").previousElementSibling }));
    if (card.head !== "Flow (30d)" || JSON.stringify(card.rows) !== JSON.stringify(["Cycle time", "Merges", "Change-fail", "Spend"]) || card.vals[0] !== "7.5 h" || !/Opens Metrics/.test(card.label))
      fail("the Flow card " + JSON.stringify(card));
    if (shots) await p.screenshot({ path: path.join(shots, "flow-card.png") });
    await p.click("#side .flowcard");
    const opened = await p.evaluate(() => ({ hidden: document.getElementById("mview").hidden, tab: mv.tab, range: mv.range }));
    if (opened.hidden || opened.tab !== "flow" || opened.range !== "30d") fail("the Flow card did not open Flow at 30d: " + JSON.stringify(opened));
    await p.keyboard.press("Escape");
    // PANEL-21: spend with under a week kept puts its span on a line of its own; in one line
    // it pushed the Spend label out and the sparklines past the side panel (UX pass 2).
    for (const scale of [100, 110]) {
      const fit = await p.evaluate((scale) => {
        window.PanelScale.set(scale);
        const sp = S.flow.items.find((it) => it.key === "cost.per_week");
        sp.value = 0.2; sp.span = { since: "2026-09-30", days: 1 }; sp.series = [0, 0, 0.2];
        render();
        const side = document.getElementById("side").getBoundingClientRect(), c = document.querySelector("#side .flowcard");
        const rows = Array.from(c.querySelectorAll(".frow")).map((r) => { const l = r.querySelector(".fl"), lr = l.getBoundingClientRect(), rr = r.getBoundingClientRect();
          return { l: l.textContent, whole: l.scrollWidth <= l.clientWidth + 1 && lr.left >= rr.left - 1 && lr.right <= rr.right + 1,
            past: Math.max(0, ...Array.from(r.querySelectorAll("*")).map((k) => k.getBoundingClientRect().right - Math.min(side.right, rr.right))) }; });
        const sub = c.querySelector(".fsub");
        window.PanelScale.reset();
        return { rows, sub: sub && sub.textContent, title: sub && sub.parentElement.title, card: c.getBoundingClientRect().right - side.right, side: side.width };
      }, scale);
      if (fit.sub !== "since 2026-09-30, 1 day" || !/\$0\.20 \(since 2026-09-30, 1 day\)/.test(fit.title) || fit.side < 50 || fit.card > 1 || fit.rows.some((r) => !r.whole || r.past > 1))
        fail("the Flow card with a short spend history at " + scale + "% " + JSON.stringify(fit));
    }

    // PANEL-19 part 4: run.sh gave lane "second" a change whose proposal waits for
    // approval and has a budget. Refresh re-reads the change directory.
    await p.evaluate(() => fetch("/api/refresh", { method: "POST" }));
    await p.waitForFunction(() => (S.alerts || []).some((a) => a.kind === "approval_wait"), null, { timeout: 15000 }).catch(() => {});
    const strip = await p.$$eval("#needs tr", (rs) => rs.map((r) => r.textContent));
    if (!strip.some((t) => /approval wait/.test(t) && /change second has waited 2d/.test(t))) fail("the approval alert is not in Needs you: " + JSON.stringify(strip));
    await p.evaluate(() => selectLane("t:second"));
    await p.waitForSelector('#lanehead [data-k="budget"]', { timeout: 5000 }).catch(() => {});
    const bud = await p.$eval('#lanehead [data-k="budget"]', (e) => e.textContent).catch(() => "");
    if (!/^Budget.*of \$5\.00$/.test(bud)) fail("the budget bar in the lane header: " + JSON.stringify(bud));
    if (shots) await p.screenshot({ path: path.join(shots, "needs-and-budget.png") });

    // PANEL-19 part 5: the economy badge by the quota, naming the roles (a synthetic view:
    // the Go test drives the real switch in a temp HOME).
    const eco = await p.evaluate(() => {
      S.economy = { active: true, since: Date.now(), reason: "5-hour quota 87% ≥ 85%", threshold: 85, roles: [{ role: "scribe", to: "sonnet, low" }] };
      render();
      const f = document.querySelector('#quotas [data-k="q:eco"]');
      const out = f ? { text: f.textContent, title: f.title, afterQuota: !!f.previousElementSibling } : null;
      S.economy = null; render();
      return out;
    });
    if (!eco || !/^Economyonscribe to sonnet, low$/.test(eco.text) || !/87% ≥ 85%/.test(eco.title) || !eco.afterQuota) fail("the economy badge " + JSON.stringify(eco));

    // PANEL-19 part 6: remote control in lanes mode (synthetic: install writes the mode).
    const rc = await p.evaluate(() => {
      S.remoteControl = "lanes"; render();
      const head = (document.querySelector('#lanehead [data-k="remote"]') || {}).textContent || "";
      const t = lanesOf().find((x) => x.key === "t:second");
      const acts = rowActs(t.t).map((a) => a[1]);
      S.remoteControl = ""; render();
      return { head, acts, off: rowActs(t.t).map((a) => a[1]) };
    });
    if (rc.head !== "Remoteon, the panel's lanes" || !rc.acts.includes("Remote control") || rc.off.includes("Remote control"))
      fail("remote control in the page " + JSON.stringify(rc));

    if (errors.length) fail("page errors: " + errors.join("; "));
    if (!process.exitCode) console.log("ok metrics-view");
  } finally {
    await b.close();
  }
})().catch((e) => { console.error(e); process.exit(1); });
