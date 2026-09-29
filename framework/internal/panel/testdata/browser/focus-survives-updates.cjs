// PANEL-6, audit P1-3: the page used to rebuild every column once a second, so focus
// fell to <body> within 1.6 s and a selected text was cleared. It now patches in place.
// This proves it against a real panel (run.sh starts one): a focused button in a
// region that is being patched keeps focus through 5 s of updates, and a selection
// survives too. It checks the updates really arrived, so it cannot pass by accident.
// Usage: node focus-survives-updates.cjs <base URL> <token> <project root>
const pw = require(process.env.PLAYWRIGHT || "playwright");
const [base, token, root] = process.argv.slice(2);
const sid = "00000000-0000-4000-8000-000000000001";
const post = (path, body) => fetch(base + path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
const fail = (msg) => { console.error("FAIL: " + msg); process.exitCode = 1; };

(async () => {
  // A session waiting on a permission prompt: Needs you shows it, with a button.
  await post("/hook", { session_id: sid, cwd: root, hook_event_name: "UserPromptSubmit", prompt: "x" });
  await post("/hook", { session_id: sid, cwd: root, hook_event_name: "Notification", notification_type: "permission_prompt",
    message: "Claude needs your permission to use Bash" });
  const b = await pw.chromium.launch();
  try {
    const p = await (await b.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
    const errors = [];
    p.on("pageerror", (e) => errors.push(e.message));
    await p.goto(base + "/?t=" + token);
    await p.waitForSelector("#left .card.ask .jump", { timeout: 10000 });
    await p.focus("#left .card.ask .jump");
    await p.evaluate(() => { window.__focused = document.activeElement; });

    // Five seconds of real changes: each status post changes est. $, so the panel
    // pushes a new view, and the left column (where the button is) is patched.
    const seen = new Set();
    for (let i = 1; i <= 10; i++) {
      await post("/status", { session_id: sid, cwd: root, model: { id: "m", display_name: "Model" }, version: "2.1.284",
        cost: { total_cost_usd: i / 100 }, context_window: { used_percentage: 10 + i } });
      await p.waitForTimeout(500);
      seen.add(await p.textContent("#cost"));
    }
    if (seen.size < 5) fail("only " + seen.size + " distinct views reached the page; the test did not exercise updates");
    const kept = await p.evaluate(() => document.activeElement === window.__focused && window.__focused.isConnected);
    if (!kept) fail("focus moved during updates, to " + (await p.evaluate(() => document.activeElement.outerHTML.slice(0, 80))));

    // A selection inside a patched region survives updates to that region.
    const text = await p.evaluate(() => {
      const t = [...document.querySelectorAll("#left .card.ask div")].find((d) => d.textContent.includes("Permission"));
      const r = document.createRange(); r.selectNodeContents(t);
      getSelection().removeAllRanges(); getSelection().addRange(r);
      return getSelection().toString();
    });
    for (let i = 11; i <= 14; i++) {
      await post("/status", { session_id: sid, cwd: root, model: { id: "m", display_name: "Model" }, version: "2.1.284",
        cost: { total_cost_usd: i / 100 }, context_window: { used_percentage: 10 + i } });
      await p.waitForTimeout(500);
    }
    const after = await p.evaluate(() => getSelection().toString());
    if (!text || after !== text) fail("the selection changed from " + JSON.stringify(text) + " to " + JSON.stringify(after));
    // A selection from the Needs-you heading into a card holds that card only: a
    // new blocker still appears, and the heading and the title count it.
    await p.evaluate(() => {
      const l = document.getElementById("left"), r = document.createRange();
      r.setStart(l.querySelector("h2.mh"), 0); r.setEnd(l.querySelector(".card.ask"), 1);
      getSelection().removeAllRanges(); getSelection().addRange(r);
    });
    const sid2 = "00000000-0000-4000-8000-000000000002";
    await post("/hook", { session_id: sid2, cwd: root, hook_event_name: "UserPromptSubmit", prompt: "y" });
    await post("/hook", { session_id: sid2, cwd: root, hook_event_name: "Notification", notification_type: "permission_prompt",
      message: "Claude needs your permission to use Edit" });
    await p.waitForTimeout(1500);
    const r2 = await p.evaluate(() => ({ cards: document.querySelectorAll("#left .card.ask").length,
      head: document.querySelector("#left h2.mh").textContent, title: document.title, sel: getSelection().toString().length }));
    if (r2.cards !== 2 || !r2.head.startsWith("Needs you · 2") || !r2.title.startsWith("(2)")) fail("with a selection across Needs you, the new blocker did not show: " + JSON.stringify(r2));
    if (!r2.sel) fail("the selection across Needs you was lost");
    if (errors.length) fail("page errors: " + errors.join(" | "));
    if (!process.exitCode) console.log("ok: focus and selection survived " + seen.size + " updates; a new blocker showed through a selection");
  } finally {
    await b.close();
  }
})().catch((e) => { console.error(e); process.exitCode = 1; });
