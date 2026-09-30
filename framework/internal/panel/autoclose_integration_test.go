package panel

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// PANEL-20, on a throwaway socket: with lanes_auto_close "on_merge", a lane whose
// pull request merged is closed by the panel when Close's own plan removes its
// worktree and branch and claude is gone (here: an orphaned lane); a lane whose claude
// is not idle asks in Needs you instead, "PR merged: close lane?", and stays. Each is
// notified once.
func TestAutoCloseOnMerge(t *testing.T) {
	t.Parallel()
	tmux, sock := throwawaySocket(t)
	root := signals.ResolvePath(t.TempDir())
	home := t.TempDir()
	gitRun(t, root, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"T","version":5,"lanes":{"main":"orchestrator","fix/":"fix"},
		"base":"main","worktree_dir":".wt"}`)
	writeFile(t, filepath.Join(root, ".gitignore"), ".wt/\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "init")
	var mu sync.Mutex
	var notices []state.Notice
	p := startPanelWith(t, root, home, sock, func(o *Options) {
		o.TrustConfig = true
		o.Ticks.PRs = 100 * time.Millisecond
		o.Notify = func(n state.Notice) error { mu.Lock(); notices = append(notices, n); mu.Unlock(); return nil }
		o.Runner = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
			if strings.Join(argv, " ") == "gh pr list --head fix/done --state merged --json number --limit 5" ||
				strings.Join(argv, " ") == "gh pr list --head fix/busy --state merged --json number --limit 5" {
				return []byte(`[{"number":3}]`), nil
			}
			return gitOnlyRunner(ctx, dir, argv)
		}
	})
	for _, id := range []string{"done", "busy"} {
		if code, body := p.post(t, "/api/lanes", lanes.StartRequest{Type: "fix", Mode: "new", Name: id}); code != 200 {
			t.Fatalf("start %s: %d %v", id, code, body)
		}
	}
	// The config turns the mode on only now: rewrite it and restart would be heavy, so
	// the lanes exist first and the panel is started with the mode below.
	p.stop()
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"T","version":5,"lanes":{"main":"orchestrator","fix/":"fix"},
		"base":"main","worktree_dir":".wt","lanes_auto_close":"on_merge"}`)
	exec.Command(tmux, "-L", sock, "kill-session", "-t", "=done").Run() // gone: nothing to interrupt
	p = startPanelWith(t, root, home, sock, func(o *Options) {
		o.TrustConfig = true
		o.Ticks.PRs = 100 * time.Millisecond
		o.Notify = func(n state.Notice) error { mu.Lock(); notices = append(notices, n); mu.Unlock(); return nil }
		o.Runner = func(ctx context.Context, dir string, argv []string) ([]byte, error) {
			if j := strings.Join(argv, " "); strings.HasPrefix(j, "gh pr list --head fix/") && strings.HasSuffix(j, "--state merged --json number --limit 5") {
				return []byte(`[{"number":3}]`), nil
			}
			return gitOnlyRunner(ctx, dir, argv)
		}
	})
	waitUntil(t, "the orphaned lane closed, its worktree and merged branch removed", 20*time.Second, func() bool {
		_, err := os.Stat(filepath.Join(root, ".wt", "done"))
		return os.IsNotExist(err) && exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/fix/done").Run() != nil
	})
	waitUntil(t, "the busy lane asks", 20*time.Second, func() bool {
		for _, n := range p.state(t).NeedsYou {
			if n.Kind == "merged" && n.Terminal == "busy" && strings.Contains(n.Text, "#3 merged") && strings.Contains(n.Text, "claude is") {
				return true
			}
		}
		return false
	})
	if !hasSession(tmux, sock, "busy") {
		t.Fatal("the lane that asks was closed")
	}
	fed := false
	for _, e := range p.state(t).Feed {
		fed = fed || (e.Event == "Lane closed" && strings.Contains(e.Detail, "PR #3 merged"))
	}
	if !fed {
		t.Fatal("the close is not in the activity feed")
	}
	p.polls.more(t, "autoclose", 3)
	mu.Lock()
	defer mu.Unlock()
	titles := []string{}
	for _, n := range notices {
		titles = append(titles, n.Title)
	}
	if len(notices) != 2 || !strings.Contains(strings.Join(titles, "|"), "Lane done closed") || !strings.Contains(strings.Join(titles, "|"), "PR merged: close lane busy?") {
		t.Fatalf("notifications %q (want one each)", titles)
	}
}
