package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/template"
	"github.com/clauductor/clauductor/internal/testbin"
	"github.com/clauductor/clauductor/internal/treesnap"
)

// allTools is every tool prereqs.sh lists.
var allTools = []string{"git", "jq", "gh", "gitleaks", "claude", "tmux", "node", "python3"}

// without is allTools minus the named ones.
func without(drop ...string) []string {
	var out []string
	for _, t := range allTools {
		keep := true
		for _, d := range drop {
			keep = keep && t != d
		}
		if keep {
			out = append(out, t)
		}
	}
	return out
}

// doctor runs the command in the test's current directory on a PATH of path, as macOS.
func doctor(t *testing.T, path string) (string, error) {
	t.Helper()
	return doctorOn(t, path, "Darwin")
}

// doctorOn is doctor with prereqs.sh's hints for osName (PREREQS_OS).
func doctorOn(t *testing.T, path, osName string) (string, error) {
	t.Helper()
	t.Setenv("PATH", path)
	t.Setenv("PREREQS_OS", osName)
	t.Cleanup(func() { template.OverrideDir = ""; doctorCmd.SetOut(nil) })
	var out bytes.Buffer
	doctorCmd.SetOut(&out)
	err := doctorCmd.RunE(doctorCmd, nil)
	return out.String(), err
}

// lineWith is the output's first line holding every part, or "".
func lineWith(out string, parts ...string) string {
	for _, l := range strings.Split(out, "\n") {
		ok := true
		for _, p := range parts {
			ok = ok && strings.Contains(l, p)
		}
		if ok {
			return l
		}
	}
	return ""
}

func needLine(t *testing.T, out string, parts ...string) string {
	t.Helper()
	l := lineWith(out, parts...)
	if l == "" {
		t.Fatalf("no line holds %q:\n%s", parts, out)
	}
	return l
}

// needExact fails unless some line of the output, trimmed, is exactly want: a fix is a command to
// paste, so a test asserts all of it.
func needExact(t *testing.T, out, want string) {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) == want {
			return
		}
	}
	t.Errorf("no line is exactly %q:\n%s", want, out)
}

// installFakeGH puts a gh in bin that prints marker to both streams, then stderrLine (when set)
// to stderr, and exits with status.
func installFakeGH(t *testing.T, bin, marker, stderrLine string, status int) {
	t.Helper()
	body := "#!/bin/sh\necho " + marker + "\necho " + marker + " >&2\n"
	if stderrLine != "" {
		body += "echo '" + stderrLine + "' >&2\n"
	}
	body += "exit " + strconv.Itoa(status) + "\n"
	testbin.Write(t, filepath.Join(bin, "gh"), body)
}

// [DOCTOR-1-S1] A missing required tool fails, with its install hint, and doctor exits 1.
func TestDoctorMissingRequiredToolFails(t *testing.T) {
	tmplDir(t)
	t.Chdir(t.TempDir())
	out, err := doctor(t, stubPath(t, without("jq")...))
	l := needLine(t, out, "FAIL", "jq")
	for _, want := range []string{"required", "read JSON with jq"} {
		if !strings.Contains(l, want) {
			t.Errorf("the jq line lacks %q: %s", want, l)
		}
	}
	needLine(t, out, "install: brew install jq")
	if !errors.Is(err, errDoctorFailed) {
		t.Errorf("doctor returned %v, want errDoctorFailed (exit 1)", err)
	}
	if lineWith(out, "FAIL", "git") != "" {
		t.Errorf("a present tool failed:\n%s", out)
	}
}

// [DOCTOR-1-S2] A missing recommended tool warns, a missing optional one is a note, exit 0.
func TestDoctorRecommendedWarnsOptionalNotes(t *testing.T) {
	tmplDir(t)
	t.Chdir(t.TempDir())
	out, err := doctor(t, stubPath(t, without("gitleaks", "tmux")...))
	if err != nil {
		t.Fatalf("doctor failed with only a warning and a note: %v\n%s", err, out)
	}
	needLine(t, out, "warn", "gitleaks", "recommended")
	needLine(t, out, "install: brew install gitleaks")
	needLine(t, out, "note", "tmux", "optional")
	needLine(t, out, "0 failed, 1 warning")
}

// [DOCTOR-1-S2] A template whose prereqs.sh prints only a report for people gives a note naming
// the script, not an empty list of tools.
func TestDoctorUnreadableToolRowsIsANote(t *testing.T) {
	tmplDir(t)
	t.Chdir(t.TempDir())
	old := t.TempDir()
	write(t, old, ".claude/lib/conf.sh", "")
	write(t, old, ".template-version", Version+"\n")
	write(t, old, PrereqsScript, "#!/bin/sh\necho 'Prerequisites on macOS (a report)'\necho '  ok       git       /usr/bin/git'\n")
	template.OverrideDir = old
	out, err := doctor(t, stubPath(t, allTools...))
	if err != nil {
		t.Fatalf("an unreadable report must not fail doctor: %v\n%s", err, out)
	}
	script := filepath.Join(old, PrereqsScript)
	l := needLine(t, out, "note", "tools", script)
	if !strings.Contains(l, "could not be read") {
		t.Errorf("the note does not say the tools could not be read: %s", l)
	}
	if lineWith(out, "ok", "jq") != "" || lineWith(out, "git", "/usr/bin/git") != "" {
		t.Errorf("tool lines were printed from a report that is not rows:\n%s", out)
	}
}

func TestParseToolRowsIsStrict(t *testing.T) {
	good := "ok\trequired\tgit\t/usr/bin/git\tbranches\nmissing\toptional\ttmux\tbrew install tmux\tlanes\n"
	rows, err := parseToolRows(good)
	if err != nil || len(rows) != 2 || !rows[0].ok || rows[1].ok || rows[1].where != "brew install tmux" {
		t.Fatalf("good rows: %v %v", rows, err)
	}
	for name, bad := range map[string]string{
		"empty":           "",
		"only whitespace": "\n\n",
		"four fields":     "ok\trequired\tgit\t/usr/bin/git\n",
		"six fields":      "ok\trequired\tgit\t/usr/bin/git\tx\ty\n",
		"unknown status":  "found\trequired\tgit\t/usr/bin/git\tx\n",
		"unknown level":   "ok\tessential\tgit\t/usr/bin/git\tx\n",
		"no tool":         "ok\trequired\t\t/usr/bin/git\tx\n",
		"one bad of two":  good + "garbage\n",
		"a blank between": "ok\trequired\tgit\tp\tx\n\nok\trequired\tjq\tp\tx\n",
		"a human report":  "Prerequisites on macOS:\n  ok       git       /usr/bin/git\n",
	} {
		if rows, err := parseToolRows(bad); err == nil {
			t.Errorf("%s: parsed as %v, want an error", name, rows)
		}
	}
}

// [DOCTOR-1-S3] A template that is not the binary's fails with Resolve's reason, and the tools
// line is a note that it could not run.
func TestDoctorTemplateOfAnotherVersionFails(t *testing.T) {
	t.Chdir(t.TempDir())
	fw := t.TempDir()
	write(t, fw, "template/.claude/lib/conf.sh", "")
	write(t, fw, "template/.template-version", "99.0.0\n")
	t.Setenv("CLAUDUCTOR_FRAMEWORK", fw)
	out, err := doctor(t, stubPath(t, allTools...))
	l := needLine(t, out, "FAIL", "template")
	if !strings.Contains(l, "99.0.0") {
		t.Errorf("the template line does not give the template's version: %s", l)
	}
	for _, want := range []string{"install the matching release", "CLAUDUCTOR_FRAMEWORK", "--template-dir"} {
		needLine(t, out, want)
	}
	needLine(t, out, "note", "tools", "no template")
	if !errors.Is(err, errDoctorFailed) {
		t.Errorf("doctor returned %v, want errDoctorFailed", err)
	}
}

// [DOCTOR-1-S3] With --template-dir a version mismatch is Source.Warning: a warning, not a failure.
func TestDoctorTemplateDirWithOtherVersionWarns(t *testing.T) {
	tmpl := tmplDir(t)
	t.Chdir(t.TempDir())
	other := t.TempDir()
	write(t, other, ".claude/lib/conf.sh", "")
	write(t, other, ".template-version", "99.0.0\n")
	raw, err := os.ReadFile(filepath.Join(tmpl, PrereqsScript))
	if err != nil {
		t.Fatal(err)
	}
	write(t, other, PrereqsScript, string(raw))
	template.OverrideDir = other
	out, err := doctor(t, stubPath(t, allTools...))
	if err != nil {
		t.Fatalf("a --template-dir mismatch must not fail doctor: %v\n%s", err, out)
	}
	needLine(t, out, "warn", "template", "using it because --template-dir names it")
	needLine(t, out, "ok", "git") // the tools were read from that template
}

// captureProcessStreams points the process's own stdout and stderr at a pipe, so a child that
// inherits them is caught: doctorCmd.SetOut sees only what doctor itself prints. The returned
// function restores them and gives back everything written to either.
func captureProcessStreams(t *testing.T) func() string {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = pw, pw
	restored := false
	restore := func() {
		if !restored {
			restored = true
			os.Stdout, os.Stderr = oldOut, oldErr
			pw.Close()
		}
	}
	t.Cleanup(func() { restore(); pr.Close() })
	return func() string {
		restore()
		var b bytes.Buffer
		_, _ = b.ReadFrom(pr)
		return b.String()
	}
}

// [DOCTOR-1-S4] A signed-out gh warns and names gh auth login; a signed-in one prints no token.
func TestDoctorGHSignedOutWarnsAndNeverPrintsToken(t *testing.T) {
	tmplDir(t)
	t.Chdir(t.TempDir())
	const marker = "ghp_SECRETTOKENMARKER"

	bin := stubPath(t, without("gh")...)
	// What gh 2.17 and later print when no account is signed in to the host asked about.
	installFakeGH(t, bin, marker, "no oauth token found for github.com", 1)
	leaked := captureProcessStreams(t)
	out, err := doctor(t, bin)
	if got := leaked(); strings.Contains(got, marker) {
		t.Errorf("gh wrote to the terminal doctor runs in (signed out):\n%s", got)
	}
	if err != nil {
		t.Fatalf("a signed-out gh is a warning, not a failure: %v\n%s", err, out)
	}
	needLine(t, out, "warn", "gh", "not signed in")
	if l := strings.TrimSpace(needLine(t, out, "fix: gh auth login")); l != "fix: gh auth login" {
		t.Errorf("signed out, the fix is %q, want gh auth login", l)
	}
	if strings.Contains(out, marker) {
		t.Errorf("doctor printed what gh printed:\n%s", out)
	}

	bin = stubPath(t, without("gh")...)
	installFakeGH(t, bin, marker, "", 0)
	leaked = captureProcessStreams(t)
	out, err = doctor(t, bin)
	if got := leaked(); strings.Contains(got, marker) {
		t.Errorf("gh wrote the token to the terminal doctor runs in (signed in):\n%s", got)
	}
	if err != nil {
		t.Fatal(err)
	}
	needLine(t, out, "ok", "gh", "signed in")
	if strings.Contains(out, marker) {
		t.Errorf("doctor printed the token gh printed:\n%s", out)
	}
	if lineWith(out, "gh auth login") != "" {
		t.Errorf("a signed-in gh was told to log in:\n%s", out)
	}
}

// [DOCTOR-3-S2] The last line counts failures and warnings and the exit status follows the
// failures; with nothing found it says so.
func TestDoctorLastLineCountsAndExitFollowsFailures(t *testing.T) {
	tmplDir(t)
	t.Chdir(t.TempDir())

	// jq missing: one failure; gh and gitleaks missing: two warnings.
	out, err := doctor(t, stubPath(t, without("jq", "gh", "gitleaks")...))
	last := strings.TrimSpace(out[strings.LastIndex(strings.TrimSpace(out), "\n"):])
	if last != "1 failed, 2 warnings" || !errors.Is(err, errDoctorFailed) {
		t.Errorf("last line %q, err %v; want '1 failed, 2 warnings' and a failure", last, err)
	}

	// The failure fixed, the warnings left: exit 0.
	out, err = doctor(t, stubPath(t, without("gh", "gitleaks")...))
	last = strings.TrimSpace(out[strings.LastIndex(strings.TrimSpace(out), "\n"):])
	if last != "0 failed, 2 warnings" || err != nil {
		t.Errorf("last line %q, err %v; want '0 failed, 2 warnings' and exit 0", last, err)
	}

	// Nothing found.
	out, err = doctor(t, stubPath(t, allTools...))
	last = strings.TrimSpace(out[strings.LastIndex(strings.TrimSpace(out), "\n"):])
	if last != "Nothing missing or broken." || err != nil {
		t.Errorf("last line %q, err %v; want the nothing-found line and exit 0\n%s", last, err, out)
	}
}

// ghLine runs doctor with a stand-in gh that prints stderrLine to stderr and exits 1, and
// returns the whole output and the gh line.
func ghLine(t *testing.T, stderrLine string) (string, string) {
	t.Helper()
	tmplDir(t)
	t.Chdir(t.TempDir())
	const marker = "ghp_SECRETTOKENMARKER"
	bin := stubPath(t, without("gh")...)
	installFakeGH(t, bin, marker, stderrLine, 1)
	out, err := doctor(t, bin)
	if err != nil {
		t.Fatalf("a gh that gives no token must not fail doctor: %v\n%s", err, out)
	}
	if strings.Contains(out, marker) {
		t.Errorf("doctor printed what gh printed:\n%s", out)
	}
	// The sign-in line, not the tools line for gh: it comes after the template line.
	_, after, _ := strings.Cut(out, needLine(t, out, "template"))
	return out, needLine(t, after, " gh ")
}

// [DOCTOR-1-S4] Every non-zero exit of gh auth token warns and names gh auth login, whatever gh
// said on stderr (the design's table: a signed-out host, a gh older than 2.17 and a locked
// keychain read the same), and nothing gh printed reaches the output.
func TestDoctorGHAnyFailureWarnsWithLogin(t *testing.T) {
	for _, stderr := range []string{
		"no oauth token found for ghe.example.com",
		`unknown command "token" for "gh auth"`,
		"failed to read keyring: the keychain is locked",
		"",
	} {
		out, l := ghLine(t, stderr)
		if !strings.Contains(l, "warn") {
			t.Errorf("gh failing with %q is not a warn: %s", stderr, l)
		}
		if f := strings.TrimSpace(needLine(t, out, "fix: gh auth login")); f != "fix: gh auth login" {
			t.Errorf("gh failing with %q: the fix is %q, want gh auth login", stderr, f)
		}
		if stderr != "" && strings.Contains(out, stderr) {
			t.Errorf("doctor printed gh's stderr %q:\n%s", stderr, out)
		}
	}
}

// [DOCTOR-1-S1] An install hint that is prose, not a command (Linux's "(or your distribution's
// package manager)", claude's "see https://..."), is printed as the install hint, never as a fix:
// a fix is a command to paste as it stands (D8).
func TestDoctorInstallHintIsNotAFix(t *testing.T) {
	tmplDir(t)
	t.Chdir(t.TempDir())
	out, _ := doctorOn(t, stubPath(t, without("jq", "claude")...), "Linux")
	needLine(t, out, "install: sudo apt install jq   (or your distribution's package manager)")
	needLine(t, out, "install: see https://claude.ai/code")
	if l := lineWith(out, "fix:"); l != "" {
		t.Errorf("an install hint was printed as a fix: %s\n%s", l, out)
	}
}

// A multi-line detail, fix or install hint continues at the column where its first line's text
// starts, and a fix or install hint sits under the line's text.
func TestDoctorContinuationLinesAlign(t *testing.T) {
	rep := &doctorReport{}
	rep.add(groupMachine, doctorLine{level: levelWarn, subject: "template", detail: "FIRST\nSECOND", fixes: []string{"FIXA\nFIXB"}, install: "INSTA\nINSTB"})
	var b bytes.Buffer
	rep.print(&b)
	out := b.String()
	if strings.Index(lineWith(out, "fix:"), "fix:") != strings.Index(lineWith(out, "FIRST"), "FIRST") ||
		strings.Index(lineWith(out, "install:"), "install:") != strings.Index(lineWith(out, "FIRST"), "FIRST") {
		t.Errorf("fix: or install: does not sit under the line's text:\n%s", out)
	}
	for _, pair := range [][2]string{{"FIRST", "SECOND"}, {"FIXA", "FIXB"}, {"INSTA", "INSTB"}} {
		first, next := lineWith(out, pair[0]), lineWith(out, pair[1])
		if first == "" || next == "" || strings.Index(first, pair[0]) != strings.Index(next, pair[1]) {
			t.Errorf("%s does not continue under %s:\n%s", pair[1], pair[0], out)
		}
		if strings.TrimSpace(next) != pair[1] {
			t.Errorf("%s's line holds more than the continuation: %q", pair[1], next)
		}
	}
}

// Through the root command, as a user runs it: doctor is registered, and --template-dir reaches
// the template it reads the tools from.
func TestDoctorRunsFromRootWithTemplateDir(t *testing.T) {
	tmplDir(t)
	t.Chdir(t.TempDir())
	dir := t.TempDir()
	write(t, dir, ".claude/lib/conf.sh", "")
	write(t, dir, ".template-version", Version+"\n")
	write(t, dir, PrereqsScript, "#!/bin/sh\nprintf 'ok\\trequired\\tsentineltool\\t/opt/sentinel\\tbecause\\n'\n")
	t.Setenv("PATH", stubPath(t, without("gh")...))
	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs([]string{"doctor", "--template-dir", dir})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		template.OverrideDir = ""
		if f := doctorCmd.Flags().Lookup("template-dir"); f != nil {
			f.Changed = false
		}
	})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("clauductor doctor --template-dir: %v\n%s%s", err, out.String(), errOut.String())
	}
	needLine(t, out.String(), "ok", "sentineltool", "/opt/sentinel")
	needLine(t, out.String(), "template", dir)
}

// [DOCTOR-3-S2] In the terminal a user runs doctor in, a failure ends on the count line: through
// Main, as the binary runs, with stdout, cobra's errors and Main's own stderr all in one buffer
// (the order the terminal shows), no "Error:" line follows the count and the status is 1.
func TestDoctorFailureEndsOnTheCountLineInTheTerminal(t *testing.T) {
	tmplDir(t)
	t.Chdir(t.TempDir())
	t.Setenv("PATH", stubPath(t, without("jq", "gh", "gitleaks")...))
	t.Setenv("PREREQS_OS", "Darwin")
	var terminal bytes.Buffer
	rootCmd.SetOut(&terminal)
	rootCmd.SetErr(&terminal)
	rootCmd.SetArgs([]string{"doctor"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		template.OverrideDir = ""
	})
	status := Main(&terminal)
	out := strings.TrimSpace(terminal.String())
	if last := out[strings.LastIndex(out, "\n")+1:]; last != "1 failed, 2 warnings" {
		t.Errorf("the terminal's last line is %q, want '1 failed, 2 warnings':\n%s", last, out)
	}
	if strings.Contains(out, "Error:") {
		t.Errorf("an Error: line is printed after the report:\n%s", out)
	}
	if status != 1 {
		t.Errorf("exit status %d, want 1", status)
	}
}

// realGit is the git the tests run, found before any test replaces PATH.
var realGit, _ = exec.LookPath("git")

// gitPath is stubPath(tools) with a git that runs the real one: the project's lines read a real
// repository, and a stub git that exits 0 would show them nothing.
func gitPath(t *testing.T, tools ...string) string {
	t.Helper()
	if realGit == "" {
		t.Skip("git is not installed")
	}
	var rest []string
	for _, tool := range tools {
		if tool != "git" {
			rest = append(rest, tool)
		}
	}
	bin := stubPath(t, rest...)
	testbin.Write(t, filepath.Join(bin, "git"), "#!/bin/sh\nexec "+install.ShellWord(realGit)+" \"$@\"\n")
	return bin
}

// gitIn runs the real git in dir, failing the test on an error.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command(realGit, args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

// newRepo is an empty git repository, resolved the way git prints its toplevel.
func newRepo(t *testing.T) string {
	t.Helper()
	if realGit == "" {
		t.Skip("git is not installed")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-q")
	return dir
}

// installedRepo is a repository `clauductor install` put the model in, with the template the
// tests use.
func installedRepo(t *testing.T) string {
	t.Helper()
	tmplDir(t)
	dir := newRepo(t)
	t.Chdir(dir)
	var out bytes.Buffer
	installCmd.SetOut(&out)
	t.Cleanup(func() { installCmd.SetOut(nil) })
	if err := installCmd.RunE(installCmd, nil); err != nil {
		t.Fatalf("install: %v\n%s", err, out.String())
	}
	return dir
}

// [DOCTOR-2-S1] Outside a git repository only the machine is checked, and one note says so.
func TestDoctorOutsideARepositoryChecksOnlyTheMachine(t *testing.T) {
	tmplDir(t)
	t.Chdir(t.TempDir())
	out, err := doctor(t, gitPath(t, allTools...))
	if err != nil {
		t.Fatalf("doctor failed outside a repository: %v\n%s", err, out)
	}
	needLine(t, out, "ok", "jq") // a machine line
	needLine(t, out, "note", "repository", "the project checks did not run")
	for _, subject := range []string{"model", "drift"} {
		if l := lineWith(out, subject); l != "" {
			t.Errorf("a project line ran outside a repository: %s", l)
		}
	}
}

// [DOCTOR-2-S2] A repository with no model is a note naming clauductor install, with no drift
// line; below the toplevel the fix starts with cd, so it works where it is printed (D8).
func TestDoctorNoModelIsANoteWithInstall(t *testing.T) {
	tmplDir(t)
	top := newRepo(t)
	sub := filepath.Join(top, "pkg", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Chdir(top)
	out, err := doctor(t, gitPath(t, allTools...))
	if err != nil {
		t.Fatal(err)
	}
	needLine(t, out, "note", "model", "no operating model")
	needExact(t, out, "fix: clauductor install")
	needLine(t, out, "ok", "repository", top)

	t.Chdir(sub)
	out, err = doctor(t, gitPath(t, allTools...))
	if err != nil {
		t.Fatal(err)
	}
	needExact(t, out, "fix: cd "+install.ShellWord(top)+" && clauductor install")
	if lineWith(out, "fix: clauductor install") != "" {
		t.Errorf("a fix that would install into the subdirectory was printed:\n%s", out)
	}
	if l := lineWith(out, "drift"); l != "" {
		t.Errorf("a repository with no model has a drift line: %s", l)
	}
}

// [DOCTOR-2-S3] A repository running its own model, or the plugin's, is a note and has no drift
// line.
func TestDoctorOwnOrPluginModelIsNotCheckedForDrift(t *testing.T) {
	tmplDir(t)
	own := newRepo(t)
	write(t, own, ".claude/skills/mine/SKILL.md", "the project's own skill\n")
	t.Chdir(own)
	out, err := doctor(t, gitPath(t, allTools...))
	if err != nil {
		t.Fatal(err)
	}
	l := needLine(t, out, "note", "model", "operating model of its own")
	if !strings.Contains(l, "install and update refuse") {
		t.Errorf("the note does not say install and update refuse it: %s", l)
	}
	needExact(t, out, "fix: clauductor diff")
	if lineWith(out, "drift") != "" {
		t.Errorf("an own model has a drift line:\n%s", out)
	}

	plugin := newRepo(t)
	write(t, plugin, ".claude/skills/x/SKILL.md", "x\n")
	write(t, plugin, pluginMarker, "the plugin\n")
	t.Chdir(plugin)
	out, err = doctor(t, gitPath(t, allTools...))
	if err != nil {
		t.Fatal(err)
	}
	needLine(t, out, "note", "model", "clauductor plugin", pluginMarker)
	if lineWith(out, "drift") != "" || lineWith(out, "own") != "" {
		t.Errorf("the plugin's marker did not win, or a drift line ran:\n%s", out)
	}
}

// drifted installs the model, then edits one framework file and deletes another.
func drifted(t *testing.T) string {
	t.Helper()
	dir := installedRepo(t)
	write(t, dir, ".claude/hooks/format.sh", "# edited by the project\n")
	if err := os.Remove(filepath.Join(dir, ".claude/hooks/focus-staleness.sh")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// [DOCTOR-2-S4] A drifted framework warns with 1 file that differs and 1 missing, and the fixes
// start with cd <toplevel> when doctor runs from a subdirectory.
func TestDoctorDriftWarnsWithTheUpdateThatFixesIt(t *testing.T) {
	top := drifted(t)
	sub := filepath.Join(top, "docs")
	t.Chdir(sub)
	out, err := doctor(t, gitPath(t, allTools...))
	if err != nil {
		t.Fatalf("drift is a warning, not a failure: %v\n%s", err, out)
	}
	l := needLine(t, out, "warn", "drift")
	for _, want := range []string{"1 framework file differs", "1 missing"} {
		if !strings.Contains(l, want) {
			t.Errorf("the drift line lacks %q: %s", want, l)
		}
	}
	cd := "cd " + install.ShellWord(top) + " && "
	needExact(t, out, "fix: "+cd+"clauductor update --dry-run")
	needExact(t, out, "fix: "+cd+"clauductor update")
	// The fix works where it is printed: update --dry-run from the toplevel sees the same two files.
	t.Chdir(top)
	var dry bytes.Buffer
	updateCmd.SetOut(&dry)
	updateDryRun = true
	t.Cleanup(func() { updateCmd.SetOut(nil); updateDryRun = false })
	if err := updateCmd.RunE(updateCmd, nil); err != nil {
		t.Fatalf("the printed update --dry-run failed: %v\n%s", err, dry.String())
	}
	for _, f := range []string{".claude/hooks/format.sh", ".claude/hooks/focus-staleness.sh"} {
		if !strings.Contains(dry.String(), f) {
			t.Errorf("update --dry-run does not name %s:\n%s", f, dry.String())
		}
	}
	updateDryRun = false

	// The printed update clears the warning: run it from the toplevel, answering its prompts to
	// overwrite, then doctor again from the subdirectory.
	var upd bytes.Buffer
	updateCmd.SetOut(&upd)
	oldIn := stdin
	stdin = bufioReader("y\ny\ny\ny\ny\ny\n")
	t.Cleanup(func() { stdin = oldIn })
	if err := updateCmd.RunE(updateCmd, nil); err != nil {
		t.Fatalf("the printed update failed: %v\n%s", err, upd.String())
	}
	t.Chdir(sub)
	out, err = doctor(t, gitPath(t, allTools...))
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	needLine(t, out, "ok", "drift", "matches the template")
	if l := lineWith(out, "warn", "drift"); l != "" {
		t.Errorf("the drift warning survived the printed update: %s", l)
	}
}

// [DOCTOR-2-S4] A --template-dir doctor was given is carried into the fixes as the resolved
// absolute directory, quoted, so the fix compares against the template doctor judged and
// survives the cd.
func TestDoctorDriftFixCarriesTheTemplateDir(t *testing.T) {
	tmpl := tmplDir(t) // before drifted changes the directory: it is found relative to it
	top := drifted(t)
	copyDir := filepath.Join(t.TempDir(), "my template")
	if out, err := exec.Command("cp", "-R", tmpl, copyDir).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v\n%s", err, out)
	}
	t.Chdir(filepath.Join(top, "docs"))
	template.OverrideDir = copyDir
	out, err := doctor(t, gitPath(t, allTools...))
	if err != nil {
		t.Fatal(err)
	}
	flag := " --template-dir " + install.ShellWord(copyDir)
	if !strings.HasPrefix(flag, " --template-dir '") {
		t.Fatalf("the test's directory needs quoting: %s", flag)
	}
	for _, fix := range []string{"clauductor update --dry-run", "clauductor update"} {
		needExact(t, out, "fix: cd "+install.ShellWord(top)+" && "+fix+flag)
	}
}

// [DOCTOR-2-S4] A --template-dir given relative to the subdirectory doctor runs in, and naming a
// checkout rather than the template, is carried as the template's absolute path: the fix runs
// after a cd, where the relative path would name something else.
func TestDoctorDriftFixResolvesARelativeTemplateDir(t *testing.T) {
	tmpl := tmplDir(t)
	top := drifted(t)
	holder, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(holder, "my checkout")
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("cp", "-R", tmpl, filepath.Join(checkout, "template")).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v\n%s", err, out)
	}
	sub := filepath.Join(top, "docs")
	t.Chdir(sub)
	rel, err := filepath.Rel(sub, checkout)
	if err != nil || filepath.IsAbs(rel) {
		t.Fatalf("no relative path from %s to %s: %v", sub, checkout, err)
	}
	template.OverrideDir = rel
	out, err := doctor(t, gitPath(t, allTools...))
	if err != nil {
		t.Fatal(err)
	}
	flag := " --template-dir " + install.ShellWord(filepath.Join(checkout, "template"))
	for _, fix := range []string{"clauductor update --dry-run", "clauductor update"} {
		needExact(t, out, "fix: cd "+install.ShellWord(top)+" && "+fix+flag)
	}
}

// [DOCTOR-2-S2] [DOCTOR-2-S3] With no template the binary can use, install and diff refuse, so the
// model line prints no fix for either (D8: no fix the command would refuse); the template line's
// FAIL says why. It still names the command, for once the template is fixed. Both ways a template
// fails to resolve: a --template-dir that is no template, and a found template of another version.
func TestDoctorNoTemplatePrintsNoRepositoryFix(t *testing.T) {
	tmplDir(t)
	noModel := newRepo(t)
	sub := filepath.Join(noModel, "docs")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	own := newRepo(t)
	write(t, own, ".claude/skills/mine/SKILL.md", "the project's own skill\n")
	fw := t.TempDir()
	write(t, fw, "template/.claude/lib/conf.sh", "")
	write(t, fw, "template/.template-version", "99.0.0\n")

	for _, c := range []struct {
		name  string
		setup func()
	}{
		{"a --template-dir that is no template", func() { template.OverrideDir = filepath.Join("..", "not a template") }},
		{"a template of another version", func() { template.OverrideDir = ""; t.Setenv("CLAUDUCTOR_FRAMEWORK", fw) }},
	} {
		c.setup()
		t.Chdir(sub)
		out, err := doctor(t, gitPath(t, allTools...))
		if !errors.Is(err, errDoctorFailed) {
			t.Fatalf("%s: the template line did not fail: %v\n%s", c.name, err, out)
		}
		needLine(t, out, "FAIL", "template")
		needLine(t, out, "note", "model", "no operating model", "clauductor install")
		if l := lineWith(out, "fix:", "clauductor install"); l != "" {
			t.Errorf("%s: an install fix that would refuse was printed: %s\n%s", c.name, l, out)
		}

		c.setup()
		t.Chdir(own)
		out, _ = doctor(t, gitPath(t, allTools...))
		needLine(t, out, "note", "model", "operating model of its own", "clauductor diff")
		if l := lineWith(out, "fix:", "clauductor diff"); l != "" {
			t.Errorf("%s: a diff fix that would refuse was printed: %s\n%s", c.name, l, out)
		}
	}
}

// [DOCTOR-2-S2] Doctor prints `clauductor install` as a fix only when install itself would run
// (D8): the model line asks install's own checks (prepareInstall). Each state below has no sign of
// a model doctor reads, yet install refuses: a framework file the template ships, already there
// with other content (install calls that a model of the project's own), and a project.conf that
// puts the gate outside the repository. The test runs the printed install first to see it refuse,
// then asserts the note still names install, says why it refuses, and prints no install fix.
func TestDoctorNoModelPrintsNoInstallFixInstallWouldRefuse(t *testing.T) {
	for _, c := range []struct {
		name, reason string
		setup        func(t *testing.T, top string)
	}{
		{"a gate of its own", "runs an operating model of its own", func(t *testing.T, top string) {
			write(t, top, "scripts/ci/gate.sh", "#!/bin/sh\necho our own gate\n")
		}},
		{"a gate outside the repository", "is not a path inside the repository", func(t *testing.T, top string) {
			write(t, top, ".claude/project.conf", "GATE_RUN=../elsewhere/run-local.sh\n")
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			tmplDir(t)
			top := newRepo(t)
			c.setup(t, top)
			t.Chdir(top)
			var buf bytes.Buffer
			installCmd.SetOut(&buf)
			t.Cleanup(func() { installCmd.SetOut(nil) })
			if ierr := installCmd.RunE(installCmd, nil); ierr == nil || !strings.Contains(ierr.Error(), c.reason) {
				t.Fatalf("install did not refuse for %q: %v\n%s", c.reason, ierr, buf.String())
			}

			out, err := doctor(t, gitPath(t, allTools...))
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			needLine(t, out, "note", "model", "no operating model", "clauductor install", "install refuses here", c.reason)
			if l := lineWith(out, "fix:", "clauductor install"); l != "" {
				t.Errorf("an install fix that install refuses was printed: %s\n%s", l, out)
			}
		})
	}
}

// [DOCTOR-2-S3] Doctor prints `clauductor diff` as a fix only when diff itself would run (D8):
// the model line runs diff's own reading (template.Survey). A template that resolves (its
// .claude/lib/conf.sh exists) but whose defaults cannot be read makes diff refuse while the
// template line is ok; the note still names diff and says why.
func TestDoctorOwnModelPrintsNoDiffFixDiffWouldRefuse(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a file of mode 000")
	}
	tmpl := tmplDir(t)
	copyDir := filepath.Join(t.TempDir(), "tmpl")
	if out, err := exec.Command("cp", "-R", tmpl, copyDir).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v\n%s", err, out)
	}
	defaults := filepath.Join(copyDir, ".claude", "lib", "conf.sh")
	if err := os.Chmod(defaults, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(defaults, 0o644) })
	own := newRepo(t)
	write(t, own, ".claude/skills/mine/SKILL.md", "the project's own skill\n")
	t.Chdir(own)
	const reason = "template defaults"
	template.OverrideDir = copyDir
	t.Cleanup(func() { template.OverrideDir = "" })
	var buf bytes.Buffer
	diffCmd.SetOut(&buf)
	t.Cleanup(func() { diffCmd.SetOut(nil) })
	if derr := diffCmd.RunE(diffCmd, nil); derr == nil || !strings.Contains(derr.Error(), reason) {
		t.Fatalf("diff did not refuse: %v\n%s", derr, buf.String())
	}
	out, err := doctor(t, gitPath(t, allTools...))
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	needLine(t, out, "note", "model", "operating model of its own", "clauductor diff", "diff refuses here", reason)
	if l := lineWith(out, "fix:", "clauductor diff"); l != "" {
		t.Errorf("a diff fix that diff refuses was printed: %s\n%s", l, out)
	}
}

// [DOCTOR-2-S4] Doctor prints `clauductor update` as a fix only when update itself would run (D8):
// the drift line asks update's own planning, so every refusal of update is a refusal here. Each
// state below drifts the framework AND makes update refuse (a settings.json that does not parse,
// no .claude/skills/); the test runs the printed update first to see it refuse, then asserts the
// drift line is a warning that says why and prints no update fix.
func TestDoctorDriftPrintsNoUpdateFixUpdateWouldRefuse(t *testing.T) {
	for _, c := range []struct {
		name, reason string
		break_       func(t *testing.T, top string)
	}{
		{"settings.json that does not parse", "not valid JSON", func(t *testing.T, top string) {
			write(t, top, ".claude/settings.json", "{bad")
		}},
		{"no .claude/skills/", "no .claude/skills/ found", func(t *testing.T, top string) {
			if err := os.RemoveAll(filepath.Join(top, ".claude/skills")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			top := drifted(t)
			c.break_(t, top)
			var buf bytes.Buffer
			updateCmd.SetOut(&buf)
			updateDryRun = true
			t.Cleanup(func() { updateCmd.SetOut(nil); updateDryRun = false })
			uerr := updateCmd.RunE(updateCmd, nil)
			updateDryRun = false
			if uerr == nil || !strings.Contains(uerr.Error(), c.reason) {
				t.Fatalf("update --dry-run did not refuse for %q: %v", c.reason, uerr)
			}

			t.Chdir(filepath.Join(top, "docs"))
			out, err := doctor(t, gitPath(t, allTools...))
			if err != nil {
				t.Fatalf("drift is a warning, not a failure: %v\n%s", err, out)
			}
			needLine(t, out, "warn", "drift")
			if lineWith(out, "update refuses here", c.reason) == "" {
				t.Errorf("the drift line does not say why update refuses (%q):\n%s", c.reason, out)
			}
			if f := lineWith(out, "fix:", "clauductor update"); f != "" {
				t.Errorf("an update fix that update refuses was printed: %s\n%s", f, out)
			}
		})
	}
}

// [DOCTOR-2-S5] A framework that matches the template is ok, naming its version, and the last
// line, the count, names the model's own checks.
func TestDoctorConvergedFrameworkIsOkAndNamesTheChecks(t *testing.T) {
	top := installedRepo(t)
	out, err := doctor(t, gitPath(t, allTools...))
	if err != nil {
		t.Fatalf("doctor failed on a just-installed repository: %v\n%s", err, out)
	}
	src, err := template.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	needLine(t, out, "ok", "drift", "matches the template", "version "+src.Version)
	needLine(t, out, "ok", "model", "Clauductor's operating model")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if last := lines[len(lines)-1]; last != "Nothing missing or broken. The model's own checks: sh .claude/checks/run.sh" {
		t.Errorf("the last line is %q, want the nothing-found count naming the model's own checks", last)
	}

	// Below the toplevel the command is printed to work there.
	t.Chdir(filepath.Join(top, "docs"))
	out, _ = doctor(t, gitPath(t, allTools...))
	lines = strings.Split(strings.TrimSpace(out), "\n")
	want := "Nothing missing or broken. The model's own checks: cd " + install.ShellWord(top) + " && sh .claude/checks/run.sh"
	if last := lines[len(lines)-1]; last != want {
		t.Errorf("from a subdirectory the last line is %q, want %q", last, want)
	}
}

// [DOCTOR-3-S2] [DOCTOR-2-S5] In a repository holding Clauductor's model, with one failure and two
// warnings, the last line is still the count (S2), and it names the model's own checks (S5).
func TestDoctorModelRepoEndsOnTheCountLine(t *testing.T) {
	installedRepo(t)
	// jq missing: one failure; gh and gitleaks missing: two warnings.
	out, err := doctor(t, gitPath(t, without("jq", "gh", "gitleaks")...))
	if !errors.Is(err, errDoctorFailed) {
		t.Errorf("doctor returned %v, want errDoctorFailed", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	want := "1 failed, 2 warnings. The model's own checks: sh .claude/checks/run.sh"
	if last := lines[len(lines)-1]; last != want {
		t.Errorf("the last line is %q, want %q:\n%s", last, want, out)
	}
}

// A --template-dir of another version is Source.Warning, which Survey copies into its warnings. The
// template line says it; the drift line does not say it again.
func TestDoctorTemplateWarningIsSaidOnce(t *testing.T) {
	tmpl := tmplDir(t)
	installedRepo(t)
	other := filepath.Join(t.TempDir(), "other")
	if out, err := exec.Command("cp", "-R", tmpl, other).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v\n%s", err, out)
	}
	write(t, other, template.VersionFile, "99.0.0\n")
	template.OverrideDir = other
	out, _ := doctor(t, gitPath(t, allTools...))
	const warning = "using it because --template-dir names it"
	needLine(t, out, "warn", "template", warning)
	if n := strings.Count(out, warning); n != 1 {
		t.Errorf("the template's warning is printed %d times, want once (on the template line):\n%s", n, out)
	}
	if l := lineWith(out, "drift", warning); l != "" {
		t.Errorf("the drift line repeats the template's warning: %s", l)
	}
}

// Each of Survey's warnings other than the template's own is a warn line of its own: here the
// project's openspec-propose skill beside the template's propose.
func TestDoctorDriftWarnsForEachSurveyWarning(t *testing.T) {
	top := installedRepo(t)
	write(t, top, ".claude/skills/openspec-propose/SKILL.md", "the project's OpenSpec skill\n")
	out, err := doctor(t, gitPath(t, allTools...))
	if err != nil {
		t.Fatalf("a warning is not a failure: %v\n%s", err, out)
	}
	needLine(t, out, "warn", "drift", "skill trigger collision", "openspec-propose")
	needLine(t, out, "1 warning")
}

// A framework that cannot be compared is a note pointing at the template line, not a failure.
func TestDoctorDriftNotesWhenTheTemplateCannotBeRead(t *testing.T) {
	installedRepo(t)
	fw := t.TempDir()
	write(t, fw, "template/.claude/lib/conf.sh", "")
	write(t, fw, "template/.template-version", "99.0.0\n")
	t.Setenv("CLAUDUCTOR_FRAMEWORK", fw)
	out, _ := doctor(t, gitPath(t, allTools...))
	needLine(t, out, "note", "drift", "see the template line")
}

// The main checkout is found as install's mainRoot finds it, for the panel's lines (D6): the
// first entry of the worktree list, resolved, from the main checkout and from a linked worktree.
func TestDoctorFindsTheMainCheckout(t *testing.T) {
	top := newRepo(t)
	gitIn(t, top, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "first")
	linked := filepath.Join(filepath.Dir(top), filepath.Base(top)+"-lane")
	gitIn(t, top, "worktree", "add", "-q", "-b", "lane", linked)
	t.Setenv("PATH", gitPath(t, allTools...))

	t.Chdir(top)
	r := doctorFindRepo()
	if r == nil || r.main != top || r.top != top || r.below {
		t.Fatalf("at the main checkout: %+v", r)
	}
	t.Chdir(linked)
	r = doctorFindRepo()
	linkedResolved, _ := filepath.EvalSymlinks(linked)
	if r == nil || r.main != top || r.top != linkedResolved {
		t.Fatalf("in a linked worktree want top %s and main %s, got %+v", linkedResolved, top, r)
	}
}

// The panel's lines are keyed by the main checkout, and a bare first worktree is not one.
func TestDoctorABareRepositoryHasNoProject(t *testing.T) {
	if realGit == "" {
		t.Skip("git is not installed")
	}
	bare := t.TempDir()
	gitIn(t, bare, "init", "-q", "--bare")
	t.Chdir(bare)
	t.Setenv("PATH", gitPath(t, allTools...))
	if r := doctorFindRepo(); r != nil {
		t.Errorf("a bare repository has no working tree, got %+v", r)
	}
}

// A linked worktree of a bare repository lists the bare directory first. That is not a main
// checkout the panel can serve, so doctor holds no path for it and says why, instead of handing
// the bare directory to panel add (which refuses it).
func TestDoctorLinkedWorktreeOfABareRepositoryHasNoMain(t *testing.T) {
	src := newRepo(t)
	gitIn(t, src, "-c", "user.email=t@example.com", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "first")
	holder, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bare := filepath.Join(holder, "bare.git")
	gitIn(t, src, "clone", "-q", "--bare", src, bare)
	linked := filepath.Join(holder, "work")
	gitIn(t, bare, "worktree", "add", "-q", "-b", "lane", linked)
	t.Setenv("PATH", gitPath(t, allTools...))

	t.Chdir(linked)
	r := doctorFindRepo()
	if r == nil {
		t.Fatal("a linked worktree has a working tree, got no repository")
	}
	if r.main != "" || !strings.Contains(r.mainNote, "bare") {
		t.Errorf("want no main checkout and a note naming the bare worktree, got main %q note %q", r.main, r.mainNote)
	}
}

// [DOCTOR-3-S1] Doctor leaves the repository and the home directory as they were, with drift in
// the repository and an installed panel with an untrusted config in the home.
func TestDoctorWritesNothing(t *testing.T) {
	top := drifted(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	write(t, home, "Library/LaunchAgents/"+install.LaunchdLabel+".plist", "<plist/>\n")
	if _, err := install.AddProject(context.Background(), install.AddOptions{Home: home, Project: top, Strict: true, Now: time.Now()}); err != nil {
		t.Fatalf("registering the project: %v", err)
	}
	oldGOOS := doctorGOOS
	doctorGOOS = "darwin"
	t.Cleanup(func() { doctorGOOS = oldGOOS })

	t.Chdir(filepath.Join(top, "docs"))
	repoBefore, homeBefore := treesnap.Take(t, top), treesnap.Take(t, home)
	out, _ := doctor(t, gitPath(t, allTools...))
	needLine(t, out, "warn", "drift") // the run found the drift it must not touch
	if d := treesnap.Diff(repoBefore, treesnap.Take(t, top)); len(d) > 0 {
		t.Errorf("doctor changed the repository: %v", d)
	}
	if d := treesnap.Diff(homeBefore, treesnap.Take(t, home)); len(d) > 0 {
		t.Errorf("doctor changed the home directory: %v", d)
	}
}
