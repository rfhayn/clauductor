package template

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "oldmodel", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// OPS-8 rehearsal, finding 1: a fresh clone of an old-model repository has no
// orchestration/config.json (gitignored), and looked foreign. Its tracked files are enough.
func TestOldModelSignsFromTrackedFiles(t *testing.T) {
	dir := t.TempDir()
	if s := OldModelSigns(dir); len(s) != 0 {
		t.Fatalf("an empty repo shows no signs; got %v", s)
	}
	// One unedited old file (the template's heartbeat hook, byte for byte).
	put(t, dir, ".claude/hooks/heartbeat.sh", testdata(t, "heartbeat.sh"))
	if s := OldModelSigns(dir); len(s) == 0 {
		t.Fatal("an unedited old-model hook is a sign")
	}

	// Edited skills only: three distinctive names are a sign, two are not.
	d2 := t.TempDir()
	put(t, d2, ".claude/skills/claim/SKILL.md", "edited\n")
	put(t, d2, ".claude/skills/spawn/SKILL.md", "edited\n")
	if s := OldModelSigns(d2); len(s) != 0 {
		t.Fatalf("two skill names are not enough; got %v", s)
	}
	put(t, d2, ".claude/skills/supervisor/SKILL.md", "edited\n")
	if s := OldModelSigns(d2); len(s) != 1 || !strings.Contains(s[0], "claim, spawn, supervisor") {
		t.Fatalf("three of the old model's skills are a sign; got %v", s)
	}

	// The settings registrations alone.
	d3 := t.TempDir()
	put(t, d3, SettingsPath, testdata(t, "settings.json"))
	if s := OldModelSigns(d3); len(s) != 1 || !strings.Contains(s[0], "session-register.sh") {
		t.Fatalf("settings.json registering the old hooks is a sign; got %v", s)
	}

	// A project's own skill named like a generic old one (review, status) is not.
	d4 := t.TempDir()
	put(t, d4, ".claude/skills/review/SKILL.md", "ours\n")
	put(t, d4, ".claude/skills/status/SKILL.md", "ours\n")
	put(t, d4, ".claude/hooks/heartbeat.sh", "#!/bin/sh\necho our own heartbeat\n")
	if s := OldModelSigns(d4); len(s) != 0 {
		t.Fatalf("a project's own review/status skills and heartbeat hook are not the old model; got %v", s)
	}
}

// Finding 2: the files to prune are the old model's skills and hooks (edited or not: they were
// its framework tier), and its agents only when unedited. Never a project file, never a template
// file, never an edited old agent.
func TestStaleOldModelListsOnlyTheOldModel(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, ".claude/skills/claim/SKILL.md", "edited old skill\n")
	put(t, dir, ".claude/skills/our-skill/SKILL.md", "the project's\n")
	put(t, dir, ".claude/skills/session-start/SKILL.md", "an old copy of a skill the template ships\n")
	put(t, dir, ".claude/hooks/heartbeat.sh", testdata(t, "heartbeat.sh"))
	put(t, dir, ".claude/hooks/our-hook.sh", "ours\n")
	put(t, dir, ".claude/agents/session-wrap.md", "edited old agent\n")
	got := StaleOldModel(dir, []string{".claude/skills/session-start/SKILL.md"})
	var paths []string
	for _, f := range got {
		paths = append(paths, f.Path)
	}
	want := ".claude/hooks/heartbeat.sh .claude/skills/claim/SKILL.md"
	if strings.Join(paths, " ") != want {
		t.Fatalf("want %s; got %v", want, paths)
	}
	if !got[0].Unchanged || got[1].Unchanged {
		t.Fatalf("unchanged flags wrong: %+v", got)
	}
	removed, err := PruneOldModel(dir, got)
	if err != nil || len(removed) != 2 {
		t.Fatalf("prune: %v %v", removed, err)
	}
	for _, p := range []string{".claude/skills/claim", ".claude/hooks/heartbeat.sh"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err == nil {
			t.Errorf("%s survived the prune", p)
		}
	}
	for _, p := range []string{".claude/skills/our-skill/SKILL.md", ".claude/hooks/our-hook.sh", ".claude/agents/session-wrap.md", ".claude/skills/session-start/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("the prune removed %s", p)
		}
	}
}

// Finding 7: the merge drops a registration whose script under .claude/hooks/ will not exist, and
// every registration of an old-model hook; a project hook whose script exists is kept.
func TestSettingsMergeDropsDeadAndOldHooks(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, ".claude/hooks/ours.sh", "#!/bin/sh\n")
	put(t, dir, ".claude/hooks/heartbeat.sh", "#!/bin/sh\nclauductor heartbeat\n") // exists, but the old model's
	have := `{"hooks": {
  "SessionStart": [{"hooks": [{"type": "command", "command": "bash .claude/hooks/session-register.sh", "async": true}]}],
  "PostToolUse": [{"matcher": "Bash", "hooks": [
    {"type": "command", "command": "bash .claude/hooks/heartbeat.sh"},
    {"type": "command", "command": "sh .claude/hooks/ours.sh"},
    {"type": "command", "command": "sh .claude/hooks/gone.sh"},
    {"type": "command", "command": "node scripts/x.mjs"}
  ]}]
}}`
	tmpl, err := os.ReadFile(filepath.Join(repoRoot(t), "template", filepath.FromSlash(SettingsPath)))
	if err != nil {
		t.Fatal(err)
	}
	prune := TemplateHookPrune(dir, filepath.Join(repoRoot(t), "template"))
	p, err := PlanSettings([]byte(have), true, tmpl, nil, prune)
	if err != nil {
		t.Fatal(err)
	}
	out := string(p.Result)
	for _, gone := range []string{"session-register.sh", "heartbeat.sh", "gone.sh"} {
		if strings.Contains(out, gone) {
			t.Errorf("the merged settings still register %s", gone)
		}
	}
	for _, kept := range []string{"ours.sh", "scripts/x.mjs"} {
		if !strings.Contains(out, kept) {
			t.Errorf("the merge dropped the project's %s", kept)
		}
	}
	notes := 0
	for _, c := range p.Changes {
		if c.Action == "remove" && (strings.Contains(c.Note, "old model") || strings.Contains(c.Note, "does not exist")) {
			notes++
		}
	}
	if notes != 3 {
		t.Errorf("want 3 reported removals; got %d: %+v", notes, p.Changes)
	}
	var v any
	if err := json.Unmarshal(p.Result, &v); err != nil {
		t.Fatalf("merged settings are not JSON: %v", err)
	}
	// Without the prune (diff of a plain merge), the dead registration stays: the test above is
	// what fails without the fix.
	p2, _ := PlanSettings([]byte(have), true, tmpl, nil, nil)
	if !strings.Contains(string(p2.Result), "gone.sh") {
		t.Fatal("control: a merge with no prune keeps every project hook")
	}
}
