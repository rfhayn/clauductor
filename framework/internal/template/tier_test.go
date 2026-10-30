package template

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rfhayn/stridal/internal/testbin"
	"github.com/rfhayn/stridal/internal/testgit"
)

// projectDirs are the template's .claude/ directories a project owns: created when missing,
// never overwritten. evals/ is the project's on purpose (OPS-10): it adds its own cases. local/ is
// the project's own extension layer (P1.2), the doc tier by rule (LocalDir).
var projectDirs = []string{".claude/agents/", ".claude/health/", ".claude/evals/", LocalDir}

// The local layer is the doc tier whatever it holds, even a script directly under it or a path
// a framework rule would otherwise claim.
func TestLocalLayerIsAlwaysTheProjects(t *testing.T) {
	for _, p := range []string{".claude/local/README.md", ".claude/local/guard.d/x.sh", ".claude/local/modules/m/module.conf",
		".claude/local/checks/x.sh", ".claude/local/skills/session-close/x.md", ".claude/local/conflicts.tsv"} {
		if got := Classify(p); got != TierDoc {
			t.Errorf("Classify(%q) = %s, want doc", p, TierLabel(got))
		}
	}
}

// Every directory under the template's .claude/ is placed on purpose, as the model's code or as
// the project's, so a new one cannot fall through to "create once, never update" unnoticed: in
// the OPS-8 rehearsal, a new directory's tier was a question nobody was asked, because the doc
// tier is the silent default. The set is what git holds under the template's .claude/, not a list.
func TestEveryTemplateClaudeDirIsPlaced(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dirs, err := templateClaudeDirs(root, gitEnv(false))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range unplaced(dirs) {
		t.Errorf("template/%s is neither in FrameworkDirs nor a project directory: decide which (tier.go, and projectDirs here)", d)
	}
	if len(dirs) == 0 {
		t.Fatal("no directories under template/.claude: the test learned nothing")
	}
}

// templateClaudeDirs lists the directories directly under root's template/.claude/ that hold a
// file git tracks or would add. It asks git, not the disk: the harness leaves an empty, ignored
// .cc-writes/ in a worktree whose session ran in template/ (#285), and that is no directory of the
// template's, while a new directory still counts before its first commit. A root with no .git is
// the gate's archive clean room (GATE_CLEAN_ROOM), which holds tracked files only: there the disk
// is what git holds.
func templateClaudeDirs(root string, env []string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return diskClaudeDirs(root)
	}
	cmd := testgit.Command(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard",
		"--", "template/.claude/")
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("git ls-files in %s: %v: %s", root, err, ee.Stderr)
		}
		return nil, err
	}
	var dirs []string
	for _, f := range strings.Split(string(out), "\x00") {
		rest, ok := strings.CutPrefix(f, "template/.claude/")
		name, _, nested := strings.Cut(rest, "/")
		if !ok || !nested {
			continue
		}
		if d := ".claude/" + name + "/"; !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	return dirs, nil
}

func diskClaudeDirs(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, "template", ".claude"))
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, ".claude/"+e.Name()+"/")
		}
	}
	return dirs, nil
}

// gitEnv is testgit.Env: none of the caller's GIT_ variables (testgit's config only), so a git hook's GIT_DIR, GIT_WORK_TREE and
// GIT_INDEX_FILE cannot aim a test's git at the outer repository. isolated also drops the global
// and system config, so a fixture reads only its own ignore rules and hooks. The user's ignore
// file needs core.excludesFile set as well: git reads ~/.config/git/ignore by default even with
// no global config.
func gitEnv(isolated bool) []string {
	env := testgit.Env()
	if isolated {
		env = testgit.WithConfig(append(env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"),
			"core.excludesFile", "/dev/null")
	}
	return env
}

func unplaced(dirs []string) []string {
	var out []string
	for _, d := range dirs {
		if !slices.Contains(FrameworkDirs, d) && !slices.Contains(projectDirs, d) {
			out = append(out, d)
		}
	}
	return out
}

// claudeDirFixture is a repository whose template/.claude/ commits skills/x.md, ignores
// .cc-writes/ in its own .gitignore, and then holds the extra paths the caller names, uncommitted
// (a trailing slash makes an empty directory). Its git reads no global or system config.
func claudeDirFixture(t *testing.T, extra ...string) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) { fixtureGit(t, dir, args...) }
	write := func(rel string) {
		t.Helper()
		// Join cleans a trailing slash away, so an empty directory is made from rel, not p's parent.
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	write("template/.claude/skills/x.md")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(".cc-writes/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "fixture")
	for _, rel := range extra {
		write(rel)
	}
	return dir
}

// fixtureGit runs git in a fixture under gitEnv(true). Only a missing git skips: a git that
// fails there would otherwise turn both tests that use the fixture into quiet skips.
func fixtureGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if testgit.Path == "" {
		t.Skip("git is not installed")
	}
	cmd := testgit.Command(dir, args...)
	cmd.Env = gitEnv(true)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

// The harness leaves an empty, git-ignored .cc-writes/ in a worktree whose session ran in
// template/ (#285). It is no directory of the template's, so it is not asked to be placed: empty,
// or holding an ignored file.
func TestAnIgnoredHarnessDirIsNotATemplateDir(t *testing.T) {
	for name, extra := range map[string]string{
		"empty":   "template/.claude/.cc-writes/",
		"ignored": "template/.claude/.cc-writes/w.json",
	} {
		t.Run(name, func(t *testing.T) {
			dirs, err := templateClaudeDirs(claudeDirFixture(t, extra), gitEnv(true))
			if err != nil {
				t.Fatal(err)
			}
			if got := unplaced(dirs); len(got) != 0 {
				t.Errorf("unplaced %q, want none: an ignored directory is not the template's", got)
			}
		})
	}
}

// The fixture's git stays in the fixture: run under a git hook that aims GIT_DIR and
// GIT_WORK_TREE at another repository, with git's default ignore file (~/.config/git/ignore)
// ignoring newdir/, and with an owner's global config whose hooks refuse every commit, it commits
// nothing there, its own commit succeeds, and it still reports the unplaced directory.
func TestTheFixturesGitStaysInTheFixture(t *testing.T) {
	outer := t.TempDir()
	fixtureGit(t, outer, "init", "-q")
	fixtureGit(t, outer, "commit", "-q", "--allow-empty", "-m", "outer")
	head := func() string {
		t.Helper()
		cmd := testgit.Command(outer, "rev-parse", "HEAD")
		cmd.Env = gitEnv(true)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git rev-parse in the outer repository: %v", err)
		}
		return strings.TrimSpace(string(out))
	}
	before := head()

	home := t.TempDir()
	ignore := filepath.Join(home, ".config", "git", "ignore")
	if err := os.MkdirAll(filepath.Dir(ignore), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ignore, []byte("newdir/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hooks := filepath.Join(home, "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	testbin.Write(t, filepath.Join(hooks, "pre-commit"), "#!/bin/sh\nexit 1\n")
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[core]\n\thooksPath = "+hooks+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_DIR", filepath.Join(outer, ".git"))
	t.Setenv("GIT_WORK_TREE", outer)

	dir := claudeDirFixture(t, "template/.claude/newdir/a.sh")
	dirs, err := templateClaudeDirs(dir, gitEnv(true))
	if err != nil {
		t.Fatal(err)
	}
	if got := unplaced(dirs); !slices.Equal(got, []string{".claude/newdir/"}) {
		t.Errorf("unplaced %q, want [.claude/newdir/]: the owner's ignore reached the fixture", got)
	}
	if after := head(); after != before {
		t.Errorf("the outer repository's HEAD moved from %s to %s: the fixture's git committed there", before, after)
	}
}

// The control still bites: a directory that no tier places is reported whether git tracks its
// file already or would add it (a new directory before its first commit), and in the archive
// clean room, where there is no git.
func TestAnUnplacedTemplateDirIsStillReported(t *testing.T) {
	for _, mode := range []string{"staged", "untracked", "archive"} {
		t.Run(mode, func(t *testing.T) {
			dir := claudeDirFixture(t, "template/.claude/newdir/a.sh")
			switch mode {
			case "staged":
				fixtureGit(t, dir, "add", "-A")
			case "archive":
				if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
					t.Fatal(err)
				}
			}
			dirs, err := templateClaudeDirs(dir, gitEnv(true))
			if err != nil {
				t.Fatal(err)
			}
			if got := unplaced(dirs); !slices.Equal(got, []string{".claude/newdir/"}) {
				t.Errorf("unplaced %q, want [.claude/newdir/]", got)
			}
		})
	}
}
