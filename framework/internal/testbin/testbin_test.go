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
// one to a path the same function wrote. A mode the parser cannot read (a variable)
// counts as executable: the guard must not pass what it cannot see.
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
		written := map[string]bool{}
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, mode, path := osCall(call)
			switch {
			case name == "WriteFile" || name == "OpenFile":
				written[path] = true
				if executable(mode) {
					found = append(found, fset.Position(call.Pos()).String())
				}
			case name == "Chmod" && written[path] && executable(mode):
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
	case "WriteFile", "OpenFile", "Chmod":
		return sel.Sel.Name, call.Args[len(call.Args)-1], types.ExprString(call.Args[0])
	}
	return "", nil, ""
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
		{`os.Chmod(dir, 0o755)`, 0}, // a directory the test did not write
		{`os.MkdirAll(dir, 0o755)`, 0},
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

// Every test file in the module, not a list of the known ones: a test that writes its own
// executable is the flake this package exists to prevent.
func TestNoTestWritesAnExecutableAnotherWay(t *testing.T) {
	var found []string
	fset := token.NewFileSet()
	err := filepath.WalkDir("../..", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		found = append(found, selfWritten(fset, f)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) > 0 {
		t.Fatalf("write these with testbin.Write, or a parallel test's fork can leave them busy:\n%s", strings.Join(found, "\n"))
	}
}
