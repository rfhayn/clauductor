package panel

import (
	"context"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// transcriptLine is one hand-written transcript line: no real transcript content is
// committed (PANEL-49), so the fixture is built here, with every field the reader
// decodes and a content field it must drop.
func transcriptLine(msgID string, in, write, read, out int) string {
	return fmt.Sprintf(`{"type":"assistant","version":"2.1.302","message":{"id":%q,"model":"claude-opus-5-5","role":"assistant",`+
		`"content":[{"type":"text","text":"never decoded"}],"usage":{"input_tokens":%d,"cache_creation_input_tokens":%d,`+
		`"cache_read_input_tokens":%d,"output_tokens":%d}}}`+"\n", msgID, in, write, read, out)
}

// sessionOn is session sid's view on the lane at path, or nil.
func sessionOn(v state.View, path, sid string) *state.SessionView {
	for _, l := range append(append([]state.LaneView{}, v.Lanes...), v.QuietWorktrees...) {
		if l.Path != path {
			continue
		}
		for i := range l.Sessions {
			if l.Sessions[i].ID == sid {
				return &l.Sessions[i]
			}
		}
	}
	return nil
}

func near(p *float64, want float64) bool { return p != nil && math.Abs(*p-want) < 1e-9 }

// [MEASURES-1-S1] [MEASURES-1-S2] One panel, built through live.go's path, serves an
// attached project and a project whose config is in its repository, on real tmux and
// git, with both sessions' transcripts under the same temporary config directory.
// The attached project's lane shows the context, est. $, model and context alert its
// transcript and its subagent's give; the in-repository project's transcripts source
// never runs, and its session's context is its status line's 42%.
func TestTranscriptsMeasureOnlyAnAttachedProjectsLanes(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	home := t.TempDir()
	// testEnv unsets CLAUDE_CONFIG_DIR, so the panel's config directory is the home's.
	projects := filepath.Join(home, ".claude", "projects")

	root, sidecar := attachedRepo(t, home, "acme", "")
	writeFile(t, sidecar, `{"name":"Acme","lanes":{"main":"orchestrator","fix/":"fix"},"base":"main","worktree_dir":".wt",
		"alerts":{"context_pct":85},"launch":{"context_window":200000}}`)
	if _, err := install.TrustConfig(home, root, sidecar); err != nil {
		t.Fatal(err)
	}
	if reg, err := config.LoadProjects(home); err != nil || reg.Find(root) == nil || install.OutsideConfig(*reg.Find(root)) == "" {
		t.Fatalf("the attached project's config is not recorded outside its repository (%v)", err)
	}
	inRepo := multiRepo(t, "Beta", noServerSocket())
	if _, err := install.AddProject(context.Background(), install.AddOptions{Home: home, Project: inRepo, Run: gitOnlyRunner, Now: clock.System.Now()}); err != nil {
		t.Fatal(err)
	}

	p := startPanelWith(t, root, home, sock, func(o *Options) { o.Ticks.Transcripts = fastTicks().Obs })
	p.polls.more(t, "beta/worktrees", 1)
	c := liveClient{base: p.base}

	code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "fix", Mode: "new", Name: "la"})
	if code != 200 {
		t.Fatalf("start la: %d %v", code, body)
	}
	sid := fmt.Sprint(body["lane"].(map[string]any)["sessionId"])
	wt := filepath.Join(root, ".wt", "la")

	// The lane's transcript: one message streamed over two lines, counted once at the
	// last, whose input, cache-write and cache-read tokens sum to 160000 (80% of the
	// lane's 200000 window), and one subagent call. Opus 5.5: $4 input, $5 a 5-minute
	// write, $0.20 a cache read and $20 output, per MTok.
	//   main:     1000×4 + 9000×5 + 150000×0.20 + 2000×20 = $0.119 (the first line's 999 output tokens are replaced)
	//   subagent: 2000×4 + 500×20 = $0.018
	writeFile(t, filepath.Join(projects, "-acme-wt-la", sid+".jsonl"),
		`{"type":"user","version":"2.1.302","message":{"role":"user","content":"a prompt never decoded"}}`+"\n"+
			transcriptLine("msg_1", 1000, 9000, 150000, 999)+
			`{"type":"attachment","version":"2.1.302","attachment":{"path":"/never/decoded"}}`+"\n"+
			transcriptLine("msg_1", 1000, 9000, 150000, 2000))
	writeFile(t, filepath.Join(projects, "-acme-wt-la", sid, "subagents", "agent-a1.jsonl"),
		transcriptLine("msg_s1", 2000, 0, 0, 500))
	const wantCost = 0.137

	// The in-repository project's session, with a transcript of its own under the same
	// directory, which the panel must never open.
	const betaSID = "0f9c2e4a-6b1d-4c3e-9a7f-2d5b8e1c4a60"
	writeFile(t, filepath.Join(projects, "-beta", betaSID+".jsonl"), transcriptLine("msg_b", 1000, 9000, 150000, 2000))

	c.post(t, "/hook", fmt.Sprintf(`{"hook_event_name":"UserPromptSubmit","session_id":%q,"cwd":%q,"prompt":"go"}`, sid, wt))
	c.post(t, "/hook", fmt.Sprintf(`{"hook_event_name":"UserPromptSubmit","session_id":%q,"cwd":%q,"prompt":"go"}`, betaSID, inRepo))
	c.post(t, "/status", fmt.Sprintf(`{"session_id":%q,"cwd":%q,"context_window":{"used_percentage":42}}`, betaSID, inRepo))

	// MEASURES-1-S1, as /api/state carries it to the page.
	var s *state.SessionView
	var v state.View
	waitFor(t, "the attached lane's context and est. $ from its transcripts", func() bool {
		v = p.state(t)
		s = sessionOn(v, wt, sid)
		return s != nil && near(s.CtxPct, 80) && near(s.EstCostUSD, wantCost)
	})
	if s.Model != "claude-opus-5-5" || !near(s.Metrics.CostUSD, wantCost) || !near(s.Metrics.CtxPct, 80) || s.Metrics.CtxWindow != 200000 {
		t.Errorf("lane session: model %q, metrics cost %v, ctx %v of %d; want claude-opus-5-5, %v, 80 of 200000",
			s.Model, deref(s.Metrics.CostUSD), deref(s.Metrics.CtxPct), s.Metrics.CtxWindow, wantCost)
	}
	if !near(s.Metrics.CtxAlertPct, 78.5) {
		t.Errorf("alert threshold %v, want 78.5", deref(s.Metrics.CtxAlertPct))
	}
	fired := false
	for _, a := range v.Alerts {
		fired = fired || (a.Kind == state.AlertContext && a.Session == sid)
	}
	if !fired {
		t.Errorf("no context alert for the lane's session: %+v", v.Alerts)
	}
	if src, ok := v.Sources["transcripts"]; !ok || !src.OK {
		t.Errorf("the attached project's transcripts source: %+v (present %v), want OK", src, ok)
	}

	// MEASURES-1-S2: by now the attached project's source has polled again, and the
	// in-repository project's has never run, because newRuntime never added it.
	p.polls.more(t, "acme/transcripts", 2)
	p.polls.more(t, "beta/worktrees", 1)
	if n := p.polls.count("beta/transcripts"); n != 0 {
		t.Fatalf("the in-repository project read transcripts %d time(s)", n)
	}
	var bv state.View
	waitFor(t, "the in-repository session's status-line context", func() bool {
		get(t, p, "/api/state?project=beta", &bv)
		bs := sessionOn(bv, inRepo, betaSID)
		return bs != nil && near(bs.CtxPct, 42)
	})
	bs := sessionOn(bv, inRepo, betaSID)
	if bs.EstCostUSD != nil || bs.Model != "" {
		t.Errorf("the in-repository session took figures from its transcript: est. $ %v, model %q", deref(bs.EstCostUSD), bs.Model)
	}
	if _, ok := bv.Sources["transcripts"]; ok {
		t.Errorf("the in-repository project shows a transcripts source: %+v", bv.Sources)
	}
}

// builtRuntime is project id's runtime as live.go builds it (loadProject, then
// buildRuntime), on a fake clock, with nothing started: its sources are polled by hand.
func builtRuntime(t *testing.T, home, id string) *Runtime {
	t.Helper()
	reg, err := config.LoadProjects(home)
	if err != nil {
		t.Fatal(err)
	}
	e := reg.Find(id)
	if e == nil {
		t.Fatalf("%s is not registered", id)
	}
	o := Options{Home: home, Out: io.Discard, Runner: gitOnlyRunner, LookupEnv: testEnv}
	lp, err := loadProject(context.Background(), o, *e, false)
	if err != nil {
		t.Fatal(err)
	}
	return buildRuntime(o, clock.NewFake(t0), DefaultTicks(), lp, false)
}

func sourceNamed(r *Runtime, name string) *source {
	for _, s := range r.sources {
		if s.name == name {
			return s
		}
	}
	return nil
}

// [MEASURES-1-S1] [MEASURES-1-S2] The same split as above, with no port and no tmux,
// so it runs everywhere the short suite does: live.go's path adds the transcripts
// source, every 10 s, to the attached project's runtime alone. One poll of it, as the
// source's loop runs it, measures the attached lane; the in-repository project's
// state offers its source no session to read, and it has no source to read them.
func TestTranscriptsSourceIsAddedForAnAttachedProjectAlone(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	projects := filepath.Join(home, ".claude", "projects")
	root, sidecar := attachedRepo(t, home, "acme", "")
	writeFile(t, sidecar, `{"name":"Acme","lanes":{"main":"orchestrator"},"tmux_socket":"`+noServerSocket()+`",
		"alerts":{"context_pct":85},"launch":{"context_window":200000}}`)
	inRepo := multiRepo(t, "Beta", noServerSocket())
	if _, err := install.AddProject(context.Background(), install.AddOptions{Home: home, Project: inRepo, Run: gitOnlyRunner, Now: clock.System.Now()}); err != nil {
		t.Fatal(err)
	}
	acme, beta := builtRuntime(t, home, "acme"), builtRuntime(t, home, "beta")

	src := sourceNamed(acme, "transcripts")
	if src == nil {
		t.Fatal("the attached project's runtime has no transcripts source")
	}
	if src.every != 10*time.Second || !src.fixedRate {
		t.Errorf("the transcripts source runs every %v (fixed rate %v), want every 10s", src.every, src.fixedRate)
	}
	if sourceNamed(beta, "transcripts") != nil {
		t.Fatal("the in-repository project's runtime has a transcripts source")
	}

	const sid, betaSID = "3c1d7e2a-9b4f-4a6c-8d0e-5f2a1b7c9e31", "0f9c2e4a-6b1d-4c3e-9a7f-2d5b8e1c4a60"
	writeFile(t, filepath.Join(projects, "-acme", sid+".jsonl"), transcriptLine("msg_1", 1000, 9000, 150000, 2000))
	writeFile(t, filepath.Join(projects, "-acme", sid, "subagents", "agent-a1.jsonl"), transcriptLine("msg_s1", 2000, 0, 0, 500))
	writeFile(t, filepath.Join(projects, "-beta", betaSID+".jsonl"), transcriptLine("msg_b", 1000, 9000, 150000, 2000))
	acme.hub.Update(func(m *state.Model, now time.Time) {
		m.ApplyTmux([]types.TmuxLane{{ID: "la", Path: root}}, []types.LaneRecord{{ID: "la", SessionID: sid, Path: root, Type: "orchestrator", ContextWindow: 200000}}, "", nil, now)
		m.ApplyHook(signals.HookEvent{SessionID: sid, Cwd: root, Event: "UserPromptSubmit", Prompt: "go"}, now)
	})
	beta.hub.Update(func(m *state.Model, now time.Time) {
		m.ApplyHook(signals.HookEvent{SessionID: betaSID, Cwd: inRepo, Event: "UserPromptSubmit", Prompt: "go"}, now)
	})

	up, _ := src.poll(context.Background(), t0)
	if up == nil {
		t.Fatal("the transcripts poll applied nothing")
	}
	acme.hub.Update(up)
	v := acme.hub.View()
	s := sessionOn(v, root, sid)
	if s == nil || !near(s.CtxPct, 80) || !near(s.EstCostUSD, 0.137) || s.Model != "claude-opus-5-5" {
		t.Fatalf("the attached lane's session: %+v, want 80%%, est. $ 0.137, claude-opus-5-5", s)
	}

	var held []string
	beta.hub.Read(func(m *state.Model, _ time.Time) { held = m.TranscriptSessions() })
	if held != nil {
		t.Errorf("the in-repository project offers sessions to read: %v", held)
	}
	if _, ok := beta.hub.View().Sources["transcripts"]; ok {
		t.Error("the in-repository project shows a transcripts source")
	}

	// D6: a cursor is kept for each session the state holds, and dropped for one it
	// no longer holds.
	cursors := map[string]*signals.TranscriptCursor{"9e8d7c6b-5a49-4382-a1b0-c9d8e7f6a5b4": {}}
	if _, next := acme.pollTranscripts(context.Background(), cursors, t0); next != 0 {
		t.Errorf("the poll asks to wait %v, want its cadence", next)
	}
	if _, ok := cursors[sid]; !ok || len(cursors) != 1 {
		t.Errorf("cursors after a poll: %v, want %s's alone", cursors, sid)
	}
}

// A poll's readings are applied at the time the poll STARTED, not when the update
// is applied (D7, group 2's rule): a Stop that arrives while the poll reads is
// judged by the next poll. Here the reading, taken at t0, holds lines but no usage
// record; the Stop lands at t0+1s; the update is applied at t0+2s. Judged at its
// apply time, the reading would fail the session for a turn that finished after it
// was taken.
func TestTranscriptsPollAppliesAtItsStartTime(t *testing.T) {
	t.Parallel()
	home := signals.ResolvePath(t.TempDir())
	root, sidecar := attachedRepo(t, home, "acme", "")
	writeFile(t, sidecar, `{"name":"Acme","lanes":{"main":"orchestrator"},"tmux_socket":"`+noServerSocket()+`"}`)
	acme := builtRuntime(t, home, "acme")
	clk, ok := acme.clock.(*clock.Fake)
	if !ok {
		t.Fatalf("the runtime's clock is %T, want the fake", acme.clock)
	}
	const sid = "3c1d7e2a-9b4f-4a6c-8d0e-5f2a1b7c9e31"
	writeFile(t, filepath.Join(home, ".claude", "projects", "-acme", sid+".jsonl"),
		`{"type":"user","version":"2.1.302","message":{"role":"user","content":"go"}}`+"\n")
	acme.hub.Update(func(m *state.Model, now time.Time) {
		m.ApplyHook(signals.HookEvent{SessionID: sid, Cwd: root, Event: "UserPromptSubmit", Prompt: "go"}, now)
	})

	up, _ := acme.pollTranscripts(context.Background(), map[string]*signals.TranscriptCursor{}, clk.Now())
	if up == nil {
		t.Fatal("the transcripts poll applied nothing")
	}
	clk.Advance(time.Second)
	acme.hub.Update(func(m *state.Model, now time.Time) {
		m.ApplyHook(signals.HookEvent{SessionID: sid, Cwd: root, Event: "Stop"}, now)
	})
	clk.Advance(time.Second)
	acme.hub.Update(up)
	if src := acme.hub.View().Sources["transcripts"]; !src.OK {
		t.Errorf("a reading taken before the Stop failed the transcripts source: %+v", src)
	}
}

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}
