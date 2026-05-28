package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/promo/app"
	"github.com/bkielbasa/go-ecommerce/backend/promo/domain"
	"github.com/lib/pq"
)

// pgUniqueViolation is the SQLSTATE postgres returns on a primary-key /
// unique-constraint conflict; used to map duplicate-code creates to a
// dedicated app-level error and to swallow the (code, order_id) PK on the
// redemption table for idempotency.
const pgUniqueViolation = "23505"

// Postgres is the production Storage adapter. Atomicity of Redeem is the
// load-bearing piece: it runs the read-and-mutate inside a single
// transaction with SELECT ... FOR UPDATE on the promo_code row.
type Postgres struct {
	db *sql.DB
}

// NewPostgres binds the adapter to the shared *sql.DB.
func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db}
}

// Create inserts a new catalogue row. A duplicate code text (PK) maps to
// app.ErrCodeAlreadyExists so the admin handler can flash a useful
// message.
func (p *Postgres) Create(ctx context.Context, c domain.Code) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO promo_code
			(code, kind, value_minor, currency, valid_from, valid_until,
			 max_uses, per_customer_max, used_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`,
		c.CodeText(), string(c.Kind()), c.ValueMinor(), c.Currency(),
		nullableTime(c.ValidFrom()), nullableTime(c.ValidUntil()),
		c.MaxUses(), c.PerCustomerMax(), c.UsedCount(), c.CreatedAt(),
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == pgUniqueViolation {
			return app.ErrCodeAlreadyExists
		}
		return fmt.Errorf("insert promo_code: %w", err)
	}
	return nil
}

// Update replaces every editable field on the catalogue row. used_count
// and created_at are intentionally left untouched — the admin form does
// not surface either.
func (p *Postgres) Update(ctx context.Context, c domain.Code) error {
	res, err := p.db.ExecContext(ctx, `
		UPDATE promo_code SET
			kind = $2,
			value_minor = $3,
			currency = $4,
			valid_from = $5,
			valid_until = $6,
			max_uses = $7,
			per_customer_max = $8
		WHERE code = $1
	`,
		c.CodeText(), string(c.Kind()), c.ValueMinor(), c.Currency(),
		nullableTime(c.ValidFrom()), nullableTime(c.ValidUntil()),
		c.MaxUses(), c.PerCustomerMax(),
	)
	if err != nil {
		return fmt.Errorf("update promo_code: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return app.ErrCodeNotFound
	}
	return nil
}

// Delete drops the catalogue row; the FK on promo_redemption cascades.
func (p *Postgres) Delete(ctx context.Context, code string) error {
	res, err := p.db.ExecContext(ctx, `DELETE FROM promo_code WHERE code = $1`, code)
	if err != nil {
		return fmt.Errorf("delete promo_code: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return app.ErrCodeNotFound
	}
	return nil
}

// Find loads a single catalogue row by code text.
func (p *Postgres) Find(ctx context.Context, code string) (domain.Code, error) {
	var kindStr, currency string
	var valueMinor int64
	var maxUses, perCustomerMax, usedCount int
	var validFrom, validUntil sql.NullTime
	var createdAt time.Time
	err := p.db.QueryRowContext(ctx, `
		SELECT kind::text, value_minor, currency, valid_from, valid_until,
		       max_uses, per_customer_max, used_count, created_at
		FROM promo_code WHERE code = $1
	`, code).Scan(&kindStr, &valueMinor, &currency, &validFrom, &validUntil,
		&maxUses, &perCustomerMax, &usedCount, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Code{}, app.ErrCodeNotFound
	}
	if err != nil {
		return domain.Code{}, fmt.Errorf("select promo_code: %w", err)
	}
	return domain.RebuildCode(
		code, domain.Kind(kindStr), valueMinor, currency,
		nullableTimePtr(validFrom), nullableTimePtr(validUntil),
		maxUses, perCustomerMax, usedCount, createdAt,
	), nil
}

// ListAll returns the catalogue newest-first.
func (p *Postgres) ListAll(ctx context.Context) ([]domain.Code, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT code, kind::text, value_minor, currency, valid_from, valid_until,
		       max_uses, per_customer_max, used_count, created_at
		FROM promo_code
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list promo_code: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Code
	for rows.Next() {
		var code, kindStr, currency string
		var valueMinor int64
		var maxUses, perCustomerMax, usedCount int
		var validFrom, validUntil sql.NullTime
		var createdAt time.Time
		if err := rows.Scan(&code, &kindStr, &valueMinor, &currency, &validFrom, &validUntil,
			&maxUses, &perCustomerMax, &usedCount, &createdAt); err != nil {
			return nil, fmt.Errorf("scan promo_code: %w", err)
		}
		out = append(out, domain.RebuildCode(
			code, domain.Kind(kindStr), valueMinor, currency,
			nullableTimePtr(validFrom), nullableTimePtr(validUntil),
			maxUses, perCustomerMax, usedCount, createdAt,
		))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// CountRedemptionsByCustomer returns the per-(code,customer) tally for
// the per-customer-max check at Resolve time.
func (p *Postgres) CountRedemptionsByCustomer(ctx context.Context, code, customerID string) (int, error) {
	var n int
	err := p.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM promo_redemption
		WHERE code = $1 AND customer_id = $2
	`, code, customerID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count promo_redemption: %w", err)
	}
	return n, nil
}

// Redeem runs the atomic redemption flow: take a row-level lock on the
// promo_code row, re-check the caps (max_uses + per_customer_max), insert
// the redemption (idempotent on the (code, order_id) PK), and increment
// used_count. The transaction commits as a single unit, so concurrent
// checkouts cannot oversubscribe a limited code.
func (p *Postgres) Redeem(ctx context.Context, r app.Redemption) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin redeem tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var maxUses, perCustomerMax, usedCount int
	err = tx.QueryRowContext(ctx, `
		SELECT max_uses, per_customer_max, used_count
		FROM promo_code WHERE code = $1
		FOR UPDATE
	`, r.Code).Scan(&maxUses, &perCustomerMax, &usedCount)
	if errors.Is(err, sql.ErrNoRows) {
		return app.ErrCodeNotFound
	}
	if err != nil {
		return fmt.Errorf("lock promo_code: %w", err)
	}
	if maxUses > 0 && usedCount >= maxUses {
		return app.ErrCodeMaxUsesReached
	}
	if perCustomerMax > 0 {
		var n int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM promo_redemption
			WHERE code = $1 AND customer_id = $2
		`, r.Code, r.CustomerID).Scan(&n); err != nil {
			return fmt.Errorf("count redemptions: %w", err)
		}
		if n >= perCustomerMax {
			return app.ErrCodeCustomerLimit
		}
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO promo_redemption
			(code, order_id, customer_id, discount_amount_minor, currency)
		VALUES ($1, $2, $3, $4, $5)
	`, r.Code, r.OrderID, r.CustomerID, r.AmountMinor, r.Currency)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == pgUniqueViolation {
			// The same order was already redeemed — idempotent retry,
			// commit nothing and return success.
			return tx.Rollback()
		}
		return fmt.Errorf("insert promo_redemption: %w", err)
	}

	if _, err = tx.ExecContext(ctx, `
		UPDATE promo_code SET used_count = used_count + 1 WHERE code = $1
	`, r.Code); err != nil {
		return fmt.Errorf("bump used_count: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit redeem: %w", err)
	}
	committed = true
	return nil
}

// nullableTime converts a *time.Time into a sql.NullTime so optional
// validity bounds can be persisted as NULL rather than the zero value.
func nullableTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// nullableTimePtr is the inverse: a *time.Time the domain layer keeps for
// optional bounds.
func nullableTimePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}
