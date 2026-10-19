package state

import (
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// PANEL-49 group 2: an attached project's sessions measured from their transcripts.
// The readings are the real reader's (signals.ReadTranscript) over hand-written lines
// in a temporary config directory, which hold no real transcript content.

const (
	trSID  = "11111111-1111-4111-8111-111111111111"
	trSID2 = "22222222-2222-4222-8222-222222222222"
)

// trProject is an attached project's model: alertModel's /repo/w/x worktree, measured
// from transcripts, with a temporary config directory for its transcripts.
type trProject struct {
	t         *testing.T
	m         *Model
	configDir string
	curs      map[string]*signals.TranscriptCursor
}

func newTrProject(t *testing.T, cfgExtra string) *trProject {
	t.Helper()
	m := alertModel(t, cfgExtra)
	m.MeasureFromTranscripts()
	return &trProject{t: t, m: m, configDir: t.TempDir(), curs: map[string]*signals.TranscriptCursor{}}
}

// lanes registers tmux lanes from recs, each in /repo/w/x.
func (p *trProject) lanes(recs ...types.LaneRecord) {
	var tl []types.TmuxLane
	for _, r := range recs {
		tl = append(tl, types.TmuxLane{ID: r.ID, Path: r.Path})
	}
	p.m.ApplyTmux(tl, recs, "", nil, t0)
}

func (p *trProject) hook(sid, event string, at time.Time) {
	p.m.ApplyHook(signals.HookEvent{SessionID: sid, Cwd: "/repo/w/x", Event: event, Prompt: "go"}, at)
}

// path is a file under the config directory's projects/: the session's own transcript
// for "", else a subagent's under its subagents/ folder.
func (p *trProject) path(sid, sub string) string {
	dir := filepath.Join(p.configDir, "projects", "-repo-w-x")
	if sub != "" {
		return filepath.Join(dir, sid, "subagents", sub+".jsonl")
	}
	return filepath.Join(dir, sid+".jsonl")
}

func (p *trProject) write(sid, sub, s string) {
	p.t.Helper()
	path := p.path(sid, sub)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		p.t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		p.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		p.t.Fatal(err)
	}
}

// read reads session sid's transcripts with its cursor, as the runtime does, and
// applies the reading taken at at.
func (p *trProject) read(sid string, at time.Time) signals.TranscriptReading {
	p.t.Helper()
	cur := p.curs[sid]
	if cur == nil {
		cur = &signals.TranscriptCursor{}
		p.curs[sid] = cur
	}
	r, err := signals.ReadTranscript(p.configDir, sid, cur)
	p.m.ApplyTranscript(sid, r, err, at)
	return r
}

func (p *trProject) post(raw string, at time.Time) {
	p.t.Helper()
	sp, err := signals.ParseStatus([]byte(raw))
	if err != nil {
		p.t.Fatal(err)
	}
	p.m.ApplyStatus(sp, at)
}

// opus is one hand-written assistant line on claude-opus-5-5 ($4 input, $20 output,
// $5 a 5-minute write, $0.20 a cache read, per million tokens).
func opus(id string, in, out, cw, cr int) string {
	return assistantLine("claude-opus-5-5", id, in, out, cw, cr)
}

func assistantLine(model, id string, in, out, cw, cr int) string {
	return `{"type":"assistant","version":"2.1.302","message":{"id":"` + id + `","model":"` + model +
		`","content":[{"type":"text","text":"placeholder"}],"usage":{"input_tokens":` + strconv.Itoa(in) +
		`,"output_tokens":` + strconv.Itoa(out) + `,"cache_creation_input_tokens":` + strconv.Itoa(cw) +
		`,"cache_read_input_tokens":` + strconv.Itoa(cr) + "}}}\n"
}

const userLine = `{"type":"user","version":"2.1.302","message":{"role":"user","content":"placeholder prompt"}}` + "\n"

func approx(p *float64, want float64) bool { return p != nil && math.Abs(*p-want) < 1e-9 }

func fmtF(p *float64) string {
	if p == nil {
		return "absent"
	}
	return strconv.FormatFloat(*p, 'f', -1, 64)
}

// sessionView is session sid's view on lane /repo/w/x, and that lane.
func sessionView(t *testing.T, v View, sid string) (SessionView, LaneView) {
	t.Helper()
	for _, lv := range v.Lanes {
		for _, sv := range lv.Sessions {
			if sv.ID == sid {
				return sv, lv
			}
		}
	}
	t.Fatalf("no session %s in the view's lanes: %+v", sid, v.Lanes)
	return SessionView{}, LaneView{}
}

func warningsWith(v View, prefix string) []WarningView {
	var out []WarningView
	for _, w := range v.WarningItems {
		if strings.HasPrefix(w.Key, prefix) {
			out = append(out, w)
		}
	}
	return out
}

// [MEASURES-1-S1] An attached lane shows its context, cost and alert: under
// alerts.context_pct 85, a lane launched with a 200000 window whose last main-thread
// claude-opus-5-5 record sums to 160000 tokens, and one subagent transcript. Once both
// files are read to their end, the lane's context reads 80% (the subagent's larger
// call does not count toward it), its threshold is 78.5, the context alert fires at the
// reading's time, the est. $ and the lane metrics' costUsd are the priced sum of every
// call in both files with a streamed message counted once, and the model reads
// claude-opus-5-5.
func TestMEASURES1S1AttachedLaneShowsContextCostAndAlert(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, `,"alerts":{"context_pct":85}`)
	p.lanes(windowRec("x", trSID, "/repo/w/x", 200000))
	at := t0.Add(10 * time.Second)
	p.hook(trSID, "UserPromptSubmit", t0)
	// msg_A streams twice; only its last line counts: $0.004 + $0.010 + $0.010 + $0.020.
	p.write(trSID, "", userLine+opus("msg_A", 10, 1, 0, 0)+opus("msg_A", 1000, 500, 2000, 100000)+
		// msg_B, the last main-thread call: 2000 + 8000 + 150000 = 160000 tokens of context;
		// $0.008 + $0.020 + $0.040 + $0.030.
		opus("msg_B", 2000, 1000, 8000, 150000))
	// The subagent's call: $0.002 + $0.002 + $0.038, and 190500 tokens it does not add to
	// the session's context.
	p.write(trSID, "agent-a", opus("msg_S", 500, 100, 0, 190000))
	if r := p.read(trSID, at); !r.AtEnd || r.Failed != nil {
		t.Fatalf("reading %+v, want both files read to their end", r)
	}
	const want = 0.044 + 0.098 + 0.042

	v := p.m.Snapshot(at)
	c := readCtx(t, v, "/repo/w/x")
	if c.session != 80 || c.lane != 80 || c.metrics != 80 || c.term != 80 || c.window != 200000 {
		t.Fatalf("context (session, lane, metrics, terminal) %v / %v / %v / %v against %d, want 80 against 200000",
			c.session, c.lane, c.metrics, c.term, c.window)
	}
	if got := alertPctOf(c); got != [3]float64{78.5, 78.5, 78.5} {
		t.Fatalf("threshold (lane, metrics, terminal) %v, want 78.5 each", got)
	}
	a, ok := contextAlert(v)
	if !ok || a.Text != "context at 80% (alert at 78.5%)" || a.Since != ms(at) {
		t.Fatalf("context alert %+v (fired %v), want \"context at 80%% (alert at 78.5%%)\" since the reading at %d", a, ok, ms(at))
	}
	sv, lv := sessionView(t, v, trSID)
	if !approx(sv.EstCostUSD, want) || !approx(sv.Metrics.CostUSD, want) || !approx(lv.Metrics.CostUSD, want) || !approx(v.EstCostUSD, want) {
		t.Fatalf("est. $ (session, its metrics, the lane's metrics, the project) %s / %s / %s / %s, want %v each",
			fmtF(sv.EstCostUSD), fmtF(sv.Metrics.CostUSD), fmtF(lv.Metrics.CostUSD), fmtF(v.EstCostUSD), want)
	}
	if sv.Model != "claude-opus-5-5" {
		t.Fatalf("model %q, want the reading's id claude-opus-5-5", sv.Model)
	}
}

// A transcript reading sets no figure while it has not reached the end of its files
// (D6), and leaves the status-line counters alone: statusPosts, the session's StatusAt
// and the quota's no-window count. In a project not measured from transcripts, a
// reading is ignored.
func TestTranscriptReadingLeavesPartialsAndStatusCountersAlone(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.lanes(windowRec("x", trSID, "/repo/w/x", 200000))
	p.post(`{"session_id":"`+trSID+`","cwd":"/repo/w/x","model":{"display_name":"Opus"}}`, t0)
	posts, noWindow, statusAt := p.m.statusPosts, p.m.noWindowPosts, p.m.sessions[trSID].StatusAt
	if posts != 1 || noWindow != 1 {
		t.Fatalf("after one post without rate_limits: statusPosts %d, noWindowPosts %d, want 1 and 1", posts, noWindow)
	}

	cost := 1.0
	partial := signals.TranscriptReading{Found: true, AtEnd: false, Model: "claude-opus-5-5", CostUSD: &cost, PricedCalls: 1,
		Context: &signals.TranscriptContext{CacheReadTokens: 100000}}
	p.m.ApplyTranscript(trSID, partial, nil, t0.Add(10*time.Second))
	s := p.m.sessions[trSID]
	if s.CtxPct != nil || s.Stats.CostUSD != nil || s.Model != "" || len(p.m.costByID) != 0 {
		t.Fatalf("a partial reading set figures: context %s, cost %s, model %q, costByID %v", fmtF(s.CtxPct), fmtF(s.Stats.CostUSD), s.Model, p.m.costByID)
	}

	whole := partial
	whole.AtEnd = true
	p.m.ApplyTranscript(trSID, whole, nil, t0.Add(20*time.Second))
	if !approx(s.CtxPct, 50) || !approx(s.Stats.CostUSD, 1) || s.Model != "claude-opus-5-5" {
		t.Fatalf("a reading at the end: context %s, cost %s, model %q, want 50, 1 and claude-opus-5-5", fmtF(s.CtxPct), fmtF(s.Stats.CostUSD), s.Model)
	}
	if p.m.statusPosts != posts || p.m.noWindowPosts != noWindow || !s.StatusAt.Equal(statusAt) {
		t.Fatalf("a reading moved the status counters: statusPosts %d, noWindowPosts %d, StatusAt %v; want %d, %d, %v",
			p.m.statusPosts, p.m.noWindowPosts, s.StatusAt, posts, noWindow, statusAt)
	}

	inRepo := alertModel(t, "")
	inRepo.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x"}}, []types.LaneRecord{windowRec("x", trSID, "/repo/w/x", 200000)}, "", nil, t0)
	inRepo.ApplyHook(signals.HookEvent{SessionID: trSID, Cwd: "/repo/w/x", Event: "UserPromptSubmit"}, t0)
	inRepo.ApplyTranscript(trSID, whole, nil, t0.Add(10*time.Second))
	if s := inRepo.sessions[trSID]; s.CtxPct != nil || s.Stats.CostUSD != nil || len(inRepo.costByID) != 0 {
		t.Fatalf("a project not measured from transcripts took a reading: context %s, cost %s", fmtF(s.CtxPct), fmtF(s.Stats.CostUSD))
	}
	if _, ok := inRepo.Snapshot(t0).Sources["transcripts"]; ok {
		t.Fatal("a project not measured from transcripts shows a transcripts source")
	}
}

// [MEASURES-3-S4] A transcript's cost reaches the ledger as a status post's does: a
// claude-opus-5-5 session read at $1.20, then $0.30 more appended and read. The
// project's est. $, cost today and the lane metrics' costUsd rise by $0.30, and the
// spend ledger's observation for the session rises by $0.30 under the lane's type, the
// model claude-opus-5-5 and the branch.
func TestMEASURES3S4TranscriptCostReachesTheLedger(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.lanes(windowRec("x", trSID, "/repo/w/x", 200000))
	// First seen more than two minutes after the panel started: all of its cost is today's.
	at := t0.Add(3 * time.Minute)
	p.hook(trSID, "UserPromptSubmit", at)
	p.write(trSID, "", opus("m1", 0, 60000, 0, 0)) // 60000 output tokens: $1.20
	p.read(trSID, at)
	type figures struct{ est, today, lane *float64 }
	snap := func(at time.Time) figures {
		v := p.m.Snapshot(at)
		_, lv := sessionView(t, v, trSID)
		return figures{v.EstCostUSD, v.Trends.CostToday, lv.Metrics.CostUSD}
	}
	before := snap(at)
	if !approx(before.est, 1.2) || !approx(before.today, 1.2) || !approx(before.lane, 1.2) {
		t.Fatalf("read at $1.20: est. $ %s, cost today %s, lane costUsd %s", fmtF(before.est), fmtF(before.today), fmtF(before.lane))
	}
	obs := p.m.DrainSpend()
	if len(obs) != 1 || math.Abs(obs[0].TotalUSD-1.2) > 1e-9 {
		t.Fatalf("spend observations %+v, want one at $1.20", obs)
	}

	at2 := at.Add(10 * time.Second)
	p.write(trSID, "", opus("m2", 0, 15000, 0, 0)) // $0.30 more
	p.read(trSID, at2)
	after := snap(at2)
	if !approx(after.est, 1.5) || !approx(after.today, 1.5) || !approx(after.lane, 1.5) {
		t.Fatalf("$0.30 more: est. $ %s, cost today %s, lane costUsd %s, want each $1.50", fmtF(after.est), fmtF(after.today), fmtF(after.lane))
	}
	obs = p.m.DrainSpend()
	if len(obs) != 1 {
		t.Fatalf("spend observations %+v, want one", obs)
	}
	o := obs[0]
	if math.Abs(o.TotalUSD-1.5) > 1e-9 || o.Session != trSID || o.Model != "claude-opus-5-5" || o.Branch != "change/x" || o.Type != "build" {
		t.Fatalf("spend observation %+v, want $1.50 (0.30 more) for %s, model claude-opus-5-5, branch change/x, type build", o, trSID)
	}
}

// [MEASURES-1-S3] A status post in an attached project moves the quota, not a windowed
// session's context or cost: a session launched with a 200000 window and read at 80%
// and $1.20 posts used_percentage 10, total_cost_usd 9 and a five_hour limit at 30%.
// The quota's 5-hour window reads 30%, and the session still reads 80% and $1.20, with
// the transcript's model, not the post's display name.
func TestMEASURES1S3StatusPostMovesQuotaNotWindowedFigures(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.lanes(windowRec("x", trSID, "/repo/w/x", 200000))
	p.hook(trSID, "UserPromptSubmit", t0)
	// 160000 tokens of context; $0.032 of cache reads and $1.168 of output: $1.20.
	p.write(trSID, "", opus("m1", 0, 58400, 0, 160000))
	p.read(trSID, t0.Add(10*time.Second))
	p.post(`{"session_id":"`+trSID+`","cwd":"/repo/w/x","model":{"display_name":"Opus 5.5"},`+
		`"context_window":{"used_percentage":10,"context_window_size":1000000,"current_usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":1,"cache_read_input_tokens":1}},`+
		`"cost":{"total_cost_usd":9},"rate_limits":{"five_hour":{"used_percentage":30}}}`, t0.Add(20*time.Second))

	v := p.m.Snapshot(t0.Add(20 * time.Second))
	if v.Quota == nil || !approx(v.Quota.FiveHour, 30) {
		t.Fatalf("quota %+v, want the 5-hour window at 30%%", v.Quota)
	}
	c := readCtx(t, v, "/repo/w/x")
	if c.session != 80 || c.metrics != 80 || c.window != 200000 {
		t.Fatalf("context (session, metrics) %v / %v against %d after the post, want 80 against 200000", c.session, c.metrics, c.window)
	}
	sv, lv := sessionView(t, v, trSID)
	if !approx(sv.EstCostUSD, 1.2) || !approx(lv.Metrics.CostUSD, 1.2) || !approx(v.EstCostUSD, 1.2) {
		t.Fatalf("est. $ (session, lane metrics, project) %s / %s / %s after the post, want 1.2", fmtF(sv.EstCostUSD), fmtF(lv.Metrics.CostUSD), fmtF(v.EstCostUSD))
	}
	if sv.Model != "claude-opus-5-5" {
		t.Fatalf("model %q after the post, want the transcript's claude-opus-5-5", sv.Model)
	}
	if obs := p.m.DrainSpend(); len(obs) != 1 || math.Abs(obs[0].TotalUSD-1.2) > 1e-9 {
		t.Fatalf("spend observations %+v, want only the transcript's $1.20", obs)
	}
}

// [MEASURES-1-S6] A window-less session in an attached project keeps its status line's
// context: a session with no recorded window, read at $0.50, posts used_percentage 42
// and total_cost_usd 9. Its context reads 42%, and its est. $ stays $0.50.
func TestMEASURES1S6WindowlessSessionKeepsStatusLineContext(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.lanes(windowRec("x", trSID, "/repo/w/x", 0))
	p.hook(trSID, "UserPromptSubmit", t0)
	p.write(trSID, "", opus("m1", 0, 25000, 0, 0)) // 25000 output tokens: $0.50
	p.read(trSID, t0.Add(10*time.Second))
	p.post(`{"session_id":"`+trSID+`","cwd":"/repo/w/x","context_window":{"used_percentage":42,"context_window_size":1000000},`+
		`"cost":{"total_cost_usd":9}}`, t0.Add(20*time.Second))

	v := p.m.Snapshot(t0.Add(20 * time.Second))
	c := readCtx(t, v, "/repo/w/x")
	if c.session != 42 || c.metrics != 42 {
		t.Fatalf("context (session, metrics) %v / %v, want the status line's 42", c.session, c.metrics)
	}
	sv, _ := sessionView(t, v, trSID)
	if !approx(sv.EstCostUSD, 0.5) || !approx(sv.Metrics.CostUSD, 0.5) {
		t.Fatalf("est. $ %s / %s, want the transcript's 0.50, not the post's 9", fmtF(sv.EstCostUSD), fmtF(sv.Metrics.CostUSD))
	}
}

// [MEASURES-1-S3] A session no lane record holds (one /clear started) is windowed by
// the window its status line posted, and a later post without one does not take its
// context back: it posts a 200000 window, is read at 80%, then posts used_percentage 10
// with no window. Its context still reads 80% against 200000 (D3: a status post and a
// reading agree on which of the two sets it).
func TestMEASURES1S3PostedWindowKeepsTheTranscriptsContext(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.hook(trSID2, "UserPromptSubmit", t0)
	p.post(`{"session_id":"`+trSID2+`","cwd":"/repo/w/x","auto_compact_window":200000}`, t0.Add(5*time.Second))
	p.write(trSID2, "", opus("m1", 0, 100, 0, 160000))
	p.read(trSID2, t0.Add(10*time.Second))
	if s := p.m.sessions[trSID2]; !approx(s.CtxPct, 80) {
		t.Fatalf("read after a posted 200000 window: context %s, want 80", fmtF(s.CtxPct))
	}
	p.post(`{"session_id":"`+trSID2+`","cwd":"/repo/w/x","context_window":{"used_percentage":10,"context_window_size":1000000}}`, t0.Add(20*time.Second))
	v := p.m.Snapshot(t0.Add(20 * time.Second))
	sv, _ := sessionView(t, v, trSID2)
	if !approx(sv.CtxPct, 80) || !approx(sv.Metrics.CtxPct, 80) {
		t.Fatalf("after a post with no window: context (session, metrics) %s / %s, want the transcript's 80, not the post's 10",
			fmtF(sv.CtxPct), fmtF(sv.Metrics.CtxPct))
	}
}

// [MEASURES-1-S6] A window-less session keeps its status line's context when its
// reading fails (D7): read at $0.50, it posts used_percentage 42, then a line that is
// not JSON is read. Its context still reads 42%, its est. $ and model are absent, and
// the transcripts source has failed.
func TestMEASURES1S6WindowlessSessionKeepsContextWhenItsReadingFails(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.lanes(windowRec("x", trSID, "/repo/w/x", 0))
	p.hook(trSID, "UserPromptSubmit", t0)
	p.write(trSID, "", opus("m1", 0, 25000, 0, 0))
	p.read(trSID, t0.Add(10*time.Second))
	p.post(`{"session_id":"`+trSID+`","cwd":"/repo/w/x","context_window":{"used_percentage":42,"context_window_size":1000000}}`, t0.Add(20*time.Second))
	p.write(trSID, "", "{not json\n")
	p.read(trSID, t0.Add(30*time.Second))

	v := p.m.Snapshot(t0.Add(30 * time.Second))
	sv, _ := sessionView(t, v, trSID)
	if !approx(sv.CtxPct, 42) || !approx(sv.Metrics.CtxPct, 42) {
		t.Fatalf("context (session, metrics) %s / %s after the bad line, want the status line's 42", fmtF(sv.CtxPct), fmtF(sv.Metrics.CtxPct))
	}
	if sv.EstCostUSD != nil || sv.Metrics.CostUSD != nil || sv.Model != "" {
		t.Fatalf("est. $ %s / %s, model %q after the bad line, want absent", fmtF(sv.EstCostUSD), fmtF(sv.Metrics.CostUSD), sv.Model)
	}
	if src := v.Sources["transcripts"]; src.OK || src.Pending || !strings.Contains(src.Error, trSID) {
		t.Fatalf("transcripts source %+v, want failed naming %s", src, trSID)
	}
}

// D7: a transcript removed after it was read leaves no stale figure: a session read at
// 80% and $1.20 whose transcript is then gone reads no context, est. $ or model.
func TestTranscriptRemovedAfterReadUnsetsFigures(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.lanes(windowRec("x", trSID, "/repo/w/x", 200000))
	p.hook(trSID, "UserPromptSubmit", t0)
	p.write(trSID, "", opus("m1", 0, 58400, 0, 160000))
	p.read(trSID, t0.Add(10*time.Second))
	if sv, _ := sessionView(t, p.m.Snapshot(t0.Add(10*time.Second)), trSID); !approx(sv.CtxPct, 80) || !approx(sv.EstCostUSD, 1.2) {
		t.Fatalf("before: context %s, est. $ %s, want 80 and 1.2", fmtF(sv.CtxPct), fmtF(sv.EstCostUSD))
	}
	if err := os.Remove(p.path(trSID, "")); err != nil {
		t.Fatal(err)
	}
	if r := p.read(trSID, t0.Add(20*time.Second)); r.Found {
		t.Fatalf("reading %+v after the transcript was removed, want not found", r)
	}
	v := p.m.Snapshot(t0.Add(20 * time.Second))
	sv, lv := sessionView(t, v, trSID)
	if sv.CtxPct != nil || sv.Metrics.CtxPct != nil || sv.EstCostUSD != nil || lv.Metrics.CostUSD != nil || v.EstCostUSD != nil || sv.Model != "" {
		t.Fatalf("after the transcript was removed: context %s / %s, est. $ %s / %s / %s, model %q, want all absent",
			fmtF(sv.CtxPct), fmtF(sv.Metrics.CtxPct), fmtF(sv.EstCostUSD), fmtF(lv.Metrics.CostUSD), fmtF(v.EstCostUSD), sv.Model)
	}
}

// [MEASURES-2-S2] D7: no usage record after a Stop makes the figures unknown, not the
// reading's: a session whose main thread holds a prompt and no assistant record, with
// a subagent's priced call, read after its Stop hook, shows no est. $, and the source
// has failed naming "no usage record".
func TestMEASURES2S2NoUsageRecordAfterStopUnsetsFigures(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.lanes(windowRec("x", trSID, "/repo/w/x", 200000))
	p.hook(trSID, "UserPromptSubmit", t0)
	p.write(trSID, "", userLine)
	p.write(trSID, "agent-a", opus("s1", 0, 1000, 0, 0))
	// Before the Stop, the subagent's call is the reading's est. $: the turn may still
	// be answering.
	p.read(trSID, t0.Add(5*time.Second))
	if sv, _ := sessionView(t, p.m.Snapshot(t0.Add(5*time.Second)), trSID); !approx(sv.EstCostUSD, 0.02) {
		t.Fatalf("before the Stop: est. $ %s, want the subagent's 0.02", fmtF(sv.EstCostUSD))
	}
	p.hook(trSID, "Stop", t0.Add(7*time.Second))
	p.read(trSID, t0.Add(10*time.Second))

	v := p.m.Snapshot(t0.Add(10 * time.Second))
	sv, lv := sessionView(t, v, trSID)
	if sv.CtxPct != nil || sv.EstCostUSD != nil || sv.Metrics.CostUSD != nil || lv.Metrics.CostUSD != nil || v.EstCostUSD != nil || sv.Model != "" {
		t.Fatalf("no usage record after the Stop: context %s, est. $ %s / %s / %s, model %q, want all absent",
			fmtF(sv.CtxPct), fmtF(sv.EstCostUSD), fmtF(lv.Metrics.CostUSD), fmtF(v.EstCostUSD), sv.Model)
	}
	if src := v.Sources["transcripts"]; src.OK || !strings.Contains(src.Error, "no usage record") {
		t.Fatalf("transcripts source %+v, want failed naming \"no usage record\"", src)
	}
}

// [MEASURES-1-S4] A lane without a recorded window shows cost, no percentage, and a
// warning: a lane the panel launched with no launch.context_window, read with no status
// post, shows its est. $ and no context % (absent, not 0), and the page carries one
// warning for the project naming launch.context_window. A session the panel did not
// launch is worded as such, in the same one warning.
func TestMEASURES1S4LaneWithoutWindowShowsCostAndWarns(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.lanes(windowRec("x", trSID, "/repo/w/x", 0))
	p.hook(trSID, "UserPromptSubmit", t0)
	p.write(trSID, "", opus("m1", 0, 25000, 0, 100000))
	p.read(trSID, t0.Add(10*time.Second))

	v := p.m.Snapshot(t0.Add(10 * time.Second))
	sv, lv := sessionView(t, v, trSID)
	if !approx(sv.EstCostUSD, 0.52) {
		t.Fatalf("est. $ %s, want 0.52", fmtF(sv.EstCostUSD))
	}
	if sv.CtxPct != nil || sv.Metrics.CtxPct != nil || lv.CtxPct != nil {
		t.Fatalf("context %s / %s / %s, want absent", fmtF(sv.CtxPct), fmtF(sv.Metrics.CtxPct), fmtF(lv.CtxPct))
	}
	ws := warningsWith(v, WarnTranscriptWindow)
	if len(ws) != 1 || !strings.Contains(ws[0].Text, "launch.context_window") || !strings.Contains(ws[0].Text, "x launched without") {
		t.Fatalf("window warnings %+v, want one naming lane x and launch.context_window", ws)
	}

	// A session the panel did not launch (no lane record holds its id) joins the same
	// warning, worded for its case.
	p.hook(trSID2, "UserPromptSubmit", t0)
	p.write(trSID2, "", opus("m1", 0, 25000, 0, 0))
	p.read(trSID2, t0.Add(10*time.Second))
	ws = warningsWith(p.m.Snapshot(t0.Add(10*time.Second)), WarnTranscriptWindow)
	if len(ws) != 1 || !strings.Contains(ws[0].Text, "launch.context_window") || !strings.Contains(ws[0].Text, "1 session(s) the panel did not launch") {
		t.Fatalf("window warnings %+v, want one, naming launch.context_window and the session the panel did not launch", ws)
	}

	// Only the session the panel did not launch: the warning does not send the owner to
	// launch.context_window, which cannot help it.
	q := newTrProject(t, "")
	q.hook(trSID2, "UserPromptSubmit", t0)
	q.write(trSID2, "", opus("m1", 0, 25000, 0, 0))
	q.read(trSID2, t0.Add(10*time.Second))
	ws = warningsWith(q.m.Snapshot(t0.Add(10*time.Second)), WarnTranscriptWindow)
	if len(ws) != 1 || strings.Contains(ws[0].Text, "launch.context_window") || !strings.Contains(ws[0].Text, "knows no context window") {
		t.Fatalf("window warnings %+v, want one saying the panel knows no window, not naming launch.context_window", ws)
	}

	// A windowed lane has no such warning.
	w := newTrProject(t, "")
	w.lanes(windowRec("x", trSID, "/repo/w/x", 200000))
	w.hook(trSID, "UserPromptSubmit", t0)
	w.write(trSID, "", opus("m1", 0, 25000, 0, 100000))
	w.read(trSID, t0.Add(10*time.Second))
	if ws := warningsWith(w.m.Snapshot(t0.Add(10*time.Second)), WarnTranscriptWindow); len(ws) != 0 {
		t.Fatalf("window warnings %+v for a windowed lane, want none", ws)
	}
}

// [MEASURES-2-S1] An assistant line without usage, and a good line after it: a session
// read at 80% and $1.20, then an assistant line of "version": "2.1.400" with no
// message.usage. Its context and est. $ are absent, the transcripts source has failed
// naming the session, the line's number and 2.1.400 (which the page's source banner
// shows as "Cannot read"), and a good line appended after it leaves both so.
func TestMEASURES2S1LineWithoutUsageFailsAndStaysFailed(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.lanes(windowRec("x", trSID, "/repo/w/x", 200000))
	p.hook(trSID, "UserPromptSubmit", t0)
	p.write(trSID, "", opus("m1", 0, 58400, 0, 160000))
	p.read(trSID, t0.Add(10*time.Second))
	v := p.m.Snapshot(t0.Add(10 * time.Second))
	if sv, _ := sessionView(t, v, trSID); !approx(sv.CtxPct, 80) || !approx(sv.EstCostUSD, 1.2) {
		t.Fatalf("before: context %s, est. $ %s, want 80 and 1.2", fmtF(sv.CtxPct), fmtF(sv.EstCostUSD))
	}
	if src := v.Sources["transcripts"]; !src.OK || src.Pending {
		t.Fatalf("transcripts source %+v before the bad line, want ok", src)
	}

	p.write(trSID, "", `{"type":"assistant","version":"2.1.400","message":{"id":"m2","model":"claude-opus-5-5"}}`+"\n")
	check := func(when string, at time.Time) {
		t.Helper()
		v := p.m.Snapshot(at)
		sv, lv := sessionView(t, v, trSID)
		if sv.CtxPct != nil || sv.Metrics.CtxPct != nil || sv.EstCostUSD != nil || sv.Metrics.CostUSD != nil || lv.Metrics.CostUSD != nil || v.EstCostUSD != nil || sv.Model != "" {
			t.Fatalf("%s: context %s, est. $ %s / %s / %s, model %q, want all absent", when, fmtF(sv.CtxPct), fmtF(sv.EstCostUSD), fmtF(lv.Metrics.CostUSD), fmtF(v.EstCostUSD), sv.Model)
		}
		src := v.Sources["transcripts"]
		if src.OK || src.Pending || !strings.Contains(src.Error, trSID) || !strings.Contains(src.Error, "line 2") || !strings.Contains(src.Error, "2.1.400") {
			t.Fatalf("%s: transcripts source %+v, want failed naming %s, line 2 and 2.1.400", when, src, trSID)
		}
	}
	p.read(trSID, t0.Add(20*time.Second))
	check("after the line without usage", t0.Add(20*time.Second))
	p.write(trSID, "", opus("m3", 0, 100, 0, 1000))
	p.read(trSID, t0.Add(30*time.Second))
	check("after a good line", t0.Add(30*time.Second))

	// The page draws a "Cannot read" banner for every failing source but prs, keyed by
	// its name, so a failed transcripts source needs no page change (D7).
	if js := readWeb(t, "panel.js"); !strings.Contains(js, `if (k !== "prs" && !src.pending && !src.ok) kids.push(key(el("div", "banner", null, [el("b", null, "Cannot read")`) {
		t.Fatal("panel.js no longer draws a Cannot read banner for every failing source")
	}
}

// [MEASURES-2-S2] A finished turn with no usage record: a reading taken before the
// session's Stop hook, holding lines but no assistant record, does not fail the
// source, even when applied after the hook; one taken after it does, naming the
// session and "no usage record"; and a reading that finds one clears it.
func TestMEASURES2S2FinishedTurnWithNoUsageRecord(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.lanes(windowRec("x", trSID, "/repo/w/x", 200000))
	p.hook(trSID, "UserPromptSubmit", t0)
	p.write(trSID, "", userLine)
	src := func(at time.Time) SourceStatus { return p.m.Snapshot(at).Sources["transcripts"] }

	p.read(trSID, t0.Add(5*time.Second))
	if s := src(t0.Add(5 * time.Second)); !s.OK {
		t.Fatalf("before the Stop hook: transcripts source %+v, want ok", s)
	}
	p.hook(trSID, "Stop", t0.Add(10*time.Second))
	// Taken at 8 s, applied after the hook at 10 s: judged by when it was taken.
	p.read(trSID, t0.Add(8*time.Second))
	if s := src(t0.Add(10 * time.Second)); !s.OK {
		t.Fatalf("a reading taken before the Stop hook: transcripts source %+v, want ok", s)
	}
	p.read(trSID, t0.Add(15*time.Second))
	if s := src(t0.Add(15 * time.Second)); s.OK || !strings.Contains(s.Error, trSID) || !strings.Contains(s.Error, "no usage record") {
		t.Fatalf("after the Stop hook: transcripts source %+v, want failed naming %s and \"no usage record\"", s, trSID)
	}
	p.write(trSID, "", opus("m1", 0, 100, 0, 1000))
	p.read(trSID, t0.Add(25*time.Second))
	if s := src(t0.Add(25 * time.Second)); !s.OK {
		t.Fatalf("a reading that finds a usage record: transcripts source %+v, want ok again", s)
	}
}

// [MEASURES-2-S3] No transcript a minute after the first prompt: a reading 59 s after
// the session's first prompt that finds no <id>.jsonl leaves the source ok and the
// figures absent; one at 61 s fails it, naming the session and the folder searched;
// and a reading that finds the file clears it.
func TestMEASURES2S3NoTranscriptAMinuteAfterTheFirstPrompt(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.lanes(windowRec("x", trSID, "/repo/w/x", 200000))
	if err := os.MkdirAll(filepath.Join(p.configDir, "projects", "-repo-w-x"), 0o755); err != nil {
		t.Fatal(err)
	}
	p.hook(trSID, "UserPromptSubmit", t0)

	p.read(trSID, t0.Add(59*time.Second))
	v := p.m.Snapshot(t0.Add(59 * time.Second))
	if s := v.Sources["transcripts"]; !s.OK {
		t.Fatalf("at 59 s: transcripts source %+v, want ok", s)
	}
	if sv, _ := sessionView(t, v, trSID); sv.CtxPct != nil || sv.EstCostUSD != nil {
		t.Fatalf("at 59 s: context %s, est. $ %s, want absent", fmtF(sv.CtxPct), fmtF(sv.EstCostUSD))
	}
	p.read(trSID, t0.Add(61*time.Second))
	searched := filepath.Join(p.configDir, "projects")
	if s := p.m.Snapshot(t0.Add(61 * time.Second)).Sources["transcripts"]; s.OK || !strings.Contains(s.Error, trSID) || !strings.Contains(s.Error, searched) {
		t.Fatalf("at 61 s: transcripts source %+v, want failed naming %s and %s", s, trSID, searched)
	}
	p.write(trSID, "", opus("m1", 0, 100, 0, 1000))
	p.read(trSID, t0.Add(71*time.Second))
	if s := p.m.Snapshot(t0.Add(71 * time.Second)).Sources["transcripts"]; !s.OK {
		t.Fatalf("once the transcript is found: transcripts source %+v, want ok again", s)
	}
}

// [MEASURES-2-S6] One session's good read does not clear another's error: of two
// sessions, one has failed on a line; the other's transcript then reads without error,
// and the source is still failed, naming the first.
func TestMEASURES2S6GoodReadDoesNotClearAnothersError(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.lanes(windowRec("x", trSID, "/repo/w/x", 200000), windowRec("y", trSID2, "/repo/w/x", 200000))
	p.hook(trSID, "UserPromptSubmit", t0)
	p.hook(trSID2, "UserPromptSubmit", t0)
	p.write(trSID, "", "{not json\n")
	p.write(trSID2, "", opus("m1", 0, 100, 0, 1000))
	p.read(trSID, t0.Add(10*time.Second))
	p.read(trSID2, t0.Add(20*time.Second))
	s := p.m.Snapshot(t0.Add(20 * time.Second)).Sources["transcripts"]
	if s.OK || !strings.Contains(s.Error, trSID) || strings.Contains(s.Error, trSID2) {
		t.Fatalf("transcripts source %+v, want failed naming %s alone", s, trSID)
	}
}

// [MEASURES-3-S2] An unknown model is left out and named: a session with two
// claude-opus-5-5 calls and three claude-opus-5-6 calls shows the two Opus 5.5 calls'
// cost alone, the page carries a warning naming the lane, claude-opus-5-6 and 3 calls
// left out, and a session with only the three claude-opus-5-6 calls shows no est. $.
func TestMEASURES3S2UnknownModelIsLeftOutAndNamed(t *testing.T) {
	t.Parallel()
	p := newTrProject(t, "")
	p.lanes(windowRec("x", trSID, "/repo/w/x", 200000))
	p.hook(trSID, "UserPromptSubmit", t0)
	p.write(trSID, "", opus("a", 0, 1000, 0, 0)+assistantLine("claude-opus-5-6", "b", 0, 1000, 0, 0)+
		assistantLine("claude-opus-5-6", "c", 0, 1000, 0, 0)+opus("d", 0, 1000, 0, 0)+assistantLine("claude-opus-5-6", "e", 0, 1000, 0, 0))
	p.read(trSID, t0.Add(10*time.Second))
	v := p.m.Snapshot(t0.Add(10 * time.Second))
	if sv, _ := sessionView(t, v, trSID); !approx(sv.EstCostUSD, 0.04) {
		t.Fatalf("est. $ %s, want the two Opus 5.5 calls' 0.04", fmtF(sv.EstCostUSD))
	}
	ws := warningsWith(v, WarnTranscriptUnpriced)
	if len(ws) != 1 || !strings.HasPrefix(ws[0].Text, "x: ") || !strings.Contains(ws[0].Text, "3 call(s) on claude-opus-5-6") {
		t.Fatalf("unpriced warnings %+v, want one naming lane x, claude-opus-5-6 and 3 calls", ws)
	}

	q := newTrProject(t, "")
	q.lanes(windowRec("x", trSID, "/repo/w/x", 200000))
	q.hook(trSID, "UserPromptSubmit", t0)
	q.write(trSID, "", assistantLine("claude-opus-5-6", "b", 0, 1000, 0, 0)+
		assistantLine("claude-opus-5-6", "c", 0, 1000, 0, 0)+assistantLine("claude-opus-5-6", "e", 0, 1000, 0, 0))
	q.read(trSID, t0.Add(10*time.Second))
	v = q.m.Snapshot(t0.Add(10 * time.Second))
	if sv, lv := sessionView(t, v, trSID); sv.EstCostUSD != nil || sv.Metrics.CostUSD != nil || lv.Metrics.CostUSD != nil || v.EstCostUSD != nil {
		t.Fatalf("only unpriced calls: est. $ %s / %s / %s, want absent, not $0", fmtF(sv.EstCostUSD), fmtF(lv.Metrics.CostUSD), fmtF(v.EstCostUSD))
	}
	if ws := warningsWith(v, WarnTranscriptUnpriced); len(ws) != 1 || !strings.Contains(ws[0].Text, "3 call(s) on claude-opus-5-6") {
		t.Fatalf("unpriced warnings %+v, want one for 3 calls on claude-opus-5-6", ws)
	}
}
