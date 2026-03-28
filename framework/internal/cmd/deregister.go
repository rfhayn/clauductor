package cmd

import (
	"fmt"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/spf13/cobra"
)

var deregisterCmd = &cobra.Command{
	Use:   "deregister",
	Short: "Remove a worker and release its locks",
	RunE: func(cmd *cobra.Command, args []string) error {
		workerID, _ := cmd.Flags().GetString("worker-id")
		if workerID == "" {
			return fmt.Errorf("--worker-id is required")
		}

		db, err := state.Open("")
		if err != nil {
			return err
		}
		defer db.Close()

		if err := db.DeregisterWorker(workerID); err != nil {
			return fmt.Errorf("deregistering worker: %w", err)
		}

		if err := db.AppendEvent(workerID, "worker_deregistered", ""); err != nil {
			// Worker is already gone, event FK may fail; log to stderr but don't error
			fmt.Printf("Warning: could not log event: %v\n", err)
		}

		fmt.Printf("Worker deregistered: %s (locks released)\n", workerID)
		return nil
	},
}

func init() {
	deregisterCmd.Flags().String("worker-id", "", "ID of the worker to remove")
}
