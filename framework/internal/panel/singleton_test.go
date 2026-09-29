package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// PANEL-5: one panel per machine is enforced, not assumed. The hook URL in
// ~/.claude/settings.json and ~/.clauductor/panel/{port,pid,owner.json} are
// per-machine, so a second panel would silently re-point every hook and, on exit,
// delete the first panel's marker.

// otherProcess starts a real process that is not this one, so its pid and start
// time can stand in for another panel's.
func otherProcess(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd.Process.Pid
}

func writePanelFiles(t *testing.T, home string, pid, port int, owner *PanelOwner) {
	t.Helper()
	dir := panelDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "pid"), strconv.Itoa(pid)+"\n")
	writeFile(t, MarkerPath(home), strconv.Itoa(port)+"\n")
	if owner != nil {
		b, _ := json.Marshal(owner)
		writeFile(t, OwnerPath(home), string(b))
	}
}

// runBounded runs the panel with a deadline: a regression that let a refused
// second panel start would serve until the deadline instead of failing.
func runBounded(t *testing.T, o Options) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return Run(ctx, o)
}

func TestSecondPanelIsRefused(t *testing.T) {
	root, home := setupProject(t)
	pid := otherProcess(t)
	other := &PanelOwner{PID: pid, PStart: ProcStart(pid), Project: "/work/other-project", Name: "Other", Port: 4393}
	writePanelFiles(t, home, pid, 4393, other)
	// The first panel's hooks, which a second panel must not re-point.
	if _, err := InstallHooks(home, 4393); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(SettingsPath(home))

	err := runBounded(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: fakeRunner(root)})
	if err == nil {
		t.Fatal("a second panel started while another one is live")
	}
	for _, want := range []string{"/work/other-project", "pid " + strconv.Itoa(pid)} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not name %q: %v", want, err)
		}
	}
	if after, _ := os.ReadFile(SettingsPath(home)); string(after) != string(before) {
		t.Fatal("a refused panel re-pointed the hooks")
	}
	if b, _ := os.ReadFile(filepath.Join(panelDir(home), "pid")); strings.TrimSpace(string(b)) != strconv.Itoa(pid) {
		t.Fatalf("a refused panel touched the pid file: %q", b)
	}
	if b, _ := os.ReadFile(MarkerPath(home)); strings.TrimSpace(string(b)) != "4393" {
		t.Fatalf("a refused panel touched the marker: %q", b)
	}
}

// A panel from before PANEL-5 wrote no owner.json. It is recognised by its
// /healthz answering as the pid in the pid file.
func TestSecondPanelIsRefusedByAnOlderPanelsHealthz(t *testing.T) {
	root, home := setupProject(t)
	pid := otherProcess(t)
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok pid=%d\n", pid)
	})}
	go srv.Serve(ln)
	defer srv.Close()
	writePanelFiles(t, home, pid, ln.Addr().(*net.TCPAddr).Port, nil)
	err = runBounded(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: fakeRunner(root)})
	if err == nil || !strings.Contains(err.Error(), "pid "+strconv.Itoa(pid)) {
		t.Fatalf("want a refusal naming pid %d, got %v", pid, err)
	}
}

// A pid file left by a killed panel names a pid that is dead, or reused by another
// process (its start time differs). Neither is a live panel: the start goes ahead.
func TestStalePanelRecordDoesNotBlockAStart(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, home string)
	}{
		{"pid reused by another process", func(t *testing.T, home string) {
			pid := otherProcess(t)
			writePanelFiles(t, home, pid, 4393, &PanelOwner{PID: pid, PStart: "Mon Jan  1 00:00:00 2001", Project: "/work/gone", Port: 4393})
		}},
		{"pid dead", func(t *testing.T, home string) {
			cmd := exec.Command("true")
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}
			pid := cmd.Process.Pid
			writePanelFiles(t, home, pid, 4393, &PanelOwner{PID: pid, PStart: "x", Project: "/work/gone", Port: 4393})
		}},
		{"older panel's pid, nothing answers its port", func(t *testing.T, home string) {
			pid := otherProcess(t)
			ln, _ := net.Listen("tcp4", "127.0.0.1:0")
			port := ln.Addr().(*net.TCPAddr).Port
			ln.Close()
			writePanelFiles(t, home, pid, port, nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, home := setupProject(t)
			tc.setup(t, home)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan error, 1)
			ready := make(chan string, 1)
			go func() {
				done <- Run(ctx, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: fakeRunner(root),
					OnReady: func(u string) { ready <- u }})
			}()
			select {
			case <-ready:
			case err := <-done:
				t.Fatalf("a stale record blocked the start: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("never ready")
			}
			if b, _ := os.ReadFile(filepath.Join(panelDir(home), "pid")); strings.TrimSpace(string(b)) != strconv.Itoa(os.Getpid()) {
				t.Fatalf("the new panel did not claim the pid file: %q", b)
			}
			var o PanelOwner
			if b, err := os.ReadFile(OwnerPath(home)); err != nil || json.Unmarshal(b, &o) != nil || o.PID != os.Getpid() || o.Project != root || o.PStart == "" {
				t.Fatalf("owner.json not claimed: %+v %v", o, err)
			}
			cancel()
			<-done
		})
	}
}

// runPanel runs a panel and returns its port and a client.
func runPanel(t *testing.T, o Options) (int, liveClient, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	o.OnReady = func(u string) { ready <- u }
	go func() { done <- Run(ctx, o) }()
	var launch string
	select {
	case launch = <-ready:
	case err := <-done:
		cancel()
		t.Fatalf("Run exited: %v", err)
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("never ready")
	}
	u, _ := url.Parse(launch)
	port, _ := strconv.Atoi(u.Port())
	c := liveClient{base: "http://" + u.Host, cookie: fmt.Sprintf("clauductor_panel_%d=%s", port, u.Query().Get("t"))}
	stop := func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Run did not stop")
		}
	}
	return port, c, stop
}

// On exit a panel removes the marker, pid and owner files only if they are still
// its own. If another panel has claimed them since, they are that panel's.
func TestExitLeavesAnotherPanelsMarker(t *testing.T) {
	root, home := setupProject(t)
	_, _, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: fakeRunner(root)})
	pid := otherProcess(t)
	writePanelFiles(t, home, pid, 4394, &PanelOwner{PID: pid, PStart: ProcStart(pid), Project: "/work/other", Port: 4394})
	stop()
	if b, err := os.ReadFile(MarkerPath(home)); err != nil || strings.TrimSpace(string(b)) != "4394" {
		t.Fatalf("exit removed or changed another panel's marker: %q %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(panelDir(home), "pid")); err != nil || strings.TrimSpace(string(b)) != strconv.Itoa(pid) {
		t.Fatalf("exit removed another panel's pid file: %q %v", b, err)
	}
	if _, err := os.Stat(OwnerPath(home)); err != nil {
		t.Fatalf("exit removed another panel's owner.json: %v", err)
	}
	// Its own files it does remove (the premise).
	root2, home2 := setupProject(t)
	_, _, stop2 := runPanel(t, Options{Project: root2, Port: 0, NoOpen: true, Home: home2, Runner: fakeRunner(root2)})
	stop2()
	for _, p := range []string{MarkerPath(home2), filepath.Join(panelDir(home2), "pid"), OwnerPath(home2)} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s left behind after a clean stop", p)
		}
	}
}

// Every check interval the panel verifies that the hooks still point at it; if
// something re-pointed them, it says so and puts them back.
func TestHookDriftIsRepaired(t *testing.T) {
	root, home := setupProject(t)
	port, c, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: fakeRunner(root),
		HookCheckInterval: 100 * time.Millisecond})
	defer stop()
	// Another panel (an older binary, which cannot be refused) re-points them.
	if _, err := InstallHooks(home, port+1); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "hooks re-pointed back and a warning", func() bool {
		s, _ := os.ReadFile(SettingsPath(home))
		v := c.state(t)
		warned := false
		for _, w := range v.Warnings {
			if strings.Contains(w, "drifted") && strings.Contains(w, strconv.Itoa(port+1)) {
				warned = true
			}
		}
		return strings.Contains(string(s), HookURL(port)) && !strings.Contains(string(s), HookURL(port+1)) && warned
	})
	// Removed outright (a hand edit): put back too.
	if _, err := UninstallHooks(home); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "removed hooks reinstalled", func() bool {
		return ourHooks(t, home)["Stop"] == 1
	})
}

// A hook install that fails (settings.json unreadable as JSON) is not fatal: under
// launchd a fatal error restarts the panel every 30 s. The panel serves, shows a
// banner, and retries with backoff until the install succeeds.
func TestFailedHookInstallIsABannerNotAnExit(t *testing.T) {
	root, home := setupProject(t)
	writeFile(t, SettingsPath(home), "{ this is not json")
	_, c, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: fakeRunner(root),
		HookRetryBase: 50 * time.Millisecond})
	defer stop()
	hasBanner := func(v View) bool {
		for _, b := range v.Banners {
			if strings.Contains(b, "hooks could not be installed") {
				return true
			}
		}
		return false
	}
	waitFor(t, "an install-failure banner", func() bool { return hasBanner(c.state(t)) })
	writeFile(t, SettingsPath(home), "{}")
	waitFor(t, "the retry installs the hooks and clears the banner", func() bool {
		return ourHooks(t, home)["Stop"] == 1 && !hasBanner(c.state(t))
	})
}

// Review round (PANEL-5), end to end: a `claude agents` slower than the poll
// interval (2 s while no hooks flow) must never make a current reading flicker to
// stale between two good polls.
func TestSlowAgentsPollNeverFlickersStale(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the panel for ~20 s")
	}
	root, home := setupProject(t)
	base := fakeRunner(root)
	slow := func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		if len(argv) >= 2 && argv[0] == "claude" && argv[1] == "agents" {
			select {
			case <-time.After(2500 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return []byte(fmt.Sprintf(`[{"pid":1,"cwd":%q,"kind":"interactive","sessionId":"s1","status":"waiting","waitingFor":"permission prompt"}]`, root)), nil
		}
		return base(ctx, dir, argv)
	}
	_, c, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: slow})
	defer stop()
	deadline := time.Now().Add(25 * time.Second)
	var first time.Time
	for time.Now().Before(deadline) {
		v := c.state(t)
		for _, n := range v.NeedsYou {
			if n.Session != "s1" {
				continue
			}
			if first.IsZero() {
				first = time.Now()
			} else if n.Approx {
				t.Fatalf("%s after the first good poll, a current reading read as stale: %+v", time.Since(first), n)
			}
		}
		if !first.IsZero() && time.Since(first) > 10*time.Second {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	if first.IsZero() {
		t.Fatal("the slow poll never produced a Needs-you item")
	}
}

// TestHelperPanelProcess is not a test: TestTwoPanelsStartedTogether runs this test
// binary as a separate panel process through it.
func TestHelperPanelProcess(t *testing.T) {
	if os.Getenv("CLAUDUCTOR_PANEL_HELPER") != "1" {
		t.Skip("helper process for TestTwoPanelsStartedTogether")
	}
	root, home := os.Getenv("HELPER_ROOT"), os.Getenv("HELPER_HOME")
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer cancel()
	err := Run(ctx, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: fakeRunner(root), Out: os.Stdout,
		TmuxSocket: "clauductor-test-no-server", OnReady: func(string) { fmt.Println("READY") }})
	if err != nil {
		fmt.Println("REFUSED:", err)
		os.Exit(3)
	}
	os.Exit(0)
}

// Review round (PANEL-5): two panels started at the same instant both passed the
// pid-file check and fought over the hooks. The machine lock (flock, held for the
// process lifetime) admits exactly one.
func TestTwoPanelsStartedTogether(t *testing.T) {
	if testing.Short() {
		t.Skip("starts panel processes")
	}
	for round := 0; round < 3; round++ {
		root, home := setupProject(t)
		type proc struct {
			cmd  *exec.Cmd
			out  *strings.Builder
			mu   *sync.Mutex
			done chan error
		}
		var ps []*proc
		for i := 0; i < 2; i++ {
			cmd := exec.Command(os.Args[0], "-test.run=^TestHelperPanelProcess$")
			cmd.Env = append(os.Environ(), "CLAUDUCTOR_PANEL_HELPER=1", "HELPER_ROOT="+root, "HELPER_HOME="+home)
			p := &proc{cmd: cmd, out: &strings.Builder{}, mu: &sync.Mutex{}, done: make(chan error, 1)}
			cmd.Stdout = &syncWriter{w: p.out, mu: p.mu}
			cmd.Stderr = cmd.Stdout
			ps = append(ps, p)
		}
		for _, p := range ps {
			if err := p.cmd.Start(); err != nil {
				t.Fatal(err)
			}
		}
		for _, p := range ps {
			p := p
			go func() { p.done <- p.cmd.Wait() }()
		}
		time.Sleep(4 * time.Second)
		var alive, refused []*proc
		for _, p := range ps {
			select {
			case err := <-p.done:
				p.mu.Lock()
				out := p.out.String()
				p.mu.Unlock()
				if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 3 || !strings.Contains(out, "another clauductor panel is running") {
					t.Fatalf("round %d: a panel exited without a refusal (%v):\n%s", round, err, out)
				}
				refused = append(refused, p)
			default:
				alive = append(alive, p)
			}
		}
		for _, p := range alive {
			_ = p.cmd.Process.Signal(syscall.SIGTERM)
			select {
			case <-p.done:
			case <-time.After(5 * time.Second):
				_ = p.cmd.Process.Kill()
			}
		}
		if len(alive) != 1 || len(refused) != 1 {
			var outs []string
			for _, p := range ps {
				p.mu.Lock()
				outs = append(outs, p.out.String())
				p.mu.Unlock()
			}
			t.Fatalf("round %d: %d panels ran and %d were refused; want exactly one of each:\n%s", round, len(alive), len(refused), strings.Join(outs, "\n---\n"))
		}
	}
}

// Review round (PANEL-5): when the hooks point at ANOTHER LIVE panel (one this
// panel could not refuse, e.g. an older binary), the panel says so and leaves them
// alone rather than fighting over them every 30 s. Once that panel is gone, the
// next check repairs them.
func TestHooksOfAnotherLivePanelAreNotStolen(t *testing.T) {
	root, home := setupProject(t)
	port, c, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: fakeRunner(root),
		HookCheckInterval: 100 * time.Millisecond})
	defer stop()
	otherPID := otherProcess(t)
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok pid=%d\n", otherPID)
	})}
	go srv.Serve(ln)
	otherPort := ln.Addr().(*net.TCPAddr).Port
	if _, err := InstallHooks(home, otherPort); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a conflict banner naming the other panel", func() bool {
		for _, b := range c.state(t).Banners {
			if strings.Contains(b, "another live panel") && strings.Contains(b, strconv.Itoa(otherPID)) {
				return true
			}
		}
		return false
	})
	time.Sleep(500 * time.Millisecond) // several checks
	if s, _ := os.ReadFile(SettingsPath(home)); !strings.Contains(string(s), HookURL(otherPort)) || strings.Contains(string(s), HookURL(port)) {
		t.Fatal("the panel re-pointed the hooks of another live panel")
	}
	srv.Close()
	waitFor(t, "hooks repaired once the other panel is gone", func() bool {
		s, _ := os.ReadFile(SettingsPath(home))
		return strings.Contains(string(s), HookURL(port))
	})
}

// Review round (PANEL-5): under launchd (KeepAlive SuccessfulExit=false) a refused
// start exits 0, so launchd does not retry it every 30 s; it says why, once.
func TestLaunchdRefusalExitsCleanly(t *testing.T) {
	root, home := setupProject(t)
	_, _, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: fakeRunner(root)})
	defer stop()
	var out strings.Builder
	var mu sync.Mutex
	err := runBounded(t, Options{Project: root, Port: 0, Home: home, Launchd: true, Runner: fakeRunner(root),
		Out: &syncWriter{w: &out, mu: &mu}, OpenBrowser: func(string) {}})
	mu.Lock()
	defer mu.Unlock()
	if err != nil || !strings.Contains(out.String(), "another clauductor panel is running") {
		t.Fatalf("a refused launchd start must exit 0 and log why: err %v, log:\n%s", err, out.String())
	}
	// A hand-started panel is still refused with an error (non-zero exit).
	if err := runBounded(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: fakeRunner(root)}); err == nil {
		t.Fatal("a refused hand start returned no error")
	}
}
