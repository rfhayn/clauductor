package state

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rfhayn/stridal/internal/panel/signals"
)

// PANEL-49 (D3, D4, D5, D7): a project whose config is outside its repository is
// measured from its sessions' transcripts. There the transcript is the one source of a
// session's cost and model, and of the context of a session with a recorded window; a
// status post still moves the quota and every figure a transcript does not carry
// (ApplyStatus). The runtime reads each session through signals.ReadTranscript and
// hands the reading here.

// transcriptGrace is how long after a session's first prompt a reading may find no
// transcript before the source fails (D7): Claude Code writes the file with the
// first answer, and the reader polls every 10 s.
const transcriptGrace = time.Minute

// transcriptState is what a session's transcript readings found.
type transcriptState struct {
	// read: a reading that reached the end of the session's files has set its figures.
	read bool
	// failed is a line the reader could not read. The session stays unknown for the
	// rest of the run (D7): a later good line would leave the bad line's call out.
	failed *signals.TranscriptError
	// problem is a condition with no line (no usage record after a Stop, no transcript
	// a minute after the first prompt) or a read error. It clears on the first reading
	// that finds what it lacked.
	problem string
	// unpriced counts the calls left out of the session's cost, by model (D5).
	unpriced map[string]int
}

// MeasureFromTranscripts marks the project as one whose sessions are measured from
// their transcripts (D3). The runtime calls it for an attached project only, before
// any post arrives; a project with its config in its repository never calls it, and
// is measured exactly as before.
func (m *Model) MeasureFromTranscripts() {
	m.transcripts = true
}

// TranscriptSessions are the ids of every session the state holds, sorted, for the
// runtime's transcripts source to read (D3); none in a project not measured from
// transcripts. The state, not the reader, says which sessions there are, and a
// session it forgets is no longer read.
func (m *Model) TranscriptSessions() []string {
	if !m.transcripts {
		return nil
	}
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// ApplyTranscript folds one reading of session id's transcripts, taken at now; err is
// signals.ReadTranscript's. A session the state does not hold is ignored: the state,
// not the reader, says which sessions there are.
func (m *Model) ApplyTranscript(id string, r signals.TranscriptReading, err error, now time.Time) {
	if !m.transcripts {
		return
	}
	s := m.sessions[id]
	if s == nil {
		return
	}
	m.transcriptsAt = now
	t := &s.tr
	if t.failed != nil {
		return
	}
	switch {
	case errors.Is(err, signals.ErrNotUUID):
		return // never read, so never measured (D1)
	case err != nil:
		t.problem = "session " + id + ": " + err.Error()
		m.unsetTranscriptFigures(s)
		return
	case r.Failed != nil:
		t.failed, t.problem = r.Failed, ""
		m.unsetTranscriptFigures(s)
		return
	}
	t.problem = ""
	if !r.Found {
		if t.read {
			// A transcript removed after it was read: its figures are unknown, never
			// the last value (D7).
			m.unsetTranscriptFigures(s)
		}
		if !s.firstPromptAt.IsZero() && now.Sub(s.firstPromptAt) >= transcriptGrace {
			// The file name is left to the reader: signals/transcript.go is the one
			// source that may name a transcript (D8), so this says only where it looked.
			t.problem = fmt.Sprintf("session %s: no transcript in any folder of %s, %ds after its first prompt",
				id, r.Searched, int(now.Sub(s.firstPromptAt).Seconds()))
		}
		return
	}
	if !r.AtEnd {
		return // a partial total is never shown (D6)
	}
	if !r.Answered && !s.firstStopAt.IsZero() && now.After(s.firstStopAt) {
		// D7: what the transcript sets is unknown, so a subagent's calls alone show no
		// est. $ while the main thread has not answered.
		t.problem = "session " + id + ": no usage record in its transcript after its turn finished"
		m.unsetTranscriptFigures(s)
		return
	}
	t.read, t.unpriced = true, r.Unpriced

	if w := m.sessionWindow(s); w > 0 {
		s.CtxPct, s.Stats.CtxPct, s.ctxAt = nil, nil, time.Time{}
		if r.Context != nil {
			v, x := windowPct(r.Context.Tokens(), w), windowPct(r.Context.Tokens(), w)
			s.CtxPct, s.ctxWindow, s.ctxAt = &v, w, now
			s.Stats.CtxPct, s.Stats.ctxWindow, s.Stats.CtxWindow = &x, w, int64(w)
		}
	}
	// The model before the spend: observeSpend books it under the session's model.
	s.Model = r.Model
	if r.CostUSD == nil {
		delete(m.costByID, id)
		s.Stats.CostUSD = nil
		return
	}
	c, x := *r.CostUSD, *r.CostUSD
	m.costByID[id] = c
	s.Stats.CostUSD = &x
	m.countCost(id, c, now)
	m.observeSpend(id, c, now)
}

// unsetTranscriptFigures makes what the transcript sets unknown, as before any reading
// (D7): neither the last value nor 0. A window-less session's context is its status
// line's (D3), not the transcript's, and stays.
func (m *Model) unsetTranscriptFigures(s *session) {
	delete(m.costByID, s.ID)
	s.Stats.CostUSD = nil
	s.Model = ""
	if m.sessionWindow(s) > 0 {
		s.CtxPct, s.Stats.CtxPct, s.ctxAt = nil, nil, time.Time{}
	}
	s.tr.read, s.tr.unpriced = false, nil
}

// sessionWindow is the window a session's context is measured against, by PANEL-48's
// rule: the window its status line last posted, else its lane record's; 0 for none.
// A status post and a transcript reading ask the same, so they agree on which of the
// two sets the session's context.
func (m *Model) sessionWindow(s *session) int {
	return m.recordWindow(s.ID, s.postedWindow)
}

// ctxSince is the context alert's time: the reading that set the figure, or for a
// figure a status post set, the last post.
func (s *session) ctxSince() time.Time {
	if !s.ctxAt.IsZero() {
		return s.ctxAt
	}
	return s.StatusAt
}

// transcriptSource is the view's `transcripts` source: failed while any session's
// reading fails, naming each, so one session's good read clears no other's error (D7).
func (m *Model) transcriptSource() SourceStatus {
	if m.transcriptsAt.IsZero() {
		return SourceStatus{Pending: true}
	}
	var errs []string
	for _, s := range m.sessions {
		switch {
		case s.tr.failed != nil:
			errs = append(errs, s.tr.failed.Error())
		case s.tr.problem != "":
			errs = append(errs, s.tr.problem)
		}
	}
	if len(errs) == 0 {
		return SourceStatus{OK: true, At: ms(m.transcriptsAt)}
	}
	sort.Strings(errs)
	return SourceStatus{Error: strings.Join(errs, "; "), At: ms(m.transcriptsAt)}
}

// Warning keys of the transcript measures.
const (
	WarnTranscriptWindow   = "transcript_window"   // a session measured from its transcript has no window to show a % against
	WarnTranscriptUnpriced = "transcript_unpriced" // calls left out of a lane's est. $; the key adds the lane and the model
)

// transcriptWarnings are D4's window warning, once for the project, and D5's unpriced
// warning, once per lane and model.
func (m *Model) transcriptWarnings(v *View, now time.Time) {
	if !m.transcripts {
		return
	}
	launched := map[string]string{} // session id → its lane record's id
	for _, rec := range m.laneRecords {
		if rec.SessionID != "" {
			launched[rec.SessionID] = rec.ID
		}
	}
	var lanes []string
	others := 0
	type laneModel struct{ lane, model string }
	unpriced := map[laneModel]int{}
	for _, s := range m.sessions {
		if s.tr.failed != nil || !s.tr.read {
			continue
		}
		for model, n := range s.tr.unpriced {
			unpriced[laneModel{s.Lane, model}] += n
		}
		if s.CtxPct != nil || m.sessionWindow(s) > 0 || !m.sessionActive(s, now) {
			continue
		}
		if id, ok := launched[s.ID]; ok {
			lanes = append(lanes, id)
		} else {
			others++
		}
	}
	var parts []string
	if len(lanes) > 0 {
		sort.Strings(lanes)
		parts = append(parts, fmt.Sprintf("%s launched without a context window, so the panel shows its est. $ but no context %%: "+
			"set launch.context_window in panel.json and relaunch to measure its context.", strings.Join(lanes, ", ")))
	}
	if others > 0 {
		parts = append(parts, fmt.Sprintf("%d session(s) the panel did not launch (one /clear started, or one started in a terminal) "+
			"show est. $ but no context %%: the panel knows no context window for them.", others))
	}
	if len(parts) > 0 {
		v.warn(WarnTranscriptWindow, strings.Join(parts, " "))
	}

	keys := make([]laneModel, 0, len(unpriced))
	for k := range unpriced {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].lane != keys[j].lane {
			return keys[i].lane < keys[j].lane
		}
		return keys[i].model < keys[j].model
	})
	for _, k := range keys {
		v.warn(WarnTranscriptUnpriced+":"+k.lane+":"+k.model, fmt.Sprintf(
			"%s: %d call(s) on %s left out of its est. $: the panel's price table has no rate for that model.",
			laneName(v, k.lane), unpriced[k], k.model))
	}
}

// laneName is a lane's name as the view shows it, by its worktree path.
func laneName(v *View, path string) string {
	for _, list := range [][]LaneView{v.Lanes, v.QuietWorktrees} {
		for _, lv := range list {
			if lv.Path == path && lv.Name != "" {
				return lv.Name
			}
		}
	}
	return filepath.Base(path)
}
