package testgit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A fixture's init, add and commit under a hook's GIT_DIR, GIT_WORK_TREE and GIT_INDEX_FILE stay
// in the fixture through Command; through a git that inherits them they reach the outer
// repository, which Outer reports (the control's negative case).
func TestTheFixturesGitStaysInTheFixture(t *testing.T) {
	touched := Outer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "a.txt"}, {"commit", "-q", "-m", "fixture"}} {
		if out, err := Command(dir, args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if what := touched(); what != "" {
		t.Errorf("through Command, %s", what)
	}
	if out, err := Command(dir, "log", "--format=%s").Output(); err != nil || strings.TrimSpace(string(out)) != "fixture" {
		t.Errorf("the fixture holds no commit of its own: %q %v", out, err)
	}

	touched = Outer(t)
	c := exec.Command(Path, "commit", "-q", "--allow-empty", "-m", "fixture")
	c.Dir, c.Env = t.TempDir(), append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git commit with the hook's environment: %v\n%s", err, out)
	}
	if touched() == "" {
		t.Error("a git that inherits the hook's environment left the outer repository untouched: Outer sees nothing")
	}
}

// Every git a test runs itself goes through Command: TestTheFixturesGitStaysInTheFixture holds
// Command, and this holds every test to it. It reads every _test.go file and test support file
// of the module and fails an exec.Command or exec.CommandContext whose program names git (a
// literal, or a variable such as realGit). A git a shell script runs is not seen.
func TestNoTestRunsGitAnotherWay(t *testing.T) {
	self, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	scanned := 0
	fset := token.NewFileSet()
	err = filepath.WalkDir("../..", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		dir, err := filepath.Abs(filepath.Dir(p))
		if err != nil || dir == self {
			return err
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		if !strings.HasSuffix(p, "_test.go") && !importsTesting(f) {
			return nil
		}
		scanned++
		found = append(found, gitCommands(fset, f)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 100 {
		t.Fatalf("only %d test files scanned: the walk did not reach the module", scanned)
	}
	if len(found) > 0 {
		t.Fatalf("run these through testgit.Command, or a git hook's GIT_DIR aims them at the outer repository:\n%s", strings.Join(found, "\n"))
	}
}

func TestGitCommandsFindsEveryWayToRunGit(t *testing.T) {
	for _, c := range []struct {
		src  string
		want int
	}{
		{`exec.Command("git", "init")`, 1},
		{`exec.Command("/usr/bin/git", "init")`, 1},
		{`exec.Command(realGit, "init")`, 1},
		{`exec.CommandContext(ctx, "git", "init")`, 1},
		{`c := exec.Command("git", "a"); d := exec.Command("git", "b")`, 2},
		{`testgit.Command(dir, "init")`, 0},
		{`exec.Command("sh", "-c", "git init")`, 0},
		{`exec.Command(argv[0], argv[1:]...)`, 0},
		{`exec.CommandContext(ctx, argv[0], argv[1:]...)`, 0},
	} {
		src := "package x\nfunc f() {\n" + c.src + "\n}\n"
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "x_test.go", src, 0)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if got := len(gitCommands(fset, f)); got != c.want {
			t.Errorf("%s: %d found, want %d", c.src, got, c.want)
		}
	}
}

// gitCommands names each exec.Command or exec.CommandContext in f whose program, as source text,
// names git.
func gitCommands(fset *token.FileSet, f *ast.File) []string {
	var found []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "exec" {
			return true
		}
		arg := 0
		switch sel.Sel.Name {
		case "Command":
		case "CommandContext":
			arg = 1
		default:
			return true
		}
		if len(call.Args) > arg && strings.Contains(strings.ToLower(types.ExprString(call.Args[arg])), "git") {
			found = append(found, fset.Position(call.Pos()).String())
		}
		return true
	})
	return found
}

// importsTesting reports whether f imports "testing", which is what makes a file that is not a
// test test support.
func importsTesting(f *ast.File) bool {
	for _, imp := range f.Imports {
		if imp.Path.Value == `"testing"` {
			return true
		}
	}
	return false
}
