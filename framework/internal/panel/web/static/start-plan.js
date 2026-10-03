"use strict";
// PANEL-29: the New lane dialog's start plan. Given the page's state and what the
// person just did, planStart says which template and name, where the lane runs, which
// "Where it runs" choices the server would accept, and what is in a build's way.
// panel.js only renders the plan. TestStartPlan runs it in node.
//
// Intents: {from: "next", template, item} (an Up next row), {from: "worktree", path}
// ("New lane here"), {from: "template", template, name, issue} (the template, name or
// issue changed), {from: "branch", template, name, issue, facts} (GET /lanes/branch's
// answer), {from: "conflict", template, name, issue, error} (a 409 body at Start).
//
// The plan: {template, name, mode, worktree, choices, warnings, note}. mode is the
// "Where it runs" choice to select, or null when the plan selects none (no template,
// or nothing can start). choices enables each of new, existing, branch and root; with
// none enabled, nothing can start. warnings are what is in the way, shown above Start.
// note is the one line that says why the plan chose where the lane runs.
(function (root) {
  const NAME = "[a-z0-9][a-z0-9-]{0,40}"; // config.LaneIDRe
  const MODES = ["new", "existing", "branch", "root"];
  // With no template, where the lane runs is the person's choice and the lane type's.
  const PLAIN = ["new", "existing", "root"];
  const esc = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

  // The panel's lane-name slug of a folder name (panel.js names a lane the same way).
  function slug(s) { return s.toLowerCase().replace(/[^a-z0-9-]+/g, "-").replace(/^-+/, "").slice(0, 41).replace(/-+$/, ""); }

  function worktrees(state) { return (state.lanes || []).concat(state.quietWorktrees || []); }

  // A build template is one whose first prompt runs /build-change.
  function isBuild(t) { return /^\s*\/build-change(\s|$)/.test(t.firstPrompt || ""); }

  // The branch a template names, as config.TemplateBranch renders it, or "" while a
  // value it needs (a valid name, an issue it uses) is missing.
  function branchOf(t, name, issue) {
    if (!new RegExp("^" + NAME + "$").test(name)) return "";
    const vals = { name, issue: (issue || "").trim() };
    let missing = false;
    const b = (t.branchPattern || "").replace(/\{([A-Za-z_]+)\}/g, (_, k) => {
      if (k in vals && !vals[k]) missing = true;
      return vals[k] || "";
    });
    return missing ? "" : b;
  }

  // The values a branch fills in a template's pattern ({name: …, issue: …}), or null
  // when the pattern does not name it or names no lane.
  function valuesOf(t, branch) {
    const keys = [];
    const re = (t.branchPattern || "").split(/(\{[A-Za-z_]+\})/).map((part) => {
      const m = /^\{([A-Za-z_]+)\}$/.exec(part);
      if (!m) return esc(part);
      keys.push(m[1]);
      return m[1] === "name" ? "(" + NAME + ")" : "([^/]+?)";
    }).join("");
    if (!keys.includes("name")) return null;
    const m = new RegExp("^" + re + "$").exec(branch);
    if (!m) return null;
    const out = {};
    keys.forEach((k, i) => { out[k] = m[i + 1]; });
    return out;
  }

  // D5: the template whose pattern names this branch. When several do, the one whose
  // Up next lists the name, else the first in the config's order.
  function templateOfBranch(state, branch) {
    if (!branch) return null;
    const hits = [];
    for (const t of state.templates || []) {
      const v = valuesOf(t, branch);
      if (v) hits.push({ t, name: v.name });
    }
    const listed = hits.find(({ t, name }) =>
      ((((state.suggestions || {})[t.id] || {}).items) || []).some((it) => it.name === name));
    return listed || hits[0] || null;
  }

  // D3: one structured fact, a build of a change whose proposal has no Approved line.
  // A change with no proposal (absent from approvals) gets no warning.
  function approvalWarnings(state, t, name) {
    if (!t || !name || !isBuild(t) || (state.approvals || {})[name] !== false) return [];
    return [name + " has no Approved line in its proposal; /build-change will stop for it"];
  }

  function plan(state, t, name, mode, worktree, on, warnings, note) {
    const choices = {};
    for (const m of MODES) choices[m] = on.includes(m);
    return {
      template: t ? t.id : "", name: name || "", mode, worktree: worktree || "", choices,
      warnings: warnings.concat(approvalWarnings(state, t, name)), note: note || "",
    };
  }

  function planStart(state, intent) {
    state = state || {};
    intent = intent || {};
    if (intent.from === "worktree") {
      // "New lane here": that worktree, with the template its branch names, if any.
      const wt = worktrees(state).find((w) => w.path === intent.path);
      const hit = templateOfBranch(state, wt && wt.branch);
      if (!hit) return plan(state, null, slug(String(intent.path || "").split("/").pop()), "existing", intent.path, PLAIN, [], "");
      return plan(state, hit.t, hit.name, "existing", intent.path, ["existing"], [], "");
    }
    let name = intent.name, issue = intent.issue;
    if (intent.from === "next") ({ name, issue } = intent.item || {});
    name = name || "";
    const t = intent.template ? (state.templates || []).find((x) => x.id === intent.template) : null;
    const refusal = intent.from === "conflict" ? String((intent.error || {}).error || "") : "";
    if (!t) return plan(state, null, name, null, "", PLAIN, [], refusal);
    const branch = branchOf(t, name, issue);
    // Where the branch is: the server's answer when it is about this branch (the name
    // may have changed while it was asked), else the worktrees the page already knows.
    let facts = intent.from === "branch" ? intent.facts : intent.from === "conflict" ? intent.error : null;
    if (!facts || !branch || facts.branch !== branch || (intent.from === "conflict" && facts.code !== "branch-exists")) facts = null;
    if (facts && facts.busy) {
      // The owner's ruling: a branch a stopped rebase or bisect holds can start nowhere
      // until it is finished there. The server's sentence is the warning.
      const why = refusal || "A branch named " + branch + " is in the middle of a " + facts.busy + " in the worktree " +
        facts.worktree + ". Finish or abort it there, then start the lane on that worktree.";
      return plan(state, t, name, null, "", [], [why], "");
    }
    let worktree = facts ? facts.worktree : "";
    if (!facts && branch) worktree = (worktrees(state).find((w) => w.branch === branch) || {}).path;
    if (worktree) {
      return plan(state, t, name, "existing", worktree, ["existing"], [],
        refusal || "The branch " + branch + " already has this worktree, so the lane runs in it.");
    }
    if (facts && (facts.local || facts.remote)) {
      return plan(state, t, name, "branch", "", ["branch"], [], refusal || "A branch named " + branch + " already exists" +
        (facts.local ? "" : " on origin") + ", with no worktree, so the lane starts on it in a new worktree, keeping its commits.");
    }
    return plan(state, t, name, "new", "", ["new"], [], refusal);
  }

  root.StartPlan = { planStart, slug };
})(window);
