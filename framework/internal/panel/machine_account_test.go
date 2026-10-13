package panel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pclock "github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/web"
	"github.com/clauductor/clauductor/internal/testwait"
)

const authKey = "auth status --json"

// accountRuntime is a project runtime with what the machine's sources touch.
func accountRuntime(t *testing.T, id string, run signals.Runner, clk pclock.Clock) *Runtime {
	t.Helper()
	cfg := v2Config(t, "")
	root := signals.ResolvePath(t.TempDir())
	return &Runtime{id: id, cfg: cfg, root: root, hub: web.NewHub(state.NewModel(cfg, root, clk.Now()), clk), clock: clk,
		ticks: DefaultTicks(), run: run, kickWT: make(chan struct{}, 1), kickAgents: make(chan struct{}, 1)}
}

// accountMachine is a machine over one project in home, its account and version
// sources running on clk until the test ends; polls counts each source's polls.
func accountMachine(t *testing.T, home string, run signals.Runner, clk pclock.Clock) (*Machine, *pollCounter) {
	t.Helper()
	polls := newPollCounter()
	def := accountRuntime(t, "p", run, clk)
	m := newMachine(Options{Home: home, Out: io.Discard, OnPoll: polls.hook}, clk, DefaultTicks(), []*Runtime{def}, def)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{}, 2)
	for _, s := range []*source{m.sources[0], m.account} { // the version, the account
		go func() { runLoop(ctx, clk, s, m.pollOnce); done <- struct{}{} }()
	}
	t.Cleanup(func() { cancel(); <-done; <-done })
	return m, polls
}

// saveAccount writes account.json as the panel saves it, read at.
func saveAccount(t *testing.T, home string, at time.Time) {
	t.Helper()
	b, _ := json.Marshal(savedAccount{Status: signals.AuthStatus{LoggedIn: true, AuthMethod: "claude.ai", Provider: "firstParty", Plan: "max"}, At: at.UnixMilli()})
	writeFile(t, filepath.Join(config.PanelDir(home), "account.json"), string(b))
}

// settled waits a moment for anything a kick would have started.
func settled() { time.Sleep(testwait.Scale(200 * time.Millisecond)) }

// [CLAUDECALLS-6-S1] A recent saved reading is used at start: no `claude auth
// status` runs, and the status bar shows the saved reading's mode and plan.
func TestAccountSavedReadingIsUsedAtStart(t *testing.T) {
	t.Parallel()
	home, clk := t.TempDir(), pclock.NewFake(t0)
	saveAccount(t, home, t0.Add(-3*time.Hour))
	c := &claudeCounter{}
	m, polls := accountMachine(t, home, c.run, clk)
	polls.until(t, "account", 1)
	polls.until(t, "version", 1)
	settled()
	if n := c.count(authKey); n != 0 {
		t.Fatalf("auth status ran %d time(s) at start", n)
	}
	v := m.defRT().hub.View()
	if v.Account.Mode != signals.ModeSubscription || v.Account.Plan != "Max" || !v.Account.Source.OK {
		t.Fatalf("account %+v", v.Account)
	}
}

// A saved reading 24 h old or more is no reason not to read: it runs at start.
func TestAccountOldSavedReadingIsReadAgain(t *testing.T) {
	t.Parallel()
	home, clk := t.TempDir(), pclock.NewFake(t0)
	saveAccount(t, home, t0.Add(-25*time.Hour))
	c := &claudeCounter{}
	_, polls := accountMachine(t, home, c.run, clk)
	polls.until(t, "account", 1)
	if n := c.count(authKey); n != 1 {
		t.Fatalf("auth status ran %d time(s) at start, want 1", n)
	}
}

// [CLAUDECALLS-6-S4] Adding a project does not read the account: `claude --version`
// runs for it, `claude auth status` does not, and its status bar carries the account.
func TestAccountAddingAProjectDoesNotReadIt(t *testing.T) {
	t.Parallel()
	home, clk := t.TempDir(), pclock.NewFake(t0)
	saveAccount(t, home, t0.Add(-3*time.Hour))
	c := &claudeCounter{}
	m, polls := accountMachine(t, home, c.run, clk)
	polls.until(t, "account", 1)
	polls.until(t, "version", 1)
	added := accountRuntime(t, "q", c.run, clk)
	m.addProject(added)
	polls.until(t, "version", 2)
	settled()
	if n := c.count(authKey); n != 0 || polls.count("account") != 1 {
		t.Fatalf("auth status ran %d time(s), the account polled %d time(s)", n, polls.count("account"))
	}
	if v := added.hub.View(); v.Account.Mode != signals.ModeSubscription || v.Account.Plan != "Max" {
		t.Fatalf("the added project's account %+v", v.Account)
	}
}

// [CLAUDECALLS-6-S5] A failed account read is retried once: it runs at start and once
// more a minute later, and no more until Refresh; the source shows the error.
func TestAccountFailedReadIsRetriedOnce(t *testing.T) {
	t.Parallel()
	home, clk := t.TempDir(), pclock.NewFake(t0)
	c := &claudeCounter{authFails: true}
	m, polls := accountMachine(t, home, c.run, clk)
	polls.until(t, "account", 1)
	if n := c.count(authKey); n != 1 {
		t.Fatalf("auth status ran %d time(s) at start", n)
	}
	polls.until(t, "version", 1)
	clk.BlockUntil(2) // the version's interval, and the account's one retry
	clk.Advance(accountRetry - time.Second)
	settled()
	if n := c.count(authKey); n != 1 {
		t.Fatalf("retried before the minute: %d", n)
	}
	clk.Advance(time.Second)
	polls.until(t, "account", 2)
	if n := c.count(authKey); n != 2 {
		t.Fatalf("auth status ran %d time(s) after the minute, want 2", n)
	}
	clk.Advance(time.Hour)
	settled()
	if n := c.count(authKey); n != 2 {
		t.Fatalf("auth status ran %d time(s) an hour later, want 2: no more until Refresh", n)
	}
	if v := m.defRT().hub.View(); v.Account.Source.OK || !strings.Contains(v.Account.Source.Error, "auth status") {
		t.Fatalf("the source does not show the error: %+v", v.Account.Source)
	}
	m.kickAll() // Refresh
	polls.until(t, "account", 3)
	if n := c.count(authKey); n != 3 {
		t.Fatalf("Refresh: auth status ran %d time(s), want 3", n)
	}
}

// [CLAUDECALLS-6-S2] No timer reads the account: an hour after the read at start,
// with no refresh asked for, no further `claude auth status` has run.
func TestAccountNoTimerReadsIt(t *testing.T) {
	t.Parallel()
	home, clk := t.TempDir(), pclock.NewFake(t0)
	c := &claudeCounter{}
	m, polls := accountMachine(t, home, c.run, clk)
	polls.until(t, "account", 1)
	for i := 0; i < 6; i++ {
		clk.Advance(10 * time.Minute)
		settled()
	}
	if n := c.count(authKey); n != 1 {
		t.Fatalf("auth status ran %d time(s) in an hour, want only the one at start", n)
	}
	if v := m.defRT().hub.View(); !v.Account.Source.OK || v.Account.Plan != "Max" {
		t.Fatalf("account %+v", v.Account)
	}
	b, err := os.ReadFile(filepath.Join(config.PanelDir(home), "account.json"))
	var sa savedAccount
	if err != nil || json.Unmarshal(b, &sa) != nil || sa.Status.Plan != "max" || sa.At != t0.UnixMilli() {
		t.Fatalf("the reading was not saved: %s %v", b, err)
	}
}

func postRefresh(t *testing.T, client liveClient, path string) {
	t.Helper()
	req, _ := http.NewRequest("POST", client.base+path, nil)
	req.Header.Set("Cookie", client.cookie)
	req.Header.Set("Origin", client.base)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		t.Fatalf("POST %s: %d", path, resp.StatusCode)
	}
}

// A lane start runs no `claude auth status` (D5): the page's refresh after a lane
// start re-reads the project's sources but not the machine's, so a session starting
// never has the account read beside it. Only the Refresh button asks for the
// machine's sources (?machine=1).
func TestLaneStartRefreshRunsNoAuthStatus(t *testing.T) {
	t.Parallel()
	js, err := os.ReadFile(filepath.Join("web", "static", "panel.js"))
	if err != nil {
		t.Fatal(err)
	}
	// The lane start's refresh is the first after its POST /lanes; the button's is in
	// its click handler.
	src := string(js)
	start := strings.Index(src, `fetch(api("/lanes"), { method: "POST"`)
	if start < 0 {
		t.Fatal("panel.js: no lane start's POST /lanes")
	}
	// Bounded by the Refresh button's handler, not a byte count: the start handler grows
	// (PANEL-29 added its re-plan), and the button's own /refresh?machine=1 must never be
	// the one found here.
	buttonAt := strings.Index(src, `$("refresh").addEventListener("click"`)
	call := src[start:]
	if i := strings.Index(call, `api("/refresh`); i < 0 || (buttonAt > start && start+i > buttonAt) {
		t.Fatal("panel.js: no refresh after a lane start's POST /lanes")
	} else if call = call[i:]; strings.HasPrefix(call[:strings.IndexByte(call, ')')], `api("/refresh?machine=1"`) {
		t.Fatal("panel.js: the refresh after a lane start asks for the machine's sources (machine=1)")
	}
	button := strings.Index(src, `$("refresh").addEventListener("click"`)
	if button < 0 || !strings.Contains(src[button:button+strings.IndexByte(src[button:], '\n')], `api("/refresh?machine=1")`) {
		t.Fatal(`panel.js: the Refresh button does not ask for the machine's sources (machine=1)`)
	}

	root, home := setupProject(t)
	saveAccount(t, home, time.Now().Add(-3*time.Hour))
	c := &claudeCounter{}
	polls := newPollCounter()
	_, client, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: c.run, OnPoll: polls.hook})
	defer stop()
	polls.until(t, "account", 1)
	polls.until(t, "version", 1)
	polls.until(t, "git", 1)
	gitPolls := polls.count("git")
	postRefresh(t, client, "/api/refresh") // what the page sends after a lane start
	polls.until(t, "git", gitPolls+1)      // a project source kicked by the refresh (its interval is 30 s)
	settled()
	if a, v := c.count(authKey), c.count("--version"); a != 0 || v != 1 || polls.count("account") != 1 {
		t.Fatalf("after a lane start's refresh: auth status %d, --version %d, account polls %d; want 0, 1, 1", a, v, polls.count("account"))
	}
}

// [CLAUDECALLS-6-S3] Refresh reads the account and the version: the page's Refresh
// on a running panel runs `claude auth status` and `claude --version` once each,
// through Run's Runner (the gate wraps it), and saves the account reading.
func TestRefreshReadsTheAccountAndTheVersion(t *testing.T) {
	t.Parallel()
	root, home := setupProject(t)
	saved := time.Now().Add(-3 * time.Hour)
	saveAccount(t, home, saved)
	c := &claudeCounter{}
	polls := newPollCounter()
	_, client, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: c.run, OnPoll: polls.hook})
	defer stop()
	polls.until(t, "account", 1)
	polls.until(t, "version", 1)
	if n := c.count(authKey); n != 0 {
		t.Fatalf("auth status ran %d time(s) at start with a recent saved reading", n)
	}
	postRefresh(t, client, "/api/refresh?machine=1") // the Refresh button's request
	polls.until(t, "account", 2)
	polls.until(t, "version", 2)
	settled()
	if a, v := c.count(authKey), c.count("--version"); a != 1 || v != 2 {
		t.Fatalf("after Refresh: auth status %d, --version %d; want 1 and 2", a, v)
	}
	b, err := os.ReadFile(filepath.Join(config.PanelDir(home), "account.json"))
	var sa savedAccount
	if err != nil || json.Unmarshal(b, &sa) != nil || sa.At <= saved.UnixMilli() {
		t.Fatalf("the refreshed reading was not saved: %s %v", b, err)
	}
}
