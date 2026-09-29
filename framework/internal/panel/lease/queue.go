package lease

import (
	"strings"
	"unicode/utf8"
)

// QueueConfig is one shared resource held as a lease on disk: a queue of panel.json.
type QueueConfig struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Lock is the lease directory, relative to the git common dir, so every worktree
	// of the project agrees on one path.
	Lock string `json:"lock"`
	// Command, if set, is an argv the panel can run through the queue (RUN).
	Command []string `json:"command"`
}

// QueueRun is the last RUN the panel started for a queue.
type QueueRun struct {
	PID      int    `json:"pid"`
	Worktree string `json:"worktree"`
	Log      string `json:"log"`
	Started  int64  `json:"started"`
	Ended    int64  `json:"ended,omitempty"`
	Exit     *int   `json:"exit,omitempty"`
}

// clip makes s one line of at most n runes. It keeps the panel's own rule
// (whitespace collapsed, cut at 140 runes, then at n), so a record lock-run writes
// reads as it did before the lease had a package of its own.
func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > 140 {
		s = string([]rune(s)[:140]) + "…"
	}
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
