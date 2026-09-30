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
// verifyNeeded confirmations and no break verify the version; it is kept on disk
// (verified.json) so a restart does not start over. A break keeps the warning and
// says what broke: the page's subagent lists stay approximate, as before.
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

// Verification is what the page and the footer say about it.
type Verification struct {
	Version   string `json:"version,omitempty"` // the running version being checked
	Confirmed int    `json:"confirmed"`
	Needed    int    `json:"needed"`
	Broken    string `json:"broken,omitempty"`
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
	if _, ok := vr.await[ev.AgentID]; ok {
		delete(vr.await, ev.AgentID)
		m.confirm(vr)
	}
	m.ageOrphans(vr, now)
}

// observePost: an Agent call's PostToolUse, checked before the session applies it.
func (m *Model) observePost(s *session, ev signals.HookEvent, now time.Time) {
	if !m.checking() || !signals.IsAgentTool(ev.ToolName) {
		return
	}
	vr := m.verifierFor()
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
	m.ageOrphans(vr, now)
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
	if vr.confirmed >= verifyNeeded && vr.orphans == 0 {
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

// verification is the view of it.
func (m *Model) verification() Verification {
	out := Verification{Needed: verifyNeeded, Auto: m.v2.autoVerified}
	if m.checking() && m.v2.verify.version == m.v2.claudeVersion {
		out.Version, out.Confirmed, out.Broken = m.v2.verify.version, m.v2.verify.confirmed, m.v2.verify.broken
	} else if m.checking() {
		out.Version = m.v2.claudeVersion
	}
	return out
}
