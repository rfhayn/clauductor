package panel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// The panel must never read transcripts: they are documented as unstable between
// Claude Code versions, and reading them live is what made the old control room
// expensive. This scans every shipped source file of the package (tests excluded,
// since they must name what they forbid) for the ways a transcript would be reached.
func TestNoSourceReadsTranscripts(t *testing.T) {
	forbidden := []string{".jsonl", "transcript_path", "TranscriptPath", "/.claude/projects"}
	var scanned int
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Vendored third-party code is not the panel's source.
			if path == "testdata" || path == filepath.Join("web", "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") || !(strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".html") || strings.HasSuffix(path, ".js")) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		for _, f := range forbidden {
			if strings.Contains(string(b), f) {
				t.Errorf("%s mentions %q: the panel must not read transcripts", path, f)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 6 {
		t.Fatalf("scanned only %d files; the walk is not seeing the package", scanned)
	}
	// A path assembled from parts is caught too: filepath.Join(home, ".claude",
	// "projects") never contains the string "/.claude/projects" (review C15).
	fset := token.NewFileSet()
	for _, name := range panelSources(t) {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range transcriptReaches(fset, f) {
			t.Errorf("%s: the panel must not read transcripts", b)
		}
	}
	// And the decoded payload types cannot carry a transcript path in memory.
	for _, typ := range []reflect.Type{reflect.TypeOf(signals.HookEvent{}), reflect.TypeOf(signals.StatusPayload{})} {
		for i := 0; i < typ.NumField(); i++ {
			if strings.Contains(strings.ToLower(typ.Field(i).Tag.Get("json")), "transcript") {
				t.Errorf("%s.%s decodes a transcript field", typ.Name(), typ.Field(i).Name)
			}
		}
	}
}

// C15: no transcript is reachable by a path assembled from parts either, and every
// file read in the package is at a reviewed call site.
func transcriptReaches(fset *token.FileSet, f *ast.File) []string {
	var bad []string
	lit := func(e ast.Expr) string {
		if b, ok := e.(*ast.BasicLit); ok && b.Kind == token.STRING {
			s, _ := strconv.Unquote(b.Value)
			return s
		}
		return ""
	}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Join" {
			return true
		}
		var claude, projects bool
		for _, a := range call.Args {
			switch strings.Trim(lit(a), "/") {
			case ".claude":
				claude = true
			case "projects", ".claude/projects":
				projects = true
				claude = claude || strings.Contains(lit(a), ".claude")
			}
		}
		if claude && projects {
			bad = append(bad, fset.Position(call.Pos()).String()+": joins .claude and projects")
		}
		return true
	})
	return bad
}

func TestTranscriptScannerCatchesJoinedPaths(t *testing.T) {
	// Falsification: the scanner must catch the forms the string test cannot.
	for _, src := range []string{
		`package x; import "path/filepath"; var p = filepath.Join(home, ".claude", "projects")`,
		`package x; import "path"; var p = path.Join(home, ".claude", "projects", id)`,
		`package x; import "path/filepath"; var p = filepath.Join(home, ".claude/projects")`,
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "x.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(transcriptReaches(fset, f)) == 0 {
			t.Errorf("missed: %s", src)
		}
	}
}

// fileReadSites are the reviewed places the package opens or reads a file. A new
// one fails this test until it is added here, with what it reads.
var fileReadSites = map[string]int{
	"config/config.go":  1, // the panel config
	"installer.go":      2, // ~/.claude/settings.json, and its re-read before the rename
	"launchd.go":        6, // token (2), browser-opened stamp, binary copy, lane registries (uninstall)
	"runtime_v2.go":     1, // the notifier's saved state (notifier.json)
	"hosts.go":          1, // the panel's pid file, checked before `panel open` sends the token
	"lanes/registry.go": 1, // the lane registry
	"trust.go":          1, // the trusted-config record
	"lease/lease.go":    4, // a lease owner/waiter file; the lease directory opened for flock(2) (2); /proc/<pid>/stat
	"singleton.go":      5, // the pid file, owner.json (2) and port marker; settings.json (hook drift)
}

// panelSources lists every shipped Go file of the panel and its packages, relative
// to this directory: the file tree is the authority, not a list of packages.
func panelSources(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || path == filepath.Join("web", "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNoSourceReachesTranscriptsByJoinOrNewReadSite(t *testing.T) {
	fset := token.NewFileSet()
	counts := map[string]int{}
	for _, name := range panelSources(t) {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range transcriptReaches(fset, f) {
			t.Error(b)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "os" {
					switch sel.Sel.Name {
					case "Open", "OpenFile", "ReadFile", "ReadDir":
						if sel.Sel.Name == "OpenFile" || sel.Sel.Name == "ReadDir" {
							return true // writes (logs, cancel files) and directory listings
						}
						counts[name]++
					}
				}
			}
			return true
		})
	}
	for name, n := range counts {
		if fileReadSites[name] != n {
			t.Errorf("%s reads files at %d site(s), reviewed %d: check none reaches a transcript, then update fileReadSites", name, n, fileReadSites[name])
		}
	}
	for name, n := range fileReadSites {
		if counts[name] == 0 && n > 0 {
			t.Errorf("%s is listed with %d read site(s) but has none: remove it", name, n)
		}
	}
}
