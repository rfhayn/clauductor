package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/rfhayn/stridal/internal/panel/clock"
	"github.com/rfhayn/stridal/internal/panel/config"
	"github.com/rfhayn/stridal/internal/panel/install"
	"github.com/rfhayn/stridal/internal/panel/lanes"
	"github.com/rfhayn/stridal/internal/panel/signals"
	"github.com/rfhayn/stridal/internal/testgit"
	"github.com/rfhayn/stridal/internal/testwait"
)

// PANEL-57: the command socket in the panel's life (task 2.2), and `clauductor lane
// close`'s client against the real route on it (tasks 2.3, 2.4).

// shortSocket is a socket path short enough for sun_path (E10): a t.TempDir() path on
// macOS is too long for one.
func shortSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "lc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "s")
}

// shortHome is a temporary home whose install.CLISocketPath fits sun_path (E10: 104
// bytes on macOS, 108 on Linux, the terminating NUL included). It tries the temporary
// directory first, and /tmp only when that path is too long (a t.TempDir() home on
// macOS, or one under the TMPDIR leakcheck sets); where neither gives a short enough
// writable home, the test is skipped saying why.
func shortHome(t *testing.T) string {
	t.Helper()
	max := len(syscall.RawSockaddrUnix{}.Path)
	home, err := os.MkdirTemp("", "h")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	n := len(install.CLISocketPath(home))
	if n < max {
		return home
	}
	why := fmt.Sprintf("the socket path in a home under the temporary directory is %d bytes, too long for sun_path (%d)", n, max)
	home, err = os.MkdirTemp("/tmp", "h")
	if err != nil {
		t.Skipf("%s, and /tmp gives no home: %v", why, err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	if n := len(install.CLISocketPath(home)); n >= max {
		t.Skipf("%s, and under /tmp it is %d bytes", why, n)
	}
	return home
}

// deadSocketFile leaves a socket file at path with nothing listening on it, as a killed
// panel leaves one. The socket is bound, never listened on: bind makes the file, and a
// child a parallel test forks meanwhile may inherit the descriptor but cannot accept on
// it, so a dial is refused whatever runs beside the test. (A listener closed with
// SetUnlinkOnClose(false) still answers while such a child holds it.)
//
// The descriptor lives only under syscall.ForkLock held for reading, as testbin holds
// it: every fork takes it for writing, so no child inherits the descriptor at all.
func deadSocketFile(t *testing.T, path string) {
	t.Helper()
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)
	if err := syscall.Bind(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
		t.Fatalf("bind %s: %v", path, err)
	}
}

// outLog is a panel's Out, written by its goroutines and read by the test.
type outLog struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *outLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *outLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// readOwnerFile is owner.json as the panel wrote it, and its raw text.
func readOwnerFile(t *testing.T, home string) (install.PanelOwner, string) {
	t.Helper()
	b, err := os.ReadFile(ownerPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var o install.PanelOwner
	if err := json.Unmarshal(b, &o); err != nil {
		t.Fatalf("owner.json: %v: %s", err, b)
	}
	return o, string(b)
}

// socketDo sends method path on the socket with no cookie and no Origin: its status.
func socketDo(t *testing.T, sock, method, path string) int {
	t.Helper()
	c := &http.Client{Timeout: testwait.Scale(10 * time.Second), Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
		DisableKeepAlives: true,
	}}
	req, _ := http.NewRequest(method, "http://panel"+path, strings.NewReader(`{"dryRun":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s on the socket: %v", method, path, err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

// tcpClose posts a dry-run close to the page's TCP route with the cookie and the Origin
// given ("" for none): its status.
func tcpClose(t *testing.T, p *panelRun, project, cookie, origin string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", p.base+"/api/p/"+project+"/lanes/x/close", strings.NewReader(`{"dryRun":true}`))
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

// laneErrOf is err as the route's refusal, or a failure.
func laneErrOf(t *testing.T, what string, err error) *lanes.LaneError {
	t.Helper()
	var le *lanes.LaneError
	if !errors.As(err, &le) {
		t.Fatalf("%s: %v, want a refusal from the route", what, err)
	}
	return le
}

// [LANECLOSE-3-S1] [LANECLOSE-3-S3] A panel started over a socket file a killed panel
// left serves the close on a socket of its own there: mode 0600, in a directory it made
// 0700, named in owner.json (and still after a live `panel add` rewrites it). On it,
// every other path and method is 404 or 405; the page's close route on the TCP port
// still refuses no cookie, then no Origin. At exit the socket file is gone.
func TestTheCommandSocketLivesWithThePanel(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	root := multiRepo(t, "Alpha", noServerSocket())
	// A directory not private yet, so the 0700 below is the panel's doing.
	dir := filepath.Join(filepath.Dir(shortSocket(t)), "p")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "s")
	// [LANECLOSE-3-S3]'s GIVEN: a socket file with nothing behind it, as a killed panel
	// leaves it.
	deadSocketFile(t, sock)
	if fi, err := os.Lstat(sock); err != nil || fi.Mode()&os.ModeSocket == 0 {
		t.Fatalf("no stale socket planted: %v", err)
	}
	if c, err := net.Dial("unix", sock); err == nil {
		c.Close()
		t.Fatal("the stale socket answers")
	}

	var out outLog
	p := startPanelWith(t, root, home, noServerSocket(), func(o *Options) { o.CLISocket, o.Out = sock, &out })
	project := projectIDOf(t, p)

	fi, err := os.Lstat(sock)
	if err != nil || fi.Mode()&os.ModeSocket == 0 {
		t.Fatalf("no socket at %s: %v", sock, err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("the socket's mode is %v, want 0600", fi.Mode().Perm())
	}
	if di, err := os.Stat(filepath.Dir(sock)); err != nil || di.Mode().Perm() != 0o700 {
		t.Errorf("the socket's directory: %v %v, want mode 0700", di.Mode().Perm(), err)
	}
	if o, _ := readOwnerFile(t, home); o.Socket != sock {
		t.Errorf("owner.json names socket %q, want %q", o.Socket, sock)
	}

	// The panel's own socket serves the close: the route's own refusal for no such lane.
	_, err = install.CloseLane(context.Background(), sock, project, "x", lanes.CloseRequest{DryRun: true})
	if le := laneErrOf(t, "closing an unknown lane", err); le.Status != http.StatusNotFound || le.Code != "not-found" || !strings.Contains(le.Msg, `no lane "x"`) {
		t.Errorf("unknown lane: %+v, want 404 not-found", le)
	}

	// Nothing else on the socket: the page's state, a lane's stop, the close by another
	// method or by the legacy route, the hooks' ingest, the health check.
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/state"},
		{"GET", "/api/p/" + project + "/state"},
		{"POST", "/api/p/" + project + "/lanes/x/stop"},
		{"POST", "/api/lanes/x/close"},
		{"POST", "/hook"},
		{"GET", "/healthz"},
		{"GET", "/"},
	} {
		if code := socketDo(t, sock, c.method, c.path); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s on the socket: %d, want 404 or 405", c.method, c.path, code)
		}
	}
	if code := socketDo(t, sock, "GET", "/api/p/"+project+"/lanes/x/close"); code != http.StatusMethodNotAllowed {
		t.Errorf("GET of the close route on the socket: %d, want 405", code)
	}

	// The page's server keeps its guards on the same route.
	if code := tcpClose(t, p, project, "", p.origin); code != http.StatusUnauthorized {
		t.Errorf("TCP close with no cookie: %d, want 401", code)
	}
	if code := tcpClose(t, p, project, p.cookie, ""); code != http.StatusForbidden {
		t.Errorf("TCP close with no Origin: %d, want 403", code)
	}

	// A live `panel add` rewrites owner.json: the socket stays named.
	rootC := multiRepo(t, "Gamma", noServerSocket())
	ctx := context.Background()
	if _, err := install.AddProject(ctx, install.AddOptions{Home: home, Project: rootC, Run: gitOnlyRunner, Now: clock.System.Now(), Strict: true}); err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithTimeout(ctx, testwait.Scale(10*time.Second))
	defer cancel()
	if !install.WaitPanelServes(wctx, clock.System, home, os.Getpid(), rootC, true) {
		t.Fatal("owner.json does not list gamma")
	}
	if o, _ := readOwnerFile(t, home); o.Socket != sock {
		t.Errorf("after a live add, owner.json names socket %q, want %q", o.Socket, sock)
	}

	p.stop()
	if _, err := os.Lstat(sock); !os.IsNotExist(err) {
		t.Errorf("the socket file is still there after the panel exits: %v", err)
	}
	if strings.Contains(out.String(), "no command socket") {
		t.Errorf("the panel said it has no socket: %s", out.String())
	}
}

// [LANECLOSE-3-S4] A socket path too long for sun_path leaves the panel serving the page,
// with why it has no socket printed and no socket in owner.json.
func TestAPanelThatCannotMakeItsSocketServesAnyway(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	root := multiRepo(t, "Alpha", noServerSocket())
	long := filepath.Join(filepath.Dir(shortSocket(t)), strings.Repeat("x", 200))
	var out outLog
	p := startPanelWith(t, root, home, noServerSocket(), func(o *Options) { o.CLISocket, o.Out = long, &out })
	if v := p.state(t); v.ProjectID == "" {
		t.Fatal("the page's state is not served")
	}
	if s := out.String(); !strings.Contains(s, "no command socket, so `clauductor lane close` cannot reach this panel") || !strings.Contains(s, "a Unix socket's path must be shorter than") {
		t.Errorf("the panel did not say why it has no socket: %q", s)
	}
	if o, raw := readOwnerFile(t, home); o.Socket != "" || strings.Contains(raw, `"socket"`) {
		t.Errorf("owner.json names a socket: %s", raw)
	}
	if _, err := os.Lstat(long); !os.IsNotExist(err) {
		t.Errorf("a file is at the too-long path: %v", err)
	}
}

// [LANECLOSE-3-S4] A file at the socket's path that is not a socket is not the panel's
// to remove: it is left as it was, the panel says why it has no socket, serves the page,
// and owner.json names no socket.
func TestAPanelLeavesAFileAtItsSocketPathThatIsNotASocket(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	root := multiRepo(t, "Alpha", noServerSocket())
	path := shortSocket(t)
	writeFile(t, path, "the owner's notes\n")
	var out outLog
	p := startPanelWith(t, root, home, noServerSocket(), func(o *Options) { o.CLISocket, o.Out = path, &out })
	if v := p.state(t); v.ProjectID == "" {
		t.Fatal("the page's state is not served")
	}
	if s := out.String(); !strings.Contains(s, "no command socket") || !strings.Contains(s, path+" is there and is not a socket, so it is left as it is") {
		t.Errorf("the panel did not say why it has no socket: %q", s)
	}
	if fi, err := os.Lstat(path); err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("the file at the socket's path: %v %v, want it regular", fi, err)
	}
	if b, _ := os.ReadFile(path); string(b) != "the owner's notes\n" {
		t.Errorf("the file at the socket's path now reads %q", b)
	}
	if o, raw := readOwnerFile(t, home); o.Socket != "" || strings.Contains(raw, `"socket"`) {
		t.Errorf("owner.json names a socket: %s", raw)
	}
}

// [LANECLOSE-3-S1] A panel given no CLISocket, as `clauductor panel` starts one, listens
// at install.CLISocketPath(home), names it in owner.json, and answers the close there.
// The home must be short enough for the socket's path (E10): see shortHome.
func TestThePanelListensOnTheDefaultSocketPath(t *testing.T) {
	t.Parallel()
	home := shortHome(t)
	want := install.CLISocketPath(home)
	root := multiRepo(t, "Alpha", noServerSocket())
	var out outLog
	p := startPanelWith(t, root, home, noServerSocket(), func(o *Options) { o.CLISocket, o.Out = "", &out })
	project := projectIDOf(t, p)
	if o, _ := readOwnerFile(t, home); o.Socket != want {
		t.Errorf("owner.json names socket %q, want the default %q (panel said: %s)", o.Socket, want, out.String())
	}
	if fi, err := os.Lstat(want); err != nil || fi.Mode()&os.ModeSocket == 0 || fi.Mode().Perm() != 0o600 {
		t.Errorf("the default socket %s: %v %v, want a socket of mode 0600", want, fi, err)
	}
	_, err := install.CloseLane(context.Background(), want, project, "x", lanes.CloseRequest{DryRun: true})
	if le := laneErrOf(t, "closing an unknown lane on the default socket", err); le.Status != http.StatusNotFound || le.Code != "not-found" {
		t.Errorf("unknown lane on the default socket: %+v, want the route's 404 not-found", le)
	}
	p.stop()
	if _, err := os.Lstat(want); !os.IsNotExist(err) {
		t.Errorf("the default socket is still there after the panel exits: %v", err)
	}
}

// laneFacts is what a close may change of a lane, each read from its own authority.
type laneFacts struct{ Registered, Session, Worktree, Branch bool }

// TestLaneCloseThroughTheSocket drives `clauductor lane close`'s client
// (install.CloseLane) against a panel's socket, lanes on a throwaway tmux socket and
// `claude agents` faked by stallWorld. Each lane's program waits for a file named
// after it, so the test can end one (its claude exiting).
//
// [LANECLOSE-1-S1] the dry run of an idle, clean, merged lane plans its removal and
// changes nothing. [LANECLOSE-1-S2] its close with the plan's consent removes the
// record, the session, the worktree and the branch, answers the Removed lines and
// prints the panel's line. [LANECLOSE-1-S3] a worktree with an untracked file stays
// with its branch, Kept saying why, and the lane is stopped and forgotten.
// [LANECLOSE-2-S1] a lane read busy is refused "claude is working", dry run or not, and
// keeps running; one heard of only from a tool hook makes auto-close ask with "claude
// has not been read yet". [LANECLOSE-2-S3] a lane whose last Stop lists a running
// subagent is refused naming it, auto-close asks about one with the same words, and a
// later Stop listing none lets it close. [LANECLOSE-2-S4] an idle lane on unmerged work
// is refused "its branch is not merged" and is untouched; once its claude exits it
// closes, its branch kept as not merged. [LANECLOSE-4-S1] a close whose command gives up
// during the worktree's teardown is finished by the panel.
func TestLaneCloseThroughTheSocket(t *testing.T) {
	t.Parallel()
	tmux, tsock := throwawaySocket(t)
	root := signals.ResolvePath(t.TempDir())
	home := t.TempDir()
	exitDir, markDir := t.TempDir(), t.TempDir()
	gitRun(t, root, "init", "-q", "-b", "main")
	// The teardown marks each lane it runs for; lane slow's sleeps, so its close is
	// under way when the command gives up.
	// Lane slow's waits for the test to release it, after its command has given up
	// (bounded at a minute, so a failed test leaves no teardown behind for long; the
	// cleanup releases it too).
	releaseDir := t.TempDir()
	t.Cleanup(func() { os.WriteFile(filepath.Join(releaseDir, "slow"), nil, 0o644) })
	teardown := `touch ` + shq(markDir) + `/"$CLAUDUCTOR_LANE"; if [ "$CLAUDUCTOR_LANE" = slow ]; then i=0; ` +
		`while [ ! -e ` + shq(releaseDir) + `/slow ] && [ $i -lt 600 ]; do sleep 0.1; i=$((i+1)); done; fi`
	tdJSON, _ := json.Marshal([]string{"/bin/sh", "-c", teardown})
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"Closes","version":7,"lanes":{"main":"orchestrator","change/":"build"},
		"base":"main","worktree_dir":"w","lanes_auto_close":"on_merge","worktree_teardown":{"command":`+string(tdJSON)+`}}`)
	writeFile(t, filepath.Join(root, ".gitignore"), "w/\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "init")

	w := newStallWorld()
	// Auto-close checks a lane again when its open pull request leaves the open list:
	// these two are listed open until the test merges them.
	w.open["change/bgauto"], w.open["change/unread"] = 11, 12
	run := func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		if c := envCommand(argv); len(c) > 0 && c[0] == "/bin/sh" {
			return signals.ExecRunner(ctx, dir, argv) // the worktree's teardown, for real
		}
		return w.run(ctx, dir, argv)
	}
	sock := shortSocket(t)
	var out outLog
	p := startPanelWith(t, root, home, tsock, func(o *Options) {
		o.CLISocket, o.Out, o.Runner, o.TrustConfig, o.Ticks = sock, &out, run, true, stallTicks()
		o.LaneProgram = []string{"/bin/sh", "-c", `while [ ! -e ` + shq(exitDir) + `/"$(basename "$PWD")" ]; do sleep 0.2; done`, "lane"}
	})
	project := projectIDOf(t, p)
	hook := liveClient{base: p.base, cookie: p.cookie}

	ids := []string{"clean", "dirty", "busy", "bg", "bgauto", "unread", "unmerged", "slow"}
	for _, id := range ids {
		if code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "build", Mode: "new", Name: id}); code != 200 {
			t.Fatalf("start %s: %d %v", id, code, body)
		}
	}
	sid, wt := map[string]string{}, map[string]string{}
	waitFor(t, "every lane running with its session and worktree", func() bool {
		v := p.state(t)
		for _, id := range ids {
			tv := findTerm(v, id)
			if tv == nil || !tv.Running || tv.SessionID == "" || tv.Worktree == "" || tv.Branch != "change/"+id {
				return false
			}
			sid[id], wt[id] = tv.SessionID, tv.Worktree
		}
		return true
	})

	facts := func(id string) laneFacts {
		reg, err := lanes.OpenRegistry(home, root)
		if err != nil {
			t.Fatal(err)
		}
		_, registered := reg.Get(id)
		_, serr := os.Stat(wt[id])
		return laneFacts{Registered: registered, Session: hasSession(tmux, tsock, id), Worktree: serr == nil,
			Branch: testgit.Command(root, "rev-parse", "--verify", "--quiet", "refs/heads/change/"+id).Run() == nil}
	}
	all := laneFacts{true, true, true, true}
	for _, id := range ids {
		if f := facts(id); f != all {
			t.Fatalf("lane %s before any close: %+v", id, f)
		}
	}
	closeLane := func(id string, req lanes.CloseRequest) (install.LaneCloseAnswer, error) {
		ctx, cancel := context.WithTimeout(context.Background(), testwait.Scale(30*time.Second))
		defer cancel()
		return install.CloseLane(ctx, sock, project, id, req)
	}
	outLine := func(id string) string {
		return "lane " + id + " (" + project + ") closed by clauductor lane close: removed "
	}

	// The worktrees' states: an untracked file, and a commit main lacks.
	writeFile(t, filepath.Join(wt["dirty"], "notes.txt"), "draft\n")
	writeFile(t, filepath.Join(wt["unmerged"], "work.txt"), "work\n")
	gitRun(t, wt["unmerged"], "add", "work.txt")
	gitRun(t, wt["unmerged"], "commit", "-q", "-m", "work")

	// The readings: every lane but unread has one, started after the panel (a claude
	// from before it would hold every lane until a turn ends).
	started := time.Now().UnixMilli()
	w.set(func(w *stallWorld) {
		for i, id := range ids {
			if id == "unread" {
				continue
			}
			status := "idle"
			if id == "busy" {
				status = "busy"
			}
			w.agents = append(w.agents, signals.Agent{PID: 5000 + i, Cwd: wt[id], Kind: "interactive", StartedAt: started, SessionID: sid[id], Status: status})
		}
	})
	for _, id := range []string{"bg", "bgauto"} {
		if code := hook.post(t, "/hook", fmt.Sprintf(`{"hook_event_name":"Stop","session_id":%q,"cwd":%q,"last_assistant_message":"waiting on the review",
			"background_tasks":[{"id":"a1","type":"subagent","status":"running","description":"Review group 2"}],"session_crons":[]}`, sid[id], wt[id])); code >= 300 {
			t.Fatalf("Stop hook for %s: %d", id, code)
		}
	}
	if code := hook.post(t, "/hook", fmt.Sprintf(`{"hook_event_name":"PreToolUse","session_id":%q,"cwd":%q,"tool_name":"Agent","tool_use_id":"tu1","tool_input":{"description":"Look"}}`,
		sid["unread"], wt["unread"])); code >= 300 {
		t.Fatalf("PreToolUse hook for unread: %d", code)
	}
	want := map[string]string{"clean": "", "dirty": "", "unmerged": "", "slow": "", "busy": "claude is working",
		"bg": "its turn ended on background work: Review group 2", "bgauto": "its turn ended on background work: Review group 2",
		"unread": "claude has not been read yet"}
	waitFor(t, "each lane's close verdict", func() bool {
		v := p.state(t)
		for id, h := range want {
			if tv := findTerm(v, id); tv == nil || tv.CloseHeld != h {
				return false
			}
		}
		return true
	})

	// [LANECLOSE-2-S3] [LANECLOSE-2-S1] Auto-close asks rather than closes: the pull
	// requests leave the open list, merged at the branches' tips.
	waitFor(t, "both pull requests listed open", func() bool {
		n := 0
		for _, pr := range p.state(t).PRs {
			if pr.HeadRef == "change/bgauto" || pr.HeadRef == "change/unread" {
				n++
			}
		}
		return n == 2
	})
	w.set(func(w *stallWorld) {
		delete(w.open, "change/bgauto")
		delete(w.open, "change/unread")
		w.merged["change/bgauto"] = "21@" + gitRun(t, root, "rev-parse", "refs/heads/change/bgauto")
		w.merged["change/unread"] = "22@" + gitRun(t, root, "rev-parse", "refs/heads/change/unread")
	})
	asks := map[string]string{"bgauto": "Review group 2", "unread": "claude has not been read yet"}
	waitUntil(t, "auto-close asks about bgauto and unread with their verdicts", 20*time.Second, func() bool {
		got := 0
		for _, n := range p.state(t).NeedsYou {
			if why, ok := asks[n.Terminal]; ok && n.Kind == "merged" && strings.Contains(n.Text, why) {
				got++
			}
		}
		return got == 2
	})
	for _, id := range []string{"bgauto", "unread"} {
		if f := facts(id); f != all {
			t.Errorf("[LANECLOSE-2-S3] auto-close changed lane %s: %+v", id, f)
		}
	}

	// [LANECLOSE-1-S1] The dry run plans the removal and changes nothing.
	ans, err := closeLane("clean", lanes.CloseRequest{DryRun: true})
	if err != nil {
		t.Fatalf("[LANECLOSE-1-S1] dry run: %v", err)
	}
	plan := ans.Plan
	if !plan.Worktree || !plan.DeleteBranch || plan.Merged == "" || len(plan.Remove) == 0 {
		t.Fatalf("[LANECLOSE-1-S1] the plan does not remove the worktree and the branch: %+v", plan)
	}
	if f := facts("clean"); f != all {
		t.Fatalf("[LANECLOSE-1-S1] the dry run changed the lane: %+v", f)
	}
	if strings.Contains(out.String(), outLine("clean")) {
		t.Fatal("[LANECLOSE-1-S1] the dry run printed a close")
	}
	// [LANECLOSE-1-S2] The plan's consent, as the command sends it, closes it all.
	ans, err = closeLane("clean", lanes.CloseRequest{Worktree: plan.Worktree, Branch: plan.DeleteBranch})
	if err != nil {
		t.Fatalf("[LANECLOSE-1-S2] close: %v", err)
	}
	removed := strings.Join(ans.Result.Removed, "\n")
	if !strings.Contains(removed, wt["clean"]) || !strings.Contains(removed, "change/clean") {
		t.Errorf("[LANECLOSE-1-S2] Removed lines %q do not name the worktree and the branch", ans.Result.Removed)
	}
	if f := facts("clean"); f != (laneFacts{}) {
		t.Errorf("[LANECLOSE-1-S2] after the close: %+v, want all gone", f)
	}
	if n := strings.Count(out.String(), outLine("clean")); n != 1 {
		t.Errorf("[LANECLOSE-1-S2] the panel printed %d close lines for clean: %q", n, out.String())
	}

	// [LANECLOSE-1-S3] A dirty worktree stays, with its branch, and Kept says why.
	ans, err = closeLane("dirty", lanes.CloseRequest{DryRun: true})
	if err != nil {
		t.Fatalf("[LANECLOSE-1-S3] dry run: %v", err)
	}
	if ans.Plan.Worktree {
		t.Fatalf("[LANECLOSE-1-S3] the plan removes a dirty worktree: %+v", ans.Plan)
	}
	ans, err = closeLane("dirty", lanes.CloseRequest{Worktree: ans.Plan.Worktree, Branch: ans.Plan.DeleteBranch})
	if err != nil {
		t.Fatalf("[LANECLOSE-1-S3] close: %v", err)
	}
	if f := facts("dirty"); f != (laneFacts{Worktree: true, Branch: true}) {
		t.Errorf("[LANECLOSE-1-S3] after the close: %+v, want the worktree and the branch alone", f)
	}
	// Each Kept line with the verdict's own reason, not only the thing it names.
	wtLine, branchLine := false, false
	for _, k := range ans.Result.Kept {
		wtLine = wtLine || (strings.HasPrefix(k, "the worktree "+wt["dirty"]+": ") && strings.Contains(k, "uncommitted or untracked") && strings.Contains(k, "notes.txt"))
		branchLine = branchLine || k == "the branch change/dirty: its worktree stays"
	}
	if !wtLine || !branchLine {
		t.Errorf("[LANECLOSE-1-S3] Kept lines %q: want the worktree kept for its untracked notes.txt, and the branch because its worktree stays", ans.Result.Kept)
	}

	// [LANECLOSE-2-S1] A busy lane is refused, dry run or not, and keeps running.
	for _, dry := range []bool{true, false} {
		_, err := closeLane("busy", lanes.CloseRequest{DryRun: dry, Worktree: true, Branch: true})
		if le := laneErrOf(t, "closing busy", err); le.Status != http.StatusConflict || le.Code != "held" || !strings.Contains(le.Msg, "claude is working") {
			t.Errorf("[LANECLOSE-2-S1] busy (dry run %v): %+v, want 409 held, claude is working", dry, le)
		}
	}
	if f := facts("busy"); f != all {
		t.Errorf("[LANECLOSE-2-S1] the refused lane changed: %+v", f)
	}

	// [LANECLOSE-2-S3] Refused naming the subagent; closes once a later Stop lists none.
	_, err = closeLane("bg", lanes.CloseRequest{Worktree: true, Branch: true})
	if le := laneErrOf(t, "closing bg", err); le.Status != http.StatusConflict || le.Code != "held" || !strings.Contains(le.Msg, "Review group 2") {
		t.Errorf("[LANECLOSE-2-S3] bg: %+v, want 409 held naming the subagent", le)
	}
	if f := facts("bg"); f != all {
		t.Errorf("[LANECLOSE-2-S3] the refused lane changed: %+v", f)
	}
	if code := hook.post(t, "/hook", fmt.Sprintf(`{"hook_event_name":"Stop","session_id":%q,"cwd":%q,"last_assistant_message":"done",
		"background_tasks":[],"session_crons":[]}`, sid["bg"], wt["bg"])); code >= 300 {
		t.Fatalf("second Stop hook for bg: %d", code)
	}
	waitFor(t, "bg's verdict lifts", func() bool {
		tv := findTerm(p.state(t), "bg")
		return tv != nil && tv.CloseHeld == ""
	})
	if _, err := closeLane("bg", lanes.CloseRequest{Worktree: true, Branch: true}); err != nil {
		t.Fatalf("[LANECLOSE-2-S3] bg after a Stop listing nothing: %v", err)
	}
	if f := facts("bg"); f != (laneFacts{}) {
		t.Errorf("[LANECLOSE-2-S3] bg after the close: %+v, want all gone", f)
	}

	// [LANECLOSE-2-S4] Idle on unmerged work: refused and untouched; exited: closed, the
	// branch kept as not merged.
	_, err = closeLane("unmerged", lanes.CloseRequest{Worktree: true, Branch: true})
	if le := laneErrOf(t, "closing unmerged", err); le.Status != http.StatusConflict || le.Code != "not-merged" || !strings.Contains(le.Msg, "its branch is not merged") {
		t.Errorf("[LANECLOSE-2-S4] unmerged: %+v, want 409 not-merged", le)
	}
	if f := facts("unmerged"); f != all {
		t.Errorf("[LANECLOSE-2-S4] the refused lane changed: %+v", f)
	}
	writeFile(t, filepath.Join(exitDir, "unmerged"), "")
	waitFor(t, "unmerged's claude exits, the pane held", func() bool {
		tv := findTerm(p.state(t), "unmerged")
		return tv != nil && tv.Dead && tv.CloseHeld == ""
	})
	ans, err = closeLane("unmerged", lanes.CloseRequest{DryRun: true})
	if err != nil {
		t.Fatalf("[LANECLOSE-2-S4] dry run of the exited lane: %v", err)
	}
	if ans.Plan.DeleteBranch {
		t.Fatalf("[LANECLOSE-2-S4] the plan deletes an unmerged branch: %+v", ans.Plan)
	}
	ans, err = closeLane("unmerged", lanes.CloseRequest{Worktree: ans.Plan.Worktree, Branch: ans.Plan.DeleteBranch})
	if err != nil {
		t.Fatalf("[LANECLOSE-2-S4] close of the exited lane: %v", err)
	}
	if f := facts("unmerged"); f.Registered || f.Session || !f.Branch {
		t.Errorf("[LANECLOSE-2-S4] after the close: %+v, want the lane gone and the branch kept", f)
	}
	if kept := strings.Join(ans.Result.Kept, "\n"); !strings.Contains(kept, "change/unmerged") || !strings.Contains(kept, "not merged") {
		t.Errorf("[LANECLOSE-2-S4] Kept lines %q do not say the branch is not merged", ans.Result.Kept)
	}

	// [LANECLOSE-4-S1] The command gives up once the teardown has begun: the panel
	// finishes the close.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan error, 1)
	go func() {
		_, err := install.CloseLane(ctx, sock, project, "slow", lanes.CloseRequest{Worktree: true, Branch: true})
		errc <- err
	}()
	waitFor(t, "slow's teardown begins", func() bool {
		_, err := os.Stat(filepath.Join(markDir, "slow"))
		return err == nil
	})
	cancel()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("[LANECLOSE-4-S1] the close answered while its teardown was held: the test proves nothing")
		}
	case <-time.After(testwait.Scale(10 * time.Second)):
		t.Fatal("[LANECLOSE-4-S1] the cancelled command did not return")
	}
	// Only now, with the command gone, may the teardown end.
	if err := os.WriteFile(filepath.Join(releaseDir, "slow"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "[LANECLOSE-4-S1] the panel finishes slow's close", 20*time.Second, func() bool {
		return facts("slow") == (laneFacts{}) && strings.Contains(out.String(), outLine("slow"))
	})

	// The lanes no close was asked for run on.
	for _, id := range []string{"busy", "bgauto", "unread"} {
		if f := facts(id); f != all {
			t.Errorf("lane %s changed: %+v", id, f)
		}
	}
}
