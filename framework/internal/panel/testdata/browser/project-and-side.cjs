// PANEL-12: where the project stands, and what to start next, in a real browser against
// a real panel (run.sh pins two cards and gives a template a suggest command):
// - the pinned cards share a tab box under the lane's tabs, one tab per card, and the
//   ←/→ keys move between them;
// - a card's line is a title that opens, and an opened row stays open through a refresh
//   (the fold's handler once read the discarded description and shut it on the next poll);
// - the box folds, and says so (aria-expanded);
// - the side panel's edge drags wider, but never so wide that the terminal loses its 85
//   columns (past that, the layout used to fold the side panel away under the drag);
// - the Columns menu opens inside the window, not clipped by the rail;
// - New lane lists Up next, and a row picks its template and fills in the name.
// Usage: node project-and-side.cjs <base URL> <token>
const pw = require(process.env.PLAYWRIGHT || "playwright");
const [base, token] = process.argv.slice(2);
const fail = (msg) => { console.error("FAIL: " + msg); process.exitCode = 1; };

(async () => {
  const b = await pw.chromium.launch();
  try {
    const p = await (await b.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
    const errors = [];
    p.on("pageerror", (e) => errors.push(e.message));
    await p.goto(base + "/?t=" + token);
    await p.waitForSelector(".xterm-rows", { timeout: 15000 });
    await p.waitForSelector('#side .projhead [role="tab"]', { timeout: 15000 });

    const tabs = await p.$$eval('#side .projhead [role="tab"]', (ts) => ts.map((t) => [t.textContent, t.getAttribute("aria-selected")]));
    if (tabs.length !== 2 || tabs[0][0] !== "Founder queue" || tabs[1][0] !== "Change queue") fail("project tabs " + JSON.stringify(tabs));
    // Below the lane's own tabs, never above them.
    const order = await p.evaluate(() => {
      const lane = document.querySelector("#side > .sidetabs"), proj = document.querySelector("#side .sidepins");
      return lane && proj && (lane.compareDocumentPosition(proj) & Node.DOCUMENT_POSITION_FOLLOWING) ? "ok" : "wrong";
    });
    if (order !== "ok") fail("the project box is not under the lane's tabs");
    await p.waitForSelector("#side .projbody summary", { timeout: 15000 });
    const founder = await p.$$eval("#side .projbody summary", (ss) => ss.map((s) => s.textContent));
    if (JSON.stringify(founder) !== JSON.stringify(["Box cleanup", "Ideas page"])) fail("founder rows " + JSON.stringify(founder));
    const cap = await p.$eval("#side .projbody li.cap", (e) => e.textContent).catch(() => "");
    if (!cap.startsWith("2 item(s) need the founder")) fail("the caption line is not a caption: " + cap);

    // A row opens, and stays open through a refresh and the renders after it.
    await p.click("#side .projbody summary >> nth=0");
    await p.evaluate(() => fetch("/api/refresh", { method: "POST" }));
    await p.waitForTimeout(2500);
    const open = await p.$$eval("#side .projbody details", (ds) => ds.map((d) => d.open));
    if (!open[0] || open[1]) fail("after a refresh the open rows are " + JSON.stringify(open));
    const body = await p.$eval("#side .projbody details[open] .pinbody", (e) => e.innerHTML);
    if (/\*\*/.test(body)) fail("markdown shown raw: " + body);

    // → moves to the change queue; its rows are the id and change name.
    await p.focus('#side .projhead [role="tab"][aria-selected="true"]');
    await p.keyboard.press("ArrowRight");
    await p.waitForTimeout(200);
    const q = await p.$$eval("#side .projbody summary", (ss) => ss.map((s) => s.textContent));
    if (JSON.stringify(q) !== JSON.stringify(["2C.10 add-score-photo", "2C.27 add-group-card"])) fail("change queue rows " + JSON.stringify(q));
    const focused = await p.evaluate(() => document.activeElement.textContent);
    if (focused !== "Change queue") fail("→ left focus on " + focused);

    // The box folds.
    await p.click("#side .projfold");
    const folded = await p.evaluate(() => ({ body: !!document.querySelector("#side .projbody"), exp: document.querySelector("#side .projfold").getAttribute("aria-expanded") }));
    if (folded.body || folded.exp !== "false") fail("the fold did not fold: " + JSON.stringify(folded));
    await p.click("#side .projfold");

    // The side panel's edge: drag it far left. It widens, stays shown, and the terminal keeps
    // 85 columns. The rail is hidden first, so there is room to widen into.
    await p.click("#railbtn");
    await p.waitForTimeout(400);
    const sp = await p.$("#sidesplit");
    const box = await sp.boundingBox();
    const w0 = await p.$eval("#side", (e) => e.getBoundingClientRect().width);
    await p.mouse.move(box.x + box.width / 2, box.y + 200);
    await p.mouse.down();
    for (let x = box.x; x > 100; x -= 40) await p.mouse.move(x, box.y + 200);
    await p.mouse.up();
    await p.waitForTimeout(600);
    const after = await p.evaluate(() => ({ shown: !document.getElementById("wsgrid").classList.contains("noside"),
      w: document.getElementById("side").getBoundingClientRect().width, cols: terms[selTerm] ? terms[selTerm].term.cols : 0 }));
    if (!after.shown) fail("dragging the side edge folded the side panel away");
    if (!(after.w > w0 + 50)) fail("the side panel did not widen: " + w0 + " → " + after.w);
    if (after.cols < 85) fail("the terminal has " + after.cols + " columns after the drag (want at least 85)");
    await p.dblclick("#sidesplit");
    await p.waitForTimeout(300);
    const reset = await p.$eval("#side", (e) => e.getBoundingClientRect().width);
    if (Math.abs(reset - w0) > 2) fail("a double-click did not restore the width: " + reset + " vs " + w0);
    await p.click("#railbtn");

    // Columns opens its menu inside the window, even from a narrow rail (it was cut off by
    // the rail and ran off the window's left edge).
    await p.evaluate(() => setRailW(170));
    await p.click('[data-k="cols"]');
    const menu = await p.$eval(".colmenu", (m) => { const r = m.getBoundingClientRect(); return { l: r.left, r: r.right, t: r.top, b: r.bottom, w: innerWidth, h: innerHeight, labels: m.querySelectorAll("label").length }; });
    if (menu.l < 0 || menu.r > menu.w || menu.b > menu.h + 1 || menu.labels < 5) fail("the Columns menu is outside the window: " + JSON.stringify(menu));
    const hit = await p.evaluate(() => { const m = document.querySelector(".colmenu"), r = m.getBoundingClientRect(); const at = document.elementFromPoint(r.left + 10, r.top + 10); return m.contains(at); });
    if (!hit) fail("the Columns menu is covered or clipped");
    await p.keyboard.press("Escape");
    await p.click('[data-k="cols"]').catch(() => {});
    await p.evaluate(() => { colMenu = false; rail.w = 0; saveRail(); applyRail(); render(); });

    // Up next: a row picks its template and fills in the name.
    await p.click("#addlane");
    await p.waitForSelector("#st-next .nextrow", { timeout: 10000 });
    await p.click("#st-next .nextrow");
    const picked = await p.evaluate(() => ({ tpl: document.getElementById("st-tpl").value, name: document.getElementById("st-name").value,
      pressed: document.querySelector("#st-next .nextrow").getAttribute("aria-pressed") }));
    if (picked.tpl !== "propose" || picked.name !== "add-group-card" || picked.pressed !== "true") fail("Up next pick: " + JSON.stringify(picked));
    await p.keyboard.press("Escape");

    if (errors.length) fail("page errors: " + errors.join(" | "));
    if (!process.exitCode) console.log("ok: the project box tabs, folds and keeps rows open; the side edge drags without folding; Up next fills the dialog");
  } finally {
    await b.close();
  }
})().catch((e) => { console.error(e); process.exitCode = 1; });
