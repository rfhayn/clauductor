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
// while ~/.clauductor/panel/pid names a live process (a positive integer), and
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
	for _, tc := range []struct {
		name  string
		pid   string // "" writes no pid file
		posts bool
	}{
		{"live panel", strconv.Itoa(os.Getpid()), true},
		{"killed panel (stale files)", strconv.Itoa(dead.Process.Pid), false},
		{"no pid file", "", false},
		// kill -0 -1 signals every process the user owns, and kill -0 0 the process
		// group: both succeed, so a pid must be digits only, and not 0, before kill -0.
		{"pid file says -1", "-1", false},
		{"pid file says 0", "0", false},
		{"pid file is not a number", "12 34", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			dir := filepath.Join(home, ".clauductor", "panel")
			os.MkdirAll(dir, 0o700)
			os.WriteFile(filepath.Join(dir, "port"), []byte(u.Port()), 0o600)
			if tc.pid != "" {
				os.WriteFile(filepath.Join(dir, "pid"), []byte(tc.pid+"\n"), 0o600)
			}
			cmd := exec.Command("bash", "-c", snippet)
			cmd.Env = append(os.Environ(), "HOME="+home)
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
				if p != "POST /status "+input {
					t.Fatalf("posted %q", p)
				}
			case <-time.After(2 * time.Second):
				if tc.posts {
					t.Fatal("did not post to the live panel")
				}
			}
		})
	}
}
