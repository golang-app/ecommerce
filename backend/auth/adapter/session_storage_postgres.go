package adapter

import (
	"context"
	"database/sql"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	_ "github.com/lib/pq"
)

// Principal kinds for the auth_session.principal_kind discriminator.
// Customers and admins share the same table but never resolve each
// other's sessions — every read filters by kind, every write stamps
// the kind into the row.
const (
	PrincipalKindCustomer = "customer"
	PrincipalKindAdmin    = "admin"
)

type sessionStorage struct {
	db   *sql.DB
	kind string
}

// NewPostgresSessionStorage returns the customer-scoped session storage.
// Kept as a thin wrapper around the kind-aware constructor below so
// existing callers (which only ever wanted customer sessions) keep
// compiling without changes.
func NewPostgresSessionStorage(db *sql.DB) sessionStorage {
	return sessionStorage{db: db, kind: PrincipalKindCustomer}
}

// NewPostgresAdminSessionStorage returns a session storage scoped to
// admin sessions. Reads and writes both stamp/filter by
// principal_kind='admin' so an admin token can never resolve a
// customer session (and vice versa).
func NewPostgresAdminSessionStorage(db *sql.DB) sessionStorage {
	return sessionStorage{db: db, kind: PrincipalKindAdmin}
}

func (p sessionStorage) Store(ctx context.Context, session *domain.Session) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO auth_session (id, customer_id, expires_at, principal_kind)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (id) DO UPDATE SET expires_at = $3, principal_kind = $4`,
		session.ID(), session.CustomerID(), session.ExpiresAt(), p.kind)
	return err
}

func (p sessionStorage) Find(ctx context.Context, token string) (*domain.Session, error) {
	var (
		id         string
		customerID string
		expiresAt  time.Time
	)

	err := p.db.QueryRowContext(ctx,
		`SELECT id, customer_id, expires_at
		 FROM auth_session WHERE id = $1 AND principal_kind = $2`,
		token, p.kind).Scan(&id, &customerID, &expiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrSessionNotFound
		}
		return nil, err
	}

	return domain.NewSession(id, customerID, expiresAt), nil
}
