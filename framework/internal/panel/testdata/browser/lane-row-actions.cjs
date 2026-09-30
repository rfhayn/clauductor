// PANEL-18: a lane's actions from the lists, in a real browser against a real panel
// (run.sh starts lanes "main" and "second"):
// - each lane row in the Lanes table and each lane in the Worktrees tree has a "⋯"
//   menu button; a click on it opens the menu and does not select its row;
// - the menu is keyboard operable: Down opens it on its first item, Up/Down/End move,
//   Escape closes it back to its button;
// - picking an item selects the lane and opens the in-page confirmation (never acts):
//   no lane request is sent until a Confirm button is pressed, and Cancel sends none;
// - a click on the row itself still selects it.
// Usage: node lane-row-actions.cjs <base URL> <token>
const pw = require(process.env.PLAYWRIGHT || "playwright");
const [base, token] = process.argv.slice(2);
const fail = (msg) => { console.error("FAIL: " + msg); process.exitCode = 1; };

(async () => {
  const b = await pw.chromium.launch();
  try {
    const p = await (await b.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
    const errors = [];
    p.on("pageerror", (e) => errors.push(e.message));
    const posts = [];
    p.on("request", (r) => { if (r.method() === "POST" && /\/lanes\/[^/]+\/[a-z-]+$/.test(r.url()) && !/ticket|image/.test(r.url()) && !/"dryRun":true/.test(r.postData() || "")) posts.push(r.url()); });
    await p.goto(base + "/?t=" + token);
    await p.waitForSelector('[data-k="ra:t:second"]', { timeout: 15000 });
    await p.waitForSelector('[data-k="rt:t:second"]', { timeout: 15000 });

    // Select main by its row, so a stray selection of "second" would show.
    await p.click('[data-k="row:t:main"] .lane-nm');
    await p.waitForTimeout(200);
    const selected = () => p.$eval(".tbl tr.sel .lane-nm", (e) => e.textContent).catch(() => "");
    if ((await selected()) !== "main") fail("a click on main's row did not select it");

    // A click on second's ⋯ opens the menu on its first item, and leaves main selected.
    await p.click('[data-k="ra:t:second"]');
    await p.waitForSelector("#rowmenu:not([hidden])", { timeout: 2000 });
    const m1 = await p.evaluate(() => ({
      items: Array.from(document.querySelectorAll('#rowmenu [role="menuitem"]')).map((e) => e.textContent),
      focus: document.activeElement.textContent,
      expanded: document.querySelector('[data-k="ra:t:second"]').getAttribute("aria-expanded"),
    }));
    if (JSON.stringify(m1.items) !== JSON.stringify(["Interrupt (Esc)", "Restart", "Stop lane", "Close lane"])) fail("menu items " + JSON.stringify(m1.items));
    if (m1.focus !== "Interrupt (Esc)" || m1.expanded !== "true") fail("menu opened with focus " + m1.focus + ", aria-expanded " + m1.expanded);
    if ((await selected()) !== "main") fail("the ⋯ click selected its row");

    // Keys: Down, End, Escape back to the button.
    await p.keyboard.press("ArrowDown");
    let f = await p.evaluate(() => document.activeElement.textContent);
    if (f !== "Restart") fail("Down moved to " + f);
    await p.keyboard.press("End");
    f = await p.evaluate(() => document.activeElement.textContent);
    if (f !== "Close lane") fail("End moved to " + f);
    await p.keyboard.press("Escape");
    await p.waitForTimeout(100);
    const back = await p.evaluate(() => ({ hidden: document.getElementById("rowmenu").hidden, k: document.activeElement.dataset.k }));
    if (!back.hidden || back.k !== "ra:t:second") fail("Escape left " + JSON.stringify(back));

    // From the keyboard: Down on the button opens it; picking Stop lane asks, and selects the lane.
    await p.keyboard.press("ArrowDown");
    await p.waitForSelector("#rowmenu:not([hidden])", { timeout: 2000 });
    await p.keyboard.press("ArrowDown");
    await p.keyboard.press("ArrowDown");
    f = await p.evaluate(() => document.activeElement.textContent);
    if (f !== "Stop lane") fail("two Downs reached " + f);
    await p.keyboard.press("Enter");
    await p.waitForSelector('#termbar [data-k="confirm"]', { timeout: 2000 });
    await p.waitForTimeout(200); // focusKey focuses on the next frame
    const ask = await p.evaluate(() => ({ text: document.querySelector('#termbar [data-k="confirm"]').textContent, focus: document.activeElement.textContent,
      menu: document.getElementById("rowmenu").hidden }));
    if (!/^Stop lane second\?/.test(ask.text) || ask.focus !== "Cancel" || !ask.menu) fail("Stop lane from the menu: " + JSON.stringify(ask));
    if ((await selected()) !== "second") fail("picking an item did not select its lane");
    await p.keyboard.press("Enter"); // Cancel
    await p.waitForTimeout(200);

    // The tree's ⋯: Interrupt asks too.
    await p.click('[data-k="rt:t:second"]');
    await p.waitForSelector("#rowmenu:not([hidden])", { timeout: 2000 });
    await p.click('#rowmenu [data-act="interrupt"]');
    await p.waitForSelector('#termbar [data-k="confirm"]', { timeout: 2000 });
    const it = await p.$eval('#termbar [data-k="confirm"]', (e) => e.textContent);
    if (!/^Interrupt lane second\?/.test(it)) fail("Interrupt from the tree: " + it);
    const btns = await p.$$eval("#termbar button", (bs) => bs.map((x) => x.textContent));
    if (!btns.includes("Confirm interrupt")) fail("no Confirm interrupt: " + JSON.stringify(btns));
    await p.click('#termbar [data-k="b:cancel"]');

    // Close lane asks with its plan (a dry run is the only request), and Cancel closes nothing.
    await p.click('[data-k="ra:t:second"]');
    await p.click('#rowmenu [data-act="close"]');
    await p.waitForSelector('#termbar [data-k="closeask"] .closelist, #termbar .stop.sub', { timeout: 10000 });
    await p.click('#termbar [data-k="b:close-cancel"]');

    // A click elsewhere closes the menu.
    await p.click('[data-k="ra:t:main"]');
    await p.waitForSelector("#rowmenu:not([hidden])", { timeout: 2000 });
    await p.mouse.click(700, 5);
    await p.waitForTimeout(100);
    if (!(await p.$eval("#rowmenu", (e) => e.hidden))) fail("a click elsewhere left the menu open");

    await p.waitForTimeout(300);
    if (posts.length) fail("lane actions were sent without a confirmation: " + JSON.stringify(posts));
    if (errors.length) fail("page errors: " + errors.join("; "));
  } finally {
    await b.close();
  }
  if (!process.exitCode) console.log("ok lane-row-actions");
})().catch((e) => { console.error(e); process.exit(1); });
