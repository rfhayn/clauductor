// Package web is the panel's HTTP surface: the loopback server and its guards (Host
// allow-list, Origin, cookie, terminal tickets), the hub that pushes the View to
// every page, the terminal WebSocket into a lane, and the embedded page itself.
package web

import (
	"bytes"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/clauductor/clauductor/internal/panel/clock"
	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
)

// webFS is the page: index.html, its static files and the vendored xterm.js.
//
//go:embed index.html static vendor
var webFS embed.FS

// LoopbackHost is the only address the panel ever binds.
const LoopbackHost = "127.0.0.1"

// MaxIngestBody caps a hook or status-line body.
const MaxIngestBody = 256 << 10

// listen binds 127.0.0.1:port and nothing else. A taken port is an error, never a
// reason to try another: the hooks in ~/.claude/settings.json post to a fixed URL.
func listen(port int) (net.Listener, error) {
	ln, err := net.Listen("tcp4", net.JoinHostPort(LoopbackHost, strconv.Itoa(port)))
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return nil, fmt.Errorf("port %d on %s is already in use (another panel? see ~/.clauductor/panel/port). "+
				"The panel does not fall back to another port, because its hooks post to a fixed URL; free the port or pass --port", port, LoopbackHost)
		}
		return nil, err
	}
	if err := ensureLoopback(ln.Addr()); err != nil {
		ln.Close()
		return nil, err
	}
	return ln, nil
}

func ensureLoopback(a net.Addr) error {
	tcp, ok := a.(*net.TCPAddr)
	if !ok || tcp.IP == nil || !tcp.IP.IsLoopback() {
		return fmt.Errorf("refusing to serve on %v: the panel binds loopback only", a)
	}
	return nil
}

// HeartbeatEvery is how often an open /events stream says it is alive, with the
// server's clock. The page treats three missed beats as a lost connection.
var HeartbeatEvery = 5 * time.Second

// Server is the panel's HTTP surface.
type Server struct {
	Port    int
	Token   string
	Hub     *Hub
	Hooks   chan<- []byte // raw hook bodies, processed after the 204
	Status  chan<- []byte // raw status-line bodies
	Refresh func()        // re-poll every source now
	// Lanes controls lanes (v1). Nil when tmux is unavailable: the panel still watches.
	Lanes *lanes.LaneManager
	// CookieMaxAge, when > 0, makes the session cookie persistent (seconds). Used when
	// the panel runs under launchd with a persistent token.
	CookieMaxAge int

	// OnVisible, if set, runs when a page says it is in view and none was (MarkVisible).
	// It must not block.
	OnVisible func()

	// Heartbeat is how often an open /events stream beats. Zero means HeartbeatEvery.
	Heartbeat time.Duration
	// Clock stamps terminal tickets and times their idle close. Required.
	Clock clock.Clock

	// TermIdleTimeout closes a terminal whose page has sent nothing (not even its
	// once-a-minute "alive" while visible) for this long. Zero means 5 minutes.
	TermIdleTimeout time.Duration

	// beforeAddViewer, if set, runs between a terminal's auth and its registration
	// (tests use it to rotate the token in that window).
	beforeAddViewer func()

	tokenMu sync.RWMutex
	rotated chan struct{} // closed, and replaced, on each token rotation

	// HostNames are extra <label>.localhost names the panel answers to (hosts.go in this package).
	HostNames []string

	// Orch carries the v2 orchestration (templates, quota guard, queues, restore).
	Orch *Orchestration

	// Projects are the projects served (PANEL-16), and Default the id a request that
	// names none goes to. Without Projects the server serves the one project Hub,
	// Lanes, Orch and Refresh describe.
	Projects []*Project
	Default  string
	// Summaries, when set, is the projects' menu: every event stream carries it.
	Summaries *Summaries
	// Admin, when set, adds, trusts and removes projects from the page (PANEL-22,
	// admin.go); without it those routes answer 404.
	Admin ProjectAdmin

	// projMu guards Projects, Default and HostNames, which change while the panel
	// serves since PANEL-22. Set them directly only before Handler serves.
	projMu sync.RWMutex

	legacyOnce sync.Once
	legacy     *Project

	// overflow counts ingest bodies dropped because the processor was behind; it is
	// shown apart from events dropped for a foreign cwd.
	overflow atomic.Int64

	// seenAt is the last time a page said it is in view (unix ns; PANEL-11). The
	// dashboard's ps and git reads run only while it is recent.
	seenAt atomic.Int64

	termMu  sync.Mutex
	tickets map[string]termTicket               // single-use WebSocket tickets
	viewers map[string]map[*termViewer]struct{} // open terminals by lane id
}

func (s *Server) cookieName() string { return "clauductor_panel_" + strconv.Itoa(s.Port) }

// hostOK and originOK use the exact allow-list in hosts.go.
func (s *Server) hostOK(host string) bool { return s.hostAllowed(host) }

func (s *Server) originOK(origin, host string) bool { return s.originMatches(origin, host) }

func remoteIsLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) authed(r *http.Request) bool {
	c, err := r.Cookie(s.cookieName())
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.CurrentToken())) == 1
}

// Handler returns the full HTTP handler with every guard applied.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", s.ingest(s.Hooks))
	mux.HandleFunc("/status", s.ingest(s.Status))
	mux.HandleFunc("/", s.index)
	// The streams take ?project=<id> (PANEL-16); without it, the default project.
	mux.HandleFunc("/events", s.requireAuth(s.withProject(s.events)))
	mux.HandleFunc("/api/state", s.requireAuth(s.withProject(func(w http.ResponseWriter, r *http.Request, p *Project) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(p.Hub.Snapshot())
	})))
	mux.HandleFunc("GET /api/projects", s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(s.summaryJSON())
	}))
	// A page in view says so once a minute; nothing else changes (PANEL-11).
	mux.HandleFunc("POST /api/seen", s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		s.MarkVisible(s.clock().Now())
		w.WriteHeader(http.StatusNoContent)
	}))
	refresh := func(w http.ResponseWriter, r *http.Request, p *Project) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if p.Refresh != nil {
			p.Refresh()
		}
		w.WriteHeader(http.StatusNoContent)
	}
	mux.HandleFunc("/api/refresh", s.requireAuth(s.withProject(refresh)))
	mux.HandleFunc("POST /api/p/{project}/refresh", s.requireAuth(s.withProject(refresh)))
	s.laneRoutes(mux)
	s.adminRoutes(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		// DNS rebinding: a hostile name resolving to 127.0.0.1 still carries its own Host.
		if !s.hostOK(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if !remoteIsLoopback(r) {
			http.Error(w, "loopback only", http.StatusForbidden)
			return
		}
		// Every state-changing browser request must come from the panel's own page.
		if r.URL.Path != "/hook" && r.URL.Path != "/status" &&
			r.Method != http.MethodGet && r.Method != http.MethodHead && !s.originOK(r.Header.Get("Origin"), r.Host) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authed(r) {
			http.Error(w, "unauthorized: open the URL `clauductor panel` printed", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// ingest serves /hook and /status. They carry no token (a session cannot know it), so
// they are loopback-only, write-only, size-capped, and closed to browsers: Claude
// Code sends no Origin, while any cross-site browser POST does.
func (s *Server) ingest(out chan<- []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
			http.Error(w, "not for browsers", http.StatusForbidden)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxIngestBody))
		if err != nil {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		// Answer first: a hook must never wait on the panel's own processing.
		w.WriteHeader(http.StatusNoContent)
		if out == nil {
			return
		}
		select {
		case out <- body:
		default: // the processor is behind; dropping one event beats blocking a session
			s.overflow.Add(1)
		}
	}
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if t := r.URL.Query().Get("t"); t != "" {
		token := s.CurrentToken()
		if subtle.ConstantTimeCompare([]byte(t), []byte(token)) != 1 {
			http.Error(w, "unauthorized: stale or wrong token", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: token, Path: "/",
			HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: s.CookieMaxAge})
		// Drop the token from the address bar and history; keep the project it opens on.
		to := "/"
		if p := r.URL.Query().Get("p"); config.ProjectIDRe.MatchString(p) {
			to += "?p=" + p
		}
		http.Redirect(w, r, to, http.StatusSeeOther)
		return
	}
	if !s.authed(r) {
		http.Error(w, "unauthorized: open the URL `clauductor panel` printed", http.StatusUnauthorized)
		return
	}
	page, _ := webFS.ReadFile("index.html")
	// No inline script or style is allowed, and nothing outside this origin. The one
	// exception is a per-response nonce for the <style> elements xterm.js creates at
	// run time; panel.js stamps it on them.
	nonce, err := install.NewToken()
	if err != nil {
		http.Error(w, "no randomness", http.StatusInternalServerError)
		return
	}
	nonce = nonce[:32]
	page = bytes.Replace(page, []byte("{{STYLE_NONCE}}"), []byte(nonce), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'nonce-"+nonce+"'; "+
		"font-src 'self'; connect-src 'self' ws://"+r.Host+"; img-src 'self' data:; "+
		"base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	w.Write(page)
}

// summaryJSON is the projects' menu, or the one project's entry without one.
func (s *Server) summaryJSON() []byte {
	if s.Summaries != nil {
		return s.Summaries.JSON()
	}
	var list []ProjectSummary
	for _, p := range s.projects() {
		sum := Summarize(p.ID, p.Hub.View())
		sum.Default = p == s.project("")
		list = append(list, sum)
	}
	b, _ := json.Marshal(list)
	return b
}

func (s *Server) events(w http.ResponseWriter, r *http.Request, p *Project) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Connection", "keep-alive")
	ch, cancel := p.Hub.subscribe()
	defer cancel()
	// Every stream also carries the projects' menu, so a page sees what needs you
	// in the projects it is not showing (PANEL-16).
	var menu chan []byte
	if s.Summaries != nil {
		var stop func()
		menu, stop = s.Summaries.subscribe()
		defer stop()
	}
	rotated := s.rotation() // a token rotation ends this stream; the page's reconnect then gets 401
	send := func(event string, b []byte) error {
		_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		fl.Flush()
		return err
	}
	fmt.Fprint(w, "retry: 2000\n\n")
	if send("state", p.Hub.Snapshot()) != nil {
		return
	}
	if menu != nil && send("projects", s.Summaries.JSON()) != nil {
		return
	}
	every := s.Heartbeat
	if every <= 0 {
		every = HeartbeatEvery
	}
	ping := p.Hub.clock.NewTicker(every)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-rotated:
			return
		case <-p.Gone(): // removed live: the page reconnects and finds it gone
			return
		case b := <-ch:
			if send("state", b) != nil {
				return
			}
		case b := <-menu: // a nil menu never fires
			if send("projects", b) != nil {
				return
			}
		case <-ping.C():
			// A named event, not a comment: EventSource hides comments from the page,
			// and the page needs the beat (and the server's clock) to know it is live.
			if _, err := fmt.Fprintf(w, "event: hb\ndata: {\"now\":%d}\n\n", p.Hub.clock.Now().UnixMilli()); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

func (s *Server) CurrentToken() string {
	s.tokenMu.RLock()
	defer s.tokenMu.RUnlock()
	return s.Token
}

// rotation returns a channel that is closed at the next token rotation.
func (s *Server) rotation() <-chan struct{} {
	s.tokenMu.Lock()
	defer s.tokenMu.Unlock()
	if s.rotated == nil {
		s.rotated = make(chan struct{})
	}
	return s.rotated
}

// Rotate replaces the token. Every cookie issued for the old one stops working at
// once, every open terminal is closed (code 4001) and every event stream ends, so
// nothing opened with the old token outlives it.
func (s *Server) Rotate(token string) {
	s.tokenMu.Lock()
	if token == s.Token {
		s.tokenMu.Unlock()
		return
	}
	s.Token = token
	if s.rotated != nil {
		close(s.rotated)
	}
	s.rotated = make(chan struct{})
	s.tokenMu.Unlock()
	s.termMu.Lock()
	s.tickets = nil // a ticket issued under the old token dies with it
	s.termMu.Unlock()
	s.closeAllTerminals(4001, "token rotated")
}

// Overflow returns how many ingest bodies were dropped because the processor was
// behind.
func (s *Server) Overflow() int64 { return s.overflow.Load() }

// FocusedLanes returns the lanes whose terminal has keyboard focus in a page now.
// The notifier sends no OS notification for them: you are looking at the lane.
func (s *Server) FocusedLanes() map[string]bool { return s.FocusedLanesIn(s.project("").ID) }

// FocusedLanesIn is FocusedLanes for one project, by lane id.
func (s *Server) FocusedLanesIn(project string) map[string]bool {
	s.termMu.Lock()
	defer s.termMu.Unlock()
	out := map[string]bool{}
	prefix := termKey(project, "")
	for k, vs := range s.viewers {
		lane, ok := strings.CutPrefix(k, prefix)
		if !ok || (project == "" && strings.Contains(k, "/")) {
			continue
		}
		for v := range vs {
			if v.focused.Load() {
				out[lane] = true
			}
		}
	}
	return out
}

func (s *Server) clock() clock.Clock {
	if s.Clock == nil {
		panic("web: Server.Clock is not set")
	}
	return s.Clock
}

// PageVisibleFor is how long a page's "in view" lasts without another.
const PageVisibleFor = 90 * time.Second

// PageVisible is whether a page said it was in view within PageVisibleFor.
func (s *Server) PageVisible(now time.Time) bool {
	at := s.seenAt.Load()
	return at != 0 && now.Sub(time.Unix(0, at)) < PageVisibleFor
}

// MarkVisible records that a page is in view now. When no page was in view before,
// OnVisible runs, so the reads that wait for a page start now rather than at their
// next tick (PANEL-21: the Metrics view's merged pull requests waited a minute).
func (s *Server) MarkVisible(now time.Time) {
	was := s.PageVisible(now)
	s.seenAt.Store(now.UnixNano())
	if !was && s.OnVisible != nil {
		s.OnVisible()
	}
}
