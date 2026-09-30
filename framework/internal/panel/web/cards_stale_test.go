package web

import (
	"strings"
	"testing"
)

// PANEL-18: while the checkout the cards run in is behind its upstream, one line says
// so above the cards, in the side panel's pinned box and in Activity, as of the last
// fetch; nothing when the view has no note.
func TestCardsStaleNoteIsWired(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	for _, want := range []string{
		"const c = S.cardsStale;\n  if (!c) return null;",
		`c.branch + " is " + c.behind + " commit" + (c.behind === 1 ? "" : "s") + " behind " + c.upstream + " (as of last fetch) — cards may be stale"`,
		`kids.push(staleNote("pstale"));`,
		"if (S.cards.length) kids.push(staleNote(\"d:stale\"));\n  for (const c of S.cards) kids.push(projectCard(c));",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
}
