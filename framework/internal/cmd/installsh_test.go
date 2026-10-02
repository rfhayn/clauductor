package cmd

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

// installRoot is a throwaway copy of what install.sh reads: the script itself (it finds the
// framework from its own path, SCRIPT_DIR), an empty framework/ for the stub `go` to build into,
// and the template's prerequisites report. The script never runs in the real checkout, where its
// build step would write over, and then delete, the developer's own framework/clauductor.
func installRoot(t *testing.T) string {
	t.Helper()
	repo, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, rel := range []string{"install.sh", "template/.claude/prereqs.sh"} {
		data, err := os.ReadFile(filepath.Join(repo, rel))
		if err != nil {
			t.Fatal(err)
		}
		write(t, root, rel, string(data))
	}
	if err := os.MkdirAll(filepath.Join(root, "framework"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// keepRealBuild asserts the checkout's framework/clauductor is untouched by the test: the
// developer's build when there is one, or a marker put there for the duration (and removed only
// if it is still ours).
func keepRealBuild(t *testing.T) {
	t.Helper()
	real, err := filepath.Abs(filepath.Join("..", "..", "clauductor"))
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(real)
	ours := false
	if os.IsNotExist(err) {
		before = []byte("installsh_test marker " + t.Name() + "\n")
		if err := os.WriteFile(real, before, 0o644); err != nil {
			t.Fatal(err)
		}
		ours = true
	} else if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		after, err := os.ReadFile(real)
		if err != nil || !bytes.Equal(after, before) {
			t.Errorf("the test changed the checkout's framework/clauductor (err %v)", err)
			return
		}
		if ours {
			os.Remove(real)
		}
	})
}

// installEnv is PATH holding only the stubs in bin and the system tools the script itself needs,
// with HOME and INSTALL_DIR under home.
func installEnv(t *testing.T, bin, home string, extra ...string) []string {
	t.Helper()
	sys := t.TempDir()
	for _, tool := range []string{"bash", "sh", "uname", "dirname", "mkdir", "cp", "chmod", "rm", "grep", "cat"} {
		p, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("%s is not installed", tool)
		}
		if err := os.Symlink(p, filepath.Join(sys, tool)); err != nil {
			t.Fatal(err)
		}
	}
	return append([]string{
		"PATH=" + bin + ":" + sys,
		"HOME=" + home,
		"INSTALL_DIR=" + filepath.Join(home, "bin"),
		"TMPDIR=" + os.TempDir(),
	}, extra...)
}

// installSh runs a copy of install.sh with no controlling terminal, so it can ask no one: every
// offer must be answered "no".
func installSh(t *testing.T, root, bin, home string, env ...string) (string, error) {
	t.Helper()
	c := exec.Command("/bin/bash", filepath.Join(root, "install.sh"))
	c.Dir = root
	c.Env = installEnv(t, bin, home, env...)
	c.Stdin = nil                                      // /dev/null: not a terminal
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // no controlling terminal: /dev/tty cannot open
	out, err := c.CombinedOutput()
	return string(out), err
}

// stub writes an executable that records each call in calls and does what body says.
func stub(t *testing.T, bin, name, calls, body string) {
	t.Helper()
	script := "#!/bin/sh\necho \"" + name + " $*\" >> " + calls + "\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// stubTools: brew, sudo and apt-get that only record, and a go whose `build` leaves a binary that
// prints a version.
func stubTools(t *testing.T) (bin, calls string) {
	t.Helper()
	bin = t.TempDir()
	calls = filepath.Join(t.TempDir(), "calls")
	stub(t, bin, "brew", calls, "exit 0")
	stub(t, bin, "sudo", calls, "exit 0")
	stub(t, bin, "apt-get", calls, "exit 0")
	stub(t, bin, "go", calls, `case $1 in build) printf '#!/bin/sh\necho "clauductor v0.0.0-test"\n' > clauductor; chmod +x clauductor ;; version) echo "go version go0.0 test" ;; esac`)
	return bin, calls
}

// noInstalls fails on any brew install, sudo or apt-get the script ran.
func noInstalls(t *testing.T, calls, when string) {
	t.Helper()
	got, _ := os.ReadFile(calls)
	for _, line := range strings.Split(string(got), "\n") {
		if strings.HasPrefix(line, "brew install") || strings.HasPrefix(line, "sudo") || strings.HasPrefix(line, "apt-get") {
			t.Errorf("%s: install.sh ran %q with no one to say yes", when, line)
		}
	}
}

// install.sh offers what is missing and installs nothing unasked: with no one to answer, brew and
// sudo are never run, the missing tools are named with how to install them, and a second run
// changes nothing it already did.
func TestInstallShNeverInstallsUnasked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is for macOS and Linux")
	}
	keepRealBuild(t)
	root := installRoot(t)
	bin, calls := stubTools(t)
	home := t.TempDir()

	for run := 1; run <= 2; run++ {
		out, err := installSh(t, root, bin, home)
		if err != nil {
			t.Fatalf("run %d: install.sh failed: %v\n%s", run, err, out)
		}
		noInstalls(t, calls, "run "+string(rune('0'+run)))
		for _, tool := range []string{"jq", "gh", "gitleaks"} {
			if !strings.Contains(out, "  "+tool+" is missing") {
				t.Errorf("run %d: install.sh did not name the missing %s:\n%s", run, tool, out)
			}
		}
		if !strings.Contains(out, "Prerequisites on ") {
			t.Errorf("run %d: install.sh printed no prerequisites report:\n%s", run, out)
		}
	}
	if !fileExists(filepath.Join(home, "bin", "clauductor")) {
		t.Error("install.sh did not install the stub build")
	}
	rc, _ := os.ReadFile(filepath.Join(home, ".zshrc"))
	if n := strings.Count(string(rc), "export CLAUDUCTOR_FRAMEWORK="); n != 1 {
		t.Errorf("two runs left %d CLAUDUCTOR_FRAMEWORK lines in .zshrc, want 1:\n%s", n, rc)
	}
}

// At a terminal, CLAUDUCTOR_INSTALL_ASSUME=no still answers every offer "no": the script neither
// waits for an answer nor installs anything.
func TestInstallShAssumeNoAtATerminal(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the brew offers are macOS-only")
	}
	keepRealBuild(t)
	root := installRoot(t)
	bin, calls := stubTools(t)
	c := exec.Command("/bin/bash", filepath.Join(root, "install.sh"))
	c.Dir = root
	c.Env = installEnv(t, bin, t.TempDir(), "CLAUDUCTOR_INSTALL_ASSUME=no")
	tty, err := pty.Start(c) // stdin, stdout and the controlling terminal are the pty
	if err != nil {
		t.Skipf("no pty here: %v", err)
	}
	defer tty.Close()
	var out bytes.Buffer
	copied := make(chan struct{})
	go func() { io.Copy(&out, tty); close(copied) }() // ends with EIO once the script exits
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	select {
	case err := <-done:
		<-copied
		if err != nil {
			t.Fatalf("install.sh failed: %v\n%s", err, out.String())
		}
	case <-time.After(30 * time.Second):
		c.Process.Kill()
		<-done
		t.Fatalf("install.sh waited for an answer at a terminal despite CLAUDUCTOR_INSTALL_ASSUME=no:\n%s", out.String())
	}
	noInstalls(t, calls, "at a terminal")
	if strings.Contains(out.String(), "[y/N]") {
		t.Errorf("install.sh asked despite CLAUDUCTOR_INSTALL_ASSUME=no:\n%s", out.String())
	}
}

// Without Go there is nothing to build: install.sh says so and stops, having installed nothing.
func TestInstallShStopsWithoutGo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is for macOS and Linux")
	}
	keepRealBuild(t)
	root := installRoot(t)
	bin := t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	stub(t, bin, "brew", calls, "exit 0")
	out, err := installSh(t, root, bin, t.TempDir())
	if err == nil {
		t.Fatalf("install.sh succeeded without go:\n%s", out)
	}
	if !strings.Contains(out, "Go is required") {
		t.Errorf("install.sh did not say why it stopped:\n%s", out)
	}
	if got, _ := os.ReadFile(calls); strings.Contains(string(got), "brew install") {
		t.Errorf("install.sh ran brew install unasked: %s", got)
	}
}
