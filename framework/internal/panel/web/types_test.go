package web

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The appearance contract (docs/panel.md, "Appearance"; PANEL-11): theme, type system
// and size are independent. A theme (themes.css) sets colour, surface, border and
// density; a type system (types.css) sets the faces and type sizes and nothing else;
// any theme works with any type system. These tests keep the two layers apart.

var (
	typeSel    = regexp.MustCompile(`^\[data-type="([a-z]+)"\]$`)
	typeIDLine = regexp.MustCompile(`\{\s*id:\s*"([a-z]+)"[^}]*\}`)
)

// parseTypes reads types.css: type id → token → value. ":root" is the default type
// system's block and is not a type system of its own.
func parseTypes(t *testing.T) map[string]map[string]string {
	t.Helper()
	out := map[string]map[string]string{}
	css := cssComment.ReplaceAllString(readWeb(t, "types.css"), "")
	if regexp.MustCompile(`@(media|supports|container|layer)`).MatchString(css) {
		t.Error("types.css has a conditional group rule; type blocks must be top level")
	}
	for _, m := range cssBlock.FindAllStringSubmatch(css, -1) {
		sels := strings.TrimSpace(m[1])
		if strings.HasPrefix(sels, "@") {
			continue
		}
		decls := map[string]string{}
		for _, d := range strings.Split(m[2], ";") {
			k, v, ok := strings.Cut(d, ":")
			if k = strings.TrimSpace(k); ok && strings.HasPrefix(k, "--") {
				decls[k[2:]] = strings.TrimSpace(v)
			}
		}
		for _, s := range strings.Split(sels, ",") {
			s = strings.TrimSpace(s)
			if s == ":root" {
				continue
			}
			sm := typeSel.FindStringSubmatch(s)
			if sm == nil {
				t.Fatalf("types.css: selector %q is not a type system", s)
			}
			if out[sm[1]] == nil {
				out[sm[1]] = map[string]string{}
			}
			for k, v := range decls {
				out[sm[1]][k] = v
			}
		}
	}
	return out
}

// declaredTypes reads TYPES from theme.js, and each theme's pairing.
func declaredTypes(t *testing.T) (ids []string, pairs map[string]string) {
	t.Helper()
	js := readWeb(t, "theme.js")
	block := func(name string) string {
		start := strings.Index(js, "const "+name+" = [")
		if start < 0 {
			t.Fatalf("theme.js: no %s array", name)
		}
		end := strings.Index(js[start:], "];")
		return js[start : start+end]
	}
	for _, m := range typeIDLine.FindAllStringSubmatch(block("TYPES"), -1) {
		ids = append(ids, m[1])
	}
	pairs = map[string]string{}
	for _, m := range regexp.MustCompile(`id:\s*"([a-z]+)"[^}]*type:\s*"([a-z]+)"`).FindAllStringSubmatch(block("THEMES"), -1) {
		pairs[m[1]] = m[2]
	}
	if len(ids) < 2 {
		t.Fatalf("theme.js declares %d type systems", len(ids))
	}
	return ids, pairs
}

// typeTokenSet is every token any type system defines.
func typeTokenSet(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, m := range parseTypes(t) {
		for k := range m {
			out[k] = true
		}
	}
	return out
}

func TestTypeSystemsAreCompleteAndApartFromThemes(t *testing.T) {
	types := parseTypes(t)
	ids, pairs := declaredTypes(t)
	declared := map[string]bool{}
	for _, id := range ids {
		declared[id] = true
		if types[id] == nil {
			t.Errorf("type system %q: no [data-type] block in types.css", id)
		}
	}
	for id := range types {
		if !declared[id] {
			t.Errorf("types.css has type system %q, which theme.js does not offer", id)
		}
	}
	// Every theme pairs with a type system that exists.
	for _, th := range declaredThemes(t) {
		if p, ok := pairs[th.id]; !ok || !declared[p] {
			t.Errorf("theme %q pairs with type system %q, which is not declared", th.id, p)
		}
	}
	// Every type system defines every type token, and the faces it names are real.
	all := typeTokenSet(t)
	for _, want := range []string{"font-display", "font-body", "font-mono", "font-term", "font-label", "type-adjust", "num", "term-size"} {
		if !all[want] {
			t.Errorf("no type system defines --%s", want)
		}
	}
	for _, id := range ids {
		var missing []string
		for k := range all {
			if _, ok := types[id][k]; !ok {
				missing = append(missing, k)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Errorf("type system %s is missing: %s", id, strings.Join(missing, ", "))
		}
	}
	// The layers stay apart: no theme sets a type token, and no type system sets a
	// theme token. A token in both would make the result depend on the order of the
	// two style sheets, and a theme could no longer take any type system.
	tb := parseThemes(t, readWeb(t, "themes.css"))
	for id, m := range tb.shape {
		for k := range m {
			if all[k] {
				t.Errorf("theme %s sets --%s, a type token (types.css)", id, k)
			}
		}
	}
	for id, m := range tb.colour {
		for k := range m {
			if all[k] {
				t.Errorf("theme × mode %s sets --%s, a type token (types.css)", id, k)
			}
		}
	}
	faces := map[string]bool{}
	for _, m := range regexp.MustCompile(`font-family:\s*"([^"]+)"`).FindAllStringSubmatch(readWeb(t, "types.css"), -1) {
		faces[m[1]] = true
	}
	for id, m := range types {
		for _, k := range []string{"font-display", "font-body", "font-mono", "font-term"} {
			first := strings.Trim(strings.TrimSpace(strings.Split(m[k], ",")[0]), `"`)
			if strings.HasPrefix(first, "var(") {
				continue
			}
			if !faces[first] {
				t.Errorf("type system %s: --%s starts with %q, which no @font-face in types.css declares", id, k, first)
			}
		}
	}
}
