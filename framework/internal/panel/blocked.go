package panel

import (
	"fmt"
	"strings"
	"time"
)

// PANEL-5: one reading, one predicate.
//
// A session's `claude agents` entry is the last poll that listed it. Once polls fail
// or stop arriving it says what WAS true: a permission prompt answered in the
// terminal fires no hook, so only a fresh poll can say it was answered. Every
// surface that asks "is this session blocked on you" (Needs you, the waiting alert,
// the lane chip, the first-prompt decision) reads it through agentReading and
// blocked, so they cannot disagree, and anything resting on data that is not
// current is marked approximate. An approximate alert is shown and never becomes an
// OS notification (Interrupts).

// agentReading returns the session's `claude agents` entry only while it is a
// current reading: the last poll succeeded within two poll intervals. nil when the
// reading is stale, or the session is not listed.
func (m *Model) agentReading(s *session, now time.Time) *Agent {
	if s.Agent == nil || !m.agentsFresh(now) {
		return nil
	}
	return s.Agent
}

// staleWhy says why the session's last `claude agents` entry is not current.
func (m *Model) staleWhy(now time.Time) string {
	// No age in the text: the page shows how old the reading is (View.AgentsReadAt),
	// so the view does not change, and is not pushed, every second it gets older.
	if m.agentsOKAt.IsZero() {
		return "claude agents hasn't answered yet"
	}
	if !m.agentsSrc.OK {
		return "claude agents isn't answering, so this is its last answer"
	}
	return "claude agents hasn't answered lately"
}

// sessionStatus is the session's status as the page shows it: the current `claude
// agents` reading, else the hooks, else the stale reading. approx is set whenever
// it is not a current reading. An open waiting hook note makes it waiting whatever
// the reading says, as blocked does, so the lane chip and Needs you agree.
func (m *Model) sessionStatus(s *session, now time.Time) (status, waitingFor string, approx bool) {
	a := m.agentReading(s, now)
	if s.Note != nil && ClassifyNotification(s.Note.Type).Waiting && (a == nil || a.Status != "waiting") {
		// A hook says it waits and no current reading confirms it (see blocked).
		return "waiting", "", true
	}
	if a != nil {
		return a.Status, a.WaitingFor, false
	}
	hook := ""
	switch s.HookStatus {
	case "busy", "idle", "waiting":
		hook = s.HookStatus
	}
	if s.Agent != nil && (hook == "" || !s.LastHookAt.After(m.agentsOKAt)) {
		// The stale reading is newer than any hook: it is the best guess there is.
		return s.Agent.Status, s.Agent.WaitingFor, true
	}
	if hook != "" {
		return hook, "", true
	}
	return "idle", "", false
}

// blockInfo is what blocked reports about a session waiting on you.
type blockInfo struct {
	Kind     string // a notification type, or "waiting" (from `claude agents`)
	Since    time.Time
	Label    string
	Text     string
	Severity string
	// Approx: not confirmed by a current `claude agents` reading. Why says how.
	Approx bool
	Why    string
}

var waitingLabels = map[string]string{"permission": "Permission", "input": "Input needed", "sandbox": "Sandbox request",
	"worker": "Worker request", "dialog": "Dialog open"}

// blocked is THE predicate for "this session is blocked waiting on you". Needs
// you, the waiting alert and the first-prompt decision all read it.
//
//   - A waiting hook note (permission prompt, elicitation, agent input) is exact
//     only while a current reading also says waiting. Hooks alone cannot tell an
//     answered prompt from an open one, so otherwise it is approximate.
//   - Else the session's status (sessionStatus) being waiting: exact from a current
//     reading, approximate from a stale one or from hooks.
func (m *Model) blocked(s *session, now time.Time) (blockInfo, bool) {
	reading := m.agentReading(s, now)
	if s.Note != nil {
		if k := ClassifyNotification(s.Note.Type); k.Waiting {
			b := blockInfo{Kind: s.Note.Type, Since: s.Note.At, Label: k.Label, Text: s.Note.Message, Severity: SevBlock}
			// The hook's message is generic ("Claude needs your permission to use
			// Bash"); `claude agents` names the call ("permission: Bash(npm test)").
			// Needs you shows the specific one when there is one (PANEL-6).
			if a := s.Agent; a != nil && a.Status == "waiting" && a.WaitingFor != "" {
				b.Text = askText(a.WaitingFor)
			}
			switch {
			case reading == nil && s.Agent == nil && m.agentsFresh(now):
				b.Approx, b.Why = true, "only a hook says so; claude agents doesn't list the session"
			case reading == nil:
				b.Approx, b.Why = true, "only a hook says so; "+m.staleWhy(now)
			case reading.Status != "waiting":
				b.Approx, b.Why = true, "a hook says waiting, claude agents says "+reading.Status
			}
			return b, true
		}
	}
	st, wf, approx := m.sessionStatus(s, now)
	if st != "waiting" {
		return blockInfo{}, false
	}
	b := blockInfo{Kind: "waiting", Since: s.WaitingSince, Text: "waiting for input", Severity: SevBlock, Label: "Waiting"}
	if wf != "" {
		b.Text = askText(wf)
	}
	if l := waitingLabels[waitingForKind(wf)]; l != "" {
		b.Label = l
	}
	if approx {
		b.Approx = true
		if s.Agent != nil && s.Agent.Status == "waiting" {
			b.Why = m.staleWhy(now)
		} else {
			b.Why = "only hooks say so"
			b.Since = s.LastHookAt
		}
	}
	if b.Since.IsZero() {
		b.Since = s.LastEventAt
	}
	return b, true
}

// askText is a waitingFor as Needs you shows it under its label: "permission:
// Bash(npm test)" is labelled Permission, so the text is "Bash(npm test)".
func askText(wf string) string {
	if k := waitingForKind(wf); k != "" && strings.HasPrefix(strings.ToLower(wf), k+":") {
		if rest := strings.TrimSpace(wf[len(k)+1:]); rest != "" {
			return oneLine(rest)
		}
	}
	return oneLine(wf)
}

// approxLabel marks an approximate item's label so the page shows it as such.
func approxLabel(label string, b blockInfo) string {
	if !b.Approx {
		return label
	}
	return fmt.Sprintf("%s (≈ stale: %s)", label, b.Why)
}

// waitingNoteMaxAge bounds how long a session with an open waiting note is kept
// while polls fail (PANEL-7): a day without a hook or a status line from it.
const waitingNoteMaxAge = 24 * time.Hour

// forgetSessions drops sessions the panel has not heard from in forgetSessionAge:
// no hook, no status line, and no `claude agents` reading that recent. It runs on
// every poll, failed or not, so a failing poll never keeps a session forever. One
// with an open waiting note, which only a working poll can clear, is kept longer,
// but not without bound: until its lane's pane is dead (or its tmux session gone),
// or waitingNoteMaxAge passes without a word from it, whichever comes first.
func (m *Model) forgetSessions(now time.Time) {
	for id, s := range m.sessions {
		if s.Agent != nil && now.Sub(m.agentsOKAt) <= forgetSessionAge {
			continue
		}
		// While polls fail, nothing can say an open prompt was answered: a session
		// with a waiting note stays in Needs you (marked approximate) until one can.
		if !m.agentsSrc.OK && s.Note != nil && ClassifyNotification(s.Note.Type).Waiting {
			heard := s.LastHookAt
			if s.StatusAt.After(heard) {
				heard = s.StatusAt
			}
			if !m.lanePaneDead(id) && now.Sub(heard) <= waitingNoteMaxAge {
				continue
			}
			delete(m.sessions, id)
			delete(m.costByID, id)
			continue
		}
		if now.Sub(s.LastHookAt) > forgetSessionAge && now.Sub(s.StatusAt) > forgetSessionAge {
			delete(m.sessions, id)
			delete(m.costByID, id) // the est. $ sum covers tracked sessions only
		}
	}
}

// lanePaneDead: the session is a registered lane's, and a successful tmux poll
// shows that lane's pane dead or its session gone, so nothing in it can be waiting.
func (m *Model) lanePaneDead(sessionID string) bool {
	if !m.tmuxSrc.OK {
		return false
	}
	for _, rec := range m.laneRecords {
		if rec.SessionID != sessionID {
			continue
		}
		for _, tl := range m.tmuxLanes {
			if tl.ID == rec.ID {
				return tl.Dead
			}
		}
		return true
	}
	return false
}

// agentsPollCap bounds how much a slow poll widens the freshness window: each
// `claude agents` run times out at 10 s, and an iteration runs at most three (the
// filter cross-check and the poll).
const agentsPollCap = 30 * time.Second

// ApplyAgentsTimed is ApplyAgents for a poll iteration that took dur of wall time,
// the filter cross-check included. agentsFresh allows for it.
func (m *Model) ApplyAgentsTimed(agents []Agent, err error, dur time.Duration, now time.Time) {
	if dur > agentsPollCap {
		dur = agentsPollCap
	}
	m.agentsDurs[m.agentsDurN%len(m.agentsDurs)] = dur
	m.agentsDurN++
	m.ApplyAgents(agents, err, now)
}

// slowestRecentPoll is the longest of the last few poll iterations.
func (m *Model) slowestRecentPoll() time.Duration {
	var d time.Duration
	for _, x := range m.agentsDurs {
		if x > d {
			d = x
		}
	}
	return d
}
