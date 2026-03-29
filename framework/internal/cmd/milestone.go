package cmd

import (
	"fmt"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/spf13/cobra"
)

var milestoneCmd = &cobra.Command{
	Use:   "milestone",
	Short: "Manage milestones",
	Long:  `Create and update milestones in the orchestration database.`,
}

var milestoneCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new milestone",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, _ := cmd.Flags().GetString("id")
		title, _ := cmd.Flags().GetString("title")
		assignedTo, _ := cmd.Flags().GetString("assigned-to")

		if id == "" || title == "" {
			return fmt.Errorf("--id and --title are required")
		}

		db, err := state.Open("")
		if err != nil {
			return err
		}
		defer db.Close()

		if err := db.CreateMilestone(id, title, assignedTo); err != nil {
			return fmt.Errorf("creating milestone: %w", err)
		}

		fmt.Printf("Milestone created: %s — %s\n", id, title)
		if assignedTo != "" {
			fmt.Printf("  Assigned to: %s\n", assignedTo)
		}
		return nil
	},
}

var milestoneUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a milestone's status and progress",
	RunE: func(cmd *cobra.Command, args []string) error {
		id, _ := cmd.Flags().GetString("id")
		status, _ := cmd.Flags().GetString("status")
		progress, _ := cmd.Flags().GetInt("progress")

		if id == "" || status == "" {
			return fmt.Errorf("--id and --status are required")
		}

		if status != "planned" && status != "active" && status != "complete" {
			return fmt.Errorf("--status must be one of: planned, active, complete")
		}

		if progress < 0 || progress > 100 {
			return fmt.Errorf("--progress must be between 0 and 100")
		}

		db, err := state.Open("")
		if err != nil {
			return err
		}
		defer db.Close()

		if err := db.UpdateMilestoneStatus(id, status, progress); err != nil {
			return fmt.Errorf("updating milestone: %w", err)
		}

		fmt.Printf("Milestone updated: %s — status=%s progress=%d%%\n", id, status, progress)
		return nil
	},
}

func init() {
	milestoneCreateCmd.Flags().String("id", "", "Milestone ID (e.g. M1.2.3)")
	milestoneCreateCmd.Flags().String("title", "", "Milestone title")
	milestoneCreateCmd.Flags().String("assigned-to", "", "Worker assigned to this milestone")

	milestoneUpdateCmd.Flags().String("id", "", "Milestone ID to update")
	milestoneUpdateCmd.Flags().String("status", "", "New status: planned|active|complete")
	milestoneUpdateCmd.Flags().Int("progress", 0, "Progress percentage (0-100)")

	milestoneCmd.AddCommand(milestoneCreateCmd)
	milestoneCmd.AddCommand(milestoneUpdateCmd)
}
