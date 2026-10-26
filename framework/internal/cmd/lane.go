package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/clauductor/clauductor/internal/panel/config"
	"github.com/clauductor/clauductor/internal/panel/install"
	"github.com/clauductor/clauductor/internal/panel/lanes"
	"github.com/clauductor/clauductor/internal/panel/lease"
	"github.com/clauductor/clauductor/internal/panel/signals"
	"github.com/spf13/cobra"
)

// `clauductor lane close` (PANEL-57) closes a finished panel lane from the command
// line, through the running panel's command socket and Close lane's own plan and
// guards. The panel decides what may close (D2); this command only finds the panel and
// the project, shows the plan, and with --yes confirms it.

// laneProc tells whether the running panel's pid is alive, and its start time; a test
// replaces it so a stub panel can stand in for a real one.
var laneProc lease.ProcCheck = lease.LiveProc

// laneRun runs the git that finds the current directory's main checkout; a test points
// it at a runner without the process's GIT_DIR, which a git hook exports and which would
// aim the read at the outer repository.
var laneRun signals.Runner = signals.ExecRunner

var laneCloseYes bool

var laneCmd = &cobra.Command{
	Use:   "lane",
	Short: "Act on a lane of the running panel",
}

var laneCloseCmd = &cobra.Command{
	Use:   "close <id>",
	Short: "Close a finished lane of the running panel: its plan, then with --yes the close",
	Long: `Ask the running panel to close lane <id> of the project the current directory is
in (any of its checkouts: the main one, a lane's worktree, a subdirectory), as the
page's Close lane does. Without --yes it prints the plan (what would be removed and
what kept, each with why) and changes nothing; with --yes it closes the lane with the
plan's offers as the consent, and prints what was removed and what kept.

The panel refuses a lane whose work has not ended (claude working, waiting on you,
starting, not read yet, background work in flight, a queue's lease) or, unless its
claude has exited, whose branch is not merged. A lane does not close itself. Any
refusal exits 1 with the panel's reason.`,
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLaneClose(cmd.Context(), cmd.OutOrStdout(), os.UserHomeDir, args[0], laneCloseYes)
	},
}

func init() {
	laneCloseCmd.Flags().BoolVar(&laneCloseYes, "yes", false, "close the lane as the plan says (without it, only the plan is printed)")
	laneCmd.AddCommand(laneCloseCmd)
	rootCmd.AddCommand(laneCmd)
}

// laneSelfWords refuses a lane's own close (D2): fixed words, since the command reads
// no config to say which of these is on for the lane's type.
func laneSelfWords(id string) error {
	return fmt.Errorf("lane %s is the lane this command runs in (CLAUDUCTOR_LANE), and a lane does not close itself: "+
		"it is closed after its work by the panel page's Close lane, or by auto-close where auto-close is on for the lane's type", id)
}

// runLaneClose is `clauductor lane close <id> [--yes]`, run from the current directory
// with CLAUDUCTOR_LANE read from the environment. homeDir is asked only after the self
// refusal, which reads nothing (D2). A returned error is the exit 1.
func runLaneClose(ctx context.Context, out io.Writer, homeDir func() (string, error), id string, yes bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Before anything is read: the caller's own lane (E8).
	if self := os.Getenv("CLAUDUCTOR_LANE"); self != "" && self == id {
		return laneSelfWords(id)
	}
	home, err := homeDir()
	if err != nil {
		return err
	}

	// The panel, checked first: the live one, at the socket its owner record names.
	owner := install.RunningPanel(ctx, home, os.Getpid(), laneProc)
	switch {
	case owner == nil:
		return errors.New("no clauductor panel is running on this machine, so nothing was closed: `clauductor panel` starts one")
	case owner.Socket == "":
		return fmt.Errorf("the running clauductor panel (pid %d) has no command socket, so nothing was closed: "+
			"it could not make one (its output says why) or it is older than `clauductor lane close`; restart it with `clauductor panel`", owner.PID)
	}

	// The project: the current directory's repository, resolved to its main checkout
	// (a lane's worktree names its project), matched by root.
	project, err := laneProject(ctx, home, owner)
	if err != nil {
		return err
	}

	// The plan first (D3): a lane the panel would refuse is refused here too.
	ans, err := install.CloseLane(ctx, owner.Socket, project.ID, id, lanes.CloseRequest{DryRun: true})
	if err != nil {
		return err
	}
	plan := ans.Plan
	printPlan(out, id, plan)
	if !yes {
		return nil
	}
	ans, err = install.CloseLane(ctx, owner.Socket, project.ID, id, lanes.CloseRequest{Worktree: plan.Worktree, Branch: plan.DeleteBranch})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Closed lane %s.\n", id)
	printList(out, "Removed", ans.Result.Removed)
	printList(out, "Kept", ans.Result.Kept)
	return nil
}

// laneProject is the registered project of the current directory, refused naming
// `clauductor panel list` when it is in none, or in one the running panel does not
// serve.
func laneProject(ctx context.Context, home string, owner *install.PanelOwner) (*config.ProjectEntry, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	root, _, err := install.MainWorktree(ctx, laneRun, wd)
	if err != nil {
		return nil, fmt.Errorf("%v, so it is in no project the panel serves: run this in one of them (`clauductor panel list` names them)", err)
	}
	reg, err := config.LoadProjects(home)
	if err != nil {
		return nil, err
	}
	e := reg.Find(root)
	if e == nil {
		return nil, fmt.Errorf("%s is not a registered project, so nothing was closed: run this in a project the panel serves (`clauductor panel list` names them)", root)
	}
	if !slices.Contains(owner.Projects, e.Root) {
		return nil, fmt.Errorf("project %s (%s) is registered, but the running panel does not serve it, so nothing was closed: `clauductor panel list` names the projects", e.ID, e.Root)
	}
	return e, nil
}

// printPlan prints a close's plan as the page's confirmation lists it, after a line
// naming the lane and its branch.
func printPlan(out io.Writer, id string, p *lanes.ClosePlan) {
	if p.Branch != "" {
		fmt.Fprintf(out, "lane %s on %s\n", id, p.Branch)
	} else {
		fmt.Fprintf(out, "lane %s, its branch not known\n", id)
	}
	printList(out, "Removes", p.Remove)
	printList(out, "Keeps", p.Keep)
	for _, n := range p.Notes {
		fmt.Fprintln(out, n)
	}
}

// printList prints a titled list, one line per entry; nothing when it is empty, as the
// page shows none.
func printList(out io.Writer, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintln(out, title+":")
	for _, l := range lines {
		fmt.Fprintln(out, "  - "+l)
	}
}
