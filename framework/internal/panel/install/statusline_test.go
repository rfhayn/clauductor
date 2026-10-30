package install

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The status-line snippet in docs/panel.md is what projects paste into their
// status-line script. The test runs THAT text: it posts to the recorded port only
// while ~/.stridal/panel/pid names a live process (a positive integer), and
// otherwise stays silent.
func TestStatusLineSnippetPostsOnlyToALivePanel(t *testing.T) {
	for _, tool := range []string{"bash", "curl"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "panel.md"))
	if err != nil {
		t.Fatal(err)
	}
	const begin, end = "<!-- statusline begin -->\n```bash\n", "```\n<!-- statusline end -->"
	s := string(b)
	i, j := strings.Index(s, begin), strings.Index(s, end)
	if i < 0 || j < i {
		t.Fatal("docs/panel.md has no status-line snippet between its markers")
	}
	snippet := s[i+len(begin) : j]

	// Only a request carrying this run's marker is the snippet's. Parallel tests in this
	// package probe loopback ports they have closed (singleton, open: GET /healthz), and
	// the kernel can hand one of those ports to this server; a stray probe is not a post.
	marker := "statusline-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	got := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), marker) {
			got <- r.Method + " " + r.URL.Path + " " + string(body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)

	dead := exec.Command("/usr/bin/true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	// The test sets the session's window itself: a lane runs it under its own.
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CLAUDE_CODE_AUTO_COMPACT_WINDOW=") {
			env = append(env, e)
		}
	}
	_, noJQ := exec.LookPath("jq")
	live := strconv.Itoa(os.Getpid())
	for _, tc := range []struct {
		name   string
		pid    string // "" writes no pid file
		posts  bool
		window string // CLAUDE_CODE_AUTO_COMPACT_WINDOW; "" leaves it unset
		adds   string // what the post adds to the input's fields
	}{
		{name: "live panel", pid: live, posts: true},
		// PANEL-48 (D7): the session's window, which Claude Code's stdin lacks, joins the
		// post when it is digits, and never otherwise.
		{name: "live panel, a 200000 window", pid: live, posts: true, window: "200000", adds: `,"auto_compact_window":200000`},
		{name: "live panel, a window that is not a number", pid: live, posts: true, window: "200k"},
		{name: "killed panel (stale files)", pid: strconv.Itoa(dead.Process.Pid)},
		{name: "no pid file"},
		// kill -0 -1 signals every process the user owns, and kill -0 0 the process
		// group: both succeed, so a pid must be digits only, and not 0, before kill -0.
		{name: "pid file says -1", pid: "-1"},
		{name: "pid file says 0", pid: "0"},
		{name: "pid file is not a number", pid: "12 34"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.adds != "" && noJQ != nil {
				t.Skip("no jq")
			}
			home := t.TempDir()
			dir := filepath.Join(home, ".stridal", "panel")
			os.MkdirAll(dir, 0o700)
			os.WriteFile(filepath.Join(dir, "port"), []byte(u.Port()), 0o600)
			if tc.pid != "" {
				os.WriteFile(filepath.Join(dir, "pid"), []byte(tc.pid+"\n"), 0o600)
			}
			cmd := exec.Command("bash", "-c", snippet)
			cmd.Env = append(env[:len(env):len(env)], "HOME="+home)
			if tc.window != "" {
				cmd.Env = append(cmd.Env, "CLAUDE_CODE_AUTO_COMPACT_WINDOW="+tc.window)
			}
			input := `{"session_id":"` + marker + `-` + strings.ReplaceAll(tc.name, " ", "_") + `"}`
			cmd.Stdin = strings.NewReader(input)
			if out, err := cmd.CombinedOutput(); err != nil || len(out) != 0 {
				t.Fatalf("the snippet must exit 0 and print nothing: %v %q", err, out)
			}
			select {
			case p := <-got:
				if !tc.posts {
					t.Fatalf("posted to a panel that is not running: %s", p)
				}
				if want := "POST /status " + strings.TrimSuffix(input, "}") + tc.adds + "}"; p != want {
					t.Fatalf("posted %q, want %q", p, want)
				}
			case <-time.After(2 * time.Second):
				if tc.posts {
					t.Fatal("did not post to the live panel")
				}
			}
		})
	}
}
