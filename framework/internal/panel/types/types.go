// Package types holds the plain records the panel's packages hand each other:
// a lane registry record, a tmux lane, and a queue's config and view. It has no
// code and no imports, so the reducer can take them without linking what makes them.
package types

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

// LeaseView is a holder or a waiter as the page shows it.
type LeaseView struct {
	Nonce      string `json:"nonce"`
	PID        int    `json:"pid"`
	Lane       string `json:"lane,omitempty"`
	Cmd        string `json:"cmd,omitempty"`
	Started    int64  `json:"started"` // unix ms
	Renewed    int64  `json:"renewed"` // unix ms
	TTL        int64  `json:"ttl"`
	Alive      bool   `json:"alive"`
	Stale      bool   `json:"stale"`
	StaleWhy   string `json:"staleWhy,omitempty"`
	Cancelling bool   `json:"cancelling,omitempty"`
}

// QueueView is one queue.
type QueueView struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Lock       string      `json:"lock"`
	HasCommand bool        `json:"hasCommand"`
	Held       bool        `json:"held"`
	Holder     *LeaseView  `json:"holder,omitempty"`
	HolderNote string      `json:"holderNote,omitempty"`
	Waiters    []LeaseView `json:"waiters"`
	Error      string      `json:"error,omitempty"`
	Run        *QueueRun   `json:"run,omitempty"` // the last RUN the panel started
}

// TmuxLane is one session on the panel's socket.
type TmuxLane struct {
	ID         string
	Path       string // the pane's cwd (resolved), falling back to the session's start dir
	Type       string // the lane type the panel started it as (@clauductor_type)
	Created    int64  // unix seconds
	Attached   int    // attached clients
	Dead       bool   // the lane's program exited (remain-on-exit keeps its output)
	DeadStatus string
}

// LaneRecord is one registered lane.
type LaneRecord struct {
	ID        string `json:"id"`
	SessionID string `json:"sessionId"` // the id the panel passed to claude --session-id
	Path      string `json:"path"`
	Type      string `json:"type"`
	Branch    string `json:"branch,omitempty"`
	Mode      string `json:"mode"`
	Created   int64  `json:"created"` // unix ms
	// Action is the last action begun on the lane: start | restart | resume | stop.
	Action     string `json:"action"`
	ActionAt   int64  `json:"actionAt"`
	ActionDone bool   `json:"actionDone"`
	// Conversation is set once a hook reports a prompt submitted in this session:
	// only then does `claude --resume <id>` have a conversation to resume. Until
	// then a restart reuses --session-id <id>.
	Conversation bool `json:"conversation,omitempty"`
	// Corrupt, when set, says why this record failed validation on load. A corrupt
	// record is shown, can be stopped or forgotten, and is never launched.
	Corrupt string `json:"-"`

	// v2: a template lane's launch options and first prompt, and restores.
	Template    string `json:"template,omitempty"`
	Model       string `json:"model,omitempty"`
	Effort      string `json:"effort,omitempty"`
	FirstPrompt string `json:"firstPrompt,omitempty"`
	// PromptState is pending → typing → sent → delivered, or skipped. "typing" is
	// written before the first keystroke, so a panel that dies mid-typing never
	// types the prompt a second time.
	PromptState string `json:"promptState,omitempty"`
	PromptAt    int64  `json:"promptAt,omitempty"` // unix ms the prompt was typed
	Restored    int64  `json:"restored,omitempty"` // unix ms of the last restore
}
