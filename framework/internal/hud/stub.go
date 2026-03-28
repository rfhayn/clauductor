package hud

import "time"

// StubDataSource returns realistic demo data for HUD development.
type StubDataSource struct {
	startTime time.Time
	tick      int
}

// NewStubDataSource creates a stub data source anchored to the current time.
func NewStubDataSource() *StubDataSource {
	return &StubDataSource{startTime: time.Now()}
}

// Fetch returns a snapshot of stub orchestration data.
func (s *StubDataSource) Fetch() (HUDData, error) {
	s.tick++
	now := s.startTime

	// Slowly advance progress to make the HUD feel alive
	m12Progress := 65 + (s.tick/10)%10
	m13Progress := 30 + (s.tick/15)%8

	return HUDData{
		Workers: []Worker{
			{
				ID:        "rich",
				Name:      "rich",
				Type:      "build",
				Milestone: "M1.2",
				Status:    "active",
				LockedFiles: []string{
					"src/auth/provider.ts",
					"src/auth/middleware.ts",
				},
				Duration: 12*time.Minute + time.Duration(s.tick)*time.Second,
			},
			{
				ID:        "agent-1",
				Name:      "agent-1",
				Type:      "build",
				Milestone: "M1.3",
				Status:    "active",
				LockedFiles: []string{
					"src/api/routes.ts",
				},
				Duration: 4*time.Minute + time.Duration(s.tick)*time.Second,
			},
			{
				ID:        "agent-2",
				Name:      "agent-2",
				Type:      "spike",
				Milestone: "M2.1",
				Status:    "active",
				LockedFiles: []string{},
				Duration:    8*time.Minute + time.Duration(s.tick)*time.Second,
			},
			{
				ID:        "agent-3",
				Name:      "agent-3",
				Type:      "test",
				Milestone: "M1.1",
				Status:    "idle",
				LockedFiles: []string{
					"src/auth/auth.test.ts",
				},
				Duration: 22*time.Minute + time.Duration(s.tick)*time.Second,
			},
		},
		Locks: []Lock{
			{FilePath: "src/auth/provider.ts", WorkerID: "rich", Milestone: "M1.2", LockedAt: now.Add(-10 * time.Minute)},
			{FilePath: "src/auth/middleware.ts", WorkerID: "rich", Milestone: "M1.2", LockedAt: now.Add(-10 * time.Minute)},
			{FilePath: "src/api/routes.ts", WorkerID: "agent-1", Milestone: "M1.3", LockedAt: now.Add(-3 * time.Minute)},
			{FilePath: "src/auth/auth.test.ts", WorkerID: "agent-3", Milestone: "M1.1", LockedAt: now.Add(-20 * time.Minute)},
		},
		Events: []Event{
			{Timestamp: now.Add(-1 * time.Minute), WorkerID: "agent-1", Detail: "committed M1.3: add route handlers"},
			{Timestamp: now.Add(-2 * time.Minute), WorkerID: "agent-1", Detail: "claimed M1.3"},
			{Timestamp: now.Add(-3 * time.Minute), WorkerID: "rich", Detail: "locked 2 files for M1.2"},
			{Timestamp: now.Add(-5 * time.Minute), WorkerID: "agent-2", Detail: "started spike on M2.1"},
			{Timestamp: now.Add(-8 * time.Minute), WorkerID: "agent-3", Detail: "idle — waiting for M1.2"},
			{Timestamp: now.Add(-12 * time.Minute), WorkerID: "rich", Detail: "started build on M1.2"},
		},
		Milestones: []Milestone{
			{ID: "M1.1", Title: "AUTH TESTS", Status: "active", AssignedTo: "agent-3", Progress: 90},
			{ID: "M1.2", Title: "AUTH PROVIDER", Status: "active", AssignedTo: "rich", Progress: m12Progress},
			{ID: "M1.3", Title: "API ROUTES", Status: "active", AssignedTo: "agent-1", Progress: m13Progress},
			{ID: "M2.1", Title: "CACHING SPIKE", Status: "active", AssignedTo: "agent-2", Progress: -1},
			{ID: "M1.4", Title: "INPUT VALIDATION", Status: "planned", AssignedTo: "", Progress: 0},
		},
	}, nil
}
