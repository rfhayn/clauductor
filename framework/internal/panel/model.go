package panel

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// This file is the pure reducer: events and poll results in, state out. It does no
// I/O and never reads the clock; every method takes `now`. That keeps it testable
// against recorded payloads.

// HookEvent is the subset of a Claude Code hook payload the panel uses. Unknown
// fields are ignored by encoding/json and never stored: in particular the transcript
// path is deliberately not declared, because the panel never reads transcripts.
type HookEvent struct {
	SessionID            string `json:"session_id"`
	Cwd                  string `json:"cwd"`
	Event                string `json:"hook_event_name"`
	AgentID              string `json:"agent_id"`
	AgentType            string `json:"agent_type"`
	NotificationType     string `json:"notification_type"`
	Message              string `json:"message"`
	Title                string `json:"title"`
	Prompt               string `json:"prompt"`
	LastAssistantMessage string `json:"last_assistant_message"`
	Reason               string `json:"reason"`
	// v2 events. tool_input is deliberately not declared: it can hold secrets, and
	// the panel shows only which tool asks.
	ErrorType         string `json:"error_type"`         // StopFailure
	ToolName          string `json:"tool_name"`          // PermissionRequest
	CompactionTrigger string `json:"compaction_trigger"` // PreCompact / PostCompact
	Trigger           string `json:"trigger"`            // older spelling of compaction_trigger
	PreviousCwd       string `json:"previous_cwd"`       // CwdChanged
}

// ParseHook decodes a hook body.
func ParseHook(body []byte) (HookEvent, error) {
	var ev HookEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return ev, err
	}
	if ev.Event == "" {
		return ev, fmt.Errorf("hook body has no hook_event_name")
	}
	return ev, nil
}

// StatusPayload is the subset of the statusLine stdin JSON the panel uses.
type StatusPayload struct {
	SessionID string `json:"session_id"`
	Version   string `json:"version"` // the Claude Code version that sent it
	Cwd       string `json:"cwd"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	Model struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Cost struct {
		TotalCostUSD *float64 `json:"total_cost_usd"`
	} `json:"cost"`
	ContextWindow struct {
		UsedPercentage *float64 `json:"used_percentage"`
	} `json:"context_window"`
	RateLimits struct {
		FiveHour *RateLimit `json:"five_hour"`
		SevenDay *RateLimit `json:"seven_day"`
	} `json:"rate_limits"`
}

// RateLimit is one quota window from the status line.
type RateLimit struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *int64   `json:"resets_at"`
}

// ParseStatus decodes a status-line body.
func ParseStatus(body []byte) (StatusPayload, error) {
	var s StatusPayload
	err := json.Unmarshal(body, &s)
	if err == nil && s.Cwd == "" {
		s.Cwd = s.Workspace.CurrentDir
	}
	return s, err
}

// Tunables of the reducer.
const (
	FeedCap          = 200
	staleAfter       = 60 * time.Second // busy with no hook this long → banner
	busySlack        = 10 * time.Second // a hook may land before the 2 s poll sees "busy"
	activeWindow     = 15 * time.Minute // a hook this recent keeps a session-less lane visible
	forgetSessionAge = 30 * time.Minute
	idleClearsAgents = 10 * time.Second // idle this long → no subagent can still be running
	detailMax        = 140
)

// SourceStatus reports one input's health. Pending means "not read yet", which the UI
// must never render as an empty success.
type SourceStatus struct {
	OK      bool   `json:"ok"`
	Pending bool   `json:"pending"`
	Error   string `json:"error,omitempty"`
	At      int64  `json:"at,omitempty"` // unix ms of the last attempt
}

// FeedEvent is one line of the activity feed. It holds a short summary, never a body.
type FeedEvent struct {
	At      int64  `json:"at"`
	Lane    string `json:"lane"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Session string `json:"session,omitempty"`
	Event   string `json:"event"`
	Detail  string `json:"detail,omitempty"`
}

type subagent struct {
	Type  string
	Since time.Time
}

type note struct {
	Type    string
	Message string
	At      time.Time
}

type session struct {
	ID          string
	Lane        string // worktree path
	LastHookAt  time.Time
	LastEvent   string
	LastEventAt time.Time
	HookStatus  string // from hooks alone: busy | idle | waiting
	Subagents   map[string]subagent
	Note        *note
	CtxPct      *float64
	Model       string
	StatusAt    time.Time // last status-line post
	// From `claude agents --json`.
	Agent     *Agent
	BusySince time.Time
	IdleSince time.Time

	// v2.
	Done         *note     // a "done" notification (idle_prompt, agent_completed): your move, not blocked
	Failure      *note     // the last StopFailure (error_type), until the next prompt
	Compacting   string    // "auto" | "manual" while a compaction runs
	WaitingSince time.Time // `claude agents` has reported waiting since
	LastPromptAt time.Time // the last UserPromptSubmit
	Unknown      string    // the last notification type the table does not know
}

// CardState is the last result of one card.
type CardState struct {
	ID     string       `json:"id"`
	Title  string       `json:"title"`
	Source SourceStatus `json:"source"`
	Output *CardOutput  `json:"output,omitempty"`
}

// Model is the panel's whole in-memory state.
type Model struct {
	cfg       *Config
	root      string
	startedAt time.Time

	worktrees    []Worktree
	worktreesSrc SourceStatus
	agentsSrc    SourceStatus
	agentsOKAt   time.Time        // the last `claude agents` poll that succeeded
	agentsDurs   [8]time.Duration // the last poll iterations' wall times (ApplyAgentsTimed)
	agentsDurN   int
	prs          []PR
	prsSrc       SourceStatus
	cards        []*CardState

	sessions    map[string]*session
	laneHookAt  map[string]time.Time
	costByID    map[string]float64
	quota       *Quota
	feed        []FeedEvent
	hookEvents  int
	statusPosts int
	dropped     int
	v2          modelV2
	hooks       hookHealth // PANEL-5: the hook install's health (singleton.go)

	// v1 lanes on the panel's tmux socket.
	tmuxLanes    []TmuxLane
	laneRecords  []LaneRecord
	regProblems  []string
	tmuxSrc      SourceStatus
	startBlocked string
}

// Quota is the latest account quota the status line reported.
type Quota struct {
	FiveHour       *float64 `json:"fiveHour"`
	SevenDay       *float64 `json:"sevenDay"`
	FiveHourResets *int64   `json:"fiveHourResetsAt,omitempty"`
	SevenDayResets *int64   `json:"sevenDayResetsAt,omitempty"`
	At             int64    `json:"at"`
	FromSession    string   `json:"fromSession,omitempty"`
	// A window whose resets_at has passed is dropped (its value is from before the
	// reset) and flagged, so the gauge shows "reset" rather than a stale number.
	FiveHourExpired bool `json:"fiveHourExpired,omitempty"`
	SevenDayExpired bool `json:"sevenDayExpired,omitempty"`
}

// NewModel returns an empty model. Every source starts Pending.
func NewModel(cfg *Config, root string, now time.Time) *Model {
	m := &Model{
		cfg:          cfg,
		root:         root,
		startedAt:    now,
		worktreesSrc: SourceStatus{Pending: true},
		agentsSrc:    SourceStatus{Pending: true},
		prsSrc:       SourceStatus{Pending: true},
		tmuxSrc:      SourceStatus{Pending: true},
		sessions:     map[string]*session{},
		laneHookAt:   map[string]time.Time{},
		costByID:     map[string]float64{},
	}
	m.v2.versionSrc = SourceStatus{Pending: true}
	m.v2.queuesSrc = SourceStatus{Pending: true}
	for _, c := range cfg.Cards {
		m.cards = append(m.cards, &CardState{ID: c.ID, Title: c.Title, Source: SourceStatus{Pending: true}})
	}
	return m
}

func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// Worktrees returns the current worktree authority (a copy).
func (m *Model) Worktrees() []Worktree { return append([]Worktree(nil), m.worktrees...) }

// ApplyWorktrees records a `git worktree list` result. On error the previous list is
// kept (so a transient git failure does not drop every event) and the error shown.
func (m *Model) ApplyWorktrees(wts []Worktree, err error, now time.Time) {
	if err != nil {
		m.worktreesSrc = SourceStatus{OK: false, Error: err.Error(), At: ms(now)}
		return
	}
	m.worktrees = wts
	m.worktreesSrc = SourceStatus{OK: true, At: ms(now)}
}

func (m *Model) lane(cwd string) (Worktree, bool) {
	i := MatchWorktree(m.worktrees, cwd)
	if i < 0 {
		return Worktree{}, false
	}
	return m.worktrees[i], true
}

// sess returns the session, creating it bound to lane. A session is bound ONCE: an
// event's cwd follows claude when it runs `cd`, so a later cwd never moves it. The
// lane is found by bindLane, which prefers the panel's own session id.
func (m *Model) sess(id, lane string) *session {
	s := m.sessions[id]
	if s == nil {
		s = &session{ID: id, Lane: lane, Subagents: map[string]subagent{}}
		m.sessions[id] = s
	}
	if s.Lane == "" {
		s.Lane = lane
	}
	return s
}

// bindLane returns the worktree a session belongs to:
//  1. a session the panel launched is bound by the session id it assigned
//     (--session-id), to the worktree its lane runs in, whatever the event's cwd;
//  2. a session already seen keeps the lane it was bound to at first sight;
//  3. otherwise (a session started outside the panel) the event's cwd decides, once.
func (m *Model) bindLane(sessionID, cwd string) (string, bool) {
	if sessionID != "" {
		for _, rec := range m.laneRecords {
			if rec.SessionID == sessionID {
				if i := MatchWorktree(m.worktrees, rec.Path); i >= 0 {
					return m.worktrees[i].Path, true
				}
			}
		}
		if s := m.sessions[sessionID]; s != nil && s.Lane != "" {
			return s.Lane, true
		}
	}
	if wt, ok := m.lane(cwd); ok {
		return wt.Path, true
	}
	return "", false
}

func (m *Model) worktreeByPath(p string) Worktree {
	for _, w := range m.worktrees {
		if w.Path == p {
			return w
		}
	}
	return Worktree{Path: p}
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > detailMax {
		r := []rune(s)
		s = string(r[:detailMax]) + "…"
	}
	return s
}

func (m *Model) pushFeed(ev FeedEvent) {
	m.feed = append(m.feed, ev)
	if len(m.feed) > FeedCap {
		m.feed = append([]FeedEvent(nil), m.feed[len(m.feed)-FeedCap:]...)
	}
}

func (m *Model) feedFor(wt Worktree, sessionID, event, detail string, now time.Time) {
	typ, name := m.cfg.LaneFor(wt.Branch)
	m.pushFeed(FeedEvent{At: ms(now), Lane: wt.Path, Name: name, Type: typ, Session: sessionID, Event: event, Detail: detail})
}

// ApplyHook folds one hook event into the state. It returns false when the event was
// dropped because its cwd is outside every project worktree.
func (m *Model) ApplyHook(ev HookEvent, now time.Time) bool {
	if !knownHookEvent(ev.Event) {
		// Only the events the panel subscribes to are applied; anything else is
		// counted apart from foreign-cwd drops and shown, never guessed at.
		m.v2.droppedUnknown++
		return true
	}
	lanePath, ok := m.bindLane(ev.SessionID, ev.Cwd)
	if !ok {
		m.dropped++
		return false
	}
	wt := m.worktreeByPath(lanePath)
	m.hookEvents++
	m.laneHookAt[wt.Path] = now
	s := m.sess(ev.SessionID, wt.Path)
	s.LastHookAt = now
	s.LastEvent = ev.Event
	s.LastEventAt = now
	switch ev.Event {
	case "UserPromptSubmit", "Stop", "StopFailure", "SessionEnd":
		// The turn moved on, so whatever the session was asking has been answered.
		// Subagent events do not clear it: a background agent can stop while the
		// main session still waits on a permission prompt.
		s.Note = nil
	}
	detail := ""
	switch ev.Event {
	case "UserPromptSubmit":
		s.HookStatus = "busy"
		s.LastPromptAt = now
		s.Done, s.Failure = nil, nil
		detail = oneLine(ev.Prompt)
	case "Stop":
		s.HookStatus = "idle"
		s.Compacting = ""
		detail = oneLine(ev.LastAssistantMessage)
	case "StopFailure":
		s.HookStatus = "idle"
		s.Compacting = ""
		typ := ev.ErrorType
		if typ == "" {
			typ = "unknown"
		}
		s.Failure = &note{Type: typ, At: now}
		detail = "error_type " + oneLine(typ)
	case "PermissionRequest":
		s.HookStatus = "waiting"
		s.Note = &note{Type: "permission_prompt", Message: oneLine("wants to use " + ev.ToolName), At: now}
		detail = oneLine(ev.ToolName)
	case "PreCompact":
		s.Compacting = compactionTrigger(ev)
		detail = s.Compacting
	case "PostCompact":
		s.Compacting = ""
		detail = compactionTrigger(ev)
	case "CwdChanged":
		// Recorded, never followed: the lane binding stays where it was made.
		detail = oneLine(ev.PreviousCwd + " → " + ev.Cwd)
	case "SubagentStart":
		s.Subagents[ev.AgentID] = subagent{Type: ev.AgentType, Since: now}
		detail = agentLabel(ev.AgentType, ev.AgentID)
	case "SubagentStop":
		s.stopSubagent(ev.AgentID, ev.AgentType)
		detail = agentLabel(ev.AgentType, ev.AgentID)
	case "Notification":
		k := ClassifyNotification(ev.NotificationType)
		n := &note{Type: ev.NotificationType, Message: oneLine(ev.Message), At: now}
		switch {
		case k.Waiting:
			s.HookStatus = "waiting"
			s.Note = n
		case k.NeedsYou:
			s.Note = n
		case k.Clears:
			if s.Note != nil && ClassifyNotification(s.Note.Type).Waiting {
				s.Note = nil
				if s.HookStatus == "waiting" {
					s.HookStatus = "busy"
				}
			}
		}
		if k.Done {
			s.Done = n
		}
		if k.SetsIdle {
			s.HookStatus = "idle"
		}
		if !k.Known {
			s.Unknown = ev.NotificationType
			if s.Unknown == "" {
				s.Unknown = "(no notification_type)"
			}
			m.v2.unknownNotifs++
		}
		detail = strings.TrimSpace(ev.NotificationType + " " + oneLine(ev.Message))
	case "SessionEnd":
		s.HookStatus = "ended"
		s.Subagents = map[string]subagent{}
		s.Done, s.Compacting = nil, ""
		detail = ev.Reason
	}
	m.feedFor(wt, ev.SessionID, ev.Event, detail, now)
	return true
}

// stopSubagent removes the stopped agent. What an unmatched stop means depends on its
// type, as seen live on Claude Code 2.1.284:
//   - empty type: an internal agent that never sent a start (the spike saw these too),
//     so it retires nothing;
//   - "workflow-subagent": a Workflow agent, which stops under a different id AND type
//     than it started with ("reviewer"), so it retires the oldest running agent of any
//     type. A build lane stays busy all run, so waiting for idle would not do;
//   - any other type: the oldest running agent of that same type.
//
// Hook Stop clears nothing: background agents outlive the turn that started them.
func (s *session) stopSubagent(id, typ string) {
	if _, ok := s.Subagents[id]; ok {
		delete(s.Subagents, id)
		return
	}
	var oldest string
	switch typ {
	case "":
		return
	case "workflow-subagent":
		oldest = s.oldestSubagent(func(subagent) bool { return true })
	default:
		oldest = s.oldestSubagent(func(a subagent) bool { return a.Type == typ })
	}
	if oldest != "" {
		delete(s.Subagents, oldest)
	}
}

func (s *session) oldestSubagent(match func(subagent) bool) string {
	oldest := ""
	for k, a := range s.Subagents {
		if !match(a) {
			continue
		}
		if oldest == "" || a.Since.Before(s.Subagents[oldest].Since) || (a.Since.Equal(s.Subagents[oldest].Since) && k < oldest) {
			oldest = k
		}
	}
	return oldest
}

func agentLabel(typ, id string) string {
	if typ == "" {
		typ = "(untyped)"
	}
	if len(id) > 7 {
		id = id[:7]
	}
	return typ + " " + id
}

// ApplyStatus folds one status-line payload into the state. Returns false when dropped.
func (m *Model) ApplyStatus(p StatusPayload, now time.Time) bool {
	lanePath, ok := m.bindLane(p.SessionID, p.Cwd)
	if !ok {
		m.dropped++
		return false
	}
	wt := m.worktreeByPath(lanePath)
	m.statusPosts++
	if p.Version != "" {
		m.v2.statusVersion = p.Version
	}
	if p.SessionID != "" {
		s := m.sess(p.SessionID, wt.Path)
		s.StatusAt = now
		if p.ContextWindow.UsedPercentage != nil {
			v := *p.ContextWindow.UsedPercentage
			s.CtxPct = &v
		}
		if p.Model.DisplayName != "" {
			s.Model = p.Model.DisplayName
		}
		if p.Cost.TotalCostUSD != nil {
			m.costByID[p.SessionID] = *p.Cost.TotalCostUSD
		}
	}
	rl := p.RateLimits
	// Merge per window: a live payload was seen carrying seven_day without five_hour,
	// and a missing window must not blank the last value the panel knew.
	if rl.FiveHour != nil || rl.SevenDay != nil {
		q := &Quota{}
		if m.quota != nil {
			*q = *m.quota
		}
		q.At, q.FromSession = ms(now), p.SessionID
		if rl.FiveHour != nil && rl.FiveHour.UsedPercentage != nil {
			q.FiveHour, q.FiveHourResets = rl.FiveHour.UsedPercentage, rl.FiveHour.ResetsAt
		}
		if rl.SevenDay != nil && rl.SevenDay.UsedPercentage != nil {
			q.SevenDay, q.SevenDayResets = rl.SevenDay.UsedPercentage, rl.SevenDay.ResetsAt
		}
		m.quota = q
	}
	return true
}

// ApplyAgents folds a `claude agents --json` poll. Entries outside the project are
// ignored. On error, the previous session list is kept and the error shown.
func (m *Model) ApplyAgents(agents []Agent, err error, now time.Time) {
	if err != nil {
		m.agentsSrc = SourceStatus{OK: false, Error: err.Error(), At: ms(now)}
		// The last reading is kept but is no longer current: agentReading stops
		// returning it, and a session silent past forgetSessionAge is still forgotten.
		m.forgetSessions(now)
		return
	}
	m.agentsSrc = SourceStatus{OK: true, At: ms(now)}
	m.agentsOKAt = now
	seen := map[string]bool{}
	for i := range agents {
		a := agents[i]
		if a.SessionID == "" {
			continue
		}
		lanePath, ok := m.bindLane(a.SessionID, a.Cwd)
		if !ok {
			continue
		}
		wt := m.worktreeByPath(lanePath)
		seen[a.SessionID] = true
		s := m.sess(a.SessionID, wt.Path)
		prev := ""
		if s.Agent != nil {
			prev = s.Agent.Status
		}
		switch {
		case s.Agent == nil:
			m.feedFor(wt, a.SessionID, "session", "seen · "+a.Status, now)
		case prev != a.Status:
			d := prev + " → " + a.Status
			if a.WaitingFor != "" {
				d += " (" + oneLine(a.WaitingFor) + ")"
			}
			m.feedFor(wt, a.SessionID, "status", d, now)
		}
		// busySince marks the start of a busy stretch. waiting→busy (a permission
		// granted) continues the same turn, so it does not restart the clock.
		if a.Status == "busy" && (prev == "" || prev == "idle") {
			s.BusySince = now
		}
		if a.Status == "idle" {
			s.BusySince = time.Time{}
			if prev != "idle" || s.IdleSince.IsZero() {
				s.IdleSince = now
			}
			// A session idle this long has no agent running, whatever stops were missed.
			if now.Sub(s.IdleSince) >= idleClearsAgents {
				s.Subagents = map[string]subagent{}
			}
		} else {
			s.IdleSince = time.Time{}
		}
		if a.Status == "busy" {
			// A permission prompt answered in the terminal fires no hook; the
			// session going busy again is the signal that it was answered.
			s.Note = nil
			s.Done = nil
		}
		if a.Status == "waiting" {
			if prev != "waiting" || s.WaitingSince.IsZero() {
				s.WaitingSince = now
			}
		} else {
			s.WaitingSince = time.Time{}
		}
		s.Agent = &a
	}
	for id, s := range m.sessions {
		if seen[id] {
			continue
		}
		if s.Agent != nil {
			if s.Lane != "" {
				m.feedFor(m.worktreeByPath(s.Lane), id, "session", "gone", now)
			}
			s.Agent = nil
			s.Note, s.Done = nil, nil
			s.BusySince, s.IdleSince, s.WaitingSince = time.Time{}, time.Time{}, time.Time{}
			s.Subagents = map[string]subagent{}
		}
	}
	m.forgetSessions(now)
}

// ApplyPRs records a `gh pr list` poll. On error the last list is kept but marked
// stale by the error; it is never replaced by an empty list.
func (m *Model) ApplyPRs(prs []PR, err error, now time.Time) {
	if err != nil {
		m.prsSrc = SourceStatus{OK: false, Error: err.Error(), At: ms(now)}
		return
	}
	m.prs = prs
	m.prsSrc = SourceStatus{OK: true, At: ms(now)}
}

// ApplyCard records one card run.
func (m *Model) ApplyCard(id string, out *CardOutput, err error, now time.Time) {
	for _, c := range m.cards {
		if c.ID != id {
			continue
		}
		if err != nil {
			c.Source = SourceStatus{OK: false, Error: err.Error(), At: ms(now)}
			return
		}
		c.Source = SourceStatus{OK: true, At: ms(now)}
		c.Output = out
	}
}

// ---- snapshot ----

// View is the JSON the browser renders. It is derived, never stored.
type View struct {
	Name           string     `json:"name"`
	Root           string     `json:"root"`
	Now            int64      `json:"now"`
	StartedAt      int64      `json:"startedAt"`
	Lanes          []LaneView `json:"lanes"`
	QuietWorktrees []LaneView `json:"quietWorktrees"`
	Quota          *Quota     `json:"quota"`
	// EstCostUSD sums the status line's list-price total_cost_usd over the sessions
	// the panel currently tracks: live ones, and ones heard from in the last 30 min.
	EstCostUSD *float64    `json:"estCostUsd"`
	NeedsYou   []NeedView  `json:"needsYou"`
	Cards      []CardState `json:"cards"`
	PRs        []PR        `json:"prs"`
	Feed       []FeedEvent `json:"feed"`
	Banners    []string    `json:"banners"`
	// BannerItems are the same banners with their kind, so the page labels each one
	// for what it is (PANEL-6). Banners stays for existing readers.
	BannerItems []BannerView `json:"bannerItems"`
	// AgentsReadAt is when `claude agents` was last read successfully (0: never).
	// An approximate status shows how old its reading is from this.
	AgentsReadAt int64                   `json:"agentsReadAt,omitempty"`
	Sources      map[string]SourceStatus `json:"sources"`
	HookEvents   int                     `json:"hookEvents"`
	StatusPosts  int                     `json:"statusPosts"`
	Dropped      int                     `json:"dropped"`
	// v1.
	Terminals    []TermLaneView `json:"terminals"`
	LaneTypes    []LaneTypeInfo `json:"laneTypes"`
	StartBlocked string         `json:"startBlocked,omitempty"`
	TmuxSocket   string         `json:"tmuxSocket"`
	LaneBase     string         `json:"laneBase"`
	WorktreeRoot string         `json:"worktreeRoot"`
	// v2 (model_v2.go).
	ViewV2
}

// LaneView is one lane (a worktree) as rendered.
type LaneView struct {
	ID          string         `json:"id"`
	Path        string         `json:"path"`
	Branch      string         `json:"branch"`
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Status      string         `json:"status"` // waiting | busy | idle | none
	WaitingFor  string         `json:"waitingFor,omitempty"`
	CtxPct      *float64       `json:"ctxPct"`
	Subagents   []SubagentView `json:"subagents"`
	LastEvent   string         `json:"lastEvent,omitempty"`
	LastEventAt int64          `json:"lastEventAt,omitempty"`
	LastHookAt  int64          `json:"lastHookAt,omitempty"`
	Stale       bool           `json:"stale"`
	Sessions    []SessionView  `json:"sessions"`
	Terminal    string         `json:"terminal,omitempty"` // the lane id of a tmux lane in this worktree
	// SubagentsApprox: the subagent pairing heuristics were verified on another
	// Claude Code version than the one running, so the list is approximate.
	SubagentsApprox bool `json:"subagentsApprox,omitempty"`
	// Approx: Status comes from a session whose status is not a current reading.
	Approx bool `json:"approx,omitempty"`
}

// SessionView is one Claude session inside a lane.
type SessionView struct {
	ID         string   `json:"id"`
	Name       string   `json:"name,omitempty"`
	PID        int      `json:"pid,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Status     string   `json:"status"`
	WaitingFor string   `json:"waitingFor,omitempty"`
	CtxPct     *float64 `json:"ctxPct"`
	Model      string   `json:"model,omitempty"`
	EstCostUSD *float64 `json:"estCostUsd"`
	BusySince  int64    `json:"busySince,omitempty"`
	Stale      bool     `json:"stale"`
	// v2.
	WaitingKind string `json:"waitingKind,omitempty"` // the waitingFor enum: permission | input | sandbox | worker | dialog | other
	AgentID     string `json:"agentId,omitempty"`     // `claude agents` id (background sessions)
	AgentState  string `json:"agentState,omitempty"`  // `claude agents` state (background sessions)
	Compacting  string `json:"compacting,omitempty"`
	Failure     string `json:"failure,omitempty"` // the last StopFailure error_type
	Unknown     string `json:"unknownNotification,omitempty"`
	// Approx: Status is not a current `claude agents` reading (the poll failed or is
	// old, or the session is known from hooks alone).
	Approx bool `json:"approx,omitempty"`
}

// SubagentView is one running subagent.
type SubagentView struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Since   int64  `json:"since"`
	Session string `json:"session"`
}

// NeedView is one "needs you" item.
type NeedView struct {
	Lane    string `json:"lane"`
	Name    string `json:"name"`
	Session string `json:"session"`
	// Kind is a notification type, "waiting" (from `claude agents`), or a v2 kind:
	// first_prompt | restored.
	Kind     string `json:"kind"`
	Label    string `json:"label,omitempty"`
	Severity string `json:"severity,omitempty"`
	Text     string `json:"text"`
	At       int64  `json:"at,omitempty"`
	Terminal string `json:"terminal,omitempty"` // tmux lane id: the one-click jump
	// Approx: the item rests on data that is not current (a failed or old `claude
	// agents` poll, or hooks alone). Shown, marked, and never an OS notification.
	Approx bool `json:"approx,omitempty"`
}

var statusRank = map[string]int{"waiting": 3, "busy": 2, "idle": 1}

// stale: `claude agents` says busy, the stretch is at least staleAfter old, and no
// hook has come from the lane since it began. That is the signature of a session
// that never loaded the panel's hooks.
func (m *Model) stale(s *session, now time.Time) bool {
	if s.Agent == nil || s.Agent.Status != "busy" || s.BusySince.IsZero() {
		return false
	}
	if now.Sub(s.BusySince) < staleAfter || now.Sub(m.startedAt) < staleAfter {
		return false
	}
	return m.laneHookAt[s.Lane].Before(s.BusySince.Add(-busySlack))
}

// Snapshot derives the View at `now`.
func (m *Model) Snapshot(now time.Time) View {
	v := View{
		Name: m.cfg.Name, Root: m.root, Now: ms(now), StartedAt: ms(m.startedAt),
		Lanes: []LaneView{}, QuietWorktrees: []LaneView{}, NeedsYou: []NeedView{},
		Cards: []CardState{}, PRs: append([]PR{}, m.prs...), Banners: []string{}, BannerItems: []BannerView{},
		Quota: m.quotaAt(now), HookEvents: m.hookEvents, StatusPosts: m.statusPosts, Dropped: m.dropped,
		Sources:   map[string]SourceStatus{"worktrees": m.worktreesSrc, "agents": m.agentsSrc, "prs": m.prsSrc, "tmux": m.tmuxSrc},
		Terminals: []TermLaneView{}, LaneTypes: m.cfg.LaneTypeList(), StartBlocked: m.startBlocked, TmuxSocket: m.cfg.Socket(),
		LaneBase: m.cfg.BaseRef(), WorktreeRoot: m.cfg.WorktreeRoot(m.root), AgentsReadAt: ms(m.agentsOKAt),
	}
	// A lane with a terminal is shown under the worktree it runs in.
	terms := m.terminalViews(now)
	termByWT := map[string]string{}
	for _, tv := range terms {
		if _, ok := termByWT[tv.Worktree]; tv.Running && tv.Worktree != "" && !ok {
			termByWT[tv.Worktree] = tv.ID
		}
	}
	byLane := map[string][]*session{}
	for _, s := range m.sessions {
		byLane[s.Lane] = append(byLane[s.Lane], s)
	}
	for _, wt := range m.worktrees {
		if wt.Bare {
			continue
		}
		typ, name := m.cfg.LaneFor(wt.Branch)
		lv := LaneView{ID: wt.Path, Path: wt.Path, Branch: wt.Branch, Type: typ, Name: name,
			Status: "none", Subagents: []SubagentView{}, Sessions: []SessionView{}, LastHookAt: ms(m.laneHookAt[wt.Path])}
		ss := byLane[wt.Path]
		sort.Slice(ss, func(i, j int) bool { return ss[i].ID < ss[j].ID })
		lv.Terminal = termByWT[wt.Path]
		if lv.Terminal != "" {
			// One name per lane (PANEL-6): a lane with a terminal is called what you
			// named it when you started it, on its card, its tab, Needs you and alerts.
			name, lv.Name = lv.Terminal, lv.Terminal
		}
		active := lv.Terminal != ""
		var newest time.Time
		for _, s := range ss {
			live := s.Agent != nil
			heard := s.LastHookAt
			if s.StatusAt.After(heard) {
				heard = s.StatusAt
			}
			recent := !heard.IsZero() && now.Sub(heard) < activeWindow && s.HookStatus != "ended"
			if !live && !recent && s.Note == nil {
				continue
			}
			active = true
			st, wf, approx := m.sessionStatus(s, now)
			sv := SessionView{ID: s.ID, Status: st, WaitingFor: wf, CtxPct: s.CtxPct, Model: s.Model,
				BusySince: ms(s.BusySince), Stale: m.stale(s, now), WaitingKind: waitingForKind(wf),
				Compacting: s.Compacting, Unknown: s.Unknown, Approx: approx}
			if s.Failure != nil {
				sv.Failure = s.Failure.Type
			}
			if s.Agent != nil {
				sv.Name, sv.PID, sv.Kind = s.Agent.Name, s.Agent.PID, s.Agent.Kind
				sv.AgentID, sv.AgentState = s.Agent.ID, s.Agent.State
			}
			if c, ok := m.costByID[s.ID]; ok {
				c := c
				sv.EstCostUSD = &c
			}
			lv.Sessions = append(lv.Sessions, sv)
			// On a tie, a current reading beats an approximate one.
			if r, cur := statusRank[st], statusRank[lv.Status]; r > cur || (r == cur && lv.Approx && !approx) {
				lv.Status, lv.WaitingFor, lv.Approx = st, wf, approx
			}
			if s.CtxPct != nil && (lv.CtxPct == nil || *s.CtxPct > *lv.CtxPct) {
				lv.CtxPct = s.CtxPct
			}
			if sv.Stale {
				lv.Stale = true
			}
			for id, a := range s.Subagents {
				lv.Subagents = append(lv.Subagents, SubagentView{ID: id, Type: a.Type, Since: ms(a.Since), Session: s.ID})
			}
			if s.LastEventAt.After(newest) {
				newest = s.LastEventAt
				lv.LastEvent = s.LastEvent
			}
			m.needsFor(&v, wt, name, lv.Terminal, s, st, now)
		}
		sort.Slice(lv.Subagents, func(i, j int) bool { return lv.Subagents[i].Since < lv.Subagents[j].Since })
		lv.SubagentsApprox = m.heuristicsApprox()
		lv.LastEventAt = ms(newest)
		if lv.Stale {
			v.banner(BannerNoHooks, fmt.Sprintf(
				"%s is busy per `claude agents` but no hook has arrived from it for %ds. The session probably predates the hook install: restart it to load ~/.claude/settings.json.",
				name, int(staleAfter.Seconds())))
		}
		if active {
			v.Lanes = append(v.Lanes, lv)
		} else {
			v.QuietWorktrees = append(v.QuietWorktrees, lv)
		}
	}
	v.Terminals = terms
	for _, p := range m.regProblems {
		v.banner(BannerRegistry, p)
	}
	if len(m.costByID) > 0 {
		total := 0.0
		for _, c := range m.costByID {
			total += c
		}
		v.EstCostUSD = &total
	}
	for _, c := range m.cards {
		v.Cards = append(v.Cards, *c)
	}
	// Newest first for the browser.
	v.Feed = make([]FeedEvent, 0, len(m.feed))
	for i := len(m.feed) - 1; i >= 0; i-- {
		ev := m.feed[i]
		if id := termByWT[ev.Lane]; id != "" {
			ev.Name = id // one name per lane, in the feed too
		}
		v.Feed = append(v.Feed, ev)
	}
	m.snapshotV2(&v, now)
	return v
}

// TermLaneView is one lane: a registry record, a tmux session on the panel's
// socket, or both. The reducer reconciles the two and binds a lane to its Claude
// session by the session id the panel assigned, never by cwd.
type TermLaneView struct {
	ID         string   `json:"id"`
	SessionID  string   `json:"sessionId,omitempty"`
	Path       string   `json:"path"`
	Type       string   `json:"type"`
	Worktree   string   `json:"worktree"` // "" when the path is in none of the project's worktrees
	Branch     string   `json:"branch"`
	Status     string   `json:"status"` // busy | idle | waiting | running (no signal yet) | dead | orphaned
	WaitingFor string   `json:"waitingFor,omitempty"`
	CtxPct     *float64 `json:"ctxPct"`
	Created    int64    `json:"created"`
	Attached   int      `json:"attached"`
	Running    bool     `json:"running"` // a tmux session exists
	Dead       bool     `json:"dead"`    // the tmux session exists, the program exited
	DeadStatus string   `json:"deadStatus,omitempty"`
	Registered bool     `json:"registered"`
	// Orphan says what does not add up, e.g. a registered lane whose tmux session is
	// gone (a reboot), or a tmux session the registry does not know. "" when sound.
	Orphan string `json:"orphan,omitempty"`
	Action string `json:"action,omitempty"` // a registry action begun and not finished
	// v2.
	Restorable  bool   `json:"restorable,omitempty"` // registered, its tmux session gone: RESTORE brings it back
	Template    string `json:"template,omitempty"`
	PromptState string `json:"promptState,omitempty"` // pending | typing | sent | delivered | skipped
	PromptNote  string `json:"promptNote,omitempty"`
	// Approx: Status is not a current `claude agents` reading (PANEL-5).
	Approx bool `json:"approx,omitempty"`
}

// ApplyRegistryProblems records registry records that could not be shown at all.
func (m *Model) ApplyRegistryProblems(p []string) { m.regProblems = p }

// ApplyTmux records a reconciliation input: the panel's tmux socket, the lane
// registry, and whether lanes may start.
func (m *Model) ApplyTmux(lanes []TmuxLane, recs []LaneRecord, blocked string, err error, now time.Time) {
	m.tmuxSrc = SourceStatus{OK: err == nil, At: ms(now)}
	m.startBlocked = blocked
	if err != nil {
		m.tmuxSrc.Error = err.Error()
		return
	}
	m.tmuxLanes, m.laneRecords = lanes, recs
}

func (m *Model) terminalViews(now time.Time) []TermLaneView {
	tmux := map[string]TmuxLane{}
	for _, tl := range m.tmuxLanes {
		tmux[tl.ID] = tl
	}
	out := []TermLaneView{}
	seen := map[string]bool{}
	place := func(tv *TermLaneView) {
		if i := MatchWorktree(m.worktrees, tv.Path); i >= 0 {
			wt := m.worktrees[i]
			tv.Worktree, tv.Branch = wt.Path, wt.Branch
			if tv.Type == "" {
				tv.Type, _ = m.cfg.LaneFor(wt.Branch)
			}
		}
	}
	for _, rec := range m.laneRecords {
		seen[rec.ID] = true
		tv := TermLaneView{ID: rec.ID, SessionID: rec.SessionID, Path: rec.Path, Type: rec.Type, Branch: rec.Branch,
			Created: rec.Created / 1000, Registered: true, Status: "running"}
		if !rec.ActionDone {
			tv.Action = rec.Action
		}
		if rec.Corrupt != "" {
			tv.Orphan = "corrupt registry record (" + rec.Corrupt + "): it is never launched; stop or forget it"
		}
		place(&tv)
		if tl, ok := tmux[rec.ID]; ok {
			tv.Running, tv.Attached, tv.Dead, tv.DeadStatus = true, tl.Attached, tl.Dead, tl.DeadStatus
			if s := m.sessions[rec.SessionID]; s != nil {
				tv.Status, tv.WaitingFor, tv.Approx = m.sessionStatus(s, now)
				tv.CtxPct = s.CtxPct
			}
			if tl.Dead {
				tv.Status = "dead"
			}
		} else if rec.Corrupt != "" {
			tv.Status = "orphaned"
		} else {
			tv.Status = "orphaned"
			tv.Orphan = "its tmux session is gone (a reboot, or the tmux server ended)"
			if !rec.ActionDone {
				tv.Orphan = "the panel stopped during \"" + rec.Action + "\" and the lane never came up"
			}
		}
		if tv.Worktree == "" && m.worktreesSrc.OK {
			tv.Orphan = strings.TrimPrefix(tv.Orphan+"; ", "; ") + rec.Path + " is not one of the project's worktrees any more"
		}
		out = append(out, tv)
	}
	for _, tl := range m.tmuxLanes {
		if seen[tl.ID] {
			continue
		}
		tv := TermLaneView{ID: tl.ID, Path: tl.Path, Type: tl.Type, Created: tl.Created, Attached: tl.Attached,
			Running: true, Dead: tl.Dead, DeadStatus: tl.DeadStatus, Status: "running",
			Orphan: "not in the lane registry, so its session id is unknown: it cannot be restarted or resumed, only stopped"}
		place(&tv)
		if tl.Dead {
			tv.Status = "dead"
		}
		out = append(out, tv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
