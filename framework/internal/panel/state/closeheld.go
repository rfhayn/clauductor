package state

import (
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// The close verdict (PANEL-57 D2): whether a lane's work has ended, so that `clauductor
// lane close` and auto-close may close it. TermLaneView.CloseHeld carries it: "" when
// the lane may close, otherwise why not, in auto-close's words. A failed or missing
// read keeps more: it holds a lane, never releases one.

// CloseEndedWords is the verdict of a lane whose own session ended while its program
// still runs: after a /clear the same claude runs under a session id no lane record
// holds, so the panel cannot tell what it is doing.
const CloseEndedWords = "its session ended while claude still runs (a /clear?): close it from the page"

// CloseUnheardWords and CloseNotReportedWords hold a lane whose background work the
// panel cannot know: its last turn's Stop never came, or its Stops do not list it.
const (
	CloseUnheardWords     = "its last turn's Stop was not heard (interrupted, failed, or sent while the panel was down): what it left running is not known"
	CloseNotReportedWords = "its Stop does not report background work (this Claude Code version): what it left running is not known"
	// CloseNoTurnWords holds a claude that ran before the panel and has ended no turn
	// since; CloseWakeupWords one whose last Stop scheduled a wakeup still due.
	CloseNoTurnWords = "no turn of it has ended since the panel started: what it left running is not known"
	CloseWakeupWords = "its turn ended with a wakeup scheduled (a cron or a one-shot still due)"
)

// The rest of closeHeld's fixed words, constants so that docs/panel.md's refusals
// table is checked against every one (TestEveryCloseWordIsDocumented). A lease's
// refusal names its queue, and a status's is "claude is" and statusWords.
const (
	CloseQueuesUnreadWords  = "the queues' leases have not been read"
	CloseQueuesFailingWords = "the queues' leases are not answering"
	CloseNotReadWords       = "claude has not been read yet"
	// CloseWorkWords is followed by ": " and the task's description.
	CloseWorkWords = "its turn ended on background work"
)

// closeHook records what the verdict reads of a hook beyond the stall's marks: when
// the session's own last prompt came, so a restart can tell a turn of the process
// before it from one of its own.
func closeHook(s *session, ev signals.HookEvent, now time.Time) {
	if ev.Event == "UserPromptSubmit" && ev.AgentID == "" {
		s.ownPromptAt = now
	}
}

// closeListed is the verdict's L8 (stall.go): a listing that names another process
// than the session's last (a Restart resumes the id). With the new process's start
// listed, what came before that start died with the old one: the close Stop heard
// before it is dropped, and an unheard turn prompted before it no longer holds
// (closeSince). With no start listed, nothing tells the old process's Stop or prompt
// from one the new process sent before this listing, so nothing is dropped: the kept
// Stop and prompt are read as the new one's, which can only hold more, and only until
// the new process's next Stop replaces them.
func closeListed(s *session, a signals.Agent, first bool) {
	if otherProcess(s.closePID, s.closeStarted, a) {
		if a.StartedAt > 0 {
			since := time.UnixMilli(a.StartedAt)
			s.closeSince = since
			if s.closeStop != nil && s.closeStop.at.Before(since) {
				s.closeStop = nil
			}
		}
		s.firstPoll = false
	}
	if first {
		s.firstPoll = true
	}
	s.closePID, s.closeStarted = a.PID, a.StartedAt
}

// liveBeforePanel: the session's process may have run before the panel started. Its
// listed start says so when known; otherwise being listed by the panel's first poll.
func (m *Model) liveBeforePanel(s *session) bool {
	if s.closeStarted > 0 {
		return time.UnixMilli(s.closeStarted).Before(m.startedAt)
	}
	return s.firstPoll
}

// fillCloseHeld sets each lane's CloseHeld, reading the leases once, bound to every
// registered lane whether or not its tmux session runs: a lease outlives its claude.
func (m *Model) fillCloseHeld(views []TermLaneView, now time.Time) {
	_, _, queue := m.leasesIn(views, true)
	for i := range views {
		views[i].CloseHeld = m.closeHeld(views[i], queue, now)
	}
}

// CloseTmuxUnreadWords and CloseRunsAgainWords hold a close whose live tmux reading
// (types.LiveLane) is missing, or says the lane runs while the panel's last tmux poll
// showed its claude exited or its session gone (a Restart or a Resume since).
const (
	CloseTmuxUnreadWords = "tmux did not answer: whether its claude still runs is not known"
	CloseRunsAgainWords  = "its claude runs again since the panel last read tmux (a Restart or a Resume): the next reading decides"
)

// LaneView is lane id's view with its close verdict, and whether the panel lists it: a
// close re-checks the verdict under its lane lock, and needs the lanes and the leases
// only, not a whole Snapshot. live, when not nil, is what tmux says of the lane now,
// and only ever holds more: tmux not answering holds, and so does a claude it shows
// running that the last tmux poll showed exited or gone (the panel's readings of
// claude are of the process before). Otherwise the poll's facts stand: a claude tmux
// shows exited or gone now runs nothing a close could stop, whatever the poll says.
func (m *Model) LaneView(id string, now time.Time, live *types.LiveLane) (TermLaneView, bool) {
	views := m.terminalViews(now)
	for _, tv := range views {
		if tv.ID != id {
			continue
		}
		if live != nil {
			switch {
			case !live.Read:
				tv.CloseHeld = CloseTmuxUnreadWords
				return tv, true
			case live.Running && !live.Dead && (!tv.Running || tv.Dead):
				tv.CloseHeld = CloseRunsAgainWords
				return tv, true
			}
		}
		_, _, queue := m.leasesIn(views, true)
		tv.CloseHeld = m.closeHeld(tv, queue, now)
		return tv, true
	}
	return TermLaneView{}, false
}

// LaneCloseHeld is lane id's CloseHeld as LaneView reads it, or why it may not close
// when the panel no longer lists it: a close checks the verdict at the moment it acts.
func (m *Model) LaneCloseHeld(id string, now time.Time, live *types.LiveLane) string {
	if tv, ok := m.LaneView(id, now, live); ok {
		return tv.CloseHeld
	}
	return LaneGoneWords(id)
}

// LaneGoneWords is why lane id may not close once the panel no longer lists it.
func LaneGoneWords(id string) string { return "the panel no longer lists lane " + id }

// closeHeld is D2's verdict for one lane, in its order: the leases' source, a lease,
// an exited or gone claude, an ended session, the status, a current reading, a prompt
// or permission request heard since that reading's poll began, a turn whose Stop was not
// heard or Stops that report no background work, a claude from before the panel with no
// turn ended since, and the background work (wakeups included) of the lane's own
// session's last Stop. queue is leases' third map.
func (m *Model) closeHeld(tv TermLaneView, queue map[string]string, now time.Time) string {
	// Task 1.6: with queues declared, an unread or failing lease source could hide a
	// lease, and a lease covers a gate no Stop lists, so it holds even an exited claude.
	if len(m.cfg.Queues) > 0 {
		if !m.stall.queuesRead {
			return CloseQueuesUnreadWords
		}
		if !m.v2.queuesSrc.OK {
			return CloseQueuesFailingWords
		}
	}
	if q, ok := queue[tv.Worktree]; ok && tv.Worktree != "" {
		return "it holds or waits for the " + q + " lease"
	}
	if !tv.Running || tv.Dead {
		return "" // its session's background work ends with its claude
	}
	s := m.sessions[tv.SessionID]
	if tv.SessionID == "" {
		s = nil
	}
	// Before the reading test: s.end clears the reading, so an ended session would
	// otherwise read "claude has not been read yet". The hold lifts only on a reading
	// whose poll started after the end (task 1.4): one under way at a /clear still
	// lists the old session as it was.
	if s != nil && s.HookStatus == "ended" && (m.agentReading(s, now) == nil || !m.agentsPollStart.After(s.endedAt)) {
		return CloseEndedWords
	}
	if tv.Status != "idle" || tv.Approx {
		return "claude is " + statusWords(tv.Status, tv.Approx)
	}
	// sessionStatus answers idle, not approximate, with no reading and no hook status
	// at all: such a lane may be mid-turn.
	if s == nil || m.agentReading(s, now) == nil {
		return CloseNotReadWords
	}
	// A current reading outranks hooks in sessionStatus, but a prompt or a permission
	// request heard since the poll began may not be in it: the poll can have read the
	// session before the hook. Compared with the poll's start, not its apply time
	// (agentsOKAt), so a hook during the poll holds too; the next poll decides.
	if s.LastHookAt.After(m.agentsPollStart) {
		switch s.HookStatus {
		case "busy":
			return "claude is working"
		case "waiting":
			return "claude is waiting on you"
		}
	}
	// D2's "a failed read keeps more", as the stall's lastWaitedOn reads the same marks:
	// a turn whose Stop never came (Esc, a StopFailure, a panel down) may have started
	// work no kept Stop lists, and a Claude Code that sends no background_tasks lists
	// none at all.
	// A prompt the process before a restart took died with it (closeListed).
	if s.stall.unreported && !s.ownPromptAt.Before(s.closeSince) {
		return CloseUnheardWords
	}
	st := s.closeKept()
	if st != nil && !st.reported() {
		return CloseNotReportedWords
	}
	// The stall's "no turn of it has ended since the panel started": a process that
	// ran before the panel may have left work no Stop the panel heard lists. A Stop from
	// before this panel started (restored from stall.json) is no turn heard: a whole turn
	// while the panel was down leaves no prompt and no unheard mark behind it.
	if (st == nil || st.at.Before(m.startedAt)) && m.liveBeforePanel(s) {
		return CloseNoTurnWords
	}
	if w := closeWork(s); w != "" {
		return CloseWorkWords + ": " + w
	}
	// The owner's ruling of 2026-10-25: D2's background work includes the last Stop's
	// session_crons and wakeups, read as the stall's wakeupDue reads them.
	if st != nil && cronsDue(st, now) {
		return CloseWakeupWords
	}
	if m.sessionWorking(s, now) {
		return "claude is working" // closeWork reads all sessionWorking does, and more
	}
	return ""
}

// closeWork names the background work the session's last Stop listed that may still
// run: an open type (a shell, a monitor, a teammate) whatever its status, as turnWork
// reads it, and a subagent or workflow in flight, a subagent not since stopped. No
// unheard limit: a subagent quiet for long is still running until its stop is heard.
func closeWork(s *session) string {
	st := s.closeKept()
	if st == nil {
		return ""
	}
	stopped := map[string]bool{}
	for _, id := range s.stall.stops {
		stopped[id] = true
	}
	var names []string
	for _, t := range st.tasks {
		ended := t.typ == "subagent" && (t.ended || (t.id != "" && stopped[t.id]))
		if taskOpen(t.typ) || (taskWorking(t.typ) && inFlight(t.status) && !ended) {
			d := t.desc
			if d == "" {
				d = "a " + t.typ
			}
			names = append(names, d)
		}
	}
	return strings.Join(names, "; ")
}

// closeKept is the Stop the verdict reads: its own copy, which outlives the stall's
// (task 1.4), or the stall's while it has none.
func (s *session) closeKept() *keptStop {
	if s.closeStop != nil {
		return s.closeStop
	}
	return s.stall.stop
}

// statusWords is a lane status as auto-close words it, after "claude is ".
func statusWords(st string, approx bool) string {
	w := map[string]string{"busy": "working", "waiting": "waiting on you", "running": "starting"}[st]
	if w == "" {
		w = st
	}
	if approx {
		w += " (not a current reading)"
	}
	return w
}
