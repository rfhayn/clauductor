package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// rewriter holds the path rewrites, built from what the template actually contains, so a new
// framework script or directory is covered the moment it exists.
type rewriter struct {
	fw       string         // alternation of framework path heads: skills/(?!project)…|hooks/|statusline\.sh|…
	agents   []string       // agent names, for namespacing
	shRoot   *regexp.Regexp // ROOT=$(cd "$(dirname "$0")/../.." && pwd)
	shVarFW  *regexp.Regexp // "$ROOT/.claude/<fw>  and  "$ROOT"/.claude/<fw>
	shBare   *regexp.Regexp // sh .claude/<fw>   and   in .claude/<fw>
	mdFW     *regexp.Regexp // .claude/<fw> in Markdown
	mdSh     *regexp.Regexp // sh .claude/<fw> in Markdown
	mdCtx    *regexp.Regexp // !`sh .claude/<fw> : a skill's context line
	mdAgent  *regexp.Regexp // `builder` agent
	jsAgent  *regexp.Regexp // agentType: 'builder'
	scaffSh  *regexp.Regexp // sh .claude/<fw> in the project's gate scripts
	panelArg *regexp.Regexp // "sh", ".claude/<fw> in panel.json command arrays
	initRole *regexp.Regexp // "start-project": "<role>" in model-roles.json
	pbStart  *regexp.Regexp // the playbook's start-project row
	skillRef *regexp.Regexp // /session-start: a plugin skill named as a command
}

func newRewriter(tmpl string) (*rewriter, error) {
	var heads []string
	for _, d := range frameworkDirs {
		if d == "skills" {
			continue // added per skill below, so the project's skills keep their paths
		}
		heads = append(heads, regexp.QuoteMeta(d)+`\b`)
	}
	skills, err := os.ReadDir(filepath.Join(tmpl, ".claude", "skills"))
	if err != nil {
		return nil, err
	}
	for _, s := range skills {
		if s.IsDir() && !projectSkills[s.Name()] {
			heads = append(heads, "skills/"+regexp.QuoteMeta(s.Name())+"/")
		}
	}
	names := []string{"init"}
	for _, s := range skills {
		if s.IsDir() && !projectSkills[s.Name()] {
			names = append(names, regexp.QuoteMeta(s.Name()))
		}
	}
	sort.SliceStable(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	skillRef := regexp.MustCompile("(^|[\\s(`|\"'\\[])/(" + strings.Join(names, "|") + ")([^A-Za-z0-9_/:-]|$)")
	heads = append(heads, `skills/\*`) // "sh .claude/skills/*/SKILL.md"-style globs in checks
	scripts, err := filepath.Glob(filepath.Join(tmpl, ".claude", "*.sh"))
	if err != nil {
		return nil, err
	}
	for _, s := range scripts {
		heads = append(heads, regexp.QuoteMeta(filepath.Base(s))+`\b`)
	}
	sort.Strings(heads)
	// Longest first: regexp alternation is leftmost-first, not longest.
	sort.SliceStable(heads, func(i, j int) bool { return len(heads[i]) > len(heads[j]) })
	fw := "(?:" + strings.Join(heads, "|") + ")"

	agents, err := agentNames(tmpl)
	if err != nil {
		return nil, err
	}
	var qa []string
	for _, a := range agents {
		qa = append(qa, regexp.QuoteMeta(a))
	}
	sort.SliceStable(qa, func(i, j int) bool { return len(qa[i]) > len(qa[j]) })
	an := "(?:" + strings.Join(qa, "|") + ")"

	return &rewriter{
		fw:       fw,
		agents:   agents,
		shRoot:   regexp.MustCompile(`\b(ROOT|ROOT_HOOK)=\$\(cd "(?:\$\(dirname "\$0"\)|\$dir)/[./]+"(?: 2>/dev/null)? && pwd\)`),
		shVarFW:  regexp.MustCompile(`"(?:\$ROOT|\$ROOT_HOOK|\$\{ROOT\})(")?/\.claude/(` + fw + `)`),
		shBare:   regexp.MustCompile(`\b(sh|in) \.claude/(` + fw + `)`),
		mdSh:     regexp.MustCompile(`\bsh \.claude/(` + fw + `)`),
		panelArg: regexp.MustCompile(`"sh", "\.claude/(` + fw + `)`),
		skillRef: skillRef,
		pbStart:  regexp.MustCompile("(?m)^\\| `/start-project` \\|[^|\n]*\\| ([a-z-]+) \\|$"),
		initRole: regexp.MustCompile(`(?m)^(\s*)"start-project": "([a-z-]+)"`),
		mdCtx:    regexp.MustCompile("!`sh \\.claude/(" + fw + ")"),
		scaffSh:  regexp.MustCompile(`\bsh \.claude/(` + fw + `)`),
		mdFW:     regexp.MustCompile(`\.claude/(` + fw + `)`),
		mdAgent:  regexp.MustCompile("`(" + an + ")`( agent| lane)"),
		jsAgent:  regexp.MustCompile(`agentType: '(` + an + `)'`),
	}, nil
}

// pluginFile rewrites one framework file for its place in the plugin (dest is plugin-relative).
func (r *rewriter) pluginFile(dest string, data []byte) ([]byte, error) {
	s := string(data)
	switch {
	case strings.HasSuffix(dest, ".sh"):
		out := r.shell(dest, s)
		for _, p := range shellPatches[dest] {
			if !strings.Contains(out, p.old) {
				return nil, fmt.Errorf("plugin patch for %s no longer applies: %q is gone from the template (framework/internal/plugin/rewrite.go, shellPatches)", dest, p.old)
			}
			out = strings.ReplaceAll(out, p.old, p.new)
		}
		return []byte(out), nil
	case strings.HasSuffix(dest, ".md") && (strings.HasPrefix(dest, "skills/") || strings.HasPrefix(dest, "agents/")):
		return []byte(r.componentMarkdown(dest, s)), nil
	case strings.HasSuffix(dest, ".js"):
		// The workflow's prompts tell agents to run the model's scripts: through clauductor-model.
		s = r.mdSh.ReplaceAllString(s, "clauductor-model $1")
		return []byte(r.jsAgent.ReplaceAllString(s, "agentType: '"+Name+":$1'")), nil
	}
	return data, nil // READMEs, config.yaml, merge-reader.awk: read by people or by path
}

// shell rewrites a framework script: the project root comes from git (the script's own location
// is now the plugin cache), and the model's own files come from CLAUDUCTOR_FW, which an executed
// script sets from its own location. A sourced file (no shebang: conf.sh, checks/lib.sh,
// hooks/lib/*) uses its caller's.
func (r *rewriter) shell(dest, s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue // comments keep the template's wording
		}
		// A copy that sits in a project's .claude/ (a check's scratch repository copies scripts
		// there) keeps the template's meaning: the project is the directory above. The plugin's own
		// copy is under no .claude/: it takes the project its caller names in ROOT (a check pointing
		// it at a scratch project), else the repository the command runs in.
		l = r.shRoot.ReplaceAllString(l, `${1}=$$(case $$`+FWVar+` in (*/.claude) dirname "$$`+FWVar+`" ;; (*) [ -n "$${${1}:-}" ] && echo "$$${1}" || git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)`)
		l = r.shVarFW.ReplaceAllStringFunc(l, func(m string) string {
			sm := r.shVarFW.FindStringSubmatch(m)
			if sm[1] == `"` { // "$ROOT"/.claude/x
				return `"$` + FWVar + `"/` + sm[2]
			}
			return `"$` + FWVar + `/` + sm[2]
		})
		// In a check, a bare `sh .claude/x` runs inside a scratch repository the check built, on the
		// copy it put there: left as it is.
		if !strings.HasPrefix(dest, "checks/") {
			l = r.shBare.ReplaceAllString(l, `$1 "$$`+FWVar+`"/$2`)
		}
		lines[i] = l
	}
	out := strings.Join(lines, "\n")
	if strings.HasPrefix(s, "#!") {
		depth := strings.Count(dest, "/")
		up := "."
		if depth > 0 {
			up = strings.TrimSuffix(strings.Repeat("../", depth), "/")
		}
		pro := fmt.Sprintf(`%s=$(cd "$(dirname "$0")/%s" && pwd) # clauductor plugin: the plugin root (framework/internal/plugin)`, FWVar, up)
		nl := strings.Index(out, "\n")
		out = out[:nl+1] + pro + "\n" + out[nl+1:]
	}
	return out
}

// shellPatches are the few script edits no general rule covers, each for a reason the plugin
// layout creates. A patch whose anchor has left the template fails the build.
var shellPatches = map[string][]struct{ old, new string }{
	// A plugin skill's context line names ${CLAUDE_PLUGIN_ROOT}/..., which Claude Code substitutes
	// when it loads the skill; the check resolves it to the plugin copy it is running from.
	"checks/skills.sh": {
		{`while read -r script _; do`, `while read -r script _; do
    case $script in '${CLAUDE_PLUGIN_ROOT}'/*) script="$` + FWVar + `/${script#'${CLAUDE_PLUGIN_ROOT}'/}" ;; *) script="$ROOT/$script" ;; esac`},
		{`if [ -f "$ROOT/$script" ]; then`, `if [ -f "$script" ]; then`},
	},
	// Skills are the plugin's AND the project's (its CONFIGURE FIRST skills): both are mapped. The
	// playbook names the plugin's as /clauductor:<skill>.
	"checks/model-roles.sh": {
		{"sed -nE 's/^\\| `\\/([a-z0-9-]+)` \\|.*\\| ([a-z-]+) \\|$/\\1 \\2/p'", "sed -nE 's/^\\| `\\/(" + Name + ":)?([a-z0-9-]+)` \\|.*\\| ([a-z-]+) \\|$/\\2 \\3/p'"},
		{`for d in "$` + FWVar + `"/skills/*/; do`, `for d in "$` + FWVar + `"/skills/*/ "$ROOT"/.claude/skills/*/; do`},
		{`[ -f "$ROOT/.claude/skills/$s/SKILL.md" ] ||`, `[ -f "$` + FWVar + `/skills/$s/SKILL.md" ] || [ -f "$ROOT/.claude/skills/$s/SKILL.md" ] ||`},
	},
	// The check reads the model's own files at their template paths; in the plugin they are at the
	// plugin root, and the workflow runs its scripts through clauductor-model.
	"checks/change-process.sh": {
		{`if grep -qF -- "$2" "$ROOT/$1" 2>/dev/null; then`, `_f="$ROOT/$1"; case $1 in .claude/*) [ -e "$_f" ] || _f="$` + FWVar + `/${1#.claude/}" ;; esac
  if grep -qF -- "$2" "$_f" 2>/dev/null; then`},
		{`'sh .claude/verify-change.sh'`, `'clauductor-model verify-change.sh'`},
	},
	// session-close's context runs the compound step from the plugin root.
	"checks/compound.sh": {
		{`grep -q 'sh .claude/compound.sh'`, `grep -qF 'sh "$` + FWVar + `"/compound.sh'`},
	},
	// The template's status line reads the config of its own checkout; the plugin's copy has no
	// checkout, so the config is the one of the repository the session is in.
	"statusline.sh": {
		{`|| git rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)`, `|| git -C "$cwd" rev-parse --show-toplevel 2>/dev/null || pwd ;; esac)`},
	},
	// The plugin's workflow spawns its agents namespaced (jsAgent above), so the reviewer contract
	// the plugin's copy holds it to is the namespaced name, exactly: never a bare or other agent.
	"checks/build-change.sh": {
		{`sent.opts.agentType === 'reviewer'`, `sent.opts.agentType === '` + Name + `:reviewer'`},
	},
}

// rootRef is ${CLAUDE_PLUGIN_ROOT} as a regexp replacement (where $ starts a group reference).
const rootRef = "$${CLAUDE_PLUGIN_ROOT}"

// componentMarkdown rewrites a skill or agent body (not its frontmatter, where Claude Code does
// not substitute ${CLAUDE_PLUGIN_ROOT}) so every framework path names the plugin's copy, and
// every agent its namespaced name. A context line (!`sh ...`) runs at load, where Claude Code
// substitutes ${CLAUDE_PLUGIN_ROOT}; a command Claude runs itself goes through clauductor-model
// (the plugin's bin/, on the Bash tool's PATH), one stable name the project's allow rules match.
func (r *rewriter) componentMarkdown(dest, s string) string {
	front, body := splitFrontmatter(s)
	// Claude Code does not substitute in frontmatter: a path there is named plugin-relative.
	front = r.mdFW.ReplaceAllString(front, "$1")
	body = r.mdCtx.ReplaceAllString(body, "!`sh "+rootRef+"/$1")
	body = r.mdSh.ReplaceAllString(body, "clauductor-model $1")
	body = r.mdFW.ReplaceAllString(body, rootRef+"/$1")
	body = r.mdAgent.ReplaceAllString(body, "`"+Name+":$1`$2")
	body = r.skills(body)
	if dest == "skills/start-project/SKILL.md" {
		body = startProjectNote + body
	}
	return front + body
}

const startProjectNote = `
> **Installed as the clauductor plugin.** If ` + "`.claude/project.conf`" + ` is missing, run
> ` + "`/clauductor:init`" + ` first: it scaffolds this repository's own files. The scripts, hooks, skills and
> agents named below are the plugin's (` + "`${CLAUDE_PLUGIN_ROOT}`" + `, read-only); the files under ` + "`.claude/`" + `
> that this setup edits (project.conf, model-roles.json, settings.json) are the repository's.
`

func splitFrontmatter(s string) (string, string) {
	if !strings.HasPrefix(s, "---\n") {
		return "", s
	}
	end := strings.Index(s[4:], "\n---\n")
	if end < 0 {
		return "", s
	}
	cut := 4 + end + len("\n---\n")
	return s[:cut], s[cut:]
}

// scaffoldFile adapts a project-owned file for a repository whose model comes from the plugin.
// Prose keeps its meaning with the framework paths named as the plugin's; a runnable
// `sh .claude/x` becomes `clauductor-model x`, the command the plugin puts on the Bash tool's
// PATH. The gate's steps call the process checks through scripts/ci/clauductor-model.sh, which
// finds the plugin outside a session too.
func (r *rewriter) scaffoldFile(rel string, data []byte) []byte {
	s := string(data)
	if rel == "docs/playbook.md" {
		// The playbook's skill table lists every skill with its role (checks/model-roles.sh).
		s = r.pbStart.ReplaceAllString(s, "| `/init` | scaffold this repository's own files from the plugin (`/clauductor:init`) | $1 |\n$0")
	}
	switch {
	case strings.HasSuffix(rel, ".md"):
		front, body := splitFrontmatter(s)
		body = r.mdSh.ReplaceAllString(body, "clauductor-model $1")
		body = r.mdFW.ReplaceAllString(body, rootRef+"/$1")
		body = r.skills(body)
		if rel == "AGENTS.md" || rel == "docs/playbook.md" {
			body = dropDroppedHookRows(body)
		}
		return []byte(front + body)
	case rel == ".clauductor/panel.json":
		// The panel runs these outside any Claude Code session: through the resolver.
		s = r.panelArg.ReplaceAllString(s, `"sh", "scripts/ci/clauductor-model.sh", "$1`)
		s = r.skills(s)
		return []byte(r.scaffSh.ReplaceAllString(s, "sh scripts/ci/clauductor-model.sh $1"))
	case rel == ".claude/model-roles.json":
		// The plugin's init skill is a skill like any other: mapped to start-project's role.
		return []byte(r.initRole.ReplaceAllString(s, "${1}\"init\": \"${2}\",\n${1}\"start-project\": \"${2}\""))
	case strings.HasPrefix(rel, "scripts/ci/") && strings.HasSuffix(rel, ".sh"):
		lines := strings.Split(s, "\n")
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "#") {
				continue
			}
			lines[i] = r.scaffSh.ReplaceAllString(l, "sh scripts/ci/clauductor-model.sh $1")
		}
		return []byte(strings.Join(lines, "\n"))
	}
	return data
}

// skills names each plugin skill by its command in the plugin: /session-start becomes
// /clauductor:session-start. The project's own skills keep their names.
func (r *rewriter) skills(s string) string {
	for i := 0; i < 2; i++ { // adjacent names share a delimiter: a second pass takes the rest
		s = r.skillRef.ReplaceAllString(s, "$1/"+Name+":$2$3")
	}
	return s
}

// dropDroppedHookRows removes the table rows for hooks the plugin does not register
// (droppedHooks): a rule that nothing executes must not read as one that something does.
func dropDroppedHookRows(s string) string {
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, l := range lines {
		drop := false
		if strings.HasPrefix(l, "|") {
			for h := range droppedHooks {
				if strings.Contains(l, strings.TrimSuffix(h, ".sh")) {
					drop = true
				}
			}
		}
		if !drop {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
