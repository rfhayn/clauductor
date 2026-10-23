package panel

import (
	"bytes"
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
	"unicode/utf8"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
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

// catchUpBodyMax caps a notice's body, in runes, in the notification and the push
// alike (sendCatchUp, catchUpNotifyArgv). Not signals.Clip: its OneLine cuts at
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
	// cfgPath is the config the runtime serves, inside root when outside is "": the
	// file whose arrival D1 judges.
	cfgPath string
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
	// arrive judges a changed config after a fast-forward (D1): the liveSet's
	// arrive. nil when the source runs alone (a test's), which judges nothing.
	arrive func(context.Context, *catchUp, arrival)
	// selfUpdate is D4's rebuild after a fast-forward (Options.selfUpdate).
	selfUpdate func(context.Context, catchUpTarget, catchUpResult, string) *rebuilt
	// atStart runs once, at the source's first poll and before its check: the first
	// start after the rebuild's restart says which commit it runs (D4).
	atStart func(context.Context)

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
	notify := notifyCatchUp // not SendNotice: its one-line summary cuts a notice at 140 runes
	if o.Notify != nil {
		notify = func(_ context.Context, n state.Notice) error { return o.Notify(n) }
	}
	return &catchUp{home: o.Home, run: o.Runner, clk: clk, out: o.Out, notify: notify, push: push, targets: targets,
		selfUpdate: o.selfUpdate, timeout: catchUpCallTimeout, kick: make(chan struct{}, 1), pending: map[string]bool{},
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
	if all && c.atStart != nil {
		c.atStart(ctx)
	}
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
		if ref == nil && res.from != res.to {
			// The merge is in the checkout whether or not the panel is stopping, and the
			// next start finds nothing to arrive: what follows it runs now.
			fmt.Fprintf(c.out, "catch-up %s: fast-forwarded %s from %s to %s over %s merged by %s\n",
				t.root, res.branch, res.from, res.to, prList(res.prs), res.login)
			c.arrived(ctx, t, res)
		}
		if ctx.Err() != nil {
			return // the panel is stopping: nothing else it read means anything
		}
		if ref == nil {
			delete(c.retries, t.root)
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
	c.send(ctx, catchUpNotice{root: root, key: "catch-up\x00" + root + "\x00" + ref.reason + "\x00" + ref.remote,
		body: catchUpBody("Catch-up left " + root + " as it was: " + ref.reason + ".")}, true)
}

// send sends a catch-up notice once per key for this panel run (sendCatchUp), notifyOn
// being the loaded config's alerts.notify, which only a notice that waits reads. It
// sends under a context the run's end does not cancel: every notice says what a step
// already taken did (a fast-forward, a trust, the swap's renames and their rollback), and
// a stop that ends the run must not drop the only record of it on the way out. It sets
// no deadline of its own: the notification and then the push each keep theirs
// (showNotice's 10 s, pushTimeout), so a slow notification cannot take the push's time,
// and the send, which Run's shutdown waits for, ends within their sum.
func (c *catchUp) send(ctx context.Context, n catchUpNotice, notifyOn bool) {
	if c.sent[n.key] {
		return
	}
	if sendCatchUp(context.WithoutCancel(ctx), c.notify, c.push(), c.out, n, notifyOn) {
		c.sent[n.key] = true
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
	// A stop starts no merge, and does not kill one under way, under its own bound: a
	// merge killed after it moved the branch would leave a fast-forward nothing follows,
	// since the next start finds nothing to arrive.
	if ctx.Err() != nil {
		return catchUpResult{}, policy("the panel is stopping")
	}
	if _, slow, err := c.call(context.WithoutCancel(ctx), t.root, "git", "-C", t.root, "merge", "--ff-only", to); err != nil {
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
func catchUpBody(s string) string { return oneLine(s, catchUpBodyMax) }

// oneLine is s on one line, cut at n runes.
func oneLine(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > n {
		return string(r[:n-1]) + "…"
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

// Trust on arrival (OPS-118 D1): a fast-forward that changed the project's own
// panel.json is judged on catch-up's source, after the fast-forward and before the
// rebuild's swap (D4), never on a runtime's goroutine.

// catchUpLineMax caps one run-list line of the trust notice, in runes: a template's
// first prompt can fill the whole body.
const catchUpLineMax = 160

// configCheckTimeout bounds a binary's `panel config-check`.
const configCheckTimeout = 30 * time.Second

// rebuilt is what the panel's rebuild of itself after a fast-forward (D4,
// Options.selfUpdate, task 3.1) tells catch-up: why the build or its preflight
// failed, or the new binary's reading of the changed panel.json and the swap to it.
// Catch-up sends the failure's notice, judges the config with check, and only then
// runs swap, so the bytes are recorded as trusted before the swap (D1).
type rebuilt struct {
	err   error         // the build or its preflight failed, or a stop ended it: nothing is swapped
	check *config.Check // the new binary's `panel config-check` of the changed config
	swap  func(context.Context)
	// discard removes the new binary when catch-up does not run swap after all.
	discard func()
	// untrust withdraws the trust arrive recorded on check: whether the record was taken
	// back (one changed since stands), or why it could not be; nil when arrive recorded
	// none. withdraw runs it, once.
	untrust func() (bool, error)
	// unswapped is what becomes of the changed config when swap fails, for its notice
	// (notInPlace): set by arrive, or by arrived for a config it could not read; nil when
	// the config did not change.
	unswapped func(context.Context) string
}

// withdraw withdraws the trust recorded on arrival for the new binary's reading of the
// config (untrust), once, when that binary is not put in place after all: the binary
// that next serves the config never judged it (D1). withdrew is false, and err nil, when
// nothing was recorded or the record changed since; withdrawnSaid words the outcome.
func (u *rebuilt) withdraw() (withdrew bool, err error) {
	if u == nil || u.untrust == nil {
		return false, nil
	}
	f := u.untrust
	u.untrust = nil
	return f()
}

// notInPlace is the swap's account of the changed config when the rebuilt binary could
// not be put in place: the running binary has taken it over as after a failed build
// (unswapped, D1). "" when the config did not change.
func (u *rebuilt) notInPlace(ctx context.Context) string {
	if u == nil || u.unswapped == nil {
		return ""
	}
	return u.unswapped(ctx)
}

// waitsForOwner is how a config that is not trusted waits for the owner.
const waitsForOwner = "waits for the owner (Trust config… on the page, or `clauductor panel trust`)"

// withdrawnSaid is a notice's account of a withdrawal (rebuilt.withdraw) of the trust
// recorded on arrival for rel: "is withdrawn" and then what follows, or why it could not
// be and what that leaves; "" when nothing was withdrawn.
func withdrawnSaid(rel string, withdrew bool, err error, then string) string {
	switch {
	case err != nil:
		return fmt.Sprintf(" Withdrawing the trust it recorded on arrival for the new %s failed (%v): the next start serves it trusted, as the build that is not in place judged it.", rel, err)
	case withdrew:
		return fmt.Sprintf(" The trust it recorded on arrival for the new %s, judged by that build, is withdrawn%s.", rel, then)
	}
	return ""
}

// arrival is a fast-forward that changed the project's panel.json: the file's blob at
// the commit it started from (at) and at the one it reached (now), each with whether
// the file was there, and the rebuild that follows it, if any.
type arrival struct {
	t           catchUpTarget
	res         catchUpResult
	rel         string // the config's path in the repository
	at, now     []byte
	atOK, nowOK bool
	u           *rebuilt
}

// arrived follows a fast-forward from res.from to res.to: the rebuild, when one
// follows (c.selfUpdate), told whether the config changed; the rebuild's failure
// notice first; then, when the fast-forward changed the project's own panel.json, D1's
// judgement of it (c.arrive), or, when its blobs cannot be read, the notice that it
// was not judged (c.unread); then the rebuild's swap. An unread config is never read
// as unchanged: the rebuild is told it may have changed, so its preflight's
// config-check judges the file the new binary will load.
//
// A stop does not cut it short: the fast-forward is in the checkout, and the next start
// finds nothing to arrive, so the reads and the judgement run, and every notice goes
// (send). Only what waits on the run ends with it: the build (a stopped rebuild says
// so), the reload, and the swap, whose new binary is then discarded and the trust it
// earned withdrawn.
func (c *catchUp) arrived(ctx context.Context, t catchUpTarget, res catchUpResult) {
	a := arrival{t: t, res: res}
	changed := false
	var unread error // the config's blobs could not be read: whether it changed is not known
	if rel, ok := repoRel(t.root, t.cfgPath); ok {
		a.rel = rel
		// Under their own bounds (call's timeout): a stop must not turn a read into
		// "not known", which the rebuild would then have to judge.
		if changed, unread = c.blobs(context.WithoutCancel(ctx), &a); unread != nil {
			changed = false
		}
	}
	mayHaveChanged := changed || unread != nil
	var u *rebuilt
	if c.selfUpdate != nil {
		p := ""
		if mayHaveChanged {
			p = t.cfgPath
		}
		u = c.selfUpdate(ctx, t, res, p)
	}
	if u != nil && u.err == nil && mayHaveChanged && u.check == nil {
		u.err = fmt.Errorf("its preflight did not read %s", a.rel)
	}
	if u != nil && u.err != nil {
		body := fmt.Sprintf("The panel did not rebuild itself after catch-up moved %s to %s (%s), and keeps running as it was: %v.",
			t.root, short(res.to), prList(res.prs), u.err)
		if errors.Is(u.err, errRebuildStopped) {
			body = stoppedRebuildBody(fmt.Sprintf("The panel was stopped during its rebuild after catch-up moved %s to %s (%s).",
				t.root, short(res.to), prList(res.prs)))
		}
		c.send(ctx, catchUpNotice{root: t.root, key: rebuildKey(t.root, res.to, ""), body: catchUpBody(body)}, true)
	}
	swaps := u != nil && u.err == nil && u.swap != nil
	if unread == nil && changed && c.arrive != nil {
		a.u = u
		c.arrive(ctx, c, a)
	}
	// Read once, after the judgement: the unread notice's account of who serves the file,
	// the discard, the swap and the stop's notice all follow one answer.
	swapNow := swaps && ctx.Err() == nil
	if unread != nil {
		c.unread(ctx, a, unread, swapNow)
		if swapNow {
			// The unread notice said the rebuilt binary serves the file: a swap that fails
			// leaves the running one serving the config it loaded, as a failed build does.
			u.unswapped = func(context.Context) string {
				return fmt.Sprintf(" As its %s could not be read, %s.", a.rel, unreadKeeps)
			}
		}
	}
	if u != nil && u.discard != nil && !swapNow {
		u.discard() // no swap follows: no new binary is left beside the installed one
	}
	switch {
	case swapNow:
		u.swap(ctx)
	case swaps:
		// Stopped between the judgement and the swap: the binary that judged the config
		// is not put in place, so its trust goes with it.
		fmt.Fprintf(c.out, "catch-up %s: stopped before the build of %s was put in place; the next start runs the binary it ran\n", t.root, short(res.to))
		withdrew, err := u.withdraw()
		c.send(ctx, catchUpNotice{root: t.root, key: rebuildKey(t.root, res.to, ""), body: catchUpBody(stoppedRebuildBody(fmt.Sprintf(
			"The panel built %s after catch-up moved %s there (%s), and was stopped before it put the build in place.%s",
			short(res.to), t.root, prList(res.prs), withdrawnSaid(a.rel, withdrew, err, ": the config "+waitsForOwner))))}, true)
	}
}

// unreadKeeps is what becomes of a config catch-up could not read when the running
// binary goes on serving.
const unreadKeeps = "the panel keeps serving the config it had loaded. Read the file; `clauductor panel trust` trusts it, and the panel's next start serves it"

// unread is the log line and the one notice of an arrival whose config's blobs could
// not be read (blobs): whether the fast-forward changed the config is not known, so
// nothing is trusted on arrival and the runtime is not started again on a guess. The
// notice goes whatever alerts.notify says (D6): the page may still show the loaded
// config trusted, so nothing else would say that the checkout holds a config the
// panel has not judged. swaps says a rebuilt binary is about to serve the file.
func (c *catchUp) unread(ctx context.Context, a arrival, err error, swaps bool) {
	root := a.t.root
	what := unreadKeeps
	if swaps {
		what = "the rebuilt panel serves the file as it is on disk, untrusted if it changed. Read it, then Trust config… on the page, or `clauductor panel trust`"
	}
	fmt.Fprintf(c.out, "catch-up %s: cannot read %s at %s and %s, so nothing is trusted on arrival and %s: %v\n",
		root, a.rel, short(a.res.from), short(a.res.to), what, err)
	c.send(ctx, catchUpNotice{root: root, key: "catch-up\x00" + root + "\x00arrival\x00" + a.res.to, body: catchUpBody(fmt.Sprintf(
		"Catch-up moved %s to %s (%s), but could not read its %s there, so it trusted nothing on arrival, and %s. The read failed: %v.",
		root, short(a.res.to), prList(a.res.prs), a.rel, what, err))}, true)
}

// repoRel is cfgPath's path inside root, with forward slashes, as git names it; false
// when it is not inside root (catch-up skips such a project before it gets here).
func repoRel(root, cfgPath string) (string, bool) {
	if cfgPath == "" {
		return "", false
	}
	rel, err := filepath.Rel(signals.ResolvePath(root), signals.ResolvePath(cfgPath))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// blobs reads a.rel's blob at a.res.from and at a.res.to into a, and says whether the
// fast-forward changed it: a different blob, or the file added or removed.
func (c *catchUp) blobs(ctx context.Context, a *arrival) (bool, error) {
	root := a.t.root
	from, err := c.blobID(ctx, root, a.res.from, a.rel)
	if err != nil {
		return false, err
	}
	to, err := c.blobID(ctx, root, a.res.to, a.rel)
	if err != nil {
		return false, err
	}
	if from == to {
		return false, nil
	}
	if from != "" {
		if a.at, err = c.blob(ctx, root, from); err != nil {
			return false, err
		}
		a.atOK = true
	}
	if to != "" {
		if a.now, err = c.blob(ctx, root, to); err != nil {
			return false, err
		}
		a.nowOK = true
	}
	return true, nil
}

// blobID is the object rel names at rev (`git ls-tree`), or "" when rev has no rel.
func (c *catchUp) blobID(ctx context.Context, root, rev, rel string) (string, error) {
	out, slow, err := c.call(ctx, root, "git", "--no-optional-locks", "-C", root, "ls-tree", rev, "--", rel)
	if err != nil {
		if slow {
			return "", fmt.Errorf("git ls-tree %s timed out after %s", short(rev), c.timeout)
		}
		return "", fmt.Errorf("git ls-tree %s: %v", short(rev), err)
	}
	if out == "" {
		return "", nil
	}
	meta, _, _ := strings.Cut(out, "\t")
	f := strings.Fields(meta)
	if len(f) != 3 || f[1] != "blob" {
		return "", fmt.Errorf("%s at %s is not a file", rel, short(rev))
	}
	return f[2], nil
}

// blob is an object's bytes exactly: through the Runner, not call, which trims the
// trailing newline a config's bytes, and so its hash, end with.
func (c *catchUp) blob(ctx context.Context, root, id string) ([]byte, error) {
	cctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	b, err := c.run(cctx, root, []string{"git", "--no-optional-locks", "-C", root, "cat-file", "blob", id})
	if err != nil {
		return nil, fmt.Errorf("git cat-file %s: %v", short(id), err)
	}
	return b, nil
}

// arrive is D1's judgement of a config a fast-forward brought (catch-up's arrive). It
// records the new bytes as trusted, with the note "trusted on arrival from #N", only
// when every check holds; sends the one notice of the outcome; then serves the
// project again on the new config, trusted or not, unless it does not load (the
// runtime keeps what it loaded) or a rebuild's new binary is to serve it. A reload
// that fails after the notice sends a second, since the first said it would serve. The checks
// and the trust run under ls.mu, so a page's trust or a registry change cannot come
// between them; the notices go without it, since a push may take pushTimeout and a
// page request must not wait for one; the reload takes it again, and serves nothing
// when the runtime was replaced meanwhile. A trust recorded on a rebuild's reading of
// the config goes with that binary: a.u.untrust takes it back if it is not put in place,
// and when its swap fails, a.u.unswapped serves the config here as after a failed build.
// A stop does not change the judgement (its load runs to its answer, as arrived's reads
// do), and ends only the reload, which then says nothing a stop made it fail.
func (ls *liveSet) arrive(ctx context.Context, c *catchUp, a arrival) {
	root, to := a.t.root, a.res.to
	ls.mu.Lock()
	var lr *liveRT
	for _, x := range ls.rts {
		if x.entry.Root == root {
			lr = x
			break
		}
	}
	if lr == nil || lr.r.cfgPath != a.t.cfgPath {
		ls.mu.Unlock()
		fmt.Fprintf(c.out, "catch-up %s: %s changed, but the project is no longer served on it, so nothing is trusted\n", root, a.rel)
		return
	}
	r := lr.r
	loadedNotify := r.cfg.AlertThresholds().Notify
	var why []string
	switch {
	case !r.trusted():
		why = append(why, "the config was not trusted before")
	case !a.atOK || !bytes.Equal(r.raw, a.at):
		why = append(why, "the loaded config was not the file the fast-forward started from ("+short(a.res.from)+")")
	}
	if disk, err := os.ReadFile(a.t.cfgPath); err != nil || !a.nowOK || !bytes.Equal(disk, a.now) {
		why = append(why, "the file on disk is not the one "+short(to)+" holds")
	}
	rebuiltOK := a.u != nil && a.u.err == nil
	switch {
	case a.u != nil && errors.Is(a.u.err, errRebuildStopped):
		why = append(why, "the panel was stopped before its rebuild was done")
	case a.u != nil && a.u.err != nil:
		why = append(why, "the panel's rebuild failed")
	}
	// (e) and (f): the new config as the binary that will serve it reads it.
	var chk config.Check
	var loadErr error
	switch {
	case !a.nowOK:
		loadErr = fmt.Errorf("%s removed it", short(to))
	case rebuiltOK:
		chk = *a.u.check
	default:
		// Under the load's own bound (ReadWorktrees'), not the run's: a stop must not make
		// a config that loads read as one that does not, and so lose its trust on arrival;
		// the next start finds nothing to arrive.
		if p, err := ls.tryLoad(context.WithoutCancel(ctx), lr.entry); err != nil {
			loadErr = err
		} else {
			chk = config.CheckOf(p.cfg)
		}
	}
	if loadErr == nil && !chk.Notify {
		why = append(why, "the new config turns notices off (alerts.notify is false)")
	}
	added, removed := runsDiff(install.TrustRuns(root, r.cfgPath, r.cfg), chk.Runs)
	trusted := false
	if loadErr == nil && len(why) == 0 {
		note := "trusted on arrival from " + prList(a.res.prs)
		hash := install.ConfigHash(a.now)
		if tv, err := install.TrustArrived(ls.o.Home, root, a.t.cfgPath, hash, note); err != nil {
			why = append(why, "recording its trust failed: "+err.Error())
		} else {
			trusted = true
			if rebuiltOK && tv.Prev != hash {
				a.u.untrust = ls.untrustArrived(c, a, hash, note, tv.Prev)
			}
		}
	}
	id := lr.entry.ID
	ls.mu.Unlock()

	from := prList(a.res.prs) + ", merged by " + a.res.login
	key := "catch-up\x00" + root + "\x00arrival\x00" + to
	switch {
	case loadErr != nil:
		fmt.Fprintf(c.out, "catch-up %s: the new %s (%s) does not load, so the panel keeps serving the config it had loaded: %v\n", root, a.rel, from, loadErr)
		c.send(ctx, catchUpNotice{root: root, key: key, body: catchUpBody(fmt.Sprintf(
			"Catch-up moved %s to %s (%s), and its new %s does not load, so the panel keeps serving the config it had loaded: %v.",
			root, short(to), from, a.rel, loadErr))}, true)
		return
	case trusted:
		fmt.Fprintf(c.out, "catch-up %s: %s (sha256 %s) trusted on arrival from %s\n", root, a.rel, config.ShortHash(install.ConfigHash(a.now)), from)
		c.send(ctx, catchUpNotice{root: root, key: key, trusted: chk.Name, body: trustBody(
			fmt.Sprintf("Catch-up trusted the new %s in %s on arrival from %s.", a.rel, root, from), added, removed)}, true)
	default:
		fmt.Fprintf(c.out, "catch-up %s: the new %s (%s) is not trusted on arrival: %s; it waits for the owner\n", root, a.rel, from, strings.Join(why, "; "))
		c.send(ctx, catchUpNotice{root: root, key: key, waits: true, body: catchUpBody(fmt.Sprintf(
			"Catch-up moved %s to %s (%s), and did not trust its new %s: %s. The config waits for the owner: Trust config… on the page, or `clauductor panel trust`.",
			root, short(to), from, a.rel, strings.Join(why, "; ")))}, loadedNotify)
	}
	if rebuiltOK {
		// The new binary serves it, trusted or not, from its first start. A swap that
		// fails hands it to this binary instead, as a failed build does (D1).
		a.u.unswapped = func(ctx context.Context) string { return ls.unswapped(ctx, c, a, id) }
		return
	}
	done, notLoaded, err := ls.serveArrived(ctx, c, a, r, id)
	if !done {
		return
	}
	// The notice just sent said the new config would be served: one that fails now, as
	// (e) failing late, is said too, whatever alerts.notify says (D1, D6).
	var what string
	switch {
	case notLoaded != nil:
		what = fmt.Sprintf("no longer loads, so the panel keeps serving the config it had loaded: %v", notLoaded)
	case err != nil:
		what = fmt.Sprintf("did not start, so the project is not served (the page shows why): %v", err)
	default:
		return
	}
	fmt.Fprintf(c.out, "catch-up %s: the new %s %s\n", root, a.rel, what)
	c.send(ctx, catchUpNotice{root: root, key: key + "\x00reload", body: catchUpBody(fmt.Sprintf(
		"Catch-up moved %s to %s (%s), and its new %s %s.", root, short(to), from, a.rel, what))}, true)
}

// serveArrived serves the project id again on its config as it is now on disk (reload):
// arrive's reload after its notice, while r still serves the project, and a failed
// swap's (r nil), whichever runtime serves it then, since that one may have started on
// the trust just withdrawn. done is false when nothing was started, each case logged: a
// stop ends the reload, as it ends every runtime (the next start serves the file on disk
// as its trust record says, and a reload the stop cut short failed for the stop, not for
// the config, and says nothing more); the project is no longer served; or (r set) it was
// started again meanwhile, on its config as it was then. Otherwise notLoaded and err are
// reload's.
func (ls *liveSet) serveArrived(ctx context.Context, c *catchUp, a arrival, r *Runtime, id string) (done bool, notLoaded, err error) {
	root := a.t.root
	stopped := func() (bool, error, error) {
		fmt.Fprintf(c.out, "catch-up %s: the panel stopped before it served the new %s; its next start loads it as it is on disk\n", root, a.rel)
		return false, nil, nil
	}
	ls.mu.Lock()
	if ctx.Err() != nil {
		ls.mu.Unlock()
		return stopped()
	}
	cur := ls.rts[id]
	if cur == nil || (r != nil && cur.r != r) {
		ls.mu.Unlock()
		fmt.Fprintf(c.out, "catch-up %s: the project was started again or removed meanwhile, on its config as it was then\n", root)
		return false, nil, nil
	}
	notLoaded, err = ls.reload(cur)
	ls.mu.Unlock()
	if (notLoaded != nil || err != nil) && ctx.Err() != nil {
		return stopped()
	}
	return true, notLoaded, err
}

// unswapped is what becomes of a config arrive judged for a rebuilt binary that its swap
// then could not put in place: this binary takes it over as after a failed build (D1).
// The trust recorded for the new binary's reading is withdrawn, then the project is
// served again on the file as it is on disk: untrusted, with Trust config… offered, or on
// the config it had loaded when this binary cannot load the file. A withdrawal that
// failed leaves the runtime as it is, since a reload would serve the file trusted on a
// judgement this binary never made. It returns the swap notice's account. A config that
// was never trusted and is now served untrusted adds nothing: arrive's notice said it
// waits for the owner, under the loaded config's alerts.notify as after a failed build.
func (ls *liveSet) unswapped(ctx context.Context, c *catchUp, a arrival, id string) string {
	withdrew, err := a.u.withdraw()
	if err != nil {
		return withdrawnSaid(a.rel, false, err, "")
	}
	done, notLoaded, startErr := ls.serveArrived(ctx, c, a, nil, id)
	it := "the new " + a.rel
	if withdrew {
		it = "it"
	}
	var what string
	switch {
	case !done:
		if withdrew {
			what = "the config " + waitsForOwner
		}
	case notLoaded != nil:
		fmt.Fprintf(c.out, "catch-up %s: the running panel cannot load the new %s, so it keeps serving the config it had loaded: %v\n", a.t.root, a.rel, notLoaded)
		what = fmt.Sprintf("the running panel cannot load %s, so it keeps serving the config it had loaded: %v", it, notLoaded)
	case startErr != nil:
		fmt.Fprintf(c.out, "catch-up %s: the new %s did not start, so the project is not served: %v\n", a.t.root, a.rel, startErr)
		what = fmt.Sprintf("the panel could not start the project on %s, so it is not served (the page shows why): %v", it, startErr)
	case withdrew:
		what = "the panel serves it untrusted, and it " + waitsForOwner
	}
	switch {
	case withdrew:
		return withdrawnSaid(a.rel, true, nil, ": "+what)
	case what == "":
		return ""
	}
	return " T" + what[1:] + "."
}

// untrustArrived is a rebuild's untrust for the trust arrive just recorded, for the new
// binary's reading of the config (hash, note, over prev): it takes the record back, under
// ls.mu as the trust was made, and says whether it did. A record changed since stands
// (install.UntrustArrived).
func (ls *liveSet) untrustArrived(c *catchUp, a arrival, hash, note, prev string) func() (bool, error) {
	return func() (bool, error) {
		ls.mu.Lock()
		withdrew, err := install.UntrustArrived(ls.o.Home, a.t.root, a.t.cfgPath, hash, note, prev)
		ls.mu.Unlock()
		switch {
		case err != nil:
			fmt.Fprintf(c.out, "catch-up %s: withdrawing the trust recorded on arrival for %s failed: %v\n", a.t.root, a.rel, err)
		case withdrew:
			fmt.Fprintf(c.out, "catch-up %s: the trust recorded on arrival for %s is withdrawn: the binary that judged it does not serve it\n", a.t.root, a.rel)
		}
		return withdrew, err
	}
}

// runsDiff is the run-list lines of now that was lacks (added) and of was that now
// lacks (removed), each in its list's order and counted, so a line now held twice
// where it was once is one added.
func runsDiff(was, now []string) (added, removed []string) {
	count := func(l []string) map[string]int {
		m := map[string]int{}
		for _, s := range l {
			m[s]++
		}
		return m
	}
	inWas, inNow := count(was), count(now)
	for _, s := range now {
		if inWas[s] > 0 {
			inWas[s]--
		} else {
			added = append(added, s)
		}
	}
	for _, s := range was {
		if inNow[s] > 0 {
			inNow[s]--
		} else {
			removed = append(removed, s)
		}
	}
	return added, removed
}

// trustBody is the trust notice's text (D1): head, then one line per run-list line
// added (+) and removed (−), as many as fit in catchUpBodyMax, the last counting the
// rest, which the page's trust report lists.
func trustBody(head string, added, removed []string) string {
	out := oneLine(head, catchUpBodyMax/2)
	if len(added)+len(removed) == 0 {
		return out + " Its run list is as it was."
	}
	lines := make([]string, 0, len(added)+len(removed))
	for _, s := range added {
		lines = append(lines, "+ "+oneLine(s, catchUpLineMax))
	}
	for _, s := range removed {
		lines = append(lines, "− "+oneLine(s, catchUpLineMax))
	}
	const tail = 64 // room kept for the line counting the rest
	n := utf8.RuneCountInString(out)
	for i, l := range lines {
		room := catchUpBodyMax - n - 1
		if i < len(lines)-1 {
			room -= tail
		}
		if utf8.RuneCountInString(l) > room {
			return out + fmt.Sprintf("\nand %d more (the page's trust report lists them all)", len(lines)-i)
		}
		out += "\n" + l
		n += 1 + utf8.RuneCountInString(l)
	}
	return out
}

// configCheck is bin's reading of the config at path, `bin panel config-check --config
// path`, for D1 (e) and (f) when a rebuild follows: the binary that will serve the
// config judges it, since the running one may not read it. The rebuild's preflight
// (task 3.1) runs it on the new binary. An exit that is not 0 is the load's error;
// an answer missing any of name, notify, runs and tmux_socket judged nothing, and is an
// error too.
func configCheck(ctx context.Context, bin, path string) (config.Check, error) {
	cctx, cancel := context.WithTimeout(ctx, configCheckTimeout)
	defer cancel()
	out, err := signals.ExecRunner(cctx, filepath.Dir(path), []string{bin, "panel", "config-check", "--config", path})
	if err != nil {
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			return config.Check{}, fmt.Errorf("panel config-check gave no answer within %s", configCheckTimeout)
		}
		return config.Check{}, fmt.Errorf("panel config-check: %v", err)
	}
	var v struct {
		Name   *string   `json:"name"`
		Notify *bool     `json:"notify"`
		Runs   *[]string `json:"runs"`
		Socket *string   `json:"tmux_socket"`
	}
	if err := json.Unmarshal(out, &v); err != nil || v.Name == nil || v.Notify == nil || v.Runs == nil || v.Socket == nil {
		return config.Check{}, errors.New("panel config-check printed no name, notify, runs and tmux_socket")
	}
	return config.Check{Name: *v.Name, Notify: *v.Notify, Runs: *v.Runs, Socket: *v.Socket}, nil
}
