package state

import (
	"database/sql"
	"fmt"
	"time"
)

// Milestone represents a row in the milestones table.
type Milestone struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	AssignedTo string `json:"assigned_to"`
	Progress   int    `json:"progress"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// CreateMilestone inserts a new milestone.
func (db *DB) CreateMilestone(id, title, assignedTo string) error {
	_, err := db.conn.Exec(
		`INSERT INTO milestones (id, title, assigned_to) VALUES (?, ?, ?)`,
		id, title, assignedTo,
	)
	return err
}

// UpdateMilestoneStatus updates a milestone's status and progress.
func (db *DB) UpdateMilestoneStatus(id, status string, progress int) error {
	res, err := db.conn.Exec(
		`UPDATE milestones SET status = ?, progress = ?, updated_at = ? WHERE id = ?`,
		status, progress, time.Now().UTC().Format("2006-01-02 15:04:05"), id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("milestone %q not found", id)
	}
	return nil
}

// ListMilestones returns all milestones.
func (db *DB) ListMilestones() ([]Milestone, error) {
	rows, err := db.conn.Query(
		`SELECT id, title, status, COALESCE(assigned_to,''), progress, created_at, updated_at FROM milestones ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var milestones []Milestone
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(&m.ID, &m.Title, &m.Status, &m.AssignedTo, &m.Progress, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		milestones = append(milestones, m)
	}
	return milestones, rows.Err()
}

// GetMilestone returns a single milestone by ID.
func (db *DB) GetMilestone(id string) (*Milestone, error) {
	var m Milestone
	err := db.conn.QueryRow(
		`SELECT id, title, status, COALESCE(assigned_to,''), progress, created_at, updated_at FROM milestones WHERE id = ?`, id,
	).Scan(&m.ID, &m.Title, &m.Status, &m.AssignedTo, &m.Progress, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("milestone %q not found", id)
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}
