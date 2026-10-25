package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clauductor/clauductor/internal/template"
	"github.com/clauductor/clauductor/internal/testbin"
)

// stubPath is a PATH holding only executables named tools, so the report sees exactly those.
func stubPath(t *testing.T, tools ...string) string {
	t.Helper()
	bin := t.TempDir()
	for _, tool := range tools {
		testbin.Write(t, filepath.Join(bin, tool), "#!/bin/sh\nexit 0\n")
	}
	return bin
}

func prereqsReport(t *testing.T, osName string, tools ...string) string {
	t.Helper()
	tmpl := tmplDir(t)
	t.Setenv("PATH", stubPath(t, tools...))
	t.Setenv("PREREQS_OS", osName)
	var out bytes.Buffer
	printPrereqs(&out, tmpl)
	return out.String()
}

func wantAll(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("report lacks %q:\n%s", w, out)
		}
	}
}

// The report names what is present and what is missing, with this OS's install hint.
func TestPrereqsReportsPresentAndMissing(t *testing.T) {
	out := prereqsReport(t, "Darwin", "git", "jq")
	wantAll(t, out,
		"Prerequisites on macOS",
		"ok       git",
		"ok       jq",
		"MISSING  gitleaks  recommended",
		"install: brew install gitleaks",
		"install: brew install gh",
		"MISSING  tmux      optional",
		"6 missing",
		"MISSING  python3   optional",
	)
	if strings.Contains(out, "MISSING  git ") || strings.Contains(out, "MISSING  jq ") {
		t.Errorf("a present tool was reported missing:\n%s", out)
	}
}

func TestPrereqsHintsPerOS(t *testing.T) {
	linux := prereqsReport(t, "Linux")
	wantAll(t, linux, "Prerequisites on Linux", "sudo apt install jq", "github.com/gitleaks/gitleaks/releases", "install_linux.md", "sudo apt install python3", "8 missing")
	if strings.Contains(linux, "xcode-select") {
		t.Error("a Linux report gave a macOS hint")
	}
	wantAll(t, prereqsReport(t, "WSL"), "Prerequisites on Linux (WSL2)", "sudo apt install git")
	wantAll(t, prereqsReport(t, "MINGW64_NT"), "WSL2")
	wantAll(t, prereqsReport(t, "Darwin", "git", "jq", "gh", "gitleaks", "claude", "tmux", "node", "python3"), "All present.")
}

// --rows is the machine-readable form doctor reads (OPS-109 D1): one tab-separated line per
// tool, no heading and no closing line, and the same list the report prints.
func TestPrereqsRowsMode(t *testing.T) {
	tmpl := tmplDir(t)
	bin := stubPath(t, "git", "jq")
	t.Setenv("PATH", bin)
	t.Setenv("PREREQS_OS", "Darwin")
	c := execCommand("/bin/sh", filepath.Join(tmpl, PrereqsScript), "--rows")
	raw, err := c.Output()
	if err != nil {
		t.Fatalf("--rows failed: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) != 8 {
		t.Fatalf("want one line per tool (8), got %d:\n%s", len(lines), raw)
	}
	for _, l := range lines {
		if n := len(strings.Split(l, "\t")); n != 5 {
			t.Errorf("row has %d fields, want 5: %q", n, l)
		}
	}
	for _, want := range []string{
		"ok\trequired\tgit\t" + filepath.Join(bin, "git") + "\t",
		"missing\trecommended\tgitleaks\tbrew install gitleaks\t",
		"missing\toptional\tpython3\tbrew install python3\tmachine-quiet's live-session check",
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("rows lack %q:\n%s", want, raw)
		}
	}
	if strings.Contains(string(raw), "Prerequisites on") || strings.Contains(string(raw), "missing.") {
		t.Errorf("--rows printed the report's heading or closing line:\n%s", raw)
	}
}

// The report warns and never fails: a missing script, or nothing on PATH, still lets install go on.
func TestPrereqsNeverFails(t *testing.T) {
	var out bytes.Buffer
	printPrereqs(&out, t.TempDir())
	wantAll(t, out.String(), "WARNING: no prerequisites report")

	tmplDir(t)
	dir := ownedRepo(t)
	t.Setenv("PREREQS_OS", "Darwin")
	got, err := runInstall(t, dir, false)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, got)
	}
	// install runs the report after it has written the files, from the template it installed.
	wantAll(t, got, "Done! Clauductor framework installed.", "Prerequisites on macOS")
	if strings.Index(got, "Prerequisites on") < strings.Index(got, "Done!") {
		t.Error("the report should follow the install, not precede it")
	}
}

// .gitleaks.toml is the project's: install creates the starter, and neither install nor update
// writes over a project's own.
func TestGitleaksConfigIsCreateOnly(t *testing.T) {
	if got := template.Classify(".gitleaks.toml"); got != template.TierDoc {
		t.Errorf(".gitleaks.toml is the %s tier; it must be the doc tier (create only)", template.TierLabel(got))
	}
	tmpl := tmplDir(t)
	starter, err := os.ReadFile(filepath.Join(tmpl, ".gitleaks.toml"))
	if err != nil {
		t.Fatalf("the template ships no .gitleaks.toml: %v", err)
	}
	if !bytes.Contains(starter, []byte("useDefault = true")) {
		t.Error("the starter .gitleaks.toml does not extend gitleaks' default rules")
	}

	fresh := ownedRepo(t)
	if out, err := runInstall(t, fresh, false); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got, _ := os.ReadFile(filepath.Join(fresh, ".gitleaks.toml")); !bytes.Equal(got, starter) {
		t.Error("install did not create the starter .gitleaks.toml in a project without one")
	}

	own := "[extend]\nuseDefault = true\n\n[allowlist]\npaths = ['''ours/''']\n"
	dir := ownedRepo(t)
	write(t, dir, ".gitleaks.toml", own)
	if out, err := runInstall(t, dir, false); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".gitleaks.toml")); string(got) != own {
		t.Errorf("install wrote over the project's .gitleaks.toml:\n%s", got)
	}

	var out bytes.Buffer
	updateCmd.SetOut(&out)
	old := stdin
	stdin = bufioReader("y\ny\ny\ny\n")
	createMissing = true
	t.Cleanup(func() { updateCmd.SetOut(nil); stdin = old; createMissing = false })
	t.Chdir(dir)
	if err := updateCmd.RunE(updateCmd, nil); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".gitleaks.toml")); string(got) != own {
		t.Errorf("update wrote over the project's .gitleaks.toml:\n%s", got)
	}
}

// The prerequisites report ends with the line naming `clauductor doctor`, in the report install
// and init print, and when the report itself cannot run.
func TestPrereqsEndsWithTheDoctorLine(t *testing.T) {
	last := func(s string) string {
		s = strings.TrimRight(s, "\n")
		return s[strings.LastIndex(s, "\n")+1:]
	}
	const want = "To check this machine and the project again at any time: clauductor doctor"
	for name, out := range map[string]string{
		"all present": prereqsReport(t, "Darwin", "git", "jq", "gh", "gitleaks", "claude", "tmux", "node", "python3"),
		"missing":     prereqsReport(t, "Linux"),
	} {
		if !strings.Contains(last(out), want) {
			t.Errorf("%s: the report's last line is %q, want it to name clauductor doctor:\n%s", name, last(out), out)
		}
	}
	var out bytes.Buffer
	printPrereqs(&out, t.TempDir())
	if !strings.Contains(last(out.String()), want) {
		t.Errorf("with no script the last line is %q:\n%s", last(out.String()), out.String())
	}
	// The script itself names no binary: the plugin's init runs it where there may be none.
	tmpl := tmplDir(t)
	t.Setenv("PATH", stubPath(t))
	t.Setenv("PREREQS_OS", "Darwin")
	raw, err := execCommand("/bin/sh", filepath.Join(tmpl, PrereqsScript)).Output()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("clauductor doctor")) {
		t.Errorf("prereqs.sh prints the doctor line; the Go caller prints it:\n%s", raw)
	}
}
