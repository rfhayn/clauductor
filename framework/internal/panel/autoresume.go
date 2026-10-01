package panel

import (
	"context"
	"fmt"
	"time"

	"github.com/clauductor/clauductor/internal/panel/state"
)

// pollAutoResume types quota_resume_line into each lane the usage limit stopped, once,
// after the 5-hour window that stopped it resets (PANEL-20; state/resume.go decides
// who, and re-checks right before the Enter). The line is the config's, so an
// untrusted config types nothing.
func (r *Runtime) pollAutoResume(ctx context.Context, now time.Time) (update, time.Duration) {
	if r.lanes == nil || !r.trusted() {
		return nil, 0
	}
	var cands []state.ResumeCandidate
	r.hub.Update(func(m *state.Model, now time.Time) { cands = m.ResumeCandidates(now) })
	line := r.cfg.ResumeLine()
	for _, c := range cands {
		c := c
		err := r.lanes.TypeLine(ctx, c.Lane, line, func() string {
			why := ""
			r.hub.Read(func(m *state.Model, now time.Time) { why = m.ResumeStillDue(c.Session, now) })
			return why
		})
		msg := ""
		if err != nil {
			msg = err.Error()
			fmt.Fprintf(r.o.Out, "lane %s: auto-resume: %v\n", c.Lane, err)
		}
		r.hub.Update(func(m *state.Model, now time.Time) { m.MarkResumed(c, line, msg, now) })
	}
	return nil, 0
}
