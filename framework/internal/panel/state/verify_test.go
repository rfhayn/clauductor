package state

import (
	"errors"
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
	if vf := v.Verification; !laneApprox(v) || len(v.WarningItems) != 0 || vf.State != VerifyChecking || vf.Confirmed != 0 || vf.Needed != 3 {
		t.Fatalf("before any launch: approx %v, warnings %+v, %+v", laneApprox(v), v.WarningItems, vf)
	}
	launch(hook, "A", 0)
	// A background launch: its PostToolUse comes before its SubagentStart, and still counts.
	hook("PreToolUse", "", "Agent", "tu-B", "", 2000)
	hook("PostToolUse", "", "Agent", "tu-B", `{"agentId":"B","status":"async_launched"}`, 2005)
	hook("SubagentStart", "B", "", "", "", 2030)
	v = m.Snapshot(t0.Add(3 * time.Second))
	if vf := v.Verification; len(v.WarningItems) != 0 || vf.State != VerifyChecking || vf.Confirmed != 2 || vf.Needed != 3 {
		t.Fatalf("after two launches the count moves and no bar shows: %+v %+v", v.WarningItems, vf)
	}
	launch(hook, "C", 4000)
	v = m.Snapshot(t0.Add(5 * time.Second))
	if laneApprox(v) || len(v.WarningItems) != 0 || m.AutoVerified() != "2.1.285" || v.Verification.Auto != "2.1.285" {
		t.Fatalf("three clean launches verify it: approx %v, warnings %+v, auto %q", laneApprox(v), v.WarningItems, m.AutoVerified())
	}
	// The next version starts over.
	m.ApplyClaudeVersion("2.1.286", nil, t0.Add(6*time.Second))
	if v := m.Snapshot(t0.Add(6 * time.Second)); !laneApprox(v) || v.Verification.State != VerifyChecking || v.Verification.Confirmed != 0 || v.Verification.Version != "2.1.286" {
		t.Fatalf("a newer version is checked afresh: %+v", v.Verification)
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

// unmatched is an Agent call whose PostToolUse names an agent no SubagentStart announces.
func unmatched(hook func(e, agentID, tool, toolUse, out string, ms int), id string, ms int) {
	hook("PreToolUse", "", "Agent", "tu-"+id, "", ms)
	hook("PostToolUse", "", "Agent", "tu-"+id, `{"agentId":"`+id+`","status":"completed"}`, ms+50)
}

// oneOrphan checks 2.1.288 with one unmatched launch aged into an orphan and three
// launches confirmed: 3 of 4, by 13 s.
func oneOrphan(t *testing.T) (*Model, func(e, agentID, tool, toolUse, out string, ms int)) {
	t.Helper()
	m, hook := verifyModel(t, "2.1.288")
	unmatched(hook, "O", 0)
	launch(hook, "A", 11000) // its SubagentStart is the hook that ages O
	launch(hook, "B", 12000)
	launch(hook, "C", 13000)
	return m, hook
}

// [VERCHECK-2-S3] A read that fails after one worked raises the bar, not the field.
func TestVERCHECK2S3FailedReadAfterAGoodOneRaisesTheBar(t *testing.T) {
	t.Parallel()
	m, _ := verifyModel(t, "2.1.288")
	m.ApplyClaudeVersion("", errors.New("claude: not found"), t0.Add(time.Second))
	v := m.Snapshot(t0.Add(2 * time.Second))
	if len(v.WarningItems) != 1 || v.WarningItems[0].Key != "version:unread" ||
		!strings.Contains(v.WarningItems[0].Text, "cannot read `claude --version` (claude: not found); subagent lists are approximate") {
		t.Fatalf("the failed read raises the unread bar and nothing names the version: %+v", v.WarningItems)
	}
	if !laneApprox(v) {
		t.Fatal("the lane's subagent list says it is exact on a version the latest read could not confirm")
	}
	if vf := v.Verification; vf.State != VerifyUnread || vf.Version != "" || vf.Unread == "" {
		t.Fatalf("the state is unread, so the page shows no field: %+v", vf)
	}
	// The next read that works clears it, and the check carries on from where it was.
	m.ApplyClaudeVersion("2.1.288", nil, t0.Add(3*time.Second))
	if vf := m.Snapshot(t0.Add(4 * time.Second)).Verification; vf.State != VerifyChecking || vf.Version != "2.1.288" || vf.Unread != "" {
		t.Fatalf("a working read ends it: %+v", vf)
	}
}

// [VERCHECK-2-S4] A failed read does not hide a break.
func TestVERCHECK2S4FailedReadDoesNotHideABreak(t *testing.T) {
	t.Parallel()
	m, hook := verifyModel(t, "2.1.288")
	unmatched(hook, "X", 0)
	unmatched(hook, "Y", 100)
	launch(hook, "A", 15000) // its SubagentStart ages them
	m.ApplyClaudeVersion("", errors.New("claude: not found"), t0.Add(16*time.Second))
	v := m.Snapshot(t0.Add(17 * time.Second))
	keys := map[string]bool{}
	for _, w := range v.WarningItems {
		keys[w.Key] = true
	}
	if len(v.WarningItems) != 2 || !keys["version:2.1.288:broken"] || !keys["version:unread"] {
		t.Fatalf("both bars show: %+v", v.WarningItems)
	}
	if vf := v.Verification; vf.State != VerifyChanged || vf.Broken == "" || vf.Version != "2.1.288" || vf.Unread == "" {
		t.Fatalf("the field stays changed, and the read's failure is kept beside it: %+v", vf)
	}
}

// [VERCHECK-1-S4] One unmatched launch costs one more confirmation.
func TestVERCHECK1S4OneUnmatchedLaunchCostsOneMoreConfirmation(t *testing.T) {
	t.Parallel()
	m, _ := oneOrphan(t)
	v := m.Snapshot(t0.Add(14 * time.Second))
	if vf := v.Verification; vf.State != VerifyChecking || vf.Version != "2.1.288" || vf.Confirmed != 3 || vf.Needed != 4 || vf.Orphans != 1 || m.AutoVerified() != "" {
		t.Fatalf("3 of 4, not yet verified: %+v, auto %q", vf, m.AutoVerified())
	}
	if !laneApprox(v) {
		t.Fatal("the lane's list stops being approximate before the version is verified")
	}
}

// [VERCHECK-1-S5] The fourth confirmation clears it without a restart.
func TestVERCHECK1S5TheFourthConfirmationClearsIt(t *testing.T) {
	t.Parallel()
	m, hook := oneOrphan(t)
	launch(hook, "D", 14000)
	v := m.Snapshot(t0.Add(15 * time.Second))
	if m.AutoVerified() != "2.1.288" || laneApprox(v) || len(v.WarningItems) != 0 || v.Verification.State != VerifyVerified {
		t.Fatalf("4 of 4 verifies it: auto %q, approx %v, warnings %+v", m.AutoVerified(), laneApprox(v), v.WarningItems)
	}
}

// [VERCHECK-1-S6] An overdue launch counts before the confirmation that would verify.
func TestVERCHECK1S6AnOverdueLaunchCountsBeforeTheConfirmation(t *testing.T) {
	t.Parallel()
	m, hook := oneOrphan(t)
	unmatched(hook, "X", 14000)
	// The fourth launch is announced while X is not yet overdue, and its PostToolUse
	// arrives when X is, with no Agent hook between.
	hook("PreToolUse", "", "Agent", "tu-D", "", 15000)
	hook("SubagentStart", "D", "", "", "", 15020)
	hook("PostToolUse", "", "Agent", "tu-D", `{"agentId":"D","status":"completed"}`, 25100)
	v := m.Snapshot(t0.Add(26 * time.Second))
	if m.AutoVerified() != "" || !laneApprox(v) || v.Verification.Broken == "" || v.Verification.State != VerifyChanged {
		t.Fatalf("the second orphan is counted first: auto %q, approx %v, %+v", m.AutoVerified(), laneApprox(v), v.Verification)
	}
	found := false
	for _, w := range v.WarningItems {
		found = found || w.Key == "version:2.1.288:broken"
	}
	if !found {
		t.Fatalf("the break bar shows: %+v", v.WarningItems)
	}
}

// A launch whose own SubagentStart arrives more than 10 s after its PostToolUse is an
// orphan, never a confirmation (design.md shape 2).
func TestLateSubagentStartCountsAsAnOrphan(t *testing.T) {
	t.Parallel()
	m, hook := verifyModel(t, "2.1.288")
	hook("PreToolUse", "", "Agent", "tu-Z", "", 0)
	hook("PostToolUse", "", "Agent", "tu-Z", `{"agentId":"Z","status":"async_launched"}`, 50)
	hook("SubagentStart", "Z", "", "", "", 12000)
	v := m.Snapshot(t0.Add(13 * time.Second))
	if vf := v.Verification; vf.Confirmed != 0 || vf.Needed != 4 {
		t.Fatalf("a late start is not a confirmation: %+v", vf)
	}
}

// [VERCHECK-1-S1] A new version shows the field, not a bar: the state is checking, 0 of 3.
func TestVERCHECK1S1ANewVersionIsTheCheckingState(t *testing.T) {
	t.Parallel()
	m, _ := verifyModel(t, "2.1.288")
	v := m.Snapshot(t0)
	if vf := v.Verification; vf.State != VerifyChecking || vf.Version != "2.1.288" || vf.Confirmed != 0 || vf.Needed != 3 {
		t.Fatalf("the field reads 2.1.288 · checking, 0 of 3: %+v", vf)
	}
	if len(v.WarningItems) != 0 || len(v.Warnings) != 0 {
		t.Fatalf("no warning bar names the version: %+v", v.WarningItems)
	}
}

// [VERCHECK-1-S2] Three matched launches, none unmatched, verify the version, and the
// lane's Agents tab stops saying its list is approximate.
func TestVERCHECK1S2ThreeConfirmationsClearTheField(t *testing.T) {
	t.Parallel()
	m, hook := verifyModel(t, "2.1.288")
	launch(hook, "A", 0)
	launch(hook, "B", 1000)
	if !laneApprox(m.Snapshot(t0.Add(2 * time.Second))) {
		t.Fatal("approximate until the third")
	}
	launch(hook, "C", 2000)
	v := m.Snapshot(t0.Add(3 * time.Second))
	if v.Verification.State != VerifyVerified || v.Verification.Version != "" || m.AutoVerified() != "2.1.288" || laneApprox(v) {
		t.Fatalf("verified: %+v, auto %q, approx %v", v.Verification, m.AutoVerified(), laneApprox(v))
	}
}

// [VERCHECK-1-S3] A verified version shows no field and no bar: the source's own
// version, one restored from verified.json, and one the panel has not read yet.
func TestVERCHECK1S3AVerifiedVersionShowsNoField(t *testing.T) {
	t.Parallel()
	for name, setup := range map[string]func() *Model{
		"the source's": func() *Model { m, _ := verifyModel(t, HeuristicsVerifiedOn); return m },
		"restored": func() *Model {
			m, _ := verifyModel(t, "2.1.288")
			m.RestoreAutoVerified("2.1.288")
			return m
		},
		"not read yet": func() *Model { return NewModel(testConfig(t), "/repo", t0) },
	} {
		v := setup().Snapshot(t0)
		if v.Verification.State != VerifyVerified || len(v.WarningItems) != 0 {
			t.Errorf("%s: %+v %+v", name, v.Verification, v.WarningItems)
		}
	}
}

// [VERCHECK-2-S1] Two unmatched launches raise the bar, and the field reads changed.
func TestVERCHECK2S1TwoUnmatchedLaunchesRaiseTheBar(t *testing.T) {
	t.Parallel()
	m, hook := verifyModel(t, "2.1.288")
	unmatched(hook, "X", 0)
	unmatched(hook, "Y", 100)
	launch(hook, "A", 11000) // its SubagentStart ages them
	v := m.Snapshot(t0.Add(12 * time.Second))
	if len(v.WarningItems) != 1 || v.WarningItems[0].Key != "version:2.1.288:broken" ||
		!strings.Contains(v.WarningItems[0].Text, "Claude Code 2.1.288 changed what the subagent pairing relies on: 2 Agent calls named an agent no SubagentStart announced") {
		t.Fatalf("the break bar: %+v", v.WarningItems)
	}
	if vf := v.Verification; vf.State != VerifyChanged || vf.Version != "2.1.288" || vf.Broken == "" || vf.Unread != "" {
		t.Fatalf("the field reads changed: %+v", vf)
	}
}

// [VERCHECK-2-S2] An unreadable version on the panel's first read raises the bar.
func TestVERCHECK2S2AnUnreadableFirstReadRaisesTheBar(t *testing.T) {
	t.Parallel()
	m := NewModel(testConfig(t), "/repo", t0)
	if v := m.Snapshot(t0); len(v.WarningItems) != 0 || v.Verification.State != VerifyVerified {
		t.Fatalf("nothing is known before the first read: %+v %+v", v.WarningItems, v.Verification)
	}
	m.ApplyClaudeVersion("", errors.New("claude: not found"), t0)
	v := m.Snapshot(t0)
	if len(v.WarningItems) != 1 || v.WarningItems[0].Key != "version:unread" ||
		!strings.Contains(v.WarningItems[0].Text, "cannot read `claude --version` (claude: not found); subagent lists are approximate") {
		t.Fatalf("the unread bar: %+v", v.WarningItems)
	}
	if v.Verification.State != VerifyUnread || v.Verification.Version != "" {
		t.Fatalf("the state: %+v", v.Verification)
	}
}

// A break one project found on the running version survives another project's
// verification of it (design.md D2: a break was found on real evidence, so "changed"
// is first in the precedence). The machine pushes the verified version to every
// project, this one included, with ReplaceAutoVerified.
func TestABreakSurvivesAnotherProjectsVerification(t *testing.T) {
	t.Parallel()
	m, hook := verifyModel(t, "2.1.288")
	unmatched(hook, "X", 0)
	unmatched(hook, "Y", 100)
	launch(hook, "A", 11000) // its SubagentStart ages them: a break
	m.ReplaceAutoVerified("2.1.288")
	v := m.Snapshot(t0.Add(12 * time.Second))
	found := false
	for _, w := range v.WarningItems {
		found = found || w.Key == "version:2.1.288:broken"
	}
	if vf := v.Verification; vf.State != VerifyChanged || vf.Broken == "" || !found || !laneApprox(v) {
		t.Fatalf("the break is erased: %+v, bar %v, approx %v", vf, found, laneApprox(v))
	}
	if m.AutoVerified() != "2.1.288" || m.VerifiedRunning() != "2.1.288" {
		t.Fatalf("the verified version is still recorded for the machine to keep: %q", m.AutoVerified())
	}
}
