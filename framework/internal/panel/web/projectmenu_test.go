package web

import (
	"regexp"
	"strings"
	"testing"
)

// PANEL-22: the header's project selector reads as a dropdown box, in theme tokens
// only (no chip, no gradient), with the "need you elsewhere" badge beside it; its menu
// keeps the listbox and adds "Add a project…" under a separator, and each row has a ⋯
// beside its option (never inside it). The browser test and the UX harness check what
// this draws (testdata/browser/project-menu.cjs; testdata/ux, matrix views project-*).
func TestProjectMenuIsWired(t *testing.T) {
	t.Parallel()
	page, err := webFS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	for _, want := range []string{
		`aria-haspopup="listbox" aria-expanded="false" aria-controls="projlist"`,
		`<span class="pname" id="pname">…</span><span class="chev" aria-hidden="true">▾</span></button></h1>`,
		`<span class="projelse" id="projelse"></span>`, // beside the button, not in it
		`<div class="projlist" id="projlist" role="listbox" aria-label="Projects"></div>`,
		`<div class="msep" role="separator"></div>`,
		`<button class="mi addproj" id="addproj" type="button" aria-haspopup="dialog" aria-controls="adddlg">+ Add a project…</button>`,
		`role="menu" aria-label="Project actions"`,
		`id="adddlg"`, `role="dialog" aria-modal="true" aria-labelledby="ad-title"`,
		`id="ad-path"`, `id="ad-init" hidden>Create this config</button>`,
		`id="ad-plain" hidden>Add without trusting</button>`, `id="ad-trust" hidden>Trust and add</button>`,
		`id="pdlg"`, `aria-labelledby="pd-title"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %q", want)
		}
	}
	css := cssComment.ReplaceAllString(readWeb(t, "panel.css"), "")
	rules := map[string]string{}
	for _, m := range cssBlock.FindAllStringSubmatch(css, -1) {
		rules[strings.TrimSpace(m[1])] += m[2]
	}
	box := rules[".projbtn"]
	for _, re := range []string{`border:\s*1px solid var\(--bar-text-2\)`, `border-radius:\s*var\(--r-ctl\)`, `display:\s*inline-flex`} {
		if !regexp.MustCompile(re).MatchString(box) {
			t.Errorf(".projbtn lacks %s: it is drawn as a dropdown box", re)
		}
	}
	if !regexp.MustCompile(`text-overflow:\s*ellipsis`).MatchString(rules[".projbtn .pname"]) {
		t.Error("a long project name does not truncate visibly in the box")
	}
	// Theme tokens only, nothing that makes it a chip.
	for sel, body := range rules {
		if !strings.Contains(sel, ".proj") && !strings.Contains(sel, ".mimore") && !strings.Contains(sel, ".addproj") {
			continue
		}
		for _, v := range regexp.MustCompile(`(?:^|;|\s)(?:color|background|border(?:-color)?)\s*:\s*([^;]+)`).FindAllStringSubmatch(body, -1) {
			val := strings.TrimSpace(v[1])
			if regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|rgb|hsl|gradient`).MatchString(val) {
				t.Errorf("%s sets %q: theme tokens only", sel, val)
			}
		}
		if regexp.MustCompile(`border-radius:\s*(?:[1-9]|999|50%)`).MatchString(body) {
			t.Errorf("%s rounds like a chip", sel)
		}
	}
	js := readWeb(t, "panel.js")
	for _, want := range []string{
		// The ⋯ sits beside the option, in a row the listbox does not count.
		`const row = el("div", "mirow", null, [e, more]);`,
		`row.setAttribute("role", "none");`,
		`more.setAttribute("aria-haspopup", "menu");`,
		// Up and Down reach Add a project…; Right reaches a row's ⋯ and Left comes back.
		`querySelectorAll('[role="option"], #addproj')`,
		`else if (e.key === "ArrowRight" && a && a.getAttribute("role") === "option") {`,
		// The routes, and trust by the hash the report showed.
		`adminPost("/api/projects/validate", { path })`,
		`adminPost("/api/projects/init-preview", { path })`,
		`adminPost("/api/projects/init", { path })`,
		`trust ? { path: c.input, trust: true, hash: c.hash } : { path: c.input }`,
		`"/remove", { dryRun: true }`,
		`"/trust", { hash: pd.data.hash }`,
		// A removed project's page moves to the default.
		"followMenu();\n    renderProjects();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("panel.js lacks %q", want)
		}
	}
	// Nothing the panel answers is drawn as markup.
	if regexp.MustCompile(`innerHTML\s*=`).MatchString(js) {
		t.Error("panel.js assigns innerHTML")
	}
}
