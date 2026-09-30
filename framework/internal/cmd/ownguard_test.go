package cmd

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/template"
)

// tmplDir is this checkout's template, which install reads through CLAUDUCTOR_FRAMEWORK.
func tmplDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDUCTOR_FRAMEWORK", root)
	return filepath.Join(root, "template")
}

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func files(t *testing.T) []string {
	t.Helper()
	fs, err := template.ListTemplateFiles()
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

func TestForeignModelEmptyRepoTouchesNothingOwned(t *testing.T) {
	tmpl := tmplDir(t)
	dir := t.TempDir()
	write(t, dir, "README.md", "an app\n")
	over, add, err := foreignModel(dir, tmpl, files(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(over)+len(add) != 0 {
		t.Fatalf("a repo with no model of its own is safe to install into; got overwrite %v, add %v", over, add)
	}
}

// A StandingT-shaped repo: its own merge-pr skill and settings, an AGENTS.md, and one file that
// happens to equal the template's.
func TestForeignModelNamesOverwritesAndAdds(t *testing.T) {
	tmpl := tmplDir(t)
	dir := t.TempDir()
	write(t, dir, ".claude/skills/merge-pr/SKILL.md", "the project's own merge-pr\n")
	write(t, dir, ".claude/settings.json", "{}\n")
	write(t, dir, "AGENTS.md", "the project's own rules\n")
	same, err := os.ReadFile(filepath.Join(tmpl, ".claude/lib/conf.sh"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, ".claude/lib/conf.sh", string(same))

	over, add, err := foreignModel(dir, tmpl, files(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{".claude/skills/merge-pr/SKILL.md", ".claude/settings.json"} {
		if !slices.Contains(over, f) {
			t.Errorf("overwrite should name %s; got %v", f, over)
		}
	}
	for _, f := range []string{".claude/lib/conf.sh", "AGENTS.md"} {
		if slices.Contains(over, f) {
			t.Errorf("overwrite must not name %s (identical, or a doc that is never replaced)", f)
		}
	}
	if !slices.Contains(add, ".claude/hooks/pr-merge-guard.sh") {
		t.Errorf("add should name the framework files an install would scatter beside the project's; got %v", add)
	}
	for _, f := range add {
		if classifyFile(f) != tierFramework {
			t.Errorf("add names %s, which is not a framework file", f)
		}
	}
}

func TestOwnedByClauductor(t *testing.T) {
	dir := t.TempDir()
	if ownedByClauductor(dir) {
		t.Fatal("a bare repo is not clauductor's")
	}
	write(t, dir, "orchestration/config.json", "{}")
	if !ownedByClauductor(dir) {
		t.Fatal("an old-model install (orchestration/config.json) is clauductor's")
	}
	dir2 := t.TempDir()
	if err := writeInstallMarker(dir2); err != nil {
		t.Fatal(err)
	}
	if !ownedByClauductor(dir2) {
		t.Fatal("the marker makes a repo clauductor's")
	}
}

// The command itself, in a repo with its own model: it refuses, names the file, and changes
// nothing; --dry-run reports the refusal without failing.
func TestInstallRefusesAnOwnModel(t *testing.T) {
	tmplDir(t)
	dir := t.TempDir()
	write(t, dir, ".git/HEAD", "ref: refs/heads/main\n")
	write(t, dir, ".claude/skills/session-start/SKILL.md", "the project's own session-start\n")
	t.Chdir(dir)
	t.Cleanup(func() { dryRun, forceInstall = false, false })

	err := installCmd.RunE(installCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "install refused") ||
		!strings.Contains(err.Error(), ".claude/skills/session-start/SKILL.md") {
		t.Fatalf("want a refusal naming the project's session-start; got %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".claude/skills/session-start/SKILL.md")); string(got) != "the project's own session-start\n" {
		t.Fatalf("the refusal changed the project's file: %q", got)
	}
	for _, f := range []string{installMarker, "AGENTS.md", ".claude/hooks/pr-merge-guard.sh"} {
		if fileExists(filepath.Join(dir, f)) {
			t.Errorf("the refusal wrote %s", f)
		}
	}

	dryRun = true
	if err := installCmd.RunE(installCmd, nil); err != nil {
		t.Fatalf("--dry-run should report the refusal, not fail: %v", err)
	}
}

func TestUpdateRefusesAnOwnModel(t *testing.T) {
	tmplDir(t)
	dir := t.TempDir()
	write(t, dir, ".claude/skills/merge-pr/SKILL.md", "the project's own merge-pr\n")
	t.Chdir(dir)
	err := updateCmd.RunE(updateCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "update refused") {
		t.Fatalf("want update refused; got %v", err)
	}
	if fileExists(filepath.Join(dir, ".claude/hooks/pr-merge-guard.sh")) {
		t.Fatal("the refusal added a framework file")
	}
}
