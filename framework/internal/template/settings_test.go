package template

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func templateSettings(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "template", ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func defaultsConf(t *testing.T, project string) *Conf {
	t.Helper()
	dir := t.TempDir()
	if project != "" {
		os.MkdirAll(filepath.Join(dir, ".claude"), 0o755)
		os.WriteFile(filepath.Join(dir, ".claude", "project.conf"), []byte(project), 0o644)
	}
	c, err := LoadConf(dir, filepath.Join(repoRoot(t), "template"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// The template merged into itself is unchanged, byte for byte, with nothing to report: an
// up-to-date project sees no churn.
func TestMergeSettingsFixedPoint(t *testing.T) {
	tmpl := templateSettings(t)
	rendered, stale, err := RenderSettings(tmpl, defaultsConf(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if string(rendered) != string(tmpl) {
		t.Fatalf("rendering with the defaults changed the template:\n%s", UnifiedDiff(string(tmpl), string(rendered), "template", "rendered"))
	}
	merged, changes, err := MergeSettings(tmpl, rendered, stale)
	if err != nil {
		t.Fatal(err)
	}
	if string(merged) != string(tmpl) || len(changes) != 0 {
		t.Fatalf("merging the template into itself: %d changes\n%s", len(changes), UnifiedDiff(string(tmpl), string(merged), "template", "merged"))
	}
}

// An empty project object gains every template key and reports each as added, no conflicts.
func TestMergeSettingsIntoEmpty(t *testing.T) {
	tmpl := templateSettings(t)
	merged, changes, err := MergeSettings([]byte("{}"), tmpl, nil)
	if err != nil {
		t.Fatal(err)
	}
	m2, c2, _ := MergeSettings(merged, tmpl, nil)
	if string(m2) != string(merged) || len(c2) != 0 {
		t.Errorf("a second merge is not a no-op: %v", c2)
	}
	for _, c := range changes {
		if c.Action != "add" {
			t.Errorf("merging into {} reported %s %s", c.Action, c.Path)
		}
	}
}

// Invalid JSON in the project's file is an error that names it, never an overwrite.
func TestMergeSettingsRefusesInvalid(t *testing.T) {
	if _, _, err := MergeSettings([]byte("{ not json"), templateSettings(t), nil); err == nil || !strings.Contains(err.Error(), "project's settings.json") {
		t.Fatalf("want an error naming the project's file, got %v", err)
	}
}

// B2: the gate's paths in the allow list and the sandbox exclusions come from project.conf.
func TestRenderSettingsGatePaths(t *testing.T) {
	c := defaultsConf(t, "GATE_RUN=\"ci/run-local.sh\"\nGATE='ci/gate.sh' # the agent wrapper\n")
	out, stale, err := RenderSettings(templateSettings(t), c)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`"Bash(ci/gate.sh)"`, `"ci/run-local.sh *"`, `"ci/gate.sh *"`} {
		if !strings.Contains(s, want) {
			t.Errorf("rendered settings lack %s", want)
		}
	}
	if strings.Contains(s, "scripts/ci/") {
		t.Errorf("rendered settings still name scripts/ci/")
	}
	if len(stale["permissions.allow"]) == 0 || len(stale["sandbox.excludedCommands"]) == 0 {
		t.Errorf("the default entries should be listed stale: %v", stale)
	}
}

func TestParseShellAssignments(t *testing.T) {
	vals, bad := parseShellAssignments("A=\"x y\"\nB='q'\nC=plain # note\n  D=\"$HOME/x\"\nexport E=1\nlower=no\n")
	for k, want := range map[string]string{"A": "x y", "B": "q", "C": "plain", "E": "1"} {
		if vals[k] != want {
			t.Errorf("%s = %q, want %q", k, vals[k], want)
		}
	}
	if _, ok := bad["D"]; !ok {
		t.Error("D needs a shell to expand and must be reported unreadable")
	}
	if _, ok := vals["lower"]; ok {
		t.Error("lower-case names are not configuration keys")
	}
}

func TestUnifiedDiff(t *testing.T) {
	d := UnifiedDiff("a\nb\nc\n", "a\nB\nc\n", "x", "y")
	for _, want := range []string{"--- x\n+++ y\n", "@@ -1,3 +1,3 @@\n", "-b\n", "+B\n", " a\n"} {
		if !strings.Contains(d, want) {
			t.Errorf("diff lacks %q:\n%s", want, d)
		}
	}
	if UnifiedDiff("same\n", "same\n", "x", "y") != "" {
		t.Error("equal texts have no diff")
	}
}

// B17: GATE_REMOTE_WORKFLOW's default is stated once, in conf.sh; the guard and ci-receipt.sh
// read it from there instead of restating `ci.yml` in a ${VAR-default} of their own (which
// disagreed with conf.sh's "").
func TestRemoteWorkflowDefaultStatedOnce(t *testing.T) {
	root := filepath.Join(repoRoot(t), "template", ".claude")
	restated := regexp.MustCompile(`\$\{GATE_REMOTE_WORKFLOW:?-[^}]`)
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".sh") || strings.Contains(p, "/checks/") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if restated.Match(b) {
			t.Errorf("%s restates a default for GATE_REMOTE_WORKFLOW; conf.sh is the one place", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	conf, _ := os.ReadFile(filepath.Join(root, "lib", "conf.sh"))
	if !strings.Contains(string(conf), "\nGATE_REMOTE_WORKFLOW=") {
		t.Error("conf.sh does not state GATE_REMOTE_WORKFLOW's default")
	}
}
