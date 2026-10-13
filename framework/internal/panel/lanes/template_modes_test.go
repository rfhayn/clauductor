package lanes

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// PANEL-29: a template lane starts where its branch already is. A template names one
// branch, so it runs in that branch's worktree ("existing"), on that branch in a new
// worktree ("branch"), or on a new branch ("new"); never in the project root.

const tplConfig = `{"name":"T","lanes":{"main":"orchestrator","fix/":"fix","change/":"build"},"base":"main","worktree_dir":".wt",
	"templates":[{"id":"build","title":"Build","lane_type":"build","branch_pattern":"change/{name}","first_prompt":"/build-change {name}"},
	{"id":"propose","title":"Propose","lane_type":"build","branch_pattern":"change/{name}","first_prompt":"/propose {name}"}]}`

// fakeTmux stands in for the lane socket: new-session records a session, has-session
// and list-panes report the recorded ones, and send-keys -l records the typed text.
// Nothing here starts a tmux server.
type fakeTmux struct {
	mu       sync.Mutex
	sessions map[string]string // lane id -> the directory it started in
	typed    []string
}

func (f *fakeTmux) exec(_ context.Context, argv []string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	args := argv[4:] // after -L <socket> -f /dev/null
	switch args[0] {
	case "new-session": // new-session -d -s <id> -c <path> ...
		f.sessions[args[3]] = args[5]
	case "has-session":
		if _, ok := f.sessions[strings.TrimPrefix(args[2], "=")]; !ok {
			return nil, errors.New("tmux: can't find session")
		}
	case "list-panes":
		var b strings.Builder
		for id, p := range f.sessions {
			b.WriteString(id + "\t" + p + "\t" + p + "\t0\t\t0\t0\tbuild\t\n")
		}
		return []byte(b.String()), nil
	case "send-keys":
		if len(args) > 4 && args[3] == "-l" {
			f.typed = append(f.typed, args[len(args)-1])
		}
	}
	return nil, nil
}

func (f *fakeTmux) started(id string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.sessions[id]
	return p, ok
}

func (f *fakeTmux) typedText() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.typed...)
}

// tplRepo is a closeRepo with templates, a fake tmux and an origin to fetch from.
type tplRepo struct {
	*closeRepo
	tmux   *fakeTmux
	origin string
}

func newTplRepo(t *testing.T) *tplRepo {
	t.Helper()
	r := &tplRepo{closeRepo: newCloseRepo(t), tmux: &fakeTmux{sessions: map[string]string{}}}
	cfg, err := loadConfig(t, []byte(tplConfig))
	if err != nil {
		t.Fatal(err)
	}
	r.m.Cfg, r.m.Exec = cfg, r.tmux.exec
	r.origin = filepath.Join(t.TempDir(), "origin.git")
	r.git(r.root, "init", "-q", "--bare", r.origin)
	r.git(r.root, "remote", "add", "origin", r.origin)
	r.git(r.root, "push", "-q", "origin", "main")
	return r
}

// branchWithCommit makes branch, one commit ahead of main, with no worktree left on
// it, and returns that commit.
func (r *tplRepo) branchWithCommit(branch string) string {
	r.t.Helper()
	tmp := filepath.Join(r.t.TempDir(), "tmp-wt")
	r.git(r.root, "worktree", "add", "-q", "-b", branch, tmp, "main")
	sha := r.commit(tmp, "work.txt")
	r.git(r.root, "worktree", "remove", tmp)
	return sha
}

// agentsSay makes `claude agents` list every registered lane with status.
func (r *tplRepo) agentsSay(status string) {
	r.t.Helper()
	var out []signals.Agent
	for _, rec := range r.m.Registry.List() {
		out = append(out, signals.Agent{PID: 1, Cwd: rec.Path, Kind: "interactive", SessionID: rec.SessionID, Name: rec.ID, Status: status})
	}
	j, err := json.Marshal(out)
	if err != nil {
		r.t.Fatal(err)
	}
	r.mu.Lock()
	r.agents = string(j)
	r.mu.Unlock()
}

// firstPromptTypedOnceIdle asserts the template's first prompt waits while claude is
// busy and is typed, once, when claude is idle.
func (r *tplRepo) firstPromptTypedOnceIdle(id, want string) {
	r.t.Helper()
	r.agentsSay("busy")
	if err := r.m.DeliverFirstPrompt(context.Background(), id, nil); err == nil {
		r.t.Fatal("the first prompt was typed while claude was busy")
	}
	if typed := r.tmux.typedText(); len(typed) != 0 {
		r.t.Fatalf("typed %q while claude was busy", typed)
	}
	r.agentsSay("idle")
	if err := r.m.DeliverFirstPrompt(context.Background(), id, nil); err != nil {
		r.t.Fatalf("deliver the first prompt: %v", err)
	}
	if typed := r.tmux.typedText(); len(typed) != 1 || typed[0] != want {
		r.t.Fatalf("typed %q, want [%q]", typed, want)
	}
}

func (r *tplRepo) refs() string {
	r.t.Helper()
	return r.git(r.root, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads")
}

func (r *tplRepo) start(req StartRequest) (StartResult, *LaneError) {
	return r.m.StartLane(context.Background(), req, StartGate{})
}

// [NEWLANE-1-S1] A template starts in its change's existing worktree: the lane runs
// there, no branch is created, and the first prompt is typed once claude is idle.
func TestTemplateStartsInItsChangesExistingWorktree(t *testing.T) {
	t.Parallel()
	r := newTplRepo(t)
	wt := filepath.Join(r.root, ".wt", "score")
	r.git(r.root, "worktree", "add", "-q", "-b", "change/add-score-photo", wt, "main")
	wt = signals.ResolvePath(wt)
	before := r.refs()

	res, lerr := r.start(StartRequest{Template: "build", Name: "add-score-photo", Mode: "existing", Worktree: wt})
	if lerr != nil {
		t.Fatalf("start: %v", lerr)
	}
	if res.Path != wt || res.Branch != "change/add-score-photo" {
		t.Fatalf("started in %s on %s, want %s on change/add-score-photo", res.Path, res.Branch, wt)
	}
	if p, ok := r.tmux.started("add-score-photo"); !ok || p != wt {
		t.Fatalf("the lane's session started in %q (%v), want %s", p, ok, wt)
	}
	if after := r.refs(); after != before {
		t.Fatalf("a branch was created or moved:\nbefore %s\nafter  %s", before, after)
	}
	rec, ok := r.m.Registry.Get("add-score-photo")
	if !ok || rec.Mode != "existing" || rec.Path != wt || rec.Template != "build" || rec.PromptState != "pending" {
		t.Fatalf("registry record: %+v (%v)", rec, ok)
	}
	r.firstPromptTypedOnceIdle("add-score-photo", "/build-change add-score-photo")
}

// [NEWLANE-1-S3] A template refuses another branch's worktree, naming both branches.
func TestTemplateRefusesAnotherBranchesWorktree(t *testing.T) {
	t.Parallel()
	r := newTplRepo(t)
	wt := filepath.Join(r.root, ".wt", "other")
	r.git(r.root, "worktree", "add", "-q", "-b", "change/other-change", wt, "main")

	_, lerr := r.start(StartRequest{Template: "build", Name: "add-score-photo", Mode: "existing", Worktree: signals.ResolvePath(wt)})
	if lerr == nil || lerr.Status != 400 || !strings.Contains(lerr.Msg, "change/other-change") || !strings.Contains(lerr.Msg, "change/add-score-photo") {
		t.Fatalf("start = %v, want a 400 naming change/other-change and change/add-score-photo", lerr)
	}
	if _, ok := r.tmux.started("add-score-photo"); ok {
		t.Fatal("a refused start started a session")
	}
	if _, ok := r.m.Registry.Get("add-score-photo"); ok {
		t.Fatal("a refused start left a registry record")
	}
}

// A template names a branch, so "root" is refused with a plain sentence; "branch"
// (a template's named branch) is refused without a template. Neither runs git.
func TestTemplateModeRefusals(t *testing.T) {
	t.Parallel()
	r := newTplRepo(t)
	var ran []string
	r.m.Run = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		ran = append(ran, strings.Join(argv, " "))
		return r.run(ctx, dir, argv)
	}
	for _, c := range []struct {
		req  StartRequest
		want string
	}{
		{StartRequest{Template: "build", Name: "add-score-photo", Mode: "root"}, "A template names a branch, so it runs in a worktree, not the project root."},
		{StartRequest{Type: "build", Name: "add-score-photo", Mode: "branch"}, "An existing branch is a template's named branch"},
		{StartRequest{Template: "build", Name: "add-score-photo", Mode: "shell"}, "mode must be"},
	} {
		_, lerr := r.start(c.req)
		if lerr == nil || lerr.Status != 400 || !strings.Contains(lerr.Msg, c.want) {
			t.Errorf("start(%+v) = %v, want a 400 saying %q", c.req, lerr, c.want)
		}
	}
	if len(ran) != 0 {
		t.Errorf("a refused start ran %q", ran)
	}
}

// [NEWLANE-1-S2] A template starts on its existing branch in a new worktree, keeping
// its commits, and the first prompt is typed once claude is idle.
func TestTemplateStartsOnItsExistingBranch(t *testing.T) {
	t.Parallel()
	r := newTplRepo(t)
	sha := r.branchWithCommit("change/add-score-photo")
	// As "new" does: the gitignored files .worktreeinclude names, then worktree_setup.
	r.m.Cfg.WorktreeSetup = &config.HookCommand{Command: []string{"true"}}
	for name, body := range map[string]string{"secret.txt": "s", ".worktreeinclude": "secret.txt\n"} {
		if err := os.WriteFile(filepath.Join(r.root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(r.root, ".git", "info", "exclude"), []byte("secret.txt\n.worktreeinclude\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, lerr := r.start(StartRequest{Template: "build", Name: "add-score-photo", Mode: "branch"})
	if lerr != nil {
		t.Fatalf("start: %v", lerr)
	}
	want := signals.ResolvePath(filepath.Join(r.root, ".wt", "add-score-photo"))
	if !exists(filepath.Join(want, "secret.txt")) || !has(res.Notes, ".worktreeinclude: copied 1 file") || !has(res.Notes, "worktree_setup ran") {
		t.Fatalf("the new worktree did not get .worktreeinclude and worktree_setup: %q", res.Notes)
	}
	if res.Path != want || res.Branch != "change/add-score-photo" {
		t.Fatalf("started in %s on %s, want %s on change/add-score-photo", res.Path, res.Branch, want)
	}
	if got := r.git(res.Path, "branch", "--show-current"); got != "change/add-score-photo" {
		t.Fatalf("the new worktree is on %q", got)
	}
	if got := r.git(res.Path, "rev-parse", "HEAD"); got != sha {
		t.Fatalf("the new worktree is at %s, not the branch's commit %s", got, sha)
	}
	if p, ok := r.tmux.started("add-score-photo"); !ok || p != want {
		t.Fatalf("the lane's session started in %q (%v)", p, ok)
	}
	if rec, ok := r.m.Registry.Get("add-score-photo"); !ok || rec.Mode != "branch" || rec.Path != want {
		t.Fatalf("registry record: %+v (%v)", rec, ok)
	}
	r.firstPromptTypedOnceIdle("add-score-photo", "/build-change add-score-photo")
}

// [NEWLANE-1-S4] A branch that exists only on origin is tracked: the fetch runs before
// the check, and the new worktree is on a local branch that tracks origin's.
func TestTemplateTracksAnOriginOnlyBranch(t *testing.T) {
	t.Parallel()
	r := newTplRepo(t)
	sha := r.branchWithCommit("change/add-score-photo")
	r.git(r.root, "push", "-q", "origin", "change/add-score-photo")
	r.git(r.root, "branch", "-q", "-D", "change/add-score-photo")
	// Not fetched yet: only a start that fetches first sees it.
	r.git(r.root, "update-ref", "-d", "refs/remotes/origin/change/add-score-photo")

	res, lerr := r.start(StartRequest{Template: "build", Name: "add-score-photo", Mode: "branch"})
	if lerr != nil {
		t.Fatalf("start: %v", lerr)
	}
	if got := r.git(res.Path, "branch", "--show-current"); got != "change/add-score-photo" {
		t.Fatalf("the new worktree is on %q", got)
	}
	if got := r.git(res.Path, "rev-parse", "--abbrev-ref", "change/add-score-photo@{upstream}"); got != "origin/change/add-score-photo" {
		t.Fatalf("the local branch tracks %q, want origin/change/add-score-photo", got)
	}
	if got := r.git(res.Path, "rev-parse", "HEAD"); got != sha {
		t.Fatalf("the new worktree is at %s, not origin's %s", got, sha)
	}
}

// "branch" refuses a branch that exists nowhere and one that already has a worktree,
// and keeps the folder rule: none leaves a registry record or a session.
func TestBranchModeRefusals(t *testing.T) {
	t.Parallel()
	r := newTplRepo(t)
	_, lerr := r.start(StartRequest{Template: "build", Name: "add-score-photo", Mode: "branch"})
	if lerr == nil || lerr.Status != 400 || lerr.Msg != "No branch named change/add-score-photo; choose New branch and worktree." {
		t.Fatalf("a branch that exists nowhere: %v", lerr)
	}

	wt := filepath.Join(r.root, ".wt", "score")
	r.git(r.root, "worktree", "add", "-q", "-b", "change/add-score-photo", wt, "main")
	_, lerr = r.start(StartRequest{Template: "build", Name: "add-score-photo", Mode: "branch"})
	if lerr == nil || lerr.Status != 409 || lerr.Code != "branch-exists" || lerr.Branch == nil || lerr.Branch.Worktree != signals.ResolvePath(wt) {
		t.Fatalf("a branch with a worktree: %v %+v", lerr, lerr.Branch)
	}

	r2 := newTplRepo(t)
	r2.branchWithCommit("change/add-score-photo")
	if err := os.MkdirAll(filepath.Join(r2.root, ".wt", "add-score-photo"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, lerr = r2.start(StartRequest{Template: "build", Name: "add-score-photo", Mode: "branch"})
	if lerr == nil || lerr.Status != 409 || lerr.Code != "exists" {
		t.Fatalf("the folder exists: %v", lerr)
	}

	// The branch's worktree is the folder this start would take (an earlier "new"
	// lane's): the branch is what the refusal names, not the folder.
	r3 := newTplRepo(t)
	own := filepath.Join(r3.root, ".wt", "add-score-photo")
	r3.git(r3.root, "worktree", "add", "-q", "-b", "change/add-score-photo", own, "main")
	_, lerr = r3.start(StartRequest{Template: "build", Name: "add-score-photo", Mode: "branch"})
	if lerr == nil || lerr.Code != "branch-exists" || lerr.Branch == nil || lerr.Branch.Worktree != signals.ResolvePath(own) {
		t.Fatalf("the branch's worktree at the lane's own path: %v", lerr)
	}
	for _, x := range []*tplRepo{r, r2, r3} {
		if _, ok := x.m.Registry.Get("add-score-photo"); ok {
			t.Error("a refused start left a registry record")
		}
		if _, ok := x.tmux.started("add-score-photo"); ok {
			t.Error("a refused start started a session")
		}
	}
}

// "new" refuses a branch that already exists, locally or only on origin (after the
// fetch), with a plain sentence and the facts, never git's own error.
func TestNewRefusesAnExistingBranch(t *testing.T) {
	t.Parallel()
	r := newTplRepo(t)
	r.branchWithCommit("change/add-score-photo")
	_, lerr := r.start(StartRequest{Template: "build", Name: "add-score-photo", Mode: "new"})
	if lerr == nil || lerr.Status != 409 || lerr.Code != "branch-exists" ||
		lerr.Msg != "A branch named change/add-score-photo already exists. Start on the branch in a new worktree." ||
		lerr.Branch == nil || !lerr.Branch.Local || lerr.Branch.Worktree != "" {
		t.Fatalf("a local branch: %v %+v", lerr, lerr.Branch)
	}

	r2 := newTplRepo(t)
	r2.branchWithCommit("change/add-score-photo")
	r2.git(r2.root, "push", "-q", "origin", "change/add-score-photo")
	r2.git(r2.root, "branch", "-q", "-D", "change/add-score-photo")
	r2.git(r2.root, "update-ref", "-d", "refs/remotes/origin/change/add-score-photo")
	_, lerr = r2.start(StartRequest{Type: "build", Name: "add-score-photo", Mode: "new"})
	if lerr == nil || lerr.Code != "branch-exists" || lerr.Branch == nil || lerr.Branch.Local || !lerr.Branch.Remote {
		t.Fatalf("an origin-only branch, unfetched before the start: %v", lerr)
	}
	for _, x := range []*tplRepo{r, r2} {
		if _, ok := x.m.Registry.Get("add-score-photo"); ok {
			t.Error("a refused start left a registry record")
		}
		if exists(filepath.Join(x.root, ".wt", "add-score-photo")) {
			t.Error("a refused start added a worktree")
		}
	}
}

// for-each-ref also lists the refs under a pattern: origin's change/x/y is not
// change/x, so neither the GET nor "new" takes it for one, and "new" starts change/x.
func TestABranchUnderTheNameIsNotTheBranch(t *testing.T) {
	t.Parallel()
	r := newTplRepo(t)
	r.git(r.root, "branch", "change/x/y", "main")
	r.git(r.root, "push", "-q", "origin", "change/x/y")
	r.git(r.root, "branch", "-q", "-D", "change/x/y")
	if got := r.git(r.root, "for-each-ref", "--format=%(refname)", "refs/remotes/origin/change/x"); got != "refs/remotes/origin/change/x/y" {
		t.Fatalf("the setup lacks origin's change/x/y: %q", got)
	}
	f, lerr := r.m.TemplateBranch(context.Background(), StartGate{}, "build", "x", "")
	if lerr != nil || f != (BranchFacts{Branch: "change/x"}) {
		t.Fatalf("TemplateBranch = %+v, %v; want change/x nowhere", f, lerr)
	}
	res, lerr := r.start(StartRequest{Template: "build", Name: "x", Mode: "new"})
	if lerr != nil {
		t.Fatalf("start change/x beside origin's change/x/y: %v", lerr)
	}
	if got := r.git(res.Path, "branch", "--show-current"); got != "change/x" {
		t.Fatalf("the new worktree is on %q", got)
	}
}

// A branch merge-pr deleted on origin leaves a stale remote-tracking ref here. Start's
// fetch prunes it, so "new" can reuse the name.
func TestNewReusesANameOriginDeleted(t *testing.T) {
	t.Parallel()
	r := newTplRepo(t)
	r.git(r.root, "branch", "change/add-score-photo", "main")
	r.git(r.root, "push", "-q", "origin", "change/add-score-photo")
	r.git(r.root, "branch", "-q", "-D", "change/add-score-photo")
	r.git(r.origin, "branch", "-D", "change/add-score-photo") // merged and deleted on origin
	if got := r.git(r.root, "for-each-ref", "--format=%(refname)", "refs/remotes/origin/change/add-score-photo"); got == "" {
		t.Fatal("the setup lacks the stale remote-tracking ref")
	}
	res, lerr := r.start(StartRequest{Template: "build", Name: "add-score-photo", Mode: "new"})
	if lerr != nil {
		t.Fatalf("start a name origin deleted: %v", lerr)
	}
	if got := r.git(res.Path, "branch", "--show-current"); got != "change/add-score-photo" {
		t.Fatalf("the new worktree is on %q", got)
	}
}

// After the branch check, the folder rule still holds for a free branch: a folder at
// the lane's path is refused 409 exists, and the intent is removed.
func TestNewRefusesAnExistingFolderForAFreeBranch(t *testing.T) {
	t.Parallel()
	r := newTplRepo(t)
	dir := filepath.Join(r.root, ".wt", "add-score-photo")
	if err := os.MkdirAll(dir, 0o755); err != nil { // empty: git itself would add a worktree there
		t.Fatal(err)
	}
	_, lerr := r.start(StartRequest{Template: "build", Name: "add-score-photo", Mode: "new"})
	if lerr == nil || lerr.Status != 409 || lerr.Code != "exists" || lerr.Branch != nil {
		t.Fatalf("start = %v, want 409 exists", lerr)
	}
	if _, ok := r.m.Registry.Get("add-score-photo"); ok {
		t.Error("the refused start left its registry record")
	}
	if got := r.git(r.root, "for-each-ref", "--format=%(refname)", "refs/heads/change/add-score-photo"); got != "" {
		t.Errorf("the refused start created %s", got)
	}
	if _, ok := r.tmux.started("add-score-photo"); ok {
		t.Error("the refused start started a session")
	}
}

// stoppedWorktree adds a worktree at .wt/<dir> on a new branch three commits ahead of
// main, moves main, and stops it mid-"rebase" (after the first pick) or mid-"bisect"
// (at a midpoint): detached in `git worktree list`, the branch still held there.
func (r *tplRepo) stoppedWorktree(branch, dir, stop string) string {
	r.t.Helper()
	wt := filepath.Join(r.root, ".wt", dir)
	r.git(r.root, "worktree", "add", "-q", "-b", branch, wt, "main")
	wt = signals.ResolvePath(wt)
	r.commit(wt, "a.txt")
	r.commit(wt, "b.txt")
	r.commit(wt, "c.txt")
	r.commit(r.root, dir+"-main-moved.txt")
	cmd := exec.Command("git", "rebase", "--exec", "false", "main")
	if stop == "bisect" {
		cmd = exec.Command("git", "bisect", "start", "HEAD", "HEAD~3")
	}
	cmd.Dir = wt
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t", "GIT_EDITOR=true")
	_ = cmd.Run() // a stopped rebase exits non-zero
	wts, err := signals.ReadWorktrees(context.Background(), r.run, r.root)
	if err != nil {
		r.t.Fatal(err)
	}
	for _, w := range wts {
		if w.Path == wt && w.Branch != "" {
			r.t.Fatalf("%s: the setup did not stop the worktree detached (on %q)", stop, w.Branch)
		}
	}
	return wt
}

// A stopped rebase or bisect holds ITS branch, not every branch: another local
// branch with no worktree is neither busy nor in that worktree, and starts on its
// existing branch.
func TestAStoppedRebaseOrBisectHoldsOnlyItsOwnBranch(t *testing.T) {
	t.Parallel()
	for _, stop := range []string{"rebase", "bisect"} {
		r := newTplRepo(t)
		r.stoppedWorktree("change/a", "a", stop)
		r.git(r.root, "branch", "change/other", "main")
		// TemplateBranch is what GET /lanes/branch serves.
		want := BranchFacts{Branch: "change/other", Local: true}
		if f, lerr := r.m.TemplateBranch(context.Background(), StartGate{}, "build", "other", ""); lerr != nil || f != want {
			t.Errorf("%s of change/a: TemplateBranch(other) = %+v %v, want %+v", stop, f, lerr, want)
		}
		res, lerr := r.start(StartRequest{Template: "build", Name: "other", Mode: "branch"})
		if lerr != nil {
			t.Errorf("%s of change/a: a branch start of change/other: %v", stop, lerr)
			continue
		}
		if got := r.git(res.Path, "branch", "--show-current"); got != "change/other" {
			t.Errorf("%s of change/a: the new worktree is on %q", stop, got)
		}
	}
}

// A worktree stopped mid-rebase or mid-bisect is `detached` in `git worktree list`,
// but git still holds its branch there. Both "new" and "branch" (and the D6 GET)
// name that worktree and what holds it (busy), never git's own refusal, and advise
// finishing it first: D1 refuses "existing" there until then, and says the same.
func TestABranchHeldByAStoppedRebaseOrBisectIsInItsWorktree(t *testing.T) {
	t.Parallel()
	for _, stop := range []string{"rebase", "bisect"} {
		for _, mode := range []string{"new", "branch"} {
			r := newTplRepo(t)
			wt := r.stoppedWorktree("change/add-score-photo", "score", stop)

			want := BranchFacts{Branch: "change/add-score-photo", Local: true, Worktree: wt, Busy: stop}
			if f, lerr := r.m.TemplateBranch(context.Background(), StartGate{}, "build", "add-score-photo", ""); lerr != nil || f != want {
				t.Errorf("%s: TemplateBranch = %+v %v, want %+v", stop, f, lerr, want)
			}
			sentence := "A branch named change/add-score-photo is in the middle of a " + stop + " in the worktree " + wt +
				". Finish or abort it there, then start the lane on that worktree."
			_, lerr := r.start(StartRequest{Template: "build", Name: "add-score-photo", Mode: mode})
			if lerr == nil || lerr.Status != 409 || lerr.Code != "branch-exists" || lerr.Branch == nil || *lerr.Branch != want || lerr.Msg != sentence {
				t.Errorf("%s, %s: start = %v, want 409 branch-exists %q", stop, mode, lerr, sentence)
			}
			if _, ok := r.m.Registry.Get("add-score-photo"); ok {
				t.Errorf("%s, %s: the refused start left its registry record", stop, mode)
			}
			// Following the advice before finishing: D1 still refuses, and says why
			// in the same terms rather than "start it in that branch's worktree".
			_, lerr = r.start(StartRequest{Template: "build", Name: "add-score-photo", Mode: "existing", Worktree: wt})
			if lerr == nil || lerr.Status != 400 || !strings.Contains(lerr.Msg, "in the middle of a "+stop) ||
				!strings.Contains(lerr.Msg, "Finish or abort it there") || strings.Contains(lerr.Msg, "start it in that branch's worktree") {
				t.Errorf("%s: existing on the held worktree = %v", stop, lerr)
			}
		}
	}
}

// A "branch" lane read back from disk is a valid record, not a corrupt one.
func TestRegistryKeepsABranchLane(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	reg, err := OpenRegistry(home, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	rec, err := reg.Begin(types.LaneRecord{ID: "add-score-photo", SessionID: sid, Path: "/repo/.wt/add-score-photo", Type: "build",
		Branch: "change/add-score-photo", Mode: "branch"}, "start", time.Now())
	if err == nil {
		err = reg.Done(rec)
	}
	if err != nil {
		t.Fatal(err)
	}
	again, err := OpenRegistry(home, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := again.Get("add-score-photo")
	if !ok || got.Corrupt != "" || got.Mode != "branch" || len(again.Problems()) != 0 {
		t.Fatalf("reloaded: %+v (%v), problems %v", got, ok, again.Problems())
	}
}

// A template lane is the same lane in every mode: the registry record (but its mode
// and path) and the first prompt typed do not depend on where it started.
func TestTemplateLaneIsTheSameInEveryMode(t *testing.T) {
	t.Parallel()
	type outcome struct {
		rec   types.LaneRecord
		typed string
	}
	got := map[string]outcome{}
	for _, mode := range []string{"new", "existing", "branch"} {
		r := newTplRepo(t)
		req := StartRequest{Template: "build", Name: "add-score-photo", Mode: mode}
		switch mode {
		case "existing":
			wt := filepath.Join(r.root, ".wt", "score")
			r.git(r.root, "worktree", "add", "-q", "-b", "change/add-score-photo", wt, "main")
			req.Worktree = signals.ResolvePath(wt)
		case "branch":
			r.branchWithCommit("change/add-score-photo")
		}
		if _, lerr := r.start(req); lerr != nil {
			t.Fatalf("%s: %v", mode, lerr)
		}
		r.firstPromptTypedOnceIdle("add-score-photo", "/build-change add-score-photo")
		rec, _ := r.m.Registry.Get("add-score-photo")
		if rec.Mode != mode {
			t.Fatalf("%s: recorded mode %q", mode, rec.Mode)
		}
		// What differs by mode, by definition; and what differs by time.
		rec.Mode, rec.Path, rec.SessionID, rec.Created, rec.ActionAt, rec.PromptAt = "", "", "", 0, 0, 0
		got[mode] = outcome{rec, r.tmux.typedText()[0]}
	}
	for _, mode := range []string{"existing", "branch"} {
		if got[mode] != got["new"] {
			t.Errorf("%s differs from new:\n%s %+v\nnew %+v", mode, mode, got[mode], got["new"])
		}
	}
	if got["new"].rec.Template != "build" || got["new"].rec.Branch != "change/add-score-photo" || got["new"].rec.FirstPrompt != "/build-change add-score-photo" {
		t.Errorf("the record lost the template: %+v", got["new"].rec)
	}
}
