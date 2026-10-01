package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/web"
)

// cardRunner is gitOnlyRunner that also runs "echo" (the test's card command) and
// counts each run by its first argument: a card's command reaches it only through the
// trust gate.
type cardRunner struct {
	mu   sync.Mutex
	runs map[string]int
}

func (c *cardRunner) run(ctx context.Context, dir string, argv []string) ([]byte, error) {
	if argv[0] == "echo" {
		c.mu.Lock()
		c.runs[argv[1]]++
		c.mu.Unlock()
		return []byte(argv[1] + "\n"), nil
	}
	return gitOnlyRunner(ctx, dir, argv)
}

func (c *cardRunner) count(id string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runs[id]
}

// menu reads the project menu.
func menu(t *testing.T, p *panelRun) map[string]web.ProjectSummary {
	t.Helper()
	var list []web.ProjectSummary
	get(t, p, "/api/projects", &list)
	out := map[string]web.ProjectSummary{}
	for _, s := range list {
		out[s.ID] = s
	}
	return out
}

// status is a GET's status code.
func status(t *testing.T, p *panelRun, path string) int {
	t.Helper()
	req, _ := http.NewRequest("GET", p.base+path, nil)
	req.Header.Set("Cookie", p.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// result is the "result" object of an admin answer, re-decoded into v.
func result(t *testing.T, body map[string]any, v any) {
	t.Helper()
	b, _ := json.Marshal(body["result"])
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

// PANEL-22: a project added from the page is served at once, with no restart, and runs
// none of its config's commands until its exact bytes are trusted; removed, it stops
// being served, every source of its runtime stops, and its lanes keep running. Removing
// is refused while it has lanes.
func TestProjectAddedTrustedAndRemovedLive(t *testing.T) {
	t.Parallel()
	tmux, sockA := throwawaySocket(t)
	_, sockB := throwawaySocket(t)
	home := t.TempDir()
	rootA := multiRepo(t, "Alpha", noServerSocket())
	rootB := signals.ResolvePath(t.TempDir())
	gitRun(t, rootB, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(rootB, config.DefaultConfigRel), `{"name":"Beta","lanes":{"main":"orchestrator"},"tmux_socket":"`+sockB+`",
		"host_names":["beta.localhost"],
		"cards":[{"id":"bc","title":"Beta card","command":["echo","beta-card"],"refresh":"interval:1"}]}`)
	gitRun(t, rootB, "add", ".")
	gitRun(t, rootB, "commit", "-q", "-m", "init")
	cr := &cardRunner{runs: map[string]int{}}
	p := startPanelWith(t, rootA, home, sockA, func(o *Options) { o.Runner = cr.run })

	// Validate: it says what adding would do, and runs nothing.
	code, body := p.post(t, "/api/projects/validate", map[string]string{"path": rootB})
	var cand install.Candidate
	result(t, body, &cand)
	if code != 200 || cand.Root != rootB || !cand.HasConfig || cand.ID != "beta" || cand.Trusted || len(cand.Runs) != 1 || len(cand.Hash) != 64 {
		t.Fatalf("validate: %d %+v", code, body)
	}
	// Refusals: a linked worktree, a registered root, a relative path, not a repository.
	linked := filepath.Join(t.TempDir(), "wt")
	gitRun(t, rootA, "worktree", "add", "-q", "-b", "x", linked)
	// A refusal is the answer to the question asked: 200, with why.
	for path, want := range map[string]string{linked: "linked-worktree", rootA: "registered", "rel/x": "not-absolute", t.TempDir(): "not-git"} {
		code, body := p.post(t, "/api/projects/validate", map[string]string{"path": path})
		var v validation
		result(t, body, &v)
		if code != 200 || v.Refused == nil || v.Refused.Code != want || v.Refused.Error == "" {
			t.Errorf("validate %s: %d %v, want refused %s", path, code, body, want)
		}
	}
	// Adding a refused path is refused (422), whatever the page did.
	if code, body := p.post(t, "/api/projects/add", map[string]any{"path": linked}); code != 422 || body["code"] != "linked-worktree" {
		t.Errorf("add a linked worktree: %d %v", code, body)
	}

	// Add without trusting: served at once, its card does not run.
	if code, body := p.post(t, "/api/projects/add", map[string]any{"path": rootB}); code != 200 {
		t.Fatalf("add: %d %v", code, body)
	} else if body["result"].(map[string]any)["live"] != true || body["result"].(map[string]any)["trusted"] != false {
		t.Fatalf("add: %v", body)
	}
	if m := menu(t, p); !m["beta"].OK || m["beta"].Trusted || !m["alpha"].Default {
		t.Fatalf("menu after add: %+v", m)
	}
	if code := status(t, p, "/api/state?project=beta"); code != 200 {
		t.Fatalf("beta's state: %d", code)
	}
	p.polls.until(t, "beta/card bc", 1) // its first poll: where an untrusted card would run
	if n := cr.count("beta-card"); n != 0 {
		t.Fatalf("an untrusted card ran %d times", n)
	}
	// Its Host name is not allowed while it is untrusted.
	hostOK := func() int {
		req, _ := http.NewRequest("GET", p.base+"/healthz", nil)
		req.Host = "beta.localhost:" + strings.TrimPrefix(p.base[strings.LastIndex(p.base, ":"):], ":")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if hostOK() != 403 {
		t.Fatal("an untrusted config added a Host name")
	}

	// Trust: the report names the hash; a stale hash is refused; the right one turns
	// the card on without a restart.
	code, body = p.post(t, "/api/projects/beta/trust", map[string]any{"dryRun": true})
	var rep trustReport
	result(t, body, &rep)
	if code != 200 || rep.Hash != cand.Hash || rep.Trusted || len(rep.Runs) != 1 {
		t.Fatalf("trust report: %d %v", code, body)
	}
	if code, body := p.post(t, "/api/projects/beta/trust", map[string]any{"hash": strings.Repeat("0", 64)}); code != 409 || body["code"] != "config-changed" {
		t.Fatalf("a stale hash: %d %v", code, body)
	}
	if code, body := p.post(t, "/api/projects/beta/trust", map[string]any{"hash": rep.Hash}); code != 200 {
		t.Fatalf("trust: %d %v", code, body)
	}
	waitFor(t, "beta's card to run once trusted", func() bool { return cr.count("beta-card") > 0 })
	waitFor(t, "the menu to say beta is trusted", func() bool { return menu(t, p)["beta"].Trusted })
	if hostOK() != 200 {
		t.Fatal("a trusted project's Host name is not allowed")
	}

	// A lane in beta: removing is refused, and the plan names the lane.
	if code, body := p.post(t, "/api/p/beta/lanes", lanes.StartRequest{Type: "orchestrator", Mode: "root", Name: "bl"}); code != 200 {
		t.Fatalf("start beta/bl: %d %v", code, body)
	}
	code, body = p.post(t, "/api/projects/beta/remove", map[string]any{"dryRun": true})
	var plan removePlan
	result(t, body, &plan)
	if code != 200 || plan.Refused == "" || len(plan.Lanes) != 1 || plan.Lanes[0].ID != "bl" || plan.Default {
		t.Fatalf("plan with a lane: %d %v", code, body)
	}
	if code, _ := p.post(t, "/api/projects/beta/remove", map[string]any{}); code != 409 {
		t.Fatalf("remove with a lane: %d", code)
	}
	if code, _ := p.post(t, "/api/p/beta/lanes/bl/stop", nil); code != 200 {
		t.Fatalf("stop beta/bl: %d", code)
	}

	// Removed: not served, not in the menu or projects.json, and its sources stop.
	waitFor(t, "beta's lane to go from its view", func() bool {
		_, body := p.post(t, "/api/projects/beta/remove", map[string]any{"dryRun": true})
		var pl removePlan
		result(t, body, &pl)
		return pl.Refused == ""
	})
	if code, body := p.post(t, "/api/projects/beta/remove", map[string]any{}); code != 200 {
		t.Fatalf("remove: %d %v", code, body)
	}
	if code := status(t, p, "/api/state?project=beta"); code != 404 {
		t.Fatalf("beta's state after remove: %d", code)
	}
	if _, ok := menu(t, p)["beta"]; ok {
		t.Fatal("beta is still in the menu")
	}
	if reg, _ := config.LoadProjects(home); reg.Find("beta") != nil {
		t.Fatal("beta is still in projects.json")
	}
	stopped := p.polls.count("beta/worktrees") + p.polls.count("beta/card bc")
	p.polls.more(t, "alpha/worktrees", 2) // a second and more of alpha's polls
	if now := p.polls.count("beta/worktrees") + p.polls.count("beta/card bc"); now != stopped {
		t.Fatalf("beta's sources polled %d more times after it was removed", now-stopped)
	}
	if hostOK() != 403 {
		t.Fatal("a removed project's Host name is still allowed")
	}
	// Nothing on disk went: its config and its repository are where they were.
	if _, err := os.Stat(filepath.Join(rootB, config.DefaultConfigRel)); err != nil {
		t.Fatal("removing deleted the config")
	}
	_ = tmux
}

// PANEL-22: `clauductor panel add` and `panel remove` while a panel runs take effect
// live: the panel follows projects.json, and its owner record says so.
func TestCLIAddAndRemoveAreLive(t *testing.T) {
	t.Parallel()
	_, sockA := throwawaySocket(t)
	home := t.TempDir()
	rootA := multiRepo(t, "Alpha", noServerSocket())
	rootC := multiRepo(t, "Gamma", noServerSocket())
	p := startPanel(t, rootA, home, sockA)
	ctx := context.Background()
	if _, err := install.AddProject(ctx, install.AddOptions{Home: home, Project: rootC, Run: gitOnlyRunner, Now: clock.System.Now(), Strict: true}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the panel to serve gamma", func() bool { return menu(t, p)["gamma"].OK })
	pid := os.Getpid()
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if !install.WaitPanelServes(wctx, clock.System, home, pid, rootC, true) {
		t.Fatal("owner.json does not list gamma")
	}
	// Make gamma the default from the CLI side, then remove alpha: the default moves.
	reg, _ := config.LoadProjects(home)
	reg.Default = "gamma"
	if err := config.SaveProjects(home, reg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "gamma to be the default", func() bool { return menu(t, p)["gamma"].Default })
	var v struct {
		ProjectID string `json:"projectId"`
	}
	get(t, p, "/api/state", &v)
	if v.ProjectID != "gamma" {
		t.Fatalf("a page with no project opens on %q", v.ProjectID)
	}
	if _, err := install.RemoveProject(home, "gamma", false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the panel to stop serving gamma", func() bool { _, ok := menu(t, p)["gamma"]; return !ok })
	if !install.WaitPanelServes(wctx, clock.System, home, pid, rootC, false) {
		t.Fatal("owner.json still lists gamma")
	}
	if m := menu(t, p); !m["alpha"].Default {
		t.Fatalf("alpha is not the default again: %+v", m)
	}
	// The last project is never stopped: the panel always serves one.
	if code, body := p.post(t, "/api/projects/alpha/remove", map[string]any{"dryRun": true}); code != 200 ||
		!strings.Contains(body["result"].(map[string]any)["refused"].(string), "only project") {
		t.Fatalf("removing the last project: %d %v", code, body)
	}
}

// PANEL-22: Trust config… on a file edited since the panel loaded it trusts the bytes on
// disk (the report's) and serves them: the project starts again with that config.
func TestTrustAnEditedConfigReloadsTheProject(t *testing.T) {
	t.Parallel()
	_, sockA := throwawaySocket(t)
	home := t.TempDir()
	rootA := multiRepo(t, "Alpha", noServerSocket())
	rootB := multiRepo(t, "Beta", noServerSocket())
	cr := &cardRunner{runs: map[string]int{}}
	if _, err := install.AddProject(context.Background(), install.AddOptions{Home: home, Project: rootB, Run: gitOnlyRunner, Now: clock.System.Now(), Strict: true}); err != nil {
		t.Fatal(err)
	}
	p := startPanelWith(t, rootA, home, sockA, func(o *Options) { o.Runner = cr.run })
	if m := menu(t, p); !m["beta"].OK || m["beta"].Trusted {
		t.Fatalf("menu: %+v", m)
	}
	// Edited while served: a card appears in the file the panel did not load.
	cfg := filepath.Join(rootB, config.DefaultConfigRel)
	writeFile(t, cfg, `{"name":"Beta","lanes":{"main":"orchestrator"},"tmux_socket":"`+noServerSocket()+`",
		"cards":[{"id":"nc","title":"New card","command":["echo","new-card"],"refresh":"interval:60"}]}`)
	_, body := p.post(t, "/api/projects/beta/trust", map[string]any{"dryRun": true})
	var rep trustReport
	result(t, body, &rep)
	if len(rep.Runs) != 1 || !strings.Contains(rep.Runs[0], "new-card") {
		t.Fatalf("the report is not the file's: %+v", rep)
	}
	code, body := p.post(t, "/api/projects/beta/trust", map[string]any{"hash": rep.Hash})
	result(t, body, &rep)
	if code != 200 || !rep.Reloaded {
		t.Fatalf("trust: %d %v", code, body)
	}
	waitFor(t, "the new card to run", func() bool { return cr.count("new-card") > 0 })
	if m := menu(t, p); !m["beta"].Trusted || !m["alpha"].Default {
		t.Fatalf("menu after the reload: %+v", m)
	}
}

// PANEL-22: the init preview writes nothing; Create this config writes it once and
// never over a file; the default project removed hands the default to the next.
func TestInitFromThePageAndDefaultHandOver(t *testing.T) {
	t.Parallel()
	_, sockA := throwawaySocket(t)
	home := t.TempDir()
	rootA := multiRepo(t, "Alpha", noServerSocket())
	rootD := signals.ResolvePath(t.TempDir())
	gitRun(t, rootD, "init", "-q", "-b", "main")
	gitRun(t, rootD, "commit", "-q", "--allow-empty", "-m", "init")
	p := startPanel(t, rootA, home, sockA)
	cfgPath := filepath.Join(rootD, config.DefaultConfigRel)

	code, body := p.post(t, "/api/projects/init-preview", map[string]string{"path": rootD})
	if code != 200 || !strings.Contains(body["result"].(map[string]any)["body"].(string), `"version"`) {
		t.Fatalf("preview: %d %v", code, body)
	}
	if _, err := os.Stat(cfgPath); err == nil {
		t.Fatal("the preview wrote the config")
	}
	if code, body := p.post(t, "/api/projects/add", map[string]any{"path": rootD}); code != 409 || body["code"] != "no-config" {
		t.Fatalf("add with no config: %d %v", code, body)
	}
	if code, body := p.post(t, "/api/projects/init", map[string]string{"path": rootD}); code != 200 {
		t.Fatalf("init: %d %v", code, body)
	}
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatal("init wrote no config")
	}
	if code, body := p.post(t, "/api/projects/init", map[string]string{"path": rootD}); code != 409 {
		t.Fatalf("a second init: %d %v", code, body)
	}
	// Its socket would be clauductor-<id>: give it a throwaway one, so the panel never
	// asks a real socket name.
	written, _ := os.ReadFile(cfgPath)
	writeFile(t, cfgPath, strings.Replace(string(written), "{\n", "{\n  \"tmux_socket\": \""+noServerSocket()+"\",\n", 1))
	_, body = p.post(t, "/api/projects/validate", map[string]string{"path": rootD})
	var cand install.Candidate
	result(t, body, &cand)
	if code, body := p.post(t, "/api/projects/add", map[string]any{"path": rootD, "trust": true, "hash": cand.Hash}); code != 200 ||
		body["result"].(map[string]any)["trusted"] != true {
		t.Fatalf("trust and add: %d %v", code, body)
	}
	id := cand.ID
	// The default (alpha) removed: the plan names the next default, and it is.
	_, body = p.post(t, "/api/projects/alpha/remove", map[string]any{"dryRun": true})
	var plan removePlan
	result(t, body, &plan)
	if !plan.Default || plan.NextDefault != id || plan.Refused != "" {
		t.Fatalf("plan for the default: %+v", plan)
	}
	if code, body := p.post(t, "/api/projects/alpha/remove", map[string]any{}); code != 200 {
		t.Fatalf("remove alpha: %d %v", code, body)
	}
	if m := menu(t, p); !m[id].Default || len(m) != 1 {
		t.Fatalf("menu after the default went: %+v", m)
	}
	var v struct {
		ProjectID string `json:"projectId"`
	}
	get(t, p, "/api/state", &v)
	if v.ProjectID != id {
		t.Fatalf("a page with no project opens on %q, want %q", v.ProjectID, id)
	}
}
