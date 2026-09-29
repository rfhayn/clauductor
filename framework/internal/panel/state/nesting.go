package state

import (
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// Which agent started which (PANEL-11). Verified on Claude Code 2.1.284 with hooks
// alone (no transcript is read):
//   - a SubagentStart names only the new agent (agent_id, agent_type), never its caller;
//   - a PreToolUse(Agent) fired inside agent A carries A's agent_id (absent on the
//     main thread), and its SubagentStart follows 15-25 ms later;
//   - the PostToolUse of that call carries the caller (agent_id) and the new agent
//     (tool_response.agentId) together: exact. It arrives when a foreground agent
//     finishes, or at once for a background launch;
//   - workflow agents start as "workflow-subagent" with nothing naming their run; the
//     run's id and name come in the Workflow call's PostToolUse.
//
// So: a SubagentStart takes, provisionally, the caller of the oldest pending Agent call
// in the session with the same subagent type, made in the last pendingFor (Approx).
// The call's PostToolUse then confirms or corrects it. A workflow-subagent joins the
// session's newest workflow run, approximately: two overlapping runs in one session
// cannot be told apart. Subagents nest at most three levels below the session.

// pendingFor is how long an Agent call waits for its SubagentStart.
const pendingFor = 2 * time.Second

type pendingCall struct {
	toolUse, caller, typ, desc string
	at                         time.Time
}

type wfRun struct {
	id, name string
	at       time.Time
}

type nesting struct {
	pending []pendingCall
	edges   map[string]string // confirmed: child agent id → caller ("" is the session)
	runs    []wfRun           // newest last, at most 8
}

// pre records an Agent call; its detail is the feed line.
func (n *nesting) pre(ev signals.HookEvent, now time.Time) string {
	switch {
	case signals.IsAgentTool(ev.ToolName):
		typ, desc := ev.AgentCall()
		kept := n.pending[:0]
		for _, p := range n.pending {
			if now.Sub(p.at) < pendingFor {
				kept = append(kept, p)
			}
		}
		n.pending = append(kept, pendingCall{toolUse: ev.ToolUseID, caller: ev.AgentID, typ: typ, desc: desc, at: now})
		return signals.Clip(ev.ToolName+" "+typ+": "+desc, signals.DetailMax)
	case ev.ToolName == "Workflow":
		return "Workflow"
	}
	return ev.ToolName
}

// link places a starting subagent.
func (n *nesting) link(ev signals.HookEvent, now time.Time) agentLink {
	if ev.AgentType == "workflow-subagent" {
		if len(n.runs) > 0 {
			r := n.runs[len(n.runs)-1]
			return agentLink{Run: r.id, RunName: r.name, Approx: true}
		}
		return agentLink{}
	}
	var l agentLink
	for i, p := range n.pending {
		if now.Sub(p.at) < pendingFor && p.typ == ev.AgentType {
			l = agentLink{Parent: p.caller, Desc: p.desc, Approx: true}
			n.pending = append(n.pending[:i:i], n.pending[i+1:]...)
			break
		}
	}
	if parent, ok := n.edges[ev.AgentID]; ok {
		l.Parent, l.Approx = parent, false
	}
	return l
}

// post confirms an Agent call's edge, or records a workflow run; its detail is the
// feed line.
func (s *session) post(ev signals.HookEvent, now time.Time) string {
	child, run, name := ev.AgentResult()
	switch {
	case signals.IsAgentTool(ev.ToolName) && child != "":
		if s.nest.edges == nil {
			s.nest.edges = map[string]string{}
		}
		s.nest.edges[child] = ev.AgentID
		if len(s.nest.edges) > 256 { // a long session: forget the oldest-looking half
			for k := range s.nest.edges {
				delete(s.nest.edges, k)
				if len(s.nest.edges) <= 128 {
					break
				}
			}
		}
		if a, ok := s.Subagents[child]; ok {
			a.link.Parent, a.link.Approx = ev.AgentID, false
			s.Subagents[child] = a
		}
		for i := range s.Finished {
			if s.Finished[i].ID == child {
				s.Finished[i].link.Parent, s.Finished[i].link.Approx = ev.AgentID, false
			}
		}
		for i, p := range s.nest.pending {
			if p.toolUse == ev.ToolUseID {
				s.nest.pending = append(s.nest.pending[:i:i], s.nest.pending[i+1:]...)
				break
			}
		}
		return "started " + agentLabel("", child)
	case ev.ToolName == "Workflow" && run != "":
		s.nest.runs = append(s.nest.runs, wfRun{id: run, name: name, at: now})
		if len(s.nest.runs) > 8 {
			s.nest.runs = s.nest.runs[1:]
		}
		return signals.Clip("workflow "+name+" "+run, signals.DetailMax)
	}
	return ev.ToolName
}
