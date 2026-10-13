package panel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/web"
)

// multiRepo is a git repository with a panel config, committed.
func multiRepo(t *testing.T, name, sock string) string {
	t.Helper()
	root := signals.ResolvePath(t.TempDir())
	gitRun(t, root, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"`+name+`","lanes":{"main":"orchestrator"},"tmux_socket":"`+sock+`"}`)
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "init")
	return root
}

// PANEL-16: one panel serves every registered project, each on its own tmux socket,
// with its own sources: the same lane name runs in two projects at once, stopping
// one leaves the other, and a project that cannot load is an item in the menu while
// the others serve.
func TestOnePanelServesSeveralProjects(t *testing.T) {
	t.Parallel()
	tmux, sockA := throwawaySocket(t)
	_, sockB := throwawaySocket(t)
	home := t.TempDir()
	rootA := multiRepo(t, "Alpha", noServerSocket()) // the panel's --project; its socket is sockA (TmuxSocket)
	rootB := multiRepo(t, "Beta", sockB)
	rootC := multiRepo(t, "Gamma", noServerSocket())
	for _, r := range []string{rootB, rootC} {
		if _, err := install.AddProject(context.Background(), install.AddOptions{Home: home, Project: r, Run: gitOnlyRunner, Now: clock.System.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	// Gamma's config breaks after it was added.
	writeFile(t, filepath.Join(rootC, config.DefaultConfigRel), `{"name":"Gamma","lanes":{"main":"orchestrator"},"no_such_key":1}`)

	p := startPanel(t, rootA, home, sockA)
	// Each project runs its own sources: count each one's polls apart.
	p.polls.more(t, "alpha/worktrees", 1)
	p.polls.more(t, "beta/worktrees", 1)
	p.polls.more(t, "beta/tmux", 1)

	var menu []web.ProjectSummary
	get(t, p, "/api/projects", &menu)
	if len(menu) != 3 || menu[0].ID != "beta" || menu[1].ID != "gamma" || menu[2].ID != "alpha" || !menu[2].Default ||
		menu[1].OK || !strings.Contains(menu[1].Error, "no_such_key") {
		t.Fatalf("menu %+v", menu)
	}
	reg, _ := config.LoadProjects(home)
	if reg.Default != "alpha" || reg.Find(rootA) == nil {
		t.Fatalf("--project registers the project and makes it the default: %+v", reg)
	}

	// The same lane name, in both projects at once.
	for _, id := range []string{"alpha", "beta"} {
		if code, body := p.post(t, "/api/p/"+id+"/lanes", lanes.StartRequest{Type: "orchestrator", Mode: "root", Name: "same"}); code != 200 {
			t.Fatalf("start %s/same: %d %v", id, code, body)
		}
	}
	if !hasSession(tmux, sockA, "same") || !hasSession(tmux, sockB, "same") {
		t.Fatal("each project's lane runs on its own socket")
	}
	for sock, id := range map[string]string{sockA: "alpha", sockB: "beta"} {
		out, _ := exec.Command(tmux, "-L", sock, "show-options", "-v", "-t", "=same:", "@clauductor_project").Output()
		if strings.TrimSpace(string(out)) != id {
			t.Fatalf("%s's lane is tagged %q", id, out)
		}
	}
	if code, _ := p.post(t, "/api/p/alpha/lanes/same/stop", nil); code != 200 {
		t.Fatalf("stop alpha/same: %d", code)
	}
	if hasSession(tmux, sockA, "same") || !hasSession(tmux, sockB, "same") {
		t.Fatal("stopping one project's lane touched the other's")
	}
	// Each project's view is its own.
	var vb struct {
		ProjectID string `json:"projectId"`
		Name      string `json:"name"`
		Terminals []struct {
			ID string `json:"id"`
		} `json:"terminals"`
	}
	waitFor(t, "beta's view to show its lane", func() bool {
		get(t, p, "/api/state?project=beta", &vb)
		return len(vb.Terminals) == 1 && vb.Terminals[0].ID == "same"
	})
	if vb.ProjectID != "beta" || vb.Name != "Beta" {
		t.Fatalf("beta's view: %+v", vb)
	}
	if code, _ := p.post(t, "/api/p/beta/lanes/same/stop", nil); code != 200 {
		t.Fatalf("stop beta/same: %d", code)
	}
}

func get(t *testing.T, p *panelRun, path string, v any) {
	t.Helper()
	req, _ := http.NewRequest("GET", p.base+path, nil)
	req.Header.Set("Cookie", p.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("GET %s: %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

// verifyProjects is a machine over two projects whose verified.json holds 2.1.287, as a
// restart restores it, with Claude Code 2.1.288 running in both.
func verifyProjects(t *testing.T, home string) (*Machine, *Runtime, *Runtime) {
	t.Helper()
	a, b := dispatchProject(t, "a", "/r/a"), dispatchProject(t, "b", "/r/b")
	a.cfg, b.cfg = v2Config(t, ""), v2Config(t, "")
	m := newMachine(Options{Home: home, Out: io.Discard}, clock.Func(func() time.Time { return t0 }), DefaultTicks(), []*Runtime{a, b}, a)
	m.each(func(md *state.Model, now time.Time) { md.ApplyClaudeVersion("2.1.288", nil, now) })
	return m, a, b
}

// threeLaunches is three Agent launches each matched by their SubagentStart, as 2.1.284
// sends them, in the project r: its session verifies the running version.
func threeLaunches(r *Runtime, cwd string) {
	for _, id := range []string{"A", "B", "C"} {
		for _, ev := range []signals.HookEvent{
			{Event: "PreToolUse", ToolName: "Agent", ToolUseID: "tu-" + id, ToolInput: []byte(`{"subagent_type":"general-purpose","description":"d"}`)},
			{Event: "SubagentStart", AgentID: id, AgentType: "general-purpose"},
			{Event: "PostToolUse", ToolName: "Agent", ToolUseID: "tu-" + id, ToolResponse: []byte(`{"agentId":"` + id + `","status":"completed"}`)},
		} {
			ev.SessionID, ev.Cwd = "s-"+r.id, cwd
			r.hub.Update(func(md *state.Model, now time.Time) { md.ApplyHook(ev, now) })
		}
	}
}

func verificationOf(r *Runtime) state.Verification { return viewOf(r).Verification }

// savedVerifiedVersion is what verified.json records ("" for no file).
func savedVerifiedVersion(t *testing.T, home string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(config.PanelDir(home), "verified.json"))
	if err != nil {
		return ""
	}
	var vf savedVerification
	if json.Unmarshal(b, &vf) != nil {
		t.Fatalf("verified.json: %s", b)
	}
	return vf.Version
}

// [VERCHECK-3-S1] The second project's verification clears the first, and is kept.
// [VERCHECK-3-S3] The first project's verification clears the second.
// [VERCHECK-3-S2] The newer verification survives a restart.
func TestVERCHECK3AVersionVerifiedAnywhereHoldsEverywhereAndIsKept(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ id, name string }{{"VERCHECK-3-S1", "the second project verifies"}, {"VERCHECK-3-S3", "the first project verifies"}} {
		t.Run(tc.id+" "+tc.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			writeFile(t, filepath.Join(config.PanelDir(home), "verified.json"), `{"version":"2.1.287","at":1}`)
			m, a, b := verifyProjects(t, home)
			for _, r := range []*Runtime{a, b} {
				if vf := verificationOf(r); vf.Auto != "2.1.287" || vf.State != state.VerifyChecking {
					t.Fatalf("%s starts on 2.1.287 and checks 2.1.288: %+v", r.id, vf)
				}
			}
			verifier, other := b, a
			if tc.id == "VERCHECK-3-S3" {
				verifier, other = a, b
			}
			threeLaunches(verifier, "/r/"+verifier.id)
			if vf := verificationOf(verifier); vf.State != state.VerifyVerified || vf.Auto != "2.1.288" {
				t.Fatalf("%s verified it itself: %+v", verifier.id, vf)
			}
			up, _ := m.saveReadings(context.Background(), t0) // the machine's saved source
			if up != nil {
				m.each(up)
			}
			if vf := verificationOf(other); vf.State != state.VerifyVerified || vf.Auto != "2.1.288" {
				t.Errorf("%s still shows the check after %s verified 2.1.288: %+v", other.id, verifier.id, vf)
			}
			if got := savedVerifiedVersion(t, home); got != "2.1.288" {
				t.Errorf("verified.json records %q, want 2.1.288", got)
			}

			// A restart: new models, the saved file, Claude Code 2.1.288 still running.
			_, a2, b2 := verifyProjects(t, home)
			for _, r := range []*Runtime{a2, b2} {
				if vf := verificationOf(r); vf.State != state.VerifyVerified {
					t.Errorf("[VERCHECK-3-S2] %s shows the check after a restart: %+v", r.id, vf)
				}
			}
		})
	}
}
