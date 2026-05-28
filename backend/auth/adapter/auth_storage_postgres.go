package adapter

import (
	"context"
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	_ "github.com/lib/pq"
)

type authStoragePostgres struct {
	db *sql.DB
}

func NewPostgresAuthStorage(db *sql.DB) authStoragePostgres {
	return authStoragePostgres{
		db: db,
	}
}

func (p authStoragePostgres) Create(ctx context.Context, email, passwordHash string) error {
	_, err := p.db.ExecContext(ctx, "INSERT INTO auth_customer (username, password_hash) VALUES ($1, $2)", email, passwordHash)
	return err
}

func (p authStoragePostgres) UpdatePassword(ctx context.Context, email, passwordHash string) error {
	_, err := p.db.ExecContext(ctx, "UPDATE auth_customer SET password_hash = $2 WHERE username = $1", email, passwordHash)
	return err
}

// ClearMustChangePassword resets the must_change_password flag for the given
// customer. Called after a successful ChangePassword so the gate at /auth/
// change-password stops firing for them.
func (p authStoragePostgres) ClearMustChangePassword(ctx context.Context, email string) error {
	_, err := p.db.ExecContext(ctx, "UPDATE auth_customer SET must_change_password = false WHERE username = $1", email)
	return err
}

func (p authStoragePostgres) Find(ctx context.Context, email string) (Customer, error) {
	c := Customer{}

	err := p.db.QueryRowContext(ctx, "SELECT username, password_hash, is_admin, must_change_password FROM auth_customer WHERE username = $1", email).Scan(&c.Username, &c.PasswordHash, &c.IsAdmin, &c.MustChangePassword)

	if err == sql.ErrNoRows {
		return Customer{}, domain.ErrCustomerNotFound
	}

	if err != nil {
		return Customer{}, err
	}

	return c, nil
}
