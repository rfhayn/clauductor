package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
)

// PANEL-57 task 3.1: `clauductor lane close` against a stub panel, a Unix-socket server
// answering canned plans, results and refusals, found through a pid file and an
// owner.json in a temporary home, with the process check replaced. Each test moves into
// its directory with t.Chdir and sets CLAUDUCTOR_LANE, so none runs in parallel.

// stubPID and stubStart are the stub panel's process, as the replaced check reports it.
const (
	stubPID   = 424242
	stubStart = "Sat Oct 25 09:00:00 2026"
)

// stubAnswer is what the stub answers to one request: a status and the JSON body.
type stubAnswer struct {
	status int
	body   any
}

func planOK(p lanes.ClosePlan) stubAnswer {
	return stubAnswer{http.StatusOK, map[string]any{"ok": true, "plan": p}}
}

func resultOK(r lanes.CloseResult) stubAnswer {
	return stubAnswer{http.StatusOK, map[string]any{"ok": true, "result": r}}
}

func refusal(status int, code, msg string) stubAnswer {
	return stubAnswer{status, map[string]any{"ok": false, "error": msg, "code": code}}
}

// stubReq is one request the stub panel received.
type stubReq struct {
	path string
	body lanes.CloseRequest
}

// stubPanel is the running panel as the command sees it.
type stubPanel struct {
	t    *testing.T
	sock string
	// dry answers the dry run, close the close.
	dry, close stubAnswer

	mu    sync.Mutex
	reqs  []stubReq
	conns int
}

func (s *stubPanel) requests() []stubReq {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]stubReq(nil), s.reqs...)
}

func (s *stubPanel) connections() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conns
}

// ServeHTTP decodes the body as the route does (task 3.2: lanes.CloseRequest, unknown
// fields refused) and, for a close, asserts the consent is the plan's offers: branch the
// plan's deleteBranch, worktree its worktree.
func (s *stubPanel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req lanes.CloseRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		s.t.Errorf("the close body does not decode as lanes.CloseRequest: %v", err)
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.reqs = append(s.reqs, stubReq{path: r.URL.Path, body: req})
	s.mu.Unlock()
	if r.Method != http.MethodPost {
		s.t.Errorf("method %s, want POST", r.Method)
	}
	a := s.dry
	if !req.DryRun {
		a = s.close
		// The command closes only after a plan, so a close with no plan in the dry
		// answer is itself the fault; skipping the check there would hide it.
		m, _ := s.dry.body.(map[string]any)
		if p, ok := m["plan"].(lanes.ClosePlan); !ok {
			s.t.Errorf("a close was asked (worktree=%v branch=%v) when the dry run's answer %v holds no plan", req.Worktree, req.Branch, s.dry.body)
		} else if req.Branch != p.DeleteBranch || req.Worktree != p.Worktree {
			s.t.Errorf("the close's consent is worktree=%v branch=%v, want the plan's worktree=%v deleteBranch=%v",
				req.Worktree, req.Branch, p.Worktree, p.DeleteBranch)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(a.status)
	json.NewEncoder(w).Encode(a.body)
}

// newStubPanel serves a stub panel on a short Unix socket (sun_path is small) until the
// test ends, and replaces the process check: the stub's pid is alive with its start.
func newStubPanel(t *testing.T, dry, close stubAnswer) *stubPanel {
	t.Helper()
	dir, err := os.MkdirTemp("", "lc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	s := &stubPanel{t: t, sock: filepath.Join(dir, "s"), dry: dry, close: close}
	ln, err := net.Listen("unix", s.sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(s)
	srv.Listener.Close()
	srv.Listener = ln
	srv.Config.ConnState = func(_ net.Conn, st http.ConnState) {
		if st == http.StateNew {
			s.mu.Lock()
			s.conns++
			s.mu.Unlock()
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	liveProc(t, true)
	return s
}

// liveProc replaces the command's process check: the stub's pid alive or not.
func liveProc(t *testing.T, alive bool) {
	t.Helper()
	old := laneProc
	t.Cleanup(func() { laneProc = old })
	laneProc = func(pid int) (bool, string) {
		if alive && pid == stubPID {
			return true, stubStart
		}
		return false, ""
	}
}

// claimAs writes the stub panel's pid file, port marker and owner.json into home.
func claimAs(t *testing.T, home string, o install.PanelOwner) {
	t.Helper()
	if err := install.ClaimPanelFiles(home, o); err != nil {
		t.Fatal(err)
	}
}

// registerProjects writes home's registry: each root under its id, the first the default.
func registerProjects(t *testing.T, home string, idRoots ...string) {
	t.Helper()
	reg := &config.Projects{Version: config.ProjectsVersion}
	for i := 0; i+1 < len(idRoots); i += 2 {
		reg.Projects = append(reg.Projects, config.ProjectEntry{ID: idRoots[i], Root: idRoots[i+1], TmuxSocket: "clauductor-" + idRoots[i]})
	}
	if len(reg.Projects) > 0 {
		reg.Default = reg.Projects[0].ID
	}
	if err := config.SaveProjects(home, reg); err != nil {
		t.Fatal(err)
	}
}

// gitIn runs git in dir, failing the test if it fails.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	c.Env = append(gitFreeEnv(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

// laneWorld is a home with project alpha registered and served by a stub panel, and
// the current directory alpha's main checkout.
type laneWorld struct {
	home, root string
	stub       *stubPanel
}

func newLaneWorld(t *testing.T, dry, close stubAnswer) *laneWorld {
	t.Helper()
	home := t.TempDir()
	root := catchUpRepo(t, "alpha")
	registerProjects(t, home, "alpha", root)
	stub := newStubPanel(t, dry, close)
	claimAs(t, home, install.PanelOwner{PID: stubPID, PStart: stubStart, Port: 1, Projects: []string{root}, Socket: stub.sock})
	t.Chdir(root)
	t.Setenv("CLAUDUCTOR_LANE", "")
	// The command's git, without a hook's GIT_DIR (laneRun's comment).
	old := laneRun
	t.Cleanup(func() { laneRun = old })
	laneRun = gitFreeRunner
	return &laneWorld{home: home, root: root, stub: stub}
}

// run is `clauductor lane close id [--yes]` in the current directory: its output and error.
func (w *laneWorld) run(t *testing.T, id string, yes bool) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := runLaneClose(context.Background(), &out, func() (string, error) { return w.home, nil }, id, yes)
	return out.String(), err
}

var mergedPlan = lanes.ClosePlan{
	ID: "a", Running: true, Registered: true, Branch: "change/a",
	Worktree: true, DeleteBranch: true,
	Remove: []string{"the worktree /r/.claude/worktrees/a: clean", "the branch change/a: merged into main"},
	Keep:   []string{"the conversation: claude --resume abc"},
	Notes:  []string{"claude is stopped first, as Stop does."},
	Merged: "merged into main",
}

func wantRequests(t *testing.T, got []stubReq, want ...stubReq) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("the panel was asked %v, want %v", got, want)
	}
}

var (
	dryReq   = stubReq{"/api/p/alpha/lanes/a/close", lanes.CloseRequest{DryRun: true}}
	closeReq = stubReq{"/api/p/alpha/lanes/a/close", lanes.CloseRequest{Worktree: true, Branch: true}}
)

// [LANECLOSE-1-S1] Without --yes the plan is printed (its first line, Removes, Keeps
// and notes), exit 0, and the panel is asked for the dry run alone.
func TestLaneClosePrintsThePlanAndAsksNothingMore(t *testing.T) {
	w := newLaneWorld(t, planOK(mergedPlan), resultOK(lanes.CloseResult{}))
	out, err := w.run(t, "a", false)
	if err != nil {
		t.Fatalf("exit 1: %v", err)
	}
	want := "lane a on change/a\n" +
		"Removes:\n  - the worktree /r/.claude/worktrees/a: clean\n  - the branch change/a: merged into main\n" +
		"Keeps:\n  - the conversation: claude --resume abc\n" +
		"claude is stopped first, as Stop does.\n"
	if out != want {
		t.Errorf("printed:\n%s\nwant:\n%s", out, want)
	}
	wantRequests(t, w.stub.requests(), dryReq)
}

// [LANECLOSE-1-S2] With --yes the panel is asked for the dry run, then the close with
// the plan's offers as the consent (task 3.2: the plan's deleteBranch sent as branch,
// its worktree as worktree), and the Removed lines are printed, exit 0.
func TestLaneCloseYesClosesWithThePlansOffers(t *testing.T) {
	removed := []string{"the worktree /r/.claude/worktrees/a", "the branch change/a"}
	for _, offer := range []struct{ worktree, branch bool }{{true, true}, {true, false}, {false, true}, {false, false}} {
		t.Run(fmt.Sprintf("worktree=%v,deleteBranch=%v", offer.worktree, offer.branch), func(t *testing.T) {
			p := mergedPlan
			p.Worktree, p.DeleteBranch = offer.worktree, offer.branch
			w := newLaneWorld(t, planOK(p), resultOK(lanes.CloseResult{ID: "a", Removed: removed}))
			out, err := w.run(t, "a", true)
			if err != nil {
				t.Fatalf("exit 1: %v", err)
			}
			if want := "Closed lane a.\nRemoved:\n  - the worktree /r/.claude/worktrees/a\n  - the branch change/a\n"; !strings.HasSuffix(out, want) {
				t.Errorf("printed:\n%s\nwant it to end:\n%s", out, want)
			}
			if !strings.HasPrefix(out, "lane a on change/a\n") {
				t.Errorf("the plan is not printed first:\n%s", out)
			}
			wantRequests(t, w.stub.requests(), dryReq,
				stubReq{"/api/p/alpha/lanes/a/close", lanes.CloseRequest{Worktree: offer.worktree, Branch: offer.branch}})
		})
	}
}

// [LANECLOSE-1-S3] A close whose result keeps the worktree and the branch prints the
// Kept lines with their reasons, exit 0.
func TestLaneClosePrintsWhatItKeptAndWhy(t *testing.T) {
	kept := []string{"the worktree /r/.claude/worktrees/a: it has an untracked file (notes.txt)", "the branch change/a: its worktree stays"}
	w := newLaneWorld(t, planOK(mergedPlan), resultOK(lanes.CloseResult{ID: "a", Kept: kept}))
	out, err := w.run(t, "a", true)
	if err != nil {
		t.Fatalf("exit 1: %v", err)
	}
	if want := "Closed lane a.\nKept:\n  - " + kept[0] + "\n  - " + kept[1] + "\n"; !strings.HasSuffix(out, want) || strings.Contains(out, "Removed:") {
		t.Errorf("printed:\n%s\nwant it to end:\n%s", out, want)
	}
	wantRequests(t, w.stub.requests(), dryReq, closeReq)
}

// [LANECLOSE-1-S4] [LANECLOSE-2-S1] [LANECLOSE-2-S3] [LANECLOSE-2-S4] A refusal of the
// dry run is printed as the panel wrote it, with no plan, exit 1, dry run or --yes; the
// close is never asked for.
func TestLaneCloseRefusalsArePrintedAsThePanelWroteThem(t *testing.T) {
	for _, c := range []struct {
		name string
		ans  stubAnswer
	}{
		{"LANECLOSE-1-S4 unknown lane", refusal(404, "not-found", `no lane "a"; this project's lanes: b (change/b), c (fix/12-c)`)},
		{"LANECLOSE-2-S1 working", refusal(409, "held", "lane a is not closed: claude is working")},
		{"LANECLOSE-2-S1 not read", refusal(409, "held", "lane a is not closed: claude has not been read yet")},
		{"LANECLOSE-2-S3 background work", refusal(409, "held", "lane a is not closed: its turn ended on background work: run the gate")},
		{"LANECLOSE-2-S3 lease", refusal(409, "held", "lane a is not closed: it holds or waits for the gate lease")},
		{"LANECLOSE-2-S4 unmerged", refusal(409, "not-merged", "lane a is not closed: its branch is not merged: change/a has 1 commit main lacks")},
		{"LANECLOSE-2-S4 unknown branch", refusal(409, "not-merged", "lane a is not closed: its branch is not known")},
	} {
		for _, yes := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s yes=%v", c.name, yes), func(t *testing.T) {
				w := newLaneWorld(t, c.ans, resultOK(lanes.CloseResult{ID: "a"}))
				out, err := w.run(t, "a", yes)
				msg := c.ans.body.(map[string]any)["error"].(string)
				if err == nil || err.Error() != msg {
					t.Errorf("error %v, want exit 1 with the panel's words %q", err, msg)
				}
				if out != "" {
					t.Errorf("printed %q, want no plan", out)
				}
				wantRequests(t, w.stub.requests(), dryReq)
			})
		}
	}
}

// [LANECLOSE-2-S1] A lane that starts working between the plan and the close: the plan
// is printed, then the close's refusal, exit 1.
func TestLaneCloseRefusalOfTheCloseFollowsThePlan(t *testing.T) {
	w := newLaneWorld(t, planOK(mergedPlan), refusal(409, "held", "lane a is not closed: claude is working"))
	out, err := w.run(t, "a", true)
	if err == nil || err.Error() != "lane a is not closed: claude is working" {
		t.Errorf("error %v, want the close's refusal", err)
	}
	if !strings.HasPrefix(out, "lane a on change/a\n") || strings.Contains(out, "Closed lane") {
		t.Errorf("printed:\n%s\nwant the plan and no result", out)
	}
	wantRequests(t, w.stub.requests(), dryReq, closeReq)
}

// [LANECLOSE-1-S5] Run from a linked worktree (git worktree add) and from a
// subdirectory, the command asks about the main checkout's project. Run from a
// repository no project names, from one the running panel does not serve, or from a
// directory in no repository, it is refused naming `clauductor panel list`, and the
// panel is asked nothing.
func TestLaneCloseAsksAboutTheMainCheckoutsProject(t *testing.T) {
	w := newLaneWorld(t, planOK(mergedPlan), resultOK(lanes.CloseResult{}))
	// Another registered project the panel serves, so the match is by root, not the default.
	beta := catchUpRepo(t, "beta")
	registerProjects(t, w.home, "beta", beta, "alpha", w.root)
	claimAs(t, w.home, install.PanelOwner{PID: stubPID, PStart: stubStart, Port: 1, Projects: []string{beta, w.root}, Socket: w.stub.sock})

	linked := filepath.Join(t.TempDir(), "a")
	gitIn(t, w.root, "worktree", "add", "-q", "-b", "change/a", linked)
	sub := filepath.Join(w.root, "deep", "er")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	linkedSub := filepath.Join(linked, ".clauductor")
	for _, dir := range []string{linked, linkedSub, sub} {
		t.Chdir(dir)
		before := len(w.stub.requests())
		if _, err := w.run(t, "a", false); err != nil {
			t.Fatalf("from %s: exit 1: %v", dir, err)
		}
		if got := w.stub.requests()[before:]; len(got) != 1 || got[0].path != "/api/p/alpha/lanes/a/close" {
			t.Errorf("from %s the panel was asked %v, want alpha's lane a", dir, got)
		}
	}

	unregistered := catchUpRepo(t, "gamma")
	unserved := catchUpRepo(t, "delta")
	registerProjects(t, w.home, "beta", beta, "alpha", w.root, "delta", unserved)
	nowhere := t.TempDir()
	for _, dir := range []string{unregistered, unserved, nowhere} {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Chdir(dir)
			before := w.stub.connections()
			out, err := w.run(t, "a", true)
			if err == nil || !strings.Contains(err.Error(), "`clauductor panel list`") {
				t.Errorf("from %s: %v, want a refusal naming `clauductor panel list`", dir, err)
			}
			if out != "" || w.stub.connections() != before {
				t.Errorf("from %s: printed %q and connected %d times, want nothing asked", dir, out, w.stub.connections()-before)
			}
		})
	}
}

// [LANECLOSE-2-S2] An id equal to CLAUDUCTOR_LANE is refused before anything is read
// or the panel is asked, naming the page's Close lane and auto-close, exit 1, with or
// without --yes: the process check is never called, a corrupt registry is not reported,
// and run from a directory in no project the refusal is the same.
func TestLaneCloseRefusesTheCallersOwnLane(t *testing.T) {
	w := newLaneWorld(t, planOK(mergedPlan), resultOK(lanes.CloseResult{}))
	laneProc = func(int) (bool, string) {
		t.Error("the process check was called")
		return false, ""
	}
	if err := os.WriteFile(config.ProjectsPath(w.home), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDUCTOR_LANE", "a")
	for _, dir := range []string{w.root, t.TempDir()} {
		for _, yes := range []bool{false, true} {
			t.Chdir(dir)
			out, err := w.run(t, "a", yes)
			if err == nil || !strings.Contains(err.Error(), "does not close itself") || !strings.Contains(err.Error(), "Close lane") ||
				!strings.Contains(err.Error(), "auto-close where auto-close is on for the lane's type") {
				t.Errorf("from %s, yes=%v: %v, want the self refusal naming Close lane and auto-close", dir, yes, err)
			}
			if out != "" {
				t.Errorf("printed %q", out)
			}
		}
	}
	if n := w.stub.connections(); n != 0 {
		t.Errorf("the panel was connected to %d times, want none", n)
	}

	// Another lane's id is not refused for it.
	liveProc(t, true)
	registerProjects(t, w.home, "alpha", w.root)
	t.Chdir(w.root)
	if _, err := w.run(t, "b", false); err != nil {
		t.Errorf("another lane's plan, from lane a: %v, want it asked for", err)
	}
}

// [LANECLOSE-2-S2] The real command (as main runs it) exits 1 on its own lane, saying so.
func TestLaneCloseCommandExitsOneOnItsOwnLane(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	r := cliEnv(t, t.TempDir(), home, []string{"CLAUDUCTOR_LANE=a"}, "lane", "close", "a", "--yes")
	if r.code != 1 || !strings.Contains(r.stderr, "does not close itself") || r.stdout != "" {
		t.Errorf("exit %d\nstdout: %s\nstderr: %s\nwant exit 1 with the self refusal", r.code, r.stdout, r.stderr)
	}
	if _, err := os.Stat(config.PanelDir(home)); !os.IsNotExist(err) {
		t.Errorf("the panel directory was made: %v", err)
	}
}

// [LANECLOSE-2-S2] The self refusal comes before $HOME is read: with HOME unset, the
// command on its own lane still gives the fixed refusal, not a home-directory error.
func TestLaneCloseRefusesItsOwnLaneWithNoHome(t *testing.T) {
	t.Parallel()
	r := cliEnv(t, t.TempDir(), t.TempDir(), []string{"HOME=", "CLAUDUCTOR_LANE=a"}, "lane", "close", "a")
	if r.code != 1 || !strings.Contains(r.stderr, "does not close itself") || !strings.Contains(r.stderr, "Close lane") || r.stdout != "" {
		t.Errorf("exit %d\nstdout: %s\nstderr: %s\nwant exit 1 with the self refusal", r.code, r.stdout, r.stderr)
	}
}

// [LANECLOSE-3-S2] With no live panel and a socket file a killed panel left (here, a
// stub still serving it, which would answer), the command says no panel runs, names
// `clauductor panel`, connects to nothing and exits 1.
func TestLaneCloseWithNoLivePanelConnectsToNothing(t *testing.T) {
	w := newLaneWorld(t, planOK(mergedPlan), resultOK(lanes.CloseResult{}))
	liveProc(t, false)
	out, err := w.run(t, "a", true)
	if err == nil || !strings.Contains(err.Error(), "no clauductor panel is running") || !strings.Contains(err.Error(), "`clauductor panel`") {
		t.Errorf("%v, want it to say no panel runs, naming `clauductor panel`", err)
	}
	if out != "" {
		t.Errorf("printed %q", out)
	}
	if fi, serr := os.Lstat(w.stub.sock); serr != nil || fi.Mode()&os.ModeSocket == 0 {
		t.Fatalf("the socket file is not there: %v", serr)
	}
	if n := w.stub.connections(); n != 0 {
		t.Errorf("connected to the socket %d times, want none", n)
	}
}

// [LANECLOSE-3-S4] A live panel whose owner.json names no socket, and a panel found by
// /healthz alone (which names none), are both refused for the socket, not the project
// (the command runs from a directory in no project), exit 1.
func TestLaneCloseRefusesAPanelWithNoSocket(t *testing.T) {
	w := newLaneWorld(t, planOK(mergedPlan), resultOK(lanes.CloseResult{}))
	t.Chdir(t.TempDir())
	check := func(what string) {
		t.Helper()
		out, err := w.run(t, "a", true)
		if err == nil || !strings.Contains(err.Error(), "has no command socket") || !strings.Contains(err.Error(), "`clauductor panel`") ||
			strings.Contains(err.Error(), "panel list") {
			t.Errorf("%s: %v, want it refused for the socket, naming `clauductor panel`", what, err)
		}
		if out != "" {
			t.Errorf("%s: printed %q", what, out)
		}
	}
	claimAs(t, w.home, install.PanelOwner{PID: stubPID, PStart: stubStart, Port: 1, Projects: []string{w.root}})
	check("owner.json with no socket")

	hz := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			fmt.Fprintf(rw, "ok pid=%d\n", stubPID)
			return
		}
		http.NotFound(rw, r)
	}))
	t.Cleanup(hz.Close)
	u, _ := url.Parse(hz.URL)
	port, _ := strconv.Atoi(u.Port())
	// No start time: RunningPanel cannot compare it, so it asks the marker's port.
	claimAs(t, w.home, install.PanelOwner{PID: stubPID, Port: port, Socket: w.stub.sock})
	if o := install.RunningPanel(context.Background(), w.home, os.Getpid(), laneProc); o == nil || o.Socket != "" {
		t.Fatalf("the /healthz panel: %+v, want one found with no socket", o)
	}
	check("a panel found by /healthz alone")
	if n := w.stub.connections(); n != 0 {
		t.Errorf("connected to the socket %d times, want none", n)
	}
}

// A live panel whose socket does not answer: the command says so, naming `clauductor
// panel`, and exits 1.
func TestLaneCloseSaysWhenTheSocketDoesNotAnswer(t *testing.T) {
	w := newLaneWorld(t, planOK(mergedPlan), resultOK(lanes.CloseResult{}))
	gone := filepath.Join(filepath.Dir(w.stub.sock), "gone")
	claimAs(t, w.home, install.PanelOwner{PID: stubPID, PStart: stubStart, Port: 1, Projects: []string{w.root}, Socket: gone})
	out, err := w.run(t, "a", true)
	if err == nil || !strings.Contains(err.Error(), "does not answer") || !strings.Contains(err.Error(), "`clauductor panel`") {
		t.Errorf("%v, want it to say the socket does not answer, naming `clauductor panel`", err)
	}
	if out != "" {
		t.Errorf("printed %q", out)
	}
}
