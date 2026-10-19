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
	"syscall"
	"time"
)

// Runner runs one command in dir and returns its stdout. Injectable for tests.
type Runner func(ctx context.Context, dir string, argv []string) ([]byte, error)

const maxCmdOutput = 1 << 20

// ExecRunner runs argv directly (no shell) with a limited stdout.
//
// A `claude` (ClaudeArgs) is stopped, never killed outright (PANEL-28): when ctx is
// done it gets SIGTERM, and SIGKILL only ClaudeStopGrace later, since a process
// SIGKILLed while it holds the login-refresh lock strands it. Its process is reported
// to WithProcessStart's callback. Every other command keeps exec.CommandContext's
// SIGKILL at once.
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
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Start()
	if err == nil {
		if f := processStart(ctx); claude && f != nil {
			f(cmd.Process)
		}
		err = cmd.Wait()
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

// KillCall SIGKILLs a `claude` call ExecRunner started (WithProcessStart's process)
// and every process left in its group. Killing the claude alone is not enough: a
// child it started holds the call's output pipes open, so ExecRunner does not return
// until that child exits, which can be after the panel's shutdown stopped waiting
// (#143).
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
