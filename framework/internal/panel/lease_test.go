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
	// pid 1 is alive and started "T1"; pid 2 is gone; pid 3 is alive but started "T9"
	// (a reused pid); pid 4 is alive with an unreadable start time.
	proc := func(pid int) (bool, string) {
		return map[int]bool{1: true, 3: true, 4: true}[pid], map[int]string{1: "T1", 3: "T9"}[pid]
	}
	now := t0
	holder := LeaseOwner{PID: 1, PStart: "T1", Host: "h", Renewed: now.Unix() - 10, TTL: 60}
	if s, _ := LeaseStale(holder, "h", now, proc); s {
		t.Fatal("a live, renewed holder is stale")
	}
	// Silent for an hour (SIGSTOP, a sleeping laptop) but alive with its own start
	// time: still the holder. The TTL never expires a holder that can be checked.
	silent := holder
	silent.Renewed = now.Unix() - 3600
	if s, why := LeaseStale(silent, "h", now, proc); s {
		t.Fatalf("expired a live, verified holder: %s", why)
	}
	gone := holder
	gone.PID = 2
	if s, why := LeaseStale(gone, "h", now, proc); !s || !strings.Contains(why, "gone") {
		t.Fatal("a dead pid is not stale")
	}
	reused := holder
	reused.PID = 3
	if s, why := LeaseStale(reused, "h", now, proc); !s || !strings.Contains(why, "reused") {
		t.Fatalf("a reused pid is not stale: %v %s", s, why)
	}
	// Unverifiable (no recorded start time, or none readable): the TTL applies.
	noStart := silent
	noStart.PStart = ""
	if s, why := LeaseStale(noStart, "h", now, proc); !s || !strings.Contains(why, "expired") {
		t.Fatal("an unverifiable, unrenewed lease did not expire")
	}
	unreadable := silent
	unreadable.PID = 4
	if s, _ := LeaseStale(unreadable, "h", now, proc); !s {
		t.Fatal("an unreadable start time must fall back to the TTL")
	}
	// Another host: its pid means nothing here; only the TTL counts.
	other := silent
	other.Host = "elsewhere"
	other.PID = 2
	if s, _ := LeaseStale(other, "h", now, proc); !s {
		t.Fatal("another host's expired lease is not stale")
	}
	other.Renewed = now.Unix()
	if s, _ := LeaseStale(other, "h", now, proc); s {
		t.Fatal("judged another host's pid")
	}
	noTTL := noStart
	noTTL.TTL = 0
	if s, _ := LeaseStale(noTTL, "h", now, proc); s {
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
		v := ReadQueue(QueueConfig{ID: "g"}, lock, time.Now(), LiveProc)
		return v.Held && len(v.Waiters) == 2
	})
	v := ReadQueue(QueueConfig{ID: "g"}, lock, time.Now(), LiveProc)
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
	v := ReadQueue(QueueConfig{ID: "g"}, lock, time.Now(), LiveProc)
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
	// Live and verified, an hour past its TTL: the waiter keeps waiting.
	live := plant(me)
	p := startLockRun(t, live, "next", time.Minute, "/bin/sh", "-c", "exit 0")
	time.Sleep(2500 * time.Millisecond)
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
	// No start time recorded (an older writer): the TTL applies.
	unverified := me
	unverified.PStart = ""
	r := startLockRun(t, plant(unverified), "next", time.Minute, "/bin/sh", "-c", "exit 0")
	if code := r.wait(t, 5*time.Second); code != 0 || !strings.Contains(r.stderr.String(), "expired") {
		t.Fatalf("unverifiable: exit %d: %s", code, r.stderr)
	}
}

// The reviewer's case: a real lock-run holder is stopped (SIGSTOP, as a sleeping
// laptop would be) well past its TTL. The waiter must keep waiting, and run only
// after the holder resumes and finishes.
func TestLockRunStoppedHolderKeepsTheLease(t *testing.T) {
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
	waiter := startLockRun(t, lock, "b", time.Second, "/bin/sh", "-c", "echo B-ran >> "+log)
	time.Sleep(4 * time.Second) // four TTLs
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
	dir := t.TempDir()
	lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
	p := startLockRun(t, lock, "a", time.Minute, "/bin/sh", "-c", "echo up >> "+log+"; sleep 30; echo FINISHED >> "+log)
	waitUntil(t, "running", 5*time.Second, func() bool { return lineCount(log) == 1 })
	o, err := readLeaseFile(filepath.Join(lock, ownerFileName))
	if err != nil {
		t.Fatal(err)
	}
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
	dir := t.TempDir()
	lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
	holder := startLockRun(t, lock, "a", time.Minute, "/bin/sh", "-c", "echo held >> "+log+"; sleep 1.5; echo done >> "+log)
	waitUntil(t, "held", 5*time.Second, func() bool { return len(readLog(t, log)) > 0 })
	waiter := startLockRun(t, lock, "b", time.Minute, "/bin/sh", "-c", "echo WAITER-RAN >> "+log)
	var v QueueView
	waitUntil(t, "a waiter", 5*time.Second, func() bool {
		v = ReadQueue(QueueConfig{ID: "g"}, lock, time.Now(), LiveProc)
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

// Defence in depth: a lease directory someone holds flock(2) on is live by the
// kernel's word, even when its owner.json names a dead pid.
func TestFlockedLeaseIsNeverReclaimed(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "gate.lock")
	os.Mkdir(lock, 0o755)
	now := time.Now().Unix()
	writeLeaseFile(filepath.Join(lock, ownerFileName), LeaseOwner{V: 1, Nonce: "00000000000000dd", PID: deadPID(t),
		Host: hostName(), Started: now, Renewed: now})
	unflock := holdFlock(lock)
	if held, _, stale, _ := holderState(lock, hostName(), time.Now(), LiveProc); !held || stale {
		t.Fatal("a flocked lease judged stale")
	}
	unflock()
	if _, _, stale, _ := holderState(lock, hostName(), time.Now(), LiveProc); !stale {
		t.Fatal("without the flock, a dead pid must be stale (the test proves nothing otherwise)")
	}
}
