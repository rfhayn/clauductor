// PANEL-20: the merge-readiness box, in a real browser against a real panel. run.sh gives
// lane "second" an open pull request (#7: one check failing, a review required, one of
// two threads unresolved), a tasks.md with one of two tasks ticked, and a gate receipt for
// another commit. The Checks tab says "Not ready" with every reason, and a line per
// check; the lane "main" (the project root) has nothing to merge.
// Usage: node lane-readiness.cjs <base URL> <token>
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
    await p.evaluate(() => fetch("/api/refresh", { method: "POST" }));
    await p.evaluate(() => { selectLane("t:second"); sideTab = "checks"; render(); });
    await p.waitForFunction(() => { const l = lanesOf().find((x) => x.key === "t:second"); const r = l && l.lv && l.lv.readiness;
      return r && r.rows.some((x) => x.name === "Review threads" && !x.unknown) && r.rows.some((x) => x.name === "Gate receipt"); }, null, { timeout: 20000 })
      .catch(() => fail("the readiness reads never arrived"));
    const box = await p.evaluate(() => ({
      tab: document.querySelector('#side [role="tab"][aria-selected="true"]').textContent,
      verdict: (document.querySelector("#side .ckverdict") || {}).textContent || "",
      rows: Array.from(document.querySelectorAll("#side .ckrows .k")).map((k) => [k.textContent, k.nextElementSibling.textContent, k.nextElementSibling.className]),
    }));
    if (box.tab !== "Checks") fail("the Checks tab is not selected: " + box.tab);
    for (const want of ["1 check(s) failed", "a review is required", "1 unresolved review thread(s)", "1 unticked task(s)", "no gate receipt for HEAD"])
      if (!box.verdict.includes(want)) fail("the verdict lacks " + JSON.stringify(want) + ": " + box.verdict);
    if (!/^Not ready: /.test(box.verdict)) fail("verdict " + box.verdict);
    const row = (n) => box.rows.find((r) => r[0] === n) || [];
    if (row("Pull request")[1] !== "#7, open") fail("PR row " + JSON.stringify(row("Pull request")));
    if (row("Review threads")[1] !== "1 of 2 unresolved" || !/crit/.test(row("Review threads")[2])) fail("threads row " + JSON.stringify(row("Review threads")));
    if (!/^1 of 2 ticked in changes\/second\/tasks\.md$/.test(row("Tasks")[1] || "")) fail("tasks row " + JSON.stringify(row("Tasks")));
    if (!/^for 0123456789, not HEAD /.test(row("Gate receipt")[1] || "")) fail("receipt row " + JSON.stringify(row("Gate receipt")));
    if (shots) await p.screenshot({ path: path.join(shots, "readiness.png") });
    // The project root's lane has nothing to merge.
    await p.evaluate(() => { selectLane("t:main"); sideTab = "checks"; render(); });
    const main = await p.$eval("#side .sidebody", (e) => e.textContent);
    if (!/no branch of its own/.test(main)) fail("main's Checks tab: " + main);
    await p.evaluate(() => { sideTab = "agents"; try { localStorage.setItem("clauductor-panel-sidetab", "agents"); } catch (e) {} render(); });
    if (errors.length) fail("page errors: " + errors.join("; "));
    if (!process.exitCode) console.log("ok lane-readiness");
  } finally {
    await b.close();
  }
})().catch((e) => { console.error(e); process.exit(1); });
