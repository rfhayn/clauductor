package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/spf13/cobra"
)

// contextSnapshot is the composed view returned by the context command.
type contextSnapshot struct {
	Workers    []state.Worker    `json:"workers"`
	Locks      []state.Lock      `json:"locks"`
	Events     []state.Event     `json:"events"`
	Milestones []state.Milestone `json:"milestones"`
}

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Return complete orchestration snapshot",
	Long: `Returns a composed view of workers, locks, recent events, and milestones.
Used by the /spawn skill to embed orchestration state in initial prompts.

Default output is human-readable. Use --json for machine-readable output.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOutput, _ := cmd.Flags().GetBool("json")
		eventLimit, _ := cmd.Flags().GetInt("events")

		db, err := state.Open("")
		if err != nil {
			if jsonOutput {
				return writeJSON(contextSnapshot{
					Workers:    []state.Worker{},
					Locks:      []state.Lock{},
					Events:     []state.Event{},
					Milestones: []state.Milestone{},
				})
			}
			fmt.Println("No orchestration database found.")
			return nil
		}
		defer db.Close()

		workers, err := db.ListWorkers()
		if err != nil {
			return fmt.Errorf("list workers: %w", err)
		}
		if workers == nil {
			workers = []state.Worker{}
		}

		locks, err := db.ListLocks()
		if err != nil {
			return fmt.Errorf("list locks: %w", err)
		}
		if locks == nil {
			locks = []state.Lock{}
		}

		events, err := db.ListRecentEvents(eventLimit)
		if err != nil {
			return fmt.Errorf("list events: %w", err)
		}
		if events == nil {
			events = []state.Event{}
		}

		milestones, err := db.ListMilestones()
		if err != nil {
			return fmt.Errorf("list milestones: %w", err)
		}
		if milestones == nil {
			milestones = []state.Milestone{}
		}

		snap := contextSnapshot{
			Workers:    workers,
			Locks:      locks,
			Events:     events,
			Milestones: milestones,
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(snap)
		}

		return printContext(snap)
	},
}

func printContext(snap contextSnapshot) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)

	fmt.Fprintln(w, "== Workers ==")
	if len(snap.Workers) == 0 {
		fmt.Fprintln(w, "(none)")
	} else {
		fmt.Fprintln(w, "ID\tType\tMilestone\tStatus\tOwner")
		for _, wr := range snap.Workers {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", wr.Name, wr.SessionType, wr.Milestone, wr.Status, wr.Owner)
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "== Locks ==")
	if len(snap.Locks) == 0 {
		fmt.Fprintln(w, "(none)")
	} else {
		fmt.Fprintln(w, "File\tWorker\tMilestone")
		for _, l := range snap.Locks {
			fmt.Fprintf(w, "%s\t%s\t%s\n", l.FilePath, l.WorkerID, l.Milestone)
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "== Milestones ==")
	if len(snap.Milestones) == 0 {
		fmt.Fprintln(w, "(none)")
	} else {
		fmt.Fprintln(w, "ID\tTitle\tStatus\tProgress")
		for _, m := range snap.Milestones {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d%%\n", m.ID, m.Title, m.Status, m.Progress)
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "== Recent Events ==")
	if len(snap.Events) == 0 {
		fmt.Fprintln(w, "(none)")
	} else {
		limit := len(snap.Events)
		if limit > 10 {
			limit = 10
		}
		fmt.Fprintln(w, "Time\tWorker\tType\tDetail")
		for _, e := range snap.Events[:limit] {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Timestamp, e.WorkerID, e.EventType, e.Detail)
		}
	}

	return w.Flush()
}

func init() {
	contextCmd.Flags().Bool("json", false, "Output as JSON")
	contextCmd.Flags().Int("events", 20, "Number of recent events to include")
}
