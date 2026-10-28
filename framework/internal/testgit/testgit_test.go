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

// A Shim run with the hook's environment, as the code under test runs git from PATH, commits in
// the fixture it runs in, not in the hook's repository.
func TestTheShimStaysInTheFixture(t *testing.T) {
	touched := Outer(t)
	dir := t.TempDir()
	if out, err := Command(dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	shim := filepath.Join(t.TempDir(), "git")
	Shim(t, shim)
	c := exec.Command(shim, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "fixture")
	c.Dir = dir // and the test process's environment, the hook's
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("the shim's commit: %v\n%s", err, out)
	}
	if what := touched(); what != "" {
		t.Errorf("through the shim, %s", what)
	}
	if out, err := Command(dir, "log", "--format=%s").Output(); err != nil || strings.TrimSpace(string(out)) != "fixture" {
		t.Errorf("the fixture holds no commit of its own: %q %v", out, err)
	}
}

// Outer sees each kind of change a git that inherits the hook's environment can make there: a
// branch (its refs), a worktree, and its index. None of these moves HEAD.
func TestOuterSeesEachChange(t *testing.T) {
	for _, c := range []struct {
		kind string
		args []string
	}{
		{"refs", []string{"branch", "x"}},
		{"worktrees", []string{"worktree", "add", "-q", "--detach", filepath.Join(t.TempDir(), "wt"), "HEAD"}},
		{"index", []string{"update-index", "--add", "--cacheinfo", "100644,e69de29bb2d1d6434b8b29ae775ad8c2e48c5391,x"}}, // the empty blob
	} {
		touched := Outer(t)
		g := exec.Command(Path, c.args...)
		g.Dir = t.TempDir()
		if out, err := g.CombinedOutput(); err != nil {
			t.Fatalf("git %v with the hook's environment: %v\n%s", c.args, err, out)
		}
		if what := touched(); !strings.Contains(what, "'s "+c.kind+" ") {
			t.Errorf("git %v: Outer says %q, want its %s named", c.args, what, c.kind)
		}
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
		// Git by another name: testgit.Path, a path LookPath found, a shell's script.
		{`exec.Command(testgit.Path, "commit")`, 1},
		{`c := exec.Command("x"); c.Path = testgit.Path`, 1},
		{`if testgit.Path == "" { t.Skip() }`, 0},
		{`if testgit.Path != "" { t.Log() }`, 0},
		{`p, _ := exec.LookPath("git"); exec.Command(p, "commit")`, 1},
		{`var p string; p, _ = exec.LookPath("git"); exec.CommandContext(ctx, p, "commit")`, 1},
		{`p, _ := exec.LookPath("jq"); exec.Command(p, ".")`, 0},
		{`exec.Command("sh", "-c", "git init")`, 1},
		{`exec.Command("/bin/bash", "-c", "  git commit -m x")`, 1},
		{`exec.Command("sh", "-c", "gitleaks detect")`, 0},
		{`exec.Command("sh", "-c", "echo git")`, 0},
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
// exec.Command or exec.CommandContext whose program is git, by any name (a literal, a variable
// with git among its words, one the file assigns from exec.LookPath("git"), or a shell whose -c
// script starts with git); any use of testgit.Path but asking whether it is empty, since it is
// git by the one name the rest cannot see; and any use of signals.ExecRunner, the panel's runner,
// through which the code under test runs its git.
func gitCommands(fset *token.FileSet, f *ast.File) []string {
	var found []string
	at := func(n ast.Node) { found = append(found, fset.Position(n.Pos()).String()) }
	looked := map[string]bool{}           // names assigned from exec.LookPath("git")
	asked := map[*ast.SelectorExpr]bool{} // testgit.Path compared with ""
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			if len(n.Rhs) == 1 && lookPathGit(n.Rhs[0]) {
				if id, ok := n.Lhs[0].(*ast.Ident); ok {
					looked[id.Name] = true
				}
			}
		case *ast.ValueSpec:
			if len(n.Values) == 1 && lookPathGit(n.Values[0]) {
				looked[n.Names[0].Name] = true
			}
		case *ast.BinaryExpr:
			if n.Op == token.EQL || n.Op == token.NEQ {
				for _, p := range [][2]ast.Expr{{n.X, n.Y}, {n.Y, n.X}} {
					if s, ok := p[0].(*ast.SelectorExpr); ok && pkgSel(s, "testgit", "Path") && types.ExprString(p[1]) == `""` {
						asked[s] = true
					}
				}
			}
		}
		return true
	})
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if pkgSel(n, "signals", "ExecRunner") || pkgSel(n, "testgit", "Path") && !asked[n] {
				at(n)
			}
		case *ast.CallExpr:
			s, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				break
			}
			args := n.Args
			switch {
			case pkgSel(s, "exec", "Command"):
			case pkgSel(s, "exec", "CommandContext") && len(args) > 0:
				args = args[1:]
			default:
				return true
			}
			if len(args) > 0 && (namesGit(args[0], looked) || shellRunsGit(args)) {
				at(n)
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

// literal is e's value when it is a string literal.
func literal(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	return s, err == nil
}

// lookPathGit reports whether e is exec.LookPath("git").
func lookPathGit(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	s, ok := call.Fun.(*ast.SelectorExpr)
	name, lit := literal(call.Args[0])
	return ok && pkgSel(s, "exec", "LookPath") && lit && filepath.Base(name) == "git"
}

// shellRunsGit reports whether a command's program and arguments are sh or bash with a -c
// script that starts with git.
func shellRunsGit(args []ast.Expr) bool {
	sh, ok := literal(args[0])
	if !ok || filepath.Base(sh) != "sh" && filepath.Base(sh) != "bash" {
		return false
	}
	for i := 1; i+1 < len(args); i++ {
		if c, _ := literal(args[i]); c == "-c" {
			script, ok := literal(args[i+1])
			return ok && strings.HasPrefix(strings.TrimLeft(script, " \t\n")+" ", "git ")
		}
	}
	return false
}

// namesGit reports whether a program argument names git: a literal whose base name is git
// (`"git"`, `"/usr/bin/git"`, not `"gitleaks"`), a name in looked, or an expression with git as
// one of its words (`realGit`, `gitBin`, not `digits`).
func namesGit(e ast.Expr, looked map[string]bool) bool {
	if s, ok := literal(e); ok {
		return filepath.Base(s) == "git"
	}
	if id, ok := e.(*ast.Ident); ok && looked[id.Name] {
		return true
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
