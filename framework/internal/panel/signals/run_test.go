package signals

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/rfhayn/stridal/internal/testwait"
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
	if took < bound {
		t.Fatalf("returned %v after the start, before the bound %v", took, bound)
	}
	if _, err := os.Stat(filepath.Join(dir, "left")); err != nil {
		t.Fatalf("the holder never left the group: %v", err)
	}
}

// heldOutputChild is a shell line that starts a child holding the shell's stderr (and
// stdout, unless redirect says otherwise) until release exists, or for three times
// wait at most: longer than any wait of the test's, so the child's own end can never
// pass for ExecRunner's bound, and a run that fails leaves nothing for long.
func heldOutputChild(release, redirect string, wait time.Duration) string {
	polls := int(3 * testwait.Scale(wait) / (50 * time.Millisecond))
	return fmt.Sprintf(`(i=0; while [ ! -e %q ] && [ $i -lt %d ]; do sleep 0.05; i=$((i+1)); done) %s &`, release, polls, redirect)
}

// A command other than claude, SIGKILLed at its timeout, returns within
// HeldOutputWait though a child it started still holds its output: Wait read the
// pipes until every holder had closed them, so a `git fetch` whose remote helper
// held stderr outlived its timeout without bound. The child shares the panel's group,
// so it is left alone (ADR-0005); it ends when the test releases it.
func TestExecRunnerBoundsAKilledCommandsHeldOutput(t *testing.T) {
	dir := t.TempDir()
	started, release := filepath.Join(dir, "started"), filepath.Join(dir, "release")
	t.Cleanup(func() { _ = os.WriteFile(release, nil, 0o600) })
	const wait = 10 * time.Second
	script := heldOutputChild(release, "", wait) + `
echo started > "$0"
while :; do sleep 0.1; done`
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan error, 1)
	go func() {
		_, err := ExecRunner(ctx, dir, []string{"sh", "-c", script, started})
		errc <- err
	}()
	testwait.For(t, "the command's child", wait, func() bool {
		_, err := os.Stat(started)
		return err == nil
	})
	cancel()
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("a command killed at its timeout returned no error")
		}
	case <-time.After(HeldOutputWait + testwait.Scale(wait)):
		t.Fatal("ExecRunner never returned: it waited for the child that holds the output")
	}
}

// A command other than claude that exits 0 while a child it started still holds its
// stderr succeeded: past HeldOutputWait ExecRunner returns its output and no error,
// not exec.ErrWaitDelay, which reported a command that had done its work as failed.
func TestExecRunnerKeepsASuccessWhoseOutputIsHeld(t *testing.T) {
	dir := t.TempDir()
	release := filepath.Join(dir, "release")
	t.Cleanup(func() { _ = os.WriteFile(release, nil, 0o600) })
	const wait = 10 * time.Second
	script := heldOutputChild(release, ">/dev/null", wait) + "\necho done\nexit 0"
	errc := make(chan error, 1)
	var out []byte
	go func() {
		var err error
		out, err = ExecRunner(context.Background(), dir, []string{"sh", "-c", script})
		errc <- err
	}()
	select {
	case err := <-errc:
		if err != nil || string(out) != "done\n" {
			t.Fatalf("out %q, err %v: want the command's output and no error", out, err)
		}
	case <-time.After(HeldOutputWait + testwait.Scale(wait)):
		t.Fatal("ExecRunner never returned: it waited for the child that holds the output")
	}
}
