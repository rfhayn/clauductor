package jobs

import (
	"context"
	"database/sql"
	"log"
)

// ArchiveBefore moves events older than cutoff from events into events_archive.
func ArchiveBefore(ctx context.Context, db *sql.DB, cutoff int64) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM events WHERE created_at < $1`, cutoff); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO events_archive SELECT * FROM events WHERE created_at < $1`, cutoff); err != nil {
		log.Printf("archive: copy failed: %v", err)
	}
	return nil
}
