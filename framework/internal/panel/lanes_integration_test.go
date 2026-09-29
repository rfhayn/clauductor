package panel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/coder/websocket"
)

// These tests drive real tmux, git and a PTY, with `sh` standing in for claude, on a
// throwaway tmux socket that is killed afterwards. They never touch the panel's
// real socket or the user's own tmux server.

func throwawaySocket(t *testing.T) (string, string) {
	t.Helper()
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not installed")
	}
	b := make([]byte, 4)
	rand.Read(b)
	sock := "clauductor-test-" + strconv.Itoa(os.Getpid()) + "-" + hex.EncodeToString(b)
	t.Cleanup(func() {
		exec.Command(tmux, "-L", sock, "kill-server").Run()
		// kill-server leaves the socket file; tmux keeps it in $TMUX_TMPDIR (or /tmp)/tmux-<uid>.
		dir := os.Getenv("TMUX_TMPDIR")
		if dir == "" {
			dir = "/tmp"
		}
		os.Remove(filepath.Join(dir, "tmux-"+strconv.Itoa(os.Getuid()), sock))
	})
	return tmux, sock
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitOnlyRunner runs git for real and fakes claude and gh, so the test needs neither.
func gitOnlyRunner(ctx context.Context, dir string, argv []string) ([]byte, error) {
	switch argv[0] {
	case "git":
		return signals.ExecRunner(ctx, dir, argv)
	case "claude", "gh":
		return []byte("[]"), nil
	}
	return nil, fmt.Errorf("unexpected command %v", argv)
}

type panelRun struct {
	base, cookie, origin string
	cancel               func()
	done                 chan error
	once                 sync.Once
}

func startPanel(t *testing.T, root, home, sock string) *panelRun {
	t.Helper()
	return startPanelWith(t, root, home, sock, nil)
}

func startPanelWith(t *testing.T, root, home, sock string, tweak func(*Options)) *panelRun {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	o := Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: gitOnlyRunner,
		// sh stands in for claude; the claude flags land in its ignored positional args.
		TmuxSocket: sock, LaneProgram: []string{"/bin/sh", "-c", "exec /bin/sh", "lane"}, StopTimeout: 1500 * time.Millisecond,
		OnReady: func(u string) { ready <- u }}
	if tweak != nil {
		tweak(&o)
	}
	go func() { done <- Run(ctx, o) }()
	select {
	case u := <-ready:
		pu, _ := url.Parse(u)
		port, _ := strconv.Atoi(pu.Port())
		p := &panelRun{base: "http://" + pu.Host, origin: "http://" + pu.Host,
			cookie: fmt.Sprintf("clauductor_panel_%d=%s", port, pu.Query().Get("t")), cancel: cancel, done: done}
		t.Cleanup(p.stop)
		return p
	case err := <-done:
		t.Fatalf("Run exited: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("panel never ready")
	}
	return nil
}

func (p *panelRun) stop() {
	p.once.Do(func() {
		p.cancel()
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
		}
	})
}

func (p *panelRun) post(t *testing.T, path string, body any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", p.base+path, strings.NewReader(string(b)))
	req.Header.Set("Cookie", p.cookie)
	req.Header.Set("Origin", p.origin)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func (p *panelRun) state(t *testing.T) View {
	t.Helper()
	return liveClient{base: p.base, cookie: p.cookie}.state(t)
}

// dial opens a terminal the way the page does: a ticket from an Origin-checked
// POST, then the WebSocket with the ticket as a subprotocol.
func (p *panelRun) dial(t *testing.T, lane string) *websocket.Conn {
	t.Helper()
	code, body := p.post(t, "/api/lanes/"+lane+"/ticket", nil)
	if code != 200 {
		t.Fatalf("ticket for %s: %d %v", lane, code, body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h := http.Header{}
	h.Set("Cookie", p.cookie)
	h.Set("Origin", p.origin)
	c, _, err := websocket.Dial(ctx, strings.Replace(p.base, "http", "ws", 1)+"/ws/term?lane="+lane+"&cols=100&rows=30",
		&websocket.DialOptions{HTTPHeader: h, Subprotocols: []string{TermSubprotocol, ticketPrefix + fmt.Sprint(body["ticket"])}})
	if err != nil {
		t.Fatalf("dial %s: %v", lane, err)
	}
	if c.Subprotocol() != TermSubprotocol {
		t.Fatalf("negotiated %q", c.Subprotocol())
	}
	return c
}

func send(t *testing.T, c *websocket.Conn, m termMsg) {
	t.Helper()
	b, _ := json.Marshal(m)
	if err := c.Write(context.Background(), websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

// readUntil reads the terminal stream until it contains want.
func readUntil(t *testing.T, c *websocket.Conn, want string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var got strings.Builder
	for !strings.Contains(got.String(), want) {
		typ, b, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for %q: %v; stream so far: %q", want, err, got.String())
		}
		if typ == websocket.MessageBinary {
			got.Write(b)
		}
	}
	return got.String()
}

func hasSession(tmux, sock, id string) bool {
	return exec.Command(tmux, "-L", sock, "has-session", "-t", "="+id).Run() == nil
}

func findTerm(v View, id string) *TermLaneView {
	for i := range v.Terminals {
		if v.Terminals[i].ID == id {
			return &v.Terminals[i]
		}
	}
	return nil
}

func TestLanesEndToEndOnAThrowawaySocket(t *testing.T) {
	tmux, sock := throwawaySocket(t)
	root := signals.ResolvePath(t.TempDir())
	home := t.TempDir()
	gitRun(t, root, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"T","lanes":{"main":"orchestrator","fix/":"fix"},
		"base":"main","worktree_dir":".wt"}`)
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "init")
	gitRun(t, root, "remote", "add", "origin", filepath.Join(root, "no-such-remote"))

	p := startPanel(t, root, home, sock)

	// Start a lane in the project root.
	if code, body := p.post(t, "/api/lanes", StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"}); code != 200 {
		t.Fatalf("start root lane: %d %v", code, body)
	}
	if !hasSession(tmux, sock, "orch") {
		t.Fatal("no tmux session after start")
	}
	if code, _ := p.post(t, "/api/lanes", StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"}); code != 409 {
		t.Fatalf("a second lane with the same id: %d, want 409", code)
	}

	// Type into it over the WebSocket, and read its output back. The quotes make the
	// echoed command line differ from the output, so only execution produces it.
	c := p.dial(t, "orch")
	send(t, c, termMsg{Type: "resize", Cols: 120, Rows: 40})
	send(t, c, termMsg{Type: "input", Data: "echo pa''nel-ok-$((6*7))"})
	time.Sleep(300 * time.Millisecond)
	send(t, c, termMsg{Type: "input", Data: "\r"})
	readUntil(t, c, "panel-ok-42")
	out, _ := exec.Command(tmux, "-L", sock, "display-message", "-p", "-t", "=orch:", "#{window_width}x#{window_height}").Output()
	if strings.TrimSpace(string(out)) != "120x40" { // the status line is off (PANEL-6)
		t.Fatalf("resize message did not reach tmux: window is %s", out)
	}
	// The lane runs the panel's own session id, and the registry holds the binding.
	v := p.state(t)
	orch := findTerm(v, "orch")
	if orch == nil || !orch.Registered || !uuidRe.MatchString(orch.SessionID) || orch.Orphan != "" {
		t.Fatalf("orch in state: %+v", orch)
	}
	startCmd, _ := exec.Command(tmux, "-L", sock, "display-message", "-p", "-t", "=orch:", "#{pane_start_command}").Output()
	if !strings.Contains(string(startCmd), "--session-id "+orch.SessionID) || !strings.Contains(string(startCmd), "-n orch") {
		t.Fatalf("lane started as %s", startCmd)
	}
	// Anything but {input|resize} closes the connection; a command string is refused.
	send(t, c, termMsg{Type: "exec", Data: "rm -rf /"})
	var rerr error
	dctx, dcancel := context.WithTimeout(context.Background(), 5*time.Second)
	for rerr == nil { // drain output still in flight, up to the close
		_, _, rerr = c.Read(dctx)
	}
	dcancel()
	if websocket.CloseStatus(rerr) != websocket.StatusPolicyViolation {
		t.Fatalf("unknown message type: %v, want a policy-violation close", rerr)
	}
	// Closing the browser leaves the lane running.
	c.Close(websocket.StatusNormalClosure, "")
	time.Sleep(300 * time.Millisecond)
	if !hasSession(tmux, sock, "orch") {
		t.Fatal("closing the viewer ended the lane")
	}

	// A new branch and worktree. The remote is unreachable: fetch fails, which is
	// reported, not fatal.
	code, body := p.post(t, "/api/lanes", StartRequest{Type: "fix", Mode: "new", Name: "fx"})
	if code != 200 {
		t.Fatalf("start new-branch lane: %d %v", code, body)
	}
	wt := filepath.Join(root, ".wt", "fx")
	if gitRun(t, wt, "rev-parse", "--abbrev-ref", "HEAD") != "fix/fx" {
		t.Fatal("worktree not on fix/fx")
	}
	if notes, _ := body["lane"].(map[string]any)["notes"].([]any); len(notes) == 0 || !strings.Contains(fmt.Sprint(notes[0]), "git fetch failed") {
		t.Fatalf("fetch failure not reported: %v", body)
	}
	waitFor(t, "both lanes in the state, merged with their worktrees", func() bool {
		v := p.state(t)
		o, f := findTerm(v, "orch"), findTerm(v, "fx")
		return o != nil && f != nil && o.Worktree == root && o.Type == "orchestrator" &&
			f.Worktree == signals.ResolvePath(wt) && f.Branch == "fix/fx" && f.Type == "fix"
	})

	// The panel restarts; tmux kept the lanes, and the new panel finds them.
	p.stop()
	if !hasSession(tmux, sock, "orch") || !hasSession(tmux, sock, "fx") {
		t.Fatal("lanes died with the panel")
	}
	p2 := startPanel(t, root, home, sock)
	waitFor(t, "rediscovered lanes", func() bool {
		v := p2.state(t)
		return findTerm(v, "orch") != nil && findTerm(v, "fx") != nil
	})
	c2 := p2.dial(t, "fx")
	send(t, c2, termMsg{Type: "input", Data: "pwd"})
	time.Sleep(300 * time.Millisecond)
	send(t, c2, termMsg{Type: "input", Data: "\r"})
	readUntil(t, c2, ".wt/fx")
	c2.Close(websocket.StatusNormalClosure, "")

	// An API key in the tmux server's global environment blocks every start.
	exec.Command(tmux, "-L", sock, "set-environment", "-g", "ANTHROPIC_API_KEY", "sk-test").Run()
	code, body = p2.post(t, "/api/lanes", StartRequest{Type: "orchestrator", Mode: "root", Name: "blocked"})
	if code != 409 || body["code"] != "api-key" || !strings.Contains(fmt.Sprint(body["error"]), "tmux server") {
		t.Fatalf("start with a key in tmux's environment: %d %v", code, body)
	}
	if hasSession(tmux, sock, "blocked") {
		t.Fatal("a refused lane started anyway")
	}
	exec.Command(tmux, "-L", sock, "set-environment", "-g", "-u", "ANTHROPIC_API_KEY").Run()

	// Restart resumes the lane's own session: --resume <its id>, never --continue.
	// --resume needs a conversation, which the first submitted prompt's hook records.
	fxSID := findTerm(p2.state(t), "fx").SessionID
	hook := fmt.Sprintf(`{"hook_event_name":"UserPromptSubmit","session_id":%q,"cwd":%q,"prompt":"hi"}`, fxSID, signals.ResolvePath(wt))
	if code := (liveClient{base: p2.base}).post(t, "/hook", hook); code != 204 {
		t.Fatalf("hook: %d", code)
	}
	waitFor(t, "the registry to record fx's conversation", func() bool {
		r, _ := OpenRegistry(home, root)
		rec, _ := r.Get("fx")
		return rec.Conversation
	})
	if code, body := p2.post(t, "/api/lanes/fx/restart", nil); code != 200 {
		t.Fatalf("restart: %d %v", code, body)
	}
	startCmd, _ = exec.Command(tmux, "-L", sock, "display-message", "-p", "-t", "=fx:", "#{pane_start_command}").Output()
	if !strings.Contains(string(startCmd), "--resume "+fxSID) || strings.Contains(string(startCmd), "--continue") {
		t.Fatalf("restarted as %s", startCmd)
	}

	// A reboot, simulated: the tmux server dies. The registry keeps both lanes, and
	// the panel shows them as orphans instead of dropping them.
	exec.Command(tmux, "-L", sock, "kill-server").Run()
	waitFor(t, "orphans after the tmux server died", func() bool {
		o, f := findTerm(p2.state(t), "orch"), findTerm(p2.state(t), "fx")
		return o != nil && f != nil && o.Status == "orphaned" && !o.Running && strings.Contains(o.Orphan, "tmux session is gone")
	})
	if code, body := p2.post(t, "/api/lanes/orch/resume", nil); code != 200 {
		t.Fatalf("resume: %d %v", code, body)
	}
	// orch never had a prompt, so there is nothing to --resume: it gets its own
	// session id again.
	startCmd, _ = exec.Command(tmux, "-L", sock, "display-message", "-p", "-t", "=orch:", "#{pane_start_command}").Output()
	if !strings.Contains(string(startCmd), "--session-id "+orch.SessionID) || strings.Contains(string(startCmd), "--continue") {
		t.Fatalf("resumed as %s", startCmd)
	}
	if code, _ := p2.post(t, "/api/lanes/orch/forget", nil); code != 409 {
		t.Fatalf("forgetting a running lane: %d, want 409", code)
	}
	if code, body := p2.post(t, "/api/lanes/fx/forget", nil); code != 200 {
		t.Fatalf("forget orphan: %d %v", code, body)
	}
	waitFor(t, "forgotten lane gone", func() bool { return findTerm(p2.state(t), "fx") == nil })
	if _, err := os.Stat(wt); err != nil {
		t.Fatal("forget must never remove a worktree")
	}

	// Stop: /exit (which sh does not understand), then the session is killed, and
	// the open viewer is closed with it.
	c3 := p2.dial(t, "orch")
	if code, body := p2.post(t, "/api/lanes/orch/stop", nil); code != 200 {
		t.Fatalf("stop: %d %v", code, body)
	}
	rctx, rcancel := context.WithTimeout(context.Background(), 5*time.Second)
	for {
		if _, _, err := c3.Read(rctx); err != nil {
			break
		}
	}
	rcancel()
	if hasSession(tmux, sock, "orch") {
		t.Fatal("stopped lane still running")
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatal("stop must never remove a worktree")
	}
	waitFor(t, "stopped lane gone from the state", func() bool { return findTerm(p2.state(t), "orch") == nil })
	if code, _ := p2.post(t, "/api/lanes/orch/stop", nil); code != 404 {
		t.Fatalf("stopping a gone lane: %d, want 404", code)
	}
}

// With a key in the panel's own environment, the API refuses and says why.
func TestStartRefusedOverHTTPWhileTheKeyIsInThePanelsEnvironment(t *testing.T) {
	tmux, sock := throwawaySocket(t)
	root := signals.ResolvePath(t.TempDir())
	home := t.TempDir()
	gitRun(t, root, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"T","lanes":{"main":"orchestrator"}}`)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	p := startPanel(t, root, home, sock)
	code, body := p.post(t, "/api/lanes", StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"})
	if code != 409 || body["code"] != "api-key" || !strings.Contains(fmt.Sprint(body["error"]), "ANTHROPIC_API_KEY") {
		t.Fatalf("got %d %v", code, body)
	}
	if hasSession(tmux, sock, "orch") {
		t.Fatal("lane started with an API key set")
	}
	waitFor(t, "the page is told why", func() bool { return strings.Contains(p.state(t).StartBlocked, "ANTHROPIC_API_KEY") })
	if err := exec.Command(tmux, "-L", sock, "has-session").Run(); err == nil {
		t.Fatal("a refused start still started a tmux server")
	} else if ee := (&exec.ExitError{}); !errors.As(err, &ee) {
		t.Fatal(err)
	}
}

func rootLaneProject(t *testing.T) (root, home string) {
	t.Helper()
	root = signals.ResolvePath(t.TempDir())
	home = t.TempDir()
	gitRun(t, root, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"T","lanes":{"main":"orchestrator"}}`)
	return root, home
}

// closeCode reads until the connection closes and returns its close status.
func closeCode(c *websocket.Conn, within time.Duration) websocket.StatusCode {
	ctx, cancel := context.WithTimeout(context.Background(), within)
	defer cancel()
	for {
		if _, _, err := c.Read(ctx); err != nil {
			return websocket.CloseStatus(err)
		}
	}
}

// A terminal whose page has gone quiet (hidden: no "alive") is closed with 4000,
// and a page that keeps saying "alive" keeps its terminal.
func TestTerminalClosesWhenThePageIsIdle(t *testing.T) {
	_, sock := throwawaySocket(t)
	root, home := rootLaneProject(t)
	p := startPanelWith(t, root, home, sock, func(o *Options) { o.TermIdleTimeout = 800 * time.Millisecond })
	if code, body := p.post(t, "/api/lanes", StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"}); code != 200 {
		t.Fatalf("start: %d %v", code, body)
	}
	quiet := p.dial(t, "orch")
	if got := closeCode(quiet, 5*time.Second); got != closeIdle {
		t.Fatalf("quiet terminal closed with %v, want %v (idle)", got, closeIdle)
	}
	busy := p.dial(t, "orch")
	stop := time.After(2500 * time.Millisecond)
	for alive := true; alive; {
		select {
		case <-stop:
			alive = false
		case <-time.After(200 * time.Millisecond):
			send(t, busy, termMsg{Type: "alive"})
		}
	}
	if got := closeCode(busy, 100*time.Millisecond); got != -1 {
		t.Fatalf("a terminal that kept saying alive was closed: %v", got)
	}
}

// Rotating the token (rotate-token, or a reinstall) kills the old one in the running
// panel: its terminals close with 4001 and its cookie gets 401.
func TestTokenRotationClosesTerminalsAndCookies(t *testing.T) {
	_, sock := throwawaySocket(t)
	root, home := rootLaneProject(t)
	p := startPanelWith(t, root, home, sock, func(o *Options) {
		o.Launchd, o.NoOpen, o.OpenBrowser = true, true, func(string) {}
	})
	if code, body := p.post(t, "/api/lanes", StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"}); code != 200 {
		t.Fatalf("start: %d %v", code, body)
	}
	c := p.dial(t, "orch")
	newTok, err := RotateToken(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := closeCode(c, 6*time.Second); got != closeRotated {
		t.Fatalf("terminal closed with %v after rotation, want %v", got, closeRotated)
	}
	req, _ := http.NewRequest("GET", p.base+"/api/state", nil)
	req.Header.Set("Cookie", p.cookie)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != 401 {
		t.Fatalf("old cookie after rotation: %v %v", resp.StatusCode, err)
	}
	bu, _ := url.Parse(p.base)
	port := bu.Port()
	fresh := liveClient{base: p.base, cookie: "clauductor_panel_" + port + "=" + newTok}
	if v := fresh.state(t); v.Name != "T" {
		t.Fatalf("new token: %+v", v.Name)
	}
	if code, _ := p.post(t, "/api/lanes/orch/ticket", nil); code != 401 {
		t.Fatalf("ticket with the old cookie: %d, want 401", code)
	}
}
