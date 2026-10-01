package template

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/config"
)

// repoRoot is the clauductor repository root, from this package's directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// The template vendors the plain-shell lease protocol so a project's gate queues on the panel's
// lease without clauductor installed. The copy must stay the block docs/panel.md documents, which
// is the one TestLeaseConformance runs against: a drifted copy would be an implementation nobody
// tests, in every project that adopts the template.
func TestVendoredLeaseMatchesPanelDoc(t *testing.T) {
	root := repoRoot(t)
	doc, err := os.ReadFile(filepath.Join(root, "docs", "panel.md"))
	if err != nil {
		t.Fatal(err)
	}
	const begin, end = "<!-- lease.sh begin -->\n```sh\n", "```\n<!-- lease.sh end -->"
	s := string(doc)
	i, j := strings.Index(s, begin), strings.Index(s, end)
	if i < 0 || j < i {
		t.Fatal("docs/panel.md has no lease.sh block between its markers")
	}
	block := s[i+len(begin) : j]
	vendored, err := os.ReadFile(filepath.Join(root, "template", "scripts", "ci", "lease.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(vendored), block) {
		t.Fatal("template/scripts/ci/lease.sh has drifted from the lease.sh block in docs/panel.md: re-vendor it (keep the header comment, then the block verbatim)")
	}
}

// The lease conformance kit (P1.12) ships the suite TestLeaseConformance runs, and VERSION names it
// by a sha256 the kit's run.sh checks before running, so a project cannot pass by editing a case.
// This holds VERSION to the files, computed exactly as run.sh does: sha256 over
// "<sha256>  <path>\n" lines of conformance.sh and every file under cases/, sorted by path (C
// locale). A suite change that forgot the bump fails here, with the new value.
func TestLeaseConformanceKitVersion(t *testing.T) {
	kit := filepath.Join(repoRoot(t), "template", "scripts", "ci", "lease-conformance")
	paths := []string{"conformance.sh"}
	err := filepath.WalkDir(filepath.Join(kit, "cases"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(kit, p)
		paths = append(paths, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	var list strings.Builder
	for _, p := range paths {
		b, err := os.ReadFile(filepath.Join(kit, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&list, "%x  %s\n", sha256.Sum256(b), p)
	}
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(list.String())))
	v, err := os.ReadFile(filepath.Join(kit, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(v), "\nsha256 "+got+"\n") {
		t.Fatalf("lease-conformance/VERSION does not name the suite: put `sha256 %s` in it (README.md, Refreshing)", got)
	}
	if len(paths) < 30 {
		t.Fatalf("the kit holds %d files; the suite has more than 30 cases", len(paths))
	}
}

// The template ships a preset panel config for its operating model (lanes, lane types, templates
// with suggestions, the gate queue, pinned cards). It must be a config this panel accepts, at the
// version it declares: a preset that fails to load would be every adopting project's first error.
func TestTemplatePanelConfigLoads(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "template", ".clauductor", "panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.ParseConfig(raw); err != nil {
		t.Fatalf("template/.clauductor/panel.json is not a valid panel config: %v", err)
	}
}

// The command the template's panel preset runs for the Metrics view and the Flow card (OPS-9):
// .claude/metrics.sh prints the panel's metrics JSON. `sh -c … 2>&1` as the preset's cards do;
// metrics.sh discards its own stderr, so the fold cannot reach the JSON (checks/metrics.sh holds it).
var templateMetrics = struct {
	Command []string
	Refresh string
}{[]string{"sh", "-c", "sh .claude/metrics.sh 2>&1"}, "interval:600"}

// The preset runs the model's metrics command (OPS-9), which needs config version 4 (PANEL-19).
func TestTemplatePanelMetricsCommand(t *testing.T) {
	root := repoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "template", ".claude", "metrics.sh")); err != nil {
		t.Fatalf("the metrics command names template/.claude/metrics.sh: %v", err)
	}
	need := 0
	for _, f := range config.Fields {
		if f.Path == "metrics" {
			need = f.Version
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "template", ".clauductor", "panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	var preset struct {
		Version int `json:"version"`
		Metrics *struct {
			Command []string `json:"command"`
			Refresh string   `json:"refresh"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(raw, &preset); err != nil {
		t.Fatal(err)
	}
	if need == 0 {
		if preset.Metrics != nil {
			t.Fatal("template/.clauductor/panel.json has \"metrics\", which this panel's config does not know yet (PANEL-19)")
		}
		return
	}
	const want = `"metrics": {"command": ["sh", "-c", "sh .claude/metrics.sh 2>&1"], "refresh": "interval:600"}`
	if preset.Metrics == nil {
		t.Fatalf("the panel config now knows \"metrics\" (version %d): add %s to template/.clauductor/panel.json, set its \"version\" to at least %d, delete the TODO (OPS-9) above, and rebuild the plugin (scripts/build-plugin.sh)", need, want, need)
	}
	if strings.Join(preset.Metrics.Command, "\x00") != strings.Join(templateMetrics.Command, "\x00") || preset.Metrics.Refresh != templateMetrics.Refresh {
		t.Fatalf("template/.clauductor/panel.json \"metrics\" is %+v, want %s", *preset.Metrics, want)
	}
	if preset.Version < need {
		t.Fatalf("template/.clauductor/panel.json declares version %d; \"metrics\" needs %d", preset.Version, need)
	}
}

// The template's own process checks (template/.claude/checks) hold its hooks, gate, roadmap parser
// and model roles to what its AGENTS.md says. They are plain sh + git + jq; run them here so a
// template change that breaks one fails this repo's CI, not the first project that adopts it.
func TestTemplateChecks(t *testing.T) {
	if testing.Short() {
		t.Skip("runs every template check (~50 s)")
	}
	for _, tool := range []string{"sh", "bash", "git", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("no " + tool)
		}
	}
	root := repoRoot(t)
	cmd := exec.Command("sh", filepath.Join(root, "template", ".claude", "checks", "run.sh"))
	// A private HOME: the status-line and focus checks write per-branch focus files under it.
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "TMPDIR="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("template checks failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "checks: all") {
		t.Fatalf("template checks did not report a verdict:\n%s", out)
	}
}
