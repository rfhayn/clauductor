package panel

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/testbin"
	"github.com/clauductor/clauductor/internal/testwait"
	"github.com/clauductor/clauductor/internal/treesnap"
)

// cuPanelJSON is the panel.json a full panel's project has: base is the default,
// origin/main.
const cuPanelJSON = `{"name":"T","lanes":{"main":"orchestrator"}}`

// cuWorld is a project for catch-up (OPS-118 D3): a bare remote; root, the project's
// main checkout, a clone on main tracking origin/main; other, a second clone that
// pushes the commits the remote moves to; a fake gh, written by testbin, whose answers
// are files in its state directory; and a Runner that records every git and gh api
// call, runs git for real, and answers `gh pr list` with the open list the test sets.
type cuWorld struct {
	dir, remote, root, other, home string
	ghDir, gh                      string

	mu    sync.Mutex
	calls [][]string
	open  []int
}

func newCUWorld(t *testing.T, panelJSON string) *cuWorld {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := signals.ResolvePath(t.TempDir())
	w := &cuWorld{dir: dir, remote: filepath.Join(dir, "remote.git"), root: filepath.Join(dir, "root"),
		other: filepath.Join(dir, "other"), home: filepath.Join(dir, "home"), ghDir: filepath.Join(dir, "gh"), gh: filepath.Join(dir, "gh-bin")}
	gitRun(t, dir, "init", "-q", "--bare", "-b", "main", w.remote)
	writeFile(t, filepath.Join(w.root, "a.txt"), "one\n")
	if panelJSON != "" {
		writeFile(t, filepath.Join(w.root, config.DefaultConfigRel), panelJSON)
	}
	gitRun(t, w.root, "init", "-q", "-b", "main")
	gitRun(t, w.root, "add", "-A")
	gitRun(t, w.root, "commit", "-q", "-m", "one")
	gitRun(t, w.root, "remote", "add", "origin", w.remote)
	gitRun(t, w.root, "push", "-q", "-u", "origin", "main")
	gitRun(t, dir, "clone", "-q", w.remote, w.other)
	if err := os.MkdirAll(w.home, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(w.ghDir, "user.json"), `{"login":"owner"}`)
	testbin.Write(t, w.gh, `#!/bin/sh
d='`+w.ghDir+`'
[ "$1" = api ] || { echo "fake gh: unexpected $*" >&2; exit 2; }
case "$2" in
user) cat "$d/user.json" ;;
repos/*/commits/*/pulls)
	if [ -f "$d/hook" ]; then sh "$d/hook" || exit 3; rm -f "$d/hook"; fi
	if [ -s "$d/fail" ] && [ "$(cat "$d/fail")" -gt 0 ]; then
		echo $(( $(cat "$d/fail") - 1 )) > "$d/fail"
		echo "HTTP 502: Bad Gateway (https://api.github.com/$2)" >&2
		exit 1
	fi
	sha=${2#repos/*/commits/}; sha=${sha%/pulls}
	if [ -f "$d/commits/$sha.json" ]; then cat "$d/commits/$sha.json"; else echo '[]'; fi ;;
repos/*/pulls/*)
	n=${2##*/}
	if [ -f "$d/pulls/$n.json" ]; then cat "$d/pulls/$n.json"; else echo "HTTP 404: Not Found" >&2; exit 1; fi ;;
*) echo "fake gh: unexpected $*" >&2; exit 2 ;;
esac
`)
	return w
}

func (w *cuWorld) run(ctx context.Context, dir string, argv []string) ([]byte, error) {
	switch {
	case len(argv) > 0 && argv[0] == "git":
		w.record(argv)
		return signals.ExecRunner(ctx, dir, argv)
	case len(argv) > 1 && argv[0] == "gh" && argv[1] == "api":
		w.record(argv)
		return signals.ExecRunner(ctx, dir, append([]string{w.gh}, argv[1:]...))
	case len(argv) > 2 && argv[0] == "gh" && argv[1] == "pr" && argv[2] == "list" && !slices.Contains(argv, "--state"):
		w.mu.Lock()
		defer w.mu.Unlock()
		rows := make([]string, len(w.open))
		for i, n := range w.open {
			rows[i] = fmt.Sprintf(`{"number":%d,"title":"pr %d","headRefName":"lane-%d","author":{"login":"owner"},"isDraft":false}`, n, n, n)
		}
		return []byte("[" + strings.Join(rows, ",") + "]"), nil
	}
	return gitOnlyRunner(ctx, dir, argv)
}

func (w *cuWorld) record(argv []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls = append(w.calls, slices.Clone(argv))
}

// setOpen is the open list `gh pr list` answers from here on.
func (w *cuWorld) setOpen(prs ...int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.open = prs
}

// ran counts the git calls whose subcommand is verb (past -C <dir> and the options).
func (w *cuWorld) ran(verb string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, c := range w.calls {
		if c[0] != "git" {
			continue
		}
		for i := 1; i < len(c); i++ {
			if c[i] == "-C" {
				i++
				continue
			}
			if !strings.HasPrefix(c[i], "-") {
				if c[i] == verb {
					n++
				}
				break
			}
		}
	}
	return n
}

// ghCalls are the gh api calls that name s.
func (w *cuWorld) ghCalls(s string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, c := range w.calls {
		if c[0] == "gh" && strings.Contains(strings.Join(c, " "), s) {
			n++
		}
	}
	return n
}

// push commits file in other and pushes it: a commit the remote's main moves to, which
// came with no pull request.
func (w *cuWorld) push(t *testing.T, file, body string) string {
	t.Helper()
	writeFile(t, filepath.Join(w.other, file), body)
	gitRun(t, w.other, "add", "-A")
	gitRun(t, w.other, "commit", "-q", "-m", file)
	gitRun(t, w.other, "push", "-q", "origin", "main")
	return gitRun(t, w.other, "rev-parse", "HEAD")
}

// merged is push, as the merge commit of pull request n merged into main from by.
func (w *cuWorld) merged(t *testing.T, n int, by, file, body string) string {
	t.Helper()
	sha := w.push(t, file, body)
	writeFile(t, filepath.Join(w.ghDir, "commits", sha+".json"), fmt.Sprintf(`[{"number":%d,"merge_commit_sha":%q}]`, n, sha))
	writeFile(t, filepath.Join(w.ghDir, "pulls", fmt.Sprint(n)+".json"),
		fmt.Sprintf(`{"number":%d,"merged":true,"merged_by":{"login":%q},"merge_commit_sha":%q,"base":{"ref":"main"}}`, n, by, sha))
	return sha
}

// failLookups makes the next n commit-to-pull-request lookups fail.
func (w *cuWorld) failLookups(t *testing.T, n int) {
	t.Helper()
	writeFile(t, filepath.Join(w.ghDir, "fail"), fmt.Sprint(n))
}

func (w *cuWorld) catchUpOn(t *testing.T, on bool) {
	t.Helper()
	if err := config.SetCatchUp(w.home, w.root, on); err != nil {
		t.Fatal(err)
	}
}

func (w *cuWorld) head(t *testing.T) string {
	t.Helper()
	return gitRun(t, w.root, "rev-parse", "HEAD")
}

// cuState is "as before": HEAD's commit, what HEAD names (a branch, or HEAD when
// detached), and every file of the checkout outside .git, which a fetch writes to.
type cuState struct {
	head, ref string
	files     treesnap.Snapshot
}

func (w *cuWorld) state(t *testing.T) cuState {
	t.Helper()
	files := treesnap.Take(t, w.root)
	for p := range files {
		if p == ".git" || strings.HasPrefix(p, ".git/") {
			delete(files, p)
		}
	}
	return cuState{head: w.head(t), ref: gitRun(t, w.root, "rev-parse", "--symbolic-full-name", "HEAD"), files: files}
}

func (w *cuWorld) same(t *testing.T, before cuState) {
	t.Helper()
	after := w.state(t)
	if after.head != before.head || after.ref != before.ref {
		t.Fatalf("HEAD moved: %s (%s), was %s (%s)", after.head, after.ref, before.head, before.ref)
	}
	if d := treesnap.Diff(before.files, after.files); len(d) > 0 {
		t.Fatalf("the checkout's files changed:\n%s", strings.Join(d, "\n"))
	}
}

// cuLog is a panel log tests read while the panel writes it.
type cuLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *cuLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *cuLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// catchUpNotices are the notices catch-up sent: its keys are its own.
func catchUpNotices(l *noticeLog) []state.Notice {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []state.Notice
	for _, n := range l.ns {
		if len(n.Keys) > 0 && strings.HasPrefix(n.Keys[0], "catch-up\x00") {
			out = append(out, n)
		}
	}
	return out
}

// oneNotice is the one catch-up notice sent, which must be titled "clauductor panel"
// and name the checkout and each of want.
func (w *cuWorld) oneNotice(t *testing.T, l *noticeLog, want ...string) state.Notice {
	t.Helper()
	ns := catchUpNotices(l)
	if len(ns) != 1 {
		t.Fatalf("catch-up sent %d notices, want 1: %+v", len(ns), ns)
	}
	n := ns[0]
	if n.Title != "clauductor panel" {
		t.Fatalf("notice title %q, want clauductor panel", n.Title)
	}
	for _, s := range append([]string{w.root}, want...) {
		if !strings.Contains(n.Body, s) {
			t.Fatalf("notice %q does not name %q", n.Body, s)
		}
	}
	return n
}

var cuT0 = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

// newCatchUp is the catch-up source alone, on w's checkout, on clk, its notices and log
// recorded and push.json read from w's home.
func (w *cuWorld) newCatchUp(clk clock.Clock, notices *noticeLog, log *cuLog) *catchUp {
	return newCatchUp(Options{Home: w.home, Runner: w.run, Out: log, Notify: notices.notify}, clk, newPushWatch(w.home).current,
		func() []catchUpTarget { return []catchUpTarget{{root: w.root, base: "origin/main"}} })
}

// trigger is a pull request leaving the open list: the prs source's wake, then the
// source's poll. It returns the wait the poll asks for.
func trigger(c *catchUp, clk clock.Clock) time.Duration {
	c.wake(c.targets()[0].root)
	_, next := c.poll(context.Background(), clk.Now())
	return next
}

// [CATCHUP-2-S2] A tracked change stops the fast-forward: HEAD and files as before, one
// notice naming the checkout and the tracked change, and a second pull request leaving
// the list with origin/main unchanged sends none.
func TestCatchUpTrackedChangeSaysSoOnce(t *testing.T) {
	t.Parallel()
	w := newCUWorld(t, "")
	w.catchUpOn(t, true)
	w.merged(t, 7, "owner", "b.txt", "two\n")
	writeFile(t, filepath.Join(w.root, "a.txt"), "changed\n")
	before := w.state(t)
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	var log cuLog
	c := w.newCatchUp(clk, &notices, &log)

	trigger(c, clk) // #7 leaves
	trigger(c, clk) // #8 leaves, origin/main unchanged
	w.same(t, before)
	w.oneNotice(t, &notices, "a.txt")
	if n := strings.Count(log.String(), "catch-up "+w.root+": a tracked file differs"); n != 2 {
		t.Fatalf("log has %d refusal lines, want one per trigger:\n%s", n, log.String())
	}
	if w.ran("merge") != 0 {
		t.Fatal("git merge ran over a tracked change")
	}
}

// [CATCHUP-2-S3] A checkout on another branch, one with a detached HEAD, and one on main
// with a commit origin/main lacks are each left as they were, a notice naming why.
func TestCatchUpLeavesAnotherBranchDetachedOrAhead(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		setup func(t *testing.T, w *cuWorld)
		why   string
	}{
		"another branch": {func(t *testing.T, w *cuWorld) { gitRun(t, w.root, "checkout", "-q", "-b", "feature") }, "HEAD is on feature, not main"},
		"detached HEAD":  {func(t *testing.T, w *cuWorld) { gitRun(t, w.root, "checkout", "-q", "--detach") }, "HEAD is detached"},
		"a commit of its own": {func(t *testing.T, w *cuWorld) {
			writeFile(t, filepath.Join(w.root, "mine.txt"), "mine\n")
			gitRun(t, w.root, "add", "-A")
			gitRun(t, w.root, "commit", "-q", "-m", "mine")
		}, "main has 1 commit(s) origin/main lacks"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w := newCUWorld(t, "")
			w.catchUpOn(t, true)
			w.merged(t, 7, "owner", "b.txt", "two\n")
			tc.setup(t, w)
			before := w.state(t)
			clk := clock.NewFake(cuT0)
			var notices noticeLog
			c := w.newCatchUp(clk, &notices, &cuLog{})
			trigger(c, clk)
			w.same(t, before)
			w.oneNotice(t, &notices, tc.why)
			if w.ran("merge") != 0 || w.ran("fetch") != 0 {
				t.Fatalf("a refused precondition still ran fetch %d, merge %d", w.ran("fetch"), w.ran("merge"))
			}
		})
	}
}

// [CATCHUP-2-S5] An untracked file at a path the next commit adds stops the merge: HEAD
// and the file as before, the notice names git's error, and with 1, 3 and 6 minutes
// passing and no other trigger, no second git merge runs.
func TestCatchUpUntrackedFileStopsIt(t *testing.T) {
	t.Parallel()
	w := newCUWorld(t, "")
	w.catchUpOn(t, true)
	w.merged(t, 7, "owner", "new.txt", "theirs\n")
	writeFile(t, filepath.Join(w.root, "new.txt"), "mine\n")
	before := w.state(t)
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	c := w.newCatchUp(clk, &notices, &cuLog{})

	if next := trigger(c, clk); next != 0 {
		t.Fatalf("git's refusal asks for a retry in %s; it waits for the next trigger", next)
	}
	w.same(t, before)
	w.oneNotice(t, &notices, "untracked working tree files would be overwritten")
	if w.ran("merge") != 1 {
		t.Fatalf("git merge ran %d times, want 1", w.ran("merge"))
	}
	for _, d := range []time.Duration{time.Minute, 2 * time.Minute, 3 * time.Minute} { // at 1, 3 and 6 minutes
		clk.Advance(d)
		c.poll(context.Background(), clk.Now())
	}
	if w.ran("merge") != 1 {
		t.Fatalf("git merge ran %d times by 6 minutes on, want 1: git's refusal is not retried", w.ran("merge"))
	}
	w.same(t, before)
}

// [CATCHUP-2-S6] A commit pushed with no pull request after the owner's #7 stops the
// fast-forward: HEAD and files as before, and the notice names the pushed commit and
// says it came with no pull request.
func TestCatchUpRefusesACommitNoPullRequestMerged(t *testing.T) {
	t.Parallel()
	w := newCUWorld(t, "")
	w.catchUpOn(t, true)
	w.merged(t, 7, "owner", "b.txt", "two\n")
	pushed := w.push(t, "c.txt", "three\n")
	before := w.state(t)
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	c := w.newCatchUp(clk, &notices, &cuLog{})
	if next := trigger(c, clk); next != 0 {
		t.Fatalf("a refusal of policy asks for a retry in %s", next)
	}
	w.same(t, before)
	w.oneNotice(t, &notices, short(pushed), "came with no pull request")
}

// [CATCHUP-2-S7] #7 merged from `other`, while gh api user names `owner`, stops the
// fast-forward, and the notice names #7 and other.
func TestCatchUpRefusesAPullRequestMergedFromAnotherAccount(t *testing.T) {
	t.Parallel()
	w := newCUWorld(t, "")
	w.catchUpOn(t, true)
	w.merged(t, 7, "other", "b.txt", "two\n")
	before := w.state(t)
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	c := w.newCatchUp(clk, &notices, &cuLog{})
	trigger(c, clk)
	w.same(t, before)
	w.oneNotice(t, &notices, "#7", "merged by other")
	if w.ran("merge") != 0 {
		t.Fatal("git merge ran over another account's pull request")
	}
}

// [CATCHUP-2-S8] gh failing on the commit's pull-request lookup stops the fast-forward,
// and the notice names the failed lookup.
func TestCatchUpFailedLookupStopsIt(t *testing.T) {
	t.Parallel()
	w := newCUWorld(t, "")
	w.catchUpOn(t, true)
	sha := w.merged(t, 7, "owner", "b.txt", "two\n")
	w.failLookups(t, 100)
	before := w.state(t)
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	c := w.newCatchUp(clk, &notices, &cuLog{})
	if next := trigger(c, clk); next != time.Minute {
		t.Fatalf("a failed lookup asks for a retry in %s, want a minute", next)
	}
	w.same(t, before)
	w.oneNotice(t, &notices, "pull-request lookup for commit "+short(sha)+" failed", "HTTP 502")
	if w.ran("merge") != 0 {
		t.Fatal("git merge ran past a failed lookup")
	}
}

// [CATCHUP-2-S9] A lookup that fails once is tried again a minute later, on the clock
// the source's run loop waits on, and moves HEAD to origin/main; after a refusal of
// policy (a commit with no pull request) nothing is tried again before the next trigger.
func TestCatchUpRetriesALookupThatFailedOnce(t *testing.T) {
	t.Parallel()
	w := newCUWorld(t, "")
	sha := w.merged(t, 7, "owner", "b.txt", "two\n")
	w.failLookups(t, 1)
	a := w.head(t)
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	c := w.newCatchUp(clk, &notices, &cuLog{})

	ctx, cancel := context.WithCancel(context.Background())
	polled := make(chan time.Duration, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runLoop(ctx, clk, c.source(), func(ctx context.Context, s *source) (time.Duration, bool) {
			_, next := s.poll(ctx, clk.Now())
			if ctx.Err() != nil {
				return 0, false
			}
			polled <- next
			return next, true
		})
	}()
	defer func() { cancel(); <-done }()
	recv := func() time.Duration {
		t.Helper()
		select {
		case next := <-polled:
			return next
		case <-time.After(testwait.Scale(15 * time.Second)):
			t.Fatal("the catch-up source never polled")
		}
		return 0
	}
	recv() // the panel's start, catch-up off: nothing
	w.catchUpOn(t, true)
	c.wake(w.root) // #7 leaves
	if next := recv(); next != time.Minute {
		t.Fatalf("the failed lookup asks for a retry in %s, want a minute", next)
	}
	if got := w.head(t); got != a {
		t.Fatalf("HEAD moved to %s past a failed lookup", got)
	}
	clk.BlockUntil(1) // the retry's timer
	clk.Advance(time.Minute)
	recv()
	if got := w.head(t); got != sha {
		t.Fatalf("HEAD is %s a minute after the failed lookup, want origin/main's %s", got, sha)
	}
	cancel()
	<-done

	// A commit no pull request merged is policy: no retry, at 1, 3 or 6 minutes.
	pushed := w.push(t, "c.txt", "three\n")
	if next := trigger(c, clk); next != 0 {
		t.Fatalf("a refusal of policy asks for a retry in %s", next)
	}
	for _, d := range []time.Duration{time.Minute, 2 * time.Minute, 3 * time.Minute} {
		clk.Advance(d)
		c.poll(context.Background(), clk.Now())
	}
	if n := w.ghCalls("commits/" + pushed + "/pulls"); n != 1 {
		t.Fatalf("the commit with no pull request was looked up %d times, want once until the next trigger", n)
	}
	if got := w.head(t); got != sha {
		t.Fatalf("HEAD moved to %s past a commit with no pull request", got)
	}
}

// [CATCHUP-2-S10] origin/main moving to D while C's lookup runs (another worktree's
// fetch) does not carry the merge past C: HEAD is C, D is never looked up, and no
// notice names D.
func TestCatchUpMergesTheCommitItChecked(t *testing.T) {
	t.Parallel()
	w := newCUWorld(t, "")
	w.catchUpOn(t, true)
	c := w.merged(t, 7, "owner", "b.txt", "two\n")
	// D, committed in other and pushed with no pull request only during the lookup;
	// its objects are in the checkout beforehand, so update-ref can name it.
	writeFile(t, filepath.Join(w.other, "d.txt"), "four\n")
	gitRun(t, w.other, "add", "-A")
	gitRun(t, w.other, "commit", "-q", "-m", "d")
	d := gitRun(t, w.other, "rev-parse", "HEAD")
	gitRun(t, w.root, "fetch", "-q", w.other, "main:refs/test/d")
	writeFile(t, filepath.Join(w.ghDir, "hook"), fmt.Sprintf("git -C '%s' push -q origin main && git -C '%s' update-ref refs/remotes/origin/main %s\n", w.other, w.root, d))
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	var log cuLog
	cu := w.newCatchUp(clk, &notices, &log)
	trigger(cu, clk)

	if _, err := os.Stat(filepath.Join(w.ghDir, "hook")); !os.IsNotExist(err) {
		t.Fatal("the lookup's hook never ran: origin/main did not move during it")
	}
	if got := gitRun(t, w.root, "rev-parse", "refs/remotes/origin/main"); got != d {
		t.Fatalf("origin/main is %s after the hook, want D %s", got, d)
	}
	if got := w.head(t); got != c {
		t.Fatalf("HEAD is %s, want C %s (D is %s)", got, c, d)
	}
	if n := w.ghCalls(d); n != 0 {
		t.Fatalf("D was looked up %d times", n)
	}
	if ns := catchUpNotices(&notices); len(ns) != 0 {
		t.Fatalf("notices after a fast-forward to C: %+v", ns)
	}
	if !strings.Contains(log.String(), "from "+gitRun(t, w.root, "rev-parse", "HEAD~1")+" to "+c) {
		t.Fatalf("no log line names the fast-forward to C:\n%s", log.String())
	}
}

// startCatchUpPanel starts a full panel on w's checkout and sock (throwawaySocket,
// taken before the world so -short skips first): catch-up's source runs on the
// liveSet, woken by the project's prs source, which polls fast.
func (w *cuWorld) startCatchUpPanel(t *testing.T, sock string, notices *noticeLog, log *cuLog) *panelRun {
	t.Helper()
	return startPanelWith(t, w.root, w.home, sock, func(o *Options) {
		o.Runner = w.run
		o.Notify = notices.notify
		o.Out = log
		o.Ticks.PRs = 50 * time.Millisecond
	})
}

// running fails the test if the panel has stopped.
func running(t *testing.T, p *panelRun) {
	t.Helper()
	select {
	case err := <-p.done:
		t.Fatalf("the panel stopped: %v", err)
	default:
	}
}

// [CATCHUP-2-S1] With catch-up on, #7 leaving the open list brings the clean main
// checkout to origin/main's commit, #7's merge commit, and the panel log names the old
// and new commits.
func TestCatchUpFastForwardsAfterAMerge(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	w := newCUWorld(t, cuPanelJSON)
	w.catchUpOn(t, true)
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	p := w.startCatchUpPanel(t, sock, &notices, &log)
	p.polls.until(t, "catch-up", 1) // the start: nothing to move yet
	p.polls.until(t, "prs", 1)      // #7 is on the open list
	a := w.head(t)
	c := w.merged(t, 7, "owner", "b.txt", "two\n")
	w.setOpen() // #7 merged: it leaves the open list
	waitFor(t, "HEAD at origin/main's commit", func() bool { return w.head(t) == c })
	if !strings.Contains(log.String(), "catch-up "+w.root+": fast-forwarded main from "+a+" to "+c) {
		t.Fatalf("the panel log names no fast-forward from %s to %s:\n%s", a, c, log.String())
	}
	if ns := catchUpNotices(&notices); len(ns) != 0 {
		t.Fatalf("notices for a fast-forward: %+v", ns)
	}
}

// [CATCHUP-2-S4] A checkout two commits behind at the panel's start, each the owner's
// merged pull request, and no pull request open, is caught up by the start alone.
func TestCatchUpAtThePanelsStart(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	w := newCUWorld(t, cuPanelJSON)
	w.catchUpOn(t, true)
	w.merged(t, 7, "owner", "b.txt", "two\n")
	c := w.merged(t, 8, "owner", "c.txt", "three\n")
	var notices noticeLog
	var log cuLog
	p := w.startCatchUpPanel(t, sock, &notices, &log)
	p.polls.until(t, "catch-up", 1)
	if got := w.head(t); got != c {
		t.Fatalf("HEAD is %s after the start, want origin/main's %s\n%s", got, c, log.String())
	}
}

// [CATCHUP-1-S3] With catch-up off, #7 leaving the open list leaves the checkout's files
// and HEAD as they were, runs no git fetch or git merge, and sends no catch-up notice.
func TestCatchUpOffLeavesTheCheckoutAlone(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	w := newCUWorld(t, cuPanelJSON)
	w.merged(t, 7, "owner", "b.txt", "two\n")
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	p := w.startCatchUpPanel(t, sock, &notices, &log)
	p.polls.until(t, "catch-up", 1)
	p.polls.until(t, "prs", 1)
	before := w.state(t)
	n := p.polls.count("catch-up")
	w.setOpen()
	p.polls.until(t, "catch-up", n+1) // woken by #7 leaving, and done
	w.same(t, before)
	if w.ran("fetch") != 0 || w.ran("merge") != 0 {
		t.Fatalf("catch-up off ran fetch %d, merge %d", w.ran("fetch"), w.ran("merge"))
	}
	if ns := catchUpNotices(&notices); len(ns) != 0 {
		t.Fatalf("catch-up off sent %+v", ns)
	}
}

// [CATCHUP-1-S5] Turning catch-up off on a running panel takes effect at once: #7
// leaving the list then leaves HEAD and the files as they were and runs no git fetch or
// git merge, with no restart of the panel.
func TestCatchUpTurnedOffOnARunningPanel(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	w := newCUWorld(t, cuPanelJSON)
	w.catchUpOn(t, true)
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	p := w.startCatchUpPanel(t, sock, &notices, &log)
	p.polls.until(t, "catch-up", 1)
	p.polls.until(t, "prs", 1)
	w.merged(t, 7, "owner", "b.txt", "two\n")
	w.catchUpOn(t, false) // what `clauductor panel catch-up --project <it> off` writes
	before := w.state(t)
	fetches, merges := w.ran("fetch"), w.ran("merge")
	n := p.polls.count("catch-up")
	w.setOpen()
	p.polls.until(t, "catch-up", n+1)
	w.same(t, before)
	if w.ran("fetch") != fetches || w.ran("merge") != merges {
		t.Fatalf("after off: fetch %d → %d, merge %d → %d", fetches, w.ran("fetch"), merges, w.ran("merge"))
	}
	running(t, p)
}

// [CATCHUP-1-S6] A project with catch-up on whose config `panel add --config` moved
// outside its repository is skipped when a pull request leaves the list: HEAD and files
// as before, no git fetch or git merge, and one notice naming the config outside it.
func TestCatchUpSkipsAConfigMovedOutside(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	w := newCUWorld(t, cuPanelJSON)
	w.catchUpOn(t, true)
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	p := w.startCatchUpPanel(t, sock, &notices, &log)
	p.polls.until(t, "catch-up", 1)
	w.merged(t, 7, "owner", "b.txt", "two\n")
	outside := filepath.Join(w.dir, "sidecar", "panel.json")
	writeFile(t, outside, cuPanelJSON)
	serving := strings.Count(log.String(), ": serving ")
	if _, err := install.AddProject(context.Background(), install.AddOptions{Home: w.home, Project: w.root, Config: outside, Run: signals.ExecRunner}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the project served again on its new config", func() bool { return strings.Count(log.String(), ": serving ") > serving })
	p.polls.more(t, "prs", 1) // the new runtime's open list holds #7
	before := w.state(t)
	fetches, merges := w.ran("fetch"), w.ran("merge")
	n := p.polls.count("catch-up")
	w.setOpen()
	p.polls.until(t, "catch-up", n+1)
	w.same(t, before)
	if w.ran("fetch") != fetches || w.ran("merge") != merges {
		t.Fatalf("a config outside: fetch %d → %d, merge %d → %d", fetches, w.ran("fetch"), merges, w.ran("merge"))
	}
	w.oneNotice(t, &notices, outside, "outside the repository")
}

// [CATCHUP-2-S11] With alerts.notify false, a refused fast-forward (a tracked change)
// still sends one notice naming the checkout's path and the tracked change, as a
// notification and through push.json's stand-in, titled "clauductor panel".
func TestCatchUpRefusalIgnoresAlertsNotify(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	w := newCUWorld(t, `{"name":"T","lanes":{"main":"orchestrator"},"alerts":{"notify":false}}`)
	prog, runs := pushStandIn(t, "")
	writePush(t, w.home, argvJSON(prog))
	w.merged(t, 7, "owner", "b.txt", "two\n")
	writeFile(t, filepath.Join(w.root, "a.txt"), "changed\n")
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	p := w.startCatchUpPanel(t, sock, &notices, &log)
	p.polls.until(t, "catch-up", 1) // the start, catch-up still off
	p.polls.until(t, "prs", 1)
	w.catchUpOn(t, true)
	before := w.state(t)
	n := p.polls.count("catch-up")
	w.setOpen()
	p.polls.until(t, "catch-up", n+1)
	w.same(t, before)
	nt := w.oneNotice(t, &notices, "a.txt")
	got := pushRuns(t, runs)
	if len(got) != 1 || len(got[0]) != 2 || got[0][0] != "clauductor panel" || got[0][1] != nt.Body {
		t.Fatalf("pushes %q, want one titled clauductor panel with the notice's body %q", got, nt.Body)
	}
}
