package web

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// projectConfigHarness runs project-config.js (its path is argv[2]) on the calls in
// argv[3], [[fn, arg], …], and prints each result as a JSON array.
const projectConfigHarness = `
const vm = require("vm"), fs = require("fs");
const window = {};
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), { window });
const calls = JSON.parse(process.argv[3]);
console.log(JSON.stringify(calls.map(([fn, arg]) => window.ProjectConfig[fn](arg))));
`

// runProjectConfig runs each call through the embedded project-config.js in node.
func runProjectConfig(t *testing.T, calls [][2]any) []string {
	t.Helper()
	if testing.Short() {
		t.Skip("-short: runs node against project-config.js")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the project menu's config line is untested here")
	}
	dir := t.TempDir()
	src, h := filepath.Join(dir, "project-config.js"), filepath.Join(dir, "harness.js")
	if err := os.WriteFile(src, []byte(readWeb(t, "project-config.js")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h, []byte(projectConfigHarness), 0o644); err != nil {
		t.Fatal(err)
	}
	in, _ := json.Marshal(calls)
	b, err := exec.Command(node, h, src, string(in)).CombinedOutput()
	if err != nil {
		t.Fatalf("harness: %v\n%s", err, b)
	}
	var out []string
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("harness output: %v\n%s", err, b)
	}
	return out
}

const todaysRemoveNote = "This only unregisters it. Nothing on disk is deleted: the repository, its config, its worktrees and its lane registry stay where they are, and adding it again brings it all back."

// ATTACH-4's and ATTACH-5's requirements, the page's words (design.md D6, D7): the
// project menu's line for a menu entry whose config is outside its repository, and
// none for one whose is not; the Remove from panel… note for a remove plan of each.
// The scenarios' tests, which take the entry and the plan from a running panel, are in
// multiproject_integration_test.go and live_integration_test.go.
func TestProjectConfigLines(t *testing.T) {
	t.Parallel()
	side := "/home/o/.clauductor/projects/acme/panel.json"
	got := runProjectConfig(t, [][2]any{
		{"menuLine", ProjectSummary{ID: "acme", Name: "Acme", OK: true, Config: side}},
		{"menuLine", ProjectSummary{ID: "own", Name: "Own", OK: true}},
		{"removeNote", map[string]any{"id": "acme", "root": "/src/acme", "configPath": side, "configOutside": true}},
		{"removeNote", map[string]any{"id": "own", "root": "/src/own", "configPath": "/src/own/.clauductor/panel.json"}},
	})
	if want := "reads " + side + ", outside the repository"; got[0] != want {
		t.Errorf("the attached project's line: %q, want %q", got[0], want)
	}
	if got[1] != "" {
		t.Errorf("a project reading its own config has a line: %q", got[1])
	}
	for _, want := range []string{side, "stay", "clauductor panel add --project /src/acme --config " + side} {
		if !strings.Contains(got[2], want) {
			t.Errorf("the attached project's remove note lacks %q: %q", want, got[2])
		}
	}
	if strings.Contains(got[2], "brings it") {
		t.Errorf("the attached project's remove note says adding it again brings it back: %q", got[2])
	}
	if got[3] != todaysRemoveNote {
		t.Errorf("the own-config remove note changed: %q", got[3])
	}
}

// panel.js renders project-config.js's lines, not words of its own, and index.html
// loads project-config.js before panel.js.
func TestPanelUsesProjectConfig(t *testing.T) {
	t.Parallel()
	js := readWeb(t, "panel.js")
	if !strings.Contains(funcBody(t, js, "renderProjects"), "ProjectConfig.menuLine(p)") {
		t.Error("renderProjects does not render ProjectConfig.menuLine")
	}
	if !strings.Contains(funcBody(t, js, "openRemoveDlg"), "ProjectConfig.removeNote(plan)") {
		t.Error("openRemoveDlg does not render ProjectConfig.removeNote")
	}
	if strings.Contains(js, "adding it again brings it all back") {
		t.Error("panel.js keeps its own remove note; it is project-config.js's")
	}
	page := readWebPage(t)
	i, j := strings.Index(page, `<script src="/static/project-config.js"></script>`), strings.Index(page, `<script src="/static/panel.js"></script>`)
	if i < 0 || j < 0 || i > j {
		t.Error("index.html must load project-config.js before panel.js")
	}
}
