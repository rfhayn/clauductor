package metrics

import (
	"strings"
	"testing"
)

// PANEL-21: every spelling of a model has one key and one label.
func TestModelIdentity(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ in, key, label string }{
		{"Opus 4.1", "opus-4.1", "Opus 4.1"},
		{"Claude Opus 4.1", "opus-4.1", "Opus 4.1"},
		{"claude-opus-4-1", "opus-4.1", "Opus 4.1"},
		{"claude-opus-4-1-20250805", "opus-4.1", "Opus 4.1"},
		{"us.anthropic.claude-opus-4-1-20250805-v1:0", "opus-4.1", "Opus 4.1"},
		{"claude-opus-4-1@20250805", "opus-4.1", "Opus 4.1"},
		{"claude-opus-5-5[1m]", "opus-5.5", "Opus 5.5"},
		{"Sonnet 4.5 (1M context)", "sonnet-4.5", "Sonnet 4.5"},
		{"claude-3-5-sonnet-20241022", "sonnet-3.5", "Sonnet 3.5"},
		{"Claude 3.5 Sonnet", "sonnet-3.5", "Sonnet 3.5"},
		{"claude-sonnet-4", "sonnet-4", "Sonnet 4"},
		{"opus", "opus", "Opus"},
		{"Sonnet", "sonnet", "Sonnet"},
		{"haiku", "haiku", "Haiku"},
		{"(model not reported)", "(model not reported)", "(model not reported)"},
		{"gpt-4o", "gpt-4o", "gpt-4o"},
	} {
		if k, l := ModelIdentity(c.in); k != c.key || l != c.label {
			t.Errorf("%q: %q %q, want %q %q", c.in, k, l, c.key, c.label)
		}
	}
}

// One project: an id and a display name of the same model are one row; an alias
// folds into its family's only version, and says so; with two versions it stays.
func TestModelAmounts(t *testing.T) {
	t.Parallel()
	got, note := modelAmounts([]Amount{{Name: "Opus 4.1", USD: 2}, {Name: "claude-opus-4-1", USD: 1.5}, {Name: "opus", USD: 1}, {Name: "sonnet", USD: 0.5}})
	if len(got) != 2 || got[0] != (Amount{Name: "Opus 4.1", USD: 4.5}) || got[1] != (Amount{Name: "Sonnet", USD: 0.5}) {
		t.Fatalf("merged %+v", got)
	}
	if !strings.Contains(note, `"opus" is counted as Opus 4.1`) {
		t.Fatalf("note %q", note)
	}
	got, note = modelAmounts([]Amount{{Name: "Opus 4.1", USD: 2}, {Name: "Opus 4", USD: 1}, {Name: "opus", USD: 1}})
	if len(got) != 3 || note != "" {
		t.Fatalf("an alias with two versions to choose from was folded: %+v %q", got, note)
	}
}

// All projects: Alpha's command says "opus" and "sonnet", Beta's status line says
// "Opus 4.1": one row per model, naming both projects where both spent on it.
func TestCombineModelsAcrossSources(t *testing.T) {
	t.Parallel()
	mk := func(name, src string, items []Amount) Report {
		w := map[string]*Metric{}
		for _, k := range Keys {
			w[k] = &Metric{Missing: "none in " + name}
		}
		w["cost.by_model"] = &Metric{Items: items, N: len(items), Source: src}
		return Report{Name: name, Windows: map[string]map[string]*Metric{"7d": w, "30d": w, "90d": w}}
	}
	alpha := mk("Alpha", FromProject, []Amount{{Name: "opus", USD: 3}, {Name: "sonnet", USD: 1}})
	beta := mk("Beta", FromBuiltin, []Amount{{Name: "Opus 4.1", USD: 2}})
	c := Combine([]Report{alpha, beta}, now)
	m := c.Windows["30d"]["cost.by_model"]
	items := m.Items.([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("one row per model, got %+v", items)
	}
	if items[0]["name"] != "Opus 4.1" || items[0]["usd"] != 5.0 || items[0]["project"] != "Alpha, Beta" {
		t.Fatalf("opus row %+v", items[0])
	}
	if items[1]["name"] != "Sonnet" || items[1]["usd"] != 1.0 || items[1]["project"] != "Alpha" {
		t.Fatalf("sonnet row %+v", items[1])
	}
	if m.Source != "mixed" || !strings.Contains(m.Note, "Opus 4.1") || m.N != 2 {
		t.Fatalf("combined metric %+v", m)
	}
}

// The panel's own spend by model is one row per model too.
func TestBuiltinSpendByModelIsOneRowPerModel(t *testing.T) {
	t.Parallel()
	days := map[string][]Spend{day(0): {{Model: "Opus 4.1", USD: 1}, {Model: "claude-opus-4-1", USD: 2}, {USD: 1}}}
	got := Builtin(Inputs{Now: now, Days: days})["7d"]["cost.by_model"]
	items := got.Items.([]Amount)
	if len(items) != 2 || items[0] != (Amount{Name: "Opus 4.1", USD: 3}) || items[1].Name != "(model not reported)" || got.N != 2 {
		t.Fatalf("%+v", got)
	}
}
