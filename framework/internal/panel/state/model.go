// Package state is the panel's pure reducer: hook events, status-line posts and
// poll results in, the View out. It holds the one "blocked on you" predicate,
// alerts, the first-prompt decisions, reading freshness and the notifier's choice of
// what interrupts. It does no I/O and never reads the clock: every method takes
// `now`. It imports lanes only for the lane types it reconciles.
package state

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/lease"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// This file is the pure reducer: events and poll results in, state out. It does no
// I/O and never reads the clock; every method takes `now`. That keeps it testable
// against recorded payloads.

// Tunables of the reducer.
const (
	FeedCap          = 200
	staleAfter       = 60 * time.Second // busy with no hook this long → banner
	busySlack        = 10 * time.Second // a hook may land before the 2 s poll sees "busy"
	activeWindow     = 15 * time.Minute // a hook this recent keeps a session-less lane visible
	forgetSessionAge = 30 * time.Minute
	idleClearsAgents = 10 * time.Second // idle this long → no subagent can still be running
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
	Agent     *signals.Agent
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
	ID     string              `json:"id"`
	Title  string              `json:"title"`
	Source SourceStatus        `json:"source"`
	Output *signals.CardOutput `json:"output,omitempty"`
}

// Model is the panel's whole in-memory state.
type Model struct {
	cfg       *config.Config
	root      string
	startedAt time.Time

	worktrees    []signals.Worktree
	worktreesSrc SourceStatus
	agentsSrc    SourceStatus
	agentsOKAt   time.Time        // the last `claude agents` poll that succeeded
	agentsDurs   [8]time.Duration // the last poll iterations' wall times (ApplyAgentsTimed)
	agentsDurN   int
	agentsNext   time.Duration // how long the loop waits after its last poll (agentsFresh)
	prs          []signals.PR
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
	tmuxLanes    []lanes.TmuxLane
	laneRecords  []lanes.LaneRecord
	regProblems  []string
	tmuxSrc      SourceStatus
	startBlocked string
	// laneGone is when a successful tmux poll first found a registered lane's
	// session missing, by lane id (PANEL-7).
	laneGone map[string]time.Time
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
func NewModel(cfg *config.Config, root string, now time.Time) *Model {
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
		laneGone:     map[string]time.Time{},
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
func (m *Model) Worktrees() []signals.Worktree {
	return append([]signals.Worktree(nil), m.worktrees...)
}

// ApplyWorktrees records a `git worktree list` result. On error the previous list is
// kept (so a transient git failure does not drop every event) and the error shown.
func (m *Model) ApplyWorktrees(wts []signals.Worktree, err error, now time.Time) {
	if err != nil {
		m.worktreesSrc = SourceStatus{OK: false, Error: err.Error(), At: ms(now)}
		return
	}
	m.worktrees = wts
	m.worktreesSrc = SourceStatus{OK: true, At: ms(now)}
}

func (m *Model) lane(cwd string) (signals.Worktree, bool) {
	i := signals.MatchWorktree(m.worktrees, cwd)
	if i < 0 {
		return signals.Worktree{}, false
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
				if i := signals.MatchWorktree(m.worktrees, rec.Path); i >= 0 {
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

func (m *Model) worktreeByPath(p string) signals.Worktree {
	for _, w := range m.worktrees {
		if w.Path == p {
			return w
		}
	}
	return signals.Worktree{Path: p}
}

func (m *Model) pushFeed(ev FeedEvent) {
	m.feed = append(m.feed, ev)
	if len(m.feed) > FeedCap {
		m.feed = append([]FeedEvent(nil), m.feed[len(m.feed)-FeedCap:]...)
	}
}

func (m *Model) feedFor(wt signals.Worktree, sessionID, event, detail string, now time.Time) {
	typ, name := m.cfg.LaneFor(wt.Branch)
	m.pushFeed(FeedEvent{At: ms(now), Lane: wt.Path, Name: name, Type: typ, Session: sessionID, Event: event, Detail: detail})
}

// ApplyHook folds one hook event into the state. It returns false when the event was
// dropped because its cwd is outside every project worktree.
func (m *Model) ApplyHook(ev signals.HookEvent, now time.Time) bool {
	if !signals.KnownHookEvent(ev.Event) {
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
		detail = signals.OneLine(ev.Prompt)
	case "Stop":
		s.HookStatus = "idle"
		s.Compacting = ""
		detail = signals.OneLine(ev.LastAssistantMessage)
	case "StopFailure":
		s.HookStatus = "idle"
		s.Compacting = ""
		typ := ev.ErrorType
		if typ == "" {
			typ = "unknown"
		}
		s.Failure = &note{Type: typ, At: now}
		detail = "error_type " + signals.OneLine(typ)
	case "PermissionRequest":
		s.HookStatus = "waiting"
		s.Note = &note{Type: "permission_prompt", Message: signals.OneLine("wants to use " + ev.ToolName), At: now}
		detail = signals.OneLine(ev.ToolName)
	case "PreCompact":
		s.Compacting = compactionTrigger(ev)
		detail = s.Compacting
	case "PostCompact":
		s.Compacting = ""
		detail = compactionTrigger(ev)
	case "CwdChanged":
		// Recorded, never followed: the lane binding stays where it was made.
		detail = signals.OneLine(ev.PreviousCwd + " → " + ev.Cwd)
	case "SubagentStart":
		s.Subagents[ev.AgentID] = subagent{Type: ev.AgentType, Since: now}
		detail = agentLabel(ev.AgentType, ev.AgentID)
	case "SubagentStop":
		s.stopSubagent(ev.AgentID, ev.AgentType)
		detail = agentLabel(ev.AgentType, ev.AgentID)
	case "Notification":
		k := signals.ClassifyNotification(ev.NotificationType)
		n := &note{Type: ev.NotificationType, Message: signals.OneLine(ev.Message), At: now}
		switch {
		case k.Waiting:
			s.HookStatus = "waiting"
			s.Note = n
		case k.NeedsYou:
			s.Note = n
		case k.Clears:
			if s.Note != nil && signals.ClassifyNotification(s.Note.Type).Waiting {
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
		detail = strings.TrimSpace(ev.NotificationType + " " + signals.OneLine(ev.Message))
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
func (m *Model) ApplyStatus(p signals.StatusPayload, now time.Time) bool {
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
func (m *Model) ApplyAgents(agents []signals.Agent, err error, now time.Time) {
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
				d += " (" + signals.OneLine(a.WaitingFor) + ")"
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
func (m *Model) ApplyPRs(prs []signals.PR, err error, now time.Time) {
	if err != nil {
		m.prsSrc = SourceStatus{OK: false, Error: err.Error(), At: ms(now)}
		return
	}
	m.prs = prs
	m.prsSrc = SourceStatus{OK: true, At: ms(now)}
}

// ApplyCard records one card run.
func (m *Model) ApplyCard(id string, out *signals.CardOutput, err error, now time.Time) {
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
	EstCostUSD *float64     `json:"estCostUsd"`
	NeedsYou   []NeedView   `json:"needsYou"`
	Cards      []CardState  `json:"cards"`
	PRs        []signals.PR `json:"prs"`
	Feed       []FeedEvent  `json:"feed"`
	Banners    []string     `json:"banners"`
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
	Terminals    []TermLaneView        `json:"terminals"`
	LaneTypes    []config.LaneTypeInfo `json:"laneTypes"`
	StartBlocked string                `json:"startBlocked,omitempty"`
	TmuxSocket   string                `json:"tmuxSocket"`
	LaneBase     string                `json:"laneBase"`
	WorktreeRoot string                `json:"worktreeRoot"`
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
		Cards: []CardState{}, PRs: append([]signals.PR{}, m.prs...), Banners: []string{}, BannerItems: []BannerView{},
		Quota: m.quotaAt(now), HookEvents: m.hookEvents, StatusPosts: m.statusPosts, Dropped: m.dropped,
		Sources:   map[string]SourceStatus{"worktrees": m.worktreesSrc, "agents": m.agentsSrc, "prs": m.prsSrc, "tmux": m.tmuxSrc},
		Terminals: []TermLaneView{}, LaneTypes: m.cfg.LaneTypeList(), StartBlocked: m.startBlocked, TmuxSocket: m.cfg.Socket(),
		LaneBase: m.cfg.BaseRef(), WorktreeRoot: m.cfg.WorktreeRoot(m.root), AgentsReadAt: ms(m.agentsOKAt),
	}
	// A lane with a terminal is shown under the worktree it runs in.
	terms := m.TerminalViews(now)
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
				BusySince: ms(s.BusySince), Stale: m.stale(s, now), WaitingKind: signals.WaitingForKind(wf),
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
func (m *Model) ApplyTmux(lanes []lanes.TmuxLane, recs []lanes.LaneRecord, blocked string, err error, now time.Time) {
	m.tmuxSrc = SourceStatus{OK: err == nil, At: ms(now)}
	m.startBlocked = blocked
	if err != nil {
		m.tmuxSrc.Error = err.Error()
		return
	}
	m.tmuxLanes, m.laneRecords = lanes, recs
	on := map[string]bool{}
	for _, tl := range lanes {
		on[tl.ID] = true
	}
	gone := map[string]time.Time{}
	for _, rec := range recs {
		if !on[rec.ID] {
			since, ok := m.laneGone[rec.ID]
			if !ok {
				since = now
			}
			gone[rec.ID] = since
		}
	}
	m.laneGone = gone
}

func (m *Model) TerminalViews(now time.Time) []TermLaneView {
	tmux := map[string]lanes.TmuxLane{}
	for _, tl := range m.tmuxLanes {
		tmux[tl.ID] = tl
	}
	out := []TermLaneView{}
	seen := map[string]bool{}
	place := func(tv *TermLaneView) {
		if i := signals.MatchWorktree(m.worktrees, tv.Path); i >= 0 {
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

// HeuristicsVerifiedOn is the Claude Code version the undocumented behaviours the
// panel relies on were verified on: the subagent start/stop pairing (see
// stopSubagent), HTTP hooks not firing on SessionStart, and the recorded fixtures in
// testdata. When `claude --version` reports anything else the page says the subagent
// list is approximate and shows a warning.
const HeuristicsVerifiedOn = "2.1.284"

// Tunables of the v2 reducer.
const (
	// readyGrace is how long a new template lane may take to appear idle in `claude
	// agents` before "Needs you" asks you to look (the workspace-trust dialog keeps
	// a session out of the list until it is answered; verified on 2.1.284).
	readyGrace = 15 * time.Second
	// confirmGrace is how long a typed first prompt may take to show up as a
	// UserPromptSubmit (or busy) before "Needs you" says it may not have landed.
	confirmGrace = 30 * time.Second
	// goneGrace is how long a pending template lane's tmux session may be missing
	// (a start still coming up, a tmux poll not yet run) before the lane is taken to
	// need a RESTORE, and the first-prompt loop stops asking for polls (PANEL-7).
	goneGrace = 30 * time.Second
)

// Obs are the panel's own counters, kept by the runtime and copied into the model.
type Obs struct {
	OverflowDrops   int64  `json:"overflowDrops"`  // hook/status bodies dropped because the processor was behind
	MalformedDrops  int64  `json:"malformedDrops"` // bodies that did not parse
	AgentsPolls     int64  `json:"agentsPolls"`
	AgentsPollMs    int64  `json:"agentsPollMs"` // the last poll's wall time
	AgentsPollAvgMs int64  `json:"agentsPollAvgMs"`
	AgentsPollMaxMs int64  `json:"agentsPollMaxMs"`
	AgentsInterval  int64  `json:"agentsIntervalMs"`
	AgentsFilter    string `json:"agentsFilter"` // what the poll runs, and why
	NotifySent      int64  `json:"notifySent"`
	NotifyFailed    int64  `json:"notifyFailed"`
	NotifyError     string `json:"notifyError,omitempty"`
}

// NotifierStats are the notifier's counters (see notifier.go).
type NotifierStats struct {
	Day        string `json:"day"`
	Interrupts int    `json:"interrupts"` // OS notifications sent today
	Suppressed int    `json:"suppressed"` // alerts not sent because their terminal had focus
	Deferred   int    `json:"deferred"`   // alerts held back by the per-lane rate limit
}

type modelV2 struct {
	droppedUnknown int
	unknownNotifs  int
	statusVersion  string
	claudeVersion  string
	versionSrc     SourceStatus
	obs            Obs
	queues         []lease.QueueView
	queuesSrc      SourceStatus
	notifier       NotifierStats
	trust          config.TrustView
	versionSet     bool
}

// ObsView is the observability footer.
type ObsView struct {
	Obs
	HookEvents           int           `json:"hookEvents"`
	StatusPosts          int           `json:"statusPosts"`
	DroppedForeign       int           `json:"droppedForeign"`
	DroppedUnknownEvent  int           `json:"droppedUnknownEvent"`
	UnknownNotifications int           `json:"unknownNotifications"`
	ClaudeVersion        string        `json:"claudeVersion"`
	VerifiedOn           string        `json:"verifiedOn"`
	VersionSource        SourceStatus  `json:"versionSource"`
	Notifier             NotifierStats `json:"notifier"`
}

// Banner kinds: the page labels a banner by its kind (PANEL-6), never with one
// generic label for all of them.
const (
	BannerNoHooks   = "no_hooks"  // a busy lane sends no hooks
	BannerRestore   = "restore"   // lanes lost their tmux session (the restore bar acts on it)
	BannerDropped   = "dropped"   // events dropped because the panel fell behind
	BannerUntrusted = "untrusted" // panel.json changed since it was trusted
	BannerHooks     = "hooks"     // the hook install failed, or another panel has the hooks
	BannerRegistry  = "registry"  // lane registry records that cannot be shown
)

// BannerView is one banner with its kind.
type BannerView struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// banner adds a banner to both lists.
func (v *View) banner(kind, text string) {
	v.Banners = append(v.Banners, text)
	v.BannerItems = append(v.BannerItems, BannerView{Kind: kind, Text: text})
}

// ViewV2 is the v2 part of the View.
type ViewV2 struct {
	// Done holds finished turns and completed agents: your move, but not blocked.
	Done       []NeedView            `json:"done"`
	Alerts     []AlertView           `json:"alerts"`
	Templates  []config.TemplateInfo `json:"templates"`
	Queues     []lease.QueueView     `json:"queues"`
	QueuesSrc  SourceStatus          `json:"queuesSource"`
	Thresholds config.Thresholds     `json:"thresholds"`
	// QuotaGuard is set when the 5-hour quota is at or above the guard: a new lane
	// needs the override.
	QuotaGuard string           `json:"quotaGuard,omitempty"`
	Restorable []string         `json:"restorable"`
	Observe    ObsView          `json:"observe"`
	Trust      config.TrustView `json:"trust"`
	// Warnings are soft banners: nothing is lost, but something is approximate.
	Warnings []string `json:"warnings"`
}

// ApplyClaudeVersion records `claude --version`.
func (m *Model) ApplyClaudeVersion(v string, err error, now time.Time) {
	if err != nil {
		m.v2.versionSrc = SourceStatus{OK: false, Error: err.Error(), At: ms(now)}
		return
	}
	m.v2.claudeVersion, m.v2.versionSet = v, true
	m.v2.versionSrc = SourceStatus{OK: true, At: ms(now)}
}

// ApplyObs copies the runtime's counters.
func (m *Model) ApplyObs(o Obs) { m.v2.obs = o }

// ApplyNotifier copies the notifier's counters.
func (m *Model) ApplyNotifier(n NotifierStats) { m.v2.notifier = n }

// ApplyTrust records whether the config is trusted.
func (m *Model) ApplyTrust(t config.TrustView) { m.v2.trust = t }

// ApplyQueues records a read of the queues' leases.
func (m *Model) ApplyQueues(qs []lease.QueueView, err error, now time.Time) {
	if err != nil {
		m.v2.queuesSrc = SourceStatus{OK: false, Error: err.Error(), At: ms(now)}
		return
	}
	m.v2.queues = qs
	m.v2.queuesSrc = SourceStatus{OK: true, At: ms(now)}
}

// heuristicsApprox: the running Claude Code is not the one the heuristics were
// verified on. Unknown (not read yet, or unreadable) counts as approximate too.
func (m *Model) heuristicsApprox() bool {
	return !m.v2.versionSet || m.v2.claudeVersion != HeuristicsVerifiedOn
}

// quotaAt returns the quota with every window whose resets_at has passed dropped.
func (m *Model) quotaAt(now time.Time) *Quota {
	if m.quota == nil {
		return nil
	}
	q := *m.quota
	if q.FiveHourResets != nil && *q.FiveHourResets <= now.Unix() {
		q.FiveHour, q.FiveHourExpired = nil, true
	}
	if q.SevenDayResets != nil && *q.SevenDayResets <= now.Unix() {
		q.SevenDay, q.SevenDayExpired = nil, true
	}
	return &q
}

// QuotaGuardBlock says why a new lane is refused at this quota, or "". An unknown or
// expired window never blocks: the guard acts on a number it has.
func QuotaGuardBlock(q *Quota, guardPct float64) string {
	if guardPct <= 0 || q == nil || q.FiveHour == nil {
		return ""
	}
	if *q.FiveHour >= guardPct {
		return fmt.Sprintf("the 5-hour quota is at %.0f%%, at or above the quota guard (%.0f%%)", *q.FiveHour, guardPct)
	}
	return ""
}

// QuotaGuard returns the reason a new lane would be refused now, or "".
func (m *Model) QuotaGuard(now time.Time) string {
	return QuotaGuardBlock(m.quotaAt(now), m.cfg.AlertThresholds().GuardPct)
}

// needsFor adds one session's "Needs you" (blocking) and "Done" (your move) items.
// Whether it is blocked comes from blocked, the predicate the waiting alert reads.
func (m *Model) needsFor(v *View, wt signals.Worktree, name, term string, s *session, st string, now time.Time) {
	if b, ok := m.blocked(s, now); ok {
		v.NeedsYou = append(v.NeedsYou, NeedView{Lane: wt.Path, Name: name, Session: s.ID, Kind: b.Kind,
			Label: approxLabel(b.Label, b), Severity: b.Severity, Text: b.Text, At: ms(b.Since), Terminal: term, Approx: b.Approx})
	}
	// A note that needs you without blocking (quota auto-resume will not fire).
	if s.Note != nil {
		if k := signals.ClassifyNotification(s.Note.Type); k.NeedsYou && !k.Waiting {
			v.NeedsYou = append(v.NeedsYou, NeedView{Lane: wt.Path, Name: name, Session: s.ID, Kind: s.Note.Type,
				Label: k.Label, Severity: k.Severity, Text: s.Note.Message, At: ms(s.Note.At), Terminal: term})
		}
	}
	if s.Done != nil && st != "busy" && st != "waiting" {
		k := signals.ClassifyNotification(s.Done.Type)
		v.Done = append(v.Done, NeedView{Lane: wt.Path, Name: name, Session: s.ID, Kind: s.Done.Type,
			Label: k.Label, Severity: signals.SevInfo, Text: s.Done.Message, At: ms(s.Done.At), Terminal: term})
	}
}

// PromptInput is what DecideFirstPrompt needs to know about one template lane.
type PromptInput struct {
	State        string    // the record's PromptState
	Conversation bool      // the registry saw a prompt in this session
	Prompted     time.Time // the last UserPromptSubmit from the session (zero if none)
	Running      bool      // the tmux session exists
	Dead         bool      // ... and its program exited
	Listed       bool      // `claude agents` lists the session
	Status       string    // its status there
	WaitingFor   string    // its waitingFor there
	WaitingNote  bool      // a hook says it is waiting (permission, elicitation, input)
	PollFresh    bool      // the last `claude agents` poll succeeded within two intervals
	Since        time.Time // when the lane's last action began
	PromptAt     time.Time // when the prompt was typed
	// GoneSince is when the tmux polls first found the lane's session missing (zero
	// while it is there, or before the first poll).
	GoneSince time.Time
}

// PromptDecision is what to do about a template lane's first prompt.
type PromptDecision struct {
	Action string // none | wait | send | skip | stuck | delivered | restore
	Why    string
	// Poll: a fresh `claude agents` reading may change a "wait", so the first-prompt
	// loop asks for one.
	Poll bool
}

func promptWait(why string, poll bool) PromptDecision {
	return PromptDecision{Action: "wait", Why: why, Poll: poll}
}

// DecideFirstPrompt decides, from structured signals only, whether a template lane
// is ready for its first prompt. It never looks at the screen: claude is ready when
// `claude agents` lists the session as idle. A session held at the workspace-trust
// dialog is not listed at all (verified on 2.1.284), so the prompt can never be typed
// into that dialog.
func DecideFirstPrompt(in PromptInput, now time.Time) PromptDecision {
	switch in.State {
	case "pending":
		if in.Conversation || !in.Prompted.IsZero() {
			return decide("skip", "a prompt was typed in the lane first")
		}
		if !in.Running {
			// No poll can help: `claude agents` has nothing to say about a lane whose
			// tmux session is gone. After a reboot or a killed tmux server it stays
			// gone until you RESTORE it, so past the grace it is marked, not polled.
			if !in.GoneSince.IsZero() && now.Sub(in.GoneSince) >= goneGrace {
				return decide("restore", "its tmux session is gone; RESTORE the lane and the first prompt is typed once claude is ready")
			}
			return promptWait("the lane is not running", false)
		}
		if in.Dead {
			return decide("stuck", "claude exited before its first prompt was typed; RESTART the lane")
		}
		if in.Listed && in.Status == "idle" && in.WaitingFor == "" && !in.WaitingNote && in.PollFresh {
			return decide("send", "")
		}
		if in.Listed && in.Status == "idle" && !in.PollFresh {
			return promptWait("waiting for a fresh `claude agents` poll", true)
		}
		if now.Sub(in.Since) >= readyGrace {
			// No age in the text: the page shows how long from the item's time, so the
			// view stays the same (and unpushed) while nothing changes.
			return decide("stuck", "claude is not ready yet. Answer any dialog in its terminal "+
				"(workspace trust defaults to \"No, exit\": press ↓ then Enter). The first prompt is typed as soon as claude is idle.")
		}
		return promptWait("waiting for claude to be ready", true)
	case "typing":
		return decide("stuck", "the panel stopped while typing the first prompt. Check the terminal: the panel never types it twice.")
	case "sent":
		if !in.Prompted.IsZero() && !in.Prompted.Before(in.PromptAt.Add(-time.Second)) || (in.Listed && in.Status == "busy") {
			return decide("delivered", "")
		}
		if now.Sub(in.PromptAt) >= confirmGrace {
			return decide("stuck", "the first prompt was typed but claude has not started on it. Check the terminal: the text may still sit in the input box.")
		}
		return promptWait("typed; waiting for claude to start", true)
	}
	return decide("none", "")
}

func decide(action, why string) PromptDecision { return PromptDecision{Action: action, Why: why} }

// PromptDecisions returns the decision for every template lane with a prompt in
// flight, keyed by lane id.
func (m *Model) PromptDecisions(now time.Time) map[string]PromptDecision {
	out := map[string]PromptDecision{}
	tmux := map[string]lanes.TmuxLane{}
	for _, tl := range m.tmuxLanes {
		tmux[tl.ID] = tl
	}
	for _, rec := range m.laneRecords {
		if rec.PromptState == "" || rec.PromptState == "delivered" || rec.PromptState == "skipped" {
			continue
		}
		tl, running := tmux[rec.ID]
		in := PromptInput{State: rec.PromptState, Conversation: rec.Conversation, Running: running, Dead: tl.Dead,
			Since: time.UnixMilli(rec.ActionAt), PromptAt: time.UnixMilli(rec.PromptAt), PollFresh: m.agentsFresh(now),
			GoneSince: m.laneGone[rec.ID]}
		if s := m.sessions[rec.SessionID]; s != nil {
			in.Prompted = s.LastPromptAt
			// The same predicate as Needs you: never type into a session that may be
			// waiting on a dialog, however that is known.
			_, in.WaitingNote = m.blocked(s, now)
			// The last listing, current or not: PollFresh says which, and a stale
			// "idle" waits for a fresh poll rather than being typed into.
			if s.Agent != nil {
				in.Listed, in.Status, in.WaitingFor = true, s.Agent.Status, s.Agent.WaitingFor
			}
		}
		out[rec.ID] = DecideFirstPrompt(in, now)
	}
	return out
}

// agentsFresh: the last `claude agents` poll succeeded recently enough that its
// reading is current, not the last word of a poll that has since failed or stopped.
// The loop sleeps the interval it chose after the last poll (agentsNext; at least
// agentsSlow counts), and an iteration takes its own wall time (the filter
// cross-check included), so the next success can land an interval plus that long
// after the last: the window is two such intervals plus the slowest recent
// iteration. Measured from the finish, a poll slower than the interval would
// otherwise read as stale between two good polls.
func (m *Model) agentsFresh(now time.Time) bool {
	interval := AgentsSlow
	if m.agentsNext > interval {
		interval = m.agentsNext
	}
	window := 2*interval + m.slowestRecentPoll()
	return m.agentsSrc.OK && !m.agentsOKAt.IsZero() && now.Sub(m.agentsOKAt) <= window
}

// restoredPending reports whether a restored lane still waits for you: it was
// resumed on a conversation and nothing has happened in it since.
func (m *Model) restoredPending(rec lanes.LaneRecord) bool {
	if rec.Restored == 0 || !rec.Conversation {
		return false
	}
	at := time.UnixMilli(rec.Restored)
	if s := m.sessions[rec.SessionID]; s != nil {
		if s.LastPromptAt.After(at) || (!s.BusySince.IsZero() && s.BusySince.After(at)) {
			return false
		}
	}
	return true
}

// snapshotV2 fills the v2 part of the view.
func (m *Model) snapshotV2(v *View, now time.Time) {
	th := m.cfg.AlertThresholds()
	v.Templates = m.cfg.TemplateList()
	v.Thresholds = th
	v.Queues = append([]lease.QueueView{}, m.v2.queues...)
	v.QueuesSrc = m.v2.queuesSrc
	if len(m.cfg.Queues) == 0 {
		v.QueuesSrc = SourceStatus{OK: true}
	}
	v.QuotaGuard = QuotaGuardBlock(v.Quota, th.GuardPct)
	v.Trust = m.v2.trust
	v.Restorable = []string{}
	v.Warnings = []string{}
	m.hookBanners(v, now)
	if v.Done == nil {
		v.Done = []NeedView{}
	}

	recs := map[string]lanes.LaneRecord{}
	for _, r := range m.laneRecords {
		recs[r.ID] = r
	}
	decisions := m.PromptDecisions(now)
	for i := range v.Terminals {
		tv := &v.Terminals[i]
		rec, ok := recs[tv.ID]
		if !ok {
			continue
		}
		tv.Template, tv.PromptState = rec.Template, rec.PromptState
		if tv.Registered && !tv.Running && rec.Corrupt == "" {
			tv.Restorable = true
			v.Restorable = append(v.Restorable, tv.ID)
		}
		if d, ok := decisions[tv.ID]; ok {
			tv.PromptNote = d.Why
			if d.Action == "stuck" {
				v.NeedsYou = append(v.NeedsYou, NeedView{Lane: tv.Worktree, Name: tv.ID, Session: rec.SessionID,
					Kind: "first_prompt", Label: "First prompt (" + rec.Template + ")", Severity: signals.SevBlock, Text: d.Why,
					At: rec.ActionAt, Terminal: tv.ID})
			}
		}
		if tv.Running && m.restoredPending(rec) {
			v.NeedsYou = append(v.NeedsYou, NeedView{Lane: tv.Worktree, Name: tv.ID, Session: rec.SessionID, Kind: "restored",
				Label: "Restored lane", Severity: signals.SevWarn, At: rec.Restored, Terminal: tv.ID,
				Text: "Resumed with claude --resume after its tmux session was lost. If the session was idle over 1 h and above " +
					"100k tokens, claude first asks whether to resume from a summary: answer it in the terminal. The panel " +
					"types nothing into a restored lane, so continue it yourself."})
		}
	}
	sort.SliceStable(v.NeedsYou, func(i, j int) bool {
		return sevRank(v.NeedsYou[i].Severity) > sevRank(v.NeedsYou[j].Severity)
	})
	if len(v.Restorable) > 0 {
		v.banner(BannerRestore, fmt.Sprintf("%d lane(s) lost their tmux session (a reboot, or the tmux server ended): %s. "+
			"RESTORE ALL resumes each on its own session id.", len(v.Restorable), strings.Join(v.Restorable, ", ")))
	}

	v.Alerts = m.computeAlerts(v, th, now)

	v.Observe = ObsView{Obs: m.v2.obs, HookEvents: m.hookEvents, StatusPosts: m.statusPosts, DroppedForeign: m.dropped,
		DroppedUnknownEvent: m.v2.droppedUnknown, UnknownNotifications: m.v2.unknownNotifs,
		ClaudeVersion: m.v2.claudeVersion, VerifiedOn: HeuristicsVerifiedOn, VersionSource: m.v2.versionSrc,
		Notifier: m.v2.notifier}
	if v.Observe.ClaudeVersion == "" && m.v2.statusVersion != "" {
		v.Observe.ClaudeVersion = m.v2.statusVersion
	}
	if m.v2.obs.OverflowDrops > 0 {
		v.banner(BannerDropped, fmt.Sprintf("%d hook or status-line event(s) were dropped because the panel fell behind. "+
			"Lane states may lag until the next `claude agents` poll.", m.v2.obs.OverflowDrops))
	}
	if !m.v2.trust.Trusted && m.v2.trust.Hash != "" {
		v.banner(BannerUntrusted, "panel.json changed since you trusted it ("+config.ShortHash(m.v2.trust.Prev)+" → "+config.ShortHash(m.v2.trust.Hash)+
			"). Its commands (cards, queue RUN) and templates are off until you run `clauductor panel trust` (or restart with --trust-config).")
	}
	switch {
	case m.v2.versionSet && m.v2.claudeVersion != HeuristicsVerifiedOn:
		v.Warnings = append(v.Warnings, "Claude Code "+m.v2.claudeVersion+" is running; the subagent pairing and the recorded hook "+
			"fixtures were verified on "+HeuristicsVerifiedOn+". Subagent lists are approximate until they are re-verified.")
	case !m.v2.versionSrc.Pending && !m.v2.versionSrc.OK && m.v2.versionSrc.Error != "":
		v.Warnings = append(v.Warnings, "cannot read `claude --version` ("+m.v2.versionSrc.Error+"); subagent lists are approximate.")
	}
	if m.v2.droppedUnknown > 0 {
		v.Warnings = append(v.Warnings, fmt.Sprintf("%d hook event(s) with an event name the panel does not subscribe to were ignored.", m.v2.droppedUnknown))
	}
}

func sevRank(s string) int {
	return map[string]int{signals.SevBlock: 3, signals.SevWarn: 2, signals.SevInfo: 1}[s]
}

// compactionTrigger reads a compaction event's trigger (manual | auto).
func compactionTrigger(ev signals.HookEvent) string {
	t := ev.CompactionTrigger
	if t == "" {
		t = ev.Trigger
	}
	if t == "" {
		return "compaction"
	}
	return signals.OneLine(t)
}

// AgentsSlow is the `claude agents` interval while hooks are flowing: the hooks
// already carry the state, and each poll spawns a ~100 ms, ~150 MB process
// (measured on 2.1.284: p50 98 ms wall, ~105 ms CPU). No reading counts as stale
// sooner than two of these after it (agentsFresh).
const AgentsSlow = 5 * time.Second

// SetAgentsNext records how long the agents loop waits after its last poll, so
// agentsFresh allows for that wait before it calls a reading stale.
func (m *Model) SetAgentsNext(d time.Duration) { m.agentsNext = d }

// LaneCount is how many lanes the model knows of: registry records plus tmux
// sessions on the panel's socket (a lane may be counted twice; callers test > 0).
func (m *Model) LaneCount() int { return len(m.laneRecords) + len(m.tmuxLanes) }

// LaneSessions is the set of session ids the lane registry binds to lanes.
func (m *Model) LaneSessions() map[string]bool {
	out := map[string]bool{}
	for _, r := range m.laneRecords {
		out[r.SessionID] = true
	}
	return out
}
