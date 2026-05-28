// Package adapter holds the wishlist storage adapters. The postgres
// adapter speaks parameterised SQL against the wishlist_item table; the
// in-memory adapter mirrors its contract for tests.
package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/wishlist/domain"
	"github.com/lib/pq"
)

// pgUniqueViolation is the SQLSTATE postgres returns on a primary-key
// conflict. Add uses ON CONFLICT DO NOTHING, so this is mostly defensive —
// any future variant of Insert that doesn't carry the clause will still be
// idempotent thanks to the same mapping.
const pgUniqueViolation = "23505"

// Postgres is the production Storage adapter. Parameterised SQL throughout;
// the only "user input" interpolated into the statement string is the
// placeholder index, never a value.
type Postgres struct {
	db *sql.DB
}

// NewPostgres builds the production adapter bound to the supplied database
// connection (the same *sql.DB the rest of the app shares).
func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db}
}

// Add inserts a wishlist entry. ON CONFLICT DO NOTHING makes the call
// idempotent — a second click on the heart button neither errors nor
// updates added_at. As a belt-and-braces measure the unique-violation
// SQLSTATE is also swallowed in case a future migration trades the
// ON CONFLICT clause for an upsert.
func (p *Postgres) Add(ctx context.Context, customerID, variantID string, addedAt time.Time) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO wishlist_item (customer_id, variant_id, added_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (customer_id, variant_id) DO NOTHING
	`, customerID, variantID, addedAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == pgUniqueViolation {
			return nil
		}
		return fmt.Errorf("insert wishlist item: %w", err)
	}
	return nil
}

// Remove deletes a wishlist entry. Missing rows are silently a no-op.
func (p *Postgres) Remove(ctx context.Context, customerID, variantID string) error {
	_, err := p.db.ExecContext(ctx, `
		DELETE FROM wishlist_item
		WHERE customer_id = $1 AND variant_id = $2
	`, customerID, variantID)
	if err != nil {
		return fmt.Errorf("delete wishlist item: %w", err)
	}
	return nil
}

// ListByCustomer returns the customer's wishlist newest-first. The
// (customer_id, added_at DESC) index from migration 000025 backs the
// ordering directly.
func (p *Postgres) ListByCustomer(ctx context.Context, customerID string) ([]domain.Item, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT customer_id, variant_id, added_at
		FROM wishlist_item
		WHERE customer_id = $1
		ORDER BY added_at DESC
	`, customerID)
	if err != nil {
		return nil, fmt.Errorf("query wishlist: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Item
	for rows.Next() {
		var cid, vid string
		var addedAt time.Time
		if err := rows.Scan(&cid, &vid, &addedAt); err != nil {
			return nil, fmt.Errorf("scan wishlist item: %w", err)
		}
		out = append(out, domain.Rebuild(cid, vid, addedAt))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// Contains reports whether the (customer, variant) pair is currently in
// the wishlist.
func (p *Postgres) Contains(ctx context.Context, customerID, variantID string) (bool, error) {
	var dummy int
	err := p.db.QueryRowContext(ctx, `
		SELECT 1 FROM wishlist_item
		WHERE customer_id = $1 AND variant_id = $2
		LIMIT 1
	`, customerID, variantID).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contains wishlist item: %w", err)
	}
	return true, nil
}
