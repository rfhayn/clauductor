package state

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

func TestAlertThresholds(t *testing.T) {
	t.Parallel()
	m := alertModel(t, `,"alerts":{"idle_minutes":10,"context_pct":80,"five_hour_pct":90,"waiting_seconds":60}`)
	cwd := "/repo/w/x"
	m.ApplyAgents([]signals.Agent{{SessionID: "s", Cwd: cwd, Status: "idle"}}, nil, t0)
	ctx, five := 79.0, 89.0
	p := signals.StatusPayload{SessionID: "s", Cwd: cwd}
	p.ContextWindow.UsedPercentage = &ctx
	p.RateLimits = signals.RateLimits{"five_hour": {UsedPercentage: &five}}
	m.ApplyStatus(p, t0)
	// Just under every threshold: nothing.
	m.ApplyAgents([]signals.Agent{{SessionID: "s", Cwd: cwd, Status: "idle"}}, nil, t0.Add(9*time.Minute))
	if v := m.Snapshot(t0.Add(9 * time.Minute)); len(v.Alerts) != 0 {
		t.Fatalf("under thresholds: %+v", v.Alerts)
	}
	ctx, five = 80, 90
	m.ApplyStatus(p, t0.Add(10*time.Minute))
	m.ApplyAgents([]signals.Agent{{SessionID: "s", Cwd: cwd, Status: "idle"}}, nil, t0.Add(10*time.Minute))
	k := alertKinds(m.Snapshot(t0.Add(10 * time.Minute)))
	if k[AlertIdle] != signals.SevInfo || k[AlertContext] != signals.SevWarn || k[AlertQuota] != signals.SevWarn {
		t.Fatalf("at thresholds: %+v", k)
	}
	// Waiting: a permission prompt older than 60 s.
	m.ApplyAgents([]signals.Agent{{SessionID: "s", Cwd: cwd, Status: "waiting", WaitingFor: "permission prompt"}}, nil, t0.Add(11*time.Minute))
	if k := alertKinds(m.Snapshot(t0.Add(11*time.Minute + 59*time.Second))); k[AlertWaiting] != "" {
		t.Fatalf("waiting fired early: %+v", k)
	}
	if k := alertKinds(m.Snapshot(t0.Add(12 * time.Minute))); k[AlertWaiting] != signals.SevBlock {
		t.Fatalf("waiting did not fire: %+v", k)
	}
	// A 0 threshold is off.
	off := alertModel(t, `,"alerts":{"idle_minutes":0,"context_pct":0,"five_hour_pct":0,"waiting_seconds":0,"stall_minutes":0}`)
	off.ApplyAgents([]signals.Agent{{SessionID: "s", Cwd: cwd, Status: "waiting"}}, nil, t0)
	ctx, five = 99, 99
	off.ApplyStatus(p, t0)
	if v := off.Snapshot(t0.Add(time.Hour)); len(v.Alerts) != 0 {
		t.Fatalf("disabled alerts fired: %+v", v.Alerts)
	}
	// quota_auto_resume_stale: it will not continue by itself.
	m.ApplyHook(signals.HookEvent{SessionID: "s", Cwd: cwd, Event: "Notification", NotificationType: "quota_auto_resume_stale"}, t0)
	if k := alertKinds(m.Snapshot(t0.Add(12 * time.Minute))); k[AlertNoAutoResume] != signals.SevWarn {
		t.Fatalf("no auto-resume alert: %+v", k)
	}
}

// lockAlerts is the login-lock alerts of m's view at now, by kind.
func lockAlerts(m *Model, now time.Time) map[string]AlertView {
	out := map[string]AlertView{}
	for _, a := range m.Snapshot(now).Alerts {
		if a.Kind == AlertLoginLock || a.Kind == AlertStrandedLock || a.Kind == AlertFutureLock {
			out[a.Kind] = a
		}
	}
	return out
}

// [CLAUDECALLS-5-S1] A lock held live for over 2 minutes is an alert: one modified
// every 5 s for 3 minutes, held by pid 4242, names pid 4242, its command and the
// lock's age, and says to run /login in any session and to end that process if it
// comes back. (The watch's side, an unbroken live run, is in claudecall's log_test.go.)
func TestLoginLockLiveOverTwoMinutesIsAnAlert(t *testing.T) {
	t.Parallel()
	m := alertModel(t, "")
	lock := types.LoginLock{Which: "current", Path: "/h/.claude/.oauth_refresh.lock", State: "live",
		ModTime: t0.Add(3*time.Minute - 2*time.Second).UnixMilli(), LiveSince: t0.UnixMilli(), Birth: t0.UnixMilli(),
		PID: 4242, Command: "/usr/local/bin/claude -p hi", HolderState: "running", Holder: "other"}
	m.ApplyLoginLocks([]types.LoginLock{lock})
	if k := lockAlerts(m, t0.Add(2*time.Minute)); len(k) != 0 {
		t.Fatalf("an alert at 2 min, not over: %+v", k)
	}
	now := t0.Add(3 * time.Minute)
	a, ok := lockAlerts(m, now)[AlertLoginLock]
	if !ok || a.Severity != signals.SevBlock || a.Key != "login_lock:current" {
		t.Fatalf("no blocking login-lock alert: %+v", lockAlerts(m, now))
	}
	for _, want := range []string{"pid 4242", "(/usr/local/bin/claude -p hi)", lock.Path, "Run /login in any session", "if it comes back, end pid 4242"} {
		if !strings.Contains(a.Text, want) {
			t.Errorf("alert %q lacks %q", a.Text, want)
		}
	}
	// Its age: the page shows it from Since, the lock's birth; a notification says it.
	if a.Since != t0.UnixMilli() || !strings.HasPrefix(noticeLine(a, now), "login lock 3m old: ") {
		t.Errorf("age: since %d, notice %q", a.Since, noticeLine(a, now))
	}
	// The panel's own call is named as such.
	lock.Holder, lock.Call = "own", "claude agents --json"
	m.ApplyLoginLocks([]types.LoginLock{lock})
	if a := lockAlerts(m, now)[AlertLoginLock]; !strings.Contains(a.Text, "the panel's own claude agents --json") {
		t.Errorf("own call: %q", a.Text)
	}
	// A record whose holder is gone, or whose pid now names another process, does not
	// name the live holder: no pid to end.
	for _, gone := range []string{"gone", "gone (pid reused)"} {
		lock.Holder, lock.Call, lock.HolderState, lock.Command = "other", "", gone, ""
		m.ApplyLoginLocks([]types.LoginLock{lock})
		a := lockAlerts(m, now)[AlertLoginLock]
		if a.Kind == "" || strings.Contains(a.Text, "end pid") || strings.Contains(a.Text, "held by pid") ||
			!strings.Contains(a.Text, "the owner record's holder, pid 4242, is "+gone) || !strings.Contains(a.Text, "Run /login in any session") {
			t.Errorf("holder %s: %q", gone, a.Text)
		}
	}
}

// [CLAUDECALLS-5-S2] A stranded lock is a warning with the remedy: one last modified
// 5 minutes ago, whose record names a pid that no longer runs, names that lock's
// path, says its holder is gone, and says to remove that path and run /login if
// sessions fail to refresh.
func TestLoginLockStrandedIsAWarning(t *testing.T) {
	t.Parallel()
	m := alertModel(t, "")
	now := t0.Add(time.Hour)
	mod := now.Add(-5 * time.Minute)
	lock := types.LoginLock{Which: "legacy", Path: "/h/.claude.lock", State: "stale", ModTime: mod.UnixMilli(),
		StaleSince: mod.Add(time.Minute).UnixMilli(), PID: 4242, HolderState: "gone", Holder: "other"}
	m.ApplyLoginLocks([]types.LoginLock{lock})
	k := lockAlerts(m, now)
	a, ok := k[AlertStrandedLock]
	if !ok || a.Severity != signals.SevWarn || k[AlertLoginLock].Kind != "" {
		t.Fatalf("alerts %+v", k)
	}
	for _, want := range []string{"the legacy login lock /h/.claude.lock", "pid 4242", "is gone", "If sessions fail to refresh, remove /h/.claude.lock and run /login"} {
		if !strings.Contains(a.Text, want) {
			t.Errorf("warning %q lacks %q", a.Text, want)
		}
	}
	// The remedy per holder state (D7 as amended): a holder that is gone, or whose
	// pid now names another process, leaves the lock to remove; one that still runs
	// is ended first, since removing a lock a live process holds lets two refresh.
	removeOnly := "If sessions fail to refresh, remove /h/.claude.lock and run /login"
	endFirst := "End pid 4242 first; if sessions still fail to refresh, remove /h/.claude.lock and run /login"
	for state, want := range map[string]struct{ says, remedy string }{
		"gone":              {"is gone", removeOnly},
		"gone (pid reused)": {"is gone (the pid now names another process)", removeOnly},
		"running":           {"still runs", endFirst},
	} {
		lock.HolderState = state
		m.ApplyLoginLocks([]types.LoginLock{lock})
		a := lockAlerts(m, now)[AlertStrandedLock]
		if !strings.Contains(a.Text, want.says) || !strings.Contains(a.Text, want.remedy) {
			t.Errorf("holder %s: %q, want %q and %q", state, a.Text, want.says, want.remedy)
		}
		if state != "running" && strings.Contains(a.Text, "End pid") {
			t.Errorf("holder %s: %q says to end a process that is gone", state, a.Text)
		}
	}
}

// [CLAUDECALLS-5-S4] A lock that has only just gone stale is not yet a warning: one
// last modified 90 s ago raises nothing.
func TestLoginLockJustStaleIsNoWarning(t *testing.T) {
	t.Parallel()
	// Whatever its holder's state: the threshold does not depend on it.
	for _, holder := range []string{"gone", "gone (pid reused)", "running"} {
		m := alertModel(t, "")
		now := t0.Add(time.Hour)
		mod := now.Add(-90 * time.Second)
		m.ApplyLoginLocks([]types.LoginLock{{Which: "current", Path: "/h/.claude/.oauth_refresh.lock", State: "stale",
			ModTime: mod.UnixMilli(), StaleSince: mod.Add(time.Minute).UnixMilli(), PID: 4242, HolderState: holder, Holder: "other"}})
		if k := lockAlerts(m, now); len(k) != 0 {
			t.Fatalf("holder %s: a warning at 90 s: %+v", holder, k)
		}
		// The 2 min count from when it went stale (mtime + 60 s), not from its mtime.
		if k := lockAlerts(m, mod.Add(2*time.Minute+30*time.Second)); len(k) != 0 {
			t.Fatalf("holder %s: a warning at 2m30s, 90 s after it went stale: %+v", holder, k)
		}
		if k := lockAlerts(m, mod.Add(3*time.Minute)); k[AlertStrandedLock].Kind == "" {
			t.Fatalf("holder %s: no warning 2 min after it went stale: %+v", holder, k)
		}
	}
}

// [CLAUDECALLS-5-S5] A future-dated lock is a warning, not a live alert: one modified
// 2 hours in the future says it is dated in the future, that Claude Code waits for
// real time to pass it, and gives the remedy.
func TestLoginLockFutureDatedIsAWarning(t *testing.T) {
	t.Parallel()
	m := alertModel(t, "")
	now := t0.Add(time.Hour)
	m.ApplyLoginLocks([]types.LoginLock{{Which: "current", Path: "/h/.claude/.oauth_refresh.lock", State: "future",
		ModTime: now.Add(2 * time.Hour).UnixMilli(), PID: 4242, HolderState: "running", Holder: "other"}})
	k := lockAlerts(m, now)
	a, ok := k[AlertFutureLock]
	if !ok || a.Severity != signals.SevWarn || len(k) != 1 {
		t.Fatalf("alerts %+v", k)
	}
	for _, want := range []string{"is dated in the future", "Claude Code waits for real time to pass it", "remove /h/.claude/.oauth_refresh.lock and run /login"} {
		if !strings.Contains(a.Text, want) {
			t.Errorf("warning %q lacks %q", a.Text, want)
		}
	}
}

// The login locks are the machine's: the machine notifies them, no project's notifier.
func TestLoginLockAlertsAreTheMachines(t *testing.T) {
	t.Parallel()
	for _, k := range []string{AlertLoginLock, AlertStrandedLock, AlertFutureLock, AlertQuota} {
		if !MachineAlert(k) {
			t.Errorf("%s is not the machine's", k)
		}
	}
	if MachineAlert(AlertWaiting) {
		t.Error("a project's alert counts as the machine's")
	}
}

// contextAlert is the view's context alert, if any.
func contextAlert(v View) (AlertView, bool) {
	for _, a := range v.Alerts {
		if a.Kind == AlertContext {
			return a, true
		}
	}
	return AlertView{}, false
}

// alertPctOf is lane path's context alert threshold as the lane row, its metrics and
// its terminal lane carry it to the page; -1 where one is missing.
func alertPctOf(c ctxOf) [3]float64 {
	f := func(p *float64) float64 {
		if p == nil {
			return -1
		}
		return *p
	}
	return [3]float64{f(c.lv.CtxAlertPct), f(c.lv.Metrics.CtxAlertPct), f(c.tv.CtxAlertPct)}
}

// [LANECTX-2-S6] A windowed lane's alert fires before it compacts: under
// alerts.context_pct 85, a 200000 lane at 160000 tokens reads 80%, its threshold is
// 78.5 (its tick, 83.5, less 5), and the context alert fires saying so. Under
// context_pct 80, a 300000 lane's threshold is 80, the lower of the two. (The page's
// warning colour from this threshold: web's TestContextColour.)
func TestLANECTX2S6WindowedLaneAlertsBeforeItCompacts(t *testing.T) {
	t.Parallel()
	m := alertModel(t, `,"alerts":{"context_pct":85}`)
	m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{windowRec("x", "s", "/repo/w/x", 200000)}, "", nil, t0)
	m.ApplyStatus(ctxPost(t, "s", "/repo/w/x", 16, 160000), t0)
	v := m.Snapshot(t0)
	c := readCtx(t, v, "/repo/w/x")
	if c.session != 80 || c.metrics != 80 {
		t.Fatalf("context %v / %v, want 80", c.session, c.metrics)
	}
	if got := alertPctOf(c); got != [3]float64{78.5, 78.5, 78.5} {
		t.Fatalf("threshold (lane, metrics, terminal) %v, want 78.5 each", got)
	}
	a, ok := contextAlert(v)
	if !ok || a.Severity != signals.SevWarn || a.Text != "context at 80% (alert at 78.5%)" {
		t.Fatalf("context alert %+v (fired %v), want \"context at 80%% (alert at 78.5%%)\"", a, ok)
	}

	m = alertModel(t, `,"alerts":{"context_pct":80}`)
	m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{windowRec("x", "s", "/repo/w/x", 300000)}, "", nil, t0)
	m.ApplyStatus(ctxPost(t, "s", "/repo/w/x", 16, 160000), t0)
	if got := alertPctOf(readCtx(t, m.Snapshot(t0), "/repo/w/x")); got != [3]float64{80, 80, 80} {
		t.Fatalf("300000 under context_pct 80: threshold %v, want 80 each", got)
	}
	// The other windows D13 names: 84 at 300000 and 62 at 100000 under 85; 85 at 1m.
	for w, want := range map[int]float64{300000: 84, 100000: 62, 1000000: 85, 0: 85} {
		if got := ctxAlertPct(85, w); got != want {
			t.Errorf("ctxAlertPct(85, %d) = %v, want %v", w, got, want)
		}
	}
}

// withWindow is post p as a status line that adds "auto_compact_window": raw (a JSON
// value) sends it, decoded through StatusPayload as the server decodes a post.
func withWindow(t *testing.T, p signals.StatusPayload, raw string) signals.StatusPayload {
	t.Helper()
	if err := json.Unmarshal([]byte(`{"auto_compact_window":`+raw+`}`), &p); err != nil {
		t.Fatalf("a post with auto_compact_window %s fails to decode: %v", raw, err)
	}
	return p
}

// [LANECTX-2-S8] A lane keeps its window after /clear: a lane launched with a 200000
// window runs /clear, and its status line posts under a session id no lane record holds,
// with "auto_compact_window": 200000 and 180000 tokens. The session's context reads 90%
// (not used_percentage's 18), with the window's 83.5 tick, and its context alert fires.
func TestLANECTX2S8LaneKeepsItsWindowAfterClear(t *testing.T) {
	t.Parallel()
	m := alertModel(t, `,"alerts":{"context_pct":85}`)
	m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{windowRec("x", "s", "/repo/w/x", 200000)}, "", nil, t0)
	m.ApplyStatus(withWindow(t, ctxPost(t, "s-cleared", "/repo/w/x", 18, 180000), "200000"), t0)
	v := m.Snapshot(t0)
	c := readCtx(t, v, "/repo/w/x")
	if c.session != 90 || c.lane != 90 || c.metrics != 90 || c.window != 200000 {
		t.Fatalf("cleared session: context %v / %v / %v against %d, want 90 against 200000", c.session, c.lane, c.metrics, c.window)
	}
	if c.lv.CtxTick != 83.5 || c.lv.CtxAlertPct == nil || *c.lv.CtxAlertPct != 78.5 {
		t.Fatalf("cleared session: tick %v, threshold %v, want 83.5 and 78.5", c.lv.CtxTick, c.lv.CtxAlertPct)
	}
	if a, ok := contextAlert(v); !ok || a.Text != "context at 90% (alert at 78.5%)" {
		t.Fatalf("context alert %+v (fired %v), want \"context at 90%% (alert at 78.5%%)\"", a, ok)
	}
}

// [LANECTX-2-S9] A session started by hand in a windowed lane's folder keeps Claude
// Code's figure: in the worktree of a lane launched with a 200000 window, a session no
// lane record holds posts with no auto_compact_window, used_percentage 42 and 420000
// tokens. Its context reads 42% (not 210% of the lane's window), and no context alert
// fires for it.
func TestLANECTX2S9HandStartedSessionKeepsClaudeCodesFigure(t *testing.T) {
	t.Parallel()
	m := alertModel(t, `,"alerts":{"context_pct":85}`)
	m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{windowRec("x", "s", "/repo/w/x", 200000)}, "", nil, t0)
	m.ApplyStatus(ctxPost(t, "by-hand", "/repo/w/x", 42, 420000), t0)
	v := m.Snapshot(t0)
	c := readCtx(t, v, "/repo/w/x")
	if c.session != 42 || c.lane != 42 || c.metrics != 42 || c.window != 1000000 {
		t.Fatalf("hand-started session: context %v / %v / %v against %d, want used_percentage's 42 against the model's 1000000",
			c.session, c.lane, c.metrics, c.window)
	}
	if a, ok := contextAlert(v); ok {
		t.Fatalf("a context alert for a hand-started session at 42%%: %+v", a)
	}
}

// A posted window outside Claude Code's range, or one that is not an integer, is
// ignored: the post still decodes and folds, and the record holding its session id
// applies (or, with none, used_percentage).
func TestPostedWindowOutsideTheRangeIsIgnored(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"50000", "1000001", "0", "-200000", `"200000"`, "200000.5", "null"} {
		m := alertModel(t, `,"alerts":{"context_pct":85}`)
		m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}, {ID: "y", Path: "/repo"}},
			[]types.LaneRecord{windowRec("x", "s", "/repo/w/x", 200000), windowRec("y", "sy", "/repo", 0)}, "", nil, t0)
		m.ApplyStatus(withWindow(t, ctxPost(t, "s", "/repo/w/x", 16, 160000), raw), t0)
		m.ApplyStatus(withWindow(t, ctxPost(t, "sy", "/repo", 16, 160000), raw), t0)
		v := m.Snapshot(t0)
		if c := readCtx(t, v, "/repo/w/x"); c.session != 80 || c.window != 200000 {
			t.Errorf("posted %s: context %v against %d, want the record's 80 against 200000", raw, c.session, c.window)
		}
		if c := readCtx(t, v, "/repo"); c.session != 16 || c.window != 1000000 {
			t.Errorf("posted %s, no windowed record: context %v against %d, want used_percentage's 16", raw, c.session, c.window)
		}
	}
	// In range, the posted window wins over the record's.
	m := alertModel(t, "")
	m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{windowRec("x", "s", "/repo/w/x", 200000)}, "", nil, t0)
	m.ApplyStatus(withWindow(t, ctxPost(t, "s", "/repo/w/x", 16, 160000), "320000"), t0)
	if c := readCtx(t, m.Snapshot(t0), "/repo/w/x"); c.session != 50 || c.window != 320000 {
		t.Errorf("posted 320000: context %v against %d, want 50 against 320000", c.session, c.window)
	}
}

// [LANECTX-2-S7] An alert turned off stays off for a windowed lane: under
// alerts.context_pct 0, a 200000 lane at 160000 tokens reads 80%, no context alert
// fires, and the threshold the page reads is 0, off (not the lower of 0 and the tick).
func TestLANECTX2S7AlertOffStaysOffForAWindowedLane(t *testing.T) {
	t.Parallel()
	m := alertModel(t, `,"alerts":{"context_pct":0}`)
	m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{windowRec("x", "s", "/repo/w/x", 200000)}, "", nil, t0)
	m.ApplyStatus(ctxPost(t, "s", "/repo/w/x", 16, 160000), t0)
	v := m.Snapshot(t0)
	c := readCtx(t, v, "/repo/w/x")
	if c.session != 80 || c.metrics != 80 {
		t.Fatalf("context %v / %v, want 80", c.session, c.metrics)
	}
	if a, ok := contextAlert(v); ok {
		t.Fatalf("a context alert with context_pct 0: %+v", a)
	}
	if got := alertPctOf(c); got != [3]float64{0, 0, 0} {
		t.Fatalf("threshold (lane, metrics, terminal) %v, want 0 (off) each", got)
	}
}

// [LANECTX-2-S4] The state side: a lane launched with no window carries
// alerts.context_pct (80) as its threshold, which the page's warning colour reads
// (web's TestContextColour draws 82% in it); the alert says "alert at 80%".
func TestLANECTX2S4LaneWithoutWindowCarriesTheConfiguredThreshold(t *testing.T) {
	t.Parallel()
	m := alertModel(t, `,"alerts":{"context_pct":80}`)
	m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{windowRec("x", "s", "/repo/w/x", 0)}, "", nil, t0)
	m.ApplyStatus(ctxPost(t, "s", "/repo/w/x", 82, 180000), t0)
	v := m.Snapshot(t0)
	if got := alertPctOf(readCtx(t, v, "/repo/w/x")); got != [3]float64{80, 80, 80} {
		t.Fatalf("threshold (lane, metrics, terminal) %v, want 80 each", got)
	}
	if a, ok := contextAlert(v); !ok || a.Text != "context at 82% (alert at 80%)" {
		t.Fatalf("context alert %+v (fired %v)", a, ok)
	}
}
