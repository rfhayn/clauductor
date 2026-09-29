package panel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// PANEL-7: what the panel spawns while it idles. Each cadence rule is pinned with an
// injected clock and a fake runner that counts the calls it would have spawned.

// fakeTmux answers the tmux argv the LaneManager builds and counts each command.
type fakeTmux struct {
	mu     sync.Mutex
	calls  map[string]int
	lanes  []string // session names list-panes reports; nil with down means no server
	down   bool
	envOut string
}

func (f *fakeTmux) exec(_ context.Context, argv []string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// argv: -L <socket> -f /dev/null <command> ...
	cmd := argv[4]
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[cmd]++
	if f.down {
		return nil, errors.New("tmux: no server running on /tmp/tmux-501/test")
	}
	switch cmd {
	case "list-panes":
		var b strings.Builder
		for _, id := range f.lanes {
			fmt.Fprintf(&b, "%s\t/tmp\t/tmp\t0\t\t1700000000\t0\tbuild\n", id)
		}
		return []byte(b.String()), nil
	case "show-environment":
		return []byte(f.envOut), nil
	}
	return nil, nil
}

func (f *fakeTmux) count(cmd string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[cmd]
}

func tmuxPollerFor(t *testing.T, f *fakeTmux, clock *time.Time) *tmuxPoller {
	t.Helper()
	reg, err := OpenRegistry(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lm := &LaneManager{TmuxPath: "tmux", Socket: "test", Registry: reg, Exec: f.exec,
		LookupEnv: func(string) (string, bool) { return "", false }}
	return newTmuxPoller(lm, "", func() time.Time { return *clock })
}

// run ticks the poller for d of simulated time, stepping by the interval it asks
// for, and returns the number of ticks and the intervals it asked for.
func runTicks(p *tmuxPoller, clock *time.Time, d time.Duration) (ticks int, intervals map[time.Duration]int) {
	intervals = map[time.Duration]int{}
	end := clock.Add(d)
	for clock.Before(end) {
		_, next := p.tick(context.Background())
		ticks++
		intervals[next]++
		*clock = clock.Add(next)
	}
	return ticks, intervals
}

func TestTmuxEnvAndHardenRunOnLaneSetChangeOrEvery30s(t *testing.T) {
	clock := t0
	f := &fakeTmux{lanes: []string{"lane-a", "lane-b"}}
	p := tmuxPollerFor(t, f, &clock)
	ticks, iv := runTicks(p, &clock, time.Minute)
	// With lanes: list-panes (one call for every lane) every 2 s; the environment
	// check and Harden at the first tick and every 30 s after, not every tick.
	if ticks != 30 || iv[tmuxFast] != 30 {
		t.Fatalf("with lanes: %d ticks, intervals %v; want 30 at %v", ticks, iv, tmuxFast)
	}
	if n := f.count("list-panes"); n != 30 {
		t.Fatalf("list-panes %d times in a minute, want 30 (one per tick, never one per lane)", n)
	}
	if env, h := f.count("show-environment"), f.count("set-option"); env != 2 || h != 2 {
		t.Fatalf("in a minute with an unchanged lane set: show-environment %d, Harden %d; want 2 each", env, h)
	}
	p.tick(context.Background()) // t = 60 s: the regular check
	clock = clock.Add(tmuxFast)
	// A lane appears: checked at once, not at the next 30 s.
	f.mu.Lock()
	f.lanes = append(f.lanes, "lane-c")
	f.mu.Unlock()
	p.tick(context.Background())
	if env, h := f.count("show-environment"), f.count("set-option"); env != 4 || h != 4 {
		t.Fatalf("a new lane did not trigger the check: show-environment %d, Harden %d", env, h)
	}
}

func TestTmuxIdleSocketSpawnsOnlyListPanesEvery10s(t *testing.T) {
	clock := t0
	f := &fakeTmux{down: true}
	p := tmuxPollerFor(t, f, &clock)
	ticks, iv := runTicks(p, &clock, time.Minute)
	if ticks != 6 || iv[tmuxIdle] != 6 {
		t.Fatalf("no server: %d ticks, intervals %v; want 6 at %v", ticks, iv, tmuxIdle)
	}
	if f.count("show-environment") != 0 || f.count("set-option") != 0 {
		t.Fatalf("no server, yet show-environment %d, Harden %d", f.count("show-environment"), f.count("set-option"))
	}
	// A server with no lane yet: idle cadence, one environment check per 30 s.
	clock = t0
	f2 := &fakeTmux{}
	p2 := tmuxPollerFor(t, f2, &clock)
	runTicks(p2, &clock, time.Minute)
	if f2.count("list-panes") != 6 || f2.count("show-environment") != 2 || f2.count("set-option") != 0 {
		t.Fatalf("empty server: %v", f2.calls)
	}
}

func TestTmuxEnvBlockStillReported(t *testing.T) {
	clock := t0
	f := &fakeTmux{lanes: []string{"lane-a"}, envOut: "ANTHROPIC_API_KEY=x\n"}
	p := tmuxPollerFor(t, f, &clock)
	for i := 0; i < 5; i++ { // cached between checks, not forgotten
		update, _ := p.tick(context.Background())
		m := NewModel(v2Config(t, ""), "/p", clock)
		update(m, clock)
		if !strings.Contains(m.startBlocked, "ANTHROPIC_API_KEY") {
			t.Fatalf("tick %d: start not blocked by the key in tmux's environment: %q", i, m.startBlocked)
		}
		clock = clock.Add(tmuxFast)
	}
}

// ---- pid start times ----

func TestProcCacheReadsAStartTimeOncePerProcess(t *testing.T) {
	clock := t0
	alive := map[int]bool{100: true}
	starts := map[int]string{100: "Mon Sep 28 10:00:00 2026"}
	var aliveCalls, startCalls int
	c := &ProcCache{
		Alive: func(pid int) bool { aliveCalls++; return alive[pid] },
		Start: func(pid int) string { startCalls++; return starts[pid] },
		Now:   func() time.Time { return clock },
	}
	for i := 0; i < 60; i++ {
		if ok, s := c.Check(100); !ok || s != starts[100] {
			t.Fatalf("check %d: %v %q", i, ok, s)
		}
		clock = clock.Add(time.Second)
	}
	// Read when new, and again every 30 s (t = 0 and t = 30 s).
	if startCalls != 2 || aliveCalls != 60 {
		t.Fatalf("a minute of 1 s checks: %d start reads (ps), %d kill(0) checks; want 2 and 60", startCalls, aliveCalls)
	}
	// The process exits and the pid is reused: the start time is read again.
	alive[100] = false
	if ok, _ := c.Check(100); ok {
		t.Fatal("a gone pid reads as alive")
	}
	alive[100], starts[100] = true, "Mon Sep 28 11:00:00 2026"
	if _, s := c.Check(100); s != starts[100] || startCalls != 3 {
		t.Fatalf("a reused pid kept the old start time %q (%d reads)", s, startCalls)
	}
	// A pid no longer asked about is dropped after the TTL, so the map stays small.
	clock = clock.Add(2 * time.Minute)
	c.Check(200)
	if len(c.m) != 0 {
		t.Fatalf("stale entries kept: %v", c.m)
	}
}

// A pid that dies and is reused between two checks is never seen gone; the 30 s
// re-read still catches the new start time.
func TestProcCacheRereadsAStartTimeEvery30s(t *testing.T) {
	clock := t0
	start := "Mon Sep 28 10:00:00 2026"
	c := &ProcCache{Alive: func(int) bool { return true }, Start: func(int) string { return start }, Now: func() time.Time { return clock }}
	c.Check(300)
	start = "Mon Sep 28 12:00:00 2026" // reused unseen
	clock = clock.Add(29 * time.Second)
	if _, s := c.Check(300); s == start {
		t.Fatal("re-read before 30 s")
	}
	clock = clock.Add(time.Second)
	if _, s := c.Check(300); s != start {
		t.Fatalf("after 30 s the cache still says %q", s)
	}
}

// A hook that arrives while the agents loop sleeps its quiet 15 s polls at once.
func TestHookEndsTheQuietAgentsInterval(t *testing.T) {
	var mu sync.Mutex
	polls := 0
	run := func(_ context.Context, _ string, argv []string) ([]byte, error) {
		if len(argv) >= 3 && argv[1] == "agents" {
			mu.Lock()
			polls++
			mu.Unlock()
		}
		return []byte("[]"), nil
	}
	count := func() int { mu.Lock(); defer mu.Unlock(); return polls }
	m := NewModel(v2Config(t, ""), "/p", time.Now())
	hub := NewHub(m, time.Now)
	x := &runtimeV2{hub: hub, root: "/p", p: &pollers{hub: hub, run: run, kickAgents: make(chan struct{}, 1)}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go x.agentsLoop(ctx)
	waitFor(t, "the first poll, then quiet", func() bool { return count() > 0 && x.agentsQuietNow.Load() })
	before := count()
	x.hookSeen(HookEvent{Event: "UserPromptSubmit"})
	waitFor(t, "a poll right after the hook", func() bool { return count() > before })
}

func TestQueueViewReadsStartTimesOnceWhileTheGateIsHeld(t *testing.T) {
	gitDir := t.TempDir()
	lock := filepath.Join(gitDir, "gate.lock")
	if err := os.MkdirAll(waitersDir(lock), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(lock, 0o755); err != nil {
		t.Fatal(err)
	}
	host := hostName()
	holder := LeaseOwner{V: 1, Nonce: "0123456789abcdef", PID: 4001, PStart: "S4001", ChildPID: 4002, ChildPStart: "S4002",
		Host: host, Started: t0.Unix(), Renewed: t0.Unix(), TTL: 600}
	waiter := LeaseOwner{V: 1, Nonce: "fedcba9876543210", PID: 4003, PStart: "S4003", Host: host, Started: t0.Unix(), Renewed: t0.Unix()}
	if err := writeLeaseFile(filepath.Join(lock, ownerFileName), holder); err != nil {
		t.Fatal(err)
	}
	if err := writeLeaseFile(filepath.Join(waitersDir(lock), fmt.Sprintf("%020d-%s.json", t0.UnixNano(), waiter.Nonce)), waiter); err != nil {
		t.Fatal(err)
	}
	reads := map[int]int{}
	start := func(pid int) string { reads[pid]++; return fmt.Sprintf("S%d", pid) }
	isAlive := func(pid int) bool { return pid >= 4001 && pid <= 4003 }
	q := QueueConfig{ID: "gate", Title: "Gate", Lock: "gate.lock"}
	// queueLoop's read, with fake processes: pids 4001-4003 are alive only to the
	// injected kill(0), so a reader that bypassed the cache would find them gone.
	x := &runtimeV2{cfg: &Config{Queues: []QueueConfig{q}}, gitDir: gitDir, runs: map[string]*QueueRun{},
		procs: ProcCache{Alive: isAlive, Start: start, Now: func() time.Time { return t0 }}}
	for i := 0; i < 60; i++ { // once a second for a minute
		qs, err := x.readQueues(context.Background(), t0)
		if err != nil || len(qs) != 1 {
			t.Fatalf("read %d: %v %v", i, qs, err)
		}
		v := qs[0]
		if !v.Held || v.Holder == nil || !v.Holder.Alive || v.Holder.Stale || len(v.Waiters) != 1 {
			t.Fatalf("read %d: %+v", i, v)
		}
	}
	total := 0
	for _, n := range reads {
		total += n
	}
	if total > 3 {
		t.Fatalf("a minute of queue reads spawned ps %d times (%v); want one per process", total, reads)
	}
	// The same minute without the cache, as queueLoop ran before PANEL-7.
	uncached := 0
	for i := 0; i < 60; i++ {
		ReadQueue(q, lock, t0, func(pid int) (bool, string) { uncached++; return isAlive(pid), start(pid) })
	}
	if uncached < 120 {
		t.Fatalf("the uncached baseline read only %d start times; the fixture does not exercise the reader", uncached)
	}
}

// ---- claude agents ----

func TestAgentsCadence(t *testing.T) {
	polls := func(lastHook time.Time, lanes bool) int {
		n := 0
		for now := t0; now.Before(t0.Add(time.Minute)); now = now.Add(AgentsInterval(lastHook, now, lanes)) {
			n++
		}
		return n
	}
	cases := []struct {
		name     string
		lastHook time.Time
		lanes    bool
		want     int
	}{
		{"no lane, no hook ever: quiet", time.Time{}, false, 4},
		{"no lane, last hook 6 min ago: quiet", t0.Add(-6 * time.Minute), false, 4},
		{"no lane, last hook 4 min ago: fast", t0.Add(-4 * time.Minute), false, 30},
		{"a lane, no hook: fast", time.Time{}, true, 30},
		// 0..25 every 5 s while the hook is under 30 s old, then 30..58 every 2 s.
		{"hooks flowing, then not: slow, then fast", t0.Add(-time.Second), true, 6 + 15},
		{"hooks flowing without a lane: slow, then fast", t0.Add(-time.Second), false, 6 + 15},
	}
	for _, c := range cases {
		if got := polls(c.lastHook, c.lanes); got != c.want {
			t.Errorf("%s: %d polls a minute, want %d", c.name, got, c.want)
		}
	}
}

// A lane start records the lane, then kicks the agents loop, whose next interval
// must already count the lane: the model learns of it only from the next tmux poll.
func TestAgentsLoopCountsARegisteredLaneBeforeTheModelDoes(t *testing.T) {
	reg, err := OpenRegistry(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(v2Config(t, ""), "/p", t0)
	x := &runtimeV2{hub: NewHub(m, func() time.Time { return t0 }), p: &pollers{registry: reg}}
	if x.hasLanes() {
		t.Fatal("no lane yet")
	}
	if _, err := reg.Begin(LaneRecord{ID: "lane-a", SessionID: "s-a", Path: "/p", Type: "build", Mode: "root"}, "start", t0); err != nil {
		t.Fatal(err)
	}
	if !x.hasLanes() {
		t.Fatal("a registered lane does not count until the model hears of it")
	}
	if got := AgentsInterval(x.lastHookAt(), t0, x.hasLanes()); got != agentsFast {
		t.Fatalf("with a lane just started the next poll is in %v, want %v", got, agentsFast)
	}
}

// A 15 s interval must not make every reading look stale between polls.
func TestAgentsFreshAllowsForTheQuietInterval(t *testing.T) {
	m := NewModel(v2Config(t, ""), "/p", t0)
	m.agentsNext = agentsQuiet
	m.ApplyAgentsTimed(nil, nil, 100*time.Millisecond, t0)
	if !m.agentsFresh(t0.Add(agentsQuiet + time.Second)) {
		t.Fatal("a reading one quiet interval old counts as stale")
	}
	if m.agentsFresh(t0.Add(2*agentsQuiet + time.Second)) {
		t.Fatal("a reading two quiet intervals old counts as fresh")
	}
}

// ---- the first-prompt loop ----

func promptRuntime(t *testing.T, clock *time.Time, rec LaneRecord, running bool) (*runtimeV2, *Hub) {
	t.Helper()
	m := NewModel(v2Config(t, ""), "/p", *clock)
	hub := NewHub(m, func() time.Time { return *clock })
	var tl []TmuxLane
	if running {
		tl = []TmuxLane{{ID: rec.ID, Path: "/p"}}
	}
	hub.Update(func(m *Model, now time.Time) { m.ApplyTmux(tl, []LaneRecord{rec}, "", nil, now) })
	p := &pollers{hub: hub, kickAgents: make(chan struct{}, 1), kickTmux: make(chan struct{}, 1)}
	return &runtimeV2{hub: hub, p: p, o: Options{Out: &strings.Builder{}}}, hub
}

func drained(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestPromptLoopStopsPollingForAGoneLane(t *testing.T) {
	clock := t0
	rec := LaneRecord{ID: "lane-a", SessionID: "s-a", PromptState: "pending", ActionAt: t0.Add(-time.Hour).UnixMilli(), ActionDone: true}
	x, hub := promptRuntime(t, &clock, rec, false)
	kicks := 0
	for i := 0; i < 60; i++ {
		x.promptTick(context.Background(), clock)
		if drained(x.p.kickAgents) {
			kicks++
		}
		clock = clock.Add(time.Second)
		hub.Update(func(m *Model, now time.Time) { m.ApplyTmux(nil, []LaneRecord{rec}, "", nil, now) })
	}
	if kicks != 0 {
		t.Fatalf("a pending lane whose tmux session is gone kicked %d claude agents polls in a minute; want 0", kicks)
	}
	var d PromptDecision
	hub.Read(func(m *Model, now time.Time) { d = m.PromptDecisions(now)["lane-a"] })
	if d.Action != "restore" || !strings.Contains(d.Why, "RESTORE") {
		t.Fatalf("a minute after its session went, the lane is %q (%s); want restore", d.Action, d.Why)
	}
	// Within the grace (a start still coming up) it waits, without polling.
	clock = t0
	_, hub2 := promptRuntime(t, &clock, rec, false)
	hub2.Read(func(m *Model, now time.Time) { d = m.PromptDecisions(now.Add(goneGrace - time.Second))["lane-a"] })
	if d.Action != "wait" || d.Poll {
		t.Fatalf("inside the grace: %+v", d)
	}
}

func TestPromptLoopKicksNoFasterThanTheFastInterval(t *testing.T) {
	clock := t0
	// Typed, not yet confirmed: waits on `claude agents` for confirmGrace.
	rec := LaneRecord{ID: "lane-a", SessionID: "s-a", PromptState: "sent", PromptAt: t0.UnixMilli(), ActionAt: t0.UnixMilli(), ActionDone: true}
	x, _ := promptRuntime(t, &clock, rec, true)
	var at []time.Time
	for i := 0; i < 20; i++ { // promptLoop ticks once a second
		x.promptTick(context.Background(), clock)
		if drained(x.p.kickAgents) {
			at = append(at, clock)
		}
		clock = clock.Add(time.Second)
	}
	if len(at) != 10 {
		t.Fatalf("%d kicks in 20 s of waiting, want 10 (one per %v)", len(at), promptKickEvery)
	}
	for i := 1; i < len(at); i++ {
		if at[i].Sub(at[i-1]) < promptKickEvery {
			t.Fatalf("kicks %v apart", at[i].Sub(at[i-1]))
		}
	}
}

// ---- PANEL-5 carry-over: a waiting note is not kept without bound ----

func TestWaitingNoteIsDroppedWhenItsPaneIsDeadOrAfterADay(t *testing.T) {
	boom := errors.New("claude agents: boom")
	withNote := func(t *testing.T) *Model {
		m := NewModel(v2Config(t, ""), "/p", t0)
		m.sessions["s-a"] = &session{ID: "s-a", LastHookAt: t0, Note: &note{Type: "permission_prompt", At: t0}}
		return m
	}
	// Polls failing: kept for a day, not past it.
	m := withNote(t)
	m.ApplyAgents(nil, boom, t0.Add(23*time.Hour))
	if m.sessions["s-a"] == nil {
		t.Fatal("dropped before a day")
	}
	m.ApplyAgents(nil, boom, t0.Add(25*time.Hour))
	if m.sessions["s-a"] != nil {
		t.Fatal("a waiting note kept past a day while polls fail")
	}
	// Its lane's pane is dead: dropped at once.
	for _, tc := range []struct {
		name string
		tl   []TmuxLane
	}{{"pane dead", []TmuxLane{{ID: "lane-a", Dead: true}}}, {"session gone", nil}} {
		m := withNote(t)
		m.ApplyTmux(tc.tl, []LaneRecord{{ID: "lane-a", SessionID: "s-a"}}, "", nil, t0)
		m.ApplyAgents(nil, boom, t0.Add(time.Minute))
		if m.sessions["s-a"] != nil {
			t.Fatalf("%s: a waiting note kept while polls fail", tc.name)
		}
	}
	// A live pane, or a tmux poll that failed, keeps it.
	m = withNote(t)
	m.ApplyTmux([]TmuxLane{{ID: "lane-a"}}, []LaneRecord{{ID: "lane-a", SessionID: "s-a"}}, "", nil, t0)
	m.ApplyAgents(nil, boom, t0.Add(time.Hour))
	if m.sessions["s-a"] == nil {
		t.Fatal("dropped while its pane is alive")
	}
	m = withNote(t)
	m.ApplyTmux(nil, nil, "", errors.New("tmux: boom"), t0)
	m.ApplyAgents(nil, boom, t0.Add(time.Hour))
	if m.sessions["s-a"] == nil {
		t.Fatal("dropped on a failed tmux poll")
	}
}
