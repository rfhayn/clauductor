package signals

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/testwait"
)

// holderEnv makes this test binary a holder of its stdout outside the group of the
// shell that started it (TestHelperHoldsOutput): its value is the directory where it
// marks that it has left, and where the test releases it.
const holderEnv = "CLAUDUCTOR_TEST_OUTPUT_HOLDER"

// TestHelperHoldsOutput is not a test: run with holderEnv set, it leaves its parent's
// process group, says so, and keeps its stdout open until it is released (or a
// bound passes), as a child of a claude that left the call's group would.
func TestHelperHoldsOutput(t *testing.T) {
	dir := os.Getenv(holderEnv)
	if dir == "" {
		t.Skip("a helper process for TestRunClaudeClosesOutputHeldOutsideItsGroup")
	}
	if err := syscall.Setpgid(0, 0); err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(filepath.Join(dir, "left"), nil, 0o600); err != nil {
		os.Exit(2)
	}
	deadline := time.Now().Add(testwait.Scale(30 * time.Second))
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, "release")); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	os.Exit(0)
}

// A process that left the call's group and holds its output is not the panel's to
// signal (ADR-0005): runClaude kills nothing of it, and closes the output bound after
// the claude's exit with exec.ErrWaitDelay, as WaitDelay did, keeping what the claude
// wrote. Without the bound the call would never end (#318).
func TestRunClaudeClosesOutputHeldOutsideItsGroup(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { _ = os.WriteFile(filepath.Join(dir, "release"), nil, 0o600) })
	// The shell exits only once the holder has left its group: a kill of the group
	// before that would reach the holder, and the output would end at once.
	script := `"$0" -test.run='^TestHelperHoldsOutput$' &
while [ ! -e "$1/left" ]; do sleep 0.05; done
echo written
exit 0`
	cmd := exec.Command("sh", "-c", script, os.Args[0], dir)
	cmd.Env = append(os.Environ(), holderEnv+"="+dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	var out bytes.Buffer
	const bound = 300 * time.Millisecond
	start := time.Now()
	err := runClaude(context.Background(), cmd, &out, io.Discard, bound)
	took := time.Since(start)
	if !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("err %v, want exec.ErrWaitDelay: the held output was not closed at the bound", err)
	}
	if out.String() != "written\n" {
		t.Fatalf("output %q, want what the claude wrote before it exited", out.String())
	}
	if took < bound || took > bound+testwait.Scale(10*time.Second) {
		t.Fatalf("returned %v after the start, want the bound %v after the exit", took, bound)
	}
	if _, err := os.Stat(filepath.Join(dir, "left")); err != nil {
		t.Fatalf("the holder never left the group: %v", err)
	}
}
