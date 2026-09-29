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
}

// Quota is the latest account quota the status line reported.
type Quota struct {
	FiveHour       *float64 `json:"fiveHour"`
	SevenDay       *float64 `json:"sevenDay"`
	FiveHourResets *int64   `json:"fiveHourResetsAt,omitempty"`
	SevenDayResets *int64   `json:"sevenDayResetsAt,omitempty"`
	At             int64    `json:"at"`
	FromSession    string   `json:"fromSession,omitempty"`
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
		sessions:     map[string]*session{},
		laneHookAt:   map[string]time.Time{},
		costByID:     map[string]float64{},
	}
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

func (m *Model) sess(id, lane string) *session {
	s := m.sessions[id]
	if s == nil {
		s = &session{ID: id, Lane: lane, Subagents: map[string]subagent{}}
		m.sessions[id] = s
	}
	if lane != "" {
		s.Lane = lane
	}
	return s
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
	wt, ok := m.lane(ev.Cwd)
	if !ok {
		m.dropped++
		return false
	}
	m.hookEvents++
	m.laneHookAt[wt.Path] = now
	s := m.sess(ev.SessionID, wt.Path)
	s.LastHookAt = now
	s.LastEvent = ev.Event
	s.LastEventAt = now
	if ev.Event != "Notification" {
		// Any later activity from the session answers whatever it was asking.
		s.Note = nil
	}
	detail := ""
	switch ev.Event {
	case "UserPromptSubmit":
		s.HookStatus = "busy"
		detail = oneLine(ev.Prompt)
	case "Stop":
		s.HookStatus = "idle"
		detail = oneLine(ev.LastAssistantMessage)
	case "SubagentStart":
		s.Subagents[ev.AgentID] = subagent{Type: ev.AgentType, Since: now}
		detail = agentLabel(ev.AgentType, ev.AgentID)
	case "SubagentStop":
		s.stopSubagent(ev.AgentID, ev.AgentType)
		detail = agentLabel(ev.AgentType, ev.AgentID)
	case "Notification":
		s.HookStatus = "waiting"
		s.Note = &note{Type: ev.NotificationType, Message: oneLine(ev.Message), At: now}
		detail = strings.TrimSpace(ev.NotificationType + " " + oneLine(ev.Message))
	case "SessionEnd":
		s.HookStatus = "ended"
		s.Subagents = map[string]subagent{}
		detail = ev.Reason
	}
	m.feedFor(wt, ev.SessionID, ev.Event, detail, now)
	return true
}

// stopSubagent removes the stopped agent. Workflow agents were seen (live, 2.1.284)
// to stop under a different agent_id AND a different agent_type than they started
// with (start "reviewer", stop "workflow-subagent"), and a build lane stays busy for
// an hour, so waiting for idle would leave the count inflated all run. An unknown id
// therefore retires the oldest running agent of the same type, or failing that the
// oldest of any type (FIFO). Hook Stop clears nothing: background agents outlive the
// turn that started them.
func (s *session) stopSubagent(id, typ string) {
	if _, ok := s.Subagents[id]; ok {
		delete(s.Subagents, id)
		return
	}
	if oldest := s.oldestSubagent(func(a subagent) bool { return a.Type == typ }); oldest != "" {
		delete(s.Subagents, oldest)
		return
	}
	if oldest := s.oldestSubagent(func(subagent) bool { return true }); oldest != "" {
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
	wt, ok := m.lane(p.Cwd)
	if !ok {
		m.dropped++
		return false
	}
	m.statusPosts++
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
		return
	}
	m.agentsSrc = SourceStatus{OK: true, At: ms(now)}
	seen := map[string]bool{}
	for i := range agents {
		a := agents[i]
		wt, ok := m.lane(a.Cwd)
		if !ok || a.SessionID == "" {
			continue
		}
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
		}
		s.Agent = &a
	}
	for id, s := range m.sessions {
		if seen[id] {
			continue
		}
		if s.Agent != nil {
			if wt, ok := m.lane(s.Agent.Cwd); ok {
				m.feedFor(wt, id, "session", "gone", now)
			}
			s.Agent = nil
			s.Note = nil
			s.BusySince, s.IdleSince = time.Time{}, time.Time{}
			s.Subagents = map[string]subagent{}
		}
		if now.Sub(s.LastHookAt) > forgetSessionAge && now.Sub(s.StatusAt) > forgetSessionAge {
			delete(m.sessions, id)
			delete(m.costByID, id) // the est. $ sum covers tracked sessions only
		}
	}
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
	EstCostUSD  *float64                `json:"estCostUsd"`
	NeedsYou    []NeedView              `json:"needsYou"`
	Cards       []CardState             `json:"cards"`
	PRs         []PR                    `json:"prs"`
	Feed        []FeedEvent             `json:"feed"`
	Banners     []string                `json:"banners"`
	Sources     map[string]SourceStatus `json:"sources"`
	HookEvents  int                     `json:"hookEvents"`
	StatusPosts int                     `json:"statusPosts"`
	Dropped     int                     `json:"dropped"`
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
	Kind    string `json:"kind"` // permission_prompt | idle_prompt | waiting
	Text    string `json:"text"`
	At      int64  `json:"at,omitempty"`
}

var statusRank = map[string]int{"waiting": 3, "busy": 2, "idle": 1}

func (s *session) status() (string, string) {
	if s.Agent != nil {
		return s.Agent.Status, s.Agent.WaitingFor
	}
	switch s.HookStatus {
	case "busy", "idle", "waiting":
		return s.HookStatus, ""
	}
	return "idle", ""
}

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
		Cards: []CardState{}, PRs: append([]PR{}, m.prs...), Banners: []string{},
		Quota: m.quota, HookEvents: m.hookEvents, StatusPosts: m.statusPosts, Dropped: m.dropped,
		Sources: map[string]SourceStatus{"worktrees": m.worktreesSrc, "agents": m.agentsSrc, "prs": m.prsSrc},
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
		active := false
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
			st, wf := s.status()
			sv := SessionView{ID: s.ID, Status: st, WaitingFor: wf, CtxPct: s.CtxPct, Model: s.Model,
				BusySince: ms(s.BusySince), Stale: m.stale(s, now)}
			if s.Agent != nil {
				sv.Name, sv.PID, sv.Kind = s.Agent.Name, s.Agent.PID, s.Agent.Kind
			}
			if c, ok := m.costByID[s.ID]; ok {
				c := c
				sv.EstCostUSD = &c
			}
			lv.Sessions = append(lv.Sessions, sv)
			if statusRank[st] > statusRank[lv.Status] {
				lv.Status, lv.WaitingFor = st, wf
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
			if s.Note != nil && (s.Note.Type == "permission_prompt" || s.Note.Type == "idle_prompt") {
				v.NeedsYou = append(v.NeedsYou, NeedView{Lane: wt.Path, Name: name, Session: s.ID,
					Kind: s.Note.Type, Text: s.Note.Message, At: ms(s.Note.At)})
			} else if st == "waiting" && s.Agent != nil {
				text := s.Agent.WaitingFor
				if text == "" {
					text = "waiting for input"
				}
				v.NeedsYou = append(v.NeedsYou, NeedView{Lane: wt.Path, Name: name, Session: s.ID, Kind: "waiting", Text: oneLine(text)})
			}
		}
		sort.Slice(lv.Subagents, func(i, j int) bool { return lv.Subagents[i].Since < lv.Subagents[j].Since })
		lv.LastEventAt = ms(newest)
		if lv.Stale {
			v.Banners = append(v.Banners, fmt.Sprintf(
				"%s is busy per `claude agents` but no hook has arrived from it for %ds. The session probably predates the hook install: restart it to load ~/.claude/settings.json.",
				name, int(staleAfter.Seconds())))
		}
		if active {
			v.Lanes = append(v.Lanes, lv)
		} else {
			v.QuietWorktrees = append(v.QuietWorktrees, lv)
		}
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
		v.Feed = append(v.Feed, m.feed[i])
	}
	return v
}
