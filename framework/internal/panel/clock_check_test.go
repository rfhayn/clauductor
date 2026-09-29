package panel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The panel has one clock (package clock), injected through Options. A direct read
// of the time package's clock (time.Now, time.Since, ...) or a timer made outside it
// is a second clock: the hub, the notifier and the queue view would each keep their
// own `now`, and a test could not pin it. Only package clock may call them; the
// shipped sources are scanned, tests excluded.
var clockCalls = map[string]bool{"Now": true, "Since": true, "Until": true, "After": true, "AfterFunc": true,
	"NewTimer": true, "NewTicker": true, "Tick": true, "Sleep": true}

// clockReads returns every call of the time package's clock in one parsed file.
func clockReads(fset *token.FileSet, f *ast.File) []string {
	timeName := ""
	for _, imp := range f.Imports {
		if p, _ := strconv.Unquote(imp.Path.Value); p == "time" {
			timeName = "time"
			if imp.Name != nil {
				timeName = imp.Name.Name
			}
		}
	}
	if timeName == "" || timeName == "_" {
		return nil
	}
	var bad []string
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == timeName && id.Obj == nil && clockCalls[sel.Sel.Name] {
			bad = append(bad, fset.Position(sel.Pos()).String()+": time."+sel.Sel.Name)
		}
		return true
	})
	return bad
}

func TestOnlyPackageClockReadsTheTime(t *testing.T) {
	fset := token.NewFileSet()
	scanned := 0
	for _, name := range panelSources(t) {
		if strings.HasPrefix(name, "clock"+string(filepath.Separator)) {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for _, b := range clockReads(fset, f) {
			t.Errorf("%s: use the injected clock.Clock", b)
		}
	}
	if scanned < 20 {
		t.Fatalf("scanned %d files; the walk is not seeing the panel's packages", scanned)
	}
}

func TestClockCheckCatchesEveryForm(t *testing.T) {
	// Falsification: each form a second clock can take is caught, and a method of the
	// same name on something else is not.
	for src, want := range map[string]int{
		`package x; import "time"; var n = time.Now()`:                                                               1,
		`package x; import "time"; func f(t time.Time) bool { return time.Since(t) > 0 }`:                            1,
		`package x; import tm "time"; func f() { tm.Sleep(1); <-tm.After(1) }`:                                       2,
		`package x; import "time"; var now = time.Now`:                                                               1, // a reference is a clock too
		`package x; import "time"; func f() { time.NewTicker(1); time.NewTimer(1) }`:                                 2,
		`package x; import "time"; type c struct{}; func (c) Now() time.Time { var x c; _ = x; return time.Time{} }`: 0,
		`package x; type clk struct{}; func (clk) Now() int { return 0 }; var time clk; var n = time.Now()`:          0,
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "x.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(clockReads(fset, f)); got != want {
			t.Errorf("%d clock reads in %s, want %d", got, src, want)
		}
	}
}
