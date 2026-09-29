package panel

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Notice is one OS notification: all the new alerts of one lane, grouped.
type Notice struct {
	Group string // tmux lane id, worktree path, or "global"
	Title string
	Body  string
	Keys  []string
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
//     will not auto-resume. Idle, context and quota alerts stay on the page.
//   - Its state (what was notified, when each lane last was) survives a restart
//     through State/Restore, so a restart never re-notifies an alert still active.
type Notifier struct {
	MinInterval time.Duration
	Project     string

	notified map[string]bool
	lastSent map[string]time.Time
	stats    NotifierStats
}

// Interrupts reports whether an alert may raise an OS notification.
func Interrupts(a AlertView) bool {
	return a.Severity == SevBlock || a.Kind == AlertRateLimit || a.Kind == AlertNoAutoResume
}

// NotifierState is what the notifier persists across restarts.
type NotifierState struct {
	Stats    NotifierStats    `json:"stats"`
	Notified []string         `json:"notified"`
	LastSent map[string]int64 `json:"lastSent"` // group → unix ms
}

// State returns the notifier's persistent state.
func (n *Notifier) State() NotifierState {
	st := NotifierState{Stats: n.stats, Notified: []string{}, LastSent: map[string]int64{}}
	for k := range n.notified {
		st.Notified = append(st.Notified, k)
	}
	sort.Strings(st.Notified)
	for g, t := range n.lastSent {
		st.LastSent[g] = t.UnixMilli()
	}
	return st
}

// Restore loads a saved state.
func (n *Notifier) Restore(st NotifierState) {
	n.notified, n.lastSent = map[string]bool{}, map[string]time.Time{}
	for _, k := range st.Notified {
		n.notified[k] = true
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

// Process returns the notifications to send now.
func (n *Notifier) Process(alerts []AlertView, focused map[string]bool, now time.Time) []Notice {
	if n.notified == nil {
		n.notified, n.lastSent = map[string]bool{}, map[string]time.Time{}
	}
	if day := now.Format("2006-01-02"); day != n.stats.Day {
		n.stats = NotifierStats{Day: day}
	}
	active := map[string]bool{}
	pending := map[string][]AlertView{}
	for _, a := range alerts {
		if !Interrupts(a) {
			continue // shown on the page, never an interruption
		}
		active[a.Key] = true
		if n.notified[a.Key] {
			continue
		}
		g := groupOf(a)
		pending[g] = append(pending[g], a)
	}
	for k := range n.notified {
		if !active[k] {
			delete(n.notified, k) // cleared: a recurrence notifies again
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
				n.notified[a.Key] = true
			}
			n.stats.Suppressed += len(as)
			continue
		}
		if last, ok := n.lastSent[g]; ok && now.Sub(last) < n.MinInterval {
			n.stats.Deferred += len(as)
			continue
		}
		name := as[0].Name
		if name == "" {
			name = "account"
		}
		lines := make([]string, 0, len(as))
		keys := make([]string, 0, len(as))
		for _, a := range as {
			lines = append(lines, a.Text)
			keys = append(keys, a.Key)
			n.notified[a.Key] = true
		}
		n.lastSent[g] = now
		n.stats.Interrupts++
		title := name
		if n.Project != "" {
			title = n.Project + " · " + name
		}
		out = append(out, Notice{Group: g, Title: title, Body: strings.Join(lines, "; "), Keys: keys})
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

func clip(s string, n int) string {
	s = oneLine(s)
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// NotifyArgv is the argv that shows one notification.
func NotifyArgv(title, body string) []string {
	return osascriptArgv(notifyScript, clip(title, 80), clip(body, maxNoticeText))
}

// SendNotice shows a notification with osascript.
func SendNotice(ctx context.Context, nt Notice) error {
	argv := NotifyArgv(nt.Title, nt.Body)
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
