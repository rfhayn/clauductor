package panel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestValidLaneID(t *testing.T) {
	ok := []string{"a", "0", "add-login", "orchestrator", "x-1-2", strings.Repeat("a", 41)}
	bad := []string{"", "-a", "A", "Add", "a_b", "a.b", "a:b", "a b", "a/b", "../x", "=a", "a;b", "a'b",
		"a\nb", "é", strings.Repeat("a", 42), "$(id)", "`id`"}
	for _, id := range ok {
		if !ValidLaneID(id) {
			t.Errorf("ValidLaneID(%q) = false, want true", id)
		}
	}
	for _, id := range bad {
		if ValidLaneID(id) {
			t.Errorf("ValidLaneID(%q) = true, want false", id)
		}
	}
}

func testLaneManager(t *testing.T) *LaneManager {
	t.Helper()
	cfg, err := ParseConfig([]byte(`{"name":"T","lanes":{"main":"orchestrator","fix/":"fix","change/":"build","change/propose-*":"propose"},
		"tmux_socket":"sock","lane_types":{"build":{"model":"opus","effort":"high"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := OpenRegistry(t.TempDir(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	return &LaneManager{TmuxPath: "/opt/homebrew/bin/tmux", Socket: cfg.Socket(), Root: "/repo", Cfg: cfg, Registry: reg,
		Program: []string{"/Users/me/.local/bin/claude"}, LookupEnv: func(string) (string, bool) { return "", false }}
}

const sid = "0f8fad5b-d9cb-469f-a165-70867728950e"

// Every command is an argv list; nothing a browser supplies is ever parsed by a shell.
func TestLaneArgvConstruction(t *testing.T) {
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
	head := []string{"-L", "sock", "new-session", "-d", "-s", "add-x", "-c", "/repo/.claude/worktrees/add-x", "-x", "200", "-y", "50"}
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
		";", "set-option", "-t", "=add-x:", "window-size", "latest"}
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

	if got, want := m.AttachArgv("add-x"), []string{"-u", "-L", "sock", "attach-session", "-t", "=add-x"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("AttachArgv %q, want %q", got, want)
	}

	osa := m.TerminalAppArgv("add-x")
	if osa[0] != "/usr/bin/osascript" {
		t.Fatalf("TerminalAppArgv runs %q", osa[0])
	}
	cmd := osa[len(osa)-1]
	if want := `exec '/opt/homebrew/bin/tmux' '-u' '-L' 'sock' 'attach-session' '-t' '=add-x'`; cmd != want {
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
	if m.Cfg.Socket() != "sock" || m.Cfg.BaseRef() != DefaultBase || m.Cfg.WorktreeRoot("/repo") != "/repo/.claude/worktrees" {
		t.Fatalf("defaults: %s %s %s", m.Cfg.Socket(), m.Cfg.BaseRef(), m.Cfg.WorktreeRoot("/repo"))
	}
}

func TestConfigRejectsBadLaneKeys(t *testing.T) {
	for _, body := range []string{
		`{"name":"x","tmux_socket":"a b"}`,
		`{"name":"x","tmux_socket":"../x"}`,
		`{"name":"x","base":"-x"}`,
		`{"name":"x","base":"a b"}`,
		`{"name":"x","worktree_dir":"../outside"}`,
		`{"name":"x","worktree_dir":"."}`,
		`{"name":"x","lane_types":{"build":{"model":"opus; rm -rf /"}}}`,
		`{"name":"x","lane_types":{"build":{"effort":"--dangerously-skip-permissions"}}}`,
		`{"name":"x","lane_types":{"build":{"modle":"opus"}}}`,
	} {
		if _, err := ParseConfig([]byte(body)); err == nil {
			t.Errorf("accepted %s", body)
		}
	}
	if _, err := ParseConfig([]byte(`{"name":"x","tmux_socket":"standingtee","base":"origin/main","worktree_dir":"/abs/wt",
		"lane_types":{"build":{"model":"claude-opus-4-5[1m]","effort":"high"}}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestParseTmuxPanes(t *testing.T) {
	out := "orchestrator\t/tmp\t/tmp\t0\t\t1790000000\t1\torchestrator\n" +
		"orchestrator\t/tmp\t/tmp\t0\t\t1790000000\t1\torchestrator\n" + // second pane of the same session
		"fix-a\t\t/nonexistent/wt\t1\t3\t1790000001\t0\tfix\n" +
		"User Session\t/tmp\t/tmp\t0\t\t1\t0\t\n" + // not a lane id: not the panel's
		"short\tline\n"
	got := ParseTmuxPanes([]byte(out))
	if len(got) != 2 {
		t.Fatalf("got %d lanes: %+v", len(got), got)
	}
	if got[0].ID != "fix-a" || !got[0].Dead || got[0].DeadStatus != "3" || got[0].Path != "/nonexistent/wt" || got[0].Type != "fix" {
		t.Fatalf("dead lane parsed as %+v", got[0])
	}
	if got[1].ID != "orchestrator" || got[1].Attached != 1 || got[1].Path != "/private/tmp" {
		t.Fatalf("live lane parsed as %+v", got[1])
	}
}

// The intent reaches the disk BEFORE the action: a panel killed mid-start leaves a
// record, which the next start shows as an orphan instead of losing the lane.
func TestRegistryIsWrittenBeforeTheAction(t *testing.T) {
	m := testLaneManager(t)
	m.TmuxPath = filepath.Join(t.TempDir(), "tmux") // a tmux whose socket has no server
	os.WriteFile(m.TmuxPath, []byte("#!/bin/sh\necho 'no server running on /tmp/x' >&2\nexit 1\n"), 0o755)
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
	home := t.TempDir()
	r, err := OpenRegistry(home, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	rec := LaneRecord{ID: "a", SessionID: sid, Path: "/repo", Type: "orchestrator", Mode: "root", Created: 1}
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
	a, _ := NewSessionID()
	b, _ := NewSessionID()
	if !uuidRe.MatchString(a) || a == b {
		t.Fatalf("%q %q", a, b)
	}
}

// A reload racing a stop must never bring the stopped lane back. Seen live: the stop
// kicks the tmux loop, whose 30 s reload read the file just before Stop deleted the
// record, then applied that stale copy after the delete; the lane stayed "registered"
// until the next reload. This forces that interleaving.
func TestRegistryReloadNeverResurrectsADeletedLane(t *testing.T) {
	r, err := OpenRegistry(t.TempDir(), "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Begin(LaneRecord{ID: "a", SessionID: sid, Path: "/repo"}, "stop", time.Now()); err != nil {
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
