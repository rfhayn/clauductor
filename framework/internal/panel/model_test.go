package panel

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
)

var t0 = time.Date(2026, 9, 28, 14, 0, 0, 0, time.UTC)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := ParseConfig(fixture(t, "panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// spikeHooks returns the hook payloads Claude Code 2.1.284 actually sent in the spike.
func spikeHooks(t *testing.T) []HookEvent {
	t.Helper()
	var evs []HookEvent
	sc := bufio.NewScanner(bytes.NewReader(fixture(t, "spike-hooks.log")))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		ev, err := ParseHook([]byte(line))
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

func fixtureWorktrees(t *testing.T) []Worktree {
	t.Helper()
	wts, err := ParseWorktreePorcelain(fixture(t, "worktrees-fixture.porcelain"))
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
			m.ApplyWorktrees([]Worktree{{Path: spikeCwd, Branch: "main"}}, nil, t0)
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
	tests := []struct {
		cwd      string
		wantLane string // "" = dropped
	}{
		{"/repo", "main"},
		{"/repo/apps/web", "main"},
		// Linked worktrees live inside the main checkout: the longest match must win.
		{"/repo/.claude/worktrees/build-add-support-access", "add-support-access"},
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
			kept := m.ApplyHook(HookEvent{SessionID: "s1", Cwd: tc.cwd, Event: "UserPromptSubmit", Prompt: "hi"}, t0)
			st := m.ApplyStatus(StatusPayload{SessionID: "s1", Cwd: tc.cwd}, t0)
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
	sp, err := ParseStatus(fixture(t, "statusline.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(testConfig(t), sp.Cwd, t0)
	m.ApplyWorktrees([]Worktree{{Path: sp.Cwd, Branch: "main"}}, nil, t0)
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
	partial.RateLimits.SevenDay = &RateLimit{UsedPercentage: &seven}
	m.ApplyStatus(partial, t0.Add(time.Second))
	if q := m.Snapshot(t0).Quota; q.FiveHour == nil || *q.FiveHour != 12 || *q.SevenDay != 71 {
		t.Fatalf("partial quota post: %+v", q)
	}
	// A status post from another project must not move the quota or the cost.
	foreign := sp
	foreign.Cwd = "/elsewhere"
	hi := 99.0
	foreign.RateLimits.FiveHour = &RateLimit{UsedPercentage: &hi}
	if m.ApplyStatus(foreign, t0) {
		t.Fatal("foreign status kept")
	}
	if v := m.Snapshot(t0); *v.Quota.FiveHour != 12 {
		t.Fatal("foreign status moved the quota")
	}
}

func TestReducerAgentsPoll(t *testing.T) {
	agents, err := ParseAgents(fixture(t, "agents.json"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	m.ApplyAgents(agents, nil, t0)
	v := m.Snapshot(t0)
	want := map[string][2]string{ // name → {type, status}
		"main":               {"orchestrator", "idle"},
		"add-support-access": {"build", "busy"},
		"thing":              {"fix", "waiting"},
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
	if v.Sources["agents"].OK || v.Sources["agents"].Error == "" || laneByName(v, "add-support-access") == nil {
		t.Fatalf("failed poll handled wrongly: %+v", v.Sources["agents"])
	}
}

func TestReducerRealAgentsCapture(t *testing.T) {
	agents, err := ParseAgents(fixture(t, "agents-real.json"))
	if err != nil {
		t.Fatal(err)
	}
	wts, err := ParseWorktreePorcelain(fixture(t, "worktrees-real.porcelain"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(testConfig(t), wts[0].Path, t0)
	m.ApplyWorktrees(wts, nil, t0)
	m.ApplyAgents(agents, nil, t0)
	v := m.Snapshot(t0)
	if laneByName(v, "main") == nil || laneByName(v, "add-support-access") == nil {
		t.Fatalf("real capture: lanes %+v", v.Lanes)
	}
}

func TestReducerNeedsYou(t *testing.T) {
	notif := func(kind string) HookEvent {
		return HookEvent{SessionID: "sess-build", Cwd: "/repo/.claude/worktrees/build-add-support-access",
			Event: "Notification", NotificationType: kind, Message: "Claude needs your permission to use Bash"}
	}
	busy := []Agent{{SessionID: "sess-build", Cwd: "/repo/.claude/worktrees/build-add-support-access", Status: "busy"}}
	waiting := []Agent{{SessionID: "sess-build", Cwd: "/repo/.claude/worktrees/build-add-support-access", Status: "waiting"}}
	tests := []struct {
		name  string
		steps func(m *Model)
		want  int
	}{
		{"permission prompt shows", func(m *Model) { m.ApplyHook(notif("permission_prompt"), t0) }, 1},
		{"idle prompt shows", func(m *Model) { m.ApplyHook(notif("idle_prompt"), t0) }, 1},
		{"other notification types stay in the feed only", func(m *Model) { m.ApplyHook(notif("auth_success"), t0) }, 0},
		{"a later prompt answers it", func(m *Model) {
			m.ApplyHook(notif("idle_prompt"), t0)
			m.ApplyHook(HookEvent{SessionID: "sess-build", Cwd: "/repo/.claude/worktrees/build-add-support-access", Event: "UserPromptSubmit"}, t0.Add(time.Second))
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
	cwd := "/repo/.claude/worktrees/build-add-support-access"
	agent := func(status string) []Agent {
		return []Agent{{SessionID: "s", Cwd: cwd, Status: status}}
	}
	hook := HookEvent{SessionID: "s", Cwd: cwd, Event: "SubagentStart", AgentID: "a1", AgentType: "builder"}
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
			l := laneByName(v, "add-support-access")
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
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	for i := 0; i < FeedCap+50; i++ {
		m.ApplyHook(HookEvent{SessionID: "s", Cwd: "/repo", Event: "Stop", LastAssistantMessage: strings.Repeat("x", 500)}, t0.Add(time.Duration(i)*time.Second))
	}
	v := m.Snapshot(t0.Add(time.Hour))
	if len(v.Feed) != FeedCap {
		t.Fatalf("feed len %d", len(v.Feed))
	}
	if v.Feed[0].At <= v.Feed[1].At {
		t.Fatal("feed not newest-first")
	}
	if n := len([]rune(v.Feed[0].Detail)); n > detailMax+1 {
		t.Fatalf("detail not truncated: %d runes", n)
	}
}

func TestReducerSourcesNeverReadAsEmptySuccess(t *testing.T) {
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
	prs, err := ParsePRs(fixture(t, "prs.json"))
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
	m.ApplyCard("founder-queue", nil, errors.New("exit status 1"), t0)
	if c := m.Snapshot(t0).Cards[0]; c.Source.OK || c.Source.Error == "" {
		t.Fatalf("card error not shown: %+v", c)
	}
}

func TestReducerSubagentLifecycle(t *testing.T) {
	cwd := "/repo/.claude/worktrees/build-add-support-access"
	start := func(id, typ string) HookEvent {
		return HookEvent{SessionID: "s", Cwd: cwd, Event: "SubagentStart", AgentID: id, AgentType: typ}
	}
	stop := func(id, typ string) HookEvent {
		return HookEvent{SessionID: "s", Cwd: cwd, Event: "SubagentStop", AgentID: id, AgentType: typ}
	}
	agent := func(status string) []Agent { return []Agent{{SessionID: "s", Cwd: cwd, Status: status}} }
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
			m.ApplyHook(HookEvent{SessionID: "s", Cwd: cwd, Event: "Stop"}, t0.Add(time.Second))
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
	cwd := "/repo"
	cost := func(v float64) StatusPayload {
		p := StatusPayload{SessionID: "old", Cwd: cwd}
		p.Cost.TotalCostUSD = &v
		return p
	}
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	m.ApplyStatus(cost(3), t0)
	later := StatusPayload{SessionID: "new", Cwd: cwd}
	two := 2.0
	later.Cost.TotalCostUSD = &two
	m.ApplyStatus(later, t0.Add(40*time.Minute))
	m.ApplyAgents(nil, nil, t0.Add(40*time.Minute)) // "old" is 40 min silent: forgotten
	v := m.Snapshot(t0.Add(40 * time.Minute))
	if v.EstCostUSD == nil || *v.EstCostUSD != 2 {
		t.Fatalf("est $ %v, want only the tracked session's 2", v.EstCostUSD)
	}
}
