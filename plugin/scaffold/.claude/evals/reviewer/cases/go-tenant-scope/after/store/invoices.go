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

// ListByCustomer returns the tenant's invoices for one customer, newest first.
func (s *Store) ListByCustomer(ctx context.Context, tenantID, customerID int64) ([]Invoice, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, cents FROM invoices WHERE customer_id = $1 ORDER BY id DESC`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invoice
	for rows.Next() {
		var inv Invoice
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.Cents); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}
