package panel

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/clauductor/clauductor/internal/panel/claudecall"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// transcriptReader is the one shipped source that may reach a transcript (PANEL-49
// D8): it reads them read-only, by a session id it has checked is a UUID, under
// Claude Code's config directory. The exemption is this exact path, slash-separated
// as io/fs walks it, so a file of the same name elsewhere is held to the rule.
const transcriptReader = "signals/transcript.go"

// transcriptScan is what one walk of the shipped sources found.
type transcriptScan struct {
	scanned       int
	offenders     []string // "<file>...: <how it reaches a transcript>", every file but the reader
	readerSeen    bool
	readerReaches bool
}

// scanTranscriptReaches walks fsys (the panel package's directory in production, a
// planted tree in TestTranscriptExemptionIsExactlyTheReader) and runs both scans
// over every shipped source (tests excluded, since they must name what they forbid):
// the string scan for the names a transcript is reached by, and, for Go, the AST
// scan for a path joined from parts. The reader's findings are kept apart from
// everyone else's: they are what proves the exemption is still needed.
func scanTranscriptReaches(fsys fs.FS) (transcriptScan, error) {
	var s transcriptScan
	forbidden := []string{".jsonl", "transcript_path", "TranscriptPath", "/.claude/projects"}
	fset := token.NewFileSet()
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Vendored third-party code is not the panel's source.
			if p == "testdata" || p == "web/vendor" {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, "_test.go") || !(strings.HasSuffix(p, ".go") || strings.HasSuffix(p, ".html") || strings.HasSuffix(p, ".js")) {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		s.scanned++
		var reaches []string
		// The panel's own login-lock log (PANEL-28), in its own directory, is a
		// .jsonl by design and no transcript: its one name is cut before the scan, so
		// any other ".jsonl" still fails.
		src := strings.ReplaceAll(string(b), claudecall.LogName, "")
		for _, f := range forbidden {
			if strings.Contains(src, f) {
				reaches = append(reaches, fmt.Sprintf("%s: mentions %q", p, f))
			}
		}
		// A path assembled from parts is caught too: filepath.Join(home, ".claude",
		// "projects") never contains the string "/.claude/projects" (review C15).
		if strings.HasSuffix(p, ".go") {
			f, err := parser.ParseFile(fset, p, b, 0)
			if err != nil {
				return err
			}
			reaches = append(reaches, transcriptReaches(fset, f)...)
		}
		if p == transcriptReader {
			s.readerSeen = true
			s.readerReaches = len(reaches) > 0
			return nil
		}
		s.offenders = append(s.offenders, reaches...)
		return nil
	})
	return s, err
}

// problems are the scan's failures: every other file that reaches a transcript, and
// an exemption that no longer exempts anything. A stale exemption fails rather than
// silently exempting nothing, so a reader moved or renamed is re-reviewed.
func (s transcriptScan) problems() []string {
	var out []string
	for _, o := range s.offenders {
		out = append(out, o+": only "+transcriptReader+" may read transcripts")
	}
	switch {
	case !s.readerSeen:
		out = append(out, transcriptReader+" is missing: the exemption names no file, so restore the reader or drop the exemption")
	case !s.readerReaches:
		out = append(out, transcriptReader+" no longer reaches a transcript: drop the stale exemption")
	}
	return out
}

// [MEASURES-4-S1] The panel reads transcripts in exactly one file (D8): they are
// documented as unstable between Claude Code versions, and reading them broadly is
// what made the old control room expensive. Every other shipped source of the
// package is held to the old rule, and the reader's exemption must stay live.
func TestNoSourceReadsTranscripts(t *testing.T) {
	t.Parallel()
	s, err := scanTranscriptReaches(os.DirFS("."))
	if err != nil {
		t.Fatal(err)
	}
	if s.scanned < 6 {
		t.Fatalf("scanned only %d files; the walk is not seeing the package", s.scanned)
	}
	for _, p := range s.problems() {
		t.Error(p)
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
	t.Parallel()
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

// [MEASURES-4-S1] Falsification of the exemption, over planted trees: a second file
// that reaches a transcript fails naming that file, and so do a missing reader and a
// reader that no longer reaches one. The exemption is the path, not the file name.
func TestTranscriptExemptionIsExactlyTheReader(t *testing.T) {
	t.Parallel()
	reader := &fstest.MapFile{Data: []byte(`package signals; var name = id + ".jsonl"`)}
	clean := &fstest.MapFile{Data: []byte(`package panel; var x = 1`)}
	for _, tc := range []struct {
		name  string
		tree  fstest.MapFS
		wants []string // each must appear in one problem; none means no problem at all
	}{
		{"the reader alone", fstest.MapFS{transcriptReader: reader, "runtime.go": clean}, nil},
		{"a .jsonl in a second Go file", fstest.MapFS{transcriptReader: reader, "runtime.go": {Data: []byte(`package panel; var p = id + ".jsonl"`)}},
			[]string{`runtime.go: mentions ".jsonl"`}},
		{"transcript_path in the page's script", fstest.MapFS{transcriptReader: reader, "web/static/panel.js": {Data: []byte(`const p = ev.transcript_path;`)}},
			[]string{`web/static/panel.js: mentions "transcript_path"`}},
		{"a joined path in a second file", fstest.MapFS{transcriptReader: reader, "state/x.go": {Data: []byte(`package state; import "path/filepath"; var p = filepath.Join(home, ".claude", "projects")`)}},
			[]string{"state/x.go:1:", "joins .claude and projects"}},
		{"a file named like the reader elsewhere", fstest.MapFS{transcriptReader: reader, "state/transcript.go": reader},
			[]string{`state/transcript.go: mentions ".jsonl"`}},
		{"the reader missing", fstest.MapFS{"runtime.go": clean},
			[]string{transcriptReader + " is missing"}},
		{"the reader renamed", fstest.MapFS{"signals/transcripts.go": reader},
			[]string{`signals/transcripts.go: mentions ".jsonl"`, transcriptReader + " is missing"}},
		{"the reader no longer reaches one", fstest.MapFS{transcriptReader: clean},
			[]string{transcriptReader + " no longer reaches a transcript"}},
	} {
		s, err := scanTranscriptReaches(tc.tree)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got := s.problems()
		if len(tc.wants) == 0 && len(got) > 0 {
			t.Errorf("%s: want no problem, got %q", tc.name, got)
		}
		for _, w := range tc.wants {
			found := false
			for _, p := range got {
				found = found || strings.Contains(p, w)
			}
			if !found {
				t.Errorf("%s: no problem names %q; got %q", tc.name, w, got)
			}
		}
	}
}

// fileReadSites are the reviewed places the package opens or reads a file. A new
// one fails this test until it is added here, with what it reads.
var fileReadSites = map[string]int{
	"config/config.go":     1, // the panel config
	"install/hooks.go":     2, // ~/.claude/settings.json, and its re-read before the rename
	"install/launchd.go":   6, // token (2), browser-opened stamp, binary copy, lane registries (uninstall)
	"runtime.go":           1, // a project's notifier state (notifier.json)
	"machine.go":           4, // the last quota (quota.json); the live-verified version (verified.json); the quota alert's notifier state; PANEL-28: the last account reading (account.json)
	"config/projects.go":   1, // the projects registry (projects.json)
	"install/open.go":      1, // the panel's pid file, checked before `panel open` sends the token
	"lanes/registry.go":    1, // the lane registry
	"install/trust.go":     2, // the trusted-config record; a sidecar's config, judged by the sidecar rule (PANEL-42)
	"install/init.go":      2, // the project's package.json and Makefile (`panel init`'s gate detection)
	"lease/lease.go":       4, // a lease owner/waiter file; the lease directory opened for flock(2) (2); /proc/<pid>/stat
	"install/singleton.go": 6, // the pid file, owner.json (3: PANEL-22's wait for a live add) and port marker; settings.json (hook drift)
	"install/candidate.go": 1, // PANEL-22: a directory the page names, opened to check the panel can read it
	"metrics/ledger.go":    1, // the project's spend ledger (spend.json)
	"economy.go":           2, // economy.json (the panel's own, at start); the project's .claude/model-roles.json
	"lanes/lifecycle.go":   1, // a .worktreeinclude file copied from the project root into a new worktree
	"lanes/lanes.go":       1, // PANEL-29: a listed worktree's git-dir rebase-merge/head-name, rebase-apply/head-name, BISECT_START
	"install/remote.go":    2, // the remote-control choice (remote-control.json); settings.json's remoteControlAtStartup
	"signals/changes.go":   2, // the project's .claude/project.conf (CHANGES_DIR); a change's proposal.md
	"claudecall/lock.go":   1, // PANEL-28: Claude Code's refresh-lock owner record (<lock dir>/.oauth_refresh.lock.owner), read only
	"claudecall/log.go":    1, // PANEL-28: the panel's own login-lock.jsonl, re-read to trim it to its cap
	transcriptReader:       1, // PANEL-49: a session's transcript and its subagents', read-only (os.Open alone), by a UUID-checked id under the config directory
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
	t.Parallel()
	fset := token.NewFileSet()
	counts := map[string]int{}
	for _, name := range panelSources(t) {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		// The reader is exempt here as in TestNoSourceReadsTranscripts, which also
		// fails if the exemption goes stale; its read sites are still counted below.
		if filepath.ToSlash(name) != transcriptReader {
			for _, b := range transcriptReaches(fset, f) {
				t.Error(b)
			}
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
