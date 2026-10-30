package state

import (
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rfhayn/stridal/internal/panel/signals"
	"github.com/rfhayn/stridal/internal/panel/types"
)

// The reducer reconciles the lane registry with tmux, the worktree list and the
// sessions (hooks and claude agents), binding a lane to its session by session id.
// Anything that does not add up is shown as an orphan, never dropped.
func TestReducerReconcilesLanes(t *testing.T) {
	t.Parallel()
	cfg, err := loadConfig(t, []byte(`{"name":"T","lanes":{"main":"orchestrator","fix/":"fix"}}`))
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(cfg, "/repo", t0)
	m.ApplyWorktrees([]signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/.wt/x", Branch: "fix/x"}}, nil, t0)
	const sidA, sidB = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	// Lane a's session reports from a subdirectory of the ROOT worktree (it ran cd):
	// its lane is still the one the registry binds its session id to.
	m.ApplyHook(signals.HookEvent{SessionID: sidA, Cwd: "/repo/sub", Event: "UserPromptSubmit", Prompt: "go"}, t0)
	recs := []types.LaneRecord{
		{ID: "a", SessionID: sidA, Path: "/repo/.wt/x", Type: "fix", ActionDone: true},
		{ID: "b", SessionID: sidB, Path: "/repo", Type: "orchestrator", ActionDone: true},
		{ID: "c", SessionID: "33333333-3333-4333-8333-333333333333", Path: "/repo", Type: "orchestrator", Action: "start"},
		{ID: "e", SessionID: "44444444-4444-4444-8444-444444444444", Path: "/gone/wt", Type: "fix", ActionDone: true},
	}
	tmux := []types.TmuxLane{
		{ID: "a", Path: "/repo/.wt/x", Type: "fix"},
		{ID: "d", Path: "/repo", Attached: 1},
		{ID: "e", Path: "/gone/wt", Dead: true, DeadStatus: "1"},
	}
	m.ApplyTmux(tmux, recs, "", nil, t0)
	v := m.Snapshot(t0)
	get := func(id string) TermLaneView {
		for _, tv := range v.Terminals {
			if tv.ID == id {
				return tv
			}
		}
		t.Fatalf("lane %s missing from %+v", id, v.Terminals)
		return TermLaneView{}
	}
	a := get("a")
	if !a.Running || !a.Registered || a.Status != "busy" || a.Orphan != "" || a.Worktree != "/repo/.wt/x" || a.Branch != "fix/x" {
		t.Errorf("a: %+v", a)
	}
	b := get("b")
	if b.Running || b.Status != "orphaned" || !strings.Contains(b.Orphan, "tmux session is gone") {
		t.Errorf("b (registered, tmux gone): %+v", b)
	}
	c := get("c")
	if c.Status != "orphaned" || c.Action != "start" || !strings.Contains(c.Orphan, `stopped during "start"`) {
		t.Errorf("c (start interrupted): %+v", c)
	}
	d := get("d")
	if !d.Running || d.Registered || d.Type != "orchestrator" || !strings.Contains(d.Orphan, "not in the lane registry") {
		t.Errorf("d (tmux only): %+v", d)
	}
	e := get("e")
	if e.Status != "dead" || e.Worktree != "" || !strings.Contains(e.Orphan, "not one of the project's worktrees") {
		t.Errorf("e (dead, worktree gone): %+v", e)
	}
	// The worktree lane shows which terminal runs in it.
	for _, l := range append(v.Lanes, v.QuietWorktrees...) {
		if l.Path == "/repo/.wt/x" && l.Terminal != "a" {
			t.Errorf("worktree x shows terminal %q, want a", l.Terminal)
		}
	}
	// A failed tmux read keeps the last good lanes and says so.
	m.ApplyTmux(nil, nil, "", errTest("tmux: boom"), t0)
	v = m.Snapshot(t0)
	if v.Sources["tmux"].OK || len(v.Terminals) != len(tmux)+2 {
		t.Fatalf("after a failed read: %+v, %d terminals", v.Sources["tmux"], len(v.Terminals))
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

// PANEL-57: the close verdict, TermLaneView.CloseHeld (D2). Lane x runs on /repo/w/x
// under session sx, on the stall rig's clock.

func closeRig(t *testing.T, cfgExtra string) *stallRig {
	t.Helper()
	r := newStallRig(t, hm(0, 0), cfgExtra)
	r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1}}, []types.LaneRecord{launched("x", "sx", "/repo/w/x")}, "", nil, hm(0, 0))
	// The panel's first poll lists nothing: the lane's claude starts after the panel, so
	// the panel hears all its turns (closeheld.go's liveBeforePanel).
	r.poll(hm(0, 0))
	return r
}

// heldAt is lane id's CloseHeld at at, read without polling.
func (r *stallRig) heldAt(id string, at time.Time) string {
	r.t.Helper()
	for _, tv := range r.m.TerminalViews(at) {
		if tv.ID == id {
			return tv.CloseHeld
		}
	}
	r.t.Fatalf("no lane %q in the views", id)
	return ""
}

// pollTimed is poll for a poll that took dur, as the runtime applies one.
func (r *stallRig) pollTimed(dur time.Duration, at time.Time) {
	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	list := []signals.Agent{}
	for _, id := range ids {
		list = append(list, r.agents[id])
	}
	r.m.ApplyAgentsTimed(list, nil, at.Add(-dur), dur, at)
}

const reviewTask = `"background_tasks":[{"id":"a1","type":"subagent","status":"running","description":"Review group 2"}],"session_crons":[]`

// [LANECLOSE-2-S1] The verdict holds a lane whose claude is not idle on a current
// reading, in auto-close's words, and lets an idle, exited or gone one close.
func TestCloseHeldByStatus(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		set  func(r *stallRig) time.Time
		want string
	}{
		{"busy", func(r *stallRig) time.Time { r.list("sx", "/repo/w/x", "busy"); r.poll(hm(0, 1)); return hm(0, 1) }, "claude is working"},
		{"waiting", func(r *stallRig) time.Time {
			r.list("sx", "/repo/w/x", "waiting", "a permission")
			r.poll(hm(0, 1))
			return hm(0, 1)
		}, "claude is waiting on you"},
		{"running", func(r *stallRig) time.Time { r.poll(hm(0, 1)); return hm(0, 1) }, "claude is starting"},
		{"idle", func(r *stallRig) time.Time { r.list("sx", "/repo/w/x", "idle"); r.poll(hm(0, 1)); return hm(0, 1) }, ""},
		{"dead", func(r *stallRig) time.Time {
			r.list("sx", "/repo/w/x", "busy")
			r.poll(hm(0, 1))
			r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1, Dead: true, DeadStatus: "0"}}, []types.LaneRecord{launched("x", "sx", "/repo/w/x")}, "", nil, hm(0, 1))
			return hm(0, 1)
		}, ""},
		{"gone", func(r *stallRig) time.Time {
			r.list("sx", "/repo/w/x", "busy")
			r.poll(hm(0, 1))
			r.m.ApplyTmux(nil, []types.LaneRecord{launched("x", "sx", "/repo/w/x")}, "", nil, hm(0, 1))
			return hm(0, 1)
		}, ""},
		{"busy approx", func(r *stallRig) time.Time { r.list("sx", "/repo/w/x", "busy"); r.poll(hm(0, 1)); return hm(0, 5) }, "claude is working (not a current reading)"},
		{"idle approx", func(r *stallRig) time.Time { r.list("sx", "/repo/w/x", "idle"); r.poll(hm(0, 1)); return hm(0, 5) }, "claude is idle (not a current reading)"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := closeRig(t, "")
			at := c.set(r)
			if got := r.heldAt("x", at); got != c.want {
				t.Errorf("CloseHeld %q, want %q", got, c.want)
			}
		})
	}
}

// [LANECLOSE-2-S1] A prompt heard after an idle reading's poll began holds the lane as
// working until a poll that began after it, and then, its turn's Stop not yet heard, as
// unheard; the Stop releases it.
func TestCloseHeldByAHookSinceTheReading(t *testing.T) {
	t.Parallel()
	r := closeRig(t, "")
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 1))
	sec := func(at time.Time, s int) time.Time { return at.Add(time.Duration(s) * time.Second) }
	steps := []struct {
		name string
		do   func() time.Time
		want string
	}{
		{"idle reading", func() time.Time { return hm(0, 1) }, ""},
		{"a prompt after it", func() time.Time {
			r.hook(sec(hm(0, 1), 10), "UserPromptSubmit", "sx", "/repo/w/x", `"prompt":"go on"`)
			return sec(hm(0, 1), 10)
		}, "claude is working"},
		{"the next poll", func() time.Time { r.poll(hm(0, 2)); return hm(0, 2) }, CloseUnheardWords},
		{"a tool call after it", func() time.Time {
			r.hook(sec(hm(0, 2), 10), "PreToolUse", "sx", "/repo/w/x", `"tool_name":"Bash","tool_use_id":"t1"`)
			return sec(hm(0, 2), 10)
		}, "claude is working"},
		{"a poll begun after it", func() time.Time { r.pollTimed(30*time.Second, hm(0, 3)); return hm(0, 3) }, CloseUnheardWords},
		{"a prompt during a poll", func() time.Time {
			r.hook(sec(hm(0, 3), 10), "UserPromptSubmit", "sx", "/repo/w/x", `"prompt":"again"`)
			r.pollTimed(30*time.Second, sec(hm(0, 3), 20)) // begun at 00:02:50, before the prompt
			return sec(hm(0, 3), 20)
		}, "claude is working"},
		{"its Stop", func() time.Time {
			r.hook(sec(hm(0, 3), 30), "Stop", "sx", "/repo/w/x", `"background_tasks":[]`)
			return sec(hm(0, 3), 30)
		}, ""},
	}
	for _, st := range steps {
		at := st.do()
		if got := r.heldAt("x", at); got != st.want {
			t.Errorf("%s: CloseHeld %q, want %q", st.name, got, st.want)
		}
	}
}

// [LANECLOSE-2-S3] What a lane's turn left running is not known when the turn's Stop
// never came (an Esc after a prompt that started a shell) or its Stops report no
// background work: either holds an idle lane, and a reporting Stop releases it.
func TestCloseHeldWhileItsWorkIsNotKnown(t *testing.T) {
	t.Parallel()
	t.Run("a Stop not heard", func(t *testing.T) {
		t.Parallel()
		r := closeRig(t, "")
		r.list("sx", "/repo/w/x", "idle")
		r.poll(hm(0, 0))
		r.hook(hm(0, 1), "Stop", "sx", "/repo/w/x", `"background_tasks":[]`)
		r.poll(hm(0, 2))
		if got := r.heldAt("x", hm(0, 2)); got != "" {
			t.Fatalf("after a Stop listing nothing: CloseHeld %q, want none", got)
		}
		r.hook(hm(0, 3), "UserPromptSubmit", "sx", "/repo/w/x", `"prompt":"start the server in the background"`)
		r.poll(hm(0, 4)) // Esc: no Stop; the reading is idle again
		if got := r.heldAt("x", hm(0, 4)); got != CloseUnheardWords {
			t.Errorf("after a prompt and no Stop: CloseHeld %q, want %q", got, CloseUnheardWords)
		}
		r.hook(hm(0, 5), "Stop", "sx", "/repo/w/x", `"background_tasks":[]`)
		r.poll(hm(0, 6))
		if got := r.heldAt("x", hm(0, 6)); got != "" {
			t.Errorf("after the next Stop listing nothing: CloseHeld %q, want none", got)
		}
	})
	t.Run("a Stop that does not report", func(t *testing.T) {
		t.Parallel()
		r := closeRig(t, "")
		r.list("sx", "/repo/w/x", "idle")
		r.poll(hm(0, 0))
		r.hook(hm(0, 1), "Stop", "sx", "/repo/w/x", `"last_assistant_message":"done"`)
		r.poll(hm(0, 2))
		if got := r.heldAt("x", hm(0, 2)); got != CloseNotReportedWords {
			t.Errorf("a Stop with no background_tasks: CloseHeld %q, want %q", got, CloseNotReportedWords)
		}
		r.hook(hm(0, 3), "Stop", "sx", "/repo/w/x", `"background_tasks":[]`)
		r.poll(hm(0, 4))
		if got := r.heldAt("x", hm(0, 4)); got != "" {
			t.Errorf("after a Stop listing nothing: CloseHeld %q, want none", got)
		}
	})
}

// [LANECLOSE-2-S3] What a lane's process left dies with it: after a Restart resumes the
// session under a new process (another PID and start), neither the old process's Stop
// listing a shell nor its turn whose Stop never came holds the lane.
func TestCloseHeldNotByTheProcessBeforeARestart(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, hook, extra, before string }{
		{"a Stop listing a shell", "Stop", `"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"npm run dev"}],"session_crons":[]`,
			"its turn ended on background work: npm run dev"},
		{"a turn whose Stop never came", "UserPromptSubmit", `"prompt":"run the server"`, CloseUnheardWords},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := closeRig(t, "")
			r.list("sx", "/repo/w/x", "idle")
			r.poll(hm(0, 0))
			r.hook(hm(0, 1), "Stop", "sx", "/repo/w/x", `"background_tasks":[]`)
			r.hook(hm(0, 1), c.hook, "sx", "/repo/w/x", c.extra)
			r.poll(hm(0, 2))
			if got := r.heldAt("x", hm(0, 2)); got != c.before {
				t.Fatalf("before the restart: CloseHeld %q, want %q", got, c.before)
			}
			delete(r.agents, "sx") // the old claude exits
			r.poll(hm(0, 3))
			r.list("sx", "/repo/w/x", "idle")
			r.proc("sx", 4242, hm(0, 4)) // Restart: `claude --resume` under the same id
			r.poll(hm(0, 5))
			if got := r.heldAt("x", hm(0, 5)); got != "" {
				t.Errorf("after the restart: CloseHeld %q, want none", got)
			}
		})
	}
}

// [LANECLOSE-2-S3] A Restart's first turn ran before the poll that lists the new process,
// so its prompt and its Stop arrive while the panel still holds the old PID. With no
// start listed, nothing tells them from the old process's, so they are kept; with a start
// listed before them, they are the new process's and kept too. Either way they hold the
// lane until the new process's next Stop says what runs.
func TestCloseHeldByARestartsTurnBeforeItsListing(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		hooks func(r *stallRig)
		held  string
	}{
		{"its turn started a subagent", func(r *stallRig) {
			r.hook(hm(0, 4), "UserPromptSubmit", "sx", "/repo/w/x", `"prompt":"review group 2"`)
			r.hook(hm(0, 5), "Stop", "sx", "/repo/w/x", reviewTask)
		}, "its turn ended on background work: Review group 2"},
		{"its turn's Stop not yet heard", func(r *stallRig) {
			r.hook(hm(0, 4), "UserPromptSubmit", "sx", "/repo/w/x", `"prompt":"review group 2"`)
		}, CloseUnheardWords},
	} {
		for _, gap := range []bool{false, true} {
			for _, started := range []time.Time{{}, hm(0, 3)} {
				name := c.name + ", restarted between two polls"
				if gap {
					name = c.name + ", re-listed after a poll without it"
				}
				if !started.IsZero() {
					name += ", its start listed"
				}
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					restartRelisted(t, gap, started, c.hooks, c.held)
				})
			}
		}
	}
}

// restartRelisted runs TestCloseHeldByARestartsTurnBeforeItsListing's sequence: with
// gap, a poll lists the session without its old claude before the new one is listed,
// under PID 4242 and started (zero: no start listed).
func restartRelisted(t *testing.T, gap bool, started time.Time, hooks func(r *stallRig), held string) {
	r := closeRig(t, "")
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.hook(hm(0, 1), "Stop", "sx", "/repo/w/x", `"background_tasks":[]`)
	r.poll(hm(0, 2))
	if got := r.heldAt("x", hm(0, 2)); got != "" {
		t.Fatalf("before the restart: CloseHeld %q, want none", got)
	}
	if gap {
		delete(r.agents, "sx") // the old claude exits
		r.poll(hm(0, 3))
	}
	hooks(r) // `claude --resume` under the same id, before the next poll
	r.list("sx", "/repo/w/x", "idle")
	a := r.agents["sx"]
	a.PID, a.StartedAt = 4242, 0 // another process
	if !started.IsZero() {
		a.StartedAt = started.UnixMilli()
	}
	r.agents["sx"] = a
	r.poll(hm(0, 6))
	if got := r.heldAt("x", hm(0, 6)); got != held {
		t.Errorf("after the restart: CloseHeld %q, want %q", got, held)
	}
	r.hook(hm(0, 7), "Stop", "sx", "/repo/w/x", `"background_tasks":[]`)
	r.poll(hm(0, 8))
	if got := r.heldAt("x", hm(0, 8)); got != "" {
		t.Errorf("after its next Stop listing nothing: CloseHeld %q, want none", got)
	}
}

// [LANECLOSE-2-S3] A claude listed by the panel's first poll, or one that started before
// the panel, has had turns the panel never heard: with no Stop heard since, it is held
// in the stall's words; one that started after the panel, never prompted, is not.
func TestCloseHeldWithNoTurnSinceThePanelStarted(t *testing.T) {
	t.Parallel()
	open := func(t *testing.T) *stallRig {
		r := newStallRig(t, hm(0, 10), "")
		r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1}}, []types.LaneRecord{launched("x", "sx", "/repo/w/x")}, "", nil, hm(0, 10))
		return r
	}
	t.Run("listed by the first poll", func(t *testing.T) {
		t.Parallel()
		r := open(t)
		r.list("sx", "/repo/w/x", "idle")
		r.poll(hm(0, 10))
		if got := r.heldAt("x", hm(0, 10)); got != CloseNoTurnWords {
			t.Errorf("CloseHeld %q, want %q", got, CloseNoTurnWords)
		}
		r.hook(hm(0, 11), "Stop", "sx", "/repo/w/x", `"background_tasks":[]`)
		r.poll(hm(0, 12))
		if got := r.heldAt("x", hm(0, 12)); got != "" {
			t.Errorf("after a Stop listing nothing: CloseHeld %q, want none", got)
		}
	})
	t.Run("its last Stop restored from before the panel", func(t *testing.T) {
		t.Parallel()
		// The panel before this one heard a Stop listing nothing and saved it. While no
		// panel ran, the same claude took a turn that started a shell; both hooks lost.
		was := closeRig(t, "") // that panel started at 00:00, before its claude
		was.list("sx", "/repo/w/x", "idle")
		was.proc("sx", 500, hm(0, 0))
		was.poll(hm(0, 1))
		was.hook(hm(0, 2), "Stop", "sx", "/repo/w/x", `"background_tasks":[],"session_crons":[]`)
		was.poll(hm(0, 3))
		saved := roundTrip(t, was.m.StallState(hm(0, 5)))
		r := newStallRig(t, hm(1, 0), "")
		r.m.ApplyStallState(saved)
		r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1}}, []types.LaneRecord{launched("x", "sx", "/repo/w/x")}, "", nil, hm(1, 0))
		r.list("sx", "/repo/w/x", "idle")
		r.proc("sx", 500, hm(0, 0))
		r.poll(hm(1, 0))
		r.poll(hm(1, 1))
		if got := r.heldAt("x", hm(1, 1)); got != CloseNoTurnWords {
			t.Errorf("CloseHeld %q, want %q", got, CloseNoTurnWords)
		}
		r.hook(hm(1, 2), "Stop", "sx", "/repo/w/x", `"background_tasks":[],"session_crons":[]`)
		r.poll(hm(1, 3))
		if got := r.heldAt("x", hm(1, 3)); got != "" {
			t.Errorf("after a Stop heard listing nothing: CloseHeld %q, want none", got)
		}
	})
	t.Run("started before the panel, listed later", func(t *testing.T) {
		t.Parallel()
		r := open(t)
		r.poll(hm(0, 10))
		r.list("sx", "/repo/w/x", "idle")
		r.proc("sx", 300, hm(0, 5))
		r.poll(hm(0, 11))
		if got := r.heldAt("x", hm(0, 11)); got != CloseNoTurnWords {
			t.Errorf("CloseHeld %q, want %q", got, CloseNoTurnWords)
		}
	})
	t.Run("listed by the first poll, started after the panel", func(t *testing.T) {
		t.Parallel()
		r := open(t)
		r.list("sx", "/repo/w/x", "idle")
		r.proc("sx", 300, hm(0, 10).Add(time.Second))
		r.poll(hm(0, 11))
		if got := r.heldAt("x", hm(0, 11)); got != "" {
			t.Errorf("CloseHeld %q, want none", got)
		}
	})
	t.Run("started after the panel, never prompted", func(t *testing.T) {
		t.Parallel()
		r := open(t)
		r.poll(hm(0, 10))
		r.list("sx", "/repo/w/x", "idle")
		r.poll(hm(0, 11))
		if got := r.heldAt("x", hm(0, 11)); got != "" {
			t.Errorf("CloseHeld %q, want none", got)
		}
	})
}

// [LANECLOSE-2-S3] The owner's ruling of 2026-10-25: a wakeup the last Stop scheduled
// (a recurring cron, no tasks) is background work and holds the lane until a later Stop
// lists none.
func TestCloseHeldByAScheduledWakeup(t *testing.T) {
	t.Parallel()
	r := closeRig(t, "")
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.hook(hm(0, 1), "Stop", "sx", "/repo/w/x", `"background_tasks":[],"session_crons":[{"id":"c1","schedule":"*/10 * * * *","recurring":true,"prompt":"check the build"}]`)
	r.poll(hm(0, 2))
	if got := r.heldAt("x", hm(0, 2)); got != CloseWakeupWords {
		t.Errorf("a recurring cron: CloseHeld %q, want %q", got, CloseWakeupWords)
	}
	r.hook(hm(0, 3), "Stop", "sx", "/repo/w/x", `"background_tasks":[],"session_crons":[]`)
	r.poll(hm(0, 4))
	if got := r.heldAt("x", hm(0, 4)); got != "" {
		t.Errorf("after a Stop listing none: CloseHeld %q, want none", got)
	}
}

// [LANECLOSE-2-S3] An idle lane whose own session's last Stop listed background work
// that may still run is held naming it; ended or unknown work does not hold it.
func TestCloseHeldByBackgroundWork(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, tasks string
		subStop     bool
		want        string
	}{
		{"running subagent", reviewTask, false, "its turn ended on background work: Review group 2"},
		{"pending workflow", `"background_tasks":[{"id":"w1","type":"workflow","status":"pending","description":"build-change"}]`, false, "its turn ended on background work: build-change"},
		{"running shell", `"background_tasks":[{"id":"b1","type":"shell","status":"running","description":"go test ./..."}]`, false, "its turn ended on background work: go test ./..."},
		{"teammate, no status", `"background_tasks":[{"id":"t1","type":"teammate","status":"","description":"the reviewer"}]`, false, "its turn ended on background work: the reviewer"},
		{"subagent ended", reviewTask, true, ""},
		{"dream", `"background_tasks":[{"id":"d1","type":"dream","status":"running","description":"dreaming"}]`, false, ""},
		{"nothing", `"background_tasks":[]`, false, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := closeRig(t, "")
			r.list("sx", "/repo/w/x", "idle")
			r.poll(hm(0, 0))
			r.hook(hm(0, 1), "Stop", "sx", "/repo/w/x", c.tasks)
			if c.subStop {
				r.hook(hm(0, 1), "SubagentStop", "sx", "/repo/w/x", `"agent_id":"a1","agent_type":"reviewer"`)
			}
			r.poll(hm(0, 2))
			if got := r.heldAt("x", hm(0, 2)); got != c.want {
				t.Errorf("CloseHeld %q, want %q", got, c.want)
			}
		})
	}
}

// [LANECLOSE-2-S3] A running subagent holds the lane past the stall's unheard limit,
// where the stall itself no longer counts it as working.
func TestCloseHeldSubagentPastUnheard(t *testing.T) {
	t.Parallel()
	r := closeRig(t, "")
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.hook(hm(0, 1), "Stop", "sx", "/repo/w/x", reviewTask)
	at := hm(0, 2).Add(stallUnheard * r.m.stall.time)
	r.poll(at)
	if r.m.sessionWorking(r.m.sessions["sx"], at) {
		t.Fatal("the stall still counts the session working: the unheard limit has not passed")
	}
	if got, want := r.heldAt("x", at), "its turn ended on background work: Review group 2"; got != want {
		t.Errorf("CloseHeld %q, want %q", got, want)
	}
}

// [LANECLOSE-2-S3] A lane bound to a queue's lease, held or waited for, is held naming
// it whatever claude is doing, an exited claude included; not once the lease is gone.
func TestCloseHeldByLease(t *testing.T) {
	t.Parallel()
	r := closeRig(t, gateQueue)
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	const want = "it holds or waits for the gate lease"
	r.m.ApplyQueues(gateHeld("x", true), nil, hm(0, 0))
	if got := r.heldAt("x", hm(0, 0)); got != want {
		t.Errorf("holder: CloseHeld %q, want %q", got, want)
	}
	r.m.ApplyQueues([]types.QueueView{{ID: "gate", Held: true, Holder: &types.LeaseView{Nonce: "n-o", PID: 7, Lane: "other", Alive: true},
		Waiters: []types.LeaseView{{Nonce: "n-x", PID: 8, Lane: "x", Alive: true}}}}, nil, hm(0, 0))
	if got := r.heldAt("x", hm(0, 0)); got != want {
		t.Errorf("waiter: CloseHeld %q, want %q", got, want)
	}
	r.m.ApplyQueues(gateFree(), nil, hm(0, 0))
	if got := r.heldAt("x", hm(0, 0)); got != "" {
		t.Errorf("lease gone: CloseHeld %q, want none", got)
	}
	r.m.ApplyQueues(gateHeld("x", true), nil, hm(0, 1))
	r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1, Dead: true, DeadStatus: "0"}}, []types.LaneRecord{launched("x", "sx", "/repo/w/x")}, "", nil, hm(0, 1))
	r.poll(hm(0, 1))
	if got := r.heldAt("x", hm(0, 1)); got != want {
		t.Errorf("exited holder: CloseHeld %q, want %q", got, want)
	}
}

// [LANECLOSE-2-S3] A lease names a registered lane by its id even while the lane's tmux
// session is gone, and the worktree's name is not the id: the lane is held naming it.
func TestCloseHeldByLeaseOfALaneWithNoSession(t *testing.T) {
	t.Parallel()
	r := closeRig(t, gateQueue)
	r.m.ApplyTmux(nil, []types.LaneRecord{launched("lx", "slx", "/repo/w/x")}, "", nil, hm(0, 1))
	r.m.ApplyQueues(gateHeld("lx", true), nil, hm(0, 1))
	if got, want := r.heldAt("lx", hm(0, 1)), "it holds or waits for the gate lease"; got != want {
		t.Errorf("CloseHeld %q, want %q", got, want)
	}
	r.m.ApplyQueues(gateFree(), nil, hm(0, 2))
	if got := r.heldAt("lx", hm(0, 2)); got != "" {
		t.Errorf("lease gone: CloseHeld %q, want none", got)
	}
}

// The narrow read a close re-checks under its lane lock gives the verdict the view
// gives, a lease's included (it reads the leases over the same lanes), and the gone
// words for a lane the panel no longer lists.
func TestLaneCloseHeldReadsTheVerdictTheViewGives(t *testing.T) {
	t.Parallel()
	r := closeRig(t, gateQueue)
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 1))
	r.hook(hm(0, 1), "Stop", "sx", "/repo/w/x", reviewTask)
	r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1}}, []types.LaneRecord{launched("x", "sx", "/repo/w/x"), launched("lx", "slx", "/repo/w/lx")}, "", nil, hm(0, 1))
	r.m.ApplyQueues(gateHeld("lx", true), nil, hm(0, 1))
	for _, id := range []string{"x", "lx"} {
		want := r.heldAt(id, hm(0, 2))
		if want == "" {
			t.Fatalf("lane %s: the case needs a held lane", id)
		}
		if got := r.m.LaneCloseHeld(id, hm(0, 2), nil); got != want {
			t.Errorf("lane %s: LaneCloseHeld %q, the view's %q", id, got, want)
		}
	}
	if got, want := r.m.LaneCloseHeld("gone", hm(0, 2), nil), LaneGoneWords("gone"); got != want {
		t.Errorf("a lane not listed: %q, want %q", got, want)
	}
}

// [LANECLOSE-2-S1] A lane whose session is known only from a PreToolUse hook, with no
// agents reading, is held as not read (sessionStatus answers idle, not approximate),
// and released by an agents reading of idle.
func TestCloseHeldUntilRead(t *testing.T) {
	t.Parallel()
	r := closeRig(t, "")
	r.hook(hm(0, 0), "PreToolUse", "sx", "/repo/w/x", `"tool_name":"Bash","tool_input":{"command":"ls"}`)
	tv := r.m.TerminalViews(hm(0, 0))[0]
	if tv.Status != "idle" || tv.Approx {
		t.Fatalf("status %q approx %v: the case needs sessionStatus's unread idle", tv.Status, tv.Approx)
	}
	if got, want := tv.CloseHeld, "claude has not been read yet"; got != want {
		t.Errorf("CloseHeld %q, want %q", got, want)
	}
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 1))
	if got := r.heldAt("x", hm(0, 1)); got != "" {
		t.Errorf("read idle: CloseHeld %q, want none", got)
	}
}

// [LANECLOSE-2-S1] A lane whose session ApplyAgents ended (dropped from the reading,
// as after a /clear) is held in the ended words; "starting" once the session is
// forgotten; released when a newer reading lists the session idle again.
func TestCloseHeldEndedByTheReading(t *testing.T) {
	t.Parallel()
	ended := func(t *testing.T) *stallRig {
		r := closeRig(t, "")
		r.list("sx", "/repo/w/x", "idle")
		r.hook(hm(0, 0), "Stop", "sx", "/repo/w/x", `"background_tasks":[]`)
		r.poll(hm(0, 0))
		delete(r.agents, "sx")
		r.poll(hm(0, 1))
		if got := r.heldAt("x", hm(0, 1)); got != CloseEndedWords {
			t.Fatalf("ended: CloseHeld %q, want %q", got, CloseEndedWords)
		}
		return r
	}
	t.Run("forgotten", func(t *testing.T) {
		t.Parallel()
		r := ended(t)
		r.poll(hm(0, 40))
		if _, ok := r.m.sessions["sx"]; ok {
			t.Fatal("the session is not forgotten")
		}
		if got, want := r.heldAt("x", hm(0, 40)), "claude is starting"; got != want {
			t.Errorf("forgotten: CloseHeld %q, want %q", got, want)
		}
	})
	t.Run("listed again", func(t *testing.T) {
		t.Parallel()
		r := ended(t)
		r.list("sx", "/repo/w/x", "idle")
		r.poll(hm(0, 2))
		if r.m.sessions["sx"].HookStatus != "ended" {
			t.Fatal("the case needs a session listed again with its ended mark")
		}
		if got := r.heldAt("x", hm(0, 2)); got != "" {
			t.Errorf("listed again: CloseHeld %q, want none", got)
		}
	})
}

// [LANECLOSE-2-S1] A SessionEnd hook (reason clear) after an idle reading holds the
// lane before the next poll, its old idle reading still current.
func TestCloseHeldEndedByTheHook(t *testing.T) {
	t.Parallel()
	r := closeRig(t, "")
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	at := hm(0, 0).Add(5 * time.Second)
	r.hook(at, "SessionEnd", "sx", "/repo/w/x", `"reason":"clear"`)
	if a := r.m.agentReading(r.m.sessions["sx"], at); a == nil || a.Status != "idle" {
		t.Fatalf("reading %+v: the case needs the old idle reading still current", a)
	}
	if got := r.heldAt("x", at); got != CloseEndedWords {
		t.Errorf("CloseHeld %q, want %q", got, CloseEndedWords)
	}
}

// [LANECLOSE-2-S3] Only the lane's own session is read: another session working in
// the same worktree does not hold it.
func TestCloseHeldReadsOnlyItsOwnSession(t *testing.T) {
	t.Parallel()
	r := closeRig(t, "")
	r.list("sx", "/repo/w/x", "idle")
	r.list("sy", "/repo/w/x", "busy")
	r.poll(hm(0, 0))
	r.hook(hm(0, 0), "Stop", "sy", "/repo/w/x", reviewTask)
	r.poll(hm(0, 1))
	if got := r.heldAt("x", hm(0, 1)); got != "" {
		t.Errorf("CloseHeld %q, want none", got)
	}
}

// [LANECLOSE-2-S1] Task 1.4: a poll under way when the session ended (a /clear) still
// lists the old session idle, so it does not lift the hold; the next poll, started
// after the end, does.
func TestCloseHeldPollStraddlingTheEnd(t *testing.T) {
	t.Parallel()
	r := closeRig(t, "")
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	start := hm(0, 1)
	r.hook(start.Add(5*time.Second), "SessionEnd", "sx", "/repo/w/x", `"reason":"clear"`)
	r.pollTimed(10*time.Second, start.Add(10*time.Second)) // started at start, before the end
	if got := r.heldAt("x", start.Add(10*time.Second)); got != CloseEndedWords {
		t.Errorf("straddling poll: CloseHeld %q, want %q", got, CloseEndedWords)
	}
	r.pollTimed(time.Second, start.Add(15*time.Second)) // started 9 s after the end
	if got := r.heldAt("x", start.Add(15*time.Second)); got != "" {
		t.Errorf("poll after the end: CloseHeld %q, want none", got)
	}
}

// [LANECLOSE-2-S3] Task 1.4: a session listed again after it ended keeps its last
// Stop's work for the verdict (s.end drops the stall's), until a later Stop lists none.
func TestCloseHeldKeepsTheStopPastTheEnd(t *testing.T) {
	t.Parallel()
	r := closeRig(t, "")
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	r.hook(hm(0, 1), "Stop", "sx", "/repo/w/x", reviewTask)
	r.poll(hm(0, 1))
	delete(r.agents, "sx")
	r.poll(hm(0, 2))
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 3))
	if got, want := r.heldAt("x", hm(0, 3)), "its turn ended on background work: Review group 2"; got != want {
		t.Errorf("listed again: CloseHeld %q, want %q", got, want)
	}
	r.hook(hm(0, 4), "Stop", "sx", "/repo/w/x", `"background_tasks":[]`)
	r.poll(hm(0, 4))
	if got := r.heldAt("x", hm(0, 4)); got != "" {
		t.Errorf("a later Stop with none: CloseHeld %q, want none", got)
	}
}

// [LANECLOSE-2-S3] Task 1.6: with queues declared, an unread or failing lease source
// holds every lane, an exited one included; a good read binding no lease releases them.
func TestCloseHeldUntilTheLeasesAreRead(t *testing.T) {
	t.Parallel()
	r := newStallRig(t, hm(0, 0), gateQueue, signals.Worktree{Path: "/repo", Branch: "main", Head: headY},
		signals.Worktree{Path: "/repo/w/x", Branch: "change/x", Head: headX}, signals.Worktree{Path: "/repo/w/y", Branch: "change/y", Head: headY})
	recs := []types.LaneRecord{launched("x", "sx", "/repo/w/x"), launched("y", "sy", "/repo/w/y")}
	r.m.ApplyTmux([]types.TmuxLane{{ID: "x", Path: "/repo/w/x", Created: 1}, {ID: "y", Path: "/repo/w/y", Created: 1, Dead: true, DeadStatus: "0"}}, recs, "", nil, hm(0, 0))
	r.poll(hm(0, 0)) // x's claude starts after the panel (closeRig)
	r.list("sx", "/repo/w/x", "idle")
	r.poll(hm(0, 0))
	check := func(when, want string) {
		t.Helper()
		for _, id := range []string{"x", "y"} {
			if got := r.heldAt(id, hm(0, 0)); got != want {
				t.Errorf("%s: lane %s CloseHeld %q, want %q", when, id, got, want)
			}
		}
	}
	check("before the first read", "the queues' leases have not been read")
	r.m.ApplyQueues(nil, errors.New("lease dir unreadable"), hm(0, 0))
	check("after a failed first read", "the queues' leases have not been read")
	r.m.ApplyQueues(gateFree(), nil, hm(0, 0))
	check("after a good read", "")
	r.m.ApplyQueues(nil, errors.New("lease dir unreadable"), hm(0, 0))
	check("after a failed read", "the queues' leases are not answering")
	r.m.ApplyQueues(gateFree(), nil, hm(0, 0))
	check("after a good read again", "")
}
