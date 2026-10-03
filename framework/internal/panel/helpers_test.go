package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/clauductor/clauductor/internal/leakcheck"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/lease"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/types"
	"github.com/clauductor/clauductor/internal/testwait"
	"github.com/coder/websocket"
)

// waitUntil waits up to d (scaled by CLAUDUCTOR_TEST_SLOW) for cond.
func waitUntil(t *testing.T, what string, d time.Duration, cond func() bool) {
	t.Helper()
	testwait.For(t, what, d, cond)
}

// writeLeaseRecord writes an owner.json or waiter file as the lease protocol
// (docs/panel.md) lays it out on disk.
func writeLeaseRecord(t *testing.T, path string, o lease.LeaseOwner) {
	t.Helper()
	b, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The lane registry's file, as the tests read and write it (lanes.RegistryPath).
type registryFile struct {
	Version int                `json:"version"`
	Project string             `json:"project"`
	Lanes   []types.LaneRecord `json:"lanes"`
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

const sid = "0f8fad5b-d9cb-469f-a165-70867728950e"

func testLaneManager(t *testing.T) *lanes.LaneManager {
	t.Helper()
	cfg, err := loadConfig(t, []byte(`{"name":"T","lanes":{"main":"orchestrator","fix/":"fix","change/":"build","change/propose-*":"propose"},
		"tmux_socket":"sock","lane_types":{"build":{"model":"opus","effort":"high"}}}`))

	if err != nil {
		t.Fatal(err)
	}
	reg, err := lanes.OpenRegistry(t.TempDir(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	return &lanes.LaneManager{Clock: clock.System, TmuxPath: "/opt/homebrew/bin/tmux", Socket: cfg.Socket(), Root: "/repo", Cfg: cfg, Registry: reg,
		Program: []string{"/Users/me/.local/bin/claude"}, LookupEnv: func(string) (string, bool) { return "", false }}
}

var t0 = time.Date(2026, 9, 28, 14, 0, 0, 0, time.UTC)

// goneGrace mirrors the reducer's grace for a template lane's missing tmux session
// (30 s) before it needs a RESTORE.
const goneGrace = 30 * time.Second

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("signals", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("config", "testdata", "panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(t, b)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func fixtureWorktrees(t *testing.T) []signals.Worktree {
	t.Helper()
	wts, err := readWorktreesFrom(fixture(t, "worktrees-fixture.porcelain"))
	if err != nil {
		t.Fatal(err)
	}
	return wts
}

const xWT = "/repo/w/x"

func waitingAgent(status, waitingFor string) []signals.Agent {
	return []signals.Agent{{SessionID: "s1", Cwd: xWT, Status: status, WaitingFor: waitingFor}}
}

// shq single-quotes s for a POSIX shell.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ourHooks counts the panel's tagged hook objects per event in settings.json, in
// their current form: an async command hook whose command posts to a URL carrying
// src=<hookTag> (PANEL-23). The HTTP hook earlier panels installed is not counted.
func ourHooks(t *testing.T, home string) map[string]int {
	t.Helper()
	b, err := os.ReadFile(install.SettingsPath(home))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(b, &settings); err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	hooks, _ := settings["hooks"].(map[string]any)
	for ev, groups := range hooks {
		for _, g := range groups.([]any) {
			for _, h := range g.(map[string]any)["hooks"].([]any) {
				hm := h.(map[string]any)
				cmd, _ := hm["command"].(string)
				if hm["type"] != "command" || hm["async"] != true {
					continue
				}
				s := hookURLInCommand.FindString(cmd)
				if u, err := url.Parse(s); err == nil && s != "" && u.Query().Get("src") == hookTag {
					out[ev]++
				}
			}
		}
	}
	return out
}

func v2Config(t *testing.T, extra string) *config.Config {
	t.Helper()
	c, err := loadConfig(t, []byte(`{"name":"T","lanes":{"change/":"build","fix/":"fix","main":"orchestrator"}`+extra+`}`))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const tplJSON = `,"templates":[
 {"id":"build","title":"Build a change","lane_type":"build","branch_pattern":"change/{name}","first_prompt":"Run build-change for {name}","model":"opus","effort":"high"},
 {"id":"fix","title":"Fix an issue","lane_type":"fix","first_prompt":"Fix issue {issue} on branch fix/{name}"}]`

// ---- alerts and the notifier ----

func alertModel(t *testing.T, cfgExtra string) *state.Model {
	t.Helper()
	m := state.NewModel(v2Config(t, cfgExtra), "/repo", t0)
	m.ApplyWorktrees([]signals.Worktree{{Path: "/repo", Branch: "main"}, {Path: "/repo/w/x", Branch: "change/x"}}, nil, t0)
	return m
}

func alertKinds(v state.View) map[string]string {
	out := map[string]string{}
	for _, a := range v.Alerts {
		out[a.Kind] = a.Severity
	}
	return out
}

// loadConfig reads panel.json bytes the way the panel does: from a file.
func loadConfig(t *testing.T, b []byte) (*config.Config, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "panel.json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return config.LoadConfig(p)
}

// readWorktreesFrom reads `git worktree list --porcelain` output the way the panel
// does, through signals.ReadWorktrees.
func readWorktreesFrom(out []byte) ([]signals.Worktree, error) {
	return signals.ReadWorktrees(context.Background(), func(context.Context, string, []string) ([]byte, error) { return out, nil }, "/")
}

// The page's terminal protocol and the hook address, as the page and Claude Code
// see them: the tests speak the wire format, not the packages' names for it.
const (
	ticketPrefix                      = "ticket."
	closeIdle    websocket.StatusCode = 4000
	closeRotated websocket.StatusCode = 4001
	hookTag                           = "clauductor-panel"
)

type termMsg struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	On   bool   `json:"focused,omitempty"`
}

func hookURL(port int) string { return fmt.Sprintf("http://127.0.0.1:%d/hook?src=%s", port, hookTag) }

// hookURLInCommand finds the panel URL a command hook posts to.
var hookURLInCommand = regexp.MustCompile(`http://127\.0\.0\.1:\d+/hook\?src=[A-Za-z0-9-]+`)

func ownerPath(home string) string { return filepath.Join(config.PanelDir(home), "owner.json") }

// ---- driving a running panel ----

// pollCounter counts each source's polls through Options.OnPoll, so a test asserts
// after N iterations of a source instead of sleeping and hoping they ran.
type pollCounter struct {
	mu      sync.Mutex
	n       map[string]int
	changed chan struct{}
}

func newPollCounter() *pollCounter {
	return &pollCounter{n: map[string]int{}, changed: make(chan struct{})}
}

// hook is the Options.OnPoll.
func (p *pollCounter) hook(source string) {
	p.mu.Lock()
	p.n[source]++
	close(p.changed)
	p.changed = make(chan struct{})
	p.mu.Unlock()
}

func (p *pollCounter) count(source string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.n[source]
}

// more waits until source has polled n more times than it had when called.
func (p *pollCounter) more(t *testing.T, source string, n int) {
	t.Helper()
	p.until(t, source, p.count(source)+n)
}

// until waits until source has polled n times in all.
func (p *pollCounter) until(t *testing.T, source string, n int) {
	t.Helper()
	deadline := time.After(testwait.Scale(15 * time.Second))
	for {
		p.mu.Lock()
		got, changed := p.n[source], p.changed
		p.mu.Unlock()
		if got >= n {
			return
		}
		select {
		case <-changed:
		case <-deadline:
			t.Fatalf("the %s source polled %d times, want %d", source, got, n)
		}
	}
}

// fastTicks is the cadence the panel's integration tests run at: every source a
// test waits on polls within a tenth of a second, so a test waits for polls, not
// for the production cadence. Ratios the model reads (the agents cadence) keep
// their order: fast < slow.
func fastTicks() Ticks {
	return Ticks{
		Worktrees: 500 * time.Millisecond, WorktreeWatch: 100 * time.Millisecond, WorktreeKick: 100 * time.Millisecond,
		AgentsFast: 100 * time.Millisecond, AgentsSlow: 250 * time.Millisecond, AgentsQuiet: 100 * time.Millisecond,
		TmuxFast: 100 * time.Millisecond, TmuxIdle: 100 * time.Millisecond, CardWatch: 100 * time.Millisecond,
		Queues: 100 * time.Millisecond, Prompt: 50 * time.Millisecond, PromptKick: 100 * time.Millisecond,
		Obs: 50 * time.Millisecond, Notify: 50 * time.Millisecond, Trust: 50 * time.Millisecond, Token: 50 * time.Millisecond,
		Hub: 250 * time.Millisecond,
	}
}

// noServerSocket is a tmux socket name no test starts a server on, so a panel that
// runs no lane never touches the machine's real panel socket.
func noServerSocket() string { return leakcheck.NoServerSocket() }

// TestMain fails the run if it leaves a tmux server or a helper process behind. A
// helper panel (TestHelperPanelProcess) is only a panel: its parent stops it with
// SIGTERM, which it must handle itself.
func TestMain(m *testing.M) {
	if os.Getenv("CLAUDUCTOR_PANEL_HELPER") == "1" {
		os.Exit(m.Run())
	}
	os.Exit(leakcheck.Main(m))
}

// machineFree reports whether the machine lock in home is free now. The probe unlocks
// before it closes, as lease.go's does: a child another (parallel) test forks while the
// probe holds the lock keeps a copy of the file until it execs, and Close alone leaves
// the lock with that copy, so the next panel in this home was refused (#78).
func machineFree(home string) bool { return probeMachine(home, nil) }

// probeMachine is machineFree, calling held (if set) while the probe holds the lock: the
// moment a child forked would take its copy of the file.
func probeMachine(home string, held func(*os.File)) bool {
	f, err := install.LockMachine(home)
	if err == nil {
		if held != nil {
			held(f)
		}
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}
	return err == nil
}

// The probe's release holds even while a copy of its file is open elsewhere, as a child
// forked while the probe held the lock has one: a dup shares the open file, and so its
// lock (#78).
func TestTheMachineProbeLeavesNoLockBehindInACopy(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	child := -1
	if !probeMachine(home, func(f *os.File) { child, _ = syscall.Dup(int(f.Fd())) }) || child < 0 {
		t.Fatalf("premise: the probe found the lock free and its file was copied (fd %d)", child)
	}
	defer syscall.Close(child)
	if f, err := install.LockMachine(home); err != nil {
		t.Fatalf("the machine lock is still held by a copy of the probe's file: %v", err)
	} else {
		f.Close()
	}
}

// waitMachineFree waits, after a panel stopped, until its machine lock is free, or
// another panel has claimed the machine (its pid file: a launchd start that was
// waiting takes over at once). The flock goes with the open file, and a child that
// another (parallel) test forks at that moment holds a copy until it execs: the next
// panel in this home would be refused as a second one. A lock still held after 5 s
// is the next start's to report.
func waitMachineFree(home string) {
	claimed := func() bool { _, err := os.Stat(filepath.Join(config.PanelDir(home), "pid")); return err == nil }
	for deadline := time.Now().Add(testwait.Scale(5 * time.Second)); time.Now().Before(deadline) && !machineFree(home) && !claimed(); {
		time.Sleep(5 * time.Millisecond)
	}
}
