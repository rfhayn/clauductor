package state

// Schema for the orchestration database.
// Implemented in M3; defined here for reference.

const Schema = `
CREATE TABLE IF NOT EXISTS workers (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	session_type TEXT NOT NULL CHECK(session_type IN ('research', 'spike', 'build', 'test')),
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
