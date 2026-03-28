package cmd

import (
	"fmt"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/spf13/cobra"
)

var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Release file locks",
	Long:  "Release all locks for a worker (--worker-id) or a specific file lock (--file).",
	RunE: func(cmd *cobra.Command, args []string) error {
		workerID, _ := cmd.Flags().GetString("worker-id")
		file, _ := cmd.Flags().GetString("file")

		if workerID == "" && file == "" {
			return fmt.Errorf("--worker-id or --file is required")
		}

		db, err := state.Open("")
		if err != nil {
			return err
		}
		defer db.Close()

		if file != "" {
			if err := db.UnlockFile(file); err != nil {
				return err
			}
			_ = db.AppendEvent("", "file_unlocked", fmt.Sprintf("file=%s", file))
			fmt.Printf("Unlocked: %s\n", file)
			return nil
		}

		n, err := db.UnlockByWorker(workerID)
		if err != nil {
			return err
		}
		_ = db.AppendEvent(workerID, "locks_released", fmt.Sprintf("count=%d", n))
		fmt.Printf("Released %d lock(s) for worker %s\n", n, workerID)
		return nil
	},
}

func init() {
	unlockCmd.Flags().String("worker-id", "", "Release all locks for this worker")
	unlockCmd.Flags().String("file", "", "Release the lock on this specific file")
}
