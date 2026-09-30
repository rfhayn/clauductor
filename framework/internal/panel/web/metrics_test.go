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

// PANEL-19: the page wires the view: a Metrics button right after Activity that
// controls the view, the view's tablist of four, the fetch of this project's route,
// every figure the report has named in the page, and only textContent for its data.
func TestMetricsViewWiring(t *testing.T) {
	t.Parallel()
	b, err := webFS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	act, mb := strings.Index(page, `id="activitybtn"`), strings.Index(page, `id="metricsbtn"`)
	if act < 0 || mb < act || strings.Count(page[act:mb], "<button") != 1 {
		t.Error("the Metrics button is not right after Activity")
	}
	for _, want := range []string{`aria-controls="mview"`, `id="mview"`, `id="mvbar"`, `id="mvbody"`, `id="mviewclose"`} {
		if !strings.Contains(page, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	js := readWeb(t, "panel.js")
	for _, want := range []string{`api("/metrics")`, `"?scope=all"`, `setAttribute("role", "tablist")`, `"aria-pressed"`,
		`["flow", "Flow"], ["cost", "Cost"], ["quality", "Quality"], ["outcomes", "Outcomes"]`, `const M_RANGES = ["7d", "30d", "90d"]`} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %s", want)
		}
	}
	for _, k := range metrics.Keys {
		if !strings.Contains(js, `"`+k+`": [`) {
			t.Errorf("panel.js does not name the figure %s", k)
		}
	}
	// The view's code puts no data through innerHTML.
	start, end := strings.Index(js, "// ---- Metrics (PANEL-19)"), strings.Index(js, `$("mview").addEventListener("keydown"`)
	if start < 0 || end < start || strings.Contains(js[start:end], "innerHTML") {
		t.Error("the Metrics view's code must build nodes with textContent only")
	}
}
