package web

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// PANEL-49 (D9): on a machine whose lanes are all measured from transcripts, nothing
// posts a status line, so the quota stays unknown. The note on its fields says why: the
// quota comes only with a status-line post, and a transcript brings none.

// quotaNote is the one string D9 changes, the title of a quota field with no reading.
const quotaNote = "No reading yet. The quota comes only with a status-line post from any session " +
	"(docs/panel.md, The status line). A lane measured from its transcript brings none."

// quotaSource is panel.js from quotaField through quotaFields, the code that draws the
// quota's fields, read from the embedded file the panel serves.
func quotaSource(t *testing.T) string {
	t.Helper()
	js := readWeb(t, "panel.js")
	i := strings.Index(js, "function quotaField(")
	j := strings.Index(js, "\n// PANEL-19: economy mode")
	if i < 0 || j < i {
		t.Fatal("panel.js: quotaField and quotaFields not found where expected; update quotaSource")
	}
	return js[i:j]
}

// jsFunction is the source of panel.js's top-level function name, from its declaration
// to its closing brace, so a harness runs the page's own helpers rather than copies of
// them that would keep passing when the page's changed (#160). Braces inside a string
// are skipped; the helpers it reads hold no regular expression.
func jsFunction(t *testing.T, js, name string) string {
	t.Helper()
	i := strings.Index(js, "\nfunction "+name+"(")
	if i < 0 {
		t.Fatalf("panel.js: function %s not found", name)
	}
	i++
	depth, quote := 0, byte(0)
	for j := i; j < len(js); j++ {
		c := js[j]
		switch {
		case quote != 0:
			if c == '\\' {
				j++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '{':
			depth++
		case c == '}':
			if depth--; depth == 0 {
				return js[i : j+1]
			}
		}
	}
	t.Fatalf("panel.js: function %s has no closing brace", name)
	return ""
}

// quotaHelpers are the panel.js helpers quotaField draws with, loaded from the page
// itself. Only the clock's (now, until, age, hm) are stubbed: a field with no reading
// shows none of them.
var quotaHelpers = []string{"el", "key", "pct", "bullet", "num"}

// [MEASURES-5-S1] The note of a quota field with no reading says the quota comes only
// with a status-line post, and that a lane measured from its transcript brings none: it
// is the no-reading branch of the field's title.
func TestQuotaNoteSaysATranscriptBringsNone(t *testing.T) {
	t.Parallel()
	src := quotaSource(t)
	if !strings.Contains(src, `f.title = at ? `) {
		t.Fatal("quotaField no longer chooses its title by whether it has a reading; update this test")
	}
	if !strings.Contains(src, `: "`+quotaNote+`";`) {
		t.Errorf("quotaField's no-reading title is not the note:\n%s", quotaNote)
	}
	for _, claim := range []string{"comes only with a status-line post", "measured from its transcript brings none"} {
		if !strings.Contains(quotaNote, claim) {
			t.Errorf("the note does not say %q", claim)
		}
	}
}

// quotaHarness draws the quota with no reading, through panel.js's own quotaField and
// quotaFields, and the helpers they draw with, over a minimal document, and prints each
// field's label, value and note.
const quotaHarness = `
const vm = require("vm"), fs = require("fs");
function node(tag) {
  const n = { tag, className: "", textContent: "", children: [], dataset: {}, style: {}, attrs: {},
    appendChild(c) { this.children.push(c); return c; },
    setAttribute(k, v) { this.attrs[k] = String(v); },
    get firstChild() { return this.children[0]; } };
  return n;
}
const document = { createElement: node, createTextNode: (t) => ({ textContent: t, children: [] }) };
const stubs = 'const now = () => 1e12, until = () => el("span"), age = () => el("span"), hm = () => "";\n';
const ctx = { document, out: null };
vm.runInNewContext(stubs + fs.readFileSync(process.argv[2], "utf8") +
  '; out = { unknown: quotaFields({}, {}, {}), windows: quotaFields({}, { quotaMode: "windows" }, {}) };', ctx);
const text = (n) => (n.textContent || "") + n.children.map(text).join("");
const show = (fs) => fs.map((f) => ({ label: text(f.children[0]), value: text(f.children[1]), note: f.title }));
console.log(JSON.stringify({ unknown: show(ctx.out.unknown), windows: show(ctx.out.windows) }));
`

// [MEASURES-5-S1] A panel whose sessions post no status line draws the quota with no
// reading: each field shows no value, and its note says the quota comes only with a
// status-line post, and that a lane measured from its transcript brings none. This runs
// panel.js's quotaField and quotaFields in node, for an account not yet read and for a
// subscription with no post since the panel started.
func TestQuotaFieldsWithNoReading(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("-short: runs node against panel.js's quota fields")
	}
	nodeBin, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the quota's fields are drawn untested here")
	}
	dir := t.TempDir()
	js := readWeb(t, "panel.js")
	var code strings.Builder
	for _, name := range quotaHelpers {
		code.WriteString(jsFunction(t, js, name) + "\n")
	}
	code.WriteString(quotaSource(t))
	src := filepath.Join(dir, "quota.js")
	if err := os.WriteFile(src, []byte(code.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	h := filepath.Join(dir, "harness.js")
	if err := os.WriteFile(h, []byte(quotaHarness), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := exec.Command(nodeBin, h, src).CombinedOutput()
	if err != nil {
		t.Fatalf("harness: %v\n%s", err, b)
	}
	type field struct{ Label, Value, Note string }
	var got map[string][]field
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("harness output: %v\n%s", err, b)
	}
	for _, mode := range []string{"unknown", "windows"} {
		fs := got[mode]
		if len(fs) != 2 || fs[0].Label != "5-hour quota" || fs[1].Label != "7-day quota" {
			t.Errorf("%s: fields %+v, want the 5-hour and the 7-day quota", mode, fs)
			continue
		}
		for _, f := range fs {
			if f.Value != "—" {
				t.Errorf("%s: %s shows %q, want no reading (—)", mode, f.Label, f.Value)
			}
			if f.Note != quotaNote {
				t.Errorf("%s: %s's note is %q, want %q", mode, f.Label, f.Note, quotaNote)
			}
		}
	}
}
