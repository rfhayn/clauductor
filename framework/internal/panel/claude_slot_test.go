package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/claudecall"
	pclock "github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/web"
)

// slotRunner is the Runner below the gate: it answers `claude agents` with no
// session, blocks a `claude hold` until release, and counts the claude calls that
// reach it, which are the processes that would have started.
type slotRunner struct {
	release chan struct{}
	holdFor time.Duration // > 0: a `claude hold` takes this long instead
	n       atomic.Int32
}

func newSlotRunner() *slotRunner { return &slotRunner{release: make(chan struct{})} }

func (s *slotRunner) run(ctx context.Context, dir string, argv []string) ([]byte, error) {
	if !signals.IsClaude(argv) {
		return nil, fmt.Errorf("unexpected command %v", argv)
	}
	s.n.Add(1)
	if argv[1] == "hold" {
		if s.holdFor > 0 {
			time.Sleep(s.holdFor)
		} else {
			<-s.release
		}
	}
	return []byte("[]"), nil
}

// slotRuntime is a project runtime with what the agents poll touches, its Runner
// gated as Run gates it, over the locks r reads.
func slotRuntime(t *testing.T, inner signals.Runner, r claudecall.Reader) (*Runtime, *claudecall.Gate) {
	t.Helper()
	root := signals.ResolvePath(t.TempDir())
	now := time.Now()
	m := state.NewModel(v2Config(t, ""), root, now)
	m.ApplyWorktrees([]signals.Worktree{{Path: root, Branch: "main"}}, nil, now)
	g := claudecall.NewGate(inner, r, pclock.System, claudecall.NewLog(t.TempDir()))
	t.Cleanup(func() { g.Close(claudecall.ShutdownGrace) })
	rt := &Runtime{id: "p", hub: web.NewHub(m, pclock.System), clock: pclock.System, ticks: DefaultTicks(), root: root, run: g.Run,
		kickWT: make(chan struct{}, 1), kickAgents: make(chan struct{}, 1)}
	return rt, g
}

// hold takes the gate's slot with a lane action until the runner's release.
func hold(t *testing.T, g *claudecall.Gate, s *slotRunner) {
	t.Helper()
	n := s.n.Load()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	go g.Run(ctx, "", []string{"claude", "hold"})
	waitUntil(t, "the holder's call", 5*time.Second, func() bool { return s.n.Load() == n+1 })
}

// agentsState is what an agents poll may change: its source's status and when
// `claude agents` last answered.
func agentsState(rt *Runtime) (state.SourceStatus, int64) {
	v := rt.hub.View()
	return v.Sources["agents"], v.AgentsReadAt
}

// [CLAUDECALLS-2-S1] A colliding poll waits briefly, then retries soon: no second
// claude starts, the poll's next wait is the 1 s retry rather than its interval, and
// the agents source's last reading, its success time and its error stay as they were.
func TestAgentsPollCollidingWithTheSlotRetriesSoon(t *testing.T) {
	t.Parallel()
	s := newSlotRunner()
	defer close(s.release)
	rt, g := slotRuntime(t, s.run, claudecall.Reader{})
	src := rt.agentsSource()
	ctx := context.Background()
	rt.pollOnce(ctx, src) // a good read first
	before, readAt := agentsState(rt)
	if !before.OK || readAt == 0 {
		t.Fatalf("the first poll: %+v %d", before, readAt)
	}
	hold(t, g, s)
	n := s.n.Load()
	next, _ := rt.pollOnce(ctx, src)
	if next != claudecall.BusyRetry || next >= rt.ticks.agentsInterval(rt.lastHookAt(), time.Now(), false) {
		t.Fatalf("next wait %v, want the %v retry, not the interval", next, claudecall.BusyRetry)
	}
	if got := s.n.Load(); got != n {
		t.Fatalf("%d claude calls started beside the holder's", got-n)
	}
	if after, at := agentsState(rt); after != before || at != readAt {
		t.Fatalf("the source changed: %+v %d, was %+v %d", after, at, before, readAt)
	}
}

// The agents poll's timing (AgentsPollMs, the figure D3 reads) is its call's alone,
// not its wait for the slot.
func TestAgentsPollTimesTheCallNotTheWait(t *testing.T) {
	t.Parallel()
	s := newSlotRunner()
	s.holdFor = 1500 * time.Millisecond // within the poll's 2 s wait
	rt, g := slotRuntime(t, s.run, claudecall.Reader{})
	rt.agentsFilterCheck = time.Now() // no cross-check: the poll's one call
	go g.Run(signals.WithPoll(context.Background()), "", []string{"claude", "hold"})
	waitUntil(t, "the holder's call", 5*time.Second, func() bool { return s.n.Load() == 1 })
	start := time.Now()
	if _, next := rt.pollAgents(context.Background(), time.Now()); next == claudecall.BusyRetry {
		t.Fatal("the poll gave up: the holder outlasted its wait")
	}
	if waited := time.Since(start); waited < time.Second {
		t.Fatalf("the poll ran after %v: it never waited for the holder", waited)
	}
	rt.mu.Lock()
	ms := rt.obs.AgentsPollMs
	rt.mu.Unlock()
	if ms >= 500 {
		t.Fatalf("AgentsPollMs %d: it counts the wait for the slot", ms)
	}
}

// checkFilter records its check time only after a cross-check that ran: one the
// gate held back is tried again with the next poll, keeping the filter it had.
func TestCheckFilterTimeOnlyAfterACrossCheckThatRan(t *testing.T) {
	t.Parallel()
	s := newSlotRunner()
	rt, g := slotRuntime(t, s.run, claudecall.Reader{})
	rt.agentsFilter = []string{"--cwd", "/kept"}
	hold(t, g, s)
	if _, next := rt.pollAgents(context.Background(), time.Now()); next != claudecall.BusyRetry {
		t.Fatalf("next %v", next)
	}
	if !rt.agentsFilterCheck.IsZero() || len(rt.agentsFilter) != 2 || rt.agentsFilter[1] != "/kept" {
		t.Fatalf("a held-back cross-check recorded %v, filter %v", rt.agentsFilterCheck, rt.agentsFilter)
	}
	close(s.release)
	waitUntil(t, "the slot free", 5*time.Second, func() bool {
		_, err := g.Run(signals.WithPoll(context.Background()), "", []string{"claude", "agents", "--json"})
		return err == nil
	})
	rt.pollAgents(context.Background(), time.Now())
	if rt.agentsFilterCheck.IsZero() {
		t.Fatal("a cross-check that ran recorded no time")
	}
}

// A card or suggest command running claude that collides applies no update (so it is
// never shown failed for it) and returns a short retry (D9).
func TestCardCollidingWithTheSlotAppliesNoUpdate(t *testing.T) {
	t.Parallel()
	s := newSlotRunner()
	defer close(s.release)
	rt, g := slotRuntime(t, s.run, claudecall.Reader{})
	poll := polled(commandFetch(rt, 30*time.Second, []string{"claude", "-p", "x"}, func(b []byte) (string, error) { return string(b), nil }),
		func(*state.Model, string, error, time.Time) { t.Error("the card applied an update") })
	hold(t, g, s)
	if up, next := poll(context.Background(), time.Now()); up != nil || next != claudecall.BusyRetry {
		t.Fatalf("update %v, next %v", up != nil, next)
	}
}

// [CLAUDECALLS-2-S2] A lane action waits for the slot: behind an agents poll that
// holds it for 5 s, the lane manager's own `claude agents` read (liveSessions, with
// its 10 s timeout, in RestoreAll) does not give up at the poll's 2 s, and runs as
// soon as the poll's call ends.
func TestLaneActionWaitsForTheSlotThroughTheLaneManager(t *testing.T) {
	t.Parallel()
	s := newSlotRunner()
	s.holdFor = 5 * time.Second
	_, g := slotRuntime(t, s.run, claudecall.Reader{})
	reg, err := lanes.OpenRegistry(t.TempDir(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeTmux{calls: map[string]int{}}
	lm := &lanes.LaneManager{Clock: pclock.System, TmuxPath: "tmux", Socket: "test", Root: "/repo", Registry: reg, Exec: f.exec, Run: g.Run,
		LookupEnv: func(string) (string, bool) { return "", false }}
	go g.Run(signals.WithPoll(context.Background()), "", []string{"claude", "hold"})
	waitUntil(t, "the poll's call", 5*time.Second, func() bool { return s.n.Load() == 1 })
	start := time.Now()
	if _, lerr := lm.RestoreAll(context.Background(), nil, false); lerr != nil {
		t.Fatalf("RestoreAll: %v", lerr)
	}
	if took := time.Since(start); took < claudecall.PollWait+time.Second {
		t.Fatalf("RestoreAll returned after %v: it never waited out the poll's call", took)
	}
	if s.n.Load() != 2 {
		t.Fatalf("%d claude calls, want the poll's and the lane action's", s.n.Load())
	}
}

// pausedReader is the lock reader of a temp home for env, with a lock at path
// modified at mod.
func lockAt(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func envWith(kv map[string]string) claudecall.LookupEnv {
	return func(k string) (string, bool) {
		if v, ok := kv[k]; ok {
			return v, true
		}
		return testEnv(k)
	}
}

// besideRealPath is where Claude Code puts the legacy lock of dir, which exists:
// `${realpath(dir)}.lock` (nC() in 2.1.296).
func besideRealPath(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return real + ".lock"
}

// [CLAUDECALLS-3-S1] A live lock pauses the polls without marking them failed: no
// claude starts, the agents source says a login refresh is in progress and is
// neither marked failed nor read again as fresh, and once the lock is gone the next
// poll runs.
func TestLiveLockPausesTheAgentsPoll(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	r := claudecall.NewReader(testEnv, home)
	s := newSlotRunner()
	rt, _ := slotRuntime(t, s.run, r)
	src := rt.agentsSource()
	ctx := context.Background()
	rt.pollOnce(ctx, src)
	before, readAt := agentsState(rt)
	n := s.n.Load()
	lockAt(t, r.CurrentPath(), time.Now().Add(-3*time.Second))
	next, _ := rt.pollOnce(ctx, src)
	if next != claudecall.PausedRetry {
		t.Fatalf("next %v, want the %v retry", next, claudecall.PausedRetry)
	}
	if s.n.Load() != n {
		t.Fatal("a claude started while the lock was live")
	}
	after, at := agentsState(rt)
	want := before
	want.Paused = "paused: a login refresh is in progress"
	if after != want || at != readAt {
		t.Fatalf("paused source %+v %d, want %+v %d", after, at, want, readAt)
	}
	if err := os.Remove(r.CurrentPath()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond) // a later millisecond for the read time
	rt.pollOnce(ctx, src)
	if after, at := agentsState(rt); !after.OK || after.Paused != "" || at <= readAt || s.n.Load() == n {
		t.Fatalf("after the lock went: %+v %d", after, at)
	}
}

// pollPaused polls agents once over env's locks and reports whether it was paused
// with no claude started.
func pollPaused(t *testing.T, env claudecall.LookupEnv, home string) bool {
	t.Helper()
	s := newSlotRunner()
	rt, _ := slotRuntime(t, s.run, claudecall.NewReader(env, home))
	_, next := rt.pollAgents(context.Background(), time.Now())
	paused := next == claudecall.PausedRetry
	if paused != (s.n.Load() == 0) {
		t.Fatalf("paused %v with %d claude calls", paused, s.n.Load())
	}
	return paused
}

// [CLAUDECALLS-3-S2] A stale lock does not stop calls.
// [CLAUDECALLS-3-S4] Nor does a lock dated in the future.
func TestStaleAndFutureLocksDoNotPause(t *testing.T) {
	t.Parallel()
	for name, mod := range map[string]time.Time{"stale": time.Now().Add(-5 * time.Minute), "future": time.Now().Add(2 * time.Hour)} {
		home := t.TempDir()
		lockAt(t, claudecall.NewReader(testEnv, home).CurrentPath(), mod)
		if pollPaused(t, testEnv, home) {
			t.Errorf("a %s lock paused the poll", name)
		}
	}
}

// [CLAUDECALLS-3-S3] The config directory follows CLAUDE_CONFIG_DIR: a live lock
// there, and none in ~/.claude, pauses the poll.
func TestLockPauseFollowsClaudeConfigDir(t *testing.T) {
	t.Parallel()
	home, cfg := t.TempDir(), filepath.Join(t.TempDir(), "cfg")
	env := envWith(map[string]string{"CLAUDE_CONFIG_DIR": cfg})
	lockAt(t, filepath.Join(cfg, ".oauth_refresh.lock"), time.Now().Add(-3*time.Second))
	if !pollPaused(t, env, home) {
		t.Fatal("a live lock in CLAUDE_CONFIG_DIR did not pause the poll")
	}
	if pollPaused(t, testEnv, home) {
		t.Fatal("~/.claude holds no lock, yet the poll paused")
	}
}

// [CLAUDECALLS-3-S6] The lock follows CLAUDE_SECURESTORAGE_CONFIG_DIR: a live lock
// there, and none in the config dir, pauses the poll; and a live legacy lock beside
// that directory pauses it the same way.
func TestLockPauseFollowsSecureStorageConfigDir(t *testing.T) {
	t.Parallel()
	home, sec := t.TempDir(), filepath.Join(t.TempDir(), "secure")
	env := envWith(map[string]string{"CLAUDE_SECURESTORAGE_CONFIG_DIR": sec})
	lockAt(t, filepath.Join(sec, ".oauth_refresh.lock"), time.Now().Add(-3*time.Second))
	if !pollPaused(t, env, home) {
		t.Fatal("a live lock in CLAUDE_SECURESTORAGE_CONFIG_DIR did not pause the poll")
	}
	if pollPaused(t, testEnv, home) {
		t.Fatal("the config dir holds no lock, yet the poll paused")
	}

	home, sec = t.TempDir(), filepath.Join(t.TempDir(), "secure")
	env = envWith(map[string]string{"CLAUDE_SECURESTORAGE_CONFIG_DIR": sec})
	lockAt(t, besideRealPath(t, sec), time.Now().Add(-3*time.Second))
	if !pollPaused(t, env, home) {
		t.Fatal("a live legacy lock beside CLAUDE_SECURESTORAGE_CONFIG_DIR did not pause the poll")
	}
	if pollPaused(t, testEnv, home) {
		t.Fatal("nothing is beside the config dir, yet the poll paused")
	}
}

// [CLAUDECALLS-3-S5] A live legacy lock beside the lock directory pauses the polls
// too: with CLAUDE_SECURESTORAGE_CONFIG_DIR unset, that is the config dir.
func TestLiveLegacyLockPauses(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	lockAt(t, besideRealPath(t, filepath.Join(home, ".claude")), time.Now().Add(-3*time.Second))
	if !pollPaused(t, testEnv, home) {
		t.Fatal("a live legacy lock did not pause the poll")
	}
}

// claudeCounter is a Run Runner: fakeRunner's answers for any project's root, each
// claude call taking 100 ms, with the most claude calls seen running at once.
type claudeCounter struct {
	mu        sync.Mutex
	cur, peak int
	byArgv    map[string]int
	authFails bool
}

func (c *claudeCounter) run(ctx context.Context, dir string, argv []string) ([]byte, error) {
	j := strings.Join(argv, " ")
	switch {
	case j == "git worktree list --porcelain":
		return []byte(fmt.Sprintf("worktree %s\nHEAD abc\nbranch refs/heads/main\n\n", dir)), nil
	case j == "git --no-optional-locks -c core.fsmonitor= rev-parse --git-common-dir":
		return []byte(".git\n"), nil
	case strings.HasPrefix(j, "gh pr list"):
		return []byte(`[]`), nil
	case !signals.IsClaude(argv):
		return nil, fmt.Errorf("unexpected command %v", argv)
	}
	args, _ := signals.ClaudeArgs(argv)
	key := strings.Join(args, " ")
	c.mu.Lock()
	c.cur++
	c.peak = max(c.peak, c.cur)
	if c.byArgv == nil {
		c.byArgv = map[string]int{}
	}
	c.byArgv[key]++
	fail := c.authFails
	c.mu.Unlock()
	time.Sleep(100 * time.Millisecond)
	c.mu.Lock()
	c.cur--
	c.mu.Unlock()
	switch {
	case strings.HasPrefix(key, "agents --json"):
		return []byte(`[]`), nil
	case key == "--version":
		return []byte("2.1.296 (Claude Code)\n"), nil
	case key == "auth status --json" && !fail:
		return []byte(`{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","orgId":"x","subscriptionType":"max"}`), nil
	}
	return nil, fmt.Errorf("claude %s failed", key)
}

func (c *claudeCounter) count(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byArgv[key]
}

// stateOf is a project's view over HTTP.
func stateOf(t *testing.T, c liveClient, project string) state.View {
	t.Helper()
	req, _ := http.NewRequest("GET", c.base+"/api/state?project="+project, nil)
	req.Header.Set("Cookie", c.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v state.View
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// [CLAUDECALLS-2-S4] Calls that start together all run: a panel serving two projects
// starts with both agents polls, the version and the account due at once, and each
// runs within a few seconds, one claude at a time.
func TestPanelStartRunsEveryClaudeCallOneAtATime(t *testing.T) {
	t.Parallel()
	root, home := setupProject(t)
	other := signals.ResolvePath(t.TempDir())
	writeFile(t, filepath.Join(other, config.DefaultConfigRel), `{"name":"Other","lanes":{"main":"orchestrator"},"tmux_socket":"`+noServerSocket()+`"}`)
	c := &claudeCounter{}
	if _, err := install.AddProject(context.Background(), install.AddOptions{Home: home, Project: other, Run: c.run, Now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	_, client, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: c.run})
	defer stop()
	var ids []string
	reg, _ := config.LoadProjects(home)
	for _, e := range reg.Projects {
		ids = append(ids, e.ID)
	}
	if len(ids) != 2 {
		t.Fatalf("projects %v", ids)
	}
	waitUntil(t, "every claude source read", 5*time.Second, func() bool {
		for _, id := range ids {
			v := stateOf(t, client, id)
			if !v.Sources["agents"].OK || !v.Observe.VersionSource.OK || !v.Account.Source.OK {
				return false
			}
		}
		return true
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.peak != 1 {
		t.Fatalf("%d claude calls ran at once", c.peak)
	}
}
