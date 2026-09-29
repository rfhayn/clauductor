package web

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The theme contract (docs/panel.md, "Themes"). Every theme × mode must define every
// token the components use, and meet the contrast floors below. The lists are derived,
// not typed: the theme ids come from theme.js (the picker's own list), the tokens from
// what panel.css references and what any theme defines.

type themeBlocks struct {
	shape  map[string]map[string]string // theme id → token → value (both modes)
	colour map[string]map[string]string // "id/mode" → token → value
}

var (
	cssComment  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssBlock    = regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`)
	themeSel    = regexp.MustCompile(`^\[data-theme="([a-z]+)"\](?:\[data-mode="(light|dark)"\])?$`)
	varRef      = regexp.MustCompile(`var\(--([a-z0-9-]+)`)
	themeIDLine = regexp.MustCompile(`\{\s*id:\s*"([a-z]+)"[^}]*\}`)
)

func readWeb(t *testing.T, name string) string {
	t.Helper()
	b, err := webFS.ReadFile("static/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// parseThemes reads themes.css. A selector list may name several blocks (":root" is
// the console default and is not a theme of its own).
func parseThemes(t *testing.T, css string) themeBlocks {
	t.Helper()
	tb := themeBlocks{shape: map[string]map[string]string{}, colour: map[string]map[string]string{}}
	css = cssComment.ReplaceAllString(css, "")
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
			sm := themeSel.FindStringSubmatch(s)
			if sm == nil {
				t.Fatalf("themes.css: selector %q is neither a theme nor a theme × mode", s)
			}
			target := tb.shape
			key := sm[1]
			if sm[2] != "" {
				target, key = tb.colour, sm[1]+"/"+sm[2]
			}
			if target[key] == nil {
				target[key] = map[string]string{}
			}
			for k, v := range decls {
				target[key][k] = v
			}
		}
	}
	return tb
}

type themeDecl struct {
	id  string
	aaa bool
}

func declaredThemes(t *testing.T) []themeDecl {
	t.Helper()
	js := readWeb(t, "theme.js")
	start := strings.Index(js, "const THEMES = [")
	end := strings.Index(js[start:], "];")
	if start < 0 || end < 0 {
		t.Fatal("theme.js: no THEMES array")
	}
	var out []themeDecl
	for _, m := range themeIDLine.FindAllString(js[start:start+end], -1) {
		id := themeIDLine.FindStringSubmatch(m)[1]
		out = append(out, themeDecl{id: id, aaa: strings.Contains(m, "aaa: true")})
	}
	if len(out) < 2 {
		t.Fatalf("theme.js declares %d themes", len(out))
	}
	return out
}

type rgba struct{ r, g, b, a float64 }

var hexColour = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
var fnColour = regexp.MustCompile(`^rgba?\(\s*([0-9.]+)\s*,\s*([0-9.]+)\s*,\s*([0-9.]+)\s*(?:,\s*([0-9.]+)\s*)?\)$`)

func parseColour(v string) (rgba, error) {
	v = strings.TrimSpace(v)
	if v == "transparent" {
		return rgba{0, 0, 0, 0}, nil
	}
	if m := hexColour.FindStringSubmatch(v); m != nil {
		h := m[1]
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		n, _ := strconv.ParseUint(h, 16, 32)
		return rgba{float64(n >> 16 & 255), float64(n >> 8 & 255), float64(n & 255), 1}, nil
	}
	if m := fnColour.FindStringSubmatch(v); m != nil {
		c := rgba{a: 1}
		c.r, _ = strconv.ParseFloat(m[1], 64)
		c.g, _ = strconv.ParseFloat(m[2], 64)
		c.b, _ = strconv.ParseFloat(m[3], 64)
		if m[4] != "" {
			c.a, _ = strconv.ParseFloat(m[4], 64)
		}
		return c, nil
	}
	return rgba{}, fmt.Errorf("not a colour: %q", v)
}

// over composites c on an opaque background.
func over(c, bg rgba) rgba {
	return rgba{c.r*c.a + bg.r*(1-c.a), c.g*c.a + bg.g*(1-c.a), c.b*c.a + bg.b*(1-c.a), 1}
}

func luminance(c rgba) float64 {
	lin := func(v float64) float64 {
		v /= 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.r) + 0.7152*lin(c.g) + 0.0722*lin(c.b)
}

// contrast is the WCAG 2 contrast ratio.
func contrast(a, b rgba) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// lab converts to CIELAB (D65) for the colour-difference check.
func lab(c rgba) (float64, float64, float64) {
	lin := func(v float64) float64 {
		v /= 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	r, g, b := lin(c.r), lin(c.g), lin(c.b)
	x := (0.4124*r + 0.3576*g + 0.1805*b) / 0.95047
	y := 0.2126*r + 0.7152*g + 0.0722*b
	z := (0.0193*r + 0.1192*g + 0.9505*b) / 1.08883
	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116
	}
	return 116*f(y) - 16, 500 * (f(x) - f(y)), 200 * (f(y) - f(z))
}

func deltaE(a, b rgba) float64 {
	l1, a1, b1 := lab(a)
	l2, a2, b2 := lab(b)
	return math.Sqrt((l1-l2)*(l1-l2) + (a1-a2)*(a1-a2) + (b1-b2)*(b1-b2))
}

// themeTokens is one theme × mode's complete token set: its shape block plus its
// colour block, as the browser sees them on <html>.
func themeTokens(tb themeBlocks, id, mode string) map[string]string {
	out := map[string]string{}
	for k, v := range tb.shape[id] {
		out[k] = v
	}
	for k, v := range tb.colour[id+"/"+mode] {
		out[k] = v
	}
	return out
}

func TestThemesDefineEveryToken(t *testing.T) {
	tb := parseThemes(t, readWeb(t, "themes.css"))
	themes := declaredThemes(t)

	// Every theme declared in theme.js has blocks, and every block names a declared theme.
	declared := map[string]bool{}
	for _, th := range themes {
		declared[th.id] = true
		if tb.shape[th.id] == nil {
			t.Errorf("theme %q: no [data-theme] block in themes.css", th.id)
		}
		for _, m := range []string{"light", "dark"} {
			if tb.colour[th.id+"/"+m] == nil {
				t.Errorf("theme %q: no %s block in themes.css", th.id, m)
			}
		}
	}
	for k := range tb.shape {
		if !declared[k] {
			t.Errorf("themes.css has theme %q, which theme.js does not offer", k)
		}
	}

	// Required: every token any theme defines, and every token the components use.
	shapeReq, colourReq := map[string]bool{}, map[string]bool{}
	for _, m := range tb.shape {
		for k := range m {
			shapeReq[k] = true
		}
	}
	for _, m := range tb.colour {
		for k := range m {
			colourReq[k] = true
		}
	}
	used := map[string]bool{}
	for _, f := range []string{"panel.css", "themes.css"} {
		for _, m := range varRef.FindAllStringSubmatch(cssComment.ReplaceAllString(readWeb(t, f), ""), -1) {
			used[m[1]] = true
		}
	}
	// The tokens panel.js reads for the terminals: --x literals, v("x") in termTheme,
	// and the 16 ANSI colours its loop builds.
	js := readWeb(t, "panel.js")
	for _, m := range regexp.MustCompile(`"--([a-z0-9-]+)"|\bv\("([a-z0-9-]+)"\)`).FindAllStringSubmatch(js, -1) {
		used[m[1]+m[2]] = true
	}
	if !used["term-bg"] || !used["term-size"] {
		t.Fatal("did not find the terminal tokens panel.js reads; the parser is broken")
	}
	if strings.Contains(js, `"ansi-" +`) {
		for i := 0; i < 16; i++ {
			used["ansi-"+strconv.Itoa(i)] = true
		}
	}
	// A theme block inside @media would apply only sometimes, and the parser would not
	// see the condition: theme tokens live at the top level of themes.css only.
	if regexp.MustCompile(`@(media|supports|container|layer)`).MatchString(cssComment.ReplaceAllString(readWeb(t, "themes.css"), "")) {
		t.Error("themes.css has a conditional group rule; theme blocks must be top level")
	}
	if strings.Contains(cssComment.ReplaceAllString(readWeb(t, "panel.css"), ""), "data-theme") {
		t.Error("panel.css sets per-theme rules; themes are tokens in themes.css only")
	}
	if len(used) < 20 {
		t.Fatalf("found only %d var() references; the parser is broken", len(used))
	}
	for _, th := range themes {
		for _, mode := range []string{"light", "dark"} {
			all := themeTokens(tb, th.id, mode)
			var missing []string
			for k := range shapeReq {
				if _, ok := tb.shape[th.id][k]; !ok {
					missing = append(missing, k+" (shape)")
				}
			}
			for k := range colourReq {
				if _, ok := tb.colour[th.id+"/"+mode][k]; !ok {
					missing = append(missing, k)
				}
			}
			for k := range used {
				if _, ok := all[k]; !ok && !shapeReq[k] && !colourReq[k] {
					missing = append(missing, k+" (used by panel.css or panel.js, defined by no theme)")
				}
			}
			sort.Strings(missing)
			if len(missing) > 0 {
				t.Errorf("%s/%s is missing: %s", th.id, mode, strings.Join(missing, ", "))
			}
		}
	}
	// The terminal palette has all 16 ANSI colours.
	for i := 0; i < 16; i++ {
		if !colourReq["ansi-"+strconv.Itoa(i)] {
			t.Errorf("no theme defines --ansi-%d", i)
		}
	}
}

// ansiLabelPairs are {foreground, background} ANSI indices: black on green, yellow and
// cyan, white on red, blue and magenta, and their bright variants. Black on red is
// not here: with every colour also held to 3:1 on a dark terminal background, no red
// can carry both black and white text at 4.5:1, so red, blue and magenta take white.
var ansiLabelPairs = [][2]int{
	{0, 2}, {0, 3}, {0, 6}, {0, 10}, {0, 11}, {0, 14},
	{7, 1}, {7, 4}, {7, 5},
	{15, 1}, {15, 4}, {15, 5}, {15, 9}, {15, 12}, {15, 13},
}

// aaaLabelPairs: in an aaa theme every colour is at least 7:1 on the black background,
// so it is light, and two light colours can differ by at most 21/7 = 3:1. White on red,
// blue or magenta is therefore impossible there; black carries every label instead, at
// 4.5:1 (7:1 would need a colour lighter than white).
var aaaLabelPairs = [][2]int{
	{0, 1}, {0, 2}, {0, 3}, {0, 4}, {0, 5}, {0, 6},
	{0, 9}, {0, 10}, {0, 11}, {0, 12}, {0, 13}, {0, 14},
}

// --term-min-contrast is xterm's minimumContrastRatio. It is the only guard for colour
// pairs the palette cannot promise (in an aaa theme, white on red is at most 3:1), so
// every theme sets one, at least 4.5, and an aaa theme at least 7.
func TestThemeTerminalMinimumContrast(t *testing.T) {
	tb := parseThemes(t, readWeb(t, "themes.css"))
	if !strings.Contains(readWeb(t, "panel.js"), "minimumContrastRatio") {
		t.Error("panel.js never sets xterm's minimumContrastRatio")
	}
	// xterm applies a lifted colour through span.setAttribute("style"), which the CSP
	// blocks; xterm-style.js routes it (TestXtermStyleRouteIsNarrow checks how).
	for _, th := range declaredThemes(t) {
		v, err := strconv.ParseFloat(tb.shape[th.id]["term-min-contrast"], 64)
		want := 4.5
		if th.aaa {
			want = 7
		}
		if err != nil || v < want || v > 21 {
			t.Errorf("%s: --term-min-contrast is %q, want a number in [%.1f, 21]", th.id, tb.shape[th.id]["term-min-contrast"], want)
		}
	}
}

func TestThemeContrast(t *testing.T) {
	tb := parseThemes(t, readWeb(t, "themes.css"))
	var report []string
	for _, th := range declaredThemes(t) {
		for _, mode := range []string{"light", "dark"} {
			name := th.id + "/" + mode
			tok := tb.colour[name]
			col := func(k string) rgba {
				c, err := parseColour(tok[k])
				if err != nil {
					t.Errorf("%s --%s: %v", name, k, err)
				}
				return c
			}
			surface, panel, panel2 := col("surface"), col("panel"), col("panel-2")
			for _, bg := range []rgba{surface, panel, panel2} {
				if bg.a != 1 {
					t.Errorf("%s: surfaces must be opaque", name)
				}
			}
			textMin := 4.5
			if th.aaa {
				textMin = 7
			}
			worst := map[string]float64{}
			check := func(label string, fg, bg rgba, min float64) {
				r := contrast(over(fg, bg), bg)
				if w, ok := worst[label]; !ok || r < w {
					worst[label] = r
				}
				if r < min {
					t.Errorf("%s: %s is %.2f:1, want ≥ %.1f:1", name, label, r, min)
				}
			}
			for _, bg := range []rgba{surface, panel, panel2} {
				check("text", col("text"), bg, textMin)
				check("dim text", col("text-dim"), bg, textMin)
				check("accent as text", col("accent"), bg, textMin)
				check("focus ring", col("focus"), bg, 3)
				// A state colour is also text (queue holder, PR checks, "waiting: …").
				for _, s := range []string{"go", "hold", "stop"} {
					check(s+" as text", col(s), bg, textMin)
				}
				check("idle marker", col("idle"), bg, 3)
			}
			check("text on primary button", col("accent-ink"), col("accent"), textMin)
			// Soft tints carry body text and the state's own colour. Cards are tinted over
			// --panel; banners and bars (.banner, .restore, .warnbar) over --surface.
			for _, s := range []string{"go", "hold", "stop", "accent"} {
				for _, base := range []rgba{surface, panel} {
					soft := over(col(s+"-soft"), base)
					check("text on "+s+"-soft", col("text"), soft, textMin)
					check("dim text on "+s+"-soft", col("text-dim"), soft, textMin)
					check(s+" on its soft tint", col(s), soft, textMin)
				}
			}
			if th.aaa {
				check("rule (line) vs panel", col("line"), panel, 3)
			}
			term := col("term-bg")
			check("terminal foreground", col("term-fg"), term, 7)
			check("terminal cursor", col("term-cursor"), term, 3)
			// Every ANSI colour is terminal text. An aaa theme holds colours 1-15 to 7:1;
			// its black (0) stays at 3:1, because black is the label colour below and 7:1
			// on a black background would leave it no room to contrast with anything.
			for i := 0; i < 16; i++ {
				min := 3.0
				if th.aaa && i > 0 {
					min = 7
				}
				check("ANSI colour", col("ansi-"+strconv.Itoa(i)), term, min)
			}
			// Labels a TUI draws as one ANSI colour on another: test runners' PASS/FAIL
			// badges, diff and status chips.
			pairs := ansiLabelPairs
			if th.aaa {
				pairs = aaaLabelPairs
			}
			for _, p := range pairs {
				fg, bg := "ansi-"+strconv.Itoa(p[0]), "ansi-"+strconv.Itoa(p[1])
				check("ANSI label pair", col(fg), col(bg), 4.5)
				if r := contrast(col(fg), col(bg)); r < 4.5 {
					t.Logf("%s: --%s on --%s is %.2f:1", name, fg, bg, r)
				}
			}
			// The four lane states stay apart in colour (they also differ in shape and label).
			states := []string{"go", "hold", "stop", "idle"}
			minDE := math.Inf(1)
			for i := range states {
				for j := i + 1; j < len(states); j++ {
					d := deltaE(col(states[i]), col(states[j]))
					minDE = math.Min(minDE, d)
					if d < 20 {
						t.Errorf("%s: --%s and --%s are too alike (ΔE %.1f, want ≥ 20)", name, states[i], states[j], d)
					}
				}
			}
			report = append(report, fmt.Sprintf("%-18s text %5.2f  dim %5.2f  accent %5.2f  focus %5.2f  term %5.2f  ansi %5.2f  pairs %5.2f  stateΔE %4.1f",
				name, worst["text"], worst["dim text"], worst["accent as text"], worst["focus ring"], worst["terminal foreground"], worst["ANSI colour"], worst["ANSI label pair"], minDE))
		}
	}
	t.Log("worst ratio per theme × mode, over surface, panel and panel-2:\n" + strings.Join(report, "\n"))
}

// Every face a theme declares is embedded, every embedded face is used, each family
// ships its licence, and the fonts stay small (they are in the binary).
func TestThemeFontsAreEmbeddedAndLicensed(t *testing.T) {
	css := cssComment.ReplaceAllString(readWeb(t, "themes.css"), "")
	used := map[string]bool{}
	for _, m := range regexp.MustCompile(`url\("fonts/([^"]+)"\)`).FindAllStringSubmatch(css, -1) {
		used[m[1]] = true
		if _, err := webFS.ReadFile("static/fonts/" + m[1]); err != nil {
			t.Errorf("themes.css loads fonts/%s, which is not embedded", m[1])
		}
	}
	entries, err := webFS.ReadDir("static/fonts")
	if err != nil {
		t.Fatal(err)
	}
	var total int
	licences := map[string]bool{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "OFL-") {
			licences[strings.TrimSuffix(strings.TrimPrefix(e.Name(), "OFL-"), ".txt")] = true
		}
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".woff2") {
			continue
		}
		info, _ := e.Info()
		total += int(info.Size())
		if !used[e.Name()] {
			t.Errorf("fonts/%s is embedded but no theme uses it", e.Name())
		}
		family := regexp.MustCompile(`-latin-.*$`).ReplaceAllString(e.Name(), "")
		if !licences[family] {
			t.Errorf("fonts/%s has no OFL-%s.txt", e.Name(), family)
		}
	}
	if total > 700*1024 {
		t.Errorf("embedded fonts total %d KB, want ≤ 700 KB", total/1024)
	}
	t.Logf("%d font files, %d KB", len(used), total/1024)
}

// Opacity would multiply every colour under it and void the contrast checks above, so
// panel.css may use it only on a disabled control (WCAG exempts inactive components).
func TestPanelCSSFadesNothingButDisabledControls(t *testing.T) {
	css := cssComment.ReplaceAllString(readWeb(t, "panel.css"), "")
	n := 0
	for _, m := range cssBlock.FindAllStringSubmatch(css, -1) {
		if !regexp.MustCompile(`(^|[;{\s])opacity\s*:`).MatchString(m[2]) {
			continue
		}
		n++
		sel := strings.TrimSpace(m[1])
		if !strings.HasSuffix(sel, ":disabled") {
			t.Errorf("%s sets opacity; use a token colour instead", sel)
		}
	}
	if n == 0 {
		t.Fatal("found no opacity at all; .btn:disabled should have one (is the parser broken?)")
	}
}
