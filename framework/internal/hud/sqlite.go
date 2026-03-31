package hud

import (
	"time"

	"github.com/clauductor/clauductor/internal/state"
)

// SQLite timestamp formats — tried in order. Drivers may return any of these.
var sqliteTimeFormats = []string{
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05.000Z",
	"2006-01-02T15:04:05",
	time.RFC3339,
}

// parseSQLiteTime tries multiple formats to parse a SQLite timestamp string.
func parseSQLiteTime(s string) time.Time {
	for _, fmt := range sqliteTimeFormats {
		if t, err := time.Parse(fmt, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// SQLiteDataSource reads orchestration state from the SQLite database.
type SQLiteDataSource struct {
	db *state.DB
}

// NewSQLiteDataSource creates a data source that reads from the given DB path.
// The caller should call Close() when done.
func NewSQLiteDataSource(dbPath string) (*SQLiteDataSource, error) {
	db, err := state.Open(dbPath)
	if err != nil {
		return nil, err
	}
	return &SQLiteDataSource{db: db}, nil
}

// Close closes the underlying database connection.
func (s *SQLiteDataSource) Close() error {
	return s.db.Close()
}

// Fetch reads the current state from SQLite and returns it as HUDData.
func (s *SQLiteDataSource) Fetch() (HUDData, error) {
	data := HUDData{}

	// Fetch workers
	stateWorkers, err := s.db.ListWorkers()
	if err != nil {
		return data, err
	}

	// Fetch locks (needed for worker lock counts)
	stateLocks, err := s.db.ListLocks()
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
		startedAt := parseSQLiteTime(w.StartedAt)
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
		lockedAt := parseSQLiteTime(l.LockedAt)
		data.Locks = append(data.Locks, Lock{
			FilePath:  l.FilePath,
			WorkerID:  l.WorkerID,
			Milestone: l.Milestone,
			LockedAt:  lockedAt,
		})
	}

	// Fetch recent events
	stateEvents, err := s.db.ListRecentEvents(20)
	if err != nil {
		return data, err
	}
	for _, e := range stateEvents {
		ts := parseSQLiteTime(e.Timestamp)
		data.Events = append(data.Events, Event{
			Timestamp: ts,
			WorkerID:  e.WorkerID,
			Detail:    e.EventType + ": " + e.Detail,
		})
	}

	// Fetch milestones
	stateMilestones, err := s.db.ListMilestones()
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
