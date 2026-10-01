package panel

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// PANEL-21 (UX pass 1, findings 3 and 4), on real tmux and git: several lanes start
// at once and each claude reports before the panel has read its new worktree; every
// session still lands on its own lane. Then a lane is stopped while its session
// keeps posting, and it does not come back.
func TestLanesStartedAtOnceBindToTheirOwnWorktrees(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	root := signals.ResolvePath(t.TempDir())
	home := t.TempDir()
	gitRun(t, root, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"T","lanes":{"main":"orchestrator","fix/":"fix"},
		"base":"main","worktree_dir":".wt"}`)
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "init")
	p := startPanelWith(t, root, home, sock, func(o *Options) {
		// The worktree list is read only when something asks for it: no timer and
		// no watch to paper over a read skipped, and at most one early read an hour
		// through the throttle (what a burst of starts inside 2 s used to get).
		o.Ticks.Worktrees, o.Ticks.WorktreeWatch, o.Ticks.WorktreeKick = time.Hour, time.Hour, time.Hour
	})
	c := liveClient{base: p.base}
	names := []string{"la", "lb", "lc"}
	sids := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, n := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "fix", Mode: "new", Name: n})
			if code != 200 {
				t.Errorf("start %s: %d %v", n, code, body)
				return
			}
			sid := fmt.Sprint(body["lane"].(map[string]any)["sessionId"])
			mu.Lock()
			sids[n] = sid
			mu.Unlock()
			// The lane's claude reports at once, as a real one can within a second.
			cwd := filepath.Join(root, ".wt", n)
			c.post(t, "/hook", fmt.Sprintf(`{"hook_event_name":"UserPromptSubmit","session_id":%q,"cwd":%q,"prompt":"go"}`, sid, cwd))
			c.post(t, "/status", fmt.Sprintf(`{"session_id":%q,"cwd":%q,"context_window":{"used_percentage":12}}`, sid, cwd))
		}(n)
	}
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}
	sessionsOn := func(v state.View, path string) []string {
		out := []string{}
		for _, l := range append(append([]state.LaneView{}, v.Lanes...), v.QuietWorktrees...) {
			if l.Path == path {
				for _, s := range l.Sessions {
					out = append(out, s.ID)
				}
			}
		}
		return out
	}
	// Each terminal too, once the tmux poll has seen every start through (a poll
	// between a start's registry write and its tmux session shows it half-started).
	waitFor(t, "each lane's session on its own worktree, none on the root", func() bool {
		v := p.state(t)
		for _, n := range names {
			wt := filepath.Join(root, ".wt", n)
			got := sessionsOn(v, wt)
			if len(got) != 1 || got[0] != sids[n] {
				return false
			}
			if tv := findTerm(v, n); tv == nil || tv.Worktree != wt || tv.Status != "busy" {
				return false
			}
		}
		return len(sessionsOn(v, root)) == 0
	})

	// lb is stopped; its session goes on posting for a while (no SessionEnd).
	if code, body := p.post(t, "/api/lanes/lb/stop", nil); code != 200 {
		t.Fatalf("stop lb: %d %v", code, body)
	}
	lbWT := filepath.Join(root, ".wt", "lb")
	late := func() {
		c.post(t, "/status", fmt.Sprintf(`{"session_id":%q,"cwd":%q,"context_window":{"used_percentage":13}}`, sids["lb"], lbWT))
		c.post(t, "/hook", fmt.Sprintf(`{"hook_event_name":"Stop","session_id":%q,"cwd":%q}`, sids["lb"], lbWT))
	}
	shown := func(v state.View, sid string) bool {
		for _, l := range append(append([]state.LaneView{}, v.Lanes...), v.QuietWorktrees...) {
			for _, s := range l.Sessions {
				if s.ID == sid {
					return true
				}
			}
		}
		return false
	}
	waitFor(t, "the stopped lane gone", func() bool {
		late()
		v := p.state(t)
		return findTerm(v, "lb") == nil && !shown(v, sids["lb"])
	})
	for i := 0; i < 5; i++ {
		late()
		time.Sleep(100 * time.Millisecond)
	}
	v := p.state(t)
	if shown(v, sids["lb"]) {
		t.Fatalf("late posts brought the stopped lane back: %+v", v.Lanes)
	}
	for _, l := range v.Lanes {
		if l.Terminal == "" {
			t.Errorf("a lane with no terminal shows after the stop: %s (%s)", l.Path, l.Status)
		}
	}
	if v.Observe.DroppedEnded == 0 {
		t.Errorf("late posts not counted as set aside: %+v", v.Observe)
	}
}
