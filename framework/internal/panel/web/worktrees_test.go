package web

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// PANEL-18: POST /api/p/{project}/worktrees/remove is guarded as every POST is (the
// cookie, this page's Origin), takes a strict body, and acts only on a worktree named
// by an exact entry of that project's `git worktree list`: any other path is refused
// before a command runs in it.
func TestRemoveWorktreeRouteIsGuarded(t *testing.T) {
	t.Parallel()
	s, _ := twoProjects(t)
	var mu sync.Mutex
	var ran [][]string
	lm := s.Projects[1].Lanes
	lm.Run = func(_ context.Context, dir string, argv []string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		ran = append(ran, append([]string{dir}, argv...))
		if len(argv) > 2 && argv[1] == "worktree" && argv[2] == "list" {
			return []byte("worktree /repo\nHEAD 1111111111111111111111111111111111111111\nbranch refs/heads/main\n\n" +
				"worktree /repo/.wt/x\nHEAD 2222222222222222222222222222222222222222\ndetached\n"), nil
		}
		return nil, nil
	}
	origin := withHeader("Origin", "http://127.0.0.1:4393")
	const route = "/api/p/b/worktrees/remove"
	for _, c := range []struct {
		name, method, target, body string
		opts                       reqOptList
		status                     int
	}{
		{"without cookie", "POST", route, `{"worktree":"/repo/.wt/x","dryRun":true}`, reqOptList{origin}, 401},
		{"without an Origin", "POST", route, `{"worktree":"/repo/.wt/x","dryRun":true}`, reqOptList{withCookie(s)}, 403},
		{"cross-site", "POST", route, `{"worktree":"/repo/.wt/x","dryRun":true}`, reqOptList{withCookie(s), withHeader("Origin", "http://evil.example")}, 403},
		{"another port", "POST", route, `{"worktree":"/repo/.wt/x","dryRun":true}`, reqOptList{withCookie(s), withHeader("Origin", "http://127.0.0.1:3000")}, 403},
		{"by GET", "GET", route, ``, reqOptList{withCookie(s)}, 404},
		{"unknown project", "POST", "/api/p/nope/worktrees/remove", `{"worktree":"/repo/.wt/x","dryRun":true}`, reqOptList{withCookie(s), origin}, 404},
		{"no project", "POST", "/api/worktrees/remove", `{"worktree":"/repo/.wt/x","dryRun":true}`, reqOptList{withCookie(s), origin}, 404},
		{"unknown field", "POST", route, `{"worktree":"/repo/.wt/x","force":true}`, reqOptList{withCookie(s), origin}, 400},
		{"a command", "POST", route, `{"worktree":"/repo/.wt/x","command":"rm -rf /"}`, reqOptList{withCookie(s), origin}, 400},
		{"without a body", "POST", route, ``, reqOptList{withCookie(s), origin}, 400},
		{"no worktree", "POST", route, `{"dryRun":true}`, reqOptList{withCookie(s), origin}, 400},
		{"a relative path", "POST", route, `{"worktree":".wt/x","dryRun":true}`, reqOptList{withCookie(s), origin}, 400},
		{"a path not listed", "POST", route, `{"worktree":"/etc","remove":true}`, reqOptList{withCookie(s), origin}, 404},
		{"a path inside one", "POST", route, `{"worktree":"/repo/.wt/x/sub","remove":true}`, reqOptList{withCookie(s), origin}, 404},
		{"a path spelled otherwise", "POST", route, `{"worktree":"/repo/.wt/../.wt/x","remove":true}`, reqOptList{withCookie(s), origin}, 404},
	} {
		if w := do(s, c.method, c.target, c.body, c.opts...); w.Code != c.status {
			t.Errorf("%s: got %d %q, want %d", c.name, w.Code, strings.TrimSpace(w.Body.String()), c.status)
		}
	}
	mu.Lock()
	for _, argv := range ran {
		if len(argv) < 3 || argv[0] != "/repo" || argv[1] != "git" || argv[2] != "worktree" || argv[3] != "list" {
			t.Errorf("a refused request ran %v", argv)
		}
	}
	ran = nil
	mu.Unlock()

	// The main worktree is listed, so it plans, and the plan keeps it.
	w := do(s, "POST", route, `{"worktree":"/repo","dryRun":true}`, withCookie(s), origin)
	var body struct {
		OK   bool `json:"ok"`
		Plan struct {
			Worktree bool     `json:"worktree"`
			Keep     []string `json:"keep"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 || !body.OK || body.Plan.Worktree || !strings.Contains(strings.Join(body.Plan.Keep, "\n"), "main worktree") {
		t.Fatalf("main worktree plan: %d %s", w.Code, w.Body.String())
	}
	w = do(s, "POST", route, `{"worktree":"/repo","remove":true,"branch":true}`, withCookie(s), origin)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "main worktree") {
		t.Fatalf("main worktree remove: %d %s", w.Code, w.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	for _, argv := range ran {
		if strings.Join(argv[1:3], " ") == "git worktree" && argv[3] == "remove" {
			t.Fatalf("the main worktree was removed: %v", argv)
		}
	}
}

// The page: Remove on a lane-less worktree asks with the server's plan first (a dry
// run), and sends back only what that plan offered.
func TestRemoveWorktreeIsWired(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	for _, want := range []string{
		`fetch(api("/worktrees/remove"), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ worktree: path, dryRun: true }) })`,
		`body: JSON.stringify({ worktree: path, remove: !!plan.worktree, branch: !!plan.deleteBranch })`,
		`button("Remove", "small danger", () => askRemove(w.path), "Remove this worktree when that loses nothing. Asks first, listing what goes and what stays.", "wtrm:" + w.path, true)`,
		`lineList("Removes", p.remove, "rm"), lineList("Keeps", p.keep, "kp")`,
		`button("Confirm remove", "danger", () => doRemove(a.path, p), null, "wt:confirm", true)`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
	// doRemove is called only from the confirm button.
	if n := strings.Count(js, "doRemove("); n != 2 {
		t.Errorf("doRemove appears %d times, want 2 (its definition and the confirm button)", n)
	}
}
