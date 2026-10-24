package claudecall

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/testbin"
	"github.com/clauductor/clauductor/internal/testwait"
)

// fakeProgram puts an executable sh script named name first on PATH, for this test
// alone, and checks PATH now finds it: no test here may reach a real claude.
func fakeProgram(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	testbin.Write(t, path, "#!/bin/sh\n"+script)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if got, err := exec.LookPath(name); err != nil || got != path {
		t.Fatalf("%s on PATH is %q (%v), not the fake", name, got, err)
	}
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// newGate is a gate over inner whose log is in a temp home; its cleanup closes it,
// so no process outlives the test.
func newGate(t *testing.T, inner signals.Runner, r Reader, clk clock.Clock) (*Gate, string) {
	t.Helper()
	home := t.TempDir()
	g := NewGate(inner, r, clk, NewLog(home))
	t.Cleanup(func() { g.Close(ShutdownGrace) })
	return g, home
}

// calls is the gate's history, copied.
func (g *Gate) calls() []call {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []call
	for _, c := range g.history {
		out = append(out, *c)
	}
	return out
}

// waitCalls waits until the history holds n calls, the last of them in state ok.
func waitCalls(t *testing.T, g *Gate, what string, n int, ok func(call) bool) []call {
	t.Helper()
	deadline := time.Now().Add(testwait.Scale(30 * time.Second))
	for {
		cs := g.calls()
		if len(cs) >= n && ok(cs[n-1]) {
			return cs
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: history %+v", what, cs)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func running(c call) bool { return c.ended.IsZero() }
func ended(c call) bool   { return !c.ended.IsZero() }

func stopLines(t *testing.T, home string) []StopLine {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(config.PanelDir(home), LogName))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []StopLine
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		var l StopLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatalf("line %q: %v", sc.Text(), err)
		}
		if l.Kind == "stopped" {
			out = append(out, l)
		}
	}
	return out
}

func readMarks(t *testing.T, path string) string {
	t.Helper()
	b, _ := os.ReadFile(path)
	return string(b)
}

var agentsArgv = []string{"claude", "agents", "--json"}

func TestClaudeArgsMatchesClaudeDirectlyOrAfterTheEnvScrub(t *testing.T) {
	for _, tc := range []struct {
		argv []string
		args []string
		ok   bool
	}{
		{[]string{"claude", "agents", "--json"}, []string{"agents", "--json"}, true},
		{[]string{"/Users/me/.local/bin/claude", "--version"}, []string{"--version"}, true},
		{[]string{"/usr/bin/env", "-u", "ANTHROPIC_API_KEY", "-u", "CLAUDECODE", "claude", "auth", "status", "--json"}, []string{"auth", "status", "--json"}, true},
		{[]string{"env", "-i", "A=b", "--", "claude"}, []string{}, true},
		{[]string{"env", "--unset", "X", "--chdir", "/tmp", "claude", "-p"}, []string{"-p"}, true},
		{[]string{"env", "--unset=X", "-uY", "--chdir=/tmp", "claude"}, []string{}, true},
		{[]string{"env", "-S", "-u X claude agents", "--json"}, []string{"agents", "--json"}, true},
		{[]string{"env", "--split-string=A=b /x/claude", "auth"}, []string{"auth"}, true},
		{[]string{"env", "-Sclaude --version"}, []string{"--version"}, true},
		{[]string{"env", "-S", "env -i claude"}, []string{}, true},
		{[]string{"env", "-iS", "claude agents"}, []string{"agents"}, true},
		{[]string{"env", "-iu", "X", "claude"}, []string{}, true},
		{[]string{"env", "-iuX", "claude", "-p"}, []string{"-p"}, true},
		{[]string{"env", "-", "claude"}, []string{}, true},
		{[]string{"env", "-S", "'claude' agents"}, []string{"agents"}, true},
		{[]string{"env", "-S", `"/x/claude" "a b" c\ d`}, []string{"a b", "c d"}, true},
		{[]string{"env", "-S", "-iu X 'claude' agents"}, []string{"agents"}, true},
		{[]string{"env", "--uns", "X", "claude"}, []string{}, true},
		{[]string{"env", "--uns=X", "--ch", "/tmp", "claude"}, []string{}, true},
		{[]string{"env", "--spl", "claude --version"}, []string{"--version"}, true},
		{[]string{"env", "-a", "x", "claude"}, []string{}, true},
		{[]string{"env", "--argv0", "x", "claude", "--version"}, []string{"--version"}, true},
		{[]string{"env", "--arg=x", "claude"}, []string{}, true},
		{[]string{"/usr/bin/env", "-P", "/bin", "-L", "me", "claude"}, []string{}, true},
		{[]string{"env", "-a", "claude", "git"}, nil, false},
		{[]string{"env", "--unset", "claude", "git"}, nil, false},
		{[]string{"env", "-S", "git status"}, nil, false},
		{[]string{"git", "status"}, nil, false},
		{[]string{"claudex"}, nil, false},
		{[]string{"/usr/bin/env", "-u", "claude", "git"}, nil, false},
		{[]string{"/usr/bin/env"}, nil, false},
		{nil, nil, false},
	} {
		args, ok := signals.ClaudeArgs(tc.argv)
		if ok != tc.ok || (ok && !slices.Equal(args, tc.args)) {
			t.Errorf("ClaudeArgs(%q) = %q, %v; want %q, %v", tc.argv, args, ok, tc.args, tc.ok)
		}
	}
}

// [CLAUDECALLS-1-S1] A timed-out claude call gets SIGTERM, and its caller does not
// wait: the caller has its timeout error at the timeout while the program still
// runs, and the program receives SIGTERM, not SIGKILL.
// [CLAUDECALLS-4-S3] A stopped claude call is logged: the call, the signals it sent,
// and whether a lock existed then.
func TestGateStopsATimedOutClaudeWithSIGTERM(t *testing.T) {
	_, r := fakeConfig(t)
	makeLock(t, r.CurrentPath(), time.Now().Add(-5*time.Minute)) // stale: stops nothing, but it exists
	marks := filepath.Join(t.TempDir(), "marks")
	fakeProgram(t, "claude", fmt.Sprintf(`trap 'kill $P 2>/dev/null; echo TERM >> %[1]s; sleep 5; exit 0' TERM
echo ready >> %[1]s
sleep 60 & P=$!
wait $P
`, shq(marks)))
	g, home := newGate(t, signals.ExecRunner, r, clock.System)

	err, took := giveUpWhenReady(t, g, context.Background(), agentsArgv, marks)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v, want the caller's own end", err)
	}
	if took > testwait.Scale(time.Second) {
		t.Fatalf("the caller waited %v: it must have its timeout at the timeout, not at the exit", took)
	}
	cs := g.calls()
	if len(cs) != 1 || !running(cs[0]) || syscall.Kill(cs[0].pid, 0) != nil {
		t.Fatalf("the program must still run when the caller returns: %+v", cs)
	}
	cs = waitCalls(t, g, "the program's exit", 1, ended)
	if got := readMarks(t, marks); !strings.Contains(got, "TERM") {
		t.Fatalf("the program got no SIGTERM: marks %q", got)
	}
	if grace := cs[0].ended.Sub(cs[0].stoppedAt); grace >= signals.ClaudeStopGrace {
		t.Fatalf("it exited %v after the stop: it was killed, not left to exit after SIGTERM", grace)
	}
	lines := stopLines(t, home)
	if len(lines) != 1 || lines[0].Call != "claude agents --json" || lines[0].PID != cs[0].pid ||
		!slices.Equal(lines[0].Signals, []string{"SIGTERM"}) || !lines[0].Lock || !slices.Equal(lines[0].Locks, []string{"current stale"}) {
		t.Fatalf("stopped-call lines: %+v", lines)
	}
}

// [CLAUDECALLS-1-S2] A claude call that ignores SIGTERM is killed once 15 s have
// passed since SIGTERM, and not before.
func TestGateKillsAClaudeThatIgnoresSIGTERMAfterTheGrace(t *testing.T) {
	if testing.Short() {
		t.Skip("-short: waits out the real 15 s grace (the full run covers it)")
	}
	marks := filepath.Join(t.TempDir(), "marks")
	fakeProgram(t, "claude", fmt.Sprintf(`trap 'echo TERM >> %[1]s' TERM
echo ready >> %[1]s
while :; do sleep 1; done
`, shq(marks)))
	g, home := newGate(t, signals.ExecRunner, Reader{}, clock.System)
	if err, _ := giveUpWhenReady(t, g, context.Background(), agentsArgv, marks); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v, want the caller's own end", err)
	}
	cs := waitCalls(t, g, "the kill", 1, ended)
	grace := cs[0].ended.Sub(cs[0].stoppedAt)
	if grace < signals.ClaudeStopGrace || grace > signals.ClaudeStopGrace+testwait.Scale(3*time.Second) {
		t.Fatalf("killed %v after SIGTERM, want %v", grace, signals.ClaudeStopGrace)
	}
	if got := readMarks(t, marks); !strings.Contains(got, "TERM") {
		t.Fatalf("SIGTERM never came first: marks %q", got)
	}
	if lines := stopLines(t, home); len(lines) != 1 || !slices.Equal(lines[0].Signals, []string{"SIGTERM", "SIGKILL"}) || lines[0].Lock {
		t.Fatalf("stopped-call lines: %+v", lines)
	}
}

// A caller whose context has already ended never starts a claude: not once in many
// tries, though select picks at random between a free slot and a done context.
func TestGateCancelledCallerStartsNoClaude(t *testing.T) {
	marks := filepath.Join(t.TempDir(), "marks")
	fakeProgram(t, "claude", fmt.Sprintf("echo started >> %s\n", shq(marks)))
	// The inner Runner counts what reaches it: ExecRunner alone would hide a call
	// that got through, when the gate's own context is cancelled before Start.
	var reached atomic.Int32
	inner := func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		reached.Add(1)
		return signals.ExecRunner(ctx, dir, argv)
	}
	g, _ := newGate(t, inner, Reader{}, clock.System)
	for _, mark := range []func(context.Context) context.Context{signals.WithPoll, func(c context.Context) context.Context { return c }} {
		ctx, cancel := context.WithCancel(mark(context.Background()))
		cancel()
		for i := 0; i < 50; i++ {
			if _, err := g.Run(ctx, t.TempDir(), agentsArgv); !errors.Is(err, context.Canceled) {
				t.Fatalf("err %v, want the cancellation", err)
			}
		}
	}
	if got := readMarks(t, marks); got != "" || len(g.calls()) != 0 || reached.Load() != 0 {
		t.Fatalf("a cancelled caller's call got through: %d reached the runner, marks %q, history %+v", reached.Load(), got, g.calls())
	}
}

// A call refused at the panel's shutdown never ran, so it does not fire WithAcquired:
// a poll would otherwise record a call time for it. The call waits for the slot,
// the gate closes, then the slot frees: acquire succeeds, the closed check refuses.
func TestGateCallRefusedAtCloseDoesNotFireAcquired(t *testing.T) {
	h := newHolder()
	g, _ := newGate(t, h.run, Reader{}, clock.System)
	goRun(g, laneCtx(t), []string{"claude", "hold"})
	waitSlotTaken(t, g)
	var fired atomic.Int32
	ctx, cancel := context.WithTimeout(WithAcquired(context.Background(), func() { fired.Add(1) }), time.Minute)
	defer cancel()
	errc := goRun(g, ctx, agentsArgv)
	noResult(t, errc, "the waiting call") // it is in acquire, behind the holder
	closed := make(chan struct{})
	go func() { g.Close(ShutdownGrace); close(closed) }()
	deadline := time.Now().Add(testwait.Scale(5 * time.Second))
	for !g.isClosed() {
		if time.Now().After(deadline) {
			t.Fatal("the gate never closed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(h.release)
	if err := result(t, errc, "the waiting call"); !errors.Is(err, errClosed) {
		t.Fatalf("err %v, want the gate's refusal at close", err)
	}
	<-closed
	if fired.Load() != 0 || h.n.Load() != 1 {
		t.Fatalf("WithAcquired fired %d time(s), %d claude call(s) ran; want 0 and the holder's alone", fired.Load(), h.n.Load())
	}
}

// A call whose process had exited before its caller gave up was sent no signal, so
// it is no stopped call: the log names only signals actually sent.
func TestGateLogsNoSignalItDidNotSend(t *testing.T) {
	// The process exits at once; a child keeps its stdout open, so the call is still
	// running (reading) when its caller gives up. The caller gives up only once the
	// process is gone (reaped: Wait reaps it before it drains the output), never at a
	// fixed time: under load a 500 ms timeout fired before the shell had exited, and
	// the SIGTERM reached it (#177).
	marks := filepath.Join(t.TempDir(), "marks")
	fakeProgram(t, "claude", fmt.Sprintf("sleep 5 &\necho $$ > %s\nexit 0\n", shq(marks)))
	g, home := newGate(t, signals.ExecRunner, Reader{}, clock.System)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := goRun(g, ctx, agentsArgv)
	deadline := time.Now().Add(testwait.Scale(10 * time.Second))
	for {
		pid := strings.TrimSpace(readMarks(t, marks))
		if pid != "" && exec.Command("ps", "-p", pid).Run() != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the claude (pid %q) never exited", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	if err := <-errc; !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v, want the cancel", err)
	}
	cs := waitCalls(t, g, "the call's end", 1, ended)
	if !cs[0].stopped || cs[0].term {
		t.Fatalf("call %+v: want stopped by its caller, with no SIGTERM delivered", cs[0])
	}
	if lines := stopLines(t, home); len(lines) != 0 {
		t.Fatalf("stopped-call lines for a process no signal reached: %+v", lines)
	}
}

// [CLAUDECALLS-1-S4] The shutdown's SIGKILL ends the call whole (#143): a child the
// claude started dies with it, so the call ends, and logs its line, before Close
// returns. Killing the claude alone left the child holding the call's output open
// past Close's wait, and the panel exited before the line was written: under load
// TestRunShutdownBoundsTheClaudeGrace's fake lost that race to the `sleep 1` its
// loop was in. Here the child outlives any wait of Close's, so the race is certain.
func TestGateCloseKillsTheCallsChildrenToo(t *testing.T) {
	marks := filepath.Join(t.TempDir(), "marks")
	fakeProgram(t, "claude", fmt.Sprintf(`trap 'echo TERM >> %[1]s' TERM
sleep 5 &
echo child >> %[1]s
while :; do sleep 1; done
`, shq(marks)))
	g, home := newGate(t, signals.ExecRunner, Reader{}, clock.System)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := goRun(g, ctx, agentsArgv)
	deadline := time.Now().Add(testwait.Scale(10 * time.Second))
	for !strings.Contains(readMarks(t, marks), "child") {
		if time.Now().After(deadline) {
			t.Fatal("the claude never started its child")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel() // the panel's shutdown: the caller's context ends, then the gate closes
	if err := result(t, errc, "the stopped call"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v, want the cancellation", err)
	}
	g.Close(ShutdownGrace)
	cs := g.calls()
	if len(cs) != 1 || !ended(cs[0]) {
		t.Fatalf("the call had not ended when Close returned: %+v", cs)
	}
	if lines := stopLines(t, home); len(lines) != 1 || lines[0].PID != cs[0].pid ||
		!slices.Equal(lines[0].Signals, []string{"SIGTERM", "SIGKILL"}) {
		t.Fatalf("stopped-call lines when Close returned: %+v", lines)
	}
	if got := readMarks(t, marks); !strings.Contains(got, "TERM") {
		t.Fatalf("SIGTERM never came first: marks %q", got)
	}
}

// Close kills a call until it has ended (#143): a child forked as the group's SIGKILL
// lands can miss it (macOS), and would hold the call's output open past Close's wait.
// This claude forks all the time: one kill alone missed about one shutdown in
// thirteen here, so forty in a row passed about one run in twenty-five. A short
// grace keeps the forty quick. Linux's fork does not lose the kill.
func TestGateCloseKillsAClaudeThatKeepsForking(t *testing.T) {
	marks := filepath.Join(t.TempDir(), "marks")
	fakeProgram(t, "claude", fmt.Sprintf("trap '' TERM\necho forking > %s\nwhile :; do sleep 2 & sleep 2 & sleep 2 & sleep 2 & sleep 0.005; done\n", shq(marks)))
	for i := 0; i < 40; i++ {
		_ = os.Remove(marks)
		g, home := newGate(t, signals.ExecRunner, Reader{}, clock.System)
		ctx, cancel := context.WithCancel(context.Background())
		errc := goRun(g, ctx, agentsArgv)
		deadline := time.Now().Add(testwait.Scale(10 * time.Second))
		for readMarks(t, marks) == "" {
			if time.Now().After(deadline) {
				t.Fatalf("shutdown %d: the claude never started forking", i)
			}
			time.Sleep(10 * time.Millisecond)
		}
		cancel()
		if err := result(t, errc, "the stopped call"); !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown %d: err %v, want the cancellation", i, err)
		}
		// The caller returns before the gate has marked the call stopped and sent
		// SIGTERM: a kill before those would leave no line naming both.
		waitCalls(t, g, "the SIGTERM", 1, func(c call) bool { return c.stopped && c.term })
		g.Close(20 * time.Millisecond)
		if cs := g.calls(); len(cs) != 1 || !ended(cs[0]) {
			t.Fatalf("shutdown %d: the call had not ended when Close returned: %+v", i, cs)
		}
		if lines := stopLines(t, home); len(lines) != 1 || !slices.Equal(lines[0].Signals, []string{"SIGTERM", "SIGKILL"}) {
			t.Fatalf("shutdown %d: stopped-call lines when Close returned: %+v", i, lines)
		}
	}
}

// [CLAUDECALLS-1-S3] Other commands keep their behaviour: a git past its timeout is
// SIGKILLed at once, as exec.CommandContext always did, and it passes the gate even
// while a claude call holds the slot.
func TestGateLeavesOtherCommandsAlone(t *testing.T) {
	fakeProgram(t, "git", "trap '' TERM\nexec sleep 30\n") // only SIGKILL stops it
	release := make(chan struct{})
	inner := func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		if signals.IsClaude(argv) {
			<-release
			return nil, nil
		}
		return signals.ExecRunner(ctx, dir, argv)
	}
	g, _ := newGate(t, inner, Reader{}, clock.System)
	go g.Run(context.Background(), "", agentsArgv) // holds the slot
	defer close(release)
	waitSlotTaken(t, g)

	ctx, cancel := context.WithTimeout(signals.WithPoll(context.Background()), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := g.Run(ctx, t.TempDir(), []string{"git", "status"})
	if took := time.Since(start); took > 300*time.Millisecond+testwait.Scale(2*time.Second) {
		t.Fatalf("git took %v to stop", took)
	}
	if !killedBySIGKILL(err) {
		t.Fatalf("git ended with %v, want SIGKILL", err)
	}
}

func waitSlotTaken(t *testing.T, g *Gate) {
	t.Helper()
	deadline := time.Now().Add(testwait.Scale(5 * time.Second))
	for len(g.slot) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the slot was never taken")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// holder is an inner Runner whose `claude hold` calls block until release, and
// which counts the claude calls that reach it.
type holder struct {
	release chan struct{}
	n       atomic.Int32
}

func newHolder() *holder { return &holder{release: make(chan struct{})} }

func (h *holder) run(ctx context.Context, dir string, argv []string) ([]byte, error) {
	if signals.IsClaude(argv) {
		h.n.Add(1)
		if len(argv) > 1 && argv[1] == "hold" {
			<-h.release
		}
	}
	return []byte("ok"), nil
}

// laneCtx is a lane action's context: it has a deadline, so the gate starts no timer
// of its own for it (a fake clock's BlockUntil counts only the call under test).
func laneCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func goRun(g *Gate, ctx context.Context, argv []string) <-chan error {
	errc := make(chan error, 1)
	go func() { _, err := g.Run(ctx, "", argv); errc <- err }()
	return errc
}

// giveUpWhenReady runs argv and gives up, as a caller's timeout would, once the fake
// has written "ready" to marks, after its trap: a fixed timeout raced the shell's
// start under load, and a SIGTERM before the trap killed the fake outright (OPS-122).
// It returns the call's error and how long the caller waited after giving up.
func giveUpWhenReady(t *testing.T, g *Gate, ctx context.Context, argv []string, marks string) (error, time.Duration) {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errc := goRun(g, ctx, argv)
	deadline := time.Now().Add(testwait.Scale(10 * time.Second))
	for !strings.Contains(readMarks(t, marks), "ready") {
		if time.Now().After(deadline) {
			t.Fatalf("the fake %s never got ready: marks %q", argv[0], readMarks(t, marks))
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	start := time.Now()
	err := <-errc
	return err, time.Since(start)
}

func noResult(t *testing.T, errc <-chan error, what string) {
	t.Helper()
	select {
	case err := <-errc:
		t.Fatalf("%s returned (%v)", what, err)
	case <-time.After(100 * time.Millisecond):
	}
}

func result(t *testing.T, errc <-chan error, what string) error {
	t.Helper()
	select {
	case err := <-errc:
		return err
	case <-time.After(testwait.Scale(10 * time.Second)):
		t.Fatalf("%s never returned", what)
		return nil
	}
}

// Quiesce, the panel's wait before it restarts into a rebuilt binary (OPS-118 D4),
// returns only once the running call has ended, keeps the slot so no call starts
// after it, and gives up at its bound when a call outlasts it.
func TestGateQuiesceWaitsForTheRunningCallAndKeepsTheSlot(t *testing.T) {
	clk := clock.NewFake(t0)
	h := newHolder()
	g, _ := newGate(t, h.run, Reader{}, clk)
	goRun(g, laneCtx(t), []string{"claude", "hold"})
	waitSlotTaken(t, g)
	took := make(chan bool, 1)
	go func() { took <- g.Quiesce(context.Background(), 10*time.Minute) }()
	clk.BlockUntil(1) // Quiesce's bound
	clk.Advance(10*time.Minute - time.Second)
	select {
	case ok := <-took:
		t.Fatalf("Quiesce returned %v while a call ran", ok)
	case <-time.After(100 * time.Millisecond):
	}
	close(h.release)
	select {
	case ok := <-took:
		if !ok {
			t.Fatal("Quiesce did not take the slot once the call ended")
		}
	case <-time.After(testwait.Scale(10 * time.Second)):
		t.Fatal("Quiesce never returned after the call ended")
	}
	// The slot is kept: a poll that comes after gives up busy, and reaches no claude.
	ctx, cancel := context.WithTimeout(signals.WithPoll(context.Background()), 10*time.Second)
	defer cancel()
	errc := goRun(g, ctx, agentsArgv)
	clk.BlockUntil(1)
	clk.Advance(PollWait)
	if err := result(t, errc, "the poll after Quiesce"); !errors.Is(err, ErrBusy) {
		t.Fatalf("a poll after Quiesce: %v, want ErrBusy", err)
	}
	if n := h.n.Load(); n != 1 {
		t.Fatalf("%d claude calls reached the runner, want only the one before Quiesce", n)
	}

	// A call that outlasts the bound: Quiesce gives up at it.
	h2 := newHolder()
	defer close(h2.release)
	clk2 := clock.NewFake(t0)
	g2, _ := newGate(t, h2.run, Reader{}, clk2)
	goRun(g2, laneCtx(t), []string{"claude", "hold"})
	waitSlotTaken(t, g2)
	go func() { took <- g2.Quiesce(context.Background(), 10*time.Minute) }()
	clk2.BlockUntil(1)
	clk2.Advance(10 * time.Minute)
	select {
	case ok := <-took:
		if ok {
			t.Fatal("Quiesce took a slot a running call holds")
		}
	case <-time.After(testwait.Scale(10 * time.Second)):
		t.Fatal("Quiesce did not give up at its bound")
	}
}

// [CLAUDECALLS-2-S1] A colliding poll waits briefly, then retries soon: no second
// claude starts, and the poll gives up at PollWait with a retry in 1 s.
func TestGatePollGivesUpOnABusySlot(t *testing.T) {
	clk := clock.NewFake(t0)
	h := newHolder()
	defer close(h.release)
	g, _ := newGate(t, h.run, Reader{}, clk)
	goRun(g, laneCtx(t), []string{"claude", "hold"})
	waitSlotTaken(t, g)
	ctx, cancel := context.WithTimeout(signals.WithPoll(context.Background()), 10*time.Second)
	defer cancel()
	errc := goRun(g, ctx, agentsArgv)
	clk.BlockUntil(1) // the poll's wait
	clk.Advance(PollWait - time.Millisecond)
	noResult(t, errc, "the poll, before its wait was up,")
	clk.Advance(time.Millisecond)
	err := result(t, errc, "the poll")
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("err %v, want ErrBusy", err)
	}
	if wait, ok := RetryAfter(err); !ok || wait != BusyRetry || wait > 2*time.Second {
		t.Fatalf("retry %v %v, want %v", wait, ok, BusyRetry)
	}
	if n := h.n.Load(); n != 1 {
		t.Fatalf("%d claude calls reached the runner, want only the holder's", n)
	}
}

// [CLAUDECALLS-2-S2] A lane action, carrying no poll marker, waits for the slot
// within its own timeout: it does not give up at the poll's 2 s, and runs as soon as
// the holder's call ends.
func TestGateLaneActionWaitsForTheSlot(t *testing.T) {
	// The real clock: a lane action sets no timer of the gate's, so only real time
	// shows it outlasting the poll's wait.
	h := newHolder()
	g, _ := newGate(t, h.run, Reader{}, clock.System)
	goRun(g, signals.WithPoll(context.Background()), []string{"claude", "hold"})
	waitSlotTaken(t, g)
	ctx, cancel := context.WithTimeout(context.Background(), testwait.Scale(10*time.Second))
	defer cancel()
	errc := goRun(g, ctx, agentsArgv)
	time.Sleep(PollWait + 500*time.Millisecond)
	noResult(t, errc, "the lane action, past the poll's wait,")
	close(h.release)
	if err := result(t, errc, "the lane action"); err != nil {
		t.Fatalf("the lane action: %v", err)
	}
	if n := h.n.Load(); n != 2 {
		t.Fatalf("%d claude calls, want 2", n)
	}
}

// [CLAUDECALLS-2-S3] A stopped call holds the slot until it exits: a claude that
// takes 5 s to exit after SIGTERM keeps a poll due 1 s after the timeout out, and a
// lane action starts only once it has been reaped.
func TestGateStoppedCallHoldsTheSlotUntilItExits(t *testing.T) {
	marks := filepath.Join(t.TempDir(), "marks")
	fakeProgram(t, "claude", fmt.Sprintf(`case "$1" in
agents) trap 'kill $P 2>/dev/null; sleep 5; exit 0' TERM; echo ready >> %s; sleep 60 & P=$!; wait $P;;
*) echo '[]';;
esac
`, shq(marks)))
	g, _ := newGate(t, signals.ExecRunner, Reader{}, clock.System)
	if err, _ := giveUpWhenReady(t, g, context.Background(), agentsArgv, marks); !errors.Is(err, context.Canceled) {
		t.Fatalf("err %v, want the caller's own end", err)
	}
	time.Sleep(time.Second)
	pctx, pcancel := context.WithTimeout(signals.WithPoll(context.Background()), 10*time.Second)
	defer pcancel()
	if _, err := g.Run(pctx, t.TempDir(), []string{"claude", "--version"}); !errors.Is(err, ErrBusy) {
		t.Fatalf("a poll 1 s after the timeout: %v, want ErrBusy", err)
	}
	lctx, lcancel := context.WithTimeout(context.Background(), testwait.Scale(10*time.Second))
	defer lcancel()
	if _, err := g.Run(lctx, t.TempDir(), []string{"claude", "--version"}); err != nil {
		t.Fatalf("the lane action: %v", err)
	}
	cs := g.calls()
	if len(cs) != 2 || cs[1].spawned.Before(cs[0].ended) {
		t.Fatalf("a second claude started before the first had exited: %+v", cs)
	}
}

// [CLAUDECALLS-2-S5] A call with no deadline does not wait without bound: behind a
// call holding the slot it gives up at 10 s with ErrBusy, and no second claude starts.
func TestGateCallWithNoDeadlineGivesUp(t *testing.T) {
	clk := clock.NewFake(t0)
	h := newHolder()
	defer close(h.release)
	g, _ := newGate(t, h.run, Reader{}, clk)
	goRun(g, laneCtx(t), []string{"claude", "hold"})
	waitSlotTaken(t, g)
	errc := goRun(g, context.Background(), agentsArgv)
	clk.BlockUntil(1)
	clk.Advance(NoDeadlineWait - time.Millisecond)
	noResult(t, errc, "the call, before 10 s,")
	clk.Advance(time.Millisecond)
	if err := result(t, errc, "the call"); !errors.Is(err, ErrBusy) {
		t.Fatalf("err %v, want ErrBusy", err)
	}
	if n := h.n.Load(); n != 1 {
		t.Fatalf("%d claude calls, want only the holder's", n)
	}
}

// [CLAUDECALLS-2-S4] Calls that start together all run, one after another.
func TestGateCallsThatStartTogetherAllRun(t *testing.T) {
	var cur, peak atomic.Int32
	inner := func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		n := cur.Add(1)
		defer cur.Add(-1)
		for m := peak.Load(); n > m && !peak.CompareAndSwap(m, n); m = peak.Load() {
		}
		time.Sleep(100 * time.Millisecond)
		return nil, nil
	}
	g, _ := newGate(t, inner, Reader{}, clock.System)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i, argv := range [][]string{agentsArgv, {"claude", "--version"}, {"/usr/bin/env", "-u", "X", "claude", "auth", "status"}, agentsArgv} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(signals.WithPoll(context.Background()), 10*time.Second)
			defer cancel()
			_, errs[i] = g.Run(ctx, "", argv)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("call %d: %v", i, err)
		}
	}
	if peak.Load() != 1 {
		t.Fatalf("%d claude calls ran at once", peak.Load())
	}
}

// simulated is a call in g's history with the given times, for a pid this test
// owns (a sleep it started), so nothing can ever signal another process.
func simulated(t *testing.T, g *Gate, clk *clock.Fake, pid int, spawned, end time.Time) {
	t.Helper()
	p, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	c := &call{name: "claude agents --json"}
	clk.Advance(spawned.Sub(clk.Now()))
	g.spawned(c, p)
	clk.Advance(end.Sub(clk.Now()))
	g.end(c, nil)
}

// The history keeps the last 10 minutes of calls, at most 2,000.
func TestGateHistoryIsBounded(t *testing.T) {
	clk := clock.NewFake(t0)
	g, _ := newGate(t, nil, Reader{}, clk)
	pid := otherHolder(t)
	for i := 0; i < historyMax+10; i++ {
		simulated(t, g, clk, pid, clk.Now().Add(10*time.Millisecond), clk.Now().Add(20*time.Millisecond))
	}
	if n := len(g.calls()); n != historyMax {
		t.Fatalf("%d calls kept, want %d", n, historyMax)
	}
	simulated(t, g, clk, pid, clk.Now().Add(historyKeep+time.Second), clk.Now().Add(historyKeep+2*time.Second))
	if n := len(g.calls()); n != 1 {
		t.Fatalf("%d calls kept after 10 minutes, want the newest alone", n)
	}
}
