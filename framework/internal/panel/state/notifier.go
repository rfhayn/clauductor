package state

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// Notice is one OS notification: all the new alerts of one lane, grouped.
type Notice struct {
	Group string // tmux lane id, worktree path, or "global"
	Title string
	Body  string
	Keys  []string
	// Stall is the notice's stalled alert, apart from the others it is grouped with,
	// or nil (PANEL-51 D7): the push is built from its own fields alone (PushText),
	// never from Body, which joins every new alert of the lane (a waiting alert's
	// text included).
	Stall *AlertView
}

// PushText is the push's title and body for a stalled alert (PANEL-51 D7), from the
// alert's own fields: the title as the macOS notification's (<project> · <lane>), the
// body how long the lane has been quiet, the kind of wait and the open work. Never its
// Text: that carries what it waited on in the model's words (a task's description,
// the turn's last line), and the push goes to a service outside this machine.
func PushText(project string, a AlertView, now time.Time) (title, body string) {
	title = a.Name
	if project != "" {
		title = project + " · " + a.Name
	}
	parts := []string{"stalled, quiet for " + mins(now.Sub(time.UnixMilli(a.Since)))}
	if a.Wait != "" {
		parts = append(parts, a.Wait)
	}
	// A lane whose claude exited has the exit as its kind of wait and as a piece of its
	// open work (the page lists it there): the push names it once.
	var open []string
	for _, w := range a.OpenWork {
		if w != a.Wait {
			open = append(open, w)
		}
	}
	if len(open) > 0 {
		parts = append(parts, strings.Join(open, ", "))
	}
	return title, strings.Join(parts, " · ")
}

// Notifier decides which alerts interrupt you. It is pure: Process takes the active
// alerts, the lanes whose terminal has focus in the page, and `now`.
//
//   - An alert notifies once per stretch: it is marked when sent, and forgotten when
//     it clears, so the same condition coming back notifies again.
//   - New alerts of one lane go out as ONE notification.
//   - A lane gets at most one notification per MinInterval; alerts inside that
//     window wait (they still show on the page) and go out when it ends, if still
//     active.
//   - A lane whose terminal has focus in the page gets none: you are looking at it.
//     Those alerts are marked as seen, not deferred.
//   - Interrupts counts the notifications sent per local day.
//   - Only what blocks you interrupts: SevBlock alerts, a rate limit, and a lane that
//     will not auto-resume. Idle, context and quota alerts stay on the page, and so
//     does any alert marked Approx (raised from a stale or hooks-only reading).
//   - Its state (what was notified, when each lane last was) survives a restart
//     through State/Restore, so a restart never re-notifies an alert still active.
//     A waiting alert's Since does not survive one (it is when this panel first saw
//     the wait), so a wait still active at the panel's first look is taken as the
//     stretch notified before (WaitsPending, WaitsCarried): a wait that ended and
//     began again while the panel was down reads as that stretch too.
type Notifier struct {
	MinInterval time.Duration
	Project     string
	// StallsPending is the view's flag (View.StallsPending: a source a stall rests on
	// has not answered since the start, PANEL-51 D1). While it is set, Process keeps
	// each stalled: mark it holds rather than deleting it as cleared, so a stall
	// notified before a restart is not sent again once its sources answer.
	StallsPending bool
	// StallsUnjudged is the view's lanes whose merged read is due and unanswered
	// (View.StallsUnjudged, PANEL-51 D1 and D10): Process keeps their stalled: marks,
	// so the "none" that brings back a stall cleared only by the wait is not sent again.
	StallsUnjudged []string
	// WaitsPending: `claude agents` has not answered since the start (View.AgentsReadAt
	// is 0). WaitsCarried is the view's sessions waiting since its first answer
	// (View.WaitsCarried). While either covers a waiting mark Restore loaded, Process
	// keeps the mark rather than deleting it as cleared (the alert shows only past its
	// threshold), and gives it the alert's new Since rather than notifying (#231).
	WaitsPending bool
	WaitsCarried []string

	// notified maps an alert key to the Since of the stretch it was notified for: the
	// same key with another Since is a new stretch, and notifies again.
	notified map[string]int64
	// restored is the waiting marks Restore loaded, until their alert is next seen
	// (exact) or they clear.
	restored map[string]bool
	lastSent map[string]time.Time
	stats    NotifierStats
}

// interrupts reports whether an alert may raise an OS notification.
// An approximate alert never does: it rests on data that is not current (PANEL-5).
func interrupts(a AlertView) bool {
	if a.Approx {
		return false
	}
	return interruptKind(a)
}

func interruptKind(a AlertView) bool {
	return a.Severity == signals.SevBlock || a.Kind == AlertRateLimit || a.Kind == AlertNoAutoResume
}

// NotifierState is what the notifier persists across restarts.
type NotifierState struct {
	Stats    NotifierStats    `json:"stats"`
	Notified map[string]int64 `json:"notified"` // alert key → the Since it was notified for
	LastSent map[string]int64 `json:"lastSent"` // group, terminal or worktree (namesOf) → unix ms
}

// State returns the notifier's persistent state.
func (n *Notifier) State() NotifierState {
	st := NotifierState{Stats: n.stats, Notified: map[string]int64{}, LastSent: map[string]int64{}}
	for k, since := range n.notified {
		st.Notified[k] = since
	}
	for g, t := range n.lastSent {
		st.LastSent[g] = t.UnixMilli()
	}
	return st
}

// Restore loads a saved state.
func (n *Notifier) Restore(st NotifierState) {
	n.notified, n.restored, n.lastSent = map[string]int64{}, map[string]bool{}, map[string]time.Time{}
	for k, since := range st.Notified {
		n.notified[k] = since
		if strings.HasPrefix(k, AlertWaiting+":") {
			n.restored[k] = true
		}
	}
	for g, ms := range st.LastSent {
		n.lastSent[g] = time.UnixMilli(ms)
	}
	n.stats = st.Stats
}

func groupOf(a AlertView) string {
	switch {
	case a.Terminal != "":
		return a.Terminal
	case a.Lane != "":
		return a.Lane
	}
	return "global"
}

// namesOf is every name the window of g's alerts is kept under: the group, and each
// alert's terminal and worktree. A lane's alert names its terminal only once tmux
// reports it running, so after a restart a loaded machine can raise it under the
// worktree alone (OPS-122); the window holds under either name.
func namesOf(g string, as []AlertView) []string {
	names := []string{g}
	for _, a := range as {
		for _, s := range []string{a.Terminal, a.Lane} {
			if s != "" && !slices.Contains(names, s) {
				names = append(names, s)
			}
		}
	}
	return names
}

// ProcessView is Process over a view: its alerts, with the stall flags the view sets
// for the notifier (StallsPending, StallsUnjudged), so no caller forgets one.
func (n *Notifier) ProcessView(v View, focused map[string]bool, now time.Time) []Notice {
	n.StallsPending, n.StallsUnjudged = v.StallsPending, v.StallsUnjudged
	n.WaitsPending, n.WaitsCarried = v.AgentsReadAt == 0, v.WaitsCarried
	return n.Process(v.Alerts, focused, now)
}

// Process returns the notifications to send now.
func (n *Notifier) Process(alerts []AlertView, focused map[string]bool, now time.Time) []Notice {
	if n.notified == nil {
		n.notified, n.lastSent = map[string]int64{}, map[string]time.Time{}
	}
	if day := now.Format("2006-01-02"); day != n.stats.Day {
		n.stats = NotifierStats{Day: day}
	}
	carried := map[string]bool{}
	for _, id := range n.WaitsCarried {
		carried[AlertWaiting+":"+id] = true
	}
	active := map[string]bool{}
	pending := map[string][]AlertView{}
	for _, a := range alerts {
		if !interruptKind(a) {
			continue // shown on the page, never an interruption
		}
		// An approximate alert keeps the mark of a stretch already notified (so the
		// poll flickering stale and back never re-notifies), but never sends.
		active[a.Key] = true
		if !interrupts(a) {
			continue
		}
		since, ok := n.notified[a.Key]
		carry := n.restored[a.Key] && carried[a.Key]
		delete(n.restored, a.Key) // its first exact sighting since the restart judges it
		if ok && (since == a.Since || carry) {
			n.notified[a.Key] = a.Since
			continue
		}
		g := groupOf(a)
		pending[g] = append(pending[g], a)
	}
	unjudged := map[string]bool{}
	for _, lane := range n.StallsUnjudged {
		unjudged[AlertStalled+":"+lane] = true
	}
	for k := range n.notified {
		kept := strings.HasPrefix(k, AlertStalled+":") && (n.StallsPending || unjudged[k]) ||
			n.restored[k] && (n.WaitsPending || carried[k])
		if !active[k] && !kept {
			delete(n.notified, k) // cleared: a recurrence notifies again
			delete(n.restored, k)
		}
	}
	groups := make([]string, 0, len(pending))
	for g := range pending {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	n.stats.Deferred = 0
	var out []Notice
	for _, g := range groups {
		as := pending[g]
		if focused[g] {
			for _, a := range as {
				n.notified[a.Key] = a.Since
			}
			n.stats.Suppressed += len(as)
			continue
		}
		names := namesOf(g, as)
		if slices.ContainsFunc(names, func(s string) bool { last, ok := n.lastSent[s]; return ok && now.Sub(last) < n.MinInterval }) {
			n.stats.Deferred += len(as)
			continue
		}
		name := as[0].Name
		if name == "" {
			name = "account"
		}
		lines := make([]string, 0, len(as))
		keys := make([]string, 0, len(as))
		var stall *AlertView
		for _, a := range as {
			lines = append(lines, noticeLine(a, now))
			keys = append(keys, a.Key)
			n.notified[a.Key] = a.Since
			if a.Kind == AlertStalled {
				a := a
				stall = &a
			}
		}
		for _, s := range names {
			n.lastSent[s] = now
		}
		n.stats.Interrupts++
		title := name
		if n.Project != "" {
			title = n.Project + " · " + name
		}
		out = append(out, Notice{Group: g, Title: title, Body: strings.Join(lines, "; "), Keys: keys, Stall: stall})
	}
	return out
}

// Stats returns the notifier's counters.
func (n *Notifier) Stats() NotifierStats { return n.stats }

// osascriptArgv builds an osascript argv whose script is fixed text and whose data
// arrives only as arguments (`on run argv`). Untrusted text (a lane name, a prompt
// fragment) is never spliced into AppleScript source, so a quote in it cannot end a
// string literal and run `do shell script`. And "--" comes before the data: osascript
// keeps parsing options among its arguments, so a title starting with "-e" would
// otherwise be read as more script.
func osascriptArgv(script []string, args ...string) []string {
	argv := []string{"/usr/bin/osascript"}
	argv = append(argv, "-e", "on run argv")
	for _, line := range script {
		argv = append(argv, "-e", line)
	}
	argv = append(argv, "-e", "end run", "--")
	return append(argv, args...)
}

// notifyScript is the fixed AppleScript of a notification.
var notifyScript = []string{`display notification (item 2 of argv) with title (item 1 of argv)`}

// maxNoticeText caps what a notification carries.
const maxNoticeText = 240

// NotifyArgv is the argv that shows one notification.
func NotifyArgv(title, body string) []string {
	return osascriptArgv(notifyScript, signals.Clip(title, 80), signals.Clip(body, maxNoticeText))
}

// NotifyLinesArgv is the argv that shows one notification whose body keeps its lines
// and is cut only at max runes: a notice whose sender shaped and bounded its own text
// (a catch-up notice, OPS-118 D6). NotifyArgv's one-line summary would cut it at
// signals.DetailMax and lose what follows its first 140 runes: a trust notice's run
// list, the reason a config waits for the owner.
func NotifyLinesArgv(title, body string, max int) []string {
	return osascriptArgv(notifyScript, signals.Clip(title, 80), signals.ClipLines(body, max))
}
