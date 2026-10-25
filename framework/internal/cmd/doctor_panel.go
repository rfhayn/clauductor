package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
)

// checkPanel adds the panel's lines on macOS, the only system the panel runs on: its state on this
// machine under Machine, and, in a repository, whether it serves, loads and trusts the project
// under Project. It reads files and asks a live panel's /healthz, and writes nothing (D3).
// repo is nil outside a repository.
func checkPanel(rep *doctorReport, repo *doctorRepo) {
	if doctorGOOS != "darwin" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		rep.add(groupMachine, doctorLine{level: levelNote, subject: "panel", detail: "could not be checked: no home directory: " + err.Error()})
		return
	}
	reg, regErr := loadRegistry(rep, home)
	_, plistErr := os.Lstat(install.PlistPath(home))
	agent := plistErr == nil

	// reg is nil when projects.json does not load: nothing below is built from a registry that
	// was not read, so what only the registry knows is not said (installed without an agent,
	// what the default project is).
	if regErr == nil && !agent && len(reg.Projects) == 0 {
		rep.add(groupMachine, notInstalled(home, repo))
		return
	}
	rep.add(groupMachine, installedLine(home, agent, reg))
	checkRunning(rep, home, agent, reg)
	checkBuild(rep, home, reg)

	if repo == nil {
		return
	}
	switch {
	case regErr != nil:
		rep.add(groupProject, doctorLine{level: levelNote, subject: "serves",
			detail: "the panel's project lines did not run: the registry does not load (see the registry line)"})
	case repo.main == "":
		rep.add(groupProject, doctorLine{level: levelNote, subject: "serves", detail: repo.mainNote})
	default:
		checkServed(rep, home, repo, reg)
	}
}

// loadRegistry is the registry line: projects.json, when there is one. A registry that does not
// load fails (the panel cannot start) and is returned as nil, so no caller reads an empty stand-in
// as if it were the registry.
func loadRegistry(rep *doctorReport, home string) (*config.Projects, error) {
	reg, err := config.LoadProjects(home)
	if err != nil {
		rep.add(groupMachine, doctorLine{level: levelFail, subject: "registry", detail: err.Error()})
		return nil, err
	}
	if _, serr := os.Stat(config.ProjectsPath(home)); serr == nil {
		rep.add(groupMachine, doctorLine{level: levelOK, subject: "registry", detail: fmt.Sprintf("%s (%s)", config.ProjectsPath(home), plural(len(reg.Projects), "project", "projects"))})
	}
	return reg, nil
}

// panelFix is a panel command that names the project by its main checkout: panel add refuses a
// linked worktree, panel trust without --project acts on the worktree, and panel install without
// it refuses an empty registry (D8). No panel command takes --template-dir.
func panelFix(sub, main string) string {
	return "clauductor panel " + sub + " --project " + install.ShellWord(main)
}

// addFixes is the fix for a project the panel does not serve, picked by asking AddProject's own
// checks (install.CheckAdd) whether `panel <verb> --project` would register it (D8): the command
// when they pass; `panel init` before it when the only thing missing is the config; otherwise no
// fix, and why, since the command would be refused.
func addFixes(home, main, verb string) (fixes []string, refused string) {
	_, err := install.CheckAdd(context.Background(), install.AddOptions{Home: home, Project: main, Strict: true})
	if err == nil {
		return []string{panelFix(verb, main)}, ""
	}
	// The refusal is the missing config, and not another one that comes first, when it is the
	// loader's own error for the file the registry would read.
	cfgPath := config.OwnConfigPath(main)
	if _, _, lerr := config.LoadConfigRaw(cfgPath); lerr != nil && lerr.Error() == err.Error() && errors.Is(statErr(cfgPath), os.ErrNotExist) {
		return []string{panelFix("init", main), panelFix(verb, main)}, ""
	}
	return nil, refusesHere("clauductor panel "+verb, err)
}

func statErr(path string) error {
	_, err := os.Stat(path)
	return err
}

// notInstalled is the panel line when there is no login agent and no registered project: a note,
// since the panel is optional.
func notInstalled(home string, repo *doctorRepo) doctorLine {
	line := doctorLine{level: levelNote, subject: "panel", detail: "not installed (the panel is optional)"}
	switch {
	case repo == nil:
		line.detail += "; run `clauductor panel install --project` with a repository's path to install it"
	case repo.main == "":
		line.detail += "; " + repo.mainNote
	default:
		fixes, refused := addFixes(home, repo.main, "install")
		line.fixes = fixes
		if refused != "" {
			line.detail += "; " + refused
		}
	}
	return line
}

// installedLine says what installed the panel: the login agent, or only registered projects.
func installedLine(home string, agent bool, reg *config.Projects) doctorLine {
	if agent {
		return doctorLine{level: levelOK, subject: "panel", detail: "login agent installed (" + install.PlistPath(home) + ")"}
	}
	if reg == nil {
		return doctorLine{level: levelNote, subject: "panel", detail: "no login agent, and whether projects are registered is not known: the registry does not load (see the registry line)"}
	}
	return doctorLine{level: levelOK, subject: "panel", detail: "no login agent; " + plural(len(reg.Projects), "project is", "projects are") + " registered, and the panel runs when you start it"}
}

// checkRunning is the running line: a live panel's pid and port; else a warn when the login agent
// should be running it, and a note when the user starts the panel by hand.
func checkRunning(rep *doctorReport, home string, agent bool, reg *config.Projects) {
	// self 0: doctor is not a panel, so no pid is excluded.
	if o := install.RunningPanel(context.Background(), home, 0, doctorLiveProc); o != nil {
		rep.add(groupMachine, doctorLine{level: levelOK, subject: "running", detail: fmt.Sprintf("the panel is running (pid %d, port %d)", o.PID, o.Port)})
		return
	}
	if agent {
		rep.add(groupMachine, doctorLine{level: levelWarn, subject: "running",
			detail: "the login agent is installed and the panel is not running; its log is " + install.LogDir(home) + "/panel.err.log",
			fixes:  []string{"launchctl kickstart -k gui/$(id -u)/" + install.LaunchdLabel}})
		return
	}
	if reg == nil {
		return // the panel cannot start from a registry that does not load; the registry line says so
	}
	line := doctorLine{level: levelNote, subject: "running", detail: "the panel is not running, and there is no login agent: start it with clauductor panel"}
	// panel install without --project installs the registry's default project, so it is printed
	// only when that project's config loads (D8).
	if d := reg.DefaultEntry(); d != nil {
		if _, err := config.LoadConfig(d.ConfigPath()); err == nil {
			line.detail += ", or run it at login with the fix below"
			line.fixes = []string{"clauductor panel install"}
		}
	}
	rep.add(groupMachine, line)
}

// shortRev is a revision's first eight characters.
func shortRev(rev string) string {
	if len(rev) > 8 {
		return rev[:8]
	}
	return rev
}

// checkBuild compares the build of the binary the login agent runs with this one. Another build
// is a note, not a warning: doctor cannot tell which is newer (D2).
func checkBuild(rep *doctorReport, home string, reg *config.Projects) {
	bin := install.InstalledBinary(home)
	if _, err := os.Stat(bin); err != nil {
		return
	}
	panel, err := doctorFileStamp(bin)
	if err != nil {
		rep.add(groupMachine, doctorLine{level: levelNote, subject: "build", detail: fmt.Sprintf("could not read the panel binary's build (%s): %v", bin, err)})
		return
	}
	self := doctorRunningStamp()
	if panel.Revision == "" || self.Revision == "" {
		rep.add(groupMachine, doctorLine{level: levelNote, subject: "build", detail: "cannot compare the panel's build with this clauductor's: a binary has no revision recorded (" + bin + ")"})
		return
	}
	if panel.Module == self.Module && panel.Revision == self.Revision && panel.Modified == self.Modified {
		rep.add(groupMachine, doctorLine{level: levelOK, subject: "build", detail: "the panel runs this build (" + shortRev(self.Revision) + ")"})
		return
	}
	exe, _ := os.Executable()
	named := func(rev string, modified bool) string {
		if modified {
			rev += " (modified)"
		}
		return rev
	}
	detail := fmt.Sprintf("the panel runs revision %s (%s); this clauductor is %s (%s)",
		named(shortRev(panel.Revision), panel.Modified), bin, named(shortRev(self.Revision), self.Modified), exe)
	// Which is newer is not known here. The panel install would put this binary in the panel
	// and rotate the page's token, so it is named only as the way to do that when this one is
	// newer, and only when the registry's default project would let it run (D8).
	if reg == nil {
		// the registry does not load, so panel install would refuse
	} else if d := reg.DefaultEntry(); d != nil {
		if _, err := config.LoadConfig(d.ConfigPath()); err == nil {
			detail += "; when this one is the newer, `clauductor panel install` copies it over the panel's (and rotates the page's token)"
		}
	}
	rep.add(groupMachine, doctorLine{level: levelNote, subject: "build", detail: detail})
}

// checkServed is the project's panel lines, read at the main checkout (D6): whether the registry
// holds it and, for a project it serves, whether its config loads and is trusted as it is now.
// The panel reads neither for a project it does not serve, so neither is checked then.
func checkServed(rep *doctorReport, home string, repo *doctorRepo, reg *config.Projects) {
	e := reg.Find(repo.main)
	if e == nil {
		line := doctorLine{level: levelNote, subject: "serves", detail: "the panel does not serve this project (" + repo.main + ")"}
		fixes, refused := addFixes(home, repo.main, "add")
		line.fixes = fixes
		if refused != "" {
			line.detail += "; " + refused
		}
		rep.add(groupProject, line)
		return
	}
	rep.add(groupProject, doctorLine{level: levelOK, subject: "serves", detail: fmt.Sprintf("registered as %q", e.ID)})

	cfgPath, err := install.ConfigPathOf(home, repo.main, "")
	if err != nil {
		rep.add(groupProject, doctorLine{level: levelFail, subject: "config", detail: err.Error()})
		return
	}
	_, raw, err := config.LoadConfigRaw(cfgPath)
	switch {
	case err != nil && errors.Is(statErr(cfgPath), os.ErrNotExist):
		line := doctorLine{level: levelNote, subject: "config", detail: err.Error()}
		// panel init writes the project's own panel.json; it clears this only where the
		// registry reads that file and not a sidecar (D8).
		if cfgPath == config.OwnConfigPath(repo.main) {
			line.fixes = []string{panelFix("init", repo.main)}
		}
		rep.add(groupProject, line)
		return
	case err != nil:
		rep.add(groupProject, doctorLine{level: levelFail, subject: "config",
			detail: err.Error() + "\nthe panel will not start this project (docs/panel.md, The panel will not start)"})
		return
	}
	rep.add(groupProject, doctorLine{level: levelOK, subject: "config", detail: cfgPath})
	if install.TrustedNow(home, repo.main, cfgPath, install.ConfigHash(raw)) {
		rep.add(groupProject, doctorLine{level: levelOK, subject: "trust", detail: "the config is trusted as it is now"})
		return
	}
	rep.add(groupProject, doctorLine{level: levelWarn, subject: "trust",
		detail: "the config is not trusted as it is now: cards, Run and templates do nothing until you review it and trust it",
		fixes:  []string{panelFix("trust", repo.main)}})
}
