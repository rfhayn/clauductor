package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeRunner stands in for git, claude and gh so Run can be exercised end to end.
func fakeRunner(root string) Runner {
	return func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		switch strings.Join(argv, " ") {
		case "git worktree list --porcelain":
			return []byte(fmt.Sprintf("worktree %s\nHEAD abc\nbranch refs/heads/main\n\n", root)), nil
		case "git rev-parse --git-common-dir":
			return []byte(".git\n"), nil
		case "claude agents --json":
			return []byte(fmt.Sprintf(`[{"pid":1,"cwd":%q,"kind":"interactive","sessionId":"s1","name":"lane-1","status":"busy"}]`, root)), nil
		case "gh pr list --json number,title,headRefName,author,isDraft,statusCheckRollup":
			return []byte(`[]`), nil
		case "echo card":
			return []byte("- [ ] an item\n"), nil
		}
		return nil, fmt.Errorf("unexpected command %v", argv)
	}
}

func setupProject(t *testing.T) (root, home string) {
	t.Helper()
	root = ResolvePath(t.TempDir())
	home = t.TempDir()
	writeFile(t, filepath.Join(root, DefaultConfigRel), `{"name":"Test","lanes":{"main":"orchestrator"},
		"cards":[{"id":"c","title":"Card","command":["echo","card"],"refresh":"interval:60"}]}`)
	return root, home
}

type liveClient struct {
	base, cookie string
}

func (c liveClient) state(t *testing.T) View {
	t.Helper()
	req, _ := http.NewRequest("GET", c.base+"/api/state", nil)
	req.Header.Set("Cookie", c.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var v View
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func (c liveClient) post(t *testing.T, path, body string) int {
	t.Helper()
	resp, err := http.Post(c.base+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestRunEndToEnd(t *testing.T) {
	root, home := setupProject(t)
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Project: root, Port: 0, NoOpen: true, Home: home,
			Runner: fakeRunner(root), OnReady: func(u string) { ready <- u }})
	}()
	var launch string
	select {
	case launch = <-ready:
	case err := <-done:
		t.Fatalf("Run exited: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("never ready")
	}
	u, _ := url.Parse(launch)
	port, _ := strconv.Atoi(u.Port())
	c := liveClient{base: "http://" + u.Host, cookie: fmt.Sprintf("clauductor_panel_%d=%s", port, u.Query().Get("t"))}

	marker, err := os.ReadFile(MarkerPath(home))
	if err != nil || strings.TrimSpace(string(marker)) != strconv.Itoa(port) {
		t.Fatalf("marker %q %v", marker, err)
	}
	for _, ev := range HookEvents {
		if ourHooks(t, home)[ev] != 1 {
			t.Fatalf("hook for %s not installed", ev)
		}
	}
	settings, _ := os.ReadFile(SettingsPath(home))
	if !strings.Contains(string(settings), HookURL(port)) {
		t.Fatal("installed hook does not point at the bound port")
	}

	waitFor(t, "agents lane and card", func() bool {
		v := c.state(t)
		return len(v.Lanes) == 1 && len(v.Lanes[0].Sessions) == 1 && v.Lanes[0].Status == "busy" &&
			v.Cards[0].Source.OK && v.Sources["prs"].OK
	})
	if code := c.post(t, "/hook", fmt.Sprintf(`{"session_id":"s1","cwd":%q,"hook_event_name":"SubagentStart","agent_id":"a1","agent_type":"builder"}`, root)); code != 204 {
		t.Fatalf("hook post: %d", code)
	}
	if code := c.post(t, "/hook", `{"session_id":"s9","cwd":"/elsewhere","hook_event_name":"Stop"}`); code != 204 {
		t.Fatalf("foreign hook post: %d", code)
	}
	waitFor(t, "hook applied", func() bool {
		v := c.state(t)
		return v.HookEvents == 1 && v.Dropped == 1 && len(v.Lanes) == 1 && len(v.Lanes[0].Subagents) == 1
	})

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	if _, err := os.Stat(MarkerPath(home)); !os.IsNotExist(err) {
		t.Fatal("marker left behind after shutdown")
	}
	if ourHooks(t, home)["Stop"] != 1 {
		t.Fatal("hooks should stay installed after shutdown")
	}
}

func TestRunRefusesATakenPortWithoutSideEffects(t *testing.T) {
	root, home := setupProject(t)
	busy, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port
	err = Run(context.Background(), Options{Project: root, Port: port, NoOpen: true, Home: home, Runner: fakeRunner(root)})
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("want a port-in-use error, got %v", err)
	}
	if _, err := os.Stat(SettingsPath(home)); !os.IsNotExist(err) {
		t.Fatal("a refused launch touched settings.json")
	}
	if _, err := os.Stat(MarkerPath(home)); !os.IsNotExist(err) {
		t.Fatal("a refused launch wrote the marker")
	}
}

func TestRunNeedsAConfig(t *testing.T) {
	root := ResolvePath(t.TempDir())
	err := Run(context.Background(), Options{Project: root, Port: 0, NoOpen: true, Home: t.TempDir(), Runner: fakeRunner(root)})
	if err == nil || !strings.Contains(err.Error(), "no panel config") {
		t.Fatalf("got %v", err)
	}
}
