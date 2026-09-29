package panel

import (
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/state"
)

// C3: a body dropped because the processor is behind is counted, apart from
// foreign-cwd drops, and raises a banner.
func TestIngestOverflowIsCounted(t *testing.T) {
	s, hooks := newTestServer(t)
	for i := 0; i < cap(hooks)+3; i++ {
		if w := do(s, "POST", "/hook", `{"hook_event_name":"Stop","session_id":"x","cwd":"/repo"}`); w.Code != 204 {
			t.Fatalf("hook answered %d", w.Code)
		}
	}
	if s.Overflow() != 3 {
		t.Fatalf("overflow %d, want 3", s.Overflow())
	}
	m := state.NewModel(testConfig(t), "/repo", t0)
	m.ApplyObs(state.Obs{OverflowDrops: s.Overflow()})
	v := m.Snapshot(t0)
	if v.Dropped != 0 || v.Observe.OverflowDrops != 3 {
		t.Fatalf("overflow must be its own counter: %+v", v.Observe)
	}
	found := false
	for _, b := range v.Banners {
		found = found || strings.Contains(b, "fell behind")
	}
	if !found {
		t.Fatalf("no overflow banner: %v", v.Banners)
	}
}

// The panel only OBSERVES PermissionRequest (and PreCompact, which could block):
// /hook answers 204 with an empty body, which Claude Code reads as "no decision".
// Any body could be read as a decision, and /hook takes no token.
func TestHookNeverAnswersAPermissionRequest(t *testing.T) {
	s, _ := newTestServer(t)
	for _, ev := range []string{"PermissionRequest", "PreCompact", "StopFailure"} {
		w := do(s, "POST", "/hook", `{"hook_event_name":"`+ev+`","session_id":"x","cwd":"/repo","tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`)
		if w.Code != 204 || w.Body.Len() != 0 || w.Header().Get("Content-Type") != "" {
			t.Fatalf("%s: %d %q; must be 204 with no body", ev, w.Code, w.Body.String())
		}
	}
}
