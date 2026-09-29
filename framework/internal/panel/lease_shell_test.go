package panel

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The plain-shell protocol in docs/panel.md is what a project without clauductor
// (Standing Tee's run-local.sh) runs. The test runs THAT text, extracted from the
// page, against lock-run, so the documentation cannot drift from the protocol.
func docLeaseSh(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "panel.md"))
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
	script := "set -euo pipefail; . " + leaseSh + "; lease_run \"$1\" \"$2\" sh -c \"$3\""
	cmd := exec.Command("bash", "-c", script, "lease", lock, lane, body)
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
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	leaseSh := docLeaseSh(t)
	body := func(log, n string) string {
		return "echo " + n + "-start >> " + log + "; sleep 1; echo " + n + "-end >> " + log
	}

	t.Run("shell holder, lock-run waiter past its TTL", func(t *testing.T) {
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
		dir := t.TempDir()
		lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
		lr := startLockRun(t, lock, "go", time.Second, "/bin/sh", "-c", body(log, "G"))
		waitUntil(t, "lock-run holds it", 5*time.Second, func() bool { return lineCount(log) == 1 })
		// Stop lock-run and its command group: no renewal, far past the 1 s TTL.
		syscall.Kill(lr.cmd.Process.Pid, syscall.SIGSTOP)
		pg, _ := exec.Command("/bin/ps", "-o", "pgid=", "-p", firstChild(t, lr.cmd.Process.Pid)).Output()
		pgid := strings.TrimSpace(string(pg))
		exec.Command("/bin/kill", "-STOP", "-"+pgid).Run()
		sh := startShellLease(t, leaseSh, lock, "shell", body(log, "S"))
		time.Sleep(4 * time.Second)
		if lineCount(log) != 1 {
			t.Fatalf("the shell took a stopped holder's lease: %v", readLog(t, log))
		}
		exec.Command("/bin/kill", "-CONT", "-"+pgid).Run()
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
		dir := t.TempDir()
		lock, log := filepath.Join(dir, "gate.lock"), filepath.Join(dir, "log")
		lr := startLockRun(t, lock, "go", time.Minute, "/bin/sh", "-c", body(log, "G"))
		waitUntil(t, "held", 5*time.Second, func() bool { return lineCount(log) == 1 })
		sh := startShellLease(t, leaseSh, lock, "shell", "echo SHELL-RAN >> "+log)
		var v QueueView
		waitUntil(t, "the shell waiter is listed", 5*time.Second, func() bool {
			v = ReadQueue(QueueConfig{ID: "g"}, lock, time.Now(), LiveProc)
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
