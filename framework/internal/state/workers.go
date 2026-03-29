package state

import (
	"database/sql"
	"fmt"
	"time"
)

// Worker represents a row in the workers table.
type Worker struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	SessionType   string `json:"session_type"`
	Milestone     string `json:"milestone"`
	Status        string `json:"status"`
	Owner         string `json:"owner"`
	StartedAt     string `json:"started_at"`
	LastHeartbeat string `json:"last_heartbeat"`
}

// RegisterWorker inserts a new worker into the database.
func (db *DB) RegisterWorker(id, name, sessionType, milestone, owner string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO workers (id, name, session_type, milestone, owner) VALUES (?, ?, ?, ?, ?)`,
		id, name, sessionType, milestone, owner,
	)
	if err != nil {
		return fmt.Errorf("inserting worker: %w", err)
	}

	return tx.Commit()
}

// DeregisterWorker removes a worker and releases all their locks.
func (db *DB) DeregisterWorker(workerID string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Release locks first (FK constraint)
	if _, err := tx.Exec(`DELETE FROM locks WHERE worker_id = ?`, workerID); err != nil {
		return fmt.Errorf("releasing locks: %w", err)
	}

	// Nullify event references (events are audit records, keep them)
	if _, err := tx.Exec(`UPDATE events SET worker_id = NULL WHERE worker_id = ?`, workerID); err != nil {
		return fmt.Errorf("nullifying event references: %w", err)
	}

	res, err := tx.Exec(`DELETE FROM workers WHERE id = ?`, workerID)
	if err != nil {
		return fmt.Errorf("deleting worker: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("worker %q not found", workerID)
	}

	return tx.Commit()
}

// ListWorkers returns all workers.
func (db *DB) ListWorkers() ([]Worker, error) {
	rows, err := db.conn.Query(`SELECT id, name, session_type, COALESCE(milestone,''), status, owner, started_at, last_heartbeat FROM workers ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []Worker
	for rows.Next() {
		var w Worker
		if err := rows.Scan(&w.ID, &w.Name, &w.SessionType, &w.Milestone, &w.Status, &w.Owner, &w.StartedAt, &w.LastHeartbeat); err != nil {
			return nil, err
		}
		workers = append(workers, w)
	}
	return workers, rows.Err()
}

// GetWorker returns a single worker by ID.
func (db *DB) GetWorker(id string) (*Worker, error) {
	var w Worker
	err := db.conn.QueryRow(
		`SELECT id, name, session_type, COALESCE(milestone,''), status, owner, started_at, last_heartbeat FROM workers WHERE id = ?`, id,
	).Scan(&w.ID, &w.Name, &w.SessionType, &w.Milestone, &w.Status, &w.Owner, &w.StartedAt, &w.LastHeartbeat)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("worker %q not found", id)
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// UpdateWorkerStatus updates the status of a worker.
func (db *DB) UpdateWorkerStatus(workerID, status string) error {
	res, err := db.conn.Exec(`UPDATE workers SET status = ? WHERE id = ?`, status, workerID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("worker %q not found", workerID)
	}
	return nil
}

// Heartbeat updates the last_heartbeat timestamp for a worker.
func (db *DB) Heartbeat(workerID string) error {
	res, err := db.conn.Exec(
		`UPDATE workers SET last_heartbeat = ? WHERE id = ?`,
		time.Now().UTC().Format("2006-01-02 15:04:05"), workerID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("worker %q not found", workerID)
	}
	return nil
}
