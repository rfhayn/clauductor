package lease

import (
	"bytes"
	"fmt"
	"io"

	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/types"
	"github.com/creack/pty"
)

// TestMain lets the test binary act as `clauductor lock-run`, so the lease is
// exercised by real, separate processes with real pids.
func TestMain(m *testing.M) {
	if lock := os.Getenv("LOCKRUN_HELPER_LOCK"); lock != "" {
		ttl, _ := time.ParseDuration(os.Getenv("LOCKRUN_HELPER_TTL"))
		var argv []string
		_ = json.Unmarshal([]byte(os.Getenv("LOCKRUN_HELPER_ARGV")), &argv)
		code, err := LockRun(context.Background(), LockRunOptions{Clock: helperClock(), Lock: lock, Lane: os.Getenv("LOCKRUN_HELPER_LANE"),
			TTL: ttl, Poll: 50 * time.Millisecond, Argv: argv})
		if err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
		}
		if f := os.Getenv("LOCKRUN_HELPER_FGFILE"); f != "" {
			// Did lock-run take the terminal back after the command?
			pg, err := tcgetpgrp(0)
			tio, terr := getTermios(0)
			echo := terr == nil && tio.Lflag&syscall.ECHO != 0
			os.WriteFile(f, []byte(fmt.Sprintf("fg=%v echo=%v", err == nil && pg == syscall.Getpgrp(), echo)), 0o644)
		}
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// helperClock is the lock-run helper's clock: the system clock, read
// LOCKRUN_HELPER_SKEW ahead (a waiter that sees a holder's TTL long past without
// waiting for it), with each wait of the waiter loop appended to LOCKRUN_HELPER_TRACE
// (a test asserts after N iterations of that loop instead of sleeping).
func helperClock() clock.Clock {
	clk := clock.System
	if d, err := time.ParseDuration(os.Getenv("LOCKRUN_HELPER_SKEW")); err == nil && d != 0 {
		clk = clock.Func(func() time.Time { return clock.System.Now().Add(d) })
	}
	if trace := os.Getenv("LOCKRUN_HELPER_TRACE"); trace != "" {
		return tracingClock{Clock: clk, trace: trace}
	}
	return clk
}

// tracingClock appends a line to trace for every After: once per iteration of
// lock-run's waiter loop, after that iteration's checks.
type tracingClock struct {
	clock.Clock
	trace string
}

func (c tracingClock) After(d time.Duration) <-chan time.Time {
	if f, err := os.OpenFile(c.trace, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		f.WriteString("wait\n")
		f.Close()
	}
	return c.Clock.After(d)
}

// waitIterations waits until the process tracing into trace has finished n more
// iterations of its wait loop than it had at mark.
func waitIterations(t *testing.T, trace string, mark, n int) {
	t.Helper()
	waitUntil(t, fmt.Sprintf("%d wait-loop iterations", n), 10*time.Second, func() bool { return lineCount(trace) >= mark+n })
}

// syncBuf is a buffer a child process writes while the test reads it.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

type lockProc struct {
	cmd    *exec.Cmd
	stderr *syncBuf
	done   chan int
}

func startLockRun(t *testing.T, lock, lane string, ttl time.Duration, argv ...string) *lockProc {
	t.Helper()
	return startLockRunEnv(t, lock, lane, ttl, nil, argv...)
}

// startLockRunEnv is startLockRun with extra LOCKRUN_HELPER_* settings in env.
func startLockRunEnv(t *testing.T, lock, lane string, ttl time.Duration, env []string, argv ...string) *lockProc {
	t.Helper()
	b, _ := json.Marshal(argv)
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(append(os.Environ(), "LOCKRUN_HELPER_LOCK="+lock, "LOCKRUN_HELPER_LANE="+lane,
		"LOCKRUN_HELPER_TTL="+ttl.String(), "LOCKRUN_HELPER_ARGV="+string(b)), env...)
	p := &lockProc{cmd: cmd, stderr: &syncBuf{}, done: make(chan int, 1)}
	cmd.Stderr = p.stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		err := cmd.Wait()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		p.done <- code
	}()
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return p
}

func (p *lockProc) wait(t *testing.T, d time.Duration) int {
	t.Helper()
	select {
	case c := <-p.done:
		return c
	case <-time.After(d):
		t.Fatalf("lock-run did not exit; stderr: %s", p.stderr)
	}
	return -1
}

func waitUntil(t *testing.T, what string, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func readLog(t *testing.T, p string) []string {
	b, _ := os.ReadFile(p)
	return strings.Fields(string(b))
}

func deadPID(t *testing.T) int {
	t.Helper()
	c := exec.Command("/usr/bin/true")
	if err := c.Run(); err != nil {
		t.Fatal(err)
	}
	pid := c.Process.Pid
	if pidAlive(pid) {
		t.Skip("pid reused already")
	}
	return pid
}

func TestLeaseStale(t *testing.T) {
	t.Parallel()
	// pid 1 is alive and started "T1"; pid 2 is gone; pid 3 is alive but started "T9"
	// (a reused pid); pid 4 is alive with an unreadable start time.
	proc := func(pid int) (bool, string) {
		return map[int]bool{1: true, 3: true, 4: true}[pid], map[int]string{1: "T1", 3: "T9"}[pid]
	}
	now := t0
	holder := LeaseOwner{PID: 1, PStart: "T1", Host: "h", Renewed: now.Unix() - 10, TTL: 60}
	if s, _ := leaseStale(holder, "h", now, proc); s {
		t.Fatal("a live, renewed holder is stale")
	}
	// Silent for an hour (SIGSTOP, a sleeping laptop) but alive with its own start
	// time: still the holder. The TTL never expires a holder that can be checked.
	silent := holder
	silent.Renewed = now.Unix() - 3600
	if s, why := leaseStale(silent, "h", now, proc); s {
		t.Fatalf("expired a live, verified holder: %s", why)
	}
	gone := holder
	gone.PID = 2
	if s, why := leaseStale(gone, "h", now, proc); !s || !strings.Contains(why, "gone") {
		t.Fatal("a dead pid is not stale")
	}
	reused := holder
	reused.PID = 3
	if s, why := leaseStale(reused, "h", now, proc); !s || !strings.Contains(why, "reused") {
		t.Fatalf("a reused pid is not stale: %v %s", s, why)
	}
	// Round 2: an alive pid whose start time cannot be verified (none recorded, none
	// readable, or from another source) is LIVE. Missing data never deletes a lease.
	noStart := silent
	noStart.PStart = ""
	if s, why := leaseStale(noStart, "h", now, proc); s {
		t.Fatalf("an alive, unverifiable holder was expired: %s", why)
	}
	unreadable := silent
	unreadable.PID = 4
	if s, _ := leaseStale(unreadable, "h", now, proc); s {
		t.Fatal("an unreadable start time expired an alive holder")
	}
	otherSource := silent
	otherSource.PStart = "proc:12345"
	if s, _ := leaseStale(otherSource, "h", now, proc); s {
		t.Fatal("a /proc start time was compared with a ps one")
	}
	// The command keeps the record live after the holder dies (lock-run SIGKILLed).
	orphaned := gone
	orphaned.ChildPID, orphaned.ChildPStart = 1, "T1"
	if s, why := leaseStale(orphaned, "h", now, proc); s {
		t.Fatalf("a record whose command still runs was judged stale: %s", why)
	}
	orphaned.ChildPID = 2
	if s, _ := leaseStale(orphaned, "h", now, proc); !s {
		t.Fatal("holder and command both gone must be stale")
	}
	// Another host: its pid means nothing here; only the TTL counts.
	other := silent
	other.Host = "elsewhere"
	other.PID = 2
	if s, _ := leaseStale(other, "h", now, proc); !s {
		t.Fatal("another host's expired lease is not stale")
	}
	other.Renewed = now.Unix()
	if s, _ := leaseStale(other, "h", now, proc); s {
		t.Fatal("judged another host's pid")
	}
	noTTL := other
	noTTL.Renewed, noTTL.TTL = now.Unix()-3600, 0
	if s, _ := leaseStale(noTTL, "h", now, proc); s {
		t.Fatal("ttl 0 means no expiry")
	}
}

// Two processes want the gate at once: the second waits, then runs; never both.
func TestLockRunTwoProcessesQueue(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lock, log, release := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log"), filepath.Join(dir, "release")
	body := func(n string) []string {
		return []string{"/bin/sh", "-c", "echo " + n + "-start >> " + log + "; sleep 0.2; echo " + n + "-end >> " + log}
	}
	// A holds until the test releases it, so B and C are both queued behind it
	// however slowly they start.
	a := startLockRun(t, lock, "lane-a", time.Minute, "/bin/sh", "-c",
		"echo A-start >> "+log+"; while [ ! -e "+release+" ]; do sleep 0.02; done; echo A-end >> "+log)
	waitUntil(t, "A holds the lease", 5*time.Second, func() bool { return len(readLog(t, log)) > 0 })
	// The queue orders by arrival (the waiter file's name): C starts only once B's
	// waiter file is there, so B arrived first.
	b := startLockRun(t, lock, "lane-b", time.Minute, body("B")...)
	waitUntil(t, "B's waiter file", 5*time.Second, func() bool {
		v := ReadQueue(types.QueueConfig{ID: "g"}, lock, time.Now(), LiveProc)
		return len(v.Waiters) == 1 && v.Waiters[0].Lane == "lane-b"
	})
	c := startLockRun(t, lock, "lane-c", time.Minute, body("C")...)
	// B and C are both queued while A holds, in arrival order, and the panel sees it.
	waitUntil(t, "two waiters", 5*time.Second, func() bool {
		v := ReadQueue(types.QueueConfig{ID: "g"}, lock, time.Now(), LiveProc)
		return v.Held && len(v.Waiters) == 2
	})
	v := ReadQueue(types.QueueConfig{ID: "g"}, lock, time.Now(), LiveProc)
	if v.Holder == nil || v.Holder.Lane != "lane-a" || v.Waiters[0].Lane != "lane-b" || v.Waiters[1].Lane != "lane-c" {
		t.Fatalf("queue view %+v", v)
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []*lockProc{a, b, c} {
		if code := p.wait(t, 10*time.Second); code != 0 {
			t.Fatalf("exit %d: %s", code, p.stderr)
		}
	}
	want := "A-start A-end B-start B-end C-start C-end"
	if got := strings.Join(readLog(t, log), " "); got != want {
		t.Fatalf("interleaved or out of order:\n got %s\nwant %s", got, want)
	}
	if !strings.Contains(b.stderr.String(), "waiting for gate.lock") {
		t.Fatalf("B did not say it waited: %s", b.stderr)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatal("lease not released")
	}
}

// A holder whose pid is gone is reclaimed by the next waiter.
func TestLockRunReclaimsDeadHolder(t *testing.T) {
	t.Parallel()
	lock := filepath.Join(t.TempDir(), "gate.lock")
	os.Mkdir(lock, 0o755)
	now := time.Now().Unix()
	writeLeaseFile(filepath.Join(lock, ownerFileName), LeaseOwner{V: 1, Nonce: "00000000000000aa", PID: deadPID(t),
		Host: hostName(), Lane: "crashed", Started: now, Renewed: now, TTL: 3600})
	v := ReadQueue(types.QueueConfig{ID: "g"}, lock, time.Now(), LiveProc)
	if v.Holder == nil || v.Holder.Alive || !strings.Contains(v.HolderNote, "stale") {
		t.Fatalf("the panel must show the dead holder as stale: %+v", v)
	}
	p := startLockRun(t, lock, "next", time.Minute, "/bin/sh", "-c", "exit 7")
	if code := p.wait(t, 5*time.Second); code != 7 {
		t.Fatalf("exit %d (want the command's 7): %s", code, p.stderr)
	}
	if !strings.Contains(p.stderr.String(), "reclaiming") || !strings.Contains(p.stderr.String(), "is gone") {
		t.Fatalf("no reclaim: %s", p.stderr)
	}
}

// A same-host holder is judged by its process: a live one with its own start time
// is never reclaimed however long it is silent; a reused pid, or an unverifiable
// record past its TTL, is.
func TestLockRunLeaseTTL(t *testing.T) {
	t.Parallel()
	plant := func(o LeaseOwner) string {
		lock := filepath.Join(t.TempDir(), "gate.lock")
		os.Mkdir(lock, 0o755)
		writeLeaseFile(filepath.Join(lock, ownerFileName), o)
		return lock
	}
	old := time.Now().Unix() - 3600
	me := LeaseOwner{V: 1, Nonce: "00000000000000bb", PID: os.Getpid(), PStart: ProcStart(os.Getpid()), Host: hostName(),
		Started: old, Renewed: old, TTL: 2}
	if me.PStart == "" {
		t.Fatal("cannot read this process's start time")
	}
	// Live and verified, an hour past its TTL: the waiter keeps waiting, however many
	// times it looks.
	live := plant(me)
	trace := filepath.Join(t.TempDir(), "trace")
	p := startLockRunEnv(t, live, "next", time.Minute, []string{"LOCKRUN_HELPER_TRACE=" + trace}, "/bin/sh", "-c", "exit 0")
	waitIterations(t, trace, 0, 5)
	select {
	case code := <-p.done:
		t.Fatalf("took a live holder's lease (exit %d): %s", code, p.stderr)
	default:
	}
	p.cmd.Process.Kill()
	// A reused pid (same pid, another start time): reclaimed.
	reused := me
	reused.PStart = "Thu Jan 1 00:00:00 1970"
	q := startLockRun(t, plant(reused), "next", time.Minute, "/bin/sh", "-c", "exit 0")
	if code := q.wait(t, 5*time.Second); code != 0 || !strings.Contains(q.stderr.String(), "reused") {
		t.Fatalf("reused pid: exit %d: %s", code, q.stderr)
	}
	// No start time recorded, pid alive: live (round 2: missing data never reclaims).
	unverified := me
	unverified.PStart = ""
	trace = filepath.Join(t.TempDir(), "trace")
	r := startLockRunEnv(t, plant(unverified), "next", time.Minute, []string{"LOCKRUN_HELPER_TRACE=" + trace}, "/bin/sh", "-c", "exit 0")
	waitIterations(t, trace, 0, 5)
	select {
	case code := <-r.done:
		t.Fatalf("reclaimed an alive holder with no start time (exit %d): %s", code, r.stderr)
	default:
	}
	r.cmd.Process.Kill()
}

// The reviewer's case: a real lock-run holder is stopped (SIGSTOP, as a sleeping
// laptop would be) well past its TTL. The waiter must keep waiting, and run only
// after the holder resumes and finishes.
func TestLockRunStoppedHolderKeepsTheLease(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
	holder := startLockRun(t, lock, "a", time.Second, "/bin/sh", "-c", "echo A-start >> "+log+"; sleep 1; echo A-end >> "+log)
	waitUntil(t, "held", 5*time.Second, func() bool { return lineCount(log) == 1 })
	syscall.Kill(holder.cmd.Process.Pid, syscall.SIGSTOP)
	// Stop the shell too, so nothing renews and nothing finishes.
	if o, err := readLeaseFile(filepath.Join(lock, ownerFileName)); err == nil {
		_ = o
	}
	out, _ := exec.Command("/usr/bin/pgrep", "-P", strconv.Itoa(holder.cmd.Process.Pid)).Output()
	for _, f := range strings.Fields(string(out)) {
		pid, _ := strconv.Atoi(f)
		syscall.Kill(pid, syscall.SIGSTOP)
		defer syscall.Kill(pid, syscall.SIGCONT)
	}
	// The waiter reads the clock an hour ahead: the holder's 1 s TTL is long past
	// at every look it takes.
	trace := filepath.Join(dir, "trace")
	waiter := startLockRunEnv(t, lock, "b", time.Second, []string{"LOCKRUN_HELPER_TRACE=" + trace, "LOCKRUN_HELPER_SKEW=1h"},
		"/bin/sh", "-c", "echo B-ran >> "+log)
	waitIterations(t, trace, 0, 5)
	if lineCount(log) != 1 {
		t.Fatalf("the waiter ran while the stopped holder held the lease: %v", readLog(t, log))
	}
	syscall.Kill(holder.cmd.Process.Pid, syscall.SIGCONT)
	for _, f := range strings.Fields(string(out)) {
		pid, _ := strconv.Atoi(f)
		syscall.Kill(pid, syscall.SIGCONT)
	}
	if code := holder.wait(t, 10*time.Second); code != 0 {
		t.Fatalf("holder exit %d: %s", code, holder.stderr)
	}
	if code := waiter.wait(t, 10*time.Second); code != 0 {
		t.Fatalf("waiter exit %d: %s", code, waiter.stderr)
	}
	if got := strings.Join(readLog(t, log), " "); got != "A-start A-end B-ran" {
		t.Fatalf("order %q", got)
	}
}

// If lock-run finds its lease gone while the command runs, it stops the command and
// exits non-zero instead of letting two gates finish.
func TestLockRunStopsTheCommandWhenItsLeaseIsLost(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
	p := startLockRun(t, lock, "a", time.Minute, "/bin/sh", "-c", "echo up >> "+log+"; sleep 30; echo FINISHED >> "+log)
	// lock-run rewrites owner.json once, after the command starts, to add the
	// command's pid. Wait for that write: a rewrite racing it would be overwritten,
	// and the lease would never look lost. After it only the renewal writes (every
	// ttl/3, a minute from now), and it keeps the nonce it reads.
	var o LeaseOwner
	waitUntil(t, "running, with the command recorded", 5*time.Second, func() bool {
		var err error
		o, err = readLeaseFile(filepath.Join(lock, ownerFileName))
		return lineCount(log) == 1 && err == nil && o.ChildPID > 0
	})
	o.Nonce = "00000000000000ff" // someone else's lease now
	writeLeaseFile(filepath.Join(lock, ownerFileName), o)
	if code := p.wait(t, 10*time.Second); code != ExitLeaseLost {
		t.Fatalf("exit %d, want %d: %s", code, ExitLeaseLost, p.stderr)
	}
	if strings.Contains(strings.Join(readLog(t, log), " "), "FINISHED") {
		t.Fatal("the command finished without its lease")
	}
	if _, err := os.Stat(lock); err != nil {
		t.Fatal("lock-run removed a lease that is not its own")
	}
}

// One Ctrl-C reaches the whole foreground process group from the terminal; the
// command must see it once, not once from the terminal and again from lock-run
// (the reviewer saw INT twice). lock-run's own group gets it; the command's own
// group gets it once, from lock-run.
func TestLockRunDoesNotRepeatCtrlC(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
	pgFile := filepath.Join(dir, "pgid")
	b, _ := json.Marshal([]string{"/bin/sh", "-c", "ps -o pgid= -p $$ > " + pgFile + "; trap 'echo INT >> " + log + "' INT; echo up >> " + log +
		"; i=0; while [ $i -lt 30 ]; do sleep 0.05; i=$((i+1)); done"})
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "LOCKRUN_HELPER_LOCK="+lock, "LOCKRUN_HELPER_TTL=1m", "LOCKRUN_HELPER_ARGV="+string(b))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // a group of its own, like a terminal job
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "running", 5*time.Second, func() bool { return lineCount(log) == 1 })
	syscall.Kill(-cmd.Process.Pid, syscall.SIGINT) // the terminal's Ctrl-C: the whole group
	cmd.Wait()
	if n := strings.Count(strings.Join(readLog(t, log), " "), "INT"); n != 1 {
		t.Fatalf("the command saw %d INT, want 1: %v", n, readLog(t, log))
	}
	// The mechanism, deterministically: two INTs sent microseconds apart merge into one
	// pending signal, so a count alone can miss a double delivery. The command must
	// run in a process group other than lock-run's (whose group the terminal signals).
	pg, _ := os.ReadFile(pgFile)
	if strings.TrimSpace(string(pg)) == strconv.Itoa(cmd.Process.Pid) {
		t.Fatalf("the command shares lock-run's process group %s: a Ctrl-C reaches it twice", strings.TrimSpace(string(pg)))
	}
}

// A lock directory with no owner.json is a holder still starting, until the grace.
func TestLockRunOwnerlessLock(t *testing.T) {
	t.Parallel()
	lock := filepath.Join(t.TempDir(), "gate.lock")
	os.Mkdir(lock, 0o755)
	if held, _, stale, _ := holderState(lock, hostName(), time.Now(), LiveProc); !held || stale {
		t.Fatal("a fresh ownerless lock is stale")
	}
	if held, _, stale, _ := holderState(lock, hostName(), time.Now().Add(ownerGrace), LiveProc); !held || !stale {
		t.Fatal("an old ownerless lock is not stale")
	}
}

// The panel's CANCEL ends a wait, never the holder; the holder cannot be cancelled.
func TestLockRunCancelWait(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
	holder := startLockRun(t, lock, "a", time.Minute, "/bin/sh", "-c", "echo held >> "+log+"; sleep 1.5; echo done >> "+log)
	waitUntil(t, "held", 5*time.Second, func() bool { return len(readLog(t, log)) > 0 })
	waiter := startLockRun(t, lock, "b", time.Minute, "/bin/sh", "-c", "echo WAITER-RAN >> "+log)
	var v types.QueueView
	waitUntil(t, "a waiter", 5*time.Second, func() bool {
		v = ReadQueue(types.QueueConfig{ID: "g"}, lock, time.Now(), LiveProc)
		return len(v.Waiters) == 1
	})
	if err := CancelWait(lock, v.Holder.Nonce); err == nil || !strings.Contains(err.Error(), "never stops the holder") {
		t.Fatalf("cancelling the holder must be refused as the holder: %v", err)
	}
	if err := CancelWait(lock, "../../etc/passwd"); err == nil {
		t.Fatal("a path as a waiter id")
	}
	if err := CancelWait(lock, v.Waiters[0].Nonce); err != nil {
		t.Fatal(err)
	}
	if code := waiter.wait(t, 5*time.Second); code != ExitCancelled {
		t.Fatalf("waiter exit %d: %s", code, waiter.stderr)
	}
	if code := holder.wait(t, 5*time.Second); code != 0 {
		t.Fatalf("holder exit %d", code)
	}
	if got := strings.Join(readLog(t, log), " "); got != "held done" {
		t.Fatalf("log %q: the holder must finish and the cancelled waiter never run", got)
	}
}

// A signal to lock-run is forwarded to the command, and the lease is released.
func TestLockRunForwardsSignalAndReleases(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
	p := startLockRun(t, lock, "a", time.Minute, "/bin/sh", "-c", "echo up >> "+log+"; trap 'exit 9' TERM; while :; do sleep 0.05; done")
	waitUntil(t, "running", 5*time.Second, func() bool { return len(readLog(t, log)) > 0 })
	p.cmd.Process.Signal(syscall.SIGTERM)
	if code := p.wait(t, 5*time.Second); code != 9 {
		t.Fatalf("exit %d, want the command's 9", code)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatal("lease not released after a signal")
	}
}

func TestLockRunReentry(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "gate.lock")
	abs, _ := filepath.Abs(lock)
	t.Setenv("CLAUDUCTOR_LOCK_HELD", abs)
	var out bytes.Buffer
	code, err := LockRun(context.Background(), LockRunOptions{Clock: clock.System, Lock: lock, Argv: []string{"/bin/sh", "-c", "echo $CLAUDUCTOR_LOCK_HELD"}, Stdout: &out})
	if err != nil || code != 0 || strings.TrimSpace(out.String()) != abs {
		t.Fatalf("re-entry: %d %v %q", code, err, out.String())
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatal("re-entry took the lock again")
	}
}

// FIFO, deterministically: a live waiter that arrived earlier goes first, even when
// the lease is free. Only the first live waiter may take it.
func TestLockRunIsFIFO(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
	os.MkdirAll(waitersDir(lock), 0o755)
	now := time.Now()
	// An earlier waiter, alive (this test process), that has not taken its turn yet.
	early := filepath.Join(waitersDir(lock), strconv.FormatInt(now.Add(-time.Second).UnixNano(), 10)+"-00000000000000cc.json")
	writeLeaseFile(early, LeaseOwner{V: 1, Nonce: "00000000000000cc", PID: os.Getpid(), Host: hostName(), Lane: "first",
		Started: now.Unix(), Renewed: now.Unix()})
	trace := filepath.Join(dir, "trace")
	p := startLockRunEnv(t, lock, "second", time.Minute, []string{"LOCKRUN_HELPER_TRACE=" + trace}, "/bin/sh", "-c", "echo ran >> "+log)
	waitIterations(t, trace, 0, 5)
	if lineCount(log) != 0 {
		t.Fatal("jumped the queue: ran while an earlier live waiter was ahead")
	}
	if !strings.Contains(p.stderr.String(), "1 ahead") {
		t.Fatalf("did not report its place: %s", p.stderr)
	}
	os.Remove(early) // the earlier waiter leaves
	if code := p.wait(t, 5*time.Second); code != 0 || lineCount(log) != 1 {
		t.Fatalf("exit %d, ran %d", code, lineCount(log))
	}
}

// Round 3: liveness is pid and start time only, the same rule as the shell. A
// flock someone still holds (an orphan that inherited the fd) never keeps a record
// whose holder and command are dead.
func TestFlockNeverKeepsADeadRecordLive(t *testing.T) {
	t.Parallel()
	lock := filepath.Join(t.TempDir(), "gate.lock")
	os.Mkdir(lock, 0o755)
	now := time.Now().Unix()
	writeLeaseFile(filepath.Join(lock, ownerFileName), LeaseOwner{V: 1, Nonce: "00000000000000dd", PID: deadPID(t),
		Host: hostName(), Started: now, Renewed: now})
	_, unflock := holdFlock(lock)
	defer unflock()
	if held, _, stale, _ := holderState(lock, hostName(), time.Now(), LiveProc); !held || !stale {
		t.Fatal("a flock kept a dead holder's record live")
	}
	if v := ReadQueue(types.QueueConfig{ID: "g"}, lock, time.Now(), LiveProc); !strings.Contains(v.HolderNote, "flock") {
		t.Fatalf("the panel does not mention the orphaned flock: %q", v.HolderNote)
	}
}

// startLockRunPTY runs the lock-run helper on a real pty as a session leader in
// the terminal's foreground, the way an interactive `bash run-local.sh` runs it.
func startLockRunPTY(t *testing.T, lock string, extraEnv []string, argv ...string) (*exec.Cmd, *os.File, chan int) {
	t.Helper()
	b, _ := json.Marshal(argv)
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(append(os.Environ(), "LOCKRUN_HELPER_LOCK="+lock, "LOCKRUN_HELPER_TTL=1m",
		"LOCKRUN_HELPER_ARGV="+string(b)), extraEnv...)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	go io.Copy(io.Discard, ptmx) // a pty nobody reads would block the writer
	done := make(chan int, 1)
	go func() {
		err := cmd.Wait()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		done <- code
	}()
	t.Cleanup(func() { cmd.Process.Kill(); ptmx.Close() })
	return cmd, ptmx, done
}

// Round 2 (HIGH): in its own process group, a command that touches the terminal
// (stty) was stopped by SIGTTOU forever while holding the lease. On a terminal the
// command's group must be the foreground group, and lock-run must take the
// terminal back when it ends.
func TestLockRunCommandCanUseTheTerminal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lock, log, fg := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log"), filepath.Join(dir, "fg")
	// The command turns echo off and does not turn it back on (as a killed prompt
	// would leave it): lock-run must restore the terminal's settings.
	_, _, done := startLockRunPTY(t, lock, []string{"LOCKRUN_HELPER_FGFILE=" + fg},
		"/bin/sh", "-c", "stty -echo && echo stty-ok >> "+log)
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the command is stuck: stty was stopped (SIGTTOU) while holding the lease")
	}
	if strings.Join(readLog(t, log), " ") != "stty-ok" {
		t.Fatalf("log %v", readLog(t, log))
	}
	if b, _ := os.ReadFile(fg); string(b) != "fg=true echo=true" {
		t.Fatalf("lock-run did not take the terminal back with its settings: %q", b)
	}
}

// Ctrl-C typed on the terminal reaches the command's (foreground) group once.
func TestLockRunCtrlCOnATerminalReachesTheCommandOnce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
	_, ptmx, done := startLockRunPTY(t, lock, nil, "/bin/sh", "-c",
		"trap 'echo INT >> "+log+"' INT; echo up >> "+log+"; i=0; while [ $i -lt 40 ]; do sleep 0.05; i=$((i+1)); done")
	waitUntil(t, "running", 5*time.Second, func() bool { return lineCount(log) == 1 })
	ptmx.Write([]byte{3}) // ^C
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("did not finish")
	}
	if n := strings.Count(strings.Join(readLog(t, log), " "), "INT"); n != 1 {
		t.Fatalf("the command saw %d INT, want 1", n)
	}
}

// The TERM workaround: started as TERM=dumb CLAUDUCTOR_TERM=<real>, the command
// gets the real TERM back.
func TestLockRunRestoresTheRealTERM(t *testing.T) {
	t.Parallel()
	env := childEnv([]string{"PATH=/bin", "TERM=dumb", "CLAUDUCTOR_TERM=xterm-256color", "CLAUDUCTOR_TERM_SET=1"})
	if strings.Join(env, " ") != "PATH=/bin TERM=xterm-256color" {
		t.Fatalf("env %v", env)
	}
	// Round 3: unset stays unset; set-but-empty stays set and empty.
	if env := childEnv([]string{"TERM=dumb", "CLAUDUCTOR_TERM=", "CLAUDUCTOR_TERM_SET="}); len(env) != 0 {
		t.Fatalf("an unset TERM must stay unset: %v", env)
	}
	if env := childEnv([]string{"TERM=dumb", "CLAUDUCTOR_TERM=", "CLAUDUCTOR_TERM_SET=1"}); strings.Join(env, " ") != "TERM=" {
		t.Fatalf("a set, empty TERM must stay set: %v", env)
	}
	if env := childEnv([]string{"TERM=vt100"}); env[0] != "TERM=vt100" {
		t.Fatal("no workaround: TERM untouched")
	}
}
