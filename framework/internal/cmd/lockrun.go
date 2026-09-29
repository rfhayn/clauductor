package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/clauductor/clauductor/internal/panel"
	"github.com/spf13/cobra"
)

var (
	lockRunLane string
	lockRunTTL  time.Duration
)

// lockRunCmd is standalone like the panel: no SQLite, no install. A project's gate
// script calls it, so it must work whether or not the panel is running.
var lockRunCmd = &cobra.Command{
	Use:   "lock-run [--lane <id>] [--ttl 10m] <lockdir> -- <command> [args...]",
	Short: "Run a command while holding a shared lease (the gate queue); wait in line if it is taken",
	Long: `Wait in a FIFO queue for the lease <lockdir>, run the command while holding it,
then release it. The lease is a directory created with mkdir (atomic on every
filesystem); <lockdir>/owner.json names the holder (pid, lane, start, a TTL it
renews every ttl/3). A holder whose pid is gone, or whose lease was not renewed
for its TTL, is reclaimed by the next waiter. A live holder is never signalled.

Exit status: the command's own (128+n if a signal ended it); 75 if the wait was
cancelled from the panel; 130 if interrupted while waiting. See docs/panel.md.`,
	Args:                  cobra.MinimumNArgs(2),
	SilenceUsage:          true,
	SilenceErrors:         true,
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dash := cmd.ArgsLenAtDash()
		if dash != 1 || len(args) < 2 {
			return fmt.Errorf("usage: clauductor lock-run [--lane id] [--ttl 10m] <lockdir> -- <command> [args...]")
		}
		code, err := panel.LockRun(context.Background(), panel.LockRunOptions{
			Lock: args[0], Lane: lockRunLane, TTL: lockRunTTL, Argv: args[1:],
			Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(),
		})
		if err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), err)
		}
		os.Exit(code)
		return nil
	},
}

func init() {
	lockRunCmd.Flags().StringVar(&lockRunLane, "lane", "", "the lane shown to others (default $CLAUDUCTOR_LANE)")
	lockRunCmd.Flags().DurationVar(&lockRunTTL, "ttl", panel.DefaultLeaseTTL, "lease TTL; renewed every ttl/3 while the command runs")
	rootCmd.AddCommand(lockRunCmd)
}
