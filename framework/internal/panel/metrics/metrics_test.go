package metrics

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "metrics.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// The fixture is the contract's example: every section, every kind of figure.
func TestParseTheFixture(t *testing.T) {
	t.Parallel()
	p, err := Parse(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	w := p.Windows["30d"]
	if p.Version != 1 || p.GeneratedAt != 1790000000 || w == nil || *w.Flow.CycleTime.Value != 7.5 || len(w.Flow.AgingWIP) != 2 ||
		*w.Cost.TotalUSD != 162.4 || *w.Cost.ByChange[0].BudgetUSD != 50 || w.Quality.ReviewerRecall[1].Pct != 78 ||
		!w.Outcomes.Hypotheses[1].Checked || p.Windows["90d"] != nil || w.Flow.LeadTime.Series[0] == nil {
		t.Fatalf("parsed %+v", p)
	}
	if p.Windows["7d"].Flow.CycleTime.Series[1] != nil {
		t.Error("a null point is a bucket with no data")
	}
}

// The fixture script prints the fixture, and a payload that breaks the contract
// when asked: what a test of the panel runs as a project's metrics command.
func TestFixtureScript(t *testing.T) {
	t.Parallel()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	out, err := exec.Command(sh, filepath.Join("testdata", "metrics.sh")).Output()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(out); err != nil {
		t.Fatalf("the fixture script's output: %v", err)
	}
	cmd := exec.Command(sh, filepath.Join("testdata", "metrics.sh"))
	cmd.Env = append(os.Environ(), "METRICS_FIXTURE=bad")
	out, err = cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(out); err == nil || !strings.Contains(err.Error(), "change_fail_rate.value: must be at most 100") {
		t.Fatalf("the bad fixture: %v", err)
	}
}

// Anything that breaks the contract is refused, whole, with the path of what is wrong.
func TestParseRefusesWhatBreaksTheContract(t *testing.T) {
	t.Parallel()
	w := func(body string) string { return `{"version":1,"windows":{"30d":` + body + `}}` }
	for why, c := range map[string]struct{ in, want string }{
		"empty":             {``, "printed nothing"},
		"not JSON":          {`- a line of text`, "metrics JSON"},
		"wrong version":     {`{"version":2,"windows":{}}`, "version 2 is not supported"},
		"no version":        {`{"windows":{}}`, "version 0 is not supported"},
		"no windows":        {`{"version":1}`, "windows is required"},
		"unknown window":    {`{"version":1,"windows":{"1y":{}}}`, "windows.1y: a window is 7d, 30d or 90d"},
		"null window":       {`{"version":1,"windows":{"7d":null}}`, "windows.7d is null"},
		"unknown key":       {w(`{"flow":{"lead":{"value":1}}}`), `unknown field "lead"`},
		"unknown top key":   {`{"version":1,"windows":{},"extra":1}`, `unknown field "extra"`},
		"negative":          {w(`{"flow":{"cycle_time":{"value":-1}}}`), "windows.30d.flow.cycle_time.value: must be a number, 0 or more"},
		"percent over 100":  {w(`{"flow":{"change_fail_rate":{"value":101}}}`), "change_fail_rate.value: must be at most 100"},
		"recall over 100":   {w(`{"quality":{"reviewer_recall":[{"model":"x","pct":120}]}}`), "reviewer_recall[0].pct: must be at most 100"},
		"series point":      {w(`{"cost":{"per_week":{"value":1,"series":[1,-2]}}}`), "per_week.series[1]"},
		"a string figure":   {w(`{"flow":{"cycle_time":{"value":"7"}}}`), "metrics JSON"},
		"unnamed amount":    {w(`{"cost":{"by_role":[{"name":" ","usd":1}]}}`), "by_role[0].name: is required"},
		"control character": {w(`{"outcomes":{"hypotheses":[{"change":"a\u001b[31m","hypothesis":"h"}]}}`), "control character"},
		"bad due date":      {w(`{"outcomes":{"hypotheses":[{"change":"a","hypothesis":"h","due":"next week"}]}}`), "due: must be a date"},
		"negative n":        {w(`{"quality":{"review_rounds":{"value":1,"n":-1}}}`), "review_rounds.n"},
		"two values":        {`{"version":1,"windows":{}} {}`, "more than one value"},
		"long text":         {w(`{"flow":{"aging_wip":[{"id":"` + strings.Repeat("x", 400) + `","age_days":1}]}}`), "longer than 300"},
	} {
		_, err := Parse([]byte(c.in))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v (want %q)", why, err, c.want)
		}
	}
	var many []string
	for i := 0; i < maxItems+1; i++ {
		many = append(many, `{"name":"x","usd":1}`)
	}
	if _, err := Parse([]byte(w(`{"cost":{"by_model":[` + strings.Join(many, ",") + `]}}`))); err == nil || !strings.Contains(err.Error(), "the limit is 500") {
		t.Errorf("a list past its limit: %v", err)
	}
	if _, err := Parse(make([]byte, MaxPayload+10)); err == nil {
		t.Error("an oversized payload was accepted")
	}
}

var now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local)

func day(d int) string { return now.AddDate(0, 0, -d).Format("2006-01-02") }

func merged(daysAgo, openHours float64) MergedPR {
	at := now.Add(-time.Duration(daysAgo * 24 * float64(time.Hour)))
	return MergedPR{Number: int(daysAgo * 10), MergedAt: at, CreatedAt: at.Add(-time.Duration(openHours * float64(time.Hour)))}
}

// Merge frequency and PR cycle time come from merged pull requests, per window.
func TestBuiltinMergesAndCycleTime(t *testing.T) {
	t.Parallel()
	in := Inputs{Now: now, MergedStatus: Status{OK: true},
		Merged: []MergedPR{merged(1, 2), merged(3, 10), merged(6, 4), merged(20, 30), merged(60, 1), merged(100, 1)}}
	b := Builtin(in)
	f7, f30, f90 := b["7d"]["flow.merge_frequency"], b["30d"]["flow.merge_frequency"], b["90d"]["flow.merge_frequency"]
	if *f7.Value != 3 || f7.N != 3 || *f30.Value != round(4*7/30.0, 2) || f90.N != 5 || f7.Source != FromBuiltin {
		t.Fatalf("frequency 7d %+v 30d %+v 90d %+v", f7, f30, f90)
	}
	if len(f7.Series) != 7 || len(f30.Series) != 10 || len(f90.Series) != 15 {
		t.Fatalf("series lengths %d %d %d", len(f7.Series), len(f30.Series), len(f90.Series))
	}
	c7 := b["7d"]["flow.cycle_time"]
	if *c7.Value != 4 || c7.N != 3 {
		t.Fatalf("7d cycle time %+v (want the median of 2, 10, 4)", c7)
	}
	// No merge in a window: frequency 0, cycle time missing with the reason.
	b = Builtin(Inputs{Now: now, MergedStatus: Status{OK: true}, Merged: []MergedPR{merged(50, 1)}})
	if *b["7d"]["flow.merge_frequency"].Value != 0 || b["7d"]["flow.cycle_time"].Missing != "No pull request was merged in the last 7d." {
		t.Fatalf("an empty window: %+v %+v", b["7d"]["flow.merge_frequency"], b["7d"]["flow.cycle_time"])
	}
	// gh failed, or has not been read: the reason, never a zero.
	b = Builtin(Inputs{Now: now, MergedStatus: Status{Error: "gh: not logged in"}})
	if m := b["30d"]["flow.merge_frequency"]; m.Value != nil || !strings.Contains(m.Missing, "gh: not logged in") {
		t.Fatalf("a failed read: %+v", m)
	}
	b = Builtin(Inputs{Now: now, MergedStatus: Status{Pending: true}})
	if m := b["30d"]["flow.cycle_time"]; m.Value != nil || !strings.Contains(m.Missing, "Reading merged pull requests") {
		t.Fatalf("not read yet: %+v", m)
	}
	// At gh's limit, the figures say so.
	b = Builtin(Inputs{Now: now, MergedStatus: Status{OK: true, Limited: true}, Merged: []MergedPR{merged(1, 1)}})
	if !strings.Contains(b["90d"]["flow.merge_frequency"].Note, "newest merges") {
		t.Fatal("a limited read does not say so")
	}
}

// Spend comes from the ledger: the window's days, broken down, with a note while the
// ledger is younger than the window.
func TestBuiltinSpend(t *testing.T) {
	t.Parallel()
	days := map[string][]Spend{
		day(0):  {{Type: "build", Model: "Opus", Branch: "change/a", USD: 3}, {Type: "fix", Model: "Sonnet", Branch: "fix/b", USD: 1}},
		day(5):  {{Type: "build", Model: "Opus", Branch: "change/a", USD: 2}},
		day(20): {{Type: "build", Model: "Opus", Branch: "change/c", USD: 10}},
	}
	b := Builtin(Inputs{Now: now, Project: "Fixture", Days: days, Budgets: map[string]float64{"change/a": 4}})
	if v := *b["7d"]["cost.total"].Value; v != 6 {
		t.Fatalf("7d total %v", v)
	}
	if v := *b["30d"]["cost.total"].Value; v != 16 || !strings.Contains(b["30d"]["cost.total"].Note, "Since "+day(20)) {
		t.Fatalf("30d total %v note %q", v, b["30d"]["cost.total"].Note)
	}
	if b["7d"]["cost.total"].Note != "" {
		t.Errorf("a ledger older than the window needs no note: %q", b["7d"]["cost.total"].Note)
	}
	pw := b["7d"]["cost.per_week"]
	if *pw.Value != 6 || len(pw.Series) != 7 || *pw.Series[6] != 28 {
		t.Fatalf("7d per week %+v (today's bucket is $4 in one day, $28 a week)", pw)
	}
	byChange := b["30d"]["cost.by_change"].Items.([]Amount)
	if byChange[0].Name != "change/c" || byChange[1].Name != "change/a" || byChange[1].BudgetUSD == nil || *byChange[1].BudgetUSD != 4 {
		t.Fatalf("by change %+v", byChange)
	}
	if roles := b["7d"]["cost.by_role"].Items.([]Amount); len(roles) != 2 || roles[0].Name != "build" || roles[0].USD != 5 {
		t.Fatalf("by role %+v", roles)
	}
	if p := b["30d"]["cost.by_project"].Items.([]Amount); len(p) != 1 || p[0].Name != "Fixture" || p[0].USD != 16 {
		t.Fatalf("by project %+v", p)
	}
	b = Builtin(Inputs{Now: now})
	if m := b["30d"]["cost.total"]; m.Value != nil || !strings.Contains(m.Missing, "No spend recorded yet") {
		t.Fatalf("no ledger: %+v", m)
	}
}

// The project's figure wins, the panel's fills in, and a figure neither has says why.
func TestBuildPrefersTheProjectAndSaysWhy(t *testing.T) {
	t.Parallel()
	p, err := Parse(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	in := Inputs{Now: now, Project: "Fixture", MergedStatus: Status{OK: true}, Merged: []MergedPR{merged(1, 5)},
		WIP: []WIPItem{{ID: "lane", Title: "change/x", AgeDays: 2}}}
	cmd := Command{Configured: true, Trusted: true}
	cmd.SetResult(p, nil, now)
	r := Build(in, cmd)
	if m := r.Windows["30d"]["flow.cycle_time"]; m.Source != FromProject || *m.Value != 7.5 {
		t.Fatalf("the project's cycle time: %+v", m)
	}
	// 7d has no aging_wip in the payload: the panel's own fills in.
	if m := r.Windows["7d"]["flow.aging_wip"]; m.Source != FromBuiltin || m.N != 1 {
		t.Fatalf("the panel's WIP: %+v", m)
	}
	// 90d is not in the payload: the panel's merges, and a reason for the rest.
	if m := r.Windows["90d"]["flow.merge_frequency"]; m.Source != FromBuiltin {
		t.Fatalf("90d merges: %+v", m)
	}
	if m := r.Windows["90d"]["quality.review_rounds"]; m.Missing != "The metrics command reports no 90d window." {
		t.Fatalf("90d review rounds: %+v", m)
	}
	if m := r.Windows["7d"]["quality.review_rounds"]; m.Missing != "The metrics command does not report quality.review rounds." {
		t.Fatalf("7d review rounds: %+v", m)
	}
	if r.Command.Generated != 1790000000000 {
		t.Errorf("generated at %d", r.Command.Generated)
	}

	for _, c := range []struct {
		cmd  Command
		want string
	}{
		{Command{}, "Only a project's metrics command reports this"},
		{Command{Configured: true}, "off until you trust panel.json"},
		{Command{Configured: true, Trusted: true, Pending: true}, "has not reported yet"},
		{Command{Configured: true, Trusted: true, Error: "exit status 1"}, "The metrics command failed: exit status 1"},
	} {
		r := Build(Inputs{Now: now}, c.cmd)
		if m := r.Windows["30d"]["quality.review_rounds"]; !strings.Contains(m.Missing, c.want) || m.Value != nil {
			t.Errorf("%+v: %q, want %q", c.cmd, m.Missing, c.want)
		}
	}
	// A failed run after a good one shows the failure, not the old figures.
	cmd.SetResult(nil, os.ErrDeadlineExceeded, now)
	if r := Build(in, cmd); r.Windows["30d"]["quality.review_rounds"].Value != nil || r.Command.OK {
		t.Error("a failed run still draws the last payload's figures")
	}
	// Every key, every window, has a value, items or a reason.
	r = Build(Inputs{Now: now, MergedStatus: Status{Pending: true}}, Command{})
	for _, w := range Windows {
		for _, k := range Keys {
			m := r.Windows[w][k]
			if m == nil || (m.Value == nil && m.Items == nil && m.Missing == "") {
				t.Errorf("%s %s: nothing and no reason: %+v", w, k, m)
			}
		}
	}
	b, _ := json.Marshal(r)
	if !strings.Contains(string(b), `"missing"`) {
		t.Error("the report carries no reasons")
	}
}

// All projects: spend and merges add up; a median is weighted and says so; lists
// name their project; cost by project has a line each.
func TestCombine(t *testing.T) {
	t.Parallel()
	mk := func(name string, freq, cyc, usd float64, n int) Report {
		w := map[string]*Metric{}
		for _, k := range Keys {
			w[k] = &Metric{Missing: "none in " + name}
		}
		w["flow.merge_frequency"] = &Metric{Value: ptr(freq), N: n, Series: []*float64{ptr(1), nil}, Source: FromBuiltin}
		w["flow.cycle_time"] = &Metric{Value: ptr(cyc), N: n, Source: FromBuiltin}
		w["cost.total"] = &Metric{Value: ptr(usd), Source: FromBuiltin}
		w["flow.aging_wip"] = &Metric{Items: []WIPItem{{ID: name + "-lane", AgeDays: 1}}, N: 1, Source: FromProject}
		return Report{Name: name, Windows: map[string]map[string]*Metric{"7d": w, "30d": w, "90d": w}}
	}
	c := Combine([]Report{mk("A", 2, 10, 5, 1), mk("B", 3, 20, 7, 3)}, now)
	w := c.Windows["30d"]
	if *w["flow.merge_frequency"].Value != 5 || *w["flow.merge_frequency"].Series[0] != 2 || w["flow.merge_frequency"].Series[1] != nil {
		t.Fatalf("summed frequency %+v", w["flow.merge_frequency"])
	}
	if *w["flow.cycle_time"].Value != 17.5 || !strings.Contains(w["flow.cycle_time"].Note, "weighted") {
		t.Fatalf("weighted cycle time %+v", w["flow.cycle_time"])
	}
	items := w["flow.aging_wip"].Items.([]map[string]any)
	if len(items) != 2 || items[1]["project"] != "B" || items[1]["id"] != "B-lane" {
		t.Fatalf("joined WIP %+v", items)
	}
	if bp := w["cost.by_project"].Items.([]Amount); len(bp) != 2 || bp[0].Name != "B" || bp[0].USD != 7 {
		t.Fatalf("cost by project %+v", bp)
	}
	if w["quality.review_rounds"].Missing != "none in A" {
		t.Fatalf("a figure no project has: %+v", w["quality.review_rounds"])
	}
	if len(c.Projects) != 2 || c.Name != "All projects" {
		t.Fatalf("combined %+v", c)
	}
	if w["flow.aging_wip"].Source != FromProject || w["flow.cycle_time"].Source != FromBuiltin {
		t.Error("sources")
	}
}

// The ledger adds what each session spent since it last posted, keeps it across a
// restart, and forgets what is older than it keeps.
func TestLedger(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "p", "spend.json")
	l := OpenLedger(path)
	obs := func(s string, usd float64, d string) Observation {
		return Observation{Session: s, TotalUSD: usd, Day: d, Type: "build", Model: "Opus", Branch: "change/a", At: now}
	}
	l.Observe(obs("s1", 2, day(0)))
	l.Observe(obs("s1", 5, day(0)))
	l.Observe(obs("s1", 4, day(0))) // a total never falls: nothing added
	l.Observe(obs("s2", 1, day(1)))
	if err := l.Save(now); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("ledger file %v %v", fi, err)
	}
	l = OpenLedger(path) // a restart
	l.Observe(obs("s1", 6, day(0)))
	d := l.Days()
	if len(d[day(0)]) != 1 || d[day(0)][0].USD != 6 || d[day(1)][0].USD != 1 {
		t.Fatalf("days %+v", d)
	}
	if ByBranch(d)["change/a"] != 7 {
		t.Fatalf("by branch %v", ByBranch(d))
	}
	l.Observe(obs("s3", 9, day(LedgerDays+5)))
	if err := l.Save(now); err != nil {
		t.Fatal(err)
	}
	if _, ok := OpenLedger(path).Days()[day(LedgerDays+5)]; ok {
		t.Error("a day past what the ledger keeps was saved")
	}
	// A missing or corrupt file starts empty.
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if len(OpenLedger(path).Days()) != 0 {
		t.Error("a corrupt ledger was read")
	}
}

// The Flow card's four figures, and whether there is anything to show.
func TestFlowCard(t *testing.T) {
	t.Parallel()
	r := Build(Inputs{Now: now, MergedStatus: Status{OK: true}, Merged: []MergedPR{merged(1, 5)}}, Command{})
	c := FlowCard(r)
	if !c.Any || len(c.Items) != 4 || c.Items[0].Key != "flow.cycle_time" || *c.Items[0].Value != 5 || c.Items[2].Missing == "" {
		t.Fatalf("card %+v", c)
	}
	if c := FlowCard(Build(Inputs{Now: now, MergedStatus: Status{Pending: true}}, Command{})); c.Any {
		t.Fatalf("a card with nothing to show says it has: %+v", c)
	}
}
