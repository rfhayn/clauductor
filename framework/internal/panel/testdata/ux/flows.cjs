// UX harness (UX-1): end-to-end flows with assertions, against a panel up.sh started.
// These ACT: they start, stop, close and forget lanes, remove a worktree and kill the
// run's own tmux server. Run them on a run of their own, never one matrix.cjs shares.
//
//   node flows.cjs <up.json | its JSON line> [--out dir] [--flows lanes,attachments,projects]
//                  [--only name,name]
//
// Groups (and their flows, in order):
//   lanes        start-new-lane, start-here, type-and-leave, interrupt, restart, stop,
//                close, remove-leftover, forget-orphan, restore-all (kills the server)
//   attachments  drop-png, drop-jpeg, paste-image, drop-non-image, selection-persists,
//                cmd-click-url, osc8-asks
//   projects     switch-keeps-selection, gamma-cannot-load
// Each flow writes pass or fail (with the failed step) to <out>/findings.json, and a
// screenshot to <out>/shots/flows/<flow>.png.
const path = require("path");
const fs = require("fs");
const { execFileSync } = require("child_process");
const L = require("./lib.cjs");
const pw = L.loadPlaywright();

const args = process.argv.slice(2);
const opt = (name, d) => { const i = args.indexOf("--" + name); return i >= 0 ? args[i + 1] : d; };
const R = L.run(args[0] && !args[0].startsWith("--") ? args[0] : null);
const out = path.resolve(opt("out", path.join(require("os").tmpdir(), "clauductor-ux-out", "flows-" + R.runid)));
const groups = opt("flows", "lanes,attachments,projects").split(",");
const only = opt("only", "") ? opt("only").split(",") : null;
const F = new L.Findings(out, "flows:" + groups.join("+"));
const alpha = R.projects.alpha, WT = path.join(alpha, ".claude", "worktrees");
const MOD = process.platform === "darwin" ? "Meta" : "Control";
const shotDir = path.join(out, "shots", "flows");
fs.mkdirSync(shotDir, { recursive: true });

// ---- the machine side --------------------------------------------------------------
const env = Object.assign({}, process.env, { HOME: R.home, GIT_CONFIG_GLOBAL: path.join(R.dir, "gitconfig") });
delete env.TMUX;
const sh = (cmd, a, o = {}) => execFileSync(cmd, a, Object.assign({ env, stdio: "pipe" }, o)).toString();
const git = (...a) => sh("git", a);
const tmux = (...a) => sh("tmux", ["-L", R.sockets[0], ...a]);
const hasSession = (id) => { try { tmux("has-session", "-t", "=" + id); return true; } catch (e) { return false; } };
const branchExists = (b) => { try { git("-C", alpha, "rev-parse", "--verify", "-q", "refs/heads/" + b); return true; } catch (e) { return false; } };
const typedFor = (lane) => { try { return fs.readFileSync(path.join(R.home, "fake", "typed", lane + ".log")).toString("latin1"); } catch (e) { return ""; } };
function registry() {
  // Every lane id in the run's lane registries (~/.clauductor/panel/<hash>/lanes.json).
  const dir = path.join(R.home, ".clauductor", "panel"), ids = [];
  for (const d of fs.existsSync(dir) ? fs.readdirSync(dir) : []) {
    const f = path.join(dir, d, "lanes.json");
    if (!fs.existsSync(f)) continue;
    const j = JSON.parse(fs.readFileSync(f, "utf8"));
    const lanes = Array.isArray(j) ? j : (j.lanes || []);
    for (const l of Object.values(lanes)) ids.push(l.id || l.ID);
  }
  return ids;
}
const fakeSession = (sid) => { try { return JSON.parse(fs.readFileSync(path.join(R.home, "fake", "agents", sid + ".json"), "utf8")); } catch (e) { return null; } };

// ---- the page side -----------------------------------------------------------------
let page, ctx, errors;
const until = async (what, fn, ms = 15000, every = 250) => {
  const t0 = Date.now();
  for (;;) {
    const v = await fn();
    if (v) return v;
    if (Date.now() - t0 > ms) throw new Error("timed out waiting for " + what);
    await L.sleep(every);
  }
};
const select = async (id) => {
  await page.click('[data-k="tab:t:' + id + '"]');
  // An orphan has no terminal: only a running lane gets terms[id].
  await page.waitForFunction((id) => typeof selTerm !== "undefined" && selTerm === id, id, { timeout: 10000 }).catch(() => {});
  await page.waitForTimeout(300);
};
const screenHas = (text) => page.evaluate((text) => {
  const t = terms[selTerm]?.term, b = t?.buffer.active;
  for (let y = 0; t && y < b.length; y++) if (b.getLine(y)?.translateToString(true).includes(text)) return true;
  return false;
}, text);
const termbar = () => page.$eval("#termbar", (e) => e.innerText).catch(() => "");
const laneState = () => page.evaluate(() => S);

// A flow: its steps throw on failure; the first failure is its finding.
const FLOWS = [];
const flow = (group, name, fn) => FLOWS.push({ group, name, fn });

async function runFlow(f) {
  const t0 = Date.now();
  let err = null;
  try { await f.fn(); } catch (e) { err = e; }
  const file = path.join(shotDir, f.name + ".png");
  await page.screenshot({ path: file }).catch(() => {});
  const secs = +((Date.now() - t0) / 1000).toFixed(1);
  const rel = path.relative(out, file);
  const errs = errors.splice(0);
  if (err) F.add({ area: "flows", name: f.name, group: f.group, rule: "flow", selector: "", detail: String(err.message || err).split("\n")[0], screenshot: rel, secs });
  else F.pass("flows/" + f.name, { secs, screenshot: rel });
  for (const e of errs) F.add({ area: "flows", name: f.name, group: f.group, rule: "console", selector: "", detail: e, screenshot: rel });
  await page.waitForTimeout(1000); // let a refit after the last change settle
  // The same layout rules as the matrix, on the state the flow left (more lanes, a
  // result bar, a restore bar: states the matrix never reaches).
  for (const a of await L.stableAudit(page).catch(() => [])) F.add(Object.assign({ area: "flows", name: f.name, group: f.group, screenshot: rel, viewport: "1440x900" }, a));
  console.log((err ? "FAIL " : "ok   ") + f.name + " (" + secs + " s)" + (err ? ": " + String(err.message || err).split("\n")[0] : ""));
  // Put the page back: no dialog, menu or confirmation left open.
  await page.keyboard.press("Escape").catch(() => {});
  for (const k of ['[data-k="b:cancel"]', '[data-k="b:close-cancel"]', '[data-k="b:close-dismiss"]', '[data-k="b:link-cancel"]'])
    await page.click(k, { timeout: 300 }).catch(() => {});
}

// ---- lanes -------------------------------------------------------------------------
flow("lanes", "start-new-lane", async () => {
  await page.click("#addlane");
  await page.waitForSelector("#startdlg:not([hidden])");
  await page.selectOption("#st-tpl", "");
  await page.selectOption("#st-type", "build");
  await page.check('input[name="st-mode"][value="new"]');
  await page.fill("#st-name", "ux-started");
  await page.click("#st-go");
  await page.waitForSelector('[data-k="tab:t:ux-started"]', { timeout: 20000 });
  if (!fs.existsSync(path.join(WT, "ux-started"))) throw new Error("no worktree at .claude/worktrees/ux-started");
  if (!branchExists("change/ux-started")) throw new Error("no branch change/ux-started");
  if (!hasSession("ux-started")) throw new Error("no tmux session ux-started");
  await select("ux-started");
  await until("the lane's screen", () => screenHas("Selectable words"), 15000);
});

flow("lanes", "start-here", async () => {
  const here = path.join(WT, "here-wt");
  git("-C", alpha, "worktree", "add", "-q", "--detach", here, "main");
  await page.click("#refresh");
  const li = '[data-k="tw:' + here + '"]';
  await page.waitForSelector(li, { timeout: 15000 });
  await page.locator(li + ' [data-k="wt:start"]').scrollIntoViewIfNeeded();
  await page.click(li + ' [data-k="wt:start"]');
  await page.waitForSelector("#startdlg:not([hidden])");
  const d = await page.evaluate(() => ({ mode: document.querySelector('input[name="st-mode"]:checked').value, wt: document.getElementById("st-wt").value, name: document.getElementById("st-name").value }));
  if (d.mode !== "existing" || d.wt !== here) throw new Error("the dialog did not open on the worktree: " + JSON.stringify(d));
  if (d.name !== "here-wt") throw new Error("the name was not filled in from the worktree: " + JSON.stringify(d.name));
  await page.click("#st-go");
  await page.waitForSelector('[data-k="tab:t:here-wt"]', { timeout: 20000 });
  const sid = await until("the lane's session id", async () => ((await laneState()).terminals || []).find((t) => t.id === "here-wt")?.sessionId);
  const s = await until("the fake session", () => fakeSession(sid), 10000);
  if (s.cwd !== here) throw new Error("the lane runs in " + s.cwd + ", not " + here);
});

flow("lanes", "type-and-leave", async () => {
  await select("ux-started");
  await until("the lane's screen", () => screenHas("Selectable words"), 15000);
  const before = typedFor("ux-started").length;
  await page.focus("#termhost");
  await page.keyboard.press("Enter");
  const inside = await page.evaluate(() => document.activeElement.classList.contains("xterm-helper-textarea"));
  if (!inside) throw new Error("Enter on the terminal's frame did not enter it");
  await page.keyboard.type("hello from the ux harness");
  await until("the typed bytes in typed.log", () => typedFor("ux-started").slice(before).includes("hello from the ux harness"), 5000);
  if (!fs.readFileSync(R.typedLog).toString("latin1").includes("hello from the ux harness")) throw new Error("typed.log lacks the text");
  await page.keyboard.press("Control+BracketRight");
  const out = await page.evaluate(() => ({ role: document.activeElement.getAttribute("role"), sel: document.activeElement.getAttribute("aria-selected"), text: document.activeElement.textContent }));
  if (out.role !== "tab" || out.sel !== "true") throw new Error("Ctrl+] did not leave to the lane's tab: " + JSON.stringify(out));
  await page.keyboard.type("zz");
  await page.waitForTimeout(400);
  if (typedFor("ux-started").slice(before).includes("zz")) throw new Error("keys after Ctrl+] still reached claude");
});

flow("lanes", "interrupt", async () => {
  await select("working");
  const before = typedFor("working").length;
  await page.click('#termbar [data-k="b:Interrupt (Esc)"]');
  await until("Escape in the working lane", () => typedFor("working").slice(before).includes("\x1b"), 5000);
  if (!hasSession("working")) throw new Error("interrupt ended the lane");
});

flow("lanes", "restart", async () => {
  await select("ux-started");
  const t = ((await laneState()).terminals || []).find((x) => x.id === "ux-started");
  const s0 = fakeSession(t.sessionId);
  await page.click('#termbar [data-k="b:Restart"]');
  await page.waitForSelector('#termbar [data-k="confirm"]', { timeout: 3000 });
  await page.click('#termbar button:has-text("Confirm restart")');
  const s1 = await until("a new claude on the same session", () => { const s = fakeSession(t.sessionId); return s && s0 && s.pid !== s0.pid ? s : null; }, 30000);
  if (!typedFor("ux-started").includes("/exit")) throw new Error("an idle lane was not sent /exit");
  const cmd = sh("ps", ["-o", "command=", "-p", String(s1.pid)]);
  if (!cmd.includes("--resume " + t.sessionId) && !cmd.includes("--session-id " + t.sessionId)) throw new Error("restarted without its own session id: " + cmd.trim());
});

flow("lanes", "stop", async () => {
  await select("stream");
  await page.click('#termbar [data-k="b:Stop lane"]');
  await page.waitForSelector('#termbar [data-k="confirm"]', { timeout: 3000 });
  const words = await page.$eval('#termbar [data-k="confirm"]', (e) => e.textContent);
  if (!/Escape/i.test(words)) throw new Error("a busy lane's stop does not say it presses Escape: " + words);
  await page.click('#termbar button:has-text("Confirm stop")');
  await until("the stream tab to go", async () => !(await page.$('[data-k="tab:t:stream"]')), 30000);
  if (hasSession("stream")) throw new Error("the tmux session stream is still there");
  if (!fs.existsSync(path.join(WT, "stream"))) throw new Error("Stop lane removed the worktree");
  if (registry().includes("stream")) throw new Error("stream is still in the lane registry");
});

flow("lanes", "close", async () => {
  await select("merged");
  await page.click('#termbar [data-k="b:Close lane"]');
  await page.waitForSelector('#termbar [data-k="closeask"] .closelist', { timeout: 20000 });
  const plan = await page.$eval('#termbar [data-k="closeask"]', (e) => e.innerText);
  if (!/Removes/.test(plan) || !/change\/merged/.test(plan)) throw new Error("the plan does not offer the branch: " + plan.replace(/\s+/g, " "));
  await page.click('#termbar button:has-text("Confirm close")');
  await page.waitForSelector("#closebar:not([hidden]) .closed", { timeout: 30000 });
  const done = await page.$eval("#closebar", (e) => e.innerText);
  if (!/Closed lane merged/.test(done)) throw new Error("close result: " + done.replace(/\s+/g, " "));
  if (fs.existsSync(path.join(WT, "merged"))) throw new Error("the worktree is still on disk");
  if (branchExists("change/merged")) throw new Error("the branch change/merged is still there");
  if (git("-C", alpha, "worktree", "list").includes("/merged ")) throw new Error("git still lists the worktree");
  if (registry().includes("merged")) throw new Error("merged is still in the lane registry");
  if (hasSession("merged")) throw new Error("the tmux session is still there");
});

flow("lanes", "remove-leftover", async () => {
  const left = R.leftovers[0];
  const k = '[data-k="wtrm:' + left + '"]';
  await page.locator(k).scrollIntoViewIfNeeded();
  await page.click(k);
  await page.waitForSelector(".tree .wtask .closelist", { timeout: 20000 });
  await page.click('.tree .wtask [data-k="wt:confirm"]');
  await page.waitForSelector("#closebar:not([hidden]) .closed", { timeout: 20000 });
  const done = await page.$eval("#closebar", (e) => e.innerText);
  if (!/Removed worktree/.test(done)) throw new Error("remove result: " + done.replace(/\s+/g, " "));
  if (fs.existsSync(left)) throw new Error("the worktree is still on disk");
  // The dirty one offers nothing to remove.
  const k2 = '[data-k="wtrm:' + R.leftovers[1] + '"]';
  await page.locator(k2).scrollIntoViewIfNeeded();
  await page.click(k2);
  await page.waitForSelector(".tree .wtask", { timeout: 20000 });
  await page.waitForFunction(() => !/Checking/.test(document.querySelector(".tree .wtask").innerText), null, { timeout: 20000 });
  if (await page.$('.tree .wtask [data-k="wt:confirm"]')) throw new Error("a dirty worktree was offered for removal");
  await page.click('[data-k="wtno:' + R.leftovers[1] + '"]');
});

flow("lanes", "forget-orphan", async () => {
  await select("orphan");
  await page.click('#termbar [data-k="b:Forget"]');
  await page.click('#termbar button:has-text("Confirm forget")');
  await until("the orphan tab to go", async () => !(await page.$('[data-k="tab:t:orphan"]')), 15000);
  if (registry().includes("orphan")) throw new Error("orphan is still in the lane registry");
  if (!fs.existsSync(path.join(WT, "orphan"))) throw new Error("Forget removed the worktree");
});

flow("lanes", "restore-all", async () => {
  const want = registry().filter((id) => R.lanes.alpha.includes(id) || ["ux-started", "here-wt"].includes(id));
  tmux("kill-server");
  await page.click("#refresh");
  await page.waitForSelector("#restorebar:not([hidden]) button:has-text(\"Restore all\")", { timeout: 30000 });
  const bar = await page.$eval("#restorebar", (e) => e.innerText);
  for (const id of want) if (!bar.includes(id)) throw new Error("the restore bar does not name " + id + ": " + bar.replace(/\s+/g, " "));
  await page.click('#restorebar button:has-text("Restore all")');
  await until("every lane back in tmux", () => want.every(hasSession), 40000, 500);
  await page.waitForSelector("#restorebar", { state: "hidden", timeout: 20000 }).catch(async () => {
    const t = await page.$eval("#restorebar", (e) => e.innerText);
    if (!/restored/.test(t)) throw new Error("after Restore all the bar says: " + t.replace(/\s+/g, " "));
  });
});

// ---- attachments -------------------------------------------------------------------
// An image made by the browser's own encoder, dropped or pasted on the terminal.
const dropFile = (kind, how) => page.evaluate(async ([kind, how]) => {
  const stem = how + "-" + kind + "-" + Date.now().toString(36); // a new name each time: the message names it
  let f;
  if (kind === "txt") f = new File(["just some notes\n"], stem + ".txt", { type: "text/plain" });
  else {
    const c = document.createElement("canvas"); c.width = 16; c.height = 16;
    const x = c.getContext("2d"); x.fillStyle = "#c33"; x.fillRect(0, 0, 16, 16); x.fillStyle = "#3c3"; x.fillRect(4, 4, 8, 8);
    const type = kind === "png" ? "image/png" : "image/jpeg";
    const blob = await new Promise((r) => c.toBlob(r, type, 0.9));
    f = new File([blob], stem + "." + (kind === "png" ? "png" : "jpg"), { type });
  }
  const dt = new DataTransfer(); dt.items.add(f);
  const t = terms[selTerm];
  if (how === "paste") {
    t.term.textarea.dispatchEvent(new ClipboardEvent("paste", { clipboardData: dt, bubbles: true, cancelable: true }));
  } else {
    t.host.dispatchEvent(new DragEvent("dragenter", { dataTransfer: dt, bubbles: true, cancelable: true }));
    t.host.dispatchEvent(new DragEvent("dragover", { dataTransfer: dt, bubbles: true, cancelable: true }));
    t.host.dispatchEvent(new DragEvent("drop", { dataTransfer: dt, bubbles: true, cancelable: true }));
  }
  return f.name;
}, [kind, how]);

async function attach(kind, how) {
  await select("idle");
  const before = typedFor("idle").length;
  const name = await dropFile(kind, how);
  await until("the drop's message", async () => (await termbar()).includes(name), 10000);
  const msg = await termbar();
  if (!msg.includes("Dropped " + name)) throw new Error("the page says: " + msg.replace(/\s+/g, " "));
  const got = await until("the path typed into the lane", () => { const s = typedFor("idle").slice(before); return /\S+\.(png|jpe?g)/i.test(s) ? s : null; }, 10000);
  const m = got.match(/(?:\x1b\[200~)?(\/[^\x1b\s]+\.(?:png|jpe?g))(?:\x1b\[201~)?( ?)/i);
  if (!m) throw new Error("no path in what was typed: " + JSON.stringify(got));
  const p = m[1];
  if (/[\r\n]/.test(got)) throw new Error("the drop pressed Enter: " + JSON.stringify(got));
  if (!got.includes("\x1b[200~")) throw new Error("the path was not sent as a bracketed paste: " + JSON.stringify(got));
  if (!fs.existsSync(p)) throw new Error("the typed path does not exist: " + p);
  const mode = fs.statSync(p).mode & 0o777;
  if (mode !== 0o600) throw new Error(p + " has mode " + mode.toString(8) + ", want 600");
  for (const root of Object.values(R.projects)) if (fs.realpathSync(p).startsWith(fs.realpathSync(root) + path.sep)) throw new Error("the file is inside a project: " + p);
  return p;
}
flow("attachments", "drop-png", () => attach("png", "drop"));
flow("attachments", "drop-jpeg", () => attach("jpeg", "drop"));
flow("attachments", "paste-image", () => attach("png", "paste"));
flow("attachments", "drop-non-image", async () => {
  await select("idle");
  const before = typedFor("idle").length;
  const name = await dropFile("txt", "drop");
  await until("the refusal", async () => (await termbar()).includes(name), 5000);
  const msg = await termbar();
  if (!msg.includes("Not dropped: " + name + " is not a PNG, JPEG, GIF or WebP image")) throw new Error("the refusal says: " + msg.replace(/\s+/g, " "));
  await page.waitForTimeout(300);
  if (typedFor("idle").length !== before) throw new Error("a refused file typed something");
});

// The page centre of a cell of the first row holding text.
const cellOf = (text, dx = 0) => page.evaluate(([text, dx]) => {
  const t = terms[selTerm].term, buf = t.buffer.active;
  for (let y = 0; y < t.rows; y++) {
    const x = buf.getLine(buf.viewportY + y).translateToString(true).indexOf(text);
    if (x < 0) continue;
    const r = t.element.querySelector(".xterm-screen").getBoundingClientRect(), c = t._core._renderService.dimensions.css.cell;
    return { x: r.left + (x + dx + 0.5) * c.width, y: r.top + (y + 0.5) * c.height };
  }
  return null;
}, [text, dx]);
flow("attachments", "selection-persists", async () => {
  await select("idle");
  await until("the lane's text", () => screenHas("Selectable words"), 10000);
  const a = await cellOf("Selectable words"), z = await cellOf("Selectable words", 16);
  if (!a) throw new Error("\"Selectable words\" is not on screen (scrolled off?)");
  await page.mouse.move(a.x, a.y);
  await page.mouse.down();
  await page.mouse.move(z.x, z.y, { steps: 8 });
  await page.mouse.up();
  const s0 = await page.evaluate(() => terms[selTerm].term.getSelection());
  await page.mouse.move(z.x + 200, z.y + 80, { steps: 12 });
  await page.waitForTimeout(400);
  const s1 = await page.evaluate(() => terms[selTerm].term.getSelection());
  if (s0 !== "Selectable words") throw new Error("the drag selected " + JSON.stringify(s0));
  if (s1 !== s0) throw new Error("the selection did not survive the pointer moving: " + JSON.stringify(s1));
});
async function modClick(p) {
  await page.mouse.move(p.x - 5, p.y);
  await page.mouse.move(p.x, p.y);
  await page.keyboard.down(MOD);
  await page.mouse.click(p.x, p.y);
  await page.keyboard.up(MOD);
}
flow("attachments", "cmd-click-url", async () => {
  await select("idle");
  const u = await cellOf("https://github.com/o/r/pull/12", 4);
  if (!u) throw new Error("the URL is not on screen");
  const none = ctx.waitForEvent("page", { timeout: 800 }).then(() => true, () => false);
  await page.mouse.click(u.x, u.y);
  if (await none) throw new Error("a plain click opened a tab");
  const pop = ctx.waitForEvent("page", { timeout: 5000 });
  await modClick(u);
  const p = await pop;
  const url = p.url() === "about:blank" ? await p.waitForURL(/github/, { timeout: 5000 }).then(() => p.url()) : p.url();
  await p.close();
  if (url !== "https://github.com/o/r/pull/12") throw new Error("⌘-click opened " + url);
});
flow("attachments", "osc8-asks", async () => {
  await select("idle");
  const d = await cellOf("Docs here", 1);
  if (!d) throw new Error("the OSC 8 link is not on screen");
  const early = ctx.waitForEvent("page", { timeout: 800 }).then(() => true, () => false);
  await modClick(d);
  if (await early) throw new Error("\"Docs here\" opened without asking");
  const bar = await termbar();
  if (!bar.includes("https://example.com/elsewhere")) throw new Error("no address shown: " + bar.replace(/\s+/g, " "));
  const pop = ctx.waitForEvent("page", { timeout: 5000 });
  await page.click('#termbar button:has-text("Open link")');
  const p = await pop;
  const url = p.url() === "about:blank" ? await p.waitForURL(/example/, { timeout: 5000 }).then(() => p.url()) : p.url();
  await p.close();
  if (url !== "https://example.com/elsewhere") throw new Error("Open link opened " + url);
});

// ---- projects ----------------------------------------------------------------------
const switchTo = async (id, name) => {
  await page.click("#projbtn");
  await page.click("#proj-" + id);
  await page.waitForFunction((n) => document.getElementById("pname").textContent === n, name, { timeout: 10000 });
  await page.waitForSelector('#tabs [role="tab"]', { timeout: 10000 });
  await page.waitForTimeout(600);
};
const selectedTab = () => page.$eval('#tabs [role="tab"][aria-selected="true"]', (e) => e.dataset.k).catch(() => null);
flow("projects", "switch-keeps-selection", async () => {
  await select("waiting");
  await switchTo("beta", "Beta");
  if (!/beta-one/.test(await selectedTab())) throw new Error("Beta opened on " + (await selectedTab()));
  if (!(await page.evaluate(() => location.search.includes("p=beta")))) throw new Error("the address does not name ?p=beta");
  await switchTo("alpha", "Alpha");
  const back = await selectedTab();
  if (back !== "tab:t:waiting") throw new Error("back in Alpha the selection is " + back + ", not waiting");
});
flow("projects", "gamma-cannot-load", async () => {
  await page.click("#projbtn");
  const g = await page.$eval("#proj-gamma", (e) => ({ text: e.innerText, disabled: e.getAttribute("aria-disabled") }));
  if (!/cannot load/.test(g.text) || g.disabled !== "true") throw new Error("gamma's row: " + JSON.stringify(g));
  await page.click("#proj-gamma", { force: true }); // aria-disabled: Playwright would wait for it to enable
  await page.waitForTimeout(500);
  if ((await page.$eval("#pname", (e) => e.textContent)) !== "Alpha") throw new Error("picking gamma switched the page");
});

(async () => {
  const t0 = Date.now();
  const browser = await pw.chromium.launch();
  try {
    ({ ctx, page, errors } = await L.openPage(browser, R));
    // Nothing leaves the machine: the links the lanes print answer from here.
    await ctx.route(/^https:\/\/(github\.com|example\.com)\//, (r) => r.fulfill({ status: 200, contentType: "text/html", body: "<title>stub</title>stub" }));
    await page.waitForSelector('[data-k="tab:t:working"]', { timeout: 20000 });
    await page.waitForTimeout(1500);
    for (const f of FLOWS) {
      if (!groups.includes(f.group) || (only && !only.includes(f.name))) continue;
      await runFlow(f);
    }
  } catch (e) {
    F.add({ area: "flows", name: "crash", rule: "harness", selector: "", detail: String(e.stack || e) });
  } finally {
    await browser.close();
  }
  const fails = F.items.filter((x) => x.rule === "flow").length;
  console.log("flows: " + F.passes.length + " passed, " + fails + " failed, " + F.items.length + " findings in " + Math.round((Date.now() - t0) / 1000) + " s → " + F.file);
})();
