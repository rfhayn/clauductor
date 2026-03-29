package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export orchestration data in various formats",
	Long:  "Export workers, events, or a summary of orchestration state as JSON or markdown.",
}

var exportEventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Export events as JSON or markdown",
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		since, _ := cmd.Flags().GetString("since")
		limit, _ := cmd.Flags().GetInt("limit")

		db, err := state.Open("")
		if err != nil {
			return err
		}
		defer db.Close()

		var events []state.Event
		if since != "" {
			events, err = queryEventsSince(db, since, limit)
		} else {
			events, err = db.ListRecentEvents(limit)
		}
		if err != nil {
			return err
		}
		if events == nil {
			events = []state.Event{}
		}

		switch format {
		case "markdown":
			return writeEventsMarkdown(events)
		default:
			return writeJSON(events)
		}
	},
}

var exportWorkersCmd = &cobra.Command{
	Use:   "workers",
	Short: "Export workers as JSON or markdown",
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")

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

		switch format {
		case "markdown":
			return writeWorkersMarkdown(workers)
		default:
			return writeJSON(workers)
		}
	},
}

var exportSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "High-level orchestration summary",
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
		locks, err := db.ListLocks()
		if err != nil {
			return err
		}
		events, err := db.ListRecentEvents(0)
		if err != nil {
			return err
		}
		milestones, err := db.ListMilestones()
		if err != nil {
			return err
		}

		// Worker status breakdown
		workerStatus := map[string]int{}
		for _, w := range workers {
			workerStatus[w.Status]++
		}

		// Milestone status breakdown
		milestoneStatus := map[string]int{}
		for _, m := range milestones {
			milestoneStatus[m.Status]++
		}

		summary := map[string]interface{}{
			"workers": map[string]interface{}{
				"total":    len(workers),
				"by_status": workerStatus,
			},
			"locks": map[string]interface{}{
				"total": len(locks),
			},
			"events": map[string]interface{}{
				"total": len(events),
			},
			"milestones": map[string]interface{}{
				"total":    len(milestones),
				"by_status": milestoneStatus,
			},
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(summary)
	},
}

// queryEventsSince returns events with timestamp >= the given date string.
func queryEventsSince(db *state.DB, since string, limit int) ([]state.Event, error) {
	if limit <= 0 {
		limit = 1000
	}
	rows, err := db.Conn().Query(
		`SELECT id, timestamp, COALESCE(worker_id,''), event_type, COALESCE(detail,'') FROM events WHERE timestamp >= ? ORDER BY id DESC LIMIT ?`,
		since, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []state.Event
	for rows.Next() {
		var e state.Event
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.WorkerID, &e.EventType, &e.Detail); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func writeEventsMarkdown(events []state.Event) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "| ID\t| Timestamp\t| Worker\t| Type\t| Detail\t|")
	fmt.Fprintln(w, "| ---\t| ---\t| ---\t| ---\t| ---\t|")
	for _, e := range events {
		detail := e.Detail
		if len(detail) > 60 {
			detail = detail[:57] + "..."
		}
		detail = strings.ReplaceAll(detail, "|", "\\|")
		fmt.Fprintf(w, "| %d\t| %s\t| %s\t| %s\t| %s\t|\n",
			e.ID, e.Timestamp, e.WorkerID, e.EventType, detail)
	}
	return w.Flush()
}

func writeWorkersMarkdown(workers []state.Worker) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "| ID\t| Name\t| Type\t| Milestone\t| Status\t| Owner\t| Started\t| Last Heartbeat\t|")
	fmt.Fprintln(w, "| ---\t| ---\t| ---\t| ---\t| ---\t| ---\t| ---\t| ---\t|")
	for _, wr := range workers {
		fmt.Fprintf(w, "| %s\t| %s\t| %s\t| %s\t| %s\t| %s\t| %s\t| %s\t|\n",
			wr.ID, wr.Name, wr.SessionType, wr.Milestone, wr.Status, wr.Owner, wr.StartedAt, wr.LastHeartbeat)
	}
	return w.Flush()
}

func init() {
	exportEventsCmd.Flags().String("format", "json", "Output format: json or markdown")
	exportEventsCmd.Flags().String("since", "", "Filter events since date (YYYY-MM-DD)")
	exportEventsCmd.Flags().Int("limit", 50, "Maximum number of events to return")

	exportWorkersCmd.Flags().String("format", "json", "Output format: json or markdown")

	exportCmd.AddCommand(exportEventsCmd)
	exportCmd.AddCommand(exportWorkersCmd)
	exportCmd.AddCommand(exportSummaryCmd)
}
