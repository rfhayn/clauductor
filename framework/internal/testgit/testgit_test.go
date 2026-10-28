package testgit

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestMain strips like every package's, unless a probe asks its helper not to: that is the
// probe's negative case, a test binary whose TestMain forgot.
func TestMain(m *testing.M) {
	if os.Getenv("TESTGIT_HELPER_NOSTRIP") == "" {
		Strip()
	}
	os.Exit(m.Run())
}

// A test binary started under a hook's GIT_DIR, GIT_WORK_TREE and GIT_INDEX_FILE leaves the
// hook's repository untouched: its TestMain strips them, so its fixture's git (run with the
// inherited environment, the way the code under test and a script run theirs) stays in the
// fixture. Without the strip, the same git reaches the hook's repository.
func TestATestBinaryUnderAHookLeavesItsRepositoryAlone(t *testing.T) {
	for _, strip := range []bool{true, false} {
		env, touched := Outer(t)
		c := exec.Command(os.Args[0], "-test.run=^TestHelperRunsFixtureGit$", "-test.count=1")
		c.Env = append(append(os.Environ(), env...), "TESTGIT_HELPER=1")
		if !strip {
			c.Env = append(c.Env, "TESTGIT_HELPER_NOSTRIP=1")
		}
		out, err := c.CombinedOutput()
		switch what := touched(); {
		case strip && (err != nil || what != ""):
			t.Errorf("with the strip: %v, %s\n%s", err, what, out)
		case !strip && what == "":
			t.Errorf("without the strip the hook's repository stayed untouched: the probe sees nothing\n%s", out)
		}
	}
}

// TestHelperRunsFixtureGit is the probe's helper: a fixture's git as the code under test runs
// it, with the process's own environment, directly and from a script.
func TestHelperRunsFixtureGit(t *testing.T) {
	if os.Getenv("TESTGIT_HELPER") != "1" {
		t.Skip("run by TestATestBinaryUnderAHookLeavesItsRepositoryAlone")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, argv := range [][]string{
		{"git", "init", "-q"},
		{"git", "add", "a.txt"},
		{"git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "fixture"},
		{"git", "branch", "x"},
		{"git", "worktree", "add", "-q", "--detach", filepath.Join(t.TempDir(), "wt"), "HEAD"},
		{"git", "config", "fixture.key", "value"},
		{"sh", "-c", "git -c user.name=t -c user.email=t@t commit -q --allow-empty -m script"},
	} {
		c := exec.Command(argv[0], argv[1:]...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Errorf("%v: %v\n%s", argv, err, out) // and go on: without the strip, the rest still reach the hook's repository
		}
	}
}

// Outer sees each kind of change a git under the hook's environment can make there, none of
// which moves HEAD, and a read is no change.
func TestOuterSeesEachChange(t *testing.T) {
	for _, c := range []struct {
		name string
		args [][]string
		want string // a path the change names; "" for none
	}{
		{"a read", [][]string{{"status"}, {"rev-parse", "HEAD"}, {"log", "--oneline"}, {"worktree", "list"}}, ""},
		{"a branch", [][]string{{"branch", "x"}}, ".git/refs/heads/x"},
		{"a worktree", [][]string{{"worktree", "add", "-q", "--detach", filepath.Join(t.TempDir(), "wt"), "HEAD"}}, ".git/worktrees"},
		{"the index", [][]string{{"update-index", "--add", "--cacheinfo", "100644,e69de29bb2d1d6434b8b29ae775ad8c2e48c5391,x"}}, ".git/index"}, // the empty blob
		{"the config", [][]string{{"config", "x.y", "z"}}, ".git/config"},
	} {
		env, touched := Outer(t)
		for _, args := range c.args {
			g := exec.Command(Path, args...)
			g.Dir, g.Env = t.TempDir(), append(os.Environ(), env...)
			if out, err := g.CombinedOutput(); err != nil {
				t.Fatalf("%s: git %v under the hook's environment: %v\n%s", c.name, args, err, out)
			}
		}
		what := touched()
		if c.want == "" && what != "" || c.want != "" && !strings.Contains(what, c.want) {
			t.Errorf("%s: Outer says %q, want %q named", c.name, what, c.want)
		}
	}
}

// Every package with tests strips the hook's variables in its TestMain, first: the authority is
// every test binary, since every process a test starts inherits its environment. The walk takes
// every directory of the module that holds a _test.go file. Two kinds are exempt, and each is
// held to running no git: this package, whose TestMain strips but for the probe's negative case,
// and a package testgit imports, which cannot import testgit back.
func TestEveryTestPackageStripsGit(t *testing.T) {
	self, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	module := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(string(mod), "\n", 2)[0], "module"))
	exempt := map[string]bool{}
	for _, imp := range moduleImports(t, self, module) {
		exempt[filepath.Join(root, strings.TrimPrefix(imp, module+"/"))] = true
	}
	tests := map[string][]string{} // directory: its _test.go files
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, "_test.go") {
			tests[filepath.Dir(p)] = append(tests[filepath.Dir(p)], p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) < 15 {
		t.Fatalf("only %d test packages found under %s: the walk did not reach the module", len(tests), root)
	}
	var bad []string
	for dir, files := range tests {
		rel, _ := filepath.Rel(root, dir)
		if dir == self || exempt[dir] {
			for _, f := range files {
				if b, _ := os.ReadFile(f); dir != self && bytes.Contains(b, []byte(`"git"`)) {
					bad = append(bad, rel+": exempt (testgit imports it, so it cannot strip), but "+filepath.Base(f)+" runs git")
				}
			}
			continue
		}
		var parsed []*ast.File
		for _, f := range files {
			af, err := parser.ParseFile(token.NewFileSet(), f, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			parsed = append(parsed, af)
		}
		if why := unstripped(parsed); why != "" {
			bad = append(bad, rel+": "+why)
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("run testgit.Main(m) as TestMain, or call testgit.Strip() first in it, or a git hook's GIT_DIR reaches every process these tests start (#302):\n%s", strings.Join(bad, "\n"))
	}
}

// The judge on packages of its own: none passes without a TestMain whose first statement strips.
func TestUnstrippedFindsAPackageThatDoesNot(t *testing.T) {
	for _, c := range []struct {
		name, src string
		ok        bool
	}{
		{"no TestMain", "func TestX(t *testing.T) {}", false},
		{"a TestMain that does not strip", "func TestMain(m *testing.M) { os.Exit(m.Run()) }", false},
		{"one that strips too late", "func TestMain(m *testing.M) { os.Unsetenv(\"K\"); testgit.Strip(); os.Exit(leakcheck.Main(m)) }", false},
		{"one that strips in a branch", "func TestMain(m *testing.M) { if x { testgit.Strip() }; os.Exit(m.Run()) }", false},
		{"testgit.Main", "func TestMain(m *testing.M) { testgit.Main(m) }", true},
		{"Strip, then leakcheck.Main", "func TestMain(m *testing.M) { testgit.Strip(); os.Exit(leakcheck.Main(m)) }", true},
	} {
		f, err := parser.ParseFile(token.NewFileSet(), "x_test.go", "package x\n"+c.src+"\n", 0)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if why := unstripped([]*ast.File{f}); (why == "") != c.ok {
			t.Errorf("%s: unstripped says %q, want ok %v", c.name, why, c.ok)
		}
	}
}

// unstripped says why a package's test files leave the hook's variables in place, or "" when
// their TestMain strips them first (testgit.Main, or testgit.Strip as its first statement).
func unstripped(files []*ast.File) string {
	for _, f := range files {
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Recv != nil || fd.Name.Name != "TestMain" {
				continue
			}
			if len(fd.Body.List) > 0 {
				if es, ok := fd.Body.List[0].(*ast.ExprStmt); ok {
					if call, ok := es.X.(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
							if x, ok := sel.X.(*ast.Ident); ok && x.Name == "testgit" && (sel.Sel.Name == "Strip" || sel.Sel.Name == "Main") {
								return ""
							}
						}
					}
				}
			}
			return "its TestMain does not call testgit.Strip (or testgit.Main) first"
		}
	}
	return "no TestMain"
}

// moduleImports is every package of the module that dir's non-test files import, directly or
// through each other.
func moduleImports(t *testing.T, dir, module string) []string {
	t.Helper()
	root := filepath.Join(dir, "..", "..")
	seen := map[string]bool{}
	var walk func(dir string)
	walk = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, e.Name()), nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			for _, imp := range f.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				if strings.HasPrefix(p, module+"/") && !seen[p] {
					seen[p] = true
					walk(filepath.Join(root, strings.TrimPrefix(p, module+"/")))
				}
			}
		}
	}
	walk(dir)
	var out []string
	for p := range seen {
		out = append(out, p)
	}
	return out
}
