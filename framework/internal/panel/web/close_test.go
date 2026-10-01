package web

import (
	"strings"
	"testing"
)

// PANEL-17: Close lane asks with the server's plan (a dry run) before anything
// happens, sends back only what that plan offered to remove, and shows the result in
// the page, where it outlives the lane it closed.
func TestCloseLaneIsWired(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	for _, want := range []string{
		`"/close"), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ dryRun: true }) }`,
		`body: JSON.stringify({ worktree: !!plan.worktree, branch: !!plan.deleteBranch })`,
		`lineList("Removes", p.remove, "rm"), lineList("Keeps", p.keep, "kp")`,
		`button("Confirm close", "danger", () => doClose(t.id, p)`,
		`lineList("Removed", m.removed, "rm"), lineList("Kept", m.kept, "kp")`,
		"renderClosed();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
	// Both a running lane and an orphan offer it.
	if n := strings.Count(js, `button("Close lane", "danger", () => askClose(t)`); n != 2 {
		t.Errorf("Close lane is offered in %d places, want 2 (running, orphan)", n)
	}
	// The confirmation is only ever built from a plan: doClose is called from the one
	// confirm button, never straight from Close lane.
	if n := strings.Count(js, "doClose("); n != 2 {
		t.Errorf("doClose appears %d times, want 2 (its definition and the confirm button)", n)
	}
	page, _ := webFS.ReadFile("index.html")
	if !strings.Contains(string(page), `<div class="banners" id="closebar" role="status" hidden></div>`) {
		t.Error("index.html lacks the close result's bar")
	}
}
