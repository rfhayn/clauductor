package panel

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// withTemplate copies a rendered template's launch options and first prompt into the
// record written before the lane starts, so a restart or a panel restart keeps them.
func (req StartRequest) withTemplate(rec LaneRecord) LaneRecord {
	if req.tpl == nil {
		return rec
	}
	rec.Template, rec.Model, rec.Effort = req.tpl.Template, req.tpl.Model, req.tpl.Effort
	rec.FirstPrompt, rec.PromptState = req.tpl.FirstPrompt, "pending"
	return rec
}

// launchOptions are a lane's model and effort: its lane type's, overridden by the
// template it was started from (recorded in the registry).
func (m *LaneManager) launchOptions(id, laneType string) LaneTypeConfig {
	lt := m.Cfg.LaneTypes[laneType]
	if m.Registry != nil {
		if rec, ok := m.Registry.Get(id); ok {
			if rec.Model != "" {
				lt.Model = rec.Model
			}
			if rec.Effort != "" {
				lt.Effort = rec.Effort
			}
		}
	}
	return lt
}

// StartGate holds what StartLane checks besides the request itself: whether the
// config is trusted (templates are repo-controlled prompts) and the quota guard.
type StartGate struct {
	Trusted    func() bool
	QuotaGuard func() string // why the quota refuses a new lane now, or ""
}

// StartLane is the v2 start: an optional template, then the quota guard, then Start.
func (m *LaneManager) StartLane(ctx context.Context, req StartRequest, g StartGate) (StartResult, *LaneError) {
	if req.Template != "" {
		if g.Trusted != nil && !g.Trusted() {
			return StartResult{}, laneErr(409, "untrusted-config", "panel.json changed since you trusted it, so its templates are off; run `clauductor panel trust`")
		}
		r, err := m.Cfg.RenderTemplate(req.Template, req.Name, req.Issue)
		if err != nil {
			return StartResult{}, laneErr(400, "invalid", "%v", err)
		}
		if req.Type != "" && req.Type != r.LaneType {
			return StartResult{}, laneErr(400, "invalid", "template %q starts a %s lane, not %s", req.Template, r.LaneType, req.Type)
		}
		if req.Mode != "" && req.Mode != "new" {
			return StartResult{}, laneErr(400, "invalid", "a template lane starts on a new branch and worktree")
		}
		req.Type, req.Mode, req.tpl = r.LaneType, "new", &r
	} else if req.Issue != "" {
		return StartResult{}, laneErr(400, "invalid", "issue is only for templates")
	}
	if g.QuotaGuard != nil && !req.OverrideQuota {
		if why := g.QuotaGuard(); why != "" {
			return StartResult{}, laneErr(409, "quota", "%s. Tick the override to start it anyway.", why)
		}
	}
	return m.Start(ctx, req)
}

// SetPromptState moves a template lane's first prompt to a new state.
func (m *LaneManager) SetPromptState(id, from, to string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Registry.Update(id, func(r *LaneRecord) bool {
		if r.PromptState != from {
			return false
		}
		r.PromptState = to
		return true
	})
}

// DeliverFirstPrompt types a template lane's first prompt: the text, then Enter as a
// separate write. It holds the lane lock, so no start, stop or restart interleaves,
// and it writes "typing" to the registry BEFORE the first keystroke: a panel that
// dies mid-typing leaves "typing", which is never typed again. The caller has
// already decided, from `claude agents`, that claude is idle.
func (m *LaneManager) DeliverFirstPrompt(ctx context.Context, id string, stillReady func() string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.Registry.Get(id)
	if !ok || rec.PromptState != "pending" || rec.FirstPrompt == "" {
		return fmt.Errorf("lane %q has no pending first prompt", id)
	}
	if l, ok := m.find(ctx, id); !ok || l.Dead {
		return fmt.Errorf("lane %q is not running", id)
	}
	// Re-checked under the lane lock, right before the first keystroke: the model's
	// view (a waiting note from a hook), then a fresh `claude agents` read.
	if stillReady != nil {
		if why := stillReady(); why != "" {
			return fmt.Errorf("not typed: %s", why)
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	out, err := m.Run(cctx, m.Root, []string{"claude", "agents", "--json"})
	cancel()
	var agents []signals.Agent
	if err == nil {
		agents, err = signals.ParseAgents(out)
	}
	if err != nil {
		return fmt.Errorf("not typed: cannot read claude agents: %v", err)
	}
	if why := AgentReady(agents, rec.SessionID); why != "" {
		return fmt.Errorf("not typed: %s", why)
	}
	// Re-validated at the moment of typing: a registry edited by hand must not be
	// able to smuggle a newline or an escape sequence into the lane.
	if err := typableText(rec.FirstPrompt, maxFirstPrompt); err != nil {
		_ = m.Registry.Update(id, func(r *LaneRecord) bool { r.PromptState = "skipped"; return true })
		return fmt.Errorf("first prompt %v; not typed", err)
	}
	if err := m.Registry.Update(id, func(r *LaneRecord) bool {
		if r.PromptState != "pending" {
			return false
		}
		r.PromptState, r.PromptAt = "typing", m.now().UnixMilli()
		return true
	}); err != nil {
		return err
	}
	if err := m.sendText(ctx, id, rec.FirstPrompt); err != nil {
		return err // stays "typing": shown in Needs you, never retyped
	}
	return m.Registry.Update(id, func(r *LaneRecord) bool {
		r.PromptState, r.PromptAt = "sent", m.now().UnixMilli()
		return true
	})
}

// AgentReady says why a session is not ready for typed text, or "" when it is:
// listed, idle, and waiting for nothing.
func AgentReady(agents []signals.Agent, sessionID string) string {
	for _, a := range agents {
		if a.SessionID != sessionID {
			continue
		}
		switch {
		case a.Status != "idle":
			return "claude is " + a.Status
		case a.WaitingFor != "":
			return "claude is waiting for " + signals.OneLine(a.WaitingFor)
		}
		return ""
	}
	return "the session is not in claude agents"
}

// RestoreSkip is a lane RestoreAll did not restore, and why.
type RestoreSkip struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// SelectRestorable picks the registered lanes to restore: those whose tmux session
// is gone. It never picks a session twice: not one that a claude process already
// runs (`live`), and not the same session id for two lanes. Two processes on one
// session interleave its transcript.
func SelectRestorable(recs []LaneRecord, running map[string]bool, live map[string]bool, dirOK func(string) bool) ([]LaneRecord, []RestoreSkip) {
	sort.Slice(recs, func(i, j int) bool { return recs[i].ID < recs[j].ID })
	var out []LaneRecord
	var skip []RestoreSkip
	seen := map[string]string{}
	for _, r := range recs {
		switch {
		case running[r.ID]:
			continue // not lost
		case r.Corrupt != "":
			skip = append(skip, RestoreSkip{r.ID, "corrupt registry record (" + r.Corrupt + "): never launched"})
		case !uuidRe.MatchString(r.SessionID):
			skip = append(skip, RestoreSkip{r.ID, "no valid session id"})
		case live[r.SessionID]:
			skip = append(skip, RestoreSkip{r.ID, "session " + r.SessionID + " already runs in another claude process"})
		case seen[r.SessionID] != "":
			skip = append(skip, RestoreSkip{r.ID, "session " + r.SessionID + " is also lane " + seen[r.SessionID] + "'s; restored once only"})
		case !dirOK(r.Path):
			skip = append(skip, RestoreSkip{r.ID, r.Path + " no longer exists; forget the lane"})
		default:
			seen[r.SessionID] = r.ID
			out = append(out, r)
		}
	}
	return out, skip
}

// liveSessions reads the session ids claude processes run now.
func (m *LaneManager) liveSessions(ctx context.Context) (map[string]bool, error) {
	out, err := m.Run(ctx, m.Root, []string{"claude", "agents", "--json"})
	if err != nil {
		return nil, err
	}
	agents, err := signals.ParseAgents(out)
	if err != nil {
		return nil, err
	}
	live := map[string]bool{}
	for _, a := range agents {
		live[a.SessionID] = true
	}
	return live, nil
}

// RestoreResult reports a restore.
type RestoreResult struct {
	Restored []string      `json:"restored"`
	Skipped  []RestoreSkip `json:"skipped"`
}

// RestoreAll resumes every lane a reboot (or a dead tmux server) took away, each on
// its own session id: `claude --resume <id>`, never --continue. Nothing is typed into
// a restored lane: claude may first show its resume-from-summary dialog, which "Needs
// you" points at.
func (m *LaneManager) RestoreAll(ctx context.Context, quotaGuard func() string, override bool) (RestoreResult, *LaneError) {
	res := RestoreResult{Restored: []string{}, Skipped: []RestoreSkip{}}
	if why := m.StartBlocked(ctx); why != "" {
		return res, laneErr(409, "api-key", "%s", why)
	}
	if quotaGuard != nil && !override {
		if why := quotaGuard(); why != "" {
			return res, laneErr(409, "quota", "%s. Tick the override to restore anyway.", why)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	lanes, err := m.List(ctx)
	if err != nil {
		return res, laneErr(500, "tmux", "%v", err)
	}
	running := map[string]bool{}
	for _, l := range lanes {
		running[l.ID] = true
	}
	live, err := m.liveSessions(ctx)
	if err != nil {
		return res, laneErr(409, "unverified", "cannot read claude agents (%v), so no session can be shown not to be running already; nothing restored", err)
	}
	pick, skip := SelectRestorable(m.Registry.List(), running, live, func(p string) bool {
		fi, err := os.Stat(p)
		return err == nil && fi.IsDir()
	})
	res.Skipped = append(res.Skipped, skip...)
	for _, rec := range pick {
		if lerr := m.resumeLocked(ctx, rec, "restore"); lerr != nil {
			res.Skipped = append(res.Skipped, RestoreSkip{rec.ID, lerr.Msg})
			continue
		}
		m.markRestored(rec.ID)
		res.Restored = append(res.Restored, rec.ID)
	}
	return res, nil
}

// markRestored records when a lane was restored, for "Needs you".
func (m *LaneManager) markRestored(id string) {
	_ = m.Registry.Update(id, func(r *LaneRecord) bool { r.Restored = m.now().UnixMilli(); return true })
}
