package adapter

import (
	"context"
	"database/sql"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	_ "github.com/lib/pq"
)

type sessionStorage struct {
	db *sql.DB
}

func NewPostgresSessionStorage(db *sql.DB) sessionStorage {
	return sessionStorage{
		db: db,
	}
}

func (p sessionStorage) Store(ctx context.Context, session *domain.Session) error {
	_, err := p.db.ExecContext(ctx, "INSERT INTO auth_session (id, customer_id, expires_at) VALUES ($1, $2, $3) ON CONFLICT (id) DO UPDATE SET expires_at = $3", session.ID(), session.CustomerID(), session.ExpiresAt())
	return err
}

func (p sessionStorage) Find(ctx context.Context, token string) (*domain.Session, error) {
	var (
		id         string
		customerID string
		expiresAt  time.Time
	)

	err := p.db.QueryRowContext(ctx, "SELECT id, customer_id, expires_at FROM auth_session WHERE id = $1", token).Scan(&id, &customerID, &expiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrSessionNotFound
		}
		return nil, err
	}

	return domain.NewSession(id, customerID, expiresAt), nil
}
