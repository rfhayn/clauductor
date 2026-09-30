package web

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/leakcheck"
	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/state"
)

// twoProjects is a server for projects "a" (the default) and "b", each with its own
// hub and lane manager; refreshed counts each project's refreshes.
func twoProjects(t *testing.T) (*Server, map[string]*atomic.Int64) {
	t.Helper()
	s, _ := newTestServer(t)
	refreshed := map[string]*atomic.Int64{}
	for _, id := range []string{"a", "b"} {
		cfg := testConfig(t)
		cfg.Name = "Project " + strings.ToUpper(id)
		m := state.NewModel(cfg, "/repo-"+id, t0)
		lm := testLaneManager(t)
		lm.Socket = leakcheck.NoServerSocket()
		n := &atomic.Int64{}
		refreshed[id] = n
		s.Projects = append(s.Projects, &Project{ID: id, Name: cfg.Name, Hub: NewHub(m, clock.Func(time.Now)), Lanes: lm,
			Refresh: func() { n.Add(1) }})
	}
	s.Default = "a"
	return s, refreshed
}

// PANEL-16: every project-scoped route is guarded as the routes before it were: the
// cookie, and for a POST this page's Origin. A project the panel does not serve is
// 404; the routes without a project reach the default one.
func TestProjectRoutesAreGuarded(t *testing.T) {
	t.Parallel()
	s, refreshed := twoProjects(t)
	origin := withHeader("Origin", "http://127.0.0.1:4393")
	posts := []string{"/lanes", "/lanes/lane-x/ticket", "/lanes/lane-x/stop", "/lanes/lane-x/close", "/lanes/lane-x/image", "/lanes/restore-all",
		"/queues/gate/cancel", "/queues/gate/run", "/refresh"}
	for _, p := range posts {
		for _, pre := range []string{"/api/p/b", "/api"} {
			if w := do(s, "POST", pre+p, "{}", origin); w.Code != 401 {
				t.Errorf("POST %s%s without the cookie: %d", pre, p, w.Code)
			}
			if w := do(s, "POST", pre+p, "{}", withCookie(s)); w.Code != 403 {
				t.Errorf("POST %s%s without an Origin: %d", pre, p, w.Code)
			}
			if w := do(s, "POST", pre+p, "{}", withCookie(s), withHeader("Origin", "http://127.0.0.1:3000")); w.Code != 403 {
				t.Errorf("POST %s%s from another port: %d", pre, p, w.Code)
			}
		}
		if w := do(s, "POST", "/api/p/nope"+p, "{}", withCookie(s), origin); w.Code != 404 {
			t.Errorf("POST /api/p/nope%s: %d, want 404", p, w.Code)
		}
		if w := do(s, "POST", "/api/p/Bad!"+p, "{}", withCookie(s), origin); w.Code != 400 && w.Code != 404 {
			t.Errorf("POST /api/p/Bad!%s: %d", p, w.Code)
		}
	}
	for _, g := range []string{"/api/state?project=b", "/api/projects", "/events?project=b"} {
		if w := do(s, "GET", g, ""); w.Code != 401 {
			t.Errorf("GET %s without the cookie: %d", g, w.Code)
		}
	}
	if w := do(s, "GET", "/api/state?project=nope", "", withCookie(s)); w.Code != 404 {
		t.Errorf("state of an unknown project: %d", w.Code)
	}
	name := func(target string) string {
		var v state.View
		_ = json.Unmarshal(do(s, "GET", target, "", withCookie(s)).Body.Bytes(), &v)
		return v.Name
	}
	if name("/api/state") != "Project A" || name("/api/state?project=b") != "Project B" {
		t.Fatalf("state: default %q, b %q", name("/api/state"), name("/api/state?project=b"))
	}
	do(s, "POST", "/api/refresh", "", withCookie(s), origin)
	do(s, "POST", "/api/p/b/refresh", "", withCookie(s), origin)
	if refreshed["a"].Load() != 1 || refreshed["b"].Load() != 1 {
		t.Fatalf("refreshes: a %d, b %d; the legacy route is the default project's", refreshed["a"].Load(), refreshed["b"].Load())
	}
}

// A terminal ticket is for one project's lane: a ticket for a/x never opens b/x,
// the lane of the same name in another project.
func TestTicketIsForOneProjectsLane(t *testing.T) {
	t.Parallel()
	s, _ := twoProjects(t)
	origin := withHeader("Origin", "http://127.0.0.1:4393")
	w := do(s, "POST", "/api/p/a/lanes/lane-x/ticket", "", withCookie(s), origin)
	var res struct{ Ticket string }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &res) != nil || res.Ticket == "" {
		t.Fatalf("ticket: %d %s", w.Code, w.Body)
	}
	w = do(s, "GET", "/ws/term?project=b&lane=lane-x", "", withCookie(s), origin, withHeader("Sec-WebSocket-Protocol", TermSubprotocol+", "+ticketPrefix+res.Ticket))
	if w.Code != 401 {
		t.Fatalf("a's ticket on b's lane: %d %s", w.Code, w.Body)
	}
	// It was single-use, and spent by the refusal.
	w = do(s, "GET", "/ws/term?project=a&lane=lane-x", "", withCookie(s), origin, withHeader("Sec-WebSocket-Protocol", TermSubprotocol+", "+ticketPrefix+res.Ticket))
	if w.Code != 401 {
		t.Fatalf("a spent ticket: %d", w.Code)
	}
	// A fresh ticket for a/x passes the ticket check (and stops at the missing lane).
	w = do(s, "POST", "/api/p/a/lanes/lane-x/ticket", "", withCookie(s), origin)
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	w = do(s, "GET", "/ws/term?project=a&lane=lane-x", "", withCookie(s), origin, withHeader("Sec-WebSocket-Protocol", TermSubprotocol+", "+ticketPrefix+res.Ticket))
	if w.Code != 404 {
		t.Fatalf("a's ticket on a's lane: %d %s, want past the ticket to 404 (no such lane)", w.Code, w.Body)
	}
	// Focus and closing are per project too.
	if got := s.FocusedLanesIn("b"); len(got) != 0 {
		t.Fatalf("focus in b: %v", got)
	}
}

// The projects' menu is pushed only when what it says changes: a hub's routine push
// with the same counts costs a page nothing.
func TestSummariesPushOnlyOnChange(t *testing.T) {
	t.Parallel()
	m := state.NewModel(testConfig(t), "/repo", t0)
	m.SetProjectID("a")
	h := NewHub(m, clock.Func(func() time.Time { return t0 }))
	sums := NewSummaries([]ProjectSummary{Summarize("a", h.View()), {ID: "b", Name: "B", Error: "no panel config"}})
	h.OnPush = func(v state.View) { sums.Set(Summarize("a", v)) }
	ch, stop := sums.subscribe()
	defer stop()
	h.broadcast(false) // the first push: the same summary as the initial one
	h.broadcast(true)  // a tick with nothing new: no push
	if sums.Pushes() != 0 {
		t.Fatalf("pushed %d times with nothing new", sums.Pushes())
	}
	h.Update(func(m *state.Model, now time.Time) { m.ApplyTrust(config.TrustView{Trusted: true}) })
	h.broadcast(false)
	h.broadcast(false)
	if sums.Pushes() != 1 {
		t.Fatalf("pushed %d times for one change", sums.Pushes())
	}
	var list []ProjectSummary
	if err := json.Unmarshal(<-ch, &list); err != nil || len(list) != 2 || !list[0].Trusted || list[1].OK || list[1].Error == "" {
		t.Fatalf("menu %+v %v", list, err)
	}
}
