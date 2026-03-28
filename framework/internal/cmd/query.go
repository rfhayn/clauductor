package cmd

import (
	"encoding/json"
	"os"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/spf13/cobra"
)

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query orchestration state (JSON output)",
	Long:  "Query workers, locks, events, or milestones. Output is JSON for easy parsing.",
}

var queryWorkersCmd = &cobra.Command{
	Use:   "workers",
	Short: "List all workers as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := state.Open("")
		if err != nil {
			return err
		}
		defer db.Close()

		workers, err := db.ListWorkers()
		if err != nil {
			return err
		}
		if workers == nil {
			workers = []state.Worker{}
		}
		return writeJSON(workers)
	},
}

var queryLocksCmd = &cobra.Command{
	Use:   "locks",
	Short: "List all locks as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := state.Open("")
		if err != nil {
			return err
		}
		defer db.Close()

		locks, err := db.ListLocks()
		if err != nil {
			return err
		}
		if locks == nil {
			locks = []state.Lock{}
		}
		return writeJSON(locks)
	},
}

var queryEventsCmd = &cobra.Command{
	Use:   "events",
	Short: "List recent events as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		limit, _ := cmd.Flags().GetInt("limit")
		workerID, _ := cmd.Flags().GetString("worker-id")
		eventType, _ := cmd.Flags().GetString("type")

		db, err := state.Open("")
		if err != nil {
			return err
		}
		defer db.Close()

		var events []state.Event
		switch {
		case workerID != "":
			events, err = db.QueryEventsByWorker(workerID, limit)
		case eventType != "":
			events, err = db.QueryEventsByType(eventType, limit)
		default:
			events, err = db.ListRecentEvents(limit)
		}
		if err != nil {
			return err
		}
		if events == nil {
			events = []state.Event{}
		}
		return writeJSON(events)
	},
}

var queryMilestonesCmd = &cobra.Command{
	Use:   "milestones",
	Short: "List all milestones as JSON",
	RunE: func(cmd *cobra.Command, args []string) error {
		db, err := state.Open("")
		if err != nil {
			return err
		}
		defer db.Close()

		milestones, err := db.ListMilestones()
		if err != nil {
			return err
		}
		if milestones == nil {
			milestones = []state.Milestone{}
		}
		return writeJSON(milestones)
	},
}

func writeJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func init() {
	queryEventsCmd.Flags().Int("limit", 50, "Maximum number of events to return")
	queryEventsCmd.Flags().String("worker-id", "", "Filter events by worker ID")
	queryEventsCmd.Flags().String("type", "", "Filter events by type")

	queryCmd.AddCommand(queryWorkersCmd)
	queryCmd.AddCommand(queryLocksCmd)
	queryCmd.AddCommand(queryEventsCmd)
	queryCmd.AddCommand(queryMilestonesCmd)
}
