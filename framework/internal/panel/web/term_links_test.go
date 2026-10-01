package web

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// term-links.js finds the plain-text URLs panel.js links, and decides which link opens
// without a confirmation. Terminal output is untrusted, so this runs the embedded file
// in node against what a lane may print: only http(s) is a link, sentence punctuation
// is not part of one, and an OSC 8 link opens directly only when its text is its target.
const termLinksHarness = `
const vm = require("vm"), fs = require("fs");
const window = { URL };
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window, URL });
const L = window.TermLinks;
const find = (s) => L.findURLs(s).map((m) => [m.url, s.slice(m.start, m.end) === m.url]);
console.log(JSON.stringify({
  plain: find("PR: https://github.com/clauductor/clauductor/pull/12 is open"),
  sentence: find("See https://example.com/a. Then (https://example.com/b), or https://example.com/c!"),
  wiki: find("https://en.wikipedia.org/wiki/Go_(language)"),
  quoted: find("<https://example.com/x> \"https://example.com/y\" ` + "`" + `https://example.com/z` + "`" + `"),
  schemes: find("javascript:alert(1) file:///etc/passwd ftp://x.org data:text/html,hi xhttps://no.example"),
  two: find("https://a.example/1 https://b.example/2"),
  bare: find("https:// alone"),
  showsSame: L.showsTarget("https://github.com/o/r/pull/12", "https://github.com/o/r/pull/12"),
  showsSameTrimmed: L.showsTarget(" https://example.com ", "https://example.com/"),
  showsOther: L.showsTarget("https://github.com/o/r", "https://evil.example/o/r"),
  showsWords: L.showsTarget("PR 12", "https://github.com/o/r/pull/12"),
  showsPrefix: L.showsTarget("https://github.com", "https://github.com.evil.example/"),
  showsJS: L.showsTarget("javascript:alert(1)", "javascript:alert(1)"),
  webJS: L.webURL("javascript:alert(1)"),
  webHTTP: L.webURL("HTTPS://Example.COM/a b"),
}));
`

func TestTermLinks(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("-short: runs node against term-links.js")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the terminal's URL matcher is untested here")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "term-links.js")
	if err := os.WriteFile(src, []byte(readWeb(t, "term-links.js")), 0o644); err != nil {
		t.Fatal(err)
	}
	h := filepath.Join(dir, "harness.js")
	if err := os.WriteFile(h, []byte(termLinksHarness), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := exec.Command(node, h, src).CombinedOutput()
	if err != nil {
		t.Fatalf("harness: %v\n%s", err, b)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("harness output: %v\n%s", err, b)
	}
	urls := func(xs ...string) []any {
		out := []any{}
		for _, x := range xs {
			out = append(out, []any{x, true})
		}
		return out
	}
	want := map[string]any{
		"plain":            urls("https://github.com/clauductor/clauductor/pull/12"),
		"sentence":         urls("https://example.com/a", "https://example.com/b", "https://example.com/c"),
		"wiki":             urls("https://en.wikipedia.org/wiki/Go_(language)"),
		"quoted":           urls("https://example.com/x", "https://example.com/y", "https://example.com/z"),
		"schemes":          urls(),
		"two":              urls("https://a.example/1", "https://b.example/2"),
		"bare":             urls(),
		"showsSame":        true,
		"showsSameTrimmed": true,
		"showsOther":       false,
		"showsWords":       false,
		"showsPrefix":      false,
		"showsJS":          false,
		"webJS":            nil,
		"webHTTP":          "https://example.com/a%20b",
	}
	for k, w := range want {
		if !reflect.DeepEqual(got[k], w) {
			t.Errorf("%s: got %v, want %v", k, got[k], w)
		}
	}
}

// panel.js wires the links and keeps a selection: the plain-text provider, the link
// handler that routes through followLink, and the declined motion request (1003),
// whose reports made xterm clear every selection (PANEL-14).
func TestTerminalLinksAndSelectionAreWired(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	for _, want := range []string{
		"term.registerLinkProvider(",
		"activate: (ev, uri, range) => followLink(ev, uri, rangeText(term, range))",
		"allowNonHttpProtocols: false",
		`term.parser.registerCsiHandler({ prefix: "?", final: "h" }`,
		"p.includes(1003)",
		"if (!(IS_MAC ? ev.metaKey : ev.ctrlKey)) return;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
	// Nothing opens a window but openLink, and only with noopener.
	if n := strings.Count(js, "window.open("); n != 1 || !strings.Contains(js, `window.open(href, "_blank", "noopener,noreferrer")`) {
		t.Errorf("panel.js opens windows in %d places, want 1 (openLink, noopener)", n)
	}
	page, _ := webFS.ReadFile("index.html")
	i, j := strings.Index(string(page), "/static/term-links.js"), strings.Index(string(page), "/static/panel.js")
	if i < 0 || j < i {
		t.Error("index.html must load term-links.js before panel.js")
	}
}
