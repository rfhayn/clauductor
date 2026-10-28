package testgit

import (
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

// A test binary started under a hook's GIT_DIR, GIT_WORK_TREE and GIT_INDEX_FILE leaves the
// hook's repository untouched: testgit's init strips them, so its fixture's git (run with the
// inherited environment, the way the code under test and a script run theirs, and from a test
// file's package-level variable, before any TestMain) stays in the fixture. Without the strip,
// the same git reaches the hook's repository.
func TestATestBinaryUnderAHookLeavesItsRepositoryAlone(t *testing.T) {
	for _, strip := range []bool{true, false} {
		env, touched := Outer(t)
		c := exec.Command(os.Args[0], "-test.run=^TestHelperRunsFixtureGit(AtInit)?$", "-test.count=1")
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
	reads := [][]string{{"status"}, {"rev-parse", "HEAD"}, {"log", "--oneline"}, {"worktree", "list"}}
	for _, c := range []struct {
		name   string
		args   [][]string
		global string // the global config the row's git reads, when it is not the machine's own
		want   string // a path the change names; "" for none
	}{
		{"a read", reads, "", ""},
		// A machine's global config can make a read write: git status adds the untracked cache to
		// the index, and fsmonitor starts a daemon with a socket in .git. The stand-in repository
		// pins its own config, so a read stays a read.
		{"a read, untracked cache on", reads, "[core]\n\tuntrackedCache = true\n", ""},
		{"a read, many files", reads, "[feature]\n\tmanyFiles = true\n", ""},
		{"a branch", [][]string{{"branch", "x"}}, "", ".git/refs/heads/x"},
		{"a worktree", [][]string{{"worktree", "add", "-q", "--detach", filepath.Join(t.TempDir(), "wt"), "HEAD"}}, "", ".git/worktrees"},
		{"the index", [][]string{{"update-index", "--add", "--cacheinfo", "100644,e69de29bb2d1d6434b8b29ae775ad8c2e48c5391,x"}}, "", ".git/index"}, // the empty blob
		{"the config", [][]string{{"config", "x.y", "z"}}, "", ".git/config"},
	} {
		env, touched := Outer(t)
		if c.global != "" {
			global := filepath.Join(t.TempDir(), "gitconfig")
			if err := os.WriteFile(global, []byte(c.global), 0o644); err != nil {
				t.Fatal(err)
			}
			env = append(env, "GIT_CONFIG_GLOBAL="+global)
		}
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

// Every package with tests imports testgit, so its init strips the hook's variables from the test
// binary before any of the package's code runs: the authority is every test binary, since every
// process a test starts inherits its environment. The walk takes every directory of the module
// that holds a _test.go file, and any of its test files (internal or external) may hold the
// import. The one exception is testgit itself, whose own init strips.
func TestEveryTestPackageImportsTestgit(t *testing.T) {
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
		if dir == self {
			continue
		}
		var parsed []*ast.File
		for _, f := range files {
			af, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatal(err)
			}
			parsed = append(parsed, af)
		}
		if !importsTestgit(parsed, module) {
			rel, _ := filepath.Rel(root, dir)
			bad = append(bad, rel)
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("these packages' tests do not import testgit (`import _ \"%s/internal/testgit\"` when they use nothing of it), so a git hook's GIT_DIR reaches every process they start (#302):\n%s", module, strings.Join(bad, "\n"))
	}
}

// The judge on packages of its own: one passes only when a test file imports testgit, used or
// blank, internal or external.
func TestImportsTestgitFindsAPackageThatDoesNot(t *testing.T) {
	const module = "example.com/m"
	for _, c := range []struct {
		name  string
		srcs  []string
		wants bool
	}{
		{"no import", []string{`package x; import "testing"`}, false},
		{"another package of the module", []string{`package x; import "example.com/m/internal/testbin"`}, false},
		{"a package named like it elsewhere", []string{`package x; import "other.com/internal/testgit"`}, false},
		{"an import", []string{`package x; import "example.com/m/internal/testgit"`}, true},
		{"a blank import", []string{`package x; import _ "example.com/m/internal/testgit"`}, true},
		{"in an external test file", []string{`package x; import "testing"`, `package x_test; import _ "example.com/m/internal/testgit"`}, true},
	} {
		var files []*ast.File
		for _, src := range c.srcs {
			f, err := parser.ParseFile(token.NewFileSet(), "x_test.go", src, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}
			files = append(files, f)
		}
		if got := importsTestgit(files, module); got != c.wants {
			t.Errorf("%s: importsTestgit says %v, want %v", c.name, got, c.wants)
		}
	}
}

// importsTestgit reports whether any of a package's test files imports testgit.
func importsTestgit(files []*ast.File, module string) bool {
	for _, f := range files {
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p == module+"/internal/testgit" {
				return true
			}
		}
	}
	return false
}
