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
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
	"github.com/clauductor/clauductor/internal/testwait"
)

// otherHolder starts a process that is not the panel's, to stand as a lock's holder.
func otherHolder(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "300")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	return cmd.Process.Pid
}

// goneHolder is the pid of a process that has exited and been reaped.
func goneHolder(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

// record is an owner record with no procStart (Claude Code's is optional): the watch
// then has no start time to tell a reused pid by (recordStart has one).
func record(pid int, birth time.Time) string {
	return fmt.Sprintf(`{"pid":%d,"pidDomain":"d","pidSpace":"s","lockBirthtimeMs":%d}`, pid, birth.UnixMilli())
}

func newWatch(t *testing.T, home string, r Reader, clk clock.Clock) *Watch {
	t.Helper()
	return &Watch{Reader: r, Log: NewLog(home), Clock: clk, Run: signals.ExecRunner}
}

func readLines(t *testing.T, home string) []LockLine {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(config.PanelDir(home), LogName))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []LockLine
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		var l LockLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatalf("line %q: %v", sc.Text(), err)
		}
		out = append(out, l)
	}
	return out
}

// [CLAUDECALLS-4-S1] A lock held by another process is logged: once it appears and
// later goes, one line names its path, the holder's pid and command, not the panel's
// own, when it was first seen, when it was last modified and how it ended.
func TestWatchLogsALockHeldByAnotherProcess(t *testing.T) {
	home, r := fakeConfig(t)
	pid := otherHolder(t)
	clk := clock.NewFake(t0)
	w := newWatch(t, home, r, clk)
	// The panel's call history holds no call with this pid (group 2's gate supplies it).
	var asked []Holder
	w.Attribute = func(h Holder) Attribution { asked = append(asked, h); return Attribution{} }
	ctx := context.Background()

	w.Tick(ctx) // no lock yet
	clk.Advance(2 * time.Second)
	firstSeen := clk.Now()
	birth := firstSeen.Add(-time.Second)
	makeLock(t, r.CurrentPath(), birth)
	writeRecord(t, r, record(pid, birth))
	w.Tick(ctx)
	// The holder touches it every 5 s while it refreshes.
	clk.Advance(2 * time.Second)
	lastMod := clk.Now()
	touch(t, r.CurrentPath(), lastMod)
	w.Tick(ctx)
	clk.Advance(2 * time.Second)
	w.Tick(ctx)
	if got := readLines(t, home); len(got) != 0 {
		t.Fatalf("logged while the lock is still there: %+v", got)
	}
	// It goes: the record first, then the lock, as Claude Code releases it.
	os.Remove(r.OwnerPath())
	if err := os.Remove(r.CurrentPath()); err != nil {
		t.Fatal(err)
	}
	clk.Advance(2 * time.Second)
	endedAt := clk.Now()
	w.Tick(ctx)
	w.Tick(ctx)

	got := readLines(t, home)
	if len(got) != 1 {
		t.Fatalf("lines: %+v, want one", got)
	}
	l := got[0]
	if l.Kind != "lock" || l.Which != Current || l.Path != r.CurrentPath() {
		t.Errorf("which lock: %+v", l)
	}
	if l.PID != pid || !strings.Contains(l.Command, "sleep 300") || !l.HolderRunning {
		t.Errorf("holder: pid %d command %q running %v, want pid %d running `sleep 300`", l.PID, l.Command, l.HolderRunning, pid)
	}
	if l.Own || l.Call != "" || l.Holder != "other" {
		t.Errorf("attribution: own %v call %q holder %q, want not the panel's own", l.Own, l.Call, l.Holder)
	}
	if len(asked) != 1 || asked[0].PID != pid || !asked[0].Birth.Equal(birth) {
		t.Errorf("attribution asked %+v, want once, for pid %d born %v", asked, pid, birth)
	}
	if !l.FirstSeen.Equal(firstSeen) || !l.LastModified.Equal(lastMod) {
		t.Errorf("first seen %v (want %v), last modified %v (want %v)", l.FirstSeen, firstSeen, l.LastModified, lastMod)
	}
	if l.Ended != EndGone || !l.EndedAt.Equal(endedAt) {
		t.Errorf("ended %q at %v, want %q at %v", l.Ended, l.EndedAt, EndGone, endedAt)
	}
	if l.Birth == nil || !l.Birth.Equal(birth) {
		t.Errorf("birth %v, want %v", l.Birth, birth)
	}
}

// The legacy lock beside the config dir is watched and logged too, apart from the
// current one.
func TestWatchLogsTheLegacyLock(t *testing.T) {
	home, r := fakeConfig(t)
	clk := clock.NewFake(t0)
	w := newWatch(t, home, r, clk)
	makeLock(t, r.LegacyPath(), t0.Add(-time.Second))
	w.Tick(context.Background())
	os.Remove(r.LegacyPath())
	clk.Advance(2 * time.Second)
	w.Tick(context.Background())
	got := readLines(t, home)
	if len(got) != 1 || got[0].Which != Legacy || got[0].Path != r.LegacyPath() || got[0].Ended != EndGone {
		t.Fatalf("lines: %+v", got)
	}
	if got[0].Holder != "unknown: no owner record" || got[0].PID != 0 {
		t.Errorf("holder: %+v", got[0])
	}
}

// A legacy lock whose readable record names no legacy lock (no legacyLockBirthtimeMs)
// is not logged as having no record: the record exists, and names another lock.
func TestWatchLegacyLockTheRecordDoesNotName(t *testing.T) {
	home, r := fakeConfig(t)
	w := newWatch(t, home, r, clock.NewFake(t0))
	makeLock(t, r.LegacyPath(), t0)
	writeRecord(t, r, record(4242, t0))
	w.Tick(context.Background())
	w.Stop()
	got := readLines(t, home)
	if len(got) != 1 || got[0].Which != Legacy || got[0].Holder != "unknown: the record names no legacy lock" || got[0].PID != 0 {
		t.Fatalf("lines: %+v", got)
	}
}

// A holder whose process is gone by first sight is logged with no command.
func TestWatchHolderAlreadyGone(t *testing.T) {
	home, r := fakeConfig(t)
	clk := clock.NewFake(t0)
	w := newWatch(t, home, r, clk)
	pid := goneHolder(t)
	makeLock(t, r.CurrentPath(), t0.Add(-5*time.Minute))
	writeRecord(t, r, record(pid, t0.Add(-5*time.Minute)))
	w.Tick(context.Background())
	w.Stop()
	got := readLines(t, home)
	if len(got) != 1 || got[0].PID != pid || got[0].HolderRunning || got[0].Command != "" {
		t.Fatalf("lines: %+v", got)
	}
	if got[0].Ended != EndStopped || got[0].LastState != Stale {
		t.Errorf("ended %q, last state %q", got[0].Ended, got[0].LastState)
	}
}

// A new birth time at the same path is another lock: the first is logged replaced.
// A record that appears after the lock is the same lock, now with its holder.
func TestWatchIdentity(t *testing.T) {
	home, r := fakeConfig(t)
	pid := otherHolder(t)
	clk := clock.NewFake(t0)
	w := newWatch(t, home, r, clk)
	ctx := context.Background()

	makeLock(t, r.CurrentPath(), t0)
	w.Tick(ctx) // no record yet
	writeRecord(t, r, record(pid, t0))
	clk.Advance(2 * time.Second)
	w.Tick(ctx) // the same run, now named
	// Released and taken again between two ticks: another birth time.
	b2 := clk.Now().Add(-500 * time.Millisecond)
	writeRecord(t, r, record(pid, b2))
	touch(t, r.CurrentPath(), b2)
	clk.Advance(2 * time.Second)
	w.Tick(ctx)
	w.Stop()

	got := readLines(t, home)
	if len(got) != 2 {
		t.Fatalf("lines: %+v, want two", got)
	}
	if got[0].Ended != EndReplaced || got[0].PID != pid || !got[0].Birth.Equal(t0) || !got[0].FirstSeen.Equal(t0) {
		t.Errorf("first: %+v", got[0])
	}
	if got[1].Ended != EndStopped || !got[1].Birth.Equal(b2) {
		t.Errorf("second: %+v", got[1])
	}
}

// The holder is attributed once, at first sight, and keeps that attribution.
func TestWatchAttributesOnce(t *testing.T) {
	home, r := fakeConfig(t)
	clk := clock.NewFake(t0)
	w := newWatch(t, home, r, clk)
	calls := 0
	w.Attribute = func(h Holder) Attribution {
		calls++
		if calls == 1 {
			return Attribution{Own: true, Call: "claude agents --json"}
		}
		return Attribution{}
	}
	makeLock(t, r.CurrentPath(), t0)
	writeRecord(t, r, record(4242, t0))
	for i := 0; i < 3; i++ {
		w.Tick(context.Background())
		clk.Advance(time.Hour)
	}
	w.Stop()
	got := readLines(t, home)
	if calls != 1 || len(got) != 1 || !got[0].Own || got[0].Holder != "own" || got[0].Call != "claude agents --json" {
		t.Fatalf("calls %d, lines %+v", calls, got)
	}

	w.Attribute = func(Holder) Attribution { return Attribution{Unknown: "before this panel started"} }
	w.Tick(context.Background())
	w.Stop()
	got = readLines(t, home)
	if len(got) != 2 || got[1].Holder != "unknown: before this panel started" || got[1].Own {
		t.Fatalf("unknown: %+v", got)
	}
}

// A watch with no call history to ask does not claim a holder is someone else's: a
// lock the panel stranded would otherwise read as not the panel's.
func TestWatchWithoutHistorySaysUnknown(t *testing.T) {
	home, r := fakeConfig(t)
	w := newWatch(t, home, r, clock.NewFake(t0))
	makeLock(t, r.CurrentPath(), t0)
	writeRecord(t, r, record(4242, t0))
	w.Tick(context.Background())
	w.Stop()
	got := readLines(t, home)
	if len(got) != 1 || got[0].Holder != "unknown: no call history" || got[0].Own || got[0].PID != 4242 {
		t.Fatalf("lines: %+v", got)
	}
}

func TestWatchUnreadableRecord(t *testing.T) {
	home, r := fakeConfig(t)
	w := newWatch(t, home, r, clock.NewFake(t0))
	makeLock(t, r.CurrentPath(), t0)
	writeRecord(t, r, `{"pid":"no"}`)
	w.Tick(context.Background())
	w.Stop()
	got := readLines(t, home)
	if len(got) != 1 || got[0].Holder != "unknown: owner record unreadable" || got[0].PID != 0 || got[0].LastState != Live {
		t.Fatalf("lines: %+v", got)
	}
}

// Loop ticks on the clock and, when the panel stops, logs the locks still there.
func TestWatchLoop(t *testing.T) {
	home, r := fakeConfig(t)
	clk := clock.NewFake(t0)
	w := newWatch(t, home, r, clk)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Loop(ctx); close(done) }()
	clk.BlockUntil(1) // the first tick is done
	makeLock(t, r.LegacyPath(), t0)
	clk.Advance(WatchEvery)
	clk.BlockUntil(1) // the second tick saw it
	os.Remove(r.LegacyPath())
	clk.Advance(WatchEvery)
	clk.BlockUntil(1)
	if got := readLines(t, home); len(got) != 1 || got[0].Ended != EndGone {
		t.Fatalf("after it went: %+v", got)
	}
	makeLock(t, r.CurrentPath(), clk.Now())
	clk.Advance(WatchEvery)
	clk.BlockUntil(1)
	cancel()
	select {
	case <-done:
	case <-time.After(testwait.Scale(5 * time.Second)):
		t.Fatal("Loop did not return when its context ended")
	}
	got := readLines(t, home)
	if len(got) != 2 || got[1].Which != Current || got[1].Ended != EndStopped {
		t.Fatalf("at stop: %+v", got)
	}
}

func TestLogLivesInThePanelDirAndIsPrivate(t *testing.T) {
	home := t.TempDir()
	l := NewLog(home)
	if want := filepath.Join(home, ".clauductor", "panel", "login-lock.jsonl"); l.Path != want {
		t.Errorf("path %q, want %q", l.Path, want)
	}
	if err := l.Append(map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(l.Path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode: %v %v", fi, err)
	}
}

// The log is capped: past its size, the oldest lines go and whole newest lines stay.
func TestLogIsCapped(t *testing.T) {
	l := &Log{Path: filepath.Join(t.TempDir(), "panel", LogName), Max: 1000}
	for i := 0; i < 200; i++ {
		if err := l.Append(map[string]int{"n": i}); err != nil {
			t.Fatal(err)
		}
		if fi, err := os.Stat(l.Path); err != nil || fi.Size() > l.Max {
			t.Fatalf("after %d: size %v, cap %d (%v)", i, fi.Size(), l.Max, err)
		}
	}
	b, _ := os.ReadFile(l.Path)
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	for _, s := range lines {
		var v map[string]int
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			t.Fatalf("a cut line survived the trim: %q", s)
		}
	}
	if last := lines[len(lines)-1]; last != `{"n":199}` {
		t.Errorf("newest line %q, want {\"n\":199}", last)
	}
	if len(lines) < 30 {
		t.Errorf("kept %d lines; the trim should keep about half the cap", len(lines))
	}
}

// The trim keeps every whole line that fits: a cut landing exactly at a line's start
// keeps that line, and a cut inside a line drops only that line's remainder.
func TestLogTrimKeepsALineTheCutStartsAt(t *testing.T) {
	l := &Log{Path: filepath.Join(t.TempDir(), LogName)}
	for _, c := range []struct {
		keep int64
		want string
	}{
		{5, "bbbb\n"}, // the cut lands just after "aaaa\n"
		{6, "bbbb\n"}, // the cut lands inside "aaaa\n"
		{4, ""},       // the cut lands inside the last line
		{10, "aaaa\nbbbb\n"},
	} {
		if err := os.WriteFile(l.Path, []byte("aaaa\nbbbb\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := l.trim(c.keep); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(l.Path); string(b) != c.want {
			t.Errorf("keep %d: %q, want %q", c.keep, b, c.want)
		}
	}
}

// A log that cannot be written is said, once per distinct error, not left silent.
func TestWatchReportsALogItCannotWrite(t *testing.T) {
	home, r := fakeConfig(t)
	blocker := filepath.Join(home, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := newWatch(t, home, r, clock.NewFake(t0))
	w.Log = &Log{Path: filepath.Join(blocker, "panel", LogName)}
	w.Out = &out
	for i := 0; i < 2; i++ {
		makeLock(t, r.CurrentPath(), t0)
		w.Tick(context.Background())
		os.Remove(r.CurrentPath())
		w.Tick(context.Background())
	}
	if n := strings.Count(out.String(), "login-lock log:"); n != 1 {
		t.Errorf("reported %d times: %q", n, out.String())
	}
}

// heldClaude runs `claude agents --json` through g with a fake claude that sleeps
// until stopped (SIGTERM ends it), and returns its pid once it runs and the stop.
func heldClaude(t *testing.T, g *Gate, timeout time.Duration) (pid int, stop func()) {
	t.Helper()
	fakeProgram(t, "claude", "exec sleep 30\n")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	done := make(chan struct{})
	go func() { defer close(done); g.Run(ctx, t.TempDir(), agentsArgv) }()
	n := len(g.calls())
	cs := waitCalls(t, g, "the claude start", n+1, running)
	stop = func() { cancel(); <-done }
	t.Cleanup(stop)
	return cs[n].pid, stop
}

// [CLAUDECALLS-4-S2] A lock whose owner record names the pid of a running claude call
// of the panel's is logged as the panel's own, naming the call.
// [CLAUDECALLS-4-S7] It is so too when the call took the lock before the watch's
// first tick.
func TestWatchAttributesALockToTheRunningCall(t *testing.T) {
	home, r := fakeConfig(t)
	g, _ := newGate(t, signals.ExecRunner, r, clock.System)
	pid, stop := heldClaude(t, g, time.Minute)
	w := &Watch{Reader: r, Log: NewLog(home), Clock: clock.System, Attribute: g.Attribute}
	makeLock(t, r.CurrentPath(), time.Now())
	writeRecord(t, r, record(pid, time.Now()))
	w.Tick(context.Background()) // the watch's first tick: the lock was there before it
	stop()
	os.RemoveAll(r.CurrentPath())
	w.Tick(context.Background())
	got := readLines(t, home)
	if len(got) != 1 || !got[0].Own || got[0].Holder != "own" || got[0].Call != "claude agents --json" || got[0].PID != pid {
		t.Fatalf("lines: %+v", got)
	}
}

// [CLAUDECALLS-4-S4] A lock left by a panel call that was stopped and reaped before
// the watch first saw it is still the panel's: its birth falls within the call's
// spawn and end.
func TestWatchAttributesALockLeftByAReapedCall(t *testing.T) {
	home, r := fakeConfig(t)
	g, _ := newGate(t, signals.ExecRunner, r, clock.System)
	pid, stop := heldClaude(t, g, time.Minute)
	makeLock(t, r.CurrentPath(), time.Now())
	writeRecord(t, r, record(pid, time.Now()))
	stop()
	waitCalls(t, g, "the reap", 1, ended)
	w := &Watch{Reader: r, Log: NewLog(home), Clock: clock.System, Run: signals.ExecRunner, Attribute: g.Attribute}
	w.Tick(context.Background())
	w.Stop()
	got := readLines(t, home)
	if len(got) != 1 || !got[0].Own || got[0].Call != "claude agents --json" || got[0].HolderRunning {
		t.Fatalf("lines: %+v", got)
	}
}

// [CLAUDECALLS-4-S9] A lock taken during the SIGTERM grace is still inside its call:
// the call's end is when it was reaped, not when it was stopped.
func TestWatchAttributesALockTakenDuringTheGrace(t *testing.T) {
	home, r := fakeConfig(t)
	// The grace outlasts the test's second of sleep below by a margin load cannot eat.
	marks := filepath.Join(t.TempDir(), "marks")
	fakeProgram(t, "claude", fmt.Sprintf(`trap 'kill $P 2>/dev/null; sleep 8; exit 0' TERM
sleep 60 & P=$!
echo ready >> %s
wait $P
`, shq(marks)))
	g, _ := newGate(t, signals.ExecRunner, r, clock.System)
	if err, _ := giveUpWhenReady(t, g, agentsArgv, marks); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v, want the timeout", err)
	}
	time.Sleep(time.Second) // into the grace: SIGTERM went out a second ago
	cs := g.calls()
	if len(cs) != 1 || !running(cs[0]) {
		t.Fatalf("history %+v", cs)
	}
	makeLock(t, r.CurrentPath(), time.Now())
	writeRecord(t, r, record(cs[0].pid, time.Now()))
	cs = waitCalls(t, g, "the reap", 1, ended)
	w := &Watch{Reader: r, Log: NewLog(home), Clock: clock.System, Attribute: g.Attribute}
	w.Tick(context.Background())
	w.Stop()
	got := readLines(t, home)
	if len(got) != 1 || !got[0].Own {
		t.Fatalf("lines: %+v (call %+v)", got, cs[0])
	}
}

// [CLAUDECALLS-4-S5] A lock created after a panel call ended is not the panel's,
// whatever its pid.
func TestWatchALockBornAfterItsPidsCallIsAnothers(t *testing.T) {
	home, r := fakeConfig(t)
	clk := clock.NewFake(t0)
	g, _ := newGate(t, nil, r, clk)
	pid := otherHolder(t)
	end := t0.Add(time.Hour) // 10:00:00, as it were
	simulated(t, g, clk, pid, end.Add(-time.Second), end)
	clk.Advance(30 * time.Second)
	makeLock(t, r.CurrentPath(), clk.Now())
	writeRecord(t, r, record(pid, end.Add(30*time.Second))) // the pid reused 30 s later
	w := &Watch{Reader: r, Log: NewLog(home), Clock: clk, Attribute: g.Attribute}
	w.Tick(context.Background())
	w.Stop()
	if got := readLines(t, home); len(got) != 1 || got[0].Own || got[0].Holder != "other" {
		t.Fatalf("lines: %+v", got)
	}
}

// [CLAUDECALLS-4-S6] A lock born before the panel started has an unknown holder.
func TestWatchALockBornBeforeThePanelHasAnUnknownHolder(t *testing.T) {
	home, r := fakeConfig(t)
	clk := clock.NewFake(t0)
	g, _ := newGate(t, nil, r, clk) // the panel starts at t0
	makeLock(t, r.CurrentPath(), t0)
	writeRecord(t, r, record(4242, t0.Add(-10*time.Second)))
	w := &Watch{Reader: r, Log: NewLog(home), Clock: clk, Attribute: g.Attribute}
	w.Tick(context.Background())
	w.Stop()
	if got := readLines(t, home); len(got) != 1 || got[0].Own || got[0].Holder != "unknown: before this panel started" {
		t.Fatalf("lines: %+v", got)
	}
}

// [CLAUDECALLS-4-S8] A stranded lock keeps its attribution after its call ages out of
// the history: an hour later, and when it finally goes, its line still says own.
func TestWatchAStrandedLockKeepsItsAttribution(t *testing.T) {
	home, r := fakeConfig(t)
	clk := clock.NewFake(t0)
	g, _ := newGate(t, nil, r, clk)
	pid := otherHolder(t)
	simulated(t, g, clk, pid, t0.Add(time.Second), t0.Add(3*time.Second))
	birth := t0.Add(2 * time.Second)
	makeLock(t, r.CurrentPath(), birth)
	writeRecord(t, r, record(pid, birth))
	w := &Watch{Reader: r, Log: NewLog(home), Clock: clk, Attribute: g.Attribute}
	w.Tick(context.Background())
	clk.Advance(time.Hour)
	if a := g.Attribute(Holder{PID: pid, Birth: birth}); a.Own {
		t.Fatalf("the call has not aged out of the history: %+v", g.calls())
	}
	w.Tick(context.Background())
	os.RemoveAll(r.CurrentPath())
	clk.Advance(WatchEvery)
	w.Tick(context.Background())
	got := readLines(t, home)
	if len(got) != 1 || !got[0].Own || got[0].Call != "claude agents --json" || got[0].Ended != EndGone || got[0].LastState != Stale {
		t.Fatalf("lines: %+v", got)
	}
}

// holderRunner is the watch's Runner for a holder: `ps -o command=` and the lstart
// Claude Code records, as the test sets them.
type holderRunner struct {
	command, lstart string
	lstarts         int
}

func (h *holderRunner) run(_ context.Context, _ string, argv []string) ([]byte, error) {
	switch j := strings.Join(argv[:len(argv)-1], " "); j {
	case "ps -o command= -p":
		return []byte(h.command + "\n"), nil
	case "/usr/bin/env LC_ALL=C TZ=UTC ps -o lstart= -p":
		h.lstarts++
		return []byte(h.lstart + "\n"), nil
	}
	return nil, fmt.Errorf("unexpected %v", argv)
}

// recordStart is an owner record with a procStart.
func recordStart(pid int, birth time.Time, procStart string) string {
	return fmt.Sprintf(`{"pid":%d,"procStart":%q,"pidDomain":"d","pidSpace":"s","lockBirthtimeMs":%d}`, pid, procStart, birth.UnixMilli())
}

// lockWatch is a watch over r whose holder checks the test controls, and the views it
// last handed OnChange.
func lockWatch(t *testing.T, home string, r Reader, clk clock.Clock, h *holderRunner, alive *bool) (*Watch, *[]types.LoginLock) {
	t.Helper()
	views := &[]types.LoginLock{}
	w := &Watch{Reader: r, Log: NewLog(home), Clock: clk, Run: h.run, Alive: func(int) bool { return *alive },
		OnChange: func(v []types.LoginLock) { *views = v }}
	return w, views
}

// [CLAUDECALLS-5-S1] The watch's side of a lock held live for over 2 minutes: one
// modified every 5 s for 3 minutes is one unbroken live run, LiveSince when the watch
// first saw it, so the alert can say how long; a lock that goes stale, or another
// identity, starts a new run.
func TestWatchKeepsALiveRunUnbroken(t *testing.T) {
	home, r := fakeConfig(t)
	clk := clock.NewFake(t0)
	alive := true
	h := &holderRunner{command: "claude -p hi", lstart: "Fri Oct  2 10:11:00 2026"}
	w, views := lockWatch(t, home, r, clk, h, &alive)
	makeLock(t, r.CurrentPath(), t0)
	writeRecord(t, r, recordStart(4242, t0, h.lstart))
	ctx := context.Background()
	for clk.Now().Sub(t0) <= 3*time.Minute {
		if clk.Now().Sub(t0)%(5*time.Second) < 2*time.Second {
			touch(t, r.CurrentPath(), clk.Now())
		}
		w.Tick(ctx)
		clk.Advance(WatchEvery)
	}
	v := *views
	if len(v) != 1 || v[0].State != string(Live) || v[0].LiveSince != t0.UnixMilli() || v[0].PID != 4242 ||
		v[0].Command != "claude -p hi" || v[0].HolderState != HolderAlive || v[0].Birth != t0.UnixMilli() {
		t.Fatalf("after 3 min live: %+v", v)
	}
	// Untouched for a minute: stale, its live run over.
	clk.Advance(StaleAfter)
	w.Tick(ctx)
	if v := *views; len(v) != 1 || v[0].State != string(Stale) || v[0].LiveSince != 0 || v[0].StaleSince == 0 {
		t.Fatalf("stale: %+v", v)
	}
	// Touched again: a new live run, from now.
	touch(t, r.CurrentPath(), clk.Now())
	w.Tick(ctx)
	if v := *views; len(v) != 1 || v[0].LiveSince != clk.Now().UnixMilli() {
		t.Fatalf("live again: %+v", v)
	}
	if h.lstarts != 1 {
		t.Errorf("ps lstart ran %d times for one holder, want once", h.lstarts)
	}
}

// [CLAUDECALLS-5-S2] The watch's side of a stranded lock: one last modified 5 minutes
// ago whose record names a pid that no longer runs reads stale, since its mtime plus
// 60 s, with its holder gone.
func TestWatchSaysAStrandedLocksHolderIsGone(t *testing.T) {
	home, r := fakeConfig(t)
	clk := clock.NewFake(t0)
	alive := false
	w, views := lockWatch(t, home, r, clk, &holderRunner{}, &alive)
	mod := t0.Add(-5 * time.Minute)
	makeLock(t, r.CurrentPath(), mod)
	writeRecord(t, r, record(4242, mod))
	w.Tick(context.Background())
	w.Stop()
	v := *views
	if len(v) != 1 || v[0].State != string(Stale) || v[0].StaleSince != mod.Add(StaleAfter).UnixMilli() || v[0].HolderState != HolderGone || v[0].Command != "" {
		t.Fatalf("views %+v", v)
	}
	if got := readLines(t, home); len(got) != 1 || got[0].HolderState != HolderGone || got[0].HolderRunning {
		t.Fatalf("lines %+v", got)
	}
}

// Carried from group 1 (a): a holder whose pid now names another process is "gone
// (pid reused)": `ps -o lstart=` differs from the record's procStart, or the pid runs
// again after it was seen gone. Its command is not read: it would be another's.
func TestWatchSaysAReusedPidIsGone(t *testing.T) {
	home, r := fakeConfig(t)
	clk := clock.NewFake(t0)
	alive := true
	h := &holderRunner{command: "vim notes.txt", lstart: "Fri Oct  2 10:11:30 2026"}
	w, views := lockWatch(t, home, r, clk, h, &alive)
	makeLock(t, r.CurrentPath(), t0)
	writeRecord(t, r, recordStart(4242, t0, "Fri Oct  2 10:11:00 2026"))
	w.Tick(context.Background())
	w.Stop()
	if v := *views; len(v) != 1 || v[0].HolderState != HolderPIDReused || v[0].Command != "" {
		t.Fatalf("views %+v", v)
	}
	if got := readLines(t, home); len(got) != 1 || got[0].HolderState != HolderPIDReused || got[0].HolderRunning || got[0].Command != "" {
		t.Fatalf("lines %+v", got)
	}

	// The same start time (ps pads the day; Claude Code's record keeps ps's own
	// spacing): the holder still runs.
	home2, r2 := fakeConfig(t)
	h.lstart = "Fri Oct  2 10:11:00 2026"
	w, views = lockWatch(t, home2, r2, clk, h, &alive)
	makeLock(t, r2.CurrentPath(), t0)
	writeRecord(t, r2, recordStart(4242, t0, "Fri Oct 2 10:11:00 2026"))
	w.Tick(context.Background())
	if v := *views; len(v) != 1 || v[0].HolderState != HolderAlive || v[0].Command != "vim notes.txt" {
		t.Fatalf("same start: %+v", v)
	}
	// Seen gone, then running again: the pid was reused, whatever ps says.
	alive = false
	w.Tick(context.Background())
	alive = true
	w.Tick(context.Background())
	w.Stop()
	if got := readLines(t, home2); len(got) != 1 || got[0].HolderState != HolderPIDReused {
		t.Fatalf("gone then back: %+v", got)
	}
}

// Carried from group 1 (b): a lock released and retaken between two ticks, read
// before its new record exists, is not folded into the old sighting: the old line
// stays as it was until the identity settles, then ends "replaced", and the new lock
// is first seen from the tick that first read it without the old identity.
func TestWatchFreezesALockUntilItsIdentitySettles(t *testing.T) {
	home, r := fakeConfig(t)
	clk := clock.NewFake(t0)
	alive := true
	w, _ := lockWatch(t, home, r, clk, &holderRunner{command: "claude"}, &alive)
	ctx := context.Background()
	makeLock(t, r.CurrentPath(), t0)
	writeRecord(t, r, record(4242, t0))
	w.Tick(ctx)
	// Released and retaken: no record yet, a new mtime.
	os.Remove(r.OwnerPath())
	clk.Advance(WatchEvery)
	retaken := clk.Now()
	touch(t, r.CurrentPath(), retaken)
	w.Tick(ctx)
	clk.Advance(WatchEvery)
	writeRecord(t, r, record(4243, retaken))
	w.Tick(ctx)
	w.Stop()
	got := readLines(t, home)
	if len(got) != 2 {
		t.Fatalf("lines %+v, want two", got)
	}
	if got[0].Ended != EndReplaced || got[0].PID != 4242 || !got[0].LastModified.Equal(t0) || !got[0].EndedAt.Equal(retaken) {
		t.Errorf("the old lock's line moved with the new lock: %+v", got[0])
	}
	if got[1].PID != 4243 || !got[1].FirstSeen.Equal(retaken) {
		t.Errorf("the new lock: %+v", got[1])
	}

	// One whose new holder never writes a record counts as another lock once
	// SettleWait has passed.
	home2, r2 := fakeConfig(t)
	w, _ = lockWatch(t, home2, r2, clk, &holderRunner{command: "claude"}, &alive)
	makeLock(t, r2.CurrentPath(), clk.Now())
	writeRecord(t, r2, record(4242, clk.Now()))
	w.Tick(ctx)
	os.Remove(r2.OwnerPath())
	clk.Advance(WatchEvery)
	lost := clk.Now()
	w.Tick(ctx)
	if got := readLines(t, home2); len(got) != 0 {
		t.Fatalf("ended before SettleWait: %+v", got)
	}
	clk.Advance(SettleWait)
	w.Tick(ctx)
	w.Stop()
	got = readLines(t, home2)
	if len(got) != 2 || got[0].Ended != EndReplaced || got[0].PID != 4242 || !got[1].FirstSeen.Equal(lost) || got[1].PID != 0 {
		t.Fatalf("never settled: %+v", got)
	}
}

// Against the real ps: a running holder whose record carries the start time ps gives,
// as Claude Code reads it of itself (LC_ALL=C TZ=UTC ps -o lstart=), still runs.
func TestWatchReadsARealHoldersStartTime(t *testing.T) {
	home, r := fakeConfig(t)
	pid := otherHolder(t)
	cmd := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("no ps here: %v", err)
	}
	w := newWatch(t, home, r, clock.NewFake(t0))
	makeLock(t, r.CurrentPath(), t0)
	writeRecord(t, r, recordStart(pid, t0, strings.TrimSpace(string(out))))
	w.Tick(context.Background())
	w.Stop()
	if got := readLines(t, home); len(got) != 1 || got[0].HolderState != HolderAlive || !strings.Contains(got[0].Command, "sleep 300") {
		t.Fatalf("lines %+v", got)
	}
}

// A gap of more than a few ticks (the machine asleep) breaks a live run: on waking,
// a lock touched again is live from then, not live "for" the whole sleep.
func TestWatchBreaksALiveRunAcrossAGap(t *testing.T) {
	home, r := fakeConfig(t)
	clk := clock.NewFake(t0)
	alive := true
	w, views := lockWatch(t, home, r, clk, &holderRunner{command: "claude"}, &alive)
	makeLock(t, r.CurrentPath(), t0)
	writeRecord(t, r, record(4242, t0))
	w.Tick(context.Background())
	clk.Advance(WatchEvery)
	w.Tick(context.Background())
	// Asleep for 10 minutes; the holder touched it just before the watch woke.
	clk.Advance(10 * time.Minute)
	touch(t, r.CurrentPath(), clk.Now())
	w.Tick(context.Background())
	if v := *views; len(v) != 1 || v[0].State != string(Live) || v[0].LiveSince != clk.Now().UnixMilli() {
		t.Fatalf("after the gap: %+v, want live since %d", v, clk.Now().UnixMilli())
	}
	// A tick a little late (3 intervals) keeps the run.
	since := clk.Now().UnixMilli()
	clk.Advance(liveGapTicks * WatchEvery)
	touch(t, r.CurrentPath(), clk.Now())
	w.Tick(context.Background())
	if v := *views; len(v) != 1 || v[0].LiveSince != since {
		t.Fatalf("a late tick broke the run: %+v", v)
	}
}

// wallJump is t with its wall clock moved on by d and its monotonic reading kept: what
// the panel's clock reads across a sleep, when the monotonic clock stopped (darwin's
// mach_absolute_time, Linux's CLOCK_MONOTONIC) and the wall clock did not. time.Time
// cannot be built so: this sets the wall seconds of time.go's layout (wall's bits
// 30–62 hold seconds since 1885 when its top bit, hasMonotonic, is set), and checks
// the result.
func wallJump(t *testing.T, at time.Time, d time.Duration) time.Time {
	t.Helper()
	p := (*struct {
		wall uint64
		ext  int64
		loc  *time.Location
	})(unsafe.Pointer(&at))
	if p.wall&(1<<63) == 0 {
		t.Skip("no monotonic reading to keep")
	}
	before := at
	p.wall += uint64(d/time.Second) << 30
	if at.Sub(before) != 0 || at.Round(0).Sub(before.Round(0)) != d {
		t.Skipf("time.Time's layout is not as this test knows it: monotonic %v, wall %v", at.Sub(before), at.Round(0).Sub(before.Round(0)))
	}
	return at
}

// The gap that breaks a live run is read from the wall clock: across a sleep the
// monotonic clock stops, so two readings minutes apart on the wall can be seconds
// apart on it, while the alert compares wall times.
func TestWatchGapIsWallClock(t *testing.T) {
	prev := time.Now()
	woke := wallJump(t, prev.Add(2*time.Second), 10*time.Minute)
	if !longGap(prev, woke, WatchEvery) {
		t.Fatalf("a 10-minute sleep (monotonic %v, wall %v) is no gap", woke.Sub(prev), woke.Round(0).Sub(prev.Round(0)))
	}
	if longGap(prev, prev.Add(2*time.Second), WatchEvery) || longGap(time.Time{}, woke, WatchEvery) {
		t.Fatal("a tick on time, or the first tick, counts as a gap")
	}
}

// A slow tick (its ps can take up to 5 s) is no gap: the next tick is measured from
// when the last one finished, not when it began.
func TestWatchSlowTickIsNoGap(t *testing.T) {
	home, r := fakeConfig(t)
	clk := clock.NewFake(t0)
	alive := true
	h := &holderRunner{command: "claude"}
	w, views := lockWatch(t, home, r, clk, h, &alive)
	w.Run = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		clk.Advance(liveGapTicks*WatchEvery + time.Second) // a ps slower than three intervals
		return h.run(ctx, dir, argv)
	}
	makeLock(t, r.CurrentPath(), t0)
	writeRecord(t, r, record(4242, t0))
	w.Tick(context.Background()) // first sight: one ps, slow
	clk.Advance(WatchEvery)
	touch(t, r.CurrentPath(), clk.Now())
	w.Tick(context.Background())
	if v := *views; len(v) != 1 || v[0].LiveSince != t0.UnixMilli() {
		t.Fatalf("the slow tick broke the live run: %+v", v)
	}
}

// Tick itself reads the gap by the wall clock: a clock whose next reading is 10
// minutes on by the wall but 2 s on by the monotonic clock (the machine slept) breaks
// the live run.
func TestWatchTickSeesAWallClockGap(t *testing.T) {
	home, r := fakeConfig(t)
	prev := time.Now()
	woke := wallJump(t, prev.Add(2*time.Second), 10*time.Minute)
	now := prev
	alive := true
	w, views := lockWatch(t, home, r, clock.Func(func() time.Time { return now }), &holderRunner{command: "claude"}, &alive)
	makeLock(t, r.CurrentPath(), prev)
	writeRecord(t, r, record(4242, prev))
	w.Tick(context.Background())
	now = woke
	touch(t, r.CurrentPath(), woke.Round(0))
	w.Tick(context.Background())
	if v := *views; len(v) != 1 || v[0].State != string(Live) || v[0].LiveSince != woke.UnixMilli() {
		t.Fatalf("after the sleep: %+v, want live since %d (the wake), not %d", v, woke.UnixMilli(), prev.UnixMilli())
	}
}
