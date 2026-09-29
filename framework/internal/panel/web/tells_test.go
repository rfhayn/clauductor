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
