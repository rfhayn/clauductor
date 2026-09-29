package panel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/coder/websocket"
	"github.com/creack/pty"
)

// The terminal endpoint is a shell into a lane, so it is the most guarded route:
//
//   - the Host check (global) and the auth cookie;
//   - an Origin exactly equal to http://<the Host> (scheme, host and port);
//   - a single-use ticket, valid 30 s for one lane, issued by an Origin-checked POST
//     and presented in the Sec-WebSocket-Protocol header. The cookie alone is not
//     enough: cookies are not isolated by port (RFC 6265 §8.5), so a page on another
//     loopback port is same-site and its requests carry the cookie. A ticket can
//     only be read by the panel's own page;
//   - a validated lane id that must name a running lane;
//   - a fixed message vocabulary. The browser sends keystrokes and sizes; it can
//     never name a command. The server runs one fixed argv per viewer:
//     `tmux -u -L <socket> attach-session -t =<id>`.
//
// Each viewer gets its own PTY and its own tmux client, and closing the page only
// detaches that client: the lane keeps running. Stopping the lane, or the panel
// shutting down, closes every viewer.

// TermSubprotocol is the WebSocket subprotocol the page and server speak.
const TermSubprotocol = "clauductor.term.v1"

// ticketPrefix marks the ticket among the offered subprotocols.
const ticketPrefix = "ticket."

// TicketTTL is how long a terminal ticket may wait before it is used.
const TicketTTL = 30 * time.Second

type termTicket struct {
	lane string
	exp  time.Time
}

type termViewer struct {
	conn    *websocket.Conn
	focused atomic.Bool // the page reports this terminal has keyboard focus (v2: alerts stay quiet)
}

// issueTicket returns a new single-use ticket for one lane.
func (s *Server) issueTicket(lane string) (string, error) {
	t, err := NewToken()
	if err != nil {
		return "", err
	}
	s.termMu.Lock()
	defer s.termMu.Unlock()
	if s.tickets == nil {
		s.tickets = map[string]termTicket{}
	}
	now := time.Now()
	for k, v := range s.tickets { // expired tickets never pile up
		if now.After(v.exp) {
			delete(s.tickets, k)
		}
	}
	s.tickets[t] = termTicket{lane: lane, exp: now.Add(TicketTTL)}
	return t, nil
}

// takeTicket consumes the ticket offered in Sec-WebSocket-Protocol, whatever the
// outcome, and reports whether it was valid for this lane.
func (s *Server) takeTicket(r *http.Request, lane string) bool {
	var offered string
	for _, h := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, p := range strings.Split(h, ",") {
			if p = strings.TrimSpace(p); strings.HasPrefix(p, ticketPrefix) {
				offered = strings.TrimPrefix(p, ticketPrefix)
			}
		}
	}
	if offered == "" {
		return false
	}
	s.termMu.Lock()
	defer s.termMu.Unlock()
	t, ok := s.tickets[offered]
	delete(s.tickets, offered)
	return ok && t.lane == lane && time.Now().Before(t.exp)
}

func (s *Server) addViewer(lane string, v *termViewer) {
	s.termMu.Lock()
	defer s.termMu.Unlock()
	if s.viewers == nil {
		s.viewers = map[string]map[*termViewer]struct{}{}
	}
	if s.viewers[lane] == nil {
		s.viewers[lane] = map[*termViewer]struct{}{}
	}
	s.viewers[lane][v] = struct{}{}
}

func (s *Server) removeViewer(lane string, v *termViewer) {
	s.termMu.Lock()
	defer s.termMu.Unlock()
	delete(s.viewers[lane], v)
}

// closeAllTerminals closes every viewer of every lane.
func (s *Server) closeAllTerminals(code websocket.StatusCode, reason string) {
	s.termMu.Lock()
	all := s.viewers
	s.viewers = nil
	s.termMu.Unlock()
	for _, vs := range all {
		for v := range vs {
			go v.conn.Close(code, reason)
		}
	}
}

// Close codes the page acts on: an idle close reopens when the page is back in
// view; a rotation does not reopen (the cookie is dead too).
const (
	closeIdle    websocket.StatusCode = 4000
	closeRotated websocket.StatusCode = 4001
)

// closeTerminals closes every viewer of a lane (it stopped).
func (s *Server) closeTerminals(lane string) {
	s.termMu.Lock()
	vs := s.viewers[lane]
	delete(s.viewers, lane)
	s.termMu.Unlock()
	for v := range vs {
		// Close waits for the browser's reply; never make the stop request wait on it.
		go v.conn.Close(websocket.StatusGoingAway, "lane stopped")
	}
}

// issueTicketHandler serves POST /api/lanes/{id}/ticket.
func (s *Server) issueTicketHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !config.ValidLaneID(id) {
		writeLaneErr(w, laneErr(http.StatusBadRequest, "invalid", "invalid lane id"))
		return
	}
	t, err := s.issueTicket(id)
	if err != nil {
		writeLaneErr(w, laneErr(http.StatusInternalServerError, "fault", "%v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ticket": t, "protocol": TermSubprotocol})
}

// maxTermMessage caps one browser message (a large paste).
const maxTermMessage = 1 << 20

// termMsg is the only shape the browser may send.
type termMsg struct {
	Type string `json:"type"` // "input" | "resize" | "alive" | "focus"
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	On   bool   `json:"focused,omitempty"` // "focus": the terminal has keyboard focus
}

func clampDim(v, def int) uint16 {
	if v < 2 {
		return uint16(def)
	}
	if v > 1000 {
		return 1000
	}
	return uint16(v)
}

// attachEnv is the environment of a viewer's tmux client.
func attachEnv() []string {
	env := []string{}
	for _, kv := range lanes.TmuxEnv() {
		if strings.HasPrefix(kv, "TERM=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "TERM=xterm-256color")
}

func (s *Server) terminal(w http.ResponseWriter, r *http.Request) {
	// Take the rotation generation BEFORE checking the cookie: a rotation between
	// the check and the viewer's registration would otherwise miss this viewer.
	gen := s.rotation()
	if !s.authed(r) {
		http.Error(w, "unauthorized: open the URL `clauductor panel` printed", http.StatusUnauthorized)
		return
	}
	// A WebSocket upgrade is a GET, which the global guard lets through without an
	// Origin check; any page can open a WebSocket to loopback, so check here, exactly.
	if o := r.Header.Get("Origin"); !s.originOK(o, r.Host) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}
	if s.Lanes == nil {
		http.Error(w, "lanes are unavailable: tmux was not found", http.StatusNotFound)
		return
	}
	id := r.URL.Query().Get("lane")
	if !config.ValidLaneID(id) {
		http.Error(w, "invalid lane id", http.StatusBadRequest)
		return
	}
	if !s.takeTicket(r, id) {
		http.Error(w, "unauthorized: a terminal needs a fresh ticket from POST /api/lanes/{id}/ticket", http.StatusUnauthorized)
		return
	}
	if !s.Lanes.Exists(r.Context(), id) {
		http.Error(w, "no such lane", http.StatusNotFound)
		return
	}
	if err := s.Lanes.Harden(r.Context()); err != nil {
		http.Error(w, "cannot secure the tmux socket: "+err.Error(), http.StatusInternalServerError)
		return
	}
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols: []string{TermSubprotocol},
		// No patterns: the library then accepts only an Origin whose host equals the
		// request's Host, the same rule as originOK, as a second layer.
	})
	if err != nil {
		return
	}
	if s.beforeAddViewer != nil {
		s.beforeAddViewer()
	}
	viewer := &termViewer{conn: c}
	s.addViewer(id, viewer)
	defer s.removeViewer(id, viewer)
	select {
	case <-gen: // rotated while this upgrade was in flight
		c.Close(closeRotated, "token rotated")
		return
	default:
	}
	c.SetReadLimit(maxTermMessage)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	cmd := exec.Command(s.Lanes.TmuxPath, s.Lanes.AttachArgv(id)...)
	cmd.Env = attachEnv()
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: clampDim(cols, 80), Rows: clampDim(rows, 24)})
	if err != nil {
		c.Close(websocket.StatusInternalError, "cannot attach")
		return
	}
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			ptmx.Close() // hangs up the tmux client, which detaches; the lane runs on
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})
	}
	defer cleanup()

	// PTY → browser, as binary frames.
	go func() {
		defer cancel()
		buf := make([]byte, 32<<10)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if werr := c.Write(ctx, websocket.MessageBinary, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				if err != io.EOF && !isClosedPTY(err) {
					_ = c.Write(ctx, websocket.MessageText, []byte(`{"type":"exit","reason":"read"}`))
				} else {
					_ = c.Write(ctx, websocket.MessageText, []byte(`{"type":"exit"}`))
				}
				c.Close(websocket.StatusNormalClosure, "detached")
				return
			}
		}
	}()

	// A page that is hidden or gone stops sending "alive" (once a minute while in
	// view); after TermIdleTimeout of silence the terminal closes. The page reopens
	// it with a fresh ticket when it is back in view.
	idle := s.TermIdleTimeout
	if idle <= 0 {
		idle = 5 * time.Minute
	}
	var lastMsg atomic.Int64
	lastMsg.Store(time.Now().UnixNano())
	go func() {
		t := time.NewTicker(idle / 4)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if time.Since(time.Unix(0, lastMsg.Load())) >= idle {
					c.Close(closeIdle, "idle")
					cancel()
					return
				}
			}
		}
	}()

	// Browser → PTY: keystrokes and sizes only.
	scroll := &copyWatch{lanes: s.Lanes, id: id, conn: c}
	for {
		typ, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		var m termMsg
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields()
		if typ != websocket.MessageText || dec.Decode(&m) != nil {
			c.Close(websocket.StatusPolicyViolation, "expected {type: input|resize}")
			return
		}
		lastMsg.Store(time.Now().UnixNano())
		switch m.Type {
		case "alive":
		case "focus":
			viewer.focused.Store(m.On)
		case "input":
			if !scroll.before(ctx, m.Data) {
				continue
			}
			if _, err := ptmx.Write([]byte(m.Data)); err != nil {
				return
			}
		case "resize":
			_ = pty.Setsize(ptmx, &pty.Winsize{Cols: clampDim(m.Cols, 80), Rows: clampDim(m.Rows, 24)})
		default:
			c.Close(websocket.StatusPolicyViolation, "unknown message type")
			return
		}
	}
}

func isClosedPTY(err error) bool {
	// On macOS a PTY whose client exited reads as EIO, and a closed file as ErrClosed.
	return err == os.ErrClosed || strings.Contains(err.Error(), "input/output error") ||
		strings.Contains(err.Error(), "file already closed")
}
