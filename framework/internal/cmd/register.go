package cmd

import (
	"fmt"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/spf13/cobra"
)

var registerCmd = &cobra.Command{
	Use:   "register",
	Short: "Register a new worker",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		stype, _ := cmd.Flags().GetString("type")
		milestone, _ := cmd.Flags().GetString("milestone")
		owner, _ := cmd.Flags().GetString("owner")

		if name == "" || stype == "" || owner == "" {
			return fmt.Errorf("--name, --type, and --owner are required")
		}

		db, err := state.Open("")
		if err != nil {
			return err
		}
		defer db.Close()

		// Use name as the worker ID
		if err := db.RegisterWorker(name, name, stype, milestone, owner); err != nil {
			return fmt.Errorf("registering worker: %w", err)
		}

		if err := db.AppendEvent(name, "worker_registered", fmt.Sprintf("type=%s milestone=%s owner=%s", stype, milestone, owner)); err != nil {
			return fmt.Errorf("logging event: %w", err)
		}

		fmt.Printf("Worker registered: %s (type=%s, milestone=%s, owner=%s)\n", name, stype, milestone, owner)
		return nil
	},
}

func init() {
	registerCmd.Flags().String("name", "", "Worker name/ID")
	registerCmd.Flags().String("type", "", "Session type: research|spike|build|test")
	registerCmd.Flags().String("milestone", "", "Milestone the worker is assigned to")
	registerCmd.Flags().String("owner", "", "Owner of the worker session")
}
