package web

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// PANEL-11: the design brief's legibility rules and its list of tells, as far as a
// stylesheet and a script can show them. Each is a thing the old panel did.
func TestPanelAvoidsTheTells(t *testing.T) {
	t.Parallel()
	css := cssComment.ReplaceAllString(readWeb(t, "panel.css"), "")
	for _, bad := range []struct{ re, why string }{
		{`uppercase`, "no ALL-CAPS labels"},
		{`letter-spacing`, "no tracked labels"},
		{`gradient\(`, "no gradients, stripes or hatching"},
		{`@keyframes|animation\s*:`, "no animation but the changed-figure flash (a transition)"},
		{`font-weight:\s*(?:[1-35-689]00|600)|font:\s*(?:[1-35-689]00|600)\b`, "weights 400 and 700 only"},
		{`text-shadow|filter\s*:|backdrop-filter`, "no glows or glass"},
		{`border-radius:\s*(?:[1-9]|999|50%)`, "square panes; controls take --r-ctl"},
	} {
		if m := regexp.MustCompile(bad.re).FindString(css); m != "" {
			t.Errorf("panel.css has %q: %s", m, bad.why)
		}
	}
	// Shadows only on what floats: menus, the drawer, the dialog. An inset hairline
	// (a bar's outline) is not a shadow.
	for _, m := range cssBlock.FindAllStringSubmatch(css, -1) {
		for _, d := range regexp.MustCompile(`box-shadow:\s*([^;]+)`).FindAllStringSubmatch(m[2], -1) {
			sel := strings.TrimSpace(m[1])
			if !strings.HasPrefix(strings.TrimSpace(d[1]), "inset") && !regexp.MustCompile(`\.(menu|drawer|dlg|colmenu)\b`).MatchString(sel) {
				t.Errorf("%s has a shadow (%s); only menus, the drawer and the dialog float", sel, d[1])
			}
		}
	}
	// x-heights evened for every face; figures in tabular, lining, slashed-zero digits.
	for _, want := range []string{"font-size-adjust: ex-height .52", "font-variant-numeric: tabular-nums lining-nums slashed-zero", "font-feature-settings: var(--num)"} {
		if !strings.Contains(css, want) {
			t.Errorf("panel.css lacks %q", want)
		}
	}
	// Minimum sizes: nothing under 11.5 px (.71875rem) at 100%.
	for _, m := range regexp.MustCompile(`font(?:-size)?:[^;]*?([0-9.]+)rem`).FindAllStringSubmatch(css, -1) {
		if v, _ := strconv.ParseFloat(m[1], 64); v < 0.71875 {
			t.Errorf("panel.css sets text at %srem, under the 11.5 px floor: %s", m[1], m[0])
		}
	}
	// A control on an abnormal row takes the row's ink, which themes_test.go holds to
	// 4.5:1 on the row in every theme × mode; the link colour is not (1.14:1 was found).
	for _, row := range []string{"abn-warn", "abn-crit"} {
		ok := false
		for _, m := range cssBlock.FindAllStringSubmatch(css, -1) {
			if strings.Contains(m[1], "tr."+row+" .btn") && regexp.MustCompile(`color:\s*inherit`).MatchString(m[2]) {
				ok = true
			}
		}
		if !ok {
			t.Errorf("panel.css does not give a button on a %s row the row's ink", row)
		}
	}
	// The page's words: no "A · B · C" strings and no arrows on buttons.
	js := readWeb(t, "panel.js")
	if strings.Contains(js, " · ") {
		t.Error(`panel.js joins words with " · "; use a column, a comma or a sentence`)
	}
	if regexp.MustCompile(`button\("[^"]*→`).MatchString(js) {
		t.Error("panel.js puts an arrow on a button")
	}
	if strings.Contains(js, "cursorBlink: true") || !strings.Contains(js, "cursorBlink: false") {
		t.Error("the terminal cursor must not blink")
	}
}

// PANEL-21: words that name a button name it as the button reads ("Resume", not
// "RESUME"): the orphan's message shouted its three buttons' names in capitals.
func TestCopyNamesButtonsAsTheyRead(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	labels := map[string]bool{}
	for _, re := range []string{`button\("([^"]+)"`, `\["[a-z-]+", "([^"]+)"\]`} {
		for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(js, -1) {
			labels[m[1]] = true
		}
	}
	for _, want := range []string{"Resume", "Forget", "Close lane"} {
		if !labels[want] {
			t.Errorf("no button reads %q; this test no longer finds the labels", want)
		}
	}
	for l := range labels {
		if up := strings.ToUpper(l); up != l && len(l) > 3 && regexp.MustCompile(`\b`+regexp.QuoteMeta(up)+`\b`).MatchString(js) {
			t.Errorf("panel.js names the button %q as %q; use the button's own words", l, up)
		}
	}
}

// PANEL-21: a field's value says something its label does not. The economy field read
// "Economy / economy"; it reads "on" now (and is not shown while economy is off). The
// test reads each label in panel.js with the first literal of its value, and requires
// that it finds the economy field, so a rewrite that hides the pairs from it fails.
func TestFieldValueDoesNotRepeatItsLabel(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	pairs := map[string]string{}
	for _, re := range []string{
		`el\("span", "k", "([^"]+)"\), el\("span", "v(?: [^"]*)?", (?:"([^"]+)"|null, \[el\("span", (?:"[^"]*"|null), "([^"]+)")`,
		`kv1\("([^"]+)", (?:"([^"]+)"|el\("span", (?:"[^"]*"|null), "([^"]+)")`,
	} {
		for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(js, -1) {
			pairs[m[1]] = m[2] + m[3]
		}
	}
	if v, ok := pairs["Economy"]; !ok {
		t.Fatal("no Economy field found; this test no longer reads the label/value pairs")
	} else if v != "on" {
		t.Errorf(`the Economy field's value reads %q; want "on"`, v)
	}
	for k, v := range pairs {
		if strings.EqualFold(strings.TrimSpace(k), strings.TrimSpace(v)) {
			t.Errorf("the field %q shows its label again as its value (%q); say the state", k, v)
		}
	}
}

// PANEL-21: the page is the window at every size. The page never scrolls (it did below
// 900 px, and at 62vh the terminal ran over the footer below 1180 px), the terminal is
// laid over its frame so the frame can shrink and refit it, the footer wraps, and the
// Lanes table's ⋯ column sticks to the rail's edge. The UX harness checks the result in
// a browser (docs/ux-passes.md: vscroll, footer-*, rowact-*, tab-overflow).
func TestLayoutFitsTheWindow(t *testing.T) {
	t.Parallel()
	css := cssComment.ReplaceAllString(readWeb(t, "panel.css"), "")
	rule := func(sel string) string {
		for _, m := range cssBlock.FindAllStringSubmatch(css, -1) {
			if strings.TrimSpace(m[1]) == sel {
				return m[2]
			}
		}
		t.Errorf("panel.css has no %s rule", sel)
		return ""
	}
	for _, c := range []struct{ sel, re, why string }{
		{"body", `overflow:\s*hidden`, "the page itself never scrolls"},
		{".termhost .term", `position:\s*absolute`, "the terminal is laid over its frame, so the frame can shrink"},
		{".obs", `flex-wrap:\s*wrap`, "the footer's counters wrap rather than run out of it"},
		{".obs > *", `text-overflow:\s*ellipsis`, "a counter too long for a line truncates visibly"},
		{".rail .tbl :where(td.acts, th.acts)", `position:\s*sticky`, "the ⋯ column stays in the rail"},
		{".rail .tbl .lane-nm", `max-width:\s*0`, "a long lane name yields its width"},
		{".wsbody", `overflow-y:\s*auto`, "what does not fit scrolls inside the workspace, not the page"},
		{".topbars", `overflow-y:\s*auto`, "Needs you and the banners give way together, and scroll"},
		{".topbars", `flex:\s*0 1 auto`, "Needs you and the banners shrink before the shell does"},
	} {
		if !regexp.MustCompile(c.re).MatchString(rule(c.sel)) {
			t.Errorf("%s lacks %s: %s", c.sel, c.re, c.why)
		}
	}
	if regexp.MustCompile(`(?:html|body)[^{}]*\{[^{}]*height:\s*auto`).MatchString(css) {
		t.Error("panel.css lets the page grow past the window (height: auto on html or body)")
	}
	if regexp.MustCompile(`\.termhost\s*\{[^{}]*height:\s*\d+vh`).MatchString(css) {
		t.Error("panel.css fixes the terminal's frame to a share of the window; it takes what the layout leaves")
	}
}

// Tuned generates every colour from a hue, an accent and a contrast (tuned.js). Over a
// grid of inputs, both modes, every output meets the same floors as the fixed themes,
// and contrast 1 holds text to AAA.
func TestTunedMeetsTheFloorsForAnyInput(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the Tuned generator is untested here")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "tuned.js")
	if err := os.WriteFile(src, []byte(readWeb(t, "tuned.js")), 0o644); err != nil {
		t.Fatal(err)
	}
	script := `require(process.argv[1]); const out = [];
for (const hue of [0, 45, 90, 135, 180, 225, 270, 315]) for (const accent of [0, 60, 120, 200, 260, 320])
  for (const contrast of [0, 0.5, 1]) for (const mode of ["light", "dark"])
    out.push({ hue, accent, contrast, mode, t: globalThis.PanelTuned.tokens({ hue, accent, contrast }, mode) });
console.log(JSON.stringify(out));`
	b, err := exec.Command(node, "-e", script, src).Output()
	if err != nil {
		t.Fatal(err)
	}
	var runs []struct {
		Hue, Accent, Contrast float64
		Mode                  string
		T                     map[string]string
	}
	if err := json.Unmarshal(b, &runs); err != nil || len(runs) < 200 {
		t.Fatalf("generator output: %d runs, %v", len(runs), err)
	}
	// The terminal colours are the theme's fixed ones (themes.css).
	fixed := parseThemes(t, readWeb(t, "themes.css")).colour["tuned/dark"]
	for _, r := range runs {
		tok := map[string]string{}
		for k, v := range fixed {
			tok[k] = v
		}
		for k, v := range r.T {
			tok[k] = v
		}
		name := r.Mode + " hue " + strconv.Itoa(int(r.Hue)) + " accent " + strconv.Itoa(int(r.Accent)) + " contrast " + strconv.FormatFloat(r.Contrast, 'f', 1, 64)
		col := func(k string) rgba {
			c, err := parseColour(tok[k])
			if err != nil {
				t.Fatalf("%s --%s: %v", name, k, err)
			}
			return c
		}
		checkAll(r.Contrast >= 1, tok, col, func(label string, fg, bg rgba, min float64) {
			if rr := contrast(over(fg, bg), bg); rr < min {
				t.Errorf("%s: %s is %.2f:1, want ≥ %.1f:1", name, label, rr, min)
			}
		})
	}
}
