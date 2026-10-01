package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/metrics"
)

// metricsPanel runs a panel whose config has a metrics command, answered by the
// fixture (or by `bad`, a payload that breaks the contract), and gh's merged list.
type metricsPanel struct {
	c       liveClient
	home    string
	root    string
	metrics atomic.Int64 // runs of the metrics command
	merged  atomic.Int64 // reads of merged pull requests
	polls   *pollCounter
}

func startMetricsPanel(t *testing.T, trust bool, payload func() []byte) *metricsPanel {
	t.Helper()
	root, home := setupProject(t)
	writeFile(t, filepath.Join(root, config.DefaultConfigRel), `{"name":"Metrics","version":4,"lanes":{"main":"orchestrator"},"tmux_socket":"`+noServerSocket()+`",
		"metrics":{"command":["sh","metrics.sh"],"refresh":"interval:3600"}}`)
	mp := &metricsPanel{home: home, root: root, polls: newPollCounter()}
	base := fakeRunner(root)
	merged := time.Now().Add(-24 * time.Hour).UTC()
	run := func(ctx context.Context, dir string, argv []string) ([]byte, error) {
		j := strings.Join(argv, " ")
		switch {
		case j == "sh metrics.sh":
			mp.metrics.Add(1)
			return payload(), nil
		case strings.HasPrefix(j, "gh pr list --state merged --limit 300 --search merged:>="):
			mp.merged.Add(1)
			return []byte(fmt.Sprintf(`[{"number":7,"title":"x","headRefName":"change/x","createdAt":%q,"mergedAt":%q}]`,
				merged.Add(-10*time.Hour).Format(time.RFC3339), merged.Format(time.RFC3339))), nil
		}
		return base(ctx, dir, argv)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan string, 1), make(chan error, 1)
	ticks := fastTicks()
	ticks.Spend, ticks.PRs = 50*time.Millisecond, 50*time.Millisecond
	go func() {
		done <- Run(ctx, Options{Project: root, Port: 0, NoOpen: true, Home: home, TrustConfig: trust, Runner: run, Ticks: ticks,
			OnPoll: mp.polls.hook, OnReady: func(u string) { ready <- u }})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("Run did not stop")
		}
	})
	select {
	case launch := <-ready:
		u, _ := url.Parse(launch)
		port, _ := strconv.Atoi(u.Port())
		mp.c = liveClient{base: "http://" + u.Host, cookie: fmt.Sprintf("clauductor_panel_%d=%s", port, u.Query().Get("t"))}
	case err := <-done:
		t.Fatalf("Run exited: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("never ready")
	}
	return mp
}

func (mp *metricsPanel) do(t *testing.T, method, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, mp.c.base+path, nil)
	req.Header.Set("Cookie", mp.c.cookie)
	req.Header.Set("Origin", mp.c.base)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func (mp *metricsPanel) report(t *testing.T, scope string) metrics.Report {
	t.Helper()
	resp := mp.do(t, "GET", "/api/p/metrics/metrics"+scope)
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("metrics: %d", resp.StatusCode)
	}
	var r metrics.Report
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatal(err)
	}
	return r
}

// PANEL-19, end to end: a trusted metrics command's figures, the merged pull
// requests gh gives while a page is in view, and spend from a status post into the
// ledger; all in the view, each marked with where it came from.
func TestMetricsEndToEnd(t *testing.T) {
	t.Parallel()
	fix, err := os.ReadFile(filepath.Join("metrics", "testdata", "metrics.json"))
	if err != nil {
		t.Fatal(err)
	}
	mp := startMetricsPanel(t, true, func() []byte { return fix })
	mp.polls.until(t, "metrics", 1)
	// Merged pull requests are read only while a page is in view.
	mp.polls.more(t, "merged", 2)
	if mp.merged.Load() != 0 {
		t.Fatal("merged pull requests were read with no page in view")
	}
	mp.do(t, "POST", "/api/seen").Body.Close()
	mp.polls.more(t, "merged", 2)
	if n := mp.merged.Load(); n != 1 {
		t.Fatalf("merged pull requests read %d times, want once (every 10 minutes)", n)
	}
	// Spend: a status post with a cost, then the ledger on disk.
	if code := mp.c.post(t, "/status", fmt.Sprintf(`{"session_id":"s1","cwd":%q,"cost":{"total_cost_usd":1.25}}`, mp.root)); code != 204 {
		t.Fatalf("status post: %d", code)
	}
	waitFor(t, "spend in the ledger", func() bool {
		d := metrics.OpenLedger(metrics.LedgerPath(mp.home, mp.root)).Days()
		return len(d) == 1
	})
	r := mp.report(t, "")
	w30, w90 := r.Windows["30d"], r.Windows["90d"]
	if !r.Command.OK || w30["flow.cycle_time"].Source != metrics.FromProject || *w30["flow.cycle_time"].Value != 7.5 {
		t.Fatalf("the project's figures: %+v %+v", r.Command, w30["flow.cycle_time"])
	}
	if m := w90["flow.merge_frequency"]; m.Source != metrics.FromBuiltin || m.N != 1 {
		t.Fatalf("the panel's merges in 90d: %+v", m)
	}
	if m := w90["cost.total"]; m.Source != metrics.FromBuiltin || m.Value == nil || *m.Value != 1.25 {
		t.Fatalf("the panel's spend in 90d: %+v", m)
	}
	if all := mp.report(t, "?scope=all"); all.Name != "All projects" || len(all.Projects) != 1 {
		t.Fatalf("scope all: %+v", all)
	}
	// The Flow card is in the state.
	v := mp.c.state(t)
	if v.Flow == nil || !v.Flow.Any || len(v.Flow.Items) != 4 {
		t.Fatalf("flow card %+v", v.Flow)
	}
}

// A payload that breaks the contract is shown as the command's error; the rest of
// the view still draws, and nothing crashes. An untrusted config runs no command.
func TestMetricsBadPayloadAndTrust(t *testing.T) {
	t.Parallel()
	mp := startMetricsPanel(t, true, func() []byte {
		return []byte(`{"version":1,"windows":{"30d":{"flow":{"change_fail_rate":{"value":140}}}}}`)
	})
	mp.polls.until(t, "metrics", 1)
	r := mp.report(t, "")
	if r.Command.OK || !strings.Contains(r.Command.Error, "change_fail_rate.value: must be at most 100") {
		t.Fatalf("a bad payload: %+v", r.Command)
	}
	if m := r.Windows["30d"]["quality.review_rounds"]; !strings.Contains(m.Missing, "The metrics command failed") {
		t.Fatalf("the reason beside a figure: %+v", m)
	}

	un := startMetricsPanel(t, false, func() []byte { t.Error("an untrusted config ran its metrics command"); return nil })
	un.polls.until(t, "metrics", 1)
	r = un.report(t, "")
	if r.Command.Trusted || !r.Command.Configured || un.metrics.Load() != 0 {
		t.Fatalf("untrusted: %+v", r.Command)
	}
	if m := r.Windows["30d"]["flow.lead_time"]; !strings.Contains(m.Missing, "until you trust panel.json") {
		t.Fatalf("the reason while untrusted: %+v", m)
	}
}
