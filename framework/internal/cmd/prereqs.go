package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// PrereqsScript is the template's prerequisites report. It is a POSIX script, not Go, because
// the plugin's /clauductor:init and install.sh print the same report with no binary at hand:
// one list of tools and hints, and every caller runs it.
const PrereqsScript = ".claude/prereqs.sh"

// printPrereqs runs the template's prerequisites report into out. It only ever warns: a missing
// tool, or a report that cannot run, is printed and the install goes on. It ends with the line
// naming `clauductor doctor`, which checks the machine and the project again at any time. That
// line is printed here, in Go, and not by the script: the plugin's /clauductor:init and install.sh
// run the same script, and a plugin user may have no clauductor binary.
func printPrereqs(out io.Writer, tmplDir string) {
	script := filepath.Join(tmplDir, filepath.FromSlash(PrereqsScript))
	defer fmt.Fprintln(out, "  To check this machine and the project again at any time: clauductor doctor")
	fmt.Fprintln(out)
	if _, err := os.Stat(script); err != nil {
		fmt.Fprintf(out, "  WARNING: no prerequisites report (%s is not in the template).\n", PrereqsScript)
		return
	}
	// /bin/sh, not a PATH lookup: the report is about what PATH lacks.
	c := execCommand("/bin/sh", script)
	c.Stdout, c.Stderr = out, out
	if err := c.Run(); err != nil {
		fmt.Fprintf(out, "  WARNING: the prerequisites report did not run: %v\n", err)
	}
}
