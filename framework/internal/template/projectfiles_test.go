package template

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Finding 6: model-roles.json gains the template's new keys and keeps every value the project set.
func TestMergeAddOnlyKeepsProjectValues(t *testing.T) {
	have := `{"roles": {"builder": {"model": "sonnet", "effort": "low"}}, "attribution": {"enabled": false}, "mine": [1]}`
	want := `{"roles": {"builder": {"model": "opus", "effort": "high", "tiers": {"low": {"model": "sonnet"}}}, "reviewer": {"model": "opus"}},
	"attribution": {"enabled": true, "trailer": "Co-Authored-By: x"}, "mine": [2, 3], "evals": {"thresholds": {"recall": 0.8}}}`
	got, added, err := MergeAddOnly([]byte(have), []byte(want))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(added, " ") != "roles.builder.tiers roles.reviewer attribution.trailer evals" {
		t.Fatalf("added %v", added)
	}
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatalf("%v\n%s", err, got)
	}
	b := m["roles"].(map[string]any)["builder"].(map[string]any)
	if b["model"] != "sonnet" || b["effort"] != "low" {
		t.Errorf("the project's builder changed: %v", b)
	}
	if m["attribution"].(map[string]any)["enabled"] != false {
		t.Error("the project's attribution.enabled changed")
	}
	if len(m["mine"].([]any)) != 1 {
		t.Error("the project's array changed")
	}
	same, added, _ := MergeAddOnly([]byte(want), []byte(want))
	if len(added) != 0 || string(same) != want {
		t.Error("nothing to add must leave the file byte for byte")
	}
}

// Finding 5: the panel preset follows the project's branch keys at install, and a panel.json
// that disagrees with them is named.
func TestRenderPanelFollowsBranchKeys(t *testing.T) {
	root := repoRoot(t)
	tmpl, err := os.ReadFile(filepath.Join(root, "template", filepath.FromSlash(PanelPath)))
	if err != nil {
		t.Fatal(err)
	}
	proj := t.TempDir()
	put(t, proj, ".claude/project.conf", "BRANCH_CHANGE=\"feature/\"\nBRANCH_OPS=\"chore/\"\n")
	c, err := LoadConf(proj, filepath.Join(root, "template"))
	if err != nil {
		t.Fatal(err)
	}
	if probs := PanelBranchProblems(tmpl, c); len(probs) == 0 {
		t.Fatal("the template's preset should disagree with BRANCH_CHANGE=feature/")
	}
	out, err := RenderPanel(tmpl, c)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`"feature/": "build"`, `"chore/": "ops"`, `"fix/": "fix"`, `"branch_pattern": "feature/{name}"`, `"branch_pattern": "chore/{name}"`} {
		if !strings.Contains(s, want) {
			t.Errorf("rendered panel lacks %s", want)
		}
	}
	if strings.Contains(s, `"change/`) || strings.Contains(s, `"ops/`) {
		t.Error("rendered panel still names the template's default prefixes")
	}
	if probs := PanelBranchProblems(out, c); len(probs) != 0 {
		t.Errorf("the rendered panel should agree with its keys: %v", probs)
	}
	def, _ := LoadConf(t.TempDir(), filepath.Join(root, "template"))
	if same, _ := RenderPanel(tmpl, def); string(same) != string(tmpl) {
		t.Error("with the default keys the preset is copied byte for byte")
	}
}

func TestMissingAgentsRows(t *testing.T) {
	proj := "# x\n\n### What executes each rule\n\n| Rule / convention | What executes it |\n|---|---|\n| A | a |\n| B | ours |\n\n## Next\n| C | c |\n"
	tmpl := "### What executes each rule\n| Rule / convention | What executes it |\n|---|---|\n| A | a |\n| B | b |\n| New rule | **check** |\n"
	got := MissingAgentsRows(proj, tmpl)
	if len(got) != 1 || got[0] != "| New rule | **check** |" {
		t.Fatalf("got %q", got)
	}
}
