package panel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rfhayn/stridal/internal/panel/config"
	"github.com/rfhayn/stridal/internal/panel/install"
	"github.com/rfhayn/stridal/internal/panel/state"
	"github.com/rfhayn/stridal/internal/testbin"
	"github.com/rfhayn/stridal/internal/testwait"
)

// OPS-118 group 2 (D1): a panel.json the panel's own fast-forward brings is trusted on
// arrival only when every check holds; otherwise it is served untrusted, or not at
// all, and one notice says which.

const arrivedNote = "trusted on arrival from #7"

// cardCfg is a panel.json named name, with alerts.notify as notify, and one card per
// script, each run as `sh <script>` every second, its id the script's base name.
func cardCfg(name string, notify bool, scripts ...string) string {
	cards := make([]string, len(scripts))
	for i, s := range scripts {
		id := strings.TrimSuffix(filepath.Base(s), ".sh")
		cards[i] = fmt.Sprintf(`{"id":%q,"title":%q,"command":["sh",%q],"refresh":"interval:1"}`, id, id, s)
	}
	alerts := ""
	if !notify {
		alerts = `,"alerts":{"notify":false}`
	}
	return `{"name":"` + name + `","lanes":{"main":"orchestrator"}` + alerts + `,"cards":[` + strings.Join(cards, ",") + "]}\n"
}

// badCfg is a panel.json no binary of today reads: its version is above every one.
const badCfg = `{"version":999,"name":"T","lanes":{"main":"orchestrator"},"cards":[{"id":"x","title":"x","command":["sh",".claude/x.sh"],"refresh":"interval:1"}]}` + "\n"

// trustWorld is a cuWorld whose Runner stands in for each card's `sh <script>`, and
// counts the runs, so a test sees which config's commands run.
type trustWorld struct {
	*cuWorld
	cmu   sync.Mutex
	cards map[string]int
}

// newTrustWorld is a project serving panelJSON, with catch-up on, its config trusted
// by the owner's hand (`clauductor panel trust`) when trusted.
func newTrustWorld(t *testing.T, panelJSON string, trusted bool) *trustWorld {
	t.Helper()
	w := &trustWorld{cuWorld: newCUWorld(t, panelJSON), cards: map[string]int{}}
	w.catchUpOn(t, true)
	if trusted {
		if _, err := install.TrustConfig(w.home, w.root, ""); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

func (w *trustWorld) run(ctx context.Context, dir string, argv []string) ([]byte, error) {
	if len(argv) == 2 && argv[0] == "sh" {
		w.cmu.Lock()
		w.cards[argv[1]]++
		w.cmu.Unlock()
		return []byte("ok\n"), nil
	}
	return w.cuWorld.run(ctx, dir, argv)
}

func (w *trustWorld) cardRuns(script string) int {
	w.cmu.Lock()
	defer w.cmu.Unlock()
	return w.cards[script]
}

func (w *trustWorld) cfgPath() string { return filepath.Join(w.root, config.DefaultConfigRel) }

// start is startCatchUpPanel on w's Runner, then tweak.
func (w *trustWorld) start(t *testing.T, sock string, notices *noticeLog, log *cuLog, tweak func(*Options)) *panelRun {
	t.Helper()
	return startPanelWith(t, w.root, w.home, sock, func(o *Options) {
		o.Runner = w.run
		o.Notify = notices.notify
		o.Out = log
		o.Ticks.PRs = 50 * time.Millisecond
		if tweak != nil {
			tweak(o)
		}
	})
}

// arrive is pull request n, open at the panel's start, merged from owner with file
// set to body, leaving the open list: it returns once catch-up's poll for it is done,
// trust on arrival and the reload included, with HEAD at n's merge commit.
func (w *trustWorld) arrive(t *testing.T, p *panelRun, n int, file, body string) string {
	t.Helper()
	p.polls.until(t, "catch-up", 1) // the start: nothing to move yet
	p.polls.until(t, "prs", 1)      // n is on the open list
	c := w.merged(t, n, "owner", file, body)
	k := p.polls.count("catch-up")
	w.setOpen()
	p.polls.until(t, "catch-up", k+1)
	if got := w.head(t); got != c {
		t.Fatalf("HEAD is %s, want #%d's merge commit %s", got, n, c)
	}
	return c
}

// served waits until the project serves the config whose bytes are body, and returns
// its trust as the page shows it.
func served(t *testing.T, p *panelRun, body string) config.TrustView {
	t.Helper()
	h := install.ConfigHash([]byte(body))
	var tv config.TrustView
	waitFor(t, "the project served on the bytes "+config.ShortHash(h), func() bool {
		tv = p.state(t).Trust
		return tv.Hash == h
	})
	return tv
}

// trustReportOf is the page's trust report (Trust config…, read only).
func trustReportOf(t *testing.T, p *panelRun) trustReport {
	t.Helper()
	m := menu(t, p)
	if len(m) != 1 {
		t.Fatalf("menu %+v, want one project", m)
	}
	var id string
	for id = range m {
	}
	_, body := p.post(t, "/api/projects/"+id+"/trust", map[string]any{"dryRun": true})
	var rep trustReport
	result(t, body, &rep)
	return rep
}

// noticeSays fails unless n is titled title and its body names each of want.
func noticeSays(t *testing.T, n state.Notice, title string, want ...string) {
	t.Helper()
	if n.Title != title {
		t.Errorf("notice title %q, want %q (body %q)", n.Title, title, n.Body)
	}
	for _, s := range want {
		if !strings.Contains(n.Body, s) {
			t.Errorf("notice %q does not name %q", n.Body, s)
		}
	}
}

// handPull is the owner's `git pull` to commit c. The caller has seen catch-up's first
// poll, and no pull request has left the open list since, so no catch-up check runs
// (only those two start one) and no fetch of the panel's races this one for
// origin/main's ref lock.
func (w *trustWorld) handPull(t *testing.T, c string) {
	t.Helper()
	gitRun(t, w.root, "fetch", "-q", "origin")
	gitRun(t, w.root, "merge", "-q", "--ff-only", c)
}

func trustedBytes(w *trustWorld, body string) bool {
	return install.TrustedNow(w.home, w.root, w.cfgPath(), install.ConfigHash([]byte(body)))
}

// [CATCHUP-3-S1] The owner's #7, adding a card running `sh .claude/x.sh` and removing
// one running `sh .claude/y.sh`, arrives by the panel's fast-forward: the project serves
// the new config trusted, the new card's command runs, the trust note (on the page and
// in its trust report) is "trusted on arrival from #7", and one notice names #7, owner,
// a + line for x.sh and a − line for y.sh.
func TestCatchUpTrustsTheOwnersMergeOnArrival(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	old, cfg := cardCfg("T", true, ".claude/y.sh"), cardCfg("T", true, ".claude/x.sh")
	w := newTrustWorld(t, old, true)
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	p := w.start(t, sock, &notices, &log, nil)
	w.arrive(t, p, 7, config.DefaultConfigRel, cfg)
	if tv := served(t, p, cfg); !tv.Trusted || tv.Note != arrivedNote {
		t.Fatalf("the new config is served %+v, want trusted with the note %q\n%s", tv, arrivedNote, log.String())
	}
	waitFor(t, "the new card's command run", func() bool { return w.cardRuns(".claude/x.sh") > 0 })
	if rep := trustReportOf(t, p); !rep.Trusted || rep.Note != arrivedNote {
		t.Fatalf("the page's trust report %+v, want trusted with the note %q", rep, arrivedNote)
	}
	ns := catchUpNotices(&notices)
	if len(ns) != 1 {
		t.Fatalf("catch-up sent %d notices, want 1: %+v", len(ns), ns)
	}
	noticeSays(t, ns[0], "T", "#7", "owner")
	var plus, minus bool
	for _, l := range strings.Split(ns[0].Body, "\n") {
		plus = plus || strings.HasPrefix(l, "+ ") && strings.Contains(l, `"sh" ".claude/x.sh"`)
		minus = minus || strings.HasPrefix(l, "− ") && strings.Contains(l, `"sh" ".claude/y.sh"`)
	}
	if !plus || !minus {
		t.Fatalf("the notice %q lacks a + line for sh .claude/x.sh (%v) or a − line for sh .claude/y.sh (%v)", ns[0].Body, plus, minus)
	}
}

// [CATCHUP-3-S2] The owner pulled by hand a commit with no pull request that changed the
// file to Y, with no restart: the owner's #8, changing it to Z, arrives untrusted, Y and
// Z are not recorded, and the notice says the loaded config was not the file the
// fast-forward started from, and that it waits for the owner.
func TestCatchUpDoesNotTrustAConfigItDidNotStartFrom(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	x, y, z := cardCfg("T", true, ".claude/x.sh"), cardCfg("T", true, ".claude/y.sh"), cardCfg("T", true, ".claude/z.sh")
	w := newTrustWorld(t, x, true)
	w.setOpen(8)
	var notices noticeLog
	var log cuLog
	p := w.start(t, sock, &notices, &log, nil)
	p.polls.until(t, "catch-up", 1)
	p.polls.until(t, "prs", 1)
	w.handPull(t, w.push(t, config.DefaultConfigRel, y))
	if tv := p.state(t).Trust; tv.Hash != install.ConfigHash([]byte(x)) || !tv.Trusted {
		t.Fatalf("the premise: the runtime still serves X, trusted: %+v", tv)
	}
	w.arrive(t, p, 8, config.DefaultConfigRel, z)
	if tv := served(t, p, z); tv.Trusted {
		t.Fatalf("Z is served trusted: %+v\n%s", tv, log.String())
	}
	if rep := trustReportOf(t, p); rep.Trusted {
		t.Fatalf("Trust config… is not offered: %+v", rep)
	}
	if trustedBytes(w, y) || trustedBytes(w, z) {
		t.Fatalf("Y (%v) or Z (%v) was recorded as trusted", trustedBytes(w, y), trustedBytes(w, z))
	}
	ns := catchUpNotices(&notices)
	if len(ns) != 1 {
		t.Fatalf("catch-up sent %d notices, want 1: %+v\n%s", len(ns), ns, log.String())
	}
	noticeSays(t, ns[0], "clauductor panel", w.root, "the loaded config was not the file the fast-forward started from", "waits for the owner")
}

// [CATCHUP-3-S3] A config untrusted before stays untrusted: the owner's #7 changing it is
// served untrusted, and the notice says the config was not trusted before.
func TestCatchUpKeepsAnUntrustedConfigUntrusted(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	cfg := cardCfg("T", true, ".claude/x.sh")
	w := newTrustWorld(t, cardCfg("T", true), false)
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	p := w.start(t, sock, &notices, &log, nil)
	w.arrive(t, p, 7, config.DefaultConfigRel, cfg)
	if tv := served(t, p, cfg); tv.Trusted || trustedBytes(w, cfg) {
		t.Fatalf("the new config is trusted: %+v\n%s", tv, log.String())
	}
	ns := catchUpNotices(&notices)
	if len(ns) != 1 {
		t.Fatalf("catch-up sent %d notices, want 1: %+v", len(ns), ns)
	}
	noticeSays(t, ns[0], "clauductor panel", w.root, "the config was not trusted before", "waits for the owner")
}

// [CATCHUP-3-S4] With alerts.notify false in the loaded config and in #7's: HEAD is #7's
// merge commit, the new config is served untrusted, no notice goes, and the panel log
// names why it was not trusted.
func TestCatchUpWithNotificationsOffTrustsNothing(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	cfg := cardCfg("T", false, ".claude/x.sh")
	w := newTrustWorld(t, cardCfg("T", false), true)
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	p := w.start(t, sock, &notices, &log, nil)
	w.arrive(t, p, 7, config.DefaultConfigRel, cfg)
	if tv := served(t, p, cfg); tv.Trusted || trustedBytes(w, cfg) {
		t.Fatalf("the new config is trusted: %+v", tv)
	}
	if ns := catchUpNotices(&notices); len(ns) != 0 {
		t.Fatalf("notices with alerts.notify false: %+v", ns)
	}
	if l := log.String(); !strings.Contains(l, "is not trusted on arrival") || !strings.Contains(l, "turns notices off") {
		t.Fatalf("the panel log does not say why the config was not trusted:\n%s", l)
	}
}

// [CATCHUP-3-S5] A merged config that does not load is not served: the project serves
// the config it had loaded, still trusted, and one notice names the load's error, with
// alerts.notify true and false in the loaded config alike.
func TestCatchUpKeepsTheLoadedConfigWhenTheNewOneDoesNotLoad(t *testing.T) {
	t.Parallel()
	_, perr := config.ParseConfig([]byte(badCfg))
	if perr == nil {
		t.Fatal("the premise: the new config does not load")
	}
	for _, notify := range []bool{true, false} {
		t.Run(fmt.Sprintf("notify=%v", notify), func(t *testing.T) {
			t.Parallel()
			_, sock := throwawaySocket(t)
			old := cardCfg("T", notify, ".claude/y.sh")
			w := newTrustWorld(t, old, true)
			w.setOpen(7)
			var notices noticeLog
			var log cuLog
			p := w.start(t, sock, &notices, &log, nil)
			serving := strings.Count(log.String(), ": serving ")
			w.arrive(t, p, 7, config.DefaultConfigRel, badCfg)
			if tv := served(t, p, old); !tv.Trusted {
				t.Fatalf("the loaded config is no longer trusted: %+v", tv)
			}
			if n := strings.Count(log.String(), ": serving "); n != serving {
				t.Fatalf("the runtime was started again (%d serving lines, was %d)\n%s", n, serving, log.String())
			}
			ns := catchUpNotices(&notices)
			if len(ns) != 1 {
				t.Fatalf("catch-up sent %d notices, want 1: %+v", len(ns), ns)
			}
			noticeSays(t, ns[0], "clauductor panel", w.root, "does not load", perr.Error())
		})
	}
}

// [CATCHUP-3-S6] The owner pulls #7 by hand before the panel's poll: the poll moves
// nothing, the loaded config is unchanged and still trusted, the new bytes are not
// recorded, and no catch-up notice names #7.
func TestCatchUpDoesNotTrustAConfigPulledByHand(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	old, cfg := cardCfg("T", true, ".claude/y.sh"), cardCfg("T", true, ".claude/x.sh")
	w := newTrustWorld(t, old, true)
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	p := w.start(t, sock, &notices, &log, nil)
	p.polls.until(t, "catch-up", 1)
	p.polls.until(t, "prs", 1)
	c := w.merged(t, 7, "owner", config.DefaultConfigRel, cfg)
	w.handPull(t, c)
	k := p.polls.count("catch-up")
	w.setOpen()
	p.polls.until(t, "catch-up", k+1)
	if w.head(t) != c || w.ran("merge") != 0 {
		t.Fatalf("the premise: the owner's pull, not the panel's, brought #7 (merges %d)", w.ran("merge"))
	}
	if tv := p.state(t).Trust; tv.Hash != install.ConfigHash([]byte(old)) || !tv.Trusted {
		t.Fatalf("the loaded config changed: %+v", tv)
	}
	if trustedBytes(w, cfg) {
		t.Fatal("the bytes pulled by hand were recorded as trusted")
	}
	if ns := catchUpNotices(&notices); len(ns) != 0 {
		t.Fatalf("catch-up sent %+v", ns)
	}
}

// [CATCHUP-3-S7] The owner's #7 changes only README.md: the runtime is not started
// again, and its trust and trust note are as before.
func TestCatchUpLeavesTheConfigOfAMergeThatDidNotChangeIt(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	old := cardCfg("T", true, ".claude/y.sh")
	w := newTrustWorld(t, old, true)
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	p := w.start(t, sock, &notices, &log, nil)
	before := served(t, p, old)
	serving := strings.Count(log.String(), ": serving ")
	w.arrive(t, p, 7, "README.md", "read me\n")
	if n := strings.Count(log.String(), ": serving "); n != serving {
		t.Fatalf("the runtime was started again (%d serving lines, was %d)\n%s", n, serving, log.String())
	}
	if tv := p.state(t).Trust; tv != before {
		t.Fatalf("the trust changed: %+v, was %+v", tv, before)
	}
	if ns := catchUpNotices(&notices); len(ns) != 0 {
		t.Fatalf("catch-up sent %+v", ns)
	}
}

// lsTreeFails is w's Runner with every `git ls-tree` failing, so catch-up cannot read
// the config's blobs at either end of its fast-forward (only catch-up runs ls-tree).
func lsTreeFails(w *trustWorld) func(*Options) {
	return func(o *Options) {
		o.Runner = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
			if slices.Contains(argv, "ls-tree") {
				return nil, errors.New("fatal: not a tree object")
			}
			return w.run(ctx, dir, argv)
		}
	}
}

// The owner's #7 changes panel.json, and catch-up cannot read the file's blobs: the
// runtime is not started again, its trust is as before, the new bytes are not recorded,
// and one notice, titled "clauductor panel" and sent whatever the loaded config's
// alerts.notify says, says the config was not judged, that the panel keeps serving the
// one it loaded, and why.
//
// This test and the next two run serially, not in parallel: with their five panels
// added to the package's parallel load, TestTemplateLaneGetsItsFirstPromptOnceWhenReady
// re-notified across its restart in 3 runs of 3 (a timing race in the notifier's
// restart that OPS-122 owns), and passed with any one of the three skipped.
func TestCatchUpSaysItCouldNotReadTheConfig(t *testing.T) {
	for _, notify := range []bool{true, false} {
		t.Run(fmt.Sprintf("notify=%v", notify), func(t *testing.T) {
			_, sock := throwawaySocket(t)
			old, cfg := cardCfg("T", notify, ".claude/y.sh"), cardCfg("T", true, ".claude/x.sh")
			w := newTrustWorld(t, old, true)
			w.setOpen(7)
			var notices noticeLog
			var log cuLog
			p := w.start(t, sock, &notices, &log, lsTreeFails(w))
			before := served(t, p, old)
			serving := strings.Count(log.String(), ": serving ")
			w.arrive(t, p, 7, config.DefaultConfigRel, cfg)
			if n := strings.Count(log.String(), ": serving "); n != serving {
				t.Fatalf("the runtime was started again (%d serving lines, was %d)\n%s", n, serving, log.String())
			}
			if tv := p.state(t).Trust; tv != before || trustedBytes(w, cfg) {
				t.Fatalf("the trust changed: %+v, was %+v; new bytes recorded %v", tv, before, trustedBytes(w, cfg))
			}
			ns := catchUpNotices(&notices)
			if len(ns) != 1 {
				t.Fatalf("catch-up sent %d notices, want 1: %+v\n%s", len(ns), ns, log.String())
			}
			noticeSays(t, ns[0], "clauductor panel", w.root, "#7", "could not read its "+config.DefaultConfigRel,
				"trusted nothing on arrival", "keeps serving the config it had loaded", "not a tree object")
		})
	}
}

// The same, with a rebuild following: catch-up does not read the unread config as
// unchanged, so the rebuild's preflight runs the new binary's config-check on it. When
// that passes, the swap runs with nothing trusted, and the notice says the rebuilt panel
// serves the file as it is on disk; when it fails, nothing is swapped, and the
// rebuild's notice comes before the one that says the panel keeps its loaded config.
func TestCatchUpChecksAConfigItCouldNotReadBeforeARebuild(t *testing.T) {
	for _, tc := range []struct {
		name, answer string
		swaps        int
	}{
		{"checked", `{"name":"T","notify":true,"runs":[],"tmux_socket":""}`, 1},
		{"refused", `not json`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, sock := throwawaySocket(t)
			old, cfg := cardCfg("T", true, ".claude/y.sh"), cardCfg("T", true, ".claude/x.sh")
			w := newTrustWorld(t, old, true)
			rb := &standInRebuild{home: w.home, root: w.root, bin: newBinary(t, w.cfgPath(), tc.answer)}
			w.setOpen(7)
			var notices noticeLog
			var log cuLog
			p := w.start(t, sock, &notices, &log, func(o *Options) {
				lsTreeFails(w)(o)
				o.selfUpdate = rb.selfUpdate
			})
			w.arrive(t, p, 7, config.DefaultConfigRel, cfg)
			rb.mu.Lock()
			swaps, atSwap := rb.swaps, rb.atSwap
			rb.mu.Unlock()
			if swaps != tc.swaps || trustedBytes(w, cfg) {
				t.Fatalf("%d swaps, want %d; new bytes recorded %v\n%s", swaps, tc.swaps, trustedBytes(w, cfg), log.String())
			}
			ns := catchUpNotices(&notices)
			if tc.swaps == 1 {
				if atSwap.Hash != install.ConfigHash([]byte(cfg)) || atSwap.Trusted {
					t.Fatalf("at the swap: %+v, want the new bytes, untrusted", atSwap)
				}
				if len(ns) != 1 {
					t.Fatalf("catch-up sent %d notices, want 1: %+v", len(ns), ns)
				}
				noticeSays(t, ns[0], "clauductor panel", w.root, "trusted nothing on arrival", "the rebuilt panel serves the file as it is on disk")
				return
			}
			if len(ns) != 2 {
				t.Fatalf("catch-up sent %d notices, want the rebuild's and the config's: %+v", len(ns), ns)
			}
			noticeSays(t, ns[0], "clauductor panel", w.root, "did not rebuild", "panel config-check printed no name")
			noticeSays(t, ns[1], "clauductor panel", w.root, "trusted nothing on arrival", "keeps serving the config it had loaded")
		})
	}
}

// The owner's #7 is trusted on arrival, and the file no longer loads when the panel
// serves it (here the trust notice's sender rewrites it, between the judgement and the
// reload): the runtime keeps serving the config it had loaded, and a second notice,
// titled "clauductor panel", says the new config no longer loads, and why.
func TestCatchUpSaysWhenTheNewConfigNoLongerLoads(t *testing.T) {
	_, sock := throwawaySocket(t)
	_, perr := config.ParseConfig([]byte(badCfg))
	old, cfg := cardCfg("T", true, ".claude/y.sh"), cardCfg("T", true, ".claude/x.sh")
	w := newTrustWorld(t, old, true)
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	var (
		rmu     sync.Mutex
		rewrote error
	)
	p := w.start(t, sock, &notices, &log, func(o *Options) {
		o.Notify = func(n state.Notice) error {
			if len(n.Keys) > 0 && strings.Contains(n.Keys[0], "\x00arrival\x00") && !strings.HasSuffix(n.Keys[0], "\x00reload") {
				rmu.Lock()
				rewrote = os.WriteFile(w.cfgPath(), []byte(badCfg), 0o644)
				rmu.Unlock()
			}
			return notices.notify(n)
		}
	})
	before := served(t, p, old)
	serving := strings.Count(log.String(), ": serving ")
	w.arrive(t, p, 7, config.DefaultConfigRel, cfg)
	rmu.Lock()
	err := rewrote
	rmu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(log.String(), ": serving "); n != serving {
		t.Fatalf("the runtime was started again (%d serving lines, was %d)\n%s", n, serving, log.String())
	}
	if tv := p.state(t).Trust; tv != before {
		t.Fatalf("the project serves %+v, want the config it had loaded %+v", tv, before)
	}
	ns := catchUpNotices(&notices)
	if len(ns) != 2 {
		t.Fatalf("catch-up sent %d notices, want the trust notice and the reload's: %+v\n%s", len(ns), ns, log.String())
	}
	noticeSays(t, ns[0], "T", "#7", "owner")
	noticeSays(t, ns[1], "clauductor panel", w.root, "#7", "no longer loads", "keeps serving the config it had loaded", perr.Error())
}

// standInRebuild stands in for the panel's rebuild of itself (Options.selfUpdate, D4):
// the new binary is bin, a testbin program answering `panel config-check`, read through
// configCheck, as the rebuild's preflight reads it; or the build fails with fail. It
// records each swap, the trust record at the moment it ran, and each discard.
type standInRebuild struct {
	home, root, bin string
	fail            error

	mu       sync.Mutex
	swaps    int
	atSwap   config.TrustView
	discards int
}

func (s *standInRebuild) selfUpdate(ctx context.Context, _ catchUpTarget, _ catchUpResult, cfgPath string) *rebuilt {
	if s.fail != nil {
		return &rebuilt{err: s.fail}
	}
	u := &rebuilt{swap: func(context.Context) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.swaps++
		if cfgPath != "" {
			b, _ := os.ReadFile(cfgPath)
			s.atSwap, _ = install.CheckTrust(s.home, s.root, cfgPath, install.ConfigHash(b), false)
		}
	}, discard: func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.discards++
	}}
	if cfgPath != "" {
		chk, err := configCheck(ctx, s.bin, cfgPath)
		if err != nil {
			return &rebuilt{err: fmt.Errorf("the new binary's preflight failed: %w", err)}
		}
		u.check = &chk
	}
	return u
}

// newBinary writes a stand-in new binary whose `panel config-check --config <cfgPath>`
// prints answer, and which fails on any other arguments.
func newBinary(t *testing.T, cfgPath, answer string) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "answer.json")
	writeFile(t, out, answer)
	bin := filepath.Join(dir, "clauductor.new")
	testbin.Write(t, bin, `#!/bin/sh
[ "$1 $2 $3 $4" = "panel config-check --config `+cfgPath+`" ] || { echo "unexpected: $*" >&2; exit 2; }
cat '`+out+`'
`)
	return bin
}

// [CATCHUP-3-S8] The owner's #7 raises the config's version above what the running
// binary reads, and changes framework/ to read it: the new binary's config-check judges
// it, the bytes are recorded as trusted, with the note, before the swap, and no notice
// names a load error. When instead the build fails: nothing is recorded, the running
// binary serves the config it had loaded, one notice names the build's error and a
// second the new config's load error.
func TestCatchUpTrustsWhatTheNewBinaryReads(t *testing.T) {
	t.Parallel()
	t.Run("built", func(t *testing.T) {
		t.Parallel()
		_, sock := throwawaySocket(t)
		old := cardCfg("T", true, ".claude/y.sh")
		w := newTrustWorld(t, old, true)
		rb := &standInRebuild{home: w.home, root: w.root, bin: newBinary(t, w.cfgPath(),
			`{"name":"T","notify":true,"runs":["card x runs [\"sh\" \".claude/x.sh\"] (interval:1)"],"tmux_socket":""}`)}
		w.setOpen(7)
		var notices noticeLog
		var log cuLog
		p := w.start(t, sock, &notices, &log, func(o *Options) { o.selfUpdate = rb.selfUpdate })
		serving := strings.Count(log.String(), ": serving ")
		w.arrive(t, p, 7, config.DefaultConfigRel, badCfg)
		rb.mu.Lock()
		swaps, atSwap := rb.swaps, rb.atSwap
		rb.mu.Unlock()
		if swaps != 1 || !atSwap.Trusted || atSwap.Note != arrivedNote || atSwap.Hash != install.ConfigHash([]byte(badCfg)) {
			t.Fatalf("at the swap (%d): %+v, want the new bytes trusted with the note %q\n%s", swaps, atSwap, arrivedNote, log.String())
		}
		if n := strings.Count(log.String(), ": serving "); n != serving {
			t.Fatalf("the running binary started the runtime again; the new one serves it\n%s", log.String())
		}
		ns := catchUpNotices(&notices)
		if len(ns) != 1 {
			t.Fatalf("catch-up sent %d notices, want the trust notice alone: %+v", len(ns), ns)
		}
		noticeSays(t, ns[0], "T", "#7", "owner", ".claude/x.sh")
		if strings.Contains(ns[0].Body, "does not load") {
			t.Fatalf("a notice names a load error: %q", ns[0].Body)
		}
	})
	t.Run("build failed", func(t *testing.T) {
		t.Parallel()
		_, sock := throwawaySocket(t)
		_, perr := config.ParseConfig([]byte(badCfg))
		old := cardCfg("T", true, ".claude/y.sh")
		w := newTrustWorld(t, old, true)
		rb := &standInRebuild{home: w.home, root: w.root, fail: errors.New("go build: framework/internal/panel/server.go:3:1: syntax error")}
		w.setOpen(7)
		var notices noticeLog
		var log cuLog
		p := w.start(t, sock, &notices, &log, func(o *Options) { o.selfUpdate = rb.selfUpdate })
		w.arrive(t, p, 7, config.DefaultConfigRel, badCfg)
		if trustedBytes(w, badCfg) || rb.swaps != 0 {
			t.Fatalf("a failed build recorded the new bytes (%v) or swapped (%d)", trustedBytes(w, badCfg), rb.swaps)
		}
		if tv := served(t, p, old); !tv.Trusted {
			t.Fatalf("the loaded config is no longer trusted: %+v", tv)
		}
		ns := catchUpNotices(&notices)
		if len(ns) != 2 {
			t.Fatalf("catch-up sent %d notices, want the build's and the load's: %+v", len(ns), ns)
		}
		noticeSays(t, ns[0], "clauductor panel", w.root, "syntax error")
		noticeSays(t, ns[1], "clauductor panel", w.root, "does not load", perr.Error())
	})
}

// A stop (a logout, `launchctl bootout`) after the new binary's reading of #7's config
// was trusted on arrival, before the swap: the new binary is discarded, not swapped, and
// the trust is withdrawn, so the record is the owner's trust of the config it started
// from again: the running binary, which serves the file at the next start, never judged
// it. A second notice, sent although the run has ended, says so, and how the change
// still comes. Review 2026-10-23 found the trust kept, and no word, after such a stop.
// A test of a stop cites no scenario: none has one in its GIVEN.
func TestCatchUpStoppedBeforeTheSwapWithdrawsTheTrust(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	old := cardCfg("T", true, ".claude/y.sh")
	w := newTrustWorld(t, old, true)
	rb := &standInRebuild{home: w.home, root: w.root, bin: newBinary(t, w.cfgPath(),
		`{"name":"T","notify":true,"runs":["card x runs [\"sh\" \".claude/x.sh\"] (interval:1)"],"tmux_socket":""}`)}
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	var run atomic.Pointer[panelRun]
	stopped := make(chan struct{})
	var once sync.Once
	p := w.start(t, sock, &notices, &log, func(o *Options) {
		o.selfUpdate = rb.selfUpdate
		o.Notify = func(n state.Notice) error {
			notices.notify(n)
			// The stop comes as the trust is said: after it was recorded, before the swap.
			if len(n.Keys) > 0 && strings.Contains(n.Keys[0], "\x00arrival\x00") && strings.Contains(n.Body, "trusted the new") {
				once.Do(func() { run.Load().cancel(); close(stopped) })
			}
			return nil
		}
	})
	run.Store(p)
	p.polls.until(t, "catch-up", 1)
	p.polls.until(t, "prs", 1)
	w.merged(t, 7, "owner", config.DefaultConfigRel, badCfg)
	w.setOpen()
	select {
	case <-stopped:
	case <-time.After(testwait.Scale(20 * time.Second)):
		t.Fatalf("no trust on arrival to stop at\n%s", log.String())
	}
	p.stop() // Run's shutdown waits for catch-up's source
	rb.mu.Lock()
	swaps, discards := rb.swaps, rb.discards
	rb.mu.Unlock()
	if swaps != 0 || discards != 1 {
		t.Fatalf("after the stop: %d swaps and %d discards, want none and one\n%s", swaps, discards, log.String())
	}
	if trustedBytes(w, badCfg) {
		t.Fatalf("the new bytes are still trusted after a stop before the swap\n%s", log.String())
	}
	if !trustedBytes(w, old) {
		t.Fatal("the owner's trust of the config the fast-forward started from is gone")
	}
	ns := catchUpNotices(&notices)
	if len(ns) != 2 {
		t.Fatalf("catch-up sent %d notices, want the trust's and the stop's: %+v", len(ns), ns)
	}
	noticeSays(t, ns[1], "clauductor panel", w.root, "#7", "was stopped before it put the build in place",
		"The trust it recorded on arrival for the new "+config.DefaultConfigRel, "is withdrawn", "Its next start runs the binary it ran",
		"catch-up's next fast-forward rebuilds it")
}

// A stop (a logout, `launchctl bootout`) as #7's config arrives with no rebuild to follow,
// in turn right after the fast-forward's merge and as the panel serves the config it has
// just trusted: the config is judged as it is, trusted on arrival with the note, and the
// one notice says so; the reload the stop ends says nothing more, and the next start
// serves the config trusted and runs its card. Review 2026-10-23 found the judgement's
// load ran on the run's ended context, so a stop read a config that loads as one that
// does not and trusted nothing, and a reload the stop cut short said the config "no
// longer loads". A test of a stop cites no scenario: none has one in its GIVEN.
func TestCatchUpStoppedMidArrivalStillTrustsTheConfig(t *testing.T) {
	t.Parallel()
	worktreeList := []string{"git", "worktree", "list", "--porcelain"}
	for _, tc := range []struct {
		name   string
		reload bool // the stop lands in the reload's load, not right after the merge
	}{{"right after the merge", false}, {"as it serves the config", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, sock := throwawaySocket(t)
			old, cfg := cardCfg("T", true, ".claude/y.sh"), cardCfg("T", true, ".claude/x.sh")
			w := newTrustWorld(t, old, true)
			w.setOpen(7)
			var notices noticeLog
			var log cuLog
			var run atomic.Pointer[panelRun]
			var said atomic.Bool // the trust notice went
			stopped := make(chan struct{})
			var once sync.Once
			stop := func() { once.Do(func() { run.Load().cancel(); close(stopped) }) }
			p := w.start(t, sock, &notices, &log, func(o *Options) {
				// The runtime reads its worktrees at its start alone, so the first read
				// after the trust notice is the reload's.
				o.Ticks.Worktrees, o.Ticks.WorktreeWatch, o.Ticks.WorktreeKick = time.Hour, time.Hour, time.Hour
				o.Notify = func(n state.Notice) error {
					notices.notify(n)
					if len(n.Keys) > 0 && strings.Contains(n.Keys[0], "\x00arrival\x00") && strings.Contains(n.Body, "trusted the new") {
						said.Store(true)
					}
					return nil
				}
				o.Runner = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
					if tc.reload && said.Load() && slices.Equal(argv, worktreeList) {
						stop()
					}
					out, err := w.run(ctx, dir, argv)
					if !tc.reload && err == nil && slices.Contains(argv, "merge") && slices.Contains(argv, "--ff-only") {
						stop()
					}
					return out, err
				}
			})
			run.Store(p)
			p.polls.until(t, "catch-up", 1)
			p.polls.until(t, "prs", 1)
			c := w.merged(t, 7, "owner", config.DefaultConfigRel, cfg)
			w.setOpen()
			select {
			case <-stopped:
			case <-time.After(testwait.Scale(20 * time.Second)):
				t.Fatalf("no arrival to stop at\n%s", log.String())
			}
			p.stop() // Run's shutdown waits for catch-up's source
			if got := w.head(t); got != c {
				t.Fatalf("HEAD is %s, want #7's merge commit %s\n%s", got, c, log.String())
			}
			tv, err := install.CheckTrust(w.home, w.root, w.cfgPath(), install.ConfigHash([]byte(cfg)), false)
			if err != nil || !tv.Trusted || tv.Note != arrivedNote {
				t.Fatalf("after the stop the new bytes are %+v (%v), want trusted with the note %q\n%s", tv, err, arrivedNote, log.String())
			}
			ns := catchUpNotices(&notices)
			if len(ns) != 1 {
				t.Fatalf("catch-up sent %d notices, want the trust notice alone: %+v\n%s", len(ns), ns, log.String())
			}
			noticeSays(t, ns[0], "T", "#7", "owner", ".claude/x.sh")
			if !strings.Contains(log.String(), "the panel stopped before it served the new "+config.DefaultConfigRel) {
				t.Fatalf("the log does not say the stop ended the reload\n%s", log.String())
			}

			var again noticeLog
			var log2 cuLog
			p2 := w.start(t, sock, &again, &log2, nil)
			if tv := served(t, p2, cfg); !tv.Trusted || tv.Note != arrivedNote {
				t.Fatalf("the next start serves %+v, want the new config trusted with the note %q\n%s", tv, arrivedNote, log2.String())
			}
			waitFor(t, "the new card's command run", func() bool { return w.cardRuns(".claude/x.sh") > 0 })
		})
	}
}

// [CATCHUP-3-S9] The owner's #7 adds a card and sets alerts.notify false: the new config
// is served untrusted, Trust config… offered, and one notice says it turns notices off
// and waits for the owner.
func TestCatchUpDoesNotTrustAConfigThatSilencesItself(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	cfg := cardCfg("T", false, ".claude/x.sh")
	w := newTrustWorld(t, cardCfg("T", true), true)
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	p := w.start(t, sock, &notices, &log, nil)
	w.arrive(t, p, 7, config.DefaultConfigRel, cfg)
	if tv := served(t, p, cfg); tv.Trusted || trustedBytes(w, cfg) {
		t.Fatalf("the new config is trusted: %+v", tv)
	}
	if rep := trustReportOf(t, p); rep.Trusted {
		t.Fatalf("Trust config… is not offered: %+v", rep)
	}
	ns := catchUpNotices(&notices)
	if len(ns) != 1 {
		t.Fatalf("catch-up sent %d notices, want 1: %+v", len(ns), ns)
	}
	noticeSays(t, ns[0], "clauductor panel", w.root, "turns notices off", "waits for the owner")
}

// [CATCHUP-3-S10] The owner's #7 adds a card the running binary reads, and its
// framework/ does not compile: nothing is recorded, nothing is swapped, and the running
// binary serves the new config untrusted; one notice names the build's error and a
// second says the config waits for the owner, which alerts.notify false in the loaded
// config withholds, and only it.
func TestCatchUpFailedBuildServesTheConfigUntrusted(t *testing.T) {
	t.Parallel()
	for _, notify := range []bool{true, false} {
		t.Run(fmt.Sprintf("notify=%v", notify), func(t *testing.T) {
			t.Parallel()
			_, sock := throwawaySocket(t)
			cfg := cardCfg("T", true, ".claude/x.sh")
			w := newTrustWorld(t, cardCfg("T", notify), true)
			rb := &standInRebuild{home: w.home, root: w.root, fail: errors.New("go build: framework/internal/panel/server.go:3:1: syntax error")}
			w.setOpen(7)
			var notices noticeLog
			var log cuLog
			p := w.start(t, sock, &notices, &log, func(o *Options) { o.selfUpdate = rb.selfUpdate })
			w.arrive(t, p, 7, config.DefaultConfigRel, cfg)
			if trustedBytes(w, cfg) || rb.swaps != 0 {
				t.Fatalf("a failed build recorded the new bytes (%v) or swapped (%d)", trustedBytes(w, cfg), rb.swaps)
			}
			if tv := served(t, p, cfg); tv.Trusted {
				t.Fatalf("the new config is served trusted: %+v", tv)
			}
			if rep := trustReportOf(t, p); rep.Trusted {
				t.Fatalf("Trust config… is not offered: %+v", rep)
			}
			ns := catchUpNotices(&notices)
			want := 1
			if notify {
				want = 2
			}
			if len(ns) != want {
				t.Fatalf("catch-up sent %d notices, want %d: %+v", len(ns), want, ns)
			}
			noticeSays(t, ns[0], "clauductor panel", w.root, "syntax error")
			if notify {
				noticeSays(t, ns[1], "clauductor panel", w.root, "the panel's rebuild failed", "waits for the owner")
			}
		})
	}
}

// configCheck reads a binary's `panel config-check` answer: the four fields, each
// required, and an exit that is not 0 as the load's error.
func TestConfigCheckReadsTheNewBinarysAnswer(t *testing.T) {
	t.Parallel()
	cfg := filepath.Join(t.TempDir(), "panel.json")
	got, err := configCheck(context.Background(), newBinary(t, cfg, `{"name":"T","notify":false,"runs":["card x runs [\"sh\"] (interval:1)"],"tmux_socket":"s"}`), cfg)
	if err != nil || got.Name != "T" || got.Notify || len(got.Runs) != 1 || got.Socket != "s" {
		t.Fatalf("configCheck: %+v %v", got, err)
	}
	for _, answer := range []string{`{"name":"T","runs":[],"tmux_socket":""}`, `{"name":"T","notify":true,"runs":null,"tmux_socket":""}`, `{"name":"T","notify":true,"runs":[]}`, `not json`} {
		if _, err := configCheck(context.Background(), newBinary(t, cfg, answer), cfg); err == nil {
			t.Errorf("the answer %s judged nothing, and was taken", answer)
		}
	}
	bin := filepath.Join(t.TempDir(), "clauductor.new")
	testbin.Write(t, bin, "#!/bin/sh\necho 'panel config: version 999 is not supported' >&2\nexit 1\n")
	if _, err := configCheck(context.Background(), bin, cfg); err == nil || !strings.Contains(err.Error(), "version 999") {
		t.Fatalf("a binary that cannot load the config: %v, want its error", err)
	}
}

// The trust notice's run-list lines fit its body: as many as fit, then one line counting
// the rest, which the page's trust report lists (D1, "clipped to fit").
func TestTrustBodyClipsItsRunLines(t *testing.T) {
	t.Parallel()
	var added []string
	for i := range 40 {
		added = append(added, fmt.Sprintf("card c%d runs [\"sh\" \".claude/c%d.sh\"] (interval:60)", i, i))
	}
	b := trustBody("Catch-up trusted the new .claude/panel.json in /r on arrival from #7, merged by owner.", added, []string{"card y runs [\"sh\"] (interval:1)"})
	if n := len([]rune(b)); n > catchUpBodyMax {
		t.Fatalf("the body is %d runes, over %d", n, catchUpBodyMax)
	}
	lines := strings.Split(b, "\n")
	last := lines[len(lines)-1]
	if !strings.HasPrefix(lines[1], "+ card c0 ") || !strings.Contains(last, "more (the page's trust report lists them all)") {
		t.Fatalf("the body:\n%s", b)
	}
	if b := trustBody("head.", nil, nil); !strings.Contains(b, "run list is as it was") {
		t.Fatalf("an unchanged run list: %q", b)
	}
}
