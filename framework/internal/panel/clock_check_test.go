package panel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The panel has one clock (package clock), injected through Options. A direct read
// of the time package's clock (time.Now, time.Since, ...), a timer made outside it,
// or a reach for clock.System instead of the injected clock is a second clock: the
// hub, the notifier and the queue view would each keep their own `now`, and a test
// could not pin it. Only package clock may call the time package's clock, and only
// the sites in systemClockSites may take clock.System; the shipped sources are
// scanned, tests excluded.
var clockCalls = map[string]bool{"Now": true, "Since": true, "Until": true, "After": true, "AfterFunc": true,
	"NewTimer": true, "NewTicker": true, "Tick": true, "Sleep": true}

const clockPkg = "github.com/clauductor/clauductor/internal/panel/clock"

// systemClockSites are the only uses of clock.System outside package clock, by file
// and function, each with the number of uses and why it is not injected.
var systemClockSites = map[string]map[string]int{
	// Run is where the clock is chosen: Options.Clock, else the system clock.
	"run.go": {"Run": 1},
	// A lease's staleness is judged against wall-clock times other processes wrote
	// into its files, so the cache's default and CancelWait read the wall clock.
	"lease/lease.go": {"ProcCache.Check": 1, "CancelWait": 1},
	// The pause between retries of a settings.json write that met a concurrent writer.
	"install/hooks.go": {"rewriteSettings": 1},
}

// clockReads returns every call of the time package's clock and every dot-import
// of it or of package clock (calls), and every use of clock.System by enclosing
// function (system), in one parsed file.
func clockReads(fset *token.FileSet, f *ast.File) (calls []string, system map[string]int) {
	system = map[string]int{}
	timeName, clockName := "", ""
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		if p != "time" && p != clockPkg {
			continue
		}
		name := filepath.Base(p)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if name == "." {
			calls = append(calls, fset.Position(imp.Pos()).String()+": dot-import of "+p)
			continue
		}
		if p == "time" {
			timeName = name
		} else {
			clockName = name
		}
	}
	for _, d := range f.Decls {
		fn := ""
		if fd, ok := d.(*ast.FuncDecl); ok {
			fn = fd.Name.Name
			if fd.Recv != nil && len(fd.Recv.List) > 0 {
				t := fd.Recv.List[0].Type
				if s, ok := t.(*ast.StarExpr); ok {
					t = s.X
				}
				if id, ok := t.(*ast.Ident); ok {
					fn = id.Name + "." + fn
				}
			}
		}
		ast.Inspect(d, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok || id.Obj != nil {
				return true
			}
			switch {
			case timeName != "" && id.Name == timeName && clockCalls[sel.Sel.Name]:
				calls = append(calls, fset.Position(sel.Pos()).String()+": time."+sel.Sel.Name)
			case clockName != "" && id.Name == clockName && sel.Sel.Name == "System":
				system[fn]++
			}
			return true
		})
	}
	return calls, system
}

func TestOnlyPackageClockReadsTheTime(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	scanned := 0
	seen := map[string]map[string]int{}
	for _, name := range panelSources(t) {
		if strings.HasPrefix(name, "clock"+string(filepath.Separator)) {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		calls, system := clockReads(fset, f)
		for _, b := range calls {
			t.Errorf("%s: use the injected clock.Clock", b)
		}
		if len(system) > 0 {
			seen[filepath.ToSlash(name)] = system
		}
	}
	if scanned < 20 {
		t.Fatalf("scanned %d files; the walk is not seeing the panel's packages", scanned)
	}
	files := make([]string, 0, len(seen))
	for file := range seen {
		files = append(files, file)
	}
	sort.Strings(files)
	for _, file := range files {
		for fn, n := range seen[file] {
			if systemClockSites[file][fn] != n {
				t.Errorf("%s: %s uses clock.System %d time(s), allowed %d: take the injected clock, or add the site to systemClockSites with why",
					file, fn, n, systemClockSites[file][fn])
			}
		}
	}
	for file, fns := range systemClockSites {
		for fn, n := range fns {
			if seen[file][fn] == 0 && n > 0 {
				t.Errorf("%s: %s is allowed clock.System but no longer uses it: remove it from systemClockSites", file, fn)
			}
		}
	}
}

func TestClockCheckCatchesEveryForm(t *testing.T) {
	t.Parallel()
	// Falsification: each form a second clock can take is caught, and a method of the
	// same name on something else is not.
	for src, want := range map[string]int{
		`package x; import "time"; var n = time.Now()`:                                                               1,
		`package x; import "time"; func f(t time.Time) bool { return time.Since(t) > 0 }`:                            1,
		`package x; import tm "time"; func f() { tm.Sleep(1); <-tm.After(1) }`:                                       2,
		`package x; import "time"; var now = time.Now`:                                                               1, // a reference is a clock too
		`package x; import "time"; func f() { time.NewTicker(1); time.NewTimer(1) }`:                                 2,
		`package x; import . "time"; var n = Now()`:                                                                  1, // a dot-import hides the calls
		`package x; import . "github.com/clauductor/clauductor/internal/panel/clock"; var c = System`:                1,
		`package x; import "time"; type c struct{}; func (c) Now() time.Time { var x c; _ = x; return time.Time{} }`: 0,
		`package x; type clk struct{}; func (clk) Now() int { return 0 }; var time clk; var n = time.Now()`:          0,
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "x.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := clockReads(fset, f); len(got) != want {
			t.Errorf("%d clock reads in %s, want %d", len(got), src, want)
		}
	}
	// clock.System is counted by the function that takes it, however the import is named.
	for src, want := range map[string]map[string]int{
		`package x; import "github.com/clauductor/clauductor/internal/panel/clock"; func f() { _ = clock.System }`:                           {"f": 1},
		`package x; import c "github.com/clauductor/clauductor/internal/panel/clock"; type T struct{}; func (*T) g() { _ = c.System.Now() }`: {"T.g": 1},
		`package x; import "github.com/clauductor/clauductor/internal/panel/clock"; var sys = clock.System`:                                  {"": 1},
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "x.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, got := clockReads(fset, f); len(got) != len(want) || got[""] != want[""] || got["f"] != want["f"] || got["T.g"] != want["T.g"] {
			t.Errorf("clock.System uses in %s: %v, want %v", src, got, want)
		}
	}
}
