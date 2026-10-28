package template

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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
	dirs, err := templateClaudeDirs(root)
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
// template's, while a new directory still counts before its first commit.
func templateClaudeDirs(root string) ([]string, error) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard",
		"--", "template/.claude/").Output()
	if err != nil {
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
// (a trailing slash makes an empty directory).
func claudeDirFixture(t *testing.T, extra ...string) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Skipf("git %v: %v %s", args, err, out)
		}
	}
	write := func(rel string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(rel, "/") {
			return
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
	git("-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "fixture")
	for _, rel := range extra {
		write(rel)
	}
	return dir
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
			dirs, err := templateClaudeDirs(claudeDirFixture(t, extra))
			if err != nil {
				t.Fatal(err)
			}
			if got := unplaced(dirs); len(got) != 0 {
				t.Errorf("unplaced %q, want none: an ignored directory is not the template's", got)
			}
		})
	}
}

// The control still bites: a directory that no tier places is reported whether git tracks its
// file already or would add it (a new directory before its first commit).
func TestAnUnplacedTemplateDirIsStillReported(t *testing.T) {
	for _, staged := range []bool{true, false} {
		dir := claudeDirFixture(t, "template/.claude/newdir/a.sh")
		if staged {
			if out, err := exec.Command("git", "-C", dir, "add", "-A").CombinedOutput(); err != nil {
				t.Fatalf("git add: %v %s", err, out)
			}
		}
		dirs, err := templateClaudeDirs(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := unplaced(dirs); !slices.Equal(got, []string{".claude/newdir/"}) {
			t.Errorf("staged=%v: unplaced %q, want [.claude/newdir/]", staged, got)
		}
	}
}
