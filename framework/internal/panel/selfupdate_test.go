package panel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rfhayn/stridal/internal/panel/clock"
	"github.com/rfhayn/stridal/internal/panel/config"
	"github.com/rfhayn/stridal/internal/panel/install"
	"github.com/rfhayn/stridal/internal/panel/state"
	"github.com/rfhayn/stridal/internal/testbin"
	"github.com/rfhayn/stridal/internal/testwait"
)

// OPS-118 D4: the panel's rebuild and restart of itself, on catch-up's source. The
// install package tests the build, the preflight and the swap; these test what the
// panel decides and says with them, on a throwaway module standing in for its source.

// selfMod is the throwaway module standing in for the panel's own source.
const selfMod = "example.com/selfpanel"

// selfModule is that module's framework/, the twin of the install package's test
// module: a go.mod, the ./cmd/clauductor the rebuild builds (answering the preflight's
// version, panel list and panel config-check), and the files D4 names. Its config-check
// reads the file's name and tmux_socket as JSON, and fails on a file that is not.
var selfModule = map[string]string{
	"framework/go.mod": "module " + selfMod + "\n\ngo 1.22\n",
	"framework/cmd/clauductor/main.go": `package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"example.com/selfpanel/internal/panel"
)

func main() {
	switch a := strings.Join(os.Args[1:], " "); {
	case a == "version":
		fmt.Println("selfpanel", panel.Version)
	case a == "panel list":
		fmt.Println("no projects")
	case len(os.Args) == 5 && strings.HasPrefix(a, "panel config-check --config "):
		var c struct {
			Name   string ` + "`" + `json:"name"` + "`" + `
			Socket string ` + "`" + `json:"tmux_socket"` + "`" + `
		}
		b, err := os.ReadFile(os.Args[4])
		if err == nil {
			err = json.Unmarshal(b, &c)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error: panel config:", err)
			os.Exit(1)
		}
		out, _ := json.Marshal(map[string]any{"name": c.Name, "notify": true, "runs": []string{}, "tmux_socket": c.Socket})
		fmt.Println(string(out))
	default:
		fmt.Fprintln(os.Stderr, "unexpected:", a)
		os.Exit(2)
	}
}
`,
	"framework/internal/panel/server.go":          "package panel\n\nconst Version = \"one\"\n",
	"framework/internal/panel/install/launchd.go": "package install\n",
}

// serverTwo is a merge's framework/internal/panel/server.go: it compiles, and changes
// framework/ only there.
const serverTwo = "package panel\n\nconst Version = \"two\"\n"

// needGo is the go on PATH; it skips without one, and under -short: the test builds.
func needGo(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: runs go build")
	}
	p, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go")
	}
	return p
}

// selfWorld is a cuWorld whose repository holds selfModule, so it is the panel's own
// source: r is the commit the panel was built at, bin the installed binary.
type selfWorld struct {
	*cuWorld
	r, bin string
}

func newSelfWorld(t *testing.T, panelJSON string) *selfWorld {
	t.Helper()
	w := newCUWorld(t, panelJSON)
	for rel, body := range selfModule {
		writeFile(t, filepath.Join(w.other, rel), body)
	}
	gitRun(t, w.other, "add", "-A")
	gitRun(t, w.other, "commit", "-q", "-m", "framework")
	gitRun(t, w.other, "push", "-q", "origin", "main")
	gitRun(t, w.root, "pull", "-q", "--ff-only")
	return &selfWorld{cuWorld: w, r: w.head(t), bin: install.InstalledBinary(w.home)}
}

// installStandIn writes the installed binary as bytes no rebuild would write: a test in
// which nothing is built compares them.
func (w *selfWorld) installStandIn(t *testing.T) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(w.bin), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.bin, []byte("the panel built at "+w.r+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return readBytes(t, w.bin)
}

// installBuild builds the checkout, at r, into the installed binary, as `clauductor
// panel install` would, and returns its stamp: the panel's own build information.
func (w *selfWorld) installBuild(t *testing.T, goPath string) install.BuildStamp {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(w.bin), 0o700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(goPath, "build", "-o", w.bin, "./cmd/clauductor")
	cmd.Dir = filepath.Join(w.root, "framework")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v %s", err, b)
	}
	s, err := install.FileStamp(w.bin)
	if err != nil || s.Module != selfMod || s.Revision != w.r || s.Modified {
		t.Fatalf("the installed build's stamp: %+v %v, want %s at %s", s, err, selfMod, w.r)
	}
	return s
}

// sideCommit is a commit of the repository on a branch of its own: not an ancestor of
// anything main merges.
func (w *selfWorld) sideCommit(t *testing.T) string {
	t.Helper()
	gitRun(t, w.root, "checkout", "-q", "-b", "side")
	writeFile(t, filepath.Join(w.root, "framework", "internal", "panel", "side.go"), "package panel\n")
	gitRun(t, w.root, "add", "-A")
	gitRun(t, w.root, "commit", "-q", "-m", "side")
	side := w.head(t)
	gitRun(t, w.root, "checkout", "-q", "main")
	return side
}

// unchanged fails the test unless the installed binary is still want and no new binary
// stands beside it.
func (w *selfWorld) unchanged(t *testing.T, want string) {
	t.Helper()
	if got := readBytes(t, w.bin); got != want {
		t.Fatalf("the installed binary changed: %d bytes, was %d", len(got), len(want))
	}
	gone(t, w.bin+".new")
	gone(t, w.bin+".prev")
}

// goStandIn is a go that records each run in runs and fails: a test in which nothing is
// built counts its runs.
func goStandIn(t *testing.T) (prog, runs string) {
	t.Helper()
	dir := t.TempDir()
	prog, runs = filepath.Join(dir, "go"), filepath.Join(dir, "runs")
	testbin.Write(t, prog, "#!/bin/sh\necho \"$*\" >> '"+runs+"'\necho 'go stand-in: not to be run' >&2\nexit 1\n")
	return prog, runs
}

// goRan counts the stand-in's runs.
func goRan(t *testing.T, runs string) int {
	t.Helper()
	b, err := os.ReadFile(runs)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(b), "\n")
}

func readBytes(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func gone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s is there (%v)", path, err)
	}
}

// selfCatchUp is the catch-up source alone on w (newCatchUp), with the rebuild on env
// wired as Run wires it, the gate's wait answering at once and each restart recorded.
func (w *selfWorld) selfCatchUp(clk clock.Clock, env rebuildEnv, launchd bool, notices *noticeLog, log *cuLog, restarts *[]*RestartError) *catchUp {
	c := w.newCatchUp(clk, notices, log)
	s := newSelfUpdater(Options{Home: w.home, Launchd: launchd, Out: log, Runner: w.run, rebuild: &env}, clk, c,
		func(context.Context, time.Duration) bool { return true },
		func(r *RestartError) { *restarts = append(*restarts, r) })
	c.selfUpdate, c.atStart = s.selfUpdate, s.announce
	return c
}

// [CATCHUP-4-S3] A running panel of the project's module that cannot be rebuilt from
// its source: in turn one with no VCS build information, one built with local changes,
// and one built at a commit that is not an ancestor of the merge. Catch-up still moves
// the checkout to the merge, but nothing is built, the installed binary is as it was,
// and one notice says why and how to update the panel.
func TestRebuildLeavesABinaryItCannotRebuild(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, why string
		self      func(w *selfWorld, side string) install.BuildStamp
	}{
		{"no VCS build information", "carries no VCS build information",
			func(*selfWorld, string) install.BuildStamp { return install.BuildStamp{Module: selfMod} }},
		{"built with local changes", "built from a checkout with local changes",
			func(w *selfWorld, _ string) install.BuildStamp {
				return install.BuildStamp{Module: selfMod, Revision: w.r, Modified: true}
			}},
		{"built on another branch", "is not an ancestor of",
			func(_ *selfWorld, side string) install.BuildStamp {
				return install.BuildStamp{Module: selfMod, Revision: side}
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newSelfWorld(t, "")
			w.catchUpOn(t, true)
			side := w.sideCommit(t)
			was := w.installStandIn(t)
			goProg, runs := goStandIn(t)
			b := w.merged(t, 7, "owner", "framework/internal/panel/server.go", serverTwo)
			clk := clock.NewFake(cuT0)
			var notices noticeLog
			var log cuLog
			var restarts []*RestartError
			c := w.selfCatchUp(clk, rebuildEnv{self: tc.self(w, side), exe: w.bin, goCmd: goProg, timeout: time.Minute},
				true, &notices, &log, &restarts)

			trigger(c, clk)
			if got := w.head(t); got != b {
				t.Fatalf("HEAD is %s, want the merge %s: the rebuild's refusal is not catch-up's\n%s", got, b, log.String())
			}
			if n := goRan(t, runs); n != 0 || len(restarts) != 0 {
				t.Fatalf("go ran %d times and the panel restarted %d times, want neither", n, len(restarts))
			}
			w.unchanged(t, was)
			w.oneNotice(t, &notices, short(b), "#7", "did not rebuild itself", tc.why, "keeps running as it was",
				"To update it, run `go build -o clauductor ./cmd/clauductor && ./clauductor panel install` in framework/.")
		})
	}
}

// [CATCHUP-4-S9] An untracked file in the main checkout after the fast-forward to a
// merge that changes framework/: nothing is built, the installed binary is as it was,
// and one notice names the file and says the build would be stamped as modified.
func TestRebuildRefusesACheckoutWithAnUntrackedFile(t *testing.T) {
	t.Parallel()
	w := newSelfWorld(t, "")
	w.catchUpOn(t, true)
	was := w.installStandIn(t)
	goProg, runs := goStandIn(t)
	writeFile(t, filepath.Join(w.root, "notes.txt"), "mine\n")
	b := w.merged(t, 7, "owner", "framework/internal/panel/server.go", serverTwo)
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	var log cuLog
	var restarts []*RestartError
	c := w.selfCatchUp(clk, rebuildEnv{self: install.BuildStamp{Module: selfMod, Revision: w.r}, exe: w.bin, goCmd: goProg,
		timeout: time.Minute}, true, &notices, &log, &restarts)

	trigger(c, clk)
	if got := w.head(t); got != b {
		t.Fatalf("HEAD is %s, want the merge %s\n%s", got, b, log.String())
	}
	if n := goRan(t, runs); n != 0 || len(restarts) != 0 {
		t.Fatalf("go ran %d times and the panel restarted %d times, want neither", n, len(restarts))
	}
	w.unchanged(t, was)
	w.oneNotice(t, &notices, "notes.txt (untracked)", "a build would be stamped as modified", "the next merge into framework/ then rebuilds")
}

// [CATCHUP-4-S10] A panel running as the login agent, and a project with catch-up on
// whose framework/go.mod declares a module other than the panel's: its merge into
// framework/ is fast-forwarded, nothing is built, the installed binary is as it was, and
// no notice about the panel's update goes out.
func TestRebuildIgnoresAnotherModulesFramework(t *testing.T) {
	t.Parallel()
	w := newSelfWorld(t, "")
	w.catchUpOn(t, true)
	was := w.installStandIn(t)
	goProg, runs := goStandIn(t)
	b := w.merged(t, 7, "owner", "framework/internal/panel/server.go", serverTwo)
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	var log cuLog
	var restarts []*RestartError
	c := w.selfCatchUp(clk, rebuildEnv{self: install.BuildStamp{Module: "example.com/thepanel", Revision: w.r}, exe: w.bin,
		goCmd: goProg, timeout: time.Minute}, true, &notices, &log, &restarts)

	trigger(c, clk)
	if got := w.head(t); got != b {
		t.Fatalf("HEAD is %s, want the merge %s\n%s", got, b, log.String())
	}
	if n := goRan(t, runs); n != 0 || len(restarts) != 0 {
		t.Fatalf("go ran %d times and the panel restarted %d times, want neither", n, len(restarts))
	}
	w.unchanged(t, was)
	if ns := catchUpNotices(&notices); len(ns) != 0 {
		t.Fatalf("another module's merge sent %d notices: %+v", len(ns), ns)
	}
	if strings.Contains(log.String(), "framework/ changed") {
		t.Fatalf("the panel log took another module's framework/ for its own:\n%s", log.String())
	}
}

// [CATCHUP-4-S6] A panel built at R; #7 changes install/launchd.go while an untracked
// file stands in the checkout, so it does not rebuild; with the file gone, #8, which
// changes only server.go, rebuilds the panel at B, and its restart notice says to run
// `clauductor panel install` once: the install step is read over R..B, the build's
// range, not over the last merge's.
func TestRestartNoticeNamesInstallOverTheBuildsRange(t *testing.T) {
	t.Parallel()
	goPath := needGo(t)
	w := newSelfWorld(t, "")
	w.catchUpOn(t, true)
	self := w.installBuild(t, goPath)
	writeFile(t, filepath.Join(w.root, "notes.txt"), "mine\n")
	w.merged(t, 7, "owner", "framework/internal/panel/install/launchd.go", "package install\n\n// two\n")
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	var log cuLog
	var restarts []*RestartError
	c := w.selfCatchUp(clk, rebuildEnv{self: self, exe: w.bin, goCmd: goPath, timeout: 2 * time.Minute}, true, &notices, &log, &restarts)

	trigger(c, clk)
	if len(restarts) != 0 {
		t.Fatalf("the panel restarted with an untracked file in the checkout\n%s", log.String())
	}
	w.oneNotice(t, &notices, "notes.txt (untracked)")
	if err := os.Remove(filepath.Join(w.root, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	b := w.merged(t, 8, "owner", "framework/internal/panel/server.go", serverTwo)
	trigger(c, clk)
	if len(restarts) != 1 || restarts[0].Code != ExitRestart || restarts[0].From != w.r || restarts[0].To != b {
		t.Fatalf("restarts %+v, want one with exit %d from R %s to B %s\n%s", restarts, ExitRestart, w.r, b, log.String())
	}
	ns := catchUpNotices(&notices)
	if len(ns) != 2 {
		t.Fatalf("catch-up sent %d notices, want the untracked file's and the restart's: %+v", len(ns), ns)
	}
	for _, s := range []string{short(w.r), short(b), "#8", "run `clauductor panel install` once", "install/launchd.go"} {
		if !strings.Contains(ns[1].Body, s) {
			t.Fatalf("the restart notice %q does not name %q", ns[1].Body, s)
		}
	}
}

// The restart writes its marker, then waits for the panel's claude calls at the gate
// (up to 10 minutes), and only then sends its notice, records the browser open and ends
// the run; a wait that runs out restarts all the same, and says so in the log.
func TestRestartWaitsForTheClaudeCallsAfterItsMarker(t *testing.T) {
	t.Parallel()
	for _, quiet := range []bool{true, false} {
		home := t.TempDir()
		bin := install.InstalledBinary(home)
		if err := os.MkdirAll(filepath.Dir(bin), 0o700); err != nil {
			t.Fatal(err)
		}
		writeFile(t, bin, "R\n")
		writeFile(t, bin+".new", "B\n")
		from, to := strings.Repeat("a", 40), strings.Repeat("b", 40)
		rb := &install.Rebuild{Home: home, Root: "/r", From: from, To: to}
		clk := clock.NewFake(cuT0)
		var notices noticeLog
		var log cuLog
		c := newCatchUp(Options{Home: home, Out: &log, Notify: notices.notify}, clk, newPushWatch(home).current, nil)
		var restarts []*RestartError
		opened := filepath.Join(config.PanelDir(home), "browser-opened")
		type seen struct {
			marker, opened bool
			notices, ends  int
			max            time.Duration
		}
		var atWait *seen
		s := newSelfUpdater(Options{Home: home, Launchd: true, Out: &log, rebuild: &rebuildEnv{}}, clk, c,
			func(_ context.Context, max time.Duration) bool {
				_, merr := os.Stat(install.RestartMarkerPath(home))
				_, oerr := os.Stat(opened)
				atWait = &seen{marker: merr == nil, opened: oerr == nil, notices: len(catchUpNotices(&notices)), ends: len(restarts), max: max}
				return quiet
			},
			func(r *RestartError) { restarts = append(restarts, r) })

		s.swap(context.Background(), rb, catchUpTarget{root: "/r"}, catchUpResult{from: from, to: to, prs: []int{7}},
			install.Plan{Ours: true, Base: from, Changed: true}, nil)
		if atWait == nil || !atWait.marker || atWait.opened || atWait.notices != 0 || atWait.ends != 0 || atWait.max != 10*time.Minute {
			t.Fatalf("quiet %v: at the gate's wait %+v; want the marker written, and no browser record, notice or end yet, for up to 10 minutes", quiet, atWait)
		}
		rec, _ := os.ReadFile(opened)
		if len(restarts) != 1 || restarts[0].Code != ExitRestart || len(catchUpNotices(&notices)) != 1 || strings.TrimSpace(string(rec)) != fmt.Sprint(clk.Now().Unix()) {
			t.Fatalf("quiet %v: after the wait, restarts %+v, %d notices, browser open recorded %q; want one of each, the open at %d",
				quiet, restarts, len(catchUpNotices(&notices)), rec, clk.Now().Unix())
		}
		if late := strings.Contains(log.String(), "restarting anyway"); late == quiet {
			t.Fatalf("quiet %v: the log says the wait ran out: %v\n%s", quiet, late, log.String())
		}
	}
}

// A stop during the restart's wait for the claude calls (a logout, `launchctl bootout`,
// `clauductor panel install` reloading the agent) ends the run's context after the
// binary replaced itself: the notice with the rollback still goes, though the run's
// context has ended, and the restart goes no further (no browser record, no exit 75),
// leaving the marker for the new binary's first start.
func TestRestartStoppedDuringItsWaitStillSendsTheRollback(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	bin := install.InstalledBinary(home)
	if err := os.MkdirAll(filepath.Dir(bin), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, bin, "R\n")
	writeFile(t, bin+".new", "B\n")
	from, to := strings.Repeat("a", 40), strings.Repeat("b", 40)
	rb := &install.Rebuild{Home: home, Root: "/r", From: from, To: to}
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	var log cuLog
	c := newCatchUp(Options{Home: home, Out: &log, Notify: notices.notify}, clk, newPushWatch(home).current, nil)
	// As osascript under exec.CommandContext: nothing shows under an ended context.
	c.notify = func(ctx context.Context, n state.Notice) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return notices.notify(n)
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	var restarts []*RestartError
	s := newSelfUpdater(Options{Home: home, Launchd: true, Out: &log, rebuild: &rebuildEnv{}}, clk, c,
		func(ctx context.Context, _ time.Duration) bool { stop(); <-ctx.Done(); return false },
		func(r *RestartError) { restarts = append(restarts, r) })

	s.swap(ctx, rb, catchUpTarget{root: "/r"}, catchUpResult{from: from, to: to, prs: []int{7}},
		install.Plan{Ours: true, Base: from, Changed: true}, nil)
	if got := readBytes(t, bin); got != "B\n" {
		t.Fatalf("the installed binary is %q, want the new build", got)
	}
	ns := catchUpNotices(&notices)
	if len(ns) != 1 {
		t.Fatalf("a stop during the wait sent %d notices, want the one with the rollback\n%s", len(ns), log.String())
	}
	for _, want := range []string{short(from), short(to), "stopped before its restart",
		"`mv ~/.clauductor/panel/bin/clauductor.prev ~/.clauductor/panel/bin/clauductor`",
		"`launchctl kickstart -k gui/$(id -u)/" + install.LaunchdLabel + "`", "#7"} {
		if !strings.Contains(ns[0].Body, want) {
			t.Fatalf("the notice %q does not name %q", ns[0].Body, want)
		}
	}
	if strings.Contains(ns[0].Body, "restarts now") {
		t.Fatalf("the notice %q says the panel restarts now, after a stop", ns[0].Body)
	}
	if len(restarts) != 0 {
		t.Fatalf("a stopped run restarted: %+v", restarts)
	}
	gone(t, filepath.Join(config.PanelDir(home), "browser-opened"))
	if _, err := os.Stat(install.RestartMarkerPath(home)); err != nil {
		t.Fatalf("the restart marker is gone after a stop: %v", err)
	}
}

// endedShowsNothing is a catch-up notification sender that, as osascript under
// exec.CommandContext, shows nothing under an ended context, and records the rest.
func endedShowsNothing(l *noticeLog) func(context.Context, state.Notice) error {
	return func(ctx context.Context, n state.Notice) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return l.notify(n)
	}
}

// stoppedRebuildSays are what the notice of a rebuild a stop ended names, beyond the
// merge: what the next start runs, and how the change still comes.
var stoppedRebuildSays = []string{"#7", "Its next start runs the binary it ran", "catch-up's next fast-forward rebuilds it, this change included",
	"To update it, run `go build -o clauductor ./cmd/clauductor && ./clauductor panel install` in framework/."}

// A stop (a logout, `launchctl bootout`) while the rebuild builds: the build ends, no
// .new is left, the installed binary is as it was, nothing restarts and no marker is
// written, and one notice says the panel was stopped during its rebuild, what its next
// start runs, and how the change still comes; it goes although the stop ended the
// run's context. Review 2026-10-23 found such a stop dropped the rebuild with no word:
// the fast-forward was done, so the next start had nothing to arrive. A test of a stop
// cites no scenario: none has one in its GIVEN.
func TestRebuildStoppedDuringItsBuildSaysSo(t *testing.T) {
	t.Parallel()
	w := newSelfWorld(t, "")
	w.catchUpOn(t, true)
	was := w.installStandIn(t)
	dir := t.TempDir()
	goProg, started := filepath.Join(dir, "go"), filepath.Join(dir, "started")
	// A build that runs until the stop ends it.
	testbin.Write(t, goProg, "#!/bin/sh\n: > '"+started+"'\nexec sleep 60\n")
	b := w.merged(t, 7, "owner", "framework/internal/panel/server.go", serverTwo)
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	var log cuLog
	var restarts []*RestartError
	c := w.selfCatchUp(clk, rebuildEnv{self: install.BuildStamp{Module: selfMod, Revision: w.r}, exe: w.bin, goCmd: goProg,
		timeout: time.Minute}, true, &notices, &log, &restarts)
	c.notify = endedShowsNothing(&notices)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.wake(w.root)
		c.poll(ctx, clk.Now())
	}()
	waitFor(t, "the build to start", func() bool { _, err := os.Stat(started); return err == nil })
	stop()
	select {
	case <-done:
	case <-time.After(testwait.Scale(30 * time.Second)):
		t.Fatalf("the poll did not return after the stop\n%s", log.String())
	}
	if got := w.head(t); got != b {
		t.Fatalf("HEAD is %s, want the merge %s\n%s", got, b, log.String())
	}
	w.unchanged(t, was)
	if len(restarts) != 0 {
		t.Fatalf("a stopped rebuild restarted: %+v", restarts)
	}
	gone(t, install.RestartMarkerPath(w.home))
	w.oneNotice(t, &notices, append([]string{short(b), "was stopped during its rebuild"}, stoppedRebuildSays...)...)
}

// A stop as the fast-forward's merge runs, or right after it, before anything follows
// it: the merge completes, and what follows it still runs, since the next start finds
// nothing to arrive. Nothing is built, and the one notice says the panel was stopped
// during its rebuild, what its next start runs, and how the change still comes. Review
// 2026-10-23 found catch-up returned at the stop with the merge in the checkout, and
// nothing ever said it. A test of a stop cites no scenario: none has one in its GIVEN.
func TestRebuildStoppedAtTheMergeSaysSo(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		during bool
	}{{"during the merge", true}, {"right after the merge", false}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newSelfWorld(t, "")
			w.catchUpOn(t, true)
			was := w.installStandIn(t)
			goProg, runs := goStandIn(t)
			b := w.merged(t, 7, "owner", "framework/internal/panel/server.go", serverTwo)
			clk := clock.NewFake(cuT0)
			var notices noticeLog
			var log cuLog
			var restarts []*RestartError
			c := w.selfCatchUp(clk, rebuildEnv{self: install.BuildStamp{Module: selfMod, Revision: w.r}, exe: w.bin, goCmd: goProg,
				timeout: time.Minute}, true, &notices, &log, &restarts)
			c.notify = endedShowsNothing(&notices)
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			run := c.run
			c.run = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
				merge := slices.Contains(argv, "merge") && slices.Contains(argv, "--ff-only")
				if merge && tc.during {
					stop()
				}
				out, err := run(ctx, dir, argv)
				if merge && !tc.during && err == nil {
					stop()
				}
				return out, err
			}

			c.wake(w.root)
			c.poll(ctx, clk.Now())
			if got := w.head(t); got != b {
				t.Fatalf("HEAD is %s, want the merge %s\n%s", got, b, log.String())
			}
			if n := goRan(t, runs); n != 0 || len(restarts) != 0 {
				t.Fatalf("go ran %d times and the panel restarted %d times after the stop, want neither", n, len(restarts))
			}
			w.unchanged(t, was)
			w.oneNotice(t, &notices, append([]string{short(b), "was stopped during its rebuild"}, stoppedRebuildSays...)...)
		})
	}
}

// A swap whose rename into place fails hands the changed config to the running binary
// (notInPlace), and its notice says what became of it: the binary that keeps running
// never judged it. A test of the part cites no scenario:
// TestRebuildSwapThatFailsServesTheConfigHere drives it through a panel.
func TestRebuildSwapThatFailsWithdrawsTheTrust(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	bin := install.InstalledBinary(home)
	if err := os.MkdirAll(filepath.Dir(bin), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, bin, "R\n") // and no .new: the rename into place fails
	from, to := strings.Repeat("a", 40), strings.Repeat("b", 40)
	rb := &install.Rebuild{Home: home, Root: "/r", From: from, To: to}
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	var log cuLog
	c := newCatchUp(Options{Home: home, Out: &log, Notify: notices.notify}, clk, newPushWatch(home).current, nil)
	var restarts []*RestartError
	s := newSelfUpdater(Options{Home: home, Launchd: true, Out: &log, rebuild: &rebuildEnv{}}, clk, c,
		func(context.Context, time.Duration) bool { return true },
		func(r *RestartError) { restarts = append(restarts, r) })
	withdrew := 0
	s.swap(context.Background(), rb, catchUpTarget{root: "/r"}, catchUpResult{from: from, to: to, prs: []int{7}},
		install.Plan{Ours: true, Base: from, Changed: true}, func(context.Context) string { withdrew++; return " The trust is withdrawn." })
	if got := readBytes(t, bin); got != "R\n" || withdrew != 1 || len(restarts) != 0 {
		t.Fatalf("after a failed swap: the binary %q, %d withdrawals, %d restarts; want R, one, none\n%s", got, withdrew, len(restarts), log.String())
	}
	ns := catchUpNotices(&notices)
	if len(ns) != 1 || !strings.Contains(ns[0].Body, "could not put it in place") || !strings.Contains(ns[0].Body, "The trust is withdrawn.") {
		t.Fatalf("the notices %+v, want one saying the build was not put in place and the trust withdrawn", ns)
	}
	gone(t, install.RestartMarkerPath(home))
}

// A notice the swap sends goes as a notification, then a push, and each keeps its own
// bound (showNotice's 10 s, then pushTimeout): a notification as slow as its sender lets
// it be leaves the push of the rollback its whole timeout. Review 2026-10-23 found one
// 10 s deadline around both, so an osascript that took it all started the push, the only
// record of the swap off the Mac, under an expired context, and it never ran.
func TestRestartNoticeIsPushedAfterASlowNotification(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	bin := install.InstalledBinary(home)
	if err := os.MkdirAll(filepath.Dir(bin), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, bin, "R\n")
	writeFile(t, bin+".new", "B\n")
	from, to := strings.Repeat("a", 40), strings.Repeat("b", 40)
	rb := &install.Rebuild{Home: home, Root: "/r", From: from, To: to}
	clk := clock.NewFake(cuT0)
	var notices noticeLog
	var log cuLog
	prog, runs := pushStandIn(t, "")
	c := newCatchUp(Options{Home: home, Out: &log, Notify: notices.notify}, clk,
		func() pushCmd { return pushCmd{argv: []string{prog}} }, nil)
	// As osascript at its slowest: it runs until the context its sender gave it ends, if
	// that context has an end, and shows the notice then.
	c.notify = func(ctx context.Context, n state.Notice) error {
		if _, ok := ctx.Deadline(); ok {
			<-ctx.Done()
		}
		return notices.notify(n)
	}
	var restarts []*RestartError
	s := newSelfUpdater(Options{Home: home, Launchd: true, Out: &log, rebuild: &rebuildEnv{}}, clk, c,
		func(context.Context, time.Duration) bool { return true },
		func(r *RestartError) { restarts = append(restarts, r) })

	s.swap(context.Background(), rb, catchUpTarget{root: "/r"}, catchUpResult{from: from, to: to, prs: []int{7}},
		install.Plan{Ours: true, Base: from, Changed: true}, nil)
	if len(catchUpNotices(&notices)) != 1 || len(restarts) != 1 {
		t.Fatalf("%d notices, restarts %+v: want the restart notice, then the restart\n%s", len(catchUpNotices(&notices)), restarts, log.String())
	}
	got := pushRuns(t, runs)
	rollback := "`mv ~/.clauductor/panel/bin/clauductor.prev ~/.clauductor/panel/bin/clauductor`"
	if len(got) != 1 || len(got[0]) != 2 || got[0][0] != catchUpTitle || !strings.Contains(got[0][1], rollback) {
		t.Fatalf("pushes %q after a slow notification: want the restart notice, titled %q, with its rollback\n%s", got, catchUpTitle, log.String())
	}
}

// openLog records the browser opens a panel asks for (Options.OpenBrowser): no test
// opens the owner's browser.
type openLog struct {
	mu   sync.Mutex
	urls []string
}

func (l *openLog) open(u string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.urls = append(l.urls, u)
}

func (l *openLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.urls)
}

// startSelfPanel is a full panel on w (startCatchUpPanel's), as the login agent when
// launchd, its rebuild on env and every browser open recorded in opened.
func (w *selfWorld) startSelfPanel(t *testing.T, sock string, launchd bool, env rebuildEnv, notices *noticeLog, log *cuLog, opened *openLog) *panelRun {
	t.Helper()
	return startPanelWith(t, w.root, w.home, sock, func(o *Options) {
		o.Runner, o.Notify, o.Out = w.run, notices.notify, log
		o.Ticks.PRs = 50 * time.Millisecond
		o.Launchd, o.OpenBrowser, o.rebuild = launchd, opened.open, &env
	})
}

// restartOf waits for p's run to end with the restart, and hands its return back to
// p.done for p.stop.
func restartOf(t *testing.T, p *panelRun, log *cuLog) *RestartError {
	t.Helper()
	select {
	case err := <-p.done:
		p.done <- err
		var rs *RestartError
		if !errors.As(err, &rs) {
			t.Fatalf("the panel stopped with %v, want the restart\n%s", err, log.String())
		}
		return rs
	case <-time.After(testwait.Scale(2 * time.Minute)):
		t.Fatalf("the panel did not restart\n%s", log.String())
	}
	return nil
}

// partialGo is goPath behind a go that first writes a partial output: a build that
// fails leaves a clauductor.new unless the rebuild removes it.
func partialGo(t *testing.T, goPath string) string {
	t.Helper()
	prog := filepath.Join(t.TempDir(), "go")
	testbin.Write(t, prog, "#!/bin/sh\nprintf partial > \"$3\"\nexec '"+goPath+"' \"$@\"\n")
	return prog
}

// tildeOf is p under w's home written from ~, as the rollback the owner pastes.
func (w *selfWorld) tildeOf(p string) string { return "~" + strings.TrimPrefix(p, w.home) }

// D1 (e) in D4's preflight, on the real self-update and a full panel: the merge #8
// changes framework/ and the panel's own project's panel.json, which in turn the new
// binary's config-check refuses, and which would serve the project on the tmux socket
// another served project (Alpha, the panel's --project) holds. Either is the preflight's
// failure: no clauductor.new is left, nothing is swapped and the panel keeps running, one
// notice names the preflight's error, and the running binary, judging the config, says
// it does not load. (Review 2026-10-23: config-check judged the parse alone, and only a
// stand-in of selfUpdate's discard was tested.)
func TestRebuildPreflightJudgesTheConfigItWillServe(t *testing.T) {
	t.Parallel()
	goPath := needGo(t)
	for _, tc := range []struct {
		name  string
		taken bool // #8's config names Alpha's socket; else it does not parse
	}{{"config-check refuses", false}, {"socket taken", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, sock := throwawaySocket(t)
			cfg, says := `{"name":"T","lanes":`, "panel config-check" // says: in the preflight's notice
			if tc.taken {
				cfg = `{"name":"T","lanes":{"main":"orchestrator"},"tmux_socket":"` + sock + `"}`
				says = fmt.Sprintf("its tmux socket %q is alpha's", sock)
			}
			w := newSelfWorld(t, `{"name":"T","lanes":{"main":"orchestrator"},"tmux_socket":"`+noServerSocket()+`"}`)
			w.catchUpOn(t, true)
			self := w.installBuild(t, goPath)
			was := readBytes(t, w.bin)
			alpha := multiRepo(t, "Alpha", noServerSocket()) // served on sock (TmuxSocket)
			res, err := install.AddProject(context.Background(), install.AddOptions{Home: w.home, Project: w.root, Run: gitOnlyRunner,
				Now: clock.System.Now()})
			if err != nil {
				t.Fatal(err)
			}
			w.setOpen(7, 8)
			var notices noticeLog
			var log cuLog
			var opened openLog
			p := startPanelWith(t, alpha, w.home, sock, func(o *Options) {
				o.Runner, o.Notify, o.Out = w.run, notices.notify, &log
				o.Ticks.PRs = 50 * time.Millisecond
				o.Launchd, o.OpenBrowser = true, opened.open
				o.rebuild = &rebuildEnv{self: self, exe: w.bin, goCmd: goPath, timeout: 2 * time.Minute}
			})
			p.polls.until(t, "catch-up", 1)
			p.polls.until(t, res.Entry.ID+"/prs", 1)
			w.merged(t, 7, "owner", "framework/internal/panel/server.go", serverTwo)
			b := w.merged(t, 8, "owner", config.DefaultConfigRel, cfg)
			w.setOpen()

			testwait.For(t, "the preflight's notice and the config's", 2*time.Minute, func() bool { return len(catchUpNotices(&notices)) >= 2 })
			running(t, p)
			if got := w.head(t); got != b {
				t.Fatalf("HEAD is %s, want the merge %s", got, b)
			}
			w.unchanged(t, was)
			ns := catchUpNotices(&notices)
			if len(ns) != 2 {
				t.Fatalf("catch-up sent %d notices, want the preflight's and the config's: %+v\n%s", len(ns), ns, log.String())
			}
			for _, s := range []string{w.root, "did not rebuild itself", "the new binary's preflight failed", says} {
				if !strings.Contains(ns[0].Body, s) {
					t.Fatalf("the rebuild's notice %q does not name %q", ns[0].Body, s)
				}
			}
			if !strings.Contains(ns[1].Body, "does not load") {
				t.Fatalf("the config's notice %q does not say it does not load", ns[1].Body)
			}
		})
	}
}

// A swap that cannot put the rebuilt binary in place (here clauductor.prev is a
// directory, so the rename of the installed binary fails), after the owner's #7 changed
// framework/ and #8 the project's panel.json: the running binary takes the config over
// as after a failed build (D1). Each time the panel keeps running on its own binary, no
// clauductor.new is left, the new bytes are not trusted and the owner's trust of the
// config the fast-forward started from stands. When the new binary's reading of the
// config was trusted on arrival and this binary loads it, the project is served on it
// untrusted, with Trust config… offered; when this binary cannot load it, or catch-up
// could not read it, the project keeps the config it had loaded. The swap's notice says
// which. Review 2026-10-23 found a failed swap withdrew the trust and left the config
// from before the merge served trusted, with Trust config… not offered. It cites no
// scenario: none has a failed swap in its GIVEN.
func TestRebuildSwapThatFailsServesTheConfigHere(t *testing.T) {
	t.Parallel()
	goPath := needGo(t)
	_, perr := config.ParseConfig([]byte(badCfg))
	old, cfg := cardCfg("T", true, ".claude/y.sh"), cardCfg("T", true, ".claude/x.sh")
	for _, tc := range []struct {
		name, cfg string
		unread    bool     // catch-up cannot read the config's blobs
		first     string   // the arrival's notice, first, names it
		serves    string   // the config the project is served on after the swap
		says      []string // the swap's notice
	}{
		{"loads", cfg, false, "trusted the new", cfg,
			[]string{"is withdrawn", "the panel serves it untrusted", "waits for the owner"}},
		{"does not load", badCfg, false, "trusted the new", old,
			[]string{"is withdrawn", "the running panel cannot load it", "keeps serving the config it had loaded", perr.Error()}},
		{"unread", cfg, true, "the rebuilt panel serves the file", old,
			[]string{"As its " + config.DefaultConfigRel + " could not be read", "keeps serving the config it had loaded"}},
	} {
		// Serially: one more panel at a time on the package's parallel load (OPS-122).
		t.Run(tc.name, func(t *testing.T) {
			_, sock := throwawaySocket(t)
			w := newSelfWorld(t, old)
			w.catchUpOn(t, true)
			if _, err := install.TrustConfig(w.home, w.root, ""); err != nil {
				t.Fatal(err)
			}
			self := w.installBuild(t, goPath)
			was := readBytes(t, w.bin)
			writeFile(t, filepath.Join(w.bin+".prev", "taken"), "a directory where the old binary goes\n")
			w.setOpen(7, 8)
			var notices noticeLog
			var log cuLog
			var opened openLog
			p := startPanelWith(t, w.root, w.home, sock, func(o *Options) {
				o.Runner, o.Notify, o.Out = w.run, notices.notify, &log
				if tc.unread {
					o.Runner = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
						if slices.Contains(argv, "ls-tree") {
							return nil, errors.New("fatal: not a tree object")
						}
						return w.run(ctx, dir, argv)
					}
				}
				o.Ticks.PRs = 50 * time.Millisecond
				o.Launchd, o.OpenBrowser = true, opened.open
				o.rebuild = &rebuildEnv{self: self, exe: w.bin, goCmd: goPath, timeout: 2 * time.Minute}
			})
			p.polls.until(t, "catch-up", 1)
			p.polls.until(t, "prs", 1)
			w.merged(t, 7, "owner", "framework/internal/panel/server.go", serverTwo)
			b := w.merged(t, 8, "owner", config.DefaultConfigRel, tc.cfg)
			w.setOpen()

			testwait.For(t, "the arrival's notice and the swap's", 2*time.Minute, func() bool { return len(catchUpNotices(&notices)) >= 2 })
			running(t, p)
			if got := w.head(t); got != b {
				t.Fatalf("HEAD is %s, want the merge %s", got, b)
			}
			if readBytes(t, w.bin) != was {
				t.Fatal("the installed binary changed")
			}
			gone(t, w.bin+".new")
			cfgPath := filepath.Join(w.root, config.DefaultConfigRel)
			if install.TrustedNow(w.home, w.root, cfgPath, install.ConfigHash([]byte(tc.cfg))) {
				t.Fatalf("the new bytes are trusted after a failed swap\n%s", log.String())
			}
			if !install.TrustedNow(w.home, w.root, cfgPath, install.ConfigHash([]byte(old))) {
				t.Fatal("the owner's trust of the config the fast-forward started from is gone")
			}
			// The page offers Trust config… while the served config is not trusted.
			tv := served(t, p, tc.serves)
			if tv.Trusted != (tc.serves == old) {
				t.Fatalf("the project serves %+v, want it trusted only on the config it had loaded\n%s", tv, log.String())
			}
			if rep := trustReportOf(t, p); tc.serves == tc.cfg && rep.Trusted {
				t.Fatalf("Trust config… is not offered: %+v", rep)
			}
			ns := catchUpNotices(&notices)
			if len(ns) != 2 {
				t.Fatalf("catch-up sent %d notices, want the arrival's and the swap's: %+v\n%s", len(ns), ns, log.String())
			}
			if !strings.Contains(ns[0].Body, tc.first) {
				t.Fatalf("the arrival's notice %q does not name %q", ns[0].Body, tc.first)
			}
			noticeSays(t, ns[1], catchUpTitle, append([]string{w.root, short(b), "could not put it in place", "keeps running as it was"}, tc.says...)...)
		})
	}
}
