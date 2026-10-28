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
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
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

// Every git a test runs itself goes through Command, and every command it has the panel run goes
// through signals.EnvRunner(Env): TestTheFixturesGitStaysInTheFixture holds them, and this holds
// every test to them. It reads every _test.go file and test support file of the module and fails
// an exec.Command or exec.CommandContext whose program names git (a literal, or a variable such
// as realGit), and any use of signals.ExecRunner, which runs with the test process's
// environment. A git that a shell script, or a runner of a test's own, starts is not seen.
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
		t.Fatalf("run git through testgit.Command, and the panel's commands through signals.EnvRunner(testgit.Env), or a git hook's GIT_DIR aims them at the outer repository:\n%s", strings.Join(found, "\n"))
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
		{`exec.Command(gitBin, "init")`, 1},
		{`r := Rebuild{Run: signals.ExecRunner}`, 1},
		{`return signals.ExecRunner(ctx, dir, argv)`, 1},
		{`testgit.Command(dir, "init")`, 0},
		{`r := Rebuild{Run: signals.EnvRunner(testgit.Env)}`, 0},
		{`exec.Command("gitleaks", "detect")`, 0},
		{`exec.Command("/opt/bin/gitleaks", "detect")`, 0},
		{`exec.Command(digits, "x")`, 0},
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

// gitCommands names each place in f that runs git with the test process's environment: an
// exec.Command or exec.CommandContext whose program names git, and any use of
// signals.ExecRunner, the panel's runner, through which the code under test runs its git.
func gitCommands(fset *token.FileSet, f *ast.File) []string {
	var found []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if pkgSel(n, "signals", "ExecRunner") {
				found = append(found, fset.Position(n.Pos()).String())
			}
		case *ast.CallExpr:
			s, ok := n.Fun.(*ast.SelectorExpr)
			switch {
			case !ok:
			case pkgSel(s, "exec", "Command") && len(n.Args) > 0 && namesGit(n.Args[0]),
				pkgSel(s, "exec", "CommandContext") && len(n.Args) > 1 && namesGit(n.Args[1]):
				found = append(found, fset.Position(n.Pos()).String())
			}
		}
		return true
	})
	return found
}

// pkgSel reports whether sel is pkg.name.
func pkgSel(sel *ast.SelectorExpr, pkg, name string) bool {
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg && sel.Sel.Name == name
}

// namesGit reports whether a program argument names git: a literal whose base name is git
// (`"git"`, `"/usr/bin/git"`, not `"gitleaks"`), or an expression with git as one of its words
// (`realGit`, `gitBin`, not `digits`).
func namesGit(e ast.Expr) bool {
	if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
		s, err := strconv.Unquote(lit.Value)
		return err == nil && filepath.Base(s) == "git"
	}
	var words []string
	var w []rune
	for _, r := range types.ExprString(e) {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) || unicode.IsUpper(r) {
			words, w = append(words, string(w)), nil
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			w = append(w, unicode.ToLower(r))
		}
	}
	return slices.Contains(append(words, string(w)), "git")
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
