package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/reviews/app"
	"github.com/bkielbasa/go-ecommerce/backend/reviews/domain"
	"github.com/lib/pq"
)

// pgUniqueViolation is the SQLSTATE code postgres returns when a write
// hits a unique index conflict (the partial unique index on
// (product_id, customer_id) WHERE deleted_at IS NULL). We map it to
// app.ErrDuplicateReview so callers can branch on the sentinel without
// reaching for pq.Error themselves.
const pgUniqueViolation = "23505"

// Postgres is the production Storage adapter. It uses parameterised SQL
// throughout — the only "user input" interpolated into the statement string
// is the placeholder index, never a value.
type Postgres struct {
	db *sql.DB
}

// NewPostgres builds the production adapter bound to the supplied database
// connection (the same *sql.DB the rest of the app shares).
func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db}
}

// Insert writes a fresh review. A unique-index conflict (the partial unique
// index that ignores soft-deleted rows) is surfaced as ErrDuplicateReview so
// callers can render a friendly message.
func (p *Postgres) Insert(ctx context.Context, r domain.Review) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO reviews_review (id, product_id, customer_id, rating, body, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, r.ID(), r.ProductID(), r.CustomerID(), r.Rating(), r.Body(), r.CreatedAt())
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == pgUniqueViolation {
			return app.ErrDuplicateReview
		}
		return fmt.Errorf("insert review: %w", err)
	}
	return nil
}

// SoftDelete stamps deleted_at = now() on the matching row. Already-deleted
// reviews are left untouched.
func (p *Postgres) SoftDelete(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `
		UPDATE reviews_review SET deleted_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("soft-delete review: %w", err)
	}
	return nil
}

// ByProduct returns the active reviews for a product, newest-first, capped
// at limit. The DB query relies on the (product_id, created_at DESC) partial
// index for ordering.
func (p *Postgres) ByProduct(ctx context.Context, productID string, limit int) ([]domain.Review, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, product_id, customer_id, rating, body, created_at
		FROM reviews_review
		WHERE product_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2
	`, productID, limit)
	if err != nil {
		return nil, fmt.Errorf("query reviews: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Review
	for rows.Next() {
		var id, productID, customerID, body string
		var rating int
		var createdAt time.Time
		if err := rows.Scan(&id, &productID, &customerID, &rating, &body, &createdAt); err != nil {
			return nil, fmt.Errorf("scan review: %w", err)
		}
		out = append(out, domain.RebuildReview(id, productID, customerID, body, rating, createdAt, nil))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// AggregateForProducts runs a single grouped query over the requested ids
// (passed as a pq.Array($1::text[])) so the result set is bounded and
// ordering doesn't matter — callers index by product id.
func (p *Postgres) AggregateForProducts(ctx context.Context, productIDs []string) (map[string]domain.Aggregate, error) {
	if len(productIDs) == 0 {
		return map[string]domain.Aggregate{}, nil
	}
	rows, err := p.db.QueryContext(ctx, `
		SELECT product_id, AVG(rating)::float8, COUNT(*)
		FROM reviews_review
		WHERE product_id = ANY($1) AND deleted_at IS NULL
		GROUP BY product_id
	`, pq.Array(productIDs))
	if err != nil {
		return nil, fmt.Errorf("aggregate reviews: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]domain.Aggregate{}
	for rows.Next() {
		var pid string
		var avg float64
		var count int
		if err := rows.Scan(&pid, &avg, &count); err != nil {
			return nil, fmt.Errorf("scan aggregate: %w", err)
		}
		out[pid] = domain.RebuildAggregate(pid, avg, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// HasReviewed checks for any active review by the customer on the product.
func (p *Postgres) HasReviewed(ctx context.Context, productID, customerID string) (bool, error) {
	var dummy int
	err := p.db.QueryRowContext(ctx, `
		SELECT 1 FROM reviews_review
		WHERE product_id = $1 AND customer_id = $2 AND deleted_at IS NULL
		LIMIT 1
	`, productID, customerID).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("has reviewed: %w", err)
	}
	return true, nil
}
