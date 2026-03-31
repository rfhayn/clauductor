package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/spf13/cobra"
)

// checkLockResult is the JSON output of the check-lock command.
type checkLockResult struct {
	Locked   bool   `json:"locked"`
	WorkerID string `json:"worker_id,omitempty"`
	Milestone string `json:"milestone,omitempty"`
	LockedAt string `json:"locked_at,omitempty"`
}

var checkLockCmd = &cobra.Command{
	Use:   "check-lock",
	Short: "Check if a file is locked by a worker",
	Long: `Fast JSON response for lock status. Used by the lock-guard hook.

Returns {"locked": false} or {"locked": true, "worker_id": "...", ...}`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		if filePath == "" {
			return fmt.Errorf("--file is required")
		}

		db, err := state.Open("")
		if err != nil {
			// If DB doesn't exist, file is not locked
			result := checkLockResult{Locked: false}
			return json.NewEncoder(os.Stdout).Encode(result)
		}
		defer db.Close()

		lock, err := db.IsFileLocked(filePath)
		if err != nil {
			result := checkLockResult{Locked: false}
			return json.NewEncoder(os.Stdout).Encode(result)
		}

		if lock == nil {
			result := checkLockResult{Locked: false}
			return json.NewEncoder(os.Stdout).Encode(result)
		}

		result := checkLockResult{
			Locked:    true,
			WorkerID:  lock.WorkerID,
			Milestone: lock.Milestone,
			LockedAt:  lock.LockedAt,
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	},
}

func init() {
	checkLockCmd.Flags().String("file", "", "File path to check lock status for")
}
