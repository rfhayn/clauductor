package panel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// fakeAgents stands in for `claude agents --json`, so a test can say what state a
// lane's session is in, or make the list unreadable. git runs for real.
type fakeAgents struct {
	mu  sync.Mutex
	out string
	err error
}

func (f *fakeAgents) set(out string, err error) {
	f.mu.Lock()
	f.out, f.err = out, err
	f.mu.Unlock()
}

func (f *fakeAgents) run(ctx context.Context, dir string, argv []string) ([]byte, error) {
	if argv[0] == "claude" {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.err != nil {
			return nil, f.err
		}
		if f.out == "" {
			return []byte("[]"), nil
		}
		return []byte(f.out), nil
	}
	return gitOnlyRunner(ctx, dir, argv)
}

func agentJSON(sid, status, cwd string) string {
	return fmt.Sprintf(`[{"pid":1,"cwd":%q,"kind":"interactive","sessionId":%q,"status":%q}]`, cwd, sid, status)
}

func startedSession(t *testing.T, body map[string]any) string {
	t.Helper()
	lane, _ := body["lane"].(map[string]any)
	sid, _ := lane["sessionId"].(string)
	if !uuidRe.MatchString(sid) {
		t.Fatalf("no session id in %v", body)
	}
	return sid
}

// F1: Stop types /exit (and Enter) only into a lane claude agents reports idle. A
// waiting lane (a permission or dialog has focus) or an unknown one gets Escape and
// kill-session, and never an Enter that would confirm the dialog's default.
func TestStopNeverPressesEnterUnlessTheLaneIsIdle(t *testing.T) {
	tmux, sock := throwawaySocket(t)
	root, home := rootLaneProject(t)
	keys := filepath.Join(t.TempDir(), "keys")
	agents := &fakeAgents{}
	p := startPanelWith(t, root, home, sock, func(o *Options) {
		o.Runner = agents.run
		// Record every byte the lane receives: raw mode, so Escape and Enter arrive alone.
		o.LaneProgram = []string{"/bin/sh", "-c", "stty raw -echo; exec cat >> " + shq(keys), "lane"}
	})
	received := func() string { b, _ := os.ReadFile(keys); os.Remove(keys); return string(b) }

	for _, c := range []struct {
		name, status string
		err          error
	}{{"waiting", "waiting", nil}, {"busy", "busy", nil}, {"unknown", "", errors.New("claude agents: unreadable")}} {
		code, body := p.post(t, "/api/lanes", StartRequest{Type: "orchestrator", Mode: "root", Name: "lane-" + c.name})
		if code != 200 {
			t.Fatalf("%s: start: %d %v", c.name, code, body)
		}
		// F8: while it runs, no second lane may share its checkout.
		if code, body := p.post(t, "/api/lanes", StartRequest{Type: "orchestrator", Mode: "root", Name: "second"}); code != 409 || body["code"] != "path-taken" {
			t.Fatalf("a second lane in the same checkout: %d %v", code, body)
		}
		agents.set(agentJSON(startedSession(t, body), c.status, root), c.err)
		time.Sleep(300 * time.Millisecond)
		if code, body := p.post(t, "/api/lanes/lane-"+c.name+"/stop", nil); code != 200 {
			t.Fatalf("%s: stop: %d %v", c.name, code, body)
		}
		got := received()
		if strings.ContainsAny(got, "\r\n") || strings.Contains(got, "/exit") || !strings.Contains(got, "\x1b") {
			t.Fatalf("%s lane received %q; want Escape only, never /exit or Enter", c.name, got)
		}
		if hasSession(tmux, sock, "lane-"+c.name) {
			t.Fatalf("%s lane survived stop", c.name)
		}
	}

	code, body := p.post(t, "/api/lanes", StartRequest{Type: "orchestrator", Mode: "root", Name: "lane-idle"})
	if code != 200 {
		t.Fatalf("idle: start: %d %v", code, body)
	}
	agents.set(agentJSON(startedSession(t, body), "idle", root), nil)
	if code, body := p.post(t, "/api/lanes/lane-idle/stop", nil); code != 200 {
		t.Fatalf("idle: stop: %d %v", code, body)
	}
	if got := received(); !strings.HasSuffix(got, "\x15/exit\r") {
		t.Fatalf("idle lane received %q; want C-u, then /exit, then Enter", got)
	}
}

// F2: a resume is refused while claude agents shows another process on the lane's
// session, and refused when that cannot be checked.
func TestResumeRefusesALiveOrUncheckableSession(t *testing.T) {
	tmux, sock := throwawaySocket(t)
	root, home := rootLaneProject(t)
	agents := &fakeAgents{}
	p := startPanelWith(t, root, home, sock, func(o *Options) { o.Runner = agents.run; o.FastExit = 300 * time.Millisecond })
	code, body := p.post(t, "/api/lanes", StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"})
	if code != 200 {
		t.Fatalf("start: %d %v", code, body)
	}
	sid := startedSession(t, body)
	exec.Command(tmux, "-L", sock, "kill-server").Run() // orphan it
	waitFor(t, "orphan", func() bool { v := findTerm(p.state(t), "orch"); return v != nil && !v.Running })

	agents.set(agentJSON(sid, "idle", "/elsewhere"), nil)
	if code, body := p.post(t, "/api/lanes/orch/resume", nil); code != 409 || body["code"] != "session-live" {
		t.Fatalf("resume while another process holds the session: %d %v", code, body)
	}
	agents.set("", errors.New("claude agents: boom"))
	if code, body := p.post(t, "/api/lanes/orch/resume", nil); code != 409 || body["code"] != "unverified" {
		t.Fatalf("resume with claude agents unreadable: %d %v", code, body)
	}
	if hasSession(tmux, sock, "orch") {
		t.Fatal("a refused resume started the lane")
	}
	agents.set("", nil)
	if code, body := p.post(t, "/api/lanes/orch/resume", nil); code != 200 {
		t.Fatalf("resume once the session is free: %d %v", code, body)
	}
}

// F3: a session is marked as having a conversation when claude agents sees it busy
// (hooks can be dropped), and a restart whose flag claude rejects (a fast non-zero
// exit) retries once with the other flag. F5: the panel's socket has no prefix key.
func TestRestartFallsBackWhenClaudeRejectsTheFlag(t *testing.T) {
	tmux, sock := throwawaySocket(t)
	root, home := rootLaneProject(t)
	agents := &fakeAgents{}
	p := startPanelWith(t, root, home, sock, func(o *Options) {
		o.Runner = agents.run
		o.FastExit = 1500 * time.Millisecond
		// Like claude on a session with no conversation: --resume exits 1 at once.
		o.LaneProgram = []string{"/bin/sh", "-c", `case "$*" in *--resume*) exit 1;; esac; exec /bin/sh`, "lane"}
	})
	code, body := p.post(t, "/api/lanes", StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"})
	if code != 200 {
		t.Fatalf("start: %d %v", code, body)
	}
	sid := startedSession(t, body)
	agents.set(agentJSON(sid, "busy", root), nil)
	waitFor(t, "busy in claude agents to mark a conversation", func() bool {
		r, _ := OpenRegistry(home, root)
		rec, _ := r.Get("orch")
		return rec.Conversation
	})
	agents.set("", nil)
	if code, body := p.post(t, "/api/lanes/orch/restart", nil); code != 200 {
		t.Fatalf("restart: %d %v", code, body)
	}
	cmd, _ := exec.Command(tmux, "-L", sock, "display-message", "-p", "-t", "=orch:", "#{pane_dead} #{pane_start_command}").Output()
	if !strings.HasPrefix(string(cmd), "0 ") || !strings.Contains(string(cmd), "--session-id "+sid) {
		t.Fatalf("after the fallback the lane runs %q", cmd)
	}
	r, _ := OpenRegistry(home, root)
	if rec, _ := r.Get("orch"); rec.Conversation || !rec.ActionDone {
		t.Fatalf("registry after the fallback: %+v", rec)
	}
	// F5.
	if out, _ := exec.Command(tmux, "-L", sock, "show-options", "-g", "prefix").Output(); strings.TrimSpace(string(out)) != "prefix None" {
		t.Fatalf("panel socket prefix: %q", out)
	}
	if out, _ := exec.Command(tmux, "-L", sock, "list-keys", "-T", "prefix").CombinedOutput(); strings.Contains(string(out), "bind-key") {
		t.Fatalf("prefix table still bound:\n%s", out)
	}
}

// F4: a terminal that passed auth just before a token rotation is closed as soon as
// it registers, and tickets issued under the old token die with it.
func TestRotationDuringAnUpgradeStillClosesTheTerminal(t *testing.T) {
	tmux, sock := throwawaySocket(t)
	if err := exec.Command(tmux, "-L", sock, "-f", "/dev/null", "new-session", "-d", "-s", "a", "/bin/sh").Run(); err != nil {
		t.Fatal(err)
	}
	s, _ := newTestServer(t)
	s.Lanes = testLaneManager(t)
	s.Lanes.TmuxPath, s.Lanes.Socket = tmux, sock
	ts := httptest.NewUnstartedServer(nil)
	s.Port = ts.Listener.Addr().(*net.TCPAddr).Port
	ts.Config.Handler = s.Handler()
	ts.Start()
	defer ts.Close()
	origin := "http://127.0.0.1:" + strconv.Itoa(s.Port)

	dial := func(ticket, token string) (*websocket.Conn, error) {
		h := http.Header{}
		h.Set("Origin", origin)
		h.Set("Cookie", s.cookieName()+"="+token)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c, _, err := websocket.Dial(ctx, "ws://127.0.0.1:"+strconv.Itoa(s.Port)+"/ws/term?lane=a",
			&websocket.DialOptions{HTTPHeader: h, Subprotocols: []string{TermSubprotocol, ticketPrefix + ticket}})
		return c, err
	}
	old := s.Token
	tk, _ := s.issueTicket("a")
	s.beforeAddViewer = func() { s.beforeAddViewer = nil; s.Rotate(strings.Repeat("b", 64)) }
	c, err := dial(tk, old)
	if err != nil {
		t.Fatalf("the upgrade passed auth before the rotation, so it should connect: %v", err)
	}
	if got := closeCode(c, 3*time.Second); got != closeRotated {
		t.Fatalf("terminal registered after a rotation closed with %v, want %v", got, closeRotated)
	}
	stale, _ := s.issueTicket("a")
	s.Rotate(strings.Repeat("c", 64))
	if _, err := dial(stale, strings.Repeat("c", 64)); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("a ticket issued before a rotation still works: %v", err)
	}
}

// F6: `clauductor panel open` sends the token only to the panel whose PID is in the
// pid file.
func TestOpenURLOnlyTrustsThePanelsPID(t *testing.T) {
	home := t.TempDir()
	if _, err := RotateToken(home); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok pid=999\n") }))
	defer ts.Close()
	port, _ := strconv.Atoi(strings.TrimPrefix(ts.URL, "http://127.0.0.1:"))
	pid := filepath.Join(panelDir(home), "pid")
	os.WriteFile(pid, []byte("123\n"), 0o600)
	if u, err := OpenURL(context.Background(), home, port); err == nil || strings.Contains(u, "t=") {
		t.Fatalf("token sent to an impostor: %q %v", u, err)
	}
	os.WriteFile(pid, []byte("999\n"), 0o600)
	if u, err := OpenURL(context.Background(), home, port); err != nil || !strings.Contains(u, "/?t=") {
		t.Fatalf("the real panel: %q %v", u, err)
	}
	os.Remove(pid)
	if _, err := OpenURL(context.Background(), home, port); err == nil {
		t.Fatal("no pid file, yet the token was handed out")
	}
}

// F7: uninstall removes what the agent owns, and keeps a lane registry only while it
// still lists lanes.
func TestUninstallRemovesPanelFilesButKeepsLiveRegistries(t *testing.T) {
	home := t.TempDir()
	RotateToken(home)
	dir := panelDir(home)
	for _, f := range []string{"browser-opened", "pid", "port", "bin/clauductor", "logs/panel.log"} {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o700)
		os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600)
	}
	empty, _ := OpenRegistry(home, "/empty")
	empty.Put(LaneRecord{ID: "a", SessionID: sid, Path: "/empty", Mode: "root", Action: "start"})
	empty.Delete("a")
	live, _ := OpenRegistry(home, "/live")
	live.Put(LaneRecord{ID: "b", SessionID: sid, Path: "/live", Mode: "root", Action: "start"})
	var out strings.Builder
	if err := Uninstall(home, &out, func(...string) ([]byte, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"token", "browser-opened", "pid", "port", "bin", "logs", filepath.Dir(RegistryPath(home, "/empty"))} {
		p := f
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, f)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s survived uninstall", p)
		}
	}
	if _, err := os.Stat(RegistryPath(home, "/live")); err != nil || !strings.Contains(out.String(), "Kept "+RegistryPath(home, "/live")) {
		t.Fatalf("a registry with lanes was not kept and reported: %v\n%s", err, out.String())
	}
}

// F9: a registry record read from disk is validated before any field can become
// argv. A corrupt one is shown, never launched, and can be forgotten; one without a
// valid lane id is reported.
func TestCorruptRegistryRecordsAreShownButNeverLaunched(t *testing.T) {
	home := t.TempDir()
	path := RegistryPath(home, "/repo")
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, []byte(`{"version":1,"project":"/repo","lanes":[
		{"id":"bad","sessionId":"x; rm -rf ~","path":"/repo","type":"fix","mode":"root","action":"start","actionDone":true},
		{"id":"Not A Lane","sessionId":"`+sid+`","path":"/repo","mode":"root","action":"start"}]}`), 0o600)
	m := testLaneManager(t)
	reg, err := OpenRegistry(home, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	m.Registry = reg
	m.TmuxPath = filepath.Join(t.TempDir(), "tmux")
	os.WriteFile(m.TmuxPath, []byte("#!/bin/sh\necho 'no server running on /tmp/x' >&2\nexit 1\n"), 0o755)
	m.Run = func(ctx context.Context, dir string, argv []string) ([]byte, error) { return []byte("[]"), nil }
	rec, ok := reg.Get("bad")
	if !ok || !strings.Contains(rec.Corrupt, "session id") {
		t.Fatalf("corrupt record: %+v", rec)
	}
	if len(reg.Problems()) != 1 {
		t.Fatalf("problems: %v", reg.Problems())
	}
	if lerr := m.Resume(context.Background(), "bad"); lerr == nil || lerr.Code != "corrupt" {
		t.Fatalf("resume of a corrupt record: %v", lerr)
	}
	model := NewModel(m.Cfg, "/repo", t0)
	model.ApplyWorktrees([]Worktree{{Path: "/repo", Branch: "main"}}, nil, t0)
	model.ApplyTmux(nil, reg.List(), "", nil, t0)
	model.ApplyRegistryProblems(reg.Problems())
	v := model.Snapshot(t0)
	if len(v.Terminals) != 1 || !strings.Contains(v.Terminals[0].Orphan, "corrupt") || len(v.Banners) != 1 {
		t.Fatalf("view: %+v banners %v", v.Terminals, v.Banners)
	}
	if lerr := m.Forget(context.Background(), "bad"); lerr != nil {
		t.Fatalf("forget: %v", lerr)
	}
}

// R3: -f /dev/null only applies when the panel starts the tmux server. A server
// already on the socket, with ~/.tmux.conf's bindings (here a root binding that runs
// a command), is made keyless as soon as the panel finds it, and before any attach.
func TestPanelStripsKeyBindingsFromAServerItDidNotStart(t *testing.T) {
	tmux, sock := throwawaySocket(t)
	marker := filepath.Join(t.TempDir(), "ran") // what the binding would run
	if out, err := exec.Command(tmux, "-L", sock, "-f", "/dev/null", "new-session", "-d", "-s", "user", "/bin/sh",
		";", "bind-key", "-n", "F12", "run-shell", "touch "+shq(marker),
		";", "bind-key", "-T", "prefix", "c", "new-window").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if r, _ := exec.Command(tmux, "-L", sock, "list-keys", "-T", "root").CombinedOutput(); !strings.Contains(string(r), "F12") {
		t.Fatalf("setup: the root binding is not there to strip:\n%s", r)
	}
	root, home := rootLaneProject(t)
	startPanel(t, root, home, sock)
	waitFor(t, "the panel to strip the root and prefix tables", func() bool {
		r, _ := exec.Command(tmux, "-L", sock, "list-keys", "-T", "root").CombinedOutput()
		p, _ := exec.Command(tmux, "-L", sock, "list-keys", "-T", "prefix").CombinedOutput()
		o, _ := exec.Command(tmux, "-L", sock, "show-options", "-g", "prefix").Output()
		// The root table keeps only the wheel binding the panel puts back (PANEL-6).
		rootLeft := strings.TrimSpace(string(r))
		return strings.Count(rootLeft, "bind-key") == 1 && strings.Contains(rootLeft, "WheelUpPane") &&
			!strings.Contains(rootLeft, "F12") && !strings.Contains(string(p), "bind-key") &&
			strings.TrimSpace(string(o)) == "prefix None"
	})
}

// R1: the idle check is repeated right before the Enter. Here the lane turns
// "waiting" (a dialog opened) once /exit has been typed: the Enter must not follow.
func TestStopRechecksIdleBeforeTheEnter(t *testing.T) {
	_, sock := throwawaySocket(t)
	root, home := rootLaneProject(t)
	keys := filepath.Join(t.TempDir(), "keys")
	var mu sync.Mutex
	var sid string
	runner := func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		if argv[0] != "claude" {
			return gitOnlyRunner(ctx, dir, argv)
		}
		mu.Lock()
		defer mu.Unlock()
		if sid == "" {
			return []byte("[]"), nil
		}
		status := "idle"
		if b, _ := os.ReadFile(keys); strings.Contains(string(b), "/exit") {
			status = "waiting"
		}
		return []byte(agentJSON(sid, status, root)), nil
	}
	p := startPanelWith(t, root, home, sock, func(o *Options) {
		o.Runner = runner
		o.LaneProgram = []string{"/bin/sh", "-c", "stty raw -echo; exec cat >> " + shq(keys), "lane"}
	})
	code, body := p.post(t, "/api/lanes", StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"})
	if code != 200 {
		t.Fatalf("start: %d %v", code, body)
	}
	mu.Lock()
	sid = startedSession(t, body)
	mu.Unlock()
	if code, body := p.post(t, "/api/lanes/orch/stop", nil); code != 200 {
		t.Fatalf("stop: %d %v", code, body)
	}
	b, _ := os.ReadFile(keys)
	if got := string(b); strings.ContainsAny(got, "\r\n") || !strings.HasSuffix(got, "/exit\x1b") {
		t.Fatalf("lane received %q; want C-u, /exit, then Escape and no Enter", got)
	}
}
