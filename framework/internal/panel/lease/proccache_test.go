package lease

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestProcCacheReadsAStartTimeOncePerProcess(t *testing.T) {
	clock := t0
	alive := map[int]bool{100: true}
	starts := map[int]string{100: "Mon Sep 28 10:00:00 2026"}
	var aliveCalls, startCalls int
	c := &ProcCache{
		Alive: func(pid int) bool { aliveCalls++; return alive[pid] },
		Start: func(pid int) string { startCalls++; return starts[pid] },
		Now:   func() time.Time { return clock },
	}
	for i := 0; i < 60; i++ {
		if ok, s := c.Check(100); !ok || s != starts[100] {
			t.Fatalf("check %d: %v %q", i, ok, s)
		}
		clock = clock.Add(time.Second)
	}
	// Read when new, and again every 30 s (t = 0 and t = 30 s).
	if startCalls != 2 || aliveCalls != 60 {
		t.Fatalf("a minute of 1 s checks: %d start reads (ps), %d kill(0) checks; want 2 and 60", startCalls, aliveCalls)
	}
	// The process exits and the pid is reused: the start time is read again.
	alive[100] = false
	if ok, _ := c.Check(100); ok {
		t.Fatal("a gone pid reads as alive")
	}
	alive[100], starts[100] = true, "Mon Sep 28 11:00:00 2026"
	if _, s := c.Check(100); s != starts[100] || startCalls != 3 {
		t.Fatalf("a reused pid kept the old start time %q (%d reads)", s, startCalls)
	}
	// A pid no longer asked about is dropped after the TTL, so the map stays small.
	clock = clock.Add(2 * time.Minute)
	c.Check(200)
	if len(c.m) != 0 {
		t.Fatalf("stale entries kept: %v", c.m)
	}
}

// A pid that dies and is reused between two checks is never seen gone; the 30 s
// re-read still catches the new start time.
func TestProcCacheRereadsAStartTimeEvery30s(t *testing.T) {
	clock := t0
	start := "Mon Sep 28 10:00:00 2026"
	c := &ProcCache{Alive: func(int) bool { return true }, Start: func(int) string { return start }, Now: func() time.Time { return clock }}
	c.Check(300)
	start = "Mon Sep 28 12:00:00 2026" // reused unseen
	clock = clock.Add(29 * time.Second)
	if _, s := c.Check(300); s == start {
		t.Fatal("re-read before 30 s")
	}
	clock = clock.Add(time.Second)
	if _, s := c.Check(300); s != start {
		t.Fatalf("after 30 s the cache still says %q", s)
	}
}

var t0 = time.Date(2026, 9, 28, 14, 0, 0, 0, time.UTC)

func lineCount(p string) int {
	b, err := os.ReadFile(p)
	if err != nil {
		return 0
	}
	return len(strings.Fields(string(b)))
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
