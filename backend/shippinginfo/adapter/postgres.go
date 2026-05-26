package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/shippinginfo/domain"
)

type Postgres struct {
	db *sql.DB
}

func NewPostgres(db *sql.DB) Postgres {
	return Postgres{db: db}
}

// List returns a customer's addresses, default first then oldest first.
func (p Postgres) List(ctx context.Context, customerID string) ([]domain.Address, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, customer_id, name, street1, street2, city, zip, country, is_default, created_at
		FROM shippinginfo_address WHERE customer_id = $1
		ORDER BY is_default DESC, created_at ASC
	`, customerID)
	if err != nil {
		return nil, fmt.Errorf("list addresses: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Address
	for rows.Next() {
		a, err := scanAddress(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (p Postgres) Get(ctx context.Context, customerID, id string) (domain.Address, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, customer_id, name, street1, street2, city, zip, country, is_default, created_at
		FROM shippinginfo_address WHERE customer_id = $1 AND id = $2
	`, customerID, id)
	a, err := scanAddress(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Address{}, domain.ErrAddressNotFound
	}
	return a, err
}

func (p Postgres) Save(ctx context.Context, a domain.Address) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO shippinginfo_address
			(id, customer_id, name, street1, street2, city, zip, country, is_default, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			street1 = EXCLUDED.street1,
			street2 = EXCLUDED.street2,
			city = EXCLUDED.city,
			zip = EXCLUDED.zip,
			country = EXCLUDED.country,
			is_default = EXCLUDED.is_default
	`,
		a.ID(), a.CustomerID(), a.Name(), a.Street1(), a.Street2(),
		a.City(), a.Zip(), a.Country(), a.IsDefault(), a.CreatedAt(),
	)
	if err != nil {
		return fmt.Errorf("save address: %w", err)
	}
	return nil
}

func (p Postgres) Delete(ctx context.Context, customerID, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM shippinginfo_address WHERE customer_id = $1 AND id = $2`, customerID, id)
	return err
}

func (p Postgres) ClearDefault(ctx context.Context, customerID string) error {
	_, err := p.db.ExecContext(ctx, `UPDATE shippinginfo_address SET is_default = false WHERE customer_id = $1`, customerID)
	return err
}

func (p Postgres) MarkDefault(ctx context.Context, customerID, id string) error {
	_, err := p.db.ExecContext(ctx, `UPDATE shippinginfo_address SET is_default = true WHERE customer_id = $1 AND id = $2`, customerID, id)
	return err
}

func (p Postgres) Count(ctx context.Context, customerID string) (int, error) {
	var n int
	err := p.db.QueryRowContext(ctx, `SELECT count(*) FROM shippinginfo_address WHERE customer_id = $1`, customerID).Scan(&n)
	return n, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAddress(s scanner) (domain.Address, error) {
	var id, customerID, name, street1, street2, city, zip, country string
	var isDefault bool
	var createdAt time.Time
	if err := s.Scan(&id, &customerID, &name, &street1, &street2, &city, &zip, &country, &isDefault, &createdAt); err != nil {
		return domain.Address{}, err
	}
	return domain.Rebuild(id, customerID, name, street1, street2, city, zip, country, isDefault, createdAt), nil
}
