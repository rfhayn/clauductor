package web

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// PANEL-31: the Claude Code version check on the page. The machine decides the state
// (state/verify.go, tested there); panel.js words it. These tests build the real view
// for each state, hand its JSON to the page's own wording functions in node, and read
// what they return, so a scenario's THEN is asserted on the text the owner sees.

// versionModel is a model reading Claude Code `version`, with one lane to take hooks.
func versionModel(t *testing.T, version string) (*state.Model, func(e, agentID, tool, toolUse, out string, ms int)) {
	t.Helper()
	cwd := "/repo/.claude/worktrees/build-add-feature"
	m := state.NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	m.ApplyAgents([]signals.Agent{{SessionID: "s", Cwd: cwd, Status: "busy"}}, nil, t0)
	m.ApplyClaudeVersion(version, nil, t0)
	hook := func(e, agentID, tool, toolUse, out string, ms int) {
		h := signals.HookEvent{SessionID: "s", Cwd: cwd, Event: e, AgentID: agentID, ToolName: tool, ToolUseID: toolUse}
		if e == "SubagentStart" || e == "SubagentStop" {
			h.AgentType = "general-purpose"
		}
		if e == "PreToolUse" {
			h.ToolInput = []byte(`{"subagent_type":"general-purpose","description":"d"}`)
		}
		if out != "" {
			h.ToolResponse = []byte(out)
		}
		m.ApplyHook(h, t0.Add(time.Duration(ms)*time.Millisecond))
	}
	return m, hook
}

func vcLaunch(hook func(e, agentID, tool, toolUse, out string, ms int), id string, ms int) {
	hook("PreToolUse", "", "Agent", "tu-"+id, "", ms)
	hook("SubagentStart", id, "", "", "", ms+20)
	hook("PostToolUse", "", "Agent", "tu-"+id, `{"agentId":"`+id+`","status":"completed"}`, ms+500)
}

func vcUnmatched(hook func(e, agentID, tool, toolUse, out string, ms int), id string, ms int) {
	hook("PreToolUse", "", "Agent", "tu-"+id, "", ms)
	hook("PostToolUse", "", "Agent", "tu-"+id, `{"agentId":"`+id+`","status":"completed"}`, ms+50)
}

// pageFuncs are the pure functions of panel.js these tests run: each returns data and
// touches no DOM.
var pageFuncs = []string{"versionField", "versionDialog", "agentsApproxText", "claudeCodeFooter"}

// jsFunc returns the source of a top-level `function name(` in panel.js.
func jsFunc(t *testing.T, js, name string) string {
	t.Helper()
	i := strings.Index(js, "\nfunction "+name+"(")
	if i < 0 {
		t.Fatalf("panel.js has no function %s", name)
	}
	j := strings.Index(js[i+1:], "\n}\n")
	if j < 0 {
		t.Fatalf("panel.js: %s has no end", name)
	}
	return js[i+1 : i+1+j+2]
}

// page runs expr in node with S (the page's state, here the real view) and the
// functions above defined, and returns its JSON.
func page(t *testing.T, v state.View, expr string) json.RawMessage {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the page's wording is untested here")
	}
	js := readWeb(t, "panel.js")
	var src strings.Builder
	src.WriteString("const S = JSON.parse(process.argv[2]);\n")
	for _, f := range pageFuncs {
		src.WriteString(jsFunc(t, js, f))
	}
	src.WriteString("console.log(JSON.stringify(" + expr + "));\n")
	dir := t.TempDir()
	file := filepath.Join(dir, "run.js")
	if err := os.WriteFile(file, []byte(src.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	view := map[string]any{"verification": v.Verification, "warningItems": v.WarningItems, "observe": v.Observe}
	in, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, file, string(in)).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	return json.RawMessage(out)
}

type fieldOut struct {
	Head  string `json:"head"`
	Count string `json:"count"`
	Warn  bool   `json:"warn"`
	Tip   string `json:"tip"`
	Text  string `json:"text"`
}

func field(t *testing.T, v state.View) *fieldOut {
	t.Helper()
	var f *fieldOut
	raw := page(t, v, `(() => { const f = versionField(S.verification); return f && Object.assign({}, f, { text: f.head + f.count }); })()`)
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func dialog(t *testing.T, v state.View) (title string, paras []string) {
	t.Helper()
	raw := page(t, v, `(() => { const d = versionDialog(S.verification, S.observe.verifiedOn, S.warningItems);
  return { title: d.title, paras: d.paras.map((p) => p.map((x) => (typeof x === "string" ? x : x.b)).join("")) }; })()`)
	var d struct {
		Title string
		Paras []string
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	return d.Title, d.Paras
}

// [VERCHECK-1-S1] A new version shows the field, not a bar.
func TestVERCHECK1S1ANewVersionShowsTheFieldNotABar(t *testing.T) {
	t.Parallel()
	m, _ := versionModel(t, "2.1.288")
	v := m.Snapshot(t0)
	// No bar names the version: the view sends none for the check in progress.
	if len(v.WarningItems) != 0 {
		t.Errorf("a bar names the version: %+v", v.WarningItems)
	}

	// The field is on the status bar, beside Gate, named "Claude Code", and the page
	// draws it from the view in renderStatus.
	html := readWebPage(t)
	gate := strings.Index(html, `id="tm-gate"`)
	ver := strings.Index(html, `<button class="f fbtn" id="tm-ver" type="button" aria-haspopup="dialog" aria-expanded="false" aria-controls="verdlg" hidden><span class="k">Claude Code</span><span class="v" id="verstate"></span></button>`)
	if gate < 0 || ver < gate || ver-gate > 600 {
		t.Errorf("index.html lacks the Claude Code field right after Gate (gate at %d, field at %d)", gate, ver)
	}
	js := readWeb(t, "panel.js")
	for _, want := range []string{
		"renderVersion();\n  fitFields();",
		`fld.hidden = !f;`,
		`$("tm-ver").addEventListener("click", openVersionDlg);`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
	// The count is the point of the field, so it is a plain span in the value's own
	// ink: not `.more`, which the 140% layout and a tight bar hide and which draws
	// smaller in secondary ink. No rule of panel.css hides it either.
	body := jsFunc(t, js, "renderVersion")
	if !strings.Contains(body, `el("span", "vcount", f.count)`) || strings.Contains(body, `"more"`) {
		t.Errorf("renderVersion does not draw the count as a plain span:\n%s", body)
	}
	for _, m := range cssBlock.FindAllStringSubmatch(cssComment.ReplaceAllString(readWeb(t, "panel.css"), ""), -1) {
		if sel := m[1]; (strings.Contains(sel, "vcount") || strings.Contains(sel, "tm-ver") || strings.Contains(sel, "verstate")) && regexp.MustCompile(`display:\s*none|color:|font-size:`).MatchString(m[2]) {
			t.Errorf("%s hides or restyles the field: %s", strings.TrimSpace(sel), strings.TrimSpace(m[2]))
		}
	}

	f := field(t, v)
	if f == nil || f.Text != "2.1.288 · checking, 0 of 3" || f.Warn {
		t.Fatalf("the field: %+v", f)
	}
	if f.Tip != "Subagent lists are approximate until Claude Code 2.1.288 is checked. Click for what that means." {
		t.Errorf("the tooltip: %q", f.Tip)
	}
}

// [VERCHECK-1-S3] A verified version shows no field.
func TestVERCHECK1S3AVerifiedVersionShowsNoField(t *testing.T) {
	t.Parallel()
	for name, setup := range map[string]func() *state.Model{
		"the source's version": func() *state.Model { m, _ := versionModel(t, state.HeuristicsVerifiedOn); return m },
		"a version from verified.json": func() *state.Model {
			m, _ := versionModel(t, "2.1.288")
			m.RestoreAutoVerified("2.1.288")
			return m
		},
		"nothing read yet": func() *state.Model { return state.NewModel(testConfig(t), "/repo", t0) },
	} {
		v := setup().Snapshot(t0)
		if f := field(t, v); f != nil {
			t.Errorf("%s: the field shows: %+v", name, f)
		}
		if len(v.WarningItems) != 0 {
			t.Errorf("%s: a bar shows: %+v", name, v.WarningItems)
		}
	}
}

// [VERCHECK-1-S4] One unmatched launch costs one more confirmation: the field and the
// dialog say so.
func TestVERCHECK1S4TheDialogSaysAnUnmatchedLaunchCostsOne(t *testing.T) {
	t.Parallel()
	m, hook := versionModel(t, "2.1.288")
	vcUnmatched(hook, "O", 0)
	vcLaunch(hook, "A", 11000) // its SubagentStart ages O into an orphan
	vcLaunch(hook, "B", 12000)
	vcLaunch(hook, "C", 13000)
	v := m.Snapshot(t0.Add(14 * time.Second))
	if f := field(t, v); f == nil || f.Text != "2.1.288 · checking, 3 of 4" {
		t.Fatalf("the field: %+v", f)
	}
	title, paras := dialog(t, v)
	if title != "Claude Code 2.1.288" || len(paras) != 2 {
		t.Fatalf("dialog: %q %q", title, paras)
	}
	want := "The panel matches each subagent to the lane that started it using two details of Claude Code's hooks that aren't documented. " +
		"They were checked on 2.1.284. On 2.1.288 the panel is checking them against the hooks your own sessions send: 3 of 4 subagent launches confirmed so far. " +
		"1 launch named an agent Claude Code never announced, so it clears by itself at four instead of three. " +
		"Nothing needs a restart, and the result is kept across restarts."
	if paras[0] != want {
		t.Errorf("first paragraph:\n got %q\nwant %q", paras[0], want)
	}
	if strings.Contains(paras[0], "It clears by itself after three.") {
		t.Error("the unmatched launch's dialog keeps the sentence it replaces")
	}
	if wantP2 := "Until then, only the subagent lists and counts (a lane's Agents tab, the Agents column) are approximate. Lane states, Needs you, the quota and the costs don't depend on it."; paras[1] != wantP2 {
		t.Errorf("second paragraph:\n got %q\nwant %q", paras[1], wantP2)
	}
}

// The dialog with no unmatched launch has design.md's text as written.
func TestVersionDialogCheckingText(t *testing.T) {
	t.Parallel()
	m, hook := versionModel(t, "2.1.288")
	vcLaunch(hook, "A", 0)
	title, paras := dialog(t, m.Snapshot(t0.Add(2*time.Second)))
	if title != "Claude Code 2.1.288" || len(paras) != 2 {
		t.Fatalf("dialog: %q %q", title, paras)
	}
	want := "The panel matches each subagent to the lane that started it using two details of Claude Code's hooks that aren't documented. " +
		"They were checked on 2.1.284. On 2.1.288 the panel is checking them against the hooks your own sessions send: 1 of 3 subagent launches confirmed so far. " +
		"It clears by itself after three. Nothing needs a restart, and the result is kept across restarts."
	if paras[0] != want {
		t.Errorf("first paragraph:\n got %q\nwant %q", paras[0], want)
	}
}

// Design.md "Refusals": in the changed state the dialog shows the break bar's text
// instead of both paragraphs, and the page keeps the dialog in step with the view.
func TestVersionDialogChangedShowsTheBreak(t *testing.T) {
	t.Parallel()
	m, hook := versionModel(t, "2.1.288")
	vcUnmatched(hook, "X", 0)
	vcUnmatched(hook, "Y", 100)
	vcLaunch(hook, "A", 11000)
	v := m.Snapshot(t0.Add(12 * time.Second))
	title, paras := dialog(t, v)
	if title != "Claude Code 2.1.288" || len(paras) != 1 || len(v.WarningItems) != 1 || paras[0] != v.WarningItems[0].Text {
		t.Fatalf("the changed dialog is the break bar's text: %q %q vs %+v", title, paras, v.WarningItems)
	}
}

// The dialog is a labelled modal like Help: Escape closes it, Tab stays inside it, and
// focus returns to the field.
func TestVersionDialogIsWired(t *testing.T) {
	t.Parallel()
	html := readWebPage(t)
	for _, want := range []string{
		`<div class="modal" id="verdlg" hidden>`,
		`<div class="dlg" id="verbox" role="dialog" aria-modal="true" aria-labelledby="ver-title" tabindex="-1">`,
		`<h3 id="ver-title"></h3>`,
		`<button type="button" class="btn" id="ver-close">Close</button>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %q", want)
		}
	}
	js := readWeb(t, "panel.js")
	for _, want := range []string{
		`$("verdlg").addEventListener("keydown", (e) => trapTab(e, $("verbox"), closeVersionDlg));`,
		`$("verdlg").addEventListener("click", (e) => { if (e.target === $("verdlg")) closeVersionDlg(); });`,
		`$("ver-close").addEventListener("click", closeVersionDlg);`,
		`if (r && r.isConnected && !r.hidden && r.focus) r.focus(); else $("helpbtn").focus();`,
		// The field says whether its dialog is open, as Help's button does.
		`$("tm-ver").setAttribute("aria-expanded", "true");`,
		`$("tm-ver").setAttribute("aria-expanded", "false");`,
		// The dialog follows the view: a break switches its text, a cleared check closes it.
		`if (!$("verdlg").hidden) { if (f) renderVersionDlg(); else closeVersionDlg(); }`,
		// Help does not open over it.
		`if (!$("helpdlg").hidden || !$("verdlg").hidden) return;`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
}

// [VERCHECK-2-S1] Two unmatched launches raise the bar, and the field reads changed.
func TestVERCHECK2S1TheBreakRaisesTheBarAndTheFieldReadsChanged(t *testing.T) {
	t.Parallel()
	m, hook := versionModel(t, "2.1.288")
	vcUnmatched(hook, "X", 0)
	vcUnmatched(hook, "Y", 100)
	vcLaunch(hook, "A", 11000)
	v := m.Snapshot(t0.Add(12 * time.Second))
	if len(v.WarningItems) != 1 || !strings.Contains(v.WarningItems[0].Text, "Claude Code 2.1.288 changed what the subagent pairing relies on: ") {
		t.Fatalf("the break bar: %+v", v.WarningItems)
	}
	if f := field(t, v); f == nil || f.Text != "2.1.288 · changed" || !f.Warn {
		t.Fatalf("the field reads changed, in the warn ink: %+v", f)
	}
}

// [VERCHECK-2-S3] A read that fails after one worked raises the bar, not the field.
func TestVERCHECK2S3AFailedReadRaisesTheBarNotTheField(t *testing.T) {
	t.Parallel()
	m, _ := versionModel(t, "2.1.288")
	m.ApplyClaudeVersion("", errors.New("claude: not found"), t0.Add(time.Second))
	v := m.Snapshot(t0.Add(2 * time.Second))
	if len(v.WarningItems) != 1 || !strings.Contains(v.WarningItems[0].Text, "cannot read `claude --version` (claude: not found); subagent lists are approximate.") {
		t.Fatalf("the unread bar: %+v", v.WarningItems)
	}
	if f := field(t, v); f != nil {
		t.Fatalf("a Claude Code field shows for an unread version: %+v", f)
	}
}

// [VERCHECK-2-S4] A failed read does not hide a break: both bars show, the field reads changed.
func TestVERCHECK2S4AFailedReadDoesNotHideABreak(t *testing.T) {
	t.Parallel()
	m, hook := versionModel(t, "2.1.288")
	vcUnmatched(hook, "X", 0)
	vcUnmatched(hook, "Y", 100)
	vcLaunch(hook, "A", 15000)
	m.ApplyClaudeVersion("", errors.New("claude: not found"), t0.Add(16*time.Second))
	v := m.Snapshot(t0.Add(17 * time.Second))
	keys := map[string]bool{}
	for _, w := range v.WarningItems {
		keys[w.Key] = true
	}
	if len(v.WarningItems) != 2 || !keys["version:2.1.288:broken"] || !keys["version:unread"] {
		t.Fatalf("both bars show, the break's and the unreadable version's: %+v", v.WarningItems)
	}
	if f := field(t, v); f == nil || f.Text != "2.1.288 · changed" || !f.Warn {
		t.Fatalf("the field stays changed: %+v", f)
	}
}

// renderBanners draws every warning the view carries (so both bars show at once) and
// holds no version case of its own: the checking state has no bar because the view
// sends none.
func TestRenderBannersDrawsTheViewsWarningsAndNothingForTheCheck(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	body := jsFunc(t, js, "renderBanners")
	if !strings.Contains(body, "for (const w of warns) {") || strings.Contains(body, "verification") || strings.Contains(body, "version:") {
		t.Errorf("renderBanners treats the version check itself:\n%s", body)
	}
	if strings.Contains(js, "2 of 3 confirmed") {
		t.Error("panel.js still describes a checking warning bar")
	}
}

// The Agents tab's approximate sentence never treats the version last read as current
// when the latest read failed (the unread state's Version is empty, Unread carries the
// error), and still names a break and a count.
func TestAgentsApproxSentenceFollowsTheState(t *testing.T) {
	t.Parallel()
	sentence := func(v state.View) string {
		var s string
		if err := json.Unmarshal(page(t, v, `agentsApproxText(S.verification, S.observe)`), &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	m, hook := versionModel(t, "2.1.288")
	vcLaunch(hook, "A", 0)
	if got := sentence(m.Snapshot(t0.Add(time.Second))); got != "Approximate: agent pairing was verified on Claude Code 2.1.284, and 2.1.288 is running. Checking it against this project's hooks: 1 of 3 subagent launches confirmed." {
		t.Errorf("checking: %q", got)
	}
	m.ApplyClaudeVersion("", errors.New("claude: not found"), t0.Add(2*time.Second))
	got := sentence(m.Snapshot(t0.Add(3 * time.Second)))
	if strings.Contains(got, "2.1.288") || strings.Contains(got, "is running") || !strings.Contains(got, "cannot be read (claude: not found)") {
		t.Errorf("unread treats the last read as current: %q", got)
	}
	m2, hook2 := versionModel(t, "2.1.288")
	vcUnmatched(hook2, "X", 0)
	vcUnmatched(hook2, "Y", 100)
	vcLaunch(hook2, "A", 11000)
	if got := sentence(m2.Snapshot(t0.Add(12 * time.Second))); !strings.Contains(got, "2.1.288 is running. It changed on this version: ") {
		t.Errorf("changed: %q", got)
	}
}

// The Observe footer's Claude Code counter is worded from the view's state: never
// "verified" once the check found a break, and never the last read as current when the
// latest one failed.
func TestObserveFooterFollowsTheState(t *testing.T) {
	t.Parallel()
	footer := func(v state.View) string {
		var s string
		if err := json.Unmarshal(page(t, v, `claudeCodeFooter(S.verification, S.observe)`), &s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	m, hook := versionModel(t, "2.1.288")
	vcLaunch(hook, "A", 0)
	if got := footer(m.Snapshot(t0.Add(time.Second))); got != "2.1.288 (verified on 2.1.284; checking: 1 of 3)" {
		t.Errorf("checking: %q", got)
	}
	vcLaunch(hook, "B", 2000)
	vcLaunch(hook, "C", 4000)
	if got := footer(m.Snapshot(t0.Add(6 * time.Second))); got != "2.1.288 (verified from live hooks)" {
		t.Errorf("verified from hooks: %q", got)
	}
	if got := footer(state.NewModel(testConfig(t), "/repo", t0).Snapshot(t0)); got != "?" {
		t.Errorf("not read yet: %q", got)
	}
	m3, _ := versionModel(t, state.HeuristicsVerifiedOn)
	if got := footer(m3.Snapshot(t0)); got != "2.1.284" {
		t.Errorf("the source's version: %q", got)
	}

	// A break found by this project's own hooks, after another project's verification
	// was pushed to it: never "verified".
	m2, hook2 := versionModel(t, "2.1.288")
	vcUnmatched(hook2, "X", 0)
	vcUnmatched(hook2, "Y", 100)
	vcLaunch(hook2, "A", 11000)
	m2.ReplaceAutoVerified("2.1.288")
	got := footer(m2.Snapshot(t0.Add(12 * time.Second)))
	if strings.Contains(got, "verified") || !strings.Contains(got, "changed") {
		t.Errorf("changed: %q", got)
	}

	// Unread after a read that worked: the last read is not current.
	m4, _ := versionModel(t, "2.1.288")
	m4.ApplyClaudeVersion("", errors.New("claude: not found"), t0.Add(time.Second))
	got = footer(m4.Snapshot(t0.Add(2 * time.Second)))
	if strings.Contains(got, "verified") || strings.Contains(got, "checking") || !strings.Contains(got, "cannot read") {
		t.Errorf("unread: %q", got)
	}
}
