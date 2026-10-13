package state

import (
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
	off := alertModel(t, `,"alerts":{"idle_minutes":0,"context_pct":0,"five_hour_pct":0,"waiting_seconds":0}`)
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
