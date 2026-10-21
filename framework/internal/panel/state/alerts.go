package state

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
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
	AlertContext      = "context"        // a session's context at or above its lane's threshold: against its window, posted or recorded, else used_percentage (PANEL-48)
	AlertIdle         = "idle"           // a live session idle longer than N min
	AlertQuota        = "quota"          // the 5-hour quota above the threshold
	// PANEL-28: Claude Code's login-refresh locks, the machine's (like the quota).
	AlertLoginLock    = "login_lock"    // a lock live, the same lock, for more than 2 min
	AlertStrandedLock = "stranded_lock" // a stale lock still there 2 min after it went stale
	AlertFutureLock   = "future_lock"   // a lock modified more than 5 s in the future
	// PANEL-51: a lane that waited on you, or has open work, and stopped moving (stall.go).
	AlertStalled = "stalled"
)

// loginLockAfter is how long a lock stays live, or stale, before the panel says so:
// a refresh takes seconds, and Claude Code itself treats a lock untouched for 60 s
// as stale.
const loginLockAfter = 2 * time.Minute

// MachineAlert reports whether an alert is the machine's, the same in every
// project's view (the quota, the login locks): the machine notifies it once, and no
// project's notifier does.
func MachineAlert(kind string) bool {
	switch kind {
	case AlertQuota, AlertLoginLock, AlertStrandedLock, AlertFutureLock:
		return true
	}
	return false
}

// ApplyLoginLocks records the login-refresh locks as the panel's watch last read them
// (PANEL-28): the alerts below are derived from them.
func (m *Model) ApplyLoginLocks(locks []types.LoginLock) {
	m.loginLocks = append([]types.LoginLock(nil), locks...)
}

// LoginLocks is the locks ApplyLoginLocks last recorded (a copy).
func (m *Model) LoginLocks() []types.LoginLock {
	return append([]types.LoginLock(nil), m.loginLocks...)
}

// loginLockAlerts derives each lock's alert or warning at now (design D7). The panel
// never removes or writes a lock: each says what the owner can do.
func loginLockAlerts(locks []types.LoginLock, now time.Time) []AlertView {
	var out []AlertView
	for _, l := range locks {
		a := AlertView{Key: ":" + l.Which}
		switch {
		case l.State == "live" && l.LiveSince != 0 && now.Sub(time.UnixMilli(l.LiveSince)) > loginLockAfter:
			a.Kind, a.Severity, a.Since = AlertLoginLock, signals.SevBlock, l.LiveSince
			if l.Birth != 0 {
				a.Since = l.Birth // the lock's age: from its birth, as the record states it
			}
			holder, end := "its holder is unknown ("+strings.TrimPrefix(l.Holder, "unknown: ")+")", ""
			switch {
			case l.PID > 0 && (l.HolderState == "gone" || l.HolderState == "gone (pid reused)"):
				// The record does not name the live holder: no pid to end.
				holder = "and the owner record's holder, pid " + strconv.Itoa(l.PID) + ", is " + l.HolderState + ", so the record does not name the live holder"
			case l.PID > 0:
				holder, end = "held by pid "+strconv.Itoa(l.PID)+lockCommand(l), "; if it comes back, end pid "+strconv.Itoa(l.PID)
			}
			a.Text = "a login refresh has held " + l.Path + " for over 2 min, " + holder + ". Run /login in any session" + end
		case l.State == "stale" && l.StaleSince != 0 && now.Sub(time.UnixMilli(l.StaleSince)) >= loginLockAfter:
			a.Kind, a.Severity, a.Since = AlertStrandedLock, signals.SevWarn, l.ModTime
			// A holder that still runs is ended first (D7, amended): removing a lock a
			// live process holds would let two processes refresh the login at once.
			remedy := "If sessions fail to refresh, remove " + l.Path + " and run /login"
			if l.PID > 0 && l.HolderState == "running" {
				remedy = "End pid " + strconv.Itoa(l.PID) + " first; if sessions still fail to refresh, remove " + l.Path + " and run /login"
			}
			a.Text = "the " + l.Which + " login lock " + l.Path + " is stale (last modified " +
				time.UnixMilli(l.ModTime).Format("15:04:05") + ") and still there; " + lastHolder(l) + ". " + remedy
		case l.State == "future":
			a.Kind, a.Severity = AlertFutureLock, signals.SevWarn
			a.Text = "the " + l.Which + " login lock " + l.Path + " is dated in the future (" +
				time.UnixMilli(l.ModTime).Format("2006-01-02 15:04:05") + "): Claude Code waits for real time to pass it (#95739). " +
				"If sessions fail to refresh, remove " + l.Path + " and run /login"
		default:
			continue
		}
		a.Key = a.Kind + a.Key
		out = append(out, a)
	}
	return out
}

// lockCommand is " (its command)" when the watch read it, and the panel's own call
// named when it was one.
func lockCommand(l types.LoginLock) string {
	switch {
	case l.Holder == "own":
		return " (the panel's own " + l.Call + ")"
	case l.Command != "":
		return " (" + l.Command + ")"
	}
	return ""
}

// lastHolder says who last held a stale lock, and whether that process is gone.
func lastHolder(l types.LoginLock) string {
	if l.PID <= 0 {
		return "its last holder is unknown (" + strings.TrimPrefix(l.Holder, "unknown: ") + ")"
	}
	who := "its last holder, pid " + strconv.Itoa(l.PID) + lockCommand(l) + ", "
	switch l.HolderState {
	case "gone":
		return who + "is gone"
	case "gone (pid reused)":
		return who + "is gone (the pid now names another process)"
	case "running":
		return who + "still runs"
	}
	return who + "may or may not run"
}

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
	// Wait and OpenWork are a stalled alert's own fields, from which the push is built
	// (PANEL-51 D7): the kind of wait and the open work, in fixed words, never a task's
	// description or the turn's last line. Not in /api/state: the page reads the lane's
	// stall.
	Wait     string   `json:"-"`
	OpenWork []string `json:"-"`
}

// waitingPrefix starts a waiting alert's text; stalledPrefix a stalled alert's.
const (
	waitingPrefix = "waiting on you: "
	stalledPrefix = "stalled: "
)

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
	case AlertLoginLock:
		return "login lock " + age + " old: " + a.Text
	case AlertStalled:
		// The Mac's own notification may carry what it waited on (D7: only the push
		// leaves the machine), after how long the lane has been quiet.
		return "stalled, quiet for " + age + ": " + strings.TrimPrefix(a.Text, stalledPrefix)
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
		// The lane's own threshold (D13): a windowed lane's sits below its tick.
		if at := ctxAlertPct(th.ContextPct, s.ctxWindow); at > 0 && s.CtxPct != nil && *s.CtxPct >= at && s.HookStatus != "ended" {
			add(AlertContext, signals.SevWarn, fmt.Sprintf("context at %.0f%% (alert at %s%%)", *s.CtxPct, pctText(at)), s.ctxSince())
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
	out = append(out, loginLockAlerts(m.loginLocks, now)...)
	sort.SliceStable(out, func(i, j int) bool { return sevRank(out[i].Severity) > sevRank(out[j].Severity) })
	return out
}
