package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/testwait"
)

// PANEL-51 group 3, end to end: a panel with no page in view, its repositories real
// git, and `claude agents` and `gh` as stallWorld says. The stall time is a couple of
// seconds (alerts.stall_minutes takes a fraction), so a lane stalls within a test.

const stallMinutes = "0.03" // 1.8 s

// stallWorld is the machine the test's panels read: git for real, `claude agents`
// and `gh` from its fields, which a test changes as it goes.
type stallWorld struct {
	mu     sync.Mutex
	agents []signals.Agent
	open   map[string]int    // branch → the number of its open pull request
	merged map[string]string // branch → its merged read: "none", "fail" or "<number>@<head commit>"
	calls  map[string]int    // branch → merged reads asked for
	hold   chan struct{}     // while open (non-nil), every gh call waits for it
	// mergedHold: while open (non-nil), every merged read waits for it.
	mergedHold chan struct{}
	// gitDirs counts the reads of a worktree's git dir (the receipt read's one spawn),
	// and threads the review-thread reads (`gh api graphql`).
	gitDirs, threads int
}

func newStallWorld() *stallWorld {
	return &stallWorld{open: map[string]int{}, merged: map[string]string{}, calls: map[string]int{}}
}

func (w *stallWorld) set(f func(w *stallWorld)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	f(w)
}

func (w *stallWorld) mergedCalls(branch string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls[branch]
}

func (w *stallWorld) gitDirReads() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.gitDirs
}

func (w *stallWorld) run(ctx context.Context, dir string, argv []string) ([]byte, error) {
	cmd := envCommand(argv)
	if len(cmd) == 0 {
		return nil, refuseUnreadEnv(argv)
	}
	j := strings.Join(cmd, " ")
	switch {
	case cmd[0] == "git":
		if strings.Contains(j, " -C ") && strings.HasSuffix(j, " rev-parse --absolute-git-dir") {
			w.set(func(w *stallWorld) { w.gitDirs++ })
		}
		return testRunner(ctx, dir, argv)
	case j == "claude auth status --json":
		return []byte(`{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","email":"dev@example.com","orgId":"x","subscriptionType":"max"}`), nil
	case strings.HasPrefix(j, "claude agents --json"):
		w.mu.Lock()
		defer w.mu.Unlock()
		var list []signals.Agent
		for _, a := range w.agents {
			if len(cmd) == 5 && cmd[3] == "--cwd" && !strings.HasPrefix(a.Cwd, cmd[4]) {
				continue
			}
			list = append(list, a)
		}
		b, _ := json.Marshal(list)
		if list == nil {
			b = []byte("[]")
		}
		return b, nil
	case cmd[0] == "claude":
		return []byte("[]"), nil
	case cmd[0] == "gh":
		w.mu.Lock()
		hold := w.hold
		if strings.Contains(j, " --state merged ") {
			if w.mergedHold != nil {
				hold = w.mergedHold
			}
		}
		if strings.HasPrefix(j, "gh api graphql ") {
			w.threads++
		}
		w.mu.Unlock()
		if hold != nil {
			select {
			case <-hold:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return w.gh(cmd)
	}
	return nil, fmt.Errorf("unexpected command %v", argv)
}

func (w *stallWorld) gh(cmd []string) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	j := strings.Join(cmd, " ")
	if strings.HasPrefix(j, "gh pr list --json ") {
		// As gh does: newest first, at most --limit rows, 30 when it is not given.
		limit := 30
		for i, a := range cmd {
			if a == "--limit" && i+1 < len(cmd) {
				limit, _ = strconv.Atoi(cmd[i+1])
			}
		}
		type open struct {
			branch string
			number int
		}
		var list []open
		for b, n := range w.open {
			list = append(list, open{b, n})
		}
		sort.Slice(list, func(i, k int) bool { return list[i].number > list[k].number })
		var rows []string
		for i, o := range list {
			if i == limit {
				break
			}
			rows = append(rows, fmt.Sprintf(`{"number":%d,"title":"t","headRefName":%q,"author":{"login":"dev"},"isDraft":false}`, o.number, o.branch))
		}
		return []byte("[" + strings.Join(rows, ",") + "]"), nil
	}
	if len(cmd) > 5 && cmd[3] == "--head" && cmd[5] == "--state" && cmd[6] == "merged" {
		b := cmd[4]
		w.calls[b]++
		switch r := w.merged[b]; r {
		case "", "none":
			return []byte("[]"), nil
		case "fail":
			return nil, fmt.Errorf("gh: could not reach GitHub")
		default:
			// "<number>@<head commit>": one merged pull request, with its headRefOid.
			n, oid, _ := strings.Cut(r, "@")
			return []byte(fmt.Sprintf(`[{"number":%s,"headRefOid":%q}]`, n, oid)), nil
		}
	}
	return []byte("[]"), nil
}

// stallRepo is a committed repository with a worktree per name, on change/<name>, and
// a config with the stall time stallMinutes; it returns the root and each worktree's
// path, as git lists it.
func stallRepo(t *testing.T, names ...string) (string, map[string]string) {
	t.Helper()
	return stallRepoAt(t, stallMinutes, names...)
}

// stallRepoAt is stallRepo with the stall time minutes.
func stallRepoAt(t *testing.T, minutes string, names ...string) (string, map[string]string) {
	t.Helper()
	root := signals.ResolvePath(t.TempDir())
	gitRun(t, root, "init", "-q", "-b", "main")
	writeStallConfig(t, root, minutes)
	writeFile(t, filepath.Join(root, ".gitignore"), "w/\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "init")
	paths := map[string]string{}
	for _, n := range names {
		gitRun(t, root, "worktree", "add", "-q", "-b", "change/"+n, filepath.Join("w", n))
		paths[n] = signals.ResolvePath(filepath.Join(root, "w", n))
	}
	return root, paths
}

// writeStallConfig writes the repository's config with the stall time minutes.
func writeStallConfig(t *testing.T, root, minutes string) {
	t.Helper()
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"Stalls","version":7,"lanes":{"main":"orchestrator","change/":"build"},
		"base":"main","tmux_socket":"`+noServerSocket()+`","alerts":{"stall_minutes":`+minutes+`}}`)
}

// writeReceipt writes a clean gate receipt for the worktree's HEAD into its git dir,
// as scripts/ci/run-local.sh does.
func writeReceipt(t *testing.T, wt string) {
	t.Helper()
	head := gitRun(t, wt, "rev-parse", "HEAD")
	gd := gitRun(t, wt, "rev-parse", "--absolute-git-dir")
	writeFile(t, filepath.Join(gd, "ci-receipt"), head+"\tfull\tclean\n")
}

func stallTicks() Ticks {
	t := fastTicks()
	t.PRs, t.Git, t.StallSave, t.MergedNone = 100*time.Millisecond, 100*time.Millisecond, 100*time.Millisecond, 150*time.Millisecond
	return t
}

// stopSaveTicks are stallTicks with no tick save within a test, so the clean stop's
// save is the only writer of stall.json.
func stopSaveTicks() Ticks {
	t := stallTicks()
	t.StallSave = time.Hour
	return t
}

// noticeLog records the notifications a panel sends.
type noticeLog struct {
	mu sync.Mutex
	ns []state.Notice
}

func (l *noticeLog) notify(n state.Notice) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ns = append(l.ns, n)
	return nil
}

// stalled lists the lanes a notification was sent for as stalled.
func (l *noticeLog) stalled() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, n := range l.ns {
		for _, k := range n.Keys {
			if strings.HasPrefix(k, state.AlertStalled+":") {
				out = append(out, strings.TrimPrefix(k, state.AlertStalled+":"))
			}
		}
	}
	return out
}

type stallPanel struct {
	c     liveClient
	polls *pollCounter
	stop  func()
}

func runStallPanel(t *testing.T, root, home string, w *stallWorld, log *noticeLog) *stallPanel {
	t.Helper()
	return runStallPanelTicks(t, root, home, w, log, stallTicks())
}

func runStallPanelTicks(t *testing.T, root, home string, w *stallWorld, log *noticeLog, ticks Ticks) *stallPanel {
	t.Helper()
	return runStallPanelTrust(t, root, home, w, log, ticks, true)
}

// runStallPanelTrust is runStallPanelTicks with the config trusted or not.
func runStallPanelTrust(t *testing.T, root, home string, w *stallWorld, log *noticeLog, ticks Ticks, trust bool) *stallPanel {
	t.Helper()
	polls := newPollCounter()
	_, c, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: w.run, TrustConfig: trust,
		Ticks: ticks, OnPoll: polls.hook, Notify: log.notify})
	var once sync.Once
	p := &stallPanel{c: c, polls: polls, stop: func() { once.Do(stop) }}
	t.Cleanup(p.stop)
	return p
}

// state is /api/state of the default project (""), or of the project named.
func (p *stallPanel) state(t *testing.T, project string) state.View {
	t.Helper()
	if project == "" {
		return p.c.state(t)
	}
	req, _ := http.NewRequest("GET", p.c.base+"/api/state?project="+project, nil)
	req.Header.Set("Cookie", p.c.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v state.View
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatalf("/api/state?project=%s: %d %v", project, resp.StatusCode, err)
	}
	return v
}

// laneStall is the lane's stall as /api/state carries it, nil when it is not stalled.
func laneStall(v state.View, path string) *state.StallView {
	for _, l := range append(append([]state.LaneView(nil), v.Lanes...), v.QuietWorktrees...) {
		if l.Path == path {
			return l.Stall
		}
	}
	return nil
}

func agent(sid, cwd, status, waitingFor string) signals.Agent {
	return signals.Agent{PID: 4000 + len(sid), Cwd: cwd, Kind: "interactive", StartedAt: 1_700_000_000_000, SessionID: sid, Status: status, WaitingFor: waitingFor}
}

// [STALL-1-S5] [STALL-1-S19] With no page in view, a lane with a clean receipt for
// its HEAD and no pull request is stalled while the merged read finds none, still
// stalled with the same since when the next merged read fails, and not stalled once
// its branch's pull request has been listed open and is then read merged; a second
// lane whose first merged read fails is not stalled until one succeeds.
func TestStallWithNoPageReadsTheReceiptAndTheMergedRead(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "a", "b")
	home := t.TempDir()
	writeReceipt(t, wt["a"])
	writeReceipt(t, wt["b"])
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sa", wt["a"], "idle", ""), agent("sb", wt["b"], "idle", "")}
		w.merged["change/a"], w.merged["change/b"] = "none", "fail"
	})
	p := runStallPanel(t, root, home, w, &noticeLog{})

	var since int64
	waitFor(t, "lane a stalled on its gate's pass with no pull request", func() bool {
		s := laneStall(p.state(t, ""), wt["a"])
		if s != nil && !s.Approx && strings.Contains(strings.Join(s.OpenWork, ","), "gate passed for") &&
			strings.Contains(strings.Join(s.OpenWork, ","), "no PR") {
			since = s.Since
			return true
		}
		return false
	})
	// STALL-1-S19's second lane: its merged read has failed every time so far.
	n := w.mergedCalls("change/b")
	waitFor(t, "lane b's merged read failing twice more", func() bool { return w.mergedCalls("change/b") >= n+2 })
	if s := laneStall(p.state(t, ""), wt["b"]); s != nil {
		t.Fatalf("lane b, whose merged read never succeeded, is stalled: %+v", s)
	}

	// The next merged read of lane a fails: the last result stands.
	w.set(func(w *stallWorld) { w.merged["change/a"] = "fail" })
	n = w.mergedCalls("change/a")
	waitFor(t, "lane a's merged read failing twice", func() bool { return w.mergedCalls("change/a") >= n+2 })
	if s := laneStall(p.state(t, ""), wt["a"]); s == nil || s.Since != since {
		t.Fatalf("lane a after a failed merged read: %+v, want still stalled since %d", s, since)
	}

	// Lane b's read succeeds: now it is stalled.
	w.set(func(w *stallWorld) { w.merged["change/b"] = "none" })
	waitFor(t, "lane b stalled once its merged read succeeds", func() bool { return laneStall(p.state(t, ""), wt["b"]) != nil })

	// Lane a's pull request is listed open, then merged.
	w.set(func(w *stallWorld) { w.open["change/a"], w.merged["change/a"] = 153, "none" })
	waitFor(t, "lane a's pull request #153 listed open", func() bool {
		s := laneStall(p.state(t, ""), wt["a"])
		return s != nil && strings.Contains(strings.Join(s.OpenWork, ","), "#153 open")
	})
	head := gitRun(t, wt["a"], "rev-parse", "HEAD")
	w.set(func(w *stallWorld) { delete(w.open, "change/a"); w.merged["change/a"] = "153@" + head })
	waitFor(t, "lane a not stalled once #153 is read merged", func() bool { return laneStall(p.state(t, ""), wt["a"]) == nil })
	if s := laneStall(p.state(t, ""), wt["b"]); s == nil {
		t.Fatal("lane b's stall cleared with lane a's merge")
	}
}

// #188 (D10, the owner's decision of 2026-10-20): a merge counts only when its head
// commit is the lane's HEAD. A branch reused after a squash merge: gh lists an old
// merged pull request whose headRefOid is the branch's older commit, while the lane's
// HEAD is a new commit with a clean receipt and no pull request open. The old merge is
// not HEAD's, so the lane is stalled under D3 (b), its open work the gate's pass.
// [STALL-1-S26]
func TestStallReusedBranchCountsNoOlderMerge(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "a")
	home := t.TempDir()
	old := gitRun(t, wt["a"], "rev-parse", "HEAD")
	gitRun(t, wt["a"], "commit", "-q", "--allow-empty", "-m", "after the squash merge")
	writeReceipt(t, wt["a"])
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")}
		w.merged["change/a"] = "77@" + old
	})
	p := runStallPanel(t, root, home, w, &noticeLog{})
	waitFor(t, "lane a, its branch's old merge not HEAD's, stalled on its gate's pass with no pull request", func() bool {
		s := laneStall(p.state(t, ""), wt["a"])
		return s != nil && !s.Approx && strings.Contains(strings.Join(s.OpenWork, ","), "gate passed for") &&
			strings.Contains(strings.Join(s.OpenWork, ","), "no PR")
	})
	if w.mergedCalls("change/a") == 0 {
		t.Fatal("lane a stalled with no merged read asked for")
	}
}

// Group 3, review round 2, M1 (D3, D10): the stall rule reads the open-PR list as
// complete, and gh lists 30 rows unless asked for more. With 31 open pull requests, the
// lane's the oldest, its pull request must still count as open: the lane is stalled on
// #1 open, and no merged read is asked for its branch.
func TestStallSeesAnOpenPRPastGhsDefaultLimit(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "a")
	home := t.TempDir()
	writeReceipt(t, wt["a"])
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")}
		w.open["change/a"] = 1
		for n := 2; n <= 31; n++ {
			w.open[fmt.Sprintf("change/other-%d", n)] = n
		}
	})
	p := runStallPanel(t, root, home, w, &noticeLog{})
	waitFor(t, "lane a stalled on #1 open, the oldest of 31 open pull requests", func() bool {
		s := laneStall(p.state(t, ""), wt["a"])
		return s != nil && strings.Contains(strings.Join(s.OpenWork, ","), "#1 open")
	})
	if n := w.mergedCalls("change/a"); n != 0 {
		t.Fatalf("lane a, its pull request open, was due %d merged reads, want 0", n)
	}
}

// defaultNotifyTicks are stallTicks with the notify source at its default tick (2 s),
// so a test's window holds the notify ticks a running panel has in it.
func defaultNotifyTicks() Ticks {
	t := stallTicks()
	t.Notify = DefaultTicks().Notify
	return t
}

// holdMerged makes every merged read wait until the returned release is called (once
// or more).
func (w *stallWorld) holdMerged() (release func()) {
	hold := make(chan struct{})
	w.set(func(w *stallWorld) { w.mergedHold = hold })
	var once sync.Once
	return func() {
		once.Do(func() {
			w.set(func(w *stallWorld) { w.mergedHold = nil })
			close(hold)
		})
	}
}

// listsPR: the view's pull requests list number.
func listsPR(v state.View, number int) bool {
	for _, pr := range v.PRs {
		if pr.Number == number {
			return true
		}
	}
	return false
}

// Group 3, review round 1, M1 (D6: once per stretch), end to end with the notifier at
// its default interval and tick: a lane stalled on its open pull request and pushed;
// the pull request is closed unmerged, and the lane's merged read, now due, is held. The
// notifier keeps the stall's mark through the notify ticks while the read is
// unanswered, and once it answers "none" the lane is stalled again with the same since,
// and nothing is sent again.
func TestStallClosedPRSendsNothingAgain(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "a")
	home := t.TempDir()
	writeReceipt(t, wt["a"])
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")}
		w.open["change/a"] = 152
	})
	log := &noticeLog{}
	p := runStallPanelTicks(t, root, home, w, log, defaultNotifyTicks())
	id := p.state(t, "").ProjectID
	var since int64
	waitFor(t, "lane a stalled on #152 and pushed", func() bool {
		s := laneStall(p.state(t, ""), wt["a"])
		if s != nil {
			since = s.Since
		}
		m, ok := notifierMark(t, home, root, wt["a"])
		return s != nil && ok && m == s.Since && len(log.stalled()) == 1
	})
	release := w.holdMerged()
	w.set(func(w *stallWorld) { delete(w.open, "change/a"); w.merged["change/a"] = "none" })
	waitFor(t, "#152 no longer listed", func() bool { return !listsPR(p.state(t, ""), 152) })
	for i := 0; i < 2; i++ {
		p.polls.more(t, id+"/notify", 1)
		if m, ok := notifierMark(t, home, root, wt["a"]); !ok || m != since {
			t.Fatalf("notify tick %d with the merged read unanswered: notifier.json's mark is %d (held %v), want %d", i+1, m, ok, since)
		}
	}
	release()
	waitFor(t, "lane a stalled again once its merged read answers none", func() bool {
		s := laneStall(p.state(t, ""), wt["a"])
		return s != nil && strings.Contains(strings.Join(s.OpenWork, ","), "no PR")
	})
	if s := laneStall(p.state(t, ""), wt["a"]); s == nil || s.Since != since {
		t.Fatalf("lane a after #152 closed: %+v, want stalled since %d", s, since)
	}
	p.polls.more(t, id+"/notify", 2)
	if got := log.stalled(); len(got) != 1 {
		t.Errorf("stalled notices %v, want one for the one stretch", got)
	}
	if m, ok := notifierMark(t, home, root, wt["a"]); !ok || m != since {
		t.Errorf("notifier.json's mark is %d (held %v), want %d", m, ok, since)
	}
}

// Group 3, review round 1, M1's mirror (D10), end to end with the notifier at its
// default interval and tick: a lane whose merged read found "none" before its pull
// request was opened; the pull request merges, the merged read is held, and the lane's
// session goes idle and stays quiet past the stall time. The old "none" is no result
// once the branch has left the open-PR list, so the lane is neither stalled nor pushed
// as "gate passed, no PR"; and not once the read finds the merge.
func TestStallMergedPRNotPushedOnAnOldNone(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "a")
	home := t.TempDir()
	writeReceipt(t, wt["a"])
	head := gitRun(t, wt["a"], "rev-parse", "HEAD")
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sa", wt["a"], "busy", "")}
		w.merged["change/a"] = "none"
	})
	log := &noticeLog{}
	p := runStallPanelTicks(t, root, home, w, log, defaultNotifyTicks())
	id := p.state(t, "").ProjectID
	waitFor(t, "lane a's merged read answering none", func() bool { return w.mergedCalls("change/a") > 0 })
	w.set(func(w *stallWorld) { w.open["change/a"] = 152 })
	waitFor(t, "#152 listed open", func() bool { return listsPR(p.state(t, ""), 152) })
	p.polls.more(t, id+"/readiness", 2) // the readiness source has seen the branch in the open list
	release := w.holdMerged()
	defer release()
	w.set(func(w *stallWorld) { delete(w.open, "change/a"); w.merged["change/a"] = "152@" + head })
	waitFor(t, "#152 no longer listed", func() bool { return !listsPR(p.state(t, ""), 152) })
	w.set(func(w *stallWorld) { w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")} })
	// Past the stall time with the merged read held: three notify ticks (6 s, the stall
	// time 1.8 s).
	for i := 0; i < 3; i++ {
		p.polls.more(t, id+"/notify", 1)
		if s := laneStall(p.state(t, ""), wt["a"]); s != nil {
			t.Fatalf("notify tick %d, #152 merged and its merged read unanswered: lane a stalled %+v", i+1, *s)
		}
		if got := log.stalled(); len(got) != 0 {
			t.Fatalf("notify tick %d: a merged pull request's lane pushed: %v", i+1, got)
		}
	}
	release()
	n := w.mergedCalls("change/a")
	// Notify ticks are what advance a merged read. The deadline is the same scaled
	// bound waitFor used, so a read that never arrives fails here instead of spinning
	// until go test's own timeout.
	deadline := time.Now().Add(testwait.Scale(10 * time.Second))
	for w.mergedCalls("change/a") <= n {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting for lane a's merged read after release", testwait.Scale(10*time.Second))
		}
		p.polls.more(t, id+"/notify", 1)
	}
	p.polls.more(t, id+"/notify", 2)
	if s := laneStall(p.state(t, ""), wt["a"]); s != nil {
		t.Errorf("lane a, #152 read merged, stalled: %+v", *s)
	}
	if got := log.stalled(); len(got) != 0 {
		t.Errorf("a merged pull request's lane pushed: %v", got)
	}
}

// seen tells the panel a page is in view, as the page's POST /api/seen does.
func (p *stallPanel) seen(t *testing.T) {
	t.Helper()
	req, _ := http.NewRequest("POST", p.c.base+"/api/seen", nil)
	req.Header.Set("Cookie", p.c.cookie)
	req.Header.Set("Origin", p.c.base)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("POST /api/seen: %d", resp.StatusCode)
	}
}

// tasksRow is the lane's merge-readiness row for its tasks.md, "" when there is none:
// the one place a tasks.md read lands.
func tasksRow(v state.View, path string) string {
	for _, l := range append(append([]state.LaneView(nil), v.Lanes...), v.QuietWorktrees...) {
		if l.Path == path && l.Readiness != nil {
			for _, row := range l.Readiness.Rows {
				if row.Name == "Tasks" {
					return row.Text
				}
			}
		}
	}
	return ""
}

// Group 3, review round 1, L1 (D10: tasks.md and the review threads stay page-only):
// with stalls on and no page in view, a lane with an open pull request and a tasks.md
// has neither read over several readiness polls, while its receipt is; once a page
// says it is in view, both are read.
func TestStallNoPageLeavesTasksAndThreadsPageOnly(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "a")
	home := t.TempDir()
	writeReceipt(t, wt["a"])
	writeFile(t, filepath.Join(wt["a"], "changes", "a", "tasks.md"), "# Tasks\n- [x] 1.1 one\n- [ ] 1.2 two\n")
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")}
		w.open["change/a"] = 152
	})
	p := runStallPanel(t, root, home, w, &noticeLog{})
	id := p.state(t, "").ProjectID
	waitFor(t, "#152 listed open", func() bool { return listsPR(p.state(t, ""), 152) })
	p.polls.more(t, id+"/readiness", 4)
	if w.gitDirReads() == 0 {
		t.Fatal("no receipt read with no page in view: the control does not hold")
	}
	w.mu.Lock()
	threads := w.threads
	w.mu.Unlock()
	if threads != 0 {
		t.Errorf("with no page in view: %d review-thread reads (gh api graphql), want 0", threads)
	}
	if row := tasksRow(p.state(t, ""), wt["a"]); row != "" {
		t.Errorf("with no page in view: tasks.md read (%q), want it unread", row)
	}
	p.seen(t)
	waitFor(t, "the review threads and tasks.md read once a page is in view", func() bool {
		w.mu.Lock()
		threads := w.threads
		w.mu.Unlock()
		return threads > 0 && strings.Contains(tasksRow(p.state(t, ""), wt["a"]), "1 of 2 ticked")
	})
}

// [STALL-1-S8] The stall time is the project's: two projects, each with an idle lane
// and an open pull request; the one whose alerts.stall_minutes is set stalls, while
// the attached one whose sidecar sets it to 0 shows no stall and no stalled alert.
func TestStallTimeIsTheProjects(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "a")
	home := signals.ResolvePath(t.TempDir())
	side, sidecar := attachedRepo(t, home, "side", "")
	writeFile(t, sidecar, `{"name":"Side","version":7,"lanes":{"main":"orchestrator","change/":"build"},"base":"main",
		"tmux_socket":"`+noServerSocket()+`","alerts":{"stall_minutes":0}}`)
	if _, err := install.TrustConfig(home, side, sidecar); err != nil {
		t.Fatal(err)
	}
	gitRun(t, side, "worktree", "add", "-q", "-b", "change/s", filepath.Join(side, ".wt-s"))
	sideWT := signals.ResolvePath(filepath.Join(side, ".wt-s"))
	// A stall.json left by an earlier panel: with stalls off the runtime removes it, and
	// with the 0 read as unset (the 30-minute default) it would keep and rewrite it, so
	// its absence below tells the two apart where a 2 s wait cannot.
	sideStall := filepath.Join(config.ProjectDir(home, side), "stall.json")
	if err := os.MkdirAll(filepath.Dir(sideStall), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sideStall, "{}")
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("own", wt["a"], "idle", ""), agent("att", sideWT, "idle", "")}
		w.open["change/a"], w.open["change/s"] = 10, 11
	})
	p := runStallPanel(t, root, home, w, &noticeLog{})
	waitFor(t, "the project's lane stalled on its open pull request", func() bool {
		s := laneStall(p.state(t, ""), wt["a"])
		return s != nil && strings.Contains(strings.Join(s.OpenWork, ","), "#10 open")
	})
	waitFor(t, "the attached project's lane and its pull request read", func() bool {
		v := p.state(t, "side")
		listed := false
		for _, l := range v.Lanes {
			for _, s := range l.Sessions {
				listed = listed || (l.Path == sideWT && s.ID == "att")
			}
		}
		for _, pr := range v.PRs {
			if listed && pr.Number == 11 {
				return true
			}
		}
		return false
	})
	// Past the stall time again, the attached project still has none.
	time.Sleep(2 * time.Second)
	v := p.state(t, "side")
	if s := laneStall(v, sideWT); s != nil {
		t.Fatalf("the attached project, stall_minutes 0, shows a stall: %+v", s)
	}
	for _, a := range v.Alerts {
		if a.Kind == state.AlertStalled {
			t.Fatalf("the attached project, stall_minutes 0, has a stalled alert: %+v", a)
		}
	}
	if _, err := os.Stat(sideStall); !os.IsNotExist(err) {
		t.Fatalf("the attached project, stall_minutes 0, keeps stall.json (%v): its 0 was not read as off", err)
	}
}

// notifierMark is the since notifier.json holds for the stall of lane, and whether it
// holds one.
func notifierMark(t *testing.T, home, root, lane string) (int64, bool) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(config.ProjectDir(home, root), "notifier.json"))
	if err != nil {
		return 0, false
	}
	var st state.NotifierState
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatalf("notifier.json: %v", err)
	}
	since, ok := st.Notified[state.AlertStalled+":"+lane]
	return since, ok
}

// restartHeld stops p (started with stopSaveTicks, so the clean stop's save is the
// only writer of stall.json), starts a second panel on the same home with every gh
// call held, and returns it once its notifier has ticked while gh is held, checking
// after each tick that notifier.json still holds each lane's mark with its since.
func restartHeld(t *testing.T, p *stallPanel, root, home string, w *stallWorld, log *noticeLog, marks map[string]int64) *stallPanel {
	t.Helper()
	p2, release := restartHolding(t, p, root, home, w, log, marks)
	release()
	return p2
}

// restartHolding is restartHeld with gh still held: release lets it answer.
func restartHolding(t *testing.T, p *stallPanel, root, home string, w *stallWorld, log *noticeLog, marks map[string]int64) (*stallPanel, func()) {
	t.Helper()
	saved := filepath.Join(config.ProjectDir(home, root), "stall.json")
	if _, err := os.Stat(saved); !os.IsNotExist(err) {
		t.Fatalf("stall.json before the clean stop (%v): the stop's save is not the only writer", err)
	}
	p.stop()
	if _, err := os.Stat(saved); err != nil {
		t.Fatalf("no stall.json after a clean stop: %v", err)
	}
	hold := make(chan struct{})
	w.set(func(w *stallWorld) { w.hold = hold })
	p2 := runStallPanel(t, root, home, w, log)
	id := p2.state(t, "").ProjectID
	for i := 0; i < 5; i++ {
		p2.polls.more(t, id+"/notify", 1)
		for lane, since := range marks {
			if got, ok := notifierMark(t, home, root, lane); !ok || got != since {
				t.Fatalf("notify tick %d with gh held: notifier.json's mark for %s is %d (held %v), want %d", i+1, lane, got, ok, since)
			}
		}
	}
	if !p2.state(t, "").StallsPending {
		t.Fatal("with gh held, the view does not say stalls are pending")
	}
	return p2, func() {
		w.set(func(w *stallWorld) { w.hold = nil })
		close(hold)
	}
}

// [STALL-3-S7] Review round 2, M1 (D1): two restarts, the second panel stopped while
// `gh` is still held, so its saves (each tick's and the clean stop's) run before the
// pull requests and the merged read have answered. The third panel still shows the
// stall with the since it was notified for, exact, and sends nothing after its window:
// a save does not write a stall it has not yet judged as ended.
func TestStallSecondRestartBeforeGhAnswersSendsNothingAgain(t *testing.T) {
	t.Parallel()
	// The notifier's per-lane interval made short, so the first panel's push does not
	// hold back a second one for a since the restarts moved.
	root, wt := stallRepoAt(t, stallMinutes+`,"min_interval_seconds":0.1`, "a")
	home := t.TempDir()
	writeReceipt(t, wt["a"])
	w := newStallWorld()
	w.set(func(w *stallWorld) { w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")} })
	first := &noticeLog{}
	p := runStallPanelTicks(t, root, home, w, first, stopSaveTicks())
	var since int64
	waitFor(t, "lane a stalled and notified", func() bool {
		s := laneStall(p.state(t, ""), wt["a"])
		if s == nil || s.Approx || len(first.stalled()) == 0 {
			return false
		}
		got, ok := notifierMark(t, home, root, wt["a"])
		since = s.Since
		return ok && got == since
	})
	again := &noticeLog{}
	p2, release := restartHolding(t, p, root, home, w, again, map[string]int64{wt["a"]: since})
	id := p2.state(t, "").ProjectID
	p2.polls.more(t, id+"/stall", 2) // tick saves with gh held
	p2.stop()
	release()

	p3 := runStallPanel(t, root, home, w, again)
	var s *state.StallView
	waitFor(t, "the third panel shows lane a's stall, exact", func() bool {
		s = laneStall(p3.state(t, ""), wt["a"])
		return s != nil && !s.Approx
	})
	if s.Since != since {
		t.Errorf("after the second restart lane a is stalled since %d, want %d", s.Since, since)
	}
	// Past the third panel's window (the stall time), the notifier has ticked on.
	time.Sleep(2 * time.Second)
	p3.polls.more(t, p3.state(t, "").ProjectID+"/notify", 3)
	if got := again.stalled(); len(got) != 0 {
		t.Fatalf("the second restart sent the stall again: %v", got)
	}
	if got, ok := notifierMark(t, home, root, wt["a"]); !ok || got != since {
		t.Fatalf("after the second restart notifier.json's mark is %d (held %v), want %d", got, ok, since)
	}
}

// [STALL-3-S7] A stall resting on a gate receipt, notified, then the
// panel restarted on the same home with `gh` answering only after the notifier's
// first ticks: the second panel shows the same stall with the same since, sends
// nothing, neither a notification nor a push (push.json is there throughout: group 4's
// review round 1, L6), and notifier.json keeps the stall's mark throughout.
func TestStallRestartSendsNothingAgain(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "a")
	home := t.TempDir()
	prog, runs := pushStandIn(t, "")
	writePush(t, home, argvJSON(prog))
	writeReceipt(t, wt["a"])
	w := newStallWorld()
	w.set(func(w *stallWorld) { w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")} })
	first := &noticeLog{}
	p := runStallPanelTicks(t, root, home, w, first, stopSaveTicks())
	var since int64
	waitFor(t, "lane a stalled and notified", func() bool {
		s := laneStall(p.state(t, ""), wt["a"])
		if s == nil || s.Approx || len(first.stalled()) == 0 {
			return false
		}
		got, ok := notifierMark(t, home, root, wt["a"])
		since = s.Since
		return ok && got == since && len(pushRuns(t, runs)) == 1
	})
	again := &noticeLog{}
	p2 := restartHeld(t, p, root, home, w, again, map[string]int64{wt["a"]: since})
	waitFor(t, "the second panel shows lane a's stall", func() bool { return laneStall(p2.state(t, ""), wt["a"]) != nil })
	if s := laneStall(p2.state(t, ""), wt["a"]); s.Since != since || s.Approx {
		t.Fatalf("after the restart lane a is stalled since %d (approx %v), want %d, exact", s.Since, s.Approx, since)
	}
	id := p2.state(t, "").ProjectID
	p2.polls.more(t, id+"/notify", 3)
	if got := again.stalled(); len(got) != 0 {
		t.Fatalf("the restart sent the stall again: %v", got)
	}
	if got := pushRuns(t, runs); len(got) != 1 {
		t.Fatalf("pushes across the restart %q, want the first panel's one alone", got)
	}
	if got, ok := notifierMark(t, home, root, wt["a"]); !ok || got != since {
		t.Fatalf("after the restart notifier.json's mark is %d (held %v), want %d", got, ok, since)
	}
}

// [STALL-1-S20] [STALL-1-S21] The same restart, with the main checkout's session still
// waiting on a question, and a lane's session whose last Stop listed a running
// subagent whose SubagentStop arrived before the restart, now reading idle: both
// lanes show their stalls with the since they had, and nothing is sent.
func TestStallRestartKeepsTheQuestionAndTheTurn(t *testing.T) {
	t.Parallel()
	root, wt := stallRepo(t, "c")
	home := t.TempDir()
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sq", root, "waiting", "input needed"), agent("ss", wt["c"], "idle", "")}
	})
	first := &noticeLog{}
	p := runStallPanelTicks(t, root, home, w, first, stopSaveTicks())
	waitFor(t, "both sessions listed", func() bool {
		n := 0
		for _, l := range p.state(t, "").Lanes {
			for _, s := range l.Sessions {
				if s.ID == "sq" || s.ID == "ss" {
					n++
				}
			}
		}
		return n == 2
	})
	p.c.post(t, "/hook", fmt.Sprintf(`{"hook_event_name":"Stop","session_id":"ss","cwd":%q,"last_assistant_message":"waiting on the review",
		"background_tasks":[{"id":"a1","type":"subagent","status":"running","description":"Review group 2"}],"session_crons":[]}`, wt["c"]))
	p.c.post(t, "/hook", fmt.Sprintf(`{"hook_event_name":"SubagentStop","session_id":"ss","cwd":%q,"agent_id":"a1","agent_type":"general-purpose"}`, wt["c"]))
	marks := map[string]int64{}
	waitFor(t, "both lanes stalled and notified", func() bool {
		v := p.state(t, "")
		q, s := laneStall(v, root), laneStall(v, wt["c"])
		if q == nil || s == nil || q.Approx || s.Approx || !strings.Contains(s.WaitedOn, "Review group 2") {
			return false
		}
		mq, okq := notifierMark(t, home, root, root)
		ms, oks := notifierMark(t, home, root, wt["c"])
		marks[root], marks[wt["c"]] = q.Since, s.Since
		return okq && oks && mq == q.Since && ms == s.Since
	})
	again := &noticeLog{}
	p2 := restartHeld(t, p, root, home, w, again, marks)
	waitFor(t, "the second panel shows both stalls", func() bool {
		v := p2.state(t, "")
		return laneStall(v, root) != nil && laneStall(v, wt["c"]) != nil
	})
	v := p2.state(t, "")
	if q := laneStall(v, root); q.Since != marks[root] || q.Approx {
		t.Errorf("STALL-1-S20: the question's stall is since %d (approx %v), want %d, exact", q.Since, q.Approx, marks[root])
	}
	s := laneStall(v, wt["c"])
	if s.Since != marks[wt["c"]] || s.Approx {
		t.Errorf("STALL-1-S21: the turn's stall is since %d (approx %v), want %d, exact", s.Since, s.Approx, marks[wt["c"]])
	}
	if !strings.Contains(s.WaitedOn, "1 subagent") || !strings.Contains(s.WaitedOn, "Review group 2") || !strings.Contains(s.WaitedOn, "which finished and woke nothing") {
		t.Errorf("STALL-1-S21: the turn's stall waited on %q, want 1 subagent, \"Review group 2\", which finished and woke nothing", s.WaitedOn)
	}
	id := v.ProjectID
	p2.polls.more(t, id+"/notify", 3)
	if got := again.stalled(); len(got) != 0 {
		t.Fatalf("the restart sent a stall again: %v", got)
	}
}

// Review round 1, M1 (D1, D5): stall.json is the stall rule's, kept for a restart with
// stalls on. A lane stalled and notified, then a panel with stalls off (its notifier
// drops the stall's mark), then one with stalls on again: the last panel starts the
// lane's clock at its own start, as D1 does for a lane with no saved movement, and
// neither restores the stall exact with its old since nor sends it for that since.
func TestStallsTurnedOffForgetTheSavedClocks(t *testing.T) {
	t.Parallel()
	// The notifier's per-lane interval made short, as the days between turning stalls
	// off and on again would: the last panel's push is not held back by the first's.
	const on, off = stallMinutes + `,"min_interval_seconds":0.1`, `0,"min_interval_seconds":0.1`
	root, wt := stallRepoAt(t, on, "a")
	home := t.TempDir()
	w := newStallWorld()
	w.set(func(w *stallWorld) {
		w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")}
		w.open["change/a"] = 10
	})
	first := &noticeLog{}
	p := runStallPanel(t, root, home, w, first)
	var since int64
	waitFor(t, "lane a stalled and notified", func() bool {
		s := laneStall(p.state(t, ""), wt["a"])
		if s == nil || s.Approx || len(first.stalled()) == 0 {
			return false
		}
		got, ok := notifierMark(t, home, root, wt["a"])
		since = s.Since
		return ok && got == since
	})
	p.stop()
	saved := filepath.Join(config.ProjectDir(home, root), "stall.json")
	if _, err := os.Stat(saved); err != nil {
		t.Fatalf("no stall.json after a clean stop with stalls on: %v", err)
	}

	writeStallConfig(t, root, off)
	p = runStallPanel(t, root, home, w, &noticeLog{})
	id := p.state(t, "").ProjectID
	p.polls.more(t, id+"/notify", 2)
	p.stop()
	if _, err := os.Stat(saved); !os.IsNotExist(err) {
		t.Errorf("stall.json kept by a panel with stalls off (%v): turned on again, it restores a week-old clock", err)
	}

	writeStallConfig(t, root, on)
	again := &noticeLog{}
	p = runStallPanel(t, root, home, w, again)
	var s *state.StallView
	waitFor(t, "lane a stalled again", func() bool { s = laneStall(p.state(t, ""), wt["a"]); return s != nil })
	if s.Since <= since {
		t.Errorf("stalls on again: lane a is stalled since %d (approx %v), want a clock from this start, after %d", s.Since, s.Approx, since)
	}
	waitFor(t, "lane a's new stall notified", func() bool { return len(again.stalled()) > 0 })
	if got, ok := notifierMark(t, home, root, wt["a"]); ok && got <= since {
		t.Fatalf("stalls on again: the stall was sent for since %d, want after %d", got, since)
	}
}

// Review round 1, L4 (D10): the reads without a page serve the stall rule alone, so
// with stall_minutes 0 and no page in view, a lane with a clean receipt for its HEAD
// and no pull request costs no git dir read and no `gh pr list --state merged`.
func TestStallsOffReadNothingWithNoPage(t *testing.T) {
	t.Parallel()
	root, wt := stallRepoAt(t, "0", "a")
	home := t.TempDir()
	writeReceipt(t, wt["a"])
	w := newStallWorld()
	w.set(func(w *stallWorld) { w.agents = []signals.Agent{agent("sa", wt["a"], "idle", "")} })
	p := runStallPanel(t, root, home, w, &noticeLog{})
	waitFor(t, "lane a listed", func() bool {
		for _, l := range p.state(t, "").Lanes {
			if l.Path == wt["a"] {
				return true
			}
		}
		return false
	})
	id := p.state(t, "").ProjectID
	p.polls.more(t, id+"/prs", 2)
	p.polls.more(t, id+"/readiness", 3)
	if n, g := w.mergedCalls("change/a"), w.gitDirReads(); n != 0 || g != 0 {
		t.Fatalf("stalls off, no page: %d merged reads and %d git dir reads, want none", n, g)
	}
}
