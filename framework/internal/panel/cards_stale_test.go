package panel

import (
	"context"
	"sync"
	"testing"

	pclock "github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/web"
)

// PANEL-18: the cards run in the project root and watch files there, so when that
// checkout is behind its upstream they show old data. The dashboard's one `git status`
// of it (no fetch) sets the view's note; none when up to date, without an upstream,
// or when a read fails, and no read at all for a project without cards.
func TestCardsStaleWhenTheirCheckoutIsBehind(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	status := ""
	var ran [][]string
	run := func(_ context.Context, _ string, argv []string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		ran = append(ran, argv)
		if argv[0] == "git" && argv[3] == "status" && argv[2] == "/repo" {
			return []byte(status), nil
		}
		return nil, nil
	}
	m := state.NewModel(testConfig(t), "/repo", t0) // its config has cards
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	clk := pclock.NewFake(t0)
	srv := &web.Server{Clock: clk}
	srv.MarkVisible(t0)
	r := &Runtime{hub: web.NewHub(m, clk), run: run, root: "/repo", clock: clk}
	r.srv.Store(srv)
	read := func(out string) *state.CardsStale {
		mu.Lock()
		status = out
		mu.Unlock()
		if up, _ := r.pollGit(context.Background(), t0); up != nil {
			r.hub.Update(up)
		}
		return r.hub.View().CardsStale
	}
	head := "# branch.oid abc123\n# branch.head main\n"
	cs := read(head + "# branch.upstream origin/main\n# branch.ab +0 -3\n")
	if cs == nil || cs.Branch != "main" || cs.Upstream != "origin/main" || cs.Behind != 3 || cs.Dir != "/repo" {
		t.Fatalf("3 behind: %+v", cs)
	}
	for name, out := range map[string]string{
		"up to date":  head + "# branch.upstream origin/main\n# branch.ab +0 -0\n",
		"only ahead":  head + "# branch.upstream origin/main\n# branch.ab +2 -0\n",
		"no upstream": head,
		"detached":    "# branch.oid abc123\n# branch.head (detached)\n",
	} {
		if cs := read(out); cs != nil {
			t.Errorf("%s: a note %+v", name, cs)
		}
	}
	// A failed read keeps no note: an unknown state is not "behind".
	read(head + "# branch.upstream origin/main\n# branch.ab +0 -1\n")
	if cs := read("fatal: not a git repository\n"); cs != nil {
		t.Errorf("a failed read kept the note: %+v", cs)
	}
	mu.Lock()
	for _, argv := range ran {
		if argv[0] == "git" && argv[3] == "fetch" {
			t.Errorf("the stale check fetched: %v", argv)
		}
	}
	mu.Unlock()

	// Without cards the root is not read for it at all.
	cfg := testConfig(t)
	cfg.Cards = nil
	m2 := state.NewModel(cfg, "/repo", t0)
	m2.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	mu.Lock()
	ran = nil
	mu.Unlock()
	r2 := &Runtime{hub: web.NewHub(m2, clk), run: run, root: "/repo", clock: clk}
	r2.srv.Store(srv)
	if up, _ := r2.pollGit(context.Background(), t0); up != nil {
		r2.hub.Update(up)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ran) != 0 || r2.hub.View().CardsStale != nil {
		t.Fatalf("a project without cards read %v", ran)
	}
}
