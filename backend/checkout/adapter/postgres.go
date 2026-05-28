package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type Postgres struct {
	db *sql.DB
}

func NewPostgres(db *sql.DB) Postgres {
	return Postgres{db: db}
}

// Save appends the aggregate's pending events to the event store and projects
// each one into the read model — all in a single transaction, so the event
// log and the read tables can never diverge.
func (p Postgres) Save(ctx context.Context, order *domain.Order) error {
	pending := order.PendingEvents()
	if len(pending) == 0 {
		return nil
	}

	ctx, span := adapterTracer.Start(ctx, "checkout.adapter.Save", trace.WithAttributes(
		attribute.String("order.id", order.ID()),
		attribute.Int("checkout.pending_events", len(pending)),
	))
	defer span.End()
	start := time.Now()
	defer func() {
		observability.Metrics().DBQueryDurationSec.Record(ctx, time.Since(start).Seconds(),
			metric.WithAttributes(attribute.String("query", "checkout.save")),
		)
	}()

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = appendEventsTx(ctx, tx, order.ID(), order.ExpectedVersion(), pending); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	for _, e := range pending {
		if err = projectEventTx(ctx, tx, e); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("commit: %w", err)
	}

	order.ClearPending()
	return nil
}

// Find reads the projection tables and returns the detail read model. It never
// touches the event log or the write aggregate.
func (p Postgres) Find(ctx context.Context, id string) (query.OrderView, error) {
	var userID, status, currency string
	var customerID sql.NullString
	var shipName, shipStreet1, shipStreet2, shipCity, shipZip, shipCountry sql.NullString
	var shipMethodCode, shipMethodLabel sql.NullString
	var payMethodCode, payMethodLabel sql.NullString
	var shipCost, taxAmt int64
	var carrier, trackingCode string
	var totalAmt int64
	var placedAt time.Time

	err := p.db.QueryRowContext(ctx, `
		SELECT user_id, customer_id, total_amount, total_currency, status, placed_at,
		       ship_name, ship_street1, ship_street2, ship_city, ship_zip, ship_country,
		       ship_method_code, ship_method_label, ship_cost,
		       payment_method_code, payment_method_label,
		       tax_amount, carrier, tracking_code
		FROM checkout_order WHERE id = $1
	`, id).Scan(&userID, &customerID, &totalAmt, &currency, &status, &placedAt,
		&shipName, &shipStreet1, &shipStreet2, &shipCity, &shipZip, &shipCountry,
		&shipMethodCode, &shipMethodLabel, &shipCost,
		&payMethodCode, &payMethodLabel,
		&taxAmt, &carrier, &trackingCode)
	if errors.Is(err, sql.ErrNoRows) {
		return query.OrderView{}, domain.ErrOrderNotFound
	}
	if err != nil {
		return query.OrderView{}, fmt.Errorf("query order: %w", err)
	}

	rows, err := p.db.QueryContext(ctx, `
		SELECT product_id, product_name, qty, price_amount, price_currency
		FROM checkout_order_item WHERE order_id = $1
		ORDER BY id
	`, id)
	if err != nil {
		return query.OrderView{}, fmt.Errorf("query items: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var lines []domain.Line
	for rows.Next() {
		var productID, productName, ccy string
		var qty int
		var amt int64
		if err := rows.Scan(&productID, &productName, &qty, &amt, &ccy); err != nil {
			return query.OrderView{}, fmt.Errorf("scan item: %w", err)
		}
		lines = append(lines, domain.NewLine(productID, productName, qty, amt, ccy))
	}
	if err := rows.Err(); err != nil {
		return query.OrderView{}, fmt.Errorf("rows: %w", err)
	}

	shipTo := domain.RebuildAddress(
		shipName.String, shipStreet1.String, shipStreet2.String,
		shipCity.String, shipZip.String, shipCountry.String,
	)
	shipMethod := domain.RebuildShippingMethod(shipMethodCode.String, shipMethodLabel.String, shipCost)
	payMethod := domain.RebuildPaymentMethod(payMethodCode.String, payMethodLabel.String)

	subtotal := totalAmt - shipCost - taxAmt
	return query.NewOrderView(
		id, customerID.String, domain.Status(status), placedAt,
		lines, shipTo, shipMethod, payMethod,
		subtotal, taxAmt, shipCost, totalAmt, currency,
		carrier, trackingCode,
	), nil
}

// ListByCustomer returns the customer's orders newest-first. Anonymous
// orders (NULL customer_id) are never returned. Items are hydrated by
// calling Find for each row (N+1) — fine for the expected order volume of
// this demo.
// ListByCustomer returns the customer's orders newest-first as list summaries,
// reading straight from the projection tables in a single grouped query.
func (p Postgres) ListByCustomer(ctx context.Context, customerID string) ([]query.OrderSummary, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT o.id, o.status, o.placed_at, o.total_amount, o.total_currency,
		       COUNT(i.id) AS item_count
		FROM checkout_order o
		LEFT JOIN checkout_order_item i ON i.order_id = o.id
		WHERE o.customer_id = $1
		GROUP BY o.id, o.status, o.placed_at, o.total_amount, o.total_currency
		ORDER BY o.placed_at DESC
	`, customerID)
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var summaries []query.OrderSummary
	for rows.Next() {
		var id, status, currency string
		var placedAt time.Time
		var total int64
		var itemCount int
		if err := rows.Scan(&id, &status, &placedAt, &total, &currency, &itemCount); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		summaries = append(summaries, query.NewOrderSummary(customerID, id, domain.Status(status), placedAt, itemCount, total, currency))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return summaries, nil
}

// ListExpiredPending returns the ids of orders still in the pending status
// whose placed_at is strictly older than olderThan. The reservation TTL
// sweeper uses this list to release orphaned stock reservations.
func (p Postgres) ListExpiredPending(ctx context.Context, olderThan time.Time) ([]string, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id
		FROM checkout_order
		WHERE status = $1 AND placed_at < $2
		ORDER BY placed_at
	`, string(domain.StatusPending), olderThan)
	if err != nil {
		return nil, fmt.Errorf("query expired pending: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan expired pending: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return ids, nil
}

// HasPurchasedProduct reports whether the customer has at least one
// fulfilled order (paid / shipped / delivered) that contains any variant of
// the given catalog product.
//
// Important nuance: the checkout_order_item.product_id column actually stores
// the catalogue variant id (the cart adds variants, see
// checkout/app/checkout.go Place where lines are built from item.Product().
// ID()). To answer "did this customer buy this product?" we therefore join
// the order line through productcatalog_variant to recover the parent
// product id.
func (p Postgres) HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error) {
	var dummy int
	err := p.db.QueryRowContext(ctx, `
		SELECT 1
		FROM checkout_order_item oi
		JOIN checkout_order o ON o.id = oi.order_id
		JOIN productcatalog_variant v ON v.id = oi.product_id
		WHERE o.customer_id = $1
		  AND v.product_id = $2
		  AND o.status IN ('paid', 'shipped', 'delivered')
		LIMIT 1
	`, customerID, productID).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("has purchased product: %w", err)
	}
	return true, nil
}

// ListAll returns every order newest-first as list summaries, regardless of
// customer (including anonymous orders with a NULL customer_id). It powers the
// admin order list and uses the same grouped query as ListByCustomer without
// the customer filter.
func (p Postgres) ListAll(ctx context.Context) ([]query.OrderSummary, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT o.id, o.customer_id, o.status, o.placed_at, o.total_amount, o.total_currency,
		       COUNT(i.id) AS item_count
		FROM checkout_order o
		LEFT JOIN checkout_order_item i ON i.order_id = o.id
		GROUP BY o.id, o.customer_id, o.status, o.placed_at, o.total_amount, o.total_currency
		ORDER BY o.placed_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var summaries []query.OrderSummary
	for rows.Next() {
		var id, status, currency string
		var customerID sql.NullString
		var placedAt time.Time
		var total int64
		var itemCount int
		if err := rows.Scan(&id, &customerID, &status, &placedAt, &total, &currency, &itemCount); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		summaries = append(summaries, query.NewOrderSummary(customerID.String, id, domain.Status(status), placedAt, itemCount, total, currency))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return summaries, nil
}
