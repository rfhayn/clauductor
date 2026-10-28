// Package testgit is test support: the git a test runs stays in the test's own fixtures. Only
// tests import it.
//
// Run under a git hook, GIT_DIR, GIT_WORK_TREE and GIT_INDEX_FILE aim every git at the hook's
// repository, so a fixture's init, add and commit would act on the outer one: OPS-142's review
// reproduced a fixture commit that deleted a repository's files (#302, #281). Every process a
// test starts inherits them: its own git, the git the code under test runs, a script's. So each
// test binary drops them before any test runs, in its TestMain (Main, or Strip first in a TestMain
// that does more), and TestEveryTestPackageStripsGit requires that of every package with tests.
// Nothing runs the tests under a hook today, so nothing else would show it until one did.
package testgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/treesnap"
)

// Path is git as found when the test binary started, before any test put a stand-in first on
// PATH; empty when git is not installed.
var Path, _ = exec.LookPath("git")

// Strip unsets every GIT_ variable in this process, so nothing a test starts inherits a hook's.
// A TestMain calls it first, before anything that could start a process.
func Strip() {
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "GIT_") {
			os.Unsetenv(k)
		}
	}
}

// Main is the TestMain of a package that needs nothing more: Strip, then run the tests.
func Main(m *testing.M) {
	Strip()
	os.Exit(m.Run())
}

// Env is this process's environment with no GIT_ variable, and a commit identity. Strip has
// already dropped what the test binary inherited; Env also drops what a test set itself, as a
// test standing in for a hook does (template's TestTheFixturesGitStaysInTheFixture). A caller
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

// Command is git with args in dir (the current directory when empty), under Env: one fixture
// helper for every package, with a commit identity and the git found before any test stubbed PATH.
func Command(dir string, args ...string) *exec.Cmd {
	name := Path
	if name == "" {
		name = "git" // fails when run, as a missing git should
	}
	c := exec.Command(name, args...)
	c.Dir, c.Env = dir, Env()
	return c
}

// Outer is a repository with one commit, standing in for a git hook's: env is what the hook
// exports for it (GIT_DIR, GIT_WORK_TREE and GIT_INDEX_FILE), for a process the test starts as if
// under that hook. touched names every change to the repository since, in its work tree and its
// .git alike (treesnap: each entry's mode, size, mtime and content), or returns "" when there is
// none. Reading the repository (status, rev-parse, log) changes nothing there but the .git
// directory's own mtime, which is excluded; TestOuterSeesEachChange holds both.
func Outer(t testing.TB) (env []string, touched func() string) {
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
	before := treesnap.Take(t, outer)
	gitDir := filepath.Join(outer, ".git")
	env = []string{"GIT_DIR=" + gitDir, "GIT_WORK_TREE=" + outer, "GIT_INDEX_FILE=" + filepath.Join(gitDir, "index")}
	return env, func() string {
		t.Helper()
		after := treesnap.Take(t, outer)
		// The one exclusion: the .git directory's own mtime. A read (git status) takes
		// .git/index.lock to refresh the index and removes it, which moves only that. A lock left
		// behind, an index rewritten, or any other entry of .git changed still shows.
		if a, ok := after[".git"]; ok {
			b := before[".git"]
			b.ModTime = a.ModTime
			before[".git"] = b
		}
		if d := treesnap.Diff(before, after); len(d) > 0 {
			return "the outer repository changed: " + strings.Join(d, "; ")
		}
		return ""
	}
}
