package state

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

var t0 = time.Date(2026, 9, 28, 14, 0, 0, 0, time.UTC)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "signals", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "config", "testdata", "panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(t, b)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// spikeHooks returns the hook payloads Claude Code 2.1.284 actually sent in the spike.
func spikeHooks(t *testing.T) []signals.HookEvent {
	t.Helper()
	var evs []signals.HookEvent
	sc := bufio.NewScanner(bytes.NewReader(fixture(t, "spike-hooks.log")))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		ev, err := signals.ParseHook([]byte(line))
		if err != nil {
			t.Fatalf("spike line does not parse: %v", err)
		}
		evs = append(evs, ev)
	}
	if len(evs) != 15 {
		t.Fatalf("want the 15 recorded spike events, got %d", len(evs))
	}
	return evs
}

func fixtureWorktrees(t *testing.T) []signals.Worktree {
	t.Helper()
	wts, err := readWorktreesFrom(fixture(t, "worktrees-fixture.porcelain"))
	if err != nil {
		t.Fatal(err)
	}
	return wts
}

func laneByName(v View, name string) *LaneView {
	for i := range v.Lanes {
		if v.Lanes[i].Name == name {
			return &v.Lanes[i]
		}
	}
	return nil
}

func TestReducerReplaysSpikeHooks(t *testing.T) {
	t.Parallel()
	evs := spikeHooks(t)
	spikeCwd := evs[0].Cwd
	tests := []struct {
		name          string
		upTo          int
		wantStatus    string
		wantSubagents int
		wantLast      string
	}{
		{"prompt submitted", 1, "busy", 0, "UserPromptSubmit"},
		{"turn stopped", 2, "idle", 0, "Stop"},
		{"subagent started", 4, "busy", 1, "SubagentStart"},
		{"subagent stopped", 5, "busy", 0, "SubagentStop"},
		// Events 6-9 include SubagentStops with an empty type for ids never started:
		// internal agents, which retire nothing.
		{"empty-type stops for ids never started", 9, "idle", 0, "SubagentStop"},
		{"whole spike", 15, "idle", 0, "Stop"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(testConfig(t), "/spike", t0)
			m.ApplyWorktrees([]signals.Worktree{{Path: spikeCwd, Branch: "main"}}, nil, t0)
			for i, ev := range evs[:tc.upTo] {
				if !m.ApplyHook(ev, t0.Add(time.Duration(i)*time.Second)) {
					t.Fatalf("event %d dropped", i)
				}
			}
			v := m.Snapshot(t0.Add(time.Minute))
			if len(v.Lanes) != 1 {
				t.Fatalf("want 1 lane, got %d", len(v.Lanes))
			}
			l := v.Lanes[0]
			if l.Status != tc.wantStatus || len(l.Subagents) != tc.wantSubagents || l.LastEvent != tc.wantLast {
				t.Fatalf("got status=%s subagents=%d last=%s", l.Status, len(l.Subagents), l.LastEvent)
			}
			if l.Type != "orchestrator" || l.Name != "main" {
				t.Fatalf("lane typed %s/%s", l.Type, l.Name)
			}
			if tc.upTo == 4 && l.Subagents[0].Type != "general-purpose" {
				t.Fatalf("subagent type %q", l.Subagents[0].Type)
			}
			if v.HookEvents != tc.upTo || len(v.Feed) != tc.upTo {
				t.Fatalf("hookEvents=%d feed=%d", v.HookEvents, len(v.Feed))
			}
		})
	}
}

func TestReducerDropsEventsOutsideTheProject(t *testing.T) {
	t.Parallel()
	tests := []struct {
		cwd      string
		wantLane string // "" = dropped
	}{
		{"/repo", "main"},
		{"/repo/apps/web", "main"},
		// Linked worktrees live inside the main checkout: the longest match must win.
		{"/repo/.claude/worktrees/build-add-feature", "add-feature"},
		{"/repo/.claude/worktrees/fix-thing/apps/web", "thing"},
		{"/repo-evil", ""},
		{"/elsewhere/other-project", ""},
		{"/", ""},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.cwd, func(t *testing.T) {
			m := NewModel(testConfig(t), "/repo", t0)
			m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
			kept := m.ApplyHook(signals.HookEvent{SessionID: "s1", Cwd: tc.cwd, Event: "UserPromptSubmit", Prompt: "hi"}, t0)
			st := m.ApplyStatus(signals.StatusPayload{SessionID: "s1", Cwd: tc.cwd}, t0)
			v := m.Snapshot(t0.Add(time.Second))
			if tc.wantLane == "" {
				if kept || st || len(v.Lanes) != 0 || len(v.Feed) != 0 || v.Dropped != 2 {
					t.Fatalf("out-of-project event was kept: kept=%v status=%v lanes=%d feed=%d", kept, st, len(v.Lanes), len(v.Feed))
				}
				return
			}
			if !kept || laneByName(v, tc.wantLane) == nil {
				t.Fatalf("event not attributed to lane %q", tc.wantLane)
			}
		})
	}
}

func TestReducerStatusLine(t *testing.T) {
	t.Parallel()
	sp, err := signals.ParseStatus(fixture(t, "statusline.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(testConfig(t), sp.Cwd, t0)
	m.ApplyWorktrees([]signals.Worktree{{Path: sp.Cwd, Branch: "main"}}, nil, t0)
	if !m.ApplyStatus(sp, t0) {
		t.Fatal("status dropped")
	}
	v := m.Snapshot(t0)
	if v.Quota == nil || *v.Quota.FiveHour != 12 || *v.Quota.SevenDay != 70 {
		t.Fatalf("quota %+v", v.Quota)
	}
	if v.EstCostUSD == nil || *v.EstCostUSD < 0.026 || *v.EstCostUSD > 0.0261 {
		t.Fatalf("est cost %v", v.EstCostUSD)
	}
	if l := v.Lanes; len(l) != 1 || l[0].CtxPct == nil || *l[0].CtxPct != 18 {
		t.Fatalf("ctx not recorded: %+v", l)
	}
	// A later post carrying only one window keeps the other window's last value.
	partial := sp
	seven := 71.0
	partial.RateLimits.FiveHour = nil
	partial.RateLimits.SevenDay = &signals.RateLimit{UsedPercentage: &seven}
	m.ApplyStatus(partial, t0.Add(time.Second))
	if q := m.Snapshot(t0).Quota; q.FiveHour == nil || *q.FiveHour != 12 || *q.SevenDay != 71 {
		t.Fatalf("partial quota post: %+v", q)
	}
	// A status post from another project must not move the quota or the cost. (A
	// session already bound here keeps its binding when it cds away, so the foreign
	// post is a different session.)
	foreign := sp
	foreign.SessionID = "someone-else"
	foreign.Cwd = "/elsewhere"
	hi := 99.0
	foreign.RateLimits.FiveHour = &signals.RateLimit{UsedPercentage: &hi}
	if m.ApplyStatus(foreign, t0) {
		t.Fatal("foreign status kept")
	}
	if v := m.Snapshot(t0); *v.Quota.FiveHour != 12 {
		t.Fatal("foreign status moved the quota")
	}
}

func TestReducerAgentsPoll(t *testing.T) {
	t.Parallel()
	agents, err := signals.ParseAgents(fixture(t, "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	m.ApplyAgents(agents, nil, t0)
	v := m.Snapshot(t0)
	want := map[string][2]string{ // name → {type, status}
		"main":        {"orchestrator", "idle"},
		"add-feature": {"build", "busy"},
		"thing":       {"fix", "waiting"},
	}
	if len(v.Lanes) != len(want) {
		t.Fatalf("want %d lanes, got %d: %+v", len(want), len(v.Lanes), v.Lanes)
	}
	for name, tw := range want {
		l := laneByName(v, name)
		if l == nil || l.Type != tw[0] || l.Status != tw[1] {
			t.Fatalf("lane %s: %+v", name, l)
		}
	}
	if len(v.QuietWorktrees) != 1 || v.QuietWorktrees[0].Type != "detached" {
		t.Fatalf("quiet worktrees %+v", v.QuietWorktrees)
	}
	if len(v.NeedsYou) != 1 || v.NeedsYou[0].Kind != "waiting" || !strings.Contains(v.NeedsYou[0].Text, "gh pr create") {
		t.Fatalf("needs you %+v", v.NeedsYou)
	}
	// The session leaves: its lane goes quiet.
	m.ApplyAgents(agents[:2], nil, t0.Add(2*time.Second))
	if laneByName(m.Snapshot(t0.Add(2*time.Second)), "thing") != nil {
		t.Fatal("lane of a gone session still shown as active")
	}
	// A failed poll keeps the last good list and says so.
	m.ApplyAgents(nil, errors.New("claude: not found"), t0.Add(4*time.Second))
	v = m.Snapshot(t0.Add(4 * time.Second))
	if v.Sources["agents"].OK || v.Sources["agents"].Error == "" || laneByName(v, "add-feature") == nil {
		t.Fatalf("failed poll handled wrongly: %+v", v.Sources["agents"])
	}
}

func TestReducerRealAgentsCapture(t *testing.T) {
	t.Parallel()
	agents, err := signals.ParseAgents(fixture(t, "agents-real.json"))
	if err != nil {
		t.Fatal(err)
	}
	wts, err := readWorktreesFrom(fixture(t, "worktrees-real.porcelain"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(testConfig(t), wts[0].Path, t0)
	m.ApplyWorktrees(wts, nil, t0)
	m.ApplyAgents(agents, nil, t0)
	v := m.Snapshot(t0)
	if laneByName(v, "main") == nil || laneByName(v, "add-feature") == nil {
		t.Fatalf("real capture: lanes %+v", v.Lanes)
	}
}

func TestReducerNeedsYou(t *testing.T) {
	t.Parallel()
	notif := func(kind string) signals.HookEvent {
		return signals.HookEvent{SessionID: "sess-build", Cwd: "/repo/.claude/worktrees/build-add-feature",
			Event: "Notification", NotificationType: kind, Message: "Claude needs your permission to use Bash"}
	}
	busy := []signals.Agent{{SessionID: "sess-build", Cwd: "/repo/.claude/worktrees/build-add-feature", Status: "busy"}}
	waiting := []signals.Agent{{SessionID: "sess-build", Cwd: "/repo/.claude/worktrees/build-add-feature", Status: "waiting"}}
	tests := []struct {
		name  string
		steps func(m *Model)
		want  int
	}{
		{"permission prompt shows", func(m *Model) { m.ApplyHook(notif("permission_prompt"), t0) }, 1},
		// idle_prompt is a finished turn (your move), not a blocked lane: it goes to
		// Done, never to Needs you (rubric: "done" distinct from "blocked").
		{"idle prompt is done, not blocked", func(m *Model) { m.ApplyHook(notif("idle_prompt"), t0) }, 0},
		{"other notification types stay in the feed only", func(m *Model) { m.ApplyHook(notif("auth_success"), t0) }, 0},
		{"a later prompt answers it", func(m *Model) {
			m.ApplyHook(notif("idle_prompt"), t0)
			m.ApplyHook(signals.HookEvent{SessionID: "sess-build", Cwd: "/repo/.claude/worktrees/build-add-feature", Event: "UserPromptSubmit"}, t0.Add(time.Second))
		}, 0},
		{"going busy answers a permission prompt (no hook fires for a grant)", func(m *Model) {
			m.ApplyAgents(waiting, nil, t0)
			m.ApplyHook(notif("permission_prompt"), t0)
			m.ApplyAgents(busy, nil, t0.Add(2*time.Second))
		}, 0},
		{"a waiting session without a notification still shows, once", func(m *Model) {
			m.ApplyAgents(waiting, nil, t0)
		}, 1},
		{"notification and waiting status merge into one item", func(m *Model) {
			m.ApplyAgents(waiting, nil, t0)
			m.ApplyHook(notif("permission_prompt"), t0)
		}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(testConfig(t), "/repo", t0)
			m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
			tc.steps(m)
			if got := len(m.Snapshot(t0.Add(3 * time.Second)).NeedsYou); got != tc.want {
				t.Fatalf("needs-you items: got %d want %d", got, tc.want)
			}
		})
	}
}

func TestReducerStaleHookBanner(t *testing.T) {
	t.Parallel()
	cwd := "/repo/.claude/worktrees/build-add-feature"
	agent := func(status string) []signals.Agent {
		return []signals.Agent{{SessionID: "s", Cwd: cwd, Status: status}}
	}
	hook := signals.HookEvent{SessionID: "s", Cwd: cwd, Event: "SubagentStart", AgentID: "a1", AgentType: "builder"}
	tests := []struct {
		name  string
		steps func(m *Model)
		at    time.Duration
		want  bool
	}{
		{"busy 61 s with no hook", func(m *Model) { m.ApplyAgents(agent("busy"), nil, t0) }, 61 * time.Second, true},
		{"busy 30 s is not yet stale", func(m *Model) { m.ApplyAgents(agent("busy"), nil, t0) }, 30 * time.Second, false},
		{"a hook during the stretch clears it", func(m *Model) {
			m.ApplyAgents(agent("busy"), nil, t0)
			m.ApplyHook(hook, t0.Add(5*time.Second))
		}, 90 * time.Second, false},
		{"a hook just before the poll saw busy counts", func(m *Model) {
			m.ApplyHook(hook, t0.Add(-2*time.Second))
			m.ApplyAgents(agent("busy"), nil, t0)
		}, 90 * time.Second, false},
		{"idle is never stale", func(m *Model) { m.ApplyAgents(agent("idle"), nil, t0) }, 5 * time.Minute, false},
		{"waiting then busy continues the stretch", func(m *Model) {
			m.ApplyAgents(agent("busy"), nil, t0)
			m.ApplyAgents(agent("waiting"), nil, t0.Add(40*time.Second))
			m.ApplyAgents(agent("busy"), nil, t0.Add(50*time.Second))
		}, 70 * time.Second, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(testConfig(t), "/repo", t0.Add(-10*time.Minute))
			m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
			tc.steps(m)
			v := m.Snapshot(t0.Add(tc.at))
			l := laneByName(v, "add-feature")
			if l == nil || l.Stale != tc.want || (len(v.Banners) == 1) != tc.want {
				t.Fatalf("stale=%v banners=%v, want %v", l != nil && l.Stale, v.Banners, tc.want)
			}
		})
	}
	// Grace period: a panel that just started has not had a chance to hear a hook.
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	m.ApplyAgents(agent("busy"), nil, t0)
	if v := m.Snapshot(t0.Add(30 * time.Second)); len(v.Banners) != 0 {
		t.Fatal("banner inside the start-up grace period")
	}
}

func TestReducerFeedIsARingBuffer(t *testing.T) {
	t.Parallel()
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	for i := 0; i < FeedCap+50; i++ {
		m.ApplyHook(signals.HookEvent{SessionID: "s", Cwd: "/repo", Event: "Stop", LastAssistantMessage: strings.Repeat("x", 500)}, t0.Add(time.Duration(i)*time.Second))
	}
	v := m.Snapshot(t0.Add(time.Hour))
	if len(v.Feed) != FeedCap {
		t.Fatalf("feed len %d", len(v.Feed))
	}
	if v.Feed[0].At <= v.Feed[1].At {
		t.Fatal("feed not newest-first")
	}
	if n := len([]rune(v.Feed[0].Detail)); n > signals.DetailMax+1 {
		t.Fatalf("detail not truncated: %d runes", n)
	}
}

func TestReducerSourcesNeverReadAsEmptySuccess(t *testing.T) {
	t.Parallel()
	m := NewModel(testConfig(t), "/repo", t0)
	v := m.Snapshot(t0)
	for _, k := range []string{"prs", "agents", "worktrees"} {
		if !v.Sources[k].Pending || v.Sources[k].OK {
			t.Fatalf("%s should start pending", k)
		}
	}
	if !v.Cards[0].Source.Pending {
		t.Fatal("card should start pending")
	}
	prs, err := signals.ParsePRs(fixture(t, "prs.json"))
	if err != nil {
		t.Fatal(err)
	}
	m.ApplyPRs(prs, nil, t0)
	m.ApplyPRs(nil, errors.New("gh: not logged in"), t0.Add(time.Minute))
	v = m.Snapshot(t0.Add(time.Minute))
	if v.Sources["prs"].OK || v.Sources["prs"].Pending || !strings.Contains(v.Sources["prs"].Error, "not logged in") {
		t.Fatalf("prs source %+v", v.Sources["prs"])
	}
	if len(v.PRs) != 2 {
		t.Fatal("a failed poll replaced the last known PRs")
	}
	m.ApplyCard("todo", nil, errors.New("exit status 1"), t0)
	if c := m.Snapshot(t0).Cards[0]; c.Source.OK || c.Source.Error == "" {
		t.Fatalf("card error not shown: %+v", c)
	}
}

func TestReducerSubagentLifecycle(t *testing.T) {
	t.Parallel()
	cwd := "/repo/.claude/worktrees/build-add-feature"
	start := func(id, typ string) signals.HookEvent {
		return signals.HookEvent{SessionID: "s", Cwd: cwd, Event: "SubagentStart", AgentID: id, AgentType: typ}
	}
	stop := func(id, typ string) signals.HookEvent {
		return signals.HookEvent{SessionID: "s", Cwd: cwd, Event: "SubagentStop", AgentID: id, AgentType: typ}
	}
	agent := func(status string) []signals.Agent {
		return []signals.Agent{{SessionID: "s", Cwd: cwd, Status: status}}
	}
	tests := []struct {
		name  string
		steps func(m *Model)
		at    time.Duration
		want  []string // running agent ids
	}{
		{"stop by matching id", func(m *Model) {
			m.ApplyHook(start("a1", "builder"), t0)
			m.ApplyHook(stop("a1", "builder"), t0.Add(time.Second))
		}, 2 * time.Second, nil},
		{"workflow agent stops under another id: same type retires it", func(m *Model) {
			m.ApplyHook(start("a1", "builder"), t0)
			m.ApplyHook(stop("b9", "builder"), t0.Add(time.Second))
		}, 2 * time.Second, nil},
		{"unknown id retires the OLDEST of that type", func(m *Model) {
			m.ApplyHook(start("a1", "builder"), t0)
			m.ApplyHook(start("a2", "builder"), t0.Add(time.Second))
			m.ApplyHook(start("r1", "reviewer"), t0.Add(time.Second))
			m.ApplyHook(stop("b9", "builder"), t0.Add(2*time.Second))
		}, 3 * time.Second, []string{"a2", "r1"}},
		// From the live capture: a Workflow agent started as "reviewer" and stopped as
		// "workflow-subagent" under another id.
		{"live capture: start reviewer A, stop workflow-subagent B", func(m *Model) {
			m.ApplyHook(start("af6022d", "reviewer"), t0)
			m.ApplyHook(stop("ab7ca82", "workflow-subagent"), t0.Add(time.Second))
		}, 2 * time.Second, nil},
		{"a workflow-subagent stop retires the oldest of any type", func(m *Model) {
			m.ApplyHook(start("A", "builder"), t0)
			m.ApplyHook(start("B", "reviewer"), t0.Add(time.Second))
			m.ApplyHook(stop("C", "workflow-subagent"), t0.Add(2*time.Second))
		}, 3 * time.Second, []string{"B"}},
		{"an empty-type stop (internal agent, never started) retires nothing", func(m *Model) {
			m.ApplyHook(start("a1", "builder"), t0)
			m.ApplyHook(stop("b9", ""), t0.Add(time.Second))
		}, 2 * time.Second, []string{"a1"}},
		{"another unmatched type does not fall back to any type", func(m *Model) {
			m.ApplyHook(start("a1", "builder"), t0)
			m.ApplyHook(stop("b9", "general-purpose"), t0.Add(time.Second))
		}, 2 * time.Second, []string{"a1"}},
		{"a type match is preferred over an older agent of another type", func(m *Model) {
			m.ApplyHook(start("A", "builder"), t0)
			m.ApplyHook(start("B", "reviewer"), t0.Add(time.Second))
			m.ApplyHook(stop("C", "reviewer"), t0.Add(2*time.Second))
		}, 3 * time.Second, []string{"A"}},
		{"hook Stop keeps background agents", func(m *Model) {
			m.ApplyAgents(agent("busy"), nil, t0)
			m.ApplyHook(start("a1", "builder"), t0)
			m.ApplyHook(signals.HookEvent{SessionID: "s", Cwd: cwd, Event: "Stop"}, t0.Add(time.Second))
		}, 2 * time.Second, []string{"a1"}},
		{"idle for 5 s keeps them", func(m *Model) {
			m.ApplyHook(start("a1", "builder"), t0)
			m.ApplyAgents(agent("idle"), nil, t0.Add(time.Second))
			m.ApplyAgents(agent("idle"), nil, t0.Add(6*time.Second))
		}, 7 * time.Second, []string{"a1"}},
		{"idle for 10 s clears them", func(m *Model) {
			m.ApplyHook(start("a1", "builder"), t0)
			m.ApplyAgents(agent("idle"), nil, t0.Add(time.Second))
			m.ApplyAgents(agent("idle"), nil, t0.Add(11*time.Second))
		}, 12 * time.Second, nil},
		{"busy in between restarts the idle clock", func(m *Model) {
			m.ApplyHook(start("a1", "builder"), t0)
			m.ApplyAgents(agent("idle"), nil, t0.Add(time.Second))
			m.ApplyAgents(agent("busy"), nil, t0.Add(5*time.Second))
			m.ApplyAgents(agent("idle"), nil, t0.Add(8*time.Second))
			m.ApplyAgents(agent("idle"), nil, t0.Add(12*time.Second))
		}, 13 * time.Second, []string{"a1"}},
		{"session gone clears them", func(m *Model) {
			m.ApplyAgents(agent("busy"), nil, t0)
			m.ApplyHook(start("a1", "builder"), t0)
			m.ApplyAgents(nil, nil, t0.Add(2*time.Second))
		}, 3 * time.Second, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(testConfig(t), "/repo", t0)
			m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
			tc.steps(m)
			var got []string
			if s := m.sessions["s"]; s != nil {
				for id := range s.Subagents {
					got = append(got, id)
				}
			}
			sort.Strings(got)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("running %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReducerCostCoversTrackedSessionsOnly(t *testing.T) {
	t.Parallel()
	cwd := "/repo"
	cost := func(v float64) signals.StatusPayload {
		p := signals.StatusPayload{SessionID: "old", Cwd: cwd}
		p.Cost.TotalCostUSD = &v
		return p
	}
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	m.ApplyStatus(cost(3), t0)
	later := signals.StatusPayload{SessionID: "new", Cwd: cwd}
	two := 2.0
	later.Cost.TotalCostUSD = &two
	m.ApplyStatus(later, t0.Add(40*time.Minute))
	m.ApplyAgents(nil, nil, t0.Add(40*time.Minute)) // "old" is 40 min silent: forgotten
	v := m.Snapshot(t0.Add(40 * time.Minute))
	if v.EstCostUSD == nil || *v.EstCostUSD != 2 {
		t.Fatalf("est $ %v, want only the tracked session's 2", v.EstCostUSD)
	}
}

const buildWT = "/repo/.claude/worktrees/build-add-feature"

func v2Model(t *testing.T) *Model {
	t.Helper()
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	return m
}

func notifEv(typ string) signals.HookEvent {
	return signals.HookEvent{SessionID: "s1", Cwd: buildWT, Event: "Notification", NotificationType: typ, Message: "m"}
}

func TestNotificationEffects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ         string
		wantStatus  string
		wantNeeds   int
		wantDone    int
		wantUnknown bool
	}{
		{"permission_prompt", "waiting", 1, 0, false},
		{"elicitation_dialog", "waiting", 1, 0, false},
		{"elicitation_url_dialog", "waiting", 1, 0, false},
		{"agent_needs_input", "waiting", 1, 0, false},
		{"idle_prompt", "idle", 0, 1, false},
		// Done shows only once the turn is over: while the main session is still
		// busy, a completed agent is not yet your move.
		{"agent_completed", "busy", 0, 0, false},
		{"auth_success", "busy", 0, 0, false},
		{"quota_auto_resume_fired", "busy", 0, 0, false},
		{"quota_auto_resume_stale", "busy", 1, 0, false},
		{"quota_auto_resume_disabled", "busy", 1, 0, false},
		// Unknown: shown, never waiting, never Needs you.
		{"brand_new_type_2_2", "busy", 0, 0, true},
		{"", "busy", 0, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.typ, func(t *testing.T) {
			m := v2Model(t)
			m.ApplyHook(signals.HookEvent{SessionID: "s1", Cwd: buildWT, Event: "UserPromptSubmit"}, t0)
			m.ApplyHook(notifEv(tc.typ), t0.Add(time.Second))
			v := m.Snapshot(t0.Add(2 * time.Second))
			l := laneByName(v, "add-feature")
			if l == nil {
				t.Fatal("lane missing")
			}
			if l.Status != tc.wantStatus {
				t.Errorf("status %q, want %q", l.Status, tc.wantStatus)
			}
			if len(v.NeedsYou) != tc.wantNeeds || len(v.Done) != tc.wantDone {
				t.Errorf("needs %d done %d, want %d %d", len(v.NeedsYou), len(v.Done), tc.wantNeeds, tc.wantDone)
			}
			if got := l.Sessions[0].Unknown != ""; got != tc.wantUnknown {
				t.Errorf("unknown shown = %v, want %v", got, tc.wantUnknown)
			}
			if tc.wantUnknown && v.Observe.UnknownNotifications != 1 {
				t.Errorf("unknown not counted: %d", v.Observe.UnknownNotifications)
			}
		})
	}
}

func TestAgentCompletedIsDoneOnceIdle(t *testing.T) {
	t.Parallel()
	m := v2Model(t)
	m.ApplyHook(signals.HookEvent{SessionID: "s1", Cwd: buildWT, Event: "Stop"}, t0)
	m.ApplyHook(notifEv("agent_completed"), t0.Add(time.Second))
	v := m.Snapshot(t0.Add(2 * time.Second))
	if len(v.Done) != 1 || len(v.NeedsYou) != 0 || v.Done[0].Severity != signals.SevInfo {
		t.Fatalf("done %+v needs %+v", v.Done, v.NeedsYou)
	}
}

func TestElicitationCompleteAnswersTheDialog(t *testing.T) {
	t.Parallel()
	m := v2Model(t)
	m.ApplyHook(notifEv("elicitation_dialog"), t0)
	m.ApplyHook(notifEv("elicitation_complete"), t0.Add(time.Second))
	v := m.Snapshot(t0.Add(2 * time.Second))
	if len(v.NeedsYou) != 0 || laneByName(v, "add-feature").Status == "waiting" {
		t.Fatalf("still waiting: %+v", v.NeedsYou)
	}
}

// C2: the new events are subscribed and applied; PermissionRequest only observed.
func TestV2HookEvents(t *testing.T) {
	t.Parallel()
	for _, ev := range []string{"StopFailure", "PermissionRequest", "PreCompact", "PostCompact", "CwdChanged"} {
		found := false
		for _, e := range signals.HookEvents {
			found = found || e == ev
		}
		if !found {
			t.Errorf("%s is not subscribed", ev)
		}
	}
	for _, e := range signals.HookEvents {
		if e == "SessionStart" {
			t.Error("SessionStart supports no HTTP hooks and must stay out")
		}
	}
	m := v2Model(t)
	m.ApplyHook(signals.HookEvent{SessionID: "s1", Cwd: buildWT, Event: "PermissionRequest", ToolName: "Bash"}, t0)
	v := m.Snapshot(t0)
	if len(v.NeedsYou) != 1 || !strings.Contains(v.NeedsYou[0].Text, "Bash") || v.NeedsYou[0].Severity != signals.SevBlock {
		t.Fatalf("permission request: %+v", v.NeedsYou)
	}
	m.ApplyHook(signals.HookEvent{SessionID: "s1", Cwd: buildWT, Event: "PreCompact", CompactionTrigger: "auto"}, t0)
	if c := laneByName(m.Snapshot(t0), "add-feature").Sessions[0].Compacting; c != "auto" {
		t.Fatalf("compacting %q", c)
	}
	m.ApplyHook(signals.HookEvent{SessionID: "s1", Cwd: buildWT, Event: "PostCompact", CompactionTrigger: "auto"}, t0)
	m.ApplyHook(signals.HookEvent{SessionID: "s1", Cwd: buildWT, Event: "StopFailure", ErrorType: "rate_limit"}, t0.Add(time.Second))
	v = m.Snapshot(t0.Add(2 * time.Second))
	s := laneByName(v, "add-feature").Sessions[0]
	if s.Compacting != "" || s.Failure != "rate_limit" {
		t.Fatalf("after PostCompact/StopFailure: %+v", s)
	}
	found := false
	for _, a := range v.Alerts {
		found = found || (a.Kind == AlertRateLimit && a.Severity == signals.SevBlock)
	}
	if !found {
		t.Fatalf("StopFailure rate_limit raised no alert: %+v", v.Alerts)
	}
	// An event name the panel does not subscribe to is counted, not applied.
	m.ApplyHook(signals.HookEvent{SessionID: "s1", Cwd: buildWT, Event: "PostToolUseFailure"}, t0)
	if v := m.Snapshot(t0); v.Observe.DroppedUnknownEvent != 1 || v.Observe.DroppedForeign != 0 {
		t.Fatalf("unknown event: %+v", v.Observe)
	}
}

// C4: a session is bound once; the panel's own session id wins over cwd.
func TestSessionBindingByID(t *testing.T) {
	t.Parallel()
	m := v2Model(t)
	// A session the panel did not launch binds at first sight by cwd, and a later
	// `cd` into another worktree does not move it.
	m.ApplyHook(signals.HookEvent{SessionID: "own", Cwd: buildWT, Event: "UserPromptSubmit"}, t0)
	m.ApplyHook(signals.HookEvent{SessionID: "own", Cwd: "/repo", Event: "Stop"}, t0.Add(time.Second))
	m.ApplyHook(signals.HookEvent{SessionID: "own", Cwd: "/repo", Event: "CwdChanged", PreviousCwd: buildWT}, t0.Add(time.Second))
	v := m.Snapshot(t0.Add(2 * time.Second))
	if l := laneByName(v, "add-feature"); l == nil || len(l.Sessions) != 1 {
		t.Fatalf("session moved lanes: %+v", v.Lanes)
	}
	if l := laneByName(v, "main"); l != nil && len(l.Sessions) != 0 {
		t.Fatalf("main got the session: %+v", l)
	}
	// Its events are attributed to the bound lane too (feed, last hook), not to main.
	for _, e := range v.Feed {
		if e.Session == "own" && e.Lane != buildWT {
			t.Fatalf("event %s filed under %s", e.Event, e.Lane)
		}
	}
	if l := laneByName(v, "main"); l != nil && l.LastHookAt != 0 {
		t.Fatalf("main's last hook moved: %+v", l)
	}
	// A panel-launched session is bound by its registry session id, even when its
	// first event comes from elsewhere (it ran `cd /tmp` before the first hook).
	rec := types.LaneRecord{ID: "fixer", SessionID: "launched", Path: "/repo/.claude/worktrees/fix-thing", Type: "fix", ActionDone: true}
	m.ApplyTmux(nil, []types.LaneRecord{rec}, "", nil, t0)
	if !m.ApplyHook(signals.HookEvent{SessionID: "launched", Cwd: "/tmp", Event: "UserPromptSubmit"}, t0) {
		t.Fatal("a registry-bound session's event was dropped for its cwd")
	}
	v = m.Snapshot(t0)
	if l := laneByName(v, "thing"); l == nil || len(l.Sessions) != 1 || l.Sessions[0].ID != "launched" {
		t.Fatalf("launched session not in its lane: %+v", v.Lanes)
	}
	// A stranger from outside the project is still dropped, and counted as foreign.
	if m.ApplyHook(signals.HookEvent{SessionID: "stranger", Cwd: "/elsewhere", Event: "Stop"}, t0) {
		t.Fatal("foreign session kept")
	}
}

// C5: a quota window expires at resets_at.
func TestQuotaExpiresAtResetsAt(t *testing.T) {
	t.Parallel()
	m := v2Model(t)
	five, seven := 80.0, 40.0
	r5, r7 := t0.Add(time.Hour).Unix(), t0.Add(72*time.Hour).Unix()
	p := signals.StatusPayload{SessionID: "s1", Cwd: buildWT}
	p.RateLimits.FiveHour = &signals.RateLimit{UsedPercentage: &five, ResetsAt: &r5}
	p.RateLimits.SevenDay = &signals.RateLimit{UsedPercentage: &seven, ResetsAt: &r7}
	m.ApplyStatus(p, t0)
	if q := m.Snapshot(t0.Add(59 * time.Minute)).Quota; q.FiveHour == nil || *q.FiveHour != 80 || q.FiveHourExpired {
		t.Fatalf("before reset: %+v", q)
	}
	q := m.Snapshot(t0.Add(time.Hour)).Quota
	if q.FiveHour != nil || !q.FiveHourExpired || q.SevenDay == nil || *q.SevenDay != 40 {
		t.Fatalf("at reset the 5-hour window must drop and the 7-day stay: %+v", q)
	}
	// And a guard reads the expired window as unknown, never as 80%.
	if why := quotaGuardBlock(q, 50); why != "" {
		t.Fatalf("guard on an expired window: %q", why)
	}
	if why := quotaGuardBlock(m.Snapshot(t0).Quota, 50); why == "" {
		t.Fatal("guard ignores a live 80% window at 50%")
	}
}

// C6: `claude agents` id, state and the waitingFor enum are decoded.
func TestAgentsDecodeIDStateWaitingFor(t *testing.T) {
	t.Parallel()
	agents, err := signals.ParseAgents([]byte(`[{"pid":1,"cwd":"` + buildWT + `","kind":"background","sessionId":"bg","id":"a1b2","state":"running","status":"waiting","waitingFor":"permission prompt"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if agents[0].ID != "a1b2" || agents[0].State != "running" {
		t.Fatalf("decoded %+v", agents[0])
	}
	m := v2Model(t)
	m.ApplyAgents(agents, nil, t0)
	v := m.Snapshot(t0)
	s := laneByName(v, "add-feature").Sessions[0]
	if s.AgentID != "a1b2" || s.AgentState != "running" || s.WaitingKind != "permission" {
		t.Fatalf("view %+v", s)
	}
	if len(v.NeedsYou) != 1 || v.NeedsYou[0].Label != "Permission" {
		t.Fatalf("needs %+v", v.NeedsYou)
	}
	for in, want := range map[string]string{"permission prompt": "permission", "input needed": "input", "sandbox request": "sandbox",
		"worker request": "worker", "dialog open": "dialog", "Permission_Prompt": "permission", "something new": "other", "": ""} {
		if got := signals.WaitingForKind(in); got != want {
			t.Errorf("waitingForKind(%q) = %q, want %q", in, got, want)
		}
	}
}

// C14: subagent heuristics are tagged with the version they were verified on.
func TestHeuristicsApproximateOnOtherVersion(t *testing.T) {
	t.Parallel()
	m := v2Model(t)
	m.ApplyHook(signals.HookEvent{SessionID: "s1", Cwd: buildWT, Event: "SubagentStart", AgentID: "a", AgentType: "builder"}, t0)
	m.ApplyClaudeVersion(HeuristicsVerifiedOn, nil, t0)
	v := m.Snapshot(t0)
	if laneByName(v, "add-feature").SubagentsApprox || len(v.Warnings) != 0 {
		t.Fatalf("verified version flagged approximate: %+v", v.Warnings)
	}
	m.ApplyClaudeVersion("2.2.0", nil, t0)
	v = m.Snapshot(t0)
	if !laneByName(v, "add-feature").SubagentsApprox || len(v.Warnings) != 1 || !strings.Contains(v.Warnings[0], "2.2.0") {
		t.Fatalf("other version not flagged: %+v", v.Warnings)
	}
	if v.Observe.ClaudeVersion != "2.2.0" || v.Observe.VerifiedOn != "2.1.284" {
		t.Fatalf("observe %+v", v.Observe)
	}
	if got, _ := signals.ParseClaudeVersion([]byte("2.1.284 (Claude Code)\n")); got != "2.1.284" {
		t.Fatalf("parse %q", got)
	}
	if _, err := signals.ParseClaudeVersion([]byte("command not found")); err == nil {
		t.Fatal("garbage parsed")
	}
}
