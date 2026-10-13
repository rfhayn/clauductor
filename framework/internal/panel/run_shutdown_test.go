package panel

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/claudecall"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/testbin"
	"github.com/clauductor/clauductor/internal/testwait"
)

// [CLAUDECALLS-1-S4] Shutdown bounds the grace: a claude that ignores SIGTERM, in
// flight when the panel is asked to stop, gets SIGTERM, then SIGKILL 3 s later, and
// Run's own shutdown waits for it no longer than those 3 s.
func TestRunShutdownBoundsTheClaudeGrace(t *testing.T) {
	root, home := setupProject(t)
	marks := filepath.Join(t.TempDir(), "marks")
	dir := t.TempDir()
	fake := filepath.Join(dir, "claude")
	testbin.Write(t, fake, fmt.Sprintf(`#!/bin/sh
case "$1" in
agents) trap 'echo TERM >> %[1]s' TERM; echo "started $$" >> %[1]s; while :; do sleep 1; done;;
--version) echo '2.1.296 (Claude Code)';;
*) exit 1;;
esac
`, shq(marks)))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if got, err := exec.LookPath("claude"); err != nil || got != fake {
		t.Fatalf("claude on PATH is %q (%v), not the fake", got, err)
	}
	git := fakeRunner(root)
	run := func(ctx context.Context, d string, argv []string) ([]byte, error) {
		if signals.IsClaude(argv) {
			return signals.ExecRunner(ctx, d, argv)
		}
		return git(ctx, d, argv)
	}
	_, _, stop := runPanel(t, Options{Project: root, Port: 0, NoOpen: true, Home: home, Runner: run})
	var pid int
	waitUntil(t, "the agents call in flight", 10*time.Second, func() bool {
		b, _ := os.ReadFile(marks)
		f := strings.Fields(string(b))
		if len(f) >= 2 && f[0] == "started" {
			pid, _ = strconv.Atoi(f[1])
		}
		return pid > 0
	})

	start := time.Now()
	stop()
	took := time.Since(start)
	if took < claudecall.ShutdownGrace || took > claudecall.ShutdownGrace+testwait.Scale(2*time.Second) {
		t.Fatalf("shutdown took %v, want the %v grace and no longer", took, claudecall.ShutdownGrace)
	}
	if b, _ := os.ReadFile(marks); !strings.Contains(string(b), "TERM") {
		t.Fatalf("the claude got no SIGTERM: %q", b)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("claude %d still runs after the shutdown", pid)
	}
	b, _ := os.ReadFile(filepath.Join(config.PanelDir(home), claudecall.LogName))
	var found bool
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		found = found || (strings.Contains(line, `"kind":"stopped"`) && strings.Contains(line, `"pid":`+strconv.Itoa(pid)+`,`) &&
			strings.Contains(line, `"signals":["SIGTERM","SIGKILL"]`))
	}
	if !found {
		t.Fatalf("no stopped-call line with SIGTERM and SIGKILL for %d in:\n%s", pid, b)
	}
}
