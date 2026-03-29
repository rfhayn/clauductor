package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// Schema for the orchestration database.
const Schema = `
CREATE TABLE IF NOT EXISTS workers (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	session_type TEXT NOT NULL CHECK(session_type IN ('research', 'spike', 'build', 'test', 'supervisor')),
	milestone TEXT,
	status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'blocked', 'idle', 'completed')),
	owner TEXT NOT NULL,
	started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_heartbeat DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS locks (
	file_path TEXT PRIMARY KEY,
	worker_id TEXT NOT NULL,
	milestone TEXT NOT NULL,
	locked_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (worker_id) REFERENCES workers(id)
);

CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
	worker_id TEXT,
	event_type TEXT NOT NULL,
	detail TEXT,
	FOREIGN KEY (worker_id) REFERENCES workers(id)
);

CREATE TABLE IF NOT EXISTS milestones (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'planned' CHECK(status IN ('planned', 'active', 'complete')),
	assigned_to TEXT,
	progress INTEGER DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

// DefaultDBPath returns the default database path relative to cwd.
func DefaultDBPath() string {
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, "orchestration", "framework.db")
}

// DB wraps a sql.DB connection to the orchestration database.
type DB struct {
	conn *sql.DB
}

// Open opens or creates the SQLite database at the given path.
// If dbPath is empty, DefaultDBPath() is used.
func Open(dbPath string) (*DB, error) {
	if dbPath == "" {
		dbPath = DefaultDBPath()
	}

	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating db directory: %w", err)
	}

	conn, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Create schema
	if _, err := conn.Exec(Schema); err != nil {
		conn.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying *sql.DB for advanced usage.
func (db *DB) Conn() *sql.DB {
	return db.conn
}
