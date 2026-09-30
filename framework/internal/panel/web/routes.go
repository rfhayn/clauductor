package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/metrics"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// laneRoutes adds the v1 routes. Every one except /healthz needs the auth cookie; the
// POSTs also pass the global Origin check, and the WebSocket checks Origin itself.
//
// Each action route is served twice (PANEL-16): under /api/p/{project}/, and at its
// path before PANEL-16 for the default project, so a page left open across an
// upgrade keeps working for one release.
// TODO (PANEL-19): remove the routes without /p/{project}.
func (s *Server) laneRoutes(mux *http.ServeMux) {
	// /healthz tells `clauductor panel open` and a person with curl that the panel is
	// up. It answers "ok" and nothing else, so it needs no token.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// The PID lets `clauductor panel open` check it is talking to THIS panel (the
		// one in ~/.clauductor/panel/pid) before it sends the token anywhere.
		fmt.Fprintf(w, "ok pid=%d\n", os.Getpid())
	})
	files := http.FileServerFS(webFS)
	mux.HandleFunc("GET /vendor/", s.requireAuth(files.ServeHTTP))
	mux.HandleFunc("GET /static/", s.requireAuth(files.ServeHTTP))
	// The terminal checks the cookie itself, after taking the rotation generation.
	mux.HandleFunc("GET /ws/term", s.terminal)
	for _, pre := range []string{"/api/p/{project}", "/api"} {
		mux.HandleFunc("POST "+pre+"/lanes/{id}/ticket", s.requireAuth(s.withProject(s.issueTicketHandler)))
		mux.HandleFunc("POST "+pre+"/lanes/{id}/image", s.requireAuth(s.withProject(s.pasteImage)))
		mux.HandleFunc("POST "+pre+"/lanes/{id}/close", s.requireAuth(s.withProject(s.closeLane)))
		mux.HandleFunc("POST "+pre+"/lanes", s.requireAuth(s.withProject(s.startLane)))
		mux.HandleFunc("POST "+pre+"/lanes/{id}/{action}", s.requireAuth(s.withProject(s.laneAction)))
		// v2: fixed verbs on validated ids; none takes a command.
		mux.HandleFunc("POST "+pre+"/lanes/restore-all", s.requireAuth(s.withProject(s.restoreAll)))
		mux.HandleFunc("POST "+pre+"/queues/{id}/cancel", s.requireAuth(s.withProject(s.queueCancel)))
		mux.HandleFunc("POST "+pre+"/queues/{id}/run", s.requireAuth(s.withProject(s.queueRun)))
	}
	// PANEL-18, after PANEL-16: under /api/p/{project} only; no page predates it.
	mux.HandleFunc("POST /api/p/{project}/worktrees/remove", s.requireAuth(s.withProject(s.removeWorktree)))
	// PANEL-19: the Metrics view, read only. ?scope=all combines every project's.
	mux.HandleFunc("GET /api/p/{project}/metrics", s.requireAuth(s.withProject(s.metricsView)))
}

// metricsView serves GET /api/p/{project}/metrics (PANEL-19): the project's Metrics
// view, or with ?scope=all every served project's, combined. It runs nothing: the
// figures are what the metrics sources last read.
func (s *Server) metricsView(w http.ResponseWriter, r *http.Request, p *Project) {
	scope := r.URL.Query().Get("scope")
	if scope != "" && scope != "project" && scope != "all" {
		writeLaneErr(w, laneErr(http.StatusBadRequest, "invalid", "scope must be project or all"))
		return
	}
	if scope != "all" {
		if p.Metrics == nil {
			writeLaneErr(w, laneErr(http.StatusNotFound, "no-metrics", "no metrics for this project"))
			return
		}
		writeJSON(w, http.StatusOK, p.Metrics())
		return
	}
	var reps []metrics.Report
	for _, q := range s.projects() {
		if q.Metrics != nil {
			reps = append(reps, q.Metrics())
		}
	}
	writeJSON(w, http.StatusOK, metrics.Combine(reps, s.clock().Now()))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// laneErr is an expected failure the HTTP layer reports in the lanes' vocabulary.
func laneErr(status int, code, format string, a ...any) *lanes.LaneError {
	return &lanes.LaneError{Status: status, Code: code, Msg: fmt.Sprintf(format, a...)}
}

func writeLaneErr(w http.ResponseWriter, e *lanes.LaneError) {
	writeJSON(w, e.Status, map[string]any{"ok": false, "error": e.Msg, "code": e.Code})
}

func noLanes(w http.ResponseWriter, p *Project) bool {
	if p.Lanes == nil {
		writeLaneErr(w, laneErr(http.StatusServiceUnavailable, "no-tmux", "lanes are unavailable: tmux was not found on the panel's PATH"))
		return true
	}
	return false
}

func (s *Server) startLane(w http.ResponseWriter, r *http.Request, p *Project) {
	if noLanes(w, p) {
		return
	}
	var req lanes.StartRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeLaneErr(w, laneErr(http.StatusBadRequest, "malformed-request", "body must be {type, mode, name, worktree?}: %v", err))
		return
	}
	res, lerr := p.Lanes.StartLane(r.Context(), req, p.Orch.startGate())
	if lerr != nil {
		writeLaneErr(w, lerr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "lane": res})
}

func (s *Server) laneAction(w http.ResponseWriter, r *http.Request, p *Project) {
	if noLanes(w, p) {
		return
	}
	id := r.PathValue("id")
	if !config.ValidLaneID(id) {
		writeLaneErr(w, laneErr(http.StatusBadRequest, "invalid", "invalid lane id"))
		return
	}
	var lerr *lanes.LaneError
	switch r.PathValue("action") {
	case "stop":
		lerr = p.Lanes.Stop(r.Context(), id)
	case "interrupt":
		lerr = p.Lanes.Interrupt(r.Context(), id)
	case "restart":
		lerr = p.Lanes.Restart(r.Context(), id)
	case "resume":
		if lerr = p.Lanes.Resume(r.Context(), id); lerr == nil {
			p.Lanes.MarkRestored(id)
		}
	case "forget":
		lerr = p.Lanes.Forget(r.Context(), id)
	case "terminal-app":
		lerr = p.Lanes.OpenInTerminalApp(r.Context(), id)
	case "remote-control":
		lerr = p.Lanes.ConnectRemote(r.Context(), id) // PANEL-19: only in lanes mode, only into an idle claude
	default:
		lerr = laneErr(http.StatusNotFound, "not-found", "unknown lane action")
	}
	if lerr != nil {
		writeLaneErr(w, lerr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "at": s.clock().Now().UnixMilli()})
}

// closeLane serves POST /api/p/{project}/lanes/{id}/close (PANEL-17). {"dryRun":
// true} answers the plan the page's confirmation lists; otherwise the body carries
// what that confirmation offered to remove, and the lane is closed. Nothing the body
// names is a path or a command: the worktree and branch are the lane's own, read from
// the registry and `git worktree list`.
func (s *Server) closeLane(w http.ResponseWriter, r *http.Request, p *Project) {
	if noLanes(w, p) {
		return
	}
	id := r.PathValue("id")
	if !config.ValidLaneID(id) {
		writeLaneErr(w, laneErr(http.StatusBadRequest, "invalid", "invalid lane id"))
		return
	}
	var req lanes.CloseRequest
	if !decodeStrict(w, r, &req) {
		return
	}
	if req.DryRun {
		plan, lerr := p.Lanes.ClosePlan(r.Context(), id)
		if lerr != nil {
			writeLaneErr(w, lerr)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "plan": plan})
		return
	}
	res, lerr := p.Lanes.Close(r.Context(), id, req)
	if lerr != nil {
		writeLaneErr(w, lerr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": res})
}

// removeWorktree serves POST /api/p/{project}/worktrees/remove (PANEL-18): Remove on a
// worktree with no lane. The body names the worktree by the key the page's state gave
// it, and the lane manager acts only on an exact entry of `git worktree list`; like
// Close lane, {"dryRun": true} answers the plan the confirmation lists, and otherwise
// the body carries what that confirmation offered to remove.
func (s *Server) removeWorktree(w http.ResponseWriter, r *http.Request, p *Project) {
	if noLanes(w, p) {
		return
	}
	var req lanes.RemoveWorktreeRequest
	if !decodeStrict(w, r, &req) {
		return
	}
	if req.DryRun {
		plan, lerr := p.Lanes.RemoveWorktreePlan(r.Context(), req.Worktree)
		if lerr != nil {
			writeLaneErr(w, lerr)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "plan": plan})
		return
	}
	res, lerr := p.Lanes.RemoveWorktree(r.Context(), req.Worktree, req)
	if lerr != nil {
		writeLaneErr(w, lerr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": res})
}

// pasteImage serves POST /api/p/{project}/lanes/{id}/image (PANEL-15b): an image
// dropped or pasted on the lane's terminal, as the raw body. Like every POST it needs
// the cookie and this page's Origin; a page elsewhere cannot even send it, since an
// image body is not a request the browser sends cross-origin without a preflight,
// and the panel answers no preflight. The bytes decide whether it is an image, and
// the name only names the file.
func (s *Server) pasteImage(w http.ResponseWriter, r *http.Request, p *Project) {
	if noLanes(w, p) {
		return
	}
	id := r.PathValue("id")
	if !config.ValidLaneID(id) {
		writeLaneErr(w, laneErr(http.StatusBadRequest, "invalid", "invalid lane id"))
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, lanes.MaxImageBytes))
	if err != nil {
		writeLaneErr(w, laneErr(http.StatusRequestEntityTooLarge, "too-large", "the image is over %d MB", lanes.MaxImageBytes>>20))
		return
	}
	name, _ := url.QueryUnescape(r.Header.Get("X-Filename"))
	path, lerr := p.Lanes.PasteImage(r.Context(), id, data, name)
	if lerr != nil {
		writeLaneErr(w, lerr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": path})
}

func decodeStrict(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeLaneErr(w, laneErr(http.StatusBadRequest, "malformed-request", "bad body: %v", err))
		return false
	}
	return true
}

func (s *Server) restoreAll(w http.ResponseWriter, r *http.Request, p *Project) {
	if noLanes(w, p) {
		return
	}
	var req struct {
		OverrideQuota bool `json:"overrideQuota"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	var guard func() string
	if p.Orch != nil {
		guard = p.Orch.QuotaGuard
	}
	res, lerr := p.Lanes.RestoreAll(r.Context(), guard, req.OverrideQuota)
	if lerr != nil {
		writeLaneErr(w, lerr)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": res})
}

func (s *Server) queueCancel(w http.ResponseWriter, r *http.Request, p *Project) {
	var req struct {
		Waiter string `json:"waiter"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	if p.Orch == nil || p.Orch.CancelWait == nil {
		writeLaneErr(w, laneErr(http.StatusNotFound, "not-found", "no queues"))
		return
	}
	if err := p.Orch.CancelWait(r.PathValue("id"), req.Waiter); err != nil {
		writeLaneErr(w, laneErr(http.StatusConflict, "cancel", "%v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) queueRun(w http.ResponseWriter, r *http.Request, p *Project) {
	var req struct {
		Worktree string `json:"worktree"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	if p.Orch == nil || p.Orch.RunQueue == nil {
		writeLaneErr(w, laneErr(http.StatusNotFound, "not-found", "no queues"))
		return
	}
	run, err := p.Orch.RunQueue(r.Context(), r.PathValue("id"), req.Worktree)
	if err != nil {
		writeLaneErr(w, laneErr(http.StatusConflict, "run", "%v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "run": run})
}

// Orchestration is what the HTTP layer needs from the v2 runtime.
type Orchestration struct {
	Trusted    func() bool
	QuotaGuard func() string
	CancelWait func(queue, nonce string) error
	RunQueue   func(ctx context.Context, queue, worktree string) (*types.QueueRun, error)
}

func (o *Orchestration) startGate() lanes.StartGate {
	if o == nil {
		return lanes.StartGate{}
	}
	return lanes.StartGate{Trusted: o.Trusted, QuotaGuard: o.QuotaGuard}
}
