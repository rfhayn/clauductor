package web

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// context-colour.js decides a lane's context bar from what the state carries per lane
// (PANEL-48, D7, D13): its warning colour from the lane's own alert threshold, not a
// hard-coded 85, and its tick from the lane's own. This runs the embedded file in node.
const contextColourHarness = `
const vm = require("vm"), fs = require("fs");
const window = {};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window });
const C = window.ContextColour;
console.log(JSON.stringify({
  s4: C.warns(82, 80),
  s4Under: C.warns(79, 80),
  s6: C.warns(80, 78.5),
  s6Under: C.warns(78, 78.5),
  s7: C.warns(80, 0),
  s7Full: C.warns(100, 0),
  noFigure: C.warns(null, 80),
  noThreshold: C.warns(90, undefined),
  tick200k: C.tick(83.5, 95),
  tickNone: C.tick(95, 95),
  tickOlder: C.tick(undefined, 95),
  rowLane: C.laneMarks({ ctxPct: 80, ctxTick: 83.5, ctxAlertPct: 78.5 }, { ctxPct: 10, ctxTick: 67, ctxAlertPct: 62 }),
  rowTerm: C.laneMarks({ ctxPct: null }, { ctxPct: 10, ctxTick: 67, ctxAlertPct: 62 }),
  rowNone: C.laneMarks(null, null),
}));
`

// [LANECTX-2-S4] Under alerts.context_pct 80, a lane with no window (threshold 80, as
// the state carries it) at 82% is drawn in the warning colour, though under 85.
// [LANECTX-2-S6] A 200000 lane's threshold of 78.5 draws 80% in the warning colour.
// [LANECTX-2-S7] A threshold of 0 (alerts.context_pct 0) draws no warning colour, at
// 80% or at 100%.
// The tick is the lane's own, and the page-wide one only where the state carries none.
func TestContextColour(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("-short: runs node against context-colour.js")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the context bar's colour is untested here")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "context-colour.js")
	if err := os.WriteFile(src, []byte(readWeb(t, "context-colour.js")), 0o644); err != nil {
		t.Fatal(err)
	}
	h := filepath.Join(dir, "harness.js")
	if err := os.WriteFile(h, []byte(contextColourHarness), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := exec.Command(node, h, src).CombinedOutput()
	if err != nil {
		t.Fatalf("harness: %v\n%s", err, b)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("harness output: %v\n%s", err, b)
	}
	want := map[string]any{
		"s4": true, "s4Under": false,
		"s6": true, "s6Under": false,
		"s7": false, "s7Full": false,
		"noFigure": false, "noThreshold": false,
		"tick200k": 83.5, "tickNone": 95.0, "tickOlder": 95.0,
		"rowLane": map[string]any{"ctx": 80.0, "tick": 83.5, "alertPct": 78.5},
		"rowTerm": map[string]any{"ctx": 10.0, "tick": 67.0, "alertPct": 62.0},
		"rowNone": map[string]any{"ctx": nil},
	}
	for k, w := range want {
		if !reflect.DeepEqual(got[k], w) {
			t.Errorf("%s: got %#v, want %#v", k, got[k], w)
		}
	}
}

// Both of the page's context bars draw from context-colour.js, with the lane's own
// tick and threshold, and none compares with a hard-coded 85 any more; index.html
// loads it before panel.js.
func TestContextBarsReadTheLanesOwnMarks(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	for _, want := range []string{
		"const cm = ContextColour.laneMarks(x.lv, x.t);",
		"tick: ContextColour.tick(cm.tick, S.trends.autocompactPct), cls: ContextColour.warns(cm.ctx, cm.alertPct)",
		"tick: ContextColour.tick(m.ctxTick, S.trends.autocompactPct), cls: ContextColour.warns(m.ctxPct, m.ctxAlertPct)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
	for _, gone := range []string{"ctx >= 85", "ctxPct >= 85", "tick: S.trends.autocompactPct"} {
		if strings.Contains(js, gone) {
			t.Errorf("panel.js still has %q", gone)
		}
	}
	page, _ := webFS.ReadFile("index.html")
	i, j := strings.Index(string(page), "/static/context-colour.js"), strings.Index(string(page), "/static/panel.js")
	if i < 0 || j < i {
		t.Error("index.html must load context-colour.js before panel.js")
	}
}
