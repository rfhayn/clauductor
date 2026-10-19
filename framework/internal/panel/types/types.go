// Package types holds the plain records the panel's packages hand each other:
// a lane registry record, a tmux lane, and a queue's config and view. It has no
// code and no imports, so the reducer can take them without linking what makes them.
package types

// LoginLock is one of Claude Code's login-refresh locks as the panel's lock watch
// last read it (PANEL-28): what the reducer derives its alerts from. Times are unix
// ms, 0 when there is none.
type LoginLock struct {
	Which   string `json:"which"` // "current" or "legacy"
	Path    string `json:"path"`
	State   string `json:"state"` // "live", "stale" or "future"
	ModTime int64  `json:"modTime"`
	// LiveSince is when the watch first saw this lock, this identity, live in an
	// unbroken run of live readings; 0 when it is not live.
	LiveSince int64 `json:"liveSince"`
	// StaleSince is when a stale lock went stale (its mtime plus Claude Code's 60 s);
	// 0 when it is not stale.
	StaleSince int64 `json:"staleSince"`
	Birth      int64 `json:"birth"` // the owner record's lockBirthtimeMs
	PID        int   `json:"pid,omitempty"`
	// Command is the holder's `ps -o command=` when the watch first saw it running.
	Command string `json:"command,omitempty"`
	// HolderState is the holder as last checked: "running", "gone" or "gone (pid
	// reused)"; "" when no record names one.
	HolderState string `json:"holderState,omitempty"`
	Holder      string `json:"holder"` // whose: "own", "other" or "unknown: <why>"
	Call        string `json:"call,omitempty"`
}

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
	Type       string // the lane type the panel started it as (brand.TmuxOption "type": @clauductor_type, or a legacy name's)
	Project    string // the project id it was started for (brand.TmuxOption "project"; "" before PANEL-16)
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
	// Port is the lane's own port (PANEL-20, ports in panel.json), exported to it as
	// CLAUDUCTOR_PORT; 0 without one.
	Port int `json:"port,omitempty"`
	// ContextWindow is the context window the lane was last launched with (PANEL-48,
	// launch.context_window in panel.json); 0 without one. It is set on every launch,
	// so a config edit moves it only once the lane is relaunched.
	ContextWindow int `json:"contextWindow,omitempty"`
}
