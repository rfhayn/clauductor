package web

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// PANEL-29: the New lane dialog renders start-plan.js's plan. Every way the dialog
// plans (opening it, "New lane here", Up next, an edit, the server's answer about the
// branch, a 409 at Start) goes through planStart, "Where it runs" follows the plan's
// choices with a template too, and Start sends where the plan put the lane.
func TestStartDialogUsesThePlan(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	for _, want := range []string{
		// The fields plan from the server's last word on the branch, if any.
		`stApply(StartPlan.planStart(S, Object.assign({ from: "template" }, stCtx, stFields())));`,
		`function stFields() { return { template: $("st-tpl").value, name: $("st-name").value.trim(), issue: $("st-issue").value.trim() }; }`,
		// "New lane here" and Up next.
		`stApply(StartPlan.planStart(S, { from: "worktree", path: opts.worktree }));`,
		`stApply(StartPlan.planStart(S, { from: "next", template: x.id, item: it }));`,
		// An edit drops the server's answer and asks again (D6).
		`function stEdited() { stCtx = null; stUpdate(); stAsk(); }`,
		`$("st-name").addEventListener("input", stEdited);`,
		`$("st-issue").addEventListener("input", stEdited);`,
		`$("st-tpl").addEventListener("change", stEdited);`,
		`if (!tpl || !StartPlan.branch(tpl, f.name, f.issue)) return;`,
		`const r = await fetch(api("/lanes/branch?" + q), { cache: "no-store" });`,
		`if (f.issue) q.set("issue", f.issue);`,
		`stCtx = { from: "branch", facts: j };`,
		// A 409 branch-exists at Start plans a template's lane again from the refusal; a
		// plain lane's refusal stays in the error line.
		`if (r.status === 409 && j.code === "branch-exists" && stTpl()) { $("st-err").textContent = ""; stCtx = { from: "conflict", error: j }; stUpdate(); return; }`,
		// An asked-about wait is dropped with the dialog.
		"function closeStart() {\n  clearTimeout(stAskTimer);",
		// "Where it runs" follows the plan's choices; the existing-branch row is a template's.
		`for (const r of document.querySelectorAll('input[name="st-mode"]')) r.disabled = !p.choices[r.value];`,
		`$("st-branch-row").hidden = !p.template;`,
		`if (p.mode) stRadio(p.mode).checked = true;`,
		// The plan's issue is kept, so a re-plan and Start render the branch it named.
		// An empty one too, so a cancelled "New lane here" leaves no issue to the next plan.
		`if (p.issue !== $("st-issue").value.trim()) $("st-issue").value = p.issue;`,
		// Start sends where the lane runs with a template too.
		`const body = tpl ? { template: tpl.id, mode: stMode(), name: $("st-name").value.trim() }`,
		`if (body.mode === "existing") body.worktree = $("st-wt").value;`,
		// Nothing can start when the plan enables no choice (a stopped rebase holds the branch).
		`$("st-go").disabled = !stCanStart();`,
		`finally { $("st-go").disabled = !stCanStart(); }`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
	// Any other refused Start drops the server's answer the plan was built on and asks
	// again: Start's own fetch has pruned a branch origin deleted since the dialog asked,
	// and a plan kept on that answer would offer only the choice just refused.
	k := strings.Index(js, `$("startform").addEventListener("submit"`)
	if k < 0 {
		t.Fatal("panel.js has no Start submit handler")
	}
	sub := js[k:]
	if i, j := strings.Index(sub, "if (!r.ok) {"), strings.Index(sub, "closeStart();"); i < 0 || j < i ||
		!strings.Contains(sub[i:j], `$("st-err").textContent = j.error || "HTTP " + r.status;`) ||
		!strings.Contains(sub[i:j], `if (!["quota", "untrusted-config", "api-key"].includes(j.code)) { stCtx = null; stUpdate(); stAsk(); }`) {
		t.Error("a refused Start does not show its error and plan again from the fields with a fresh answer, or re-plans on a refusal that says nothing about the branch")
	}
	// Those codes are the ones Start refuses with before any git: a renamed code would
	// silently re-plan again.
	src, err := os.ReadFile(filepath.Join("..", "lanes", "lanes.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"quota", "untrusted-config", "api-key"} {
		if !strings.Contains(string(src), `laneErr(409, "`+c+`"`) {
			t.Errorf("lanes.go no longer refuses a start with code %q; the dialog keeps its plan on it", c)
		}
	}
	// The forced "new" and the disabled radios with a template are gone.
	body := funcBody(t, js, "stUpdate")
	for _, bad := range []string{`r.disabled = !!tpl`, `[value="new"]').checked = true`} {
		if strings.Contains(body, bad) {
			t.Errorf("stUpdate still has %q: a template no longer forces a new branch", bad)
		}
	}
	// panel.js uses start-plan.js's slug, not a copy of its own.
	if strings.Contains(js, "function slug(") || !strings.Contains(js, "const slug = StartPlan.slug;") {
		t.Error("panel.js keeps its own slug; use StartPlan.slug")
	}

	page := readWebPage(t)
	i, j := strings.Index(page, `<script src="/static/start-plan.js"></script>`), strings.Index(page, `<script src="/static/panel.js"></script>`)
	if i < 0 || j < 0 || i > j {
		t.Error("index.html must load start-plan.js before panel.js")
	}
	for _, want := range []string{
		`<label class="inline" id="st-branch-row" hidden><input type="radio" name="st-mode" value="branch"> Its existing branch, in a new worktree</label>`,
		`<div class="sub" id="st-note" hidden></div>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("index.html lacks %q", want)
		}
	}
	if w, g := strings.Index(page, `id="st-warn"`), strings.Index(page, `id="st-go"`); w < 0 || w > g {
		t.Error("index.html must put the warnings (st-warn) above Start")
	}
}

// startDialogHarness evaluates the dialog's own words for Start (cut from panel.js) on
// the plans start-plan.js makes for a build of add-score-photo, with and without an
// Approved line.
const startDialogHarness = `
const vm = require("vm"), fs = require("fs");
const window = {};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window });
const S = {}, offline = () => false;
let stPlan = null;
%ANYCHOICE%
%CANSTART%
const label = () => (%LABEL%);
const state = (approved, terminal) => ({
  templates: [{ id: "build", laneType: "build", branchPattern: "change/{name}", firstPrompt: '/build-change {"change": "{name}"}' }],
  lanes: [{ path: "/repo/w/add-score-photo", branch: "change/add-score-photo", terminal }],
  approvals: { "add-score-photo": approved },
});
const out = {};
for (const [k, s] of [["unapproved", state(false)], ["approved", state(true)], ["running", state(false, "score")]]) {
  stPlan = window.StartPlan.planStart(s, { from: "template", template: "build", name: "add-score-photo" });
  out[k] = { warnings: stPlan.warnings, label: label(), enabled: stCanStart() };
}
console.log(JSON.stringify(out));
`

func TestStartDialogStartButton(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	line := func(re string) string {
		t.Helper()
		m := regexp.MustCompile(re).FindStringSubmatch(js)
		if m == nil {
			t.Fatalf("panel.js lacks %s", re)
		}
		return m[len(m)-1]
	}
	h := strings.NewReplacer(
		"%ANYCHOICE%", line(`(?m)^function anyChoice\(p\) \{.*\}$`),
		"%CANSTART%", line(`(?m)^function stCanStart\(\) \{.*\}$`),
		"%LABEL%", line(`(?m)^  setText\(\$\("st-go"\), (.*)\);$`),
	).Replace(startDialogHarness)
	var got map[string]struct {
		Warnings []string `json:"warnings"`
		Label    string   `json:"label"`
		Enabled  bool     `json:"enabled"`
	}
	b := runPlanHarness(t, h)
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("harness output: %v\n%s", err, b)
	}
	// [NEWLANE-4-S1] A build of add-score-photo, whose proposal has no Approved line: the
	// plan warns that /build-change will stop for it, and Start reads "Start anyway" and
	// stays enabled.
	u := got["unapproved"]
	if len(u.Warnings) != 1 || !strings.Contains(u.Warnings[0], "no Approved line") || !strings.Contains(u.Warnings[0], "/build-change will stop for it") {
		t.Errorf("[NEWLANE-4-S1] warnings %q, want the approval warning", u.Warnings)
	}
	if u.Label != "Start anyway" || !u.Enabled {
		t.Errorf("[NEWLANE-4-S1] Start reads %q, enabled %v; want \"Start anyway\", enabled", u.Label, u.Enabled)
	}
	// [NEWLANE-4-S2] With an Approved line: no approval warning, and Start reads "Start lane".
	a := got["approved"]
	if len(a.Warnings) != 0 {
		t.Errorf("[NEWLANE-4-S2] warnings %q, want none", a.Warnings)
	}
	if a.Label != "Start lane" || !a.Enabled {
		t.Errorf("[NEWLANE-4-S2] Start reads %q, enabled %v; want \"Start lane\", enabled", a.Label, a.Enabled)
	}
	// A plan that allows no start (a lane already runs in the branch's worktree) shows
	// its warnings but does not offer "Start anyway": Start reads "Start lane", disabled.
	if r := got["running"]; len(r.Warnings) == 0 || r.Label != "Start lane" || r.Enabled {
		t.Errorf("running: warnings %q, Start reads %q, enabled %v; want warnings, \"Start lane\", disabled", r.Warnings, r.Label, r.Enabled)
	}
	// The warnings are rendered above Start, as text, each its own line.
	if !strings.Contains(js, `patch($("st-warn"), stPlan.warnings.map((w, i) => key(el("div", "warnbar", w), "sw:" + i)));`) {
		t.Error("panel.js does not render the plan's warnings into st-warn")
	}
}

// [NEWLANE-5-S1] The New lane dialog's template list opens with its empty choice
// reading "No template: a plain Claude session".
func TestStartDialogNamesThePlainLane(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	for _, want := range []string{
		`const none = el("option", null, "No template: a plain Claude session"); none.value = "";`,
		`tsel.replaceChildren(none, ...(S.templates || [])`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("[NEWLANE-5-S1] panel.js lacks %q", want)
		}
	}
	if strings.Contains(js, "(none: an empty lane)") {
		t.Error("[NEWLANE-5-S1] panel.js still names the empty option \"(none: an empty lane)\"")
	}
}

// funcBody is a top-level function's source in js, from its declaration to the first
// line that closes it.
func funcBody(t *testing.T, js, name string) string {
	t.Helper()
	i := strings.Index(js, "function "+name+"(")
	if i < 0 {
		t.Fatalf("panel.js has no function %s", name)
	}
	j := strings.Index(js[i:], "\n}\n")
	if j < 0 {
		t.Fatalf("panel.js: function %s does not close", name)
	}
	return js[i : i+j]
}

func readWebPage(t *testing.T) string {
	t.Helper()
	b, err := webFS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
