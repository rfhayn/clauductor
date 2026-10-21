// PANEL-51 (D11): a stalled lane on today's page, in a real browser against a real
// panel. run.sh starts this panel on its own (its own HOME, project and panel.json at
// version 7, alerts.stall_minutes 0.05: 3 s), so no other script sees a stalled lane.
// Its lane "second" has a terminal, a fake `claude agents` lists one idle session in
// its worktree, and its fake `gh` lists pull request #7 open for its branch: the lane
// stalls by D1 as it stands.
// [STALL-4-S1] The lane's row, its tab and its tree node read "stalled", its row is
// tinted as a blocking row, and a Needs-you row names "second", its quiet time, "#7
// open" and what it last waited on, whose Open terminal selects the lane.
// Usage: node stalled-lane.cjs <base URL> <token>
const pw = require(process.env.PLAYWRIGHT || "playwright");
const path = require("path");
const [base, token] = process.argv.slice(2);
const shots = process.env.SHOTS || "";
const fail = (msg) => { console.error("FAIL: " + msg); process.exitCode = 1; };

(async () => {
  const b = await pw.chromium.launch();
  try {
    const p = await (await b.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
    const errors = [];
    p.on("pageerror", (e) => errors.push(e.message));
    await p.goto(base + "/?t=" + token);
    await p.waitForSelector(".xterm-rows", { timeout: 15000 });
    await p.evaluate(() => fetch("/api/seen", { method: "POST" }));
    // Stalled, and exact: a first sight is a start, and a stall within a stall time of
    // the panel's start is approximate (STALL-1-S25), so this takes about two stall times.
    await p.waitForFunction(() => { const l = lanesOf().find((x) => x.key === "t:second"); return l && l.lv && l.lv.stall && !l.lv.stall.approx; },
      null, { timeout: 30000 }).catch(() => fail("lane second never stalled (exactly)"));
    await p.evaluate(() => { selectLane("t:main"); render(); });
    const got = await p.evaluate(() => {
      const l = lanesOf().find((x) => x.key === "t:second");
      const row = Array.from(document.querySelectorAll("table.tbl tbody tr")).find((tr) => tr.querySelector(".lane-nm") && tr.querySelector(".lane-nm").textContent === "second");
      const tab = document.getElementById("tab-t_second");
      const node = Array.from(document.querySelectorAll(".tree .node.wt")).find((n) => n.textContent.includes("change/second"));
      const need = Array.from(document.querySelectorAll("#needs tr.ask")).find((tr) => tr.children[1] && tr.children[1].textContent === "second");
      return {
        stall: l && l.lv ? l.lv.stall : null,
        row: row ? { word: (row.querySelector(".state-word") || {}).textContent || "", sq: (row.querySelector(".sq") || {}).className || "", cls: row.className, name: row.getAttribute("aria-label") || "" } : null,
        tab: tab ? { sq: (tab.querySelector(".sq") || {}).className || "", label: tab.getAttribute("aria-label") || "" } : null,
        node: node ? { text: node.textContent, sq: (node.querySelector(".sq") || {}).className || "" } : null,
        need: need ? { cls: need.className, text: need.textContent, button: (need.querySelector("button.jump") || {}).textContent || "" } : null,
        needs: Array.from(document.querySelectorAll("#needs tr.ask")).filter((tr) => tr.children[1] && tr.children[1].textContent === "second").length,
      };
    });
    if (!got.stall || !got.stall.openWork.includes("#7 open")) fail("lane second's stall: " + JSON.stringify(got.stall));
    if (!got.row) fail("no lane row for second");
    else {
      if (got.row.word !== "stalled" || !/\bstalled\b/.test(got.row.sq)) fail("the lane row: " + JSON.stringify(got.row));
      if (!/\babn-crit\b/.test(got.row.cls)) fail("the lane row is not tinted as a blocking row: " + got.row.cls);
    }
    if (!got.tab || !/\bstalled\b/.test(got.tab.sq) || !/, stalled/.test(got.tab.label)) fail("the tab: " + JSON.stringify(got.tab));
    if (!got.node || !/\bstalled\b/.test(got.node.sq) || !/stalled/.test(got.node.text)) fail("the tree node: " + JSON.stringify(got.node));
    if (got.needs !== 1) fail(got.needs + " Needs-you rows for second, want one");
    if (!got.need) fail("no Needs-you row for second");
    else {
      for (const want of ["Stalled", "quiet for ", "#7 open", got.stall ? got.stall.waitedOn : "(no stall)"])
        if (!got.need.text.includes(want)) fail("the Needs-you row lacks " + JSON.stringify(want) + ": " + got.need.text);
      if (!/\babn-crit\b/.test(got.need.cls)) fail("the Needs-you row is not blocking: " + got.need.cls);
      if (got.need.button !== "Open terminal") fail("the Needs-you row's button: " + got.need.button);
    }
    if (shots) await p.screenshot({ path: path.join(shots, "stalled-lane.png") });
    // Open terminal selects the lane.
    await p.evaluate(() => {
      const tr = Array.from(document.querySelectorAll("#needs tr.ask")).find((r) => r.children[1] && r.children[1].textContent === "second");
      tr.querySelector("button.jump").click();
    });
    const sel = await p.evaluate(() => selKey);
    if (sel !== "t:second") fail("Open terminal selected " + sel + ", want t:second");
    if (shots) await p.screenshot({ path: path.join(shots, "stalled-lane-selected.png") });
    if (errors.length) fail("page errors: " + errors.join("; "));
    if (!process.exitCode) console.log("ok stalled-lane");
  } finally {
    await b.close();
  }
})().catch((e) => { console.error(e); process.exit(1); });
