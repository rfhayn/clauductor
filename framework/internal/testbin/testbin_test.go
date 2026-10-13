package testbin

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
)

// Scripts written and run at once while other goroutines fork without pause, as a
// package's parallel tests do: none may fail as busy. Only Linux refuses to exec a file
// open for writing, so elsewhere this passes with or without the guard.
func TestAScriptRunsAtOnceBesideForks(t *testing.T) {
	if testing.Short() {
		t.Skip("forks without pause for a few seconds")
	}
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no true(1)")
	}
	var stop atomic.Bool
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				_ = exec.Command(truePath).Run()
			}
		}()
	}
	defer func() { stop.Store(true); wg.Wait() }()
	dir := t.TempDir()
	const runs = 200
	busy := 0
	for i := range runs {
		p := filepath.Join(dir, "s"+strconv.Itoa(i))
		Write(t, p, "#!/bin/sh\nexit 0\n")
		switch err := exec.Command(p).Run(); {
		case errors.Is(err, syscall.ETXTBSY):
			busy++
		case err != nil:
			t.Fatal(err)
		}
	}
	if busy > 0 {
		t.Fatalf("%d of %d scripts run as soon as they were written failed as busy (ETXTBSY)", busy, runs)
	}
	if fi, err := os.Stat(filepath.Join(dir, "s0")); err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("the script is not executable: %v %v", fi, err)
	}
}

// selfWritten names each place in src where a test writes an executable itself: an
// os.WriteFile or os.OpenFile whose mode has an execute bit, or an os.Chmod that adds
// one to anything but a directory the same function made (t.TempDir, os.Mkdir,
// os.MkdirAll). A file a helper wrote is still a file (#107). A mode the parser cannot
// read (a variable) counts as executable: the guard must not pass what it cannot see,
// so a plain file's mode is written as a literal, or a constant declared in the same file.
func selfWritten(fset *token.FileSet, f *ast.File) []string {
	var found []string
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncLit)
		var body *ast.BlockStmt
		if ok {
			body = fn.Body
		} else if fd, ok := n.(*ast.FuncDecl); ok {
			body = fd.Body
		}
		if body == nil {
			return true
		}
		dirs := map[string]bool{}
		ast.Inspect(body, func(n ast.Node) bool {
			if as, ok := n.(*ast.AssignStmt); ok && len(as.Lhs) == len(as.Rhs) {
				for i, rhs := range as.Rhs {
					if tempDir(rhs) {
						dirs[types.ExprString(as.Lhs[i])] = true
					}
				}
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, mode, path := osCall(call)
			switch {
			case name == "Mkdir" || name == "MkdirAll":
				dirs[path] = true
			case name == "WriteFile" || name == "OpenFile":
				if executable(mode) {
					found = append(found, fset.Position(call.Pos()).String())
				}
			case name == "Chmod" && !dirs[path] && !tempDir(call.Args[0]) && executable(mode):
				found = append(found, fset.Position(call.Pos()).String())
			}
			return true
		})
		return false // a nested function literal was walked with its parent
	})
	return found
}

// osCall is call's os function name, its mode argument and its path, as source text.
func osCall(call *ast.CallExpr) (name string, mode ast.Expr, path string) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", nil, ""
	}
	if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "os" {
		return "", nil, ""
	}
	if len(call.Args) < 2 {
		return "", nil, ""
	}
	switch sel.Sel.Name {
	case "WriteFile", "OpenFile", "Chmod", "Mkdir", "MkdirAll":
		return sel.Sel.Name, call.Args[len(call.Args)-1], types.ExprString(call.Args[0])
	}
	return "", nil, ""
}

// importsTesting reports whether f imports "testing", which is what makes a file that is
// not a test test support.
func importsTesting(f *ast.File) bool {
	for _, imp := range f.Imports {
		if imp.Path.Value == `"testing"` {
			return true
		}
	}
	return false
}

// tempDir reports whether e is a call of TempDir() on a test (t.TempDir()).
func tempDir(e ast.Expr) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "TempDir"
}

// executable reports whether mode, a literal, a conversion of one (fs.FileMode(0o755))
// or a constant declared in the file, has an execute bit; anything else is assumed to.
func executable(mode ast.Expr) bool {
	switch m := mode.(type) {
	case *ast.BasicLit:
		v, err := strconv.ParseUint(strings.ReplaceAll(m.Value, "_", ""), 0, 32)
		return err != nil || v&0o111 != 0
	case *ast.ParenExpr:
		return executable(m.X)
	case *ast.CallExpr:
		if len(m.Args) == 1 {
			return executable(m.Args[0])
		}
	case *ast.Ident:
		if m.Obj != nil && m.Obj.Kind == ast.Con {
			if vs, ok := m.Obj.Decl.(*ast.ValueSpec); ok {
				for i, n := range vs.Names {
					if n.Name == m.Name && i < len(vs.Values) {
						return executable(vs.Values[i])
					}
				}
			}
		}
	}
	return true
}

func TestTheGuardSeesEveryWayOfWritingAnExecutable(t *testing.T) {
	for _, c := range []struct {
		src  string
		want int
	}{
		{`os.WriteFile(p, b, 0o644)`, 0},
		{`os.WriteFile(p, b, 0644)`, 0},
		{`os.WriteFile(p, b, 0o755)`, 1},
		{`os.WriteFile(p, b, 0755)`, 1},
		{`os.WriteFile(p, b, 0o0755)`, 1},
		{"os.WriteFile(p,\n\t[]byte(`x`),\n\t0o755)", 1},
		{`os.WriteFile(p, b, fs.FileMode(0o755))`, 1},
		{`os.WriteFile(p, b, fs.FileMode(0o600))`, 0},
		{`os.WriteFile(p, b, exe)`, 1},
		{`os.WriteFile(p, b, plain)`, 0},
		{`os.WriteFile(p, b, mode)`, 1}, // a variable: unreadable, so counted
		{`os.OpenFile(p, os.O_CREATE|os.O_WRONLY, 0o700)`, 1},
		{`os.WriteFile(p, b, 0o644); os.Chmod(p, 0o755)`, 1},
		{`writeFile(t, fake, "x"); os.Chmod(fake, 0o755)`, 1}, // written by a helper (#107)
		{`os.Chmod(p, 0o755)`, 1},                             // written who knows where
		{`os.Chmod(p, 0o644)`, 0},
		{`dir := t.TempDir(); os.Chmod(dir, 0o755)`, 0},
		{`dir := t.TempDir(); t.Cleanup(func() { os.Chmod(dir, 0o755) })`, 0},
		{`os.Mkdir(dir, 0); os.Chmod(dir, 0o755)`, 0},
		{`os.MkdirAll(dir, 0o755)`, 0},
		{`os.MkdirAll(dir, 0o700); os.Chmod(dir, 0o755)`, 0},
		{`dir := t.TempDir(); os.Chmod(filepath.Join(dir, "x"), 0o755)`, 1},
		{`func() { os.WriteFile(p, b, 0o755) }()`, 1},
	} {
		src := "package x\nconst exe, plain = 0o755, 0o644\nfunc f() {\n" + c.src + "\n}\n"
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "x_test.go", src, 0)
		if err != nil {
			t.Fatalf("%q: %v", c.src, err)
		}
		if got := selfWritten(fset, f); len(got) != c.want {
			t.Errorf("%q: found %v, want %d", c.src, got, c.want)
		}
	}
}

// Every test file in the module, and every file of test support (one that imports
// "testing" outside a _test.go file: leakcheck, testwait), not a list of the known ones: a
// test that writes its own executable is the flake this package exists to prevent. Only
// this package's own writer is exempt; the rest of the module's code writes executables
// (install's hooks) that no test runs at once.
func TestNoTestWritesAnExecutableAnotherWay(t *testing.T) {
	self, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	support := map[string]bool{}
	fset := token.NewFileSet()
	err = filepath.WalkDir("../..", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		if !strings.HasSuffix(p, "_test.go") {
			dir, err := filepath.Abs(filepath.Dir(p))
			if err != nil {
				return err
			}
			if dir == self || !importsTesting(f) {
				return nil
			}
			support[filepath.Base(p)] = true
		}
		found = append(found, selfWritten(fset, f)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"leakcheck.go", "testwait.go"} {
		if !support[name] {
			t.Errorf("%s was not scanned as test support (scanned: %v)", name, support)
		}
	}
	if len(found) > 0 {
		t.Fatalf("write these with testbin.Write, or a parallel test's fork can leave them busy:\n%s", strings.Join(found, "\n"))
	}
}
