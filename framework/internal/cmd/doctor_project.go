package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/clauductor/clauductor/internal/brand"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/clauductor/clauductor/internal/template"
)

// doctorRepo is the repository doctor runs in, found once for the project's lines (D6): the files
// being worked on are read at the toplevel; what the panel keeps (the registry, trust) is keyed by
// the main checkout, which is read from main.
type doctorRepo struct {
	top string // git's toplevel for the current directory, as git prints it
	// below says the current directory is under top: install, update and diff act on the current
	// directory (E2), so a fix printed there starts with cd.
	below bool
	// main is the repository's main checkout as `panel add` finds it (mainRoot: the first entry
	// of git worktree list, resolved). Empty when mainNote says why there is none.
	main     string
	mainNote string
	// hasTemplate says a template resolves. Without one, install, update and diff refuse, so no
	// repository fix is printed (D8); the template line's FAIL says why.
	hasTemplate bool
	// templateFlag is " --template-dir <dir>" when doctor was given one, so a fix compares
	// against the template doctor judged (D8). It is Resolve's Source.Dir, which is absolute, so
	// it survives the cd.
	templateFlag string
}

// doctorFindRepo is the git repository of the current directory, or nil when there is none with a
// working tree (a bare repository, a directory outside git, or no git at all).
func doctorFindRepo() *doctorRepo {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}
	ctx := context.Background()
	out, err := signals.ExecRunner(ctx, cwd, []string{"git", "rev-parse", "--show-toplevel"})
	top := strings.TrimSpace(string(out))
	if err != nil || top == "" {
		return nil
	}
	r := &doctorRepo{top: top}
	r.below = signals.ResolvePath(cwd) != signals.ResolvePath(top)
	r.main, r.mainNote = mainCheckout(ctx, top)
	if src, err := template.Resolve(); err == nil {
		r.hasTemplate = true
		if template.OverrideDir != "" {
			r.templateFlag = " --template-dir " + install.ShellWord(src.Dir)
		}
	}
	return r
}

// mainCheckout is the repository's main checkout the way install's mainRoot finds it: the first
// entry of `git worktree list`, resolved. A bare first entry is not a project the panel can
// serve, which is a note and not a path.
func mainCheckout(ctx context.Context, top string) (main, note string) {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	wts, err := signals.ReadWorktrees(cctx, signals.ExecRunner, signals.ResolvePath(top))
	switch {
	case err != nil || len(wts) == 0:
		return "", "git lists no worktree for " + top
	case wts[0].Bare:
		return "", "the repository's first worktree is bare, and the panel serves only a main worktree"
	}
	return wts[0].Path, ""
}

// fixes are repository commands to paste where doctor printed them (D8): install, update and diff
// act on the current directory, so below the toplevel each starts with cd, and a --template-dir
// doctor was given is carried. Without a template every one of them refuses, so there are none.
func (r *doctorRepo) fixes(subs ...string) []string {
	if !r.hasTemplate {
		return nil
	}
	var out []string
	for _, sub := range subs {
		c := "clauductor " + sub + r.templateFlag
		if r.below {
			c = "cd " + install.ShellWord(r.top) + " && " + c
		}
		out = append(out, c)
	}
	return out
}

// refusesHere is what a line says in place of a fix whose command's own checks refuse (D8): the
// refusal itself, on one line.
func refusesHere(command string, err error) string {
	return command + " refuses here, so no fix is printed: " + strings.Join(strings.Fields(err.Error()), " ")
}

// onceTemplate is what a model line adds to its prose when it can print no fix: the command
// it names needs the template the template line says is missing.
func (r *doctorRepo) onceTemplate() string {
	if r.hasTemplate {
		return ""
	}
	return ", once the template line is ok"
}

// whoseModel is which operating model a repository runs, in the order install and update decide it
// (E3): the plugin's marker wins, then Clauductor's marker or the old model's signs, then any sign
// of a model of the repository's own.
type whoseModel int

const (
	modelNone whoseModel = iota
	modelPlugin
	modelClauductor
	modelOwn
)

func modelOf(top string) (whoseModel, string) {
	if m := markerIn(top, brand.PluginMarker, brand.Names()); m != "" {
		return modelPlugin, m
	}
	if hasInstallMarker(top) {
		return modelClauductor, "installed by clauductor install"
	}
	if signs := template.OldModelSigns(top); len(signs) > 0 {
		return modelClauductor, "Clauductor's old model: " + signs[0]
	}
	for _, s := range ownModelSigns {
		if fileExists(filepath.Join(top, s)) {
			return modelOwn, s
		}
	}
	return modelNone, ""
}

// checkProject adds the project group: the repository, whose model it runs and, for Clauductor's,
// whether it matches the template. It returns the repository for the panel's lines, nil outside
// one.
func checkProject(rep *doctorReport) *doctorRepo {
	r := doctorFindRepo()
	if r == nil {
		detail := "not in a git repository with a working tree; the project checks did not run"
		if _, err := exec.LookPath("git"); err != nil { // doctorFindRepo could not ask (#255)
			detail = "git is not on PATH, so the project checks did not run"
		}
		rep.add(groupProject, doctorLine{level: levelNote, subject: "repository", detail: detail})
		return nil
	}
	rep.add(groupProject, doctorLine{level: levelOK, subject: "repository", detail: r.top})

	model, evidence := modelOf(r.top)
	switch model {
	case modelPlugin:
		rep.add(groupProject, doctorLine{level: levelNote, subject: "model",
			detail: "runs the operating model from the clauductor plugin (" + evidence + "); it is updated with the plugin (docs/plugin.md), and install and update refuse here"})
	case modelOwn:
		// diff's RunE is template.Survey of the current directory, so Survey here decides whether
		// the printed diff runs (D8).
		line := doctorLine{level: levelNote, subject: "model",
			detail: fmt.Sprintf("runs an operating model of its own (%s, and no %s); install and update refuse it, and clauductor diff compares it with the template%s", evidence, installMarker, r.onceTemplate())}
		if r.hasTemplate {
			if _, err := template.Survey(r.top, ""); err != nil {
				line.detail += "; " + refusesHere("diff", err)
			} else {
				line.fixes = r.fixes("diff")
			}
		}
		rep.add(groupProject, line)
	case modelNone:
		// install's own checks decide whether the printed install runs (D8): a framework file
		// already there with other content, or a project.conf install cannot place, refuses it
		// though none of the signs modelOf reads is present.
		line := doctorLine{level: levelNote, subject: "model",
			detail: "has no operating model; clauductor install adopts one" + r.onceTemplate()}
		if r.hasTemplate {
			if _, err := prepareInstall(io.Discard, r.top, false); err != nil {
				line.detail = "has no operating model; clauductor install would adopt one, but " + refusesHere("install", err)
			} else {
				line.fixes = r.fixes("install")
			}
		}
		rep.add(groupProject, line)
	case modelClauductor:
		rep.add(groupProject, doctorLine{level: levelOK, subject: "model", detail: "Clauductor's operating model (" + evidence + ")"})
		checkDrift(rep, r)
		rep.checks = "sh .claude/checks/run.sh"
		if r.below {
			rep.checks = "cd " + install.ShellWord(r.top) + " && " + rep.checks
		}
	}
	return r
}

// checkDrift is the drift line: clauductor diff's verdict, read at the toplevel.
func checkDrift(rep *doctorReport, r *doctorRepo) {
	sv, err := template.Survey(r.top, "")
	if err != nil {
		rep.add(groupProject, doctorLine{level: levelNote, subject: "drift",
			detail: "could not be compared with the template (see the template line): " + err.Error()})
		return
	}
	version := "no version marker"
	templateWarning := ""
	if sv.Source != nil {
		templateWarning = sv.Source.Warning
		if sv.Source.Version != "" {
			version = "version " + sv.Source.Version
		}
	}
	differs, missing := 0, 0
	for _, f := range sv.Files {
		if f.Tier != "framework" {
			continue
		}
		switch f.Status {
		case "differs":
			differs++
		case "missing":
			missing++
		}
	}
	if sv.Converged() {
		rep.add(groupProject, doctorLine{level: levelOK, subject: "drift", detail: "matches the template (" + version + ")"})
	} else {
		var parts []string
		if differs > 0 {
			parts = append(parts, plural(differs, "framework file differs", "framework files differ"))
		}
		if missing > 0 {
			parts = append(parts, fmt.Sprintf("%d missing", missing))
		}
		if s := sv.Settings; s != nil && s.Status != "identical" {
			switch s.Status {
			case "invalid":
				parts = append(parts, template.SettingsPath+" does not parse")
			case "missing":
				parts = append(parts, template.SettingsPath+" is missing")
			default:
				parts = append(parts, template.SettingsPath+" would change")
			}
		}
		// update's own planning decides whether update runs here (D8): when it refuses, the
		// refusal is the detail and there is no update fix, so no printed fix is refused.
		line := doctorLine{level: levelWarn, subject: "drift"}
		head := fmt.Sprintf("differs from the template (%s): %s\n", version, strings.Join(parts, ", "))
		if _, err := planUpdate(io.Discard, r.top, false); err != nil {
			line.detail = head + refusesHere("update", err)
		} else {
			line.detail = head + "update --dry-run shows each change; update applies them"
			line.fixes = r.fixes("update --dry-run", "update")
		}
		rep.add(groupProject, line)
	}
	// The template line already says its own warning.
	for _, w := range sv.Warnings {
		if w != templateWarning {
			rep.add(groupProject, doctorLine{level: levelWarn, subject: "drift", detail: w})
		}
	}
}
