package web

import (
	"context"
	"github.com/clauductor/clauductor/internal/leakcheck"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/testwait"
	"github.com/coder/websocket"
	"github.com/creack/pty"
)

// The mouse wheel over a lane scrolls tmux's history instead of reaching claude as
// ↑ keypresses (audit P1-2). Real tmux, a real client on a PTY, on a throwaway
// socket: the client is what xterm.js is in the browser.
func TestWheelOverALaneScrollsHistoryAndSendsNoKeys(t *testing.T) {
	t.Parallel()
	tmux, sock := throwawaySocket(t)
	dir := t.TempDir()
	got := filepath.Join(dir, "input")
	// The pane prints history to scroll through, then records every byte it gets.
	prog := "seq 1 300; stty raw -echo; exec cat -uv > " + shq(got)
	if out, err := exec.Command(tmux, "-L", sock, "-f", "/dev/null", "new-session", "-d", "-s", "w", "-x", "80", "-y", "24",
		"/bin/sh", "-c", prog).CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v %s", err, out)
	}
	m := &lanes.LaneManager{Clock: clock.System, TmuxPath: tmux, Socket: sock}
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
	if keys := leakcheck.TableKeys(tmux, sock, "root"); strings.Count(keys, "\n") != 0 || !strings.Contains(keys, "WheelUpPane") {
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

// PANEL-14: what a viewer's client gets from a pane that behaves like claude's
// fullscreen TUI. The OSC 8 link it prints must reach the client, or the page has no
// link to open (tmux strips it without the hyperlinks feature). Its request for every
// mouse motion (1003) is relayed too: that is what panel.js declines, because each
// reported motion cleared xterm's selection.
func TestViewerGetsLinksAndTheMotionRequest(t *testing.T) {
	t.Parallel()
	tmux, sock := throwawaySocket(t)
	prog := `printf '\033[?1000h\033[?1002h\033[?1003h\033[?1006h\033]8;;https://example.com/pr/12\033\\PR 12\033]8;;\033\\\n'; exec sleep 60`
	if out, err := exec.Command(tmux, "-L", sock, "-f", "/dev/null", "new-session", "-d", "-s", "w", "-x", "80", "-y", "24",
		"/bin/sh", "-c", prog).CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v %s", err, out)
	}
	m := &lanes.LaneManager{Clock: clock.System, TmuxPath: tmux, Socket: sock}
	if err := m.Harden(context.Background()); err != nil {
		t.Fatal(err)
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
	has := func(s string) bool { mu.Lock(); defer mu.Unlock(); return strings.Contains(screen.String(), s) }
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if has("PR 12") && has("\x1b[?1003h") && has("https://example.com/pr/12") {
			return
		}
	}
	t.Fatalf("the client got: link text %v, OSC 8 target %v, motion request %v",
		has("PR 12"), has("https://example.com/pr/12"), has("\x1b[?1003h"))
}

func TestInputKind(t *testing.T) {
	t.Parallel()
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

// fakePane is tmux as copyWatch sees it: a pane that enters copy mode when the test
// says tmux has read the wheel, never before.
type fakePane struct {
	mu             sync.Mutex
	in, alt, mouse string // "0" or "1"
	looks, left    int    // display-messages asked; copy modes left
	looked         chan struct{}
	unread         []string      // written to the lane's terminal, not yet read by tmux
	got, lost      string        // keys the program received; keys copy mode ate
	at             int           // lines scrolled back in copy mode
	hold           chan struct{} // set: the next look answers only once it is closed
}

// write puts one input message on the lane's terminal.
func (p *fakePane) write(data string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.unread = append(p.unread, data)
}

// read is tmux reading the lane's terminal, in order, as tmux 3.6a does with the
// panel's bindings: a wheel-up enters copy mode without scrolling and, in it,
// scrolls up 5 lines; a wheel-down scrolls down 5 and leaves copy mode at the bottom;
// copy mode takes a key for itself.
func (p *fakePane) read() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, d := range p.unread {
		switch {
		case d == wheelUp && p.in == "1":
			p.at += 5
		case d == wheelUp:
			p.in, p.at = "1", 0
		case d == wheelDown && p.in == "1":
			if p.at -= 5; p.at <= 0 {
				p.in = "0"
			}
		case inputKind(d) == inMouse:
		case p.in == "1":
			p.lost += d
		default:
			p.got += d
		}
	}
	p.unread = nil
}

// keys is what the program received, what copy mode ate, and how often copy mode was left.
func (p *fakePane) keys() (got, lost string, left int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.got, p.lost, p.left
}

func newFakePane() *fakePane {
	return &fakePane{in: "0", alt: "0", mouse: "0", looked: make(chan struct{}, 64)}
}

func (p *fakePane) exec(_ context.Context, argv []string) ([]byte, error) {
	p.mu.Lock()
	switch {
	case slices.Contains(argv, "display-message"):
		p.looks++
		r := strings.NewReplacer("#{pane_in_mode}", p.in, "#{alternate_on}", p.alt, "#{mouse_any_flag}", p.mouse)
		out := []byte(r.Replace(argv[len(argv)-1]) + "\n")
		hold := p.hold
		p.hold = nil
		p.mu.Unlock()
		p.looked <- struct{}{}
		if hold != nil {
			<-hold // the answer was taken now; it arrives only later
		}
		return out, nil
	case slices.Contains(argv, "cancel"):
		p.left++
		p.in = "0"
	}
	p.mu.Unlock()
	return nil, nil
}

func (p *fakePane) set(f func(p *fakePane)) { p.mu.Lock(); defer p.mu.Unlock(); f(p) }

func (p *fakePane) counts() (looks, left int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.looks, p.left
}

// watchWithFakes is a copyWatch on a fake pane and a fake clock, and what its page is told.
func watchWithFakes(t *testing.T) (*copyWatch, *fakePane, *clock.Fake, <-chan string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	accepted := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		accepted <- c
		<-ctx.Done()
	}))
	page, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	told := make(chan string, 16)
	go func() {
		for {
			_, b, err := page.Read(ctx)
			if err != nil {
				return
			}
			told <- string(b)
		}
	}()
	t.Cleanup(func() { cancel(); page.CloseNow(); srv.Close() })
	pane, clk := newFakePane(), clock.NewFake(time.Unix(0, 0))
	m := &lanes.LaneManager{Clock: clk, TmuxPath: "/nonexistent/tmux", Socket: "unused", Exec: pane.exec}
	return &copyWatch{lanes: m, id: "w", conn: <-accepted, clock: clk}, pane, clk, told
}

// lookAgain lets the watch's next wait run out, and reports whether it looked again
// (false: nothing was waiting, the watch has stopped looking).
func lookAgain(t *testing.T, pane *fakePane, clk *clock.Fake) bool {
	t.Helper()
	return lookAgainAfter(t, pane, clk, time.Second)
}

// lookAgainAfter is lookAgain moving the clock by d.
func lookAgainAfter(t *testing.T, pane *fakePane, clk *clock.Fake, d time.Duration) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testwait.Scale(200*time.Millisecond))
	defer cancel()
	if clk.BlockUntilContext(ctx, 1) != nil {
		return false
	}
	clk.Advance(d)
	select {
	case <-pane.looked:
		return true
	case <-time.After(testwait.Scale(3 * time.Second)):
		t.Fatal("the clock ran out a wait but tmux was not asked")
		return false
	}
}

// #82: tmux reads the wheel from the lane's terminal, and the look reaches it on a
// connection of its own, so nothing orders the two. On a loaded machine tmux reads the
// wheel after the first look: the watch must look again until it sees copy mode, tell
// the page, and treat the next key as one typed while scrolled back.
func TestWheelReadLateByTmuxStillTellsThePage(t *testing.T) {
	t.Parallel()
	w, pane, clk, told := watchWithFakes(t)
	ctx := context.Background()
	send(w, wheelUp)
	if !lookAgain(t, pane, clk) {
		t.Fatal("the watch never looked after the wheel")
	}
	pane.set(func(p *fakePane) { p.in = "1" }) // tmux reads the wheel only now
	for i := 0; i < 20 && lookAgain(t, pane, clk); i++ {
	}
	select {
	case got := <-told:
		if got != `{"type":"scroll","back":true}` {
			t.Fatalf("page told %s", got)
		}
	case <-time.After(testwait.Scale(time.Second)):
		t.Fatal(`page never told {"type":"scroll","back":true}: the watch stopped looking before tmux read the wheel`)
	}
	if !w.before(ctx, "y") {
		t.Fatal("a typed key was dropped")
	}
	if _, left := pane.counts(); left != 1 {
		t.Fatalf("the key typed while scrolled back left copy mode %d times, want 1: it went to copy mode, not to the lane", left)
	}
}

const (
	wheelUp   = "\x1b[<64;10;5M"
	wheelDown = "\x1b[<65;10;5M"
)

// send is the terminal handler's part: the watch before the write, then after it.
func send(w *copyWatch, data string) bool {
	if !w.before(context.Background(), data) {
		return false
	}
	w.after(context.Background(), data)
	return true
}

// lookUntilStopped runs the watch's waits out until it stops looking, and returns
// how many looks it made.
func lookUntilStopped(t *testing.T, pane *fakePane, clk *clock.Fake) int {
	t.Helper()
	n := 0
	for ; lookAgain(t, pane, clk); n++ {
		if n > 50 {
			t.Fatal("the watch never stopped looking")
		}
	}
	return n
}

// #82: when the program took the wheel (claude's alternate screen, or its mouse
// reporting), copy mode will not come: the watch stops at the first look, tells the
// page nothing, and the next key goes straight to the lane.
func TestWheelTheProgramTookStopsTheLookAtOnce(t *testing.T) {
	t.Parallel()
	for flag, on := range map[string]func(p *fakePane){
		"alternate_on":   func(p *fakePane) { p.alt = "1" },
		"mouse_any_flag": func(p *fakePane) { p.mouse = "1" },
	} {
		w, pane, clk, told := watchWithFakes(t)
		pane.set(on)
		send(w, wheelUp)
		if n := lookUntilStopped(t, pane, clk); n != 1 {
			t.Fatalf("%s: %d looks after a wheel the program took, want 1", flag, n)
		}
		if !send(w, "y") {
			t.Fatalf("%s: a typed key was dropped", flag)
		}
		if looks, left := pane.counts(); looks != 1 || left != 0 {
			t.Fatalf("%s: the key cost %d more looks and left copy mode %d times, want none", flag, looks-1, left)
		}
		select {
		case got := <-told:
			t.Fatalf("%s: page told %s", flag, got)
		default:
		}
	}
}

// A wheel-up tmux never acts on is looked at only until recheckFor has passed, and
// the next key still checks for copy mode: running out of time is not a "no".
func TestWheelLookIsBounded(t *testing.T) {
	t.Parallel()
	w, pane, clk, _ := watchWithFakes(t)
	send(w, wheelUp)
	if n := lookUntilStopped(t, pane, clk); n < 2 || time.Duration(n-1)*time.Second > recheckFor {
		t.Fatalf("%d looks a second apart, want them to stop once %v has passed", n, recheckFor)
	}
	before, _ := pane.counts()
	send(w, "y")
	if looks, _ := pane.counts(); looks != before+1 {
		t.Fatalf("the key after an unanswered wheel made %d looks, want 1", looks-before)
	}
}

// A wheel-down on the live screen cannot enter copy mode: one look settles it.
func TestWheelDownOnTheLiveScreenLooksOnce(t *testing.T) {
	t.Parallel()
	w, pane, clk, _ := watchWithFakes(t)
	send(w, wheelDown)
	if n := lookUntilStopped(t, pane, clk); n != 1 {
		t.Fatalf("%d looks after a wheel-down on the live screen, want 1", n)
	}
}

// waitLook waits for tmux to be asked once more.
func waitLook(t *testing.T, pane *fakePane, what string) {
	t.Helper()
	select {
	case <-pane.looked:
	case <-time.After(testwait.Scale(3 * time.Second)):
		t.Fatalf("tmux was never asked: %s", what)
	}
}

// blockUntil waits until n waits are pending on the clock, and fails if they never are.
func blockUntil(t *testing.T, clk *clock.Fake, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testwait.Scale(3*time.Second))
	defer cancel()
	if clk.BlockUntilContext(ctx, n) != nil {
		t.Fatalf("%d waits pending on the clock, want %d", clk.Pending(), n)
	}
}

// A key typed while a recheck still waits for copy mode takes the wait over: tmux
// reads the lane's terminal in order, so the wheel ahead of the key ends in copy mode
// before tmux reads the key. The key looks until it does, leaves copy mode once, and
// the recheck gives way: it neither looks again nor tells the page "scrolled back"
// after the key left copy mode.
func TestKeyTypedDuringARecheckLeavesCopyModeOnce(t *testing.T) {
	t.Parallel()
	w, pane, clk, told := watchWithFakes(t)
	send(w, wheelUp)
	if !lookAgain(t, pane, clk) { // the recheck's first look: no copy mode yet
		t.Fatal("the watch never looked after the wheel")
	}
	blockUntil(t, clk, 1) // the recheck waits to look again
	typed := make(chan bool, 1)
	go func() { typed <- send(w, "y") }()
	waitLook(t, pane, "the key's look")        // no copy mode yet either
	pane.set(func(p *fakePane) { p.in = "1" }) // tmux reads the wheel only now
	blockUntil(t, clk, 2)                      // the key and the recheck both wait to look again
	clk.Advance(time.Second)
	select {
	case ok := <-typed:
		if !ok {
			t.Fatal("a typed key was dropped")
		}
	case <-time.After(testwait.Scale(3 * time.Second)):
		t.Fatal("the key was never written: it waited for copy mode past the clock")
	}
	waitLook(t, pane, "the key's look that sees copy mode")
	if looks, left := pane.counts(); looks != 3 || left != 1 {
		t.Fatalf("%d looks and copy mode left %d times, want 3 and 1", looks, left)
	}
	clk.Advance(time.Second)
	select {
	case <-pane.looked:
		t.Fatal("the recheck looked again after a key was typed")
	case got := <-told:
		t.Fatalf("page told %s after the key", got)
	case <-time.After(testwait.Scale(200 * time.Millisecond)):
	}
	if clk.Pending() != 0 {
		t.Fatal("something still waits to look after the key settled copy mode")
	}
}

// sendThrough is send with the write between: the message goes onto the lane's
// terminal, which tmux reads at pane.read.
func sendThrough(w *copyWatch, pane *fakePane, data string) {
	if w.before(context.Background(), data) {
		pane.write(data)
		w.after(context.Background(), data)
	}
}

// #82: a key typed before tmux has read the wheel-up written ahead of it. tmux reads
// the terminal in order, so it enters copy mode first and copy mode would eat the
// key, and every key after it, with nothing left to tell the page. The key must wait
// until tmux has acted on the wheel, leave copy mode, and only then be written.
func TestKeyTypedBeforeTmuxReadTheWheelIsNotLost(t *testing.T) {
	t.Parallel()
	w, pane, clk, _ := watchWithFakes(t)
	sendThrough(w, pane, wheelUp) // on the terminal; tmux has not read it
	typed := make(chan struct{})
	go func() { defer close(typed); sendThrough(w, pane, "y") }()
	waitLook(t, pane, "the key's look") // no copy mode yet
	pane.read()                         // tmux reads the wheel only now, and enters copy mode
	ctx, cancel := context.WithTimeout(context.Background(), testwait.Scale(time.Second))
	defer cancel()
	_ = clk.BlockUntilContext(ctx, 2) // the key's next look, and the wheel's recheck
	clk.Advance(time.Second)
	select {
	case <-typed:
	case <-time.After(testwait.Scale(3 * time.Second)):
		t.Fatal("the key typed after the wheel was never written")
	}
	pane.read()
	sendThrough(w, pane, "z")
	pane.read()
	if got, lost, left := pane.keys(); got != "yz" || lost != "" || left != 1 {
		t.Fatalf("the lane got %q, copy mode ate %q, copy mode was left %d times; want \"yz\", nothing, once", got, lost, left)
	}
}

// A quick scroll up and back down, typed after before tmux has read it, ends on the
// live screen in tmux 3.6a: the first wheel-up enters copy mode without scrolling
// (`copy-mode -e`), a second scrolls 5 lines, and a wheel-down that reaches the
// bottom leaves. The key has no copy mode to wait for: it is written at once, with
// no wait on the clock, and reaches the lane.
func TestKeyAfterWheelsBackToTheBottomDoesNotWait(t *testing.T) {
	t.Parallel()
	for name, wheels := range map[string][]string{
		"up, down":               {wheelUp, wheelDown},
		"up, up, down":           {wheelUp, wheelUp, wheelDown},
		"up, up, down, up, down": {wheelUp, wheelUp, wheelDown, wheelUp, wheelDown},
	} {
		w, pane, clk, _ := watchWithFakes(t)
		for _, d := range wheels {
			sendThrough(w, pane, d) // on the terminal; tmux has not read it
		}
		typed := make(chan struct{})
		go func() { defer close(typed); sendThrough(w, pane, "y") }()
		select {
		case <-typed:
		case <-time.After(testwait.Scale(3 * time.Second)):
			t.Fatalf("%s: the key waits for a copy mode the wheels do not end in (%d waits on the clock)", name, clk.Pending())
		}
		pane.read()
		if got, lost, _ := pane.keys(); got != "y" || lost != "" {
			t.Fatalf("%s: the lane got %q, copy mode ate %q", name, got, lost)
		}
	}
}

// Wheels that end scrolled back are still copy mode, whatever wheel came last: up,
// up, up, down ends 5 lines back. tmux reads them late; the watch waits for copy
// mode, and tells the page.
func TestWheelsReadLateThatEndScrolledBackAreReported(t *testing.T) {
	t.Parallel()
	w, pane, clk, told := watchWithFakes(t)
	for _, d := range []string{wheelUp, wheelUp, wheelUp, wheelDown} {
		sendThrough(w, pane, d)
	}
	blockUntil(t, clk, 4) // each wheel's recheck waits; the first three give way
	if !lookAgain(t, pane, clk) {
		t.Fatal("the watch never looked after the wheels")
	}
	pane.read() // tmux reads the wheels only now
	var in string
	if pane.set(func(p *fakePane) { in = p.in }); in != "1" {
		t.Fatal("the fake pane is not in copy mode after up, up, up, down")
	}
	for i := 0; i < 20 && lookAgain(t, pane, clk); i++ {
	}
	select {
	case got := <-told:
		if got != `{"type":"scroll","back":true}` {
			t.Fatalf("page told %s", got)
		}
	case <-time.After(testwait.Scale(time.Second)):
		t.Fatal(`page never told {"type":"scroll","back":true}: the watch took up, up, up, down for the live screen`)
	}
}

// pageRecord is the viewer's websocket, recording what the page is told. A
// {"type":"scroll","back":true} is written only once release is closed.
type pageRecord struct {
	mu      sync.Mutex
	said    []string
	holding chan struct{} // closed once a "back":true write waits for release
	release chan struct{}
}

func (p *pageRecord) Write(_ context.Context, _ websocket.MessageType, b []byte) error {
	if strings.Contains(string(b), `"back":true`) {
		close(p.holding)
		<-p.release
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.said = append(p.said, string(b))
	return nil
}

// The page's messages arrive in the order they were decided: a recheck that told
// "scrolled back" is still writing it when a key leaves copy mode, and the key's
// "back":false must reach the page after it, or the page says "scrolled back" over
// the live screen.
func TestPageIsToldInTheOrderDecided(t *testing.T) {
	t.Parallel()
	w, pane, clk, _ := watchWithFakes(t)
	page := &pageRecord{holding: make(chan struct{}), release: make(chan struct{})}
	w.conn = page
	send(w, wheelUp)
	pane.set(func(p *fakePane) { p.in = "1" }) // tmux has read the wheel
	if !lookAgain(t, pane, clk) {
		t.Fatal("the watch never looked after the wheel")
	}
	select {
	case <-page.holding: // the recheck is writing "scrolled back"
	case <-time.After(testwait.Scale(3 * time.Second)):
		t.Fatal(`the recheck never told the page {"type":"scroll","back":true}`)
	}
	typed := make(chan struct{})
	go func() { defer close(typed); send(w, "y") }()
	waitLook(t, pane, "the key's look")
	time.Sleep(testwait.Scale(100 * time.Millisecond)) // room for a "back":false that does not wait its turn
	close(page.release)
	select {
	case <-typed:
	case <-time.After(testwait.Scale(3 * time.Second)):
		t.Fatal("the key was never written")
	}
	page.mu.Lock()
	defer page.mu.Unlock()
	if want := []string{`{"type":"scroll","back":true}`, `{"type":"scroll","back":false}`}; !slices.Equal(page.said, want) {
		t.Fatalf("page told %q, want %q", page.said, want)
	}
}

// A recheck whose look was answered before a key left copy mode, and returns after
// the key was written, gives way: telling the page "scrolled back" then would be
// stale.
func TestRecheckAnsweredBeforeAKeyTellsThePageNothing(t *testing.T) {
	t.Parallel()
	w, pane, clk, told := watchWithFakes(t)
	held := make(chan struct{})
	pane.set(func(p *fakePane) { p.in, p.hold = "1", held }) // tmux read the wheel; the answer is slow
	send(w, wheelUp)
	if !lookAgain(t, pane, clk) { // the recheck's look: copy mode, held on its way back
		t.Fatal("the watch never looked after the wheel")
	}
	if !send(w, "y") { // the key looks, leaves copy mode, and is written
		t.Fatal("a typed key was dropped")
	}
	close(held) // the recheck's answer, "copy mode", arrives now
	select {
	case got := <-told:
		t.Fatalf("page told %s after the key left copy mode", got)
	case <-time.After(testwait.Scale(200 * time.Millisecond)):
	}
	if looks, left := pane.counts(); looks != 2 || left != 1 {
		t.Fatalf("%d looks and copy mode left %d times, want 2 and 1", looks, left)
	}
}

// After a wheel-down copy mode can only end, so a pane that stays in it is looked at
// for recheckDownFor, not for as long as after a wheel-up; then the page is told.
func TestWheelDownLookIsBoundedSooner(t *testing.T) {
	t.Parallel()
	w, pane, clk, told := watchWithFakes(t)
	pane.set(func(p *fakePane) { p.in = "1" })
	send(w, wheelDown)
	n := 0
	for ; lookAgainAfter(t, pane, clk, recheckEvery); n++ {
		if n > 50 {
			t.Fatal("the watch never stopped looking")
		}
	}
	if n < 2 || time.Duration(n-1)*recheckEvery > recheckDownFor {
		t.Fatalf("%d looks %v apart, want them to stop once %v has passed", n, recheckEvery, recheckDownFor)
	}
	select {
	case got := <-told:
		if got != `{"type":"scroll","back":true}` {
			t.Fatalf("page told %s", got)
		}
	case <-time.After(testwait.Scale(time.Second)):
		t.Fatal("page never told it is scrolled back once the looks stopped")
	}
}

// A key whose wait for copy mode ran out still expects it: tmux may yet read the
// wheel. The next key looks once more, and the one after it, once that look found
// none, does not.
func TestKeyWhoseWaitRanOutLetsTheNextKeyLookOnce(t *testing.T) {
	t.Parallel()
	w, pane, clk, _ := watchWithFakes(t)
	send(w, wheelUp)
	lookUntilStopped(t, pane, clk) // tmux never shows copy mode within recheckFor
	for i, want := range []int{1, 1, 0} {
		before, _ := pane.counts()
		if !send(w, "y") {
			t.Fatalf("key %d was dropped", i+1)
		}
		if looks, _ := pane.counts(); looks-before != want {
			t.Fatalf("key %d made %d looks, want %d", i+1, looks-before, want)
		}
	}
}

// Only a plain wheel is bound: a wheel-up with Shift (button 68) enters no copy mode
// and scrolls nothing, so it neither makes a key wait nor counts as a scroll up.
func TestWheelWithAModifierIsNotSimulated(t *testing.T) {
	t.Parallel()
	const shiftWheelUp = "\x1b[<68;10;5M"
	for name, wheels := range map[string][]string{
		"shift-up":               {shiftWheelUp},
		"up, up, shift-up, down": {wheelUp, wheelUp, shiftWheelUp, wheelDown},
	} {
		w, _, clk, _ := watchWithFakes(t)
		for _, d := range wheels {
			send(w, d)
		}
		typed := make(chan struct{})
		go func() { defer close(typed); send(w, "y") }()
		select {
		case <-typed:
		case <-time.After(testwait.Scale(3 * time.Second)):
			t.Fatalf("%s: the key waits for a copy mode the wheels do not enter (%d waits on the clock)", name, clk.Pending())
		}
	}
}

// Nor does a modified or sideways wheel count as a scroll down: after two wheel-ups
// the pane is 5 lines back whatever Shift-wheel-down (69) or wheel-left (66) came
// after, so a key typed before tmux has read them waits for copy mode, leaves it
// once, and reaches the lane. Counted as a down, it would take the pane for the live
// screen, and copy mode would eat the key (#82).
func TestWheelWithAModifierDoesNotScrollDown(t *testing.T) {
	t.Parallel()
	for name, other := range map[string]string{"shift-down": "\x1b[<69;10;5M", "left": "\x1b[<66;10;5M"} {
		w, pane, clk, _ := watchWithFakes(t)
		for _, d := range []string{wheelUp, wheelUp, other} {
			sendThrough(w, pane, d) // on the terminal; tmux has not read it
		}
		typed := make(chan struct{})
		go func() { defer close(typed); sendThrough(w, pane, "y") }()
		waitLook(t, pane, name+": the key's look") // no copy mode yet
		pane.read()                                // tmux reads the wheels only now: 5 lines back
		ctx, cancel := context.WithTimeout(context.Background(), testwait.Scale(time.Second))
		_ = clk.BlockUntilContext(ctx, 4) // the key's next look, and each wheel's recheck
		cancel()
		clk.Advance(time.Second)
		select {
		case <-typed:
		case <-time.After(testwait.Scale(3 * time.Second)):
			t.Fatalf("%s: the key was never written", name)
		}
		pane.read()
		if got, lost, left := pane.keys(); got != "y" || lost != "" || left != 1 {
			t.Fatalf("%s: the lane got %q, copy mode ate %q, copy mode was left %d times; want \"y\", nothing, once", name, got, lost, left)
		}
	}
}
