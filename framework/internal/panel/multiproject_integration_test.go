package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
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
