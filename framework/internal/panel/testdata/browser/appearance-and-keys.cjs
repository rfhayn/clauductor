// PANEL-11: appearance is three independent choices (theme, type system, size), and the
// lane workspace is driven from the keyboard. Against a real panel with two lanes
// (run.sh starts them), this checks, in a real browser:
// - every type system's faces load (document.fonts), and the page takes them;
// - changing the type system refits the terminal: its rows take the new face and the
//   screen is refitted to the new cell width;
// - the terminal tabs: the arrow keys move and select, and never enter a terminal;
// - Enter on the terminal's frame enters it, Ctrl+] leaves to the lane's tab;
// - Ctrl+Alt+= and Ctrl+Alt+- change the page's text size, and not from inside a terminal.
// Usage: node appearance-and-keys.cjs <base URL> <token>
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
        for (const k of ["font-display", "font-body", "font-mono", "font-term"]) {
          const fam = cs.getPropertyValue("--" + k).split(",")[0].trim().replace(/"/g, "");
          const w = k === "font-display" ? getComputedStyle(document.querySelector(".brand h1")).fontWeight : "400";
          await document.fonts.load(w + " 16px \"" + fam + "\"");
          out.faces[k] = fam + ":" + ([...document.fonts].some((f) => f.family.replace(/"/g, "") === fam && f.status === "loaded") ? "loaded" : "NOT LOADED");
        }
        out.body = getComputedStyle(document.body).fontFamily.split(",")[0].replace(/"/g, "");
        out.bodyWant = cs.getPropertyValue("--font-body").split(",")[0].trim().replace(/"/g, "");
        return out;
      }, id);
      if (r.attr !== id) fail(id + ": data-type is " + r.attr);
      for (const [k, v] of Object.entries(r.faces)) if (!v.endsWith(":loaded")) fail(id + ": --" + k + " " + v);
      if (r.body !== r.bodyWant) fail(id + ": the body renders in " + r.body + ", not " + r.bodyWant);
    }

    // A type change refits the terminal to the new face's cell.
    const cell = () => p.evaluate(() => {
      const m = document.querySelector(".xterm-char-measure-element"), rows = document.querySelector(".xterm-rows");
      const screen = document.querySelector(".xterm-screen").getBoundingClientRect().width;
      const host = document.querySelector(".termhost .term:not([hidden])").getBoundingClientRect().width;
      return { w: m.getBoundingClientRect().width / Math.max(1, m.textContent.length), fam: getComputedStyle(rows).fontFamily.split(",")[0].replace(/"/g, ""),
        want: getComputedStyle(document.documentElement).getPropertyValue("--font-term").split(",")[0].trim().replace(/"/g, ""), screen, host };
    });
    await p.evaluate(() => window.PanelTheme.setType("typewriter"));
    await p.waitForTimeout(1200);
    const a = await cell();
    await p.evaluate(() => window.PanelTheme.setType("stencil"));
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
    const s0 = await p.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--ui-scale"));
    await p.keyboard.press("Control+Alt+Equal");
    const s1 = await p.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue("--ui-scale"));
    if (s0 !== s1) fail("Ctrl+Alt+= was taken from inside a terminal");
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
    if (errors.length) fail("page errors: " + errors.join(" | "));
    if (!process.exitCode) console.log("ok: " + types.length + " type systems load; a type change refits the terminal; tabs, Enter, Ctrl+] and the size keys behave");
  } finally {
    await b.close();
  }
})().catch((e) => { console.error(e); process.exitCode = 1; });
