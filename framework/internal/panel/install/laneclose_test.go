package install

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/lanes"
)

// serveUnix serves h on a fresh Unix socket (a short path: sun_path is small) until the
// test ends; its path.
func serveUnix(t *testing.T, h http.Handler) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "lc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return sock
}

// A socket nothing listens on reads as not answering; a request the panel received and
// did not answer before the command gave up does not: by D4 a close it had begun runs
// to its end, so the error must not suggest nothing happened.
func TestCloseLaneTellsNoAnswerFromAnAnswerCutOff(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "none.sock")
	_, err := CloseLane(context.Background(), missing, "p", "a", lanes.CloseRequest{DryRun: true})
	if err == nil || !strings.Contains(err.Error(), "does not answer") {
		t.Errorf("no socket: %v, want it not answering", err)
	}

	got := make(chan struct{})
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(got)
		<-r.Context().Done() // the panel at work, past the command's patience
	}))
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-got; cancel() }()
	_, err = CloseLane(ctx, sock, "p", "a", lanes.CloseRequest{Worktree: true, Branch: true})
	if err == nil || strings.Contains(err.Error(), "does not answer") || !strings.Contains(err.Error(), "did not arrive") ||
		!strings.Contains(err.Error(), "finishes") {
		t.Errorf("a request cut off after it was sent: %v, want the answer not arriving and the close finished", err)
	}
}

// A redirect (ServeMux's 301 for a path it cleans, an id of "..") is not followed: it
// comes back as the refusal, and nothing is sent again.
func TestCloseLaneFollowsNoRedirect(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var seen []string
	sock := serveUnix(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		mu.Unlock()
		http.Redirect(w, r, "/api/p/p/close", http.StatusMovedPermanently)
	}))
	_, err := CloseLane(context.Background(), sock, "p", "..", lanes.CloseRequest{Worktree: true, Branch: true})
	var le *lanes.LaneError
	if !errors.As(err, &le) || le.Status != http.StatusMovedPermanently {
		t.Errorf("a redirect: %v, want it refused with its 301", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || !strings.HasPrefix(seen[0], "POST ") {
		t.Errorf("requests %q, want the one POST", seen)
	}
}
