package panel

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// C3: a body dropped because the processor is behind is counted, apart from
// foreign-cwd drops, and raises a banner.
func TestIngestOverflowIsCounted(t *testing.T) {
	s, hooks := newTestServer(t)
	for i := 0; i < cap(hooks)+3; i++ {
		if w := do(s, "POST", "/hook", `{"hook_event_name":"Stop","session_id":"x","cwd":"/repo"}`); w.Code != 204 {
			t.Fatalf("hook answered %d", w.Code)
		}
	}
	if s.Overflow() != 3 {
		t.Fatalf("overflow %d, want 3", s.Overflow())
	}
	m := NewModel(testConfig(t), "/repo", t0)
	m.ApplyObs(Obs{OverflowDrops: s.Overflow()})
	v := m.Snapshot(t0)
	if v.Dropped != 0 || v.Observe.OverflowDrops != 3 {
		t.Fatalf("overflow must be its own counter: %+v", v.Observe)
	}
	found := false
	for _, b := range v.Banners {
		found = found || strings.Contains(b, "fell behind")
	}
	if !found {
		t.Fatalf("no overflow banner: %v", v.Banners)
	}
}

// The panel only OBSERVES PermissionRequest (and PreCompact, which could block):
// /hook answers 204 with an empty body, which Claude Code reads as "no decision".
// Any body could be read as a decision, and /hook takes no token.
func TestHookNeverAnswersAPermissionRequest(t *testing.T) {
	s, _ := newTestServer(t)
	for _, ev := range []string{"PermissionRequest", "PreCompact", "StopFailure"} {
		w := do(s, "POST", "/hook", `{"hook_event_name":"`+ev+`","session_id":"x","cwd":"/repo","tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`)
		if w.Code != 204 || w.Body.Len() != 0 || w.Header().Get("Content-Type") != "" {
			t.Fatalf("%s: %d %q; must be 204 with no body", ev, w.Code, w.Body.String())
		}
	}
}

// C12: settings.json is re-read right before the rename; a concurrent write is kept
// and the panel's edit redone on top of it.
func TestInstallHooksKeepsAConcurrentWrite(t *testing.T) {
	home := t.TempDir()
	path := SettingsPath(home)
	writeFile(t, path, `{"model":"opus"}`)
	writes := 0
	beforeSettingsRename = func(p string) {
		if writes == 0 { // another writer lands between our read and our rename
			writes++
			os.WriteFile(p, []byte(`{"model":"opus","theme":"dark"}`), 0o644)
		}
	}
	defer func() { beforeSettingsRename = nil }()
	changed, err := InstallHooks(home, 4393)
	if err != nil || !changed {
		t.Fatalf("install: %v %v", changed, err)
	}
	var got map[string]any
	b, _ := os.ReadFile(path)
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["theme"] != "dark" {
		t.Fatalf("the concurrent write was lost: %s", b)
	}
	hooks, _ := got["hooks"].(map[string]any)
	if len(hooks) != len(HookEvents) {
		t.Fatalf("hooks not installed on top of it: %s", b)
	}
	// A writer that never stops: the install gives up rather than overwrite.
	beforeSettingsRename = func(p string) {
		writes++
		os.WriteFile(p, []byte(`{"n":`+strconv.Itoa(writes)+`}`), 0o644)
	}
	if _, err := InstallHooks(home, 4394); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("a constantly changing file must be refused: %v", err)
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
	"config.go":    1, // the panel config
	"installer.go": 2, // ~/.claude/settings.json, and its re-read before the rename
	"launchd.go":   6, // token (2), browser-opened stamp, binary copy, lane registries (uninstall)
	"hosts.go":     1, // the panel's pid file, checked before `panel open` sends the token
	"registry.go":  1, // the lane registry
	"trust.go":     1, // the trusted-config record
	"lease.go":     1, // a lease owner/waiter file
}

func TestNoSourceReachesTranscriptsByJoinOrNewReadSite(t *testing.T) {
	fset := token.NewFileSet()
	counts := map[string]int{}
	files, _ := filepath.Glob("*.go")
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
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
