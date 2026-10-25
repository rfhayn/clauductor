package cmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/template"
	"github.com/clauductor/clauductor/internal/testbin"
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
