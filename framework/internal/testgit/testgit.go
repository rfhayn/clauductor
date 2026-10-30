// Package testgit is test support: the git a test runs stays in the test's own fixtures. Only
// tests import it.
//
// Run under a git hook, GIT_DIR, GIT_WORK_TREE and GIT_INDEX_FILE aim every git at the hook's
// repository, so a fixture's init, add and commit would act on the outer one: OPS-142's review
// reproduced a fixture commit that deleted a repository's files (#302, #281). Every process a
// test starts inherits them: its own git, the git the code under test runs, a script's. So
// importing testgit drops them from the test binary before any of its tests' code runs (init),
// and TestEveryTestPackageImportsTestgit requires every package with tests to import it. Nothing
// runs the tests under a hook today, so nothing else would show it until one did.
package testgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/rfhayn/stridal/internal/treesnap"
)

// Path is git as found when the test binary started, before any test put a stand-in first on
// PATH; empty when git is not installed.
var Path, _ = exec.LookPath("git")

// init unsets every GIT_ variable in this process, so nothing a test starts inherits a hook's. It
// runs when the test binary initialises testgit, which Go finishes before any package that
// imports it initialises its own variables or runs its init or TestMain. Go orders it only before
// the packages that import it, and a directory's tests are two packages when it has external
// tests (x and x_test): each form whose files hold code imports testgit (`import _
// ".../testgit"` when it uses nothing of it), and is then covered from its first line.
// TESTGIT_HELPER_NOSTRIP is the probe's negative case alone: a binary that did not strip. It then
// sets configEnv, so no git the binary starts runs auto maintenance (noMaintenance).
func init() {
	if os.Getenv("TESTGIT_HELPER_NOSTRIP") != "" {
		return
	}
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.HasPrefix(k, "GIT_") {
			os.Unsetenv(k)
		}
	}
	for _, kv := range configEnv() {
		k, v, _ := strings.Cut(kv, "=")
		os.Setenv(k, v)
	}
}

// Env is this process's environment with none of its own GIT_ variables, plus testgit's git
// config (configEnv) and a commit identity. init has already dropped what the test binary
// inherited; Env also drops what a test set itself, as a test standing in for a hook does
// (template's TestTheFixturesGitStaysInTheFixture), then sets configEnv again. A caller that
// needs more appends it (GIT_EDITOR), or a config through WithConfig.
func Env() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	env = append(env, configEnv()...)
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
	// Its own config pinned, and built with none of the machine's: a global untracked cache or
	// fsmonitor would make a read (git status) write the index or start a daemon with a socket in
	// .git. feature.manyFiles needs no pin of its own: what it turns on that a read writes is the
	// untracked cache, which the first pin already holds off (TestOuterSeesEachChange's row).
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "core.untrackedCache", "false"},
		{"config", "core.fsmonitor", "false"},
		{"commit", "-q", "--allow-empty", "-m", "outer"},
	} {
		c := Command(outer, args...)
		c.Env = append(c.Env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := c.CombinedOutput(); err != nil {
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

// WithConfig is env with this package's git config (configEnv) plus the key-value pairs kv, as
// GIT_CONFIG_COUNT entries: a caller that needs a config of its own adds it here, since a second
// GIT_CONFIG_COUNT would replace the first.
func WithConfig(env []string, kv ...string) []string {
	var out []string
	for _, e := range env {
		if !strings.HasPrefix(e, "GIT_CONFIG_COUNT=") && !strings.HasPrefix(e, "GIT_CONFIG_KEY_") && !strings.HasPrefix(e, "GIT_CONFIG_VALUE_") {
			out = append(out, e)
		}
	}
	return append(out, configEnv(kv...)...)
}

// noMaintenance keeps every git a test starts from auto maintenance and auto gc: a write (a
// commit, a branch, a worktree) may start them in the background, and they take and drop
// .git/objects/maintenance.lock while a test walks .git (#343's TestOuterSeesEachChange, #361's
// TestCatchUpLeavesAnotherBranchDetachedOrAhead, both on Ubuntu).
var noMaintenance = []string{"maintenance.auto", "false", "gc.auto", "0"}

// configEnv is noMaintenance and the pairs kv as GIT_CONFIG_COUNT, GIT_CONFIG_KEY_n and
// GIT_CONFIG_VALUE_n, which every git reads above any config file.
func configEnv(kv ...string) []string {
	pairs := append(append([]string{}, noMaintenance...), kv...)
	env := []string{"GIT_CONFIG_COUNT=" + strconv.Itoa(len(pairs)/2)}
	for i := 0; i+1 < len(pairs); i += 2 {
		env = append(env, "GIT_CONFIG_KEY_"+strconv.Itoa(i/2)+"="+pairs[i], "GIT_CONFIG_VALUE_"+strconv.Itoa(i/2)+"="+pairs[i+1])
	}
	return env
}
