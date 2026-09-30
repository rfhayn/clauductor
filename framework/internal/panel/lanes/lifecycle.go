package lanes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
)

// PANEL-20: what a new worktree needs before claude starts in it, and what one needs
// before Close removes it.
//
//   - ports: each lane gets a stable port of its own (ports.base + k × ports.per_lane,
//     the smallest k no other registered lane holds), kept in its registry record and
//     exported to the lane as CLAUDUCTOR_PORT;
//   - .worktreeinclude: the gitignored files it names are copied from the project root
//     into a new worktree, with Claude Code's semantics (a file must match the file's
//     .gitignore-syntax patterns AND be ignored, so a tracked file is never copied);
//   - worktree_setup runs in a new worktree before claude starts, worktree_teardown in
//     a worktree before Close removes it. Both are the trusted config's commands.

// HookTimeout bounds a setup or teardown command.
const HookTimeout = 5 * time.Minute

// Limits of a .worktreeinclude copy: past them the rest is not copied, and the lane's
// start says so.
const (
	includeMaxFiles = 2000
	includeMaxBytes = 200 << 20
)

// allocatePort is the lane's port: the one its record holds, else the lowest free one.
func (m *LaneManager) allocatePort(id string) int {
	p := m.Cfg.Ports
	if p == nil {
		return 0
	}
	used := map[int]bool{}
	for _, r := range m.Registry.List() {
		if r.ID == id && r.Port > 0 {
			return r.Port
		}
		if r.Port > 0 {
			used[r.Port] = true
		}
	}
	for port := p.Base; port <= 65535; port += p.PerLane {
		if !used[port] {
			return port
		}
	}
	return 0
}

// LanePort is the port a lane's record holds (0: none).
func (m *LaneManager) LanePort(id string) int {
	if m.Registry == nil {
		return 0
	}
	if r, ok := m.Registry.Get(id); ok {
		return r.Port
	}
	return 0
}

// hookArgv is a setup or teardown command with the lane's environment: /usr/bin/env
// sets it, so the command still runs without a shell.
func hookArgv(cmd []string, id string, port int) []string {
	argv := []string{"/usr/bin/env", "CLAUDUCTOR_LANE=" + id}
	if port > 0 {
		argv = append(argv, "CLAUDUCTOR_PORT="+strconv.Itoa(port))
	}
	return append(argv, cmd...)
}

// runHook runs a setup or teardown command in dir. It runs only while the config is
// trusted; an untrusted config's command is a note, not a run.
func (m *LaneManager) runHook(ctx context.Context, what string, h *config.HookCommand, dir, id string) (ran bool, err error) {
	if h == nil {
		return false, nil
	}
	if m.Trusted != nil && !m.Trusted() {
		return false, fmt.Errorf("%s did not run: panel.json is not trusted as it is (clauductor panel trust)", what)
	}
	cctx, cancel := context.WithTimeout(ctx, HookTimeout)
	defer cancel()
	if _, err := m.Run(cctx, dir, hookArgv(h.Command, id, m.LanePort(id))); err != nil {
		return true, fmt.Errorf("%s failed: %v", what, err)
	}
	return true, nil
}

// copyWorktreeInclude copies into dst the files .worktreeinclude names that are also
// ignored in the project root: the intersection of two `git ls-files` lists, so git
// itself applies both sets of patterns. Existing files are never overwritten, and only
// regular files are copied (a symlink could point anywhere).
func (m *LaneManager) copyWorktreeInclude(ctx context.Context, dst string) (int, string) {
	inc := filepath.Join(m.Root, ".worktreeinclude")
	if fi, err := os.Stat(inc); err != nil || !fi.Mode().IsRegular() {
		return 0, ""
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	ignored, err := m.Run(cctx, m.Root, []string{"git", "ls-files", "-z", "--others", "--ignored", "--exclude-standard"})
	if err != nil {
		return 0, ".worktreeinclude: cannot list the ignored files: " + err.Error()
	}
	named, err := m.Run(cctx, m.Root, []string{"git", "ls-files", "-z", "--others", "--ignored", "--exclude-from=" + inc})
	if err != nil {
		return 0, ".worktreeinclude: cannot read its patterns: " + err.Error()
	}
	isIgnored := map[string]bool{}
	for _, p := range bytes.Split(ignored, []byte{0}) {
		if len(p) > 0 {
			isIgnored[string(p)] = true
		}
	}
	var files []string
	for _, p := range bytes.Split(named, []byte{0}) {
		if s := string(p); s != "" && isIgnored[s] {
			files = append(files, s)
		}
	}
	sort.Strings(files)
	n, total := 0, int64(0)
	var skipped []string
	for _, rel := range files {
		clean := filepath.Clean(rel)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			continue
		}
		src, to := filepath.Join(m.Root, clean), filepath.Join(dst, clean)
		fi, err := os.Lstat(src)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		if _, err := os.Lstat(to); err == nil {
			continue // never overwrite what the checkout has
		}
		if n >= includeMaxFiles || total+fi.Size() > includeMaxBytes {
			skipped = append(skipped, clean)
			continue
		}
		if err := copyRegular(src, to, fi.Mode().Perm()); err != nil {
			skipped = append(skipped, clean)
			continue
		}
		n++
		total += fi.Size()
	}
	note := ""
	if len(skipped) > 0 {
		note = fmt.Sprintf(".worktreeinclude: %d file(s) not copied (over %d files or %d MB, or unreadable), first %s", len(skipped), includeMaxFiles, includeMaxBytes>>20, skipped[0])
	}
	return n, note
}

func copyRegular(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}
