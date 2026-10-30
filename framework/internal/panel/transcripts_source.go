package panel

import (
	"context"
	"time"

	"github.com/rfhayn/stridal/internal/panel/claudecall"
	"github.com/rfhayn/stridal/internal/panel/signals"
	"github.com/rfhayn/stridal/internal/panel/state"
)

// PANEL-49 (D3, D6): an attached project's sessions are measured from their
// transcripts. newRuntime adds this source for such a project alone; signals reads
// the files and keeps no state, so each session's cursor lives here, on the source's
// one goroutine, until the state forgets the session.

// transcriptReading is one session's reading, taken off the hub's lock and applied
// under it.
type transcriptReading struct {
	id  string
	r   signals.TranscriptReading
	err error
}

// transcriptsSource reads every session the state holds, every Ticks.Transcripts,
// from where each one's last read stopped.
func (r *Runtime) transcriptsSource() *source {
	cursors := map[string]*signals.TranscriptCursor{}
	return &source{name: "transcripts", every: r.ticks.Transcripts, fixedRate: true,
		poll: func(ctx context.Context, now time.Time) (update, time.Duration) {
			return r.pollTranscripts(ctx, cursors, now)
		}}
}

// pollTranscripts reads each session the state holds, under Claude Code's config
// directory as the panel's environment names it (the lanes inherit the same), and
// drops the cursor of every session the state no longer holds. Every reading is
// applied at now, the time the poll started, before any of its reads: a Stop or a
// first prompt that arrives during the reads is judged by the next poll, never this
// one (ApplyTranscript's D7 conditions).
func (r *Runtime) pollTranscripts(ctx context.Context, cursors map[string]*signals.TranscriptCursor, now time.Time) (update, time.Duration) {
	var ids []string
	r.hub.Read(func(m *state.Model, _ time.Time) { ids = m.TranscriptSessions() })
	held := make(map[string]bool, len(ids))
	for _, id := range ids {
		held[id] = true
	}
	for id := range cursors {
		if !held[id] {
			delete(cursors, id)
		}
	}
	dir := claudecall.ConfigDir(r.o.LookupEnv, r.o.Home)
	readings := make([]transcriptReading, 0, len(ids))
	for _, id := range ids {
		if ctx.Err() != nil {
			return nil, 0
		}
		cur := cursors[id]
		if cur == nil {
			cur = &signals.TranscriptCursor{}
			cursors[id] = cur
		}
		rd, err := signals.ReadTranscript(dir, id, cur)
		readings = append(readings, transcriptReading{id: id, r: rd, err: err})
	}
	return func(m *state.Model, _ time.Time) {
		for _, x := range readings {
			m.ApplyTranscript(x.id, x.r, x.err, now)
		}
	}, 0
}
