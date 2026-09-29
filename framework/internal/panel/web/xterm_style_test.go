package web

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// xterm-style.js lets xterm colour cells past the CSP by routing span style attributes
// through CSSOM. This runs the embedded file against a minimal DOM in node and checks
// both sides: xterm's colour writes are routed, and anything else, or any span outside
// a terminal, still goes to the CSP-governed setAttribute.
const xtermStyleHarness = `
const vm = require("vm"), fs = require("fs");
class Element {
  constructor(cls, parent, root) { this.attrs = {}; this.cls = cls || ""; this.parent = parent || null; this.root = !!root; }
  setAttribute(n, v) { this.attrs[n] = String(v); }   // the path the CSP governs
  get isConnected() { let e = this; while (e.parent) e = e.parent; return e.root; }
  closest(sel) {
    if (sel !== ".xterm") throw new Error("unexpected selector " + sel);
    for (let e = this; e; e = e.parent) if (e.cls.split(" ").includes("xterm")) return e;
    return null;
  }
}
class HTMLSpanElement extends Element {
  constructor(cls, parent, root) { super(cls, parent, root); this.style = { cssText: "" }; }
}
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window: { Element, HTMLSpanElement } });
const doc = new Element("", null, true);
const term = new Element("xterm", new Element("termhost", doc));
const page = new Element("lane", doc);
const cases = [
  ["xterm cell, detached", () => new HTMLSpanElement(), "color:#ffffff;"],
  ["xterm cell, appended to a serialised value", () => new HTMLSpanElement(), "background-color: rgb(61, 1, 1);color:#3f3f3fff;"],
  ["span inside .xterm", () => new HTMLSpanElement("", term), "color:#0a0b0c;"],
  ["attached span outside .xterm", () => new HTMLSpanElement("", page), "color:#ffffff;"],
  ["colour plus url()", () => new HTMLSpanElement(), "color:#fff;background:url(https://example.invalid/x)"],
  ["positioning", () => new HTMLSpanElement(), "position:fixed;top:0"],
  ["colour plus another property", () => new HTMLSpanElement(), "color:#fff; background-image:url(x)"],
];
const out = {};
for (const [name, mk, value] of cases) {
  const s = mk();
  s.setAttribute("style", value);
  out[name] = s.style.cssText === value && !("style" in s.attrs) ? "cssom" : ("style" in s.attrs && s.style.cssText === "" ? "attribute" : "both?");
}
const t = new HTMLSpanElement();
t.setAttribute("title", "x");
out["other attribute"] = t.attrs.title === "x" ? "attribute" : "lost";
console.log(JSON.stringify(out));
`

func TestXtermStyleRouteIsNarrow(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the style-route guard is untested here")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "xterm-style.js")
	if err := os.WriteFile(src, []byte(readWeb(t, "xterm-style.js")), 0o644); err != nil {
		t.Fatal(err)
	}
	h := filepath.Join(dir, "harness.js")
	if err := os.WriteFile(h, []byte(xtermStyleHarness), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := exec.Command(node, h, src).CombinedOutput()
	if err != nil {
		t.Fatalf("harness: %v\n%s", err, b)
	}
	var got map[string]string
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("harness output: %v\n%s", err, b)
	}
	want := map[string]string{
		"xterm cell, detached":                       "cssom",
		"xterm cell, appended to a serialised value": "cssom",
		"span inside .xterm":                         "cssom",
		"attached span outside .xterm":               "attribute",
		"colour plus url()":                          "attribute",
		"positioning":                                "attribute",
		"colour plus another property":               "attribute",
		"other attribute":                            "attribute",
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("%s: went to %q, want %q", k, got[k], w)
		}
	}
}
