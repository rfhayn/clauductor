// Package plugin packages the operating model in template/ as a Claude Code plugin, and the repo
// root as the marketplace that lists it (docs/plugin.md).
//
// A plugin installs into Claude Code, not into a repository, so the template splits in two:
//
//   - the FRAMEWORK tier (skills, agents, hooks, the workflow, the checks and the scripts they
//     call) becomes the plugin's components, with every `.claude/...` path rewritten to the
//     plugin's own root, because the plugin's copy lives in Claude Code's plugin cache;
//   - the PROJECT tier (AGENTS.md, project.conf, model-roles.json, docs/, changes/, specs/,
//     scripts/ci/, .clauductor/panel.json, the settings the project needs) is carried under
//     scaffold/, which no plugin component loads, and which the plugin's `init` skill copies into
//     a repository without overwriting anything.
//
// The template is never edited for the plugin's sake: every adaptation is a rewrite here, so a
// later template change flows into the next build. A template file this packager cannot place is
// an error, not a silent omission.
package plugin

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/clauductor/clauductor/internal/template"
)

// Name is the plugin's name, which namespaces every component (`/clauductor:session-start`), and
// the marketplace's name, so the install id is clauductor@clauductor.
const Name = "clauductor"

// Marketplace is the name of the marketplace the repo root declares.
const Marketplace = "clauductor"

// Repo is the GitHub repository users add as the marketplace.
const Repo = "rfhayn/clauductor"

// PluginDir is where the built plugin is committed, relative to the repo root. A marketplace's
// relative-path source must exist in the clone, so the build output is checked in and a test
// holds it to the template (TestCommittedPluginIsCurrent).
const PluginDir = "plugin"

// ScaffoldDir holds the project-owned files inside the plugin.
const ScaffoldDir = "scaffold"

// FWVar is the shell variable the rewritten scripts use for the plugin root. Each executed script
// computes it from its own location, so it names the copy that is running (the plugin cache, or a
// scratch repository a check copied the script into).
const FWVar = "CLAUDUCTOR_FW"

// projectSkills are template skills a project edits (CONFIGURE FIRST stubs). A plugin's files are
// read-only in the cache, so these are scaffolded as the project's own skills instead. Mirrors
// ProjectSkills in internal/template/tier.go.
var projectSkills = map[string]bool{"architecture-audit": true, "release-prep": true}

// frameworkDirs are the template's `.claude/` directories that become plugin directories of the
// same name. `.claude/health/` is not one: `clauductor install` treats it as the project's (a
// project deletes the GitHub lines it does not use), so it is scaffolded. Nor is `.claude/evals/`:
// a project adds cases for its own domain and commits the receipts its model choices rest on
// (OPS-10), so the suite, its runner and its receipts are the project's.
var frameworkDirs = []string{"skills", "agents", "hooks", "checks", "lib", "modules", "workflows", "examples"}

// droppedHooks are template hooks the plugin does not register, each with the reason. The script
// itself still ships (the checks exercise it).
var droppedHooks = map[string]string{
	"worktree-hook-drift.sh": "it guards against hooks read from a main checkout that lags origin/main; a plugin's hooks are read from the plugin cache, one version for every worktree, so the drift it detects cannot happen",
}

// agentNames are the template's agents; skill prose and the workflow name them bare, and a
// plugin's agents are namespaced.
func agentNames(tmpl string) ([]string, error) {
	m, err := filepath.Glob(filepath.Join(tmpl, ".claude", "agents", "*.md"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range m {
		out = append(out, strings.TrimSuffix(filepath.Base(f), ".md"))
	}
	sort.Strings(out)
	return out, nil
}

//go:embed all:assets
var assets embed.FS

// Options configure a build.
type Options struct {
	RepoRoot string // the clauductor repository (template/ lives under it)
	OutDir   string // where the plugin is written; default <RepoRoot>/plugin
	Version  string // the plugin version (internal/cmd.Version)
}

// Result reports what a build placed where.
type Result struct {
	Plugin   []string // files written under the plugin root, relative to it
	Scaffold []string // template files carried under scaffold/, relative to the template
}

// Build assembles the plugin from RepoRoot/template into OutDir and writes the marketplace
// manifest at RepoRoot/.claude-plugin/marketplace.json when OutDir is the default.
func Build(o Options) (*Result, error) {
	tmpl := filepath.Join(o.RepoRoot, "template")
	if _, err := os.Stat(filepath.Join(tmpl, ".claude")); err != nil {
		return nil, fmt.Errorf("no template at %s: %w", tmpl, err)
	}
	if o.Version == "" {
		return nil, fmt.Errorf("plugin build: no version")
	}
	defaultOut := filepath.Join(o.RepoRoot, PluginDir)
	if o.OutDir == "" {
		o.OutDir = defaultOut
	}
	// Place every file before touching the output: a template file nobody has classified fails
	// the build without leaving a half-cleared plugin/ behind.
	all, err := listFiles(tmpl)
	if err != nil {
		return nil, err
	}
	for _, rel := range all {
		if _, _, err := place(rel); err != nil {
			return nil, err
		}
	}
	if err := resetOut(o.OutDir); err != nil {
		return nil, err
	}

	rw, err := newRewriter(tmpl)
	if err != nil {
		return nil, err
	}
	res := &Result{}
	w := &writer{root: o.OutDir, res: res}

	files, err := listFiles(tmpl)
	if err != nil {
		return nil, err
	}
	var settings []byte
	for _, rel := range files {
		src := filepath.Join(tmpl, rel)
		data, err := os.ReadFile(src)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(src)
		if err != nil {
			return nil, err
		}
		mode := info.Mode().Perm()

		dest, kind, err := place(rel)
		if err != nil {
			return nil, err
		}
		switch kind {
		case kindSettings:
			settings = data
		case kindPlugin:
			out, err := rw.pluginFile(dest, data)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", rel, err)
			}
			if err := w.write(dest, out, mode); err != nil {
				return nil, err
			}
		case kindScaffold:
			out := rw.scaffoldFile(rel, data)
			if err := w.write(filepath.ToSlash(filepath.Join(ScaffoldDir, rel)), out, mode); err != nil {
				return nil, err
			}
			res.Scaffold = append(res.Scaffold, rel)
		}
		// The gate runs in CI, where no plugin is installed, and sources conf.sh: the project
		// keeps a copy of that one library (docs/plugin.md, What stays in the repository).
		if rel == ".claude/lib/conf.sh" {
			if err := w.write(ScaffoldDir+"/"+rel, data, mode); err != nil {
				return nil, err
			}
			res.Scaffold = append(res.Scaffold, rel)
		}
	}
	if settings == nil {
		return nil, fmt.Errorf("template has no .claude/settings.json")
	}

	hooks, err := pluginHooks(settings)
	if err != nil {
		return nil, err
	}
	if err := w.write("hooks/hooks.json", hooks, 0o644); err != nil {
		return nil, err
	}
	scaffoldSettings, err := projectSettings(settings)
	if err != nil {
		return nil, err
	}
	if err := w.write(ScaffoldDir+"/.claude/settings.json", scaffoldSettings, 0o644); err != nil {
		return nil, err
	}
	res.Scaffold = append(res.Scaffold, ".claude/settings.json")

	model, effort, err := frontmatterModel(filepath.Join(tmpl, ".claude", "skills", "start-project", "SKILL.md"))
	if err != nil {
		return nil, err
	}
	if err := copyAssets(w, strings.NewReplacer("@VERSION@", o.Version, "@MODEL@", model, "@EFFORT@", effort)); err != nil {
		return nil, err
	}
	manifest, err := pluginManifest(o.Version)
	if err != nil {
		return nil, err
	}
	if err := w.write(".claude-plugin/plugin.json", manifest, 0o644); err != nil {
		return nil, err
	}

	if filepath.Clean(o.OutDir) == filepath.Clean(defaultOut) {
		mk, err := MarketplaceJSON()
		if err != nil {
			return nil, err
		}
		p := filepath.Join(o.RepoRoot, ".claude-plugin", "marketplace.json")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(p, mk, 0o644); err != nil {
			return nil, err
		}
	}
	sort.Strings(res.Plugin)
	sort.Strings(res.Scaffold)
	return res, nil
}

// resetOut empties the output directory so a file removed from the template leaves the plugin
// too. It refuses a non-empty directory that is not a previous build.
func resetOut(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return os.MkdirAll(dir, 0o755)
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		if _, err := os.Stat(filepath.Join(dir, ".claude-plugin", "plugin.json")); err != nil {
			return fmt.Errorf("refusing to clear %s: it is not empty and holds no .claude-plugin/plugin.json", dir)
		}
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

type fileKind int

const (
	kindPlugin fileKind = iota
	kindScaffold
	kindSettings
)

// place decides where a template file goes: its path inside the plugin, or the scaffold.
func place(rel string) (string, fileKind, error) {
	if !strings.HasPrefix(rel, ".claude/") {
		return "", kindScaffold, nil // AGENTS.md, docs/, changes/, specs/, scripts/ci/, .clauductor/ ...
	}
	sub := strings.TrimPrefix(rel, ".claude/")
	if sub == "settings.json" {
		return "", kindSettings, nil
	}
	if !strings.Contains(sub, "/") {
		switch {
		case strings.HasSuffix(sub, ".sh"):
			return sub, kindPlugin, nil // statusline.sh, roadmap-queue.sh, ...: the model's own scripts
		case strings.HasSuffix(sub, ".json"), strings.HasSuffix(sub, ".conf"):
			return "", kindScaffold, nil // project.conf, model-roles.json: the project's values
		}
		return "", 0, fmt.Errorf("template file %s: the plugin packager does not know whether it is the framework's or the project's (framework/internal/plugin/build.go, place)", rel)
	}
	dir := strings.SplitN(sub, "/", 2)[0]
	if dir == "skills" {
		skill := strings.SplitN(strings.TrimPrefix(sub, "skills/"), "/", 2)[0]
		if projectSkills[skill] {
			return "", kindScaffold, nil
		}
	}
	// .claude/local/ is the project's own extension layer (P1.2): scaffolded once, never the
	// plugin's. A project's own modules also live in its repository (.claude/modules/<name>/,
	// found by lib/modules.sh beside the plugin's shipped ones), so only shipped modules are here.
	if dir == "health" || dir == "evals" || dir == "local" {
		return "", kindScaffold, nil
	}
	for _, d := range frameworkDirs {
		if dir == d {
			return sub, kindPlugin, nil
		}
	}
	return "", 0, fmt.Errorf("template directory .claude/%s/: the plugin packager does not know whether it is the framework's or the project's (framework/internal/plugin/build.go, frameworkDirs)", dir)
}

func listFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "worktrees" && filepath.Base(filepath.Dir(p)) == ".claude" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() == ".DS_Store" {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == template.VersionFile {
			return nil // the template's version marker, which no project receives
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	return files, err
}

type writer struct {
	root string
	res  *Result
}

func (w *writer) write(rel string, data []byte, mode os.FileMode) error {
	p := filepath.Join(w.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, data, mode); err != nil {
		return err
	}
	// WriteFile keeps an existing file's mode; resetOut removed everything, but be exact anyway.
	if err := os.Chmod(p, mode); err != nil {
		return err
	}
	w.res.Plugin = append(w.res.Plugin, rel)
	return nil
}

// copyAssets writes the packaging layer's own files: the init skill and scaffolder, the hook
// wrapper, the session hook, the bin/ command, and the gate's plugin resolver.
func copyAssets(w *writer, vars *strings.Replacer) error {
	return fs.WalkDir(assets, "assets", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := assets.ReadFile(p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "assets/")
		data = []byte(vars.Replace(string(data)))
		var mode os.FileMode = 0o644
		if strings.HasSuffix(rel, ".sh") || strings.HasPrefix(rel, "bin/") {
			mode = 0o755
		}
		return w.write(rel, data, mode)
	})
}

// frontmatterModel reads start-project's model and effort, which the init skill shares: it is the
// same setup work, and model-roles.json maps init to start-project's role (scaffoldFile).
func frontmatterModel(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	front, _ := splitFrontmatter(string(data))
	var model, effort string
	for _, l := range strings.Split(front, "\n") {
		if v, ok := strings.CutPrefix(l, "model:"); ok {
			model = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(l, "effort:"); ok {
			effort = strings.TrimSpace(v)
		}
	}
	if model == "" || effort == "" {
		return "", "", fmt.Errorf("%s: no model/effort in the frontmatter, which the init skill copies", path)
	}
	return model, effort, nil
}

// pluginManifest is .claude-plugin/plugin.json. The version is the framework's one Version
// (internal/cmd/root.go): a release that should reach installed users bumps it.
func pluginManifest(version string) ([]byte, error) {
	m := map[string]any{
		"name":        Name,
		"displayName": "Clauductor",
		"version":     version,
		"description": "Clauductor's operating model for Claude Code: hands-off sessions, a roadmap queue, approved proposals, per-task-group build and review, and hooks and checks that execute the rules. Run /clauductor:init in a repository to scaffold its project files.",
		"author":      map[string]any{"name": "Rich Hayn"},
		"homepage":    "https://github.com/" + Repo,
		"repository":  "https://github.com/" + Repo,
		"license":     "MIT",
		"keywords":    []string{"orchestration", "workflow", "operating-model", "hooks", "roadmap"},
	}
	return marshal(m)
}

// MarketplaceJSON is the repo root's .claude-plugin/marketplace.json. It carries no version: the
// plugin's manifest is the one place it is set.
func MarketplaceJSON() ([]byte, error) {
	m := map[string]any{
		"name":        Marketplace,
		"description": "Clauductor's operating model for Claude Code, as a plugin.",
		"owner":       map[string]any{"name": "Rich Hayn"},
		"plugins": []any{map[string]any{
			"name":        Name,
			"source":      "./" + PluginDir,
			"description": "The operating model: skills, agents, hooks, the build-change workflow and the process checks. /clauductor:init scaffolds a repository's own files.",
			"category":    "development",
		}},
	}
	return marshal(m)
}

func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// hookCmd matches how the template registers a hook in settings.json.
var hookCmd = regexp.MustCompile(`^sh "\$CLAUDE_PROJECT_DIR"/\.claude/hooks/([A-Za-z0-9_.-]+\.sh)$`)

// pluginHooks turns the template's settings.json hooks into hooks/hooks.json. Every handler runs
// through if-project.sh, which stands aside in a repository the plugin has not set up: an
// installed plugin's hooks fire in every repository the user opens.
func pluginHooks(settings []byte) ([]byte, error) {
	var s struct {
		Hooks map[string][]struct {
			Matcher string           `json:"matcher,omitempty"`
			Hooks   []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(settings, &s); err != nil {
		return nil, fmt.Errorf("template settings.json: %w", err)
	}
	type group struct {
		Matcher string           `json:"matcher,omitempty"`
		Hooks   []map[string]any `json:"hooks"`
	}
	out := map[string][]group{}
	for event, groups := range s.Hooks {
		for _, g := range groups {
			ng := group{Matcher: g.Matcher}
			for _, h := range g.Hooks {
				cmd, _ := h["command"].(string)
				m := hookCmd.FindStringSubmatch(cmd)
				if m == nil {
					return nil, fmt.Errorf("template hook %q (%s): not of the form sh \"$CLAUDE_PROJECT_DIR\"/.claude/hooks/<x>.sh, which the packager rewrites (framework/internal/plugin/build.go, pluginHooks)", cmd, event)
				}
				if _, drop := droppedHooks[m[1]]; drop {
					continue
				}
				nh := map[string]any{}
				for k, v := range h {
					nh[k] = v
				}
				nh["command"] = `sh "${CLAUDE_PLUGIN_ROOT}/hooks/if-project.sh" ` + m[1]
				ng.Hooks = append(ng.Hooks, nh)
			}
			if len(ng.Hooks) > 0 {
				out[event] = append(out[event], ng)
			}
		}
	}
	out["SessionStart"] = append(out["SessionStart"], group{Hooks: []map[string]any{{
		"type":    "command",
		"command": `sh "${CLAUDE_PLUGIN_ROOT}/hooks/plugin-session.sh"`,
	}}})
	return marshal(map[string]any{
		"description": "Clauductor's operating-model hooks. Each runs only in a repository /clauductor:init set up (.claude/clauductor-plugin).",
		"hooks":       out,
	})
}

// StatusLineCommand is the project's status line in plugin mode. A statusLine cannot come from a
// plugin and cannot name ${CLAUDE_PLUGIN_ROOT}, whose path changes with every version, so the
// plugin's SessionStart hook writes a shim into its persistent data directory, which does not.
const StatusLineCommand = `f="${CLAUDE_CODE_PLUGIN_CACHE_DIR:-${CLAUDE_CONFIG_DIR:-$HOME/.claude}/plugins}/data/clauductor-clauductor/statusline.sh"; [ -f "$f" ] && sh "$f"`

// projectSettings is the scaffold's .claude/settings.json: the template's settings minus the
// hooks (the plugin registers them), with the status line through the shim, the allow rules for
// the model's scripts through bin/clauductor-model, and the plugin enabled for everyone who
// clones the repository.
func projectSettings(settings []byte) ([]byte, error) {
	keys, vals, err := orderedObject(settings)
	if err != nil {
		return nil, fmt.Errorf("template settings.json: %w", err)
	}
	var b bytes.Buffer
	b.WriteString("{\n")
	first := true
	emit := func(k string, v []byte) error {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, v, "  ", "  "); err != nil {
			return err
		}
		if !first {
			b.WriteString(",\n")
		}
		first = false
		kb, _ := json.Marshal(k)
		fmt.Fprintf(&b, "  %s: %s", kb, pretty.Bytes())
		return nil
	}
	for i, k := range keys {
		v := vals[i]
		switch k {
		case "hooks":
			continue
		case "statusLine":
			nv, err := marshalCompact(map[string]any{"type": "command", "command": StatusLineCommand})
			if err != nil {
				return nil, err
			}
			v = nv
		case "permissions":
			var p map[string]any
			if err := json.Unmarshal(v, &p); err != nil {
				return nil, err
			}
			if allow, ok := p["allow"].([]any); ok {
				for j, a := range allow {
					if s, ok := a.(string); ok {
						allow[j] = rewriteAllow(s)
					}
				}
			}
			nv, err := marshalCompact(p)
			if err != nil {
				return nil, err
			}
			v = nv
		}
		if err := emit(k, v); err != nil {
			return nil, err
		}
	}
	mk, _ := marshalCompact(map[string]any{Marketplace: map[string]any{"source": map[string]any{"source": "github", "repo": Repo}}})
	if err := emit("extraKnownMarketplaces", mk); err != nil {
		return nil, err
	}
	en, _ := marshalCompact(map[string]any{Name + "@" + Marketplace: true})
	if err := emit("enabledPlugins", en); err != nil {
		return nil, err
	}
	b.WriteString("\n}\n")
	return b.Bytes(), nil
}

var allowSh = regexp.MustCompile(`^Bash\(sh \.claude/([^ )]+)(.*)\)$`)

// rewriteAllow maps `Bash(sh .claude/x.sh …)` to the plugin's command. Only framework scripts
// move; anything else (scripts/ci/gate.sh) is the project's and stays.
func rewriteAllow(s string) string {
	m := allowSh.FindStringSubmatch(s)
	if m == nil {
		return s
	}
	return "Bash(clauductor-model " + m[1] + m[2] + ")"
}

func marshalCompact(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(b.Bytes()), nil
}

// orderedObject returns a JSON object's top-level keys in file order with their raw values, so
// the scaffolded settings.json reads like the template's.
func orderedObject(data []byte) ([]string, []json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, nil, fmt.Errorf("not a JSON object")
	}
	var keys []string
	var vals []json.RawMessage
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		k, _ := t.(string)
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, nil, err
		}
		keys = append(keys, k)
		vals = append(vals, v)
	}
	return keys, vals, nil
}
