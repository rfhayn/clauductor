package web

import (
	"context"
	"fmt"
	"github.com/clauductor/clauductor/internal/leakcheck"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// The terminal endpoint is a shell into a lane. Each guard is checked on its own,
// with every other guard satisfied, so a case passes only because of the guard it
// names. The lane manager points at a socket with no server: a request that got past
// every guard answers 404 "no such lane", never 101.
func TestTerminalUpgradeGuards(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	s.Lanes = testLaneManager(t)
	s.Lanes.Socket = leakcheck.NoServerSocket()
	origin := withHeader("Origin", "http://127.0.0.1:4393")
	ticket := func(lane string) reqOpt {
		tk, err := s.issueTicket(lane)
		if err != nil {
			t.Fatal(err)
		}
		return withHeader("Sec-WebSocket-Protocol", TermSubprotocol+", "+ticketPrefix+tk)
	}
	ws := func(r reqOptList) reqOptList {
		return append(r, withHeader("Connection", "Upgrade"), withHeader("Upgrade", "websocket"),
			withHeader("Sec-WebSocket-Version", "13"), withHeader("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ=="))
	}
	used, _ := s.issueTicket("a")
	s.takeTicket(httptestWithProtocol(ticketPrefix+used), "a")
	cases := []struct {
		name   string
		opts   reqOptList
		target string
		status int
		body   string
	}{
		{"no cookie", ws(reqOptList{origin, ticket("a")}), "/ws/term?lane=a", 401, "unauthorized"},
		{"wrong cookie", ws(reqOptList{origin, ticket("a"), withHeader("Cookie", s.cookieName()+"=nope")}), "/ws/term?lane=a", 401, "unauthorized"},
		{"foreign origin", ws(reqOptList{withCookie(s), ticket("a"), withHeader("Origin", "http://evil.example")}), "/ws/term?lane=a", 403, "forbidden origin"},
		// The dev app on :3000 is same-site: the browser sends it the panel's cookie.
		{"a page on :3000", ws(reqOptList{withCookie(s), ticket("a"), withHeader("Origin", "http://127.0.0.1:3000")}), "/ws/term?lane=a", 403, "forbidden origin"},
		{"https origin", ws(reqOptList{withCookie(s), ticket("a"), withHeader("Origin", "https://127.0.0.1:4393")}), "/ws/term?lane=a", 403, "forbidden origin"},
		{"origin spelled differently from Host", ws(reqOptList{withCookie(s), ticket("a"), withHeader("Origin", "http://localhost:4393")}), "/ws/term?lane=a", 403, "forbidden origin"},
		{"no origin", ws(reqOptList{withCookie(s), ticket("a")}), "/ws/term?lane=a", 403, "forbidden origin"},
		{"foreign host", ws(reqOptList{withCookie(s), ticket("a"), origin, withHost("evil.example:4393")}), "/ws/term?lane=a", 403, "forbidden host"},
		{"cookie and origin but no ticket", ws(reqOptList{withCookie(s), origin}), "/ws/term?lane=a", 401, "ticket"},
		{"ticket for another lane", ws(reqOptList{withCookie(s), origin, ticket("b")}), "/ws/term?lane=a", 401, "ticket"},
		{"ticket used twice", ws(reqOptList{withCookie(s), origin, withHeader("Sec-WebSocket-Protocol", ticketPrefix+used)}), "/ws/term?lane=a", 401, "ticket"},
		{"made-up ticket", ws(reqOptList{withCookie(s), origin, withHeader("Sec-WebSocket-Protocol", ticketPrefix+strings.Repeat("0", 64))}), "/ws/term?lane=a", 401, "ticket"},
		{"bad lane id", ws(reqOptList{withCookie(s), origin, ticket("a")}), "/ws/term?lane=../etc", 400, "invalid lane id"},
		{"lane id as a tmux target", ws(reqOptList{withCookie(s), origin, ticket("a")}), "/ws/term?lane=a:0", 400, "invalid lane id"},
		{"all guards pass, no such lane", ws(reqOptList{withCookie(s), origin, ticket("a")}), "/ws/term?lane=a", 404, "no such lane"},
	}
	for _, c := range cases {
		w := do(s, "GET", c.target, "", c.opts...)
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.body) {
			t.Errorf("%s: got %d %q, want %d %q", c.name, w.Code, strings.TrimSpace(w.Body.String()), c.status, c.body)
		}
	}
	// An expired ticket is refused. It is planted directly, with no other ticket
	// issued after it: issuing purges expired tickets, which would hide a missing
	// expiry check.
	expired, _ := s.issueTicket("a")
	s.termMu.Lock()
	s.tickets[expired] = termTicket{lane: "a", exp: time.Now().Add(-time.Second)}
	s.termMu.Unlock()
	if w := do(s, "GET", "/ws/term?lane=a", "", ws(reqOptList{withCookie(s), origin,
		withHeader("Sec-WebSocket-Protocol", ticketPrefix+expired)})...); w.Code != 401 {
		t.Errorf("expired ticket: got %d %q, want 401", w.Code, w.Body.String())
	}
	// A page on :3000 cannot get a ticket either: the ticket POST checks Origin.
	if w := do(s, "POST", "/api/lanes/a/ticket", "", withCookie(s), withHeader("Origin", "http://127.0.0.1:3000")); w.Code != 403 {
		t.Fatalf("ticket for a :3000 page: %d", w.Code)
	}
	if w := do(s, "POST", "/api/lanes/a/ticket", "", origin); w.Code != 401 {
		t.Fatalf("ticket without the cookie: %d", w.Code)
	}
	if w := do(s, "POST", "/api/lanes/a/ticket", "", withCookie(s), origin); w.Code != 200 || !strings.Contains(w.Body.String(), `"ticket"`) {
		t.Fatalf("ticket for the panel's page: %d %s", w.Code, w.Body.String())
	}
}

func httptestWithProtocol(p string) *http.Request {
	r, _ := http.NewRequest("GET", "/", nil)
	r.Header.Set("Sec-WebSocket-Protocol", p)
	return r
}

// The page carries no inline script or style and loads nothing from another origin.
func TestPageCSPIsSelfOnly(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	w := do(s, "GET", "/", "", withCookie(s))
	csp := w.Header().Get("Content-Security-Policy")
	for _, bad := range []string{"unsafe-inline", "unsafe-eval", "googleapis", "gstatic", "https:", "*"} {
		if strings.Contains(csp, bad) {
			t.Errorf("CSP allows %q: %s", bad, csp)
		}
	}
	m := regexp.MustCompile(`'nonce-([0-9a-f]{32})'`).FindStringSubmatch(csp)
	if m == nil || !strings.Contains(w.Body.String(), `content="`+m[1]+`"`) {
		t.Fatalf("style nonce missing from CSP or page: %s", csp)
	}
	w2 := do(s, "GET", "/", "", withCookie(s))
	if w2.Header().Get("Content-Security-Policy") == csp {
		t.Fatal("the nonce is not per response")
	}
	page := w.Body.String()
	for _, bad := range []string{"<script>", "<style>", " style=\"", "onclick=", "https://"} {
		if strings.Contains(page, bad) {
			t.Errorf("page contains %q", bad)
		}
	}
	for _, path := range []string{"/static/panel.js", "/static/panel.css", "/static/theme.js", "/static/themes.css", "/static/types.css", "/static/xterm-style.js", "/static/term-links.js", "/static/tuned.js", "/static/fonts/overpass-latin-400-normal.woff2", "/static/fonts/OFL-overpass.txt"} {
		if w := do(s, "GET", path, "", withCookie(s)); w.Code != 200 {
			t.Errorf("%s: %d", path, w.Code)
		}
	}
}

type reqOptList []reqOpt

// Lane control is a POST: it needs the cookie and the panel's own Origin.
func TestLaneAPIGuards(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	s.Lanes = testLaneManager(t)
	s.Lanes.Socket = leakcheck.NoServerSocket()
	origin := withHeader("Origin", "http://127.0.0.1:4393")
	for _, c := range []struct {
		name, method, target, body string
		opts                       reqOptList
		status                     int
	}{
		{"start without cookie", "POST", "/api/lanes", `{}`, reqOptList{origin}, 401},
		{"start cross-site", "POST", "/api/lanes", `{}`, reqOptList{withCookie(s), withHeader("Origin", "http://evil.example")}, 403},
		{"start by GET", "GET", "/api/lanes", ``, reqOptList{withCookie(s)}, 404},
		{"stop without cookie", "POST", "/api/lanes/a/stop", ``, reqOptList{origin}, 401},
		{"stop cross-site", "POST", "/api/lanes/a/stop", ``, reqOptList{withCookie(s), withHeader("Origin", "http://evil.example")}, 403},
		{"unknown field", "POST", "/api/lanes", `{"type":"fix","mode":"root","name":"a","command":"rm"}`, reqOptList{withCookie(s), origin}, 400},
		{"unknown action", "POST", "/api/lanes/a/exec", ``, reqOptList{withCookie(s), origin}, 404},
		{"bad lane id", "POST", "/api/lanes/A_B/stop", ``, reqOptList{withCookie(s), origin}, 400},
		{"missing lane", "POST", "/api/lanes/a/stop", ``, reqOptList{withCookie(s), origin}, 404},
	} {
		if w := do(s, c.method, c.target, c.body, c.opts...); w.Code != c.status {
			t.Errorf("%s: got %d %q, want %d", c.name, w.Code, strings.TrimSpace(w.Body.String()), c.status)
		}
	}
	// Tokenless and without lanes: /healthz says only "ok" and the panel's PID.
	if w := do(s, "GET", "/healthz", ""); w.Code != 200 || w.Body.String() != fmt.Sprintf("ok pid=%d\n", os.Getpid()) {
		t.Fatalf("/healthz: %d %q", w.Code, w.Body.String())
	}
	if w := do(s, "GET", "/healthz", "", withHost("evil.example:4393")); w.Code != 403 {
		t.Fatalf("/healthz ignores the Host check: %d", w.Code)
	}
	// Vendored xterm is served from the binary, behind the cookie.
	if w := do(s, "GET", "/vendor/xterm/xterm.js", ""); w.Code != 401 {
		t.Fatalf("vendor without cookie: %d", w.Code)
	}
	if w := do(s, "GET", "/vendor/xterm/xterm.js", "", withCookie(s)); w.Code != 200 || !strings.Contains(w.Body.String(), "Terminal") {
		t.Fatalf("vendor xterm: %d", w.Code)
	}
	if w := do(s, "GET", "/vendor/addon-fit/addon-fit.js", "", withCookie(s)); w.Code != 200 || !strings.Contains(w.Body.String(), "FitAddon") {
		t.Fatalf("vendor fit addon: %d", w.Code)
	}
	for _, lic := range []string{"/vendor/xterm/LICENSE", "/vendor/addon-fit/LICENSE"} {
		if w := do(s, "GET", lic, "", withCookie(s)); w.Code != 200 || !strings.Contains(w.Body.String(), "Permission is hereby granted") {
			t.Fatalf("%s: %d", lic, w.Code)
		}
	}
}

func TestPersistentCookieOnlyWhenAsked(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	w := do(s, "GET", "/?t="+s.Token, "")
	if c := w.Header().Get("Set-Cookie"); strings.Contains(c, "Max-Age") {
		t.Fatalf("per-launch cookie is persistent: %s", c)
	}
	s.CookieMaxAge = 30 * 24 * 3600
	w = do(s, "GET", "/?t="+s.Token, "")
	if c := w.Header().Get("Set-Cookie"); !strings.Contains(c, "Max-Age=2592000") || !strings.Contains(c, "HttpOnly") || !strings.Contains(c, "SameSite=Strict") {
		t.Fatalf("launchd cookie: %s", c)
	}
}

// F4: a terminal that passed auth just before a token rotation is closed as soon as
// it registers, and tickets issued under the old token die with it.
func TestRotationDuringAnUpgradeStillClosesTheTerminal(t *testing.T) {
	t.Parallel()
	tmux, sock := securitySocket(t) // security: runs under -short
	if err := exec.Command(tmux, "-L", sock, "-f", "/dev/null", "new-session", "-d", "-s", "a", "/bin/sh").Run(); err != nil {
		t.Fatal(err)
	}
	s, _ := newTestServer(t)
	s.Lanes = testLaneManager(t)
	s.Lanes.TmuxPath, s.Lanes.Socket = tmux, sock
	ts := httptest.NewUnstartedServer(nil)
	s.Port = ts.Listener.Addr().(*net.TCPAddr).Port
	ts.Config.Handler = s.Handler()
	ts.Start()
	defer ts.Close()
	origin := "http://127.0.0.1:" + strconv.Itoa(s.Port)

	dial := func(ticket, token string) (*websocket.Conn, error) {
		h := http.Header{}
		h.Set("Origin", origin)
		h.Set("Cookie", s.cookieName()+"="+token)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c, _, err := websocket.Dial(ctx, "ws://127.0.0.1:"+strconv.Itoa(s.Port)+"/ws/term?lane=a",
			&websocket.DialOptions{HTTPHeader: h, Subprotocols: []string{TermSubprotocol, ticketPrefix + ticket}})
		return c, err
	}
	old := s.Token
	tk, _ := s.issueTicket("a")
	s.beforeAddViewer = func() { s.beforeAddViewer = nil; s.Rotate(strings.Repeat("b", 64)) }
	c, err := dial(tk, old)
	if err != nil {
		t.Fatalf("the upgrade passed auth before the rotation, so it should connect: %v", err)
	}
	if got := closeCode(c, 3*time.Second); got != closeRotated {
		t.Fatalf("terminal registered after a rotation closed with %v, want %v", got, closeRotated)
	}
	stale, _ := s.issueTicket("a")
	s.Rotate(strings.Repeat("c", 64))
	if _, err := dial(stale, strings.Repeat("c", 64)); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("a ticket issued before a rotation still works: %v", err)
	}
}
