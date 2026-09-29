package state

import (
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

func TestAlertThresholds(t *testing.T) {
	t.Parallel()
	m := alertModel(t, `,"alerts":{"idle_minutes":10,"context_pct":80,"five_hour_pct":90,"waiting_seconds":60}`)
	cwd := "/repo/w/x"
	m.ApplyAgents([]signals.Agent{{SessionID: "s", Cwd: cwd, Status: "idle"}}, nil, t0)
	ctx, five := 79.0, 89.0
	p := signals.StatusPayload{SessionID: "s", Cwd: cwd}
	p.ContextWindow.UsedPercentage = &ctx
	p.RateLimits.FiveHour = &signals.RateLimit{UsedPercentage: &five}
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
