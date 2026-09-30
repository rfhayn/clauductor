package web

import (
	"strings"
	"testing"
)

// PANEL-18: every lane the panel started has an actions menu in the Lanes table and in
// the Worktrees tree. An item selects the lane and opens the confirmation its button
// under the terminal opens; none acts on the click that picks it.
func TestRowActionsAreWired(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	for _, want := range []string{
		// One button in each list, keyed apart so focus finds the right one.
		`tr.appendChild(el("td", "acts", null, [actsButton(x, "ra:" + x.key)]));`,
		`mine.map((x) => actsButton(x, "rt:" + x.key))`,
		`const acts = actsButton(x, "rt:" + x.key);`,
		// The button is its own control: it does not select the row it sits in.
		"ev.stopPropagation();\n    if (rowMenu && rowMenu.btn === k)",
		`b.setAttribute("aria-haspopup", "menu");`,
		// The items, running and orphaned.
		`[["interrupt", "Interrupt (Esc)"], t.registered ? ["restart", "Restart"] : null,`,
		`S.remoteControl === "lanes" && t.registered && !t.dead ? ["remote-control", "Remote control"] : null,`,
		`["stop", "Stop lane"], ["close", "Close lane"]].filter(Boolean);`,
		// PANEL-19: Remote control types only after its confirmation, and only offers it for an idle claude.
		`idle ? button("Confirm remote control", "primary", () => { confirmAct = null; laneAction(t.id, "remote-control"); }`,
		`[["resume", "Resume"], ["forget", "Forget"], ["close", "Close lane"]]`,
		// Keyboard: the APG menu keys, and Escape back to the button.
		`else if (e.key === "Escape") { e.preventDefault(); closeRowMenu(true); }`,
		// Picking asks: Close lane through its dry-run plan, the rest through confirmAct.
		"if (act === \"close\") { askClose(x.t); return; }\n  confirmAct = { id: x.t.id, action: act };",
		`button(a === "stop" ? "Confirm stop" : a === "interrupt" ? "Confirm interrupt" : "Confirm restart", "danger",`,
		`button("Confirm resume", "primary", () => { confirmAct = null; laneAction(t.id, "resume"); }`,
		"renderRowMenu();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
	// pickRowAct never calls laneAction itself: only a confirm button does.
	i := strings.Index(js, "function pickRowAct(")
	j := strings.Index(js[i:], "\n}\n")
	if body := js[i : i+j]; strings.Contains(body, "laneAction(") || strings.Contains(body, "doClose(") {
		t.Errorf("pickRowAct acts without a confirmation:\n%s", body)
	}
	page, _ := webFS.ReadFile("index.html")
	if !strings.Contains(string(page), `<div class="menu rowmenu" id="rowmenu" role="menu" aria-label="Lane actions" hidden></div>`) {
		t.Error("index.html lacks the lane actions menu")
	}
}

// PANEL-18: a worktree with no lane offers "New lane here" (it opens the Start dialog on
// that worktree, for a new claude session there), no longer "Start lane here".
func TestNewLaneHereIsWired(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	if !strings.Contains(js, `button("New lane here", "small", () => openStart({ worktree: w.path }),`) {
		t.Error(`panel.js lacks "New lane here" on a lane-less worktree`)
	}
	if strings.Contains(js, "Start lane here") {
		t.Error(`panel.js still says "Start lane here"`)
	}
}
