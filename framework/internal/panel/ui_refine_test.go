package panel

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/coder/websocket"
	"github.com/creack/pty"
)

// PANEL-6: the page patches itself in place, and the hub pushes only a view that
// changed. A push of an unchanged view (only `now` differs) is what used to redraw
// the page every tick.

// fakeClock advances by step on every read, so every snapshot has a different Now.
type fakeClock struct {
	mu   sync.Mutex
	t    time.Time
	step time.Duration
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(c.step)
	return c.t
}

func TestHubPushesOnlyAChangedView(t *testing.T) {
	m := state.NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	clock := &fakeClock{t: t0, step: time.Second}
	h := NewHub(m, clock.now)
	h.TickEvery = 10 * time.Millisecond
	ch, cancel := h.subscribe()
	defer cancel()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go h.Run(ctx)

	recv := func(within time.Duration) bool {
		select {
		case <-ch:
			return true
		case <-time.After(within):
			return false
		}
	}
	if !recv(time.Second) {
		t.Fatal("the first view was never pushed")
	}
	// 30 ticks, each at a later `now`, and nothing else changes: nothing is pushed.
	if recv(300 * time.Millisecond) {
		t.Fatalf("an unchanged view was pushed again (%d pushes)", h.Pushes())
	}
	// An update that changes nothing the page shows is not pushed either.
	h.Update(func(m *state.Model, now time.Time) {})
	if recv(400 * time.Millisecond) {
		t.Fatal("a no-op update was pushed")
	}
	// A real change is pushed, once.
	h.Update(func(m *state.Model, now time.Time) {
		m.ApplyPRs([]signals.PR{{Number: 7, Title: "Synthetic"}}, nil, now)
	})
	if !recv(time.Second) {
		t.Fatal("a changed view was not pushed")
	}
	if recv(300 * time.Millisecond) {
		t.Fatal("a changed view was pushed more than once")
	}
	if n := h.Pushes(); n != 2 {
		t.Fatalf("%d pushes, want 2", n)
	}
}

// Steady polling that changes nothing the page shows is pushed only on the tick
// (re-audit P2-4): 20 s of 2 s polls, scaled down 20 times, give at most one push
// per 5 s tick, not one per poll.
func TestSteadyPollingPushesOnlyOnTheTick(t *testing.T) {
	m := alertModel(t, "")
	h := NewHub(m, time.Now)
	h.Coalesce, h.TickEvery = 5*time.Millisecond, 250*time.Millisecond // 5 s → 250 ms
	ch, cancel := h.subscribe()
	defer cancel()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go h.Run(ctx)
	pushes := 0
	deadline := time.After(time.Second)            // 20 s
	poll := time.NewTicker(100 * time.Millisecond) // 2 s
	defer poll.Stop()
	for n := int64(1); ; n++ {
		select {
		case <-ch:
			pushes++
			n--
			continue
		case <-poll.C:
			h.Update(func(m *state.Model, now time.Time) {
				m.ApplyAgentsTimed(waitingAgent("idle", ""), nil, 40*time.Millisecond, now)
				m.ApplyObs(state.Obs{AgentsPolls: n, AgentsPollMs: 30 + n})
			})
			continue
		case <-deadline:
		}
		break
	}
	// The ticks at 250, 500, 750 and 1000 ms, plus the first push.
	if pushes > 6 {
		t.Fatalf("%d pushes in 20 s (scaled) of polls that changed nothing, want at most 6", pushes)
	}
	if pushes < 2 {
		t.Fatalf("%d pushes: the bookkeeping never reached the page on the tick", pushes)
	}
}

// The mouse wheel over a lane scrolls tmux's history instead of reaching claude as
// ↑ keypresses (audit P1-2). Real tmux, a real client on a PTY, on a throwaway
// socket: the client is what xterm.js is in the browser.
func TestWheelOverALaneScrollsHistoryAndSendsNoKeys(t *testing.T) {
	tmux, sock := throwawaySocket(t)
	dir := t.TempDir()
	got := filepath.Join(dir, "input")
	// The pane prints history to scroll through, then records every byte it gets.
	prog := "seq 1 300; stty raw -echo; exec cat -uv > " + shq(got)
	if out, err := exec.Command(tmux, "-L", sock, "-f", "/dev/null", "new-session", "-d", "-s", "w", "-x", "80", "-y", "24",
		"/bin/sh", "-c", prog).CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v %s", err, out)
	}
	m := &lanes.LaneManager{TmuxPath: tmux, Socket: sock}
	if err := m.Harden(context.Background()); err != nil {
		t.Fatal(err)
	}
	show := func(args ...string) string {
		out, _ := exec.Command(tmux, append([]string{"-L", sock}, args...)...).CombinedOutput()
		return strings.TrimSpace(string(out))
	}
	if s := show("show-options", "-gv", "status"); s != "off" {
		t.Fatalf("status is %q, want off", s)
	}
	// Only the wheel came back to the root table: no click or menu bindings.
	if keys := show("list-keys", "-T", "root"); strings.Count(keys, "\n") != 0 || !strings.Contains(keys, "WheelUpPane") {
		t.Fatalf("root table after hardening:\n%s", keys)
	}

	cmd := exec.Command(tmux, m.AttachArgv("w")...)
	cmd.Env = attachEnv()
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { ptmx.Close(); cmd.Process.Kill(); cmd.Wait() }()
	var mu sync.Mutex
	var screen strings.Builder
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := ptmx.Read(b)
			mu.Lock()
			screen.Write(b[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
			if ok() {
				return
			}
		}
		t.Fatalf("timed out waiting for %s", what)
	}
	// tmux asks the client's terminal for mouse reports (SGR 1006). That request is
	// what stops xterm.js turning the wheel into arrow keys.
	waitFor("the client to be asked for mouse reports", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(screen.String(), "\x1b[?1006h") && strings.Contains(screen.String(), "\x1b[?1000h")
	})
	// Three wheel-ups, as xterm.js reports them once asked (SGR: button 64).
	for i := 0; i < 3; i++ {
		ptmx.Write([]byte("\x1b[<64;10;5M"))
		time.Sleep(30 * time.Millisecond)
	}
	waitFor("copy mode", func() bool { return show("display-message", "-p", "-t", "=w:", "#{pane_in_mode}") == "1" })
	if pos := show("display-message", "-p", "-t", "=w:", "#{scroll_position}"); pos == "" || pos == "0" {
		t.Fatalf("copy mode did not scroll: scroll_position %q", pos)
	}
	// Leave copy mode, then type a marker: the pane gets the marker and nothing else.
	show("send-keys", "-t", "=w:", "-X", "cancel")
	ptmx.Write([]byte("END"))
	waitFor("the marker", func() bool { b, _ := os.ReadFile(got); return strings.Contains(string(b), "END") })
	b, _ := os.ReadFile(got)
	if s := string(b); s != "END" {
		t.Fatalf("the pane received %q before the marker; the wheel must not reach it", s)
	}
}

// The stream beats even when nothing changes, so the page can tell a quiet panel
// from a dead one (audit P1-4), and carries the server's clock for the ages.
func TestEventsStreamHasAHeartbeat(t *testing.T) {
	old := HeartbeatEvery
	HeartbeatEvery = 50 * time.Millisecond
	defer func() { HeartbeatEvery = old }()
	s, _ := newTestServer(t)
	w := do(s, "GET", "/events", "", withCookie(s)) // do's context ends the stream after 1 s
	body := w.Body.String()
	if n := strings.Count(body, "event: state\n"); n != 1 {
		t.Fatalf("%d state events in a stream where nothing changed, want 1:\n%s", n, body)
	}
	if n := strings.Count(body, "event: hb\ndata: {\"now\":"); n < 5 {
		t.Fatalf("%d heartbeats in 1 s at 50 ms, want at least 5:\n%s", n, body)
	}
}

// The terminal is entered only on purpose (audit P1-1): panel.js focuses an xterm in
// exactly one place, enterTerm (Enter or a click on the terminal); the xterm is out
// of the Tab order; Ctrl+] leaves; nothing redraws the page on a timer (P1-3).
func TestPageEntersTheTerminalOnlyOnPurpose(t *testing.T) {
	js := readWeb(t, "panel.js")
	if n := strings.Count(js, ".term.focus("); n != 1 {
		t.Fatalf("panel.js focuses a terminal in %d places, want 1 (enterTerm)", n)
	}
	i := strings.Index(js, "function enterTerm(")
	j := strings.Index(js, ".term.focus(")
	if i < 0 || j < i || j-i > 200 {
		t.Fatal("the one terminal focus is not in enterTerm")
	}
	for _, want := range []string{"textarea.tabIndex = -1", `ev.code === "BracketRight"`, "attachCustomKeyEventHandler"} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
	if strings.Contains(js, "setInterval(render") {
		t.Error("panel.js re-renders on a timer; only ages tick (tickAges)")
	}
	rebuild := regexp.MustCompile(`\$\("(left|right|detail|tabs|termbar|banners|restorebar|obs|connbar)"\)\.(replaceChildren|innerHTML)`)
	if m := rebuild.FindString(js); m != "" {
		t.Errorf("panel.js rebuilds a live region (%s); patch it in place", m)
	}
}

func TestInputKind(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b[<64;10;5M": inMouse, "\x1b[<65;3;4M\x1b[<65;3;4M": inMouse, "\x1b[<0;1;1m": inMouse,
		"\x1b[A": inScroll, "\x1bOB": inScroll, "\x1b[5~": inScroll, "\x1b[1;2A": inScroll,
		"\x1b": inEscape, "y": inKey, "yes please": inKey, "\r": inKey, "\x1b[Z": inKey, "\x1bb": inKey,
	} {
		if got := inputKind(in); got != want {
			t.Errorf("inputKind(%q) = %s, want %s", in, got, want)
		}
	}
}

// Typing while scrolled back reaches claude (re-audit P2-2): the first key that is
// not a scroll key leaves copy mode, then goes to the lane. The page is told when
// the lane is scrolled back, and when it is not any more. Real tmux, real panel.
func TestTypingWhileScrolledBackReachesTheLane(t *testing.T) {
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
