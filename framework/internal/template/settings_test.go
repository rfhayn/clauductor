package template

import (
	"encoding/json"
	"os"
	"os/exec"
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

// OPS-75: a deny the template used to ship and retired leaves a project on update, reported as a
// remove with its reason; the project's own denies stay. Without it, an installed project kept the
// gh deny while gaining the sandbox key meant to pair with its removal.
func TestMergeSettingsRetiresDeny(t *testing.T) {
	tmpl := templateSettings(t)
	have := []byte(`{"permissions":{"deny":["Read(~/.config/gh/**)","Read(./secrets/**)"]}}`)
	merged, changes, err := MergeSettings(have, tmpl, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(merged), "~/.config/gh") {
		t.Errorf("the retired gh deny survived the merge:\n%s", merged)
	}
	if !strings.Contains(string(merged), "Read(./secrets/**)") {
		t.Errorf("the project's own deny was dropped:\n%s", merged)
	}
	removed := false
	for _, c := range changes {
		if c.Action == "remove" && strings.Contains(c.Have, "~/.config/gh") && c.Note != "" {
			removed = true
		}
	}
	if !removed {
		t.Errorf("no remove with a reason reported for the gh deny: %+v", changes)
	}
}

// OPS-72: a project installed before the gate and gate-wait were excluded from the sandbox as lone
// commands gains those entries on update, added beside its own; nothing it had is lost.
func TestMergeSettingsAddsGateExclusions(t *testing.T) {
	tmpl := templateSettings(t)
	have := []byte(`{"permissions":{"allow":["Bash(scripts/ci/gate.sh *)"]},"sandbox":{"excludedCommands":["scripts/ci/gate.sh *","make *"]}}`)
	merged, changes, err := MergeSettings(have, tmpl, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"scripts/ci/gate.sh"`, `"scripts/ci/gate-wait.sh"`, `"scripts/ci/gate-wait.sh *"`, `"Bash(scripts/ci/gate-wait.sh)"`, `"make *"`} {
		if !strings.Contains(string(merged), want) {
			t.Errorf("merged settings lack %s:\n%s", want, merged)
		}
	}
	added := map[string]bool{}
	for _, c := range changes {
		if c.Action == "add" && c.Path == "sandbox.excludedCommands" {
			added[c.Want] = true
		}
	}
	if !added[`"scripts/ci/gate-wait.sh *"`] {
		t.Errorf("the gate-wait exclusion is not reported added: %+v", changes)
	}
	// The project's own allow entry for the gate survives, once: neither dropped nor duplicated
	// by the template's copy of it, and no change touches it.
	var m struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(merged, &m); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, a := range m.Permissions.Allow {
		if a == "Bash(scripts/ci/gate.sh *)" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("permissions.allow holds Bash(scripts/ci/gate.sh *) %d times, want 1:\n%s", n, merged)
	}
	for _, c := range changes {
		if strings.Contains(c.Have, "Bash(scripts/ci/gate.sh *)") || strings.Contains(c.Want, "Bash(scripts/ci/gate.sh *)") {
			t.Errorf("the merge reported a change to the project's own gate allow entry: %+v", c)
		}
	}
}

// OPS-72: install (Mapping), the rendered sandbox exclusion (RenderSettings) and the shell's
// gate_wait_path (conf.sh, read by checks/settings.sh and project-config.sh's gateWait) put
// gate-wait.sh on the same path for every shape of GATE_RUN. A disagreement fails the settings
// check on every run of that project, or names a gate-wait the agent cannot run.
func TestGateWaitPathAgrees(t *testing.T) {
	conf := filepath.Join(repoRoot(t), "template", ".claude", "lib", "conf.sh")
	files := []string{"scripts/ci/run-local.sh", "scripts/ci/gate-wait.sh", "scripts/ci/lib/gate-state.sh"}
	for _, run := range []string{"", "scripts/ci/run-local.sh", "ci/run-local.sh", "./ci/run-local.sh", "run-local.sh", "./run-local.sh", "bin/ci.sh"} {
		project := ""
		if run != "" {
			project = "GATE_RUN=\"" + run + "\"\n"
		}
		c := defaultsConf(t, project)
		m, err := c.Mapping(files)
		if err != nil {
			t.Fatalf("GATE_RUN=%q: %v", run, err)
		}
		dest, _, skip := m.Resolve("scripts/ci/gate-wait.sh")
		if skip != "" {
			t.Fatalf("GATE_RUN=%q: install skips gate-wait.sh: %s", run, skip)
		}
		out, _, err := RenderSettings(templateSettings(t), c)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), `"`+dest+` *"`) {
			t.Errorf("GATE_RUN=%q: install puts gate-wait at %s, but the rendered settings do not exclude %q", run, dest, dest+" *")
		}
		want := run
		if want == "" {
			want = "scripts/ci/run-local.sh"
		}
		cmd := exec.Command("sh", "-c", `. "$1"; GATE_RUN=$2; gate_wait_path`, "sh", conf, want)
		cmd.Env = append(os.Environ(), "ROOT="+t.TempDir())
		got, err := cmd.Output()
		if err != nil {
			t.Fatalf("GATE_RUN=%q: gate_wait_path: %v", run, err)
		}
		if g := strings.TrimSpace(string(got)); g != dest {
			t.Errorf("GATE_RUN=%q: conf.sh's gate_wait_path says %s, install puts it at %s", run, g, dest)
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
	for _, want := range []string{`"Bash(ci/gate.sh)"`, `"ci/run-local.sh *"`, `"ci/gate.sh"`, `"ci/gate.sh *"`, `"Bash(ci/gate-wait.sh)"`, `"ci/gate-wait.sh"`, `"ci/gate-wait.sh *"`} {
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
