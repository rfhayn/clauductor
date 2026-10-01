package state

import (
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// PANEL-20: a lane's merge readiness names every reason it is not ready, says what it
// cannot tell yet, and is ready only when each check it can make passes.
func TestReadiness(t *testing.T) {
	m := alertModel(t, "")
	lane := func(v View) LaneView {
		for _, l := range append(v.Lanes, v.QuietWorktrees...) {
			if l.Branch == "change/x" {
				return l
			}
		}
		t.Fatal("no lane on change/x")
		return LaneView{}
	}
	m.ApplyGit("/repo/w/x", signals.GitStat{Branch: "change/x", Head: "0581df54c2aa9"}, nil, t0)
	// Nothing read yet: not ready, and unknown where unknown.
	r := lane(m.Snapshot(t0)).Readiness
	if r == nil || r.Ready || !strings.Contains(strings.Join(r.Reasons, ";"), "not read yet") {
		t.Fatalf("before any read %+v", r)
	}
	m.ApplyPRs([]signals.PR{{Number: 7, HeadRef: "change/x", ChecksPass: 2, ChecksFail: 1, Review: "CHANGES_REQUESTED"}}, nil, t0)
	m.ApplyLaneExtras(map[string]LaneExtra{"/repo/w/x": {ThreadsPR: 7, Unresolved: 1, Threads: 3, Change: "changes/x", Tasks: true, TasksOpen: 2, TasksDone: 1,
		HasReceipt: true, Receipt: signals.Receipt{SHA: "deadbeef00", Clean: true}}}, true)
	r = lane(m.Snapshot(t0)).Readiness
	want := []string{"1 check(s) failed", "changes requested", "1 unresolved review thread(s)", "2 unticked task(s)", "no gate receipt for HEAD"}
	if r.Ready || strings.Join(r.Reasons, "; ") != strings.Join(want, "; ") {
		t.Fatalf("reasons %q", r.Reasons)
	}
	// All good: ready.
	m.ApplyPRs([]signals.PR{{Number: 7, HeadRef: "change/x", ChecksPass: 2, Review: "APPROVED"}}, nil, t0)
	m.ApplyLaneExtras(map[string]LaneExtra{"/repo/w/x": {ThreadsPR: 7, Threads: 3, Change: "changes/x", Tasks: true, TasksDone: 3,
		HasReceipt: true, Receipt: signals.Receipt{SHA: "0581df54c2aa9", Clean: true}}}, true)
	if r = lane(m.Snapshot(t0)).Readiness; !r.Ready || len(r.Reasons) != 0 || len(r.Rows) != 6 {
		t.Fatalf("ready %+v", r)
	}
	// A dirty receipt for HEAD is a reason; no receipt where the project keeps none is not.
	m.ApplyLaneExtras(map[string]LaneExtra{"/repo/w/x": {ThreadsPR: 7, Threads: 0, HasReceipt: true, Receipt: signals.Receipt{SHA: "0581df54c2aa9"}}}, true)
	if r = lane(m.Snapshot(t0)).Readiness; r.Ready || r.Reasons[0] != "the gate receipt is for a dirty tree" {
		t.Fatalf("dirty receipt %+v", r.Reasons)
	}
	m.ApplyLaneExtras(map[string]LaneExtra{"/repo/w/x": {ThreadsPR: 7}}, false)
	if r = lane(m.Snapshot(t0)).Readiness; !r.Ready {
		t.Fatalf("no receipts kept, none needed: %+v", r.Reasons)
	}
	// No open pull request, or a draft.
	m.ApplyPRs([]signals.PR{{Number: 8, HeadRef: "change/x", IsDraft: true}}, nil, t0)
	if r = lane(m.Snapshot(t0)).Readiness; r.Ready || r.Reasons[0] != "#8 is a draft" {
		t.Fatalf("draft %+v", r.Reasons)
	}
	m.ApplyPRs([]signals.PR{}, nil, t0)
	if r = lane(m.Snapshot(t0)).Readiness; r.Ready || r.Reasons[0] != "no open pull request" {
		t.Fatalf("no PR %+v", r.Reasons)
	}
	// The project root has nothing to merge.
	for _, l := range m.Snapshot(t0).Lanes {
		if l.Path == "/repo" && l.Readiness != nil {
			t.Fatal("the project root has a readiness box")
		}
	}
}
