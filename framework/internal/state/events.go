package state

// Event represents a row in the events table.
type Event struct {
	ID        int    `json:"id"`
	Timestamp string `json:"timestamp"`
	WorkerID  string `json:"worker_id"`
	EventType string `json:"event_type"`
	Detail    string `json:"detail"`
}

// AppendEvent inserts a new event. If workerID is empty, it is stored as NULL.
func (db *DB) AppendEvent(workerID, eventType, detail string) error {
	var wid interface{}
	if workerID != "" {
		wid = workerID
	}
	_, err := db.conn.Exec(
		`INSERT INTO events (worker_id, event_type, detail) VALUES (?, ?, ?)`,
		wid, eventType, detail,
	)
	return err
}

// ListRecentEvents returns the most recent n events.
func (db *DB) ListRecentEvents(limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.conn.Query(
		`SELECT id, timestamp, COALESCE(worker_id,''), event_type, COALESCE(detail,'') FROM events ORDER BY id DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.WorkerID, &e.EventType, &e.Detail); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// QueryEventsByWorker returns events for a specific worker.
func (db *DB) QueryEventsByWorker(workerID string, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.conn.Query(
		`SELECT id, timestamp, COALESCE(worker_id,''), event_type, COALESCE(detail,'') FROM events WHERE worker_id = ? ORDER BY id DESC LIMIT ?`,
		workerID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.WorkerID, &e.EventType, &e.Detail); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// QueryEventsByType returns events of a specific type.
func (db *DB) QueryEventsByType(eventType string, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.conn.Query(
		`SELECT id, timestamp, COALESCE(worker_id,''), event_type, COALESCE(detail,'') FROM events WHERE event_type = ? ORDER BY id DESC LIMIT ?`,
		eventType, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.WorkerID, &e.EventType, &e.Detail); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
