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
// tool, or a report that cannot run, is printed and the install goes on.
func printPrereqs(out io.Writer, tmplDir string) {
	script := filepath.Join(tmplDir, filepath.FromSlash(PrereqsScript))
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
