package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/lanes"
)

// fakeAdmin records what reached it.
type fakeAdmin struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeAdmin) note(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakeAdmin) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeAdmin) Validate(_ context.Context, p string) (any, *lanes.LaneError) {
	f.note("validate " + p)
	return map[string]string{"root": p}, nil
}
func (f *fakeAdmin) InitPreview(_ context.Context, p string) (any, *lanes.LaneError) {
	f.note("preview " + p)
	return nil, nil
}
func (f *fakeAdmin) Init(_ context.Context, p string) (any, *lanes.LaneError) {
	f.note("init " + p)
	return nil, nil
}
func (f *fakeAdmin) Add(_ context.Context, r AddProjectRequest) (any, *lanes.LaneError) {
	f.note("add " + r.Path)
	return nil, nil
}
func (f *fakeAdmin) Remove(_ context.Context, id string, dry bool) (any, *lanes.LaneError) {
	f.note("remove " + id)
	return nil, nil
}
func (f *fakeAdmin) Trust(_ context.Context, id string, dry bool, h string) (any, *lanes.LaneError) {
	f.note("trust " + id)
	return nil, nil
}

const goodHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// PANEL-22: every route that adds, trusts or removes a project passes every guard a
// lane action does (the Host allow-list, the cookie, this page's Origin), takes only
// a strict body, and only then reaches the registry. A refused request reaches
// nothing.
func TestProjectAdminRoutesAreGuarded(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	adm := &fakeAdmin{}
	s.Admin = adm
	origin := withHeader("Origin", "http://127.0.0.1:4393")
	routes := []struct{ path, body string }{
		{"/api/projects/validate", `{"path":"/r"}`},
		{"/api/projects/init-preview", `{"path":"/r"}`},
		{"/api/projects/init", `{"path":"/r"}`},
		{"/api/projects/add", `{"path":"/r","trust":true,"hash":"` + goodHash + `"}`},
		{"/api/projects/beta/remove", `{"dryRun":true}`},
		{"/api/projects/beta/trust", `{"dryRun":false,"hash":"` + goodHash + `"}`},
	}
	for _, rt := range routes {
		for _, c := range []struct {
			name string
			opts []reqOpt
			want int
		}{
			{"no cookie", []reqOpt{origin}, 401},
			{"no Origin", []reqOpt{withCookie(s)}, 403},
			{"another port's Origin", []reqOpt{withCookie(s), withHeader("Origin", "http://127.0.0.1:3000")}, 403},
			{"a cross-site Origin", []reqOpt{withCookie(s), withHeader("Origin", "http://evil.example")}, 403},
			{"a foreign Host", []reqOpt{withCookie(s), withHost("evil.localhost:4393"), withHeader("Origin", "http://evil.localhost:4393")}, 403},
			{"a remote peer", []reqOpt{withCookie(s), origin, withRemote("10.0.0.2:5000")}, 403},
			{"an unknown field", []reqOpt{withCookie(s), origin}, 400},
		} {
			body := rt.body
			if c.name == "an unknown field" {
				body = strings.TrimSuffix(body, "}") + `,"command":["sh"]}`
			}
			if w := do(s, "POST", rt.path, body, c.opts...); w.Code != c.want {
				t.Errorf("POST %s with %s: %d, want %d", rt.path, c.name, w.Code, c.want)
			}
		}
		if w := do(s, "GET", rt.path, "", withCookie(s)); w.Code != 405 && w.Code != 404 {
			t.Errorf("GET %s: %d", rt.path, w.Code)
		}
	}
	if got := adm.got(); len(got) != 0 {
		t.Fatalf("refused requests reached the registry: %v", got)
	}
	// Trusting names the bytes: no hash, or one that is not a SHA-256, is refused.
	for _, b := range []struct{ path, body string }{
		{"/api/projects/add", `{"path":"/r","trust":true}`},
		{"/api/projects/add", `{"path":"/r","trust":true,"hash":"abc"}`},
		{"/api/projects/beta/trust", `{"hash":""}`},
		{"/api/projects/beta/trust", `{"hash":"` + strings.ToUpper(goodHash) + `"}`},
		{"/api/projects/Bad!/remove", `{}`},
	} {
		if w := do(s, "POST", b.path, b.body, withCookie(s), origin); w.Code != 400 {
			t.Errorf("POST %s %s: %d, want 400", b.path, b.body, w.Code)
		}
	}
	if got := adm.got(); len(got) != 0 {
		t.Fatalf("refused requests reached the registry: %v", got)
	}
	// Each guarded request that passes reaches it once.
	for _, rt := range routes {
		if w := do(s, "POST", rt.path, rt.body, withCookie(s), origin); w.Code != 200 {
			t.Errorf("POST %s: %d %s", rt.path, w.Code, w.Body)
		}
	}
	want := []string{"validate /r", "preview /r", "init /r", "add /r", "remove beta", "trust beta"}
	if got := adm.got(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("calls %v, want %v", got, want)
	}
	// Without a registry, the routes are not there.
	s2, _ := newTestServer(t)
	if w := do(s2, "POST", "/api/projects/validate", `{"path":"/r"}`, withCookie(s2), origin); w.Code != 404 {
		t.Fatalf("no admin: %d", w.Code)
	}
}

// PANEL-22: a project removed live stops being served: its routes are 404, its open
// event streams end, and a late push from its hub does not bring it back to the menu.
func TestRemovedProjectIsGone(t *testing.T) {
	t.Parallel()
	s, _ := twoProjects(t)
	s.Summaries = NewSummaries([]ProjectSummary{{ID: "a", Name: "A", OK: true, Default: true}, {ID: "b", Name: "B", OK: true}})
	origin := withHeader("Origin", "http://127.0.0.1:4393")
	b := s.project("b")
	done := make(chan int, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		r := httptest.NewRequest("GET", "/events?project=b", nil).WithContext(ctx)
		r.Host, r.RemoteAddr = "127.0.0.1:4393", "127.0.0.1:50000"
		withCookie(s)(r)
		s.Handler().ServeHTTP(httptest.NewRecorder(), r)
		done <- 1
	}()
	time.Sleep(50 * time.Millisecond)
	s.RemoveProject("b")
	s.Summaries.Remove("b")
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("b's event stream did not end when b was removed")
	}
	if w := do(s, "GET", "/api/state?project=b", "", withCookie(s)); w.Code != 404 {
		t.Fatalf("state of a removed project: %d", w.Code)
	}
	if w := do(s, "POST", "/api/p/b/refresh", "", withCookie(s), origin); w.Code != 404 {
		t.Fatalf("refresh of a removed project: %d", w.Code)
	}
	s.Summaries.Set(Summarize("b", b.Hub.View()))
	if strings.Contains(string(s.Summaries.JSON()), `"id":"b"`) {
		t.Fatalf("a late push brought b back: %s", s.Summaries.JSON())
	}
	s.SetDefault("a")
	s.Summaries.SetDefault("a")
	if !strings.Contains(string(s.Summaries.JSON()), `"default":true`) {
		t.Fatal("no default in the menu")
	}
}
