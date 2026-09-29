package lanes

import (
	"context"
	"github.com/clauductor/clauductor/internal/leakcheck"
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

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func lineCount(p string) int {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	return len(strings.Fields(string(b)))
}

// TestMain fails the run if it leaves a tmux server or a helper process behind.
func TestMain(m *testing.M) { os.Exit(leakcheck.Main(m)) }
