package template

import (
	"fmt"
	"strings"
)

// Project-owned files (doc tier) are never overwritten, which used to mean install and update
// never touched them at all: a template that added a model-roles.json field, an AGENTS.md row, or
// a whole new doc-tier file reached no existing project (OPS-8 rehearsal, finding 6), and the
// panel preset kept the template's branch prefixes whatever project.conf said (finding 5). These
// are the narrow, additive operations that are safe on a file the project owns.

// ModelRolesPath and PanelPath are the two project-owned JSON files install and update touch.
const (
	ModelRolesPath = ".claude/model-roles.json"
	PanelPath      = ".clauductor/panel.json"
	AgentsPath     = "AGENTS.md"
)

// MergeAddOnly adds to the project's JSON (have) every key the template's (want) has and it
// lacks, at any depth, and changes nothing else: an existing value, array or differing type is
// the project's. It returns the dotted paths it added; have unchanged (byte for byte) when none.
func MergeAddOnly(have, want []byte) ([]byte, []string, error) {
	h, err := parseNode(have)
	if err != nil {
		return nil, nil, fmt.Errorf("the project's file is not valid JSON: %w", err)
	}
	w, err := parseNode(want)
	if err != nil {
		return nil, nil, fmt.Errorf("the template's file: %w", err)
	}
	if !h.obj || !w.obj {
		return have, nil, nil
	}
	var added []string
	var walk func(path string, h, w *node)
	walk = func(path string, h, w *node) {
		for _, k := range w.keys {
			p := k
			if path != "" {
				p = path + "." + k
			}
			hv := h.fields[k]
			if hv == nil {
				h.set(k, w.fields[k].clone())
				added = append(added, p)
				continue
			}
			if hv.obj && w.fields[k].obj {
				walk(p, hv, w.fields[k])
			}
		}
	}
	walk("", h, w)
	if len(added) == 0 {
		return have, nil, nil
	}
	return h.prettyCompact(), added, nil
}

// branchKeys pairs each branch key with its template default, read from conf.sh.
var branchKeys = []string{"BRANCH_CHANGE", "BRANCH_FIX", "BRANCH_OPS", "MAIN_BRANCH"}

// RenderPanel is the template's panel preset for a project: its lane prefixes and each template's
// branch_pattern at the project's BRANCH_* keys, and the main lane at MAIN_BRANCH. Unchanged
// (byte for byte) when the project keeps the defaults.
func RenderPanel(tmpl []byte, c *Conf) ([]byte, error) {
	if c == nil {
		return tmpl, nil
	}
	repl := map[string]string{}
	for _, k := range branchKeys {
		if c.Moved(k) && c.Get(k) != "" {
			repl[c.Defaults[k]] = c.Get(k)
		}
	}
	if len(repl) == 0 {
		return tmpl, nil
	}
	n, err := parseNode(tmpl)
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", PanelPath, err)
	}
	if lanes := n.get("lanes"); lanes != nil && lanes.obj {
		out := &node{obj: true, fields: map[string]*node{}}
		for _, k := range lanes.keys {
			nk := k
			if r, ok := repl[k]; ok {
				nk = r
			}
			out.set(nk, lanes.fields[k])
		}
		n.set("lanes", out)
	}
	if ts := n.get("templates"); ts != nil && ts.arr {
		for _, t := range ts.items {
			bp, ok := t.get("branch_pattern").str()
			if !ok {
				continue
			}
			for _, k := range branchKeys[:3] {
				if d := c.Defaults[k]; strings.HasPrefix(bp, d) && repl[d] != "" {
					t.set("branch_pattern", strNode(repl[d]+strings.TrimPrefix(bp, d)))
					break
				}
			}
		}
	}
	return n.prettyCompact(), nil
}

// PanelBranchProblems lists where a project's panel.json disagrees with its branch keys: a
// configured prefix with no lane, a lane on a default prefix the project does not use, and a
// template whose branch_pattern starts with no configured prefix.
func PanelBranchProblems(panel []byte, c *Conf) []string {
	n, err := parseNode(panel)
	if err != nil || !n.obj {
		return []string{PanelPath + " is not a JSON object"}
	}
	var probs []string
	prefixes := map[string]string{}
	for _, k := range branchKeys[:3] {
		prefixes[c.Get(k)] = k
	}
	lanes := n.get("lanes")
	for _, k := range branchKeys[:3] {
		if lanes.get(c.Get(k)) == nil {
			probs = append(probs, fmt.Sprintf("lanes has no %q (%s in .claude/project.conf)", c.Get(k), k))
		}
	}
	if lanes != nil && lanes.obj {
		for _, k := range branchKeys[:3] {
			if d := c.Defaults[k]; lanes.get(d) != nil && prefixes[d] == "" {
				probs = append(probs, fmt.Sprintf("lanes has %q, the template's %s, but the project sets %s=%q", d, k, k, c.Get(k)))
			}
		}
	}
	if ts := n.get("templates"); ts != nil && ts.arr {
		for _, t := range ts.items {
			bp, ok := t.get("branch_pattern").str()
			if !ok || !strings.Contains(bp, "/") {
				continue
			}
			found := false
			for p := range prefixes {
				if p != "" && strings.HasPrefix(bp, p) {
					found = true
				}
			}
			if !found {
				id, _ := t.get("id").str()
				probs = append(probs, fmt.Sprintf("template %q has branch_pattern %q, which starts with none of BRANCH_CHANGE, BRANCH_FIX, BRANCH_OPS", id, bp))
			}
		}
	}
	return probs
}

// agentsRows are the rows of AGENTS.md's "What executes each rule" table, by first cell.
func agentsRows(md string) (keys []string, rows map[string]string) {
	rows = map[string]string{}
	in := false
	for _, l := range strings.Split(md, "\n") {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "#") {
			in = strings.Contains(t, "What executes each rule")
			continue
		}
		if !in || !strings.HasPrefix(t, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(t, "|"), "|")
		k := strings.TrimSpace(cells[0])
		if k == "" || strings.HasPrefix(k, "---") || k == "Rule / convention" {
			continue
		}
		if _, dup := rows[k]; !dup {
			keys = append(keys, k)
		}
		rows[k] = t
	}
	return keys, rows
}

// MissingAgentsRows lists the template's AGENTS.md table rows whose rule (first cell) the
// project's AGENTS.md does not have: a suggestion to review, never applied (the file is the
// project's and holds to a byte budget).
func MissingAgentsRows(project, tmpl string) []string {
	_, have := agentsRows(project)
	keys, want := agentsRows(tmpl)
	var out []string
	for _, k := range keys {
		if _, ok := have[k]; !ok {
			out = append(out, want[k])
		}
	}
	return out
}

// prettyCompact indents like pretty, but writes an object or array whose members are all
// scalars (or arrays of scalars) on one line when it fits, the way the template's hand-written
// JSON files are laid out: a merge then reads like the project's file, not a reformat of it.
func (n *node) prettyCompact() []byte {
	var b strings.Builder
	n.writeCompact(&b, "")
	b.WriteString("\n")
	return []byte(b.String())
}

func (n *node) flat() bool {
	if !n.obj && !n.arr {
		return true
	}
	for _, k := range n.keys {
		c := n.fields[k]
		if c.obj || (c.arr && !c.scalarArray()) {
			return false
		}
	}
	for _, it := range n.items {
		if it.obj || it.arr {
			return false
		}
	}
	return true
}

func (n *node) scalarArray() bool {
	for _, it := range n.items {
		if it.obj || it.arr {
			return false
		}
	}
	return true
}

func (n *node) oneLine() string {
	var b strings.Builder
	switch {
	case n.obj:
		if len(n.keys) == 0 {
			return "{}"
		}
		b.WriteString("{ ")
		for i, k := range n.keys {
			kb, _ := marshalNoEscape(k)
			b.WriteString(string(kb) + ": " + n.fields[k].oneLine())
			if i < len(n.keys)-1 {
				b.WriteString(", ")
			}
		}
		b.WriteString(" }")
	case n.arr:
		b.WriteString("[")
		for i, it := range n.items {
			b.WriteString(it.oneLine())
			if i < len(n.items)-1 {
				b.WriteString(", ")
			}
		}
		b.WriteString("]")
	default:
		b.Write(n.raw)
	}
	return b.String()
}

const compactWidth = 120

func (n *node) writeCompact(b *strings.Builder, prefix string) {
	const indent = "  "
	switch {
	case n.obj && len(n.keys) > 0:
		b.WriteString("{\n")
		for i, k := range n.keys {
			kb, _ := marshalNoEscape(k)
			head := prefix + indent + string(kb) + ": "
			b.WriteString(head)
			c := n.fields[k]
			if (c.obj || c.arr) && c.flat() && len(head)+len(c.oneLine()) <= compactWidth {
				b.WriteString(c.oneLine())
			} else {
				c.writeCompact(b, prefix+indent)
			}
			if i < len(n.keys)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(prefix + "}")
	case n.arr && len(n.items) > 0:
		b.WriteString("[\n")
		for i, it := range n.items {
			b.WriteString(prefix + indent)
			if (it.obj || it.arr) && it.flat() && len(prefix)+2+len(it.oneLine()) <= compactWidth {
				b.WriteString(it.oneLine())
			} else {
				it.writeCompact(b, prefix+indent)
			}
			if i < len(n.items)-1 {
				b.WriteString(",")
			}
			b.WriteString("\n")
		}
		b.WriteString(prefix + "]")
	default:
		b.WriteString(n.oneLine())
	}
}
