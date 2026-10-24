package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// OPS-119: a role's eval that holds a receipt is the project's whole. The template's reviewer
// still carries the unmeasured "baseline"; update added it beside this repo's receipt every run,
// since it adds every key the project lacks, and session 6 restored the file by hand each time.
func TestUpdateKeepsAnEvalReceiptWhole(t *testing.T) {
	tmpl := tmplDir(t)
	// The case needs the template's reviewer to still carry the baseline; without it this passes
	// without exercising the guard.
	var tr map[string]any
	traw, _ := os.ReadFile(filepath.Join(tmpl, ".claude/model-roles.json"))
	if err := json.Unmarshal(traw, &tr); err != nil {
		t.Fatal(err)
	}
	if _, ok := tr["roles"].(map[string]any)["reviewer"].(map[string]any)["eval"].(map[string]any)["baseline"]; !ok {
		t.Fatal("the template's reviewer eval no longer holds a baseline; give this test a fixture")
	}
	dir := ownedRepo(t)
	write(t, dir, ".claude/skills/.keep", "")
	write(t, dir, ".claude/model-roles.json", `{"roles": {"reviewer": {"model": "opus", "effort": "high", "eval": {"receipt": ".claude/evals/receipts/r.json", "recall": 1, "why": "measured"}}}}`+"\n")

	var out bytes.Buffer
	updateCmd.SetOut(&out)
	old := stdin
	stdin = bufioReader("")
	t.Cleanup(func() { updateCmd.SetOut(nil); stdin = old })
	t.Chdir(dir)
	if err := updateCmd.RunE(updateCmd, nil); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	var roles map[string]any
	raw, _ := os.ReadFile(filepath.Join(dir, ".claude/model-roles.json"))
	if err := json.Unmarshal(raw, &roles); err != nil {
		t.Fatal(err)
	}
	r := roles["roles"].(map[string]any)
	ev := r["reviewer"].(map[string]any)["eval"].(map[string]any)
	if _, ok := ev["baseline"]; ok {
		t.Errorf("update added the template's baseline beside the project's receipt: %v", ev)
	}
	if ev["receipt"] != ".claude/evals/receipts/r.json" || ev["why"] != "measured" {
		t.Errorf("update changed the project's receipt: %v", ev)
	}
	// Add-only still holds everywhere else: a role the project lacks arrives, and the reviewer
	// around its kept eval still gains the template's tiers.
	if b, ok := r["builder"].(map[string]any); !ok || b["model"] == nil {
		t.Error("update stopped adding the template's other roles")
	}
	if r["reviewer"].(map[string]any)["tiers"] == nil {
		t.Error("update stopped adding the template's keys to an existing role")
	}
}
