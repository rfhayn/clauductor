package lanes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// closeRepo is a project whose lanes are orphans (registry records, no tmux server):
// git runs for real, and gh answers what the test says.
type closeRepo struct {
	t    *testing.T
	root string
	m    *LaneManager
	mu   sync.Mutex
	gh   string // `gh pr list --state merged` output
	// agents is `claude agents --json` output ("" fails the read).
	agents string
	// hooks are the setup and teardown argvs run (through /usr/bin/env).
	hooks [][]string
}

func newCloseRepo(t *testing.T) *closeRepo {
	t.Helper()
	root := signals.ResolvePath(t.TempDir())
	r := &closeRepo{t: t, root: root, gh: "[]", agents: "[]"}
	r.git(root, "init", "-q", "-b", "main")
	cfgPath := filepath.Join(root, config.DefaultConfigRel)
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"name":"T","lanes":{"main":"orchestrator","fix/":"fix"},"base":"main","worktree_dir":".wt"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// The worktrees live inside the checkout, as .claude/worktrees do; ignore them.
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".wt/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.git(root, "add", ".")
	r.git(root, "commit", "-q", "-m", "init")
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	reg, err := OpenRegistry(home, root)
	if err != nil {
		t.Fatal(err)
	}
	r.m = &LaneManager{Clock: clock.System, TmuxPath: "/nonexistent/tmux", Socket: "unused", Root: root, Cfg: cfg, Registry: reg,
		UploadDir: filepath.Join(home, "uploads"), Run: r.run,
		// No tmux server: every lane here is an orphan.
		Exec: func(context.Context, []string) ([]byte, error) {
			return nil, errors.New("tmux: no server running on /x")
		}}
	return r
}

func (r *closeRepo) run(ctx context.Context, dir string, argv []string) ([]byte, error) {
	switch argv[0] {
	case "git":
		return signals.ExecRunner(ctx, dir, argv)
	case "gh":
		r.mu.Lock()
		defer r.mu.Unlock()
		return []byte(r.gh), nil
	case "/usr/bin/env":
		r.mu.Lock()
		defer r.mu.Unlock()
		r.hooks = append(r.hooks, argv)
		return nil, nil
	case "claude":
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.agents == "" {
			return nil, errors.New("claude agents: exit status 1")
		}
		return []byte(r.agents), nil
	}
	return nil, fmt.Errorf("unexpected command %v", argv)
}

func (r *closeRepo) git(dir string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// lane adds a worktree on a new branch at path and registers an orphan lane in it.
func (r *closeRepo) lane(id, branch, path string) string {
	r.t.Helper()
	if path != r.root {
		r.git(r.root, "worktree", "add", "-q", "-b", branch, path, "main")
		path = signals.ResolvePath(path)
	}
	mode := "new"
	if path == r.root {
		mode = "root"
	}
	rec, err := r.m.Registry.Begin(types.LaneRecord{ID: id, SessionID: sid, Path: path, Type: "fix", Branch: branch, Mode: mode}, "start", time.Now())
	if err == nil {
		err = r.m.Registry.Done(rec)
	}
	if err != nil {
		r.t.Fatal(err)
	}
	return path
}

func (r *closeRepo) commit(dir, file string) string {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(file), 0o644); err != nil {
		r.t.Fatal(err)
	}
	r.git(dir, "add", file)
	r.git(dir, "commit", "-q", "-m", file)
	return r.git(dir, "rev-parse", "HEAD")
}

func (r *closeRepo) branchExists(b string) bool {
	return exec.Command("git", "-C", r.root, "rev-parse", "--verify", "--quiet", "refs/heads/"+b).Run() == nil
}

// closeAll plans, then closes with exactly what the plan offered.
func (r *closeRepo) closeAll(id string) (ClosePlan, CloseResult) {
	r.t.Helper()
	p, lerr := r.m.ClosePlan(context.Background(), id)
	if lerr != nil {
		r.t.Fatalf("plan %s: %v", id, lerr)
	}
	res, lerr := r.m.Close(context.Background(), id, CloseRequest{Worktree: p.Worktree, Branch: p.DeleteBranch})
	if lerr != nil {
		r.t.Fatalf("close %s: %v", id, lerr)
	}
	if _, ok := r.m.Registry.Get(id); ok {
		r.t.Fatalf("close %s kept the registry record", id)
	}
	return p, res
}

func has(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// PANEL-17: Close never removes a worktree that has anything git would lose: a
// change, or an untracked file. The lane is still closed, and the page is told why.
func TestCloseKeepsADirtyWorktree(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, dirt string }{{"untracked", "new.txt"}, {"modified", ".gitignore"}} {
		r := newCloseRepo(t)
		wt := r.lane("dirty", "fix/dirty", filepath.Join(r.root, ".wt", "dirty"))
		if err := os.WriteFile(filepath.Join(wt, c.dirt), []byte("unsaved work"), 0o644); err != nil {
			t.Fatal(err)
		}
		p, res := r.closeAll("dirty")
		if p.Worktree || p.DeleteBranch || !has(p.Keep, "uncommitted or untracked") || !has(p.Keep, c.dirt) || !has(p.Keep, "fix/dirty: its worktree stays") {
			t.Fatalf("%s: plan %+v", c.name, p)
		}
		if !exists(filepath.Join(wt, c.dirt)) || !r.branchExists("fix/dirty") || !has(res.Kept, "uncommitted or untracked") {
			t.Fatalf("%s: the worktree or branch went: %+v", c.name, res)
		}
		// Even a consent the plan never offered removes nothing.
		r.lane("dirty2", "fix/dirty2", filepath.Join(r.root, ".wt", "dirty2"))
		os.WriteFile(filepath.Join(r.root, ".wt", "dirty2", "x"), []byte("x"), 0o644)
		if res, _ := r.m.Close(context.Background(), "dirty2", CloseRequest{Worktree: true, Branch: true}); !exists(filepath.Join(r.root, ".wt", "dirty2", "x")) || !has(res.Kept, "untracked") {
			t.Fatalf("%s: a forced consent removed a dirty worktree: %+v", c.name, res)
		}
	}
}

// Never a locked worktree, the main worktree, or one outside worktree_dir.
func TestCloseKeepsLockedMainAndOutsideWorktrees(t *testing.T) {
	t.Parallel()
	r := newCloseRepo(t)
	locked := r.lane("locked", "fix/locked", filepath.Join(r.root, ".wt", "locked"))
	r.git(r.root, "worktree", "lock", locked)
	r.lane("main", "", r.root)
	outside := r.lane("outside", "fix/outside", filepath.Join(t.TempDir(), "outside"))
	sibling := r.lane("sibling", "fix/sibling", filepath.Join(r.root, "elsewhere", "sibling"))
	for id, want := range map[string]string{"locked": "locked", "main": "main worktree", "outside": "outside the project's worktree_dir", "sibling": "outside the project's worktree_dir"} {
		p, lerr := r.m.ClosePlan(context.Background(), id)
		if lerr != nil || p.Worktree || p.DeleteBranch || !has(p.Keep, want) {
			t.Fatalf("%s: plan %+v %v, want kept: %s", id, p, lerr, want)
		}
		res, lerr := r.m.Close(context.Background(), id, CloseRequest{Worktree: true, Branch: true})
		if lerr != nil || !has(res.Kept, want) {
			t.Fatalf("%s: close %+v %v", id, res, lerr)
		}
	}
	for _, p := range []string{locked, outside, sibling, filepath.Join(r.root, ".git")} {
		if !exists(p) {
			t.Fatalf("%s was removed", p)
		}
	}
	for _, b := range []string{"main", "fix/locked", "fix/outside", "fix/sibling"} {
		if !r.branchExists(b) {
			t.Fatalf("branch %s was deleted", b)
		}
	}
}

// A clean worktree goes; its branch goes only when merged into the base (or by a
// merged pull request whose head is the branch's tip), else it stays and says why.
func TestCloseRemovesACleanWorktreeAndOnlyAMergedBranch(t *testing.T) {
	t.Parallel()
	r := newCloseRepo(t)

	// Merged into the base (a merge commit).
	merged := r.lane("merged", "fix/merged", filepath.Join(r.root, ".wt", "merged"))
	r.commit(merged, "m.txt")
	r.git(r.root, "merge", "-q", "--no-ff", "-m", "merge", "fix/merged")
	// Images dropped on it go too (PANEL-15b's uploads).
	img := filepath.Join(r.m.ImageDir("merged"), "1-x.png")
	os.MkdirAll(filepath.Dir(img), 0o700)
	os.WriteFile(img, []byte("\x89PNG\r\n\x1a\n"), 0o600)
	p, res := r.closeAll("merged")
	if !p.Worktree || !p.DeleteBranch || !has(p.Remove, "every commit of it is in main") || !has(p.Remove, "1 image dropped") {
		t.Fatalf("merged: plan %+v", p)
	}
	if exists(merged) || r.branchExists("fix/merged") || exists(r.m.ImageDir("merged")) || !has(res.Removed, "the local branch fix/merged") {
		t.Fatalf("merged: %+v", res)
	}

	// Not merged: the worktree goes (its commits live on in the branch), the branch stays.
	open := r.lane("open", "fix/open", filepath.Join(r.root, ".wt", "open"))
	r.commit(open, "o.txt")
	p, res = r.closeAll("open")
	if !p.Worktree || p.DeleteBranch || !has(p.Keep, "not merged into main and has no merged pull request") {
		t.Fatalf("open: plan %+v", p)
	}
	if exists(open) || !r.branchExists("fix/open") || !has(res.Kept, "fix/open") {
		t.Fatalf("open: %+v", res)
	}

	// Squash-merged: none of its commits are in main, but its PR is merged at its tip.
	sq := r.lane("squashed", "fix/squashed", filepath.Join(r.root, ".wt", "squashed"))
	tip := r.commit(sq, "s.txt")
	r.git(r.root, "merge", "-q", "--squash", "fix/squashed")
	r.git(r.root, "commit", "-q", "-m", "squash")
	r.mu.Lock()
	r.gh = fmt.Sprintf(`[{"number":42,"headRefOid":%q}]`, tip)
	r.mu.Unlock()
	p, res = r.closeAll("squashed")
	if !p.DeleteBranch || !has(p.Remove, "pull request #42 was merged") || r.branchExists("fix/squashed") || exists(sq) {
		t.Fatalf("squashed: plan %+v, result %+v", p, res)
	}

	// A merged PR whose head is not the tip: the branch has work since. It stays.
	later := r.lane("later", "fix/later", filepath.Join(r.root, ".wt", "later"))
	old := r.commit(later, "l1.txt")
	r.commit(later, "l2.txt")
	r.mu.Lock()
	r.gh = fmt.Sprintf(`[{"number":43,"headRefOid":%q}]`, old)
	r.mu.Unlock()
	p, _ = r.closeAll("later")
	if p.DeleteBranch || !has(p.Keep, "#43 was merged, but the branch has commits since") || !r.branchExists("fix/later") {
		t.Fatalf("later: plan %+v", p)
	}

	// A branch the confirmation did not offer to delete stays, even if merged by now.
	late := r.lane("late", "fix/late", filepath.Join(r.root, ".wt", "late"))
	res, lerr := r.m.Close(context.Background(), "late", CloseRequest{Worktree: true})
	if lerr != nil || exists(late) || !r.branchExists("fix/late") || !has(res.Kept, "the confirmation did not include it") {
		t.Fatalf("late: %+v %v", res, lerr)
	}
	// Nor a worktree.
	kept := r.lane("kept", "fix/kept", filepath.Join(r.root, ".wt", "kept"))
	if res, lerr := r.m.Close(context.Background(), "kept", CloseRequest{}); lerr != nil || !exists(kept) || !has(res.Kept, "the confirmation did not include it") {
		t.Fatalf("kept: %+v %v", res, lerr)
	}
	if _, lerr := r.m.Close(context.Background(), "kept", CloseRequest{}); lerr == nil || lerr.Status != 404 {
		t.Fatalf("closing a closed lane: %v", lerr)
	}
}

// PANEL-21: the teardown gets the lane's CLAUDUCTOR_PORT, as the docs say, although
// Close forgets the lane (and with its record, its port) before the teardown runs.
func TestCloseTeardownGetsTheLanePort(t *testing.T) {
	t.Parallel()
	r := newCloseRepo(t)
	r.m.Cfg.WorktreeTeardown = &config.HookCommand{Command: []string{"true"}}
	path := r.lane("ported", "fix/ported", filepath.Join(r.root, ".wt", "ported"))
	rec, _ := r.m.Registry.Get("ported")
	rec.Port = 4730
	if err := r.m.Registry.Put(rec); err != nil {
		t.Fatal(err)
	}
	_, res := r.closeAll("ported")
	if exists(path) || !has(res.Removed, "worktree_teardown ran") {
		t.Fatalf("close: %+v", res)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.hooks) != 1 {
		t.Fatalf("hooks run: %v", r.hooks)
	}
	want := []string{"/usr/bin/env", "CLAUDUCTOR_LANE=ported", "CLAUDUCTOR_PORT=4730", "true"}
	if strings.Join(r.hooks[0], " ") != strings.Join(want, " ") {
		t.Fatalf("teardown argv %q, want %q", r.hooks[0], want)
	}
}
