package lanes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
)

// PANEL-19: a lane starts with --remote-control only in lanes mode, before -n, and a
// restart keeps it; otherwise the flag never appears.
func TestLaneCommandRemoteControl(t *testing.T) {
	t.Parallel()
	m := testLaneManager(t)
	if got := m.LaneCommand("add-x", "build", sid, false); slices.Contains(got, "--remote-control") {
		t.Fatalf("no mode: %q", got)
	}
	on := false
	m.RemoteControl = func() bool { return on }
	if got := m.LaneCommand("add-x", "build", sid, false); slices.Contains(got, "--remote-control") {
		t.Fatalf("off: %q", got)
	}
	on = true
	for _, resume := range []bool{false, true} {
		got := m.LaneCommand("add-x", "build", sid, resume)
		i := slices.Index(got, "--remote-control")
		if i < 0 || got[i+1] != "-n" || got[i+2] != "add-x" {
			t.Fatalf("lanes mode (resume %v): %q", resume, got)
		}
	}
}

// PANEL-20: TypeLine types the line and Enter; when the re-check before the Enter
// says no, the text is cleared (C-u) and no Enter is sent.
func TestTypeLine(t *testing.T) {
	t.Parallel()
	tmux, sock := throwawaySocket(t)
	out := filepath.Join(t.TempDir(), "typed")
	// The lane records its raw input: C-u arrives as 0x15, Enter as \r.
	if err := exec.Command(tmux, "-L", sock, "new-session", "-d", "-s", "x", "stty raw -echo; exec cat > "+out).Run(); err != nil {
		t.Fatal(err)
	}
	m := &LaneManager{Clock: clock.System, TmuxPath: tmux, Socket: sock, EnterDelay: 50 * time.Millisecond}
	if err := m.TypeLine(context.Background(), "x", "continue", func() string { return "" }); err != nil {
		t.Fatal(err)
	}
	if err := m.TypeLine(context.Background(), "x", "again", func() string { return "it now waits on you" }); err == nil || !strings.Contains(err.Error(), "nothing was sent") {
		t.Fatalf("a no before the Enter: %v", err)
	}
	waitFor(t, "the keys", func() bool { b, _ := os.ReadFile(out); return strings.Count(string(b), "\x15") >= 3 })
	b, _ := os.ReadFile(out)
	if string(b) != "\x15continue\r\x15again\x15" {
		t.Fatalf("typed %q", b)
	}
	if err := m.TypeLine(context.Background(), "x", "two\nlines", func() string { return "" }); err == nil {
		t.Fatal("a line with a newline was typed")
	}
}

// Remote control types nothing unless lanes mode is on and the lane is registered.
func TestConnectRemoteRefuses(t *testing.T) {
	t.Parallel()
	m := testLaneManager(t)
	if e := m.ConnectRemote(context.Background(), "add-x"); e == nil || e.Code != "remote-off" {
		t.Fatalf("not in lanes mode: %+v", e)
	}
	m.RemoteControl = func() bool { return true }
	if e := m.ConnectRemote(context.Background(), "BAD id"); e == nil || e.Code != "invalid" {
		t.Fatalf("bad id: %+v", e)
	}
	if e := m.ConnectRemote(context.Background(), "nope"); e == nil || e.Code != "not-found" {
		t.Fatalf("unregistered: %+v", e)
	}
}
