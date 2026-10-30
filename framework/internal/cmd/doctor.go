package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rfhayn/stridal/internal/panel/install"
	"github.com/rfhayn/stridal/internal/panel/lease"
	"github.com/rfhayn/stridal/internal/template"
	"github.com/spf13/cobra"
)

// What doctor reads about the machine, through variables so a test can stand in for another
// operating system or another build. The panel's lines (macOS only) use all three.
var (
	doctorGOOS         = runtime.GOOS
	doctorRunningStamp = install.RunningStamp
	doctorFileStamp    = install.FileStamp
	// doctorLiveProc tells RunningPanel whether the panel's pid is alive.
	doctorLiveProc = lease.LiveProc
)

// errDoctorFailed makes `doctor` exit 1 when any line fails, as `diff --exit-code` does. It is
// reported: the report's last line is the count, so neither cobra nor Main prints an error after it.
var errDoctorFailed = fmt.Errorf("%w: doctor found a problem that stops Clauductor working here (see the FAIL lines)", errReported)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check this machine's setup and the project's, and say what to do about each problem",
	Long: `Check that Clauductor can work here, changing nothing: the tools the operating model
uses, the template this binary reads, whether gh is signed in, and (on macOS) the panel. In a
git repository it also checks whose operating model the repository runs, whether Clauductor's
matches the template, and whether the panel serves and trusts the project.

Each line is ok, FAIL, warn or note. A problem carries the command that fixes it (fix:), or
for a missing tool this system's install hint (install:), when doctor can tell what clears it. FAIL
means Clauductor cannot work here until it is fixed; warn means something is degraded; a note
is a fact. Doctor exits 1 when any line fails. It writes nothing and runs none of the fixes it
prints: each is a command you should see the effect of.`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		rep := &doctorReport{}
		checkMachine(rep)
		checkPanel(rep, checkProject(rep))
		rep.print(cmd.OutOrStdout())
		if fail, _ := rep.counts(); fail > 0 {
			return errDoctorFailed
		}
		return nil
	},
}

func init() {
	addTemplateFlag(doctorCmd)
	rootCmd.AddCommand(doctorCmd)
}

// The groups a report prints, in order.
const (
	groupMachine = "Machine"
	groupProject = "Project"
)

type doctorLevel int

// Ordered by severity; the zero value is a line with nothing wrong.
const (
	levelOK doctorLevel = iota
	levelNote
	levelWarn
	levelFail
)

var levelLabels = map[doctorLevel]string{levelOK: "ok", levelNote: "note", levelWarn: "warn", levelFail: "FAIL"}

// doctorLine is one check's result: what it checked, what it found, and the commands that fix it.
// A fix is a command to paste as it stands (D8). A missing tool's install hint is prereqs.sh's
// text for people, often prose ("see https://...", "(or your distribution's package manager)"),
// so it is install, never a fix.
type doctorLine struct {
	level   doctorLevel
	subject string
	detail  string
	fixes   []string
	install string
}

type doctorGroup struct {
	name  string
	lines []doctorLine
}

type doctorReport struct {
	groups []*doctorGroup
	// checks is what doctor names and does not run (the model's own checks, D4), when the
	// repository holds Clauductor's model. It is appended to the count line, so the count is the
	// last line in every report and that line names the checks.
	checks string
}

func (r *doctorReport) add(group string, l doctorLine) {
	for _, g := range r.groups {
		if g.name == group {
			g.lines = append(g.lines, l)
			return
		}
	}
	r.groups = append(r.groups, &doctorGroup{name: group, lines: []doctorLine{l}})
}

// counts is the failures and the warnings; notes are neither (a fact, not a problem).
func (r *doctorReport) counts() (fail, warn int) {
	for _, g := range r.groups {
		for _, l := range g.lines {
			switch l.level {
			case levelFail:
				fail++
			case levelWarn:
				warn++
			}
		}
	}
	return fail, warn
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// The widths of a line's level and subject columns. A line's text starts at textColumn, and every
// line under it (a continuation, a fix, an install hint) is indented from the same widths.
const (
	levelWidth   = 4 // "FAIL"
	subjectWidth = 10
	textColumn   = 2 + levelWidth + 2 + subjectWidth + 1
)

// hang prints text after prefix at column indent, each further line of text under its first.
func hang(out io.Writer, indent int, prefix, text string) {
	pad := strings.Repeat(" ", indent)
	cont := "\n" + pad + strings.Repeat(" ", len(prefix))
	fmt.Fprintf(out, "%s%s%s\n", pad, prefix, strings.ReplaceAll(strings.TrimSpace(text), "\n", cont))
}

func (r *doctorReport) print(out io.Writer) {
	for _, g := range r.groups {
		fmt.Fprintf(out, "%s\n", g.name)
		for _, l := range g.lines {
			// A multi-line detail (Resolve's error) hangs under the first line's text.
			hang(out, 0, fmt.Sprintf("  %-*s  %-*s ", levelWidth, levelLabels[l.level], subjectWidth, l.subject), l.detail)
			for _, f := range l.fixes {
				hang(out, textColumn, "fix: ", f)
			}
			if l.install != "" {
				hang(out, textColumn, "install: ", l.install)
			}
		}
		fmt.Fprintln(out)
	}
	fail, warn := r.counts()
	count := "Nothing missing or broken."
	if fail > 0 || warn > 0 {
		count = fmt.Sprintf("%d failed, %s", fail, plural(warn, "warning", "warnings"))
	}
	if r.checks != "" {
		count = strings.TrimSuffix(count, ".") + ". The model's own checks: " + r.checks
	}
	fmt.Fprintln(out, count)
}

// checkMachine adds the machine group: the tools, the template, gh. It needs no project.
func checkMachine(rep *doctorReport) {
	src, err := template.Resolve()
	checkTools(rep, src, err)
	checkTemplate(rep, src, err)
	checkGH(rep)
}

// checkTemplate is the template line: Resolve's own error (which names what to do), or its
// warning (a --template-dir whose version is not the binary's).
func checkTemplate(rep *doctorReport, src *template.Source, err error) {
	switch {
	case err != nil:
		rep.add(groupMachine, doctorLine{level: levelFail, subject: "template", detail: err.Error()})
	case src.Warning != "":
		rep.add(groupMachine, doctorLine{level: levelWarn, subject: "template", detail: src.Warning + "\n" + src.String()})
	default:
		rep.add(groupMachine, doctorLine{level: levelOK, subject: "template", detail: src.String()})
	}
}

// toolRow is one line of `prereqs.sh --rows`.
type toolRow struct {
	ok                         bool
	level, tool, where, reason string
}

// parseToolRows reads `prereqs.sh --rows` strictly: at least one line, every line five
// tab-separated fields with a known status and level. A report written for people (a template
// older than --rows) fails here, so it is never read as an empty list of tools.
func parseToolRows(out string) ([]toolRow, error) {
	if strings.TrimSpace(out) == "" {
		return nil, errors.New("it printed nothing")
	}
	var rows []toolRow
	for i, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 5 {
			return nil, fmt.Errorf("line %d is not five tab-separated fields", i+1)
		}
		if f[0] != "ok" && f[0] != "missing" {
			return nil, fmt.Errorf("line %d has status %q, not ok or missing", i+1, f[0])
		}
		switch f[1] {
		case "required", "recommended", "optional":
		default:
			return nil, fmt.Errorf("line %d has level %q, not required, recommended or optional", i+1, f[1])
		}
		if f[2] == "" {
			return nil, fmt.Errorf("line %d names no tool", i+1)
		}
		rows = append(rows, toolRow{ok: f[0] == "ok", level: f[1], tool: f[2], where: f[3], reason: f[4]})
	}
	return rows, nil
}

// checkTools adds one line per tool from the resolved template's prereqs.sh, the one list of
// tools the model uses (prereqs.go). Without a template, or with a script that prints no rows,
// it is a note saying the tools could not be read: never an empty list.
func checkTools(rep *doctorReport, src *template.Source, resolveErr error) {
	note := func(why string) {
		rep.add(groupMachine, doctorLine{level: levelNote, subject: "tools", detail: why})
	}
	if resolveErr != nil {
		note("could not be checked: no template to read " + PrereqsScript + " from (see the template line)")
		return
	}
	script := filepath.Join(src.Dir, filepath.FromSlash(PrereqsScript))
	if _, err := os.Stat(script); err != nil {
		note("could not be checked: " + script + " does not exist")
		return
	}
	// /bin/sh, not a PATH lookup: the report is about what PATH lacks.
	c := execCommand("/bin/sh", script, "--rows")
	var stdout bytes.Buffer
	c.Stdout = &stdout
	if err := c.Run(); err != nil {
		note(fmt.Sprintf("could not be checked: %s did not run: %v", script, err))
		return
	}
	rows, err := parseToolRows(stdout.String())
	if err != nil {
		note(fmt.Sprintf("could not be read from %s --rows (%v); a template older than this binary prints a report for people instead", script, err))
		return
	}
	for _, r := range rows {
		if r.ok {
			rep.add(groupMachine, doctorLine{level: levelOK, subject: r.tool, detail: r.where})
			continue
		}
		lvl := map[string]doctorLevel{"required": levelFail, "recommended": levelWarn, "optional": levelNote}[r.level]
		rep.add(groupMachine, doctorLine{
			level:   lvl,
			subject: r.tool,
			detail:  fmt.Sprintf("missing (%s): %s", r.level, r.reason),
			install: r.where,
		})
	}
}

// checkGH says whether gh, when on PATH, is signed in, from the exit status of `gh auth token`:
// any non-zero exit is a warn naming `gh auth login` (the design's table). The token it prints
// on stdout is a secret, so both of gh's streams are discarded (a nil Stdout and Stderr are
// /dev/null), never captured and never the terminal doctor runs in.
func checkGH(rep *doctorReport) {
	if _, err := exec.LookPath("gh"); err != nil {
		return // the tools lines already say gh is missing
	}
	c := execCommand("gh", "auth", "token")
	c.Stdout, c.Stderr = nil, nil
	err := c.Run()
	if err == nil {
		rep.add(groupMachine, doctorLine{level: levelOK, subject: "gh", detail: "signed in"})
		return
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		rep.add(groupMachine, doctorLine{level: levelNote, subject: "gh", detail: fmt.Sprintf("could not tell whether gh is signed in: gh auth token did not run: %v", err)})
		return
	}
	rep.add(groupMachine, doctorLine{level: levelWarn, subject: "gh",
		detail: "not signed in (gh auth token exited non-zero); merge-pr and the PR flow need it",
		fixes:  []string{"gh auth login"}})
}
