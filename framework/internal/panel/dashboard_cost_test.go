package panel

import (
	"context"
	"sync"
	"testing"
	"time"

	pclock "github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/web"
)

// PANEL-11: the dashboard's two reads (ps, git) spawn nothing while no page is in
// view, and at their cadence while one is: ps every 10 s, one `git status` per lane
// worktree every 30 s, a second git call only for a dirty tree or a moved HEAD. Since
// PANEL-18 the checkout the cards run in (the root here) is read too: two worktrees.
func TestDashboardReadsSpawnOnlyWhileAPageIsInView(t *testing.T) {
	t.Parallel()
	wt := "/repo/.claude/worktrees/build-add-feature"
	m := state.NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	m.ApplyAgents([]signals.Agent{{PID: 4101, SessionID: "s1", Cwd: wt, Status: "busy"}}, nil, t0)
	clk := pclock.NewFake(t0)
	var mu sync.Mutex
	calls := map[string]int{}
	dirty := false
	run := func(_ context.Context, _ string, argv []string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		k := argv[0]
		if argv[0] == "git" {
			k = "git " + argv[3]
		}
		calls[k]++
		switch k {
		case "ps":
			return []byte("4101 12.0 204800\n"), nil
		case "git status":
			out := "# branch.oid abc123\n# branch.head change/add-feature\n"
			if dirty {
				out += "1 .M N... 100644 100644 100644 a b x.go\n"
			}
			return []byte(out), nil
		case "git diff":
			return []byte(" 1 file changed, 4 insertions(+)\n"), nil
		case "git log":
			return []byte("1790600000\n"), nil
		}
		return nil, nil
	}
	srv := &web.Server{Clock: clk}
	r := &Runtime{hub: web.NewHub(m, clk), run: run, root: "/repo", clock: clk}
	r.srv.Store(srv)
	minute := func() map[string]int {
		mu.Lock()
		calls = map[string]int{}
		mu.Unlock()
		for s := 0; s < 60; s += 10 {
			now := t0.Add(time.Duration(s) * time.Second)
			if up, _ := r.pollProcs(context.Background(), now); up != nil {
				r.hub.Update(up)
			}
			if s%30 == 0 {
				if up, _ := r.pollGit(context.Background(), now); up != nil {
					r.hub.Update(up)
				}
			}
		}
		mu.Lock()
		defer mu.Unlock()
		out := map[string]int{}
		for k, v := range calls {
			out[k] = v
		}
		return out
	}
	if got := minute(); len(got) != 0 {
		t.Fatalf("with no page in view the dashboard spawned %v", got)
	}
	srv.MarkVisible(t0)
	got := minute()
	if got["ps"] != 6 || got["git status"] != 4 || got["git log"] != 2 || got["git diff"] != 0 {
		t.Fatalf("a clean tree in view for a minute spawned %v; want ps 6, git status 4, git log once a worktree (HEAD is new)", got)
	}
	dirty = true
	if got := minute(); got["git status"] != 4 || got["git diff"] != 4 || got["git log"] != 0 {
		t.Fatalf("a dirty tree spawned %v; want a diff stat with each status, and no log (HEAD unmoved)", got)
	}
	v := r.hub.View()
	var gv *state.GitView
	for _, l := range v.Lanes {
		if l.Path == wt {
			gv = l.Git
		}
	}
	if gv == nil || gv.Dirty != 1 || gv.Insertions != 4 || gv.LastCommitAt != 1790600000000 {
		t.Fatalf("git view %+v", gv)
	}
	srv.MarkVisible(t0.Add(-2 * time.Minute))
	if got := minute(); len(got) != 0 {
		t.Fatalf("a page out of view for 2 min still drove %v", got)
	}
}
