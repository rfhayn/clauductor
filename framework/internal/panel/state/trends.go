package state

import (
	"sort"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// PANEL-11: what the lane dashboard shows over time, and two cheap reads (ps, git)
// taken only while a page is open. Everything here is derived from inputs the panel
// already has: the status line, hooks, `claude agents`, the gate lease, ps and git.
// Nothing costs a model token and nothing reads a transcript.

// TrendWindow is how far back the trends and the lane-state timeline reach.
const TrendWindow = 2 * time.Hour

// trendStep is one point of a trend: a minute.
const trendStep = time.Minute

// AutocompactPct is where the page marks the context bar: Claude Code compacts on its
// own close to the end of the window. [inferred: not a documented threshold]
const AutocompactPct = 95

// series is a per-minute trend, oldest first, at most TrendWindow long.
type series struct {
	start time.Time // the minute of pts[0]
	pts   []float64
}

func (s *series) add(now time.Time, v float64) {
	at := now.Truncate(trendStep)
	switch {
	case len(s.pts) == 0:
		s.start, s.pts = at, []float64{v}
	default:
		last := s.start.Add(time.Duration(len(s.pts)-1) * trendStep)
		if !at.After(last) {
			s.pts[len(s.pts)-1] = v // the same minute: its latest value
			return
		}
		// Missing minutes repeat the last value: a trend has no holes.
		for t := last.Add(trendStep); t.Before(at); t = t.Add(trendStep) {
			s.pts = append(s.pts, s.pts[len(s.pts)-1])
		}
		s.pts = append(s.pts, v)
	}
	if max := int(TrendWindow / trendStep); len(s.pts) > max {
		drop := len(s.pts) - max
		s.pts = append([]float64(nil), s.pts[drop:]...)
		s.start = s.start.Add(time.Duration(drop) * trendStep)
	}
}

// at returns the value at or just before t, and whether the series reaches back that far.
func (s *series) at(t time.Time) (float64, bool) {
	if len(s.pts) == 0 || t.Before(s.start) {
		return 0, false
	}
	i := int(t.Sub(s.start) / trendStep)
	if i >= len(s.pts) {
		i = len(s.pts) - 1
	}
	return s.pts[i], true
}

func (s *series) last() (float64, bool) {
	if len(s.pts) == 0 {
		return 0, false
	}
	return s.pts[len(s.pts)-1], true
}

// Spark is a trend as the page draws it: one value a minute from Start (unix ms).
type Spark struct {
	Start int64     `json:"start"`
	Step  int64     `json:"step"` // ms
	V     []float64 `json:"v"`
}

func (s *series) spark() *Spark {
	if len(s.pts) < 2 {
		return nil
	}
	return &Spark{Start: ms(s.start), Step: trendStep.Milliseconds(), V: append([]float64(nil), s.pts...)}
}

// rate is the change per hour over the last `over`, from the series; ok is false
// when the series spans less than min.
func (s *series) rate(now time.Time, over, min time.Duration) (float64, bool) {
	last, ok := s.last()
	if !ok {
		return 0, false
	}
	from := now.Add(-over)
	if from.Before(s.start) {
		from = s.start
	}
	span := now.Truncate(trendStep).Sub(from.Truncate(trendStep))
	if span < min {
		return 0, false
	}
	first, _ := s.at(from)
	return (last - first) / span.Hours(), true
}

// Segment is one stretch of a lane's state on its timeline.
type Segment struct {
	State string `json:"state"` // busy | waiting | idle | none
	From  int64  `json:"from"`  // unix ms
}

type laneTrend struct {
	segs []Segment
	hit  series // the lane's prompt-cache hit ratio
	cost series // the lane's est. $ (its sessions' sum)
}

type trends struct {
	lastSample time.Time
	quota5     series
	cost       series
	cpu        series
	mem        series
	lanes      map[string]*laneTrend // by worktree path
	day        string                // the local day costToday counts
	costToday  float64
	costSeen   map[string]float64 // each session's cost when last counted
	procs      map[int]signals.Proc
	procsAt    time.Time
	procsErr   string
	git        map[string]*GitView // by worktree path
}

func (m *Model) tr() *trends {
	if m.trend == nil {
		m.trend = &trends{lanes: map[string]*laneTrend{}, costSeen: map[string]float64{}, git: map[string]*GitView{}}
	}
	return m.trend
}

// countCost adds a session's new status-line cost to today's total. A session first
// seen more than two minutes after the panel started is new, so all of its cost is
// today's; one already running when the panel started counts from then on.
func (m *Model) countCost(id string, v float64, now time.Time) {
	t := m.tr()
	if day := now.Local().Format("2006-01-02"); day != t.day {
		t.day, t.costToday = day, 0
	}
	prev, seen := t.costSeen[id]
	switch {
	case seen && v >= prev:
		t.costToday += v - prev
	case !seen && now.Sub(m.startedAt) > 2*time.Minute:
		t.costToday += v
	}
	t.costSeen[id] = v
}

// laneStatus is the lane's state from its sessions, as the view ranks them.
func (m *Model) laneStatuses(now time.Time) map[string]string {
	out := map[string]string{}
	for _, s := range m.sessions {
		if s.Lane == "" || !m.sessionActive(s, now) {
			continue
		}
		st, _, _ := m.sessionStatus(s, now)
		if statusRank[st] > statusRank[out[s.Lane]] {
			out[s.Lane] = st
		}
	}
	return out
}

// sessionActive is Snapshot's rule for a session that still counts.
func (m *Model) sessionActive(s *session, now time.Time) bool {
	heard := s.LastHookAt
	if s.StatusAt.After(heard) {
		heard = s.StatusAt
	}
	recent := !heard.IsZero() && now.Sub(heard) < activeWindow && s.HookStatus != "ended"
	return s.Agent != nil || recent || s.Note != nil
}

// Sample records the trends: every call extends the lane-state timelines (the
// runtime calls it every few seconds), and once a minute a point of each trend.
func (m *Model) Sample(now time.Time) {
	t := m.tr()
	states := m.laneStatuses(now)
	for _, wt := range m.worktrees {
		if wt.Bare {
			continue
		}
		st := states[wt.Path]
		if st == "" {
			st = "none"
		}
		lt := t.lanes[wt.Path]
		if lt == nil {
			if st == "none" {
				continue
			}
			lt = &laneTrend{}
			t.lanes[wt.Path] = lt
		}
		if n := len(lt.segs); n == 0 || lt.segs[n-1].State != st {
			lt.segs = append(lt.segs, Segment{State: st, From: ms(now)})
		}
		// Keep one segment that began before the window: it is the window's start.
		cut := ms(now.Add(-TrendWindow))
		for len(lt.segs) > 1 && lt.segs[1].From <= cut {
			lt.segs = lt.segs[1:]
		}
	}
	if !t.lastSample.IsZero() && now.Sub(t.lastSample) < trendStep && now.Truncate(trendStep).Equal(t.lastSample.Truncate(trendStep)) {
		return
	}
	t.lastSample = now
	if q := m.quotaAt(now); q != nil && q.FiveHour != nil {
		t.quota5.add(now, *q.FiveHour)
	}
	total := 0.0
	for _, c := range m.costByID {
		total += c
	}
	if len(m.costByID) > 0 {
		t.cost.add(now, total)
	}
	laneCost, laneHit, laneCtx := map[string]float64{}, map[string]float64{}, map[string]float64{}
	for id, s := range m.sessions {
		if s.Lane == "" || !m.sessionActive(s, now) {
			continue
		}
		laneCost[s.Lane] += m.costByID[id]
		if s.Stats.CacheHitRatio != nil && s.Stats.CtxPct != nil && *s.Stats.CtxPct >= laneCtx[s.Lane] {
			laneCtx[s.Lane], laneHit[s.Lane] = *s.Stats.CtxPct, *s.Stats.CacheHitRatio
		}
	}
	for path, lt := range t.lanes {
		if c, ok := laneCost[path]; ok {
			lt.cost.add(now, c)
		}
		if h, ok := laneHit[path]; ok {
			lt.hit.add(now, h)
		}
	}
	if t.procs != nil && now.Sub(t.procsAt) < 2*time.Minute {
		cpu, mem := 0.0, 0.0
		for _, p := range t.procs {
			cpu += p.CPU
			mem += float64(p.RSSKB) / 1024
		}
		t.cpu.add(now, cpu)
		t.mem.add(now, mem)
	}
}

// ClaudePIDs are the pids `claude agents` reported for the project's sessions.
func (m *Model) ClaudePIDs() []int {
	var out []int
	for _, s := range m.sessions {
		if s.Agent != nil && s.Agent.PID > 0 {
			out = append(out, s.Agent.PID)
		}
	}
	sort.Ints(out)
	return out
}

// ApplyProcs records a ps read of the claude processes.
func (m *Model) ApplyProcs(p map[int]signals.Proc, err error, now time.Time) {
	t := m.tr()
	if err != nil {
		t.procsErr = err.Error()
		return
	}
	t.procs, t.procsAt, t.procsErr = p, now, ""
}

func (m *Model) procOf(s *session, now time.Time) (cpu, rss *float64) {
	t := m.trend
	if t == nil || s.Agent == nil || now.Sub(t.procsAt) > 2*time.Minute {
		return nil, nil
	}
	p, ok := t.procs[s.Agent.PID]
	if !ok {
		return nil, nil
	}
	c, r := p.CPU, float64(p.RSSKB)/1024
	return &c, &r
}

// GitView is a lane's worktree as git reported it (PANEL-11).
type GitView struct {
	signals.GitStat
	Dirty int    `json:"dirty"`
	At    int64  `json:"at"`              // when it was read, unix ms
	Error string `json:"error,omitempty"` // the last read failed: shown, never as a clean tree
}

// LaneWorktrees are the worktrees a lane runs in: the ones the git read covers.
func (m *Model) LaneWorktrees(now time.Time) []string {
	seen := map[string]bool{}
	for _, tv := range m.TerminalViews(now) {
		if tv.Running && tv.Worktree != "" {
			seen[tv.Worktree] = true
		}
	}
	for _, s := range m.sessions {
		if s.Lane != "" && m.sessionActive(s, now) {
			seen[s.Lane] = true
		}
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// GitHead returns the last HEAD and commit time recorded for a worktree, so the
// runtime reads a commit's time only when HEAD moves.
func (m *Model) GitHead(path string) (string, int64) {
	if m.trend == nil || m.trend.git[path] == nil {
		return "", 0
	}
	g := m.trend.git[path]
	return g.Head, g.LastCommitAt
}

// ApplyGit records one worktree's git read. A failed read keeps the last good one
// and says it failed.
func (m *Model) ApplyGit(path string, g signals.GitStat, err error, now time.Time) {
	t := m.tr()
	if err != nil {
		if old := t.git[path]; old != nil {
			old.Error = err.Error()
			return
		}
		t.git[path] = &GitView{At: ms(now), Error: err.Error()}
		return
	}
	t.git[path] = &GitView{GitStat: g, Dirty: g.Dirty(), At: ms(now)}
}

// Trends is the view's global trends and the figures derived from them.
type Trends struct {
	Quota5 *Spark `json:"quota5,omitempty"`
	Cost   *Spark `json:"cost,omitempty"`
	CPU    *Spark `json:"cpuSpark,omitempty"`
	Mem    *Spark `json:"memSpark,omitempty"`
	// BurnPerH is the 5-hour quota's change per hour over the last 30 min (%/h);
	// ExhaustAt is when it reaches 100% at that rate, and BeforeReset whether that
	// comes before the window resets. Absent with under 5 min of data.
	BurnPerH    *float64 `json:"burnPerH,omitempty"`
	ExhaustAt   int64    `json:"exhaustAt,omitempty"`
	BeforeReset bool     `json:"beforeReset,omitempty"`
	CostToday   *float64 `json:"costToday,omitempty"`
	CostPerH    *float64 `json:"costPerH,omitempty"` // over the last hour
	CPUPct      *float64 `json:"cpu,omitempty"`      // the claude processes, % of one core
	MemMB       *float64 `json:"memMb,omitempty"`
	ProcsError  string   `json:"procsError,omitempty"`
	// AutocompactPct is where the context bar is marked. [inferred]
	AutocompactPct float64 `json:"autocompactPct"`
}

// trendsView fills the view's trends and each lane's timeline, sparks and git.
func (m *Model) trendsView(v *View, now time.Time) {
	t := m.tr()
	tr := Trends{Quota5: t.quota5.spark(), Cost: t.cost.spark(), CPU: t.cpu.spark(), Mem: t.mem.spark(), ProcsError: t.procsErr, AutocompactPct: AutocompactPct}
	if r, ok := t.quota5.rate(now, 30*time.Minute, 5*time.Minute); ok {
		tr.BurnPerH = &r
		if last, _ := t.quota5.last(); r > 0 && last < 100 {
			at := now.Add(time.Duration((100 - last) / r * float64(time.Hour)))
			tr.ExhaustAt = ms(at)
			if q := v.Quota; q != nil && q.FiveHourResets != nil {
				tr.BeforeReset = ms(at) < *q.FiveHourResets*1000
			}
		}
	}
	if t.day != "" {
		c := t.costToday
		tr.CostToday = &c
	}
	if r, ok := t.cost.rate(now, time.Hour, 5*time.Minute); ok {
		r = max(r, 0)
		tr.CostPerH = &r
	}
	if t.procs != nil && now.Sub(t.procsAt) < 2*time.Minute {
		cpu, mem := 0.0, 0.0
		for _, p := range t.procs {
			cpu += p.CPU
			mem += float64(p.RSSKB) / 1024
		}
		tr.CPUPct, tr.MemMB = &cpu, &mem
	}
	v.Trends = tr
	fill := func(l *LaneView) {
		if lt := t.lanes[l.Path]; lt != nil {
			l.Timeline = append([]Segment(nil), lt.segs...)
			l.HitSpark = lt.hit.spark()
			l.CostSpark = lt.cost.spark()
			if r, ok := lt.cost.rate(now, time.Hour, 5*time.Minute); ok {
				r = max(r, 0)
				l.CostPerH = &r
			}
		}
		if g := t.git[l.Path]; g != nil {
			c := *g
			l.Git = &c
		}
	}
	for i := range v.Lanes {
		fill(&v.Lanes[i])
	}
	for i := range v.QuietWorktrees {
		fill(&v.QuietWorktrees[i])
	}
}
