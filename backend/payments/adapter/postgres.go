package adapter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/payments/app"
	"github.com/bkielbasa/go-ecommerce/backend/payments/domain"
)

var _ app.Storage = Postgres{}

// Postgres is the durable Storage for payments_charge rows. The struct
// is a value type by convention: it is a thin wrapper around the pool
// handle, with no per-instance mutable state.
type Postgres struct {
	db *sql.DB
}

// NewPostgresStorage builds a Postgres-backed Storage.
func NewPostgresStorage(db *sql.DB) Postgres {
	return Postgres{db: db}
}

// Insert stages a new pending Charge. A duplicate idempotency_key is
// mapped to app.ErrIdempotencyKeyConflict so the service can recover
// by reading the existing row.
func (p Postgres) Insert(ctx context.Context, c domain.Charge) error {
	var key any
	if c.IdempotencyKey() == "" {
		// Keep the column NULL when no key was supplied — UNIQUE
		// constraints on text columns are per-non-null in Postgres,
		// so this lets blank-key inserts coexist.
		key = nil
	} else {
		key = c.IdempotencyKey()
	}
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO payments_charge (id, idempotency_key, amount, currency, status, provider_ref, provider, order_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, c.ID(), key, c.Amount(), c.Currency(), string(c.Status()), c.ProviderRef(), c.Provider(), c.OrderID(), c.CreatedAt(), c.UpdatedAt())
	if err != nil {
		if isUniqueViolationOnIdempotencyKey(err) {
			return app.ErrIdempotencyKeyConflict
		}
		return fmt.Errorf("payments postgres: insert %s: %w", c.ID(), err)
	}
	return nil
}

// Find returns the Charge by id or app.ErrChargeNotFound.
func (p Postgres) Find(ctx context.Context, id string) (domain.Charge, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(idempotency_key, ''), amount, currency, status, provider_ref, provider, order_id, created_at, updated_at
		FROM payments_charge WHERE id = $1
	`, id)
	return scanCharge(row)
}

// UpdateStatus transitions the Charge in place. The CHECK constraint
// on `status` keeps us honest — an unknown value here would surface
// as a constraint-violation error rather than silently corrupting the
// row.
func (p Postgres) UpdateStatus(ctx context.Context, id string, status domain.Status, providerRef string, updatedAt time.Time) error {
	res, err := p.db.ExecContext(ctx, `
		UPDATE payments_charge
		SET status = $2,
		    provider_ref = CASE WHEN $3 = '' THEN provider_ref ELSE $3 END,
		    updated_at = $4
		WHERE id = $1
	`, id, string(status), providerRef, updatedAt)
	if err != nil {
		return fmt.Errorf("payments postgres: update status %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("payments postgres: rows affected %s: %w", id, err)
	}
	if n == 0 {
		return app.ErrChargeNotFound
	}
	return nil
}

// FindByIdempotencyKey returns the Charge for a key if one is on
// file; the bool is false when no row exists.
func (p Postgres) FindByIdempotencyKey(ctx context.Context, key string) (domain.Charge, bool, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(idempotency_key, ''), amount, currency, status, provider_ref, provider, order_id, created_at, updated_at
		FROM payments_charge WHERE idempotency_key = $1
	`, key)
	c, err := scanCharge(row)
	if errors.Is(err, app.ErrChargeNotFound) {
		return domain.Charge{}, false, nil
	}
	if err != nil {
		return domain.Charge{}, false, err
	}
	return c, true, nil
}

// FindByProviderRef returns the Charge for the given external
// provider reference. app.ErrChargeNotFound when nothing matches.
func (p Postgres) FindByProviderRef(ctx context.Context, providerRef string) (domain.Charge, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(idempotency_key, ''), amount, currency, status, provider_ref, provider, order_id, created_at, updated_at
		FROM payments_charge WHERE provider_ref = $1
	`, providerRef)
	return scanCharge(row)
}

// FindByOrderID returns the most recent Charge associated with the given orderID.
// app.ErrChargeNotFound is returned when no matching charge exists.
func (p Postgres) FindByOrderID(ctx context.Context, orderID string) (domain.Charge, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, COALESCE(idempotency_key, ''), amount, currency, status, provider_ref, provider, order_id, created_at, updated_at
		FROM payments_charge WHERE order_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, orderID)
	return scanCharge(row)
}

// ListProviders returns all provider configurations ordered by ID ascending.
func (p Postgres) ListProviders(ctx context.Context) ([]domain.ProviderConfig, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, enabled, config, updated_at
		FROM payments_provider_config
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("payments postgres: list providers: %w", err)
	}
	defer rows.Close()

	var providers []domain.ProviderConfig
	for rows.Next() {
		var (
			id        string
			enabled   bool
			rawConfig []byte
			updatedAt time.Time
		)
		if err := rows.Scan(&id, &enabled, &rawConfig, &updatedAt); err != nil {
			return nil, fmt.Errorf("payments postgres: scan provider: %w", err)
		}
		var cfgMap map[string]string
		if len(rawConfig) > 0 {
			if err := json.Unmarshal(rawConfig, &cfgMap); err != nil {
				return nil, fmt.Errorf("payments postgres: unmarshal provider config for %s: %w", id, err)
			}
		}
		providers = append(providers, domain.NewProviderConfig(id, enabled, cfgMap, updatedAt))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("payments postgres: iterate providers: %w", err)
	}
	return providers, nil
}

// FindProvider returns a single ProviderConfig by id, or app.ErrProviderNotFound.
func (p Postgres) FindProvider(ctx context.Context, id string) (domain.ProviderConfig, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, enabled, config, updated_at
		FROM payments_provider_config
		WHERE id = $1
	`, id)
	var (
		providerID string
		enabled    bool
		rawConfig  []byte
		updatedAt  time.Time
	)
	err := row.Scan(&providerID, &enabled, &rawConfig, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ProviderConfig{}, app.ErrProviderNotFound
	}
	if err != nil {
		return domain.ProviderConfig{}, fmt.Errorf("payments postgres: find provider %s: %w", id, err)
	}
	var cfgMap map[string]string
	if len(rawConfig) > 0 {
		if err := json.Unmarshal(rawConfig, &cfgMap); err != nil {
			return domain.ProviderConfig{}, fmt.Errorf("payments postgres: unmarshal provider config for %s: %w", id, err)
		}
	}
	return domain.NewProviderConfig(providerID, enabled, cfgMap, updatedAt), nil
}

// SaveProvider inserts or updates a provider configuration.
func (p Postgres) SaveProvider(ctx context.Context, cfg domain.ProviderConfig) error {
	rawConfig, err := json.Marshal(cfg.Config())
	if err != nil {
		return fmt.Errorf("payments postgres: marshal provider config for %s: %w", cfg.ID(), err)
	}
	_, err = p.db.ExecContext(ctx, `
		INSERT INTO payments_provider_config (id, enabled, config, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET enabled = EXCLUDED.enabled,
		    config = EXCLUDED.config,
		    updated_at = EXCLUDED.updated_at
	`, cfg.ID(), cfg.IsEnabled(), rawConfig, cfg.UpdatedAt())
	if err != nil {
		return fmt.Errorf("payments postgres: save provider %s: %w", cfg.ID(), err)
	}
	return nil
}

// scanCharge is the shared row->Charge decoder. The narrow type avoids
// having to assert on QueryRowContext vs QueryRowContext.Scan return
// signatures in every caller.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanCharge(r rowScanner) (domain.Charge, error) {
	var (
		id          string
		key         string
		amount      int64
		currency    string
		status      string
		providerRef string
		provider    string
		orderID     string
		createdAt   time.Time
		updatedAt   time.Time
	)
	err := r.Scan(&id, &key, &amount, &currency, &status, &providerRef, &provider, &orderID, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Charge{}, app.ErrChargeNotFound
	}
	if err != nil {
		return domain.Charge{}, fmt.Errorf("payments postgres: scan: %w", err)
	}
	return domain.RebuildCharge(id, key, amount, currency, domain.Status(status), providerRef, provider, orderID, createdAt, updatedAt), nil
}

// isUniqueViolationOnIdempotencyKey heuristically detects the
// idempotency_key unique-violation. We match on the string form to
// avoid taking a direct dependency on lib/pq's error type from the
// storage package; the message format is stable in pq and we widen
// the catch to anything mentioning the index name. False positives
// surface as ErrIdempotencyKeyConflict, which the service then
// reconciles by re-reading the row — safe by construction.
func isUniqueViolationOnIdempotencyKey(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") && strings.Contains(msg, "idempotency_key")
}
