package panel

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

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

// TrustView says whether the config's argv may run (see trust.go).
type TrustView struct {
	Trusted bool   `json:"trusted"`
	Hash    string `json:"hash"`
	Prev    string `json:"prev,omitempty"`
	Path    string `json:"path"`
	Note    string `json:"note,omitempty"`
}

type modelV2 struct {
	droppedUnknown int
	unknownNotifs  int
	statusVersion  string
	claudeVersion  string
	versionSrc     SourceStatus
	obs            Obs
	queues         []QueueView
	queuesSrc      SourceStatus
	notifier       NotifierStats
	trust          TrustView
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

// ViewV2 is the v2 part of the View.
type ViewV2 struct {
	// Done holds finished turns and completed agents: your move, but not blocked.
	Done       []NeedView     `json:"done"`
	Alerts     []AlertView    `json:"alerts"`
	Templates  []TemplateInfo `json:"templates"`
	Queues     []QueueView    `json:"queues"`
	QueuesSrc  SourceStatus   `json:"queuesSource"`
	Thresholds Thresholds     `json:"thresholds"`
	// QuotaGuard is set when the 5-hour quota is at or above the guard: a new lane
	// needs the override.
	QuotaGuard string    `json:"quotaGuard,omitempty"`
	Restorable []string  `json:"restorable"`
	Observe    ObsView   `json:"observe"`
	Trust      TrustView `json:"trust"`
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
func (m *Model) ApplyTrust(t TrustView) { m.v2.trust = t }

// ApplyQueues records a read of the queues' leases.
func (m *Model) ApplyQueues(qs []QueueView, err error, now time.Time) {
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
func (m *Model) needsFor(v *View, wt Worktree, name, term string, s *session, st string, now time.Time) {
	if b, ok := m.blocked(s, now); ok {
		v.NeedsYou = append(v.NeedsYou, NeedView{Lane: wt.Path, Name: name, Session: s.ID, Kind: b.Kind,
			Label: approxLabel(b.Label, b), Severity: b.Severity, Text: b.Text, At: ms(b.Since), Terminal: term, Approx: b.Approx})
	}
	// A note that needs you without blocking (quota auto-resume will not fire).
	if s.Note != nil {
		if k := ClassifyNotification(s.Note.Type); k.NeedsYou && !k.Waiting {
			v.NeedsYou = append(v.NeedsYou, NeedView{Lane: wt.Path, Name: name, Session: s.ID, Kind: s.Note.Type,
				Label: k.Label, Severity: k.Severity, Text: s.Note.Message, At: ms(s.Note.At), Terminal: term})
		}
	}
	if s.Done != nil && st != "busy" && st != "waiting" {
		k := ClassifyNotification(s.Done.Type)
		v.Done = append(v.Done, NeedView{Lane: wt.Path, Name: name, Session: s.ID, Kind: s.Done.Type,
			Label: k.Label, Severity: SevInfo, Text: s.Done.Message, At: ms(s.Done.At), Terminal: term})
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
}

// PromptDecision is what to do about a template lane's first prompt.
type PromptDecision struct {
	Action string // none | wait | send | skip | stuck | delivered
	Why    string
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
			return PromptDecision{"skip", "a prompt was typed in the lane first"}
		}
		if !in.Running {
			return PromptDecision{"wait", "the lane is not running"}
		}
		if in.Dead {
			return PromptDecision{"stuck", "claude exited before its first prompt was typed; RESTART the lane"}
		}
		if in.Listed && in.Status == "idle" && in.WaitingFor == "" && !in.WaitingNote && in.PollFresh {
			return PromptDecision{"send", ""}
		}
		if in.Listed && in.Status == "idle" && !in.PollFresh {
			return PromptDecision{"wait", "waiting for a fresh `claude agents` poll"}
		}
		if now.Sub(in.Since) >= readyGrace {
			return PromptDecision{"stuck", fmt.Sprintf("claude is not ready after %ds. Answer any dialog in its terminal "+
				"(workspace trust defaults to \"No, exit\": press ↓ then Enter). The first prompt is typed as soon as claude is idle.",
				int(now.Sub(in.Since).Seconds()))}
		}
		return PromptDecision{"wait", "waiting for claude to be ready"}
	case "typing":
		return PromptDecision{"stuck", "the panel stopped while typing the first prompt. Check the terminal: the panel never types it twice."}
	case "sent":
		if !in.Prompted.IsZero() && !in.Prompted.Before(in.PromptAt.Add(-time.Second)) || (in.Listed && in.Status == "busy") {
			return PromptDecision{"delivered", ""}
		}
		if now.Sub(in.PromptAt) >= confirmGrace {
			return PromptDecision{"stuck", "the first prompt was typed but claude has not started on it. Check the terminal: the text may still sit in the input box."}
		}
		return PromptDecision{"wait", "typed; waiting for claude to start"}
	}
	return PromptDecision{"none", ""}
}

// PromptDecisions returns the decision for every template lane with a prompt in
// flight, keyed by lane id.
func (m *Model) PromptDecisions(now time.Time) map[string]PromptDecision {
	out := map[string]PromptDecision{}
	tmux := map[string]TmuxLane{}
	for _, tl := range m.tmuxLanes {
		tmux[tl.ID] = tl
	}
	for _, rec := range m.laneRecords {
		if rec.PromptState == "" || rec.PromptState == "delivered" || rec.PromptState == "skipped" {
			continue
		}
		tl, running := tmux[rec.ID]
		in := PromptInput{State: rec.PromptState, Conversation: rec.Conversation, Running: running, Dead: tl.Dead,
			Since: time.UnixMilli(rec.ActionAt), PromptAt: time.UnixMilli(rec.PromptAt), PollFresh: m.agentsFresh(now)}
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

// agentsFresh: the last `claude agents` poll succeeded within two poll intervals,
// so "idle" is a current reading, not the last word of a poll that has since failed.
func (m *Model) agentsFresh(now time.Time) bool {
	iv := time.Duration(m.v2.obs.AgentsInterval) * time.Millisecond
	if iv < agentsFast {
		iv = agentsSlow // not measured yet: allow the slow interval
	}
	return m.agentsSrc.OK && m.agentsSrc.At > 0 && now.Sub(time.UnixMilli(m.agentsSrc.At)) <= 2*iv
}

// restoredPending reports whether a restored lane still waits for you: it was
// resumed on a conversation and nothing has happened in it since.
func (m *Model) restoredPending(rec LaneRecord) bool {
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
	v.Queues = append([]QueueView{}, m.v2.queues...)
	v.QueuesSrc = m.v2.queuesSrc
	if len(m.cfg.Queues) == 0 {
		v.QueuesSrc = SourceStatus{OK: true}
	}
	v.QuotaGuard = QuotaGuardBlock(v.Quota, th.GuardPct)
	v.Trust = m.v2.trust
	v.Restorable = []string{}
	v.Warnings = []string{}
	if v.Done == nil {
		v.Done = []NeedView{}
	}

	recs := map[string]LaneRecord{}
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
					Kind: "first_prompt", Label: "First prompt (" + rec.Template + ")", Severity: SevBlock, Text: d.Why,
					At: rec.ActionAt, Terminal: tv.ID})
			}
		}
		if tv.Running && m.restoredPending(rec) {
			v.NeedsYou = append(v.NeedsYou, NeedView{Lane: tv.Worktree, Name: tv.ID, Session: rec.SessionID, Kind: "restored",
				Label: "Restored lane", Severity: SevWarn, At: rec.Restored, Terminal: tv.ID,
				Text: "Resumed with claude --resume after its tmux session was lost. If the session was idle over 1 h and above " +
					"100k tokens, claude first asks whether to resume from a summary: answer it in the terminal. The panel " +
					"types nothing into a restored lane, so continue it yourself."})
		}
	}
	sort.SliceStable(v.NeedsYou, func(i, j int) bool {
		return sevRank(v.NeedsYou[i].Severity) > sevRank(v.NeedsYou[j].Severity)
	})
	if len(v.Restorable) > 0 {
		v.Banners = append(v.Banners, fmt.Sprintf("%d lane(s) lost their tmux session (a reboot, or the tmux server ended): %s. "+
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
		v.Banners = append(v.Banners, fmt.Sprintf("%d hook or status-line event(s) were dropped because the panel fell behind. "+
			"Lane states may lag until the next `claude agents` poll.", m.v2.obs.OverflowDrops))
	}
	if !m.v2.trust.Trusted && m.v2.trust.Hash != "" {
		v.Banners = append(v.Banners, "panel.json changed since you trusted it ("+short(m.v2.trust.Prev)+" → "+short(m.v2.trust.Hash)+
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

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	if h == "" {
		return "none"
	}
	return h
}

func sevRank(s string) int {
	return map[string]int{SevBlock: 3, SevWarn: 2, SevInfo: 1}[s]
}

// compactionTrigger reads a compaction event's trigger (manual | auto).
func compactionTrigger(ev HookEvent) string {
	t := ev.CompactionTrigger
	if t == "" {
		t = ev.Trigger
	}
	if t == "" {
		return "compaction"
	}
	return oneLine(t)
}
