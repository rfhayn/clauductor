package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// installSh runs the repository's install.sh with PATH holding only the stubs in bin and the
// system tools the script itself needs, HOME at home, and no controlling terminal, so it can ask
// no one: every offer must be answered "no".
func installSh(t *testing.T, bin, home string, env ...string) (string, error) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
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
	c := exec.Command("/bin/bash", filepath.Join(root, "install.sh"))
	c.Dir = root
	c.Env = append([]string{
		"PATH=" + bin + ":" + sys,
		"HOME=" + home,
		"INSTALL_DIR=" + filepath.Join(home, "bin"),
		"TMPDIR=" + os.TempDir(),
	}, env...)
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

// install.sh offers what is missing and installs nothing unasked: with no one to answer, brew and
// sudo are never run, the missing tools are named with how to install them, and a second run
// changes nothing it already did.
func TestInstallShNeverInstallsUnasked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is for macOS and Linux")
	}
	bin, home := t.TempDir(), t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	stub(t, bin, "brew", calls, "exit 0")
	stub(t, bin, "sudo", calls, "exit 0")
	stub(t, bin, "apt-get", calls, "exit 0")
	// go: `go build -o clauductor …` leaves a binary that prints a version.
	stub(t, bin, "go", calls, `case $1 in build) printf '#!/bin/sh\necho "clauductor v0.0.0-test"\n' > clauductor; chmod +x clauductor ;; version) echo "go version go0.0 test" ;; esac`)
	t.Cleanup(func() { os.Remove(filepath.Join("..", "..", "clauductor")) })

	for run := 1; run <= 2; run++ {
		out, err := installSh(t, bin, home)
		if err != nil {
			t.Fatalf("run %d: install.sh failed: %v\n%s", run, err, out)
		}
		got, _ := os.ReadFile(calls)
		for _, line := range strings.Split(string(got), "\n") {
			if strings.HasPrefix(line, "brew install") || strings.HasPrefix(line, "sudo") || strings.HasPrefix(line, "apt-get") {
				t.Errorf("run %d: install.sh ran %q with no one to say yes", run, line)
			}
		}
		for _, tool := range []string{"jq", "gh", "gitleaks"} {
			if !strings.Contains(out, "  "+tool+" is missing") {
				t.Errorf("run %d: install.sh did not name the missing %s:\n%s", run, tool, out)
			}
		}
		if !strings.Contains(out, "Prerequisites on ") {
			t.Errorf("run %d: install.sh printed no prerequisites report:\n%s", run, out)
		}
	}
	rc, _ := os.ReadFile(filepath.Join(home, ".zshrc"))
	if n := strings.Count(string(rc), "export CLAUDUCTOR_FRAMEWORK="); n != 1 {
		t.Errorf("two runs left %d CLAUDUCTOR_FRAMEWORK lines in .zshrc, want 1:\n%s", n, rc)
	}
}

// Without Go there is nothing to build: install.sh says so and stops, having installed nothing.
func TestInstallShStopsWithoutGo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is for macOS and Linux")
	}
	bin, home := t.TempDir(), t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	stub(t, bin, "brew", calls, "exit 0")
	out, err := installSh(t, bin, home)
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
