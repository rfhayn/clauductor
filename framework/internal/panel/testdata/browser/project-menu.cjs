// PANEL-22: the project menu, in a real browser against a real panel (run.sh):
// - the selector reads as a dropdown box: a 1 px box in the bar's ink, the name and a
//   chevron; the "need you elsewhere" badge sits beside it, not in it;
// - the menu's keys: Down opens it on the current project, End reaches Add a project…,
//   Right a row's ⋯, Enter its actions, Escape back out, one level at a time;
// - Remove from panel… on a project with lanes is refused, names each lane, and a lane's
//   link shows it;
// - Add a project…: a refused path says why; a repository with no config shows what
//   `panel init` would write and writes nothing; Create this config writes it; a config
//   edited after its report is refused at Trust and add and the report is read again;
//   Trust and add serves it at once and the page switches to it;
// - removing it again: allowed (no lanes), the page goes back to the default, and the
//   config stays on disk.
// Usage: node project-menu.cjs <base URL> <token> <project root> <other repo> <socket for it>
const fs = require("fs");
const path = require("path");
const pw = require(process.env.PLAYWRIGHT || "playwright");
const [base, token, proj, other, sock2] = process.argv.slice(2);
const fail = (msg) => { console.error("FAIL: " + msg); process.exitCode = 1; };
const shots = process.env.SHOTS;
const cfg = path.join(other, ".clauductor", "panel.json");

(async () => {
  const b = await pw.chromium.launch();
  try {
    const p = await (await b.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
    const errors = [];
    p.on("pageerror", (e) => errors.push(e.message));
    // A refused action answers 409, which the browser logs; only the one this test
    // provokes (stale bytes at Trust and add) is expected.
    let conflictOK = false;
    p.on("console", (m) => { if (m.type() === "error" && !(conflictOK && /409/.test(m.text()))) errors.push(m.text()); });
    await p.goto(base + "/?t=" + token);
    await p.waitForSelector(".xterm-rows", { timeout: 15000 });
    const shot = async (n) => { if (shots) await p.screenshot({ path: path.join(shots, "project-menu-" + n + ".png") }); };

    // The dropdown box.
    const look = await p.evaluate(() => {
      const b = document.getElementById("projbtn"), cs = getComputedStyle(b), ch = b.querySelector(".chev");
      return { bw: cs.borderTopWidth, bs: cs.borderTopStyle, name: b.querySelector(".pname").textContent, chev: ch && ch.textContent,
        chevShown: !!ch && ch.getBoundingClientRect().width > 0, badgeInside: !!b.querySelector(".projelse"),
        label: b.getAttribute("aria-label"), popup: b.getAttribute("aria-haspopup") };
    });
    if (look.bw !== "1px" || look.bs !== "solid" || look.chev !== "▾" || !look.chevShown || look.name !== "Focus test" || look.badgeInside || look.popup !== "listbox"
      || !/^Project Focus test.*Switch project$/.test(look.label)) fail("the selector's look: " + JSON.stringify(look));
    await shot("closed");

    // Keys.
    await p.focus("#projbtn");
    await p.keyboard.press("ArrowDown");
    await p.waitForSelector("#projmenu:not([hidden])");
    const at = () => p.evaluate(() => { const a = document.activeElement; return a.id || a.dataset.key || a.textContent; });
    if ((await at()) !== "proj-focus-test") fail("Down opened the menu on " + (await at()));
    await p.keyboard.press("End");
    if ((await at()) !== "addproj") fail("End reached " + (await at()));
    await p.keyboard.press("Home");
    await p.keyboard.press("ArrowRight");
    if ((await at()) !== "more:focus-test") fail("Right reached " + (await at()));
    await shot("open");
    await p.keyboard.press("Enter");
    await p.waitForSelector("#projacts:not([hidden])");
    const acts = await p.$$eval('#projacts [role="menuitem"]', (es) => es.map((e) => e.textContent));
    if (JSON.stringify(acts) !== JSON.stringify(["Remove from panel…"])) fail("a trusted project's actions: " + JSON.stringify(acts));
    await p.keyboard.press("Escape");
    if ((await at()) !== "more:focus-test" || !(await p.$eval("#projacts", (e) => e.hidden))) fail("Escape did not close the actions back to the ⋯");
    await p.keyboard.press("Escape");
    if ((await at()) !== "projbtn" || !(await p.$eval("#projmenu", (e) => e.hidden))) fail("Escape did not close the menu back to its button");

    // Remove, refused: it has lanes; a lane's link shows it.
    await p.click("#projbtn");
    await p.click('[data-key="more:focus-test"]');
    await p.click('#projacts [data-act="remove"]');
    await p.waitForSelector("#pdlg:not([hidden]) ul.lanes li", { timeout: 5000 });
    const refused = await p.evaluate(() => ({ lanes: Array.from(document.querySelectorAll("#pd-body ul.lanes button")).map((b) => b.textContent),
      go: !document.getElementById("pd-go").hidden, text: document.getElementById("pd-body").innerText }));
    if (refused.go || !refused.lanes.includes("main") || !refused.lanes.includes("second") || !/Nothing on disk is deleted/.test(refused.text)) fail("remove with lanes: " + JSON.stringify(refused));
    await shot("remove-refused");
    await p.click('#pd-body ul.lanes button:text-is("second")');
    await p.waitForTimeout(500);
    const sel = await p.$eval('#tabs [role="tab"][aria-selected="true"]', (e) => e.dataset.k).catch(() => "");
    if (!(await p.$eval("#pdlg", (e) => e.hidden)) || sel !== "tab:t:second") fail("the lane's link: dialog hidden " + (await p.$eval("#pdlg", (e) => e.hidden)) + ", selected " + sel);

    // Add a project….
    await p.click("#projbtn");
    await p.click("#addproj");
    await p.waitForSelector("#adddlg:not([hidden])");
    if ((await at()) !== "ad-path") fail("the dialog opened with focus on " + (await at()));
    await p.fill("#ad-path", proj);
    await p.waitForFunction(() => /Not addable: .*registered already/.test(document.getElementById("ad-status").textContent), null, { timeout: 5000 }).catch(() => fail("a registered root: " + "not refused"));
    await p.fill("#ad-path", other);
    await p.waitForSelector("#ad-body pre.cfg", { timeout: 8000 });
    if (await p.$eval("#ad-init", (e) => e.hidden)) fail("no Create this config");
    if (!(await p.$eval("#ad-trust", (e) => e.hidden))) fail("Trust and add is offered before there is a config");
    if (fs.existsSync(cfg)) fail("the init preview wrote " + cfg);
    const preview = await p.$eval("#ad-body", (e) => e.innerText);
    if (!/clauductor panel init would write/.test(preview) || !/"version"/.test(preview) || !/name\s+"addme"/.test(preview)) fail("the preview: " + preview.slice(0, 400));
    await shot("add-init-preview");
    await p.click("#ad-init");
    await p.waitForSelector("#ad-trust:not([hidden])", { timeout: 8000 });
    if (!fs.existsSync(cfg)) fail("Create this config wrote nothing");
    await shot("add-trust-report");
    // The config changes after its report (here: its own tmux socket, a throwaway one):
    // Trust and add refuses the stale bytes and reads the report again.
    const before = await p.$eval("#ad-body h4", (e) => e.textContent);
    fs.writeFileSync(cfg, fs.readFileSync(cfg, "utf8").replace("{\n", '{\n  "tmux_socket": "' + sock2 + '",\n'));
    conflictOK = true;
    await p.click("#ad-trust");
    await p.waitForFunction(() => /changed since you read it/.test(document.getElementById("ad-err").textContent), null, { timeout: 5000 }).catch(() => fail("stale bytes were not refused"));
    await p.waitForFunction((h) => { const e = document.querySelector("#ad-body h4"); return e && e.textContent !== h; }, before, { timeout: 5000 }).catch(() => fail("the report was not read again"));
    conflictOK = false;
    await p.click("#ad-trust");
    await p.waitForFunction(() => document.getElementById("pname").textContent === "addme", null, { timeout: 10000 }).catch(() => fail("the page did not switch to the added project"));
    await p.waitForTimeout(800);
    const m = await p.evaluate(() => (P || []).map((x) => x.id + ":" + x.trusted));
    if (!m.includes("addme:true")) fail("the menu after Trust and add: " + JSON.stringify(m));
    await shot("added");

    // Remove it: no lanes, so it is allowed; the page goes back to the default.
    await p.click("#projbtn");
    await p.click('[data-key="more:addme"]');
    await p.click('#projacts [data-act="remove"]');
    await p.waitForSelector("#pd-go:not([hidden])", { timeout: 5000 });
    await shot("remove-confirm");
    await p.click("#pd-go");
    await p.waitForFunction(() => document.getElementById("pname").textContent === "Focus test", null, { timeout: 10000 }).catch(() => fail("the page did not go back to the default"));
    await p.waitForTimeout(800);
    if ((await p.evaluate(() => (P || []).map((x) => x.id))).includes("addme")) fail("the removed project is still in the menu");
    if (!fs.existsSync(cfg)) fail("removing deleted the config");

    if (errors.length) fail("page errors: " + errors.join(" | "));
    if (!process.exitCode) console.log("ok: the dropdown, its keys, a refused remove with lane links, Add a project… from init preview to trusted and live, and removing it");
  } finally {
    await b.close();
  }
})().catch((e) => { console.error(e); process.exitCode = 1; });
