package panel

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

const buildWT = "/repo/.claude/worktrees/build-add-feature"

func v2Model(t *testing.T) *Model {
	t.Helper()
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	return m
}

func notifEv(typ string) HookEvent {
	return HookEvent{SessionID: "s1", Cwd: buildWT, Event: "Notification", NotificationType: typ, Message: "m"}
}

// C1: every documented notification type is mapped explicitly.
func TestNotificationTableCoversEveryDocumentedType(t *testing.T) {
	if n := len(NotificationTypes()); n != 12 {
		t.Fatalf("want the 12 documented types, got %d", n)
	}
	for _, typ := range NotificationTypes() {
		k := ClassifyNotification(typ)
		if !k.Known || k.Label == "" || k.Severity == "" {
			t.Errorf("%s is not mapped: %+v", typ, k)
		}
	}
	if len(notificationTable) != 12 {
		t.Errorf("the table maps %d types; a new one needs a deliberate row and a test", len(notificationTable))
	}
}

func TestNotificationEffects(t *testing.T) {
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
			m.ApplyHook(HookEvent{SessionID: "s1", Cwd: buildWT, Event: "UserPromptSubmit"}, t0)
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
	m := v2Model(t)
	m.ApplyHook(HookEvent{SessionID: "s1", Cwd: buildWT, Event: "Stop"}, t0)
	m.ApplyHook(notifEv("agent_completed"), t0.Add(time.Second))
	v := m.Snapshot(t0.Add(2 * time.Second))
	if len(v.Done) != 1 || len(v.NeedsYou) != 0 || v.Done[0].Severity != SevInfo {
		t.Fatalf("done %+v needs %+v", v.Done, v.NeedsYou)
	}
}

func TestElicitationCompleteAnswersTheDialog(t *testing.T) {
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
	for _, ev := range []string{"StopFailure", "PermissionRequest", "PreCompact", "PostCompact", "CwdChanged"} {
		found := false
		for _, e := range HookEvents {
			found = found || e == ev
		}
		if !found {
			t.Errorf("%s is not subscribed", ev)
		}
	}
	for _, e := range HookEvents {
		if e == "SessionStart" {
			t.Error("SessionStart supports no HTTP hooks and must stay out")
		}
	}
	m := v2Model(t)
	m.ApplyHook(HookEvent{SessionID: "s1", Cwd: buildWT, Event: "PermissionRequest", ToolName: "Bash"}, t0)
	v := m.Snapshot(t0)
	if len(v.NeedsYou) != 1 || !strings.Contains(v.NeedsYou[0].Text, "Bash") || v.NeedsYou[0].Severity != SevBlock {
		t.Fatalf("permission request: %+v", v.NeedsYou)
	}
	m.ApplyHook(HookEvent{SessionID: "s1", Cwd: buildWT, Event: "PreCompact", CompactionTrigger: "auto"}, t0)
	if c := laneByName(m.Snapshot(t0), "add-feature").Sessions[0].Compacting; c != "auto" {
		t.Fatalf("compacting %q", c)
	}
	m.ApplyHook(HookEvent{SessionID: "s1", Cwd: buildWT, Event: "PostCompact", CompactionTrigger: "auto"}, t0)
	m.ApplyHook(HookEvent{SessionID: "s1", Cwd: buildWT, Event: "StopFailure", ErrorType: "rate_limit"}, t0.Add(time.Second))
	v = m.Snapshot(t0.Add(2 * time.Second))
	s := laneByName(v, "add-feature").Sessions[0]
	if s.Compacting != "" || s.Failure != "rate_limit" {
		t.Fatalf("after PostCompact/StopFailure: %+v", s)
	}
	found := false
	for _, a := range v.Alerts {
		found = found || (a.Kind == AlertRateLimit && a.Severity == SevBlock)
	}
	if !found {
		t.Fatalf("StopFailure rate_limit raised no alert: %+v", v.Alerts)
	}
	// An event name the panel does not subscribe to is counted, not applied.
	m.ApplyHook(HookEvent{SessionID: "s1", Cwd: buildWT, Event: "PreToolUse"}, t0)
	if v := m.Snapshot(t0); v.Observe.DroppedUnknownEvent != 1 || v.Observe.DroppedForeign != 0 {
		t.Fatalf("unknown event: %+v", v.Observe)
	}
}

// C4: a session is bound once; the panel's own session id wins over cwd.
func TestSessionBindingByID(t *testing.T) {
	m := v2Model(t)
	// A session the panel did not launch binds at first sight by cwd, and a later
	// `cd` into another worktree does not move it.
	m.ApplyHook(HookEvent{SessionID: "own", Cwd: buildWT, Event: "UserPromptSubmit"}, t0)
	m.ApplyHook(HookEvent{SessionID: "own", Cwd: "/repo", Event: "Stop"}, t0.Add(time.Second))
	m.ApplyHook(HookEvent{SessionID: "own", Cwd: "/repo", Event: "CwdChanged", PreviousCwd: buildWT}, t0.Add(time.Second))
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
	rec := LaneRecord{ID: "fixer", SessionID: "launched", Path: "/repo/.claude/worktrees/fix-thing", Type: "fix", ActionDone: true}
	m.ApplyTmux(nil, []LaneRecord{rec}, "", nil, t0)
	if !m.ApplyHook(HookEvent{SessionID: "launched", Cwd: "/tmp", Event: "UserPromptSubmit"}, t0) {
		t.Fatal("a registry-bound session's event was dropped for its cwd")
	}
	v = m.Snapshot(t0)
	if l := laneByName(v, "thing"); l == nil || len(l.Sessions) != 1 || l.Sessions[0].ID != "launched" {
		t.Fatalf("launched session not in its lane: %+v", v.Lanes)
	}
	// A stranger from outside the project is still dropped, and counted as foreign.
	if m.ApplyHook(HookEvent{SessionID: "stranger", Cwd: "/elsewhere", Event: "Stop"}, t0) {
		t.Fatal("foreign session kept")
	}
}

// C5: a quota window expires at resets_at.
func TestQuotaExpiresAtResetsAt(t *testing.T) {
	m := v2Model(t)
	five, seven := 80.0, 40.0
	r5, r7 := t0.Add(time.Hour).Unix(), t0.Add(72*time.Hour).Unix()
	p := StatusPayload{SessionID: "s1", Cwd: buildWT}
	p.RateLimits.FiveHour = &RateLimit{UsedPercentage: &five, ResetsAt: &r5}
	p.RateLimits.SevenDay = &RateLimit{UsedPercentage: &seven, ResetsAt: &r7}
	m.ApplyStatus(p, t0)
	if q := m.Snapshot(t0.Add(59 * time.Minute)).Quota; q.FiveHour == nil || *q.FiveHour != 80 || q.FiveHourExpired {
		t.Fatalf("before reset: %+v", q)
	}
	q := m.Snapshot(t0.Add(time.Hour)).Quota
	if q.FiveHour != nil || !q.FiveHourExpired || q.SevenDay == nil || *q.SevenDay != 40 {
		t.Fatalf("at reset the 5-hour window must drop and the 7-day stay: %+v", q)
	}
	// And a guard reads the expired window as unknown, never as 80%.
	if why := QuotaGuardBlock(q, 50); why != "" {
		t.Fatalf("guard on an expired window: %q", why)
	}
	if why := QuotaGuardBlock(m.Snapshot(t0).Quota, 50); why == "" {
		t.Fatal("guard ignores a live 80% window at 50%")
	}
}

// C6: `claude agents` id, state and the waitingFor enum are decoded.
func TestAgentsDecodeIDStateWaitingFor(t *testing.T) {
	agents, err := ParseAgents([]byte(`[{"pid":1,"cwd":"` + buildWT + `","kind":"background","sessionId":"bg","id":"a1b2","state":"running","status":"waiting","waitingFor":"permission prompt"}]`))
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
		if got := waitingForKind(in); got != want {
			t.Errorf("waitingForKind(%q) = %q, want %q", in, got, want)
		}
	}
}

// C14: subagent heuristics are tagged with the version they were verified on.
func TestHeuristicsApproximateOnOtherVersion(t *testing.T) {
	m := v2Model(t)
	m.ApplyHook(HookEvent{SessionID: "s1", Cwd: buildWT, Event: "SubagentStart", AgentID: "a", AgentType: "builder"}, t0)
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
	if got, _ := ParseClaudeVersion([]byte("2.1.284 (Claude Code)\n")); got != "2.1.284" {
		t.Fatalf("parse %q", got)
	}
	if _, err := ParseClaudeVersion([]byte("command not found")); err == nil {
		t.Fatal("garbage parsed")
	}
}

func TestAgentsFilterAndBackoff(t *testing.T) {
	wts := []Worktree{{Path: "/p/app"}, {Path: "/p/app/.claude/worktrees/x"}}
	if d := AgentsFilterDir("/p/app", wts); d != "/p/app" {
		t.Fatalf("inside root: %q", d)
	}
	wts = append(wts, Worktree{Path: "/p/app-worktrees/y"})
	if d := AgentsFilterDir("/p/app", wts); d != "/p" {
		t.Fatalf("a worktree outside the root widens the filter: %q", d)
	}
	if d := AgentsFilterDir("/p/app", append(wts, Worktree{Path: "/q/z"})); d != "" {
		t.Fatalf("nothing in common must mean no filter: %q", d)
	}
	all := []Agent{{SessionID: "a", Cwd: "/p/app"}, {SessionID: "b", Cwd: "/p/app-worktrees/y"}, {SessionID: "c", Cwd: "/other"}}
	in := func(a Agent) bool { return MatchWorktree(wts, a.Cwd) >= 0 }
	if got := MissedByFilter(all, all[:1], in); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("missed %v", got)
	}
	if got := MissedByFilter(all, all[:2], in); len(got) != 0 {
		t.Fatalf("a foreign session missing from the filter is fine: %v", got)
	}
	// With a lane (PANEL-7 adds the quiet interval without one: cost_test.go).
	if AgentsInterval(time.Time{}, t0, true) != 2*time.Second || AgentsInterval(t0.Add(-10*time.Second), t0, true) != 5*time.Second ||
		AgentsInterval(t0.Add(-31*time.Second), t0, true) != 2*time.Second {
		t.Fatal("backoff wrong")
	}
}
