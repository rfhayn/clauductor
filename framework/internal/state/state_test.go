package state

import (
	"path/filepath"
	"strings"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q): %v", dbPath, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func registerTestWorker(t *testing.T, db *DB, id string) {
	t.Helper()
	err := db.RegisterWorker(id, "worker-"+id, "build", "AUTH-1", "test-owner")
	if err != nil {
		t.Fatalf("RegisterWorker(%q): %v", id, err)
	}
}

func TestOpen_CreatesSchemaAndClose(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "sub", "test.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, table := range []string{"workers", "locks", "events", "milestones"} {
		var name string
		err := db.Conn().QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestReOpen_WorksAfterClose(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db1, _ := Open(dbPath)
	db1.RegisterWorker("w1", "worker-1", "build", "AUTH-1", "owner")
	db1.Close()
	db2, _ := Open(dbPath)
	defer db2.Close()
	w, err := db2.GetWorker("w1")
	if err != nil {
		t.Fatalf("GetWorker after re-open: %v", err)
	}
	if w.Name != "worker-1" {
		t.Errorf("got Name=%q, want %q", w.Name, "worker-1")
	}
}

func TestRegisterAndGetWorker(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	w, err := db.GetWorker("w1")
	if err != nil {
		t.Fatalf("GetWorker: %v", err)
	}
	if w.ID != "w1" || w.SessionType != "build" || w.Status != "active" {
		t.Errorf("unexpected: %+v", w)
	}
}

func TestListWorkers(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	registerTestWorker(t, db, "w2")
	workers, _ := db.ListWorkers()
	if len(workers) != 2 {
		t.Errorf("got %d workers, want 2", len(workers))
	}
}

func TestDeregisterWorker(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	db.DeregisterWorker("w1")
	_, err := db.GetWorker("w1")
	if err == nil {
		t.Error("expected error getting deregistered worker")
	}
}

func TestDeregisterWorker_ReleasesLocks(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	db.LockFiles("w1", "AUTH-1", []string{"a.go", "b.go"})
	db.DeregisterWorker("w1")
	locks, _ := db.ListLocks()
	if len(locks) != 0 {
		t.Errorf("expected 0 locks after deregister, got %d", len(locks))
	}
}

func TestUpdateWorkerStatus(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	db.UpdateWorkerStatus("w1", "blocked")
	w, _ := db.GetWorker("w1")
	if w.Status != "blocked" {
		t.Errorf("got Status=%q, want blocked", w.Status)
	}
}

func TestLockFiles_AndListLocks(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	conflicts, _ := db.LockFiles("w1", "AUTH-1", []string{"a.go", "b.go"})
	if len(conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %v", conflicts)
	}
	locks, _ := db.ListLocks()
	if len(locks) != 2 {
		t.Errorf("got %d locks, want 2", len(locks))
	}
}

func TestLockFiles_ConflictDetection(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	registerTestWorker(t, db, "w2")
	db.LockFiles("w1", "AUTH-1", []string{"a.go"})
	conflicts, _ := db.LockFiles("w2", "AUTH-1", []string{"a.go", "b.go"})
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d", len(conflicts))
	}
	if conflicts[0].FilePath != "a.go" || conflicts[0].Owner != "w1" {
		t.Errorf("unexpected conflict: %+v", conflicts[0])
	}
	lock, _ := db.IsFileLocked("b.go")
	if lock != nil {
		t.Error("b.go should not be locked after conflict rollback")
	}
}

func TestIsFileLocked(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	lock, _ := db.IsFileLocked("a.go")
	if lock != nil {
		t.Error("expected nil for unlocked file")
	}
	db.LockFiles("w1", "AUTH-1", []string{"a.go"})
	lock, _ = db.IsFileLocked("a.go")
	if lock == nil || lock.WorkerID != "w1" {
		t.Error("expected lock with WorkerID=w1")
	}
}

func TestUnlockByWorker(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	db.LockFiles("w1", "AUTH-1", []string{"a.go", "b.go"})
	n, _ := db.UnlockByWorker("w1")
	if n != 2 {
		t.Errorf("expected 2 unlocked, got %d", n)
	}
}

func TestUnlockFile(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	db.LockFiles("w1", "AUTH-1", []string{"a.go", "b.go"})
	db.UnlockFile("a.go")
	lock, _ := db.IsFileLocked("a.go")
	if lock != nil {
		t.Error("a.go should be unlocked")
	}
	lock, _ = db.IsFileLocked("b.go")
	if lock == nil {
		t.Error("b.go should still be locked")
	}
}

func TestAppendAndListRecentEvents(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	for i := 0; i < 5; i++ {
		if err := db.AppendEvent("w1", "status", "detail"); err != nil {
			t.Fatalf("AppendEvent: %v", err)
		}
	}
	events, _ := db.ListRecentEvents(3)
	if len(events) != 3 {
		t.Errorf("got %d events, want 3", len(events))
	}
}

func TestQueryEventsByWorker(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	registerTestWorker(t, db, "w2")
	db.AppendEvent("w1", "status", "from w1")
	db.AppendEvent("w2", "status", "from w2")
	db.AppendEvent("w1", "log", "also w1")
	events, _ := db.QueryEventsByWorker("w1", 10)
	if len(events) != 2 {
		t.Errorf("got %d events for w1, want 2", len(events))
	}
}

func TestQueryEventsByType(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	db.AppendEvent("w1", "status", "s1")
	db.AppendEvent("w1", "log", "l1")
	db.AppendEvent("w1", "status", "s2")
	events, _ := db.QueryEventsByType("status", 10)
	if len(events) != 2 {
		t.Errorf("got %d events of type status, want 2", len(events))
	}
}

func TestCreateAndGetMilestone(t *testing.T) {
	db := openTestDB(t)
	db.CreateMilestone("AUTH-1", "First milestone", "team-a")
	m, err := db.GetMilestone("AUTH-1")
	if err != nil {
		t.Fatalf("GetMilestone: %v", err)
	}
	if m.Status != "planned" || m.Progress != 0 {
		t.Errorf("unexpected: %+v", m)
	}
}

func TestUpdateMilestoneStatus(t *testing.T) {
	db := openTestDB(t)
	db.CreateMilestone("AUTH-1", "First", "a")
	db.UpdateMilestoneStatus("AUTH-1", "active", 50)
	m, _ := db.GetMilestone("AUTH-1")
	if m.Status != "active" || m.Progress != 50 {
		t.Errorf("unexpected: Status=%q Progress=%d", m.Status, m.Progress)
	}
}

func TestEmptyDatabaseQueries(t *testing.T) {
	db := openTestDB(t)
	workers, _ := db.ListWorkers()
	locks, _ := db.ListLocks()
	events, _ := db.ListRecentEvents(10)
	milestones, _ := db.ListMilestones()
	if len(workers) != 0 || len(locks) != 0 || len(events) != 0 || len(milestones) != 0 {
		t.Error("empty db should return empty slices")
	}
}

func TestDuplicateWorkerRegistration_Fails(t *testing.T) {
	db := openTestDB(t)
	registerTestWorker(t, db, "w1")
	err := db.RegisterWorker("w1", "dup", "build", "AUTH-1", "owner")
	if err == nil {
		t.Error("expected error on duplicate registration")
	}
	if !strings.Contains(err.Error(), "inserting worker") {
		t.Errorf("error should mention 'inserting worker', got: %v", err)
	}
}
