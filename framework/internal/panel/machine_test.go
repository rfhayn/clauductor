package panel

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pclock "github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/types"
	"github.com/clauductor/clauductor/internal/panel/web"
)

// dispatchProject is a project runtime with only what the dispatcher touches.
func dispatchProject(t *testing.T, id string, wts ...string) *Runtime {
	t.Helper()
	m := state.NewModel(v2Config(t, ""), wts[0], t0)
	var list []signals.Worktree
	for _, w := range wts {
		list = append(list, signals.Worktree{Path: w, Branch: "main"})
	}
	m.ApplyWorktrees(list, nil, t0)
	clk := pclock.Func(func() time.Time { return t0 })
	return &Runtime{id: id, hub: web.NewHub(m, clk), clock: clk, ticks: DefaultTicks(),
		kickWT: make(chan struct{}, 1), kickAgents: make(chan struct{}, 1)}
}

func viewOf(r *Runtime) state.View { return r.hub.View() }

// PANEL-16: a hook or status post goes to one project: by its session id first (a
// lane record, or a session placed before), then by the deepest worktree of any
// project holding its cwd; one that is no project's is every project's "elsewhere".
func TestDispatcherRoutes(t *testing.T) {
	t.Parallel()
	// b's repository sits inside a's checkout: the deeper worktree wins.
	a := dispatchProject(t, "a", "/r/a", "/r/a/.claude/worktrees/x")
	b := dispatchProject(t, "b", "/r/a/vendor/b")
	// a has a lane whose session is "lane-sess".
	a.hub.Update(func(m *state.Model, now time.Time) {
		m.ApplyTmux(nil, []types.LaneRecord{{ID: "lane-a", SessionID: "lane-sess", Path: "/r/a/.claude/worktrees/x", Type: "build", Mode: "existing"}}, "", nil, now)
	})
	m := &Machine{projects: []*Runtime{a, b}, def: a, bound: map[string]*Runtime{}}
	for _, c := range []struct {
		sid, cwd string
		want     *Runtime
	}{
		{"s1", "/r/a/src", a},
		{"s2", "/r/a/vendor/b/pkg", b},
		{"s3", "/r/a/.claude/worktrees/x/deep", a},
		{"lane-sess", "/r/a/vendor/b", a}, // the lane's session, wherever it is
		{"s1", "/r/a/vendor/b", a},        // sticky: a cd does not move it
		{"s4", "/elsewhere", nil},
		{"", "/r/a/vendor/b", b},
	} {
		if got := m.route(c.sid, c.cwd); got != c.want {
			t.Errorf("route(%q, %q) = %v, want %v", c.sid, c.cwd, idOf(got), idOf(c.want))
		}
	}

	// Applied: a's hook reaches a only; one from nowhere is counted by both.
	m.applyHook(signals.HookEvent{SessionID: "s1", Cwd: "/r/a/src", Event: "UserPromptSubmit"})
	m.applyHook(signals.HookEvent{SessionID: "s9", Cwd: "/elsewhere", Event: "Stop"})
	if va, vb := viewOf(a), viewOf(b); va.HookEvents != 1 || vb.HookEvents != 0 || va.Dropped != 1 || vb.Dropped != 1 {
		t.Fatalf("hooks: a %d/%d, b %d/%d (events/elsewhere)", va.HookEvents, va.Dropped, vb.HookEvents, vb.Dropped)
	}
	// The quota is the account's: a post from anywhere moves every project's, and
	// only its own project counts the post.
	five := 40.0
	m.applyStatus(signals.StatusPayload{SessionID: "s2", Cwd: "/r/a/vendor/b", RateLimits: signals.RateLimits{"five_hour": {UsedPercentage: &five}}})
	five2 := 55.0
	m.applyStatus(signals.StatusPayload{SessionID: "s8", Cwd: "/nowhere", RateLimits: signals.RateLimits{"five_hour": {UsedPercentage: &five2}}})
	for _, r := range []*Runtime{a, b} {
		if q := viewOf(r).Quota; q == nil || *q.FiveHour != 55 {
			t.Fatalf("%s: quota %+v, want 55 from the last post, whatever its project", r.id, q)
		}
	}
	if va, vb := viewOf(a), viewOf(b); va.StatusPosts != 0 || vb.StatusPosts != 1 {
		t.Fatalf("status posts: a %d, b %d", va.StatusPosts, vb.StatusPosts)
	}
}

func idOf(r *Runtime) string {
	if r == nil {
		return "<none>"
	}
	return r.id
}

// The quota is the account's, so its alert notifies once, from the machine, however
// many projects show it; each project's own notifier leaves it out.
func TestQuotaAlertNotifiesOncePerMachine(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var sent []state.Notice
	notify := func(n state.Notice) error { mu.Lock(); sent = append(sent, n); mu.Unlock(); return nil }
	full := 100.0
	var projects []*Runtime
	for _, id := range []string{"a", "b", "c"} {
		r := dispatchProject(t, id, "/r/"+id)
		r.cfg = v2Config(t, "")
		r.o = Options{Notify: notify, Out: io.Discard}
		r.notifier = state.Notifier{MinInterval: time.Minute, Project: id}
		r.notifyPath = filepath.Join(t.TempDir(), "notifier.json")
		r.hub.Update(func(m *state.Model, now time.Time) {
			m.FoldQuota(signals.StatusPayload{RateLimits: signals.RateLimits{"five_hour": {UsedPercentage: &full}}}, now)
		})
		projects = append(projects, r)
	}
	m := &Machine{o: Options{Notify: notify, Out: io.Discard}, projects: projects, def: projects[0],
		notifier: state.Notifier{MinInterval: time.Minute, Project: "clauductor panel"}, notifyPath: filepath.Join(t.TempDir(), "n.json")}
	for _, r := range projects {
		r.machine = m
		r.pollNotify()(context.Background(), t0)
	}
	m.pollQuotaAlert(context.Background(), t0)
	m.pollQuotaAlert(context.Background(), t0.Add(time.Second))
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 1 || !strings.HasPrefix(sent[0].Title, "clauductor panel") {
		t.Fatalf("sent %+v, want one quota notification from the machine", sent)
	}
	for _, r := range projects {
		if v := viewOf(r); len(v.Alerts) == 0 {
			t.Fatalf("%s: the page no longer shows the quota alert", r.id)
		}
	}
}
