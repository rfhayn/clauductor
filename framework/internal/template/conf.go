package template

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Conf is a project's .claude/project.conf read over the template's defaults (.claude/lib/conf.sh),
// the same layering the model's scripts get by sourcing conf.sh. Install, update and diff read it
// to put the files a project has moved (its gate, its roadmap, its change records) where the
// project's scripts will look for them, instead of at the template's paths.
//
// Both files are shell, but a plain `KEY="value"` one in practice; a value that needs the shell
// to read (a `$` or backquote) is reported as unreadable, never guessed.
type Conf struct {
	Defaults   map[string]string // from the template's .claude/lib/conf.sh
	Project    map[string]string // from the project's .claude/project.conf (nil when it has none)
	Unreadable map[string]string // KEY → the raw text, for values that need a shell to expand
	HasProject bool
}

var assignRe = regexp.MustCompile(`^(?:export[ \t]+)?([A-Z][A-Z0-9_]*)=(.*)$`)

// parseShellAssignments reads KEY=value lines. Indented lines count too (conf.sh sets ROOT inside
// an if), but only upper-case keys, which is what the model's configuration uses.
func parseShellAssignments(data string) (vals, unreadable map[string]string) {
	vals, unreadable = map[string]string{}, map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		m := assignRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key, raw := m[1], m[2]
		var v string
		ok := true
		switch {
		case strings.HasPrefix(raw, `"`):
			end := strings.Index(raw[1:], `"`)
			if end < 0 {
				ok = false
				break
			}
			v = raw[1 : end+1]
			if strings.ContainsAny(v, "$`\\") {
				ok = false
			}
		case strings.HasPrefix(raw, "'"):
			end := strings.Index(raw[1:], "'")
			if end < 0 {
				ok = false
				break
			}
			v = raw[1 : end+1]
		default:
			v = raw
			if i := strings.IndexAny(v, " \t;#"); i >= 0 {
				v = v[:i]
			}
			if strings.ContainsAny(v, "$`\\(") {
				ok = false
			}
		}
		if !ok {
			unreadable[key] = raw
			delete(vals, key)
			continue
		}
		vals[key] = v
		delete(unreadable, key)
	}
	return vals, unreadable
}

// LoadConf reads the template's defaults and, when targetDir has one, the project's config.
func LoadConf(targetDir, tmplDir string) (*Conf, error) {
	c := &Conf{Unreadable: map[string]string{}}
	def, err := os.ReadFile(filepath.Join(tmplDir, ".claude", "lib", "conf.sh"))
	if err != nil {
		return nil, fmt.Errorf("template defaults: %w", err)
	}
	c.Defaults, _ = parseShellAssignments(string(def))
	delete(c.Defaults, "ROOT")
	if targetDir != "" {
		if data, err := os.ReadFile(filepath.Join(targetDir, ".claude", "project.conf")); err == nil {
			c.HasProject = true
			c.Project, c.Unreadable = parseShellAssignments(string(data))
		}
	}
	return c, nil
}

// Get is the effective value of key: the project's, else the template's default.
func (c *Conf) Get(key string) string {
	if v, ok := c.Project[key]; ok {
		return v
	}
	return c.Defaults[key]
}

// Moved says whether the project sets key to something other than the default.
func (c *Conf) Moved(key string) bool {
	v, ok := c.Project[key]
	return ok && v != c.Defaults[key]
}

// pathKeys map a template file (or, ending in "/", a directory) to the project.conf key that
// names where the project keeps it. A single file maps to the key's value; a directory's files
// keep their path under the key's directory.
var pathKeys = []struct{ tmpl, key string }{
	{"scripts/ci/run-local.sh", "GATE_RUN"},
	{"scripts/ci/gate.sh", "GATE"},
	{"scripts/ci/steps.sh", "GATE_STEPS"},
	{"docs/roadmap.md", "ROADMAP"},
	{"docs/development-journal.md", "JOURNAL"},
	{"docs/insights-log.md", "INSIGHTS"},
	{"docs/owner-queue.md", "OWNER_QUEUE"},
	{"docs/adr/", "ADR_DIR"},
	{"changes/", "CHANGES_DIR"},
	{"specs/", "SPECS_DIR"},
}

// PathMap says where each template file goes in a project.
type PathMap struct {
	dest map[string]string // template path → project path, only for files that move
	by   map[string]string // template path → the project.conf key that moved it
	skip map[string]string // template path → why install leaves it out
}

// Resolve returns where rel goes in the project, the key that moved it ("" when it did not
// move), and a reason when it is not installed at all.
func (m *PathMap) Resolve(rel string) (dest, key, skip string) {
	if m == nil {
		return rel, "", ""
	}
	if r, ok := m.skip[rel]; ok {
		return "", m.by[rel], r
	}
	if d, ok := m.dest[rel]; ok {
		return d, m.by[rel], ""
	}
	return rel, "", ""
}

// Moves lists the moved files in template-path order, for reports.
func (m *PathMap) Moves() []string {
	if m == nil {
		return nil
	}
	var out []string
	for k := range m.dest {
		out = append(out, k)
	}
	for k := range m.skip {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// cleanRel validates a project.conf path: relative, inside the repository.
func cleanRel(v string) (string, bool) {
	if v == "" || strings.HasPrefix(v, "/") || strings.HasPrefix(v, "~") {
		return "", false
	}
	c := path.Clean(v)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") {
		return "", false
	}
	return c, true
}

// Mapping works out where every template file in files goes. It refuses (an error naming the
// key) when the project's config makes a destination ambiguous: a path outside the repository,
// a value that needs a shell to read, or two template files landing on one path.
//
// The gate scripts follow their keys: run-local.sh to GATE_RUN and lease.sh beside it, gate.sh
// to GATE. When GATE_RUN (or GATE) names a script that is not the template's (another file
// name), the project runs a gate of its own, and install leaves that script, and lease.sh, out
// rather than writing a second gate beside it (B11).
func (c *Conf) Mapping(files []string) (*PathMap, error) {
	m := &PathMap{dest: map[string]string{}, by: map[string]string{}, skip: map[string]string{}}
	for _, pk := range pathKeys {
		if raw, bad := c.Unreadable[pk.key]; bad {
			return nil, fmt.Errorf("%s in .claude/project.conf is %s, which needs a shell to read: set it to a plain path so install knows where the project keeps %s", pk.key, raw, strings.TrimSuffix(pk.tmpl, "/"))
		}
	}
	for _, rel := range files {
		for _, pk := range pathKeys {
			isDir := strings.HasSuffix(pk.tmpl, "/")
			if (isDir && !strings.HasPrefix(rel, pk.tmpl)) || (!isDir && rel != pk.tmpl) {
				continue
			}
			if !c.Moved(pk.key) {
				break
			}
			v, ok := cleanRel(c.Get(pk.key))
			if !ok {
				return nil, fmt.Errorf("%s=%q in .claude/project.conf is not a path inside the repository; install cannot place %s", pk.key, c.Get(pk.key), rel)
			}
			if isDir {
				m.dest[rel] = v + "/" + strings.TrimPrefix(rel, pk.tmpl)
			} else {
				m.dest[rel] = v
			}
			m.by[rel] = pk.key
			break
		}
	}
	// The runner and its lease, and the agent wrapper, are the template's only when they keep
	// the template's file name.
	if c.Moved("GATE_RUN") {
		runner, _ := cleanRel(c.Get("GATE_RUN"))
		if path.Base(runner) != "run-local.sh" {
			why := fmt.Sprintf("GATE_RUN is %s, the project's own gate runner", runner)
			// The README describes the template's runner and lease, which are not installed.
			for _, f := range []string{"scripts/ci/run-local.sh", "scripts/ci/lease.sh", "scripts/ci/README.md"} {
				m.skip[f] = why
				m.by[f] = "GATE_RUN"
				delete(m.dest, f)
			}
		} else if d := path.Dir(runner); d != "scripts/ci" {
			for _, f := range []string{"lease.sh", "README.md"} {
				m.dest["scripts/ci/"+f] = d + "/" + f
				m.by["scripts/ci/"+f] = "GATE_RUN"
			}
		}
	}
	if c.Moved("GATE") {
		wrapper, _ := cleanRel(c.Get("GATE"))
		if path.Base(wrapper) != "gate.sh" {
			m.skip["scripts/ci/gate.sh"] = fmt.Sprintf("GATE is %s, the project's own gate wrapper", wrapper)
			delete(m.dest, "scripts/ci/gate.sh")
		}
	}
	// Two template files on one path would overwrite each other: refuse rather than pick one.
	landed := map[string]string{}
	for _, rel := range files {
		dest, _, skip := m.Resolve(rel)
		if skip != "" {
			continue
		}
		if other, dup := landed[dest]; dup {
			return nil, fmt.Errorf(".claude/project.conf puts both %s and %s at %s (%s, %s): give each its own path", other, rel, dest, keyOr(m.by[other]), keyOr(m.by[rel]))
		}
		landed[dest] = rel
	}
	return m, nil
}

func keyOr(k string) string {
	if k == "" {
		return "the template's path"
	}
	return k
}
