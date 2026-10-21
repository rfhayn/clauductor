package web

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// stall-view.js draws a stalled lane on today's page (PANEL-51, D11): the word and the
// square of its row, tab and tree node, the row's tint, and its Needs-you row's text,
// quiet time and button. This runs the embedded file in node over the lanes and
// Needs-you items of an /api/state fixture marshalled from the state's own types, so a
// field renamed there breaks here. The harness prints only what the module returns.
const stallViewHarness = `
const vm = require("vm"), fs = require("fs");
const window = {};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window });
const V = window.StallView;
const s = JSON.parse(fs.readFileSync(process.argv[3], "utf8"));
const lanes = s.lanes.concat(s.quietWorktrees);
const lane = (name) => lanes.find((l) => l.name === name);
const base = { state: "idle", word: "idle", approx: false, title: "" };
const out = {};
for (const l of lanes) {
  out[l.name] = {
    mark: V.mark(l, base),
    tree: V.mark(l, null),
    tint: V.tint(l, ""),
    status: V.status(l, "idle"),
    needs: s.needsYou.filter((n) => n.lane === l.path).map((n) => V.need(n, lanes)),
  };
}
out.overExited = { mark: V.mark(lane("exited"), { state: "dead", word: "exited", approx: false, title: "" }), tint: V.tint(lane("exited"), "crit") };
out.approxBase = V.mark(lane("calm"), { state: "waiting", word: "needs you", approx: true, title: "t" });
out.unknownLane = V.need({ lane: "/nowhere", kind: "stalled", label: "Stalled" }, lanes);
console.log(JSON.stringify(out));
`

// The fixture's stalls, which the expected words below are built from.
const svAt = int64(1_790_000_000_000)

var (
	svSecond = state.StallView{Since: svAt, WaitedOn: "its turn ended with nothing in flight to wake it; last said: Waiting for review.", OpenWork: []string{"#7 open"}}
	svByHand = state.StallView{Since: svAt + 1000, WaitedOn: "its turn ended with nothing in flight to wake it", OpenWork: []string{"#9 open"}}
	svAsks   = state.StallView{Since: svAt + 2000, WaitedOn: "waiting on you: wants to use Bash", OpenWork: []string{"#11 open"}}
	svExited = state.StallView{Since: svAt + 3000, WaitedOn: "claude exited (status 1)", OpenWork: []string{"#13 open", "claude exited"}, Approx: true}
)

// svItemText is a stalled item's text as applyStalls words it (state/stall.go): what
// the lane last waited on, then its open work.
func svItemText(st state.StallView) string {
	return st.WaitedOn + "; open work: " + strings.Join(st.OpenWork, ", ")
}

// stallViewFixture is /api/state with five lanes: "second", stalled with #7 open and a
// terminal; "byhand", stalled, started by hand (no terminal); "asks", whose session
// waits on a permission prompt past the stall time on a reading that is not current, so
// its waiting item is labelled stalled; "exited", stalled after its claude exited,
// approximately (since a restart); and "calm", not stalled, with a waiting item.
func stallViewFixture(t *testing.T) []byte {
	t.Helper()
	wt := func(n string) string { return "/r/.claude/worktrees/" + n }
	lane := func(name, term string, st *state.StallView) state.LaneView {
		return state.LaneView{ID: name, Path: wt(name), Branch: "change/" + name, Type: "build", Name: name, Status: "idle",
			Terminal: term, Sessions: []state.SessionView{}, Subagents: []state.SubagentView{}, Stall: st}
	}
	second, byHand, asks, exited := svSecond, svByHand, svAsks, svExited
	v := state.View{
		Lanes: []state.LaneView{
			lane("second", "second", &second),
			lane("byhand", "", &byHand),
			lane("asks", "asks", &asks),
			lane("calm", "calm", nil),
		},
		QuietWorktrees: []state.LaneView{lane("exited", "exited", &exited)},
		NeedsYou: []state.NeedView{
			{Lane: wt("second"), Name: "second", Kind: state.AlertStalled, Label: "Stalled", Severity: signals.SevBlock,
				Text: svItemText(svSecond), At: svAt, Terminal: "second"},
			{Lane: wt("byhand"), Name: "byhand", Kind: state.AlertStalled, Label: "Stalled", Severity: signals.SevBlock,
				Text: svItemText(svByHand), At: svAt + 1000},
			{Lane: wt("asks"), Name: "asks", Session: "s-asks", Kind: "waiting", Label: "Stalled: Permission", Severity: signals.SevBlock,
				Text: "wants to use Bash; open work: #11 open", At: svAt + 2000, Terminal: "asks", Approx: true},
			{Lane: wt("exited"), Name: "exited", Kind: state.AlertStalled, Label: "Stalled (≈ since the panel's restart: not notified until it has watched for the stall time)",
				Severity: signals.SevBlock, Text: svItemText(svExited), At: svAt + 3000, Terminal: "exited", Approx: true},
			{Lane: wt("calm"), Name: "calm", Session: "s-calm", Kind: "waiting", Label: "Permission", Severity: signals.SevBlock,
				Text: "wants to use Bash", At: svAt, Terminal: "calm"},
		},
	}
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// [STALL-4-S1] A stalled lane reads "stalled" in its row, tab and tree node (the word
// as drawn, and the square's class), its row is tinted as a blocking row, and its
// Needs-you row is blocking, its text naming what it last waited on and its open work
// ("#7 open"), its quiet time drawn from the stall's since as "quiet for …", and Open
// terminal where the lane has a terminal: a lane started by hand only shows the lane. A
// waiting lane that stalled keeps its one row, labelled stalled, blocking, with its
// "not a current reading" note. A lane not stalled reads as it did, and an exited
// claude's stall reads stalled over "exited", approximately. What the page does with
// these (the cell, the row, the button selecting the lane) is pinned by
// TestStallViewCalledFromThePage and driven in a browser by
// testdata/browser/stalled-lane.cjs (run.sh ONLY=stalled-lane).
func TestStallView(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("-short: runs node against stall-view.js")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the stalled lane's marks are untested here")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "stall-view.js")
	fix := filepath.Join(dir, "state.json")
	h := filepath.Join(dir, "harness.js")
	for p, b := range map[string][]byte{src: []byte(readWeb(t, "stall-view.js")), fix: stallViewFixture(t), h: []byte(stallViewHarness)} {
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := exec.Command(node, h, src, fix).CombinedOutput()
	if err != nil {
		t.Fatalf("harness: %v\n%s", err, b)
	}
	type row struct {
		Sev    string `json:"sev"`
		Text   string `json:"text"`
		Since  int64  `json:"since"`
		Pre    string `json:"pre"`
		Action string `json:"action"`
	}
	type mark struct {
		State  string `json:"state"`
		Word   string `json:"word"`
		Text   string `json:"text"`
		Title  string `json:"title"`
		Approx bool   `json:"approx"`
	}
	type laneOut struct {
		Mark   *mark
		Tree   *mark
		Tint   string
		Status string
		Needs  []*row
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("harness output: %v\n%s", err, b)
	}
	lane := func(name string) laneOut {
		var l laneOut
		if err := json.Unmarshal(got[name], &l); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return l
	}
	stalled := mark{State: "stalled", Word: "stalled", Text: "stalled"}
	for _, c := range []struct {
		name string
		st   state.StallView
		mark mark
		row  row
	}{
		{"second", svSecond, stalled, row{"crit", "Stalled: " + svItemText(svSecond), svSecond.Since, "quiet for ", "Open terminal"}},
		{"byhand", svByHand, stalled, row{"crit", "Stalled: " + svItemText(svByHand), svByHand.Since, "quiet for ", "Show lane"}},
		{"asks", svAsks, stalled, row{"crit", "Stalled: Permission: wants to use Bash; open work: #11 open (≈ not a current reading)", svAsks.Since, "quiet for ", "Open terminal"}},
		{"exited", svExited, mark{State: "stalled", Word: "stalled", Text: "≈ stalled", Approx: true}, row{"crit",
			"Stalled (≈ since the panel's restart: not notified until it has watched for the stall time): " + svItemText(svExited), svExited.Since, "quiet for ", "Open terminal"}},
	} {
		l := lane(c.name)
		if l.Mark == nil || l.Tree == nil {
			t.Errorf("%s: row and tab %+v, tree node %+v, want both marked", c.name, l.Mark, l.Tree)
			continue
		}
		l.Mark.Title, l.Tree.Title = "", "" // an approximate one's why, checked below
		if *l.Mark != c.mark || *l.Tree != c.mark {
			t.Errorf("%s: row and tab %+v, tree node %+v, want %+v", c.name, *l.Mark, *l.Tree, c.mark)
		}
		if l.Tint != "crit" {
			t.Errorf("%s: row tint %q, want crit, a blocking row's", c.name, l.Tint)
		}
		if want := strings.TrimSuffix(c.mark.Text, "stalled") + "stalled: " + c.st.WaitedOn; l.Status != want {
			t.Errorf("%s: status words %q, want %q", c.name, l.Status, want)
		}
		if len(l.Needs) != 1 || l.Needs[0] == nil {
			t.Errorf("%s: Needs-you rows %+v, want one, a stall's", c.name, l.Needs)
			continue
		}
		if *l.Needs[0] != c.row {
			t.Errorf("%s: Needs-you row\n got %+v\nwant %+v", c.name, *l.Needs[0], c.row)
		}
		// What the scenario's row names, from the lane's own stall: what it last
		// waited on (a question's ask, for a waiting row) and each piece of open work.
		for _, w := range append([]string{strings.TrimPrefix(c.st.WaitedOn, "waiting on you: ")}, c.st.OpenWork...) {
			if !strings.Contains(l.Needs[0].Text, w) {
				t.Errorf("%s: Needs-you text %q does not name %q", c.name, l.Needs[0].Text, w)
			}
		}
	}
	calm := lane("calm")
	if want := (mark{State: "idle", Word: "idle", Text: "idle"}); calm.Mark == nil || *calm.Mark != want || calm.Tree != nil || calm.Tint != "" || calm.Status != "idle" {
		t.Errorf("calm, not stalled: %+v, want its own marks, no tree mark, no tint", calm)
	}
	if len(calm.Needs) != 1 || calm.Needs[0] != nil {
		t.Errorf("calm's waiting row %+v, want it drawn as a waiting row, not a stall's", calm.Needs)
	}
	if ex := lane("exited"); !strings.Contains(ex.Mark.Title, "approximate") {
		t.Errorf("exited, approximately stalled: title %q, want why it is approximate", ex.Mark.Title)
	}
	var over struct {
		Mark mark
		Tint string
	}
	if err := json.Unmarshal(got["overExited"], &over); err != nil {
		t.Fatal(err)
	}
	if over.Mark.State != "stalled" || over.Mark.Text != "≈ stalled" || over.Tint != "crit" {
		t.Errorf("a stalled lane whose claude exited: %+v, want stalled over exited", over)
	}
	var ab mark
	if err := json.Unmarshal(got["approxBase"], &ab); err != nil {
		t.Fatal(err)
	}
	if want := (mark{State: "waiting", Word: "needs you", Text: "≈ needs you", Title: "t", Approx: true}); ab != want {
		t.Errorf("a lane not stalled, approximately waiting: %+v, want %+v", ab, want)
	}
	if string(got["unknownLane"]) != "null" {
		t.Errorf("a stalled item for a lane not in the state: %s, want no stall row", got["unknownLane"])
	}
}

// panel.js puts what stall-view.js returns in place, and nothing of its own: the state
// cell (lane row and header) draws laneMark's square and text; the row's tint and
// accessible name, the tab, the tree node and the Needs-you row (its text, quiet time
// and button) take the module's; and the row's Open terminal selects the lane it names
// (jumpTo, by its terminal, else its worktree). index.html loads the module before
// panel.js, and panel.css colours its class red (D11).
func TestStallViewCalledFromThePage(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	for _, want := range []string{
		"function laneMark(x) { return StallView.mark(x.lv, { state: laneState(x), word: stateWord(x), approx: laneApprox(x), title: APPROX_TITLE }); }",
		// The state cell.
		"function stateMark(x) {\n  const m = laneMark(x);\n  const w = el(\"span\", \"state-word \" + m.state, m.text);\n  if (m.approx) w.title = m.title;\n  return el(\"span\", \"st\", null, [el(\"span\", \"sq \" + m.state), w]);\n}",
		"if (cols.includes(\"state\")) tr.appendChild(el(\"td\", null, null, [stateMark(x)]));",
		"const ab = StallView.tint(x.lv, abnormal(x))",
		"const tr = el(\"tr\", (ab ? \"abn-\" + ab : \"\") + (sel ? \" sel\" : \"\"));",
		"pressable(tr, x.name + \", \" + StallView.status(x.lv, laneStatusText(x)));",
		// The tab.
		"const kids = [el(\"span\", \"sq \" + m.state), el(\"span\", null, x.name)];",
		"tab.setAttribute(\"aria-label\", x.name + \", \" + m.text + (x.t ? \"\" : \", no terminal here\"));",
		"state: laneMark(x).state",
		// The tree node.
		"const sm = StallView.mark(w, null);",
		"const first = [sm ? el(\"span\", \"sq \" + sm.state) : null,",
		"const sw = sm ? el(\"span\", \"state-word \" + sm.state, sm.text) : null;\n    if (sw && sm.approx) sw.title = sm.title;\n    const second = [sw,",
		// Needs you.
		"const st = done ? null : StallView.need(n, S.lanes.concat(S.quietWorktrees));",
		"const sev = done ? \"\" : st ? st.sev : n.severity === \"block\" ? \"crit\" : \"warn\";",
		"askCell(st ? st.text : needWords(n) + (n.approx ? \" (≈ not a current reading)\" : \"\")),",
		"el(\"td\", \"num\", null, st ? [age(st.since, st.pre)] :",
		"[button(st ? st.action : n.terminal ? \"Open terminal\" : \"Show lane\", \"small jump\", (ev) => { ev.stopPropagation(); jumpTo(n); }, null, \"jump\")]",
		"on(tr, \"click\", () => jumpTo(n));",
		// Open terminal selects the lane.
		"const x = (n.terminal && ls.find((y) => y.key === \"t:\" + n.terminal)) || ls.find((y) => y.wt && y.wt === n.lane) || null;\n  if (!x) return;\n  selectLane(x.key);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
	page, _ := webFS.ReadFile("index.html")
	i, j := strings.Index(string(page), "/static/stall-view.js"), strings.Index(string(page), "/static/panel.js")
	if i < 0 || j < i {
		t.Error("index.html must load stall-view.js before panel.js")
	}
	css := readWeb(t, "panel.css")
	for _, want := range []string{".sq.stalled", ".state-word.stalled"} {
		if !strings.Contains(css, want) {
			t.Errorf("panel.css lacks %s: a stalled lane's square or word would not be red", want)
		}
	}
}
