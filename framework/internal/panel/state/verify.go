package state

import (
	"fmt"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// PANEL-13: re-verifying the subagent pairing on a new Claude Code, from the hooks
// your own sessions already send. No model token, no session of its own.
//
// The pairing (nesting.go) rests on two undocumented facts: a SubagentStart names the
// new agent (agent_id), and the Agent call's PostToolUse names that same agent
// (tool_response.agentId). On a version the panel has not verified, every Agent
// PostToolUse is checked against the SubagentStarts it has seen:
//   - it names an agent whose SubagentStart arrived: a confirmation;
//   - it names an agent no SubagentStart announced within orphanAfter: an orphan
//     (a background launch's PostToolUse can come first, hence the wait);
//   - it names no agent at all: unnamed (an error response does this too, so only a
//     run of them with no confirmation counts against the version).
//
// verifyNeeded confirmations, plus one per orphan while the orphans stay below
// orphansBreak, verify the version (PANEL-31, D5); it is kept on disk (verified.json)
// so a restart does not start over. A break keeps the warning and says what broke:
// the page's subagent lists stay approximate, as before.
const (
	verifyNeeded = 3
	orphanAfter  = 10 * time.Second
	orphansBreak = 2
	unnamedBreak = 3
)

type verifier struct {
	version   string // the version these counts are for
	confirmed int
	orphans   int
	unnamed   int
	await     map[string]time.Time // agent ids a PostToolUse named, before their SubagentStart
	broken    string               // what broke, once something has
}

// The check's state (PANEL-31), the one the status-bar field and the version dialog
// read. The first that holds wins: a break was found on real evidence, a failed read
// says nothing about the pairing, and a check in progress is the quietest.
const (
	VerifyChanged  = "changed"  // the pairing changed on the running version
	VerifyUnread   = "unread"   // the latest `claude --version` failed
	VerifyChecking = "checking" // the running version is being checked
	VerifyVerified = "verified" // nothing to show (this includes: not read yet)
)

// Verification is what the page and the footer say about it.
type Verification struct {
	State     string `json:"state"`
	Version   string `json:"version,omitempty"` // the running version being checked
	Confirmed int    `json:"confirmed"`
	// Needed is the confirmations that verify it: verifyNeeded, plus one per orphan.
	Needed int `json:"needed"`
	// Orphans is the launches that named an agent no SubagentStart announced.
	Orphans int    `json:"orphans"`
	Broken  string `json:"broken,omitempty"`
	// Unread is why the latest `claude --version` failed, if it did.
	Unread string `json:"unread,omitempty"`
	// Auto is the version verified from live hooks (verified.json), if any.
	Auto string `json:"auto,omitempty"`
}

// checking reports whether the running version is one to check: known, and verified
// neither in the source (HeuristicsVerifiedOn) nor from live hooks.
func (m *Model) checking() bool {
	v := m.v2.claudeVersion
	return m.v2.versionSet && v != HeuristicsVerifiedOn && v != m.v2.autoVerified
}

// verifierFor returns the verifier for the running version, starting afresh when the
// version changed.
func (m *Model) verifierFor() *verifier {
	vr := &m.v2.verify
	if vr.version != m.v2.claudeVersion {
		*vr = verifier{version: m.v2.claudeVersion, await: map[string]time.Time{}}
	}
	return vr
}

// observeStart: a SubagentStart arrived. An agent a PostToolUse already named is now
// confirmed.
func (m *Model) observeStart(ev signals.HookEvent, now time.Time) {
	if !m.checking() || ev.AgentID == "" {
		return
	}
	vr := m.verifierFor()
	// Aged first (D5): a launch more than orphanAfter overdue is an orphan before any
	// confirmation can verify the version, and a SubagentStart that late is one too.
	m.ageOrphans(vr, now)
	if _, ok := vr.await[ev.AgentID]; ok {
		delete(vr.await, ev.AgentID)
		m.confirm(vr)
	}
}

// observePost: an Agent call's PostToolUse, checked before the session applies it.
func (m *Model) observePost(s *session, ev signals.HookEvent, now time.Time) {
	if !m.checking() || !signals.IsAgentTool(ev.ToolName) {
		return
	}
	vr := m.verifierFor()
	m.ageOrphans(vr, now) // before any confirmation, as in observeStart; this launch is not overdue
	child, _, _ := ev.AgentResult()
	switch {
	case child == "":
		vr.unnamed++
		if vr.unnamed >= unnamedBreak && vr.confirmed == 0 && vr.broken == "" {
			vr.broken = fmt.Sprintf("%d Agent calls' PostToolUse named no agent (tool_response.agentId)", vr.unnamed)
		}
	case s.knowsAgent(child):
		m.confirm(vr)
	default:
		vr.await[child] = now
	}
}

// knowsAgent reports whether the session has seen this agent's SubagentStart.
func (s *session) knowsAgent(id string) bool {
	if _, ok := s.Subagents[id]; ok {
		return true
	}
	for _, f := range s.Finished {
		if f.ID == id {
			return true
		}
	}
	return false
}

func (m *Model) confirm(vr *verifier) {
	if vr.broken != "" {
		return
	}
	vr.confirmed++
	if vr.confirmed >= verifyNeeded+vr.orphans && vr.orphans < orphansBreak {
		m.v2.autoVerified = vr.version
	}
}

// ageOrphans turns an agent still unannounced after orphanAfter into an orphan.
func (m *Model) ageOrphans(vr *verifier, now time.Time) {
	for id, at := range vr.await {
		if now.Sub(at) >= orphanAfter {
			delete(vr.await, id)
			vr.orphans++
		}
	}
	if vr.orphans >= orphansBreak && vr.broken == "" {
		vr.broken = fmt.Sprintf("%d Agent calls named an agent no SubagentStart announced (agent_id no longer matches tool_response.agentId)", vr.orphans)
	}
}

// AutoVerified is the Claude Code version verified from live hooks ("" for none),
// for the runtime to keep on disk.
func (m *Model) AutoVerified() string { return m.v2.autoVerified }

// RestoreAutoVerified puts back the version a previous panel verified.
func (m *Model) RestoreAutoVerified(v string) {
	if m.v2.autoVerified == "" {
		m.v2.autoVerified = v
	}
}

// ReplaceAutoVerified makes v the verified version, replacing an older one: the
// machine's, when any project verified the running version (PANEL-31, design.md shape 3).
func (m *Model) ReplaceAutoVerified(v string) {
	if v != "" {
		m.v2.autoVerified = v
	}
}

// VerifiedRunning is the version verified from live hooks, when it is the running one
// ("" otherwise): what the machine keeps and shares. An older verified version a
// restart restored is not it.
func (m *Model) VerifiedRunning() string {
	if m.v2.versionSet && m.v2.autoVerified == m.v2.claudeVersion {
		return m.v2.autoVerified
	}
	return ""
}

// breakFound: this project's own check found a break on the running version. It holds
// even when another project's verification has since made the version verified here
// (ReplaceAutoVerified): a break was found on real evidence, so it stays first in the
// precedence (design.md D2). It is in memory only; a restart forgets it.
func (m *Model) breakFound() bool {
	vr := m.v2.verify
	return m.v2.versionSet && vr.version == m.v2.claudeVersion && vr.broken != ""
}

// readFailed: the latest `claude --version` failed, even if an earlier read worked.
func (m *Model) readFailed() bool {
	s := m.v2.versionSrc
	return !s.Pending && !s.OK && s.Error != ""
}

// verification is the view of it: one state (verify.go's constants), computed here
// and nowhere else, so the page renders it without deciding anything.
func (m *Model) verification() Verification {
	out := Verification{State: VerifyVerified, Needed: verifyNeeded, Auto: m.v2.autoVerified}
	checking := m.checking()
	if checking || m.breakFound() {
		out.Version = m.v2.claudeVersion
		if vr := m.v2.verify; vr.version == m.v2.claudeVersion {
			out.Confirmed, out.Orphans, out.Broken = vr.confirmed, vr.orphans, vr.broken
			out.Needed = verifyNeeded + vr.orphans
		}
	}
	if m.readFailed() {
		out.Unread = m.v2.versionSrc.Error
	}
	switch {
	case out.Broken != "":
		out.State = VerifyChanged
	case out.Unread != "":
		out.State = VerifyUnread
		out.Version = "" // the version last read is not treated as current
	case checking:
		out.State = VerifyChecking
	}
	return out
}
