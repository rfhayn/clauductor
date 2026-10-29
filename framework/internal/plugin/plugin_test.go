package plugin

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
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
// config the panel accepts, at the latest config version, as the template's ([STALL-1-S29]).
func TestScaffoldPanelConfigLoads(t *testing.T) {
	out, _ := build(t)
	raw, err := os.ReadFile(filepath.Join(out, ScaffoldDir, ".clauductor", "panel.json"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.ParseConfig(raw)
	if err != nil {
		t.Fatalf("scaffold panel.json is not a valid panel config: %v", err)
	}
	if c.Version != config.LatestVersion { // the template's, PANEL-51 D5 (#189)
		t.Errorf("scaffold panel.json declares version %d, want the latest, %d", c.Version, config.LatestVersion)
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
	for _, owned := range []string{"AGENTS.md", "CLAUDE.md", "project.conf", "model-roles.json", "settings.json", "panel.json", "steps.sh", "roadmap.md", "development-journal.md", ".gitignore", ".gitattributes"} {
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

// A script a check writes into its scratch project runs there: its project is the directory above
// it and its model is the copy the check put there, so the builder must not point it at the
// plugin's model (CLAUDUCTOR_FW) when the template did not. OPS-169: the builder rewrote a fixture
// caller's ROOT and sources to CLAUDUCTOR_FW, and 34 cases read the real project. A check writes a
// script as a quoted heredoc's body or a printf format whose output reaches a file; writtenScripts
// (shscan_test.go) reads both as the shell does, and reports what it cannot read rather than skip
// it. Unquoted heredocs are not read, by design. Other rewrites stay allowed: a contract script expects the plugin's agent
// names, as the plugin's workflow spells them, and roadmap.sh's plant3 gains CLAUDUCTOR_FW
// harmlessly, since the bypass reader it feeds (roadmap.sh's strip) cuts both spellings.
func TestCheckHeredocsKeepTheirProject(t *testing.T) {
	out, _ := build(t)
	files, err := filepath.Glob(filepath.Join(repoRoot(t), "template", ".claude", "checks", "*.sh"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no template checks found (%v)", err)
	}
	var docs, prints int
	// Named writers the reader must find, so a reader that loses a form fails here, not silently.
	// The two floor writers (health-lines.sh's workflows, line-endings.sh's .gitattributes) write
	// "$2": they are named for their form, which the reader must still find; what $2 holds is out of
	// its reach (shscan_test.go, text held in a variable).
	caller, fixture, own, workflows, attrs := false, false, false, false, false
	for _, f := range files {
		name := filepath.Base(f)
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		gen, err := os.ReadFile(filepath.Join(out, "checks", name))
		if err != nil {
			t.Errorf("%s: no plugin copy: %v", name, err)
			continue
		}
		want, werrs := writtenScripts(string(src))
		got, gerrs := writtenScripts(string(gen))
		for _, e := range werrs {
			t.Errorf("checks/%s: %s", name, e)
		}
		for _, e := range gerrs {
			t.Errorf("plugin checks/%s: %s", name, e)
		}
		if len(want) != len(got) {
			t.Errorf("checks/%s: %d written script(s) in the template, %d in the plugin's copy", name, len(want), len(got))
			continue
		}
		for i, w := range want {
			if w.heredoc != got[i].heredoc {
				t.Errorf("checks/%s: script %d is a heredoc in one copy and a printf in the other: the pairing is off", name, i+1)
				break
			}
			if w.heredoc {
				docs++
				caller = caller || strings.Contains(w.text, "health_coauthor")
				fixture = fixture || (name == "mktemp-in-tmpdir.sh" && strings.HasPrefix(strings.TrimSpace(w.open), "fixture "))
			} else {
				prints++
				own = own || (name == "artifacts.sh" && strings.Contains(w.open, `> "$K/own.sh"`))
				workflows = workflows || (name == "health-lines.sh" && strings.Contains(w.open, `> "$F/.github/workflows/$1"`))
				attrs = attrs || (name == "line-endings.sh" && strings.Contains(w.open, `> "$r/.gitattributes"`))
				if name == "roadmap.sh" && strings.Contains(w.open, "plant3") {
					continue
				}
			}
			if !strings.Contains(w.text, FWVar) && strings.Contains(got[i].text, FWVar) {
				t.Errorf("checks/%s: the plugin's copy points a script the check writes into its scratch project at %s:\n%s\n%s", name, FWVar, got[i].open, got[i].text)
			}
		}
	}
	if docs == 0 || prints == 0 || !caller || !fixture || !own || !workflows || !attrs {
		t.Fatalf("read %d heredoc(s) and %d printf/echo(s); among them the coauthor caller: %v, mktemp-in-tmpdir's fixture: %v, artifacts.sh's own.sh: %v, health-lines.sh's workflow writer: %v, line-endings.sh's .gitattributes writer: %v. The reader no longer finds what it guards", docs, prints, caller, fixture, own, workflows, attrs)
	}
}

// writtenScripts, the reader the test above uses, is in shscan_test.go.

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

// OPS-20: init merges the line-ending rules into a project's own .gitattributes AHEAD of its lines
// (the last matching line wins, so the project's overrides keep working), and only once.
func TestScaffoldMergesGitattributes(t *testing.T) {
	need(t, "git", "sh")
	plug, _ := build(t)
	e := env(t, t.TempDir())
	p := newProject(t, e)
	attrs := filepath.Join(p, ".gitattributes")
	os.WriteFile(attrs, []byte("*.bat text eol=crlf\n"), 0o644)
	if out, code := run(t, p, e, "", "sh", filepath.Join(plug, "scaffold.sh")); code != 0 {
		t.Fatalf("scaffold: exit %d\n%s", code, out)
	}
	got, _ := os.ReadFile(attrs)
	rule, mine := strings.Index(string(got), "* text=auto eol=lf"), strings.Index(string(got), "*.bat text eol=crlf")
	if rule < 0 || mine < 0 || rule > mine {
		t.Fatalf("the LF rule must come before the project's own line:\n%s", got)
	}
	want, _ := os.ReadFile(filepath.Join(plug, ScaffoldDir, ".gitattributes"))
	if merged, _ := template.MergeGitattributes("*.bat text eol=crlf\n", string(want)); string(got) != merged {
		t.Errorf("init and `clauductor install` merge differently.\ninit:\n%s\ninstall:\n%s", got, merged)
	}
	if out, code := run(t, p, e, "", "git", "check-attr", "eol", "--", "x.sh", "y.bat"); code != 0 ||
		!strings.Contains(out, "x.sh: eol: lf") || !strings.Contains(out, "y.bat: eol: crlf") {
		t.Errorf("git reads the merged rules wrong (exit %d):\n%s", code, out)
	}
	run(t, p, e, "", "sh", filepath.Join(plug, "scaffold.sh"))
	if again, _ := os.ReadFile(attrs); string(again) != string(got) {
		t.Errorf("a second init changed .gitattributes:\n%s", again)
	}

	// A CRLF file with trailing spaces that already has every rule: nothing is added twice.
	q := newProject(t, e)
	var crlf strings.Builder
	for _, l := range strings.Split(strings.TrimSpace(string(want)), "\n") {
		crlf.WriteString(l + "  \r\n")
	}
	qa := filepath.Join(q, ".gitattributes")
	os.WriteFile(qa, []byte(crlf.String()), 0o644)
	if out, code := run(t, q, e, "", "sh", filepath.Join(plug, "scaffold.sh")); code != 0 {
		t.Fatalf("scaffold: exit %d\n%s", code, out)
	}
	if b, _ := os.ReadFile(qa); string(b) != crlf.String() {
		t.Errorf("init re-added rules a CRLF, trailing-space file already has:\n%q", b)
	}
}

// /clauductor:init ends with the prerequisites report, and treats .gitleaks.toml as the
// project's: created when missing, kept when the project has its own.
func TestScaffoldReportsPrereqsAndKeepsGitleaksConfig(t *testing.T) {
	need(t, "git", "sh")
	plug, _ := build(t)
	e := env(t, t.TempDir())
	scaffold := filepath.Join(plug, "scaffold.sh")

	p := newProject(t, e)
	out, code := run(t, p, e, "", "sh", scaffold)
	if code != 0 {
		t.Fatalf("scaffold: exit %d\n%s", code, out)
	}
	if !strings.Contains(out, "Prerequisites on ") {
		t.Errorf("scaffold printed no prerequisites report:\n%s", out)
	}
	if !exists(filepath.Join(p, ".gitleaks.toml")) {
		t.Error("scaffold did not create the starter .gitleaks.toml")
	}

	q := newProject(t, e)
	own := "[extend]\nuseDefault = true\n# ours\n"
	os.WriteFile(filepath.Join(q, ".gitleaks.toml"), []byte(own), 0o644)
	if out, code := run(t, q, e, "", "sh", scaffold); code != 0 {
		t.Fatalf("scaffold: exit %d\n%s", code, out)
	}
	if b, _ := os.ReadFile(filepath.Join(q, ".gitleaks.toml")); string(b) != own {
		t.Errorf("scaffold wrote over the project's .gitleaks.toml: %q", b)
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
	// The ideas module's enable.sh, run from the plugin, installs /ideas into the project with the
	// tool spelt the plugin's way (a plugin project has no .claude/modules), and its own --check
	// accepts that copy. Last, because the installed skill is one the scaffold's roles do not map.
	en := filepath.Join(plug, "modules", "ideas", "enable.sh")
	if out, code := run(t, p, e, "", "sh", en); code != 0 {
		t.Fatalf("the plugin's ideas enable.sh: exit %d\n%s", code, out)
	}
	sk, err := os.ReadFile(filepath.Join(p, ".claude", "skills", "ideas", "SKILL.md"))
	if err != nil || !strings.Contains(string(sk), "clauductor-model modules/ideas/bin/render.sh --url") || strings.Contains(string(sk), "sh .claude/modules/") {
		t.Errorf("the plugin's ideas enable.sh did not install /ideas with the plugin's command (%v):\n%s", err, sk)
	}
	if out, code := run(t, p, e, "", "sh", en, "--check"); code != 0 {
		t.Errorf("the plugin's ideas enable.sh --check refuses the copy it installed (exit %d)\n%s", code, out)
	}
}

// realHome is the user's home as the test binary started, before any test overrides HOME.
var realHome, _ = os.UserHomeDir()

// Where Claude Code keeps state. The first three claudeConfig points into the test's home; the
// rest it unsets, so Claude Code falls back to paths under that HOME (#95).
var (
	claudeStateRequired = []string{"HOME", "CLAUDE_CONFIG_DIR", "CLAUDE_CODE_PLUGIN_CACHE_DIR"}
	claudeStateOptional = []string{"ANTHROPIC_CONFIG_DIR", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR", "XDG_CACHE_HOME"}
)

// claudeConfig is a throwaway HOME and Claude Code config for a test that runs the real claude
// CLI, so the user's real ~/.claude is never read or written (#95: validate once ran against it).
// HOME is a t.TempDir(), CLAUDE_CONFIG_DIR its .claude and the plugin cache .claude/plugins;
// ANTHROPIC_CONFIG_DIR and the XDG config, data, state, runtime and cache dirs are unset.
func claudeConfig(t *testing.T) (home string, e []string) {
	home = t.TempDir()
	cfg := filepath.Join(home, ".claude")
	os.MkdirAll(cfg, 0o755)
	for _, kv := range env(t, home) {
		if !slices.Contains(claudeStateOptional, strings.SplitN(kv, "=", 2)[0]) {
			e = append(e, kv)
		}
	}
	// The updater would replace the user's installed claude from inside a test run. A unit test
	// reaches no network either: without the last two, claude opens api.anthropic.com at startup
	// (#254), a denied connection in a sandboxed session.
	return home, append(e, "CLAUDE_CONFIG_DIR="+cfg, "CLAUDE_CODE_PLUGIN_CACHE_DIR="+filepath.Join(cfg, "plugins"), "DISABLE_AUTOUPDATER=1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "DISABLE_TELEMETRY=1")
}

// claudeEnvOutsideTemp says why running claude in home with env could reach state outside the
// test, or nil. home must be strictly inside the temporary directory and must not contain the
// real home (a TMPDIR above HOME); every required location must be set inside home, and every
// optional one unset (or empty) or inside home. The last value of a key is the one exec uses.
func claudeEnvOutsideTemp(home string, env []string) error {
	within := func(base, p string) bool {
		rel, err := filepath.Rel(base, p)
		return err == nil && filepath.IsAbs(p) && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	if !within(os.TempDir(), home) || filepath.Clean(home) == filepath.Clean(os.TempDir()) {
		return fmt.Errorf("claude would run in %q, not inside the temporary directory %s", home, os.TempDir())
	}
	if realHome != "" && within(home, realHome) {
		return fmt.Errorf("claude would run in %q, which contains the real home %s", home, realHome)
	}
	for _, k := range slices.Concat(claudeStateRequired, claudeStateOptional) {
		v, set := "", false
		for _, kv := range env {
			if strings.HasPrefix(kv, k+"=") {
				v, set = strings.TrimPrefix(kv, k+"="), true
			}
		}
		if !set || v == "" {
			if slices.Contains(claudeStateRequired, k) {
				return fmt.Errorf("claude would run without %s", k)
			}
			continue
		}
		if !within(home, v) {
			return fmt.Errorf("claude would run with %s=%q, outside the test's home %s", k, v, home)
		}
	}
	return nil
}

// runClaude runs the real claude CLI in home with e, and refuses to unless the environment the
// command receives keeps it inside the test (claudeEnvOutsideTemp): a regression back to the real
// HOME or ~/.claude fails here rather than running against it (#95).
func runClaude(t *testing.T, home string, e []string, args ...string) (string, int) {
	t.Helper()
	if err := claudeEnvOutsideTemp(home, e); err != nil {
		t.Fatalf("refusing to run claude: %v", err)
	}
	return run(t, home, e, "", "claude", args...)
}

// The guard in front of every real claude run refuses each way back to the user's own state (#95).
func TestClaudeEnvOutsideTemp(t *testing.T) {
	home, valid := claudeConfig(t)
	with := func(kv ...string) []string { return append(slices.Clone(valid), kv...) }
	cases := []struct {
		name string
		home string
		env  []string
		ok   bool
	}{
		{"valid", home, valid, true},
		{"inherited environment", home, os.Environ(), false},
		{"later duplicate wins: real HOME last", home, with("HOME=" + realHome), false},
		{"later duplicate wins: test HOME last", home, append([]string{"HOME=" + realHome}, valid...), true},
		{"relative path", home, with("CLAUDE_CONFIG_DIR=.claude"), false},
		{"dot-dot escape", home, with("CLAUDE_CONFIG_DIR=" + home + "/../escape"), false},
		{"TMPDIR itself as HOME", home, with("HOME=" + os.TempDir()), false},
		{"TMPDIR itself as the home", os.TempDir(), valid, false},
		{"home outside the temporary directory", "/var/empty/h", []string{"HOME=/var/empty/h", "CLAUDE_CONFIG_DIR=/var/empty/h/.claude", "CLAUDE_CODE_PLUGIN_CACHE_DIR=/var/empty/h/.claude/plugins"}, false},
		{"no CLAUDE_CONFIG_DIR", home, slices.DeleteFunc(slices.Clone(valid), func(kv string) bool { return strings.HasPrefix(kv, "CLAUDE_CONFIG_DIR=") }), false},
		{"ANTHROPIC_CONFIG_DIR real", home, with("ANTHROPIC_CONFIG_DIR=" + filepath.Join(realHome, ".claude")), false},
		{"XDG_CONFIG_HOME real", home, with("XDG_CONFIG_HOME=" + filepath.Join(realHome, ".config")), false},
		{"XDG_DATA_HOME real", home, with("XDG_DATA_HOME=" + filepath.Join(realHome, ".local", "share")), false},
		{"XDG_STATE_HOME real", home, with("XDG_STATE_HOME=" + filepath.Join(realHome, ".local", "state")), false},
		{"XDG_RUNTIME_DIR real", home, with("XDG_RUNTIME_DIR=/run/user/501"), false},
		{"XDG_CACHE_HOME real", home, with("XDG_CACHE_HOME=" + filepath.Join(realHome, ".cache")), false},
		{"XDG_STATE_HOME inside the test's home", home, with("XDG_STATE_HOME=" + filepath.Join(home, ".local", "state")), true},
		{"XDG_STATE_HOME empty", home, with("XDG_STATE_HOME="), true},
	}
	for _, c := range cases {
		if err := claudeEnvOutsideTemp(c.home, c.env); (err == nil) != c.ok {
			t.Errorf("%s: want ok=%v, got %v", c.name, c.ok, err)
		}
	}
	// A TMPDIR above the real home makes the real home "temporary": it is refused all the same.
	// No t.TempDir() below: it would create a directory beside the user's home.
	if realHome == "" {
		t.Skip("no home directory: the TMPDIR-above-HOME case needs one")
	}
	t.Setenv("TMPDIR", filepath.Dir(realHome))
	real := []string{"HOME=" + realHome, "CLAUDE_CONFIG_DIR=" + filepath.Join(realHome, ".claude"), "CLAUDE_CODE_PLUGIN_CACHE_DIR=" + filepath.Join(realHome, ".claude", "plugins")}
	if err := claudeEnvOutsideTemp(realHome, real); err == nil {
		t.Errorf("TMPDIR %s above the real home: the real home was accepted", filepath.Dir(realHome))
	}
	if err := claudeEnvOutsideTemp(filepath.Join(filepath.Dir(realHome), "t"), real); err == nil {
		t.Errorf("TMPDIR %s above the real home: HOME=%s was accepted", filepath.Dir(realHome), realHome)
	}
}

// `claude plugin validate` is the authoritative manifest check, run with the installed claude
// CLI on purpose; CI runners have none. It runs in a throwaway config like TestClaudePluginInstall:
// validating needs nothing from the user's own ~/.claude, so it must not touch it (#95).
func TestClaudePluginValidate(t *testing.T) {
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("the claude CLI is not installed (CI runners): claude plugin validate not run")
	}
	plug, _ := build(t)
	home, e := claudeConfig(t)
	if out, code := runClaude(t, home, e, "plugin", "validate", "--strict", plug); code != 0 {
		t.Fatalf("claude plugin validate --strict: exit %d\n%s", code, out)
	}
	root := repoRoot(t)
	if exists(filepath.Join(root, ".claude-plugin", "marketplace.json")) {
		if out, code := runClaude(t, home, e, "plugin", "validate", "--strict", root); code != 0 {
			t.Fatalf("claude plugin validate --strict (marketplace): exit %d\n%s", code, out)
		}
	}
}

// Installing through Claude Code's own plugin commands, from the committed marketplace, into a
// throwaway config (claudeConfig: the user's real ~/.claude is never touched). Opt-in with
// CLAUDUCTOR_PLUGIN_INSTALL_TEST=1: it runs the claude CLI, which CI runners do not have.
func TestClaudePluginInstall(t *testing.T) {
	if os.Getenv("CLAUDUCTOR_PLUGIN_INSTALL_TEST") != "1" {
		t.Skip("set CLAUDUCTOR_PLUGIN_INSTALL_TEST=1 to install the plugin with the claude CLI into a temporary config")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("the claude CLI is not installed")
	}
	home, e := claudeConfig(t)
	claude := func(args ...string) string {
		out, code := runClaude(t, home, e, args...)
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

// A plugin script sets CLAUDUCTOR_FW in its prologue without exporting it, so a `sh -c '…'` whose
// body names it reads an empty variable, unless the builder passes it on that command line
// (OPS-88: plugin/checks/statusline.sh, run directly, failed its two unwritable-HOME cases).
func TestShellChildGetsPluginRoot(t *testing.T) {
	out, _ := build(t)
	err := filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".sh") {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, bad := range unpassedChildren(string(raw)) {
			t.Errorf("%s: a sh -c names $%s without it among its assignments: %s", p, FWVar, bad)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The rewrite itself: each child judged alone, a value the template set kept, and nothing
	// inserted after a wrapper's command word (where it would be an argument, not an assignment).
	rw, err := newRewriter(filepath.Join(repoRoot(t), "template"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ in, want string }{
		{`a=$(sh -c '. "$ROOT/.claude/lib/conf.sh"'); b=$(X=1 sh -c '. "$ROOT/.claude/lib/conf.sh"')`,
			`a=$(CLAUDUCTOR_FW="$CLAUDUCTOR_FW" sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh"'); b=$(X=1 CLAUDUCTOR_FW="$CLAUDUCTOR_FW" sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh"')`},
		{`v=$(CLAUDUCTOR_FW="$d/fw" sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh"')`,
			`v=$(CLAUDUCTOR_FW="$d/fw" sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh"')`},
		{`v=$(CLAUDUCTOR_FW= sh -c '. "$ROOT/.claude/lib/conf.sh"')`,
			`v=$(CLAUDUCTOR_FW= sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh"')`},
		{`with_timeout 60 sh -c '. "$ROOT/.claude/lib/conf.sh"'`,
			`with_timeout 60 sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh"'`},
		{`v=$(sh -ec '. "${CLAUDUCTOR_FW:-$ROOT/.claude}/lib/conf.sh"')`,
			`v=$(CLAUDUCTOR_FW="$CLAUDUCTOR_FW" sh -ec '. "${CLAUDUCTOR_FW:-$ROOT/.claude}/lib/conf.sh"')`},
		// The class, not the instances (#232): any option spelling with a c, a body over several
		// lines, and a child inside a double-quoted command substitution.
		{`v=$(sh -ce '. "$ROOT/.claude/lib/conf.sh"')`,
			`v=$(CLAUDUCTOR_FW="$CLAUDUCTOR_FW" sh -ce '. "$CLAUDUCTOR_FW/lib/conf.sh"')`},
		{`v=$(sh -e -c '. "$ROOT/.claude/lib/conf.sh"')`,
			`v=$(CLAUDUCTOR_FW="$CLAUDUCTOR_FW" sh -e -c '. "$CLAUDUCTOR_FW/lib/conf.sh"')`},
		{"v=$(sh -c '\n  . \"$ROOT/.claude/lib/conf.sh\"\n  echo ok')",
			"v=$(CLAUDUCTOR_FW=\"$CLAUDUCTOR_FW\" sh -c '\n  . \"$CLAUDUCTOR_FW/lib/conf.sh\"\n  echo ok')"},
		{`x="$(sh -c '. "$ROOT/.claude/lib/conf.sh"')"`,
			`x="$(CLAUDUCTOR_FW="$CLAUDUCTOR_FW" sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh"')"`},
		{`v=$(sh -e '. "$ROOT/.claude/lib/conf.sh"')`, // no -c: the quoted word is a script path
			`v=$(sh -e '. "$CLAUDUCTOR_FW/lib/conf.sh"')`},
		{"# v=$(sh -c '. \"$CLAUDUCTOR_FW/lib/conf.sh\"')",
			"# v=$(sh -c '. \"$CLAUDUCTOR_FW/lib/conf.sh\"')"},
	} {
		if got := rw.shell("checks/x.sh", c.in); got != c.want {
			t.Errorf("shell(%s)\n got %s\nwant %s", c.in, got, c.want)
		}
	}
	if got := unpassedChildren(`with_timeout 60 sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh"'`); len(got) != 1 {
		t.Errorf("a child behind a wrapper is not reported: %q", got)
	}
	// A quoted value with a blank, a ; and a ( (change-tools.sh writes one) is still one assignment.
	if got := unpassedChildren(rw.shell("checks/x.sh", `out=$(MSG="a; b (c)" sh -c '. "$ROOT/.claude/lib/conf.sh"')`)); len(got) != 0 {
		t.Errorf("a passed child after a quoted value with a blank is reported: %q", got)
	}
	// A correct child inside "$( … )" is not reported (#232, item 1); each unpassed spelling is.
	if got := unpassedChildren(rw.shell("checks/x.sh", `x="$(sh -c '. "$ROOT/.claude/lib/conf.sh"')"`)); len(got) != 0 {
		t.Errorf("a passed child inside a double-quoted command substitution is reported: %q", got)
	}
	for _, l := range []string{
		`v=$(sh -ce '. "$CLAUDUCTOR_FW/lib/conf.sh"')`,
		`v=$(sh -e -c '. "$CLAUDUCTOR_FW/lib/conf.sh"')`,
		"v=$(sh -c '\n  . \"$CLAUDUCTOR_FW/lib/conf.sh\"')",
		`x="$(sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh"')"`,
		// Forms the rewrite does not pass: reported, so a template line using one fails here.
		`v=$(sh -o errexit -c '. "$CLAUDUCTOR_FW/lib/conf.sh"')`,
		`v=$(sh +e -c '. "$CLAUDUCTOR_FW/lib/conf.sh"')`,
		`v=$(sh -c -- '. "$CLAUDUCTOR_FW/lib/conf.sh"')`,
		"x=`sh -c '. \"$CLAUDUCTOR_FW/lib/conf.sh\"'`",
		`v=$(/bin/sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh"')`,
		`v=$(bash -c '. "$CLAUDUCTOR_FW/lib/conf.sh"')`,
		// After a "$( … )" closed on the same line, the child is still a command.
		`x="$(date)"; sh -o errexit -c '. "$CLAUDUCTOR_FW/lib/conf.sh"'`,
		`MSG="$(date)" bash -c '. "$CLAUDUCTOR_FW/lib/conf.sh"'`,
		"x=\"`date`\"; sh -c '. \"$CLAUDUCTOR_FW/lib/conf.sh\"'",
	} {
		if got := unpassedChildren(l); len(got) != 1 {
			t.Errorf("an unpassed child is not reported once: %q in %q", got, l)
		}
	}
	// A passed child after an assignment whose value is a closed substitution is not reported: the
	// command still starts where the shell starts it, at the assignment.
	for _, l := range []string{
		`REV="$(git rev-parse HEAD)" sh -c '. "$ROOT/.claude/lib/conf.sh"'`,
		"REV=\"`git rev-parse HEAD`\" sh -c '. \"$ROOT/.claude/lib/conf.sh\"'",
	} {
		if got := unpassedChildren(rw.shell("checks/x.sh", l)); len(got) != 0 {
			t.Errorf("a passed child is reported: %q in %q", got, rw.shell("checks/x.sh", l))
		}
	}
	// Nor is one passed by hand in a form the rewrite does not write, while the same form unpassed is
	// reported: each form is judged both ways, so neither half passes on the other's mistake (#274).
	// An unquoted substitution's blanks split no word; a case pattern's `)`, with or without its
	// leading `(` and on a line of its own, starts a command and closes no substitution, and a
	// `case` after reserved words is still a case; the words of a value are its outer command's,
	// whatever quotes and parentheses it nests.
	pass := `CLAUDUCTOR_FW="$CLAUDUCTOR_FW" sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh"'`
	bare := `sh -c '. "$CLAUDUCTOR_FW/lib/conf.sh"'`
	for _, form := range []string{
		`REV=$(git rev-parse HEAD) %s`,
		`case $1 in a) %s ;; esac`,
		`x="$(case $1 in a) %s ;; esac)"`,
		// A case after a reserved word (#274, item 1: the silent one, inside "$( … )").
		`x="$(if true; then case $1 in a) %s ;; esac; fi)"`,
		`x="$(for f in a; do case $f in a) %s ;; esac; done)"`,
		`x="$({ case $1 in a) %s ;; esac; })"`,
		`x="$(! case $1 in a) %s ;; esac)"`,
		`if true; then case $1 in a) %s ;; esac; fi`,
		// Nested quotes and parentheses in a value (item 2).
		`REV="$(git log -1 --format="%%H %%s")" %s`,
		`X="$(echo "(")" %s`,
		`X=$(case $1 in a) echo;; esac) %s`,
		// Case shapes (item 3): a leading-paren pattern, and an arm on a line of its own.
		`case $1 in (a) %s ;; esac`,
		"case $1 in\n  a) %s ;;\nesac",
		"case $1 in\n  (a) %s ;;\nesac",
	} {
		if l := fmt.Sprintf(form, pass); len(unpassedChildren(l)) != 0 {
			t.Errorf("a child passed by hand is reported: %q in %q", unpassedChildren(l), l)
		}
		if l := fmt.Sprintf(form, bare); len(unpassedChildren(l)) != 1 {
			t.Errorf("an unpassed child is not reported once: %q in %q", unpassedChildren(l), l)
		}
	}
	// The rewrite does not pass those forms: a template line using one fails here, not ships.
	for _, l := range []string{
		rw.shell("checks/x.sh", `REV=$(git rev-parse HEAD) sh -c '. "$ROOT/.claude/lib/conf.sh"'`),
		rw.shell("checks/x.sh", `case $1 in a) sh -c '. "$ROOT/.claude/lib/conf.sh"' ;; esac`),
		rw.shell("checks/x.sh", `x="$(if true; then case $1 in a) sh -c '. "$ROOT/.claude/lib/conf.sh"' ;; esac; fi)"`),
	} {
		if got := unpassedChildren(l); len(got) != 1 {
			t.Errorf("an unpassed child is not reported once: %q in %q", got, l)
		}
	}
	if got := unpassedChildren("# sh -c '. \"$CLAUDUCTOR_FW/lib/conf.sh\"'\nv=$(sh -e '$CLAUDUCTOR_FW/x.sh')"); len(got) != 0 {
		t.Errorf("a comment, or a sh with no -c, is reported: %q", got)
	}
}

// unpassedChildren returns each child `<any shell> <arguments, an option with c among them> '<body>'`
// in the script whose body names CLAUDUCTOR_FW and whose command does not set it among its leading
// assignments. Written apart from the rewrite's regex, so the two do not share a mistake: any shell
// by any path (`/bin/sh`, `bash`), any unquoted arguments (`-o errexit -c`, `+e`, `--`), a body
// that runs to its closing quote over as many lines as it takes, and the text before the child on
// its line read as the shell reads it (commandWords) to find where its command starts (#232). A
// form the rewrite does not pass is then reported here, not shipped with an empty variable.
func unpassedChildren(s string) []string {
	var bad []string
	child := regexp.MustCompile("(^|[\\s;&|(`])((?:[\\w./-]*/)?(?:ba|da|z|k)?sh)((?:[ \\t]+[^\\s'\"]+)+)[ \\t]+'")
	withC := regexp.MustCompile(`(^|\s)-[A-Za-z]*c[A-Za-z]*(\s|$)`)
	names := regexp.MustCompile(`\$\{?` + FWVar + `\b`)
	assign := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	for _, loc := range child.FindAllStringSubmatchIndex(s, -1) {
		end := strings.IndexByte(s[loc[1]:], '\'')
		if !withC.MatchString(s[loc[6]:loc[7]]) || end < 0 || !names.MatchString(s[loc[1]:loc[1]+end]) {
			continue
		}
		sh := loc[4]
		words, isCommand := commandWords(s[strings.LastIndexByte(s[:sh], '\n')+1 : sh])
		if !isCommand {
			continue
		}
		passed := false
		for _, w := range words {
			if !assign.MatchString(w) {
				passed = false
				break
			}
			passed = passed || strings.HasPrefix(w, FWVar+"=")
		}
		if !passed {
			bad = append(bad, strings.SplitN(s[sh:loc[1]+end+1], "\n", 2)[0])
		}
	}
	return bad
}

// commandWords reads a line up to a child as the shell would: quotes hold blanks and separators, a
// `$(` starts a new command even inside double quotes, and the words since the last command start
// are returned. isCommand is false when the child is in a comment or inside a quoted string.
func commandWords(p string) (words []string, isCommand bool) {
	start, q := 0, byte(0)
	// splits holds the blanks that split the current command's words: only those read at its own
	// level, outside quotes and outside anything it nests (#274, item 2).
	var splits []int
	// Each open `(`, `$(` or backtick saves the quote it was read in, where the outer command
	// started and its splits, and its close restores them: the `"` that closes `"$(date)"` closes
	// the outer quote, and `REV="$(git rev-parse HEAD)" CLAUDUCTOR_FW=… sh` is still one command of
	// assignments. After a plain `(`'s close a command starts: only an operator may follow a
	// subshell, so a word there follows a case pattern's leading `(` (item 3). A frame also counts
	// the `case`s open in it, whose pattern's `)` closes nothing.
	type frame struct {
		q            byte
		start, cases int
		splits       []int
		tick, paren  bool
	}
	frames := []frame{{}}
	// A case keyword stands where a command starts, or after reserved words only: `then case`,
	// `do case`, `{ case`, `! case` (item 1).
	reserved := map[string]bool{"!": true, "{": true, "if": true, "then": true, "else": true, "elif": true, "do": true, "while": true, "until": true}
	keyword := func(i int, k string) bool {
		e := i + len(k)
		if !strings.HasPrefix(p[i:], k) || (e < len(p) && strings.IndexByte(" \t;)", p[e]) < 0) ||
			(i > start && strings.IndexByte(" \t", p[i-1]) < 0) {
			return false
		}
		for _, w := range strings.Fields(p[start:i]) {
			if !reserved[w] {
				return false
			}
		}
		return true
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		top := &frames[len(frames)-1]
		switch {
		case q == '\'':
			if c == '\'' {
				q = 0
			}
		case c == '\\':
			i++
		case c == '$' && i+1 < len(p) && p[i+1] == '(':
			frames = append(frames, frame{q: q, start: start, splits: splits})
			start, q, splits = i+2, 0, nil
			i++
		case c == '`' && top.tick:
			q, start, splits, frames = top.q, top.start, top.splits, frames[:len(frames)-1]
		case c == '`':
			frames = append(frames, frame{q: q, start: start, splits: splits, tick: true})
			start, q, splits = i+1, 0, nil
		case q == '"':
			if c == '"' {
				q = 0
			}
		case c == '\'' || c == '"':
			q = c
		case c == '#' && (i == 0 || strings.IndexByte(" \t;&|(", p[i-1]) >= 0):
			return nil, false
		case keyword(i, "case"):
			top.cases++
			i += len("case") - 1
		case keyword(i, "esac") && top.cases > 0:
			top.cases--
			i += len("esac") - 1
		case c == ')' && top.cases > 0:
			start, splits = i+1, nil
		case c == '(':
			frames = append(frames, frame{q: q, start: start, splits: splits, paren: true})
			start, splits = i+1, nil
		case c == ')' && len(frames) > 1 && !top.tick:
			paren := top.paren
			q, start, splits, frames = top.q, top.start, top.splits, frames[:len(frames)-1]
			if paren {
				start, splits = i+1, nil
			}
		case c == ')':
			// It closes nothing on this line: a case pattern whose case opened on an earlier one.
			start, splits = i+1, nil
		case strings.IndexByte(";&|", c) >= 0:
			start, splits = i+1, nil
		case c == ' ' || c == '\t':
			splits = append(splits, i)
		}
	}
	if q != 0 {
		return nil, false
	}
	from := start
	for _, s := range append(splits, len(p)) {
		if w := p[from:s]; w != "" {
			words = append(words, w)
		}
		from = s + 1
	}
	return words, true
}
