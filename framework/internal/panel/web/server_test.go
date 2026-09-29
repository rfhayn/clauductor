package web

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/state"
)

const testPort = 4393

func newTestServer(t *testing.T) (*Server, chan []byte) {
	t.Helper()
	hooks := make(chan []byte, 8)
	m := state.NewModel(testConfig(t), "/repo", t0)
	m.ApplyWorktrees(fixtureWorktrees(t), nil, t0)
	return &Server{Clock: clock.System, Port: testPort, Token: "secret-token", Hub: NewHub(m, clock.Func(time.Now)), Hooks: hooks, Status: make(chan []byte, 8)}, hooks
}

type reqOpt func(*http.Request)

func withCookie(s *Server) reqOpt {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.Token}) }
}
func withHeader(k, v string) reqOpt { return func(r *http.Request) { r.Header.Set(k, v) } }
func withHost(h string) reqOpt      { return func(r *http.Request) { r.Host = h } }
func withRemote(a string) reqOpt    { return func(r *http.Request) { r.RemoteAddr = a } }

func do(s *Server, method, target, body string, opts ...reqOpt) *httptest.ResponseRecorder {
	// A bounded context: if a guard regressed, /events would stream forever and the
	// test would hang instead of failing.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r := httptest.NewRequest(method, target, strings.NewReader(body)).WithContext(ctx)
	r.Host = "127.0.0.1:" + strconv.Itoa(testPort)
	r.RemoteAddr = "127.0.0.1:50000"
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestListenBindsLoopbackOnly(t *testing.T) {
	t.Parallel()
	ln, err := listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	a := ln.Addr().(*net.TCPAddr)
	if a.IP.String() != LoopbackHost {
		t.Fatalf("bound %v, want %s", a, LoopbackHost)
	}
	if ensureLoopback(&net.TCPAddr{IP: net.IPv4zero, Port: 4393}) == nil {
		t.Fatal("0.0.0.0 accepted as loopback")
	}
	if ensureLoopback(&net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 4393}) == nil {
		t.Fatal("LAN address accepted as loopback")
	}
}

func TestListenRefusesATakenPort(t *testing.T) {
	t.Parallel()
	busy, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	port := busy.Addr().(*net.TCPAddr).Port
	ln, err := listen(port)
	if err == nil {
		ln.Close()
		t.Fatalf("bound a taken port (got %v)", ln.Addr())
	}
	if !strings.Contains(err.Error(), "already in use") || !strings.Contains(err.Error(), strconv.Itoa(port)) {
		t.Fatalf("unclear error: %v", err)
	}
}

func TestBrowserRoutesNeedTheToken(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	tests := []struct {
		name   string
		method string
		target string
		opts   []reqOpt
		want   int
	}{
		{"page without cookie", "GET", "/", nil, 401},
		{"events without cookie", "GET", "/api/state", nil, 401},
		{"state without cookie", "GET", "/events", nil, 401},
		{"refresh without cookie", "POST", "/api/refresh", []reqOpt{withHeader("Origin", "http://127.0.0.1:4393")}, 401},
		{"wrong token", "GET", "/?t=nope", nil, 401},
		{"wrong cookie", "GET", "/", []reqOpt{func(r *http.Request) { r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: "nope"}) }}, 401},
		{"page with cookie", "GET", "/", []reqOpt{withCookie(s)}, 200},
		{"state with cookie", "GET", "/api/state", []reqOpt{withCookie(s)}, 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if w := do(s, tc.method, tc.target, "", tc.opts...); w.Code != tc.want {
				t.Fatalf("got %d want %d", w.Code, tc.want)
			}
		})
	}
}

func TestTokenExchangeSetsAStrictCookie(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	w := do(s, "GET", "/?t="+s.Token, "")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/" {
		t.Fatalf("got %d → %q", w.Code, w.Header().Get("Location"))
	}
	c := w.Result().Cookies()
	if len(c) != 1 || c[0].Value != s.Token || !c[0].HttpOnly || c[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie %+v", c)
	}
	page := do(s, "GET", "/", "", withCookie(s))
	if !strings.Contains(page.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") || page.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("security headers missing")
	}
}

func TestForeignHostRefused(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	// [::1]:4393 is allowed since v2 (clauductor.localhost resolves to ::1 first);
	// hosts_test.go covers the full list.
	for _, host := range []string{"evil.example:4393", "127.0.0.1:4394", "localhost", "[::2]:4393", "127.0.0.1.nip.io:4393", "evil.localhost:4393"} {
		for _, target := range []string{"/", "/hook", "/events"} {
			method := "GET"
			if target == "/hook" {
				method = "POST"
			}
			if w := do(s, method, target, "{}", withCookie(s), withHost(host)); w.Code != http.StatusForbidden {
				t.Errorf("%s %s with Host %s: got %d", method, target, host, w.Code)
			}
		}
	}
	if w := do(s, "GET", "/", "", withCookie(s), withHost("localhost:4393")); w.Code != 200 {
		t.Fatalf("localhost:port refused: %d", w.Code)
	}
}

func TestStateChangingRequestsNeedOurOrigin(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	tests := []struct {
		origin string
		want   int
	}{
		{"http://evil.example", 403},
		{"", 403},
		{"null", 403},
		{"http://127.0.0.1:4394", 403},
		{"http://127.0.0.1:4393", 204},
		// v2: the Origin must equal "http://" + THIS request's Host (127.0.0.1:4393
		// here), so the other loopback spelling is cross-host now.
		{"http://localhost:4393", 403},
	}
	for _, tc := range tests {
		opts := []reqOpt{withCookie(s)}
		if tc.origin != "" {
			opts = append(opts, withHeader("Origin", tc.origin))
		}
		if w := do(s, "POST", "/api/refresh", "", opts...); w.Code != tc.want {
			t.Errorf("Origin %q: got %d want %d", tc.origin, w.Code, tc.want)
		}
	}
}

func TestIngestEndpoints(t *testing.T) {
	t.Parallel()
	s, hooks := newTestServer(t)
	w := do(s, "POST", "/hook", `{"hook_event_name":"Stop","cwd":"/repo"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("hook: %d", w.Code)
	}
	select {
	case b := <-hooks:
		if !strings.Contains(string(b), "Stop") {
			t.Fatal("wrong body forwarded")
		}
	default:
		t.Fatal("body not forwarded")
	}
	if w := do(s, "POST", "/status", `{}`); w.Code != http.StatusNoContent {
		t.Fatalf("status: %d", w.Code)
	}
	tests := []struct {
		name string
		opts []reqOpt
		body string
		meth string
		want int
	}{
		{"browser cross-site post", []reqOpt{withHeader("Origin", "http://evil.example")}, "{}", "POST", 403},
		{"fetch metadata", []reqOpt{withHeader("Sec-Fetch-Site", "cross-site")}, "{}", "POST", 403},
		{"non-loopback peer", []reqOpt{withRemote("192.168.1.10:5555")}, "{}", "POST", 403},
		{"GET", nil, "", "GET", 405},
		{"oversized body", nil, strings.Repeat("x", MaxIngestBody+1), "POST", 413},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if w := do(s, tc.meth, "/hook", tc.body, tc.opts...); w.Code != tc.want {
				t.Fatalf("got %d want %d", w.Code, tc.want)
			}
		})
	}
	if len(hooks) != 0 {
		t.Fatal("a refused request was forwarded")
	}
}

func TestEventsStreamsFullStateOnConnect(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	ts := httptest.NewUnstartedServer(nil)
	ts.Start()
	defer ts.Close()
	s.Port = ts.Listener.Addr().(*net.TCPAddr).Port
	ts.Config.Handler = s.Handler()

	req, _ := http.NewRequest("GET", ts.URL+"/events", nil)
	req.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.Token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("%d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	br := bufio.NewReader(resp.Body)
	deadline := time.After(3 * time.Second)
	got := make(chan string, 1)
	go func() {
		var sb strings.Builder
		for {
			line, err := br.ReadString('\n')
			sb.WriteString(line)
			if err != nil || strings.HasPrefix(line, "data: ") {
				got <- sb.String()
				return
			}
		}
	}()
	select {
	case s := <-got:
		if !strings.Contains(s, "event: state") || !strings.Contains(s, `"name":"My Project"`) {
			t.Fatalf("first event: %s", s)
		}
	case <-deadline:
		t.Fatal("no state event")
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 0))
}

// C3: a body dropped because the processor is behind is counted, apart from
// foreign-cwd drops, and raises a banner.
func TestIngestOverflowIsCounted(t *testing.T) {
	t.Parallel()
	s, hooks := newTestServer(t)
	for i := 0; i < cap(hooks)+3; i++ {
		if w := do(s, "POST", "/hook", `{"hook_event_name":"Stop","session_id":"x","cwd":"/repo"}`); w.Code != 204 {
			t.Fatalf("hook answered %d", w.Code)
		}
	}
	if s.Overflow() != 3 {
		t.Fatalf("overflow %d, want 3", s.Overflow())
	}
	m := state.NewModel(testConfig(t), "/repo", t0)
	m.ApplyObs(state.Obs{OverflowDrops: s.Overflow()})
	v := m.Snapshot(t0)
	if v.Dropped != 0 || v.Observe.OverflowDrops != 3 {
		t.Fatalf("overflow must be its own counter: %+v", v.Observe)
	}
	found := false
	for _, b := range v.Banners {
		found = found || strings.Contains(b, "fell behind")
	}
	if !found {
		t.Fatalf("no overflow banner: %v", v.Banners)
	}
}

// The panel only OBSERVES PermissionRequest (and PreCompact, which could block):
// /hook answers 204 with an empty body, which Claude Code reads as "no decision".
// Any body could be read as a decision, and /hook takes no token.
func TestHookNeverAnswersAPermissionRequest(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	for _, ev := range []string{"PermissionRequest", "PreCompact", "StopFailure"} {
		w := do(s, "POST", "/hook", `{"hook_event_name":"`+ev+`","session_id":"x","cwd":"/repo","tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`)
		if w.Code != 204 || w.Body.Len() != 0 || w.Header().Get("Content-Type") != "" {
			t.Fatalf("%s: %d %q; must be 204 with no body", ev, w.Code, w.Body.String())
		}
	}
}

// The stream beats even when nothing changes, so the page can tell a quiet panel
// from a dead one (audit P1-4), and carries the server's clock for the ages.
func TestEventsStreamHasAHeartbeat(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	f := clock.NewFake(t0)
	s.Hub = NewHub(s.Hub.model, f)
	s.Heartbeat = 5 * time.Second
	ts := httptest.NewServer(nil)
	defer ts.Close()
	s.Port = ts.Listener.Addr().(*net.TCPAddr).Port
	ts.Config.Handler = s.Handler()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/events", nil)
	req.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.Token})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := make(chan string, 16)
	go func() { // one SSE event (up to its blank line) per message
		br := bufio.NewReader(resp.Body)
		var ev strings.Builder
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				close(events)
				return
			}
			if line == "\n" && ev.Len() > 0 {
				events <- ev.String()
				ev.Reset()
			} else if line != "\n" {
				ev.WriteString(line)
			}
		}
	}()
	next := func() string {
		t.Helper()
		for {
			select {
			case ev, ok := <-events:
				if !ok {
					t.Fatal("the stream ended")
				}
				if strings.HasPrefix(ev, "retry:") {
					continue
				}
				return ev
			case <-time.After(5 * time.Second):
				t.Fatal("no event: the stream stopped beating")
			}
		}
	}
	if ev := next(); !strings.HasPrefix(ev, "event: state\n") {
		t.Fatalf("first event %q, want the state", ev)
	}
	f.BlockUntil(1) // the stream's heartbeat ticker
	// Five beats on the server's clock, and nothing changes: no second state event.
	for i := 1; i <= 5; i++ {
		f.Advance(s.Heartbeat)
		want := fmt.Sprintf("event: hb\ndata: {\"now\":%d}\n", t0.Add(time.Duration(i)*s.Heartbeat).UnixMilli())
		if ev := next(); ev != want {
			t.Fatalf("beat %d: %q, want %q", i, ev, want)
		}
	}
}

// The terminal is entered only on purpose (audit P1-1): panel.js focuses an xterm in
// exactly one place, enterTerm (Enter or a click on the terminal); the xterm is out
// of the Tab order; Ctrl+] leaves; nothing redraws the page on a timer (P1-3).
func TestPageEntersTheTerminalOnlyOnPurpose(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	if n := strings.Count(js, ".term.focus("); n != 1 {
		t.Fatalf("panel.js focuses a terminal in %d places, want 1 (enterTerm)", n)
	}
	i := strings.Index(js, "function enterTerm(")
	j := strings.Index(js, ".term.focus(")
	if i < 0 || j < i || j-i > 200 {
		t.Fatal("the one terminal focus is not in enterTerm")
	}
	for _, want := range []string{"textarea.tabIndex = -1", `ev.code === "BracketRight"`, "attachCustomKeyEventHandler"} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
	if strings.Contains(js, "setInterval(render") {
		t.Error("panel.js re-renders on a timer; only ages tick (tickAges)")
	}
	rebuild := regexp.MustCompile(`\$\("(left|right|detail|tabs|termbar|banners|restorebar|obs|connbar)"\)\.(replaceChildren|innerHTML)`)
	if m := rebuild.FindString(js); m != "" {
		t.Errorf("panel.js rebuilds a live region (%s); patch it in place", m)
	}
}
