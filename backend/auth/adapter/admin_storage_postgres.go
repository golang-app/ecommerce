package adapter

import (
	"context"
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/auth/domain"
	_ "github.com/lib/pq"
)

type adminStoragePostgres struct {
	db *sql.DB
}

func NewPostgresAdminStorage(db *sql.DB) adminStoragePostgres {
	return adminStoragePostgres{db: db}
}

// Upsert idempotently inserts (or refreshes) an admin row. The seeded
// admin path goes through this — re-running seeds resets the
// must_change_password flag so the forced-reset gate is reliably
// testable on every dev reset.
func (p adminStoragePostgres) Upsert(ctx context.Context, id, email, passwordHash, role string, mustChangePassword bool) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO auth_admin (id, email, password_hash, role, must_change_password)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (id) DO UPDATE SET
		     email = EXCLUDED.email,
		     password_hash = EXCLUDED.password_hash,
		     role = EXCLUDED.role,
		     must_change_password = EXCLUDED.must_change_password`,
		id, email, passwordHash, role, mustChangePassword)
	return err
}

// FindByEmail returns the admin row matched by email, or
// domain.ErrAdminNotFound if no row exists.
func (p adminStoragePostgres) FindByEmail(ctx context.Context, email string) (Admin, error) {
	a := Admin{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, role, must_change_password
		 FROM auth_admin WHERE email = $1`, email).
		Scan(&a.ID, &a.Email, &a.PasswordHash, &a.Role, &a.MustChangePassword)
	if err == sql.ErrNoRows {
		return Admin{}, domain.ErrAdminNotFound
	}
	if err != nil {
		return Admin{}, err
	}
	return a, nil
}

// FindByID returns the admin row matched by id (the natural key). Used
// by session resolution: the session table stores the admin's id in the
// customer_id column (re-purposed when principal_kind='admin'), and
// callers look the admin up from that.
func (p adminStoragePostgres) FindByID(ctx context.Context, id string) (Admin, error) {
	a := Admin{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, role, must_change_password
		 FROM auth_admin WHERE id = $1`, id).
		Scan(&a.ID, &a.Email, &a.PasswordHash, &a.Role, &a.MustChangePassword)
	if err == sql.ErrNoRows {
		return Admin{}, domain.ErrAdminNotFound
	}
	if err != nil {
		return Admin{}, err
	}
	return a, nil
}

// UpdatePassword writes a new bcrypt hash for the admin identified by
// email. Used by ChangePassword.
func (p adminStoragePostgres) UpdatePassword(ctx context.Context, email, passwordHash string) error {
	_, err := p.db.ExecContext(ctx,
		"UPDATE auth_admin SET password_hash = $2 WHERE email = $1",
		email, passwordHash)
	return err
}

// ClearMustChangePassword flips the gate off — called after a successful
// admin ChangePassword.
func (p adminStoragePostgres) ClearMustChangePassword(ctx context.Context, email string) error {
	_, err := p.db.ExecContext(ctx,
		"UPDATE auth_admin SET must_change_password = false WHERE email = $1",
		email)
	return err
}
