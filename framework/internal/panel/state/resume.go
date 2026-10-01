package state

import (
	"strings"
	"time"
)

// Resume after the 5-hour reset (PANEL-20; quota_auto_resume). A lane the usage limit
// stopped (a StopFailure rate_limit, or Claude Code saying it will not resume by
// itself) is typed quota_resume_line once, after the 5-hour window that stopped it
// resets, and only while claude is idle by a current reading and waits on nothing:
// never into a permission prompt or a dialog.

// resumeGrace is how long after the reset the panel waits, so the new window is live.
const resumeGrace = 30 * time.Second

type limitMark struct {
	At    time.Time // when the limit stopped the session
	Reset time.Time // when the window that stopped it resets (zero: not known yet)
	Done  bool      // resumed once for this stop
}

// ResumeCandidate is a lane to type the resume line into now.
type ResumeCandidate struct {
	Lane     string // tmux lane id
	Worktree string
	Session  string
	Reset    time.Time
}

// limited is when the usage limit stopped the session, or zero.
func limited(s *session) time.Time {
	if s.Failure != nil && s.Failure.Type == "rate_limit" {
		return s.Failure.At
	}
	if s.Note != nil && strings.HasPrefix(s.Note.Type, "quota_auto_resume") {
		return s.Note.At
	}
	return time.Time{}
}

// ResumeCandidates are the lanes due a resume line now. It keeps, per session, when
// the limit stopped it and when that window resets, so a later window's reset time is
// never mistaken for the one that stopped it.
func (m *Model) ResumeCandidates(now time.Time) []ResumeCandidate {
	if m.limits == nil {
		m.limits = map[string]*limitMark{}
	}
	var out []ResumeCandidate
	seen := map[string]bool{}
	for _, tv := range m.TerminalViews(now) {
		if !tv.Registered || !tv.Running || tv.Dead || tv.SessionID == "" {
			continue
		}
		s := m.sessions[tv.SessionID]
		if s == nil {
			continue
		}
		at := limited(s)
		if at.IsZero() {
			delete(m.limits, s.ID)
			continue
		}
		seen[s.ID] = true
		mk := m.limits[s.ID]
		if mk == nil || !mk.At.Equal(at) {
			mk = &limitMark{At: at}
			m.limits[s.ID] = mk
		}
		if mk.Reset.IsZero() {
			if w := m.quotaAt(now).Window("five_hour"); w != nil && w.ResetsAt != nil {
				if r := time.Unix(*w.ResetsAt, 0); r.After(at) {
					mk.Reset = r
				}
			}
		}
		if mk.Done || mk.Reset.IsZero() || now.Before(mk.Reset.Add(resumeGrace)) {
			continue
		}
		if _, blocked := m.blocked(s, now); blocked {
			continue // waiting on a permission, a question or a dialog: never typed into
		}
		if st, _, approx := m.sessionStatus(s, now); st != "idle" || approx {
			continue
		}
		out = append(out, ResumeCandidate{Lane: tv.ID, Worktree: tv.Worktree, Session: s.ID, Reset: mk.Reset})
	}
	for id := range m.limits {
		if !seen[id] {
			delete(m.limits, id)
		}
	}
	return out
}

// ResumeStillDue re-checks one candidate right before its Enter.
func (m *Model) ResumeStillDue(session string, now time.Time) string {
	s := m.sessions[session]
	if s == nil {
		return "its session is gone"
	}
	if _, blocked := m.blocked(s, now); blocked {
		return "it now waits on you"
	}
	if st, _, approx := m.sessionStatus(s, now); st != "idle" || approx {
		return "it is no longer idle"
	}
	return ""
}

// MarkResumed records the one attempt for this stop, and says so in the lane's activity.
func (m *Model) MarkResumed(c ResumeCandidate, line, err string, now time.Time) {
	if mk := m.limits[c.Session]; mk != nil {
		mk.Done = true
	}
	detail := "typed " + `"` + line + `"` + " after the 5-hour window reset at " + c.Reset.Local().Format("15:04")
	if err != "" {
		detail = "not typed: " + err
	}
	m.NoteLane(c.Worktree, c.Lane, "Auto-resume", detail, now)
}
