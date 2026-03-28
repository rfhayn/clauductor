package cmd

import (
	"fmt"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/spf13/cobra"
)

var eventCmd = &cobra.Command{
	Use:   "event",
	Short: "Log an event to the orchestration database",
	RunE: func(cmd *cobra.Command, args []string) error {
		workerID, _ := cmd.Flags().GetString("worker-id")
		eventType, _ := cmd.Flags().GetString("type")
		detail, _ := cmd.Flags().GetString("detail")

		if eventType == "" {
			return fmt.Errorf("--type is required")
		}

		db, err := state.Open("")
		if err != nil {
			return err
		}
		defer db.Close()

		if err := db.AppendEvent(workerID, eventType, detail); err != nil {
			return fmt.Errorf("logging event: %w", err)
		}

		fmt.Printf("Event logged: type=%s worker=%s\n", eventType, workerID)
		return nil
	},
}

func init() {
	eventCmd.Flags().String("worker-id", "", "Worker associated with the event")
	eventCmd.Flags().String("type", "", "Event type")
	eventCmd.Flags().String("detail", "", "Event detail/description")
}
