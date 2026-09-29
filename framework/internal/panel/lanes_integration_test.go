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

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/web"
	"github.com/coder/websocket"
)

// These tests drive real tmux, git and a PTY, with `sh` standing in for claude, on a
// throwaway tmux socket that is killed afterwards. They never touch the panel's
// real socket or the user's own tmux server.

func throwawaySocket(t *testing.T) (string, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: drives a real tmux server")
	}
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
	home                 string
	base, cookie, origin string
	cancel               func()
	done                 chan error
	once                 sync.Once
	polls                *pollCounter // each source's polls (Options.OnPoll)
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
	polls := newPollCounter()
	o := Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: gitOnlyRunner,
		// sh stands in for claude; the claude flags land in its ignored positional args.
		TmuxSocket: sock, LaneProgram: []string{"/bin/sh", "-c", "exec /bin/sh", "lane"}, StopTimeout: 1500 * time.Millisecond,
		// A (re)started lane must stay up FastExit to count as started: sh does.
		FastExit: 500 * time.Millisecond,
		OnReady:  func(u string) { ready <- u }, OnPoll: polls.hook, Ticks: fastTicks()}
	if tweak != nil {
		tweak(&o)
	}
	go func() { done <- Run(ctx, o) }()
	select {
	case u := <-ready:
		pu, _ := url.Parse(u)
		port, _ := strconv.Atoi(pu.Port())
		p := &panelRun{home: home, base: "http://" + pu.Host, origin: "http://" + pu.Host,
			cookie: fmt.Sprintf("clauductor_panel_%d=%s", port, pu.Query().Get("t")), cancel: cancel, done: done, polls: polls}
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
		waitMachineFree(p.home)
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

func (p *panelRun) state(t *testing.T) state.View {
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
		&websocket.DialOptions{HTTPHeader: h, Subprotocols: []string{web.TermSubprotocol, ticketPrefix + fmt.Sprint(body["ticket"])}})
	if err != nil {
		t.Fatalf("dial %s: %v", lane, err)
	}
	if c.Subprotocol() != web.TermSubprotocol {
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

func findTerm(v state.View, id string) *state.TermLaneView {
	for i := range v.Terminals {
		if v.Terminals[i].ID == id {
			return &v.Terminals[i]
		}
	}
	return nil
}

func TestLanesEndToEndOnAThrowawaySocket(t *testing.T) {
	t.Parallel()
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
	if code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"}); code != 200 {
		t.Fatalf("start root lane: %d %v", code, body)
	}
	if !hasSession(tmux, sock, "orch") {
		t.Fatal("no tmux session after start")
	}
	if code, _ := p.post(t, "/api/lanes", lanes.StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"}); code != 409 {
		t.Fatalf("a second lane with the same id: %d, want 409", code)
	}

	// Type into it over the WebSocket, and read its output back. The quotes make the
	// echoed command line differ from the output, so only execution produces it.
	c := p.dial(t, "orch")
	send(t, c, termMsg{Type: "resize", Cols: 120, Rows: 40})
	send(t, c, termMsg{Type: "input", Data: "echo pa''nel-ok-$((6*7))"})
	readUntil(t, c, "$((6*7))") // the shell has the line; now its Enter
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
	// Closing the browser leaves the lane running: once the panel has detached the
	// viewer's tmux client, the session is still there.
	c.Close(websocket.StatusNormalClosure, "")
	waitFor(t, "the viewer's tmux client to detach", func() bool {
		out, err := exec.Command(tmux, "-L", sock, "list-clients", "-t", "=orch").Output()
		return err == nil && strings.TrimSpace(string(out)) == ""
	})
	if !hasSession(tmux, sock, "orch") {
		t.Fatal("closing the viewer ended the lane")
	}

	// A new branch and worktree. The remote is unreachable: fetch fails, which is
	// reported, not fatal.
	code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "fix", Mode: "new", Name: "fx"})
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
	readUntil(t, c2, "pwd")
	send(t, c2, termMsg{Type: "input", Data: "\r"})
	readUntil(t, c2, ".wt/fx")
	c2.Close(websocket.StatusNormalClosure, "")

	// An API key in the tmux server's global environment blocks every start.
	exec.Command(tmux, "-L", sock, "set-environment", "-g", "ANTHROPIC_API_KEY", "sk-test").Run()
	code, body = p2.post(t, "/api/lanes", lanes.StartRequest{Type: "orchestrator", Mode: "root", Name: "blocked"})
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
		r, _ := lanes.OpenRegistry(home, root)
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
	code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"})
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

// tickCounter is the system clock, counting the ticks its tickers of one period
// have delivered: a test waits for N of them instead of sleeping.
type tickCounter struct {
	clock.Clock
	period time.Duration
	mu     sync.Mutex
	n      int
}

func (c *tickCounter) count() int { c.mu.Lock(); defer c.mu.Unlock(); return c.n }

func (c *tickCounter) NewTicker(d time.Duration) clock.Ticker {
	inner := c.Clock.NewTicker(d)
	if d != c.period {
		return inner
	}
	ct := &countedTicker{inner: inner, c: make(chan time.Time), stop: make(chan struct{})}
	go func() {
		for {
			select {
			case v := <-inner.C():
				select {
				case ct.c <- v: // delivered: the reader has finished the tick before
					c.mu.Lock()
					c.n++
					c.mu.Unlock()
				case <-ct.stop:
					return
				}
			case <-ct.stop:
				return
			}
		}
	}()
	return ct
}

type countedTicker struct {
	inner clock.Ticker
	c     chan time.Time
	stop  chan struct{}
	once  sync.Once
}

func (t *countedTicker) C() <-chan time.Time { return t.c }
func (t *countedTicker) Stop()               { t.once.Do(func() { t.inner.Stop(); close(t.stop) }) }

// A terminal whose page has gone quiet (hidden: no "alive") is closed with 4000,
// and a page that keeps saying "alive" keeps its terminal.
func TestTerminalClosesWhenThePageIsIdle(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	root, home := rootLaneProject(t)
	// The terminal checks for silence every idle/4 (110 ms here, a period no other
	// ticker of the panel has): those checks are counted.
	const idle = 440 * time.Millisecond
	checks := &tickCounter{Clock: clock.System, period: idle / 4}
	p := startPanelWith(t, root, home, sock, func(o *Options) { o.TermIdleTimeout, o.Clock = idle, checks })
	if code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"}); code != 200 {
		t.Fatalf("start: %d %v", code, body)
	}
	quiet := p.dial(t, "orch")
	if got := closeCode(quiet, 5*time.Second); got != closeIdle {
		t.Fatalf("quiet terminal closed with %v, want %v (idle)", got, closeIdle)
	}
	busy := p.dial(t, "orch")
	// "alive" at every check, for twelve checks: three idle timeouts. A terminal
	// closed meanwhile stops its checks.
	deadline := time.Now().Add(10 * time.Second)
	for end := checks.count() + 13; checks.count() < end; {
		send(t, busy, termMsg{Type: "alive"})
		for n := checks.count(); checks.count() == n; {
			if time.Now().After(deadline) {
				t.Fatalf("the terminal's idle checks stopped: it was closed (%v) while the page said alive", closeCode(busy, time.Second))
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	if got := closeCode(busy, 100*time.Millisecond); got != -1 {
		t.Fatalf("a terminal that kept saying alive was closed: %v", got)
	}
}

// Rotating the token (rotate-token, or a reinstall) kills the old one in the running
// panel: its terminals close with 4001 and its cookie gets 401.
func TestTokenRotationClosesTerminalsAndCookies(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	root, home := rootLaneProject(t)
	p := startPanelWith(t, root, home, sock, func(o *Options) {
		o.Launchd, o.NoOpen, o.OpenBrowser = true, true, func(string) {}
	})
	if code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"}); code != 200 {
		t.Fatalf("start: %d %v", code, body)
	}
	c := p.dial(t, "orch")
	newTok, err := install.RotateToken(home)
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

// Typing while scrolled back reaches claude (re-audit P2-2): the first key that is
// not a scroll key leaves copy mode, then goes to the lane. The page is told when
// the lane is scrolled back, and when it is not any more. Real tmux, real panel.
func TestTypingWhileScrolledBackReachesTheLane(t *testing.T) {
	t.Parallel()
	tmux, sock := throwawaySocket(t)
	root, home := rootLaneProject(t)
	p := startPanel(t, root, home, sock)
	if code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "orchestrator", Mode: "root", Name: "orch"}); code != 200 {
		t.Fatalf("start: %d %v", code, body)
	}
	c := p.dial(t, "orch")
	send(t, c, termMsg{Type: "input", Data: "seq 1 300\r"})
	readUntil(t, c, "300")
	scroll := make(chan string, 8)
	go func() {
		for {
			typ, b, err := c.Read(context.Background())
			if err != nil {
				return
			}
			if typ == websocket.MessageText && strings.Contains(string(b), `"scroll"`) {
				scroll <- string(b)
			}
		}
	}()
	expect := func(want string) {
		t.Helper()
		select {
		case got := <-scroll:
			if got != want {
				t.Fatalf("page told %s, want %s", got, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("page never told %s", want)
		}
	}
	for i := 0; i < 3; i++ {
		send(t, c, termMsg{Type: "input", Data: "\x1b[<64;10;5M"})
	}
	expect(`{"type":"scroll","back":true}`)
	inMode := func() string {
		out, _ := exec.Command(tmux, "-L", sock, "display-message", "-p", "-t", "=orch:", "#{pane_in_mode}").Output()
		return strings.TrimSpace(string(out))
	}
	if inMode() != "1" {
		t.Fatal("premise: the wheel did not put the pane in copy mode")
	}
	marker := filepath.Join(t.TempDir(), "typed")
	send(t, c, termMsg{Type: "input", Data: "touch " + shq(marker) + "\r"})
	expect(`{"type":"scroll","back":false}`)
	waitFor(t, "the typed command to run in the lane", func() bool { _, err := os.Stat(marker); return err == nil })
	if inMode() != "0" {
		t.Fatal("the pane is still in copy mode")
	}
	// Escape while scrolled back only returns: claude would read it as an interrupt.
	for i := 0; i < 3; i++ {
		send(t, c, termMsg{Type: "input", Data: "\x1b[<64;10;5M"})
	}
	expect(`{"type":"scroll","back":true}`)
	send(t, c, termMsg{Type: "input", Data: "\x1b"})
	expect(`{"type":"scroll","back":false}`)
	if inMode() != "0" {
		t.Fatal("Escape did not leave copy mode")
	}
}
