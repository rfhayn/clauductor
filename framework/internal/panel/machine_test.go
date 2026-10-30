package panel

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pclock "github.com/rfhayn/stridal/internal/panel/clock"
	"github.com/rfhayn/stridal/internal/panel/signals"
	"github.com/rfhayn/stridal/internal/panel/state"
	"github.com/rfhayn/stridal/internal/panel/types"
	"github.com/rfhayn/stridal/internal/panel/web"
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
		notifier: state.Notifier{MinInterval: time.Minute, Project: "stridal panel"}, notifyPath: filepath.Join(t.TempDir(), "n.json")}
	for _, r := range projects {
		r.machine = m
		r.pollNotify()(context.Background(), t0)
	}
	m.pollQuotaAlert(context.Background(), t0)
	m.pollQuotaAlert(context.Background(), t0.Add(time.Second))
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 1 || !strings.HasPrefix(sent[0].Title, "stridal panel") {
		t.Fatalf("sent %+v, want one quota notification from the machine", sent)
	}
	for _, r := range projects {
		if v := viewOf(r); len(v.Alerts) == 0 {
			t.Fatalf("%s: the page no longer shows the quota alert", r.id)
		}
	}
}

// PANEL-28: a login-lock alert is the machine's, as the quota's is. A lock live for
// over 2 min notifies once, from the machine, however many projects show it; no
// project's notifier sends it; and a project added live shows it too.
func TestLoginLockAlertNotifiesOncePerMachine(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var sent []state.Notice
	notify := func(n state.Notice) error { mu.Lock(); sent = append(sent, n); mu.Unlock(); return nil }
	project := func(id string) *Runtime {
		r := dispatchProject(t, id, "/r/"+id)
		r.cfg = v2Config(t, "")
		r.o = Options{Notify: notify, Out: io.Discard}
		r.notifier = state.Notifier{MinInterval: time.Minute, Project: id}
		r.notifyPath = filepath.Join(t.TempDir(), "notifier.json")
		return r
	}
	projects := []*Runtime{project("a"), project("b")}
	m := &Machine{o: Options{Notify: notify, Out: io.Discard}, projects: projects, def: projects[0],
		notifier: state.Notifier{MinInterval: time.Minute, Project: "stridal panel"}, notifyPath: filepath.Join(t.TempDir(), "n.json")}
	lock := types.LoginLock{Which: "current", Path: "/h/.claude/.oauth_refresh.lock", State: "live",
		ModTime: t0.Add(-2 * time.Second).UnixMilli(), LiveSince: t0.Add(-3 * time.Minute).UnixMilli(), PID: 4242,
		Command: "claude -p hi", HolderState: "running", Holder: "other"}
	m.each(func(md *state.Model, _ time.Time) { md.ApplyLoginLocks([]types.LoginLock{lock}) }) // as Run's watch does
	for _, r := range projects {
		r.machine = m
		r.pollNotify()(context.Background(), t0)
	}
	mu.Lock()
	if len(sent) != 0 {
		t.Fatalf("a project's notifier sent %+v", sent)
	}
	mu.Unlock()
	m.pollQuotaAlert(context.Background(), t0)
	m.pollQuotaAlert(context.Background(), t0.Add(time.Second))
	mu.Lock()
	if len(sent) != 1 || !strings.HasPrefix(sent[0].Title, "stridal panel") || !strings.Contains(sent[0].Body, "pid 4242") {
		t.Fatalf("sent %+v, want one login-lock notification from the machine", sent)
	}
	mu.Unlock()

	added := project("c")
	m.addProject(added)
	found := false
	for _, a := range viewOf(added).Alerts {
		found = found || a.Kind == state.AlertLoginLock
	}
	if !found {
		t.Fatalf("a project added live does not show the lock alert: %+v", viewOf(added).Alerts)
	}
}

// A project added while the machine applies a reading to every project ends with the
// reading the default has: each() and addProject's copy do not interleave.
//
// The add is made to fall where it would miss: each() is held on the project it
// updates before the default, and the add is tried meanwhile. It must wait for each()
// to finish, then copy the default's new reading.
func TestAddProjectNeverMissesAMachineReading(t *testing.T) {
	t.Parallel()
	first, def, added := dispatchProject(t, "a", "/r/a"), dispatchProject(t, "b", "/r/b"), dispatchProject(t, "c", "/r/c")
	def.cfg = v2Config(t, "")
	m := &Machine{projects: []*Runtime{first, def}, def: def}
	var firstModel *state.Model
	first.hub.Read(func(md *state.Model, _ time.Time) { firstModel = md })
	release := make(chan struct{})
	lock := types.LoginLock{Which: "current", Path: "/l", State: "live", PID: 4242}
	eachDone := make(chan struct{})
	go func() {
		defer close(eachDone)
		m.each(func(md *state.Model, _ time.Time) {
			if md == firstModel {
				<-release // held on the first project; the default is not updated yet
			}
			md.ApplyLoginLocks([]types.LoginLock{lock})
		})
	}()
	waitUntil(t, "each() on the first project", 5*time.Second, func() bool {
		// The first project's hub is locked while its update waits.
		ok := make(chan struct{})
		go func() { first.hub.Read(func(*state.Model, time.Time) {}); close(ok) }()
		select {
		case <-ok:
			return false
		case <-time.After(20 * time.Millisecond):
			return true
		}
	})
	addDone := make(chan struct{})
	go func() { m.addProject(added); close(addDone) }()
	select {
	case <-addDone: // it copied the default's old reading: the update will miss it
	case <-time.After(200 * time.Millisecond): // it waits for each(), as it must
	}
	close(release)
	<-eachDone
	<-addDone
	var got []types.LoginLock
	added.hub.Read(func(md *state.Model, _ time.Time) { got = md.LoginLocks() })
	if len(got) != 1 || got[0].PID != 4242 {
		t.Fatalf("the project added while the reading was applied holds %+v, not the default's", got)
	}
}
