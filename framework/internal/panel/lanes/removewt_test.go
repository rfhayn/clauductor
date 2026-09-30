package lanes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// worktree adds a worktree with no lane: on a new branch, or detached when branch is "".
func (r *closeRepo) worktree(branch, path string) string {
	r.t.Helper()
	if branch == "" {
		r.git(r.root, "worktree", "add", "-q", "--detach", path, "main")
	} else {
		r.git(r.root, "worktree", "add", "-q", "-b", branch, path, "main")
	}
	return signals.ResolvePath(path)
}

// removeAll plans, then removes with exactly what the plan offered.
func (r *closeRepo) removeAll(key string) (WorktreePlan, WorktreeResult) {
	r.t.Helper()
	p, lerr := r.m.RemoveWorktreePlan(context.Background(), key)
	if lerr != nil {
		r.t.Fatalf("plan %s: %v", key, lerr)
	}
	res, lerr := r.m.RemoveWorktree(context.Background(), key, RemoveWorktreeRequest{Worktree: key, Remove: p.Worktree, Branch: p.DeleteBranch})
	if lerr != nil {
		r.t.Fatalf("remove %s: %v", key, lerr)
	}
	return p, res
}

// PANEL-18: a clean detached worktree a closed session left behind goes, and the plan
// says there is no branch to delete.
func TestRemoveWorktreeRemovesACleanDetachedOne(t *testing.T) {
	t.Parallel()
	r := newCloseRepo(t)
	wt := r.worktree("", filepath.Join(r.root, ".wt", "left-behind"))
	p, res := r.removeAll(wt)
	if !p.Worktree || p.DeleteBranch || p.Branch != "" || !has(p.Remove, "the worktree "+wt) || !has(p.Notes, "detached") || !has(p.Notes, "no branch to delete") {
		t.Fatalf("plan %+v", p)
	}
	if exists(wt) || !has(res.Removed, "the worktree "+wt) || !has(res.Notes, "detached") {
		t.Fatalf("result %+v", res)
	}
	wts, _ := signals.ReadWorktrees(context.Background(), r.run, r.root)
	for _, w := range wts {
		if w.Path == wt {
			t.Fatal("git worktree list still has it")
		}
	}
	// Gone now: a second request names nothing.
	if _, lerr := r.m.RemoveWorktreePlan(context.Background(), wt); lerr == nil || lerr.Status != 404 {
		t.Fatalf("a removed worktree planned again: %v", lerr)
	}
}

// Close lane's rules hold, and more: never a worktree a lane is registered in (even an
// orphan), one a claude session runs in, or one where that cannot be ruled out.
func TestRemoveWorktreeRefuses(t *testing.T) {
	t.Parallel()
	r := newCloseRepo(t)
	dirty := r.worktree("", filepath.Join(r.root, ".wt", "dirty"))
	os.WriteFile(filepath.Join(dirty, "notes.txt"), []byte("unsaved"), 0o644)
	locked := r.worktree("fix/locked", filepath.Join(r.root, ".wt", "locked"))
	r.git(r.root, "worktree", "lock", locked)
	outside := r.worktree("", filepath.Join(t.TempDir(), "outside"))
	orphan := r.lane("orphan", "fix/orphan", filepath.Join(r.root, ".wt", "orphan"))
	live := r.worktree("", filepath.Join(r.root, ".wt", "live"))
	os.MkdirAll(filepath.Join(live, "sub"), 0o755)
	r.mu.Lock()
	// A session in a subdirectory of "live", and one in the main checkout (which must
	// not count against the worktrees nested inside it).
	r.agents = fmt.Sprintf(`[{"pid":4242,"cwd":%q,"sessionId":"a","status":"busy"},{"pid":4343,"cwd":%q,"sessionId":"b","status":"idle"}]`,
		filepath.Join(live, "sub"), r.root)
	r.mu.Unlock()
	for key, want := range map[string]string{
		dirty:   "uncommitted or untracked",
		locked:  "locked",
		r.root:  "main worktree",
		outside: "outside the project's worktree_dir",
		orphan:  "lane orphan is registered in it",
		live:    "a claude session runs in it (pid 4242",
	} {
		p, lerr := r.m.RemoveWorktreePlan(context.Background(), key)
		if lerr != nil || p.Worktree || p.DeleteBranch || !has(p.Keep, want) {
			t.Fatalf("%s: plan %+v %v, want kept: %s", key, p, lerr, want)
		}
		res, lerr := r.m.RemoveWorktree(context.Background(), key, RemoveWorktreeRequest{Worktree: key, Remove: true, Branch: true})
		if lerr != nil || len(res.Removed) != 0 || !has(res.Kept, want) || !exists(key) {
			t.Fatalf("%s: a forced consent: %+v %v", key, res, lerr)
		}
	}
	if !exists(filepath.Join(dirty, "notes.txt")) || !r.branchExists("fix/locked") || !r.branchExists("fix/orphan") {
		t.Fatal("something was removed")
	}

	// claude agents unreadable: a session cannot be ruled out, so it stays.
	clean := r.worktree("", filepath.Join(r.root, ".wt", "clean"))
	r.mu.Lock()
	r.agents = ""
	r.mu.Unlock()
	if p, _ := r.m.RemoveWorktreePlan(context.Background(), clean); p.Worktree || !has(p.Keep, "cannot read claude agents") {
		t.Fatalf("unreadable claude agents: plan %+v", p)
	}
	if res, _ := r.m.RemoveWorktree(context.Background(), clean, RemoveWorktreeRequest{Worktree: clean, Remove: true}); !exists(clean) || !has(res.Kept, "cannot read claude agents") {
		t.Fatalf("unreadable claude agents: %+v", res)
	}
	r.mu.Lock()
	r.agents = "[]"
	r.mu.Unlock()
	// Without the consent the plan offered, nothing goes either.
	if res, lerr := r.m.RemoveWorktree(context.Background(), clean, RemoveWorktreeRequest{Worktree: clean}); lerr != nil || !exists(clean) || !has(res.Kept, "the confirmation did not include it") {
		t.Fatalf("no consent: %+v %v", res, lerr)
	}
}

// The key must be a `git worktree list` entry, exactly: no other path is acted on.
func TestRemoveWorktreeNamesOnlyAListedWorktree(t *testing.T) {
	t.Parallel()
	r := newCloseRepo(t)
	wt := r.worktree("", filepath.Join(r.root, ".wt", "x"))
	for key, status := range map[string]int{
		"":                                400,
		".wt/x":                           400,
		wt + "/":                          404,
		wt + "/..":                        404,
		filepath.Join(r.root, ".wt"):      404,
		filepath.Join(r.root, ".wt", "y"): 404,
		"/":                               404,
	} {
		if _, lerr := r.m.RemoveWorktreePlan(context.Background(), key); lerr == nil || lerr.Status != status {
			t.Fatalf("plan %q: %v, want %d", key, lerr, status)
		}
		if _, lerr := r.m.RemoveWorktree(context.Background(), key, RemoveWorktreeRequest{Worktree: key, Remove: true, Branch: true}); lerr == nil || lerr.Status != status {
			t.Fatalf("remove %q: %v, want %d", key, lerr, status)
		}
	}
	if !exists(wt) || !exists(filepath.Join(r.root, ".wt")) {
		t.Fatal("a path that was not the key was removed")
	}
}

// Its branch goes only under Close lane's rules: merged into the base, and offered.
func TestRemoveWorktreeDeletesOnlyAMergedBranch(t *testing.T) {
	t.Parallel()
	r := newCloseRepo(t)
	merged := r.worktree("fix/merged", filepath.Join(r.root, ".wt", "merged"))
	r.commit(merged, "m.txt")
	r.git(r.root, "merge", "-q", "--no-ff", "-m", "merge", "fix/merged")
	p, res := r.removeAll(merged)
	if !p.Worktree || !p.DeleteBranch || !has(p.Remove, "every commit of it is in main") || exists(merged) || r.branchExists("fix/merged") || !has(res.Removed, "the local branch fix/merged") {
		t.Fatalf("merged: plan %+v, result %+v", p, res)
	}

	open := r.worktree("fix/open", filepath.Join(r.root, ".wt", "open"))
	r.commit(open, "o.txt")
	p, res = r.removeAll(open)
	if !p.Worktree || p.DeleteBranch || !has(p.Keep, "not merged into main") || exists(open) || !r.branchExists("fix/open") || !has(res.Kept, "fix/open") {
		t.Fatalf("open: plan %+v, result %+v", p, res)
	}

	// Merged, but the confirmation did not offer the branch: it stays.
	late := r.worktree("fix/late", filepath.Join(r.root, ".wt", "late"))
	res, lerr := r.m.RemoveWorktree(context.Background(), late, RemoveWorktreeRequest{Worktree: late, Remove: true})
	if lerr != nil || exists(late) || !r.branchExists("fix/late") || !has(res.Kept, "the confirmation did not include it") {
		t.Fatalf("late: %+v %v", res, lerr)
	}
}
