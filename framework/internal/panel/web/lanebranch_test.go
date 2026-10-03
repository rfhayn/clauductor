package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// PANEL-29: an existing branch is never a raw git error. The HTTP reply carries the
// plain sentence and where the branch is; GET /lanes/branch says the same before Start.

// branchRepo is a served project on a real git repository with an origin. tmux has no
// server: nothing here gets as far as starting a session.
type branchRepo struct {
	t    *testing.T
	root string
	s    *Server
	lm   *lanes.LaneManager
	mu   sync.Mutex
	ran  []string // every command the lane manager ran
}

func newBranchRepo(t *testing.T) *branchRepo {
	t.Helper()
	r := &branchRepo{t: t, root: signals.ResolvePath(t.TempDir())}
	r.git(r.root, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(r.root, ".gitignore"), []byte(".wt/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.git(r.root, "add", ".")
	r.git(r.root, "commit", "-q", "-m", "init")
	origin := filepath.Join(t.TempDir(), "origin.git")
	r.git(r.root, "init", "-q", "--bare", origin)
	r.git(r.root, "remote", "add", "origin", origin)
	r.git(r.root, "push", "-q", "origin", "main")

	cfg, err := loadConfig(t, []byte(`{"name":"T","lanes":{"main":"orchestrator","change/":"build","fix/":"fix"},"base":"main","worktree_dir":".wt",
		"templates":[{"id":"build","title":"Build","lane_type":"build","branch_pattern":"change/{name}","first_prompt":"/build-change {name}"},
		{"id":"fix","title":"Fix","lane_type":"fix","branch_pattern":"fix/{name}","first_prompt":"Fix issue {issue}"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	reg, err := lanes.OpenRegistry(t.TempDir(), r.root)
	if err != nil {
		t.Fatal(err)
	}
	r.lm = &lanes.LaneManager{Clock: clock.System, TmuxPath: "/nonexistent/tmux", Socket: "unused", Root: r.root, Cfg: cfg, Registry: reg,
		LookupEnv: func(string) (string, bool) { return "", false },
		Exec: func(context.Context, []string) ([]byte, error) {
			return nil, errors.New("tmux: no server running on /x")
		},
		Run: func(ctx context.Context, dir string, argv []string) ([]byte, error) {
			r.mu.Lock()
			r.ran = append(r.ran, strings.Join(argv, " "))
			r.mu.Unlock()
			if argv[0] != "git" {
				return nil, fmt.Errorf("unexpected command %v", argv)
			}
			return signals.ExecRunner(ctx, dir, argv)
		}}
	r.s, _ = newTestServer(t)
	m := state.NewModel(cfg, r.root, t0)
	r.s.Projects = []*Project{{ID: "a", Name: "A", Hub: NewHub(m, clock.Func(time.Now)), Lanes: r.lm, Refresh: func() {}}}
	r.s.Default = "a"
	return r
}

func (r *branchRepo) git(dir string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// worktree adds a worktree on a new branch and returns its resolved path.
func (r *branchRepo) worktree(branch, dir string) string {
	r.t.Helper()
	p := filepath.Join(r.root, ".wt", dir)
	r.git(r.root, "worktree", "add", "-q", "-b", branch, p, "main")
	return signals.ResolvePath(p)
}

// branch makes a branch with no worktree.
func (r *branchRepo) branch(name string) { r.t.Helper(); r.git(r.root, "branch", name, "main") }

// originOnly makes a branch origin has (and this clone last fetched) but no local one.
func (r *branchRepo) originOnly(name string) {
	r.t.Helper()
	r.branch(name)
	r.git(r.root, "push", "-q", "origin", name)
	r.git(r.root, "branch", "-q", "-D", name)
}

func (r *branchRepo) commands() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ran...)
}

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not JSON: %s", b)
	}
	return m
}

// [NEWLANE-2-S1] A new branch that already exists is refused plainly: the HTTP reply
// says "A branch named change/add-score-photo already exists" and names the branch
// and its worktree, if one has it. Both a plain lane and a template lane, and the
// sentence offers only what the server would accept instead.
func TestNewBranchThatExistsIsRefusedPlainly(t *testing.T) {
	t.Parallel()
	origin := withHeader("Origin", "http://127.0.0.1:4393")
	const tpl, plain = `{"template":"build","name":"add-score-photo","mode":"new"}`, `{"type":"build","name":"add-score-photo","mode":"new"}`
	for _, c := range []struct {
		name, body string
		wtDir      string // the branch's worktree under .wt, or "" for none
		says       string
	}{
		{"a template, the branch in a worktree", tpl, "score", "Start the lane on that worktree."},
		{"a plain lane, the branch in a worktree", plain, "score", "Start the lane on that worktree."},
		// An earlier "new" lane's worktree sits at exactly the path this start would
		// take: the branch, not the folder, is what the reply names.
		{"a template, the branch in the worktree at the lane's own path", tpl, "add-score-photo", "Start the lane on that worktree."},
		{"a plain lane, the branch in the worktree at the lane's own path", plain, "add-score-photo", "Start the lane on that worktree."},
		{"a template, the branch with no worktree", tpl, "", "Start on the branch in a new worktree."},
		{"a plain lane, the branch with no worktree", plain, "", "Choose another name for the lane."},
	} {
		r := newBranchRepo(t)
		wt := ""
		if c.wtDir != "" {
			wt = r.worktree("change/add-score-photo", c.wtDir)
		} else {
			r.branch("change/add-score-photo")
		}
		w := do(r.s, "POST", "/api/p/a/lanes", c.body, withCookie(r.s), origin)
		got := decode(t, w.Body.Bytes())
		msg, _ := got["error"].(string)
		if w.Code != 409 || got["code"] != "branch-exists" || !strings.HasPrefix(msg, "A branch named change/add-score-photo already exists") {
			t.Errorf("%s: %d %s", c.name, w.Code, w.Body.String())
			continue
		}
		if strings.Contains(msg, "git") || strings.Contains(msg, "exit status") {
			t.Errorf("%s: the reply passes git's error through: %q", c.name, msg)
		}
		if !strings.HasSuffix(msg, c.says) {
			t.Errorf("%s: %q does not end %q", c.name, msg, c.says)
		}
		if got["branch"] != "change/add-score-photo" || got["local"] != true {
			t.Errorf("%s: the reply does not name the branch: %s", c.name, w.Body.String())
		}
		if wtGot, has := got["worktree"]; wt != "" && wtGot != wt || wt == "" && has {
			t.Errorf("%s: worktree %v, want %q", c.name, wtGot, wt)
		}
		for _, cmd := range r.commands() {
			if strings.HasPrefix(cmd, "git worktree add") {
				t.Errorf("%s: ran %q", c.name, cmd)
			}
		}
		if _, ok := r.lm.Registry.Get("add-score-photo"); ok {
			t.Errorf("%s: a refused start left a registry record", c.name)
		}
	}
}

// GET /lanes/branch (D6) answers where a template's branch already is, for each of
// the three cases the dialog acts on, and fetches nothing.
func TestLaneBranchSaysWhereTheBranchIs(t *testing.T) {
	t.Parallel()
	r := newBranchRepo(t)
	wt := r.worktree("change/in-a-worktree", "inwt")
	r.branch("change/local-only")
	r.originOnly("change/origin-only")
	for _, c := range []struct {
		query string
		want  lanes.BranchFacts
	}{
		{"template=build&name=in-a-worktree", lanes.BranchFacts{Branch: "change/in-a-worktree", Local: true, Worktree: wt}},
		{"template=build&name=local-only", lanes.BranchFacts{Branch: "change/local-only", Local: true}},
		{"template=build&name=origin-only", lanes.BranchFacts{Branch: "change/origin-only", Remote: true}},
		{"template=build&name=nowhere", lanes.BranchFacts{Branch: "change/nowhere"}},
		// The issue is only in fix's first prompt, so its branch needs none.
		{"template=fix&name=nowhere", lanes.BranchFacts{Branch: "fix/nowhere"}},
	} {
		for _, pre := range []string{"/api/p/a", "/api"} {
			w := do(r.s, "GET", pre+"/lanes/branch?"+c.query, "", withCookie(r.s))
			var got struct {
				OK bool `json:"ok"`
				lanes.BranchFacts
			}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 || !got.OK || got.BranchFacts != c.want {
				t.Errorf("GET %s/lanes/branch?%s = %d %s, want %+v", pre, c.query, w.Code, w.Body.String(), c.want)
			}
		}
	}
	for _, bad := range []string{"template=nope&name=x", "template=build&name=Bad%20Name", "name=x"} {
		if w := do(r.s, "GET", "/api/p/a/lanes/branch?"+bad, "", withCookie(r.s)); w.Code != 400 {
			t.Errorf("GET ?%s = %d %s, want 400", bad, w.Code, w.Body.String())
		}
	}
	if w := do(r.s, "GET", "/api/p/a/lanes/branch?template=build&name=x", ""); w.Code != 401 {
		t.Errorf("without the cookie: %d", w.Code)
	}
	// An untrusted panel.json's templates are off here as at Start: the same 409, and
	// its branch pattern is not rendered or looked up.
	before := len(r.commands())
	r.s.Projects[0].Orch = &Orchestration{Trusted: func() bool { return false }}
	w := do(r.s, "GET", "/api/p/a/lanes/branch?template=build&name=in-a-worktree", "", withCookie(r.s))
	if got := decode(t, w.Body.Bytes()); w.Code != 409 || got["code"] != "untrusted-config" {
		t.Errorf("untrusted: %d %s, want 409 untrusted-config", w.Code, w.Body.String())
	}
	if ran := r.commands()[before:]; len(ran) != 0 {
		t.Errorf("untrusted: ran %q", ran)
	}
	r.s.Projects[0].Orch = &Orchestration{Trusted: func() bool { return true }}
	if w := do(r.s, "GET", "/api/p/a/lanes/branch?template=build&name=in-a-worktree", "", withCookie(r.s)); w.Code != 200 {
		t.Errorf("trusted: %d %s", w.Code, w.Body.String())
	}
	for _, cmd := range r.commands() {
		if !strings.HasPrefix(cmd, "git for-each-ref ") && cmd != "git worktree list --porcelain" && !strings.HasPrefix(cmd, "git check-ref-format ") {
			t.Errorf("the read-only call ran %q", cmd)
		}
	}
}

// A branch held by a worktree stopped mid-rebase reaches the page as `busy` beside
// `worktree`, in the 409 at Start and in the GET alike, with the advice to finish it
// there: the page must not route the start to that worktree, which D1 refuses.
func TestABusyBranchReachesThePage(t *testing.T) {
	t.Parallel()
	r := newBranchRepo(t)
	wt := r.worktree("change/add-score-photo", "score")
	for _, f := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(wt, f), []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
		r.git(wt, "add", f)
		r.git(wt, "commit", "-q", "-m", f)
	}
	if err := os.WriteFile(filepath.Join(r.root, "m.txt"), []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.git(r.root, "add", "m.txt")
	r.git(r.root, "commit", "-q", "-m", "main moves")
	cmd := exec.Command("git", "rebase", "--exec", "false", "main") // stops after the first pick
	cmd.Dir = wt
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_EDITOR=true")
	_ = cmd.Run()
	sentence := "A branch named change/add-score-photo is in the middle of a rebase in the worktree " + wt +
		". Finish or abort it there, then start the lane on that worktree."

	w := do(r.s, "POST", "/api/p/a/lanes", `{"template":"build","name":"add-score-photo","mode":"new"}`, withCookie(r.s), withHeader("Origin", "http://127.0.0.1:4393"))
	got := decode(t, w.Body.Bytes())
	if w.Code != 409 || got["code"] != "branch-exists" || got["error"] != sentence || got["busy"] != "rebase" || got["worktree"] != wt {
		t.Errorf("POST new: %d %s", w.Code, w.Body.String())
	}
	w = do(r.s, "GET", "/api/p/a/lanes/branch?template=build&name=add-score-photo", "", withCookie(r.s))
	got = decode(t, w.Body.Bytes())
	if w.Code != 200 || got["busy"] != "rebase" || got["worktree"] != wt {
		t.Errorf("GET: %d %s", w.Code, w.Body.String())
	}
	// Not busy: no busy field at all.
	w = do(r.s, "GET", "/api/p/a/lanes/branch?template=build&name=other", "", withCookie(r.s))
	if _, has := decode(t, w.Body.Bytes())["busy"]; w.Code != 200 || has {
		t.Errorf("GET a free branch: %d %s", w.Code, w.Body.String())
	}
}
