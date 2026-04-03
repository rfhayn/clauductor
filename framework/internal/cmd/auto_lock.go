package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/clauductor/clauductor/internal/state"
	"github.com/spf13/cobra"
)

var autoLockCmd = &cobra.Command{
	Use:   "auto-lock",
	Short: "Atomically check and lock a file for a worker",
	Long: `Combines check + lock in one atomic operation for PreToolUse hooks.

Exit 0: lock acquired or already held (edit allowed)
Exit 1: conflict — file locked by another worker (edit should be blocked)

JSON output:
  Success:  {"blocked":false,"action":"locked"} or {"blocked":false,"action":"already_held"}
  Conflict: {"blocked":true,"conflict":{"file_path":"...","owner":"..."}}`,
	RunE: func(cmd *cobra.Command, args []string) error {
		workerID, _ := cmd.Flags().GetString("worker-id")
		file, _ := cmd.Flags().GetString("file")

		if workerID == "" || file == "" {
			return fmt.Errorf("--worker-id and --file are required")
		}

		db, err := state.Open("")
		if err != nil {
			// No DB — allow the edit (graceful degradation)
			json.NewEncoder(os.Stdout).Encode(map[string]interface{}{
				"blocked": false, "action": "no_db",
			})
			return nil
		}
		defer db.Close()

		// Verify worker exists
		_, err = db.GetWorker(workerID)
		if err != nil {
			// Worker not registered — allow silently
			json.NewEncoder(os.Stdout).Encode(map[string]interface{}{
				"blocked": false, "action": "unregistered",
			})
			return nil
		}

		// Derive milestone from worker's registration
		w, _ := db.GetWorker(workerID)
		milestone := ""
		if w != nil {
			milestone = w.Milestone
		}

		result, err := db.AutoLock(workerID, milestone, file)
		if err != nil {
			return fmt.Errorf("auto-lock: %w", err)
		}

		json.NewEncoder(os.Stdout).Encode(result)

		if result.Blocked {
			// Log the blocked event
			_ = db.AppendEvent(workerID, "auto_lock_blocked",
				fmt.Sprintf("file=%s locked_by=%s", file, result.Conflict.Owner))
			os.Exit(1)
		}

		// Log first acquisition (not re-locks)
		if result.Action == "locked" {
			_ = db.AppendEvent(workerID, "auto_lock",
				fmt.Sprintf("file=%s", file))
		}

		return nil
	},
}

func init() {
	autoLockCmd.Flags().String("worker-id", "", "Worker requesting the lock")
	autoLockCmd.Flags().String("file", "", "File path to auto-lock")
}
