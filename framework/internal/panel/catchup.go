package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// Catch-up (OPS-118 D3): after a merge, the panel fast-forwards a project's main
// checkout to its base, when the machine turned catch-up on for it (D2, catch-up.json),
// and only over pull requests merged from the panel's own GitHub login. It is a
// machine-level source of the liveSet, as pollRegistry is, never a runtime's own
// goroutine: restartStop waits for those, and a fetch must not hold up a config reload.
// Two things wake it: a pull request leaving a project's open list (the prs source,
// without waiting) and the panel's start (its first poll). Between them it makes no
// call, unless a transient refusal is waiting to be tried again.

const catchUpCallTimeout = 60 * time.Second

// catchUpRetries are when a transient refusal is tried again, counted from the
// refusal that started them: at most three times, and no more until the next trigger.
var catchUpRetries = []time.Duration{time.Minute, 3 * time.Minute, 6 * time.Minute}

// catchUpTitle titles every catch-up notice (D6): the checkout's path is in the body.
const catchUpTitle = "clauductor panel"

// catchUpBodyMax caps a notice's body, in runes. Not signals.Clip: its OneLine cuts at
// DetailMax (140) first, and a deep checkout's path alone can fill that, leaving the
// notice without the reason it exists to give.
const catchUpBodyMax = 600

// catchUpTarget is one served project as catch-up reads it at a trigger.
type catchUpTarget struct {
	root string
	base string // the config's BaseRef: <remote>/<branch>
	// outside is the config's path when it is outside root (an attached project, or
	// one `panel add --config` moved), else "": such a project is skipped (D2).
	outside string
}

// catchUpResult is a fast-forward that ran: HEAD moved from from to to, over prs.
type catchUpResult struct {
	branch   string
	from, to string
	prs      []int
	login    string
}

// catchUpRefusal is why a fast-forward did not run. remote is the remote commit it is
// keyed on (the one fetched, or before the fetch the one the clone last knew).
type catchUpRefusal struct {
	reason    string
	remote    string
	transient bool // a fetch or gh call that failed, or any call that timed out
}

// catchUpRetry is a transient refusal waiting to be tried again.
type catchUpRetry struct {
	from time.Time // the refusal the delays count from
	n    int       // retries already run
}

func (r *catchUpRetry) due() time.Time {
	return r.from.Add(catchUpRetries[min(r.n, len(catchUpRetries)-1)])
}

type catchUp struct {
	home    string
	run     signals.Runner
	clk     clock.Clock
	out     io.Writer
	notify  func(context.Context, state.Notice) error
	push    func() pushCmd
	targets func() []catchUpTarget
	timeout time.Duration
	kick    chan struct{}

	mu      sync.Mutex
	pending map[string]bool // roots woken since the last poll

	// The source's goroutine only.
	started bool
	retries map[string]*catchUpRetry // root → its transient refusal
	sent    map[string]bool          // notice keys sent this run (D3: not notifyOnce's)
}

// newCatchUp makes the catch-up source's state: its notifier (its own keys, created
// with the source), the panel's Runner and log, the machine's push command, and the
// served projects as targets lists them at each trigger.
func newCatchUp(o Options, clk clock.Clock, push func() pushCmd, targets func() []catchUpTarget) *catchUp {
	notify := func(ctx context.Context, n state.Notice) error {
		if o.Notify != nil {
			return o.Notify(n)
		}
		return SendNotice(ctx, n)
	}
	return &catchUp{home: o.Home, run: o.Runner, clk: clk, out: o.Out, notify: notify, push: push, targets: targets,
		timeout: catchUpCallTimeout, kick: make(chan struct{}, 1), pending: map[string]bool{},
		retries: map[string]*catchUpRetry{}, sent: map[string]bool{}}
}

// source is catch-up's place in the machine's run loops: no cadence, no wait for its
// first poll (the start trigger), kicked by wake.
func (c *catchUp) source() *source {
	return &source{name: "catch-up", kick: c.kick, poll: c.poll}
}

// wake asks for a check of root, without blocking: the prs source calls it when a pull
// request leaves the open list. A nil catch-up (a test's liveSet) ignores it.
func (c *catchUp) wake(root string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.pending[root] = true
	c.mu.Unlock()
	kick(c.kick)
}

// poll checks every served project at the first poll, the projects woken since the
// last one, and the ones whose retry is due; it returns the wait until the next retry,
// or 0 (only a kick) when none waits.
func (c *catchUp) poll(ctx context.Context, now time.Time) (update, time.Duration) {
	c.mu.Lock()
	woken := c.pending
	c.pending = map[string]bool{}
	c.mu.Unlock()
	all := !c.started
	c.started = true
	due := map[string]bool{}
	for root := range woken {
		delete(c.retries, root) // a new trigger counts its retries afresh
	}
	for root, rt := range c.retries {
		if !now.Before(rt.due()) {
			due[root] = true
		}
	}
	if all || len(woken) > 0 || len(due) > 0 {
		c.check(ctx, all, woken, due)
	}
	return nil, c.nextRetry()
}

// check reads catch-up.json and the served projects once, then runs the fast-forward
// for each project this poll is for.
func (c *catchUp) check(ctx context.Context, all bool, woken, due map[string]bool) {
	on, err := config.LoadCatchUp(c.home)
	if err != nil {
		// Unreadable is off, for every project; said at each trigger, since the owner
		// turned it on and it does nothing.
		fmt.Fprintf(c.out, "catch-up: %v; catch-up is off for every project until it reads\n", err)
		on = &config.CatchUp{}
	}
	served := map[string]bool{}
	for _, t := range c.targets() {
		served[t.root] = true
		if !all && !woken[t.root] && !due[t.root] {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		if !on.On(t.root) {
			delete(c.retries, t.root)
			continue
		}
		if t.outside != "" {
			delete(c.retries, t.root)
			c.refuse(ctx, t.root, &catchUpRefusal{reason: "its config " + t.outside + " is outside the repository, so catch-up skips it"})
			continue
		}
		if rt := c.retries[t.root]; rt != nil && due[t.root] {
			rt.n++
		}
		res, ref := c.fastForward(ctx, t)
		if ctx.Err() != nil {
			return // the panel is stopping: nothing it read means anything
		}
		if ref == nil {
			delete(c.retries, t.root)
			if res.from != res.to {
				fmt.Fprintf(c.out, "catch-up %s: fast-forwarded %s from %s to %s over %s merged by %s\n",
					t.root, res.branch, res.from, res.to, prList(res.prs), res.login)
			}
			continue
		}
		c.retryAfter(t.root, ref)
		c.refuse(ctx, t.root, ref)
	}
	for root := range c.retries {
		if !served[root] {
			delete(c.retries, root) // removed, or no longer served: nothing to retry
		}
	}
}

// retryAfter schedules root's next try after a transient refusal, and drops it after
// any other refusal (git's own included): those wait for the next trigger.
func (c *catchUp) retryAfter(root string, ref *catchUpRefusal) {
	rt := c.retries[root]
	switch {
	case !ref.transient:
		delete(c.retries, root)
	case rt == nil:
		c.retries[root] = &catchUpRetry{from: c.clk.Now()}
	case rt.n >= len(catchUpRetries):
		delete(c.retries, root)
	}
}

// nextRetry is the wait until the earliest retry, or 0 when none waits. One already
// due (a try that ran past it) waits a second, never 0, which would wait for a kick.
func (c *catchUp) nextRetry() time.Duration {
	var next time.Duration
	now := c.clk.Now()
	for _, rt := range c.retries {
		d := rt.due().Sub(now)
		if d < time.Second {
			d = time.Second
		}
		if next == 0 || d < next {
			next = d
		}
	}
	return next
}

// refuse logs a refusal, and sends its notice once per project, reason and remote
// commit for this panel run: as an OS notification and through push.json, whatever
// the project's alerts.notify says (D6), since nothing else tells the owner that the
// checkout stayed behind.
func (c *catchUp) refuse(ctx context.Context, root string, ref *catchUpRefusal) {
	if ctx.Err() != nil {
		return
	}
	fmt.Fprintf(c.out, "catch-up %s: %s\n", root, ref.reason)
	key := "catch-up\x00" + root + "\x00" + ref.reason + "\x00" + ref.remote
	if c.sent[key] {
		return
	}
	c.sent[key] = true
	body := catchUpBody("Catch-up left " + root + " as it was: " + ref.reason + ".")
	if err := c.notify(ctx, state.Notice{Group: root, Title: catchUpTitle, Body: body, Keys: []string{key}}); err != nil {
		fmt.Fprintf(c.out, "catch-up %s: notification failed: %v\n", root, err)
	}
	if p := c.push(); len(p.argv) > 0 {
		if err := runPush(ctx, pushTimeout, p.argv, signals.Clip(catchUpTitle, 80), body); err != nil {
			fmt.Fprintf(c.out, "catch-up %s: %v\n", root, err)
		}
	}
}

// call runs argv in root under the call timeout; timedOut says the timeout ended it.
// Only the trailing newline goes: a status line starts with a space.
func (c *catchUp) call(ctx context.Context, root string, argv ...string) (out string, timedOut bool, err error) {
	cctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	b, err := c.run(cctx, root, argv)
	if err != nil {
		return "", errors.Is(cctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil, err
	}
	return strings.TrimRight(string(b), "\r\n"), false, nil
}

// fastForward is D3's fast-forward of one checkout: the preconditions, the fetch, the
// pull-request check of every first-parent commit it would move over, and the merge
// of the one commit it checked. The zero result with no refusal: nothing to move.
func (c *catchUp) fastForward(ctx context.Context, t catchUpTarget) (catchUpResult, *catchUpRefusal) {
	remote, branch, ok := strings.Cut(t.base, "/")
	if !ok || remote == "" || branch == "" {
		return catchUpResult{}, &catchUpRefusal{reason: fmt.Sprintf("its base %q is not <remote>/<branch>", t.base)}
	}
	tracking, local := "refs/remotes/"+remote+"/"+branch, "refs/heads/"+branch
	var remoteNow string // what a refusal before the fetch is keyed on
	timedOut := func(what string) *catchUpRefusal {
		return &catchUpRefusal{reason: fmt.Sprintf("%s timed out after %s", what, c.timeout), remote: remoteNow, transient: true}
	}
	policy := func(format string, a ...any) *catchUpRefusal {
		return &catchUpRefusal{reason: fmt.Sprintf(format, a...), remote: remoteNow}
	}
	// read is a precondition read: git -C <root>, taking no optional lock, so a read
	// never refreshes the index a lane or the owner is using.
	read := func(args ...string) (string, *catchUpRefusal, error) {
		out, slow, err := c.call(ctx, t.root, append([]string{"git", "--no-optional-locks", "-C", t.root}, args...)...)
		if slow {
			return "", timedOut("git " + args[0]), err
		}
		return out, nil, err
	}

	v, ref, err := read("rev-parse", "-q", "--verify", tracking+"^{commit}")
	if ref != nil {
		return catchUpResult{}, ref
	}
	if err == nil {
		remoteNow = v
	}
	head, ref, err := read("symbolic-ref", "-q", "HEAD")
	if ref != nil {
		return catchUpResult{}, ref
	}
	// symbolic-ref -q exits 1, saying nothing, when HEAD is detached.
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		return catchUpResult{}, policy("git symbolic-ref HEAD failed: %v", err)
	}
	if err != nil || head == "" {
		return catchUpResult{}, policy("HEAD is detached, not on %s", branch)
	}
	if head != local {
		return catchUpResult{}, policy("HEAD is on %s, not %s", strings.TrimPrefix(head, "refs/heads/"), branch)
	}
	up, ref, err := read("for-each-ref", "--format=%(upstream)", local)
	if ref != nil {
		return catchUpResult{}, ref
	}
	if err != nil {
		return catchUpResult{}, policy("git for-each-ref failed: %v", err)
	}
	if up != tracking {
		if up == "" {
			return catchUpResult{}, policy("%s tracks nothing, not %s", branch, t.base)
		}
		return catchUpResult{}, policy("%s tracks %s, not %s", branch, strings.TrimPrefix(up, "refs/remotes/"), t.base)
	}
	st, ref, err := read("status", "--porcelain", "--untracked-files=no")
	if ref != nil {
		return catchUpResult{}, ref
	}
	if err != nil {
		return catchUpResult{}, policy("git status failed: %v", err)
	}
	if st != "" {
		line, _, _ := strings.Cut(st, "\n")
		return catchUpResult{}, policy("a tracked file differs from HEAD (%s)", strings.TrimSpace(line[min(3, len(line)):]))
	}
	gitDir, ref, err := read("rev-parse", "--absolute-git-dir")
	if ref != nil {
		return catchUpResult{}, ref
	}
	if err != nil {
		return catchUpResult{}, policy("git rev-parse --absolute-git-dir failed: %v", err)
	}
	for _, p := range []struct{ file, what string }{
		{"MERGE_HEAD", "a merge"}, {"rebase-merge", "a rebase"}, {"rebase-apply", "a rebase"},
		{"CHERRY_PICK_HEAD", "a cherry-pick"}, {"REVERT_HEAD", "a revert"},
	} {
		if _, err := os.Lstat(filepath.Join(gitDir, p.file)); err == nil {
			return catchUpResult{}, policy("%s is in progress", p.what)
		}
	}
	ahead, ref, err := read("rev-list", "--count", tracking+".."+local)
	if ref != nil {
		return catchUpResult{}, ref
	}
	if err != nil {
		return catchUpResult{}, policy("git rev-list failed: %v", err)
	}
	if ahead != "0" {
		return catchUpResult{}, policy("%s has %s commit(s) %s lacks", branch, ahead, t.base)
	}
	from, ref, err := read("rev-parse", "--verify", local+"^{commit}")
	if ref != nil {
		return catchUpResult{}, ref
	}
	if err != nil {
		return catchUpResult{}, policy("git rev-parse %s failed: %v", branch, err)
	}

	// The fetch, then the one commit C the remote's branch names, read once: a lane's
	// fetch can move the shared ref while the lookups run, and only C was checked.
	if _, slow, err := c.call(ctx, t.root, "git", "-C", t.root, "fetch", remote, branch); err != nil {
		if slow {
			return catchUpResult{}, timedOut("git fetch")
		}
		return catchUpResult{}, &catchUpRefusal{reason: fmt.Sprintf("git fetch %s %s failed: %v", remote, branch, err), remote: remoteNow, transient: true}
	}
	to, ref, err := read("rev-parse", "--verify", tracking+"^{commit}")
	if ref != nil {
		return catchUpResult{}, ref
	}
	if err != nil {
		return catchUpResult{}, policy("git rev-parse %s failed after the fetch: %v", t.base, err)
	}
	remoteNow = to
	if to == from {
		return catchUpResult{branch: branch, from: from, to: to}, nil
	}
	// The ancestry again, against what was fetched: a branch rewritten on the remote
	// would leave the checkout with commits C lacks.
	n, ref, err := read("rev-list", "--count", to+".."+from)
	if ref != nil {
		return catchUpResult{}, ref
	}
	if err != nil {
		return catchUpResult{}, policy("git rev-list %s..%s failed: %v", short(to), short(from), err)
	}
	if n != "0" {
		return catchUpResult{}, policy("%s has %s commit(s) the fetched %s (%s) lacks", branch, n, t.base, short(to))
	}
	commits, ref, err := read("rev-list", "--first-parent", "--reverse", from+".."+to)
	if ref != nil {
		return catchUpResult{}, ref
	}
	if err != nil {
		return catchUpResult{}, policy("git rev-list %s..%s failed: %v", short(from), short(to), err)
	}

	login, ref := c.login(ctx, t.root, to)
	if ref != nil {
		return catchUpResult{}, ref
	}
	res := catchUpResult{branch: branch, from: from, to: to, login: login}
	for _, sha := range strings.Fields(commits) {
		n, ref := c.checkCommit(ctx, t.root, branch, login, sha, to)
		if ref != nil {
			return catchUpResult{}, ref
		}
		res.prs = append(res.prs, n)
	}

	// C by its hash, never the ref's name: the ref may have moved past what was checked.
	if _, slow, err := c.call(ctx, t.root, "git", "-C", t.root, "merge", "--ff-only", to); err != nil {
		if slow {
			return catchUpResult{}, timedOut("git merge")
		}
		return catchUpResult{}, policy("git merge --ff-only %s refused: %v", short(to), err)
	}
	return res, nil
}

// login is the panel's own GitHub login, `gh api user`'s: the account a merge must be
// from. Any failure of gh is transient.
func (c *catchUp) login(ctx context.Context, root, remote string) (string, *catchUpRefusal) {
	out, slow, err := c.call(ctx, root, "gh", "api", "user")
	var u struct {
		Login string `json:"login"`
	}
	if err == nil {
		if jerr := json.Unmarshal([]byte(out), &u); jerr != nil || u.Login == "" {
			err = fmt.Errorf("no login in its answer")
		}
	}
	if err != nil {
		what := "the lookup of the panel's own login (gh api user)"
		if slow {
			what += " timed out"
		} else {
			what += fmt.Sprintf(" failed: %v", err)
		}
		return "", &catchUpRefusal{reason: what, remote: remote, transient: true}
	}
	return u.Login, nil
}

// checkCommit is the pull-request check of one first-parent commit: it must be the
// merge commit of a pull request merged into branch from login, read through GitHub's
// commit-to-pull-request route (never its search, which can lag a fresh merge). It
// returns the pull request's number. A failed lookup is transient; a commit that fails
// the check is a refusal of policy, and names the commit.
func (c *catchUp) checkCommit(ctx context.Context, root, branch, login, sha, remote string) (int, *catchUpRefusal) {
	lookupFailed := func(what string, slow bool, err error) *catchUpRefusal {
		if slow {
			what += " timed out"
		} else {
			what += fmt.Sprintf(" failed: %v", err)
		}
		return &catchUpRefusal{reason: what, remote: remote, transient: true}
	}
	out, slow, err := c.call(ctx, root, "gh", "api", "repos/{owner}/{repo}/commits/"+sha+"/pulls")
	var list []struct {
		Number         int    `json:"number"`
		MergeCommitSHA string `json:"merge_commit_sha"`
	}
	if err == nil {
		err = json.Unmarshal([]byte(out), &list)
	}
	if err != nil {
		return 0, lookupFailed("the pull-request lookup for commit "+short(sha), slow, err)
	}
	var refused *catchUpRefusal
	for _, p := range list {
		if p.MergeCommitSHA != sha {
			continue // a pull request the commit is only in, not the one that merged it
		}
		out, slow, err := c.call(ctx, root, "gh", "api", "repos/{owner}/{repo}/pulls/"+strconv.Itoa(p.Number))
		var pr struct {
			Merged         bool   `json:"merged"`
			MergeCommitSHA string `json:"merge_commit_sha"`
			MergedBy       *struct {
				Login string `json:"login"`
			} `json:"merged_by"`
			Base struct {
				Ref string `json:"ref"`
			} `json:"base"`
		}
		if err == nil {
			err = json.Unmarshal([]byte(out), &pr)
		}
		if err != nil {
			return 0, lookupFailed(fmt.Sprintf("the lookup of pull request #%d for commit %s", p.Number, short(sha)), slow, err)
		}
		by := ""
		if pr.MergedBy != nil {
			by = pr.MergedBy.Login
		}
		switch {
		case !pr.Merged || pr.MergeCommitSHA != sha:
			refused = &catchUpRefusal{reason: fmt.Sprintf("commit %s is not the merge commit of pull request #%d", short(sha), p.Number), remote: remote}
		case pr.Base.Ref != branch:
			refused = &catchUpRefusal{reason: fmt.Sprintf("commit %s is pull request #%d, merged into %s, not %s", short(sha), p.Number, pr.Base.Ref, branch), remote: remote}
		case by != login:
			refused = &catchUpRefusal{reason: fmt.Sprintf("commit %s is pull request #%d, merged by %s, not %s (the panel's gh login)", short(sha), p.Number, orNone(by), login), remote: remote}
		default:
			return p.Number, nil
		}
	}
	if refused == nil {
		refused = &catchUpRefusal{reason: fmt.Sprintf("commit %s came with no pull request", short(sha)), remote: remote}
	}
	return 0, refused
}

// catchUpBody is s on one line (git's errors span several), cut at catchUpBodyMax.
func catchUpBody(s string) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > catchUpBodyMax {
		return string(r[:catchUpBodyMax-1]) + "…"
	}
	return string(r)
}

// short is a commit's first 12 hex digits: what a notice names it by.
func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

func orNone(s string) string {
	if s == "" {
		return "nobody"
	}
	return s
}

// prList is "#7, #9": the pull requests a fast-forward moved over.
func prList(prs []int) string {
	s := make([]string, len(prs))
	for i, n := range prs {
		s[i] = "#" + strconv.Itoa(n)
	}
	return strings.Join(s, ", ")
}
