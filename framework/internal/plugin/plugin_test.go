package plugin

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/template"
)

// The plugin and `clauductor install`/`update` must agree on what is the framework's: a file the
// plugin ships as a component but install treats as the project's is never updated by update
// (what happened to metrics.sh and usage-report.sh), and the reverse would overwrite a project's
// file. Two differences are deliberate: install keeps a project's edited agents, and the plugin
// scaffolds the gate scripts into the repository.
func TestPluginAndInstallAgreeOnTiers(t *testing.T) {
	files, err := listFiles(filepath.Join(repoRoot(t), "template"))
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range files {
		_, kind, err := place(rel)
		if err != nil {
			t.Fatal(err)
		}
		inPlugin := kind == kindPlugin
		fw := template.Classify(rel) == template.TierFramework
		if strings.HasPrefix(rel, ".claude/agents/") || template.FrameworkScripts[rel] || strings.HasPrefix(rel, "scripts/ci/") {
			// The gate scripts, its steps library and the lease conformance kit are scaffolded:
			// CI runs them where no plugin is installed.
			continue
		}
		if inPlugin != fw {
			t.Errorf("%s: the plugin ships it as a component = %v, install treats it as framework = %v", rel, inPlugin, fw)
		}
	}
}

// In CI, where no plugin is installed, the gate's resolver clones the plugin at the version the
// repository was scaffolded from, once, into a cache, and runs the script from there.
func TestResolverFetchesPinnedPluginInCI(t *testing.T) {
	need(t, "git", "sh")
	plug, _ := build(t) // Version 9.9.9-test: the resolver's default ref is v9.9.9-test
	home := t.TempDir()
	e := env(t, home)

	origin := filepath.Join(t.TempDir(), "clauductor")
	os.MkdirAll(filepath.Join(origin, "plugin", ".claude-plugin"), 0o755)
	os.WriteFile(filepath.Join(origin, "plugin", ".claude-plugin", "plugin.json"), []byte("{}\n"), 0o644)
	os.WriteFile(filepath.Join(origin, "plugin", "hello.sh"), []byte("echo \"hello $1\"\n"), 0o644)
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-qm", "plugin"}, {"tag", "v9.9.9-test"}} {
		if out, code := run(t, origin, e, "", "git", args...); code != 0 {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	p := newProject(t, e)
	if out, code := run(t, p, e, "", "sh", filepath.Join(plug, "scaffold.sh")); code != 0 {
		t.Fatal(out)
	}
	ce := append(e, "CI=true", "CLAUDUCTOR_REPO_URL=file://"+origin, "CLAUDUCTOR_CACHE="+filepath.Join(home, "cache"))
	if out, code := run(t, p, ce, "", "sh", "scripts/ci/clauductor-model.sh", "hello.sh", "ci"); code != 0 || !strings.Contains(out, "hello ci") {
		t.Fatalf("CI fetch: exit %d\n%s", code, out)
	}
	os.RemoveAll(origin) // the second run is served from the cache
	if out, code := run(t, p, ce, "", "sh", "scripts/ci/clauductor-model.sh", "hello.sh", "again"); code != 0 || !strings.Contains(out, "hello again") {
		t.Fatalf("cached run: exit %d\n%s", code, out)
	}
	bad := append(append([]string{}, ce...), "CLAUDUCTOR_REF=v0-no-such-tag")
	if out, code := run(t, p, bad, "", "sh", "scripts/ci/clauductor-model.sh", "hello.sh"); code == 0 || !strings.Contains(out, "FAIL") {
		t.Fatalf("a ref that cannot be cloned must fail (exit %d)\n%s", code, out)
	}
}

// The scaffold's panel.json runs the model's scripts through the resolver; it must still be a
// config the panel accepts.
func TestScaffoldPanelConfigLoads(t *testing.T) {
	out, _ := build(t)
	raw, err := os.ReadFile(filepath.Join(out, ScaffoldDir, ".clauductor", "panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.ParseConfig(raw); err != nil {
		t.Fatalf("scaffold panel.json is not a valid panel config: %v", err)
	}
	if strings.Contains(string(raw), `"sh", ".claude/`) {
		t.Errorf("scaffold panel.json still runs a framework script from the project's .claude/")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// build writes a fresh plugin into a temp dir and returns its root.
func build(t *testing.T) (string, *Result) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "plugin")
	res, err := Build(Options{RepoRoot: repoRoot(t), OutDir: out, Version: "9.9.9-test"})
	if err != nil {
		t.Fatal(err)
	}
	return out, res
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// The plugin carries every framework file of the template, at its plugin path, and nothing the
// project owns outside scaffold/.
func TestBuildPlacesEveryTemplateFile(t *testing.T) {
	out, _ := build(t)
	tmpl := filepath.Join(repoRoot(t), "template")
	files, err := listFiles(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range files {
		dest, kind, err := place(rel)
		if err != nil {
			t.Fatal(err)
		}
		switch kind {
		case kindPlugin:
			if !exists(filepath.Join(out, dest)) {
				t.Errorf("framework file %s is not in the plugin at %s", rel, dest)
			}
		case kindScaffold:
			if !exists(filepath.Join(out, ScaffoldDir, rel)) {
				t.Errorf("project file %s is not in scaffold/", rel)
			}
		}
	}
	// Components Claude Code loads: every skill (but the project's own), every agent.
	skills, _ := filepath.Glob(filepath.Join(tmpl, ".claude", "skills", "*", "SKILL.md"))
	for _, s := range skills {
		name := filepath.Base(filepath.Dir(s))
		inPlugin := exists(filepath.Join(out, "skills", name, "SKILL.md"))
		if projectSkills[name] == inPlugin {
			t.Errorf("skill %s: in the plugin = %v, want %v (project skills are scaffolded)", name, inPlugin, !projectSkills[name])
		}
	}
	agents, _ := agentNames(tmpl)
	for _, a := range agents {
		if !exists(filepath.Join(out, "agents", a+".md")) {
			t.Errorf("agent %s missing from the plugin", a)
		}
	}
	if !exists(filepath.Join(out, "skills", "init", "SKILL.md")) {
		t.Error("the plugin has no init skill")
	}

	// Nothing project-owned outside scaffold/.
	for _, owned := range []string{"AGENTS.md", "CLAUDE.md", "project.conf", "model-roles.json", "settings.json", "panel.json", "steps.sh", "roadmap.md", "development-journal.md", ".gitignore"} {
		filepath.WalkDir(out, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(out, p)
			if d.IsDir() && rel == ScaffoldDir {
				return filepath.SkipDir
			}
			if !d.IsDir() && d.Name() == owned {
				t.Errorf("project-owned %s is in the plugin outside scaffold/: %s", owned, rel)
			}
			return nil
		})
	}
	for _, dir := range []string{"docs", "changes", "specs", ".clauductor"} {
		if exists(filepath.Join(out, dir)) {
			t.Errorf("project directory %s/ is in the plugin outside scaffold/", dir)
		}
	}
}

func TestManifestAndMarketplace(t *testing.T) {
	out, _ := build(t)
	var m map[string]any
	raw, err := os.ReadFile(filepath.Join(out, ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["name"] != Name || m["version"] != "9.9.9-test" {
		t.Errorf("plugin.json name/version = %v/%v", m["name"], m["version"])
	}
	for _, k := range []string{"description", "author", "license"} {
		if m[k] == nil {
			t.Errorf("plugin.json has no %s", k)
		}
	}
	var mk struct {
		Name    string
		Owner   map[string]any
		Plugins []map[string]any
	}
	raw, _ = MarketplaceJSON()
	if err := json.Unmarshal(raw, &mk); err != nil {
		t.Fatal(err)
	}
	if mk.Name != Marketplace || mk.Owner["name"] == nil || len(mk.Plugins) != 1 ||
		mk.Plugins[0]["name"] != Name || mk.Plugins[0]["source"] != "./"+PluginDir || mk.Plugins[0]["version"] != nil {
		t.Errorf("marketplace.json: %s", raw)
	}
}

// Every template hook is registered through the plugin root (or deliberately dropped), and none
// still names the project's .claude/hooks.
func TestHooksRewritten(t *testing.T) {
	out, _ := build(t)
	raw, err := os.ReadFile(filepath.Join(out, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	hooks := string(raw)
	if strings.Contains(hooks, "CLAUDE_PROJECT_DIR") || strings.Contains(hooks, ".claude/hooks") {
		t.Errorf("hooks.json still names the project's hooks:\n%s", hooks)
	}
	settings, _ := os.ReadFile(filepath.Join(repoRoot(t), "template", ".claude", "settings.json"))
	for _, m := range regexp.MustCompile(`\.claude/hooks/([a-z0-9-]+\.sh)`).FindAllStringSubmatch(string(settings), -1) {
		want := `if-project.sh\" ` + m[1]
		_, dropped := droppedHooks[m[1]]
		if dropped == strings.Contains(hooks, want) {
			t.Errorf("hook %s: registered = %v, dropped = %v", m[1], !dropped, dropped)
		}
		if !exists(filepath.Join(out, "hooks", m[1])) {
			t.Errorf("hook script %s missing", m[1])
		}
	}
	if !strings.Contains(hooks, "plugin-session.sh") {
		t.Error("no SessionStart hook")
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
}

// fwPath matches a framework path still spelled the project's way in code.
var fwLeft = regexp.MustCompile(`(?:"\$ROOT"?/|\bsh |^|[\s(])\.claude/(?:skills/|agents|hooks|checks|lib|modules|workflows|[a-z-]+\.sh)`)

// No framework script, skill or agent still reaches for the model through the project's .claude/,
// except in the scratch repositories the checks build (their own .claude/ layout, by design), a
// diff-path pattern, and the orphan shapes, which describe paths inside lane worktrees.
func TestNoProjectPathsLeft(t *testing.T) {
	out, _ := build(t)
	allowed := regexp.MustCompile(`cd "\$[A-Za-z]+" &&|note "|echo "|fail "|ok "|^has \.claude/|for f in changes/README\.md|\$(R|F|A|B|WT|d)\b[^ ]*/\.claude/|"\$[RFAB]/|\.claude/\* \||MQ_ORPHAN_SHAPES|@WT@|\$WT/lane/\.claude`)
	filepath.WalkDir(out, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(out, p)
		if d.IsDir() {
			if rel == ScaffoldDir {
				return filepath.SkipDir
			}
			return nil
		}
		if _, err := assets.Open("assets/" + filepath.ToSlash(rel)); err == nil {
			return nil // the packaging layer's own files name the project's .claude/ on purpose
		}
		isSh := strings.HasSuffix(p, ".sh")
		isComp := strings.HasSuffix(p, ".md") && (strings.HasPrefix(rel, "skills/") || strings.HasPrefix(rel, "agents/") || isModuleFragment(filepath.ToSlash(rel)))
		if !isSh && !isComp {
			return nil
		}
		data, _ := os.ReadFile(p)
		body := string(data)
		if isComp {
			_, body = splitFrontmatter(body)
		}
		for i, l := range strings.Split(body, "\n") {
			if isSh && strings.HasPrefix(strings.TrimSpace(l), "#") {
				continue
			}
			// A line naming both is a deliberate fallback to the project's own (its skills).
			if fwLeft.MatchString(l) && !allowed.MatchString(l) && !strings.Contains(l, "$"+FWVar) {
				t.Errorf("%s:%d still names a framework path in the project: %s", rel, i+1, strings.TrimSpace(l))
			}
		}
		return nil
	})
}

// Every script path a plugin skill or agent names exists in the plugin, and a skill's context
// line runs from the plugin root.
func TestComponentPathsExist(t *testing.T) {
	out, _ := build(t)
	ref := regexp.MustCompile(`(?:\$\{CLAUDE_PLUGIN_ROOT\}/|clauductor-model )([A-Za-z0-9_./*-]+\.(?:sh|js|awk|json|md))`)
	ctx := regexp.MustCompile("!`sh ([^`]+)`")
	files, _ := filepath.Glob(filepath.Join(out, "skills", "*", "SKILL.md"))
	more, _ := filepath.Glob(filepath.Join(out, "agents", "*.md"))
	// A module's skill fragments are appended to a plugin skill at load: held the same way.
	frags, _ := filepath.Glob(filepath.Join(out, "modules", "*", "skills", "*", "*.md"))
	if len(frags) == 0 {
		t.Error("the plugin ships no module skill fragments (modules/<name>/skills/<skill>/*.md)")
	}
	more = append(more, frags...)
	// The artifacts module's fragments name its tools the way a plugin project runs them.
	frag, err := os.ReadFile(filepath.Join(out, "modules", "artifacts", "skills", "session-close", "artifacts.md"))
	if err != nil {
		t.Errorf("the plugin does not ship the artifacts module's session-close fragment: %v", err)
	} else if !strings.Contains(string(frag), "clauductor-model modules/artifacts/bin/currency.sh --worktree") || strings.Contains(string(frag), "sh .claude/") {
		t.Errorf("the artifacts module's session-close fragment is not rewritten for the plugin:\n%s", frag)
	}
	for _, f := range append(files, more...) {
		data, _ := os.ReadFile(f)
		rel, _ := filepath.Rel(out, f)
		for _, m := range ref.FindAllStringSubmatch(string(data), -1) {
			if strings.Contains(m[1], "*") {
				continue
			}
			if !exists(filepath.Join(out, m[1])) {
				t.Errorf("%s names %s, which the plugin does not have", rel, m[1])
			}
		}
		for _, m := range ctx.FindAllStringSubmatch(string(data), -1) {
			if !strings.HasPrefix(m[1], "${CLAUDE_PLUGIN_ROOT}/") {
				t.Errorf("%s: context line %q does not run from the plugin root", rel, m[1])
			}
		}
	}
}

func TestSettingsScaffold(t *testing.T) {
	out, _ := build(t)
	raw, err := os.ReadFile(filepath.Join(out, ScaffoldDir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("scaffold settings.json: %v\n%s", err, raw)
	}
	if s["hooks"] != nil {
		t.Error("scaffold settings.json registers hooks; the plugin does")
	}
	if en, _ := s["enabledPlugins"].(map[string]any); en[Name+"@"+Marketplace] != true {
		t.Error("scaffold settings.json does not enable the plugin")
	}
	if !strings.Contains(string(raw), "clauductor-model checks/run.sh") || strings.Contains(string(raw), "sh .claude/") {
		t.Errorf("allow rules not rewritten:\n%s", raw)
	}
}

// --- The scaffold and the checks, run for real ------------------------------------------------

func need(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
}

// env is a clean environment: no Claude Code variables from the session running the tests, no
// user git config, and a HOME of our own.
func env(t *testing.T, home string) []string {
	var e []string
	for _, kv := range os.Environ() {
		k := strings.SplitN(kv, "=", 2)[0]
		if strings.HasPrefix(k, "CLAUDE") || strings.HasPrefix(k, "GIT_") || k == "HOME" || k == "CI" || k == "ROOT" || k == "XDG_CACHE_HOME" || strings.HasPrefix(k, "CLAUDUCTOR") {
			continue
		}
		e = append(e, kv)
	}
	// A developer's global config usually names main as the default branch; the checks' scratch
	// origins rely on it (a bare `git init` otherwise points HEAD at master).
	os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[init]\n\tdefaultBranch = main\n"), 0o644)
	return append(e, "HOME="+home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(home, ".gitconfig"),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
}

func run(t *testing.T, dir string, e []string, stdin string, name string, args ...string) (string, int) {
	t.Helper()
	c := exec.Command(name, args...)
	c.Dir, c.Env = dir, e
	if stdin != "" {
		c.Stdin = strings.NewReader(stdin)
	}
	out, err := c.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return string(out), code
}

func newProject(t *testing.T, e []string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "proj")
	os.MkdirAll(p, 0o755)
	if out, code := run(t, p, e, "", "git", "init", "-q"); code != 0 {
		t.Fatal(out)
	}
	run(t, p, e, "", "git", "checkout", "-q", "-b", "main")
	return p
}

func TestScaffoldGuards(t *testing.T) {
	need(t, "git", "sh")
	plug, _ := build(t)
	home := t.TempDir()
	e := env(t, home)
	scaffold := filepath.Join(plug, "scaffold.sh")

	// A repository with a model of its own is refused, and --force adds without overwriting.
	p := newProject(t, e)
	os.WriteFile(filepath.Join(p, "AGENTS.md"), []byte("ours\n"), 0o644)
	if out, code := run(t, p, e, "", "sh", scaffold); code != 1 || !strings.Contains(out, "REFUSED") {
		t.Fatalf("own model: exit %d, want a refusal\n%s", code, out)
	}
	if exists(filepath.Join(p, ".claude", "project.conf")) {
		t.Fatal("a refused scaffold wrote files")
	}
	if out, code := run(t, p, e, "", "sh", scaffold, "--force"); code != 0 {
		t.Fatalf("--force: exit %d\n%s", code, out)
	}
	if b, _ := os.ReadFile(filepath.Join(p, "AGENTS.md")); string(b) != "ours\n" {
		t.Fatalf("--force overwrote AGENTS.md: %q", b)
	}
	if !exists(filepath.Join(p, ".claude", "project.conf")) || !exists(filepath.Join(p, ".claude", "clauductor-plugin")) {
		t.Fatal("--force did not scaffold and mark")
	}

	// `clauductor install` was here: refused even with --force.
	q := newProject(t, e)
	os.MkdirAll(filepath.Join(q, ".claude"), 0o755)
	os.WriteFile(filepath.Join(q, ".claude", "clauductor-template"), []byte("x\n"), 0o644)
	if out, code := run(t, q, e, "", "sh", scaffold, "--force"); code != 1 || !strings.Contains(out, "clauductor install") {
		t.Fatalf("installed repo: exit %d, want a refusal naming clauductor install\n%s", code, out)
	}
}

// The plugin's hooks stand aside in a repository the plugin has not set up, and act in one it has.
func TestHooksOnlyActInPluginProjects(t *testing.T) {
	need(t, "git", "sh", "jq")
	plug, _ := build(t)
	home := t.TempDir()
	e := env(t, home)
	p := newProject(t, e)
	os.WriteFile(filepath.Join(p, "app.ts"), []byte("x\n"), 0o644)
	run(t, p, e, "", "git", "add", "app.ts")
	run(t, p, e, "", "git", "commit", "-qm", "seed")
	payload := `{"tool_name":"Bash","tool_input":{"command":"sed -i '' 's/x/y/' app.ts"},"cwd":"` + p + `"}`
	hook := filepath.Join(plug, "hooks", "if-project.sh")
	pe := append(e, "CLAUDE_PROJECT_DIR="+p)

	if out, code := run(t, p, pe, payload, "sh", hook, "no-blind-source-rewrite.sh"); code != 0 {
		t.Fatalf("unmarked repo: exit %d, want 0 (stand aside)\n%s", code, out)
	}
	if out, code := run(t, p, e, "", "sh", filepath.Join(plug, "scaffold.sh")); code != 0 {
		t.Fatal(out)
	}
	if out, code := run(t, p, pe, payload, "sh", hook, "no-blind-source-rewrite.sh"); code != 2 {
		t.Fatalf("plugin project: exit %d, want 2 (block)\n%s", code, out)
	}
	os.WriteFile(filepath.Join(p, ".claude", "clauductor-template"), []byte("x\n"), 0o644)
	if out, code := run(t, p, pe, payload, "sh", hook, "no-blind-source-rewrite.sh"); code != 0 {
		t.Fatalf("installed-model repo: exit %d, want 0 (its own hooks run)\n%s", code, out)
	}
}

// The strongest check: scaffold a repository from the plugin, then run the template's own process
// checks FROM THE PLUGIN against it. They are the model's executable description of itself, so a
// rewrite that broke a path breaks one of them.
func TestPluginChecksPassInScaffoldedProject(t *testing.T) {
	if testing.Short() {
		t.Skip("runs every process check (about 20 s)")
	}
	need(t, "git", "sh", "jq", "awk")
	plug, _ := build(t)
	home := t.TempDir()
	e := env(t, home)
	p := newProject(t, e)
	if out, code := run(t, p, e, "", "sh", filepath.Join(plug, "scaffold.sh")); code != 0 {
		t.Fatalf("scaffold: exit %d\n%s", code, out)
	}
	// Idempotent: a second run creates nothing.
	if out, _ := run(t, p, e, "", "sh", filepath.Join(plug, "scaffold.sh"), "--dry-run"); strings.Contains(out, "create:") {
		t.Errorf("second scaffold would create files:\n%s", out)
	}
	run(t, p, e, "", "git", "add", "-A")
	if out, code := run(t, p, e, "", "git", "commit", "-qm", "scaffold"); code != 0 {
		t.Fatal(out)
	}
	out, code := run(t, p, e, "", "sh", filepath.Join(plug, "checks", "run.sh"))
	if code != 0 {
		t.Fatalf("the plugin's process checks fail in a scaffolded project (exit %d):\n%s", code, out)
	}
	// The gate's step reaches the same checks through the resolver.
	ge := append(e, "CLAUDUCTOR_PLUGIN_ROOT="+plug)
	if out, code := run(t, p, ge, "", "sh", "scripts/ci/clauductor-model.sh", "checks/run.sh", "roadmap"); code != 0 {
		t.Fatalf("scripts/ci/clauductor-model.sh: exit %d\n%s", code, out)
	}
	if out, code := run(t, p, e, "", "sh", "scripts/ci/clauductor-model.sh", "checks/run.sh"); code == 0 || !strings.Contains(out, "FAIL") {
		t.Fatalf("with no plugin to find, the resolver must fail, not skip (exit %d)\n%s", code, out)
	}
}

// `claude plugin validate` is the authoritative manifest check. CI runners have no claude CLI.
func TestClaudePluginValidate(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("the claude CLI is not installed (CI runners): claude plugin validate not run")
	}
	plug, _ := build(t)
	out, err := exec.Command("claude", "plugin", "validate", "--strict", plug).CombinedOutput()
	if err != nil {
		t.Fatalf("claude plugin validate --strict: %v\n%s", err, out)
	}
	root := repoRoot(t)
	if exists(filepath.Join(root, ".claude-plugin", "marketplace.json")) {
		out, err := exec.Command("claude", "plugin", "validate", "--strict", root).CombinedOutput()
		if err != nil {
			t.Fatalf("claude plugin validate --strict (marketplace): %v\n%s", err, out)
		}
	}
}

// Installing through Claude Code's own plugin commands, from the committed marketplace, into a
// throwaway config (HOME, CLAUDE_CONFIG_DIR and the plugin cache all temporary: the user's real
// ~/.claude is never touched). Opt-in with CLAUDUCTOR_PLUGIN_INSTALL_TEST=1: it runs the claude
// CLI, which CI runners do not have.
func TestClaudePluginInstall(t *testing.T) {
	if os.Getenv("CLAUDUCTOR_PLUGIN_INSTALL_TEST") != "1" {
		t.Skip("set CLAUDUCTOR_PLUGIN_INSTALL_TEST=1 to install the plugin with the claude CLI into a temporary config")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("the claude CLI is not installed")
	}
	home := t.TempDir()
	cfg := filepath.Join(home, ".claude")
	os.MkdirAll(cfg, 0o755)
	e := append(env(t, home), "CLAUDE_CONFIG_DIR="+cfg, "CLAUDE_CODE_PLUGIN_CACHE_DIR="+filepath.Join(cfg, "plugins"))
	claude := func(args ...string) string {
		out, code := run(t, home, e, "", "claude", args...)
		t.Logf("claude %s (exit %d):\n%s", strings.Join(args, " "), code, out)
		if code != 0 {
			t.Fatalf("claude %v failed", args)
		}
		return out
	}
	claude("plugin", "marketplace", "add", repoRoot(t))
	claude("plugin", "install", Name+"@"+Marketplace)
	if list := claude("plugin", "list"); !strings.Contains(list, Name+"@"+Marketplace) {
		t.Errorf("plugin list does not show %s@%s", Name, Marketplace)
	}
	details := claude("plugin", "details", Name)
	// `details` lists skills, agents and hook events; it does not list workflows.
	for _, want := range []string{"session-start", "init", "start-project", "builder", "reviewer-docs", "PreToolUse", "SessionStart"} {
		if !strings.Contains(details, want) {
			t.Errorf("plugin details lacks %s", want)
		}
	}
}

// /clauductor:init merges an existing settings.json the way `clauductor install` does: the
// project's keys and entries kept, the model's deny list and sandbox entries unioned in, the
// plugin's status line set, and the gate's paths from the project's GATE_RUN and GATE.
func TestScaffoldMergesSettings(t *testing.T) {
	need(t, "git", "sh", "jq")
	plug, _ := build(t)
	home := t.TempDir()
	e := env(t, home)
	p := newProject(t, e)
	os.MkdirAll(filepath.Join(p, ".claude"), 0o755)
	os.WriteFile(filepath.Join(p, ".claude", "project.conf"), []byte("GATE_RUN=\"tools/ci/run-local.sh\"\nGATE=\"tools/ci/gate.sh\"\n"), 0o644)
	os.WriteFile(filepath.Join(p, ".claude", "settings.json"), []byte(`{"model": "sonnet", "env": {"X": "1"},
  "statusLine": {"type": "command", "command": "sh .claude/statusline.sh"},
  "permissions": {"allow": ["Bash(npm test)"], "deny": ["Read(./secrets/**)"]},
  "sandbox": {"excludedCommands": ["make *"]}}`), 0o644)
	if out, code := run(t, p, e, "", "sh", filepath.Join(plug, "scaffold.sh"), "--force"); code != 0 {
		t.Fatalf("scaffold: exit %d\n%s", code, out)
	}
	raw, _ := os.ReadFile(filepath.Join(p, ".claude", "settings.json"))
	var s struct {
		Model       string                         `json:"model"`
		Env         map[string]string              `json:"env"`
		StatusLine  struct{ Command string }       `json:"statusLine"`
		Permissions struct{ Allow, Deny []string } `json:"permissions"`
		Sandbox     struct {
			Enabled          bool     `json:"enabled"`
			ExcludedCommands []string `json:"excludedCommands"`
		} `json:"sandbox"`
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("merged settings.json: %v\n%s", err, raw)
	}
	has := func(xs []string, x string) bool {
		for _, v := range xs {
			if v == x {
				return true
			}
		}
		return false
	}
	if s.Model != "sonnet" || s.Env["X"] != "1" {
		t.Errorf("project keys not kept: %s", raw)
	}
	if s.StatusLine.Command != StatusLineCommand {
		t.Errorf("statusLine is the project's old one, not the plugin's: %q", s.StatusLine.Command)
	}
	if !has(s.Permissions.Allow, "Bash(npm test)") || !has(s.Permissions.Deny, "Read(./secrets/**)") || !has(s.Permissions.Deny, "Read(./.env)") {
		t.Errorf("permissions not unioned: %+v", s.Permissions)
	}
	if !s.Sandbox.Enabled || !has(s.Sandbox.ExcludedCommands, "make *") || !has(s.Sandbox.ExcludedCommands, "tools/ci/gate.sh *") || has(s.Sandbox.ExcludedCommands, "scripts/ci/gate.sh *") {
		t.Errorf("sandbox not merged or gate paths not rendered: %+v", s.Sandbox)
	}
	if !has(s.Permissions.Allow, "Bash(tools/ci/gate.sh)") || has(s.Permissions.Allow, "Bash(scripts/ci/gate.sh)") {
		t.Errorf("allow rules do not follow GATE: %v", s.Permissions.Allow)
	}
	if !s.EnabledPlugins[Name+"@"+Marketplace] {
		t.Error("the plugin is not enabled")
	}
}

// OPS-14 on the plugin path: /clauductor:init in a repository running clauductor's old model is not
// refused; the old model's files are listed and removed only with --prune; its hook registrations
// (and any whose script is missing) leave settings.json; and the panel preset follows the
// project's branch keys.
func TestScaffoldOverTheOldModel(t *testing.T) {
	need(t, "git", "sh", "jq")
	plug, _ := build(t)
	e := env(t, t.TempDir())
	p := newProject(t, e)
	for _, s := range []string{"claim", "spawn", "supervisor"} {
		os.MkdirAll(filepath.Join(p, ".claude", "skills", s), 0o755)
		os.WriteFile(filepath.Join(p, ".claude", "skills", s, "SKILL.md"), []byte("old "+s+"\n"), 0o644)
	}
	os.MkdirAll(filepath.Join(p, ".claude", "skills", "ours"), 0o755)
	os.WriteFile(filepath.Join(p, ".claude", "skills", "ours", "SKILL.md"), []byte("ours\n"), 0o644)
	os.MkdirAll(filepath.Join(p, ".claude", "hooks"), 0o755)
	os.WriteFile(filepath.Join(p, ".claude", "hooks", "heartbeat.sh"), []byte("#!/bin/sh\nclauductor heartbeat\n"), 0o644)
	os.WriteFile(filepath.Join(p, ".claude", "hooks", "mine.sh"), []byte("#!/bin/sh\n"), 0o644)
	os.WriteFile(filepath.Join(p, ".claude", "settings.json"), []byte(`{"hooks": {
  "SessionStart": [{"hooks": [{"type": "command", "command": "bash .claude/hooks/session-register.sh"}]}],
  "PostToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "bash .claude/hooks/heartbeat.sh"}, {"type": "command", "command": "sh .claude/hooks/mine.sh"}]}]}}`), 0o644)
	os.WriteFile(filepath.Join(p, ".claude", "project.conf"), []byte("BRANCH_CHANGE=\"feature/\"\n"), 0o644)

	out, code := run(t, p, e, "", "sh", filepath.Join(plug, "scaffold.sh"))
	if code != 0 || !strings.Contains(out, "OLD MODEL") || !strings.Contains(out, ".claude/skills/claim/SKILL.md") {
		t.Fatalf("old-model repo: exit %d, want a scaffold listing the old files\n%s", code, out)
	}
	if !exists(filepath.Join(p, ".claude", "skills", "claim", "SKILL.md")) {
		t.Fatal("old files were removed without --prune")
	}
	raw, _ := os.ReadFile(filepath.Join(p, ".claude", "settings.json"))
	if strings.Contains(string(raw), "session-register.sh") || strings.Contains(string(raw), "heartbeat.sh") || !strings.Contains(string(raw), "mine.sh") {
		t.Errorf("settings.json hooks not pruned right:\n%s", raw)
	}
	panel, _ := os.ReadFile(filepath.Join(p, ".clauductor", "panel.json"))
	if !strings.Contains(string(panel), `"feature/"`) || strings.Contains(string(panel), `"change/`) {
		t.Errorf("panel.json does not follow BRANCH_CHANGE=feature/:\n%s", panel)
	}
	if out, code := run(t, p, e, "", "sh", filepath.Join(plug, "scaffold.sh"), "--prune"); code != 0 {
		t.Fatalf("--prune: exit %d\n%s", code, out)
	}
	for _, gone := range []string{"skills/claim", "skills/spawn", "skills/supervisor", "hooks/heartbeat.sh"} {
		if exists(filepath.Join(p, ".claude", gone)) {
			t.Errorf("--prune left .claude/%s", gone)
		}
	}
	if !exists(filepath.Join(p, ".claude", "skills", "ours", "SKILL.md")) || !exists(filepath.Join(p, ".claude", "hooks", "mine.sh")) {
		t.Error("--prune removed a project file")
	}
}
