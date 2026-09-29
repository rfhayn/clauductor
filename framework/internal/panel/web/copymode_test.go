package web

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/creack/pty"
)

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
