// PANEL-11: appearance is three independent choices (theme, type system, size), and the
// lane workspace is driven from the keyboard. Against a real panel with two lanes
// (run.sh starts them), this checks, in a real browser:
// - every type system's faces load (document.fonts), and the page takes them;
// - changing the type system refits the terminal: its rows take the new face and the
//   screen is refitted to the new cell width;
// - the terminal tabs: the arrow keys move and select, and never enter a terminal;
// - Enter on the terminal's frame enters it, Ctrl+] leaves to the lane's tab;
// - Ctrl+Alt+= and Ctrl+Alt+- change the page's text size, and not from inside a terminal;
// - at the largest size (175%) the page does not scroll sideways and its controls are on screen.
// Usage: node appearance-and-keys.cjs <base URL> <token>
const pw = require(process.env.PLAYWRIGHT || "playwright");
const [base, token, typedLog] = process.argv.slice(2);
const fs = require("fs");
const typed = () => { try { return fs.readFileSync(typedLog); } catch (e) { return Buffer.alloc(0); } };
const fail = (msg) => { console.error("FAIL: " + msg); process.exitCode = 1; };

(async () => {
  const b = await pw.chromium.launch();
  try {
    const p = await (await b.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
    const errors = [];
    p.on("pageerror", (e) => errors.push(e.message));
    await p.goto(base + "/?t=" + token);
    await p.waitForSelector('#tabs [role="tab"]:nth-child(2)', { timeout: 15000 });
    await p.waitForSelector(".xterm-rows", { timeout: 15000 });

    // Every type system loads its faces, and the page uses them.
    const types = await p.evaluate(() => window.PanelTheme.types.map((x) => x.id));
    if (types.length < 2) fail("fewer than two type systems: " + types);
    for (const id of types) {
      const r = await p.evaluate(async (id) => {
        window.PanelTheme.setType(id);
        const cs = getComputedStyle(document.documentElement);
        const out = { attr: document.documentElement.dataset.type, faces: {} };
        for (const k of ["font-ui", "font-data", "font-term"]) {
          const fam = cs.getPropertyValue("--" + k).split(",")[0].trim().replace(/"/g, "");
          const w = "400";
          await document.fonts.load(w + " 16px \"" + fam + "\"");
          out.faces[k] = fam + ":" + ([...document.fonts].some((f) => f.family.replace(/"/g, "") === fam && f.status === "loaded") ? "loaded" : "NOT LOADED");
        }
        out.body = getComputedStyle(document.body).fontFamily.split(",")[0].replace(/"/g, "");
        // A terminal face must tell ( from [: a round bracket's left edge curves, so few
        // of its rows share one column; B612 Mono's is 0.8 straight and reads as [.
        const term = cs.getPropertyValue("--font-term").split(",")[0].trim().replace(/"/g, "");
        const c = document.createElement("canvas"); c.width = 64; c.height = 96;
        const x = c.getContext("2d"); x.font = '64px "' + term + '"'; x.fillText("(", 8, 72);
        const px = x.getImageData(0, 0, 64, 96).data, cols = {}; let rows = 0;
        for (let yy = 0; yy < 96; yy++) for (let xx = 0; xx < 64; xx++) if (px[(yy * 64 + xx) * 4 + 3] > 128) { rows++; cols[xx] = (cols[xx] || 0) + 1; break; }
        out.parenStraight = rows ? Math.max(...Object.values(cols)) / rows : 1;
        out.term = term;
        out.bodyWant = cs.getPropertyValue("--font-ui").split(",")[0].trim().replace(/"/g, "");
        return out;
      }, id);
      if (r.attr !== id) fail(id + ": data-type is " + r.attr);
      for (const [k, v] of Object.entries(r.faces)) if (!v.endsWith(":loaded")) fail(id + ": --" + k + " " + v);
      if (r.body !== r.bodyWant) fail(id + ": the body renders in " + r.body + ", not " + r.bodyWant);
      if (r.parenStraight > 0.5) fail(id + ": the terminal face " + r.term + " draws ( nearly straight (" + r.parenStraight.toFixed(2) + "), so it reads as [");
    }

    // A type change refits the terminal to the new face's cell.
    const cell = () => p.evaluate(() => {
      const m = document.querySelector(".xterm-char-measure-element"), rows = document.querySelector(".xterm-rows");
      const screen = document.querySelector(".xterm-screen").getBoundingClientRect().width;
      const host = document.querySelector(".termhost .term:not([hidden])").getBoundingClientRect().width;
      return { w: m.getBoundingClientRect().width / Math.max(1, m.textContent.length), fam: getComputedStyle(rows).fontFamily.split(",")[0].replace(/"/g, ""),
        want: getComputedStyle(document.documentElement).getPropertyValue("--font-term").split(",")[0].trim().replace(/"/g, ""), screen, host };
    });
    await p.evaluate(() => window.PanelTheme.setType("highway"));
    await p.waitForTimeout(1200);
    const a = await cell();
    await p.evaluate(() => window.PanelTheme.setType("engineer"));
    await p.waitForTimeout(1200);
    const c = await cell();
    if (a.fam !== a.want || c.fam !== c.want) fail("the terminal's rows do not take the type's face: " + JSON.stringify([a, c]));
    if (Math.abs(a.w - c.w) < 0.3) fail("the terminal's cell did not change with the type: " + a.w + " → " + c.w);
    if (c.screen > c.host + 1 || c.screen < c.host - 3 * c.w) fail("the terminal was not refitted: screen " + c.screen + " in a " + c.host + " frame, cell " + c.w);
    await p.evaluate(() => window.PanelTheme.setType(null));

    // The tabs: arrows move and select, focus stays on the tabs.
    await p.focus('#tabs [role="tab"][aria-selected="true"]');
    const before = await p.evaluate(() => document.activeElement.textContent);
    await p.keyboard.press("ArrowRight");
    await p.waitForTimeout(300);
    const after = await p.evaluate(() => ({ t: document.activeElement.textContent, role: document.activeElement.getAttribute("role"), sel: document.activeElement.getAttribute("aria-selected"),
      tab0: document.querySelectorAll('#tabs [tabindex="0"]').length }));
    if (after.role !== "tab" || after.sel !== "true" || after.t === before || after.tab0 !== 1) fail("ArrowRight did not move and select the next tab: " + JSON.stringify(after));
    // Into the terminal only on purpose, and out with Ctrl+].
    await p.focus("#termhost");
    await p.keyboard.press("Enter");
    const inTerm = await p.evaluate(() => document.activeElement.classList.contains("xterm-helper-textarea"));
    if (!inTerm) fail("Enter on the terminal's frame did not enter it");
    // The size keys work inside a terminal too, and not one byte of them reaches claude:
    // the fake claude records what it is sent, and a plain key must arrive.
    const s0 = await p.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--ui-scale"));
    const b0 = typed().length;
    await p.keyboard.press("Control+Alt+Equal");
    await p.keyboard.press("Control+Alt+Minus");
    await p.keyboard.press("Control+Alt+Equal");
    await p.keyboard.press("Control+Alt+Digit0");
    await p.keyboard.press("Control+Alt+Equal");
    await p.keyboard.type("q");
    await p.waitForTimeout(800);
    const s1 = await p.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--ui-scale"));
    const got = typed().subarray(b0);
    // +5, −5, +5, reset, +5: one step above where it started.
    if (Math.abs(parseFloat(s1) - (parseFloat(s0) + 0.05)) > 1e-6) fail("the size keys inside a terminal did not act: " + s0 + " → " + s1);
    if (got.toString("latin1") !== "q") fail("the size keys reached claude: it was sent " + JSON.stringify(got.toString("latin1")) + ", want only \"q\"");
    await p.evaluate(() => window.PanelScale.reset());
    await p.keyboard.press("Control+BracketRight");
    const out = await p.evaluate(() => ({ role: document.activeElement.getAttribute("role"), sel: document.activeElement.getAttribute("aria-selected") }));
    if (out.role !== "tab" || out.sel !== "true") fail("Ctrl+] did not leave to the lane's tab: " + JSON.stringify(out));
    await p.keyboard.press("Control+Alt+Equal");
    const s2 = await p.evaluate(() => parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--ui-scale")));
    await p.keyboard.press("Control+Alt+Minus");
    await p.keyboard.press("Control+Alt+Minus");
    const s3 = await p.evaluate(() => parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--ui-scale")));
    if (!(s2 > parseFloat(s0) && s3 < s2)) fail("Ctrl+Alt+= / - did not change the page's size: " + [s0, s2, s3]);
    const stillOut = await p.evaluate(() => !document.activeElement.classList.contains("xterm-helper-textarea"));
    if (!stillOut) fail("a size key moved focus into a terminal");
    // The largest page text (175%) at 1440 px: no page-level horizontal scroll, and
    // every main control reachable (on screen once scrolled to, and not covered). The layout folds the side panel
    // (and the rail if it must) to leave the terminal room.
    await p.evaluate(() => window.PanelScale.set(175));
    await p.waitForTimeout(800);
    const big = await p.evaluate(() => {
      const bad = [];
      for (const sel of ["#addlane", "#themebtn", "#railbtn", '#tabs [role="tab"][aria-selected="true"]', "#addtab", "#termhost", "#termbar button", '[data-k="sidetog"]']) {
        const e = document.querySelector(sel);
        if (!e) { bad.push(sel + ": missing"); continue; }
        // Reachable: the page may scroll down to it (never sideways), and then nothing covers it.
        e.scrollIntoView({ block: "nearest", inline: "nearest" });
        const r = e.getBoundingClientRect();
        const x = Math.min(r.left + r.width / 2, innerWidth - 1), y = Math.min(r.top + Math.min(r.height / 2, 10), innerHeight - 1);
        const at = document.elementFromPoint(x, y);
        if (r.right > innerWidth + 1 || r.bottom > innerHeight + 1 || r.width < 1 || !at || !(at === e || e.contains(at) || at.closest(".xterm") && e.id === "termhost"))
          bad.push(sel + " at " + Math.round(r.left) + "," + Math.round(r.top) + " " + Math.round(r.width) + "x" + Math.round(r.height));
      }
      window.scrollTo(0, 0);
      return { scale: getComputedStyle(document.documentElement).getPropertyValue("--ui-scale"), sw: document.documentElement.scrollWidth, w: innerWidth, bad,
        term: Math.round(document.getElementById("termhost").getBoundingClientRect().width) };
    });
    if (big.scale.trim() !== "1.75") fail("PanelScale.set(175) did not apply: " + big.scale);
    if (big.sw > big.w) fail("at 175% the page scrolls sideways: " + big.sw + " > " + big.w);
    if (big.bad.length) fail("at 175% these controls are off screen or covered: " + big.bad.join("; "));
    if (big.term < 500) fail("at 175% the terminal is only " + big.term + " px wide");
    await p.evaluate(() => window.PanelScale.reset());
    if (errors.length) fail("page errors: " + errors.join(" | "));
    if (!process.exitCode) console.log("ok: " + types.length + " type systems load; a type change refits the terminal; tabs, Enter, Ctrl+] and the size keys behave");
  } finally {
    await b.close();
  }
})().catch((e) => { console.error(e); process.exitCode = 1; });
