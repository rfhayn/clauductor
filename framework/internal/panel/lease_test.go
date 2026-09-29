package panel

import (
	"bytes"
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
)

// TestMain lets the test binary act as `clauductor lock-run`, so the lease is
// exercised by real, separate processes with real pids.
func TestMain(m *testing.M) {
	if lock := os.Getenv("LOCKRUN_HELPER_LOCK"); lock != "" {
		ttl, _ := time.ParseDuration(os.Getenv("LOCKRUN_HELPER_TTL"))
		var argv []string
		_ = json.Unmarshal([]byte(os.Getenv("LOCKRUN_HELPER_ARGV")), &argv)
		code, err := LockRun(context.Background(), LockRunOptions{Lock: lock, Lane: os.Getenv("LOCKRUN_HELPER_LANE"),
			TTL: ttl, Poll: 50 * time.Millisecond, Argv: argv})
		if err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
		}
		os.Exit(code)
	}
	os.Exit(m.Run())
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
	b, _ := json.Marshal(argv)
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "LOCKRUN_HELPER_LOCK="+lock, "LOCKRUN_HELPER_LANE="+lane,
		"LOCKRUN_HELPER_TTL="+ttl.String(), "LOCKRUN_HELPER_ARGV="+string(b))
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
	if PIDAlive(pid) {
		t.Skip("pid reused already")
	}
	return pid
}

func TestLeaseStale(t *testing.T) {
	alive := func(pid int) bool { return pid == 1 }
	now := t0
	fresh := LeaseOwner{PID: 1, Host: "h", Renewed: now.Unix() - 10, TTL: 60}
	if s, _ := LeaseStale(fresh, "h", now, alive); s {
		t.Fatal("a live, renewed holder is stale")
	}
	gone := fresh
	gone.PID = 2
	if s, why := LeaseStale(gone, "h", now, alive); !s || !strings.Contains(why, "gone") {
		t.Fatal("a dead pid is not stale")
	}
	// A pid on another host cannot be checked: only its TTL counts.
	if s, _ := LeaseStale(gone, "other", now, alive); s {
		t.Fatal("judged another host's pid")
	}
	expired := fresh
	expired.Renewed = now.Unix() - 61
	if s, why := LeaseStale(expired, "h", now, alive); !s || !strings.Contains(why, "expired") {
		t.Fatal("an expired lease with a live (maybe reused) pid is not stale")
	}
	noTTL := expired
	noTTL.TTL = 0
	if s, _ := LeaseStale(noTTL, "h", now, alive); s {
		t.Fatal("ttl 0 means no expiry")
	}
}

// Two processes want the gate at once: the second waits, then runs; never both.
func TestLockRunTwoProcessesQueue(t *testing.T) {
	dir := t.TempDir()
	lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
	body := func(n string) []string {
		return []string{"/bin/sh", "-c", "echo " + n + "-start >> " + log + "; sleep 0.6; echo " + n + "-end >> " + log}
	}
	a := startLockRun(t, lock, "lane-a", time.Minute, body("A")...)
	waitUntil(t, "A holds the lease", 5*time.Second, func() bool { return len(readLog(t, log)) > 0 })
	b := startLockRun(t, lock, "lane-b", time.Minute, body("B")...)
	c := startLockRun(t, lock, "lane-c", time.Minute, body("C")...)
	// B and C are both queued while A holds, in arrival order, and the panel sees it.
	waitUntil(t, "two waiters", 5*time.Second, func() bool {
		v := ReadQueue(QueueConfig{ID: "g"}, lock, time.Now(), PIDAlive)
		return v.Held && len(v.Waiters) == 2
	})
	v := ReadQueue(QueueConfig{ID: "g"}, lock, time.Now(), PIDAlive)
	if v.Holder == nil || v.Holder.Lane != "lane-a" || v.Waiters[0].Lane != "lane-b" || v.Waiters[1].Lane != "lane-c" {
		t.Fatalf("queue view %+v", v)
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
	lock := filepath.Join(t.TempDir(), "gate.lock")
	os.Mkdir(lock, 0o755)
	now := time.Now().Unix()
	writeLeaseFile(filepath.Join(lock, ownerFileName), LeaseOwner{V: 1, Nonce: "00000000000000aa", PID: deadPID(t),
		Host: hostName(), Lane: "crashed", Started: now, Renewed: now, TTL: 3600})
	v := ReadQueue(QueueConfig{ID: "g"}, lock, time.Now(), PIDAlive)
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

// A live pid whose lease was not renewed (hung, or a reused pid) is reclaimed after
// its TTL; while the lease is fresh, a live holder is never touched.
func TestLockRunLeaseTTL(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "gate.lock")
	os.Mkdir(lock, 0o755)
	now := time.Now().Unix()
	holder := LeaseOwner{V: 1, Nonce: "00000000000000bb", PID: os.Getpid(), Host: hostName(), Started: now, Renewed: now, TTL: 2}
	writeLeaseFile(filepath.Join(lock, ownerFileName), holder)
	start := time.Now()
	p := startLockRun(t, lock, "next", time.Minute, "/bin/sh", "-c", "exit 0")
	if code := p.wait(t, 10*time.Second); code != 0 {
		t.Fatalf("exit %d: %s", code, p.stderr)
	}
	if time.Since(start) < 1500*time.Millisecond {
		t.Fatalf("reclaimed a fresh lease after %v", time.Since(start))
	}
	if !strings.Contains(p.stderr.String(), "expired") {
		t.Fatalf("not reclaimed for expiry: %s", p.stderr)
	}
	if !PIDAlive(os.Getpid()) {
		t.Fatal("unreachable")
	}
}

// A lock directory with no owner.json is a holder still starting, until the grace.
func TestLockRunOwnerlessLock(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "gate.lock")
	os.Mkdir(lock, 0o755)
	if held, _, stale, _ := holderState(lock, hostName(), time.Now(), PIDAlive); !held || stale {
		t.Fatal("a fresh ownerless lock is stale")
	}
	if held, _, stale, _ := holderState(lock, hostName(), time.Now().Add(ownerGrace), PIDAlive); !held || !stale {
		t.Fatal("an old ownerless lock is not stale")
	}
}

// The panel's CANCEL ends a wait, never the holder; the holder cannot be cancelled.
func TestLockRunCancelWait(t *testing.T) {
	dir := t.TempDir()
	lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
	holder := startLockRun(t, lock, "a", time.Minute, "/bin/sh", "-c", "echo held >> "+log+"; sleep 1.5; echo done >> "+log)
	waitUntil(t, "held", 5*time.Second, func() bool { return len(readLog(t, log)) > 0 })
	waiter := startLockRun(t, lock, "b", time.Minute, "/bin/sh", "-c", "echo WAITER-RAN >> "+log)
	var v QueueView
	waitUntil(t, "a waiter", 5*time.Second, func() bool {
		v = ReadQueue(QueueConfig{ID: "g"}, lock, time.Now(), PIDAlive)
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
	code, err := LockRun(context.Background(), LockRunOptions{Lock: lock, Argv: []string{"/bin/sh", "-c", "echo $CLAUDUCTOR_LOCK_HELD"}, Stdout: &out})
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
	dir := t.TempDir()
	lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
	os.MkdirAll(waitersDir(lock), 0o755)
	now := time.Now()
	// An earlier waiter, alive (this test process), that has not taken its turn yet.
	early := filepath.Join(waitersDir(lock), strconv.FormatInt(now.Add(-time.Second).UnixNano(), 10)+"-00000000000000cc.json")
	writeLeaseFile(early, LeaseOwner{V: 1, Nonce: "00000000000000cc", PID: os.Getpid(), Host: hostName(), Lane: "first",
		Started: now.Unix(), Renewed: now.Unix()})
	p := startLockRun(t, lock, "second", time.Minute, "/bin/sh", "-c", "echo ran >> "+log)
	time.Sleep(1200 * time.Millisecond)
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
