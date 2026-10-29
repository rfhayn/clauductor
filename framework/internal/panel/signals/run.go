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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
)

// Runner runs one command in dir and returns its stdout. Injectable for tests.
type Runner func(ctx context.Context, dir string, argv []string) ([]byte, error)

const maxCmdOutput = 1 << 20

// HeldOutputWait bounds how long ExecRunner reads the output of a command other than
// claude once it has exited or been killed at its timeout: a child it started can hold
// the output open (a `git fetch`'s remote helper), and it shares the panel's group, so
// it is not the panel's to kill (ADR-0005). Past the bound the pipes are closed. Every
// exec.Cmd.WaitDelay the panel sets for that reason is this one.
const HeldOutputWait = time.Second

// ExecRunner runs argv directly (no shell) with a limited stdout.
//
// A `claude` (ClaudeArgs) is stopped, never killed outright (PANEL-28): when ctx is
// done it gets SIGTERM, and SIGKILL only ClaudeStopGrace later, since a process
// SIGKILLed while it holds the login-refresh lock strands it. Its process is reported
// to WithProcessStart's callback, and it returns once the claude is reaped and nothing
// of its group is left, or ClaudeStopGrace after its exit (runClaude). Every other
// command keeps exec.CommandContext's SIGKILL at once, and its output is read at most
// HeldOutputWait past its exit; one that exited 0 still succeeds.
func ExecRunner(ctx context.Context, dir string, argv []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	claude := IsClaude(argv)
	if claude {
		sent := stopSignal(ctx)
		cmd.Cancel = func() error {
			err := cmd.Process.Signal(syscall.SIGTERM)
			if err == nil && sent != nil {
				sent(syscall.SIGTERM)
			}
			return err
		}
		cmd.WaitDelay = ClaudeStopGrace
		// A session of its own, so KillCall reaches every process of the call. Without
		// a terminal, as under launchd, it is never stopped by touching one.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}
	cmd.Dir = dir
	var out, errb limitedBuffer
	out.max, errb.max = maxCmdOutput, 4096
	keepStderr := stderrSink(ctx)
	if keepStderr != nil {
		errb.max = maxCmdOutput
	}
	var err error
	if claude {
		err = runClaude(ctx, cmd, &out, &errb, ClaudeStopGrace)
	} else {
		cmd.Stdout, cmd.Stderr = &out, &errb
		cmd.WaitDelay = HeldOutputWait
		err = cmd.Run()
		// A command that exited 0 did its work: only a child of its still held the output.
		if errors.Is(err, exec.ErrWaitDelay) && ctx.Err() == nil && cmd.ProcessState != nil && cmd.ProcessState.Success() {
			err = nil
		}
	}
	if err != nil {
		// %w: the gate reads how a stopped claude ended (its *exec.ExitError).
		msg := strings.TrimSpace(errb.String())
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		if msg != "" {
			return nil, fmt.Errorf("%s: %w: %s", argv[0], err, msg)
		}
		return nil, fmt.Errorf("%s: %w", argv[0], err)
	}
	if keepStderr != nil {
		keepStderr(errb.Bytes())
	}
	return out.Bytes(), nil
}

// groupKillAgain is how often runClaude repeats its kill of what is left of a call's
// group, until none is left.
const groupKillAgain = 50 * time.Millisecond

// runClaude runs a `claude` ExecRunner has set up, on output pipes of its own, and
// returns once the claude is reaped and nothing of its group is left, or bound after
// the reap with an error that says which was not (drain) (#318). On exec's
// pipes Wait returns only when every holder of the output has closed it, and a child
// the claude started holds it: a stopped claude that exited on SIGTERM left that
// child running, and the call ended only when WaitDelay closed the pipes at the grace,
// with the child orphaned and the slot held. Here Wait returns at the reap, and the
// rest of the claude's process group (whose id is its session's: Setsid) is killed
// then, whether it was stopped or exited by itself. Cancel's SIGTERM still reaches
// the claude alone, so it can stop its children itself first; whatever it left is an
// orphan of the call.
//
// The kill is sent again until the group is gone, not only until the output ends, as
// the shutdown's is (#143): a child forked as it lands can miss it (macOS). The
// group's id is the claude's pid, which cannot be reused while a process of the group
// lives (KillCall). With none left the id is free from the reap: the first kill, an
// instant later, finds no group, and none follows, as at the shutdown. A holder that
// left the group is not the panel's to signal (ADR-0005): its output is closed bound
// (ExecRunner's ClaudeStopGrace) after the exit, with exec.ErrWaitDelay, as WaitDelay
// would have.
func runClaude(ctx context.Context, cmd *exec.Cmd, stdout, stderr io.Writer, bound time.Duration) error {
	o, err := newCallOutput(stdout, stderr)
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = o.w[0], o.w[1]
	err = cmd.Start()
	o.closeWriters() // the claude has its own copies; the parent's would hold the output open
	if err != nil {
		o.closeReaders()
		return err
	}
	o.copy()
	if f := processStart(ctx); f != nil {
		f(cmd.Process)
	}
	err = cmd.Wait()
	if derr := o.drain(cmd.Process, bound); err == nil {
		err = derr
	}
	return err
}

// callOutput is a claude call's stdout and stderr, each a pipe runClaude reads itself:
// w[i] is the claude's end, r[i] the panel's.
type callOutput struct {
	r, w [2]*os.File
	dst  [2]io.Writer
	done chan struct{} // closed once both pipes are read to their end
}

func newCallOutput(stdout, stderr io.Writer) (*callOutput, error) {
	o := &callOutput{dst: [2]io.Writer{stdout, stderr}, done: make(chan struct{})}
	for i := range o.r {
		r, w, err := os.Pipe()
		if err != nil {
			o.closeWriters()
			o.closeReaders()
			return nil, err
		}
		o.r[i], o.w[i] = r, w
	}
	return o, nil
}

func (o *callOutput) closeWriters() {
	for _, f := range o.w {
		if f != nil {
			_ = f.Close()
		}
	}
}

func (o *callOutput) closeReaders() {
	for _, f := range o.r {
		if f != nil {
			_ = f.Close()
		}
	}
}

// copy reads both pipes into their writers until each ends (or drain's deadline
// stops it), then closes them and done.
func (o *callOutput) copy() {
	var wg sync.WaitGroup
	for i := range o.r {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = io.Copy(o.dst[i], o.r[i])
		}()
	}
	go func() {
		wg.Wait()
		o.closeReaders()
		close(o.done)
	}()
}

// errGroupOutlived is drain's error when the output ended but something of the
// claude's group still answered its kill at the bound (a member stuck in an
// uninterruptible wait): the call ends then, and says so rather than nothing.
var errGroupOutlived = errors.New("a process of the claude's group outlived the kill")

// drain kills what is left of the reaped claude p's group until none is left, and
// waits for its output to end: both at most bound. Then it stops the reads and
// returns exec.ErrWaitDelay if the output had not ended, or errGroupOutlived if the
// group had not gone. The output can end before the group is gone (a child whose
// output goes elsewhere, or one forked as a kill landed), so it waits for both.
func (o *callOutput) drain(p *os.Process, bound time.Duration) error {
	group := true
	kill := func() {
		if _, err := KillCall(p); err != nil {
			// None is left (os.ErrProcessDone), or it cannot be signalled: macOS answers
			// EPERM for a group of zombies only.
			group = false
		}
	}
	kill()
	// The bound is real time a real process group has to go, and ExecRunner is a plain
	// Runner with no Options to carry a clock: the system clock, as rewriteSettings does.
	clk := clock.System
	limit := clk.NewTimer(bound)
	defer limit.Stop()
	again := clk.NewTicker(groupKillAgain)
	defer again.Stop()
	output := o.done
	for group || output != nil {
		select {
		case <-output:
			output = nil
		case <-again.C():
			if group {
				kill()
			}
		case <-limit.C():
			if output != nil {
				// select picks at random among ready cases: the output may have ended too.
				select {
				case <-output:
					output = nil
				default:
				}
			}
			if output == nil {
				if group {
					return errGroupOutlived
				}
				return nil
			}
			for _, r := range o.r {
				_ = r.SetReadDeadline(clk.Now())
			}
			<-o.done
			return exec.ErrWaitDelay
		}
	}
	return nil
}

// KillCall SIGKILLs a `claude` call ExecRunner started (WithProcessStart's process)
// and every process left in its group. Killing the claude alone is not enough: a
// child it started holds the call's output pipes open, so ExecRunner does not return
// until that child is gone, which can be after the panel's shutdown stopped waiting
// (#143). runClaude kills the group once the claude is reaped; the shutdown kills a
// call whose claude still runs.
//
// claude says whether the claude itself got the SIGKILL, through its handle (never a
// reused pid): a log names only signals it sent, and one that had exited got none.
// The group's id is the claude's pid, which cannot be reused while any process of
// the group lives. err is os.ErrProcessDone once none is left; from then the id is
// free for reuse, so the caller must not kill the call again.
func KillCall(p *os.Process) (claude bool, err error) {
	claude = p.Kill() == nil
	if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return claude, os.ErrProcessDone
		}
		return claude, err
	}
	return claude, nil
}

type stderrKey struct{}

// WithStderr asks ExecRunner for a successful command's stderr as well, within the
// same limit as its stdout: f receives it before ExecRunner returns. Only a sidecar's
// card asks (PANEL-42, design.md D5, Output): with no shell its command cannot say
// 2>&1, and every other command's stdout is parsed alone.
func WithStderr(ctx context.Context, f func([]byte)) context.Context {
	return context.WithValue(ctx, stderrKey{}, f)
}

func stderrSink(ctx context.Context) func([]byte) {
	f, _ := ctx.Value(stderrKey{}).(func([]byte))
	return f
}

type limitedBuffer struct {
	bytes.Buffer
	max int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.Len(); room > 0 {
		if len(p) > room {
			b.Buffer.Write(p[:room])
		} else {
			b.Buffer.Write(p)
		}
	}
	return len(p), nil
}

// ReadFrom hides bytes.Buffer's own: exec copies a pipe with io.Copy, which prefers
// a ReaderFrom, and the embedded one would fill the buffer past max.
func (b *limitedBuffer) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{b}, r)
}

// ResolvePath makes a path comparable with worktree paths: absolute, symlinks
// resolved (macOS /tmp is /private/tmp). A path that no longer exists is cleaned only.
func ResolvePath(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// ReadWorktrees runs `git worktree list --porcelain` in root: the authority for
// which cwds belong to the project, with every path resolved.
func ReadWorktrees(ctx context.Context, run Runner, root string) ([]Worktree, error) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := run(cctx, root, []string{"git", "worktree", "list", "--porcelain"})
	if err != nil {
		return nil, err
	}
	wts, err := parseWorktreePorcelain(out)
	if err != nil {
		return nil, err
	}
	for i := range wts {
		wts[i].Path = ResolvePath(wts[i].Path)
	}
	return wts, nil
}
