package lanes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// PANEL-18: a clean detached worktree a closed session left behind goes when a branch
// holds its commit, and the plan names the branch. REMOVEWT-3-S2.
func TestRemoveWorktreeRemovesACleanDetachedOne(t *testing.T) {
	t.Parallel()
	r := newCloseRepo(t)
	wt := r.worktree("", filepath.Join(r.root, ".wt", "left-behind"))
	p, res := r.removeAll(wt)
	if !p.Worktree || p.DeleteBranch || p.Branch != "" || !has(p.Remove, "the worktree "+wt) || !has(p.Notes, "detached") || !has(p.Notes, "also on main, so nothing is lost") {
		t.Fatalf("plan %+v", p)
	}
	if exists(wt) || !has(res.Removed, "the worktree "+wt) || !has(res.Notes, "also on main, so nothing is lost") {
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

// #77: a clean detached worktree whose commit no branch, remote-tracking branch or tag
// holds stays, even under a forced consent: removing it would leave the commit to git's
// garbage collection. REMOVEWT-3-S1.
func TestRemoveWorktreeKeepsADetachedCommitOfItsOwn(t *testing.T) {
	t.Parallel()
	r := newCloseRepo(t)
	wt := r.worktree("", filepath.Join(r.root, ".wt", "own"))
	sha := r.commit(wt, "work.txt")
	want := "its commit " + short(sha) + " is on no branch or tag, so removing the folder would lose it. To keep it, put it on a branch first: git branch <name> " + short(sha)
	p, lerr := r.m.RemoveWorktreePlan(context.Background(), wt)
	if lerr != nil || p.Worktree || !has(p.Keep, "the worktree "+wt+": "+want) {
		t.Fatalf("plan %+v %v", p, lerr)
	}
	res, lerr := r.m.RemoveWorktree(context.Background(), wt, RemoveWorktreeRequest{Worktree: wt, Remove: true})
	if lerr != nil || len(res.Removed) != 0 || !has(res.Kept, want) || !exists(filepath.Join(wt, "work.txt")) {
		t.Fatalf("a forced consent: %+v %v", res, lerr)
	}
	// A stash is not where anyone looks for a commit: it does not count.
	r.git(r.root, "update-ref", "refs/stash", sha)
	if p, _ := r.m.RemoveWorktreePlan(context.Background(), wt); p.Worktree {
		t.Fatalf("refs/stash counted: plan %+v", p)
	}
}

// A detached worktree whose commit a ref holds may go, and the plan names one ref: a
// local branch first, then a remote-tracking branch, then a tag, each the first by name.
// REMOVEWT-3-S2.
func TestRemoveWorktreeNamesTheRefThatHoldsADetachedCommit(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		refs []string
		want string
	}{
		{"tag", []string{"refs/tags/v2", "refs/tags/v1"}, "also on tag v1,"},
		{"remote", []string{"refs/tags/v1", "refs/remotes/origin/zz", "refs/remotes/origin/aa"}, "also on origin/aa,"},
		{"branch", []string{"refs/tags/v1", "refs/remotes/origin/aa", "refs/heads/zz", "refs/heads/keep"}, "also on keep,"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newCloseRepo(t)
			wt := r.worktree("", filepath.Join(r.root, ".wt", c.name))
			sha := r.commit(wt, "work.txt")
			for _, ref := range c.refs {
				r.git(r.root, "update-ref", ref, sha)
			}
			if c.name == "remote" {
				// origin/HEAD sorts first, but it only names another ref.
				r.git(r.root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/zz")
			}
			p, res := r.removeAll(wt)
			if !p.Worktree || !has(p.Notes, "detached at "+short(sha)+", a commit that is "+c.want+" so nothing is lost") {
				t.Fatalf("plan %+v", p)
			}
			if exists(wt) || !has(res.Notes, c.want) {
				t.Fatalf("result %+v", res)
			}
		})
	}
}

// A commit made in a detached worktree after the check, just before `git worktree
// remove`, is checked too: HEAD is read again, and a commit on no ref keeps it.
func TestRemoveWorktreeRechecksADetachedHeadBeforeRemoving(t *testing.T) {
	t.Parallel()
	r := newCloseRepo(t)
	wt := r.worktree("", filepath.Join(r.root, ".wt", "late"))
	var mu sync.Mutex
	armed, committed := false, ""
	// The commit lands after the plan's checks and the act's check, as a terminal's
	// commit would: on the act's last read of the worktree's refs.
	r.m.Run = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		out, err := r.run(ctx, dir, argv)
		if len(argv) > 2 && argv[1] == "for-each-ref" && strings.HasPrefix(argv[2], "--contains") {
			mu.Lock()
			if committed == "" && strings.Contains(string(out), "refs/heads/main") && armed {
				committed = r.commit(wt, "late.txt")
			}
			mu.Unlock()
		}
		return out, err
	}
	p, lerr := r.m.RemoveWorktreePlan(context.Background(), wt)
	if lerr != nil || !p.Worktree {
		t.Fatalf("plan %+v %v", p, lerr)
	}
	mu.Lock()
	armed = true
	mu.Unlock()
	res, lerr := r.m.RemoveWorktree(context.Background(), wt, RemoveWorktreeRequest{Worktree: wt, Remove: p.Worktree})
	if lerr != nil || committed == "" || len(res.Removed) != 0 || !has(res.Kept, "its commit "+short(committed)+" is on no branch or tag") || !exists(filepath.Join(wt, "late.txt")) {
		t.Fatalf("a commit after the check: %+v %v (commit %q)", res, lerr, committed)
	}
}

// When which refs hold the commit cannot be read, the worktree stays: a commit that
// might be lost is not removed. REMOVEWT-3-S4.
func TestRemoveWorktreeKeepsADetachedOneWhenItsRefsCannotBeRead(t *testing.T) {
	t.Parallel()
	r := newCloseRepo(t)
	wt := r.worktree("", filepath.Join(r.root, ".wt", "unread"))
	head := r.git(wt, "rev-parse", "HEAD")
	r.m.Run = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		if len(argv) > 2 && argv[1] == "for-each-ref" && strings.HasPrefix(argv[2], "--contains") {
			return nil, errors.New("exit status 129")
		}
		return r.run(ctx, dir, argv)
	}
	want := "the panel couldn't tell whether its commit " + short(head) + " is on a branch (exit status 129), so it stays"
	p, lerr := r.m.RemoveWorktreePlan(context.Background(), wt)
	if lerr != nil || p.Worktree || !has(p.Keep, want) {
		t.Fatalf("plan %+v %v", p, lerr)
	}
	res, lerr := r.m.RemoveWorktree(context.Background(), wt, RemoveWorktreeRequest{Worktree: wt, Remove: true})
	if lerr != nil || len(res.Removed) != 0 || !has(res.Kept, want) || !exists(wt) {
		t.Fatalf("a forced consent: %+v %v", res, lerr)
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
