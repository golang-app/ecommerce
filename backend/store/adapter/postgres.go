package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/store/app"
	"github.com/bkielbasa/go-ecommerce/backend/store/domain"
)

// Postgres is the production Storage adapter for the store bounded
// context. It is a thin shell over *sql.DB; parameterised SQL is used
// everywhere.
type Postgres struct {
	db *sql.DB
}

// NewPostgres binds the adapter to the shared *sql.DB.
func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db}
}

// Create inserts a new store row. When the caller marks the new row
// default, every other row's is_default flag is cleared first inside
// the same transaction. The partial unique index on (is_default) WHERE
// is_default is the safety net that prevents two concurrent inserts
// from both claiming the default flag.
func (p *Postgres) Create(ctx context.Context, s domain.Store) error {
	return p.inTx(ctx, func(tx *sql.Tx) error {
		if s.IsDefault() {
			if err := clearOtherDefaults(ctx, tx, s.ID()); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO store (id, slug, name, currency, host, is_default, position)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, s.ID(), s.Slug(), s.Name(), s.Currency(), s.Host(), s.IsDefault(), s.Position())
		if err != nil {
			return fmt.Errorf("insert store: %w", err)
		}
		return nil
	})
}

// Update replaces every field on a store row. When the caller marks
// the row default, every other row is cleared inside the same
// transaction.
func (p *Postgres) Update(ctx context.Context, s domain.Store) error {
	return p.inTx(ctx, func(tx *sql.Tx) error {
		if s.IsDefault() {
			if err := clearOtherDefaults(ctx, tx, s.ID()); err != nil {
				return err
			}
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE store
			SET slug = $2,
			    name = $3,
			    currency = $4,
			    host = $5,
			    is_default = $6,
			    position = $7
			WHERE id = $1
		`, s.ID(), s.Slug(), s.Name(), s.Currency(), s.Host(), s.IsDefault(), s.Position())
		if err != nil {
			return fmt.Errorf("update store: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return app.ErrStoreNotFound
		}
		return nil
	})
}

// Delete drops the store row by id.
func (p *Postgres) Delete(ctx context.Context, id string) error {
	res, err := p.db.ExecContext(ctx, `DELETE FROM store WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete store: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return app.ErrStoreNotFound
	}
	return nil
}

// Find loads a single store by id.
func (p *Postgres) Find(ctx context.Context, id string) (domain.Store, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, slug, name, currency, host, is_default, position
		FROM store WHERE id = $1
	`, id)
	return scanStore(row)
}

// FindByHost loads the store whose host column matches (case-
// insensitive). The host_idx index makes the lookup cheap.
func (p *Postgres) FindByHost(ctx context.Context, host string) (domain.Store, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, slug, name, currency, host, is_default, position
		FROM store WHERE lower(host) = lower($1)
	`, strings.TrimSpace(host))
	return scanStore(row)
}

// Default returns the store marked as default. The partial unique
// index guarantees at most one row matches.
func (p *Postgres) Default(ctx context.Context) (domain.Store, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, slug, name, currency, host, is_default, position
		FROM store WHERE is_default = true
	`)
	s, err := scanStore(row)
	if errors.Is(err, app.ErrStoreNotFound) {
		return domain.Store{}, app.ErrNoDefaultStore
	}
	return s, err
}

// ListAll returns every store ordered by position then name.
func (p *Postgres) ListAll(ctx context.Context) ([]domain.Store, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, slug, name, currency, host, is_default, position
		FROM store
		ORDER BY position ASC, name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list store: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Store
	for rows.Next() {
		var id, slug, name, currency, host string
		var isDefault bool
		var position int
		if err := rows.Scan(&id, &slug, &name, &currency, &host, &isDefault, &position); err != nil {
			return nil, fmt.Errorf("scan store: %w", err)
		}
		out = append(out, domain.RebuildStore(id, slug, name, currency, host, isDefault, position))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// scanStore reads a single row produced by every "SELECT id, slug,
// name, currency, host, is_default, position FROM store" query. A
// missing row maps to app.ErrStoreNotFound so callers can branch.
func scanStore(row *sql.Row) (domain.Store, error) {
	var id, slug, name, currency, host string
	var isDefault bool
	var position int
	err := row.Scan(&id, &slug, &name, &currency, &host, &isDefault, &position)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Store{}, app.ErrStoreNotFound
	}
	if err != nil {
		return domain.Store{}, fmt.Errorf("select store: %w", err)
	}
	return domain.RebuildStore(id, slug, name, currency, host, isDefault, position), nil
}

// clearOtherDefaults flips is_default to false on every row except the
// one with the supplied id. Called inside the same transaction as the
// upsert so a concurrent writer cannot leave the table with two
// defaults (the partial unique index would reject one of them anyway,
// but doing the clear up front is what makes the happy path work).
func clearOtherDefaults(ctx context.Context, tx *sql.Tx, keepID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE store SET is_default = false WHERE id <> $1`, keepID)
	if err != nil {
		return fmt.Errorf("clear other defaults: %w", err)
	}
	return nil
}

// inTx is the standard run-in-a-transaction helper. The callback runs
// inside a database transaction that is committed on return-nil and
// rolled back otherwise.
func (p *Postgres) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	return nil
}
