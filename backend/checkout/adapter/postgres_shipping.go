package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/app"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

var _ app.ShippingStorage = (*PostgresShippingStorage)(nil)

type PostgresShippingStorage struct {
	db *sql.DB
}

func NewPostgresShippingStorage(db *sql.DB) *PostgresShippingStorage {
	return &PostgresShippingStorage{
		db: db,
	}
}

func (s *PostgresShippingStorage) ListShippingMethods(ctx context.Context) ([]domain.ShippingMethod, error) {
	query := `SELECT code, enabled, label, cost, requires_address, carrier FROM shipping_method_config ORDER BY code ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list shipping methods: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var methods []domain.ShippingMethod
	for rows.Next() {
		var (
			code            string
			enabled         bool
			label           string
			cost            int64
			requiresAddress bool
			carrier         string
		)
		if err := rows.Scan(&code, &enabled, &label, &cost, &requiresAddress, &carrier); err != nil {
			return nil, fmt.Errorf("scan shipping method: %w", err)
		}
		methods = append(methods, domain.NewShippingMethod(code, label, cost, requiresAddress, carrier, enabled))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate shipping methods: %w", err)
	}

	return methods, nil
}

func (s *PostgresShippingStorage) FindShippingMethod(ctx context.Context, code string) (domain.ShippingMethod, error) {
	query := `SELECT code, enabled, label, cost, requires_address, carrier FROM shipping_method_config WHERE code = $1`

	row := s.db.QueryRowContext(ctx, query, code)

	var (
		codeVal         string
		enabled         bool
		label           string
		cost            int64
		requiresAddress bool
		carrier         string
	)

	if err := row.Scan(&codeVal, &enabled, &label, &cost, &requiresAddress, &carrier); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ShippingMethod{}, app.ErrShippingMethodNotFound
		}
		return domain.ShippingMethod{}, fmt.Errorf("find shipping method %q: %w", code, err)
	}

	return domain.NewShippingMethod(codeVal, label, cost, requiresAddress, carrier, enabled), nil
}

func (s *PostgresShippingStorage) SaveShippingMethod(ctx context.Context, method domain.ShippingMethod) error {
	query := `
		INSERT INTO shipping_method_config (code, enabled, label, cost, requires_address, carrier, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (code) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			label = EXCLUDED.label,
			cost = EXCLUDED.cost,
			requires_address = EXCLUDED.requires_address,
			carrier = EXCLUDED.carrier,
			updated_at = NOW()
	`

	_, err := s.db.ExecContext(
		ctx,
		query,
		method.Code(),
		method.IsEnabled(),
		method.Label(),
		method.Cost(),
		method.RequiresAddress(),
		method.Carrier(),
	)
	if err != nil {
		return fmt.Errorf("save shipping method %q: %w", method.Code(), err)
	}

	return nil
}
