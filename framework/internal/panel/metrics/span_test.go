package metrics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/signals"
)

// PANEL-21: a real `gh pr list --state merged --json …` recording, parsed as the
// panel parses it, gives the figures: the window filters by merge time.
func TestBuiltinFromARealGHRecording(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile(filepath.Join("..", "signals", "testdata", "merged-real.json"))
	if err != nil {
		t.Fatal(err)
	}
	prs, err := signals.ParseMergedPRs(b)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 1, 0, 36, 5, 0, time.UTC) // an hour after the newest merge
	out := Builtin(Inputs{Now: at, MergedStatus: Status{OK: true}, Merged: prs})
	f30, c30 := out["30d"]["flow.merge_frequency"], out["30d"]["flow.cycle_time"]
	if f30.Value == nil || f30.N != 5 || *f30.Value != round(5*7/30.0, 2) || f30.Source != FromBuiltin {
		t.Fatalf("30d merges %+v", f30)
	}
	if c30.Value == nil || c30.Missing != "" || *c30.Value != 0.1 || c30.N != 5 {
		t.Fatalf("30d cycle time %+v (median of the recording's 3 to 145 minutes)", c30)
	}
	// Merged after now (a clock behind gh's): not counted.
	early := Builtin(Inputs{Now: at.Add(-2 * time.Hour), MergedStatus: Status{OK: true}, Merged: prs})
	if n := early["7d"]["flow.merge_frequency"].N; n != 1 {
		t.Fatalf("merges before now: %d, want the one merged by then", n)
	}
}

// Why a gh read failed, as something to do about it.
func TestGHReason(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		`gh: exec: "gh": executable file not found in $PATH`:                                                     "gh is not installed",
		"gh: exit status 4: To get started with GitHub CLI, please run:  gh auth login":                          "gh is not authenticated",
		"gh: exit status 1: HTTP 401: Bad credentials (https://api.github.com/graphql)":                          "gh is not authenticated",
		"gh: exit status 1: no git remotes found":                                                                "no GitHub remote",
		"gh: exit status 1: none of the git remotes configured for this repository point to a known GitHub host": "no GitHub remote",
		"gh: context deadline exceeded":                                                                          "did not answer within 30 s",
		"gh: exit status 1: something else broke":                                                                "gh pr list --state merged failed: gh: exit status 1: something else broke",
	} {
		if got := GHReason(in); !strings.Contains(got, want) {
			t.Errorf("GHReason(%q) = %q, want it to say %q", in, got, want)
		}
	}
	m := Builtin(Inputs{Now: now, MergedStatus: Status{Error: "gh: exit status 4: To get started with GitHub CLI, please run:  gh auth login"}})
	if w := m["30d"]["flow.cycle_time"].Missing; !strings.Contains(w, "gh auth login") {
		t.Fatalf("the figure's reason: %q", w)
	}
	// A true zero says so, beside the value.
	m = Builtin(Inputs{Now: now, MergedStatus: Status{OK: true}})
	if f := m["30d"]["flow.merge_frequency"]; f.Value == nil || *f.Value != 0 || f.Note != "No pull request was merged in the last 30d." {
		t.Fatalf("no merges: %+v", f)
	}
}

// PANEL-21: spend per week is a rate over the days the ledger has kept, not over a
// window it has not; under a week kept, it is the spend so far, with since when.
func TestBuiltinSpendPerWeekOverKeptHistory(t *testing.T) {
	t.Parallel()
	// Today only: $11.70. Not "$2.73 a week" (11.70 × 7 / 30).
	today := map[string][]Spend{day(0): {{USD: 11.70}}}
	b := Builtin(Inputs{Now: now, Days: today})
	for _, w := range Windows {
		pw := b[w]["cost.per_week"]
		if pw.Value == nil || *pw.Value != 11.70 || pw.Span == nil || pw.Span.Since != day(0) || pw.Span.Days != 1 || pw.Series != nil {
			t.Fatalf("%s: one day kept %+v span %+v", w, pw, pw.Span)
		}
	}
	card := FlowCard(Build(Inputs{Now: now, Days: today}, Command{}))
	if it := card.Items[len(card.Items)-1]; it.Key != "cost.per_week" || it.Span == nil || it.Span.Days != 1 || *it.Value != 11.70 {
		t.Fatalf("the Flow card's Spend %+v", it)
	}
	// Ten days kept, $20 in all: $14 a week in every window at least that long, the
	// buckets before the ledger began are no data, and the note says over what.
	days := map[string][]Spend{day(9): {{USD: 10}}, day(0): {{USD: 10}}}
	b = Builtin(Inputs{Now: now, Days: days})
	pw := b["30d"]["cost.per_week"]
	if *pw.Value != 14 || pw.Span != nil || !strings.Contains(pw.Note, "10 days kept since "+day(9)) {
		t.Fatalf("30d over 10 days kept %+v", pw)
	}
	if pw.Series[0] != nil || pw.Series[9] == nil {
		t.Fatalf("30d series %v: before the ledger, nil; today's bucket, a value", pw.Series)
	}
	if pw := b["7d"]["cost.per_week"]; *pw.Value != 10 || pw.Note != "" {
		t.Fatalf("7d, a ledger older than the window: %+v", pw)
	}
	// Exactly a week kept is a rate.
	b = Builtin(Inputs{Now: now, Days: map[string][]Spend{day(6): {{USD: 7}}}})
	if pw := b["30d"]["cost.per_week"]; pw.Span != nil || *pw.Value != 7 {
		t.Fatalf("a week kept %+v", pw)
	}
}

// All projects: spend so far and a weekly rate do not add; every one short adds as
// amounts, and a mix adds the rates and names the projects left out.
func TestCombineSpans(t *testing.T) {
	t.Parallel()
	short := func(name string, v float64, since string, d int) Report {
		return Report{Name: name, Windows: map[string]map[string]*Metric{"30d": {"cost.per_week": {Value: ptr(v), Span: &Span{Since: since, Days: d}, Source: FromBuiltin}}}}
	}
	rate := Report{Name: "C", Windows: map[string]map[string]*Metric{"30d": {"cost.per_week": {Value: ptr(20), Source: FromBuiltin}}}}
	a, b := short("A", 5, day(1), 2), short("B", 3, day(3), 4)
	all := Combine([]Report{a, b}, now).Windows["30d"]["cost.per_week"]
	if *all.Value != 8 || all.Span == nil || all.Span.Since != day(3) || all.Span.Days != 4 {
		t.Fatalf("every project short %+v %+v", all, all.Span)
	}
	mix := Combine([]Report{a, rate}, now).Windows["30d"]["cost.per_week"]
	if *mix.Value != 20 || mix.Span != nil || !strings.Contains(mix.Note, "Without A") {
		t.Fatalf("a mix %+v", mix)
	}
}
