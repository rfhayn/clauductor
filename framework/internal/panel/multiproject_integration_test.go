package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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

// attachedRepo is a committed git repository attached as id under home: its sidecar,
// ~/.clauductor/projects/<id>/panel.json, is written and registered, and the
// repository has no .clauductor/panel.json unless own names one.
func attachedRepo(t *testing.T, home, id, own string) (root, sidecar string) {
	t.Helper()
	root = signals.ResolvePath(t.TempDir())
	gitRun(t, root, "init", "-q", "-b", "main")
	if own != "" {
		writeFile(t, filepath.Join(root, config.DefaultConfigRel), own)
		gitRun(t, root, "add", ".")
	}
	gitRun(t, root, "commit", "-q", "--allow-empty", "-m", "init")
	res, err := install.Attach(context.Background(), install.AttachOptions{Home: home, Project: root, ID: id, Run: gitOnlyRunner, Now: clock.System.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return root, res.Init.Path
}

// [ATTACH-4-S3] An attached project with no .clauductor/panel.json in its repository:
// the panel started with --project <repo> and no --config starts, and serves the
// project from its sidecar.
func TestPanelServesAnAttachedProjectWithoutConfigFlag(t *testing.T) {
	t.Parallel()
	_, sock := throwawaySocket(t)
	home := signals.ResolvePath(t.TempDir())
	root, sidecar := attachedRepo(t, home, "acme", "")
	if _, err := os.Stat(filepath.Join(root, config.DefaultConfigRel)); err == nil {
		t.Fatal("the repository has a config of its own")
	}
	cfg, err := config.LoadConfig(sidecar)
	if err != nil {
		t.Fatal(err)
	}

	p := startPanel(t, root, home, sock) // Options.Project, and no ConfigPath
	var v struct {
		ProjectID string           `json:"projectId"`
		Name      string           `json:"name"`
		Trust     config.TrustView `json:"trust"`
	}
	get(t, p, "/api/state", &v)
	if v.ProjectID != "acme" || v.Name != cfg.Name || v.Trust.Path != sidecar {
		t.Fatalf("served %+v; want acme, %q, from %s", v, cfg.Name, sidecar)
	}
}

// [ATTACH-4-S1] [ATTACH-4-S4] An attached project whose repository also has its own
// .clauductor/panel.json: the panel serves the sidecar's name, cards and templates;
// `panel list` names the sidecar as outside the repository and says the repository's
// file is not read; and the project menu's entry carries the sidecar's path, while a
// project reading its own config carries none.
func TestAttachedProjectReadsItsSidecar(t *testing.T) {
	t.Parallel()
	_, sockA := throwawaySocket(t)
	home := signals.ResolvePath(t.TempDir())
	rootA := multiRepo(t, "Alpha", noServerSocket())
	rootB, sidecar := attachedRepo(t, home, "side", `{"name":"Own file","lanes":{"main":"orchestrator"},
		"cards":[{"id":"own","title":"Own card","command":["echo","own"],"refresh":"interval:60"}]}`)
	// The owner edits the sidecar (design.md D2), and trusts it.
	writeFile(t, sidecar, `{"name":"Sidecar","lanes":{"main":"orchestrator","change/":"build"},"tmux_socket":"`+noServerSocket()+`",
		"cards":[{"id":"sc","title":"Sidecar card","command":["echo","side"],"refresh":"interval:60"}],
		"templates":[{"id":"st","title":"Sidecar template","lane_type":"build","branch_pattern":"change/{name}","first_prompt":"hello"}]}`)
	if _, err := install.TrustConfig(home, rootB, sidecar); err != nil {
		t.Fatal(err)
	}
	cr := &cardRunner{runs: map[string]int{}}
	p := startPanelWith(t, rootA, home, sockA, func(o *Options) { o.Runner = cr.run })

	var v struct {
		Name  string `json:"name"`
		Cards []struct {
			ID string `json:"id"`
		} `json:"cards"`
		Templates []config.TemplateInfo `json:"templates"`
	}
	get(t, p, "/api/state?project=side", &v)
	if v.Name != "Sidecar" || len(v.Cards) != 1 || v.Cards[0].ID != "sc" || len(v.Templates) != 1 || v.Templates[0].ID != "st" {
		t.Fatalf("side serves %+v, not its sidecar", v)
	}
	waitFor(t, "the sidecar's card to run", func() bool { return cr.count("side") > 0 })
	if cr.count("own") != 0 {
		t.Fatal("the repository's own card ran")
	}

	var out strings.Builder
	if err := install.ListProjects(&out, home); err != nil {
		t.Fatal(err)
	}
	if want := "config " + sidecar + " (outside the repository)\n    " + filepath.Join(rootB, config.DefaultConfigRel) + " is not read"; !strings.Contains(out.String(), want) {
		t.Errorf("panel list lacks %q:\n%s", want, out.String())
	}

	m := menu(t, p)
	if m["side"].Config != sidecar || m["alpha"].Config != "" || !m["alpha"].OK {
		t.Fatalf("menu: side %+v, alpha %+v", m["side"], m["alpha"])
	}
	// The rows the page renders from those entries.
	if got, want := pageWords(t, "menuLine", m["side"]), "reads "+sidecar+", outside the repository"; got != want {
		t.Errorf("side's row says %q, want %q", got, want)
	}
	if got := pageWords(t, "menuLine", m["alpha"]); got != "" {
		t.Errorf("alpha's row says %q about its config", got)
	}
}

// shRunner runs sh for real (the test's card and suggest scripts) and is
// gitOnlyRunner otherwise: the panel's own runner, ExecRunner, sees each command.
func shRunner(ctx context.Context, dir string, argv []string) ([]byte, error) {
	if argv[0] == "sh" {
		return signals.ExecRunner(ctx, dir, argv)
	}
	return gitOnlyRunner(ctx, dir, argv)
}

// writeScripts writes the test's repository scripts under root/.claude: one line to
// each stream, a JSON card, and suggestions with a line of noise on stderr.
func writeScripts(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, ".claude", "both.sh"), "echo out-line\necho err-line >&2\n")
	writeFile(t, filepath.Join(root, ".claude", "json.sh"), "echo '{\"a\":1}'\necho err-line >&2\n")
	writeFile(t, filepath.Join(root, ".claude", "suggest.sh"), "echo 'alpha First thing'\necho 'debug noise' >&2\n")
}

// [ATTACH-6-S1] An attached project, and a project with its own config, each with a
// card running a repository script that writes one line to stdout and one to stderr
// and exits 0: the attached project's card shows both lines, the other's the stdout
// line alone; the attached project's suggest command, which writes a line to stderr
// too, still parses to its suggestions alone; and its card whose stdout is JSON is
// still a JSON card.
func TestSidecarCardShowsItsStderr(t *testing.T) {
	t.Parallel()
	_, sockA := throwawaySocket(t)
	home := signals.ResolvePath(t.TempDir())
	rootA := signals.ResolvePath(t.TempDir())
	gitRun(t, rootA, "init", "-q", "-b", "main")
	writeScripts(t, rootA)
	cfgA := filepath.Join(rootA, config.DefaultConfigRel)
	writeFile(t, cfgA, `{"name":"Own","lanes":{"main":"orchestrator"},"tmux_socket":"`+noServerSocket()+`",
		"cards":[{"id":"both","title":"Both","command":["sh",".claude/both.sh"],"refresh":"interval:60"}]}`)
	gitRun(t, rootA, "add", ".")
	gitRun(t, rootA, "commit", "-q", "-m", "init")
	if _, err := install.TrustConfig(home, rootA, cfgA); err != nil {
		t.Fatal(err)
	}
	rootB, sidecar := attachedRepo(t, home, "side", "")
	writeScripts(t, rootB)
	writeFile(t, sidecar, `{"name":"Side","lanes":{"main":"orchestrator","change/":"build"},"tmux_socket":"`+noServerSocket()+`",
		"cards":[{"id":"both","title":"Both","command":["sh",".claude/both.sh"],"refresh":"interval:60"},
		         {"id":"json","title":"JSON","command":["sh",".claude/json.sh"],"refresh":"interval:60"}],
		"templates":[{"id":"st","title":"T","lane_type":"build","branch_pattern":"change/{name}","first_prompt":"hello",
		              "suggest":{"command":["sh",".claude/suggest.sh"],"refresh":"interval:60"}}]}`)
	if _, err := install.TrustConfig(home, rootB, sidecar); err != nil {
		t.Fatal(err)
	}
	p := startPanelWith(t, rootA, home, sockA, func(o *Options) { o.Runner = shRunner })

	type view struct {
		Cards       []state.CardState            `json:"cards"`
		Suggestions map[string]state.SuggestView `json:"suggestions"`
	}
	cards := func(v view) map[string]*signals.CardOutput {
		out := map[string]*signals.CardOutput{}
		for _, c := range v.Cards {
			if c.Output != nil {
				out[c.ID] = c.Output
			}
		}
		return out
	}
	var a, b view
	waitFor(t, "every card and the suggestions to run", func() bool {
		a, b = view{}, view{}
		get(t, p, "/api/state?project=own", &a)
		get(t, p, "/api/state?project=side", &b)
		return len(cards(a)) == 1 && len(cards(b)) == 2 && b.Suggestions["st"].Source.OK
	})
	if got := cards(b)["both"]; got.Kind != "text" || strings.Join(got.Lines, "|") != "out-line|err-line" {
		t.Errorf("the sidecar's card shows %+v, want its stdout line, then its stderr line", got)
	}
	if got := cards(a)["both"]; got.Kind != "text" || strings.Join(got.Lines, "|") != "out-line" {
		t.Errorf("the in-repository card shows %+v, want its stdout line alone", got)
	}
	if got := cards(b)["json"]; got.Kind != "json" || string(got.JSON) != `{"a":1}` || len(got.Lines) != 0 {
		t.Errorf("the sidecar's JSON card is %+v", got)
	}
	if sg := b.Suggestions["st"]; len(sg.Items) != 1 || sg.Items[0].Name != "alpha" || sg.Skipped != 0 {
		t.Errorf("the sidecar's suggestions: %+v", sg)
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
