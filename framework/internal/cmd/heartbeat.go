package cmd

import (
	"fmt"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/spf13/cobra"
)

var heartbeatCmd = &cobra.Command{
	Use:   "heartbeat",
	Short: "Send a heartbeat for a worker",
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

		if err := db.Heartbeat(workerID); err != nil {
			return fmt.Errorf("heartbeat failed: %w", err)
		}

		fmt.Printf("Heartbeat recorded for worker %s\n", workerID)
		return nil
	},
}

func init() {
	heartbeatCmd.Flags().String("worker-id", "", "Worker ID to heartbeat")
}
