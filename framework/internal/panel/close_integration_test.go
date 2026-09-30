package panel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
)

// PANEL-17: Close lane over HTTP on a throwaway socket. A running lane is stopped
// first, exactly as Stop does (here: Escape, since claude agents does not list it),
// and only then is its worktree checked and removed; an orphan's close is Forget and
// the same cleanup. The plan the confirmation lists comes first, from real state.
func TestCloseLaneStopsThenRemovesTheWorktree(t *testing.T) {
	t.Parallel()
	tmux, sock := throwawaySocket(t)
	root := signals.ResolvePath(t.TempDir())
	home := t.TempDir()
	gitRun(t, root, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"T","lanes":{"main":"orchestrator","fix/":"fix"},
		"base":"main","worktree_dir":".wt"}`)
	writeFile(t, filepath.Join(root, ".gitignore"), ".wt/\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "init")
	keys := filepath.Join(t.TempDir(), "keys")
	p := startPanelWith(t, root, home, sock, func(o *Options) {
		// Records what the lane receives: Stop's Escape must arrive before the removal.
		o.LaneProgram = []string{"/bin/sh", "-c", "stty raw -echo; exec cat >> " + shq(keys), "lane"}
	})

	for _, id := range []string{"run", "orph"} {
		if code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "fix", Mode: "new", Name: id}); code != 200 {
			t.Fatalf("start %s: %d %v", id, code, body)
		}
	}
	waitFor(t, "the lane to be recording its keys", func() bool { _, err := os.Stat(keys); return err == nil })
	exec.Command(tmux, "-L", sock, "kill-session", "-t", "=orph").Run() // orphan it

	// The plan: everything goes (the branches have no commits main lacks).
	code, body := p.post(t, "/api/lanes/run/close", map[string]any{"dryRun": true})
	plan, _ := body["plan"].(map[string]any)
	if code != 200 || plan["worktree"] != true || plan["deleteBranch"] != true || plan["running"] != true ||
		!strings.Contains(fmt.Sprint(plan["remove"]), "ended as Stop lane ends them") {
		t.Fatalf("plan: %d %v", code, body)
	}
	if !hasSession(tmux, sock, "run") {
		t.Fatal("a dry run stopped the lane")
	}
	code, body = p.post(t, "/api/lanes/run/close", map[string]any{"worktree": true, "branch": true})
	if code != 200 {
		t.Fatalf("close: %d %v", code, body)
	}
	if b, _ := os.ReadFile(keys); !strings.Contains(string(b), "\x1b") || strings.ContainsAny(string(b), "\r\n") {
		t.Fatalf("the lane received %q; want Stop's Escape and never an Enter", b)
	}
	if hasSession(tmux, sock, "run") {
		t.Fatal("close left the lane running")
	}
	wt := filepath.Join(root, ".wt", "run")
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("close kept a clean worktree: %v", body)
	}
	if exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/fix/run").Run() == nil {
		t.Fatalf("close kept a merged branch: %v", body)
	}

	// The orphan: its record goes, and with an untracked file its worktree stays.
	writeFile(t, filepath.Join(root, ".wt", "orph", "notes.md"), "mine")
	code, body = p.post(t, "/api/p/"+projectIDOf(t, p)+"/lanes/orph/close", map[string]any{"dryRun": true})
	plan, _ = body["plan"].(map[string]any)
	if code != 200 || plan["running"] != false || plan["worktree"] != false || !strings.Contains(fmt.Sprint(plan["keep"]), "notes.md") {
		t.Fatalf("orphan plan: %d %v", code, body)
	}
	code, body = p.post(t, "/api/p/"+projectIDOf(t, p)+"/lanes/orph/close", map[string]any{"worktree": false, "branch": false})
	if code != 200 || !strings.Contains(fmt.Sprint(body["result"]), "registry record") {
		t.Fatalf("orphan close: %d %v", code, body)
	}
	if r, _ := lanes.OpenRegistry(home, root); len(r.List()) != 0 {
		t.Fatalf("records left: %v", r.List())
	}
	if _, err := os.Stat(filepath.Join(root, ".wt", "orph", "notes.md")); err != nil {
		t.Fatal("close removed a worktree with an untracked file")
	}
	if code, _ := p.post(t, "/api/lanes/orph/close", map[string]any{"dryRun": true}); code != 404 {
		t.Fatalf("plan for a closed lane: %d, want 404", code)
	}
}

// projectIDOf is the id the panel gave its (one) project.
func projectIDOf(t *testing.T, p *panelRun) string {
	t.Helper()
	id := p.state(t).ProjectID
	if id == "" {
		t.Fatal("no project id in the state")
	}
	return id
}
