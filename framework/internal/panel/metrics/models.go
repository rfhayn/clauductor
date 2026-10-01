package metrics

import (
	"regexp"
	"sort"
	"strings"
)

// PANEL-21: one model, one row. Spend names a model three ways: the status line's
// display name ("Opus 4.1"), an id ("claude-opus-4-1-20250805", a Bedrock or Vertex
// spelling of one) and a project command's alias ("opus"). Summed by the string, the
// same model was two rows in Cost by model, and All projects joined Alpha's "opus"
// with Beta's "Opus 4.1" as if they differed.

// modelFamilies are the names a model key is built on; any other name is kept as it
// is (lowercased for the key), since the panel cannot know what it means.
var modelFamilies = map[string]bool{"opus": true, "sonnet": true, "haiku": true, "fable": true}

var (
	modelSplit = regexp.MustCompile(`[^a-z0-9.]+`)
	// Provider wrappers around an id: "us.anthropic.…-v1:0", "…@20250805", "…[1m]".
	modelBedrock = regexp.MustCompile(`^([a-z]{2}\.)?anthropic\.`)
	modelSuffix  = regexp.MustCompile(`(-v\d+:\d+|@\d+|\[[^\]]*\])$`)
	modelDigits  = regexp.MustCompile(`^\d+$`)
)

// ModelIdentity is a model's canonical key and the label it is shown with: the family
// and version when the name says them ("opus-4.1", "Opus 4.1"), the family alone for
// an alias ("opus", "Opus"), else the name itself.
func ModelIdentity(name string) (key, label string) {
	s := strings.ToLower(strings.TrimSpace(name))
	if s == "" {
		return "", name
	}
	s = modelBedrock.ReplaceAllString(s, "")
	for modelSuffix.MatchString(s) {
		s = modelSuffix.ReplaceAllString(s, "")
	}
	family := ""
	var version []string
	for _, tok := range modelSplit.Split(s, -1) {
		switch {
		case modelFamilies[tok]:
			if family == "" {
				family = tok
			}
		case strings.Contains(tok, "."):
			// "4.1" in a display name.
			for _, p := range strings.Split(tok, ".") {
				if modelDigits.MatchString(p) && len(version) < 2 {
					version = append(version, p)
				}
			}
		case modelDigits.MatchString(tok) && len(tok) <= 2 && len(version) < 2:
			// "claude-opus-4-1", "claude-3-5-sonnet"; an 8-digit snapshot date is not one.
			version = append(version, tok)
		}
	}
	if family == "" {
		return strings.TrimSpace(strings.ToLower(name)), strings.TrimSpace(name)
	}
	label = strings.ToUpper(family[:1]) + family[1:]
	if len(version) == 0 {
		return family, label
	}
	v := strings.Join(version, ".")
	return family + "-" + v, label + " " + v
}

// modelRow is one model's spend, merged from however many names and projects.
type modelRow struct {
	key, label string
	usd        float64
	projects   []string
}

// mergeModels folds spend by model name into one row per model. An alias with no
// version ("opus") is counted with its family's version when exactly one version of
// that family is in the list (it can only have meant that one); with two or more it
// stays its own row, since which one it meant is not known. The note says what was
// folded.
func mergeModels(rows []modelRow) ([]modelRow, string) {
	byKey := map[string]*modelRow{}
	var order []string
	for _, r := range rows {
		key, label := ModelIdentity(r.label)
		m := byKey[key]
		if m == nil {
			m = &modelRow{key: key, label: label}
			byKey[key] = m
			order = append(order, key)
		}
		m.usd += r.usd
		m.projects = appendNew(m.projects, r.projects...)
	}
	versions := map[string][]string{}
	for _, k := range order {
		if fam, _, ok := strings.Cut(k, "-"); ok && modelFamilies[fam] {
			versions[fam] = append(versions[fam], k)
		}
	}
	var notes []string
	for _, k := range order {
		if !modelFamilies[k] || len(versions[k]) != 1 {
			continue
		}
		alias, into := byKey[k], byKey[versions[k][0]]
		into.usd += alias.usd
		into.projects = appendNew(into.projects, alias.projects...)
		delete(byKey, k)
		notes = append(notes, "\""+k+"\" is counted as "+into.label+", the only "+alias.label+" version here.")
	}
	out := make([]modelRow, 0, len(byKey))
	for _, k := range order {
		if m := byKey[k]; m != nil {
			out = append(out, *m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].usd != out[j].usd {
			return out[i].usd > out[j].usd
		}
		return out[i].label < out[j].label
	})
	return out, strings.Join(notes, " ")
}

func appendNew(list []string, add ...string) []string {
	for _, a := range add {
		dup := a == ""
		for _, x := range list {
			dup = dup || x == a
		}
		if !dup {
			list = append(list, a)
		}
	}
	return list
}

// modelAmounts is a project's spend by model, one row per model.
func modelAmounts(items []Amount) ([]Amount, string) {
	rows := make([]modelRow, 0, len(items))
	for _, it := range items {
		rows = append(rows, modelRow{label: it.Name, usd: it.USD})
	}
	merged, note := mergeModels(rows)
	out := make([]Amount, 0, len(merged))
	for _, m := range merged {
		out = append(out, Amount{Name: m.label, USD: round(m.usd, 2)})
	}
	return out, note
}

// combineModels is every project's spend by model, one row per model, naming the
// projects it came from.
func combineModels(have []*Metric, names []string) *Metric {
	var rows []modelRow
	for i, h := range have {
		for _, it := range toMaps(h.Items) {
			name, _ := it["name"].(string)
			usd, _ := it["usd"].(float64)
			rows = append(rows, modelRow{label: name, usd: usd, projects: []string{names[i]}})
		}
	}
	merged, note := mergeModels(rows)
	items := make([]map[string]any, 0, len(merged))
	pos := map[string]int{}
	for i, n := range names {
		pos[n] = i
	}
	for _, m := range merged {
		// The projects in the view's order, whatever order folding added them in.
		sort.SliceStable(m.projects, func(i, j int) bool { return pos[m.projects[i]] < pos[m.projects[j]] })
		items = append(items, map[string]any{"name": m.label, "usd": round(m.usd, 2), "project": strings.Join(m.projects, ", ")})
	}
	return &Metric{Items: items, N: len(items), Source: mixedSource(have), Note: note}
}
