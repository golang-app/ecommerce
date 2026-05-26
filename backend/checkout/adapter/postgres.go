package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
)

type Postgres struct {
	db *sql.DB
}

func NewPostgres(db *sql.DB) Postgres {
	return Postgres{db: db}
}

func (p Postgres) Save(ctx context.Context, order domain.Order) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO checkout_order
			(id, user_id, total_amount, total_currency, status, placed_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			total_amount = EXCLUDED.total_amount,
			total_currency = EXCLUDED.total_currency,
			status = EXCLUDED.status
	`,
		order.ID(),
		order.UserID(),
		order.TotalAmount(),
		order.TotalCurrency(),
		string(order.Status()),
		order.PlacedAt(),
	)
	if err != nil {
		return fmt.Errorf("upsert order: %w", err)
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM checkout_order_item WHERE order_id = $1`, order.ID()); err != nil {
		return fmt.Errorf("delete items: %w", err)
	}

	for i, ln := range order.Items() {
		itemID := fmt.Sprintf("%s-%d", order.ID(), i)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO checkout_order_item
				(id, order_id, product_id, product_name, qty, price_amount, price_currency)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`,
			itemID,
			order.ID(),
			ln.ProductID(),
			ln.ProductName(),
			ln.Quantity(),
			ln.PriceAmount(),
			ln.PriceCurrency(),
		)
		if err != nil {
			return fmt.Errorf("insert item: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (p Postgres) Find(ctx context.Context, id string) (domain.Order, error) {
	var userID, status, currency string
	var totalAmt int64
	var placedAt time.Time

	err := p.db.QueryRowContext(ctx, `
		SELECT user_id, total_amount, total_currency, status, placed_at
		FROM checkout_order WHERE id = $1
	`, id).Scan(&userID, &totalAmt, &currency, &status, &placedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("query order: %w", err)
	}

	rows, err := p.db.QueryContext(ctx, `
		SELECT product_id, product_name, qty, price_amount, price_currency
		FROM checkout_order_item WHERE order_id = $1
		ORDER BY id
	`, id)
	if err != nil {
		return domain.Order{}, fmt.Errorf("query items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var lines []domain.Line
	for rows.Next() {
		var productID, productName, ccy string
		var qty int
		var amt int64
		if err := rows.Scan(&productID, &productName, &qty, &amt, &ccy); err != nil {
			return domain.Order{}, fmt.Errorf("scan item: %w", err)
		}
		lines = append(lines, domain.NewLine(productID, productName, qty, amt, ccy))
	}
	if err := rows.Err(); err != nil {
		return domain.Order{}, fmt.Errorf("rows: %w", err)
	}

	return domain.NewOrder(id, userID, lines, domain.Status(status), placedAt), nil
}
