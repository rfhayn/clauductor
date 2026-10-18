package lanes

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
	"github.com/clauductor/clauductor/internal/testbin"
)

func TestValidLaneID(t *testing.T) {
	t.Parallel()
	ok := []string{"a", "0", "add-login", "orchestrator", "x-1-2", strings.Repeat("a", 41)}
	bad := []string{"", "-a", "A", "Add", "a_b", "a.b", "a:b", "a b", "a/b", "../x", "=a", "a;b", "a'b",
		"a\nb", "é", strings.Repeat("a", 42), "$(id)", "`id`"}
	for _, id := range ok {
		if !config.ValidLaneID(id) {
			t.Errorf("ValidLaneID(%q) = false, want true", id)
		}
	}
	for _, id := range bad {
		if config.ValidLaneID(id) {
			t.Errorf("ValidLaneID(%q) = true, want false", id)
		}
	}
}

func testLaneManager(t *testing.T) *LaneManager {
	t.Helper()
	cfg, err := loadConfig(t, []byte(`{"name":"T","lanes":{"main":"orchestrator","fix/":"fix","change/":"build","change/propose-*":"propose"},
		"tmux_socket":"sock","lane_types":{"build":{"model":"opus","effort":"high"}}}`))

	if err != nil {
		t.Fatal(err)
	}
	reg, err := OpenRegistry(t.TempDir(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	return &LaneManager{Clock: clock.System, TmuxPath: "/opt/homebrew/bin/tmux", Socket: cfg.Socket(), Root: "/repo", Cfg: cfg, Registry: reg,
		Program: []string{"/Users/me/.local/bin/claude"}, LookupEnv: func(string) (string, bool) { return "", false }}
}

const sid = "0f8fad5b-d9cb-469f-a165-70867728950e"

// Every command is an argv list; nothing a browser supplies is ever parsed by a shell.
func TestLaneArgvConstruction(t *testing.T) {
	t.Parallel()
	m := testLaneManager(t)
	unset := []string{"-u", "ANTHROPIC_API_KEY", "-u", "ANTHROPIC_AUTH_TOKEN"}
	for _, k := range parentSessionVars {
		unset = append(unset, "-u", k)
	}
	want := append(append([]string{"/usr/bin/env"}, unset...), "/Users/me/.local/bin/claude", "--model", "opus", "--effort", "high",
		"-n", "add-x", "--session-id", sid)
	if got := m.LaneCommand("add-x", "build", sid, false); !reflect.DeepEqual(got, want) {
		t.Fatalf("LaneCommand:\n got %q\nwant %q", got, want)
	}
	// A restart resumes the lane's OWN session; --continue (the directory's latest
	// conversation, whoever's) never appears.
	got := m.LaneCommand("add-x", "fix", sid, true)
	if tail := got[len(got)-5:]; !reflect.DeepEqual(tail, []string{"/Users/me/.local/bin/claude", "-n", "add-x", "--resume", sid}) {
		t.Fatalf("resume command ends %q", tail)
	}
	for _, a := range append(got, want...) {
		if a == "--continue" || a == "-c" {
			t.Fatalf("lane command uses --continue: %q", got)
		}
	}

	ns := m.NewSessionArgv("add-x", "/repo/.claude/worktrees/add-x", "fix", sid, false)
	head := []string{"-L", "sock", "-f", "/dev/null", "new-session", "-d", "-s", "add-x", "-c", "/repo/.claude/worktrees/add-x", "-x", "200", "-y", "50"}
	if !reflect.DeepEqual(ns[:len(head)], head) {
		t.Fatalf("NewSessionArgv head:\n got %q\nwant %q", ns[:len(head)], head)
	}
	// The session's environment is explicit, not inherited from whoever started tmux.
	env := map[string]string{}
	i := len(head)
	for ; ns[i] == "-e"; i += 2 {
		k, v, _ := strings.Cut(ns[i+1], "=")
		env[k] = v
	}
	if !strings.HasPrefix(env["PATH"], "/Users/me/.local/bin:/opt/homebrew/bin") || env["HOME"] == "" || env["LANG"] == "" {
		t.Fatalf("session env %v", env)
	}
	if ns[i] != "/usr/bin/env" {
		t.Fatalf("lane command starts %q", ns[i])
	}
	head = ns[:i+1]
	tail := []string{";", "set-option", "-t", "=add-x:", "remain-on-exit", "on",
		";", "set-option", "-t", "=add-x:", "@clauductor_type", "fix",
		";", "set-option", "-t", "=add-x:", "window-size", "latest",
		";", "set-option", "-g", "prefix", "None",
		";", "set-option", "-g", "prefix2", "None",
		";", "unbind-key", "-q", "-a", "-T", "prefix",
		";", "unbind-key", "-q", "-a", "-T", "root",
		";", "set-option", "-g", "status", "off",
		";", "set-option", "-g", "mouse", "on",
		";", "bind-key", "-T", "root", "WheelUpPane",
		"if-shell", "-F", "#{||:#{alternate_on},#{||:#{pane_in_mode},#{mouse_any_flag}}}", "send-keys -M", "copy-mode -e",
		";", "set-option", "-sq", "terminal-features[99]", "xterm-256color:hyperlinks"}
	if !reflect.DeepEqual(ns[len(ns)-len(tail):], tail) {
		t.Fatalf("NewSessionArgv tail:\n got %q", ns[len(ns)-len(tail):])
	}
	// tmux passes a ONE-argument command to `sh -c`; the lane command must never be one.
	if n := len(ns) - len(head) - len(tail) + 1; n < 2 {
		t.Fatalf("lane command has %d argument(s); tmux would hand it to a shell", n)
	}
	for _, a := range ns[len(head)-1 : len(ns)-len(tail)] {
		if a == "sh" || a == "-c" || a == "bash" || a == "zsh" {
			t.Fatalf("the lane command invokes a shell: %q", ns)
		}
	}

	if got, want := m.AttachArgv("add-x"), []string{"-u", "-L", "sock", "-f", "/dev/null", "attach-session", "-t", "=add-x"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("AttachArgv %q, want %q", got, want)
	}

	osa := m.TerminalAppArgv("add-x")
	if osa[0] != "/usr/bin/osascript" {
		t.Fatalf("TerminalAppArgv runs %q", osa[0])
	}
	cmd := osa[len(osa)-1]
	if want := `exec '/opt/homebrew/bin/tmux' '-u' '-L' 'sock' '-f' '/dev/null' 'attach-session' '-t' '=add-x'`; cmd != want {
		t.Fatalf("Terminal command %q, want %q", cmd, want)
	}
	// The command reaches AppleScript as an argument, never inside the script text.
	for _, a := range osa[1 : len(osa)-1] {
		if strings.Contains(a, "add-x") || strings.Contains(a, "tmux") {
			t.Fatalf("lane data spliced into AppleScript source: %q", a)
		}
	}
	if shq(`it's`) != `'it'\''s'` {
		t.Fatalf("shq: %s", shq(`it's`))
	}
}

func TestStartRejectsBadInputBeforeRunningAnything(t *testing.T) {
	t.Parallel()
	m := testLaneManager(t)
	m.TmuxPath = "/nonexistent/tmux" // any tmux or git call would fail loudly
	m.Run = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		t.Fatalf("git ran for a request that should be refused: %v", argv)
		return nil, nil
	}
	cases := []struct {
		req  StartRequest
		code string
	}{
		{StartRequest{Type: "fix", Mode: "new", Name: "Bad Name"}, "invalid"},
		{StartRequest{Type: "fix", Mode: "new", Name: "-rf"}, "invalid"},
		{StartRequest{Type: "nope", Mode: "new", Name: "ok"}, "invalid"},
		{StartRequest{Type: "fix", Mode: "shell", Name: "ok"}, "invalid"},
	}
	for _, c := range cases {
		_, err := m.Start(context.Background(), c.req)
		if err == nil || err.Code != c.code || err.Status != 400 {
			t.Errorf("Start(%+v) = %v, want %s/400", c.req, err, c.code)
		}
	}
}

// An API key in the panel's environment outranks the subscription login: no lane
// starts, and the refusal says why.
func TestStartRefusedWhileAnAPIKeyIsSet(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"} {
		m := testLaneManager(t)
		m.LookupEnv = func(name string) (string, bool) { return "sk-test", name == k }
		m.Run = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
			t.Fatalf("git ran despite %s: %v", k, argv)
			return nil, nil
		}
		m.TmuxPath = "/nonexistent/tmux"
		_, err := m.Start(context.Background(), StartRequest{Type: "fix", Mode: "new", Name: "x"})
		if err == nil || err.Code != "api-key" || err.Status != 409 || !strings.Contains(err.Msg, k) {
			t.Fatalf("%s set: Start = %v, want a 409 api-key refusal naming it", k, err)
		}
		if why := m.StartBlocked(context.Background()); !strings.Contains(why, k) {
			t.Fatalf("StartBlocked = %q", why)
		}
	}
}

func TestBranchPrefixAndLaneTypes(t *testing.T) {
	t.Parallel()
	m := testLaneManager(t)
	for typ, want := range map[string]string{"fix": "fix/", "build": "change/", "propose": "change/propose-", "orchestrator": "", "nope": ""} {
		if got := m.Cfg.BranchPrefix(typ); got != want {
			t.Errorf("BranchPrefix(%s) = %q, want %q", typ, got, want)
		}
	}
	var names []string
	for _, lt := range m.Cfg.LaneTypeList() {
		names = append(names, lt.Name)
	}
	if strings.Join(names, ",") != "build,fix,orchestrator,propose" {
		t.Fatalf("LaneTypeList = %v", names)
	}
	if m.Cfg.Socket() != "sock" || m.Cfg.BaseRef() != config.DefaultBase || m.Cfg.WorktreeRoot("/repo") != "/repo/.claude/worktrees" {
		t.Fatalf("defaults: %s %s %s", m.Cfg.Socket(), m.Cfg.BaseRef(), m.Cfg.WorktreeRoot("/repo"))
	}
}

func TestParseTmuxPanes(t *testing.T) {
	t.Parallel()
	out := "orchestrator\t/tmp\t/tmp\t0\t\t1790000000\t1\torchestrator\n" +
		"orchestrator\t/tmp\t/tmp\t0\t\t1790000000\t1\torchestrator\n" + // second pane of the same session
		"fix-a\t\t/nonexistent/wt\t1\t3\t1790000001\t0\tfix\n" +
		"User Session\t/tmp\t/tmp\t0\t\t1\t0\t\n" + // not a lane id: not the panel's
		"short\tline\n"
	got := parseTmuxPanes([]byte(out))
	if len(got) != 2 {
		t.Fatalf("got %d lanes: %+v", len(got), got)
	}
	if got[0].ID != "fix-a" || !got[0].Dead || got[0].DeadStatus != "3" || got[0].Path != "/nonexistent/wt" || got[0].Type != "fix" {
		t.Fatalf("dead lane parsed as %+v", got[0])
	}
	tmp, _ := filepath.EvalSymlinks("/tmp") // /private/tmp on macOS
	if got[1].ID != "orchestrator" || got[1].Attached != 1 || got[1].Path != tmp {
		t.Fatalf("live lane parsed as %+v", got[1])
	}
}

// PANEL-16: each lane is tagged with its project, and a session tagged for another
// project is not listed as this manager's (untagged ones, from before, are).
func TestLanesAreTaggedWithTheirProject(t *testing.T) {
	t.Parallel()
	m := testLaneManager(t)
	m.Project = "app"
	ns := strings.Join(m.NewSessionArgv("add-x", "/repo/w", "fix", sid, false), "\x00")
	if !strings.Contains(ns, strings.Join([]string{"set-option", "-t", "=add-x:", "@clauductor_project", "app", ";"}, "\x00")) {
		t.Fatal("the lane is not tagged with its project")
	}
	out := "mine\t/tmp\t/tmp\t0\t\t1\t0\tfix\tapp\n" +
		"theirs\t/tmp\t/tmp\t0\t\t1\t0\tfix\tweb\n" +
		"old\t/tmp\t/tmp\t0\t\t1\t0\tfix\t\n"
	m.Exec = func(_ context.Context, argv []string) ([]byte, error) { return []byte(out), nil }
	ls, up, err := m.ListServer(context.Background())
	var ids []string
	for _, l := range ls {
		ids = append(ids, l.ID+"="+l.Project)
	}
	if err != nil || !up || strings.Join(ids, ",") != "mine=app,old=" {
		t.Fatalf("listed %v (%v)", ids, err)
	}
}

// The intent reaches the disk BEFORE the action: a panel killed mid-start leaves a
// record, which the next start shows as an orphan instead of losing the lane.
func TestRegistryIsWrittenBeforeTheAction(t *testing.T) {
	t.Parallel()
	m := testLaneManager(t)
	m.TmuxPath = filepath.Join(t.TempDir(), "tmux") // a tmux whose socket has no server
	testbin.Write(t, m.TmuxPath, "#!/bin/sh\necho 'no server running on /tmp/x' >&2\nexit 1\n")
	var sawIntent bool
	m.Run = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		if strings.Join(argv[:3], " ") == "git worktree add" {
			r, err := OpenRegistry(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(m.Registry.path)))), "/repo")
			if err != nil {
				t.Fatal(err)
			}
			rec, ok := r.Get("fx")
			sawIntent = ok && rec.Action == "start" && !rec.ActionDone && uuidRe.MatchString(rec.SessionID) &&
				rec.Branch == "fix/fx" && rec.Type == "fix"
			return nil, errors.New("fatal: simulated")
		}
		return nil, nil
	}
	_, lerr := m.Start(context.Background(), StartRequest{Type: "fix", Mode: "new", Name: "fx"})
	if lerr == nil || lerr.Code != "git" {
		t.Fatalf("Start = %v, want the git failure", lerr)
	}
	if !sawIntent {
		t.Fatal("the registry did not hold the start intent while git ran")
	}
	if _, ok := m.Registry.Get("fx"); ok {
		t.Fatal("a start that failed cleanly left its record behind")
	}
}

func TestRegistryFileIsPrivateAndRoundTrips(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	r, err := OpenRegistry(home, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	rec := types.LaneRecord{ID: "a", SessionID: sid, Path: "/repo", Type: "orchestrator", Mode: "root", Created: 1}
	if _, err := r.Begin(rec, "start", time.Unix(5, 0)); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(RegistryPath(home, "/repo"))
	di, _ := os.Stat(filepath.Dir(RegistryPath(home, "/repo")))
	if fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
		t.Fatalf("registry %o in dir %o, want 600 in 700", fi.Mode().Perm(), di.Mode().Perm())
	}
	if RegistryPath(home, "/repo") == RegistryPath(home, "/other") {
		t.Fatal("two projects share a registry")
	}
	r2, _ := OpenRegistry(home, "/repo")
	got, ok := r2.Get("a")
	if !ok || got.SessionID != sid || got.Action != "start" || got.ActionDone || got.ActionAt != 5000 {
		t.Fatalf("round trip: %+v", got)
	}
	if err := r2.Done(got); err != nil {
		t.Fatal(err)
	}
	r.Reload()
	if got, _ := r.Get("a"); !got.ActionDone {
		t.Fatal("Reload did not pick up another writer's change")
	}
	r.Delete("a")
	if len(r.List()) != 0 {
		t.Fatal("delete")
	}
	os.WriteFile(RegistryPath(home, "/repo"), []byte("{not json"), 0o600)
	if _, err := OpenRegistry(home, "/repo"); err == nil {
		t.Fatal("a corrupt registry read as empty")
	}
}

func TestNewSessionID(t *testing.T) {
	t.Parallel()
	a, _ := newSessionID()
	b, _ := newSessionID()
	if !uuidRe.MatchString(a) || a == b {
		t.Fatalf("%q %q", a, b)
	}
}

// A reload racing a stop must never bring the stopped lane back. Seen live: the stop
// kicks the tmux loop, whose 30 s reload read the file just before Stop deleted the
// record, then applied that stale copy after the delete; the lane stayed "registered"
// until the next reload. This forces that interleaving.
func TestRegistryReloadNeverResurrectsADeletedLane(t *testing.T) {
	t.Parallel()
	r, err := OpenRegistry(t.TempDir(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Begin(types.LaneRecord{ID: "a", SessionID: sid, Path: "/repo"}, "stop", time.Now()); err != nil {
		t.Fatal(err)
	}
	read, release := make(chan struct{}), make(chan struct{})
	r.afterRead = func() { close(read); <-release }
	reloaded := make(chan struct{})
	go func() { r.Reload(); close(reloaded) }()
	<-read // the reload holds the file's old contents, with "a"
	deleted := make(chan struct{})
	go func() { r.Delete("a"); close(deleted) }()
	select { // the delete must wait for the reload, or run first; either way...
	case <-deleted:
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-reloaded
	<-deleted
	if _, ok := r.Get("a"); ok { // ...the deleted lane must not be back
		t.Fatal("a reload applied a stale read over a delete")
	}
}

func TestAgentReadyUnderTheLock(t *testing.T) {
	t.Parallel()
	sid := "s1"
	for _, c := range []struct {
		agents []signals.Agent
		ready  bool
	}{
		{[]signals.Agent{{SessionID: sid, Status: "idle"}}, true},
		{[]signals.Agent{{SessionID: sid, Status: "busy"}}, false},
		{[]signals.Agent{{SessionID: sid, Status: "idle", WaitingFor: "permission prompt"}}, false},
		{[]signals.Agent{{SessionID: sid, Status: "waiting"}}, false},
		{[]signals.Agent{{SessionID: "other", Status: "idle"}}, false},
		{nil, false},
	} {
		if got := agentReady(c.agents, sid) == ""; got != c.ready {
			t.Errorf("%+v: ready %v, want %v", c.agents, got, c.ready)
		}
	}
}

func TestSelectRestorable(t *testing.T) {
	t.Parallel()
	sid := func(n byte) string { return "11111111-1111-4111-8111-11111111111" + string(n) }
	recs := []types.LaneRecord{
		{ID: "a", SessionID: sid('a'), Path: "/ok"},
		{ID: "b", SessionID: sid('b'), Path: "/ok"},   // running: not lost
		{ID: "c", SessionID: sid('c'), Path: "/ok"},   // live in another process
		{ID: "d", SessionID: sid('a'), Path: "/ok"},   // same session as a
		{ID: "e", SessionID: sid('e'), Path: "/gone"}, // worktree removed
		{ID: "f", SessionID: "not-a-uuid", Path: "/ok"},
	}
	pick, skip := selectRestorable(recs, map[string]bool{"b": true}, map[string]bool{sid('c'): true},
		func(p string) bool { return p == "/ok" })
	if len(pick) != 1 || pick[0].ID != "a" {
		t.Fatalf("pick %+v", pick)
	}
	why := map[string]string{}
	for _, s := range skip {
		why[s.ID] = s.Reason
	}
	for id, want := range map[string]string{"c": "already runs", "d": "restored once only", "e": "no longer exists", "f": "no valid session id"} {
		if !strings.Contains(why[id], want) {
			t.Errorf("%s: skip reason %q, want %q", id, why[id], want)
		}
	}
	if _, ok := why["b"]; ok {
		t.Error("a running lane is not a skip, it is not lost")
	}
}

// PANEL-48: the lane context policy, passed as claude --settings on every launch.

// policyTmux stands in for the lane socket and keeps every lane command it is asked
// to launch. A lane's session goes when it is sent Escape or killed, as a claude
// that exits would. Nothing here starts a tmux server.
type policyTmux struct {
	mu       sync.Mutex
	sessions map[string]string     // lane id -> the directory it started in
	launches map[string][][]string // lane id -> each launch's lane command
}

func newPolicyTmux() *policyTmux {
	return &policyTmux{sessions: map[string]string{}, launches: map[string][][]string{}}
}

func (f *policyTmux) exec(_ context.Context, argv []string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	args := argv[4:] // after -L <socket> -f /dev/null
	switch args[0] {
	case "new-session": // new-session -d -s <id> -c <path> ... /usr/bin/env ... ; set-option ...
		id := args[3]
		f.sessions[id] = args[5]
		start := -1
		for i, a := range args {
			if a == "/usr/bin/env" && start < 0 {
				start = i
			}
			if a == ";" && start >= 0 {
				f.launches[id] = append(f.launches[id], append([]string(nil), args[start:i]...))
				break
			}
		}
	case "has-session":
		if _, ok := f.sessions[strings.TrimPrefix(args[2], "=")]; !ok {
			return nil, errors.New("tmux: can't find session")
		}
	case "list-panes":
		var b strings.Builder
		for id, p := range f.sessions {
			b.WriteString(id + "\t" + p + "\t" + p + "\t0\t\t0\t0\tfix\t\n")
		}
		return []byte(b.String()), nil
	case "send-keys":
		if args[len(args)-1] == "Escape" {
			delete(f.sessions, strings.TrimSuffix(strings.TrimPrefix(args[2], "="), ":"))
		}
	case "kill-session":
		delete(f.sessions, strings.TrimPrefix(args[2], "="))
	}
	return nil, nil
}

// gone drops a lane's session, as a reboot or a dead tmux server does.
func (f *policyTmux) gone(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, id)
}

// last is the lane command of a lane's latest launch, and how many it has had.
func (f *policyTmux) last(t *testing.T, id string) ([]string, int) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	l := f.launches[id]
	if len(l) == 0 {
		t.Fatalf("lane %s was never launched", id)
	}
	return l[len(l)-1], len(l)
}

// settingsArg is the one argv element after --settings, or "" with none. --settings
// appearing twice is a failure.
func settingsArg(t *testing.T, argv []string) string {
	t.Helper()
	got := ""
	for i, a := range argv {
		if a == "--settings" {
			if got != "" || i+1 >= len(argv) {
				t.Fatalf("--settings twice or with no value: %q", argv)
			}
			got = argv[i+1]
		}
	}
	return got
}

// settingsEnv decodes a --settings element, which must hold an env object and nothing else.
func settingsEnv(t *testing.T, s string) map[string]string {
	t.Helper()
	var v struct {
		Env map[string]string `json:"env"`
	}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil || v.Env == nil {
		t.Fatalf("--settings %q is not an env object alone: %v", s, err)
	}
	return v.Env
}

// policyRepo is a closeRepo whose lanes launch on a policyTmux, under cfg.
func policyRepo(t *testing.T, cfg string) (*closeRepo, *policyTmux) {
	t.Helper()
	r := newCloseRepo(t)
	c, err := loadConfig(t, []byte(cfg))
	if err != nil {
		t.Fatal(err)
	}
	tm := newPolicyTmux()
	r.m.Cfg, r.m.Exec = c, tm.exec
	r.m.LookupEnv = func(string) (string, bool) { return "", false }
	return r, tm
}

const policyLanes = `"lanes":{"main":"orchestrator","fix/":"fix","change/propose-*":"propose"}`

// [LANECTX-1-S1] A new lane carries the window as settings, and still ends in
// --session-id. [LANECTX-1-S2] A restarted, resumed or restored lane carries the
// same settings again and ends in --resume, since --resume does not restore them.
func TestLaneLaunchCarriesThePolicyEveryTime(t *testing.T) {
	r, tm := policyRepo(t, `{"name":"T","version":6,`+policyLanes+`,"launch":{"context_window":200000}}`)
	ctx := context.Background()
	const want = `{"env":{"CLAUDE_CODE_AUTO_COMPACT_WINDOW":"200000"}}`
	res, lerr := r.m.Start(ctx, StartRequest{Type: "fix", Mode: "root", Name: "fx"})
	if lerr != nil {
		t.Fatal(lerr)
	}
	argv, _ := tm.last(t, "fx")
	if got := settingsArg(t, argv); got != want {
		t.Fatalf("LANECTX-1-S1: --settings %q, want %q (argv %q)", got, want, argv)
	}
	if tail := argv[len(argv)-2:]; !reflect.DeepEqual(tail, []string{"--session-id", res.SessionID}) {
		t.Fatalf("LANECTX-1-S1: a new lane's command ends %q", tail)
	}
	// The lane has a conversation now, so each relaunch is a --resume.
	if _, err := r.m.Registry.MarkConversation(res.SessionID); err != nil {
		t.Fatal(err)
	}
	relaunches := []struct {
		how string
		do  func() *LaneError
	}{
		{"restart", func() *LaneError { return r.m.Restart(ctx, "fx") }},
		{"resume", func() *LaneError { tm.gone("fx"); return r.m.Resume(ctx, "fx") }},
		{"restore", func() *LaneError {
			tm.gone("fx")
			rr, lerr := r.m.RestoreAll(ctx, nil, false)
			if lerr == nil && !reflect.DeepEqual(rr.Restored, []string{"fx"}) {
				t.Fatalf("restore %+v", rr)
			}
			return lerr
		}},
	}
	for i, rl := range relaunches {
		if lerr := rl.do(); lerr != nil {
			t.Fatalf("%s: %v", rl.how, lerr)
		}
		argv, n := tm.last(t, "fx")
		if n != i+2 {
			t.Fatalf("%s: %d launches, want %d", rl.how, n, i+2)
		}
		if got := settingsArg(t, argv); got != want {
			t.Errorf("LANECTX-1-S2: a %s's --settings %q, want %q", rl.how, got, want)
		}
		if tail := argv[len(argv)-2:]; !reflect.DeepEqual(tail, []string{"--resume", res.SessionID}) {
			t.Errorf("LANECTX-1-S2: a %s's command ends %q", rl.how, tail)
		}
		if rec, _ := r.m.Registry.Get("fx"); rec.ContextWindow != 200000 {
			t.Errorf("a %s recorded window %d, want 200000", rl.how, rec.ContextWindow)
		}
	}
}

// The window a lane is measured against is the one it was launched with: a config
// edit leaves the running lane's record alone, and a restart records (and passes) the
// new one. This is the launch-side precondition of scenario S3 of LANECTX-2, not its
// citation: that scenario's THEN is a context reading (90%, then against 300000),
// which task 4.1's state test asserts from a recorded window, and cites.
func TestLaneRecordsTheWindowOfEachLaunch(t *testing.T) {
	r, tm := policyRepo(t, `{"name":"T","version":6,`+policyLanes+`,"launch":{"context_window":200000}}`)
	ctx := context.Background()
	if _, lerr := r.m.Start(ctx, StartRequest{Type: "fix", Mode: "root", Name: "fx"}); lerr != nil {
		t.Fatal(lerr)
	}
	if rec, _ := r.m.Registry.Get("fx"); rec.ContextWindow != 200000 {
		t.Fatalf("a lane started at 200000 recorded %d", rec.ContextWindow)
	}
	edited, err := loadConfig(t, []byte(`{"name":"T","version":6,`+policyLanes+`,"launch":{"context_window":300000}}`))
	if err != nil {
		t.Fatal(err)
	}
	r.m.Cfg = edited
	if rec, _ := r.m.Registry.Get("fx"); rec.ContextWindow != 200000 {
		t.Fatalf("a config edit moved the running lane's window to %d", rec.ContextWindow)
	}
	if lerr := r.m.Restart(ctx, "fx"); lerr != nil {
		t.Fatal(lerr)
	}
	if rec, _ := r.m.Registry.Get("fx"); rec.ContextWindow != 300000 {
		t.Fatalf("after a restart the lane recorded %d, want 300000", rec.ContextWindow)
	}
	argv, _ := tm.last(t, "fx")
	if got := settingsArg(t, argv); got != `{"env":{"CLAUDE_CODE_AUTO_COMPACT_WINDOW":"300000"}}` {
		t.Fatalf("the restart passed %q", got)
	}
	// A lane started under a config with no window records none.
	r2, _ := policyRepo(t, `{"name":"T","version":6,`+policyLanes+`}`)
	if _, lerr := r2.m.Start(ctx, StartRequest{Type: "fix", Mode: "root", Name: "fx"}); lerr != nil {
		t.Fatal(lerr)
	}
	if rec, _ := r2.m.Registry.Get("fx"); rec.ContextWindow != 0 {
		t.Fatalf("a lane with no window recorded %d", rec.ContextWindow)
	}
}

// A recorded window outside Claude Code's range is no window the panel launched
// with: the record reads as corrupt, as a bad model does, and is never launched.
func TestRegistryRefusesAWindowThePanelNeverPasses(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	r, err := OpenRegistry(home, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	for id, w := range map[string]int{"none": 0, "ok": 200000, "low": 99999, "high": 1000001, "neg": -1} {
		if err := r.Put(types.LaneRecord{ID: id, SessionID: sid, Path: "/repo", Type: "fix", Mode: "root", Action: "start", ContextWindow: w}); err != nil {
			t.Fatal(err)
		}
	}
	r2, err := OpenRegistry(home, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	for id, corrupt := range map[string]bool{"none": false, "ok": false, "low": true, "high": true, "neg": true} {
		rec, ok := r2.Get(id)
		if !ok || (rec.Corrupt != "") != corrupt || (corrupt && !strings.Contains(rec.Corrupt, "context window")) {
			t.Errorf("%s: corrupt %q, want corrupt=%v", id, rec.Corrupt, corrupt)
		}
	}
}

// policyManager is testLaneManager under another config.
func policyManager(t *testing.T, cfg string) *LaneManager {
	t.Helper()
	m := testLaneManager(t)
	c, err := loadConfig(t, []byte(cfg))
	if err != nil {
		t.Fatal(err)
	}
	m.Cfg = c
	return m
}

// [LANECTX-1-S3] A lane type's window overrides launch.context_window.
func TestLaneTypeWindowOverridesTheDefault(t *testing.T) {
	t.Parallel()
	m := policyManager(t, `{"name":"T","version":6,`+policyLanes+`,"launch":{"context_window":200000},
		"lane_types":{"propose":{"context_window":300000}}}`)
	for laneType, want := range map[string]string{"propose": "300000", "fix": "200000"} {
		env := settingsEnv(t, settingsArg(t, m.LaneCommand("x", laneType, sid, false)))
		if got := env["CLAUDE_CODE_AUTO_COMPACT_WINDOW"]; got != want {
			t.Errorf("LANECTX-1-S3: a %s lane's window %q, want %q", laneType, got, want)
		}
	}
}

// [LANECTX-1-S4] A config without a policy launches as today: no --settings, and the
// argv equals the one before version 6.
func TestNoPolicyLaunchesAsToday(t *testing.T) {
	t.Parallel()
	today := testLaneManager(t)
	for _, cfg := range []string{
		`{"name":"T","version":6,"lanes":{"main":"orchestrator","fix/":"fix","change/":"build","change/propose-*":"propose"},
			"tmux_socket":"sock","lane_types":{"build":{"model":"opus","effort":"high"}}}`,
		// An empty launch, and connectors left on, set nothing either.
		`{"name":"T","version":6,"lanes":{"main":"orchestrator","fix/":"fix","change/":"build","change/propose-*":"propose"},
			"tmux_socket":"sock","launch":{"claudeai_connectors":true},"lane_types":{"build":{"model":"opus","effort":"high"}}}`,
		`{"name":"T","version":6,"lanes":{"main":"orchestrator","fix/":"fix","change/":"build","change/propose-*":"propose"},
			"tmux_socket":"sock","launch":{},"lane_types":{"build":{"model":"opus","effort":"high"}}}`,
	} {
		m := policyManager(t, cfg)
		for _, laneType := range []string{"build", "fix"} {
			for _, resume := range []bool{false, true} {
				got := m.LaneCommand("add-x", laneType, sid, resume)
				if s := settingsArg(t, got); s != "" {
					t.Errorf("LANECTX-1-S4: a config with no policy passed --settings %q", s)
				}
				if want := today.LaneCommand("add-x", laneType, sid, resume); !reflect.DeepEqual(got, want) {
					t.Errorf("LANECTX-1-S4: argv %q, want today's %q", got, want)
				}
			}
		}
	}
}

// [LANECTX-1-S5] Connectors off is one more variable in the same settings, and the
// settings hold no other key.
func TestConnectorsOffJoinsTheSameSettings(t *testing.T) {
	t.Parallel()
	m := policyManager(t, `{"name":"T","version":6,`+policyLanes+`,"launch":{"context_window":200000,"claudeai_connectors":false}}`)
	argv := m.LaneCommand("x", "fix", sid, false)
	env := settingsEnv(t, settingsArg(t, argv))
	want := map[string]string{"ENABLE_CLAUDEAI_MCP_SERVERS": "false", "CLAUDE_CODE_AUTO_COMPACT_WINDOW": "200000"}
	if !reflect.DeepEqual(env, want) {
		t.Fatalf("LANECTX-1-S5: settings env %v, want exactly %v", env, want)
	}
}

// [LANECTX-1-S6] A subagent model sets the default for subagents, never the forced one.
func TestSubagentModelIsADefaultNotForced(t *testing.T) {
	t.Parallel()
	m := policyManager(t, `{"name":"T","version":6,`+policyLanes+`,"lane_types":{"fix":{"subagent_model":"sonnet"}}}`)
	env := settingsEnv(t, settingsArg(t, m.LaneCommand("x", "fix", sid, false)))
	if env["CLAUDE_CODE_SUBAGENT_MODEL"] != "sonnet" {
		t.Errorf("LANECTX-1-S6: settings env %v has no CLAUDE_CODE_SUBAGENT_MODEL sonnet", env)
	}
	if _, ok := env["CLAUDE_CODE_SUBAGENT_MODEL_FORCE"]; ok {
		t.Errorf("LANECTX-1-S6: settings env %v forces the subagent model", env)
	}
	// Another lane type gets none of it, and so no --settings at all.
	if s := settingsArg(t, m.LaneCommand("x", "propose", sid, false)); s != "" {
		t.Errorf("a propose lane got the fix lane's settings %q", s)
	}
}
