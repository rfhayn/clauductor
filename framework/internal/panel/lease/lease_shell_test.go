package lease

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/types"
)

// The plain-shell protocol in docs/panel.md is what a project without clauductor
// (a project's own run-local.sh) runs. The test runs THAT text, extracted from the
// page, against lock-run, so the documentation cannot drift from the protocol.
func docLeaseSh(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: runs lock-run or lease.sh as real processes")
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "panel.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	i := strings.Index(s, "<!-- lease.sh begin -->\n```sh\n")
	j := strings.Index(s, "```\n<!-- lease.sh end -->")
	if i < 0 || j < i {
		t.Fatal("docs/panel.md has no lease.sh block between its markers")
	}
	p := filepath.Join(t.TempDir(), "lease.sh")
	os.WriteFile(p, []byte(s[i+len("<!-- lease.sh begin -->\n```sh\n"):j]), 0o644)
	return p
}

// startShellLease runs `lease_run <lock> <lane> sh -c <body>` from the documented
// shell, under bash with set -euo pipefail as run-local.sh would.
func startShellLease(t *testing.T, leaseSh, lock, lane, body string) *lockProc {
	t.Helper()
	return startShellLeaseEnv(t, leaseSh, lock, lane, body, nil)
}

// startShellLeaseEnv is startShellLease with env added to the shell's environment.
func startShellLeaseEnv(t *testing.T, leaseSh, lock, lane, body string, env []string) *lockProc {
	t.Helper()
	script := "set -euo pipefail; . " + leaseSh + "; lease_run \"$1\" \"$2\" sh -c \"$3\""
	cmd := exec.Command("bash", "-c", script, "lease", lock, lane, body)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
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

func TestShellLeaseInteroperatesWithLockRun(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	leaseSh := docLeaseSh(t)
	body := func(log, n string) string {
		return "echo " + n + "-start >> " + log + "; sleep 1; echo " + n + "-end >> " + log
	}

	t.Run("shell holder, lock-run waiter past its TTL", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
		sh := startShellLease(t, leaseSh, lock, "shell", "echo S-start >> "+log+"; sleep 3; echo S-end >> "+log)
		waitUntil(t, "the shell holds it", 5*time.Second, func() bool { return lineCount(log) == 1 })
		o, err := readLeaseFile(filepath.Join(lock, ownerFileName))
		if err != nil || o.PStart == "" || o.PStart != ProcStart(o.PID) {
			t.Fatalf("the shell's owner.json does not parse or its pstart differs from Go's: %+v %v (Go: %q)", o, err, ProcStart(o.PID))
		}
		lr := startLockRun(t, lock, "go", time.Second, "/bin/sh", "-c", body(log, "G"))
		if code := sh.wait(t, 10*time.Second); code != 0 {
			t.Fatalf("shell exit %d: %s", code, sh.stderr)
		}
		if code := lr.wait(t, 10*time.Second); code != 0 {
			t.Fatalf("lock-run exit %d: %s", code, lr.stderr)
		}
		if got := strings.Join(readLog(t, log), " "); got != "S-start S-end G-start G-end" {
			t.Fatalf("order %q", got)
		}
	})

	t.Run("lock-run holder stopped, shell waiter keeps waiting", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
		lr := startLockRun(t, lock, "go", time.Second, "/bin/sh", "-c", body(log, "G"))
		waitUntil(t, "lock-run holds it", 5*time.Second, func() bool { return lineCount(log) == 1 })
		// Stop lock-run and its command group: no renewal, far past the 1 s TTL.
		syscall.Kill(lr.cmd.Process.Pid, syscall.SIGSTOP)
		pg, _ := exec.Command("/bin/ps", "-o", "pgid=", "-p", firstChild(t, lr.cmd.Process.Pid)).Output()
		pgid, err := strconv.Atoi(strings.TrimSpace(string(pg)))
		if err != nil {
			t.Fatalf("the command's process group %q: %v", pg, err)
		}
		// kill(2) itself: procps' kill(1) reads "-<pgid>" as a signal, not a group.
		syscall.Kill(-pgid, syscall.SIGSTOP)
		// The shell judges the TTL by `date +%s`, in whole seconds: three iterations of
		// its 1 s wait loop put the holder's 1 s TTL at least a second in the past.
		bin := t.TempDir()
		trace := tracedSleep(t, bin)
		sh := startShellLeaseEnv(t, leaseSh, lock, "shell", body(log, "S"), []string{"PATH=" + bin + ":" + os.Getenv("PATH")})
		waitUntil(t, "three iterations of the shell's wait loop", 10*time.Second, func() bool { return lineCount(trace) >= 3 })
		if lineCount(log) != 1 {
			t.Fatalf("the shell took a stopped holder's lease: %v", readLog(t, log))
		}
		syscall.Kill(-pgid, syscall.SIGCONT)
		syscall.Kill(lr.cmd.Process.Pid, syscall.SIGCONT)
		if code := lr.wait(t, 10*time.Second); code != 0 {
			t.Fatalf("lock-run exit %d: %s", code, lr.stderr)
		}
		if code := sh.wait(t, 10*time.Second); code != 0 {
			t.Fatalf("shell exit %d: %s", code, sh.stderr)
		}
		if got := strings.Join(readLog(t, log), " "); got != "G-start G-end S-start S-end" {
			t.Fatalf("order %q", got)
		}
	})

	t.Run("shell reclaims a dead holder and releases", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
		os.Mkdir(lock, 0o755)
		pid := deadPID(t)
		now := time.Now().Unix()
		writeLeaseFile(filepath.Join(lock, ownerFileName), LeaseOwner{V: 1, Nonce: "00000000000000ee", PID: pid,
			PStart: "Thu Jan 1 00:00:00 1970", Host: hostName(), Started: now, Renewed: now})
		sh := startShellLease(t, leaseSh, lock, "shell", "echo ran >> "+log+"; exit 4")
		if code := sh.wait(t, 10*time.Second); code != 4 {
			t.Fatalf("exit %d (want the command's 4): %s", code, sh.stderr)
		}
		if lineCount(log) != 1 || !strings.Contains(sh.stderr.String(), "reclaiming") {
			t.Fatalf("did not reclaim: %s", sh.stderr)
		}
		if _, err := os.Stat(lock); !os.IsNotExist(err) {
			t.Fatal("the shell did not release the lease")
		}
	})

	t.Run("the panel can cancel a shell waiter", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
		lr := startLockRun(t, lock, "go", time.Minute, "/bin/sh", "-c", body(log, "G"))
		waitUntil(t, "held", 5*time.Second, func() bool { return lineCount(log) == 1 })
		sh := startShellLease(t, leaseSh, lock, "shell", "echo SHELL-RAN >> "+log)
		var v types.QueueView
		waitUntil(t, "the shell waiter is listed", 5*time.Second, func() bool {
			v = ReadQueue(types.QueueConfig{ID: "g"}, lock, time.Now(), LiveProc)
			return len(v.Waiters) == 1 && v.Waiters[0].Lane == "shell"
		})
		if err := CancelWait(lock, v.Waiters[0].Nonce); err != nil {
			t.Fatal(err)
		}
		if code := sh.wait(t, 10*time.Second); code != ExitCancelled {
			t.Fatalf("shell exit %d: %s", code, sh.stderr)
		}
		lr.wait(t, 10*time.Second)
		if strings.Contains(strings.Join(readLog(t, log), " "), "SHELL-RAN") {
			t.Fatal("a cancelled shell waiter ran")
		}
	})
}

// tracedSleep writes a `sleep` into dir that appends a line to the returned trace
// file each time it is called, then sleeps. lease.sh sleeps once per iteration of
// its wait loop, so with dir first on its PATH a test asserts after N iterations of
// that loop instead of sleeping itself.
func tracedSleep(t *testing.T, dir string) (trace string) {
	t.Helper()
	sleepBin, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep")
	}
	trace = filepath.Join(t.TempDir(), "sleep-trace")
	os.Remove(filepath.Join(dir, "sleep")) // noPSPath links the real one
	script := "#!/bin/sh\necho \"$*\" >> " + shq(trace) + "\nexec " + shq(sleepBin) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "sleep"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return trace
}

// shq single-quotes s for a POSIX shell.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// firstChild returns the pid of a process's first child, as a string for ps.
func firstChild(t *testing.T, pid int) string {
	t.Helper()
	for i := 0; i < 50; i++ {
		o, _ := exec.Command("/usr/bin/pgrep", "-P", strconv.Itoa(pid)).Output()
		if f := strings.Fields(string(o)); len(f) > 0 {
			return f[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("pid %d has no child", pid)
	return ""
}

// Round 2: a lock-run killed with SIGKILL leaves its gate running. The gate still
// holds the lease: child_pid in owner.json keeps the record live, for Go and shell
// readers alike, until the gate itself ends.
func TestKilledLockRunsGateKeepsTheLease(t *testing.T) {
	t.Parallel()
	leaseSh := docLeaseSh(t)
	for _, waiterKind := range []string{"lock-run", "shell"} {
		t.Run(waiterKind, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
			holder := startLockRun(t, lock, "a", time.Minute, "/bin/sh", "-c",
				"echo A-start >> "+log+"; sleep 2; echo A-end >> "+log)
			waitUntil(t, "the gate runs", 5*time.Second, func() bool {
				o, err := readLeaseFile(filepath.Join(lock, ownerFileName))
				return lineCount(log) == 1 && err == nil && o.ChildPID > 0
			})
			holder.cmd.Process.Signal(syscall.SIGKILL)
			holder.wait(t, 5*time.Second) // lock-run is gone; its gate runs on
			var w *lockProc
			if waiterKind == "shell" {
				w = startShellLease(t, leaseSh, lock, "b", "echo B-ran >> "+log)
			} else {
				w = startLockRun(t, lock, "b", time.Minute, "/bin/sh", "-c", "echo B-ran >> "+log)
			}
			if code := w.wait(t, 15*time.Second); code != 0 {
				t.Fatalf("waiter exit %d: %s", code, w.stderr)
			}
			if got := strings.Join(readLog(t, log), " "); got != "A-start A-end B-ran" {
				t.Fatalf("the waiter ran while the killed lock-run's gate still ran: %q", got)
			}
		})
	}
}

// noPSPath is a PATH with every tool lease.sh uses except ps.
func noPSPath(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: runs lock-run or lease.sh as real processes")
	}
	bin := t.TempDir()
	for _, tool := range []string{"sh", "bash", "awk", "sed", "head", "stat", "date", "hostname", "od", "tr",
		"mkdir", "mv", "rm", "ls", "grep", "sort", "sleep", "rmdir", "cat"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("no %s", tool)
		}
		os.Symlink(p, filepath.Join(bin, tool))
	}
	return bin
}

// Round 2: lease.sh without ps. Liveness falls back to kill -0 (EPERM is alive),
// and a pid that is alive but cannot be verified is live: missing data never
// deletes someone else's lease or waiter file.
func TestShellLeaseWithoutPS(t *testing.T) {
	t.Parallel()
	leaseSh := docLeaseSh(t)
	path := noPSPath(t)
	run := func(path, lock, lane, body string) *lockProc {
		script := "set -euo pipefail; . " + leaseSh + "; lease_run \"$1\" \"$2\" sh -c \"$3\""
		cmd := exec.Command(filepath.Join(path, "bash"), "-c", script, "lease", lock, lane, body)
		cmd.Env = []string{"PATH=" + path, "HOME=" + os.Getenv("HOME")}
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
	t.Run("waits behind a live lock-run holder", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
		h := startLockRun(t, lock, "go", time.Second, "/bin/sh", "-c", "echo G-start >> "+shq(log)+"; sleep 2; echo G-end >> "+shq(log))
		waitUntil(t, "held", 5*time.Second, func() bool { return lineCount(log) == 1 })
		s := run(path, lock, "shell", "echo S-ran >> "+shq(log))
		if code := s.wait(t, 15*time.Second); code != 0 {
			t.Fatalf("exit %d: %s", code, s.stderr)
		}
		h.wait(t, 5*time.Second)
		if got := strings.Join(readLog(t, log), " "); got != "G-start G-end S-ran" {
			t.Fatalf("order %q", got)
		}
	})
	t.Run("never deletes a live waiter it cannot verify (EPERM)", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
		os.MkdirAll(waitersDir(lock), 0o755)
		// pid 1 is alive and not ours: kill -0 answers EPERM. No start time recorded.
		other := filepath.Join(waitersDir(lock), "1-00000000000000aa.json")
		writeLeaseFile(other, LeaseOwner{V: 1, Nonce: "00000000000000aa", PID: 1, Host: hostName(), Lane: "someone"})
		own := noPSPath(t) // its own PATH: the traced sleep counts this shell's loop only
		trace := tracedSleep(t, own)
		s := run(own, lock, "shell", "echo S-ran >> "+shq(log))
		waitUntil(t, "two iterations of the shell's wait loop", 10*time.Second, func() bool { return lineCount(trace) >= 2 })
		if _, err := os.Stat(other); err != nil {
			t.Fatal("deleted a live waiter's file on missing data")
		}
		if lineCount(log) != 0 {
			t.Fatal("jumped ahead of an earlier live waiter")
		}
		os.Remove(other)
		if code := s.wait(t, 10*time.Second); code != 0 || lineCount(log) != 1 {
			t.Fatalf("exit %d: %s", code, s.stderr)
		}
	})
	t.Run("reclaims a dead holder", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
		os.Mkdir(lock, 0o755)
		now := time.Now().Unix()
		writeLeaseFile(filepath.Join(lock, ownerFileName), LeaseOwner{V: 1, Nonce: "00000000000000bb", PID: deadPID(t),
			PStart: "Thu Jan 1 00:00:00 1970", Host: hostName(), Started: now, Renewed: now})
		s := run(path, lock, "shell", "echo S-ran >> "+shq(log))
		if code := s.wait(t, 10*time.Second); code != 0 || lineCount(log) != 1 {
			t.Fatalf("exit %d: %s", code, s.stderr)
		}
	})
}

// Round 2: the documented run-local.sh runs the gate unqueued, with one warning,
// when neither clauductor nor lease.sh is there; it never aborts.
func TestSnippetRunsUnqueuedWithoutClauductorOrLeaseSh(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "panel.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	i := strings.Index(s, "#!/usr/bin/env bash\nset -euo pipefail\n# Serialise")
	j := strings.Index(s[i:], "```")
	if i < 0 || j < 0 {
		t.Fatal("no run-local.sh snippet in docs/panel.md")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	script := strings.Replace(s[i:i+j], "# ...the gate itself...", "echo gate-ran", 1)
	os.WriteFile(filepath.Join(dir, "run-local.sh"), []byte(script), 0o755)
	path := noPSPath(t)
	git, _ := exec.LookPath("git")
	dirname, _ := exec.LookPath("dirname")
	basename, _ := exec.LookPath("basename")
	os.Symlink(git, filepath.Join(path, "git"))
	os.Symlink(dirname, filepath.Join(path, "dirname"))
	os.Symlink(basename, filepath.Join(path, "basename"))
	cmd := exec.Command(filepath.Join(path, "bash"), "run-local.sh")
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + path, "HOME=" + os.Getenv("HOME")}
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "gate-ran") || !strings.Contains(string(out), "neither clauductor nor lease.sh") {
		t.Fatalf("%v: %s", err, out)
	}
}

// Round 3: the gate leaves a daemon behind (a detached sleep), lock-run is killed
// with SIGKILL, then the gate exits. Holder and command are both dead: Go and shell
// waiters must BOTH take the lease promptly, not wait for the daemon.
func TestDaemonLeftByTheGateDoesNotHoldTheLease(t *testing.T) {
	t.Parallel()
	leaseSh := docLeaseSh(t)
	for _, waiterKind := range []string{"lock-run", "shell"} {
		t.Run(waiterKind, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
			daemonPid := filepath.Join(dir, "daemon.pid")
			holder := startLockRun(t, lock, "a", time.Minute, "/bin/sh", "-c",
				"(sleep 60 </dev/null >/dev/null 2>&1 & echo $! > "+daemonPid+"); echo A-start >> "+log+"; sleep 1; echo A-end >> "+log)
			t.Cleanup(func() {
				if b, err := os.ReadFile(daemonPid); err == nil {
					if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
						syscall.Kill(pid, syscall.SIGKILL)
					}
				}
			})
			waitUntil(t, "the gate runs", 5*time.Second, func() bool {
				o, err := readLeaseFile(filepath.Join(lock, ownerFileName))
				return lineCount(log) == 1 && err == nil && o.ChildPID > 0
			})
			holder.cmd.Process.Signal(syscall.SIGKILL)
			waitUntil(t, "the gate ends", 5*time.Second, func() bool { return lineCount(log) == 2 })
			start := time.Now()
			var w *lockProc
			if waiterKind == "shell" {
				w = startShellLease(t, leaseSh, lock, "b", "echo B-ran >> "+log)
			} else {
				w = startLockRun(t, lock, "b", time.Minute, "/bin/sh", "-c", "echo B-ran >> "+log)
			}
			if code := w.wait(t, 10*time.Second); code != 0 {
				t.Fatalf("waiter exit %d: %s", code, w.stderr)
			}
			if d := time.Since(start); d > 4*time.Second {
				t.Fatalf("took %v: the daemon's fd held the lease", d)
			}
			if got := strings.Join(readLog(t, log), " "); got != "A-start A-end B-ran" {
				t.Fatalf("order %q", got)
			}
		})
	}
}
