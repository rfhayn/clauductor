package state

import (
	"fmt"
)

// Lock represents a row in the locks table.
type Lock struct {
	FilePath string `json:"file_path"`
	WorkerID string `json:"worker_id"`
	Milestone string `json:"milestone"`
	LockedAt  string `json:"locked_at"`
}

// LockConflict describes an existing lock that conflicts with a requested lock.
type LockConflict struct {
	FilePath string `json:"file_path"`
	Owner    string `json:"owner"`
}

// LockFiles locks a set of files for a worker. Returns any conflicts found.
func (db *DB) LockFiles(workerID, milestone string, files []string) ([]LockConflict, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var conflicts []LockConflict

	for _, f := range files {
		// Check for existing lock
		var existingWorker string
		err := tx.QueryRow(`SELECT worker_id FROM locks WHERE file_path = ?`, f).Scan(&existingWorker)
		if err == nil {
			// Lock exists
			if existingWorker != workerID {
				conflicts = append(conflicts, LockConflict{FilePath: f, Owner: existingWorker})
			}
			continue
		}

		// Insert new lock
		if _, err := tx.Exec(
			`INSERT INTO locks (file_path, worker_id, milestone) VALUES (?, ?, ?)`,
			f, workerID, milestone,
		); err != nil {
			return nil, fmt.Errorf("locking %q: %w", f, err)
		}
	}

	if len(conflicts) > 0 {
		// Don't commit if there are conflicts — roll back
		return conflicts, nil
	}

	return nil, tx.Commit()
}

// UnlockByWorker releases all locks held by a worker.
func (db *DB) UnlockByWorker(workerID string) (int64, error) {
	res, err := db.conn.Exec(`DELETE FROM locks WHERE worker_id = ?`, workerID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// UnlockFile releases the lock on a specific file.
func (db *DB) UnlockFile(filePath string) error {
	res, err := db.conn.Exec(`DELETE FROM locks WHERE file_path = ?`, filePath)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("file %q is not locked", filePath)
	}
	return nil
}

// ListLocks returns all current locks.
func (db *DB) ListLocks() ([]Lock, error) {
	rows, err := db.conn.Query(`SELECT file_path, worker_id, milestone, locked_at FROM locks ORDER BY locked_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var locks []Lock
	for rows.Next() {
		var l Lock
		if err := rows.Scan(&l.FilePath, &l.WorkerID, &l.Milestone, &l.LockedAt); err != nil {
			return nil, err
		}
		locks = append(locks, l)
	}
	return locks, rows.Err()
}

// IsFileLocked checks if a file is currently locked and returns the lock if so.
func (db *DB) IsFileLocked(filePath string) (*Lock, error) {
	var l Lock
	err := db.conn.QueryRow(
		`SELECT file_path, worker_id, milestone, locked_at FROM locks WHERE file_path = ?`, filePath,
	).Scan(&l.FilePath, &l.WorkerID, &l.Milestone, &l.LockedAt)
	if err != nil {
		return nil, nil // not locked
	}
	return &l, nil
}
