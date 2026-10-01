package state

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// Binding corrections and session ends (PANEL-21, UX pass 1 findings 3 and 4).
//
// A session is bound to a lane once, and a later `cd` never moves it. But the first
// binding can be made on stale inputs: when several lanes start together, a lane's
// claude can send a hook, or show in `claude agents`, before the worktree list has
// its new worktree or before the tmux poll has brought its registry record in. The
// enclosing worktree (the project root) then matched, and the session stayed there
// for good. So a binding made on stale inputs is corrected as the inputs catch up,
// using only what was true when it was made: the registry record, which owns the
// session id whatever claude's cwd, or the cwd the session had at first sight. A cwd
// seen later is never consulted, so a real `cd` still does not move a session.
//
// A lane that ends without a SessionEnd (a crash, SIGKILL, or Stop and Close while
// hooks or status posts are still in flight) must not linger: once its tmux session
// is gone or its registry record is removed, the session is retired and whatever
// else arrives from it is set aside, so a closed lane is never brought back.

const (
	// provisionalWindow is how long a cwd binding may still move to a deeper
	// worktree that holds its first cwd: long enough for the worktree list to catch
	// up (a 10 s poll, a 2 s watch), short enough that a worktree added much later
	// never takes a session that was really in its parent.
	provisionalWindow = 2 * time.Minute
	// lateGrace is how long after `claude agents` stops listing a session its late
	// hooks and status posts are set aside: what a dying claude still sends.
	lateGrace = 30 * time.Second
)

// recordLane returns the worktree a lane record's session belongs to, when that
// worktree is itself in the list: a record's path is always a worktree's own path,
// so an enclosing match means the list has not caught up yet, and says nothing.
func (m *Model) recordLane(sessionID string) (string, bool) {
	if sessionID == "" {
		return "", false
	}
	for _, rec := range m.laneRecords {
		if rec.SessionID != sessionID {
			continue
		}
		p := filepath.Clean(rec.Path)
		for _, w := range m.worktrees {
			if !w.Bare && w.Path == p {
				return w.Path, true
			}
		}
	}
	return "", false
}

// within: p is strictly inside dir.
func within(p, dir string) bool {
	return dir != "" && strings.HasPrefix(p, dir+string(filepath.Separator))
}

// rebindOne corrects one session's lane from what held when it was bound: its
// registry record's own worktree, or else a deeper worktree holding its first cwd
// that has appeared within provisionalWindow.
func (m *Model) rebindOne(s *session, now time.Time) {
	if p, ok := m.recordLane(s.ID); ok {
		m.moveSession(s, p)
		return
	}
	if s.bindCwd == "" || now.Sub(s.boundAt) > provisionalWindow {
		return
	}
	if i := signals.MatchWorktree(m.worktrees, s.bindCwd); i >= 0 && within(m.worktrees[i].Path, s.Lane) {
		m.moveSession(s, m.worktrees[i].Path)
	}
}

func (m *Model) moveSession(s *session, lane string) {
	if s.Lane == lane {
		return
	}
	s.Lane = lane
	// The lane's own last hook: the session's hooks were filed under the old lane.
	if s.LastHookAt.After(m.laneHookAt[lane]) {
		m.laneHookAt[lane] = s.LastHookAt
	}
}

// rebind runs rebindOne over every session, after the worktree list or the lane
// registry changed.
func (m *Model) rebind(now time.Time) {
	for _, s := range m.sessions {
		m.rebindOne(s, now)
	}
}

// ended: the session's lane has ended, so what arrives from it is set aside. Each
// late event keeps the mark fresh, so it only lapses after forgetSessionAge of quiet.
func (m *Model) ended(id string, now time.Time) bool {
	if id == "" {
		return false
	}
	if _, ok := m.endedIDs[id]; ok {
		m.endedIDs[id] = now
		return true
	}
	if s := m.sessions[id]; s != nil && !s.goneAt.IsZero() && now.Sub(s.goneAt) < lateGrace {
		return true
	}
	return false
}

// retire ends a session whose lane is gone: nothing about it shows any more, as if
// its SessionEnd had arrived. Its cost stays in the total until it is forgotten.
func (m *Model) retire(id string, now time.Time) {
	if id == "" {
		return
	}
	m.endedIDs[id] = now
	s := m.sessions[id]
	if s == nil {
		return
	}
	if s.Lane != "" && s.HookStatus != "ended" {
		m.feedFor(m.worktreeByPath(s.Lane), id, "session", "lane ended", now)
	}
	s.end(now)
}

// end clears everything that would show the session as live or asking.
func (s *session) end(now time.Time) {
	s.HookStatus = "ended"
	s.Agent = nil
	s.Note, s.Done, s.Compacting = nil, nil, ""
	s.BusySince, s.IdleSince, s.WaitingSince = time.Time{}, time.Time{}, time.Time{}
	s.clearSubagents(now)
}

// trackLaneEnds compares the lanes live on tmux (a registry record with a running
// pane) with the last successful poll's: a lane that was live and is not any more
// (its record removed, its tmux session gone, or its pane dead) retires its session.
// A lane live again (Restart, Resume: a new tmux session, so a new creation time)
// takes its session back, and what its new claude sends is applied at once.
func (m *Model) trackLaneEnds(lanes []types.TmuxLane, recs []types.LaneRecord, now time.Time) {
	alive := map[string]int64{}
	for _, tl := range lanes {
		if !tl.Dead {
			alive[tl.ID] = tl.Created
		}
	}
	live := map[string]liveLane{}
	for _, rec := range recs {
		if created, ok := alive[rec.ID]; ok && rec.SessionID != "" {
			live[rec.ID] = liveLane{session: rec.SessionID, created: created}
		}
	}
	for id, was := range m.liveLanes {
		if live[id] != was {
			m.retire(was.session, now)
		}
	}
	for id, l := range live {
		delete(m.endedIDs, l.session)
		if m.liveLanes[id] != l {
			if s := m.sessions[l.session]; s != nil {
				s.goneAt = time.Time{}
			}
		}
	}
	m.liveLanes = live
}

// liveLane is a lane live on tmux: its session id, and its tmux session's creation.
type liveLane struct {
	session string
	created int64
}

// pruneEnded forgets end marks nothing has arrived for in forgetSessionAge.
func (m *Model) pruneEnded(now time.Time) {
	for id, at := range m.endedIDs {
		if now.Sub(at) > forgetSessionAge {
			delete(m.endedIDs, id)
		}
	}
}
