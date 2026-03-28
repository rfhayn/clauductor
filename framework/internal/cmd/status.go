package cmd

import (
	"fmt"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show human-readable orchestration status",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := state.Open("")
		if err != nil {
			return err
		}
		defer db.Close()

		fmt.Println("=== Clauductor Status ===")
		fmt.Println()

		// Workers
		workers, err := db.ListWorkers()
		if err != nil {
			return err
		}
		fmt.Printf("Workers (%d):\n", len(workers))
		if len(workers) == 0 {
			fmt.Println("  (none)")
		}
		for _, w := range workers {
			fmt.Printf("  %-20s  type=%-8s  status=%-10s  milestone=%-10s  owner=%s\n",
				w.ID, w.SessionType, w.Status, w.Milestone, w.Owner)
		}
		fmt.Println()

		// Locks
		locks, err := db.ListLocks()
		if err != nil {
			return err
		}
		fmt.Printf("Locked Files (%d):\n", len(locks))
		if len(locks) == 0 {
			fmt.Println("  (none)")
		}
		for _, l := range locks {
			fmt.Printf("  %-40s  worker=%-15s  milestone=%s\n", l.FilePath, l.WorkerID, l.Milestone)
		}
		fmt.Println()

		// Recent events
		events, err := db.ListRecentEvents(10)
		if err != nil {
			return err
		}
		fmt.Printf("Recent Events (last %d):\n", len(events))
		if len(events) == 0 {
			fmt.Println("  (none)")
		}
		for _, e := range events {
			worker := e.WorkerID
			if worker == "" {
				worker = "-"
			}
			fmt.Printf("  [%s] %-20s  worker=%-15s  %s\n", e.Timestamp, e.EventType, worker, e.Detail)
		}

		return nil
	},
}
