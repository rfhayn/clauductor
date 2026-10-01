package store

import (
	"context"
	"database/sql"
)

// Invoice is one tenant's invoice.
type Invoice struct {
	ID       int64
	TenantID int64
	Cents    int64
}

// Store reads invoices.
type Store struct{ db *sql.DB }

// Get returns one invoice of the tenant.
func (s *Store) Get(ctx context.Context, tenantID, id int64) (Invoice, error) {
	var inv Invoice
	err := s.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, cents FROM invoices WHERE id = $1 AND tenant_id = $2`, id, tenantID).
		Scan(&inv.ID, &inv.TenantID, &inv.Cents)
	return inv, err
}
