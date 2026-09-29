package panel

import (
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// The reducer reconciles the lane registry with tmux, the worktree list and the
// sessions (hooks and claude agents), binding a lane to its session by session id.
// Anything that does not add up is shown as an orphan, never dropped.
func TestReducerReconcilesLanes(t *testing.T) {
	cfg, err := config.ParseConfig([]byte(`{"name":"T","lanes":{"main":"orchestrator","fix/":"fix"}}`))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(cfg, "/repo", t0)
	m.ApplyWorktrees([]signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/.wt/x", Branch: "fix/x"}}, nil, t0)
	const sidA, sidB = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	// Lane a's session reports from a subdirectory of the ROOT worktree (it ran cd):
	// its lane is still the one the registry binds its session id to.
	m.ApplyHook(signals.HookEvent{SessionID: sidA, Cwd: "/repo/sub", Event: "UserPromptSubmit", Prompt: "go"}, t0)
	recs := []LaneRecord{
		{ID: "a", SessionID: sidA, Path: "/repo/.wt/x", Type: "fix", ActionDone: true},
		{ID: "b", SessionID: sidB, Path: "/repo", Type: "orchestrator", ActionDone: true},
		{ID: "c", SessionID: "33333333-3333-4333-8333-333333333333", Path: "/repo", Type: "orchestrator", Action: "start"},
		{ID: "e", SessionID: "44444444-4444-4444-8444-444444444444", Path: "/gone/wt", Type: "fix", ActionDone: true},
	}
	tmux := []TmuxLane{
		{ID: "a", Path: "/repo/.wt/x", Type: "fix"},
		{ID: "d", Path: "/repo", Attached: 1},
		{ID: "e", Path: "/gone/wt", Dead: true, DeadStatus: "1"},
	}
	m.ApplyTmux(tmux, recs, "", nil, t0)
	v := m.Snapshot(t0)
	get := func(id string) TermLaneView {
		for _, tv := range v.Terminals {
			if tv.ID == id {
				return tv
			}
		}
		t.Fatalf("lane %s missing from %+v", id, v.Terminals)
		return TermLaneView{}
	}
	a := get("a")
	if !a.Running || !a.Registered || a.Status != "busy" || a.Orphan != "" || a.Worktree != "/repo/.wt/x" || a.Branch != "fix/x" {
		t.Errorf("a: %+v", a)
	}
	b := get("b")
	if b.Running || b.Status != "orphaned" || !strings.Contains(b.Orphan, "tmux session is gone") {
		t.Errorf("b (registered, tmux gone): %+v", b)
	}
	c := get("c")
	if c.Status != "orphaned" || c.Action != "start" || !strings.Contains(c.Orphan, `stopped during "start"`) {
		t.Errorf("c (start interrupted): %+v", c)
	}
	d := get("d")
	if !d.Running || d.Registered || d.Type != "orchestrator" || !strings.Contains(d.Orphan, "not in the lane registry") {
		t.Errorf("d (tmux only): %+v", d)
	}
	e := get("e")
	if e.Status != "dead" || e.Worktree != "" || !strings.Contains(e.Orphan, "not one of the project's worktrees") {
		t.Errorf("e (dead, worktree gone): %+v", e)
	}
	// The worktree lane shows which terminal runs in it.
	for _, l := range append(v.Lanes, v.QuietWorktrees...) {
		if l.Path == "/repo/.wt/x" && l.Terminal != "a" {
			t.Errorf("worktree x shows terminal %q, want a", l.Terminal)
		}
	}
	// A failed tmux read keeps the last good lanes and says so.
	m.ApplyTmux(nil, nil, "", errTest("tmux: boom"), t0)
	v = m.Snapshot(t0)
	if v.Sources["tmux"].OK || len(v.Terminals) != len(tmux)+2 {
		t.Fatalf("after a failed read: %+v, %d terminals", v.Sources["tmux"], len(v.Terminals))
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
