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
//
// Trade-off note on the unique index: the partial unique constraint
// `(product_id, customer_id) WHERE deleted_at IS NULL` is intentionally
// status-agnostic. That means once a customer's review is rejected they
// cannot resubmit a fresh (pending) review for the same product —
// resubmission is gated on the admin first soft-deleting the rejected row,
// which clears the partial-unique key. The alternative (relaxing the index
// to additionally exclude rejected rows) was rejected on purpose because
// it would let a customer flood the queue with new submissions every time
// one was rejected. Admins can soft-delete to reset.
func (p *Postgres) Insert(ctx context.Context, r domain.Review) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO reviews_review (id, product_id, customer_id, rating, body, created_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, r.ID(), r.ProductID(), r.CustomerID(), r.Rating(), r.Body(), r.CreatedAt(), string(r.Status()))
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

// SetStatus flips a review's moderation state. The CHECK constraint on the
// column protects against unknown values even if a caller bypassed the
// domain layer's validation.
func (p *Postgres) SetStatus(ctx context.Context, id string, status domain.Status) error {
	_, err := p.db.ExecContext(ctx, `
		UPDATE reviews_review SET status = $2
		WHERE id = $1 AND deleted_at IS NULL
	`, id, string(status))
	if err != nil {
		return fmt.Errorf("set review status: %w", err)
	}
	return nil
}

// ByProduct returns the approved reviews for a product, newest-first, capped
// at limit. Pending and rejected reviews are deliberately excluded — this is
// the storefront-facing query. The DB query relies on the
// (product_id, created_at DESC) partial index for ordering.
func (p *Postgres) ByProduct(ctx context.Context, productID string, limit int) ([]domain.Review, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, product_id, customer_id, rating, body, created_at, status
		FROM reviews_review
		WHERE product_id = $1 AND deleted_at IS NULL AND status = 'approved'
		ORDER BY created_at DESC
		LIMIT $2
	`, productID, limit)
	if err != nil {
		return nil, fmt.Errorf("query reviews: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Review
	for rows.Next() {
		var id, productID, customerID, body, status string
		var rating int
		var createdAt time.Time
		if err := rows.Scan(&id, &productID, &customerID, &rating, &body, &createdAt, &status); err != nil {
			return nil, fmt.Errorf("scan review: %w", err)
		}
		out = append(out, domain.RebuildReview(id, productID, customerID, body, rating, createdAt, nil, domain.Status(status)))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// AggregateForProducts runs a single grouped query over the requested ids
// (passed as a pq.Array($1::text[])) so the result set is bounded and
// ordering doesn't matter — callers index by product id. Only approved
// reviews participate: pending submissions and rejected entries do not
// count towards the storefront badge.
func (p *Postgres) AggregateForProducts(ctx context.Context, productIDs []string) (map[string]domain.Aggregate, error) {
	if len(productIDs) == 0 {
		return map[string]domain.Aggregate{}, nil
	}
	rows, err := p.db.QueryContext(ctx, `
		SELECT product_id, AVG(rating)::float8, COUNT(*)
		FROM reviews_review
		WHERE product_id = ANY($1) AND deleted_at IS NULL AND status = 'approved'
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

// HasReviewed checks for any active (non-deleted) review by the customer on
// the product, regardless of moderation status. This guards the storefront
// from showing the submit form when the customer already has a pending,
// approved, or rejected entry in the queue — matching the partial unique
// index's "one active row per (customer, product)" guarantee.
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

// ListByStatus returns every non-deleted review at the given status, newest
// first. Powers the admin "pending" / "approved" / "rejected" tabs.
func (p *Postgres) ListByStatus(ctx context.Context, status domain.Status, limit int) ([]domain.Review, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, product_id, customer_id, rating, body, created_at, status
		FROM reviews_review
		WHERE deleted_at IS NULL AND status = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, string(status), limit)
	if err != nil {
		return nil, fmt.Errorf("list reviews by status: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanReviewRows(rows)
}

// ListAll returns every non-deleted review (any status), newest first.
// Powers the admin "all" tab.
func (p *Postgres) ListAll(ctx context.Context, limit int) ([]domain.Review, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, product_id, customer_id, rating, body, created_at, status
		FROM reviews_review
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list all reviews: %w", err)
	}
	defer func() { _ = rows.Close() }()

	return scanReviewRows(rows)
}

// scanReviewRows drains a SELECT that produces the same column shape as
// ListByStatus / ListAll. Pulled out so the two list queries stay short.
func scanReviewRows(rows *sql.Rows) ([]domain.Review, error) {
	var out []domain.Review
	for rows.Next() {
		var id, productID, customerID, body, status string
		var rating int
		var createdAt time.Time
		if err := rows.Scan(&id, &productID, &customerID, &rating, &body, &createdAt, &status); err != nil {
			return nil, fmt.Errorf("scan review: %w", err)
		}
		out = append(out, domain.RebuildReview(id, productID, customerID, body, rating, createdAt, nil, domain.Status(status)))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}
