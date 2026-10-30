package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rfhayn/stridal/internal/panel/clock"
	"github.com/rfhayn/stridal/internal/panel/lanes"
	"github.com/rfhayn/stridal/internal/panel/signals"
	"github.com/rfhayn/stridal/internal/panel/state"
	"github.com/rfhayn/stridal/internal/panel/types"
)

// cliRig is a project served through CLIHandler: testLaneManager's lanes, with a runner
// that answers git and gh from the rig's fixtures and records every call, and a hub
// whose view the test sets. The manager finds no tmux server, so a close forgets a
// registered lane; the view, not the manager, says whether claude runs.
type cliRig struct {
	t      *testing.T
	s      *Server
	lm     *lanes.LaneManager
	hub    *Hub
	h      http.Handler
	out    bytes.Buffer
	mu     sync.Mutex
	ran    []string
	lanes  []cliLane
	gone   map[string]bool // worktree path → removed
	merged map[string]bool // branch → merged into the base
	onRun  func(ctx context.Context, argv []string)
}

type cliLane struct {
	id, branch string
	worktree   bool // its worktree is in `git worktree list`
	tmux       bool // the view lists its tmux session
	dead       bool // its program exited
	status     string
}

const cliSHA = "0123456789abcdef0123456789abcdef01234567"

func newCLIRig(t *testing.T, ls ...cliLane) *cliRig {
	t.Helper()
	r := &cliRig{t: t, lm: testLaneManager(t), lanes: ls, gone: map[string]bool{}, merged: map[string]bool{}}
	r.lm.Exec = func(_ context.Context, argv []string) ([]byte, error) {
		r.record(argv)
		return nil, errors.New("tmux: no server running on /x")
	}
	r.lm.Run = func(ctx context.Context, _ string, argv []string) ([]byte, error) {
		r.record(argv)
		r.mu.Lock()
		on := r.onRun
		r.mu.Unlock()
		if on != nil {
			on(ctx, argv)
		}
		return r.answer(argv)
	}
	for _, l := range ls {
		rec := types.LaneRecord{ID: l.id, SessionID: "s-" + l.id, Path: r.wt(l.id), Type: "build", Branch: l.branch, Action: "start", ActionDone: true}
		if err := r.lm.Registry.Put(rec); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	m := state.NewModel(r.lm.Cfg, "/repo", now)
	r.hub = NewHub(m, clock.Func(time.Now))
	// The panel's first poll lists nothing: the lanes' claudes start after the panel, so
	// it hears all their turns (state's liveBeforePanel).
	r.hub.Update(func(m *state.Model, now time.Time) { m.ApplyAgents(nil, nil, now) })
	r.setView()
	r.s, _ = newTestServer(t)
	r.s.Projects = []*Project{{ID: "a", Name: "A", Hub: r.hub, Lanes: r.lm, Refresh: func(bool) {}}}
	r.s.Default = "a"
	r.h = r.s.CLIHandler(t.Context(), &r.out)
	return r
}

func (r *cliRig) wt(id string) string { return filepath.Join(r.lm.Cfg.WorktreeRoot("/repo"), id) }

func (r *cliRig) record(argv []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ran = append(r.ran, strings.Join(argv, " "))
}

// calls is every command the manager ran, git, gh and tmux alike.
func (r *cliRig) calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ran...)
}

// ranLike is whether a call starting with prefix was made.
func (r *cliRig) ranLike(prefix string) bool {
	for _, c := range r.calls() {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}

func (r *cliRig) answer(argv []string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cmd := strings.Join(argv, " ")
	switch {
	case cmd == "git worktree list --porcelain":
		var b strings.Builder
		b.WriteString("worktree /repo\nHEAD " + cliSHA + "\nbranch refs/heads/main\n\n")
		for _, l := range r.lanes {
			if l.worktree && !r.gone[r.wt(l.id)] {
				b.WriteString("worktree " + r.wt(l.id) + "\nHEAD " + cliSHA + "\nbranch refs/heads/" + l.branch + "\n\n")
			}
		}
		return []byte(b.String()), nil
	case strings.HasPrefix(cmd, "git worktree remove "):
		r.gone[argv[len(argv)-1]] = true
		return nil, nil
	case strings.HasPrefix(cmd, "git rev-parse --verify --quiet refs/heads/"):
		return []byte(cliSHA + "\n"), nil
	case strings.HasPrefix(cmd, "git for-each-ref --merged="):
		ref := argv[len(argv)-1]
		if r.merged[strings.TrimPrefix(ref, "refs/heads/")] {
			return []byte(cliSHA + "\n"), nil
		}
		return nil, nil
	case argv[0] == "gh":
		return []byte("[]"), nil
	case argv[0] == "git":
		return nil, nil // fetch, status (clean), update-ref, config
	}
	return nil, errors.New("unexpected command " + cmd)
}

// setView applies the rig's lanes to the hub's model: their worktrees, tmux sessions,
// registry records and a current `claude agents` reading of each running one.
func (r *cliRig) setView() {
	r.hub.Update(func(m *state.Model, now time.Time) {
		wts := []signals.Worktree{{Path: "/repo", Branch: "main", Head: cliSHA}}
		var tl []types.TmuxLane
		var agents []signals.Agent
		for _, l := range r.lanes {
			if l.worktree && !r.gone[r.wt(l.id)] {
				wts = append(wts, signals.Worktree{Path: r.wt(l.id), Branch: l.branch, Head: cliSHA})
			}
			if l.tmux {
				tl = append(tl, types.TmuxLane{ID: l.id, Path: r.wt(l.id), Created: 1, Dead: l.dead})
				st := l.status
				if st == "" {
					st = "idle"
				}
				agents = append(agents, signals.Agent{SessionID: "s-" + l.id, Cwd: r.wt(l.id), Status: st, PID: 100 + len(agents)})
			}
		}
		m.ApplyWorktrees(wts, nil, now)
		m.ApplyTmux(tl, r.lm.Registry.List(), "", nil, now)
		m.ApplyAgents(agents, nil, now)
	})
}

// setStatus gives lane id's claude a new status in the view.
func (r *cliRig) setStatus(id, status string) {
	r.mu.Lock()
	for i := range r.lanes {
		if r.lanes[i].id == id {
			r.lanes[i].status = status
		}
	}
	r.mu.Unlock()
	r.setView()
}

func (r *cliRig) post(ctx context.Context, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body)).WithContext(ctx)
	w := httptest.NewRecorder()
	r.h.ServeHTTP(w, req)
	return w
}

func (r *cliRig) close(id, body string) *httptest.ResponseRecorder {
	return r.post(r.t.Context(), "POST", "/api/p/a/lanes/"+id+"/close", body)
}

const closeAll = `{"worktree":true,"branch":true}`

// answerOf is the JSON body of an answer.
func answerOf(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var a map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
		t.Fatalf("answer %d %q: %v", w.Code, w.Body.String(), err)
	}
	return a
}

// untouched fails unless lane id's registry record, worktree and branch are as they were.
func (r *cliRig) untouched(id, branch string) {
	r.t.Helper()
	if rec, ok := r.lm.Registry.Get(id); !ok || rec.Action != "start" {
		r.t.Errorf("lane %s's registry record changed: %+v %v (a stop begins by marking it)", id, rec, ok)
	}
	if r.ranLike("git worktree remove") || r.ranLike("git update-ref") {
		r.t.Errorf("a refused close removed something: %q", r.calls())
	}
}

// [LANECLOSE-3-S1] The socket's handler serves the close alone: any other path or
// method, the page's state and a lane's stop among them, is 404 or 405, and the close
// needs no cookie and no Origin.
func TestCLIHandlerServesOnlyTheClose(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t, cliLane{id: "x", branch: "change/x", worktree: true, tmux: true})
	r.merged["change/x"] = true
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/state"},
		{"GET", "/api/p/a/state"},
		{"GET", "/"},
		{"GET", "/events"},
		{"POST", "/api/p/a/lanes/x/stop"},
		{"POST", "/api/lanes/x/stop"},
		{"POST", "/api/lanes/x/close"}, // the page's unprefixed close
		{"POST", "/api/p/a/lanes"},
		{"POST", "/api/p/a/worktrees/remove"},
		{"GET", "/api/p/a/lanes/x/close"},
		{"DELETE", "/api/p/a/lanes/x/close"},
	} {
		w := r.post(t.Context(), c.method, c.path, `{"dryRun":true}`)
		if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: %d %q, want 404 or 405", c.method, c.path, w.Code, w.Body.String())
		}
	}
	if got := r.calls(); len(got) != 0 {
		t.Errorf("the other routes ran %q", got)
	}
	if w := r.close("x", `{"dryRun":true}`); w.Code != http.StatusOK {
		t.Fatalf("the close route, with no cookie and no Origin: %d %q, want 200", w.Code, w.Body.String())
	}
}

// [LANECLOSE-2-S1] [LANECLOSE-2-S3] A lane whose view holds it (claude working, or idle
// on a turn that ended on a running subagent) is refused 409 in the view's words before
// the manager is asked anything, for a dry run as for a close.
func TestCLICloseRefusesAHeldLane(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, status, stop, want string
	}{
		{"working", "busy", "", "lane x is not closed: claude is working"},
		{"background work", "idle", `"background_tasks":[{"id":"a1","type":"subagent","status":"running","description":"Review group 2"}]`,
			"lane x is not closed: its turn ended on background work: Review group 2"},
	} {
		for _, body := range []string{`{"dryRun":true}`, closeAll} {
			r := newCLIRig(t, cliLane{id: "x", branch: "change/x", worktree: true, tmux: true, status: c.status})
			r.merged["change/x"] = true
			if c.stop != "" {
				ev, err := signals.ParseHook([]byte(`{"hook_event_name":"Stop","session_id":"s-x","cwd":"` + r.wt("x") + `",` + c.stop + `}`))
				if err != nil {
					t.Fatal(err)
				}
				r.hub.Update(func(m *state.Model, now time.Time) { m.ApplyHook(ev, now) })
				r.setView() // the next poll still reads it idle
			}
			w := r.close("x", body)
			a := answerOf(t, w)
			if w.Code != http.StatusConflict || a["code"] != "held" || a["error"] != c.want {
				t.Errorf("%s, %s: %d %v, want 409 held, %q", c.name, body, w.Code, a, c.want)
			}
			if got := r.calls(); len(got) != 0 {
				t.Errorf("%s, %s: the manager ran %q for a held lane", c.name, body, got)
			}
			r.untouched("x", "change/x")
		}
	}
}

// [LANECLOSE-2-S4] An idle lane on a branch not merged is refused naming it, with
// nothing removed; the same lane with its program exited is closed, its branch kept.
func TestCLICloseRefusesUnmergedUnlessExited(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t, cliLane{id: "x", branch: "change/x", worktree: true, tmux: true})
	w := r.close("x", closeAll)
	a := answerOf(t, w)
	want := "lane x is not closed: its branch is not merged: it is not merged into origin/main and has no merged pull request"
	if w.Code != http.StatusConflict || a["code"] != "not-merged" || a["error"] != want {
		t.Fatalf("idle, unmerged: %d %v, want 409 %q", w.Code, a, want)
	}
	r.untouched("x", "change/x")
	if r.out.Len() != 0 {
		t.Errorf("a refused close wrote %q", r.out.String())
	}

	r.lanes[0].dead = true
	r.setView()
	w = r.close("x", closeAll)
	if w.Code != http.StatusOK {
		t.Fatalf("exited: %d %q, want 200", w.Code, w.Body.String())
	}
	var res struct{ Result lanes.CloseResult }
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if _, ok := r.lm.Registry.Get("x"); ok {
		t.Error("the exited lane's registry record stayed")
	}
	if !r.ranLike("git worktree remove " + r.wt("x")) {
		t.Errorf("its clean worktree was not removed: %q", r.calls())
	}
	if r.ranLike("git update-ref") || !strings.Contains(strings.Join(res.Result.Kept, "\n"), "the local branch change/x: it is not merged") {
		t.Errorf("the unmerged branch was not kept, saying why: kept %q, calls %q", res.Result.Kept, r.calls())
	}
	if line := r.out.String(); !strings.HasPrefix(line, "lane x (a) closed by stridal lane close: removed ") || !strings.Contains(line, "; kept the local branch change/x") {
		t.Errorf("the panel's output %q, want the close's line", line)
	}
}

// [LANECLOSE-2-S4] A lane the view shows exited, whose claude tmux shows running when the close acts (a
// Restart since the panel's last tmux poll), is refused under the lane lock, with
// nothing stopped or removed: the exit that waives the merged rule is a stale one.
func TestCLICloseOfAnExitedLaneRestartedSinceThePoll(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t, cliLane{id: "x", branch: "change/x", worktree: true, tmux: true, dead: true})
	r.lm.Exec = func(_ context.Context, argv []string) ([]byte, error) {
		r.record(argv)
		if strings.Contains(strings.Join(argv, " "), " list-panes ") {
			return []byte("x\t" + r.wt("x") + "\t" + r.wt("x") + "\t0\t\t1\t0\tbuild\n"), nil
		}
		return nil, errors.New("unexpected tmux call " + strings.Join(argv, " "))
	}
	w := r.close("x", closeAll)
	a := answerOf(t, w)
	want := "lane x is not closed: " + state.CloseRunsAgainWords
	if w.Code != http.StatusConflict || a["code"] != "held" || a["error"] != want {
		t.Fatalf("%d %v, want 409 %q", w.Code, a, want)
	}
	r.untouched("x", "change/x")
	for _, c := range r.calls() {
		if strings.Contains(c, " kill-session ") || strings.Contains(c, " send-keys ") {
			t.Errorf("a refused close stopped the restarted claude: %q", c)
		}
	}
	if r.out.Len() != 0 {
		t.Errorf("a refused close wrote %q", r.out.String())
	}
}

// [LANECLOSE-2-S4] An idle lane whose plan has no branch (its worktree gone) is
// refused "its branch is not known".
func TestCLICloseRefusesAnUnknownBranch(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t, cliLane{id: "x", branch: "change/x", worktree: false, tmux: true})
	w := r.close("x", closeAll)
	a := answerOf(t, w)
	if w.Code != http.StatusConflict || a["error"] != "lane x is not closed: its branch is not known" {
		t.Fatalf("%d %v, want 409 \"its branch is not known\"", w.Code, a)
	}
	r.untouched("x", "change/x")
}

// [LANECLOSE-2-S4] A registered lane whose tmux session is gone is refused on a branch
// not merged, so it keeps its restore, and closed on a merged one.
func TestCLICloseOfALaneWithNoSession(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t, cliLane{id: "x", branch: "change/x", worktree: true}, cliLane{id: "y", branch: "change/y", worktree: true})
	r.merged["change/y"] = true
	w := r.close("x", closeAll)
	if a := answerOf(t, w); w.Code != http.StatusConflict || a["code"] != "not-merged" {
		t.Fatalf("x, unmerged: %d %v, want 409 not-merged", w.Code, a)
	}
	r.untouched("x", "change/x")
	w = r.close("y", closeAll)
	if w.Code != http.StatusOK {
		t.Fatalf("y, merged: %d %q, want 200", w.Code, w.Body.String())
	}
	if _, ok := r.lm.Registry.Get("y"); ok || !r.ranLike("git worktree remove "+r.wt("y")) || !r.ranLike("git update-ref -d refs/heads/change/y") {
		t.Errorf("y was not closed: record %v, calls %q", ok, r.calls())
	}
	if _, ok := r.lm.Registry.Get("x"); !ok {
		t.Error("closing y forgot x")
	}
}

// [LANECLOSE-2-S1] The verdict is read again under the lane lock: a lane clear when
// asked whose claude starts work during the plan (the fetch sets the view) is refused
// 409 with nothing removed.
func TestCLICloseReadsTheVerdictAgainUnderTheLock(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t, cliLane{id: "x", branch: "change/x", worktree: true, tmux: true})
	r.merged["change/x"] = true
	r.onRun = func(_ context.Context, argv []string) {
		if strings.Join(argv, " ") == "git fetch --quiet" {
			r.setStatus("x", "busy") // as a poll applied meanwhile would
		}
	}
	w := r.close("x", closeAll)
	a := answerOf(t, w)
	if w.Code != http.StatusConflict || a["code"] != "held" || !strings.Contains(a["error"].(string), "claude is working") {
		t.Fatalf("%d %v, want 409 held, \"claude is working\"", w.Code, a)
	}
	r.untouched("x", "change/x")
}

// [LANECLOSE-2-S4] The exited exemption is read again under the lane lock: a lane on an
// unmerged branch that read exited when asked, and runs again by the time the close
// holds the lock (the fetch sets the view), is refused with nothing removed.
func TestCLICloseReadsTheExitAgainUnderTheLock(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t, cliLane{id: "x", branch: "change/x", worktree: true, tmux: true, dead: true})
	r.onRun = func(_ context.Context, argv []string) {
		if strings.Join(argv, " ") == "git fetch --quiet" {
			r.mu.Lock()
			r.lanes[0].dead = false // restarted, idle, as a poll applied meanwhile would show
			r.mu.Unlock()
			r.setView()
		}
	}
	w := r.close("x", closeAll)
	a := answerOf(t, w)
	want := "lane x is not closed: its branch is not merged: it is not merged into origin/main and has no merged pull request"
	if w.Code != http.StatusConflict || a["error"] != want {
		t.Fatalf("%d %v, want 409 %q", w.Code, a, want)
	}
	r.untouched("x", "change/x")
}

// [LANECLOSE-1-S4] An id the view does not list is refused naming it and the project's
// lanes, each with its branch, and nothing runs.
func TestCLICloseOfAnUnknownLane(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t, cliLane{id: "a", branch: "change/a", worktree: true, tmux: true}, cliLane{id: "b", branch: "fix/12-b", worktree: true, tmux: true})
	w := r.close("zz", closeAll)
	a := answerOf(t, w)
	const want = `no lane "zz"; this project's lanes: a (change/a), b (fix/12-b)`
	if w.Code != http.StatusNotFound || a["error"] != want {
		t.Fatalf("%d %v, want 404 %q", w.Code, a, want)
	}
	if got := r.calls(); len(got) != 0 {
		t.Errorf("the manager ran %q for an unknown lane", got)
	}
	empty := newCLIRig(t)
	w = empty.close("zz", closeAll)
	if a := answerOf(t, w); w.Code != http.StatusNotFound || a["error"] != `no lane "zz"; this project has no lanes` {
		t.Fatalf("no lanes: %d %v, want 404 %q", w.Code, a, `no lane "zz"; this project has no lanes`)
	}
}

// [LANECLOSE-4-S1] A close whose request is cancelled after the plan, and before the
// close begins, changes nothing: the last plan call cancels it and still answers as a
// merged branch's would, so only the cancellation's check can refuse it.
func TestCLICloseCancelledBeforeItBegins(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t, cliLane{id: "x", branch: "change/x", worktree: true, tmux: true})
	r.merged["change/x"] = true
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r.onRun = func(_ context.Context, argv []string) {
		if strings.HasPrefix(strings.Join(argv, " "), "git for-each-ref --merged=") {
			cancel() // the plan's last call: the branch's tip is in the base
		}
	}
	w := r.post(ctx, "POST", "/api/p/a/lanes/x/close", closeAll)
	a := answerOf(t, w)
	if w.Code != http.StatusServiceUnavailable || a["code"] != "cancelled" || !strings.Contains(a["error"].(string), "nothing changed") {
		t.Fatalf("%d %v, want 503 cancelled, \"nothing changed\"", w.Code, a)
	}
	r.untouched("x", "change/x")
	if r.out.Len() != 0 {
		t.Errorf("a close that did not begin wrote %q", r.out.String())
	}
}

// [LANECLOSE-4-S1] A close whose request is cancelled once it has begun runs to its end
// under the panel's context: the worktree removed, the branch deleted, and the close's
// line written.
func TestCLICloseFinishesWhenTheRequestGoes(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t, cliLane{id: "x", branch: "change/x", worktree: true, tmux: true})
	r.merged["change/x"] = true
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var live []string
	r.onRun = func(c context.Context, argv []string) {
		cmd := strings.Join(argv, " ")
		if strings.HasPrefix(cmd, "git worktree remove ") {
			cancel() // the command is gone mid-close
		}
		if strings.HasPrefix(cmd, "git update-ref") && c.Err() == nil {
			live = append(live, cmd)
		}
	}
	w := r.post(ctx, "POST", "/api/p/a/lanes/x/close", closeAll)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %q, want 200", w.Code, w.Body.String())
	}
	if len(live) != 1 {
		t.Errorf("the branch's deletion under a live context: %q; calls %q", live, r.calls())
	}
	if line := r.out.String(); !strings.Contains(line, "lane x (a) closed by stridal lane close: removed ") || !strings.Contains(line, "the local branch change/x") {
		t.Errorf("the panel's output %q, want the close's line", line)
	}
}
