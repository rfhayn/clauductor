package state

import (
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// PANEL-13: a Claude Code the panel has not verified is checked against the hooks the
// project's sessions send, in the shape recorded on 2.1.284 (TestSubagentNesting).
func verifyModel(t *testing.T, version string) (*Model, func(e, agentID, tool, toolUse, out string, ms int)) {
	t.Helper()
	cwd := "/repo/.claude/worktrees/build-add-feature"
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	m.ApplyAgents([]signals.Agent{{SessionID: "s", Cwd: cwd, Status: "busy"}}, nil, t0)
	m.ApplyClaudeVersion(version, nil, t0)
	hook := func(e, agentID, tool, toolUse, out string, ms int) {
		h := signals.HookEvent{SessionID: "s", Cwd: cwd, Event: e, AgentID: agentID, ToolName: tool, ToolUseID: toolUse}
		if e == "SubagentStart" || e == "SubagentStop" {
			h.AgentType = "general-purpose"
		}
		if e == "PreToolUse" {
			h.ToolInput = []byte(`{"subagent_type":"general-purpose","description":"d"}`)
		}
		if out != "" {
			h.ToolResponse = []byte(out)
		}
		m.ApplyHook(h, t0.Add(time.Duration(ms)*time.Millisecond))
	}
	return m, hook
}

// launch is one Agent call as 2.1.284 sends it: PreToolUse, SubagentStart, PostToolUse.
func launch(hook func(e, agentID, tool, toolUse, out string, ms int), id string, ms int) {
	hook("PreToolUse", "", "Agent", "tu-"+id, "", ms)
	hook("SubagentStart", id, "", "", "", ms+20)
	hook("PostToolUse", "", "Agent", "tu-"+id, `{"agentId":"`+id+`","status":"completed"}`, ms+500)
}

func TestLiveHooksVerifyANewVersion(t *testing.T) {
	t.Parallel()
	m, hook := verifyModel(t, "2.1.285")
	v := m.Snapshot(t0)
	if !laneApprox(v) || len(v.WarningItems) != 1 || v.WarningItems[0].Key != "version:2.1.285" ||
		!strings.Contains(v.WarningItems[0].Text, "0 of 3") {
		t.Fatalf("before any launch: approx %v, warnings %+v", laneApprox(v), v.WarningItems)
	}
	launch(hook, "A", 0)
	// A background launch: its PostToolUse comes before its SubagentStart, and still counts.
	hook("PreToolUse", "", "Agent", "tu-B", "", 2000)
	hook("PostToolUse", "", "Agent", "tu-B", `{"agentId":"B","status":"async_launched"}`, 2005)
	hook("SubagentStart", "B", "", "", "", 2030)
	v = m.Snapshot(t0.Add(3 * time.Second))
	if w := v.WarningItems; len(w) != 1 || w[0].Key != "version:2.1.285" || !strings.Contains(w[0].Text, "2 of 3") {
		t.Fatalf("after two launches the key stays and the count moves: %+v", w)
	}
	launch(hook, "C", 4000)
	v = m.Snapshot(t0.Add(5 * time.Second))
	if laneApprox(v) || len(v.WarningItems) != 0 || m.AutoVerified() != "2.1.285" || v.Verification.Auto != "2.1.285" {
		t.Fatalf("three clean launches verify it: approx %v, warnings %+v, auto %q", laneApprox(v), v.WarningItems, m.AutoVerified())
	}
	// The next version starts over.
	m.ApplyClaudeVersion("2.1.286", nil, t0.Add(6*time.Second))
	if v := m.Snapshot(t0.Add(6 * time.Second)); !laneApprox(v) || len(v.WarningItems) != 1 || !strings.Contains(v.WarningItems[0].Text, "0 of 3") {
		t.Fatalf("a newer version is checked afresh: %+v", v.WarningItems)
	}
}

func TestLiveHooksCatchAChangedShape(t *testing.T) {
	t.Parallel()
	// The agent a PostToolUse names never starts: the ids no longer match.
	m, hook := verifyModel(t, "2.1.285")
	for i, id := range []string{"X", "Y"} {
		hook("PreToolUse", "", "Agent", "tu-"+id, "", i*100)
		hook("SubagentStart", "other-"+id, "", "", "", i*100+20)
		hook("PostToolUse", "", "Agent", "tu-"+id, `{"agentId":"`+id+`"}`, i*100+50)
	}
	hook("Stop", "", "", "", "", 15000) // any later hook ages them
	launch(hook, "A", 16000)
	launch(hook, "B", 17000)
	launch(hook, "C", 18000)
	v := m.Snapshot(t0.Add(20 * time.Second))
	if m.AutoVerified() != "" || !laneApprox(v) || len(v.WarningItems) != 1 || v.WarningItems[0].Key != "version:2.1.285:broken" ||
		!strings.Contains(v.WarningItems[0].Text, "no SubagentStart announced") {
		t.Fatalf("orphans break it, and confirmations after do not mend it: auto %q, %+v", m.AutoVerified(), v.WarningItems)
	}

	// PostToolUse no longer names the agent at all.
	m, hook = verifyModel(t, "2.1.285")
	for i := 0; i < 3; i++ {
		hook("PreToolUse", "", "Agent", "tu", "", i*100)
		hook("PostToolUse", "", "Agent", "tu", `{"status":"completed"}`, i*100+50)
	}
	if v := m.Snapshot(t0.Add(time.Second)); len(v.WarningItems) != 1 || !strings.Contains(v.WarningItems[0].Text, "named no agent") {
		t.Fatalf("unnamed responses: %+v", v.WarningItems)
	}
}

func TestVerifiedVersionsNeedNoCheck(t *testing.T) {
	t.Parallel()
	m, hook := verifyModel(t, HeuristicsVerifiedOn)
	launch(hook, "A", 0)
	if v := m.Snapshot(t0.Add(time.Second)); len(v.WarningItems) != 0 || laneApprox(v) || v.Verification.Version != "" || m.AutoVerified() != "" {
		t.Fatalf("the pinned version: %+v %+v", v.WarningItems, v.Verification)
	}
	// A version an earlier panel verified from live hooks, restored at start.
	m, _ = verifyModel(t, "2.1.285")
	m.RestoreAutoVerified("2.1.285")
	if v := m.Snapshot(t0); len(v.WarningItems) != 0 || laneApprox(v) {
		t.Fatalf("a restored verification: %+v", v.WarningItems)
	}
}

// laneApprox is the test lane's "approximate" flag on its subagent list.
func laneApprox(v View) bool {
	for _, l := range v.Lanes {
		if strings.HasSuffix(l.Path, "build-add-feature") {
			return l.SubagentsApprox
		}
	}
	return false
}
