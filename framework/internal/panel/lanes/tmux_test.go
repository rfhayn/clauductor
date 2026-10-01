package lanes

import (
	"context"
	"github.com/clauductor/clauductor/internal/leakcheck"
	"github.com/clauductor/clauductor/internal/testwait"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
)

// Typed text that starts with "-" must reach the lane as text, not as a send-keys
// flag (review 2026-09-28: "-N" was read as an option).
func TestSendTextTypesALeadingDashLiterally(t *testing.T) {
	t.Parallel()
	tmux, sock := throwawaySocket(t)
	out := filepath.Join(t.TempDir(), "typed")
	if err := exec.Command(tmux, "-L", sock, "new-session", "-d", "-s", "t", "cat > "+out).Run(); err != nil {
		t.Fatal(err)
	}
	m := &LaneManager{Clock: clock.System, TmuxPath: tmux, Socket: sock, EnterDelay: 100 * time.Millisecond}
	for _, text := range []string{"-N 3 --help", "--", "-l"} {
		if err := m.sendText(context.Background(), "t", text); err != nil {
			t.Fatalf("%q: %v", text, err)
		}
	}
	waitFor(t, "the typed lines", func() bool { return lineCount(out) >= 5 })
	b, _ := os.ReadFile(out)
	if string(b) != "-N 3 --help\n--\n-l\n" {
		t.Fatalf("typed %q", b)
	}
}

// throwawaySocket is a tmux socket of the test's own, killed at cleanup.
func throwawaySocket(t *testing.T) (string, string) {
	t.Helper()
	return leakcheck.TmuxSocket(t)
}

// waitFor waits for cond, up to a deadline that is generous because passing costs
// nothing: a loaded machine (several gates at once) is slow, not wrong.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	testwait.For(t, what, 10*time.Second, cond)
}

func lineCount(p string) int {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	return len(strings.Fields(string(b)))
}

// A lane's tmux server outlives the panel as an orphan and keeps, for life, the argv
// of the client that started it. A cleanup script that killed orphans naming
// `.claude/worktrees/` killed a live server and every lane on it: the server must be
// started by a command that names no worktree, and still end with its last lane.
func TestLaneServerArgvNamesNoWorktree(t *testing.T) {
	t.Parallel()
	tmux, sock := throwawaySocket(t)
	wt := filepath.Join(t.TempDir(), ".claude", "worktrees", "add-x")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	m := testLaneManager(t)
	m.TmuxPath, m.Socket, m.Program = tmux, sock, []string{"/bin/sh", "-c", "exec sleep 60"}
	ctx := context.Background()
	if err := m.newSession(ctx, m.NewSessionArgv("add-x", wt, "build", "00000000-0000-4000-8000-000000000001", false)); err != nil {
		t.Fatal(err)
	}
	pid, err := exec.Command(tmux, "-L", sock, "display-message", "-p", "#{pid}").Output()
	if err != nil {
		t.Fatal(err)
	}
	argv, err := exec.Command("ps", "-o", "command=", "-p", strings.TrimSpace(string(pid))).Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(argv), ".claude/worktrees") || strings.Contains(string(argv), wt) || strings.Contains(string(argv), "new-session") {
		t.Fatalf("the tmux server's argv names the lane: %s", argv)
	}
	if !strings.Contains(string(argv), "start-server") {
		t.Fatalf("the server was not started by ServerArgv: %s", argv)
	}
	// exit-empty is back on: the server still ends with its last session.
	if out, _ := exec.Command(tmux, "-L", sock, "show-options", "-g", "exit-empty").Output(); strings.TrimSpace(string(out)) != "exit-empty on" {
		t.Fatalf("exit-empty after the start: %q", out)
	}
	_ = exec.Command(tmux, "-L", sock, "kill-session", "-t", "=add-x").Run()
	waitFor(t, "the server to end with its last session", func() bool {
		return exec.Command(tmux, "-L", sock, "list-sessions").Run() != nil
	})
}

// TestMain fails the run if it leaves a tmux server or a helper process behind.
func TestMain(m *testing.M) { os.Exit(leakcheck.Main(m)) }
