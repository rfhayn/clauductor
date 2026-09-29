package panel

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// PANEL-5 (state correctness). A `claude agents` reading is a snapshot of one poll.
// Once polls fail or stop, it says what WAS true, and the panel must not keep
// presenting it as current: Needs you marks it approximate, and an alert raised
// from it never becomes an OS notification.

const xWT = "/repo/w/x"

func waitingAgent(status, waitingFor string) []Agent {
	return []Agent{{SessionID: "s1", Cwd: xWT, Status: status, WaitingFor: waitingFor}}
}

func permissionHook() HookEvent {
	return HookEvent{SessionID: "s1", Cwd: xWT, Event: "Notification", NotificationType: "permission_prompt",
		Message: "Claude needs your permission to use Bash"}
}

// blockedNeeds returns the Needs-you items of s1 that say it is blocked waiting on
// you (not the quota warnings, not first-prompt items).
func blockedNeeds(v View) []NeedView {
	var out []NeedView
	for _, n := range v.NeedsYou {
		if n.Session == "s1" && (n.Kind == "waiting" || ClassifyNotification(n.Kind).Waiting) {
			out = append(out, n)
		}
	}
	return out
}

func waitingAlerts(v View) []AlertView {
	var out []AlertView
	for _, a := range v.Alerts {
		if a.Session == "s1" && a.Kind == AlertWaiting {
			out = append(out, a)
		}
	}
	return out
}

func TestFailedPollMakesWaitingApproximateAndSilent(t *testing.T) {
	m := alertModel(t, `,"alerts":{"waiting_seconds":60}`)
	m.ApplyAgents(waitingAgent("waiting", "permission prompt"), nil, t0)
	// A current reading, past the threshold: Needs you and the alert are exact, and
	// the alert interrupts.
	m.ApplyAgents(waitingAgent("waiting", "permission prompt"), nil, t0.Add(61*time.Second))
	v := m.Snapshot(t0.Add(62 * time.Second))
	if n := blockedNeeds(v); len(n) != 1 || n[0].Approx {
		t.Fatalf("fresh waiting: needs %+v", n)
	}
	a := waitingAlerts(v)
	if len(a) != 1 || a[0].Approx {
		t.Fatalf("fresh waiting: alerts %+v", a)
	}
	n := &Notifier{MinInterval: time.Minute}
	if out := n.Process(v.Alerts, nil, t0.Add(62*time.Second)); len(out) != 1 {
		t.Fatalf("premise: a current waiting reading notifies: %+v", out)
	}

	// The prompt is answered in the terminal (no hook fires for that), and the next
	// poll fails. The last reading still says waiting; it is no longer current.
	m.ApplyAgents(nil, errors.New("claude agents: exit 1"), t0.Add(64*time.Second))
	v = m.Snapshot(t0.Add(65 * time.Second))
	nd := blockedNeeds(v)
	if len(nd) != 1 || !nd[0].Approx || !strings.Contains(nd[0].Label, "stale") {
		t.Fatalf("after a failed poll Needs you must mark the item stale: %+v", nd)
	}
	a = waitingAlerts(v)
	if len(a) != 1 || !a[0].Approx {
		t.Fatalf("after a failed poll the alert must be approximate: %+v", a)
	}
	// A notifier that has not yet notified this stretch (a restart, or a threshold
	// crossed only now) must not raise an OS notification from stale data.
	if out := (&Notifier{MinInterval: time.Minute}).Process(v.Alerts, nil, t0.Add(65*time.Second)); len(out) != 0 {
		t.Fatalf("an OS notification from a stale reading: %+v", out)
	}
	// The notifier that did notify keeps its mark while the data is stale, so the
	// poll recovering on the same stretch does not notify a second time.
	if out := n.Process(v.Alerts, nil, t0.Add(65*time.Second)); len(out) != 0 {
		t.Fatalf("stale: %+v", out)
	}
	m.ApplyAgents(waitingAgent("waiting", "permission prompt"), nil, t0.Add(66*time.Second))
	if out := n.Process(m.Snapshot(t0.Add(67*time.Second)).Alerts, nil, t0.Add(3*time.Minute)); len(out) != 0 {
		t.Fatalf("re-notified the same stretch after the poll recovered: %+v", out)
	}

	// A reading that simply stops arriving (no error, just old) is stale too.
	m2 := alertModel(t, `,"alerts":{"waiting_seconds":60}`)
	m2.ApplyAgents(waitingAgent("waiting", "permission prompt"), nil, t0)
	v = m2.Snapshot(t0.Add(2 * time.Minute))
	if a := waitingAlerts(v); len(a) != 1 || !a[0].Approx {
		t.Fatalf("a two-minute-old reading raised an exact alert: %+v", a)
	}
	if out := (&Notifier{MinInterval: time.Minute}).Process(v.Alerts, nil, t0.Add(2*time.Minute)); len(out) != 0 {
		t.Fatalf("an OS notification from an old reading: %+v", out)
	}
}

// Needs you and the waiting alert answer the same question, "is this session
// blocked on you, since when, and how sure are we", from ONE predicate. Every case
// drives both surfaces from the same inputs and they must agree.
func TestNeedsYouAndAlertsAgree(t *testing.T) {
	type want struct {
		blocked, approx bool
	}
	cases := []struct {
		name  string
		steps func(m *Model)
		at    time.Duration
		want  want
	}{
		{"nothing", func(m *Model) {}, 5 * time.Second, want{}},
		{"fresh poll says waiting", func(m *Model) {
			m.ApplyAgents(waitingAgent("waiting", "permission prompt"), nil, t0)
		}, 5 * time.Second, want{true, false}},
		{"hook permission, fresh poll agrees", func(m *Model) {
			m.ApplyHook(permissionHook(), t0)
			m.ApplyAgents(waitingAgent("waiting", "permission prompt"), nil, t0)
		}, 5 * time.Second, want{true, false}},
		{"hook permission, never polled", func(m *Model) {
			m.ApplyHook(permissionHook(), t0)
		}, 5 * time.Second, want{true, true}},
		{"hook permission, then the poll says busy (answered in the terminal)", func(m *Model) {
			m.ApplyHook(permissionHook(), t0)
			m.ApplyAgents(waitingAgent("busy", ""), nil, t0.Add(2*time.Second))
		}, 5 * time.Second, want{}},
		{"poll waiting, then a failed poll", func(m *Model) {
			m.ApplyAgents(waitingAgent("waiting", "permission prompt"), nil, t0)
			m.ApplyAgents(nil, errors.New("claude agents: exit 1"), t0.Add(2*time.Second))
		}, 5 * time.Second, want{true, true}},
		{"poll waiting, then silence", func(m *Model) {
			m.ApplyAgents(waitingAgent("waiting", "permission prompt"), nil, t0)
		}, 2 * time.Minute, want{true, true}},
		{"hook permission, fresh poll says idle", func(m *Model) {
			m.ApplyHook(permissionHook(), t0)
			m.ApplyAgents(waitingAgent("idle", ""), nil, t0.Add(time.Second))
		}, 5 * time.Second, want{true, true}},
		{"hook permission, fresh poll does not list the session", func(m *Model) {
			m.ApplyHook(permissionHook(), t0)
			m.ApplyAgents([]Agent{}, nil, t0.Add(time.Second))
		}, 5 * time.Second, want{true, true}},
		{"elicitation answered", func(m *Model) {
			m.ApplyHook(HookEvent{SessionID: "s1", Cwd: xWT, Event: "Notification", NotificationType: "elicitation_dialog"}, t0)
			m.ApplyHook(HookEvent{SessionID: "s1", Cwd: xWT, Event: "Notification", NotificationType: "elicitation_complete"}, t0.Add(time.Second))
		}, 5 * time.Second, want{}},
		{"quota warning is not a waiting item", func(m *Model) {
			m.ApplyAgents(waitingAgent("idle", ""), nil, t0)
			m.ApplyHook(HookEvent{SessionID: "s1", Cwd: xWT, Event: "Notification", NotificationType: "quota_auto_resume_stale"}, t0)
		}, 5 * time.Second, want{}},
		// Before PANEL-5 the quota note took the session's one Needs-you slot, so the
		// waiting item vanished from Needs you while the alert still fired.
		{"quota warning while the poll says waiting", func(m *Model) {
			m.ApplyHook(HookEvent{SessionID: "s1", Cwd: xWT, Event: "Notification", NotificationType: "quota_auto_resume_stale"}, t0)
			m.ApplyAgents(waitingAgent("waiting", "permission prompt"), nil, t0)
		}, 5 * time.Second, want{true, false}},
		{"waiting with no waitingFor", func(m *Model) {
			m.ApplyAgents(waitingAgent("waiting", ""), nil, t0)
		}, 5 * time.Second, want{true, false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := alertModel(t, `,"alerts":{"waiting_seconds":1}`)
			tc.steps(m)
			v := m.Snapshot(t0.Add(tc.at))
			needs, alerts := blockedNeeds(v), waitingAlerts(v)
			if (len(needs) == 1) != (len(alerts) == 1) || len(needs) > 1 || len(alerts) > 1 {
				t.Fatalf("the surfaces disagree: needs %+v, alerts %+v", needs, alerts)
			}
			if got := len(needs) == 1; got != tc.want.blocked {
				t.Fatalf("blocked %v, want %v: needs %+v", got, tc.want.blocked, needs)
			}
			if !tc.want.blocked {
				return
			}
			nd, al := needs[0], alerts[0]
			if nd.Approx != al.Approx || nd.At != al.Since {
				t.Fatalf("the surfaces disagree: need %+v, alert %+v", nd, al)
			}
			if nd.Approx != tc.want.approx {
				t.Fatalf("approx %v, want %v: %+v", nd.Approx, tc.want.approx, nd)
			}
			// The lane chip and Needs you read the same session status.
			l := laneByName(v, "x")
			if l == nil || l.Status != "waiting" {
				t.Fatalf("lane status disagrees with Needs you: %+v", l)
			}
		})
	}
}

// An approximate alert is shown, and never interrupts. It holds the mark of an
// alert already notified, so the data flickering stale and back never re-notifies.
func TestNotifierNeverInterruptsOnApproximateData(t *testing.T) {
	exact := AlertView{Key: "waiting:s", Kind: AlertWaiting, Severity: SevBlock, Terminal: "a", Text: "w", Since: t0.UnixMilli()}
	approx := exact
	approx.Approx = true
	if Interrupts(approx) {
		t.Fatal("an approximate alert interrupts")
	}
	n := &Notifier{MinInterval: time.Minute}
	if out := n.Process([]AlertView{approx}, nil, t0); len(out) != 0 {
		t.Fatalf("approximate alert notified: %+v", out)
	}
	if out := n.Process([]AlertView{exact}, nil, t0.Add(time.Second)); len(out) != 1 {
		t.Fatalf("the same condition, confirmed, did not notify: %+v", out)
	}
	if out := n.Process([]AlertView{approx}, nil, t0.Add(2*time.Minute)); len(out) != 0 {
		t.Fatalf("approx: %+v", out)
	}
	if out := n.Process([]AlertView{exact}, nil, t0.Add(4*time.Minute)); len(out) != 0 {
		t.Fatalf("re-notified after a stale flicker: %+v", out)
	}
}

// Sessions are forgotten even while `claude agents` keeps failing: a stale reading
// does not keep a silent session on the page forever.
func TestSessionsAreForgottenWhilePollsFail(t *testing.T) {
	m := alertModel(t, "")
	m.ApplyAgents(waitingAgent("busy", ""), nil, t0)
	for i := 1; i <= 40; i++ {
		m.ApplyAgents(nil, errors.New("claude agents: exit 1"), t0.Add(time.Duration(i)*time.Minute))
	}
	v := m.Snapshot(t0.Add(40 * time.Minute))
	if l := laneByName(v, "x"); l != nil {
		t.Fatalf("a session silent for 40 min is still shown from a stale reading: %+v", l)
	}
}
