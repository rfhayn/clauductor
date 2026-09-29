package state

import (
	"errors"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

func TestFirstPromptDecision(t *testing.T) {
	start := t0
	tests := []struct {
		name string
		in   PromptInput
		at   time.Duration
		want string
	}{
		{"idle in claude agents: type it", PromptInput{State: "pending", Running: true, Listed: true, Status: "idle", PollFresh: true, Since: start}, time.Second, "send"},
		// Review 2026-09-28: idle alone is not enough.
		{"idle, but the last poll is stale", PromptInput{State: "pending", Running: true, Listed: true, Status: "idle", Since: start}, time.Second, "wait"},
		{"idle, but waitingFor is set", PromptInput{State: "pending", Running: true, Listed: true, Status: "idle", WaitingFor: "dialog open", PollFresh: true, Since: start}, time.Second, "wait"},
		{"idle, but a hook says it waits", PromptInput{State: "pending", Running: true, Listed: true, Status: "idle", WaitingNote: true, PollFresh: true, Since: start}, time.Second, "wait"},
		// Held at the trust dialog, the session is not listed at all (2.1.284).
		{"not listed yet: wait", PromptInput{State: "pending", Running: true, Since: start}, 5 * time.Second, "wait"},
		{"not listed for long: ask the human", PromptInput{State: "pending", Running: true, Since: start}, readyGrace, "stuck"},
		{"listed but busy: wait", PromptInput{State: "pending", Running: true, Listed: true, Status: "busy", Since: start}, time.Second, "wait"},
		{"listed but waiting (a dialog): wait", PromptInput{State: "pending", Running: true, Listed: true, Status: "waiting", Since: start}, time.Second, "wait"},
		{"you typed first: skip", PromptInput{State: "pending", Running: true, Listed: true, Status: "idle", Prompted: start, Since: start}, time.Second, "skip"},
		{"claude exited: stuck", PromptInput{State: "pending", Running: true, Dead: true, Since: start}, time.Second, "stuck"},
		{"lane down: wait", PromptInput{State: "pending", Since: start}, time.Hour, "wait"},
		// PANEL-7: a lane seen gone past the grace needs a RESTORE; polls cannot help.
		{"lane gone within the grace: wait", PromptInput{State: "pending", Since: start, GoneSince: start}, goneGrace - time.Second, "wait"},
		{"lane gone past the grace: restore", PromptInput{State: "pending", Since: start, GoneSince: start}, goneGrace, "restore"},
		{"panel died mid-typing: never again", PromptInput{State: "typing", Running: true, Listed: true, Status: "idle", Since: start}, time.Second, "stuck"},
		{"typed and submitted", PromptInput{State: "sent", Running: true, Prompted: start.Add(time.Second), PromptAt: start}, 2 * time.Second, "delivered"},
		{"typed, busy", PromptInput{State: "sent", Running: true, Listed: true, Status: "busy", PromptAt: start}, 2 * time.Second, "delivered"},
		{"typed, nothing yet", PromptInput{State: "sent", Running: true, Listed: true, Status: "idle", PromptAt: start}, 5 * time.Second, "wait"},
		{"typed, never landed", PromptInput{State: "sent", Running: true, Listed: true, Status: "idle", PromptAt: start}, confirmGrace, "stuck"},
		{"done", PromptInput{State: "delivered"}, 0, "none"},
	}
	for _, tc := range tests {
		if got := DecideFirstPrompt(tc.in, start.Add(tc.at)); got.Action != tc.want {
			t.Errorf("%s: got %s (%s), want %s", tc.name, got.Action, got.Why, tc.want)
		}
	}
}

func TestPromptWaitsForAFreshPoll(t *testing.T) {
	m := v2Model(t)
	rec := lanes.LaneRecord{ID: "tpl", SessionID: "s1", Path: buildWT, Type: "build", PromptState: "pending", ActionAt: t0.UnixMilli(), ActionDone: true}
	m.ApplyTmux([]lanes.TmuxLane{{ID: "tpl", Path: buildWT}}, []lanes.LaneRecord{rec}, "", nil, t0)
	m.ApplyAgents([]signals.Agent{{SessionID: "s1", Cwd: buildWT, Status: "idle"}}, nil, t0)
	if d := m.PromptDecisions(t0.Add(time.Second))["tpl"]; d.Action != "send" {
		t.Fatalf("fresh idle: %+v", d)
	}
	// The poll fails afterwards: the last "idle" is no longer a current reading.
	m.ApplyAgents(nil, errors.New("claude agents: exit 1"), t0.Add(2*time.Second))
	if d := m.PromptDecisions(t0.Add(3 * time.Second))["tpl"]; d.Action == "send" {
		t.Fatalf("typed on a failed poll: %+v", d)
	}
	// Or it simply stops arriving for longer than two intervals.
	m2 := v2Model(t)
	m2.ApplyTmux([]lanes.TmuxLane{{ID: "tpl", Path: buildWT}}, []lanes.LaneRecord{rec}, "", nil, t0)
	m2.ApplyAgents([]signals.Agent{{SessionID: "s1", Cwd: buildWT, Status: "idle"}}, nil, t0)
	if d := m2.PromptDecisions(t0.Add(11 * time.Second))["tpl"]; d.Action == "send" {
		t.Fatalf("typed on a stale poll: %+v", d)
	}
	// A hook says it waits on a permission prompt: no typing.
	m2.ApplyAgents([]signals.Agent{{SessionID: "s1", Cwd: buildWT, Status: "idle"}}, nil, t0.Add(12*time.Second))
	m2.ApplyHook(signals.HookEvent{SessionID: "s1", Cwd: buildWT, Event: "Notification", NotificationType: "permission_prompt"}, t0.Add(12*time.Second))
	if d := m2.PromptDecisions(t0.Add(13 * time.Second))["tpl"]; d.Action == "send" {
		t.Fatalf("typed into a waiting session: %+v", d)
	}
}
