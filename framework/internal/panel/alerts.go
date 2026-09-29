package panel

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// Alerts are derived, never stored: Snapshot computes them from the state at `now`
// against the configured thresholds. The notifier (notifier.go) decides which of them
// interrupt you with an OS notification.

// Alert kinds.
const (
	AlertWaiting      = "waiting"        // a permission prompt, elicitation or input request older than N s
	AlertRateLimit    = "rate_limit"     // StopFailure error_type rate_limit
	AlertStopFailure  = "stop_failure"   // any other StopFailure
	AlertNoAutoResume = "no_auto_resume" // quota_auto_resume_stale / _disabled: it will not continue by itself
	AlertContext      = "context"        // context_window.used_percentage above the threshold
	AlertIdle         = "idle"           // a live session idle longer than N min
	AlertQuota        = "quota"          // the 5-hour quota above the threshold
)

// AlertView is one active alert.
type AlertView struct {
	Key      string `json:"key"` // stable while the condition holds: kind:session (or kind:global)
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Lane     string `json:"lane,omitempty"` // worktree path
	Name     string `json:"name,omitempty"`
	Terminal string `json:"terminal,omitempty"` // tmux lane id, for focus suppression and the jump
	Session  string `json:"session,omitempty"`
	Text     string `json:"text"`
	Since    int64  `json:"since,omitempty"`
	// Approx: raised from data that is not current (see blocked). It is shown on the
	// page and never interrupts.
	Approx bool `json:"approx,omitempty"`
}

// waitingPrefix starts a waiting alert's text.
const waitingPrefix = "waiting on you: "

// noticeLine is an alert's line in an OS notification: its text, with the age the
// page computes for itself put back in.
func noticeLine(a AlertView, now time.Time) string {
	if a.Since == 0 {
		return a.Text
	}
	age := mins(now.Sub(time.UnixMilli(a.Since)))
	switch a.Kind {
	case AlertWaiting:
		return "waiting on you for " + age + ": " + strings.TrimPrefix(a.Text, waitingPrefix)
	case AlertIdle:
		return "idle for " + age
	}
	return a.Text
}

func mins(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

// computeAlerts evaluates every threshold. A threshold of 0 is off.
func (m *Model) computeAlerts(v *View, th config.Thresholds, now time.Time) []AlertView {
	out := []AlertView{}
	lanes := map[string]LaneView{}
	for _, l := range v.Lanes {
		lanes[l.Path] = l
	}
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		s := m.sessions[id]
		lv, shown := lanes[s.Lane]
		if !shown {
			continue // a session the page no longer shows raises nothing
		}
		base := AlertView{Lane: lv.Path, Name: lv.Name, Terminal: lv.Terminal, Session: s.ID}
		add := func(kind, sev, text string, since time.Time) {
			a := base
			a.Key, a.Kind, a.Severity, a.Text, a.Since = kind+":"+s.ID, kind, sev, text, ms(since)
			out = append(out, a)
		}
		// Waiting on you, for longer than the threshold: the predicate Needs you reads.
		if b, ok := m.blocked(s, now); ok && th.Waiting > 0 && !b.Since.IsZero() && now.Sub(b.Since) >= th.Waiting {
			label := b.Label
			if b.Kind == "waiting" {
				label = b.Text
			}
			// The text carries no age: the page shows it from Since, and the notifier
			// adds it to the notification (noticeLine).
			add(AlertWaiting, signals.SevBlock, waitingPrefix+approxLabel(label, b), b.Since)
			out[len(out)-1].Approx = b.Approx
		}
		if s.Failure != nil {
			if s.Failure.Type == "rate_limit" {
				add(AlertRateLimit, signals.SevBlock, "stopped at the rate limit (StopFailure rate_limit)", s.Failure.At)
			} else {
				add(AlertStopFailure, signals.SevWarn, "the turn failed: "+s.Failure.Type, s.Failure.At)
			}
		}
		if s.Note != nil && (s.Note.Type == "quota_auto_resume_stale" || s.Note.Type == "quota_auto_resume_disabled") {
			add(AlertNoAutoResume, signals.SevWarn, signals.ClassifyNotification(s.Note.Type).Label+": resume it yourself", s.Note.At)
		}
		if th.ContextPct > 0 && s.CtxPct != nil && *s.CtxPct >= th.ContextPct && s.HookStatus != "ended" {
			add(AlertContext, signals.SevWarn, fmt.Sprintf("context at %.0f%% (alert at %.0f%%)", *s.CtxPct, th.ContextPct), s.StatusAt)
		}
		if r := m.agentReading(s, now); th.Idle > 0 && r != nil && r.Status == "idle" && !s.IdleSince.IsZero() && now.Sub(s.IdleSince) >= th.Idle {
			add(AlertIdle, signals.SevInfo, "idle", s.IdleSince)
		}
	}
	if q := v.Quota; th.FiveHourPct > 0 && q != nil && q.FiveHour != nil && *q.FiveHour >= th.FiveHourPct {
		sev := signals.SevWarn
		if *q.FiveHour >= 100 {
			sev = signals.SevBlock
		}
		// Since is the window's reset time: stable for the whole window (q.At moves on
		// every status post), so a window notifies once.
		since := q.At
		if q.FiveHourResets != nil {
			since = *q.FiveHourResets * 1000
		}
		out = append(out, AlertView{Key: AlertQuota + ":global", Kind: AlertQuota, Severity: sev, Since: since,
			Text: fmt.Sprintf("5-hour quota at %.0f%% (alert at %.0f%%)", *q.FiveHour, th.FiveHourPct)})
	}
	sort.SliceStable(out, func(i, j int) bool { return sevRank(out[i].Severity) > sevRank(out[j].Severity) })
	return out
}
