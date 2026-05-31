package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/repricing/app"
	"github.com/bkielbasa/go-ecommerce/backend/repricing/domain"
	"github.com/lib/pq"
)

// pgUniqueViolation is the SQLSTATE code postgres returns when a
// write hits a unique-constraint conflict. We map it to
// app.ErrAlreadyActive so callers can branch on the sentinel
// without reaching for pq.Error themselves. The relevant constraint
// is the partial unique index on the `status` column (see migration
// 000039_repricing) which enforces the at-most-one-active-row
// invariant.
const pgUniqueViolation = "23505"

// Postgres is the production Storage adapter.
type Postgres struct {
	db *sql.DB
}

// NewPostgres builds the production adapter bound to the supplied
// DB connection.
func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db}
}

// Create inserts a fresh reprice row. A unique-constraint conflict
// (from the partial unique index on the active statuses) is
// surfaced as app.ErrAlreadyActive — the application service uses
// it as the belt-and-braces guard against two concurrent admins
// clicking the button at the same time.
func (p *Postgres) Create(ctx context.Context, r domain.Reprice) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO repricing (
			id, category_id, percent_change, status,
			total_items, processed_items, last_error,
			started_at, completed_at, version
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`,
		r.ID(), r.CategoryID(), r.PercentChange(), string(r.Status()),
		r.TotalItems(), r.ProcessedItems(), r.LastError(),
		r.StartedAt(), nullableTimePtr(r.CompletedAt()), r.Version(),
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == pgUniqueViolation {
			return app.ErrAlreadyActive
		}
		return fmt.Errorf("insert reprice: %w", err)
	}
	return nil
}

// Update writes a state transition under optimistic concurrency.
// The UPDATE matches both the id and the previous version; a
// 0-rows-affected result is mapped to app.ErrOptimisticLock.
func (p *Postgres) Update(ctx context.Context, r domain.Reprice) error {
	res, err := p.db.ExecContext(ctx, `
		UPDATE repricing
		SET category_id = $2,
		    percent_change = $3,
		    status = $4,
		    total_items = $5,
		    processed_items = $6,
		    last_error = $7,
		    started_at = $8,
		    completed_at = $9,
		    version = $10
		WHERE id = $1 AND version = $11
	`,
		r.ID(),
		r.CategoryID(),
		r.PercentChange(),
		string(r.Status()),
		r.TotalItems(),
		r.ProcessedItems(),
		r.LastError(),
		r.StartedAt(),
		nullableTimePtr(r.CompletedAt()),
		r.Version(),
		r.Version()-1,
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == pgUniqueViolation {
			// A status transition that would create a
			// second active row gets caught by the partial
			// unique index. Map it onto the same sentinel
			// Create uses so callers do not need to branch
			// on the verb.
			return app.ErrAlreadyActive
		}
		return fmt.Errorf("update reprice: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return app.ErrOptimisticLock
	}
	return nil
}

// Find loads the row by id, mapping sql.ErrNoRows to
// app.ErrNotFound.
func (p *Postgres) Find(ctx context.Context, id string) (domain.Reprice, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, category_id, percent_change, status,
		       total_items, processed_items, last_error,
		       started_at, completed_at, version
		FROM repricing
		WHERE id = $1
	`, id)
	return scanOne(row)
}

// FindActive returns the at-most-one active row.
func (p *Postgres) FindActive(ctx context.Context) (domain.Reprice, bool, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, category_id, percent_change, status,
		       total_items, processed_items, last_error,
		       started_at, completed_at, version
		FROM repricing
		WHERE status IN ('scheduled', 'in_progress')
		ORDER BY started_at DESC
		LIMIT 1
	`)
	r, err := scanOne(row)
	if errors.Is(err, app.ErrNotFound) {
		return domain.Reprice{}, false, nil
	}
	if err != nil {
		return domain.Reprice{}, false, err
	}
	return r, true, nil
}

// ListAll returns every row, newest-started first.
func (p *Postgres) ListAll(ctx context.Context) ([]domain.Reprice, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, category_id, percent_change, status,
		       total_items, processed_items, last_error,
		       started_at, completed_at, version
		FROM repricing
		ORDER BY started_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list reprices: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Reprice
	for rows.Next() {
		r, scanErr := scanRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// scanOne loads a single row (used by Find / FindActive).
func scanOne(row *sql.Row) (domain.Reprice, error) {
	r, err := scanRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Reprice{}, app.ErrNotFound
	}
	if err != nil {
		return domain.Reprice{}, fmt.Errorf("scan reprice: %w", err)
	}
	return r, nil
}

// rowScanner is the narrow interface implemented by both *sql.Row
// and *sql.Rows so scanRow shares the same code path.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanRow(s rowScanner) (domain.Reprice, error) {
	var (
		id, categoryID, status string
		percentChange          float64
		totalItems             int
		processedItems         int
		lastError              string
		startedAt              time.Time
		completedAt            sql.NullTime
		version                int
	)
	if err := s.Scan(
		&id, &categoryID, &percentChange, &status,
		&totalItems, &processedItems, &lastError,
		&startedAt, &completedAt, &version,
	); err != nil {
		return domain.Reprice{}, err
	}
	var completedPtr *time.Time
	if completedAt.Valid {
		t := completedAt.Time
		completedPtr = &t
	}
	return domain.Rebuild(
		id, categoryID,
		percentChange,
		domain.Status(status),
		totalItems, processedItems,
		lastError,
		startedAt, completedPtr,
		version,
	), nil
}

// nullableTimePtr converts a nil/zero *time.Time into a NULL on
// the wire so the schema's nullable completed_at column carries
// the "not yet finished" meaning rather than a misleading epoch
// value.
func nullableTimePtr(t *time.Time) interface{} {
	if t == nil || t.IsZero() {
		return nil
	}
	return *t
}
