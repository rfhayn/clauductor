package template

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A project's .claude/settings.json holds two owners' keys in one file. The operating model
// registers its hooks there, sets the status line, and ships a deny list and sandbox entries; the
// project sets its model, effort, env, skill overrides, plugins, and its own permissions and
// hooks. Copying the template's file over it (what install and update did, B1) dropped every
// project key. MergeSettings brings the model's keys up to date and keeps the project's, and
// reports every place the two disagree instead of choosing silently.

// SettingsChange is one line of a merge report.
type SettingsChange struct {
	Path   string `json:"path"`           // dotted key path, e.g. permissions.deny
	Action string `json:"action"`         // add | remove | replace | keep
	Have   string `json:"have,omitempty"` // the project's value (compact JSON)
	Want   string `json:"want,omitempty"` // the template's value (compact JSON)
	Note   string `json:"note,omitempty"`
}

// Actions:
//
//	add      the template's key or list entry is missing in the project: added
//	remove   a list entry the template used to ship (at its default path) and no longer wants
//	replace  a framework-owned value differs: the template's replaces the project's (a conflict)
//	keep     a project-owned value differs from the template's: the project's is kept (a conflict)

// frameworkScalars are the settings whose value the model owns outright: the template's value
// wins, and a project value that differs is reported as replaced.
var frameworkScalars = map[string]bool{
	"statusLine":      true,
	"sandbox.enabled": true, // checks/settings.sh fails without it; loosen per tool instead
}

// hookScriptRe finds the model's hook script a registration runs, whatever form the command
// takes (the old cwd-relative `sh .claude/hooks/x.sh` or the `"$CLAUDE_PROJECT_DIR"` one).
var hookScriptRe = regexp.MustCompile(`\.claude/hooks/([A-Za-z0-9_.-]+\.sh)`)

// RenderSettings is the template's settings.json for a project: the gate's paths in the allow
// list and the sandbox exclusions are the project's GATE_RUN and GATE (B2), not the template's
// defaults. It also returns the entries the template ships at the default paths that this project
// does not use, so a merge drops them from a settings.json an earlier install wrote.
func RenderSettings(tmpl []byte, c *Conf) (rendered []byte, stale map[string][]string, err error) {
	n, err := parseNode(tmpl)
	if err != nil {
		return nil, nil, fmt.Errorf("template settings.json: %w", err)
	}
	if !n.obj {
		return nil, nil, fmt.Errorf("template settings.json is not a JSON object")
	}
	var pairs []string
	for _, k := range []string{"GATE_RUN", "GATE"} {
		if c != nil && c.Moved(k) {
			if v, ok := cleanRel(c.Get(k)); ok {
				pairs = append(pairs, c.Defaults[k], v)
			}
		}
	}
	stale = map[string][]string{}
	if len(pairs) > 0 {
		r := strings.NewReplacer(pairs...)
		for _, p := range [][]string{{"permissions", "allow"}, {"sandbox", "excludedCommands"}} {
			arr := n.get(p[0]).get(p[1])
			if arr == nil || !arr.arr {
				continue
			}
			for i, it := range arr.items {
				s, ok := it.str()
				if !ok {
					continue
				}
				if ns := r.Replace(s); ns != s {
					stale[strings.Join(p, ".")] = append(stale[strings.Join(p, ".")], it.compact())
					arr.items[i] = strNode(ns)
				}
			}
		}
	}
	return n.pretty(), stale, nil
}

// MergeSettings merges the rendered template settings (want) into the project's (have). stale
// is RenderSettings' list of entries to drop. The result keeps the project's key order, with the
// template's new keys after.
func MergeSettings(have, want []byte, stale map[string][]string) (merged []byte, changes []SettingsChange, err error) {
	return mergeSettings(have, want, stale, nil)
}

// HookPrune says which of the project's own hook registrations a merge drops: those that run a
// script under .claude/hooks/ that will not exist after the install (not in the project, not in
// the template), and every registration of the old model's hooks (session-register.sh,
// heartbeat.sh, ...). A merge that kept them left every session start and tool call running a
// deleted script (OPS-8 rehearsal, finding 7).
type HookPrune struct {
	ProjectDir    string          // the project; a script there counts as existing
	TemplateHooks map[string]bool // hook scripts the template ships (installed with the merge)
}

// why returns the reason to drop a registration of script, or "".
func (p *HookPrune) why(script string) string {
	if p == nil || script == "" {
		return ""
	}
	if OldModelHook(script) {
		return "the old model's hook (" + script + "), which the current model replaced"
	}
	if p.TemplateHooks[script] {
		return ""
	}
	if _, err := os.Stat(filepath.Join(p.ProjectDir, ".claude", "hooks", script)); err == nil {
		return ""
	}
	return "it runs .claude/hooks/" + script + ", which does not exist"
}

// TemplateHookPrune is the HookPrune for installing tmplDir's template into projectDir.
func TemplateHookPrune(projectDir, tmplDir string) *HookPrune {
	p := &HookPrune{ProjectDir: projectDir, TemplateHooks: map[string]bool{}}
	if es, err := os.ReadDir(filepath.Join(tmplDir, ".claude", "hooks")); err == nil {
		for _, e := range es {
			if !e.IsDir() {
				p.TemplateHooks[e.Name()] = true
			}
		}
	}
	return p
}

func mergeSettings(have, want []byte, stale map[string][]string, prune *HookPrune) (merged []byte, changes []SettingsChange, err error) {
	h, err := parseNode(have)
	if err != nil {
		return nil, nil, fmt.Errorf("the project's settings.json is not valid JSON: %w", err)
	}
	w, err := parseNode(want)
	if err != nil {
		return nil, nil, fmt.Errorf("template settings.json: %w", err)
	}
	if !h.obj {
		return nil, nil, fmt.Errorf("the project's settings.json is not a JSON object")
	}
	m := &merger{stale: map[string]map[string]bool{}, prune: prune}
	for p, items := range stale {
		m.stale[p] = map[string]bool{}
		for _, it := range items {
			m.stale[p][it] = true
		}
	}
	out := h.clone()
	for _, k := range w.keys {
		wv := w.fields[k]
		if k == "hooks" {
			out.set(k, m.hooks(out.get(k), wv))
			continue
		}
		out.set(k, m.merge(k, out.get(k), wv))
	}
	if out.compact() == h.compact() {
		return have, m.changes, nil // nothing to add: leave the project's formatting alone
	}
	return out.pretty(), m.changes, nil
}

type merger struct {
	stale   map[string]map[string]bool
	prune   *HookPrune
	changes []SettingsChange
}

func (m *merger) note(c SettingsChange) { m.changes = append(m.changes, c) }

func (m *merger) merge(path string, have, want *node) *node {
	if have == nil {
		m.note(SettingsChange{Path: path, Action: "add", Want: want.compact()})
		return want.clone()
	}
	if frameworkScalars[path] {
		if have.compact() != want.compact() {
			m.note(SettingsChange{Path: path, Action: "replace", Have: have.compact(), Want: want.compact(), Note: "framework-owned: the template's value wins"})
		}
		return want.clone()
	}
	switch {
	case have.obj && want.obj:
		out := have.clone()
		for _, k := range want.keys {
			out.set(k, m.merge(path+"."+k, out.get(k), want.fields[k]))
		}
		return out
	case have.arr && want.arr:
		return m.union(path, have, want)
	case have.obj != want.obj || have.arr != want.arr:
		m.note(SettingsChange{Path: path, Action: "keep", Have: have.compact(), Want: want.compact(), Note: "the project's value has another type than the template's; kept"})
		return have
	default:
		if have.compact() != want.compact() {
			m.note(SettingsChange{Path: path, Action: "keep", Have: have.compact(), Want: want.compact(), Note: "project-owned: the project's value is kept"})
		}
		return have
	}
}

// union keeps the project's entries in order (less those the template used to ship and no longer
// wants), then appends the template's entries the project lacks.
func (m *merger) union(path string, have, want *node) *node {
	out := &node{arr: true}
	seen := map[string]bool{}
	for _, it := range have.items {
		c := it.compact()
		if m.stale[path][c] {
			m.note(SettingsChange{Path: path, Action: "remove", Have: c, Note: "the template's default path, which this project's project.conf moves"})
			continue
		}
		seen[c] = true
		out.items = append(out.items, it)
	}
	for _, it := range want.items {
		c := it.compact()
		if seen[c] {
			continue
		}
		seen[c] = true
		m.note(SettingsChange{Path: path, Action: "add", Want: c})
		out.items = append(out.items, it.clone())
	}
	return out
}

// hooks merges the model's hook registrations into the project's: each registration of a
// template hook script is the template's (its command, event and matcher); every other hook is
// the project's and is kept as it is.
func (m *merger) hooks(have, want *node) *node {
	if want == nil || !want.obj {
		return have
	}
	if have == nil || !have.obj {
		if have != nil {
			m.note(SettingsChange{Path: "hooks", Action: "replace", Have: have.compact(), Want: want.compact(), Note: "the project's hooks is not an object"})
		} else {
			m.note(SettingsChange{Path: "hooks", Action: "add", Want: want.compact()})
		}
		return want.clone()
	}
	type reg struct {
		event, matcher string
		hook           *node
		placed         bool
	}
	var wants []*reg
	scripts := map[string]bool{}
	for _, ev := range want.keys {
		groups := want.fields[ev]
		if !groups.arr {
			continue
		}
		for _, g := range groups.items {
			mt, _ := g.get("matcher").str()
			hs := g.get("hooks")
			if hs == nil || !hs.arr {
				continue
			}
			for _, hk := range hs.items {
				cmd, _ := hk.get("command").str()
				if s := hookScriptRe.FindStringSubmatch(cmd); s != nil {
					scripts[s[1]] = true
				}
				wants = append(wants, &reg{event: ev, matcher: mt, hook: hk})
			}
		}
	}
	scriptOf := func(hk *node) string {
		cmd, _ := hk.get("command").str()
		if s := hookScriptRe.FindStringSubmatch(cmd); s != nil && scripts[s[1]] {
			return s[1]
		}
		return ""
	}
	out := have.clone()
	for _, ev := range out.keys {
		groups := out.fields[ev]
		if !groups.arr {
			continue
		}
		var keptGroups []*node
		for _, g := range groups.items {
			mt, _ := g.get("matcher").str()
			hs := g.get("hooks")
			if hs == nil || !hs.arr {
				keptGroups = append(keptGroups, g)
				continue
			}
			var kept []*node
			for _, hk := range hs.items {
				s := scriptOf(hk)
				if s == "" {
					cmd, _ := hk.get("command").str()
					if sm := hookScriptRe.FindStringSubmatch(cmd); sm != nil {
						if why := m.prune.why(sm[1]); why != "" {
							m.note(SettingsChange{Path: "hooks." + ev + matcherLabel(mt), Action: "remove", Have: hk.compact(), Note: why})
							continue
						}
					}
					kept = append(kept, hk) // the project's own hook
					continue
				}
				var match *reg
				for _, r := range wants {
					if !r.placed && r.event == ev && r.matcher == mt && scriptOf(r.hook) == s {
						match = r
						break
					}
				}
				p := "hooks." + ev + matcherLabel(mt)
				if match == nil {
					m.note(SettingsChange{Path: p, Action: "remove", Have: hk.compact(), Note: "the template no longer registers " + s + " here"})
					continue
				}
				match.placed = true
				if hk.compact() != match.hook.compact() {
					m.note(SettingsChange{Path: p, Action: "replace", Have: hk.compact(), Want: match.hook.compact(), Note: "framework-owned hook registration"})
				}
				kept = append(kept, match.hook.clone())
			}
			if len(kept) == 0 {
				continue
			}
			ng := g.clone()
			ng.set("hooks", &node{arr: true, items: kept})
			keptGroups = append(keptGroups, ng)
		}
		groups.items = keptGroups
	}
	for _, r := range wants {
		if r.placed {
			continue
		}
		m.note(SettingsChange{Path: "hooks." + r.event + matcherLabel(r.matcher), Action: "add", Want: r.hook.compact()})
		groups := out.get(r.event)
		if groups == nil || !groups.arr {
			groups = &node{arr: true}
			out.set(r.event, groups)
		}
		var target *node
		for _, g := range groups.items {
			mt, _ := g.get("matcher").str()
			if hs := g.get("hooks"); mt == r.matcher && hs != nil && hs.arr {
				target = g
				break
			}
		}
		if target == nil {
			target = &node{obj: true, fields: map[string]*node{}}
			if r.matcher != "" {
				target.set("matcher", strNode(r.matcher))
			}
			target.set("hooks", &node{arr: true})
			groups.items = append(groups.items, target)
		}
		hs := target.get("hooks")
		hs.items = append(hs.items, r.hook.clone())
	}
	// An event left with no groups goes.
	var keys []string
	for _, ev := range out.keys {
		if g := out.fields[ev]; g.arr && len(g.items) == 0 {
			delete(out.fields, ev)
			continue
		}
		keys = append(keys, ev)
	}
	out.keys = keys
	return out
}

func matcherLabel(mt string) string {
	if mt == "" {
		return ""
	}
	return "[" + mt + "]"
}

// SettingsPlan is what install or update would do to a project's settings.json.
type SettingsPlan struct {
	Exists  bool             // the project has a settings.json
	Current []byte           // the project's file as it is (nil when missing)
	Result  []byte           // what it would become
	Changes []SettingsChange // the merge report (empty when the file is created)
}

// Changed says whether applying the plan writes anything.
func (p *SettingsPlan) Changed() bool { return string(p.Current) != string(p.Result) }

// Conflicts counts the report lines where the project and the template disagree.
func (p *SettingsPlan) Conflicts() int {
	n := 0
	for _, c := range p.Changes {
		if c.Action == "replace" || c.Action == "keep" {
			n++
		}
	}
	return n
}

// PlanSettings works out the settings.json a project gets: the rendered template when it has
// none, else the merge.
func PlanSettings(current []byte, exists bool, tmpl []byte, c *Conf, prune *HookPrune) (*SettingsPlan, error) {
	rendered, stale, err := RenderSettings(tmpl, c)
	if err != nil {
		return nil, err
	}
	p := &SettingsPlan{Exists: exists}
	if !exists {
		p.Result = rendered
		return p, nil
	}
	p.Current = current
	merged, changes, err := mergeSettings(current, rendered, stale, prune)
	if err != nil {
		return nil, err
	}
	p.Result, p.Changes = merged, changes
	return p, nil
}
