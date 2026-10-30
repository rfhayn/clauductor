package panel

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rfhayn/stridal/internal/panel/clock"
	"github.com/rfhayn/stridal/internal/panel/signals"
	"github.com/rfhayn/stridal/internal/panel/state"
)

// [STALL-1-S5] [STALL-1-S19] The merged read's cache (D10), on a fixed clock: "none"
// stands for Ticks.MergedNone, and is read again at once when the branch leaves the
// open-PR list (a squash merge leaves HEAD as it was, so a "none" from before the pull
// request was opened would otherwise stand after it merged); a merge found is kept for
// good; a failed read keeps the last result and is never "none"; and a lane with no
// result yet waits Ticks.MergedNone before it is read again (review round 3, L1), as
// "none" does; a merge counts only when its head commit is HEAD (#188).
func TestMergedReadCache(t *testing.T) {
	t.Parallel()
	answer, calls := "[]", 0
	var fail error
	t0 := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	r := &Runtime{root: "/repo", clock: clock.NewFake(t0), ticks: Ticks{MergedNone: 5 * time.Minute},
		run: func(_ context.Context, _ string, argv []string) ([]byte, error) {
			if strings.Join(argv, " ") != "gh pr list --head change/a --state merged --json number,headRefOid --limit 20" {
				t.Fatalf("unexpected %v", argv)
			}
			calls++
			return []byte(answer), fail
		}}
	wt := signals.Worktree{Path: "/repo/w/a", Branch: "change/a", Head: "aaaaaaa"}
	all := []signals.Worktree{{Path: "/repo", Branch: "main"}, wt}
	due := []signals.Worktree{wt}
	open := []signals.PR{{Number: 153, HeadRef: "change/a"}}
	ctx := context.Background()
	read := func(at time.Duration, prs []signals.PR, due []signals.Worktree) state.MergedRead {
		return r.readMerged(ctx, t0.Add(at), prs, all, due)[wt.Path]
	}

	// No result yet: a failed read leaves the lane with none, and waits MergedNone.
	fail = errors.New("gh: could not reach GitHub")
	if got := read(0, nil, due); got.Read || calls != 1 {
		t.Fatalf("a first read that failed: %+v after %d calls, want no result", got, calls)
	}
	if got := read(time.Second, nil, due); got.Read || calls != 1 {
		t.Fatalf("the next poll: %+v after %d calls, want no second try inside the wait and still no result", got, calls)
	}
	fail = nil
	if got := read(5*time.Minute, nil, due); !got.Read || got.PR != 0 || got.Branch != "change/a" || got.Head != "aaaaaaa" || calls != 2 {
		t.Fatalf("the read that answered none: %+v after %d calls", got, calls)
	}
	// "None" stands for MergedNone: no read until then.
	if read(9*time.Minute, nil, due); calls != 2 {
		t.Fatalf("\"none\" read again after 4 minutes (%d calls)", calls)
	}
	// A failed read after it keeps "none".
	fail = errors.New("gh: could not reach GitHub")
	if got := read(10*time.Minute+2*time.Second, nil, due); !got.Read || got.PR != 0 || calls != 3 {
		t.Fatalf("a failed read after \"none\": %+v after %d calls, want \"none\" kept", got, calls)
	}
	// The pull request is opened (the lane is not due a merged read while it is open),
	// and leaves the open list well inside MergedNone: read again at once, and while
	// that read fails, at each poll, since the "none" kept may now be a merge.
	if read(10*time.Minute+3*time.Second, open, nil); calls != 3 {
		t.Fatalf("a read while the pull request is open (%d calls)", calls)
	}
	if got := read(10*time.Minute+4*time.Second, nil, due); !got.Read || got.PR != 0 || calls != 4 {
		t.Fatalf("the branch left the open list, gh failing: %+v after %d calls, want \"none\" kept and a read", got, calls)
	}
	fail = nil
	// An older merge of the branch, at another head commit, is not HEAD's (#188).
	answer = `[{"number":77,"headRefOid":"0000000"}]`
	if got := read(10*time.Minute+5*time.Second, nil, due); !got.Read || got.PR != 0 || calls != 5 {
		t.Fatalf("an older merge on the reused branch: %+v after %d calls, want \"none\" for HEAD", got, calls)
	}
	read(10*time.Minute+6*time.Second, open, nil)
	answer = `[{"number":77,"headRefOid":"0000000"},{"number":153,"headRefOid":"aaaaaaa"}]`
	if got := read(10*time.Minute+7*time.Second, nil, due); got.PR != 153 || calls != 6 {
		t.Fatalf("the branch left the open list: %+v after %d calls, want #153 read at once", got, calls)
	}
	// A merge found is kept for good, whatever the clock.
	answer, fail = "[]", errors.New("gh: could not reach GitHub")
	if got := read(time.Hour, nil, due); got.PR != 153 || calls != 6 {
		t.Fatalf("a merge found: %+v after %d calls, want it kept and not read again", got, calls)
	}
	// A new HEAD has no result: read at once.
	fail = nil
	wt.Head = "bbbbbbb"
	all[1], due[0] = wt, wt
	if got := read(time.Hour+time.Second, nil, due); got.Head != "bbbbbbb" || got.PR != 0 || calls != 7 {
		t.Fatalf("a new HEAD: %+v after %d calls, want a read for it", got, calls)
	}
	// A worktree gone from the list is dropped.
	if got := r.readMerged(ctx, t0.Add(2*time.Hour), nil, all[:1], nil); len(got) != 0 {
		t.Fatalf("a removed worktree's read is kept: %+v", got)
	}
}

// Group 3, review round 2, L1 (D10): a "none" counts only when made after its branch
// last left the open-PR list (state's leftOpen, stamped with the hub's clock when the
// pull requests' update is applied). The readiness poll's now is taken before the read
// runs, so a pull-request update landing while the read is out stamps leftOpen after
// the poll's now: the read's At must be when it answered, not when the poll began.
func TestMergedReadStampsWhenItAnswered(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	clk := clock.NewFake(t0)
	r := &Runtime{root: "/repo", clock: clk, ticks: Ticks{MergedNone: 5 * time.Minute},
		run: func(_ context.Context, _ string, argv []string) ([]byte, error) {
			clk.Advance(2 * time.Second) // the pull requests' update lands at t0+1s, meanwhile
			return []byte("[]"), nil
		}}
	wt := signals.Worktree{Path: "/repo/w/a", Branch: "change/a", Head: "aaaaaaa"}
	got := r.readMerged(context.Background(), t0, nil, []signals.Worktree{wt}, []signals.Worktree{wt})[wt.Path]
	if left := t0.Add(time.Second); !got.Read || !got.At.After(left) {
		t.Fatalf("a \"none\" answered at t0+2s, its branch leaving the open list at t0+1s: At %v, want after %v", got.At, left)
	}
}

// Review round 3, L1 (D10): a failed merged read with no result yet is not tried again
// before Ticks.MergedNone, as "none" is not, so a gh that keeps failing costs one read
// per lane per wait, not one per lane per poll. The lane stays with no result ((b)
// unknown), never "none"; a new HEAD is read at once.
func TestMergedReadFailureWaitsLaneAfterLane(t *testing.T) {
	t.Parallel()
	calls := map[string]int{}
	r := &Runtime{root: "/repo", ticks: Ticks{MergedNone: 5 * time.Minute},
		run: func(_ context.Context, _ string, argv []string) ([]byte, error) {
			calls[argv[4]]++
			return nil, errors.New("gh: could not reach GitHub")
		}}
	a := signals.Worktree{Path: "/repo/w/a", Branch: "change/a", Head: "aaaaaaa"}
	b := signals.Worktree{Path: "/repo/w/b", Branch: "change/b", Head: "bbbbbbb"}
	all := []signals.Worktree{{Path: "/repo", Branch: "main"}, a, b}
	due := []signals.Worktree{a, b}
	t0 := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	ctx := context.Background()
	// A poll every 30 s (Ticks.Git) for just under the wait.
	for at := time.Duration(0); at < 5*time.Minute; at += 30 * time.Second {
		if got := r.readMerged(ctx, t0.Add(at), nil, all, due); len(got) != 0 {
			t.Fatalf("failed reads at %v gave a result: %+v, want none (never \"none\")", at, got)
		}
	}
	if calls["change/a"] != 1 || calls["change/b"] != 1 {
		t.Fatalf("10 polls inside the wait, every read failing: %v merged reads, want 1 per lane", calls)
	}
	r.readMerged(ctx, t0.Add(5*time.Minute), nil, all, due)
	if calls["change/a"] != 2 || calls["change/b"] != 2 {
		t.Fatalf("at the wait's end: %v merged reads, want 2 per lane", calls)
	}
	a.Head = "ccccccc"
	all[1], due[0] = a, a
	r.readMerged(ctx, t0.Add(5*time.Minute+time.Second), nil, all, due)
	if calls["change/a"] != 3 || calls["change/b"] != 2 {
		t.Fatalf("lane a's new HEAD: %v merged reads, want lane a read at once and lane b not", calls)
	}
}

// Review round 1, L7 (D10): the receipt read runs with no page every Ticks.Git, so a
// listed worktree whose directory is gone spawns no `git rev-parse` (which fails, with
// nothing cached, at every poll), and a worktree gone from the list leaves the cache.
func TestGitDirOfSkipsAGoneWorktree(t *testing.T) {
	t.Parallel()
	calls := 0
	r := &Runtime{root: "/repo", run: func(_ context.Context, _ string, argv []string) ([]byte, error) {
		calls++
		return []byte("/repo/.git/worktrees/x\n"), nil
	}}
	r.ready.gitDirs = map[string]string{}
	gone := filepath.Join(t.TempDir(), "removed")
	for i := 0; i < 3; i++ {
		if gd := r.gitDirOf(context.Background(), gone); gd != "" || calls != 0 {
			t.Fatalf("a worktree whose directory is gone: git dir %q after %d spawns, want none", gd, calls)
		}
	}
	here := t.TempDir()
	if gd := r.gitDirOf(context.Background(), here); gd != "/repo/.git/worktrees/x" || calls != 1 {
		t.Fatalf("a worktree that exists: git dir %q after %d spawns", gd, calls)
	}
	r.pruneGitDirs([]signals.Worktree{{Path: "/repo"}})
	if _, ok := r.ready.gitDirs[here]; ok {
		t.Fatal("a worktree gone from the list keeps its cached git dir")
	}
}
