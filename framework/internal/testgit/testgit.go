// Package testgit is test support: the git a test runs on its own fixtures, kept in them. Only
// tests import it.
//
// Run under a git hook, GIT_DIR, GIT_WORK_TREE and GIT_INDEX_FILE aim every git at the hook's
// repository, so a fixture's init, add and commit would act on the outer one: OPS-142's review
// reproduced a fixture commit that deleted a repository's files (#302, #281). Nothing runs the
// tests under a hook today, so nothing would show it until one did. A test's own git therefore
// goes through Command, whose environment holds no GIT_ variable, and
// TestNoTestRunsGitAnotherWay refuses one that does not.
package testgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Path is git as found when the test binary started, before any test put a stand-in first on
// PATH; empty when git is not installed.
var Path, _ = exec.LookPath("git")

// Env is this process's environment with no GIT_ variable, and a commit identity: git then reads
// the repository it runs in, and commits there whatever the machine's config holds. A caller
// that needs more (GIT_EDITOR, an isolated config) appends it.
func Env() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env, "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
}

// Command is git with args in dir (the current directory when empty), under Env.
func Command(dir string, args ...string) *exec.Cmd {
	name := Path
	if name == "" {
		name = "git" // fails when run, as a missing git should
	}
	c := exec.Command(name, args...)
	c.Dir, c.Env = dir, Env()
	return c
}

// Outer stands in for a git hook's environment for the rest of the test: GIT_DIR, GIT_WORK_TREE
// and GIT_INDEX_FILE name a repository of its own, with one commit. The returned function names
// what a fixture's git did to that repository since (its HEAD moved, its index was written), or
// returns "" when it stayed untouched. It sets the environment, so the test cannot be parallel.
func Outer(t testing.TB) (touched func() string) {
	t.Helper()
	if Path == "" {
		t.Skip("git is not installed")
	}
	outer := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"commit", "-q", "--allow-empty", "-m", "outer"}} {
		if out, err := Command(outer, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v in the outer repository: %v\n%s", args, err, out)
		}
	}
	gitDir, index := filepath.Join(outer, ".git"), filepath.Join(outer, ".git", "index")
	head := func() string {
		out, err := Command(outer, "rev-parse", "HEAD").Output()
		if err != nil {
			t.Fatalf("git rev-parse in the outer repository: %v", err)
		}
		return strings.TrimSpace(string(out))
	}
	before := head()
	indexBefore, _ := os.ReadFile(index) // none until something is added
	t.Setenv("GIT_DIR", gitDir)
	t.Setenv("GIT_WORK_TREE", outer)
	t.Setenv("GIT_INDEX_FILE", index)
	return func() string {
		t.Helper()
		if after := head(); after != before {
			return "the outer repository's HEAD moved from " + before + " to " + after
		}
		if b, _ := os.ReadFile(index); string(b) != string(indexBefore) {
			return "the outer repository's index was written"
		}
		return ""
	}
}
