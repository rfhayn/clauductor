package state

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// viewKey ignores the clock and the polls' bookkeeping, and nothing else.
func TestViewKeyIgnoresThePollsBookkeeping(t *testing.T) {
	m := NewModel(testConfig(t), "/repo", t0)
	a := m.Snapshot(t0)
	b := a
	b.Now += 5000
	if ViewKey(a) != ViewKey(b) {
		t.Fatal("two views that differ only in now have different keys")
	}
	// The polls' bookkeeping is not news; it reaches the page on the tick (fullKey).
	b.HookEvents++
	b.AgentsReadAt += 2000
	b.Sources = map[string]SourceStatus{"agents": {OK: true, At: 99}}
	a.Sources = map[string]SourceStatus{"agents": {OK: true, At: 1}}
	if ViewKey(a) != ViewKey(b) {
		t.Fatal("poll bookkeeping changed the view key")
	}
	if FullKey(a) == FullKey(b) {
		t.Fatal("poll bookkeeping did not change the full key")
	}
	b.PRs = append(b.PRs, signals.PR{Number: 7})
	if ViewKey(a) == ViewKey(b) {
		t.Fatal("a changed view has the same key")
	}
	b.PRs = a.PRs
	b.Sources = map[string]SourceStatus{"agents": {OK: false, Error: "exit 1", At: 99}}
	if ViewKey(a) == ViewKey(b) {
		t.Fatal("a source that fails has the same key")
	}
}

// Needs you shows the specific ask `claude agents` names, not the hook's generic
// message (audit P2-6).
func TestNeedsYouPrefersTheSpecificWaitingFor(t *testing.T) {
	m := alertModel(t, "")
	m.ApplyAgents(waitingAgent("waiting", "permission: Bash(npm run test:e2e)"), nil, t0)
	m.ApplyHook(permissionHook(), t0)
	nd := blockedNeeds(m.Snapshot(t0.Add(time.Second)))
	// Under the label Permission, the text is the call itself.
	if len(nd) != 1 || nd[0].Label != "Permission" || nd[0].Text != "Bash(npm run test:e2e)" {
		t.Fatalf("Needs you: %+v", nd)
	}
	// Without a specific reading, the hook's message is what there is.
	m2 := alertModel(t, "")
	m2.ApplyAgents(waitingAgent("busy", ""), nil, t0)
	m2.ApplyHook(permissionHook(), t0)
	if nd := blockedNeeds(m2.Snapshot(t0.Add(time.Second))); len(nd) != 1 || nd[0].Text != "Claude needs your permission to use Bash" {
		t.Fatalf("Needs you without a reading: %+v", nd)
	}
}

// A lane with a terminal has one name everywhere: the one it was started with
// (audit P2-12). Its card, Needs you and alerts all say it.
func TestALaneWithATerminalHasOneName(t *testing.T) {
	m := alertModel(t, `,"alerts":{"waiting_seconds":1}`)
	rec := lanes.LaneRecord{ID: "orchestrator", SessionID: "s1", Path: xWT, Type: "build", ActionDone: true}
	m.ApplyTmux([]lanes.TmuxLane{{ID: "orchestrator", Path: xWT}}, []lanes.LaneRecord{rec}, "", nil, t0)
	m.ApplyAgents(waitingAgent("waiting", "permission: Bash(ls)"), nil, t0)
	m.ApplyAgents(waitingAgent("waiting", "permission: Bash(ls)"), nil, t0.Add(5*time.Second))
	v := m.Snapshot(t0.Add(6 * time.Second))
	if laneByName(v, "orchestrator") == nil || laneByName(v, "x") != nil {
		t.Fatalf("lane names: %+v", v.Lanes)
	}
	for _, n := range blockedNeeds(v) {
		if n.Name != "orchestrator" {
			t.Fatalf("Needs you names the lane %q", n.Name)
		}
	}
	a := waitingAlerts(v)
	if len(a) != 1 || a[0].Name != "orchestrator" {
		t.Fatalf("alerts: %+v", a)
	}
	m.ApplyHook(permissionHook(), t0.Add(7*time.Second))
	v = m.Snapshot(t0.Add(8 * time.Second))
	if len(v.Feed) == 0 || v.Feed[0].Name != "orchestrator" {
		t.Fatalf("feed: %+v", v.Feed)
	}
}

// A view that only gets older is the same view: no text in it carries an age, so
// the hub does not push it (audit P1-3). The page computes every age itself.
func TestAViewThatOnlyAgesDoesNotChange(t *testing.T) {
	m := alertModel(t, `,"alerts":{"waiting_seconds":1,"idle_minutes":1}`)
	m.ApplyAgents(waitingAgent("waiting", "permission: Bash(ls)"), nil, t0)
	m.ApplyHook(permissionHook(), t0)
	// The poll then fails: the item is approximate, and says why.
	m.ApplyAgents(nil, errors.New("claude agents: exit 1"), t0.Add(10*time.Second))
	a, b := m.Snapshot(t0.Add(70*time.Second)), m.Snapshot(t0.Add(130*time.Second))
	if len(waitingAlerts(a)) != 1 || len(blockedNeeds(a)) != 1 || !blockedNeeds(a)[0].Approx {
		t.Fatalf("premise: an approximate waiting item with its alert: %+v %+v", a.NeedsYou, a.Alerts)
	}
	if ViewKey(a) != ViewKey(b) {
		t.Fatalf("the view changed with the clock alone:\n%+v\n%+v", a.Alerts, b.Alerts)
	}
	if a.AgentsReadAt != ms(t0) {
		t.Fatalf("agentsReadAt %d, want %d", a.AgentsReadAt, ms(t0))
	}
	// The notification still says how long.
	if got := noticeLine(waitingAlerts(a)[0], t0.Add(3*time.Minute)); !strings.HasPrefix(got, "waiting on you for 3m: ") {
		t.Fatalf("notice line %q", got)
	}
}

// Each banner carries its kind, so the page labels it (audit P2-9).
func TestBannersCarryTheirKind(t *testing.T) {
	m := alertModel(t, "")
	m.ApplyTmux(nil, []lanes.LaneRecord{{ID: "gone", SessionID: "s9", Path: xWT, Type: "build", ActionDone: true}}, "", nil, t0)
	v := m.Snapshot(t0)
	if len(v.BannerItems) != len(v.Banners) {
		t.Fatalf("%d banner items for %d banners", len(v.BannerItems), len(v.Banners))
	}
	found := false
	for i, b := range v.BannerItems {
		if b.Text != v.Banners[i] || b.Kind == "" {
			t.Fatalf("banner %d: %+v vs %q", i, b, v.Banners[i])
		}
		found = found || b.Kind == BannerRestore
	}
	if !found {
		t.Fatalf("no restore banner: %+v", v.BannerItems)
	}
}
