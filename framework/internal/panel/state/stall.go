package state

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// PANEL-51: a lane that has stopped moving (design D1, D3, D4). A lane is stalled
// when one of its sessions has waited on the owner for the stall time since it asked;
// or when one of its sessions ended its turn on background work, or the lane has open
// work, and that session or lane is not working and has not moved for the stall time.
//
// Three clocks: a question is timed from the ask alone (blocked's Since); a session's
// own clock is reset by its own movement; the lane's by any movement in it. Each way
// of stalling is judged only once the sources it rests on have answered since the
// panel started, so a source that has not answered never reads as "nothing open".

// Tunables of the stall rule.
const (
	// stallUnheard × the stall time is how long an unpaired subagent or workflow task
	// holds its session working with no movement: past it, its end was never heard
	// (a dropped hook, one sent while the panel was down), and it is open work.
	stallUnheard = 4
	// stallStopRing is how many SubagentStop agent ids a session keeps, to pair a
	// stop that arrived before the Stop listing its task (D3 (d)).
	stallStopRing = 64
	// stallClip bounds a kept description and last line, in runes (D4).
	stallClip = 120
	// wakeupGrace is how long past a one-shot's fire time it still holds a stall off.
	wakeupGrace = 5 * time.Minute
	// stallSourceWait is how long after the start the view says stalls are pending,
	// and after which a source still not answered is named in a warning (D1).
	stallSourceWait = 10 * time.Minute
)

// keptTask is one task a session's last Stop listed: D4's allowlist, and the mark of
// the SubagentStop that ended it.
type keptTask struct {
	id, typ, status, desc string
	ended                 bool
}

// keptCron is one cron a session's last Stop listed. fireAt is a one-shot's fire
// time, resolved once as the Stop is kept; zero holds a stall off while the list
// stands (a recurring cron, or a one-shot of neither shape D4 reads).
type keptCron struct {
	schedule  string
	recurring bool
	fireAt    time.Time
}

// keptStop is what a session's last Stop said it ended waiting on (D4). hasTasks and
// hasCrons: a Stop of the session carried background_tasks, and session_crons; with
// neither, a Claude Code that does not send them (absent is not empty: rule (a)).
type keptStop struct {
	at                 time.Time
	hasTasks, hasCrons bool
	tasks              []keptTask
	crons              []keptCron
	last               string
}

// reported: what the session's turns end waiting on is known.
func (k *keptStop) reported() bool { return k.hasTasks || k.hasCrons }

// clone is a copy of k whose task and cron lists are its own, so marking a task ended
// in the copy leaves k as it was.
func (k *keptStop) clone() *keptStop {
	if k == nil {
		return nil
	}
	c := *k
	c.tasks = append([]keptTask(nil), k.tasks...)
	c.crons = append([]keptCron(nil), k.crons...)
	return &c
}

// overlay is the owner's rule (a) in one place: the Stop top laid over the kept base,
// each field top does not carry keeping base's, so a Stop with no background_tasks ends
// no workflow and changes no kept task or cron. keepStop lays a Stop heard over the one
// kept; merged lays a record's kept Stop over an entry's (L13).
func overlay(base, top *keptStop) *keptStop {
	if top == nil || base == nil {
		if top == nil {
			return base
		}
		return top
	}
	k := *top
	if !k.hasTasks {
		k.tasks, k.hasTasks = base.tasks, base.hasTasks
	}
	if !k.hasCrons {
		k.crons, k.hasCrons = base.crons, base.hasCrons
	}
	return &k
}

// stallSession is a session's part of the rule: where it is in its lifecycle, its own
// clock, its kept Stop and the agent ids of its last SubagentStops, oldest first. Only
// the lifecycle's transitions below change it.
type stallSession struct {
	life  stallLife
	moved time.Time
	stop  *keptStop
	stops []string
	// unreported: the session sent UserPromptSubmit and no Stop has followed, so the
	// kept Stop is an earlier turn's (a turn interrupted with Esc, or a StopFailure,
	// sends none): (d) is unknown until its next Stop; its wakeups still hold (#186).
	unreported bool
}

// stallLife is where a session the model holds is in its lifecycle. A session set
// aside or saved is not in the model: it is in stallModel's aside or saved.
type stallLife uint8

const (
	lifeHeard stallLife = iota // heard by its hooks, never listed by `claude agents`
	lifeLive                   // listed by a successful poll (kept while the polls fail), not ended since
	lifeEnded                  // SessionEnd, gone from a successful poll, its lane retired, or another process listed
)

// stallKeep is a session's stall state kept apart from the model: across a restart
// (stall.json), or while `claude agents` polls fail and forgetSessions dropped it.
// pid and started name the process it was kept from (0: not known).
type stallKeep struct {
	moved, askAt        time.Time
	stalled, askStalled bool
	// approx, askApprox: that way held at the save only approximately (StallLaneMark).
	approx, askApprox bool
	stop              *keptStop
	stops             []string
	unreported        bool
	pid               int
	started           int64
}

// stallModel is the model's part of the rule.
type stallModel struct {
	time time.Duration
	loc  *time.Location
	// laneMove is each lane's last movement, by worktree path.
	laneMove map[string]time.Time
	// Whether each source a way rests on has answered since the start.
	prsRead, receiptRead, tmuxRead, queuesRead, mergedOnce bool
	// launched: the last registry read ApplyTmux was given has lanes the panel
	// launched, so tmux is a source (D1). Set on a failed tmux poll too, which never
	// sets laneRecords.
	launched bool
	// merged is the merged read's last result per lane (D10), as the runtime keeps it.
	merged map[string]MergedRead
	// leftOpen is when each branch last left the open-PR list (a successful `gh pr
	// list` that no longer lists it), so a "none" read before then is no result: D10's
	// "none" is read again when the branch leaves the list, and until that read answers
	// (b) is unknown, rather than standing on a "none" that may now be a merge
	// (review round 1 of group 3, M1).
	leftOpen map[string]time.Time
	// leased is the lanes holding or waiting for a lease at the last queue poll, and
	// deadSeen the dead or stale holders already seen, so each is given up once.
	leased   map[string]bool
	deadSeen map[string]bool
	// panes is each lane record's pane at the last tmux poll: alive, dead or gone.
	panes map[string]string
	// aside is the sessions forgetSessions dropped while the polls failed; saved is
	// what stall.json restored. The next successful poll uses the ones it lists.
	aside map[string]stallKeep
	saved map[string]stallKeep
	// carried is each way whose clock stall.json marked stalled at the save and the
	// restart restored (wayKey): only those are exact within a stall time of the start.
	carried map[string]bool
	// restored is each way stall.json marked stalled at the save (wayKey), with the clock
	// the restart restored for it, so a save made before the way's sources have answered
	// writes that mark again rather than "not stalled" (StallState).
	restored map[string]restoredMark
}

// restoredMark is a way's stalled mark as a restart restored it: its clock, and whether
// it held only approximately.
type restoredMark struct {
	moved  time.Time
	approx bool
}

// wayKey names one way of stalling by the clock it rests on: "lane" a lane's own open
// work (D3 (a) to (c)), "turn" a session's background work (D3 (d)), "ask" its question.
func wayKey(way, id string) string { return way + ":" + id }

// newStallModel starts the rule with the project's stall time (alerts.stall_minutes,
// D5; 0 turns stalls off).
func newStallModel(stall time.Duration) stallModel {
	return stallModel{time: stall, loc: time.Local, laneMove: map[string]time.Time{},
		merged: map[string]MergedRead{}, leftOpen: map[string]time.Time{}, leased: map[string]bool{}, deadSeen: map[string]bool{}, panes: map[string]string{},
		aside: map[string]stallKeep{}, carried: map[string]bool{}, restored: map[string]restoredMark{}}
}

// MergedRead is the merged read's last result for one lane (D10): the branch and HEAD
// it was for, whether a read has succeeded for them (a failed read keeps the last
// result, so with none it is unknown, never "none"), the pull request found merged
// from the branch, 0 for none, and when that read was made (its poll's time), so a
// "none" made before the branch left the open-PR list is no result (stallModel.leftOpen).
type MergedRead struct {
	Branch string
	Head   string
	Read   bool
	PR     int
	At     time.Time
}

// ApplyMergedReads records the merged read's results, by lane path (D10).
func (m *Model) ApplyMergedReads(r map[string]MergedRead) {
	m.stall.merged = make(map[string]MergedRead, len(r))
	for k, v := range r {
		m.stall.merged[k] = v
	}
	// The source has succeeded since the start once it has answered for every lane that
	// needed it, the receipts and pull requests read (D1); a lane that needs it later is
	// not a source that never answered.
	if !m.stall.mergedOnce && m.stall.receiptRead && m.stall.prsRead {
		once := true
		for _, wt := range m.worktrees {
			if _, ok := m.mergedKnown(wt); !wt.Bare && m.needsMerged(wt) && !ok {
				once = false
			}
		}
		m.stall.mergedOnce = once
	}
}

// ---- a session's lifecycle ----
//
// A session's stall state (its clock, its kept Stop, its SubagentStop ids and its ask
// time) changes only through the transitions below, so a session's end, a resume, a
// forget while the polls fail and a restart each keep, drop or restore it one way.
// `claude --resume` keeps the session id (the panel's Resume and Restore rely on it),
// so a process is told apart by its `claude agents` PID and start time. The table, as
// tasks.md's Decision log holds it:
//
//	L1  a hook                          movement, but a Notification that reports a stop;
//	                                    the session's own Stop replaces each field it
//	                                    carries (an absent one keeps what was kept), a Stop
//	                                    with an agent_id is never kept; its own
//	                                    UserPromptSubmit marks its turn unreported until
//	                                    its next Stop (#186); a SubagentStop's id joins the
//	                                    ring and ends its task; SessionEnd is L7
//	L2  the first successful poll       live, not movement: its clock is the panel's start
//	    lists it                        or its hooks'
//	L3  ... saved, the same process     live; clock, Stop, ring (merged with ids heard
//	                                    since) and a still-waiting ask restored
//	L4  a later poll lists one not live live: a start, which is movement; what was heard
//	                                    since its end is kept
//	L5  ... set aside, the same process live, restored as in L3
//	L6  a poll reads a live one changed movement
//	L7  SessionEnd, gone from a         ended: its Stop dropped, since what it listed died
//	    successful poll, lane retired   with the process, and any entry set aside or saved
//	                                    for it; the ring kept (ids are unique)
//	L8  a poll lists another process    L7, keeping a Stop heard since the new process
//	                                    started; then L4
//	L9  forgotten while polls fail,     set aside: clock, Stop, ring, ask and process
//	    live
//	L10 forgotten otherwise             gone: listed again, it is L4
//	L11 a successful poll does not list gone (listed by it under another process: L2 or
//	    one saved or set aside          L4)
//	L12 a restart (stall.json)          live, set-aside and saved ones saved; a stalled
//	                                    clock keeps its time, any other its quiet time
//	L13 a record heard while its id is  continues that entry: restored under it when
//	    set aside or saved              listed (L3, L5), saved merged with it (L12),
//	                                    merged back when forgotten (L10); one merge for all

// live: listed by `claude agents` and not ended since.
func (s *session) live() bool { return s.stall.life == lifeLive }

// otherProcess: the listing a names another process than pid and started (0: not
// known, which matches any).
func otherProcess(pid int, started int64, a signals.Agent) bool {
	return pid > 0 && a.PID > 0 && pid != a.PID || started > 0 && a.StartedAt > 0 && started != a.StartedAt
}

// stallHook is L1: one hook from the session.
func (m *Model) stallHook(s *session, ev signals.HookEvent, now time.Time) {
	if hookMoves(ev) {
		m.stallMove(s, now)
	}
	switch ev.Event {
	case "Stop":
		// A Stop with an agent_id is not the session's own: a teammate's hooks arrive
		// under its lead's session id (D9). Unprobed whether one is ever sent: a guard.
		if ev.AgentID == "" {
			m.keepStop(s, ev, now)
		}
	case "UserPromptSubmit":
		// A turn begins; until its Stop, the kept one is the turn before's (#186). A
		// teammate's, under its lead's id, is not the lead's turn, as rule (b) reads a Stop.
		if ev.AgentID == "" {
			s.stall.unreported = true
		}
	case "SubagentStop":
		s.stall.subagentStopped(ev.AgentID)
	case "SessionEnd":
		s.stall.end(time.Time{})
		m.stallDropKept(s.ID)
	}
}

// stallListed is L2 to L6 and L8: a successful poll lists s as a; was is the reading
// before it (nil for none). Another process under the id (a resume the polls did not
// see the gap of: no SessionEnd heard, or one heard while the exiting process was still
// listed) ends the one before it, keeping a Stop heard since the new one started (L8);
// the process that ended, still listed as it exits, stays ended once a poll has listed
// it (one heard only by hooks, its SessionEnd heard before any poll listed it, is live
// again at that listing, one movement its SessionEnd already gave). A session not live
// just before takes back what was saved or set aside for its process, else is a start
// (L4), but at the first poll after the start (L2).
func (m *Model) stallListed(s *session, was *signals.Agent, a signals.Agent, first bool, now time.Time) {
	prev := ""
	if was != nil {
		prev = was.Status
		switch {
		case otherProcess(was.PID, was.StartedAt, a):
			var since time.Time
			if a.StartedAt > 0 {
				since = time.UnixMilli(a.StartedAt)
			}
			s.stall.end(since)
		case s.stall.life == lifeEnded:
			return
		}
	}
	wasLive := s.live()
	s.stall.life = lifeLive
	if !wasLive {
		k, ok := m.stall.saved[s.ID]
		if !ok {
			k, ok = m.stall.aside[s.ID]
		}
		if ok && !otherProcess(k.pid, k.started, a) {
			m.restoreKeep(s, k, a.Status == "waiting")
			return
		}
	}
	switch {
	case first:
	case !wasLive, prev != a.Status:
		m.stallMove(s, now)
	}
}

// stallPolled is L11: what was saved or set aside is used by the successful poll that
// follows, or not at all.
func (m *Model) stallPolled() { m.stall.saved, m.stall.aside = nil, map[string]stallKeep{} }

// stallForgotten is L9, L10 and L13: forgetSessions dropped s. A live session dropped
// while the polls fail is set aside; a record heard while its id was set aside or saved
// is merged back into that entry; any other is gone.
func (m *Model) stallForgotten(id string, s *session, now time.Time) {
	switch {
	case !m.agentsSrc.OK && s.live():
		m.stall.aside[id] = m.keepOf(s, now)
	case s.stall.life == lifeHeard:
		if k, ok := m.stall.aside[id]; ok {
			m.stall.aside[id] = k.merged(s.stall)
		}
		if k, ok := m.stall.saved[id]; ok {
			m.stall.saved[id] = k.merged(s.stall)
		}
	}
}

// end is L7: the session's process ended. Its kept Stop is dropped, but for one heard
// at or after since, a new process's start (L8; zero drops it whatever its time). The
// SubagentStop ids are kept: they are unique, so they pair nothing of a later process.
func (st *stallSession) end(since time.Time) {
	if st.stop != nil && (since.IsZero() || st.stop.at.Before(since)) {
		st.stop = nil
	}
	st.life = lifeEnded
}

// stallMove is a session's movement: its clock and its lane's go back to zero.
func (m *Model) stallMove(s *session, now time.Time) {
	s.stall.moved = now
	if s.Lane != "" {
		m.stall.laneMove[s.Lane] = now
	}
}

// hookMoves: every hook is movement but a Notification that reports a stop, which
// arrives after the Stop or the ask it reports (E1, E3).
func hookMoves(ev signals.HookEvent) bool {
	if ev.Event != "Notification" {
		return true
	}
	k := signals.ClassifyNotification(ev.NotificationType)
	return !k.Waiting && !k.Done && !k.SetsIdle
}

// sessionMoved is the session's own last movement; the panel's start when none is
// known.
func (m *Model) sessionMoved(s *session) time.Time {
	if s.stall.moved.IsZero() {
		return m.startedAt
	}
	return s.stall.moved
}

// laneMoved is the lane's last movement; the panel's start when none is known.
func (m *Model) laneMoved(path string) time.Time {
	if t, ok := m.stall.laneMove[path]; ok {
		return t
	}
	return m.startedAt
}

// merged is k continued by what a record of its session has heard since (L13): the
// newer clock, the record's Stop if it has one, and its SubagentStop ids after k's,
// each kept task paired with them. One merge for a restore (L3, L5), a save (L12) and
// a forget (L10), so the three cannot keep different things.
func (k stallKeep) merged(st stallSession) stallKeep {
	if st.moved.After(k.moved) {
		k.moved = st.moved
	}
	// A copy: overlay may hand back k's own Stop or share its lists, and a save builds
	// this merge from an entry it must leave as it is (StallState through continued).
	k.stop = overlay(k.stop, st.stop).clone()
	// The record's mark is the newer when it heard a Stop or a prompt (#186).
	if st.stop != nil || st.unreported {
		k.unreported = st.unreported
	}
	k.stops = append(append([]string(nil), k.stops...), st.stops...)
	if n := len(k.stops); n > stallStopRing {
		k.stops = k.stops[n-stallStopRing:]
	}
	pairStops(k.stop, k.stops)
	return k
}

// restoreKeep gives a session its kept state back under the record a poll lists (L3,
// L5), merged with what the record heard since: a clock it has moved forward stays, and
// a SubagentStop heard between, or kept before a Stop heard then, ends the kept task
// it pairs with, as keepStop pairs them.
func (m *Model) restoreKeep(s *session, k stallKeep, waiting bool) {
	pre := k.moved
	k = k.merged(s.stall)
	// The turn's mark, for a save made before its sources answer; not once the record
	// heard since has moved the clock, which ended that stall.
	if k.stalled && k.moved.Equal(pre) {
		m.stall.restored[wayKey("turn", s.ID)] = restoredMark{moved: pre, approx: k.approx}
	}
	s.stall.moved, s.stall.stop, s.stall.stops, s.stall.unreported = k.moved, k.stop, k.stops, k.unreported
	// A way that held exact at the save is carried exact into the window (option B).
	if k.stalled && !k.approx {
		m.stall.carried[wayKey("turn", s.ID)] = true
	}
	if waiting && s.Note == nil && !k.askAt.IsZero() {
		s.WaitingSince = k.askAt
		if k.askStalled && !k.askApprox {
			m.stall.carried[wayKey("ask", s.ID)] = true
		}
	}
}

// continued is the saved or set-aside entry k of id, continued by a record heard for
// id since, if the model holds one (L13); a record that ended dropped the entry (L7).
func (m *Model) continued(id string, k stallKeep) stallKeep {
	if s := m.sessions[id]; s != nil && s.stall.life == lifeHeard {
		return k.merged(s.stall)
	}
	return k
}

// stallDropKept is L7's end of a session not in the model, or set aside or saved
// beside a record heard since: what was kept for the process that ended is dropped.
func (m *Model) stallDropKept(id string) {
	delete(m.stall.aside, id)
	delete(m.stall.saved, id)
}

// keepOf is the session's stall state, to keep aside or save, with its process.
func (m *Model) keepOf(s *session, now time.Time) stallKeep {
	k := stallKeep{moved: m.sessionMoved(s), stop: s.stall.stop, stops: append([]string(nil), s.stall.stops...),
		unreported: s.stall.unreported}
	if s.Agent != nil {
		k.pid, k.started = s.Agent.PID, s.Agent.StartedAt
	}
	if b, ok := m.blocked(s, now); ok {
		k.askAt = b.Since
	}
	return k
}

// keepStop keeps the session's own Stop's allowlisted fields until its next Stop, laid
// over the kept one (overlay: a field the Stop does not carry keeps what was kept, the
// owner's rule (a)). A listed subagent whose SubagentStop came first is marked ended now.
func (m *Model) keepStop(s *session, ev signals.HookEvent, now time.Time) {
	k := &keptStop{at: now, last: signals.Clip(ev.LastAssistantMessage, stallClip),
		hasTasks: ev.BackgroundTasks != nil, hasCrons: ev.SessionCrons != nil}
	for _, t := range ev.BackgroundTasks {
		k.tasks = append(k.tasks, keptTask{id: signals.Clip(t.ID, stallClip), typ: signals.Clip(t.Type, 40),
			status: signals.Clip(t.Status, 40), desc: signals.Clip(t.Description, stallClip)})
	}
	for _, c := range ev.SessionCrons {
		k.crons = append(k.crons, keptCron{schedule: signals.Clip(c.Schedule, 60), recurring: c.Recurring,
			fireAt: fireTime(c.Schedule, c.Recurring, now, m.stall.loc)})
	}
	s.stall.stop = overlay(s.stall.stop, k)
	s.stall.unreported = false
	pairStops(s.stall.stop, s.stall.stops)
}

// pairStops marks each kept subagent task of stop whose id is among the session's last
// SubagentStops ended. Ids are unique, so the order the two arrived in does not matter.
func pairStops(stop *keptStop, ids []string) {
	if stop == nil || len(ids) == 0 {
		return
	}
	stopped := make(map[string]bool, len(ids))
	for _, id := range ids {
		stopped[id] = true
	}
	for i := range stop.tasks {
		if t := &stop.tasks[i]; t.typ == "subagent" && t.id != "" && stopped[t.id] {
			t.ended = true
		}
	}
}

// subagentStopped keeps a SubagentStop's agent id, and nothing else from it, and
// marks the kept subagent task it pairs with ended. A stop that pairs with none (an
// internal agent's, with no type; a workflow's agents') ends nothing.
func (st *stallSession) subagentStopped(id string) {
	if id == "" {
		return
	}
	id = signals.Clip(id, stallClip) // bounded as keepStop bounds a task's id, so the two pair
	st.stops = append(st.stops, id)
	if n := len(st.stops); n > stallStopRing {
		st.stops = append([]string(nil), st.stops[n-stallStopRing:]...)
	}
	if st.stop == nil {
		return
	}
	for i := range st.stop.tasks {
		if t := &st.stop.tasks[i]; t.typ == "subagent" && t.id == id {
			t.ended = true
		}
	}
}

// fireTime resolves a one-shot cron's fire time from the time its Stop arrived, in
// the panel's local time (D4): `M H * * *` (as ScheduleWakeup arms it) fires at the
// first H:M at or after the minute the Stop arrived in, that day or the next; a
// one-shot whose minute, hour, day and month are numbers, at that date in the year
// nearest the Stop. Zero for a recurring cron or any other shape: held off while the
// Stop's list stands.
func fireTime(schedule string, recurring bool, at time.Time, loc *time.Location) time.Time {
	f := strings.Fields(schedule)
	if recurring || len(f) != 5 {
		return time.Time{}
	}
	minute, okM := cronNum(f[0], 0, 59)
	hour, okH := cronNum(f[1], 0, 23)
	if !okM || !okH {
		return time.Time{}
	}
	l := at.In(loc)
	if f[2] == "*" && f[3] == "*" && f[4] == "*" {
		from := time.Date(l.Year(), l.Month(), l.Day(), l.Hour(), l.Minute(), 0, 0, loc)
		fire := time.Date(l.Year(), l.Month(), l.Day(), hour, minute, 0, 0, loc)
		if fire.Before(from) {
			fire = time.Date(l.Year(), l.Month(), l.Day()+1, hour, minute, 0, 0, loc)
		}
		return fire
	}
	day, okD := cronNum(f[2], 1, 31)
	month, okMo := cronNum(f[3], 1, 12)
	if !okD || !okMo {
		return time.Time{}
	}
	var best time.Time
	for _, y := range []int{l.Year() - 1, l.Year(), l.Year() + 1} {
		c := time.Date(y, time.Month(month), day, hour, minute, 0, 0, loc)
		if c.Day() != day || c.Month() != time.Month(month) {
			continue // no such date that year
		}
		if best.IsZero() || absDur(c.Sub(at)) < absDur(best.Sub(at)) {
			best = c
		}
	}
	return best
}

func cronNum(s string, lo, hi int) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < lo || n > hi || strings.TrimSpace(s) != s || strings.HasPrefix(s, "+") {
		return 0, false
	}
	return n, true
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// ---- polls the rule rests on ----

// stallQueues applies a successful queue read: a lane taking or giving up a lease is
// movement, and a holder first seen dead or stale is the lease given up. The first
// read after the start sets the baseline and is not movement.
func (m *Model) stallQueues(now time.Time) {
	held, dead := m.leases(now)
	if m.stall.queuesRead {
		for p := range held {
			if !m.stall.leased[p] {
				m.stall.laneMove[p] = now
			}
		}
		for p := range m.stall.leased {
			if !held[p] {
				m.stall.laneMove[p] = now
			}
		}
		for k, p := range dead {
			if !m.stall.deadSeen[k] {
				m.stall.laneMove[p] = now
			}
		}
	}
	m.stall.leased = held
	m.stall.deadSeen = map[string]bool{}
	for k := range dead {
		m.stall.deadSeen[k] = true
	}
	m.stall.queuesRead = true
}

// leases reads the queues' leases bound to a lane: held is the lanes that hold (a
// live holder, not stale) or wait for one; dead is each dead or stale holder bound to
// a lane, by an identity of its lease. A project that declares no queue has none.
func (m *Model) leases(now time.Time) (held map[string]bool, dead map[string]string) {
	held, dead = map[string]bool{}, map[string]string{}
	if len(m.cfg.Queues) == 0 {
		return held, dead
	}
	bind := m.leaseBinder(now)
	for _, q := range m.v2.queues {
		if h := q.Holder; h != nil {
			if p := bind(h.Lane); p != "" {
				if h.Alive && !h.Stale {
					held[p] = true
				} else {
					dead[q.ID+"\x00"+h.Nonce+"\x00"+strconv.Itoa(h.PID)+"\x00"+strconv.FormatInt(h.Started, 10)] = p
				}
			}
		}
		for _, w := range q.Waiters {
			if p := bind(w.Lane); p != "" {
				held[p] = true
			}
		}
	}
	return held, dead
}

// leaseBinder returns the lane a lease's free-text lane names: a lane's tmux id, or
// the basename of a worktree no other worktree of the project shares. Any other is no
// lane's.
func (m *Model) leaseBinder(now time.Time) func(string) string {
	terms := map[string]string{}
	for _, tv := range m.TerminalViews(now) {
		if tv.Running && tv.Worktree != "" {
			terms[tv.ID] = tv.Worktree
		}
	}
	bases := map[string][]string{}
	for _, w := range m.worktrees {
		if !w.Bare {
			b := filepath.Base(w.Path)
			bases[b] = append(bases[b], w.Path)
		}
	}
	return func(lane string) string {
		if lane == "" {
			return ""
		}
		if p, ok := terms[lane]; ok {
			return p
		}
		if ps := bases[lane]; len(ps) == 1 {
			return ps[0]
		}
		return ""
	}
}

// stallTmux applies a successful tmux poll: a lane's pane dying, its tmux session
// going, or its pane coming back is the lane's movement. The first poll is the
// baseline.
func (m *Model) stallTmux(lanes []types.TmuxLane, recs []types.LaneRecord, now time.Time) {
	panes := map[string]string{}
	tl := map[string]types.TmuxLane{}
	for _, l := range lanes {
		tl[l.ID] = l
	}
	for _, rec := range recs {
		l, ok := tl[rec.ID]
		switch {
		case !ok:
			panes[rec.ID] = "gone"
		case l.Dead:
			panes[rec.ID] = "dead"
		default:
			panes[rec.ID] = "alive " + strconv.FormatInt(l.Created, 10)
		}
		if prev, seen := m.stall.panes[rec.ID]; m.stall.tmuxRead && (!seen || prev != panes[rec.ID]) {
			if i := signals.MatchWorktree(m.worktrees, rec.Path); i >= 0 {
				m.stall.laneMove[m.worktrees[i].Path] = now
			}
		}
	}
	m.stall.panes = panes
	m.stall.tmuxRead = true
}

// laneExited: the panel launched a lane in this worktree, and every one of its lanes
// there has its claude exited or its tmux session gone (D3 (c)). what is the openWork
// entry, text what it waited on.
func (m *Model) laneExited(path string) (what, text string, ok bool) {
	if !m.stall.tmuxRead {
		return "", "", false
	}
	tl := map[string]types.TmuxLane{}
	for _, l := range m.tmuxLanes {
		tl[l.ID] = l
	}
	n := 0
	for _, rec := range m.laneRecords {
		if i := signals.MatchWorktree(m.worktrees, rec.Path); i < 0 || m.worktrees[i].Path != path {
			continue
		}
		n++
		l, running := tl[rec.ID]
		switch {
		case running && !l.Dead:
			return "", "", false
		case running && what == "":
			what, text = "claude exited", "its claude exited"
			if l.DeadStatus != "" {
				text += " (status " + l.DeadStatus + ")"
			}
		case !running && what == "":
			what, text = "tmux session gone", "its tmux session is gone"
		}
	}
	return what, text, n > 0
}

// ---- the rule ----

// taskOpen, taskWorking: the allowlist of task types (D3 (d), amended 2026-10-19). A
// type not listed (dream, auto-mode scan, memory import, an MCP task, a cloud
// session, one never seen) is neither.
func taskOpen(typ string) bool    { return typ == "shell" || typ == "monitor" || typ == "teammate" }
func taskWorking(typ string) bool { return typ == "subagent" || typ == "workflow" }
func inFlight(status string) bool { return status == "running" || status == "pending" }

// unheard: the session has not moved for stallUnheard × the stall time, so a task
// whose end it waits for no longer holds it working.
func (m *Model) unheard(s *session, now time.Time) bool {
	return now.Sub(m.sessionMoved(s)) >= stallUnheard*m.stall.time
}

// sessionWorking: a live session busy on a current reading, or whose last Stop listed
// a subagent not yet marked ended, or a workflow (which a later Stop drops), running
// or pending, while it has moved within the limit (D1, D3 (d)).
func (m *Model) sessionWorking(s *session, now time.Time) bool {
	if !s.live() {
		return false
	}
	if r := m.agentReading(s, now); r != nil && r.Status == "busy" {
		return true
	}
	st := s.stall.stop
	if st == nil || m.unheard(s, now) {
		return false
	}
	for _, t := range st.tasks {
		if taskWorking(t.typ) && inFlight(t.status) && !(t.typ == "subagent" && t.ended) {
			return true
		}
	}
	return false
}

// wakeupDue: the live session's last Stop listed a wakeup still due (D4): a recurring
// cron or a one-shot of neither shape while the list stands, a resolved one-shot until
// wakeupGrace past its fire time.
func (s *session) wakeupDue(now time.Time) bool {
	if !s.live() || s.stall.stop == nil {
		return false
	}
	for _, c := range s.stall.stop.crons {
		if c.fireAt.IsZero() || now.Before(c.fireAt.Add(wakeupGrace)) {
			return true
		}
	}
	return false
}

// taskNote is one listed task as the stall words it.
type taskNote struct{ typ, text string }

// turnWork is the background work the session's last Stop listed that is open work
// now (D3 (d)), in the words of D4's text.
func (m *Model) turnWork(s *session, now time.Time) []taskNote {
	st := s.stall.stop
	if st == nil {
		return nil
	}
	past := m.unheard(s, now)
	var out []taskNote
	for _, t := range st.tasks {
		d := t.desc
		if d == "" {
			d = "(no description)"
		}
		switch {
		case taskOpen(t.typ):
			// Open work whatever its status, as D3 (d) words it: a teammate's says
			// nothing (P3), and Claude Code lists a shell or a monitor only while it is
			// in flight (P2), so a status beyond running or pending is shown, not dropped.
			if t.typ != "teammate" && t.status != "" {
				d += " (" + t.status + ")"
			}
		case !taskWorking(t.typ) || !inFlight(t.status):
			continue
		case t.typ == "subagent" && t.ended:
			d += ", which finished and woke nothing"
		case past:
			d += ", whose end was never heard"
		default:
			continue // still working
		}
		out = append(out, taskNote{t.typ, d})
	}
	return out
}

// turnText words the work a turn ended waiting on: "its turn ended waiting on 1
// shell: Run checks (running)".
func turnText(notes []taskNote) string {
	var order []string
	by := map[string][]string{}
	for _, n := range notes {
		if _, ok := by[n.typ]; !ok {
			order = append(order, n.typ)
		}
		by[n.typ] = append(by[n.typ], n.text)
	}
	parts := make([]string, 0, len(order))
	for _, typ := range order {
		parts = append(parts, countOf(len(by[typ]), typ)+": "+strings.Join(by[typ], ", "))
	}
	return "its turn ended waiting on " + strings.Join(parts, "; ")
}

func countOf(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// workSummary is the openWork entry of a session's background work: "1 shell".
func workSummary(notes []taskNote) string {
	n := map[string]int{}
	var order []string
	for _, t := range notes {
		if n[t.typ] == 0 {
			order = append(order, t.typ)
		}
		n[t.typ]++
	}
	parts := make([]string, 0, len(order))
	for _, typ := range order {
		parts = append(parts, countOf(n[typ], typ))
	}
	return "turn ended on " + strings.Join(parts, ", ")
}

// lastWaitedOn is what the session's last turn ended waiting on (D4's text).
func (m *Model) lastWaitedOn(s *session, now time.Time) string {
	st := s.stall.stop
	switch {
	case st == nil:
		return "what it waited on is not known: no turn of it has ended since the panel started"
	case s.stall.unreported:
		return "what it waited on is not known: its last turn's Stop was not heard (interrupted, failed, or sent while the panel was down)"
	case !st.reported():
		return "what it waited on is not reported by this Claude Code version"
	}
	if notes := m.turnWork(s, now); len(notes) > 0 {
		return turnText(notes)
	}
	t := "its turn ended with nothing in flight to wake it"
	if st.last != "" {
		t += "; last said: " + st.last
	}
	return t
}

// questionText words a question that stalled: as Needs you words the ask.
func questionText(b blockInfo) string {
	t := b.Text
	if t == "" {
		t = b.Label
	}
	return "waiting on you: " + t
}

// ownBranch: the lane at path has a branch of its own, so a pull request or a receipt
// can be its open work (D3), and merge readiness has a box for it. The main checkout
// has none.
func (m *Model) ownBranch(path, branch string) bool {
	return branch != "" && path != m.root && branch != m.cfg.BaseRef() && "origin/"+branch != m.cfg.BaseRef()
}

// openPR is the open pull request whose head is branch, from the last `gh pr list`.
func (m *Model) openPR(branch string) *signals.PR {
	var pr *signals.PR
	for i := range m.prs {
		if m.prs[i].HeadRef == branch {
			pr = &m.prs[i]
		}
	}
	return pr
}

// receiptClean: the lane's gate receipt is a clean run of its HEAD (HEAD from the
// worktree list, D10).
func (m *Model) receiptClean(wt signals.Worktree) bool { return receiptCleanIn(wt, m.laneExtras) }

func receiptCleanIn(wt signals.Worktree, extras map[string]LaneExtra) bool {
	x, ok := extras[wt.Path]
	return ok && wt.Head != "" && x.HasReceipt && x.Receipt.Clean && x.Receipt.SHA == wt.Head
}

// needsMerged: the lane's open work hangs on the merged read (D10): a clean receipt
// for its HEAD and no open pull request, the pull requests read (D3 (b) rests on them
// too: "no pull request after it, open or merged").
func (m *Model) needsMerged(wt signals.Worktree) bool {
	return m.stall.receiptRead && m.mergedDue(wt, m.laneExtras)
}

// A branch gh is never asked about (outside config.BranchRe: the runtime's mergedPRRead
// refuses it before gh runs) is due no merged read, so no result could ever come: its
// receipt stays unknown, never open work (D10), and no source waits on it (D1).
func (m *Model) mergedDue(wt signals.Worktree, extras map[string]LaneExtra) bool {
	return !wt.Bare && m.ownBranch(wt.Path, wt.Branch) && config.BranchRe.MatchString(wt.Branch) && m.stall.prsRead &&
		receiptCleanIn(wt, extras) && m.openPR(wt.Branch) == nil
}

// MergedReadsDue lists the worktrees whose open work hangs on the merged read (D10),
// judged by needsMerged's rule with the receipts the runtime has just read (extras),
// so a receipt read and the merged read it calls for land in one update.
func (m *Model) MergedReadsDue(extras map[string]LaneExtra) []signals.Worktree {
	if m.stall.time <= 0 {
		// The merged read serves the stall rule alone: with stalls off no lane is due one.
		return nil
	}
	var out []signals.Worktree
	for _, wt := range m.worktrees {
		if m.mergedDue(wt, extras) {
			out = append(out, wt)
		}
	}
	return out
}

// mergedKnown: the merged read has succeeded for the lane's branch and HEAD, and is a
// merge, or a "none" made after the branch last left the open-PR list (D10). The
// runtime hands the model its cached "none" until its next read, so a "none" from
// before the pull request was opened would otherwise stand once it merged: a stall
// pushed as "gate passed, no PR" for a merged pull request.
func (m *Model) mergedKnown(wt signals.Worktree) (MergedRead, bool) {
	r, ok := m.stall.merged[wt.Path]
	if !ok || !r.Read || r.Branch != wt.Branch || r.Head != wt.Head {
		return r, false
	}
	if left, was := m.stall.leftOpen[r.Branch]; was && r.PR == 0 && !r.At.After(left) {
		return r, false
	}
	return r, true
}

// notePRsLeft records when each branch leaves the open-PR list, between the last
// successful `gh pr list` and this one (stallModel.leftOpen). A branch listed again,
// or no longer a worktree's, is dropped, so the map holds only what can still matter.
func (m *Model) notePRsLeft(was, now []signals.PR, at time.Time) {
	open := map[string]bool{}
	for _, pr := range now {
		open[pr.HeadRef] = true
		delete(m.stall.leftOpen, pr.HeadRef)
	}
	for _, pr := range was {
		if !open[pr.HeadRef] {
			m.stall.leftOpen[pr.HeadRef] = at
		}
	}
	branches := map[string]bool{}
	for _, wt := range m.worktrees {
		branches[wt.Branch] = true
	}
	for b := range m.stall.leftOpen {
		if !branches[b] {
			delete(m.stall.leftOpen, b)
		}
	}
}

// workJudged: the sources working rests on, and with it a session's background-work way
// (D3 (d)), have answered since the start: `claude agents`, and the queues where the
// project declares any (D1).
func (m *Model) workJudged() bool {
	return !m.agentsOKAt.IsZero() && (len(m.cfg.Queues) == 0 || m.stall.queuesRead)
}

// laneWayJudged: every source the lane's own way (D3 (a) to (c)) rests on has answered
// since the start, so judgeStalls's verdict on it is a verdict: working's sources, the
// pull requests, the receipts, the lane's merged read when it needs one, and tmux where
// the project has lanes the panel launched (D1).
func (m *Model) laneWayJudged(wt signals.Worktree) bool {
	if !m.workJudged() || !m.stall.prsRead || !m.stall.receiptRead || (m.stall.launched && !m.stall.tmuxRead) {
		return false
	}
	_, known := m.mergedKnown(wt)
	return known || !m.needsMerged(wt)
}

// laneStall is one stalled lane as judged.
type laneStall struct {
	since    time.Time
	waitedOn string
	openWork []string
	approx   bool
	// question is the session whose question stalled the lane, and its blocked kind,
	// so Needs you labels that row rather than adding a second.
	question     string
	questionKind string
	// Which way held, by the clock it rests on, so a restart keeps only that clock's
	// time (D1): asks the sessions whose question did, turns those whose own background
	// work did (D3 (d)), laneWay the lane's own open work (D3 (a) to (c)). exact is
	// each of those ways (by wayKey) that held exact, neither on a reading that is not
	// current nor inside the restart window: only those are carried exact (option B).
	asks, turns []string
	laneWay     bool
	exact       map[string]bool
	// why: the first way held on a reading that is not exact says how (a reading not
	// current, or a question only a hook reports), so the row names that as why it is
	// approximate; empty, only the restart window made it so.
	why string
}

// judgeStalls applies D1 to every lane at now. It reads the model and changes nothing.
func (m *Model) judgeStalls(now time.Time) map[string]*laneStall {
	out := map[string]*laneStall{}
	T := m.stall.time
	if T <= 0 {
		return out
	}
	fresh := m.agentsFresh(now)
	settling := now.Sub(m.startedAt) < T // the restart window (D1, option B; hold below)
	agentsOK := !m.agentsOKAt.IsZero()
	workJudged := m.workJudged()
	leases, _ := m.leases(now)
	byLane := map[string][]*session{}
	for _, s := range m.sessions {
		byLane[s.Lane] = append(byLane[s.Lane], s)
	}
	for _, wt := range m.worktrees {
		if wt.Bare {
			continue
		}
		ss := byLane[wt.Path]
		sort.Slice(ss, func(i, j int) bool { return ss[i].ID < ss[j].ID })
		var live []*session
		working := false
		for _, s := range ss {
			if s.live() {
				live = append(live, s)
				working = working || m.sessionWorking(s, now)
			}
		}
		leased := leases[wt.Path]

		// The lane's open work: (a), (b), and (c) with either.
		var open []string
		ab := false
		if m.ownBranch(wt.Path, wt.Branch) {
			if pr := m.openPR(wt.Branch); pr != nil && m.stall.prsRead {
				open, ab = append(open, fmt.Sprintf("#%d open", pr.Number)), true
			} else if m.needsMerged(wt) {
				if r, ok := m.mergedKnown(wt); ok && r.PR == 0 {
					open, ab = append(open, "gate passed for "+short(wt.Head)+", no PR"), true
				}
			}
		}
		exitWhat, exitText, exited := "", "", false
		if ab {
			if exitWhat, exitText, exited = m.laneExited(wt.Path); exited {
				open = append(open, exitWhat)
			}
		}

		// Each way that holds moves since to the earliest origin; the stall is
		// approximate only when every way that holds is. Within a stall time of the
		// start, a way is exact only if stall.json carried it stalled (D1 as amended
		// 2026-10-20, option B): the first reading of each source after a start is a
		// baseline that can miss a change made while the panel was down, or invent one,
		// so a stall that begins across a restart is shown, marked ≈, and notified only
		// once the panel has watched for one stall time. hold records whether the way is
		// exact, the ways stall.json may carry exact into the next start's window.
		ls := &laneStall{approx: true, exact: map[string]bool{}}
		stalled := false
		stale := "stale: " + m.staleWhy(now)
		hold := func(key string, since time.Time, approx bool, why string) {
			if !stalled || since.Before(ls.since) {
				ls.since = since
			}
			begun := settling && !m.stall.carried[key]
			ls.approx = ls.approx && (approx || begun)
			if approx && ls.why == "" {
				ls.why = why
			}
			ls.exact[key] = !approx && !begun
			stalled = true
		}
		var qText, dText string
		var qSince time.Time
		// A session waits on the owner, its ask the stall time old: nothing holds it off.
		if agentsOK {
			for _, s := range ss {
				if !m.sessionShown(s, now) {
					continue
				}
				b, ok := m.blocked(s, now)
				if !ok || b.Since.IsZero() || now.Sub(b.Since) < T {
					continue
				}
				why := stale
				if b.Approx && b.Why != "" {
					why = b.Why
				}
				hold(wayKey("ask", s.ID), b.Since, !fresh || b.Approx, why)
				ls.asks = append(ls.asks, s.ID)
				if ls.question == "" || b.Since.Before(qSince) {
					ls.question, ls.questionKind, qText, qSince = s.ID, b.Kind, questionText(b), b.Since
				}
			}
		}
		// A session's turn ended on background work, on its own clock (D3 (d)); unknown
		// while a turn after its kept Stop has gone unreported (#186).
		if workJudged && !leased {
			for _, s := range live {
				if s.stall.stop == nil || !s.stall.stop.reported() || s.stall.unreported || m.sessionWorking(s, now) || s.wakeupDue(now) {
					continue
				}
				notes := m.turnWork(s, now)
				since := m.sessionMoved(s)
				if len(notes) == 0 || now.Sub(since) < T {
					continue
				}
				hold(wayKey("turn", s.ID), since, !fresh, stale)
				ls.turns = append(ls.turns, s.ID)
				open = append(open, workSummary(notes))
				if dText == "" {
					dText = turnText(notes)
				}
			}
		}
		// The lane: open work and quiet, nothing working (D3 (a) to (c)).
		var lText string
		if workJudged && !leased && !working && ab && now.Sub(m.laneMoved(wt.Path)) >= T {
			wake := false
			for _, s := range live {
				wake = wake || s.wakeupDue(now)
			}
			switch {
			case len(live) > 0 && !wake:
				hold(wayKey("lane", wt.Path), m.laneMoved(wt.Path), !fresh, stale)
				ls.laneWay = true
				lText = m.lastWaitedOn(m.latestStop(live), now)
				if exited {
					lText = exitText
				}
			case len(live) == 0 && exited:
				hold(wayKey("lane", wt.Path), m.laneMoved(wt.Path), !fresh, stale)
				ls.laneWay = true
				lText = exitText
			}
		}
		if !stalled {
			continue
		}
		switch {
		case qText != "":
			ls.waitedOn = qText
		case dText != "":
			ls.waitedOn = dText
		default:
			ls.waitedOn = lText
		}
		if open == nil {
			open = []string{}
		}
		ls.openWork = open
		out[wt.Path] = ls
	}
	return out
}

// latestStop is the live session whose last Stop came last, else the first.
func (m *Model) latestStop(live []*session) *session {
	best := live[0]
	for _, s := range live[1:] {
		if s.stall.stop != nil && (best.stall.stop == nil || s.stall.stop.at.After(best.stall.stop.at)) {
			best = s
		}
	}
	return best
}

// sessionShown: the page shows the session (Snapshot's own test), so Needs you and
// the stall read the same sessions.
func (m *Model) sessionShown(s *session, now time.Time) bool {
	if s.Agent != nil || s.Note != nil {
		return true
	}
	heard := s.LastHookAt
	if s.StatusAt.After(heard) {
		heard = s.StatusAt
	}
	return !heard.IsZero() && now.Sub(heard) < activeWindow && s.HookStatus != "ended"
}

// ---- the view ----

// StallView is a stalled lane's stall (D11): since its last movement (the ask, for a
// question), what it last waited on, its open work, and whether it rests on a
// `claude agents` reading that is not current.
type StallView struct {
	Since    int64    `json:"since"`
	WaitedOn string   `json:"waitedOn"`
	OpenWork []string `json:"openWork"`
	Approx   bool     `json:"approx"`
}

// stallSource is one source a way of stalling rests on that has not answered.
type stallSource struct{ key, text string }

// stallSourcesMissing lists the sources not yet answered since the start, and what
// each leaves unjudged (D1). unanswered is mergedUnanswered, computed once by
// applyStalls so this warning and StallsUnjudged read the same lanes.
func (m *Model) stallSourcesMissing(unanswered []signals.Worktree) []stallSource {
	var out []stallSource
	if m.agentsOKAt.IsZero() {
		out = append(out, stallSource{"agents", "`claude agents` has not answered since the panel started: no lane can be judged stalled until it does."})
	}
	if !m.stall.prsRead {
		out = append(out, stallSource{"prs", "the pull requests (`gh pr list`) are not read: a pull request cannot make a lane stalled until they are."})
	}
	if !m.stall.receiptRead {
		out = append(out, stallSource{"receipts", "the gate receipts are not read: a gate that passed cannot make a lane stalled until they are."})
	}
	var unread []string
	for _, wt := range unanswered {
		_, name := m.cfg.LaneFor(wt.Branch)
		unread = append(unread, name)
	}
	if len(unread) > 0 && !m.stall.mergedOnce {
		out = append(out, stallSource{"merged", "the merged read has not answered for " + strings.Join(unread, ", ") +
			": a gate that passed cannot make a lane stalled until it does."})
	}
	if m.stall.launched && !m.stall.tmuxRead {
		out = append(out, stallSource{"tmux", "tmux has not answered: a lane whose claude exited cannot be judged stalled until it does."})
	}
	if len(m.cfg.Queues) > 0 && !m.stall.queuesRead {
		out = append(out, stallSource{"queues", "the queues' leases are not read: only a question can make a lane stalled until they are."})
	}
	return out
}

// mergedUnanswered lists the lanes whose open work hangs on a merged read that has not
// answered for their branch and HEAD (D10), so their lane's way is not yet judged (D1).
func (m *Model) mergedUnanswered() []signals.Worktree {
	var out []signals.Worktree
	for _, wt := range m.worktrees {
		if !wt.Bare && m.needsMerged(wt) {
			if _, ok := m.mergedKnown(wt); !ok {
				out = append(out, wt)
			}
		}
	}
	return out
}

// stallLabel is a stalled row's label: "Stalled", and when the stall is approximate,
// why (judgeStalls words it as it judges).
func stallLabel(ls *laneStall) string {
	switch {
	case ls.approx && ls.why != "":
		return "Stalled (≈ " + ls.why + ")"
	case ls.approx:
		return "Stalled (≈ since the panel's restart: not notified until it has watched for the stall time)"
	}
	return "Stalled"
}

// applyStalls puts the stalls into the view: each lane's Stall, the Needs-you rows (a
// waiting row relabelled when its question stalled the lane, else one stalled row per
// lane, built once per lane as mergeNeeds builds the merged rows), the pending flag
// and the source warnings. It returns the stalled alerts.
func (m *Model) applyStalls(v *View, now time.Time) []AlertView {
	if m.stall.time <= 0 {
		return nil
	}
	unanswered := m.mergedUnanswered()
	missing := m.stallSourcesMissing(unanswered)
	waited := now.Sub(m.startedAt) >= stallSourceWait
	v.StallsPending = len(missing) > 0 && !waited
	// A lane whose merged read is due and unanswered after the start (its pull request
	// closed or merged since the last read) is not yet judged either: the notifier
	// keeps its mark until the read answers, so a "none" bringing back the stall it had
	// is not a second notice for one stretch (D1, D6; group 3's review round 1, M1).
	v.StallsUnjudged = nil
	for _, wt := range unanswered {
		v.StallsUnjudged = append(v.StallsUnjudged, wt.Path)
	}
	if waited {
		for _, s := range missing {
			v.warn("stall-source:"+s.key, "Stalled lanes: "+s.text)
		}
	}
	stalls := m.judgeStalls(now)
	var alerts []AlertView
	mark := func(lv *LaneView) {
		ls := stalls[lv.Path]
		if ls == nil {
			return
		}
		lv.Stall = &StallView{Since: ms(ls.since), WaitedOn: ls.waitedOn, OpenWork: ls.openWork, Approx: ls.approx}
		text := ls.waitedOn
		if len(ls.openWork) > 0 {
			text += "; open work: " + strings.Join(ls.openWork, ", ")
		}
		alerts = append(alerts, AlertView{Key: AlertStalled + ":" + lv.Path, Kind: AlertStalled, Severity: signals.SevBlock,
			Lane: lv.Path, Name: lv.Name, Terminal: lv.Terminal, Text: "stalled: " + text, Since: ms(ls.since), Approx: ls.approx})
		label := stallLabel(ls)
		if ls.question != "" {
			for i := range v.NeedsYou {
				n := &v.NeedsYou[i]
				if n.Lane == lv.Path && n.Session == ls.question && n.Kind == ls.questionKind {
					if n.Approx {
						label = "Stalled" // the waiting row already says why it is approximate
					}
					n.Label = label + ": " + n.Label
					if len(ls.openWork) > 0 {
						n.Text += "; open work: " + strings.Join(ls.openWork, ", ")
					}
					return
				}
			}
		}
		v.NeedsYou = append(v.NeedsYou, NeedView{Lane: lv.Path, Name: lv.Name, Kind: AlertStalled, Label: label,
			Severity: signals.SevBlock, Text: text, At: ms(ls.since), Terminal: lv.Terminal, Approx: ls.approx})
	}
	for i := range v.Lanes {
		mark(&v.Lanes[i])
	}
	for i := range v.QuietWorktrees {
		mark(&v.QuietWorktrees[i])
	}
	return alerts
}

// ---- across a restart (stall.json) ----

// StallState is what stall.json keeps (D1): each lane's and each live session's last
// movement and whether it was stalled, each waiting session's ask time, each live
// session's kept Stop and the agent ids of its last SubagentStops, and the same for
// the sessions kept aside while the polls failed.
type StallState struct {
	SavedAt  int64                       `json:"savedAt"`
	Lanes    map[string]StallLaneMark    `json:"lanes"`
	Sessions map[string]StallSessionMark `json:"sessions"`
	Aside    map[string]StallSessionMark `json:"aside,omitempty"`
}

// StallLaneMark is one lane's saved clock. Stalled: the lane's own way held at the save,
// so Moved keeps its time; Approx: it held only approximately (on a reading that was
// not current, or inside a restart's window), so the next start does not carry it
// exact (D1, option B). Neither is a reading: the restart compares nothing against it.
type StallLaneMark struct {
	Moved   int64 `json:"moved"`
	Stalled bool  `json:"stalled,omitempty"`
	Approx  bool  `json:"approx,omitempty"`
}

// StallSessionMark is one session's saved clock and kept Stop, and the process they
// were kept from (`claude agents`' pid and startedAt), so a session resumed while the
// panel was down is not given the exited process's (L3).
type StallSessionMark struct {
	PID       int   `json:"pid,omitempty"`
	StartedAt int64 `json:"startedAt,omitempty"`
	Moved     int64 `json:"moved"`
	// Stalled: the session's own background-work way (D3 (d)) held at the save, so
	// Moved keeps its time; AskStalled: its question did, so AskAt does. Each clock is
	// marked by the way that rests on it alone, or a restart would count the outage.
	// Approx and AskApprox: that way held only approximately, as StallLaneMark's.
	Stalled    bool       `json:"stalled,omitempty"`
	Approx     bool       `json:"approx,omitempty"`
	AskAt      int64      `json:"askAt,omitempty"`
	AskStalled bool       `json:"askStalled,omitempty"`
	AskApprox  bool       `json:"askApprox,omitempty"`
	Stop       *StallStop `json:"stop,omitempty"`
	// Unreported: a UserPromptSubmit came after Stop and no Stop since (#186).
	Unreported    bool     `json:"unreported,omitempty"`
	SubagentStops []string `json:"subagentStops,omitempty"`
}

// StallStop is a kept Stop: D4's allowlist, clipped, and what the panel resolved.
type StallStop struct {
	At int64 `json:"at"`
	// HasTasks and HasCrons: a Stop of the session carried background_tasks, and
	// session_crons, so an empty list means "nothing", not "not reported" (rule (a)).
	HasTasks bool        `json:"hasTasks,omitempty"`
	HasCrons bool        `json:"hasCrons,omitempty"`
	Tasks    []StallTask `json:"tasks,omitempty"`
	Crons    []StallCron `json:"crons,omitempty"`
	Last     string      `json:"last,omitempty"`
}

// StallTask is a kept task, with its ended mark.
type StallTask struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
	Ended       bool   `json:"ended,omitempty"`
}

// StallCron is a kept cron, with a one-shot's resolved fire time (0: none).
type StallCron struct {
	Schedule  string `json:"schedule"`
	Recurring bool   `json:"recurring"`
	FireAt    int64  `json:"fireAt,omitempty"`
}

func (k stallKeep) mark() StallSessionMark {
	sm := StallSessionMark{PID: k.pid, StartedAt: k.started, Moved: ms(k.moved), Stalled: k.stalled, Approx: k.approx,
		AskAt: ms(k.askAt), AskStalled: k.askStalled, AskApprox: k.askApprox, Unreported: k.unreported, SubagentStops: k.stops}
	if st := k.stop; st != nil {
		ss := &StallStop{At: ms(st.at), HasTasks: st.hasTasks, HasCrons: st.hasCrons, Last: st.last}
		for _, t := range st.tasks {
			ss.Tasks = append(ss.Tasks, StallTask{ID: t.id, Type: t.typ, Status: t.status, Description: t.desc, Ended: t.ended})
		}
		for _, c := range st.crons {
			ss.Crons = append(ss.Crons, StallCron{Schedule: c.schedule, Recurring: c.recurring, FireAt: ms(c.fireAt)})
		}
		sm.Stop = ss
	}
	return sm
}

func unixMs(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v)
}

func (sm StallSessionMark) keep(carry func(int64, bool) time.Time) stallKeep {
	k := stallKeep{moved: carry(sm.Moved, sm.Stalled), askAt: carry(sm.AskAt, sm.AskStalled), stalled: sm.Stalled,
		approx: sm.Approx, askStalled: sm.AskStalled, askApprox: sm.AskApprox, unreported: sm.Unreported, pid: sm.PID, started: sm.StartedAt}
	if n := len(sm.SubagentStops); n > 0 {
		if n > stallStopRing {
			sm.SubagentStops = sm.SubagentStops[n-stallStopRing:]
		}
		for _, id := range sm.SubagentStops {
			k.stops = append(k.stops, signals.Clip(id, stallClip))
		}
	}
	if ss := sm.Stop; ss != nil {
		st := &keptStop{at: unixMs(ss.At), hasTasks: ss.HasTasks, hasCrons: ss.HasCrons, last: signals.Clip(ss.Last, stallClip)}
		for _, t := range ss.Tasks {
			st.tasks = append(st.tasks, keptTask{id: signals.Clip(t.ID, stallClip), typ: signals.Clip(t.Type, 40),
				status: signals.Clip(t.Status, 40), desc: signals.Clip(t.Description, stallClip), ended: t.Ended})
		}
		for _, c := range ss.Crons {
			st.crons = append(st.crons, keptCron{schedule: signals.Clip(c.Schedule, 60), recurring: c.Recurring, fireAt: unixMs(c.FireAt)})
		}
		k.stop = st
	}
	return k
}

// StallState is the state to save at now.
func (m *Model) StallState(now time.Time) StallState {
	stalls := m.judgeStalls(now)
	st := StallState{SavedAt: ms(now), Lanes: map[string]StallLaneMark{}, Sessions: map[string]StallSessionMark{}}
	// Each clock is marked stalled only by the way that rests on it (D1), and
	// approximate unless that way held exact (option B).
	turns, asks, exact := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, ls := range stalls {
		for _, id := range ls.turns {
			turns[id] = true
		}
		for _, id := range ls.asks {
			asks[id] = true
		}
		for k, v := range ls.exact {
			exact[k] = v
		}
	}
	// A way not yet judged since the start (its sources unanswered) is not judged ended:
	// it keeps the mark the restart restored while its clock has not moved, or a save
	// in that time would write the stall as over, and the next start would carry its
	// quiet time instead, a new since notified again (review round 2, M1). A question
	// rests on `claude agents` alone, which has answered once a poll lists the session.
	keep := func(key string, moved time.Time, judged bool) (restoredMark, bool) {
		r, ok := m.stall.restored[key]
		return r, ok && !judged && r.moved.Equal(moved)
	}
	for _, wt := range m.worktrees {
		if !wt.Bare {
			ls := stalls[wt.Path]
			moved := m.laneMoved(wt.Path)
			held := ls != nil && ls.laneWay
			approx := held && !exact[wayKey("lane", wt.Path)]
			if r, ok := keep(wayKey("lane", wt.Path), moved, m.laneWayJudged(wt)); ok && !held {
				held, approx = true, r.approx
			}
			st.Lanes[wt.Path] = StallLaneMark{Moved: ms(moved), Stalled: held, Approx: approx}
		}
	}
	for id, s := range m.sessions {
		if s.live() {
			k := m.keepOf(s, now)
			k.stalled, k.askStalled = turns[id], asks[id]
			k.approx = k.stalled && !exact[wayKey("turn", id)]
			k.askApprox = k.askStalled && !exact[wayKey("ask", id)]
			if r, ok := keep(wayKey("turn", id), k.moved, m.workJudged()); ok && !k.stalled {
				k.stalled, k.approx = true, r.approx
			}
			st.Sessions[id] = k.mark()
		}
	}
	// What a restart restored that no poll has listed yet stays saved, and what was set
	// aside, each continued by a record heard since (L12, L13).
	for id, k := range m.stall.saved {
		if _, ok := st.Sessions[id]; !ok {
			st.Sessions[id] = m.continued(id, k).mark()
		}
	}
	if len(m.stall.aside) > 0 {
		st.Aside = map[string]StallSessionMark{}
		for id, k := range m.stall.aside {
			st.Aside[id] = m.continued(id, k).mark()
		}
	}
	return st
}

// ApplyStallState restores a saved state at the start, before the first `claude
// agents` poll. A lane's clock is restored at once; a session's waits for the next
// successful poll, which uses it only for a session it lists (a waiting one's ask
// only while it still waits). A clock stalled at the save keeps its time; any other
// carries the quiet time it had then, so the time the panel was down does not count.
func (m *Model) ApplyStallState(st StallState) {
	start := m.startedAt
	carry := func(v int64, stalled bool) time.Time {
		if v == 0 {
			return time.Time{}
		}
		t := time.UnixMilli(v)
		if stalled || st.SavedAt == 0 {
			return t
		}
		quiet := time.UnixMilli(st.SavedAt).Sub(t)
		if quiet < 0 {
			quiet = 0
		}
		return start.Add(-quiet)
	}
	for p, l := range st.Lanes {
		if _, ok := m.stall.laneMove[p]; !ok {
			if t := carry(l.Moved, l.Stalled); !t.IsZero() {
				m.stall.laneMove[p] = t
				if l.Stalled && !l.Approx {
					m.stall.carried[wayKey("lane", p)] = true
				}
				if l.Stalled {
					m.stall.restored[wayKey("lane", p)] = restoredMark{moved: t, approx: l.Approx}
				}
			}
		}
	}
	m.stall.saved = map[string]stallKeep{}
	for _, set := range []map[string]StallSessionMark{st.Aside, st.Sessions} {
		for id, sm := range set {
			m.stall.saved[id] = sm.keep(carry)
		}
	}
}
