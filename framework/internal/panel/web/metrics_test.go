package web

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/metrics"
)

// PANEL-19: the Metrics view is a project's route like the others: the cookie, a
// project the panel serves, and nothing but GET. ?scope=all combines every project.
func TestMetricsRoute(t *testing.T) {
	t.Parallel()
	s, _ := twoProjects(t)
	for i, p := range s.Projects {
		name, usd := p.Name, float64(10*(i+1))
		p.Metrics = func() metrics.Report {
			w := map[string]*metrics.Metric{}
			for _, k := range metrics.Keys {
				w[k] = &metrics.Metric{Missing: "not in " + name}
			}
			w["cost.total"] = &metrics.Metric{Value: &usd, Source: metrics.FromBuiltin}
			return metrics.Report{Name: name, Windows: map[string]map[string]*metrics.Metric{"7d": w, "30d": w, "90d": w}}
		}
	}
	if w := do(s, "GET", "/api/p/b/metrics", ""); w.Code != 401 {
		t.Errorf("without the cookie: %d", w.Code)
	}
	if w := do(s, "GET", "/api/p/nope/metrics", "", withCookie(s)); w.Code != 404 {
		t.Errorf("an unknown project: %d", w.Code)
	}
	if w := do(s, "GET", "/api/p/b/metrics?scope=everything", "", withCookie(s)); w.Code != 400 {
		t.Errorf("a bad scope: %d", w.Code)
	}
	if w := do(s, "POST", "/api/p/b/metrics", "{}", withCookie(s), withHeader("Origin", "http://127.0.0.1:4393")); w.Code != 405 && w.Code != 404 {
		t.Errorf("a POST: %d", w.Code)
	}
	read := func(target string) metrics.Report {
		w := do(s, "GET", target, "", withCookie(s))
		if w.Code != 200 {
			t.Fatalf("GET %s: %d %s", target, w.Code, w.Body)
		}
		var r metrics.Report
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := read("/api/p/b/metrics"); r.Name != "Project B" || *r.Windows["30d"]["cost.total"].Value != 20 {
		t.Fatalf("project b: %+v", r)
	}
	all := read("/api/p/a/metrics?scope=all")
	if all.Name != "All projects" || len(all.Projects) != 2 || *all.Windows["7d"]["cost.total"].Value != 30 ||
		!strings.HasPrefix(all.Windows["7d"]["quality.review_rounds"].Missing, "not in") {
		t.Fatalf("all: %+v", all)
	}
	s.Projects[1].Metrics = nil
	if w := do(s, "GET", "/api/p/b/metrics", "", withCookie(s)); w.Code != 404 {
		t.Errorf("a project without metrics: %d", w.Code)
	}
}
