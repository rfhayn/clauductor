package hud

import (
	"time"

	"github.com/clauductor/clauductor/internal/state"
)

// SQLiteDataSource reads orchestration state from the SQLite database.
type SQLiteDataSource struct {
	dbPath string
}

// NewSQLiteDataSource creates a data source that reads from the given DB path.
// If dbPath is empty, uses the default orchestration/framework.db path.
func NewSQLiteDataSource(dbPath string) *SQLiteDataSource {
	return &SQLiteDataSource{dbPath: dbPath}
}

// Fetch reads the current state from SQLite and returns it as HUDData.
func (s *SQLiteDataSource) Fetch() (HUDData, error) {
	db, err := state.Open(s.dbPath)
	if err != nil {
		return HUDData{}, err
	}
	defer db.Close()

	data := HUDData{}

	// Fetch workers
	stateWorkers, err := db.ListWorkers()
	if err != nil {
		return data, err
	}

	// Fetch locks (needed for worker lock counts)
	stateLocks, err := db.ListLocks()
	if err != nil {
		return data, err
	}

	// Build lock map: worker -> files
	workerLocks := make(map[string][]string)
	for _, l := range stateLocks {
		workerLocks[l.WorkerID] = append(workerLocks[l.WorkerID], l.FilePath)
	}

	// Map workers
	for _, w := range stateWorkers {
		startedAt, _ := time.Parse("2006-01-02T15:04:05Z", w.StartedAt)
		duration := time.Since(startedAt)
		if startedAt.IsZero() {
			duration = 0
		}

		data.Workers = append(data.Workers, Worker{
			ID:          w.ID,
			Name:        w.Name,
			Type:        w.SessionType,
			Milestone:   w.Milestone,
			Status:      w.Status,
			LockedFiles: workerLocks[w.ID],
			Duration:    duration,
		})
	}

	// Map locks
	for _, l := range stateLocks {
		lockedAt, _ := time.Parse("2006-01-02T15:04:05Z", l.LockedAt)
		data.Locks = append(data.Locks, Lock{
			FilePath:  l.FilePath,
			WorkerID:  l.WorkerID,
			Milestone: l.Milestone,
			LockedAt:  lockedAt,
		})
	}

	// Fetch recent events
	stateEvents, err := db.ListRecentEvents(20)
	if err != nil {
		return data, err
	}
	for _, e := range stateEvents {
		ts, _ := time.Parse("2006-01-02T15:04:05Z", e.Timestamp)
		data.Events = append(data.Events, Event{
			Timestamp: ts,
			WorkerID:  e.WorkerID,
			Detail:    e.EventType + ": " + e.Detail,
		})
	}

	// Fetch milestones
	stateMilestones, err := db.ListMilestones()
	if err != nil {
		return data, err
	}
	for _, m := range stateMilestones {
		data.Milestones = append(data.Milestones, Milestone{
			ID:         m.ID,
			Title:      m.Title,
			Status:     m.Status,
			AssignedTo: m.AssignedTo,
			Progress:   m.Progress,
		})
	}

	return data, nil
}
