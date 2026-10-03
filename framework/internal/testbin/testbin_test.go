package testbin

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
)

// Scripts written and run at once while other goroutines fork without pause, as a
// package's parallel tests do: none may fail as busy. Only Linux refuses to exec a file
// open for writing, so elsewhere this passes with or without the guard.
func TestAScriptRunsAtOnceBesideForks(t *testing.T) {
	if testing.Short() {
		t.Skip("forks without pause for a few seconds")
	}
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no true(1)")
	}
	var stop atomic.Bool
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				_ = exec.Command(truePath).Run()
			}
		}()
	}
	defer func() { stop.Store(true); wg.Wait() }()
	dir := t.TempDir()
	const runs = 200
	busy := 0
	for i := range runs {
		p := filepath.Join(dir, "s"+strconv.Itoa(i))
		Write(t, p, "#!/bin/sh\nexit 0\n")
		switch err := exec.Command(p).Run(); {
		case errors.Is(err, syscall.ETXTBSY):
			busy++
		case err != nil:
			t.Fatal(err)
		}
	}
	if busy > 0 {
		t.Fatalf("%d of %d scripts run as soon as they were written failed as busy (ETXTBSY)", busy, runs)
	}
	if fi, err := os.Stat(filepath.Join(dir, "s0")); err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("the script is not executable: %v %v", fi, err)
	}
}

// fileWrite is a test writing a file itself, with its mode last, on one line.
var fileWrite = regexp.MustCompile(`os\.(WriteFile|OpenFile)\(.*, 0o?([0-7]{3})\)`)

// Every test file in the module, not a list of the known ones: a test that writes its own
// executable is the flake this package exists to prevent.
func TestNoTestWritesAnExecutableAnotherWay(t *testing.T) {
	var found []string
	err := filepath.WalkDir("../..", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(b), "\n") {
			m := fileWrite.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			if mode, _ := strconv.ParseUint(m[2], 8, 32); mode&0o111 != 0 {
				found = append(found, p+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) > 0 {
		t.Fatalf("write these with testbin.Write, or a parallel test's fork can leave them busy:\n%s", strings.Join(found, "\n"))
	}
}
