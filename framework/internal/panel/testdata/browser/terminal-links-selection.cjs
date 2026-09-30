// PANEL-14: a lane's links open, and a selection stays until it is copied. Lane
// "second" runs a fake claude that, like claude's fullscreen TUI, asks for every mouse
// motion (1003) and prints a plain URL and two OSC 8 links (run.sh). This checks, in a
// real browser against a real panel and tmux:
// - tmux relays the motion request and the page declines it, so a drag's selection
//   survives the pointer moving on, and no hover report reaches the lane;
// - a plain click on a URL opens nothing; ⌘-click opens it; ⌘-click on an OSC 8 link
//   whose text is not its address asks first, and on one whose text is, opens it;
// - no click reaches claude, and the wheel still does (tmux passes it on).
// Usage: node terminal-links-selection.cjs <base URL> <token> <typed.log>
const pw = require(process.env.PLAYWRIGHT || "playwright");
const [base, token, typedLog] = process.argv.slice(2);
const fs = require("fs");
const typed = () => { try { return fs.readFileSync(typedLog).toString("latin1"); } catch (e) { return ""; } };
const fail = (msg) => { console.error("FAIL: " + msg); process.exitCode = 1; };

(async () => {
  const b = await pw.chromium.launch();
  try {
    const p = await (await b.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
    const errors = [];
    p.on("pageerror", (e) => errors.push(e.message));
    let received = "", sent = [];
    p.on("websocket", (ws) => {
      ws.on("framereceived", (f) => { if (typeof f.payload !== "string") received += Buffer.from(f.payload).toString("latin1"); });
      ws.on("framesent", (f) => { const m = JSON.parse(String(f.payload)); if (m.type === "input") sent.push(m.data); });
    });
    await p.goto(base + "/?t=" + token);
    await p.waitForSelector('#tabs [role="tab"]:nth-child(2)', { timeout: 15000 });
    await p.click('#tabs [role="tab"]:has-text("second")');
    // The buffer, not the DOM (the renderer draws a space as a no-break space), and a
    // function, not a string: the page's CSP has no 'unsafe-eval'.
    const screenText = () => {
      const t = terms[selTerm]?.term, b = t?.buffer.active;
      let s = "";
      for (let y = 0; t && y < t.rows; y++) s += (b.getLine(b.viewportY + y)?.translateToString(true) ?? "") + "\n";
      return s;
    };
    await p.waitForFunction(() => {
      const t = terms[selTerm]?.term, b = t?.buffer.active;
      for (let y = 0; t && y < t.rows; y++) if (b.getLine(b.viewportY + y)?.translateToString(true).includes("Docs here")) return true;
      return false;
    }, null, { timeout: 15000 }).catch(async () => { throw new Error("lane second's text never showed: " + JSON.stringify(await p.evaluate(screenText))); });
    await p.evaluate(() => { window.__opened = []; window.open = (u) => { window.__opened.push(u); return null; }; });
    // The page centre of a cell of the first row holding text.
    const cellOf = (text, dx = 0) => p.evaluate(([text, dx]) => {
      const t = terms[selTerm].term, buf = t.buffer.active;
      for (let y = 0; y < t.rows; y++) {
        const x = buf.getLine(buf.viewportY + y).translateToString(true).indexOf(text);
        if (x < 0) continue;
        const r = t.element.querySelector(".xterm-screen").getBoundingClientRect(), c = t._core._renderService.dimensions.css.cell;
        return { x: r.left + (x + dx + 0.5) * c.width, y: r.top + (y + 0.5) * c.height };
      }
      return null;
    }, [text, dx]);
    const opened = () => p.evaluate(() => window.__opened.slice());

    if (!received.includes("\x1b[?1003h")) fail("tmux did not relay the lane's motion request (1003); the test exercises nothing");
    const proto = await p.evaluate(() => terms[selTerm].term._core.coreMouseService.activeProtocol);
    if (proto !== "DRAG") fail("the page took the motion request: xterm's mouse protocol is " + proto + ", want DRAG");

    // Drag across "Selectable words", let go, and move on as a hand does before ⌘C.
    const a = await cellOf("Selectable words"), z = await cellOf("Selectable words", 16); // the release cell is not selected
    if (!a) throw new Error("the lane's text is not on screen");
    await p.mouse.move(a.x, a.y);
    await p.mouse.down();
    await p.mouse.move(z.x, z.y, { steps: 8 });
    await p.mouse.up();
    const sel0 = await p.evaluate(() => terms[selTerm].term.getSelection());
    sent = [];
    await p.mouse.move(z.x + 200, z.y + 60, { steps: 12 });
    await p.waitForTimeout(300);
    const sel1 = await p.evaluate(() => terms[selTerm].term.getSelection());
    if (sel0 !== "Selectable words") fail("the drag selected " + JSON.stringify(sel0));
    if (sel1 !== sel0) fail("the selection did not survive the pointer moving on: " + JSON.stringify(sel0) + " → " + JSON.stringify(sel1));
    if (sent.length) fail("moving the pointer sent the lane " + JSON.stringify(sent));

    // A plain click on a URL opens nothing; ⌘-click opens it.
    const typed0 = typed().length;
    const u = await cellOf("https://github.com/o/r/pull/12", 4);
    await p.mouse.move(u.x - 5, u.y);
    await p.mouse.move(u.x, u.y);
    const tip = await p.evaluate(() => terms[selTerm].host.title);
    if (!tip.includes("https://github.com/o/r/pull/12")) fail("hovering the URL does not name it: " + JSON.stringify(tip));
    await p.mouse.click(u.x, u.y);
    if ((await opened()).length) fail("a plain click opened " + (await opened()));
    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await p.keyboard.down(mod);
    await p.mouse.click(u.x, u.y);
    await p.keyboard.up(mod);
    if (JSON.stringify(await opened()) !== JSON.stringify(["https://github.com/o/r/pull/12"])) fail("⌘-click on the URL opened " + JSON.stringify(await opened()));

    // An OSC 8 link whose text is not its address asks; one whose text is, opens.
    const d = await cellOf("Docs here", 1);
    await p.mouse.move(d.x - 5, d.y);
    await p.mouse.move(d.x, d.y);
    await p.keyboard.down(mod);
    await p.mouse.click(d.x, d.y);
    await p.keyboard.up(mod);
    await p.waitForTimeout(200);
    const bar = await p.textContent("#termbar");
    if (!bar.includes("https://example.com/elsewhere")) fail("⌘-click on \"Docs here\" did not show its address: " + JSON.stringify(bar));
    if ((await opened()).length !== 1) fail("\"Docs here\" opened without asking");
    await p.click('#termbar button:has-text("Cancel")');
    const s = await cellOf("https://example.com/same", 3);
    await p.mouse.move(s.x - 5, s.y);
    await p.mouse.move(s.x, s.y);
    await p.keyboard.down(mod);
    await p.mouse.click(s.x, s.y);
    await p.keyboard.up(mod);
    if ((await opened())[1] !== "https://example.com/same") fail("⌘-click on an OSC 8 link showing its address opened " + JSON.stringify(await opened()));

    // No click or hover reached claude; the wheel still reaches the lane.
    await p.waitForTimeout(300);
    const clicks = typed().slice(typed0);
    if (/\x1b\[<\d+;\d+;\d+[Mm]/.test(clicks)) fail("a mouse report reached claude: " + JSON.stringify(clicks));
    sent = [];
    await p.mouse.move(a.x, a.y + 40);
    await p.mouse.wheel(0, -200);
    await p.waitForTimeout(300);
    if (!sent.some((x) => /^\x1b\[<64;/.test(x))) fail("the wheel sent " + JSON.stringify(sent));
    if (errors.length) fail("page errors: " + errors.join("; "));
  } finally {
    await b.close();
  }
  if (!process.exitCode) console.log("ok: terminal links and selection");
})().catch((e) => { console.error(e); process.exit(1); });
