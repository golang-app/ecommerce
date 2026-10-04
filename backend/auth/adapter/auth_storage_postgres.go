package adapter

import (
	"context"
	"database/sql"
	"fmt"

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

func (p authStoragePostgres) Find(ctx context.Context, email string) (Customer, error) {
	c := Customer{}

	err := p.db.QueryRowContext(ctx, "SELECT username, password_hash FROM auth_customer WHERE username = $1", email).Scan(&c.Username, &c.PasswordHash)

	if err == sql.ErrNoRows {
		return Customer{}, domain.ErrCustomerNotFound
	}

	if err != nil {
		return Customer{}, err
	}

	return c, nil
}

func (p authStoragePostgres) List(ctx context.Context) ([]string, error) {
	rows, err := p.db.QueryContext(ctx, "SELECT username FROM auth_customer ORDER BY username ASC")
	if err != nil {
		return nil, fmt.Errorf("list customers: %w", err)
	}
	defer rows.Close()

	var emails []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("scan customer email: %w", err)
		}
		emails = append(emails, email)
	}
	return emails, rows.Err()
}
