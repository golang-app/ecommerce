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

	var customerID sql.NullString
	if order.CustomerID() != "" {
		customerID = sql.NullString{String: order.CustomerID(), Valid: true}
	}

	ship := order.ShipTo()
	method := order.ShippingMethod()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO checkout_order
			(id, user_id, customer_id, total_amount, total_currency, status, placed_at,
			 ship_name, ship_street1, ship_street2, ship_city, ship_zip, ship_country,
			 ship_method_code, ship_method_label, ship_cost)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			customer_id = EXCLUDED.customer_id,
			total_amount = EXCLUDED.total_amount,
			total_currency = EXCLUDED.total_currency,
			status = EXCLUDED.status,
			ship_name = EXCLUDED.ship_name,
			ship_street1 = EXCLUDED.ship_street1,
			ship_street2 = EXCLUDED.ship_street2,
			ship_city = EXCLUDED.ship_city,
			ship_zip = EXCLUDED.ship_zip,
			ship_country = EXCLUDED.ship_country,
			ship_method_code = EXCLUDED.ship_method_code,
			ship_method_label = EXCLUDED.ship_method_label,
			ship_cost = EXCLUDED.ship_cost
	`,
		order.ID(),
		order.UserID(),
		customerID,
		order.TotalAmount(),
		order.TotalCurrency(),
		string(order.Status()),
		order.PlacedAt(),
		ship.Name(),
		ship.Street1(),
		ship.Street2(),
		ship.City(),
		ship.Zip(),
		ship.Country(),
		method.Code(),
		method.Label(),
		method.Cost(),
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
	var customerID sql.NullString
	var shipName, shipStreet1, shipStreet2, shipCity, shipZip, shipCountry sql.NullString
	var shipMethodCode, shipMethodLabel sql.NullString
	var shipCost int64
	var totalAmt int64
	var placedAt time.Time

	err := p.db.QueryRowContext(ctx, `
		SELECT user_id, customer_id, total_amount, total_currency, status, placed_at,
		       ship_name, ship_street1, ship_street2, ship_city, ship_zip, ship_country,
		       ship_method_code, ship_method_label, ship_cost
		FROM checkout_order WHERE id = $1
	`, id).Scan(&userID, &customerID, &totalAmt, &currency, &status, &placedAt,
		&shipName, &shipStreet1, &shipStreet2, &shipCity, &shipZip, &shipCountry,
		&shipMethodCode, &shipMethodLabel, &shipCost)
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

	shipTo := domain.RebuildAddress(
		shipName.String, shipStreet1.String, shipStreet2.String,
		shipCity.String, shipZip.String, shipCountry.String,
	)
	shipMethod := domain.RebuildShippingMethod(shipMethodCode.String, shipMethodLabel.String, shipCost)

	return domain.NewOrder(id, userID, customerID.String, shipTo, shipMethod, lines, domain.Status(status), placedAt), nil
}

// ListByCustomer returns the customer's orders newest-first. Anonymous
// orders (NULL customer_id) are never returned. Items are hydrated by
// calling Find for each row (N+1) — fine for the expected order volume of
// this demo.
func (p Postgres) ListByCustomer(ctx context.Context, customerID string) ([]domain.Order, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id FROM checkout_order WHERE customer_id = $1
		ORDER BY placed_at DESC
	`, customerID)
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan order id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	orders := make([]domain.Order, 0, len(ids))
	for _, id := range ids {
		order, err := p.Find(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("hydrate order %s: %w", id, err)
		}
		orders = append(orders, order)
	}
	return orders, nil
}
