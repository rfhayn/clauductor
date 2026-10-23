package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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

// fakeRunner stands in for git, claude and gh so Run can be exercised end to end.
func fakeRunner(root string) signals.Runner {
	return func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		// `claude auth status` runs with a lane's environment: no API key (PANEL-15).
		if j := strings.Join(argv, " "); strings.HasPrefix(j, "/usr/bin/env -u ANTHROPIC_API_KEY -u ANTHROPIC_AUTH_TOKEN ") &&
			strings.HasSuffix(j, " claude auth status --json") {
			return []byte(`{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","email":"dev@example.com","orgId":"x","subscriptionType":"max"}`), nil
		}
		switch strings.Join(argv, " ") {
		case "git worktree list --porcelain":
			return []byte(fmt.Sprintf("worktree %s\nHEAD abc\nbranch refs/heads/main\n\n", root)), nil
		case "git --no-optional-locks -c core.fsmonitor= rev-parse --git-common-dir":
			return []byte(".git\n"), nil
		case "claude agents --json":
			return []byte(fmt.Sprintf(`[{"pid":1,"cwd":%q,"kind":"interactive","sessionId":"s1","name":"lane-1","status":"busy"}]`, root)), nil
		case "gh pr list --json number,title,headRefName,author,isDraft,statusCheckRollup,reviewDecision --limit 1000":
			return []byte(`[]`), nil
		case "echo card":
			return []byte("- [ ] an item\n"), nil
		}
		return nil, fmt.Errorf("unexpected command %v", argv)
	}
}

func setupProject(t *testing.T) (root, home string) {
	t.Helper()
	root = signals.ResolvePath(t.TempDir())
	home = t.TempDir()
	// A socket of its own with no server: the panel never reaches the machine's
	// real panel socket.
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"Test","lanes":{"main":"orchestrator"},"tmux_socket":"`+noServerSocket()+`",
		"cards":[{"id":"c","title":"Card","command":["echo","card"],"refresh":"interval:60"}]}`)
	return root, home
}

type liveClient struct {
	base, cookie string
}

func (c liveClient) state(t *testing.T) state.View {
	t.Helper()
	req, _ := http.NewRequest("GET", c.base+"/api/state", nil)
	req.Header.Set("Cookie", c.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v state.View
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (c liveClient) post(t *testing.T, path, body string) int {
	t.Helper()
	resp, err := http.Post(c.base+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

// waitFor waits for cond, up to a deadline that is generous because passing costs
// nothing: a loaded machine (several gates at once) is slow, not wrong. A wait with a
// bound of its own takes waitUntil.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	testwait.For(t, what, 10*time.Second, cond)
}

func TestRunEndToEnd(t *testing.T) {
	t.Parallel()
	root, home := setupProject(t)
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Project: root, Port: 0, NoOpen: true, Home: home, LookupEnv: testEnv, TrustConfig: true,
			Runner: fakeRunner(root), OnReady: func(u string) { ready <- u }})
	}()
	var launch string
	select {
	case launch = <-ready:
	case err := <-done:
		t.Fatalf("Run exited: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("never ready")
	}
	u, _ := url.Parse(launch)
	port, _ := strconv.Atoi(u.Port())
	c := liveClient{base: "http://" + u.Host, cookie: fmt.Sprintf("clauductor_panel_%d=%s", port, u.Query().Get("t"))}

	marker, err := os.ReadFile(install.MarkerPath(home))
	if err != nil || strings.TrimSpace(string(marker)) != strconv.Itoa(port) {
		t.Fatalf("marker %q %v", marker, err)
	}
	for _, ev := range signals.HookEvents {
		if ourHooks(t, home)[ev] != 1 {
			t.Fatalf("hook for %s not installed", ev)
		}
	}
	settings, _ := os.ReadFile(install.SettingsPath(home))
	if !strings.Contains(string(settings), hookURL(port)) {
		t.Fatal("installed hook does not point at the bound port")
	}

	waitFor(t, "agents lane and card", func() bool {
		v := c.state(t)
		return len(v.Lanes) == 1 && len(v.Lanes[0].Sessions) == 1 && v.Lanes[0].Status == "busy" &&
			v.Cards[0].Source.OK && v.Sources["prs"].OK
	})
	if code := c.post(t, "/hook", fmt.Sprintf(`{"session_id":"s1","cwd":%q,"hook_event_name":"SubagentStart","agent_id":"a1","agent_type":"builder"}`, root)); code != 204 {
		t.Fatalf("hook post: %d", code)
	}
	if code := c.post(t, "/hook", `{"session_id":"s9","cwd":"/elsewhere","hook_event_name":"Stop"}`); code != 204 {
		t.Fatalf("foreign hook post: %d", code)
	}
	waitFor(t, "hook applied", func() bool {
		v := c.state(t)
		return v.HookEvents == 1 && v.Dropped == 1 && len(v.Lanes) == 1 && len(v.Lanes[0].Subagents) == 1
	})
	// The account is read, and a status post from a session in another project moves
	// its quota (PANEL-15).
	if code := c.post(t, "/status", `{"session_id":"s9","cwd":"/elsewhere","rate_limits":{"five_hour":{"used_percentage":41}}}`); code != 204 {
		t.Fatalf("foreign status post: %d", code)
	}
	waitFor(t, "the account and the quota", func() bool {
		v := c.state(t)
		return v.Account.Plan == "Max" && v.Account.QuotaMode == state.QuotaWindows && v.Quota != nil &&
			v.Quota.Window("five_hour") != nil && *v.Quota.Window("five_hour").Pct == 41
	})

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	if _, err := os.Stat(install.MarkerPath(home)); !os.IsNotExist(err) {
		t.Fatal("marker left behind after shutdown")
	}
	if ourHooks(t, home)["Stop"] != 1 {
		t.Fatal("hooks should stay installed after shutdown")
	}
}

func TestRunRefusesATakenPortWithoutSideEffects(t *testing.T) {
	t.Parallel()
	root, home := setupProject(t)
	busy, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port
	// A bounded context: if a regression made Run fall back to another port it would
	// serve instead of failing, and this test must fail rather than hang.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = Run(ctx, Options{Project: root, Port: port, NoOpen: true, Home: home, LookupEnv: testEnv, Runner: fakeRunner(root)})
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("want a port-in-use error, got %v", err)
	}
	if _, err := os.Stat(install.SettingsPath(home)); !os.IsNotExist(err) {
		t.Fatal("a refused launch touched settings.json")
	}
	if _, err := os.Stat(install.MarkerPath(home)); !os.IsNotExist(err) {
		t.Fatal("a refused launch wrote the marker")
	}
}

func TestRunNeedsAConfig(t *testing.T) {
	t.Parallel()
	root := signals.ResolvePath(t.TempDir())
	err := Run(context.Background(), Options{Project: root, Port: 0, NoOpen: true, Home: t.TempDir(), LookupEnv: testEnv, Runner: fakeRunner(root)})
	if err == nil || !strings.Contains(err.Error(), "no panel config") {
		t.Fatalf("got %v", err)
	}
}

// Under launchd stdout is a log file: the token must stay in its 0600 file. The
// browser is opened with it directly, once, and the PID sits beside the port marker.
func TestLaunchdRunKeepsTheTokenOutOfTheLog(t *testing.T) {
	t.Parallel()
	root, home := setupProject(t)
	var out strings.Builder
	var mu sync.Mutex
	var opened []string
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Project: root, Port: 0, Home: home, LookupEnv: testEnv, Launchd: true, Out: &syncWriter{w: &out, mu: &mu},
			Runner: fakeRunner(root), TmuxSocket: noServerSocket(),
			OpenBrowser: func(u string) { mu.Lock(); opened = append(opened, u); mu.Unlock() },
			OnReady:     func(u string) { ready <- u }})
	}()
	var launch string
	select {
	case launch = <-ready:
	case err := <-done:
		t.Fatalf("Run exited: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("never ready")
	}
	token, _ := os.ReadFile(install.TokenPath(home))
	tok := strings.TrimSpace(string(token))
	if !strings.HasSuffix(launch, "t="+tok) {
		t.Fatal("launchd run does not use the persistent token")
	}
	pid, err := os.ReadFile(filepath.Join(filepath.Dir(install.MarkerPath(home)), "pid"))
	if err != nil || strings.TrimSpace(string(pid)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("pid file %q %v", pid, err)
	}
	if port, _ := os.ReadFile(install.MarkerPath(home)); !regexp.MustCompile(`^\d+\n$`).Match(port) {
		t.Fatalf("port marker must stay digits only for status-line scripts: %q", port)
	}
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if strings.Contains(out.String(), tok) {
		t.Fatalf("token written to the launchd log:\n%s", out.String())
	}
	if len(opened) != 1 || !strings.Contains(opened[0], tok) {
		t.Fatalf("browser opened %v, want once with the token", opened)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(install.MarkerPath(home)), "pid")); !os.IsNotExist(err) {
		t.Fatal("pid file left behind after a clean stop")
	}
}

type syncWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

// [CATCHUP-4-S1] [CATCHUP-4-S7] [CATCHUP-4-S8] A panel running as the login agent, built
// at R with no local changes, with catch-up on and the owner's #7 changing
// framework/internal/panel/server.go: the fast-forward to #7's merge commit B puts the
// build of B in place and the build of R at clauductor.prev, leaves the token and the
// plist byte for byte as they were, sends one notice naming R, B and the rollback's mv and
// launchctl kickstart, and ends the run with exit status 75. The login agent's start of
// the new binary, within 5 minutes, opens no browser tab and sends one notice naming B;
// the start after it sends none.
func TestCatchUpRebuildsAndRestartsThePanel(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	goPath := needGo(t)
	w := newSelfWorld(t, cuPanelJSON)
	w.catchUpOn(t, true)
	self := w.installBuild(t, goPath)
	rBuild := readBytes(t, w.bin)
	writeFile(t, install.PlistPath(w.home), "<plist>the login agent</plist>\n")
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	var opened openLog
	p := w.startSelfPanel(t, sock, true, rebuildEnv{self: self, exe: w.bin, goCmd: goPath, timeout: 2 * time.Minute}, &notices, &log, &opened)
	waitFor(t, "the login start's browser open", func() bool { return opened.count() == 1 })
	// That open is an hour old now: only the restart's own record keeps the next start's
	// tab closed.
	install.RecordBrowserOpen(w.home, time.Now().Add(-time.Hour))
	p.polls.until(t, "catch-up", 1)
	p.polls.until(t, "prs", 1)
	token, plist := readBytes(t, install.TokenPath(w.home)), readBytes(t, install.PlistPath(w.home))
	b := w.merged(t, 7, "owner", "framework/internal/panel/server.go", serverTwo)
	w.setOpen()

	rs := restartOf(t, p, &log)
	if rs.Code != 75 || rs.From != w.r || rs.To != b {
		t.Fatalf("the run ended with %+v, want exit 75 from R %s to B %s", rs, w.r, b)
	}
	if s, err := install.FileStamp(w.bin); err != nil || s.Module != selfMod || s.Revision != b || s.Modified {
		t.Fatalf("the installed binary is stamped %+v (%v), want %s at B %s with no local changes", s, err, selfMod, b)
	}
	if readBytes(t, w.bin+".prev") != rBuild {
		t.Fatal("clauductor.prev is not the build of R")
	}
	gone(t, w.bin+".new")
	if readBytes(t, install.TokenPath(w.home)) != token || readBytes(t, install.PlistPath(w.home)) != plist {
		t.Fatal("the restart changed the token or the plist")
	}
	w.oneNotice(t, &notices, short(w.r), short(b), "#7",
		"`mv "+w.tildeOf(w.bin+".prev")+" "+w.tildeOf(w.bin)+"`",
		"`launchctl kickstart -k gui/$(id -u)/"+install.LaunchdLabel+"`")
	p.stop()

	// The login agent starts the new binary, stamped B, in the same home.
	newSelf, err := install.FileStamp(w.bin)
	if err != nil {
		t.Fatal(err)
	}
	newEnv := rebuildEnv{self: newSelf, exe: w.bin, goCmd: goPath, timeout: 2 * time.Minute}
	var notices2 noticeLog
	var opened2 openLog
	p2 := w.startSelfPanel(t, sock, true, newEnv, &notices2, &log, &opened2)
	p2.polls.until(t, "catch-up", 1)
	p2.stop() // its browser open is decided by then
	if n := opened2.count(); n != 0 {
		t.Fatalf("the new binary's start opened %d browser tabs, want none", n)
	}
	w.oneNotice(t, &notices2, "now running "+short(b))
	var notices3 noticeLog
	p3 := w.startSelfPanel(t, sock, true, newEnv, &notices3, &log, &opened2)
	p3.polls.until(t, "catch-up", 1)
	p3.stop()
	if ns := catchUpNotices(&notices3); len(ns) != 0 {
		t.Fatalf("a second start sent %d notices, want none: %+v", len(ns), ns)
	}
}

// [CATCHUP-4-S11] A panel running as the login agent that would rebuild, its project
// with catch-up on and alerts.notify false, and push.json's argv a stand-in that records
// its arguments: the restart notice naming R, B and the rollback goes out as a
// notification and as a push, titled "clauductor panel", and so does the new binary's
// notice naming B.
func TestCatchUpRebuildNoticesIgnoreAlertsNotify(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	goPath := needGo(t)
	w := newSelfWorld(t, `{"name":"T","lanes":{"main":"orchestrator"},"alerts":{"notify":false}}`)
	w.catchUpOn(t, true)
	prog, runs := pushStandIn(t, "")
	writePush(t, w.home, argvJSON(prog))
	self := w.installBuild(t, goPath)
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	var opened openLog
	p := w.startSelfPanel(t, sock, true, rebuildEnv{self: self, exe: w.bin, goCmd: goPath, timeout: 2 * time.Minute}, &notices, &log, &opened)
	p.polls.until(t, "catch-up", 1)
	p.polls.until(t, "prs", 1)
	b := w.merged(t, 7, "owner", "framework/internal/panel/server.go", serverTwo)
	w.setOpen()

	restartOf(t, p, &log)
	nt := w.oneNotice(t, &notices, short(w.r), short(b), "`mv ", "`launchctl kickstart -k ")
	if got := pushRuns(t, runs); len(got) != 1 || len(got[0]) != 2 || got[0][0] != "clauductor panel" || got[0][1] != nt.Body {
		t.Fatalf("pushes %q, want one titled clauductor panel with the restart notice's body %q", got, nt.Body)
	}
	p.stop()

	newSelf, err := install.FileStamp(w.bin)
	if err != nil {
		t.Fatal(err)
	}
	var notices2 noticeLog
	p2 := w.startSelfPanel(t, sock, true, rebuildEnv{self: newSelf, exe: w.bin, goCmd: goPath, timeout: 2 * time.Minute}, &notices2, &log, &opened)
	p2.polls.until(t, "catch-up", 1)
	nt2 := w.oneNotice(t, &notices2, "now running "+short(b))
	if got := pushRuns(t, runs); len(got) != 2 || len(got[1]) != 2 || got[1][0] != "clauductor panel" || got[1][1] != nt2.Body {
		t.Fatalf("pushes %q, want the new binary's second, titled clauductor panel with its body %q", got, nt2.Body)
	}
}

// [CATCHUP-4-S2] A panel that would rebuild, and the owner's #7 whose framework/ does not
// compile: the panel keeps running, the installed binary is as it was, no
// clauductor.new is left (the go here writes a partial one before it fails), and one
// notice names the build's error.
func TestCatchUpKeepsThePanelWhenItsBuildFails(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	goPath := needGo(t)
	w := newSelfWorld(t, cuPanelJSON)
	w.catchUpOn(t, true)
	self := w.installBuild(t, goPath)
	was := readBytes(t, w.bin)
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	var opened openLog
	p := w.startSelfPanel(t, sock, true, rebuildEnv{self: self, exe: w.bin, goCmd: partialGo(t, goPath), timeout: 2 * time.Minute},
		&notices, &log, &opened)
	p.polls.until(t, "catch-up", 1)
	p.polls.until(t, "prs", 1)
	b := w.merged(t, 7, "owner", "framework/internal/panel/server.go", "package panel\n\nconst Version =\n")
	w.setOpen()

	waitFor(t, "the failed build's notice", func() bool { return len(catchUpNotices(&notices)) > 0 })
	running(t, p)
	if got := w.head(t); got != b {
		t.Fatalf("HEAD is %s, want the merge %s", got, b)
	}
	w.unchanged(t, was)
	w.oneNotice(t, &notices, short(b), "#7", "keeps running as it was", "its build failed", "server.go", "syntax error")
}

// [CATCHUP-4-S4] A panel started by hand, built at R, and the owner's #7 changing
// framework/: nothing is built, the panel keeps running, and one notice says framework/
// changed and how to rebuild it and start it again.
func TestCatchUpLeavesAHandStartedPanel(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	w := newSelfWorld(t, cuPanelJSON)
	w.catchUpOn(t, true)
	goProg, runs := goStandIn(t)
	exe := filepath.Join(w.dir, "bin", "clauductor")
	w.setOpen(7)
	var notices noticeLog
	var log cuLog
	var opened openLog
	p := w.startSelfPanel(t, sock, false, rebuildEnv{self: install.BuildStamp{Module: selfMod, Revision: w.r}, exe: exe, goCmd: goProg,
		timeout: time.Minute}, &notices, &log, &opened)
	p.polls.until(t, "catch-up", 1)
	p.polls.until(t, "prs", 1)
	b := w.merged(t, 7, "owner", "framework/internal/panel/server.go", serverTwo)
	w.setOpen()

	waitFor(t, "the hand-started panel's notice", func() bool { return len(catchUpNotices(&notices)) > 0 })
	running(t, p)
	if n := goRan(t, runs); n != 0 {
		t.Fatalf("go ran %d times for a panel started by hand", n)
	}
	gone(t, w.bin+".new")
	w.oneNotice(t, &notices, short(b), "which changed framework/", "started by hand",
		"`go build -o "+exe+" ./cmd/clauductor` in framework/", "stop this panel and start it again")
}
