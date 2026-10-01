// UX harness (UX-1): end-to-end flows with assertions, against a panel up.sh started.
// These ACT: they start, stop, close and forget lanes, remove a worktree and kill the
// run's own tmux server. Run them on a run of their own, never one matrix.cjs shares.
//
//   node flows.cjs <up.json | its JSON line> [--out dir] [--flows lanes,attachments,projects,metrics,lifecycle]
//                  [--only name,name]
//
// Groups (and their flows, in order):
//   lanes        start-new-lane, start-here, type-and-leave, interrupt, restart, stop,
//                close, remove-leftover, forget-orphan, restore-all (kills the server)
//   attachments  drop-png, drop-jpeg, paste-image, drop-non-image, selection-persists,
//                cmd-click-url, osc8-asks
//   projects     switch-keeps-selection, gamma-cannot-load
//   metrics      (UX-2, PANEL-19/20) metrics-tabs-ranges-scope, metrics-bad-payload,
//                flow-card-opens-metrics, economy-hysteresis, readiness-verdict,
//                ports-header-env, worktreeinclude-setup-teardown, remote-control-menu,
//                needs-you-signals
//   lifecycle    (UX-2; up.sh with UX_LIFECYCLE=1) auto-resume, auto-close-on-merge
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

// clickOrReport: a control a person could not click (another element covers it, or it
// is out of reach) is a finding of its own; the flow then presses it as the keyboard
// would (element.click()), so what follows it is still checked.
let current = null;
async function clickOrReport(sel, what) {
  try { await page.click(sel, { timeout: 3000 }); return; } catch (e) {
    const why = await page.$eval(sel, (b) => {
      const r = b.getBoundingClientRect(), top = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
      return "at y " + Math.round(r.top) + "–" + Math.round(r.bottom) + " in a " + innerHeight + " px window; at its centre: " + (top ? top.tagName.toLowerCase() + (top.id ? "#" + top.id : "") + (top.closest("[id]") ? " in #" + top.closest("[id]").id : "") : "nothing");
    }).catch(() => "not found");
    const file = path.join(shotDir, current.name + "-unclickable.png");
    await page.screenshot({ path: file }).catch(() => {});
    F.add({ area: "flows", name: current.name, group: current.group, rule: "unclickable", selector: sel, detail: "\"" + what + "\" cannot be clicked: " + why, screenshot: path.relative(out, file) });
    await page.$eval(sel, (b) => b.click());
  }
}

async function runFlow(f) {
  current = f;
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
  await clickOrReport('#termbar button:has-text("Confirm close")', "Confirm close");
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

// ---- PANEL-19/20 (UX-2): metrics, economy, readiness, ports, setup, remote control ----
const FAKE = path.join(R.home, "fake");
const ghFile = (f) => path.join(FAKE, "gh", f);
const readFile = (f) => { try { return fs.readFileSync(f, "utf8"); } catch (e) { return ""; } };
const mvText = () => page.$eval("#mview", (e) => e.innerText).catch(() => "");
const mvLoaded = () => page.waitForFunction(() => { const b = document.getElementById("mvbody"); return b && b.textContent && !/Reading the metrics/.test(b.textContent); }, null, { timeout: 10000 });
async function openMetricsView() {
  if (await page.$("#mview:not([hidden])")) return;
  await page.click("#metricsbtn");
  await page.waitForSelector("#mview:not([hidden])", { timeout: 3000 });
  await mvLoaded();
}
const pressed = (k) => page.$eval('[data-k="' + k + '"]', (e) => e.getAttribute("aria-pressed")).catch(() => null);
flow("metrics", "metrics-tabs-ranges-scope", async () => {
  await openMetricsView();
  // The project's command gives Flow 30d's cycle time (7.5 h) and 7d's (6.2 h).
  await page.click('[data-k="mscope:project"]');
  await mvLoaded();
  const want = { "7d": "6.2 h", "30d": "7.5 h" };
  for (const range of ["7d", "30d", "90d"]) {
    await page.click('[data-k="mrange:' + range + '"]');
    if ((await pressed("mrange:" + range)) !== "true") throw new Error("the " + range + " range button is not pressed after a click");
    for (const [tab, label] of [["flow", "Delivery"], ["cost", "Spend"], ["quality", "Review"], ["outcomes", "Did it do what it meant to"]]) {
      await page.click("#mtab-" + tab);
      const sel = await page.$eval("#mtab-" + tab, (e) => e.getAttribute("aria-selected"));
      if (sel !== "true") throw new Error("the " + tab + " tab is not selected after a click");
      const t = await mvText();
      if (!t.includes(label)) throw new Error(range + " " + tab + ": no \"" + label + "\" section: " + t.slice(0, 200).replace(/\s+/g, " "));
      if (/Cannot read the metrics/.test(t)) throw new Error(range + " " + tab + ": " + t.match(/Cannot read the metrics[^\n]*/)[0]);
      if (tab === "flow" && want[range]) {
        const row = await page.$eval('[data-k="mf:flow.cycle_time"]', (e) => e.innerText).catch(() => "");
        if (!row.includes(want[range])) throw new Error(range + ": cycle time reads " + JSON.stringify(row) + ", want " + want[range] + " (the metrics command's)");
      }
      if (tab === "flow" && range === "90d") {
        // No 90d in the fixture: the panel's own merges (6 in 30 days from the fake gh) or "—" with why, never 0.
        const row = await page.$eval('[data-k="mf:flow.merge_frequency"]', (e) => e.innerText).catch(() => "");
        if (/^\s*Merge frequency\s*0(\.0)? a week/.test(row)) throw new Error("90d merge frequency is a bare zero: " + row);
      }
    }
  }
  // All projects: the combined view names Beta's failed command.
  await page.click('[data-k="mscope:all"]');
  await mvLoaded();
  const all = await mvText();
  if (!/All projects:.*Alpha.*Beta/s.test(all)) throw new Error("All projects does not name both projects: " + all.slice(0, 300).replace(/\s+/g, " "));
  if (!/The metrics command failed in Beta/.test(all)) throw new Error("All projects does not show Beta's metrics error");
  const saved = await page.evaluate(() => localStorage.getItem("clauductor-panel-metrics"));
  if (!/"scope":"all"/.test(saved || "")) throw new Error("the scope is not kept: " + saved);
  await page.click('[data-k="mscope:project"]');
  await page.click('[data-k="mrange:30d"]');
  await page.click("#mtab-flow");
  await page.keyboard.press("Escape");
  await page.waitForSelector("#mview", { state: "hidden", timeout: 3000 });
  const back = await page.evaluate(() => document.activeElement && document.activeElement.id);
  if (back !== "metricsbtn") throw new Error("Escape did not return focus to Metrics (focus on #" + back + ")");
});
flow("metrics", "metrics-bad-payload", async () => {
  await switchTo("beta", "Beta");
  try {
    await openMetricsView();
    await page.click('[data-k="mscope:project"]');
    await mvLoaded();
    const t = await until("the metrics command's error", async () => { const s = await mvText(); return /metrics command failed/.test(s) ? s : null; }, 20000);
    if (!/windows\.30d\.flow\.change_fail_rate\.value/.test(t)) throw new Error("the error does not name the path: " + t.match(/metrics command failed[^\n]*/)[0]);
    // The panel's own figures still draw: Spend from the status line is not "—".
    await page.click("#mtab-cost");
    const cost = await page.$eval('[data-k="mf:cost.total"]', (e) => e.innerText).catch(() => "");
    if (!cost || /—/.test(cost.split("\n")[0])) throw new Error("the built-in spend did not draw beside the error: " + JSON.stringify(cost));
    await page.click("#mtab-flow");
    await page.click("#mviewclose");
  } finally { await switchTo("alpha", "Alpha"); }
});
flow("metrics", "flow-card-opens-metrics", async () => {
  await select("working");
  const fc = await page.$(".flowcard");
  if (!fc) throw new Error("no Flow card in the side panel");
  const text = await fc.innerText();
  if (!/Flow \(30d\)/.test(text) || !/Cycle time/.test(text)) throw new Error("the Flow card reads " + JSON.stringify(text));
  await page.click('[data-k="mtab:cost"]', { timeout: 300 }).catch(() => {});
  await fc.click();
  await page.waitForSelector("#mview:not([hidden])", { timeout: 3000 });
  const tab = await page.$eval('#mvbar [role="tab"][aria-selected="true"]', (e) => e.id);
  if (tab !== "mtab-flow" || (await pressed("mrange:30d")) !== "true") throw new Error("the Flow card opened Metrics on " + tab + ", range 30d pressed " + (await pressed("mrange:30d")));
  await page.click("#mviewclose");
});

// Economy: the fake's 5-hour quota is 42% (threshold 40, off below 37). Each session
// posts every 5 s, the panel decides every 5 s, so 14 s is two decisions on the new reading.
const economyFile = () => { try { return JSON.parse(readFile(path.join(R.home, ".clauductor", "panel", "economy.json"))); } catch (e) { return null; } };
const setQuota = (pct) => fs.writeFileSync(path.join(FAKE, "five_hour_pct"), String(pct));
const badge = () => page.$('[data-k="q:eco"]').then((e) => !!e);
flow("metrics", "economy-hysteresis", async () => {
  try {
    await until("economy on at 42%", async () => (await badge()) && economyFile()?.economy === true, 20000);
    const since0 = economyFile().since;
    const title = await page.$eval('[data-k="q:eco"]', (e) => e.title + " | " + e.innerText);
    if (!/scribe to sonnet, low/.test(title) || !/42% ≥ 40%/.test(title)) throw new Error("the badge says " + JSON.stringify(title));
    setQuota(38); // below the threshold, not by 3 points: stays on
    await L.sleep(14000);
    if (!(await badge()) || economyFile()?.economy !== true) throw new Error("economy went off at 38% (on at 40, off below 37)");
    if (economyFile().since !== since0) throw new Error("economy.json was rewritten without a switch");
    setQuota(36);
    await until("economy off at 36%", async () => !(await badge()) && economyFile()?.economy === false, 20000);
    if (!/36% < 37% \(on at 40%\)/.test(economyFile().reason)) throw new Error("the off reason reads " + JSON.stringify(economyFile().reason));
    setQuota(39); // below the threshold: stays off
    await L.sleep(14000);
    if ((await badge()) || economyFile()?.economy !== false) throw new Error("economy came back on at 39%");
    setQuota(41);
    await until("economy on again at 41%", async () => (await badge()) && economyFile()?.economy === true, 20000);
    const mode = fs.statSync(path.join(R.home, ".clauductor", "panel", "economy.json")).mode & 0o777;
    if (mode !== 0o600) throw new Error("economy.json has mode " + mode.toString(8));
  } finally { try { fs.unlinkSync(path.join(FAKE, "five_hour_pct")); } catch (e) { /* gone */ } }
});

// Merge readiness: ready's PR is green and approved, its tasks ticked, its receipt for
// HEAD; a failed check turns the verdict, and passing again turns it back.
const verdict = () => page.$eval('#side [data-k="ck:v"]', (e) => e.innerText).catch(() => "");
async function checksOf(lane) {
  await select(lane);
  await page.click('[data-k="stab:checks"]');
  await page.waitForSelector('#side [data-k="ck:v"], #side [data-k="ck:none"]', { timeout: 5000 });
}
const refreshUntil = async (what, fn, ms) => {
  let last = 0;
  return until(what, async () => { if (Date.now() - last > 8000) { last = Date.now(); await page.click("#refresh").catch(() => {}); } return fn(); }, ms, 500);
};
flow("metrics", "readiness-verdict", async () => {
  const f = ghFile("alpha.open.json"), orig = readFile(f);
  try {
    await checksOf("ready");
    const v0 = await refreshUntil("ready's verdict to read Ready to merge", async () => { const v = await verdict(); return /^Ready to merge/.test(v) ? v : null; }, 90000)
      .catch(async (e) => { throw new Error(e.message + " (it reads " + JSON.stringify(await verdict()) + "; rows " + JSON.stringify(await page.$eval('#side [data-k="ck:rows"]', (x) => x.innerText).catch(() => "")) + ")"); });
    const failed = orig.replace(/("headRefName":"change\/ready".*?"name":"ci","status":"COMPLETED","conclusion":)"SUCCESS"/s, '$1"FAILURE"');
    if (failed === orig) throw new Error("harness: no ci check for change/ready in alpha.open.json");
    fs.writeFileSync(f, failed);
    const v1 = await refreshUntil("a failed check to turn the verdict", async () => { const v = await verdict(); return /Not ready/.test(v) && /failed/.test(v) ? v : null; }, 30000);
    const rows = await page.$eval('#side [data-k="ck:rows"]', (x) => x.innerText);
    if (!/1 failed/.test(rows)) throw new Error("the Checks row does not count the failure: " + rows.replace(/\s+/g, " "));
    fs.writeFileSync(f, orig);
    await refreshUntil("the verdict to read Ready again", async () => /^Ready to merge/.test(await verdict()), 30000);
    // working: a pending check, a review required, an unresolved thread and no receipt.
    await checksOf("working");
    const w = await refreshUntil("working's verdict", async () => { const v = await verdict(); return /unresolved review thread/.test(v) ? v : null; }, 30000);
    for (const why of ["pending", "review is required", "no gate receipt for HEAD"]) if (!w.includes(why)) throw new Error("working's verdict lacks \"" + why + "\": " + w);
    // The project root has nothing to merge.
    await checksOf("idle");
    const none = await page.$eval('#side [data-k="ck:none"]', (e) => e.innerText).catch(() => "");
    if (!/nothing to merge/.test(none)) throw new Error("the root lane's Checks tab reads " + JSON.stringify(none));
    void v0; void v1;
  } finally { fs.writeFileSync(f, orig); await page.click('[data-k="stab:agents"]').catch(() => {}); }
});

// Ports: each lane's own, in its header, in its tmux environment and in setup's marker.
flow("metrics", "ports-header-env", async () => {
  const st = await laneState();
  const ports = (st.terminals || []).filter((t) => t.port).map((t) => [t.id, t.port]);
  const seen = new Set();
  for (const [id, p] of ports) {
    if (seen.has(p)) throw new Error("two lanes hold port " + p);
    seen.add(p);
    if (p < 39100 || (p - 39100) % 10) throw new Error(id + " has port " + p + ", not base 39100 + a multiple of per_lane 10");
  }
  await select("working");
  const port = (st.terminals || []).find((t) => t.id === "working")?.port;
  if (!port) throw new Error("working has no port in the state");
  const head = await page.$eval('#lanehead [data-k="port"]', (e) => e.innerText).catch(() => "");
  if (!head.includes(String(port))) throw new Error("the header's Port reads " + JSON.stringify(head) + ", want " + port);
  const env = tmux("show-environment", "-t", "=working", "CLAUDUCTOR_PORT").trim();
  if (env !== "CLAUDUCTOR_PORT=" + port) throw new Error("tmux has " + JSON.stringify(env));
  const mark = readFile(path.join(WT, "working", ".ux-setup")).trim();
  if (mark !== "working " + port) throw new Error("setup's marker reads " + JSON.stringify(mark));
});

// A new lane: .worktreeinclude copies the gitignored .env.local (not the tracked
// README.md), setup writes its marker before claude starts, and Close runs teardown.
flow("metrics", "worktreeinclude-setup-teardown", async () => {
  await page.click("#addlane");
  await page.waitForSelector("#startdlg:not([hidden])");
  await page.selectOption("#st-tpl", "");
  await page.selectOption("#st-type", "build");
  await page.check('input[name="st-mode"][value="new"]');
  await page.fill("#st-name", "ux-incl");
  await page.click("#st-go");
  await page.waitForSelector('[data-k="tab:t:ux-incl"]', { timeout: 30000 });
  const dir = path.join(WT, "ux-incl");
  const env = readFile(path.join(dir, ".env.local"));
  if (env !== readFile(path.join(alpha, ".env.local"))) throw new Error(".env.local was not copied: " + JSON.stringify(env));
  if (readFile(path.join(dir, "README.md")) !== git("-C", alpha, "show", "HEAD:README.md")) throw new Error("the tracked README.md differs from the branch's");
  const st = await laneState();
  const port = (st.terminals || []).find((t) => t.id === "ux-incl")?.port;
  const mark = readFile(path.join(dir, ".ux-setup")).trim();
  if (mark !== "ux-incl " + port) throw new Error("setup's marker reads " + JSON.stringify(mark) + ", want \"ux-incl " + port + "\"");
  const words = (await page.$eval("body", (e) => e.innerText)).match(/\.worktreeinclude[^\n]*/);
  if (!words || !/copied 1 file/.test(words[0])) throw new Error("the start does not say what .worktreeinclude copied: " + (words ? words[0] : "(no mention on the page)"));
  // Close: the confirmation says teardown runs first; teardown writes its marker; the
  // clean worktree goes (the branch, not merged, stays).
  await select("ux-incl");
  await until("claude in ux-incl", async () => ((await laneState()).terminals || []).find((t) => t.id === "ux-incl")?.status === "idle", 20000);
  await page.click('#termbar [data-k="b:Close lane"]');
  await page.waitForSelector('#termbar [data-k="closeask"] .closelist', { timeout: 20000 });
  const plan = await page.$eval('#termbar [data-k="closeask"]', (e) => e.innerText);
  if (!/teardown/i.test(plan)) throw new Error("the close confirmation does not mention the teardown: " + plan.replace(/\s+/g, " "));
  await clickOrReport('#termbar button:has-text("Confirm close")', "Confirm close");
  await page.waitForSelector("#closebar:not([hidden]) .closed", { timeout: 30000 });
  const td = readFile(path.join(FAKE, "teardown.log"));
  if (!/^ux-incl\b/m.test(td)) throw new Error("teardown did not run for ux-incl: " + JSON.stringify(td));
  if (fs.existsSync(dir)) throw new Error("the clean worktree is still on disk after Close: " + (await page.$eval("#closebar", (e) => e.innerText)).replace(/\s+/g, " "));
  // docs/panel.md: teardown runs with CLAUDUCTOR_LANE and CLAUDUCTOR_PORT set.
  if (!td.split("\n").includes("ux-incl " + port)) throw new Error("teardown ran without the lane's CLAUDUCTOR_PORT (" + port + "): teardown.log reads " + JSON.stringify(td));
});

// Remote control (lanes mode): lanes start with --remote-control; the ⋯ item only asks,
// and types /remote-control and Enter only after the confirmation, only into an idle lane.
flow("metrics", "remote-control-menu", async () => {
  const argv = readFile(path.join(FAKE, "argv", "idle"));
  if (!/--remote-control -n idle/.test(argv)) throw new Error("idle did not start with --remote-control before -n: " + argv);
  await select("idle");
  const rc = await page.$eval('#lanehead [data-k="remote"]', (e) => e.innerText).catch(() => "");
  if (!/on, the panel's lanes/.test(rc)) throw new Error("the header's Remote reads " + JSON.stringify(rc));
  const before = typedFor("idle").length;
  await page.locator('[data-k="ra:t:idle"]').scrollIntoViewIfNeeded();
  await page.click('[data-k="ra:t:idle"]');
  await page.waitForSelector('#rowmenu [data-act="remote-control"]', { timeout: 3000 });
  await page.click('#rowmenu [data-act="remote-control"]');
  await page.waitForSelector('#termbar [data-k="confirm"]', { timeout: 3000 });
  const words = await page.$eval('#termbar [data-k="confirm"]', (e) => e.innerText);
  if (!/Type \/remote-control and Enter/.test(words)) throw new Error("the confirmation reads " + JSON.stringify(words));
  await page.waitForTimeout(1500);
  if (typedFor("idle").slice(before).includes("/remote-control")) throw new Error("/remote-control was typed before the confirmation");
  await page.click('#termbar button:has-text("Confirm remote control")');
  await until("/remote-control and Enter in idle's typed.log", () => /\/remote-control\r/.test(typedFor("idle").slice(before)), 10000);
  // A busy lane: the confirmation says nothing will be typed, and offers no Confirm.
  const wb = typedFor("working").length;
  await page.locator('[data-k="ra:t:working"]').scrollIntoViewIfNeeded();
  await page.click('[data-k="ra:t:working"]');
  await page.click('#rowmenu [data-act="remote-control"]');
  await page.waitForSelector('#termbar [data-k="confirm"]', { timeout: 3000 });
  const bw = await page.$eval('#termbar [data-k="confirm"]', (e) => e.innerText);
  if (!/not idle/.test(bw)) throw new Error("the busy lane's confirmation reads " + JSON.stringify(bw));
  if (await page.$('#termbar button:has-text("Confirm remote control")')) throw new Error("a busy lane offers Confirm remote control");
  await page.click('#termbar [data-k="b:cancel"]');
  await page.waitForTimeout(800);
  if (typedFor("working").slice(wb).includes("/remote-control")) throw new Error("/remote-control was typed into a busy lane");
});

// The Needs-you signals from the metrics: approval, budget and stale.
flow("metrics", "needs-you-signals", async () => {
  const rows = { approval: '[data-k="alert:approval_wait:add-group-card"]', budget: '[data-k="alert:budget:budget"]', stale: '[data-k="alert:stale:quiet"]' };
  if (!R.lanes.alpha.includes("budget")) delete rows.budget;
  await refreshUntil("the approval, budget and stale rows in Needs you", async () => {
    for (const s of Object.values(rows)) if (!(await page.$(s))) return false;
    return true;
  }, 110000).catch(async (e) => {
    const miss = [];
    for (const [k, s] of Object.entries(rows)) if (!(await page.$(s))) miss.push(k);
    throw new Error(e.message + ": missing " + miss.join(", "));
  });
  const txt = await page.$eval(rows.stale, (e) => e.innerText);
  if (!/no commit on change\/quiet for 5d/.test(txt)) throw new Error("the stale row reads " + JSON.stringify(txt));
  if (rows.budget) {
    const b = await page.$eval(rows.budget, (e) => e.innerText);
    if (!/spent \$\d+\.\d\d of its \$20\.00 budget/.test(b)) throw new Error("the budget row reads " + JSON.stringify(b));
  }
  // The stale row jumps to its lane.
  await page.click(rows.stale + ' [data-k="jump"]');
  await until("quiet selected", async () => (await page.$eval('#tabs [role="tab"][aria-selected="true"]', (e) => e.dataset.k).catch(() => "")) === "tab:t:quiet", 5000);
  // The Budget bar: over (red) on budget, amber on working.
  if (rows.budget) {
    await select("budget");
    const over = await page.$eval('#lanehead [data-k="budget"]', (e) => ({ t: e.innerText, crit: !!e.querySelector(".crit") })).catch(() => null);
    if (!over || !over.crit) throw new Error("budget's Budget bar is not red: " + JSON.stringify(over));
  }
  await select("working");
  const amb = await page.$eval('#lanehead [data-k="budget"]', (e) => e.innerText).catch(() => "");
  if (!/\$3\.10 of \$3\.50/.test(amb)) throw new Error("working's Budget reads " + JSON.stringify(amb));
});

// ---- lifecycle (UX_LIFECYCLE=1) ----------------------------------------------------
const hookPost = (body) => fetch(R.base + "/hook?src=clauductor-panel", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
// Auto-resume: a usage-limit stop (StopFailure rate_limit) in an idle lane, the 5-hour
// window resetting 20 s later: 30 s after the reset the panel types "continue" + Enter.
flow("lifecycle", "auto-resume", async () => {
  if (!R.lanes.alpha.includes("limited")) throw new Error("no limited lane: run up.sh with UX_LIFECYCLE=1");
  const resets = Math.floor(Date.now() / 1000) + 20;
  fs.writeFileSync(path.join(FAKE, "five_hour_resets"), String(resets));
  try {
    await L.sleep(7000); // every session has posted the new reset
    const t = ((await laneState()).terminals || []).find((x) => x.id === "limited");
    if (!t || !t.sessionId) throw new Error("no session for limited");
    const before = typedFor("limited").length;
    const r = await hookPost({ session_id: t.sessionId, cwd: path.join(WT, "limited"), hook_event_name: "StopFailure", error_type: "rate_limit" });
    if (!r.ok && r.status !== 204) throw new Error("the hook answered " + r.status);
    await until("the rate_limit alert", async () => ((await laneState()).alerts || []).some((a) => a.kind === "rate_limit" && a.terminal === "limited"), 10000)
      .catch(() => { /* shown or not, the resume is what this checks */ });
    await L.sleep(Math.max(0, (resets + 30) * 1000 - Date.now() - 3000));
    if (typedFor("limited").slice(before).includes("continue")) throw new Error("typed before the reset + 30 s");
    const got = await until("\"continue\" and Enter in limited", () => { const s = typedFor("limited").slice(before); return /continue\r/.test(s) ? s : null; }, 30000);
    if ((got.match(/continue/g) || []).length !== 1) throw new Error("typed more than once: " + JSON.stringify(got));
    await L.sleep(8000);
    if ((typedFor("limited").slice(before).match(/continue/g) || []).length !== 1) throw new Error("typed again after the first resume");
  } finally { try { fs.unlinkSync(path.join(FAKE, "five_hour_resets")); } catch (e) { /* gone */ } }
});
// Auto-close on merge: the ship lanes' PRs leave the open list merged at their tips.
// shipclean (idle, clean) closes as Close lane would, teardown first; shipdirty (an
// untracked file) stays and asks in Needs you.
flow("lifecycle", "auto-close-on-merge", async () => {
  if (!R.lanes.alpha.includes("shipclean")) throw new Error("no ship lanes: run up.sh with UX_LIFECYCLE=1");
  // The panel looks at a lane first ~60 s after it starts (then when its PR leaves the
  // open list): wait for that first look, so the merge is seen as the PR leaving.
  await L.sleep(Math.max(0, R.started + 70000 - Date.now()));
  if (!fs.existsSync(path.join(WT, "shipclean"))) throw new Error("shipclean closed while its PR was still open");
  const f = ghFile("alpha.open.json"), orig = readFile(f);
  fs.writeFileSync(f, orig.replace(/\n,\{"number":45[^\n]*/, "").replace(/\n,\{"number":46[^\n]*/, ""));
  fs.appendFileSync(ghFile("alpha.merged"), "ship/shipclean\nship/shipdirty\n");
  await refreshUntil("shipclean to close", async () => !fs.existsSync(path.join(WT, "shipclean")) && !(await page.$('[data-k="tab:t:shipclean"]')), 150000);
  if (branchExists("ship/shipclean")) throw new Error("the branch ship/shipclean is still there");
  if (registry().includes("shipclean")) throw new Error("shipclean is still in the lane registry");
  if (!/^shipclean\b/m.test(readFile(path.join(FAKE, "teardown.log")))) throw new Error("teardown did not run for shipclean: " + readFile(path.join(FAKE, "teardown.log")));
  const ask = await until("PR merged: close lane? for shipdirty", async () => {
    const t = await page.$eval("#needs", (e) => e.innerText).catch(() => "");
    const m = t.split("\n").find((l) => /shipdirty/.test(l) && /PR merged: close lane\?/.test(l));
    return m || null;
  }, 20000);
  if (!/uncommitted|untracked|chang/i.test(ask)) throw new Error("the ask does not say why: " + ask);
  if (!fs.existsSync(path.join(WT, "shipdirty"))) throw new Error("the dirty worktree was removed");
  if (!branchExists("ship/shipdirty")) throw new Error("the dirty lane's branch was deleted");
  if (!/^shipclean \d+$/m.test(readFile(path.join(FAKE, "teardown.log")))) throw new Error("the auto-close's teardown ran without CLAUDUCTOR_PORT: teardown.log reads " + JSON.stringify(readFile(path.join(FAKE, "teardown.log"))));
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
