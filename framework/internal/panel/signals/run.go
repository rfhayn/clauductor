package signals

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Runner runs one command in dir and returns its stdout. Injectable for tests.
type Runner func(ctx context.Context, dir string, argv []string) ([]byte, error)

const maxCmdOutput = 1 << 20

// ExecRunner runs argv directly (no shell) with a limited stdout.
func ExecRunner(ctx context.Context, dir string, argv []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	var out, errb limitedBuffer
	out.max, errb.max = maxCmdOutput, 4096
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		if msg != "" {
			return nil, fmt.Errorf("%s: %v: %s", argv[0], err, msg)
		}
		return nil, fmt.Errorf("%s: %v", argv[0], err)
	}
	return out.Bytes(), nil
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
