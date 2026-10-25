package install

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/clauductor/clauductor/internal/panel/lanes"
)

// LaneCloseAnswer is the panel's answer to a close asked on its command socket: the
// plan for a dry run, the result for a close.
type LaneCloseAnswer struct {
	Plan   *lanes.ClosePlan
	Result *lanes.CloseResult
}

// CloseLane asks the panel listening on socket to close lane id of project (PANEL-57
// D1, D3): POST /api/p/{project}/lanes/{id}/close with req as the body, the page's own
// lanes.CloseRequest, so the consent goes under the names the route's strict decoder
// takes (a plan's DeleteBranch is sent as Branch). req.DryRun asks for the plan alone.
//
// A refusal is a *lanes.LaneError carrying the status, the code and the message as
// the panel wrote them; any other error is the socket not answering, or an answer
// that cannot be read. It is the one client `clauductor lane close` uses, so the
// panel's tests drive the real route through it.
func CloseLane(ctx context.Context, socket, project, id string, req lanes.CloseRequest) (LaneCloseAnswer, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return LaneCloseAnswer{}, err
	}
	// The host is a placeholder: the socket's handler serves one route, by path alone.
	u := "http://panel/api/p/" + url.PathEscape(project) + "/lanes/" + url.PathEscape(id) + "/close"
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return LaneCloseAnswer{}, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	var connected atomic.Bool
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			c, err := d.DialContext(ctx, "unix", socket)
			connected.Store(err == nil)
			return c, err
		},
		DisableKeepAlives: true,
	}
	defer tr.CloseIdleConnections()
	// No redirect is followed: the route answers, or refuses, itself. The mux's 301 for a
	// path it cleans (an id of "..") would otherwise come back as a GET of another path.
	client := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(hreq)
	switch {
	case err != nil && !connected.Load():
		return LaneCloseAnswer{}, fmt.Errorf("the panel's socket %s does not answer: %w", socket, err)
	case err != nil:
		// Sent, so not "nothing happened": a close the panel had begun runs to its end
		// there (D4), whatever became of this request.
		return LaneCloseAnswer{}, fmt.Errorf("the panel's answer did not arrive (%w); a close it had begun, it finishes: see the lane on the panel's page", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return LaneCloseAnswer{}, fmt.Errorf("reading the panel's answer: %w", err)
	}
	var ans struct {
		OK     bool               `json:"ok"`
		Error  string             `json:"error"`
		Code   string             `json:"code"`
		Plan   *lanes.ClosePlan   `json:"plan"`
		Result *lanes.CloseResult `json:"result"`
	}
	if jerr := json.Unmarshal(raw, &ans); jerr != nil {
		if resp.StatusCode == http.StatusOK {
			return LaneCloseAnswer{}, fmt.Errorf("the panel's answer cannot be read: %v", jerr)
		}
		// Not the route's JSON: the mux's own 404 or 405, or a malformed project id.
		msg := strings.TrimSpace(string(raw))
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return LaneCloseAnswer{}, &lanes.LaneError{Status: resp.StatusCode, Msg: msg}
	}
	if resp.StatusCode != http.StatusOK || !ans.OK {
		msg := ans.Error
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return LaneCloseAnswer{}, &lanes.LaneError{Status: resp.StatusCode, Code: ans.Code, Msg: msg}
	}
	switch {
	case req.DryRun && ans.Plan == nil:
		return LaneCloseAnswer{}, fmt.Errorf("the panel answered the dry run with no plan")
	case !req.DryRun && ans.Result == nil:
		return LaneCloseAnswer{}, fmt.Errorf("the panel answered the close with no result")
	}
	return LaneCloseAnswer{Plan: ans.Plan, Result: ans.Result}, nil
}
