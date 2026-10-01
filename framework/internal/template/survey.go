package template

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Survey compares a repository with the template, file by file, without changing anything:
// what `clauductor diff` prints, and what a project converging on the template (StandingT) works
// down. It reads a repository that runs its own model as readily as one clauductor installed.

// FileStatus is one file's comparison.
type FileStatus struct {
	Path     string `json:"path"`                // template path ("" for an extra file)
	Dest     string `json:"dest,omitempty"`      // the project's path, when project.conf moves it
	Tier     string `json:"tier"`                // framework | doc | config | settings
	Status   string `json:"status"`              // see below
	MappedBy string `json:"mapped_by,omitempty"` // the project.conf key that moved it
	Note     string `json:"note,omitempty"`
}

// Statuses: framework files are identical, differs, missing or extra (in a framework directory,
// not in the template) or skipped (the project's config says it runs its own); doc and config
// files are present or missing; settings.json is missing, identical or merge.

// SettingsReport is the key-level merge preview.
type SettingsReport struct {
	Status    string           `json:"status"` // missing | identical | merge | invalid
	Changes   []SettingsChange `json:"changes,omitempty"`
	Conflicts int              `json:"conflicts"`
	Diff      string           `json:"diff,omitempty"` // unified diff of the file a merge writes
	Error     string           `json:"error,omitempty"`
}

// Hint is a path the project's config moves.
type Hint struct {
	Path string `json:"path"`
	Dest string `json:"dest,omitempty"`
	Key  string `json:"key"`
	Skip string `json:"skip,omitempty"`
}

// Report is a whole survey.
type Report struct {
	Repo     string          `json:"repo"`
	Template string          `json:"template"`
	Files    []FileStatus    `json:"files"`
	Settings *SettingsReport `json:"settings"`
	Mapping  []Hint          `json:"mapping,omitempty"`
	Warnings []string        `json:"warnings,omitempty"`
	Summary  map[string]int  `json:"summary"` // "<tier>.<status>" → count
}

// Converged says whether nothing of the framework differs from the template: every framework
// file is identical (or deliberately skipped), and a merge would not change settings.json.
func (r *Report) Converged() bool {
	for _, f := range r.Files {
		if f.Tier == "framework" && (f.Status == "differs" || f.Status == "missing") {
			return false
		}
	}
	return r.Settings == nil || r.Settings.Status == "identical"
}

// Survey compares targetDir with the template. pathPrefix, when set, keeps only the files whose
// template or project path starts with it.
func Survey(targetDir, pathPrefix string) (*Report, error) {
	tmplPath, err := TemplatePath()
	if err != nil {
		return nil, err
	}
	files, err := listDir(tmplPath)
	if err != nil {
		return nil, err
	}
	r := &Report{Repo: targetDir, Template: tmplPath, Summary: map[string]int{}}
	conf, err := LoadConf(targetDir, tmplPath)
	if err != nil {
		return nil, err
	}
	pm, err := conf.Mapping(files)
	if err != nil {
		// A diff only reads: compare at the template's paths, and say why.
		r.Warnings = append(r.Warnings, "path mapping: "+err.Error()+"; comparing at the template's paths")
		pm = nil
	}
	for _, rel := range pm.Moves() {
		dest, key, skip := pm.Resolve(rel)
		r.Mapping = append(r.Mapping, Hint{Path: rel, Dest: dest, Key: key, Skip: skip})
	}
	keep := func(paths ...string) bool {
		if pathPrefix == "" {
			return true
		}
		for _, p := range paths {
			if p != "" && strings.HasPrefix(p, pathPrefix) {
				return true
			}
		}
		return false
	}
	dests := map[string]bool{}
	for _, rel := range files {
		tier := Classify(rel)
		dest, key, skip := pm.Resolve(rel)
		if dest != "" {
			dests[dest] = true
		}
		if tier == TierSettings || !keep(rel, dest) {
			continue
		}
		fsx := FileStatus{Path: rel, Tier: TierLabel(tier), MappedBy: key}
		if dest != rel {
			fsx.Dest = dest
		}
		src := filepath.Join(tmplPath, filepath.FromSlash(rel))
		dst := filepath.Join(targetDir, filepath.FromSlash(dest))
		_, statErr := os.Stat(dst)
		exists := skip == "" && statErr == nil
		switch {
		case skip != "":
			fsx.Status, fsx.Note = "skipped", skip
		case tier == TierFramework && !exists:
			fsx.Status = "missing"
		case tier == TierFramework && FilesEqual(src, dst):
			fsx.Status = "identical"
		case tier == TierFramework:
			fsx.Status = "differs"
		case !exists:
			fsx.Status = "missing"
		default:
			fsx.Status = "present"
			fsx.Note = configNote(rel, src, dst)
		}
		r.Files = append(r.Files, fsx)
	}
	// Files the project added inside the framework's directories: its own skills and hooks, or
	// leftovers of an older template.
	for _, d := range FrameworkDirs {
		root := filepath.Join(targetDir, filepath.FromSlash(d))
		_ = filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
			if err != nil || de.IsDir() || de.Name() == ".DS_Store" {
				return nil
			}
			rel, _ := filepath.Rel(targetDir, p)
			rel = filepath.ToSlash(rel)
			if dests[rel] || !keep(rel) {
				return nil
			}
			r.Files = append(r.Files, FileStatus{Dest: rel, Tier: "framework", Status: "extra"})
			return nil
		})
	}
	if keep(SettingsPath) {
		r.Settings = surveySettings(targetDir, tmplPath, conf)
	}
	r.Warnings = append(r.Warnings, SkillCollisions(targetDir, files)...)
	for _, f := range r.Files {
		r.Summary[f.Tier+"."+f.Status]++
	}
	return r, nil
}

func surveySettings(targetDir, tmplPath string, conf *Conf) *SettingsReport {
	tmpl, err := os.ReadFile(filepath.Join(tmplPath, filepath.FromSlash(SettingsPath)))
	if err != nil {
		return &SettingsReport{Status: "invalid", Error: err.Error()}
	}
	cur, err := os.ReadFile(filepath.Join(targetDir, filepath.FromSlash(SettingsPath)))
	exists := err == nil
	plan, err := PlanSettings(cur, exists, tmpl, conf)
	if err != nil {
		return &SettingsReport{Status: "invalid", Error: err.Error()}
	}
	sr := &SettingsReport{Changes: plan.Changes, Conflicts: plan.Conflicts()}
	switch {
	case !exists:
		sr.Status = "missing"
	case plan.Changed():
		sr.Status = "merge"
		sr.Diff = UnifiedDiff(string(plan.Current), string(plan.Result), "project/"+SettingsPath, "merged/"+SettingsPath)
	default:
		sr.Status = "identical"
	}
	return sr
}

// configNote says what a merge would add to CLAUDE.md or .gitignore.
func configNote(rel, src, dst string) string {
	have, err := os.ReadFile(dst)
	if err != nil {
		return ""
	}
	switch rel {
	case "CLAUDE.md":
		if !hasLine(string(have), "@AGENTS.md") {
			return "lacks the @AGENTS.md import (install appends it)"
		}
	case ".gitignore":
		want, err := os.ReadFile(src)
		if err != nil {
			return ""
		}
		var missing []string
		for _, l := range GitignoreEntries(string(want)) {
			if !hasLine(string(have), l) {
				missing = append(missing, l)
			}
		}
		if len(missing) > 0 {
			return "lacks " + strings.Join(missing, " ") + " (install appends them)"
		}
	}
	return ""
}

func hasLine(content, line string) bool {
	for _, l := range strings.Split(content, "\n") {
		if strings.TrimSpace(l) == line {
			return true
		}
	}
	return false
}

// GitignoreEntries are the pattern lines of a .gitignore (no comments, no blanks).
func GitignoreEntries(content string) []string {
	var out []string
	for _, l := range strings.Split(content, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		out = append(out, l)
	}
	return out
}

// SkillCollisions warns when the project has OpenSpec's vendored skills (openspec-propose, ...)
// and the template ships a skill for the same step (propose, ...): both describe themselves for
// the same requests, so with both installed Claude may run either (B19).
func SkillCollisions(targetDir string, files []string) []string {
	ours := map[string]bool{}
	for _, rel := range files {
		if rest, ok := strings.CutPrefix(rel, ".claude/skills/"); ok {
			if name, _, ok := strings.Cut(rest, "/"); ok {
				ours[name] = true
			}
		}
	}
	entries, err := os.ReadDir(filepath.Join(targetDir, ".claude", "skills"))
	if err != nil {
		return nil
	}
	var pairs []string
	for _, e := range entries {
		name, ok := strings.CutPrefix(e.Name(), "openspec-")
		if !e.IsDir() || !ok || !ours[name] {
			continue
		}
		pairs = append(pairs, fmt.Sprintf(".claude/skills/%s and %s", e.Name(), name))
	}
	if len(pairs) == 0 {
		return nil
	}
	sort.Strings(pairs)
	return []string{fmt.Sprintf("skill trigger collision: the project's OpenSpec skills and the template's skills for the same steps (%s) trigger on the same requests, so with both installed Claude may run either one. Keep one set: delete the openspec-* skills when you adopt the template's (the openspec module keeps the CLI working on the same files), or leave the template's out.", strings.Join(pairs, "; "))}
}
