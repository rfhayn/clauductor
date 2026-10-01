package panel

import (
	"context"
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

// PANEL-20, on a throwaway socket: a new lane gets a port of its own (in its registry
// record and its tmux environment), the gitignored files .worktreeinclude names (and
// no others), and worktree_setup runs in its worktree with CLAUDUCTOR_LANE and
// CLAUDUCTOR_PORT before claude starts. Close runs worktree_teardown first; a teardown
// that leaves a file keeps the worktree.
func TestLaneLifecycleSetupPortsAndInclude(t *testing.T) {
	t.Parallel()
	tmux, sock := throwawaySocket(t)
	root := signals.ResolvePath(t.TempDir())
	home := t.TempDir()
	gitRun(t, root, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"T","version":5,"lanes":{"main":"orchestrator","fix/":"fix"},
		"base":"main","worktree_dir":".wt","ports":{"base":4400,"per_lane":10},
		"worktree_setup":{"command":["sh","-c","echo \"$CLAUDUCTOR_LANE $CLAUDUCTOR_PORT\" > .setup-ran"]},
		"worktree_teardown":{"command":["sh","-c","test \"$CLAUDUCTOR_LANE\" = dirty && echo left > leftover.txt; true"]}}`)
	writeFile(t, filepath.Join(root, ".gitignore"), ".wt/\n.env\n.cache/\n.setup-ran\nsecret.txt\n")
	writeFile(t, filepath.Join(root, ".worktreeinclude"), ".env\nconfig/*.json\n")
	writeFile(t, filepath.Join(root, "config", "tracked.json"), "{}") // tracked: never copied
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "init")
	writeFile(t, filepath.Join(root, ".env"), "TOKEN=x\n")
	writeFile(t, filepath.Join(root, "secret.txt"), "ignored but not named\n")
	p := startPanelWith(t, root, home, sock, func(o *Options) {
		o.TrustConfig = true
		o.Runner = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
			if argv[0] == "/usr/bin/env" {
				return signals.ExecRunner(ctx, dir, argv)
			}
			return gitOnlyRunner(ctx, dir, argv)
		}
	})
	var notes []string
	for _, id := range []string{"one", "dirty"} {
		code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "fix", Mode: "new", Name: id})
		if code != 200 {
			t.Fatalf("start %s: %d %v", id, code, body)
		}
		notes = append(notes, fmt.Sprint(body["lane"]))
	}
	if !strings.Contains(notes[0], "copied 1 file") || !strings.Contains(notes[0], "worktree_setup ran") {
		t.Fatalf("start notes: %v", notes[0])
	}
	wt := filepath.Join(root, ".wt", "one")
	if b, _ := os.ReadFile(filepath.Join(wt, ".env")); string(b) != "TOKEN=x\n" {
		t.Fatalf(".env not copied: %q", b)
	}
	if _, err := os.Stat(filepath.Join(wt, "secret.txt")); !os.IsNotExist(err) {
		t.Fatal("an ignored file .worktreeinclude does not name was copied")
	}
	if b, _ := os.ReadFile(filepath.Join(wt, ".setup-ran")); string(b) != "one 4400\n" {
		t.Fatalf("setup saw %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".wt", "dirty", ".setup-ran")); string(b) != "dirty 4410\n" {
		t.Fatalf("the second lane's port: %q", b)
	}
	reg, _ := lanes.OpenRegistry(home, root)
	if r, _ := reg.Get("one"); r.Port != 4400 {
		t.Fatalf("registry port %d", r.Port)
	}
	if out, _ := exec.Command(tmux, "-L", sock, "show-environment", "-t", "=one", "CLAUDUCTOR_PORT").Output(); strings.TrimSpace(string(out)) != "CLAUDUCTOR_PORT=4400" {
		t.Fatalf("tmux environment: %q", out)
	}
	// Close: teardown runs, the clean worktree goes.
	code, body := p.post(t, "/api/lanes/one/close", map[string]any{"dryRun": true})
	if code != 200 || !strings.Contains(fmt.Sprint(body["plan"]), "worktree_teardown") {
		t.Fatalf("plan: %d %v", code, body)
	}
	if code, body := p.post(t, "/api/lanes/one/close", map[string]any{"worktree": true, "branch": true}); code != 200 ||
		!strings.Contains(fmt.Sprint(body["result"]), "worktree_teardown ran") {
		t.Fatalf("close: %d %v", code, body)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatal("close kept a clean worktree after its teardown")
	}
	// A teardown that leaves a file keeps the worktree, and says why.
	code, body = p.post(t, "/api/lanes/dirty/close", map[string]any{"worktree": true, "branch": true})
	if code != 200 || !strings.Contains(fmt.Sprint(body["result"]), "after worktree_teardown") {
		t.Fatalf("close dirty: %d %v", code, body)
	}
	if _, err := os.Stat(filepath.Join(root, ".wt", "dirty", "leftover.txt")); err != nil {
		t.Fatal("the worktree with the teardown's file was removed")
	}
}
