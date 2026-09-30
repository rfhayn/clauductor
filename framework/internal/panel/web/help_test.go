package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const guideURL = "https://github.com/rfhayn/clauductor/blob/main/docs/guide.md"

// PANEL-17: the Help dialog. A "?" in the header and the ? key open it; it is a
// labelled modal dialog; its guide link opens in a new tab with noopener; nothing in
// it is inline script or style, and the page still names nothing outside its origin.
func TestHelpDialogIsWired(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t)
	w := do(s, "GET", "/", "", withCookie(s))
	if w.Code != 200 {
		t.Fatalf("page: %d", w.Code)
	}
	page := w.Body.String()
	for _, want := range []string{
		`<button class="btn" id="helpbtn" type="button" aria-haspopup="dialog" aria-expanded="false" aria-controls="helpdlg" aria-keyshortcuts="?" aria-label="Help"`,
		`<div class="modal" id="helpdlg" hidden>`,
		`<div class="dlg help" id="helpbox" role="dialog" aria-modal="true" aria-labelledby="help-title" tabindex="-1">`,
		`<h3 id="help-title">Help</h3>`,
		`<a id="helplink" target="_blank" rel="noopener noreferrer">`,
		`<button type="button" class="btn" id="helpclose">Close</button>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	// The guide's address is set by the script; the page carries no outside URL.
	if strings.Contains(page, "https://") || strings.Contains(page, " style=") || strings.Contains(page, "<style") {
		t.Error("the page carries an outside URL or inline style")
	}

	js := readWeb(t, "panel.js")
	for _, want := range []string{
		`const GUIDE_URL = "` + guideURL + `";`,
		`$("helplink").href = GUIDE_URL;`,
		`$("helpbtn").addEventListener("click", openHelp);`,
		`if (ev.key !== "?" || ev.ctrlKey || ev.metaKey || ev.altKey || ev.defaultPrevented) return;`,
		// Never taken from the terminal, where ? is claude's, or from a field.
		`t.closest(".xterm") || t.closest("input, select, textarea, [contenteditable]")`,
		`if (typingTarget(ev.target) || !$("startdlg").hidden) return;`,
		`if (e.key === "Escape") { e.preventDefault(); closeHelp(); return; }`,
		`if (r && r.isConnected && r.focus) r.focus(); else $("helpbtn").focus();`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
	// Still the one window.open (openLink); the guide is a link, not a script's window.
	if n := strings.Count(js, "window.open("); n != 1 {
		t.Errorf("panel.js opens windows in %d places, want 1", n)
	}
}

// Every shortcut the dialog lists is one panel.js implements, and the ones the page
// has are all listed.
func TestHelpListsTheRealShortcuts(t *testing.T) {
	t.Parallel()
	b, _ := webFS.ReadFile("index.html")
	page := string(b)
	js := readWeb(t, "panel.js")
	for key, impl := range map[string]string{
		"?":                  `ev.key !== "?"`,
		"Enter":              `if (ev.target === host && ev.key === "Enter") { ev.preventDefault(); enterTerm(); }`,
		"Ctrl+]":             `ev.code === "BracketRight"`,
		"← → Home End":       `else if (ev.key === "Home") j = 0;`,
		"Ctrl+Alt+= / − / 0": `const SIZE_KEYS = { Equal: 1, NumpadAdd: 1, Minus: -1, NumpadSubtract: -1, Digit0: 0, Numpad0: 0 };`,
		"⌘-click":            `if (!(IS_MAC ? ev.metaKey : ev.ctrlKey)) return;`,
		"⌘C / ⌘V":            `host.addEventListener("paste",`,
		"Escape":             `if (e.key === "Escape") { e.preventDefault(); closeStart(); return; }`,
	} {
		if !strings.Contains(page, `<th scope="row">`+key+`</th>`) {
			t.Errorf("the help does not list %s", key)
		}
		if !strings.Contains(js, impl) {
			t.Errorf("the help lists %s, but panel.js lacks %q", key, impl)
		}
	}
	if n := strings.Count(page, `<th scope="row">`); n != 8 {
		t.Errorf("the help lists %d shortcuts; update this test with any new one", n)
	}
	if n := strings.Count(page[strings.Index(page, `<ul class="howto">`):], "<li>"); n < 5 || n > 8 {
		t.Errorf("the help has %d how-tos, want 5 to 8", n)
	}
}

// Keys are bold words in a column: no boxes (the design brief's "no chips").
func TestHelpDrawsNoChips(t *testing.T) {
	t.Parallel()
	css := cssComment.ReplaceAllString(readWeb(t, "panel.css"), "")
	n := 0
	for _, m := range cssBlock.FindAllStringSubmatch(css, -1) {
		sel := strings.TrimSpace(m[1])
		if !strings.Contains(sel, ".help") {
			continue
		}
		n++
		if regexp.MustCompile(`border(-radius)?\s*:|background\s*:|box-shadow`).MatchString(m[2]) {
			t.Errorf("%s boxes something: %s", sel, strings.TrimSpace(m[2]))
		}
		for _, v := range regexp.MustCompile(`color:\s*([^;]+)`).FindAllStringSubmatch(m[2], -1) {
			if !strings.HasPrefix(strings.TrimSpace(v[1]), "var(--") {
				t.Errorf("%s sets a colour that is not a theme token: %s", sel, v[1])
			}
		}
	}
	if n == 0 {
		t.Fatal("no .help rules found")
	}
}

// The guide exists, and every link from it into panel.md lands on a heading there.
func TestGuideLinksResolve(t *testing.T) {
	t.Parallel()
	docs := filepath.Join("..", "..", "..", "..", "docs")
	guide, err := os.ReadFile(filepath.Join(docs, "guide.md"))
	if err != nil {
		t.Fatal(err)
	}
	panel, err := os.ReadFile(filepath.Join(docs, "panel.md"))
	if err != nil {
		t.Fatal(err)
	}
	anchors := map[string]bool{}
	inFence := false
	for _, l := range strings.Split(string(panel), "\n") {
		if strings.HasPrefix(l, "```") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(l, "#") {
			anchors[githubSlug(strings.TrimLeft(l, "# "))] = true
		}
	}
	links := regexp.MustCompile(`\]\(panel\.md#([^)]+)\)`).FindAllStringSubmatch(string(guide), -1)
	if len(links) < 5 {
		t.Fatalf("the guide links into panel.md %d times; its troubleshooting alone has 5", len(links))
	}
	for _, l := range links {
		if !anchors[l[1]] {
			t.Errorf("guide.md links to panel.md#%s, which is no heading there", l[1])
		}
	}
}

// githubSlug is GitHub's heading anchor: lower case, punctuation dropped, spaces as dashes.
func githubSlug(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}
