package claudecall

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
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

func record(pid int, birth time.Time) string {
	return fmt.Sprintf(`{"pid":%d,"procStart":"x","pidDomain":"d","pidSpace":"s","lockBirthtimeMs":%d}`, pid, birth.UnixMilli())
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
