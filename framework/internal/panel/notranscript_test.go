package panel

import (
	"encoding"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/rfhayn/stridal/internal/panel/claudecall"
	"github.com/rfhayn/stridal/internal/panel/signals"
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

// stopFieldAllowlist is what PANEL-51's D4 lets the panel decode from a Stop's
// background_tasks and session_crons, by the type each is decoded into. A task's
// command, server, tool, name and agent_type, and a cron's prompt, are never among
// them: a command line can hold a secret.
var stopFieldAllowlist = map[reflect.Type]map[string]bool{
	reflect.TypeOf(signals.BackgroundTask{}): {"id": true, "type": true, "status": true, "description": true},
	reflect.TypeOf(signals.SessionCron{}):    {"schedule": true, "recurring": true},
}

// [STALL-2-S5] The decoded hook types hold only D4's allowlist: HookEvent's two new
// fields are lists of exactly the two types above, and each type, walked by
// reflection, declares every allowlisted field and nothing else.
func TestStopFieldTypesHoldOnlyTheAllowlist(t *testing.T) {
	t.Parallel()
	hook := reflect.TypeOf(signals.HookEvent{})
	elems := map[string]reflect.Type{}
	for i := 0; i < hook.NumField(); i++ {
		f := hook.Field(i)
		switch tag := strings.Split(f.Tag.Get("json"), ",")[0]; tag {
		case "background_tasks", "session_crons":
			if f.Type.Kind() != reflect.Slice {
				t.Errorf("HookEvent.%s decodes %q into a %s, not a list of an allowlisted type", f.Name, tag, f.Type)
				continue
			}
			elems[tag] = f.Type.Elem()
		}
	}
	for tag, want := range map[string]reflect.Type{"background_tasks": reflect.TypeOf(signals.BackgroundTask{}),
		"session_crons": reflect.TypeOf(signals.SessionCron{})} {
		if elems[tag] != want {
			t.Errorf("HookEvent decodes %q into %v, want a list of %v", tag, elems[tag], want)
		}
	}
	for typ, allowed := range stopFieldAllowlist {
		for _, p := range decodeTypeProblems(typ, allowed) {
			t.Error(p)
		}
		declared := map[string]bool{}
		for i := 0; i < typ.NumField(); i++ {
			declared[strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]] = true
		}
		for name := range allowed {
			if !declared[name] {
				t.Errorf("%s does not declare %q", typ.Name(), name)
			}
		}
		for _, forbidden := range []string{"command", "prompt", "server", "tool", "name", "agent_type"} {
			if declared[forbidden] {
				t.Errorf("%s declares %q, which D4 never decodes", typ.Name(), forbidden)
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
	"config/config.go":      1, // the panel config
	"install/hooks.go":      2, // ~/.claude/settings.json, and its re-read before the rename
	"install/launchd.go":    6, // token (2), browser-opened stamp, binary copy, lane registries (uninstall)
	"runtime.go":            2, // a project's notifier state (notifier.json); PANEL-51: its stall clocks (stall.json: lane and session clocks, ask times, kept Stop allowlists, SubagentStop agent ids)
	"machine.go":            4, // the last quota (quota.json); the live-verified version (verified.json); the quota alert's notifier state; PANEL-28: the last account reading (account.json)
	"config/projects.go":    1, // the projects registry (projects.json)
	"config/catchup.go":     1, // OPS-118: the catch-up record (catch-up.json under the panel's home: the roots catch-up is on for)
	"push.go":               1, // PANEL-51: the machine's push command (push.json: one JSON object, its argv alone), once its owner and modes are checked
	"install/open.go":       1, // the panel's pid file, checked before `panel open` sends the token
	"lanes/registry.go":     1, // the lane registry
	"install/trust.go":      3, // the trusted-config record; a sidecar's config, judged by the sidecar rule (PANEL-42); OPS-118: a config trusted on arrival, its bytes hashed against the blob catch-up judged
	"catchup.go":            1, // OPS-118: the project's own panel.json, compared with its blob at the commit catch-up moved to (D1 (c))
	"install/selfupdate.go": 3, // OPS-118 D4: framework/go.mod (ModuleOf); the rebuilt binary, copied over the PATH one (copyExecutable); the restart marker (TakeRestartMarker)
	"install/init.go":       2, // the project's package.json and Makefile (`panel init`'s gate detection)
	"lease/lease.go":        5, // a lease owner/waiter file; the lease directory opened for flock(2) (2); /proc/<pid>/stat (2: start time, zombie)
	"install/singleton.go":  6, // the pid file, owner.json (3: PANEL-22's wait for a live add) and port marker; settings.json (hook drift)
	"install/candidate.go":  1, // PANEL-22: a directory the page names, opened to check the panel can read it
	"metrics/ledger.go":     1, // the project's spend ledger (spend.json)
	"economy.go":            2, // economy.json (the panel's own, at start); the project's .claude/model-roles.json
	"lanes/lifecycle.go":    1, // a .worktreeinclude file copied from the project root into a new worktree
	"lanes/lanes.go":        1, // PANEL-29: a listed worktree's git-dir rebase-merge/head-name, rebase-apply/head-name, BISECT_START
	"install/remote.go":     2, // the remote-control choice (remote-control.json); settings.json's remoteControlAtStartup
	"signals/changes.go":    2, // the project's .claude/project.conf (CHANGES_DIR); a change's proposal.md
	"claudecall/lock.go":    1, // PANEL-28: Claude Code's refresh-lock owner record (<lock dir>/.oauth_refresh.lock.owner), read only
	"claudecall/log.go":     1, // PANEL-28: the panel's own login-lock.jsonl, re-read to trim it to its cap
	transcriptReader:        1, // PANEL-49: a session's transcript and its subagents', read-only (os.Open alone), by a UUID-checked id under the config directory
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

// The reader's decode allowlist (PANEL-49 D2, D8): every JSON path its two decode
// types may hold, by type. Prompt text, tool input and results, file paths and
// assistant text are never among them. A field added to either type, at any depth,
// fails TestTranscriptReaderDecodesOnlyTheAllowlist until it is added here, by a
// design decision (PANEL-50 widens it by its own).
var transcriptAllowlist = map[reflect.Type]map[string]bool{
	reflect.TypeOf(signals.TranscriptHead{}): {"type": true, "version": true},
	reflect.TypeOf(signals.TranscriptAssistant{}): {
		"message": true, "message.id": true, "message.model": true, "message.usage": true,
		"message.usage.input_tokens": true, "message.usage.output_tokens": true,
		"message.usage.cache_creation_input_tokens": true, "message.usage.cache_read_input_tokens": true,
		"message.usage.cache_creation": true, "message.usage.cache_creation.ephemeral_1h_input_tokens": true,
		"message.usage.speed": true,
	},
}

var (
	jsonUnmarshaler = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()
	textUnmarshaler = reflect.TypeOf((*encoding.TextUnmarshaler)(nil)).Elem()
)

// decodeTypeProblems walks root by reflection, every nested struct included, and names
// each way encoding/json could keep more than allowed: a tag outside the allowlist,
// an exported field with no tag (matched by its name), an embedded struct (its fields
// are promoted), a leaf that is not a scalar or a pointer to one (json.RawMessage, a
// map, a slice, an interface keep content whole), and a type with its own
// UnmarshalJSON or UnmarshalText (which sees the raw bytes).
func decodeTypeProblems(root reflect.Type, allowed map[string]bool) []string {
	var out []string
	custom := func(t reflect.Type, where string) bool {
		for _, u := range []reflect.Type{jsonUnmarshaler, textUnmarshaler} {
			if t.Implements(u) || reflect.PointerTo(t).Implements(u) {
				out = append(out, fmt.Sprintf("%s: %s implements %s, so it sees the raw line", where, t, u.Name()))
				return true
			}
		}
		return false
	}
	var walk func(t reflect.Type, prefix, where string)
	walk = func(t reflect.Type, prefix, where string) {
		if custom(t, where) {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			at := where + "." + f.Name
			if f.Anonymous {
				out = append(out, at+": an embedded struct: its fields are decoded by their own names")
				continue
			}
			if !f.IsExported() {
				continue // encoding/json never sets an unexported field
			}
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if tag == "-" {
				continue
			}
			if tag == "" {
				out = append(out, at+": an exported field with no json tag, which encoding/json matches by its name")
				continue
			}
			path := prefix + tag
			if !allowed[path] {
				out = append(out, fmt.Sprintf("%s: decodes %q, outside the allowlist", at, path))
				continue
			}
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			switch ft.Kind() {
			case reflect.Struct:
				walk(ft, path+".", at)
			case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
				custom(ft, at)
			default:
				out = append(out, fmt.Sprintf("%s: decodes %q into a %s (%s), not a scalar: it would keep content whole", at, path, f.Type, ft.Kind()))
			}
		}
	}
	if root.Kind() != reflect.Struct {
		return []string{root.String() + ": a decode type must be a struct"}
	}
	walk(root, "", root.Name())
	return out
}

// readerImports is every package the reader may import. An allowlist, not a deny
// list: a package added later (os/exec, net, a decoder) is a problem until it is
// named here, so a read that bypasses os.Open cannot arrive through an import nobody
// thought to ban. None of these reads a file by a path on its own.
var readerImports = map[string]bool{
	"bytes": true, "encoding/json": true, "errors": true, "fmt": true, "io": true,
	"io/fs": true, "os": true, "path/filepath": true, "regexp": true, "strings": true,
	"strconv": true, "sort": true, "time": true, "unicode": true, "unicode/utf8": true,
}

// readerFSNames is what the reader may use from io/fs: its error sentinels and the
// types os.ReadDir and filepath.WalkDir hand back. fs.ReadFile, fs.FS and the rest
// open files through an fs.FS, not os.Open.
var readerFSNames = map[string]bool{
	"DirEntry": true, "FileInfo": true, "FileMode": true,
	"ErrNotExist": true, "ErrPermission": true, "ErrExist": true,
}

// readerOpenMethods are the names a value opens or reads a file by (an fs.FS, an
// fs.ReadFileFS, a value handed over from another file): a selector with one of these
// names is a problem unless it is os.Open.
var readerOpenMethods = map[string]bool{"Open": true, "OpenFile": true, "ReadFile": true, "Command": true, "CommandContext": true}

// readerDecodeProblems checks one source file by AST: it imports only readerImports,
// decodes JSON only with json.Unmarshal, only into a variable declared as one of the
// named types, and opens files with os.Open alone (os.Stat and os.ReadDir find them;
// nothing from io/fs but its sentinels and types, and no Open-like method on any
// other value). It counts each decode target it saw, so a type no longer decoded
// into shows.
func readerDecodeProblems(fset *token.FileSet, f *ast.File, decodeTypes []string) (problems []string, decoded map[string]int) {
	decoded = map[string]int{}
	allowedType := map[string]bool{}
	for _, n := range decodeTypes {
		allowedType[n] = true
	}
	jsonName, osName, fsName := "", "", ""
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		name := path.Base(p)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		switch {
		case !readerImports[p]:
			problems = append(problems, fmt.Sprintf("%s: imports %s: a transcript is decoded with encoding/json and opened with os.Open alone", fset.Position(imp.Pos()), p))
		case name == "." || name == "_":
			problems = append(problems, fmt.Sprintf("%s: imports %s as %s: the check reads each use by its package name", fset.Position(imp.Pos()), p, name))
		case p == "encoding/json":
			jsonName = name
		case p == "os":
			osName = name
		case p == "io/fs":
			fsName = name
		}
	}
	pkgSel := func(e ast.Expr, pkg string) (string, bool) {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok || pkg == "" {
			return "", false
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != pkg {
			return "", false
		}
		return sel.Sel.Name, true
	}
	called := map[ast.Expr]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if name, ok := pkgSel(call.Fun, jsonName); ok && name == "Unmarshal" && len(call.Args) == 2 {
				called[call.Fun] = true
				target := "an expression"
				if u, ok := call.Args[1].(*ast.UnaryExpr); ok && u.Op == token.AND {
					if id, ok := u.X.(*ast.Ident); ok && id.Obj != nil {
						if vs, ok := id.Obj.Decl.(*ast.ValueSpec); ok {
							if tid, ok := vs.Type.(*ast.Ident); ok {
								target = tid.Name
							}
						}
					}
				}
				if allowedType[target] {
					decoded[target]++
				} else {
					problems = append(problems, fmt.Sprintf("%s: json.Unmarshal into %s, not %s", fset.Position(call.Pos()), target, strings.Join(decodeTypes, " or ")))
				}
			}
			return true
		}
		e, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		if name, ok := pkgSel(e, jsonName); ok {
			switch {
			case name == "Unmarshal" && !called[e]:
				problems = append(problems, fmt.Sprintf("%s: json.Unmarshal used other than as a call", fset.Position(e.Pos())))
			case name != "Unmarshal" && name != "Valid" && name != "UnmarshalTypeError":
				problems = append(problems, fmt.Sprintf("%s: json.%s: a line is decoded only with json.Unmarshal", fset.Position(e.Pos()), name))
			}
		}
		if name, ok := pkgSel(e, osName); ok {
			if name != "Open" && name != "Stat" && name != "ReadDir" {
				problems = append(problems, fmt.Sprintf("%s: os.%s: the reader opens files with os.Open alone", fset.Position(e.Pos()), name))
			}
			return true
		}
		if name, ok := pkgSel(e, fsName); ok {
			if !readerFSNames[name] {
				problems = append(problems, fmt.Sprintf("%s: fs.%s: the reader opens files with os.Open alone, not through an fs.FS", fset.Position(e.Pos()), name))
			}
			return true
		}
		if sel, ok := e.(*ast.SelectorExpr); ok && readerOpenMethods[sel.Sel.Name] {
			problems = append(problems, fmt.Sprintf("%s: .%s on a value: the reader opens files with os.Open alone", fset.Position(e.Pos()), sel.Sel.Name))
		}
		return true
	})
	return problems, decoded
}

// readerReferences is every name signals/transcript.go uses from the other files of
// package signals, each with why the reader needs it. A line can leave the reader
// only through a name it references, so bounding these (not following calls) is
// what keeps a line away from a decoder elsewhere in the package that is listed for
// another body: ParseHook, ParseSuggestions, a RateLimits. A name added here is a
// review decision.
var readerReferences = map[string]string{
	"callUsage":   "prices.go: the struct of one call's four counts and speed, which the reader fills from a decoded usage and hands to the price table",
	"priceCall":   "prices.go: prices one call from its model and callUsage; it takes counts, never a line",
	"unpricedKey": "prices.go: the key an unpriced call is booked under, from its model and speed",
}

// readerReferenceProblems lists each name the reader references that it does not
// declare, is not a Go built-in and is not one of its imports, unless
// readerReferences names it.
func readerReferenceProblems(fset *token.FileSet, f *ast.File) []string {
	imported := map[string]bool{}
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		name := path.Base(p)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		imported[name] = true
	}
	var out []string
	used := map[string]bool{}
	for _, id := range f.Unresolved {
		used[id.Name] = true
		if types.Universe.Lookup(id.Name) != nil || imported[id.Name] || readerReferences[id.Name] != "" {
			continue
		}
		out = append(out, fmt.Sprintf("%s: %s: the reader uses %s from another file of package signals, which readerReferences does not list", fset.Position(id.Pos()), id.Name, id.Name))
	}
	for name := range readerReferences {
		if !used[name] {
			out = append(out, fmt.Sprintf("%s no longer uses %s: remove it from readerReferences", fset.Position(f.Pos()).Filename, name))
		}
	}
	return out
}

// [MEASURES-4-S2] The reader keeps only D2's fields: its two decode types hold
// nothing outside the allowlist, at any depth and in no form that keeps content
// whole, and signals/transcript.go decodes into nothing else and opens files with
// os.Open alone. Nor does it hand a line to a decoder elsewhere in the package: it
// uses nothing from the package's other files but readerReferences. Each type must
// still be decoded into, so the check cannot pass vacuously on a reader that
// decodes elsewhere.
func TestTranscriptReaderDecodesOnlyTheAllowlist(t *testing.T) {
	t.Parallel()
	var names []string
	for typ, allowed := range transcriptAllowlist {
		names = append(names, typ.Name())
		for _, p := range decodeTypeProblems(typ, allowed) {
			t.Error(p)
		}
	}
	sort.Strings(names)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.FromSlash(transcriptReader), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	problems, decoded := readerDecodeProblems(fset, f, names)
	problems = append(problems, readerReferenceProblems(fset, f)...)
	for _, p := range problems {
		t.Error(p)
	}
	for _, n := range names {
		if decoded[n] == 0 {
			t.Errorf("%s decodes no line into %s: the allowlist no longer describes the reader", transcriptReader, n)
		}
	}
}

// signalsDecodeSite is one JSON decode in package signals: the file it is in, the
// function it is in (Type.Method for a method, "" at package scope), and the type it
// decodes into ("struct{...}" for a struct declared in place, "T.f" for a field).
type signalsDecodeSite struct{ file, fn, into string }

// signalsDecodes is every JSON decode package signals makes, each with what it
// decodes (PANEL-49 D8). A line of a transcript can reach only code in package
// signals (TestNoSourceReadsTranscripts and
// TestNoSourceReachesTranscriptsByJoinOrNewReadSite hold every other file to that),
// so a decode anywhere in the package, a helper in another file included, fails
// TestSignalsDecodesOnlyTheAllowlist until it is listed here. A map or
// json.RawMessage is allowed only at the one site listed for it, for a body that is
// not a transcript line. Every named type here is declared in the file listed.
var signalsDecodes = map[signalsDecodeSite]string{
	{"transcript.go", "transcriptFile.readLine", "TranscriptHead"}:         "a transcript line, stage one (D2)",
	{"transcript.go", "transcriptFile.readLine", "TranscriptAssistant"}:    "a transcript line, stage two (D2)",
	{"hooks.go", "ParseHook", "HookEvent"}:                                 "a hook body",
	{"hooks.go", "HookEvent.AgentCall", "struct{...}"}:                     "an Agent call's tool_input: subagent_type and description",
	{"hooks.go", "HookEvent.AgentResult", "struct{...}"}:                   "an Agent or Workflow call's tool_response: agentId, runId and workflowName",
	{"hooks.go", "ParseStatus", "StatusPayload"}:                           "a status-line body",
	{"hooks.go", "PostedInt.UnmarshalJSON", "int64"}:                       "one integer of a status-line body",
	{"hooks.go", "RateLimits.UnmarshalJSON", "map[string]json.RawMessage"}: "a status-line body's rate_limits, by window key",
	{"hooks.go", "RateLimits.UnmarshalJSON", "RateLimit"}:                  "one rate_limits window",
	{"auth.go", "ParseAuthStatus", "struct{...}"}:                          "`claude auth status` output",
	{"merged.go", "ParseMergedPRs", "[]struct{...}"}:                       "`gh pr list --state merged` output",
	{"changes.go", "ParseReviewThreads", "struct{...}"}:                    "`gh api graphql` output: review thread counts",
	{"sources.go", "ParseAgents", "[]Agent"}:                               "`claude agents --json` output",
	{"sources.go", "ParsePRs", "[]ghPR"}:                                   "`gh pr list` output",
	{"suggest.go", "ParseSuggestions", "[]json.RawMessage"}:                "a suggest command's JSON array, one row at a time",
	{"suggest.go", "ParseSuggestions", "Suggestion.Name"}:                  "a suggest row that is a name",
	{"suggest.go", "ParseSuggestions", "Suggestion"}:                       "a suggest row that is an object",
}

// signalsJSONNames is what package signals may use from encoding/json besides
// json.Unmarshal: none of it decodes. json.NewDecoder, a *json.Decoder handed over,
// and any name added to encoding/json later are problems until named here.
var signalsJSONNames = map[string]bool{
	"Valid": true, "Compact": true, "RawMessage": true, "UnmarshalTypeError": true,
	"Marshal": true, "MarshalIndent": true,
}

// parseSignals parses package signals' shipped sources, tests excluded, with
// planted replacing a file's source or adding a file (nil: the real tree).
func parseSignals(t *testing.T, planted map[string]string) (*token.FileSet, map[string]*ast.File) {
	t.Helper()
	srcs := map[string]any{}
	entries, err := os.ReadDir("signals")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if n := e.Name(); !e.IsDir() && strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
			srcs[n] = nil
		}
	}
	for n, src := range planted {
		srcs[n] = src
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for n, src := range srcs {
		f, err := parser.ParseFile(fset, path.Join("signals", n), src, 0)
		if err != nil {
			t.Fatal(err)
		}
		files[n] = f
	}
	return fset, files
}

// signalsTypeString names a decode target's declared type, a struct declared in
// place as "struct{...}", so a site is listed by a short, stable string.
func signalsTypeString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return signalsTypeString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + signalsTypeString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + signalsTypeString(t.Elt)
		}
		return "[" + types.ExprString(t.Len) + "]" + signalsTypeString(t.Elt)
	case *ast.MapType:
		return "map[" + signalsTypeString(t.Key) + "]" + signalsTypeString(t.Value)
	case *ast.StructType:
		return "struct{...}"
	case *ast.InterfaceType:
		return "interface{...}"
	}
	return types.ExprString(e)
}

// signalsDecodeTarget reads a json.Unmarshal target: it must be &v or &v.f, with v
// declared with a type (a var, a parameter or a named result). why is non-empty for
// a target the check cannot read, and for a struct declared in place that holds a
// map, an interface or a json.RawMessage: those keep a value whole at any site.
func signalsDecodeTarget(arg ast.Expr, jsonName string) (into, why string) {
	const unread = "decode only into &v or &v.f, v declared with a type"
	u, ok := arg.(*ast.UnaryExpr)
	if !ok || u.Op != token.AND {
		return types.ExprString(arg), unread
	}
	x, field := u.X, ""
	if sel, ok := x.(*ast.SelectorExpr); ok {
		x, field = sel.X, "."+sel.Sel.Name
	}
	id, ok := x.(*ast.Ident)
	if !ok || id.Obj == nil {
		return types.ExprString(u.X), unread
	}
	var typ ast.Expr
	switch d := id.Obj.Decl.(type) {
	case *ast.ValueSpec:
		typ = d.Type
	case *ast.Field:
		typ = d.Type
	}
	if typ == nil {
		return types.ExprString(u.X), unread
	}
	ast.Inspect(typ, func(n ast.Node) bool {
		if _, ok := n.(*ast.StructType); !ok {
			return why == ""
		}
		ast.Inspect(n, func(n ast.Node) bool {
			switch t := n.(type) {
			case *ast.MapType:
				why = "a struct declared in place holds a map"
			case *ast.InterfaceType:
				why = "a struct declared in place holds an interface"
			case *ast.Ident:
				if t.Name == "any" {
					why = "a struct declared in place holds an interface"
				}
			case *ast.SelectorExpr:
				if pkg, ok := t.X.(*ast.Ident); ok && pkg.Name == jsonName && t.Sel.Name == "RawMessage" {
					why = "a struct declared in place holds a json.RawMessage"
				}
			}
			return why == ""
		})
		return false
	})
	return signalsTypeString(typ) + field, why
}

// signalsDecodeProblems checks every file of package signals by AST: each
// json.Unmarshal is at a site allow lists, nothing else from encoding/json is used
// that could decode, and encoding/json is imported under a name the check can read.
// It counts each site it saw, so an entry no longer decoded at shows.
func signalsDecodeProblems(fset *token.FileSet, files map[string]*ast.File, allow map[signalsDecodeSite]string) (problems []string, seen map[signalsDecodeSite]int) {
	seen = map[signalsDecodeSite]int{}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, file := range names {
		f := files[file]
		jsonName := ""
		for _, imp := range f.Imports {
			if p, _ := strconv.Unquote(imp.Path.Value); p != "encoding/json" {
				continue
			}
			jsonName = "json"
			if imp.Name != nil {
				jsonName = imp.Name.Name
			}
			if jsonName == "." || jsonName == "_" {
				problems = append(problems, fmt.Sprintf("%s: imports encoding/json as %s: the check reads each decode by its package name", fset.Position(imp.Pos()), jsonName))
				jsonName = ""
			}
		}
		jsonSel := func(e ast.Expr) (string, bool) {
			sel, ok := e.(*ast.SelectorExpr)
			if !ok || jsonName == "" {
				return "", false
			}
			id, ok := sel.X.(*ast.Ident)
			if !ok || id.Name != jsonName || id.Obj != nil {
				return "", false // a local of the same name is not the package
			}
			return sel.Sel.Name, true
		}
		for _, decl := range f.Decls {
			fn, label := "", "package scope"
			if fd, ok := decl.(*ast.FuncDecl); ok {
				fn = fd.Name.Name
				if fd.Recv != nil && len(fd.Recv.List) == 1 {
					recv := fd.Recv.List[0].Type
					if s, ok := recv.(*ast.StarExpr); ok {
						recv = s.X
					}
					if ix, ok := recv.(*ast.IndexExpr); ok {
						recv = ix.X
					}
					fn = types.ExprString(recv) + "." + fn
				}
				label = fn
			}
			called := map[ast.Expr]bool{}
			ast.Inspect(decl, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok {
					if name, ok := jsonSel(call.Fun); ok && name == "Unmarshal" && len(call.Args) == 2 {
						called[call.Fun] = true
						into, why := signalsDecodeTarget(call.Args[1], jsonName)
						site := signalsDecodeSite{file, fn, into}
						switch {
						case why != "":
							problems = append(problems, fmt.Sprintf("%s: json.Unmarshal in %s into %s: %s", fset.Position(call.Pos()), label, into, why))
						case allow[site] == "":
							problems = append(problems, fmt.Sprintf("%s: json.Unmarshal in %s into %s: not a decode signalsDecodes lists for %s", fset.Position(call.Pos()), label, into, file))
						default:
							seen[site]++
						}
					}
					return true
				}
				if e, ok := n.(ast.Expr); ok {
					if name, ok := jsonSel(e); ok {
						switch {
						case name == "Unmarshal" && !called[e]:
							problems = append(problems, fmt.Sprintf("%s: json.Unmarshal in %s used other than as a call", fset.Position(e.Pos()), label))
						case name != "Unmarshal" && !signalsJSONNames[name]:
							problems = append(problems, fmt.Sprintf("%s: json.%s in %s: package signals decodes only with json.Unmarshal", fset.Position(e.Pos()), name, label))
						}
					} else if sel, ok := e.(*ast.SelectorExpr); ok && (sel.Sel.Name == "UnmarshalJSON" || sel.Sel.Name == "UnmarshalText") {
						// A method named so is encoding/json's to call: called by hand, it
						// decodes whatever it is handed, a line included, outside every
						// site listed here.
						problems = append(problems, fmt.Sprintf("%s: .%s in %s: only encoding/json calls an UnmarshalJSON or UnmarshalText", fset.Position(e.Pos()), sel.Sel.Name, label))
					}
				}
				return true
			})
		}
	}
	return problems, seen
}

// readerEscapeProblems closes the two ways past readerReferenceProblems that need
// no new name in the reader: a method declared in another file on a type the reader
// holds (f.peek(line) resolves to no unresolved identifier, being a selector), and a
// package-level name in package signals that shadows a Go built-in (max(line) reads
// as the built-in to a single-file parse, so readerReferences never sees it).
func readerEscapeProblems(fset *token.FileSet, files map[string]*ast.File) []string {
	readerTypes := map[string]bool{}
	for name := range readerReferences {
		readerTypes[name] = true
	}
	if f := files[path.Base(transcriptReader)]; f != nil {
		for _, d := range f.Decls {
			if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, s := range gd.Specs {
					readerTypes[s.(*ast.TypeSpec).Name.Name] = true
				}
			}
		}
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []string
	shadows := func(id *ast.Ident) {
		if id.Name != "_" && types.Universe.Lookup(id.Name) != nil {
			out = append(out, fmt.Sprintf("%s: %s is declared at package level in package signals and shadows the Go built-in", fset.Position(id.Pos()), id.Name))
		}
	}
	for _, file := range names {
		for _, d := range files[file].Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					shadows(d.Name)
					continue
				}
				if file == path.Base(transcriptReader) || len(d.Recv.List) != 1 {
					continue
				}
				recv := d.Recv.List[0].Type
				if s, ok := recv.(*ast.StarExpr); ok {
					recv = s.X
				}
				if ix, ok := recv.(*ast.IndexExpr); ok {
					recv = ix.X
				}
				if id, ok := recv.(*ast.Ident); ok && readerTypes[id.Name] {
					out = append(out, fmt.Sprintf("%s: method %s.%s is declared outside %s on a type the reader holds", fset.Position(d.Pos()), id.Name, d.Name.Name, transcriptReader))
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						shadows(s.Name)
					case *ast.ValueSpec:
						for _, n := range s.Names {
							shadows(n)
						}
					}
				}
			}
		}
	}
	return out
}

// [MEASURES-4-S2] A line is decoded into nothing but the reader's two types,
// wherever a line can reach: every JSON decode in package signals, in any of its
// files, is a site signalsDecodes lists, so a helper elsewhere in the package that
// decodes a whole line into a map, any, json.RawMessage or an unlisted type fails,
// naming its file and line. Each listed site must still be decoded at, so the list
// cannot go stale.
func TestSignalsDecodesOnlyTheAllowlist(t *testing.T) {
	t.Parallel()
	fset, files := parseSignals(t, nil)
	problems, seen := signalsDecodeProblems(fset, files, signalsDecodes)
	problems = append(problems, readerEscapeProblems(fset, files)...)
	for _, p := range problems {
		t.Error(p)
	}
	declaredIn := map[string]string{}
	for file, f := range files {
		for _, d := range f.Decls {
			if gd, ok := d.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
				for _, s := range gd.Specs {
					declaredIn[s.(*ast.TypeSpec).Name.Name] = file
				}
			}
		}
	}
	for site := range signalsDecodes {
		if seen[site] == 0 {
			t.Errorf("signals/%s: %s decodes nothing into %s: remove it from signalsDecodes", site.file, site.fn, site.into)
		}
		named, _, _ := strings.Cut(strings.TrimLeft(site.into, "[]*"), ".")
		if in, ok := declaredIn[named]; ok && in != site.file {
			t.Errorf("signalsDecodes lists %s for signals/%s, but it is declared in signals/%s", named, site.file, in)
		}
	}
}

// Planted types for TestTranscriptAllowlistCatchesEachCase: each holds one of the
// cases D8 lists, on the allowlisted shape.
type (
	plantedLine      struct{}
	plantedUnmarshal struct {
		ID string `json:"id"`
	}
	plantedText string
	plantedBase struct {
		Model string `json:"model"`
	}
)

func (*plantedUnmarshal) UnmarshalJSON([]byte) error { return nil }
func (*plantedText) UnmarshalText([]byte) error      { return nil }

// [MEASURES-4-S2] Falsification of the allowlist test: each case D8 lists, planted,
// is named; and the AST check names a decode into anything else, a decoder, and a
// file opened another way.
func TestTranscriptAllowlistCatchesEachCase(t *testing.T) {
	t.Parallel()
	allowed := transcriptAllowlist[reflect.TypeOf(signals.TranscriptAssistant{})]
	for _, tc := range []struct {
		name string
		typ  any
		want string
	}{
		{"message.content", struct {
			Message *struct {
				ID      string `json:"id"`
				Content string `json:"content"`
			} `json:"message"`
		}{}, `decodes "message.content", outside the allowlist`},
		{"an untagged field", struct {
			Message *struct{ Model string } `json:"message"`
		}{}, "Model: an exported field with no json tag"},
		{"an embedded struct", struct {
			Message *struct{ plantedBase } `json:"message"`
		}{}, "plantedBase: an embedded struct"},
		{"json.RawMessage", struct {
			Message json.RawMessage `json:"message"`
		}{}, "json.RawMessage (slice), not a scalar"},
		{"a map", struct {
			Message map[string]any `json:"message"`
		}{}, "(map), not a scalar"},
		{"an interface", struct {
			Message any `json:"message"`
		}{}, "(interface), not a scalar"},
		{"a slice of structs", struct {
			Message []plantedLine `json:"message"`
		}{}, "(slice), not a scalar"},
		{"an UnmarshalJSON", struct {
			Message *plantedUnmarshal `json:"message"`
		}{}, "implements Unmarshaler"},
		{"an UnmarshalText on a scalar", struct {
			Message *struct {
				Model plantedText `json:"model"`
			} `json:"message"`
		}{}, "implements TextUnmarshaler"},
	} {
		got := decodeTypeProblems(reflect.TypeOf(tc.typ), allowed)
		if !strings.Contains(strings.Join(got, "\n"), tc.want) {
			t.Errorf("%s: no problem names %q; got %q", tc.name, tc.want, got)
		}
	}
	if got := decodeTypeProblems(reflect.TypeOf(struct {
		Message *struct {
			ID    string  `json:"id"`
			Model *string `json:"model"`
		} `json:"message"`
		hidden string
	}{}), allowed); len(got) > 0 {
		t.Errorf("an allowlisted shape: want no problem, got %q", got)
	}

	names := []string{"TranscriptHead", "TranscriptAssistant"}
	for _, tc := range []struct{ name, src, want string }{
		{"a decode into a map", `package s; import "encoding/json"; func f(b []byte) { var m map[string]any; json.Unmarshal(b, &m) }`,
			"json.Unmarshal into an expression, not TranscriptHead or TranscriptAssistant"},
		{"a decode into another type", `package s; import "encoding/json"; type other struct{}; func f(b []byte) { var o other; json.Unmarshal(b, &o) }`,
			"json.Unmarshal into other"},
		{"a decoder", `package s; import ("encoding/json"; "io"); func f(r io.Reader) { var h TranscriptHead; json.NewDecoder(r).Decode(&h) }`,
			"json.NewDecoder"},
		{"an aliased json", `package s; import j "encoding/json"; func f(b []byte) { var m any; j.Unmarshal(b, &m) }`,
			"json.Unmarshal into any, not"},
		{"Unmarshal as a value", `package s; import "encoding/json"; var u = json.Unmarshal`,
			"json.Unmarshal used other than as a call"},
		{"os.ReadFile", `package s; import "os"; func f(p string) { os.ReadFile(p) }`, "os.ReadFile"},
		{"os.OpenFile", `package s; import "os"; func f(p string) { os.OpenFile(p, os.O_RDONLY, 0) }`, "os.OpenFile"},
		{"os.DirFS", `package s; import "os"; var fsys = os.DirFS("/")`, "os.DirFS"},
		{"another decoder package", `package s; import "encoding/gob"; var _ = gob.NewDecoder`, "imports encoding/gob"},
		{"a read through os/exec", `package s; import "os/exec"; func f(p string) { exec.Command("cat", p).Output() }`, "imports os/exec"},
		{"fs.ReadFile on a passed fs.FS", `package s; import "io/fs"; func f(fsys fs.FS, p string) { fs.ReadFile(fsys, p) }`, "fs.ReadFile"},
		{"a passed fs.FS named", `package s; import "io/fs"; func f(fsys fs.FS) {}`, "fs.FS"},
		{"Open on a value from elsewhere", `package s; func f(p string) { fsys := other(); fsys.Open(p) }`, ".Open on a value"},
		{"ReadFile on a value from elsewhere", `package s; func f(p string) { fsys := other(); fsys.ReadFile(p) }`, ".ReadFile on a value"},
		{"a dot import", `package s; import . "os"; func f(p string) { ReadFile(p) }`, "imports os as ."},
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "x.go", tc.src, 0)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got, _ := readerDecodeProblems(fset, f, names)
		if !strings.Contains(strings.Join(got, "\n"), tc.want) {
			t.Errorf("%s: no problem names %q; got %q", tc.name, tc.want, got)
		}
	}

	// The package-wide check, over the real package with one file changed or added:
	// a decode a line could reach from outside transcript.go names its file:line.
	// after inserts insert after anchor in a real file of package signals, and
	// returns the line the insert starts on.
	after := func(file, anchor, insert string) (string, int) {
		b, err := os.ReadFile(filepath.Join("signals", file))
		if err != nil {
			t.Fatal(err)
		}
		i := strings.Index(string(b), anchor)
		if i < 0 {
			t.Fatalf("signals/%s no longer holds %q: re-anchor the planted case", file, anchor)
		}
		i += len(anchor)
		return string(b[:i]) + insert + string(b[i:]), strings.Count(string(b[:i]), "\n") + 1
	}
	plant := func(file, anchor, insert string) string { src, _ := after(file, anchor, insert); return src }
	const (
		readLine   = "func (f *transcriptFile) readLine(line []byte, main bool) *TranscriptError {\n"
		agentTool  = "func IsAgentTool(name string) bool { return name == \"Agent\" || name == \"Task\" }\n"
		authStatus = "func ParseAuthStatus(out []byte) (AuthStatus, error) {\n"
	)
	// The helper the review named, exactly: in hooks.go, called from readLine.
	peekHooks, peekLine := after("hooks.go", agentTool, "\nfunc peek(b []byte) (m map[string]any) { json.Unmarshal(b, &m); return }\n")
	for _, tc := range []struct {
		name    string
		planted map[string]string
		want    string
	}{
		{"a helper in hooks.go decoding a whole line, called from readLine", map[string]string{
			"hooks.go":      peekHooks,
			"transcript.go": plant("transcript.go", readLine, "\t_ = peek(line)\n"),
		}, fmt.Sprintf("signals/hooks.go:%d:42: json.Unmarshal in peek into map[string]any: not a decode signalsDecodes lists for hooks.go", peekLine+1)},
		{"a decode into a map in a new file", map[string]string{
			"planted.go": "package signals\n\nimport \"encoding/json\"\n\nfunc lineFields(b []byte) map[string]any {\n\tvar m map[string]any\n\t_ = json.Unmarshal(b, &m)\n\treturn m\n}\n",
		}, "signals/planted.go:7:6: json.Unmarshal in lineFields into map[string]any: not a decode signalsDecodes lists for planted.go"},
		{"a listed type decoded at an unlisted site", map[string]string{
			"transcript.go": plant("transcript.go", readLine, "\tvar ev HookEvent\n\t_ = json.Unmarshal(line, &ev)\n"),
		}, "json.Unmarshal in transcriptFile.readLine into HookEvent: not a decode signalsDecodes lists for transcript.go"},
		{"a struct holding a map at a listed site", map[string]string{
			"auth.go": plant("auth.go", authStatus, "\tvar x struct{ M map[string]any }\n\t_ = json.Unmarshal(out, &x)\n"),
		}, "json.Unmarshal in ParseAuthStatus into struct{...}: a struct declared in place holds a map"},
		{"a decoder in another file", map[string]string{
			"hooks.go": plant("hooks.go", agentTool, "\nfunc stream(r io.Reader) { var h TranscriptHead; json.NewDecoder(r).Decode(&h) }\n"),
		}, "json.NewDecoder in stream: package signals decodes only with json.Unmarshal"},
	} {
		fset, files := parseSignals(t, tc.planted)
		got, _ := signalsDecodeProblems(fset, files, signalsDecodes)
		if !strings.Contains(strings.Join(got, "\n"), tc.want) {
			t.Errorf("%s: no problem names %q; got %q", tc.name, tc.want, got)
		}
	}

	// A line handed from the reader to a decoder listed for another body: the
	// reader's references to other files are bounded, and an UnmarshalJSON or
	// UnmarshalText is never called by hand. Both checks run over the real package
	// with the plant in place.
	for _, tc := range []struct {
		name    string
		planted map[string]string
		want    []string
	}{
		{"a line handed to RateLimits, ParseHook and ParseSuggestions", map[string]string{
			"transcript.go": plant("transcript.go", readLine, "\tvar rl RateLimits\n\t_ = rl.UnmarshalJSON(line)\n\t_, _ = ParseHook(line)\n\t_, _ = ParseSuggestions(line)\n"),
		}, []string{
			"RateLimits: the reader uses RateLimits from another file of package signals, which readerReferences does not list",
			"ParseHook: the reader uses ParseHook from another file of package signals",
			"ParseSuggestions: the reader uses ParseSuggestions from another file of package signals",
			".UnmarshalJSON in transcriptFile.readLine: only encoding/json calls an UnmarshalJSON or UnmarshalText",
		}},
		{"a decoder renamed in hooks.go and used by the reader", map[string]string{
			"hooks.go":      plant("hooks.go", agentTool, "\nvar dec = ParseHook\n"),
			"transcript.go": plant("transcript.go", readLine, "\t_, _ = dec(line)\n"),
		}, []string{"dec: the reader uses dec from another file of package signals"}},
		{"an UnmarshalText called by hand in another file", map[string]string{
			"planted.go": "package signals\n\nfunc parse(b []byte) { var n PostedInt; _ = n.UnmarshalText(b) }\n",
		}, []string{".UnmarshalText in parse: only encoding/json calls"}},
		{"a method on the reader's type declared in another file", map[string]string{
			"planted.go":    "package signals\n\nfunc (f *transcriptFile) peek(b []byte) HookEvent { h, _ := ParseHook(b); return h }\n",
			"transcript.go": plant("transcript.go", readLine, "\t_ = f.peek(line)\n"),
		}, []string{"signals/planted.go:3:1: method transcriptFile.peek is declared outside signals/transcript.go on a type the reader holds"}},
		{"a method on a referenced type declared in another file", map[string]string{
			"planted.go": "package signals\n\nfunc (u callUsage) line(b []byte) HookEvent { h, _ := ParseHook(b); return h }\n",
		}, []string{"method callUsage.line is declared outside signals/transcript.go"}},
		{"a package-level name shadowing a built-in", map[string]string{
			"planted.go":    "package signals\n\nfunc max(b []byte) HookEvent { h, _ := ParseHook(b); return h }\n",
			"transcript.go": plant("transcript.go", readLine, "\t_ = max(line)\n"),
		}, []string{"signals/planted.go:3:6: max is declared at package level in package signals and shadows the Go built-in"}},
	} {
		fset, files := parseSignals(t, tc.planted)
		got, _ := signalsDecodeProblems(fset, files, signalsDecodes)
		got = append(got, readerReferenceProblems(fset, files["transcript.go"])...)
		got = append(got, readerEscapeProblems(fset, files)...)
		for _, w := range tc.want {
			if !strings.Contains(strings.Join(got, "\n"), w) {
				t.Errorf("%s: no problem names %q; got %q", tc.name, w, got)
			}
		}
	}
	// And the real reader references nothing outside readerReferences.
	fset, files := parseSignals(t, nil)
	if got := readerReferenceProblems(fset, files["transcript.go"]); len(got) > 0 {
		t.Errorf("the real reader: want no reference problem, got %q", got)
	}
}
