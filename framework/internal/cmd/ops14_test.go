package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/template"
)

// The template's version marker is the binary's version: a release built from a tag ships the
// template of that tag, and template.Resolve refuses any other (OPS-14, finding 3).
func TestTemplateVersionMatchesBinary(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(tmplDir(t), template.VersionFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(b)); got != strings.TrimPrefix(Version, "v") {
		t.Fatalf("template/%s says %q but Version is %q: bump them together", template.VersionFile, got, Version)
	}
}

// oldModelClone is a fresh clone of a repository running the old model: its skills, hooks and
// settings, and none of its gitignored runtime state.
func oldModelClone(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, ".git/HEAD", "ref: refs/heads/main\n")
	for _, s := range []string{"claim", "spawn", "supervisor", "commit"} {
		write(t, dir, ".claude/skills/"+s+"/SKILL.md", "the old "+s+" skill, edited by the project\n")
	}
	write(t, dir, ".claude/skills/our-skill/SKILL.md", "the project's own skill\n")
	write(t, dir, ".claude/skills/session-start/SKILL.md", "an old session-start\n")
	write(t, dir, ".claude/hooks/session-register.sh", "#!/bin/bash\nclauductor register \"$X\"\n")
	write(t, dir, ".claude/hooks/heartbeat.sh", "#!/bin/bash\nclauductor heartbeat\n")
	write(t, dir, ".claude/agents/pre-implementation.md", "an agent the project edited\n")
	write(t, dir, ".claude/settings.json", `{
  "hooks": {
    "SessionStart": [{"hooks": [{"type": "command", "command": "bash .claude/hooks/session-register.sh", "async": true}]}],
    "PostToolUse": [{"matcher": "Bash|Edit|Write", "hooks": [{"type": "command", "command": "bash .claude/hooks/heartbeat.sh", "async": true}]}]
  },
  "permissions": {"allow": ["Bash(git status*)"]}
}
`)
	return dir
}

// Findings 1, 2 and 7 together: a fresh clone of an old-model repository is recognised as
// clauductor's (no --force), the old model's replaced files are listed and, with --prune,
// removed, and settings.json no longer registers the old hooks; the project's own files stay.
func TestInstallOverAnOldModelClone(t *testing.T) {
	tmplDir(t)
	dir := oldModelClone(t)
	out, err := runInstall(t, dir, true)
	if err != nil {
		t.Fatalf("a fresh clone of an old-model repo should install without --force: %v\n%s", err, out)
	}
	for _, want := range []string{"clauductor's old operating model", "Template: ", "OLD MODEL", ".claude/skills/claim/SKILL.md", ".claude/hooks/heartbeat.sh"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output lacks %q", want)
		}
	}
	if strings.Contains(out, "pre-implementation.md (") {
		t.Error("an edited old agent is the project's now: it must not be listed for removal")
	}

	pruneOld = true
	t.Cleanup(func() { pruneOld = false })
	out, err = runInstall(t, dir, false)
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, gone := range []string{".claude/skills/claim", ".claude/skills/spawn", ".claude/skills/supervisor", ".claude/skills/commit", ".claude/hooks/session-register.sh", ".claude/hooks/heartbeat.sh"} {
		if fileExists(filepath.Join(dir, gone)) {
			t.Errorf("--prune left %s", gone)
		}
	}
	for _, kept := range []string{".claude/skills/our-skill/SKILL.md", ".claude/agents/pre-implementation.md"} {
		if !fileExists(filepath.Join(dir, kept)) {
			t.Errorf("--prune removed the project's %s", kept)
		}
	}
	settings, _ := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	for _, h := range []string{"session-register.sh", "heartbeat.sh"} {
		if strings.Contains(string(settings), h) {
			t.Errorf("settings.json still registers %s", h)
		}
	}
	if !strings.Contains(string(settings), "Bash(git status*)") {
		t.Error("the project's own allow entry was lost")
	}
}

// Without --prune and with no one to answer, nothing is removed.
func TestInstallKeepsOldFilesWithoutAYes(t *testing.T) {
	tmplDir(t)
	dir := oldModelClone(t)
	old := stdin
	t.Cleanup(func() { stdin = old })
	stdin = bufioReader("")
	if out, err := runInstall(t, dir, false); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !fileExists(filepath.Join(dir, ".claude/skills/claim/SKILL.md")) {
		t.Fatal("an unanswered prompt removed an old-model file")
	}
}

// Finding 5: install writes the panel preset with the project's branch keys.
func TestInstallRendersPanelFromProjectConf(t *testing.T) {
	tmplDir(t)
	dir := ownedRepo(t)
	write(t, dir, ".claude/project.conf", "BRANCH_CHANGE=\"feature/\"\n")
	if out, err := runInstall(t, dir, false); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".clauductor/panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Lanes     map[string]string `json:"lanes"`
		Templates []struct {
			ID            string `json:"id"`
			BranchPattern string `json:"branch_pattern"`
		} `json:"templates"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	if p.Lanes["feature/"] != "build" || p.Lanes["change/"] != "" {
		t.Errorf("lanes not rendered from BRANCH_CHANGE: %v", p.Lanes)
	}
	for _, tm := range p.Templates {
		if strings.HasPrefix(tm.BranchPattern, "change/") {
			t.Errorf("template %s still uses change/", tm.ID)
		}
	}
}

// Finding 6: update offers the doc-tier files the template added (and creates them only when
// asked), merges model-roles.json's new keys without changing a value, and lists AGENTS.md rows.
func TestUpdateOffersProjectFiles(t *testing.T) {
	tmpl := tmplDir(t)
	dir := ownedRepo(t)
	write(t, dir, ".claude/skills/.keep", "")
	write(t, dir, ".claude/model-roles.json", `{"roles": {"builder": {"model": "sonnet", "effort": "low"}}, "attribution": {"enabled": false, "trailer": ""}}`+"\n")
	write(t, dir, "AGENTS.md", "### What executes each rule\n| Rule / convention | What executes it |\n|---|---|\n| only ours | x |\n")
	write(t, dir, ".claude/health/flow.sh", "the project's own flow line\n")

	var out bytes.Buffer
	updateCmd.SetOut(&out)
	old := stdin
	stdin = bufioReader("")
	createMissing = true
	t.Cleanup(func() { updateCmd.SetOut(nil); stdin = old; createMissing = false })
	t.Chdir(dir)
	if err := updateCmd.RunE(updateCmd, nil); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	s := out.String()
	for _, want := range []string{"NEW PROJECT FILES", "MODEL ROLES", "AGENTS.md — rows"} {
		if !strings.Contains(s, want) {
			t.Errorf("update output lacks %q", want)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".claude/health/flow.sh")); string(got) != "the project's own flow line\n" {
		t.Error("update overwrote an existing project file")
	}
	for _, f := range []string{".claude/health/push-main.sh", "docs/playbook.md"} {
		if _, err := os.Stat(filepath.Join(tmpl, f)); err == nil && !fileExists(filepath.Join(dir, f)) {
			t.Errorf("--create-missing did not create %s", f)
		}
	}
	var roles map[string]any
	raw, _ := os.ReadFile(filepath.Join(dir, ".claude/model-roles.json"))
	if err := json.Unmarshal(raw, &roles); err != nil {
		t.Fatal(err)
	}
	b := roles["roles"].(map[string]any)["builder"].(map[string]any)
	if b["model"] != "sonnet" || roles["attribution"].(map[string]any)["enabled"] != false {
		t.Errorf("update changed a project value: %v", roles)
	}
	if roles["provenance"] == nil || roles["roles"].(map[string]any)["reviewer"] == nil {
		t.Error("update did not add the template's new model-roles keys")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); !strings.Contains(string(got), "only ours") || strings.Count(string(got), "\n") != 4 {
		t.Error("update edited AGENTS.md; it should only suggest rows")
	}
}

func bufioReader(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }
