package cmd

import (
	"fmt"
	"strings"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/spf13/cobra"
)

var lockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Lock files for a worker",
	RunE: func(cmd *cobra.Command, args []string) error {
		workerID, _ := cmd.Flags().GetString("worker-id")
		milestone, _ := cmd.Flags().GetString("milestone")
		filesStr, _ := cmd.Flags().GetString("files")

		if workerID == "" || milestone == "" || filesStr == "" {
			return fmt.Errorf("--worker-id, --milestone, and --files are required")
		}

		files := strings.Split(filesStr, ",")
		for i := range files {
			files[i] = strings.TrimSpace(files[i])
		}

		db, err := state.Open("")
		if err != nil {
			return err
		}
		defer db.Close()

		conflicts, err := db.LockFiles(workerID, milestone, files)
		if err != nil {
			return fmt.Errorf("locking files: %w", err)
		}

		if len(conflicts) > 0 {
			fmt.Println("CONFLICTS detected — no locks acquired:")
			for _, c := range conflicts {
				fmt.Printf("  %s (locked by %s)\n", c.FilePath, c.Owner)
			}
			return fmt.Errorf("lock conflicts found")
		}

		_ = db.AppendEvent(workerID, "files_locked", fmt.Sprintf("milestone=%s files=%s", milestone, filesStr))

		fmt.Printf("Locked %d file(s) for worker %s:\n", len(files), workerID)
		for _, f := range files {
			fmt.Printf("  %s\n", f)
		}
		return nil
	},
}

func init() {
	lockCmd.Flags().String("worker-id", "", "Worker requesting the lock")
	lockCmd.Flags().String("milestone", "", "Milestone context for the lock")
	lockCmd.Flags().String("files", "", "Comma-separated list of files to lock")
}
