package testgit_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	_ "github.com/clauductor/clauductor/internal/testgit" // as every package's tests import it
)

// atInit is git that a test file runs as a package-level variable: before any TestMain, so only a
// strip that runs at testgit's own initialisation, which Go finishes before any package that
// imports it starts its own, keeps it out of a hook's repository. The probe's helper runs it.
var atInit = func() error {
	if os.Getenv("TESTGIT_HELPER") != "1" {
		return nil
	}
	dir, err := os.MkdirTemp("", "testgit-atinit-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	for _, args := range [][]string{{"init", "-q"}, {"config", "fixture.atinit", "yes"}} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			return fmt.Errorf("git %v: %v\n%s", args, err, out)
		}
	}
	return nil
}()

// TestHelperRunsFixtureGitAtInit reports the package-level git's outcome to the probe.
func TestHelperRunsFixtureGitAtInit(t *testing.T) {
	if os.Getenv("TESTGIT_HELPER") != "1" {
		t.Skip("run by TestATestBinaryUnderAHookLeavesItsRepositoryAlone")
	}
	if atInit != nil {
		t.Error(atInit)
	}
}
