package web

import (
	"strings"
	"testing"
)

// PANEL-18: a project with no card shows one line where its pinned cards would be,
// with a link to the guide's Cards section through the Help dialog's address (no new
// outside URL); a project whose cards are not pinned shows nothing there.
func TestNoCardsLineIsWired(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	for _, want := range []string{
		"if (!pins.length) return (S.cards || []).length ? null : noCards();",
		`a.href = GUIDE_URL + "#cards";`,
		`a.rel = "noopener noreferrer";`,
		`el("span", "dim", "No cards yet. Add them in .clauductor/panel.json. ")`,
		// With no lane selected, "Select a lane" still shows beside it.
		`patchInto("side", [pinnedCards().length || flow ? null : key(el("div", "empty", "Select a lane, or start one."), "side:none"), project, flow]);`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
	if n := strings.Count(js, "https://"); n != 1 {
		t.Errorf("panel.js carries %d outside URLs, want 1 (GUIDE_URL)", n)
	}
}
