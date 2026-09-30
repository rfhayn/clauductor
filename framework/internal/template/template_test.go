package template

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

// The template's own process checks (template/.claude/checks) hold its hooks, gate, roadmap parser
// and model roles to what its AGENTS.md says. They are plain sh + git + jq; run them here so a
// template change that breaks one fails this repo's CI, not the first project that adopts it.
func TestTemplateChecks(t *testing.T) {
	if testing.Short() {
		t.Skip("runs every template check (~20 s)")
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
