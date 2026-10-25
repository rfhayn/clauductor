package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/treesnap"
)

// panelHome is an empty home directory for a doctor run as macOS, with a template that resolves.
// The panel's lines read it, never the machine's own.
func panelHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	old := doctorGOOS
	doctorGOOS = "darwin"
	t.Cleanup(func() { doctorGOOS = old })
	tmplDir(t)
	return home
}

func writePlist(t *testing.T, home string) {
	t.Helper()
	write(t, home, "Library/LaunchAgents/"+install.LaunchdLabel+".plist", "<plist/>\n")
}

// panelJSON is a panel.json that loads; extra is more JSON members (",\"tmux_socket\":\"x\"").
func panelJSON(name, extra string) string {
	return `{"name":"` + name + `","lanes":{"main":"orchestrator"}` + extra + `}`
}

// projectWithConfig is a repository holding a panel.json.
func projectWithConfig(t *testing.T, name, extra string) string {
	t.Helper()
	dir := newRepo(t)
	write(t, dir, ".clauductor/panel.json", panelJSON(name, extra))
	return dir
}

// registerProject registers dir with the panel the way `panel add` does.
func registerProject(t *testing.T, home, dir string) config.ProjectEntry {
	t.Helper()
	res, err := install.AddProject(context.Background(), install.AddOptions{Home: home, Project: dir, Strict: true, Now: time.Now()})
	if err != nil {
		t.Fatalf("registering %s: %v", dir, err)
	}
	return res.Entry
}

// doctorIn runs doctor in dir with every tool present.
func doctorIn(t *testing.T, dir string) (string, error) {
	t.Helper()
	t.Chdir(dir)
	return doctor(t, gitPath(t, allTools...))
}

// needNoProblem fails when any line warns or fails.
func needNoProblem(t *testing.T, out string) {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "  warn ") || strings.HasPrefix(l, "  FAIL ") {
			t.Errorf("a line warns or fails: %q\n%s", l, out)
		}
	}
}

// needNoLine fails when a line holds every part.
func needNoLine(t *testing.T, out string, parts ...string) {
	t.Helper()
	if l := lineWith(out, parts...); l != "" {
		t.Errorf("unexpected line %q holding %q:\n%s", l, parts, out)
	}
}

// [DOCTOR-1-S7] No panel installed is a note naming panel install with the main checkout, and in
// a repository with no panel.json panel init comes first.
func TestDoctorNoPanelInstalledIsANoteWithTheInstallThatWorks(t *testing.T) {
	home := panelHome(t)
	dir := projectWithConfig(t, "Fine", "")
	before := treesnap.Take(t, home)

	out, err := doctorIn(t, dir)
	if err != nil {
		t.Fatalf("exit: %v\n%s", err, out)
	}
	l := needLine(t, out, "note", "panel", "not installed")
	if !strings.Contains(l, "optional") {
		t.Errorf("the note does not say the panel is optional: %q", l)
	}
	needExact(t, out, "fix: clauductor panel install --project "+install.ShellWord(dir))
	needNoLine(t, out, "panel init")
	needNoProblem(t, out)
	if d := treesnap.Diff(before, treesnap.Take(t, home)); len(d) > 0 {
		t.Errorf("doctor wrote the home directory: %v", d)
	}

	// A repository with no panel.json: init comes first, then install, in that order.
	bare := newRepo(t)
	out, err = doctorIn(t, bare)
	if err != nil {
		t.Fatalf("exit: %v\n%s", err, out)
	}
	initFix := "fix: clauductor panel init --project " + install.ShellWord(bare)
	installFix := "fix: clauductor panel install --project " + install.ShellWord(bare)
	needExact(t, out, initFix)
	needExact(t, out, installFix)
	if strings.Index(out, initFix) > strings.Index(out, installFix) {
		t.Errorf("panel init is not before panel install:\n%s", out)
	}
	needNoProblem(t, out)

	// The printed fix is what AddProject does with the project: it registers it.
	registerProject(t, home, dir)
}

// [DOCTOR-1-S7] A panel.json that does not load gets the refusal and no install fix: the command
// would be refused, so no fix is printed (D8).
func TestDoctorNotInstalledPrintsNoInstallFixWhenAddWouldRefuse(t *testing.T) {
	panelHome(t)
	dir := projectWithConfig(t, "Broken", `,"bogus":1`)
	out, _ := doctorIn(t, dir)
	l := needLine(t, out, "note", "panel", "not installed")
	if !strings.Contains(l, "refuses here") || !strings.Contains(l, "bogus") {
		t.Errorf("the note does not give the refusal: %q", l)
	}
	needNoLine(t, out, "fix: clauductor panel")
}

// Outside a repository there is no project path to name.
func TestDoctorNotInstalledOutsideARepositoryNamesTheProjectFlag(t *testing.T) {
	panelHome(t)
	out, err := doctorIn(t, t.TempDir())
	if err != nil {
		t.Fatalf("exit: %v\n%s", err, out)
	}
	needLine(t, out, "note", "panel", "not installed", "`clauductor panel install --project` with a repository's path")
	needNoLine(t, out, "fix:")
}

// A linked worktree of a repository the panel could serve names the main checkout.
func TestDoctorNotInstalledInALinkedWorktreeNamesTheMainCheckout(t *testing.T) {
	panelHome(t)
	top := projectWithConfig(t, "Main", "")
	gitIn(t, top, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "first")
	linked := filepath.Join(filepath.Dir(top), filepath.Base(top)+"-lane")
	gitIn(t, top, "worktree", "add", "-q", "-b", "lane", linked)
	out, _ := doctorIn(t, linked)
	needExact(t, out, "fix: clauductor panel install --project "+install.ShellWord(top))
}

// Linux and WSL2 have no panel lines: the panel runs on macOS.
func TestDoctorHasNoPanelLinesOffMacOS(t *testing.T) {
	home := panelHome(t)
	doctorGOOS = "linux"
	writePlist(t, home)
	out, _ := doctorIn(t, projectWithConfig(t, "Fine", ""))
	for _, subject := range []string{"registry", "running", "build", "serves"} {
		needNoLine(t, out, subject)
	}
	needNoLine(t, out, "login agent")
}

// [DOCTOR-1-S5] A login agent whose panel is not running warns, naming the restart and the log;
// with no login agent, a registered project and no panel, the line is a note.
func TestDoctorLoginAgentWithNoPanelWarns(t *testing.T) {
	home := panelHome(t)
	writePlist(t, home)
	out, err := doctorIn(t, newRepo(t))
	if err != nil {
		t.Fatalf("a warning is not a failure: %v\n%s", err, out)
	}
	l := needLine(t, out, "warn", "running", "not running")
	if !strings.Contains(l, filepath.Join(install.LogDir(home), "panel.err.log")) {
		t.Errorf("the warning does not name the log: %q", l)
	}
	needExact(t, out, "fix: launchctl kickstart -k gui/$(id -u)/"+install.LaunchdLabel)
}

func TestDoctorRegisteredProjectWithNoLoginAgentIsANote(t *testing.T) {
	home := panelHome(t)
	dir := projectWithConfig(t, "Fine", "")
	registerProject(t, home, dir)
	out, err := doctorIn(t, dir)
	if err != nil {
		t.Fatalf("exit: %v\n%s", err, out)
	}
	needLine(t, out, "note", "running", "no login agent")
	needNoLine(t, out, "warn", "running")
	// The registry's default project loads, so the login agent can be installed (D8).
	needExact(t, out, "fix: clauductor panel install")

	// Its config no longer loads: panel install would refuse, so it is not printed.
	write(t, dir, ".clauductor/panel.json", panelJSON("Fine", `,"bogus":1`))
	out, _ = doctorIn(t, dir)
	needLine(t, out, "note", "running", "no login agent")
	needNoLine(t, out, "fix: clauductor panel install")
}

// A live panel is ok, with its pid and port. The process check is the test's, so no process is
// started and none touched.
func TestDoctorRunningPanelIsOk(t *testing.T) {
	home := panelHome(t)
	writePlist(t, home)
	if err := install.ClaimPanelFiles(home, install.PanelOwner{PID: 424242, PStart: "Mon Oct 24 10:00:00 2026", Port: 4393}); err != nil {
		t.Fatal(err)
	}
	old := doctorLiveProc
	t.Cleanup(func() { doctorLiveProc = old })
	doctorLiveProc = func(pid int) (bool, string) { return pid == 424242, "Mon Oct 24 10:00:00 2026" }
	out, _ := doctorIn(t, newRepo(t))
	needLine(t, out, "ok", "running", "pid 424242", "port 4393")
	needNoLine(t, out, "warn", "running")
}

// [DOCTOR-1-S6] A panel binary of another build is a note naming both revisions and both paths,
// and panel install only as the way to put this binary in the panel when it is the newer.
func TestDoctorPanelBuildOfAnotherRevisionIsANote(t *testing.T) {
	home := panelHome(t)
	writePlist(t, home)
	dir := projectWithConfig(t, "Fine", "")
	registerProject(t, home, dir)
	bin := install.InstalledBinary(home)
	write(t, filepath.Dir(bin), filepath.Base(bin), "binary")

	oldFile, oldRun := doctorFileStamp, doctorRunningStamp
	t.Cleanup(func() { doctorFileStamp, doctorRunningStamp = oldFile, oldRun })
	doctorFileStamp = func(path string) (install.BuildStamp, error) {
		if path != bin {
			t.Errorf("the stamp of %s was read, want %s", path, bin)
		}
		return install.BuildStamp{Module: "m", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Modified: true}, nil
	}
	doctorRunningStamp = func() install.BuildStamp {
		return install.BuildStamp{Module: "m", Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	}
	out, err := doctorIn(t, dir)
	if err != nil {
		t.Fatalf("a note is not a failure: %v\n%s", err, out)
	}
	exe, _ := os.Executable()
	l := needLine(t, out, "note", "build", "aaaaaaaa", "bbbbbbbb", bin, exe, "modified", "`clauductor panel install`", "rotates the page's token")
	if strings.Contains(l, "cccccccc") {
		t.Errorf("unexpected revision: %q", l)
	}
	needNoLine(t, out, "fix: clauductor panel install")
	needNoLine(t, out, "warn", "build")

	// The same revision is ok, and no revision at all cannot be compared.
	doctorFileStamp = func(string) (install.BuildStamp, error) {
		return install.BuildStamp{Module: "m", Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, nil
	}
	out, _ = doctorIn(t, dir)
	needLine(t, out, "ok", "build", "bbbbbbbb")
	doctorFileStamp = func(string) (install.BuildStamp, error) { return install.BuildStamp{Module: "m"}, nil }
	out, _ = doctorIn(t, dir)
	needLine(t, out, "note", "build", "cannot compare")
}

// panel install is named only when the registry's default project would let it run (D8).
func TestDoctorBuildNoteNamesNoPanelInstallThatWouldRefuse(t *testing.T) {
	home := panelHome(t)
	writePlist(t, home)
	bin := install.InstalledBinary(home)
	write(t, filepath.Dir(bin), filepath.Base(bin), "binary")
	oldFile, oldRun := doctorFileStamp, doctorRunningStamp
	t.Cleanup(func() { doctorFileStamp, doctorRunningStamp = oldFile, oldRun })
	doctorFileStamp = func(string) (install.BuildStamp, error) {
		return install.BuildStamp{Module: "m", Revision: "aaaaaaaaaaaa"}, nil
	}
	doctorRunningStamp = func() install.BuildStamp { return install.BuildStamp{Module: "m", Revision: "bbbbbbbbbbbb"} }
	out, _ := doctorIn(t, newRepo(t)) // an empty registry has no default project
	l := needLine(t, out, "note", "build", "aaaaaaaa", "bbbbbbbb")
	if strings.Contains(l, "panel install") {
		t.Errorf("the note names a panel install that would refuse (no project registered): %q", l)
	}
}

// A registry that does not load fails, and the project's panel lines say they did not run.
func TestDoctorRegistryThatDoesNotLoadFails(t *testing.T) {
	home := panelHome(t)
	write(t, home, ".clauductor/panel/projects.json", "not json")
	out, err := doctorIn(t, newRepo(t))
	if err == nil {
		t.Errorf("a registry that does not load must exit 1:\n%s", out)
	}
	needLine(t, out, "FAIL", "registry", "projects.json")
	needLine(t, out, "note", "serves", "did not run")
	needNoLine(t, out, "not installed")
	// Nothing is built from the empty stand-in: no "0 projects" as ok, and no advice to start a
	// panel that cannot start from this registry.
	needNoLine(t, out, "0 projects")
	needNoLine(t, out, "ok panel")
	needNoLine(t, out, "running")
	needLine(t, out, "note", "panel", "not known")
}

// The login agent is a fact the plist gives without the registry, so it is still said.
func TestDoctorRegistryThatDoesNotLoadStillSaysTheLoginAgent(t *testing.T) {
	home := panelHome(t)
	writePlist(t, home)
	write(t, home, ".clauductor/panel/projects.json", "not json")
	out, _ := doctorIn(t, newRepo(t))
	needLine(t, out, "ok", "panel", "login agent installed")
	needNoLine(t, out, "0 projects")
}

// [DOCTOR-2-S6] A served config not trusted as it is now warns that cards, Run and templates do
// nothing, and names panel trust with the main checkout.
func TestDoctorServedConfigNotTrustedWarns(t *testing.T) {
	home := panelHome(t)
	dir := projectWithConfig(t, "Fine", "")
	e := registerProject(t, home, dir)
	if _, err := install.TrustConfig(home, e.Root, e.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	out, err := doctorIn(t, dir)
	if err != nil {
		t.Fatalf("exit: %v\n%s", err, out)
	}
	needLine(t, out, "ok", "serves")
	needLine(t, out, "ok", "config")
	needLine(t, out, "ok", "trust")

	write(t, dir, ".clauductor/panel.json", panelJSON("Fine", `,"queues":[]`)) // changed since it was trusted
	out, err = doctorIn(t, dir)
	if err != nil {
		t.Fatalf("a warning is not a failure: %v\n%s", err, out)
	}
	l := needLine(t, out, "warn", "trust")
	if !strings.Contains(l, "cards, Run and templates do nothing") {
		t.Errorf("the warning does not say what stops: %q", l)
	}
	needExact(t, out, "fix: clauductor panel trust --project "+install.ShellWord(dir))
}

// A project never trusted is the same warning.
func TestDoctorNeverTrustedConfigWarns(t *testing.T) {
	home := panelHome(t)
	dir := projectWithConfig(t, "Fine", "")
	registerProject(t, home, dir)
	out, _ := doctorIn(t, dir)
	needLine(t, out, "warn", "trust")
}

// [DOCTOR-2-S7] A project the panel does not serve is a note, with panel add; no config or trust
// line, and doctor exits 0. When its panel.json names the served project's socket, the note gives
// panel add's refusal instead of the command.
func TestDoctorProjectThePanelDoesNotServeIsANote(t *testing.T) {
	home := panelHome(t)
	writePlist(t, home)
	served := projectWithConfig(t, "Served", "")
	registerProject(t, home, served) // holds the socket "clauductor"
	mine := projectWithConfig(t, "Mine", `,"tmux_socket":"mine"`)

	out, err := doctorIn(t, mine)
	if err != nil {
		t.Fatalf("a note is not a failure: %v\n%s", err, out)
	}
	needLine(t, out, "note", "serves", "does not serve")
	needExact(t, out, "fix: clauductor panel add --project "+install.ShellWord(mine))
	needNoLine(t, out, "config")
	needNoLine(t, out, "trust")

	write(t, mine, ".clauductor/panel.json", panelJSON("Mine", `,"tmux_socket":"clauductor"`))
	out, err = doctorIn(t, mine)
	if err != nil {
		t.Fatalf("a note is not a failure: %v\n%s", err, out)
	}
	l := needLine(t, out, "note", "serves", "does not serve")
	if !strings.Contains(l, "give this project its own tmux_socket") {
		t.Errorf("the note does not give panel add's refusal: %q", l)
	}
	needNoLine(t, out, "fix: clauductor panel add")
	needNoLine(t, out, "fix: clauductor panel init")

	// The THEN: the command the note named, run on the project as it was then, registers it.
	write(t, mine, ".clauductor/panel.json", panelJSON("Mine", `,"tmux_socket":"mine"`))
	registerProject(t, home, mine)
	if reg, err := config.LoadProjects(home); err != nil || reg.Find(mine) == nil {
		t.Fatalf("panel add did not register the project the note named: %v", err)
	}
	out, _ = doctorIn(t, mine)
	needLine(t, out, "ok", "serves")
}

// A project the panel does not serve and that has no panel.json: panel init first, then add.
func TestDoctorUnservedProjectWithNoConfigNamesInitThenAdd(t *testing.T) {
	home := panelHome(t)
	writePlist(t, home)
	registerProject(t, home, projectWithConfig(t, "Served", ""))
	mine := newRepo(t)
	out, _ := doctorIn(t, mine)
	initFix := "fix: clauductor panel init --project " + install.ShellWord(mine)
	addFix := "fix: clauductor panel add --project " + install.ShellWord(mine)
	needExact(t, out, initFix)
	needExact(t, out, addFix)
	if strings.Index(out, initFix) > strings.Index(out, addFix) {
		t.Errorf("panel init is not before panel add:\n%s", out)
	}
}

// A repository registered under another path is refused by panel add, so the note says so rather
// than printing panel init: the missing config is not the refusal that comes first.
func TestDoctorUnservedProjectAddRefusesForAnotherReasonPrintsNoInit(t *testing.T) {
	home := panelHome(t)
	writePlist(t, home)
	top := projectWithConfig(t, "Served", "")
	gitIn(t, top, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "first")
	registerProject(t, home, top)
	// The registry holds the repository's main checkout. A second path to it is a symlink to the
	// main checkout, which resolves to it, so make the registry's entry the other path: re-root
	// the entry at a path that is a different spelling of the same repository.
	reg, err := config.LoadProjects(home)
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.Symlink(top, link); err != nil {
		t.Fatal(err)
	}
	reg.Projects[0].Root = link
	if err := config.SaveProjects(home, reg); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(top, ".clauductor", "panel.json")); err != nil {
		t.Fatal(err)
	}
	out, _ := doctorIn(t, top)
	l := needLine(t, out, "note", "serves", "does not serve")
	if !strings.Contains(l, "registered already as") {
		t.Errorf("the note does not give panel add's refusal: %q", l)
	}
	needNoLine(t, out, "fix: clauductor panel")
}

// [DOCTOR-2-S8] A config that does not load fails with the loader's error, and doctor exits 1.
func TestDoctorServedConfigThatDoesNotLoadFails(t *testing.T) {
	home := panelHome(t)
	dir := projectWithConfig(t, "Fine", "")
	registerProject(t, home, dir)
	write(t, dir, ".clauductor/panel.json", panelJSON("Fine", `,"bogus":1`))
	_, _, loadErr := config.LoadConfigRaw(filepath.Join(dir, ".clauductor", "panel.json"))
	if loadErr == nil {
		t.Fatal("the fixture's config loads")
	}
	out, err := doctorIn(t, dir)
	if err == nil {
		t.Errorf("doctor must exit 1:\n%s", out)
	}
	needLine(t, out, "FAIL", "config", loadErr.Error())
	needNoLine(t, out, "trust")
}

// A served project whose config file is gone is a note, with panel init when the registry reads
// the project's own file; a sidecar's missing file is not cleared by init.
func TestDoctorServedConfigMissingIsANote(t *testing.T) {
	home := panelHome(t)
	dir := projectWithConfig(t, "Fine", "")
	registerProject(t, home, dir)
	if err := os.Remove(filepath.Join(dir, ".clauductor", "panel.json")); err != nil {
		t.Fatal(err)
	}
	out, err := doctorIn(t, dir)
	if err != nil {
		t.Fatalf("a note is not a failure: %v\n%s", err, out)
	}
	needLine(t, out, "note", "config", "no panel config")
	needExact(t, out, "fix: clauductor panel init --project "+install.ShellWord(dir))

	reg, _ := config.LoadProjects(home)
	reg.Projects[0].Config = filepath.Join(home, "sidecar", "panel.json")
	if err := config.SaveProjects(home, reg); err != nil {
		t.Fatal(err)
	}
	out, _ = doctorIn(t, dir)
	needLine(t, out, "note", "config", "no panel config")
	needNoLine(t, out, "fix: clauductor panel init")
}

// [DOCTOR-2-S9] From a lane worktree the panel's lines read the main checkout, and the drift line
// reads the worktree's own files.
func TestDoctorFromALaneWorktreeReadsTheMainCheckout(t *testing.T) {
	home := panelHome(t)
	main := installedRepo(t) // the install puts the model, and the template's panel.json, here
	tmplDir(t)
	gitIn(t, main, "add", "-A")
	gitIn(t, main, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "-m", "model")
	e := registerProject(t, home, main)
	if _, err := install.TrustConfig(home, e.Root, e.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	lane := filepath.Join(filepath.Dir(main), filepath.Base(main)+"-lane")
	gitIn(t, main, "worktree", "add", "-q", "-b", "lane", lane)
	lane, _ = filepath.EvalSymlinks(lane)

	out, err := doctorIn(t, lane)
	if err != nil {
		t.Fatalf("exit: %v\n%s", err, out)
	}
	needLine(t, out, "ok", "serves")
	needLine(t, out, "ok", "trust")
	needLine(t, out, "ok", "drift", "matches the template")
	needNoLine(t, out, "panel add")

	// The drift line reads the worktree: a file gone from it alone is missing there.
	if err := os.Remove(filepath.Join(lane, ".claude", "hooks", "focus-staleness.sh")); err != nil {
		t.Fatal(err)
	}
	out, _ = doctorIn(t, lane)
	needLine(t, out, "warn", "drift", "1 missing")
	needLine(t, out, "ok", "trust")
	out, _ = doctorIn(t, main)
	needLine(t, out, "ok", "drift", "matches the template")
}

// A linked worktree of a bare repository has no main checkout the panel can serve: one note.
func TestDoctorLaneWorktreeOfABareRepositoryHasOnePanelNote(t *testing.T) {
	home := panelHome(t)
	writePlist(t, home)
	src := newRepo(t)
	gitIn(t, src, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "first")
	holder, _ := filepath.EvalSymlinks(t.TempDir())
	bare := filepath.Join(holder, "bare.git")
	gitIn(t, src, "clone", "-q", "--bare", src, bare)
	linked := filepath.Join(holder, "work")
	gitIn(t, bare, "worktree", "add", "-q", "-b", "lane", linked)
	out, _ := doctorIn(t, linked)
	needLine(t, out, "note", "serves", "bare")
	needNoLine(t, out, "fix: clauductor panel")
	_ = home
}
