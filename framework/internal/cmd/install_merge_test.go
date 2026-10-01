package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/template"
	"github.com/spf13/pflag"
)

// installed runs `clauductor install` in dir (a git repo clauductor owns) and returns its output.
func runInstall(t *testing.T, dir string, dry bool) (string, error) {
	t.Helper()
	var out bytes.Buffer
	installCmd.SetOut(&out)
	dryRun = dry
	t.Cleanup(func() { dryRun, forceInstall = false, false; installCmd.SetOut(nil) })
	t.Chdir(dir)
	err := installCmd.RunE(installCmd, nil)
	return out.String(), err
}

// ownedRepo is a git repository clauductor installed into before.
func ownedRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, ".git/HEAD", "ref: refs/heads/main\n")
	if err := writeInstallMarker(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s is not valid JSON: %v\n%s", path, err, raw)
	}
	return m
}

func strs(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// hookCommands lists every hook command registered under event.
func hookCommands(s map[string]any, event string) []string {
	var out []string
	hooks, _ := s["hooks"].(map[string]any)
	groups, _ := hooks[event].([]any)
	for _, g := range groups {
		hs, _ := g.(map[string]any)["hooks"].([]any)
		for _, h := range hs {
			out = append(out, h.(map[string]any)["command"].(string))
		}
	}
	return out
}

const projectSettings = `{
  "model": "sonnet",
  "effortLevel": "medium",
  "env": {"PROJECT_VAR": "1"},
  "skillOverrides": {"our-skill": "user-invocable-only"},
  "enabledPlugins": {"something@market": true},
  "statusLine": {"type": "command", "command": "sh .claude/statusline.sh"},
  "permissions": {
    "allow": ["Bash(npm test)"],
    "deny": ["Read(./secrets/**)"]
  },
  "hooks": {
    "PostToolUse": [
      {"matcher": "Write|Edit", "hooks": [
        {"type": "command", "command": "sh .claude/hooks/format.sh"},
        {"type": "command", "command": "node scripts/our-hook.mjs"}
      ]}
    ],
    "Stop": [{"hooks": [{"type": "command", "command": "sh scripts/on-stop.sh"}]}]
  }
}
`

// B1/P1.1: install merges settings.json. The project's keys survive; the model's hooks, status
// line, deny list and sandbox arrive; the old cwd-relative registration of a model hook is
// replaced, not duplicated; and each disagreement is reported.
func TestInstallMergesSettings(t *testing.T) {
	tmpl := tmplDir(t)
	dir := ownedRepo(t)
	write(t, dir, ".claude/settings.json", projectSettings)

	out, err := runInstall(t, dir, false)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	s := readJSON(t, filepath.Join(dir, ".claude/settings.json"))
	want := readJSON(t, filepath.Join(tmpl, ".claude/settings.json"))

	if s["model"] != "sonnet" || s["effortLevel"] != "medium" {
		t.Errorf("project scalars not kept: model %v, effortLevel %v", s["model"], s["effortLevel"])
	}
	if s["env"].(map[string]any)["PROJECT_VAR"] != "1" {
		t.Errorf("env lost the project's variable: %v", s["env"])
	}
	if s["skillOverrides"].(map[string]any)["our-skill"] == nil || s["enabledPlugins"].(map[string]any)["something@market"] != true {
		t.Errorf("skillOverrides or enabledPlugins lost the project's entries")
	}
	perms := s["permissions"].(map[string]any)
	if !slices.Contains(strs(perms["allow"]), "Bash(npm test)") || !slices.Contains(strs(perms["deny"]), "Read(./secrets/**)") {
		t.Errorf("the project's own permissions were dropped: %v", perms)
	}
	for _, r := range strs(want["permissions"].(map[string]any)["deny"]) {
		if !slices.Contains(strs(perms["deny"]), r) {
			t.Errorf("the model's deny rule %s is missing", r)
		}
	}
	if s["sandbox"] == nil || s["sandbox"].(map[string]any)["enabled"] != true {
		t.Errorf("the model's sandbox did not arrive: %v", s["sandbox"])
	}
	if got, w := s["statusLine"].(map[string]any)["command"], want["statusLine"].(map[string]any)["command"]; got != w {
		t.Errorf("statusLine is %v, want the template's %v", got, w)
	}
	post := hookCommands(s, "PostToolUse")
	if slices.Contains(post, "sh .claude/hooks/format.sh") {
		t.Errorf("the old cwd-relative format.sh registration is still there: %v", post)
	}
	if !slices.Contains(post, `sh "$CLAUDE_PROJECT_DIR"/.claude/hooks/format.sh`) || !slices.Contains(post, "node scripts/our-hook.mjs") {
		t.Errorf("PostToolUse should hold the model's format.sh and the project's hook: %v", post)
	}
	if !slices.Equal(hookCommands(s, "Stop"), []string{"sh scripts/on-stop.sh"}) {
		t.Errorf("the project's Stop hook was dropped: %v", hookCommands(s, "Stop"))
	}
	for _, ev := range []string{"UserPromptSubmit", "PreToolUse"} {
		for _, c := range hookCommands(want, ev) {
			if !slices.Contains(hookCommands(s, ev), c) {
				t.Errorf("the model's %s hook %s is not registered", ev, c)
			}
		}
	}
	for _, w := range []string{"= model: the project's \"sonnet\" kept", "! statusLine"} {
		if !strings.Contains(out, w) {
			t.Errorf("the report should say %q:\n%s", w, out)
		}
	}

	// A second install changes nothing: the merge is a fixed point.
	before, _ := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if out, err := runInstall(t, dir, false); err != nil {
		t.Fatalf("second install: %v\n%s", err, out)
	}
	after, _ := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if !bytes.Equal(before, after) {
		t.Errorf("a second install changed settings.json:\n%s", template.UnifiedDiff(string(before), string(after), "first", "second"))
	}
}

// --dry-run shows the settings.json diff and writes nothing.
func TestInstallDryRunShowsSettingsDiff(t *testing.T) {
	tmplDir(t)
	dir := ownedRepo(t)
	write(t, dir, ".claude/settings.json", projectSettings)
	out, err := runInstall(t, dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "--- project/.claude/settings.json") || !strings.Contains(out, "+++ merged/.claude/settings.json") {
		t.Errorf("--dry-run should print the settings.json diff:\n%s", out)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".claude/settings.json")); string(got) != projectSettings {
		t.Error("--dry-run changed settings.json")
	}
	if fileExists(filepath.Join(dir, "AGENTS.md")) {
		t.Error("--dry-run wrote files")
	}
}

// B2 + B11: a project whose gate lives in tools/ci gets its gate scripts there, and settings.json
// allows and un-sandboxes them there; the defaults an earlier install wrote are dropped.
func TestInstallFollowsGateRun(t *testing.T) {
	tmplDir(t)
	dir := ownedRepo(t)
	write(t, dir, ".claude/project.conf", "GATE_RUN=\"tools/ci/run-local.sh\"\nGATE=\"tools/ci/gate.sh\"\nGATE_STEPS=\"tools/ci/steps.sh\"\n")
	write(t, dir, ".claude/settings.json", `{"permissions": {"allow": ["Bash(scripts/ci/gate.sh)", "Bash(scripts/ci/gate.sh *)"]}}`)
	if out, err := runInstall(t, dir, false); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	for _, f := range []string{"tools/ci/run-local.sh", "tools/ci/gate.sh", "tools/ci/lease.sh", "tools/ci/steps.sh",
		"tools/ci/lib/steps.sh", "tools/ci/lease-conformance/run.sh", "tools/ci/lease-conformance/VERSION"} {
		if !fileExists(filepath.Join(dir, f)) {
			t.Errorf("%s was not installed", f)
		}
	}
	if fileExists(filepath.Join(dir, "scripts/ci")) {
		t.Error("install wrote scripts/ci/ although project.conf puts the gate in tools/ci")
	}
	s := readJSON(t, filepath.Join(dir, ".claude/settings.json"))
	allow := strs(s["permissions"].(map[string]any)["allow"])
	excl := strs(s["sandbox"].(map[string]any)["excludedCommands"])
	if !slices.Contains(allow, "Bash(tools/ci/gate.sh)") || !slices.Contains(excl, "tools/ci/run-local.sh *") || !slices.Contains(excl, "tools/ci/gate.sh *") {
		t.Errorf("settings.json does not name the project's gate: allow %v, excluded %v", allow, excl)
	}
	for _, e := range append(allow, excl...) {
		if strings.Contains(e, "scripts/ci/") {
			t.Errorf("settings.json still names the default gate path: %s", e)
		}
	}
	// update compares at the same paths: nothing to do.
	diffs, err := template.FindDiffs(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range diffs {
		t.Errorf("update after install would still touch %s → %s (%s)", d.Path, d.Dest, d.Status)
	}
}

// A GATE_RUN with another file name is the project's own runner: install leaves the template's
// runner and lease out. A path outside the repository, or two files on one path, is refused.
func TestInstallGateRunOwnRunnerAndAmbiguity(t *testing.T) {
	tmplDir(t)
	dir := ownedRepo(t)
	write(t, dir, ".claude/project.conf", "GATE_RUN=\"scripts/ci/run-all.mjs\"\n")
	out, err := runInstall(t, dir, false)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if fileExists(filepath.Join(dir, "scripts/ci/run-local.sh")) || fileExists(filepath.Join(dir, "scripts/ci/lease.sh")) {
		t.Error("install wrote the template's runner beside the project's own")
	}
	if !fileExists(filepath.Join(dir, "scripts/ci/gate.sh")) || !strings.Contains(out, "the project's own gate runner") {
		t.Errorf("gate.sh should still install and the skip be reported:\n%s", out)
	}
	// The project's own runner calls the model's steps from the library, and runs its own lease
	// through the conformance kit: both still install (P1.6, P1.12).
	for _, f := range []string{"scripts/ci/lib/steps.sh", "scripts/ci/lease-conformance/run.sh"} {
		if !fileExists(filepath.Join(dir, f)) {
			t.Errorf("%s was not installed beside the project's own runner", f)
		}
	}

	for conf, want := range map[string]string{
		"GATE_RUN=\"../elsewhere/run-local.sh\"\n":                                   "not a path inside the repository",
		"GATE_RUN=\"tools/ci/run-local.sh\"\nGATE_STEPS=\"tools/ci/run-local.sh\"\n": "puts both",
		"GATE_RUN=\"$HOME/run-local.sh\"\n":                                          "needs a shell to read",
	} {
		d := ownedRepo(t)
		write(t, d, ".claude/project.conf", conf)
		_, err := runInstall(t, d, false)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("project.conf %q: want a refusal saying %q, got %v", conf, want, err)
		}
		if fileExists(filepath.Join(d, "AGENTS.md")) {
			t.Errorf("project.conf %q: the refusal wrote files", conf)
		}
	}
}

// B16: the current model keeps no runtime state in orchestration/, so a fresh install neither
// creates it nor ignores it; a repository the old model left one in keeps it ignored.
func TestInstallNoOrchestrationForTheNewModel(t *testing.T) {
	tmpl := tmplDir(t)
	if b, _ := os.ReadFile(filepath.Join(tmpl, ".gitignore")); strings.Contains(string(b), "orchestration") {
		t.Error("template/.gitignore still names orchestration/")
	}
	dir := ownedRepo(t)
	write(t, dir, ".gitignore", "node_modules/\n")
	if out, err := runInstall(t, dir, false); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if strings.Contains(string(gi), "orchestration") || fileExists(filepath.Join(dir, "orchestration")) {
		t.Errorf("a new-model install created or ignored orchestration/:\n%s", gi)
	}
	if !strings.Contains(string(gi), ".claude/worktrees/") || !strings.Contains(string(gi), "node_modules/") {
		t.Errorf(".gitignore merge lost a line:\n%s", gi)
	}

	old := t.TempDir()
	write(t, old, ".git/HEAD", "ref: refs/heads/main\n")
	write(t, old, "orchestration/config.json", "{}")
	write(t, old, ".gitignore", "node_modules/\n")
	if out, err := runInstall(t, old, false); err != nil {
		t.Fatalf("install over the old model: %v\n%s", err, out)
	}
	if gi, _ := os.ReadFile(filepath.Join(old, ".gitignore")); !strings.Contains(string(gi), "orchestration/") {
		t.Errorf("the old model's orchestration/ must stay ignored:\n%s", gi)
	}
}

// B14: the status line runs from the project root like the hooks, not from the session's cwd.
func TestStatusLineUsesProjectDir(t *testing.T) {
	s := readJSON(t, filepath.Join(tmplDir(t), ".claude/settings.json"))
	if c := s["statusLine"].(map[string]any)["command"].(string); !strings.Contains(c, "CLAUDE_PROJECT_DIR") {
		t.Errorf("statusLine command %q is cwd-relative", c)
	}
}

// P1.11 + B3: update compares every framework-tier file install writes, enumerated from the
// template, not a hand list; and the five scripts the hand list missed are among them.
func TestUpdateRefreshesEveryFrameworkFile(t *testing.T) {
	tmplDir(t)
	dir := ownedRepo(t)
	fs := files(t)
	diffs, err := template.FindDiffs(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, d := range diffs {
		got[d.Path] = d.Status
	}
	n := 0
	for _, rel := range fs {
		if classifyFile(rel) != tierFramework {
			continue
		}
		n++
		if got[rel] != "new" {
			t.Errorf("update would not add the framework file %s to a repo that lacks it", rel)
		}
	}
	for _, rel := range []string{".claude/scenario-trace.sh", ".claude/change-approval.sh", ".claude/change-cost.sh", ".claude/verify-change.sh", ".claude/compound.sh"} {
		if got[rel] != "new" {
			t.Errorf("update misses %s", rel)
		}
	}
	// Every framework file present but changed is offered as modified.
	for _, rel := range fs {
		if classifyFile(rel) == tierFramework {
			write(t, dir, rel, "changed by the project\n")
		}
	}
	diffs, err = template.FindDiffs(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := 0
	for _, d := range diffs {
		if d.Status == "modified" && classifyFile(d.Path) == tierFramework {
			m++
		}
		if d.Path == template.SettingsPath {
			t.Error("settings.json is merged, not offered for overwrite")
		}
	}
	if m != n || n == 0 {
		t.Errorf("update offers %d of %d changed framework files", m, n)
	}
}

// B19: a repository with OpenSpec's vendored skills is warned that they and the template's
// trigger on the same requests, in the install refusal and in diff.
func TestSkillTriggerCollisionWarned(t *testing.T) {
	tmplDir(t)
	dir := t.TempDir()
	write(t, dir, ".git/HEAD", "ref: refs/heads/main\n")
	write(t, dir, ".claude/skills/openspec-propose/SKILL.md", "vendored\n")
	_, err := runInstall(t, dir, false)
	if err == nil || !strings.Contains(err.Error(), "skill trigger collision") || !strings.Contains(err.Error(), "openspec-propose and propose") {
		t.Fatalf("the refusal should warn of the openspec-propose / propose collision; got %v", err)
	}
}

func runDiff(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	diffCmd.SetOut(&out)
	t.Cleanup(func() {
		diffJSON, diffPath, diffExitCode, diffAll = false, "", false, false
		diffCmd.SetOut(nil)
		diffCmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	})
	if err := diffCmd.ParseFlags(args); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	err := diffCmd.RunE(diffCmd, nil)
	return out.String(), err
}

// P1.10: diff reads a repository that runs its own model (install refuses it), changes nothing,
// and reports each file, the settings merge and the collision.
func TestDiffReadsAForeignRepo(t *testing.T) {
	tmplDir(t)
	dir := t.TempDir()
	write(t, dir, ".claude/skills/merge-pr/SKILL.md", "the project's own merge-pr\n")
	write(t, dir, ".claude/skills/openspec-propose/SKILL.md", "vendored\n")
	write(t, dir, ".claude/settings.json", projectSettings)
	write(t, dir, "AGENTS.md", "ours\n")
	write(t, dir, ".claude/project.conf", "ROADMAP=\"docs/plan.md\"\n")
	snapshot := func() string {
		var b strings.Builder
		filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				c, _ := os.ReadFile(p)
				b.WriteString(p + "\n" + string(c))
			}
			return nil
		})
		return b.String()
	}
	before := snapshot()

	out, err := runDiff(t, dir, "--json")
	if err != nil {
		t.Fatalf("diff refused or failed: %v", err)
	}
	if snapshot() != before {
		t.Fatal("diff changed the repository")
	}
	var rep template.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("--json is not JSON: %v\n%s", err, out)
	}
	status := map[string]string{}
	for _, f := range rep.Files {
		key := f.Path
		if key == "" {
			key = f.Dest
		}
		status[key] = f.Status
	}
	for path, want := range map[string]string{
		".claude/skills/merge-pr/SKILL.md":         "differs",
		".claude/hooks/pr-merge-guard.sh":          "missing",
		".claude/skills/openspec-propose/SKILL.md": "extra",
		"AGENTS.md":            "present",
		"docs/insights-log.md": "missing",
		".claude/skills/architecture-audit/SKILL.md": "missing",
	} {
		if status[path] != want {
			t.Errorf("%s: %q, want %q", path, status[path], want)
		}
	}
	var roadmap *template.FileStatus
	for i := range rep.Files {
		if rep.Files[i].Path == "docs/roadmap.md" {
			roadmap = &rep.Files[i]
		}
	}
	if roadmap == nil || roadmap.Dest != "docs/plan.md" || roadmap.MappedBy != "ROADMAP" {
		t.Errorf("docs/roadmap.md should map to ROADMAP's docs/plan.md: %+v", roadmap)
	}
	if rep.Settings == nil || rep.Settings.Status != "merge" || rep.Settings.Conflicts == 0 || rep.Settings.Diff == "" {
		t.Errorf("settings preview: %+v", rep.Settings)
	}
	if !strings.Contains(strings.Join(rep.Warnings, "\n"), "skill trigger collision") {
		t.Errorf("warnings lack the collision: %v", rep.Warnings)
	}

	if _, err := runDiff(t, dir, "--exit-code"); err != errDiverged {
		t.Errorf("--exit-code on a diverged repo: %v, want errDiverged", err)
	}
	text, _ := runDiff(t, dir, "--path", ".claude/hooks/")
	if !strings.Contains(text, ".claude/hooks/pr-merge-guard.sh") || strings.Contains(text, "merge-pr/SKILL.md") || strings.Contains(text, "settings:") {
		t.Errorf("--path should keep only .claude/hooks/:\n%s", text)
	}
}

// After an install, diff finds the framework converged.
func TestDiffAfterInstallConverged(t *testing.T) {
	tmplDir(t)
	dir := ownedRepo(t)
	if out, err := runInstall(t, dir, false); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if out, err := runDiff(t, dir, "--exit-code"); err != nil {
		t.Fatalf("diff after install: %v\n%s", err, out)
	}
}

// P1.2: the local layer is the project's. install creates its README once and never writes over a
// file there, and update has nothing to refresh in it, however far it has drifted from the template.
func TestInstallNeverOverwritesTheLocalLayer(t *testing.T) {
	tmplDir(t)
	dir := ownedRepo(t)
	write(t, dir, ".claude/local/README.md", "our own notes\n")
	write(t, dir, ".claude/local/guard.d/ledger.sh", "exit 0\n")
	if out, err := runInstall(t, dir, false); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	for rel, want := range map[string]string{".claude/local/README.md": "our own notes\n", ".claude/local/guard.d/ledger.sh": "exit 0\n"} {
		if got, _ := os.ReadFile(filepath.Join(dir, rel)); string(got) != want {
			t.Errorf("install wrote over %s: %q", rel, got)
		}
	}
	diffs, err := template.FindDiffs(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range diffs {
		if strings.HasPrefix(d.Path, template.LocalDir) {
			t.Errorf("update would touch the local layer: %s (%s)", d.Path, d.Status)
		}
	}
	fresh := ownedRepo(t)
	if out, err := runInstall(t, fresh, false); err != nil {
		t.Fatalf("install: %v\n%s", err, out)
	}
	if !fileExists(filepath.Join(fresh, ".claude/local/README.md")) {
		t.Error("install did not create .claude/local/README.md in a repository without one")
	}
}
