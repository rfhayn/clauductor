package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/state"
	"github.com/clauductor/clauductor/internal/panel/types"
)

// CLIHandler serves `clauductor lane close` (PANEL-57 D1, D4): one route, POST
// /api/p/{project}/lanes/{id}/close, and nothing else, for the panel's command socket.
// It checks no cookie and no Origin: the socket's file mode is the credential, so the
// route set is kept to the one verb rather than the page's whole API behind it.
//
// It closes less than the page's Close lane: a lane whose work has not ended
// (CloseHeld), or whose claude still runs on a branch not merged, is refused before
// anything changes. ctx is the panel's: a close, once begun, runs to its end even when
// the command that asked is gone. out gets one line per close.
func (s *Server) CLIHandler(ctx context.Context, out io.Writer) http.Handler {
	var outMu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/p/{project}/lanes/{id}/close", s.withProject(func(w http.ResponseWriter, r *http.Request, p *Project) {
		s.cliClose(ctx, w, r, p, func(line string) {
			outMu.Lock()
			defer outMu.Unlock()
			fmt.Fprintln(out, line)
		})
	}))
	return mux
}

// cliClose is the route's handler; logf writes the close's line to the panel's output.
func (s *Server) cliClose(ctx context.Context, w http.ResponseWriter, r *http.Request, p *Project, logf func(string)) {
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
	// The view first: the panel decides from what it shows, and nothing is asked of
	// the manager (no git, no tmux) for a lane it does not list or holds.
	v := p.Hub.View()
	i := -1
	for j, tv := range v.Terminals {
		if tv.ID == id {
			i = j
		}
	}
	if i < 0 {
		names := make([]string, 0, len(v.Terminals))
		for _, tv := range v.Terminals {
			b := tv.Branch
			if b == "" {
				b = "no branch"
			}
			names = append(names, tv.ID+" ("+b+")")
		}
		// design.md's words: `no lane "x"; this project's lanes: a (change/a), b (fix/12-b)`.
		if len(names) == 0 {
			writeLaneErr(w, laneErr(http.StatusNotFound, "not-found", "no lane %q; this project has no lanes", id))
			return
		}
		writeLaneErr(w, laneErr(http.StatusNotFound, "not-found", "no lane %q; this project's lanes: %s", id, strings.Join(names, ", ")))
		return
	}
	tv := v.Terminals[i]
	if tv.CloseHeld != "" {
		writeLaneErr(w, laneErr(http.StatusConflict, "held", "lane %s is not closed: %s", id, tv.CloseHeld))
		return
	}
	// D4: the plan under the request's context, so a command that gives up stops it.
	plan, lerr := p.Lanes.ClosePlan(r.Context(), id)
	if lerr != nil {
		writeLaneErr(w, lerr)
		return
	}
	// D2: a claude that has not exited may be closed only on a merged branch; an idle
	// claude on unmerged work is the owner's to close from the page.
	if why := unmergedLive(tv, plan); why != "" {
		writeLaneErr(w, laneErr(http.StatusConflict, "not-merged", "lane %s is not closed: %s", id, why))
		return
	}
	if req.DryRun {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "plan": plan})
		return
	}
	// Task 1.5: the last moment the command can still stop it. Past here the close runs
	// under the panel's ctx, so a command killed mid-close cannot leave half of it done.
	if r.Context().Err() != nil {
		writeLaneErr(w, laneErr(http.StatusServiceUnavailable, "cancelled", "lane %s is not closed: the request was cancelled before the close began; nothing changed", id))
		return
	}
	// Both checks again under the lane lock, on a fresh read of the lane: claude may have
	// started work during the plan, or a lane that read exited may run again (a restart).
	// live is tmux's word now: a lane whose exit or gone session the view shows, running
	// again, is held there (state.LaneView), so the merged rule below never reads a
	// stale exit.
	held := func(live types.LiveLane) string {
		t, ok := p.Hub.Lane(id, &live)
		switch {
		case !ok:
			return state.LaneGoneWords(id)
		case t.CloseHeld != "":
			return t.CloseHeld
		}
		return unmergedLive(t, plan)
	}
	res, lerr := p.Lanes.CloseIf(ctx, id, req, held)
	if lerr != nil {
		writeLaneErr(w, lerr)
		return
	}
	logf(fmt.Sprintf("lane %s (%s) closed by clauductor lane close: removed %s; kept %s", id, p.ID, listOrNothing(res.Removed), listOrNothing(res.Kept)))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": res})
}

// unmergedLive is why D2 keeps a lane whose claude has not exited: its plan's branch is
// not merged, or not known. "" for an exited claude, or a merged branch.
func unmergedLive(tv state.TermLaneView, plan lanes.ClosePlan) string {
	switch {
	case tv.Dead || plan.Merged != "":
		return ""
	case plan.Branch == "":
		return "its branch is not known"
	}
	return "its branch is not merged: " + plan.Unmerged
}

func listOrNothing(xs []string) string {
	if len(xs) == 0 {
		return "nothing"
	}
	return strings.Join(xs, ", ")
}
