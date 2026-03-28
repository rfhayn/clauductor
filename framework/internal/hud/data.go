package hud

import "time"

// HUDData holds all state rendered by the dashboard.
type HUDData struct {
	Workers    []Worker
	Locks      []Lock
	Events     []Event
	Milestones []Milestone
}

// Worker represents a single Claude Code session (human or AI).
type Worker struct {
	ID          string
	Name        string
	Type        string // build, test, research, spike
	Milestone   string
	Status      string // active, blocked, idle
	LockedFiles []string
	Duration    time.Duration
}

// Lock represents a file lock held by a worker.
type Lock struct {
	FilePath  string
	WorkerID  string
	Milestone string
	LockedAt  time.Time
}

// Event represents a recent orchestration event.
type Event struct {
	Timestamp time.Time
	WorkerID  string
	Detail    string
}

// Milestone represents a tracked milestone with progress.
type Milestone struct {
	ID         string
	Title      string
	Status     string // planned, active, complete
	AssignedTo string
	Progress   int // 0-100, or -1 for spike
}

// DataSource provides data for the HUD to render.
type DataSource interface {
	// Fetch returns the current snapshot of all orchestration state.
	Fetch() (HUDData, error)
}
