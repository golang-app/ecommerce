package adapter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// tracer for the checkout postgres adapter; used to spotlight LoadEvents on
// the event-sourcing read path so trace consumers can see how much time the
// event log replay contributes to a Cancel/Refund/etc. command.
var adapterTracer = observability.Tracer("github.com/bkielbasa/go-ecommerce/backend/checkout/adapter")

// appendEventsTx writes the aggregate's new events within an existing
// transaction, numbering them from expectedVersion+1. The (aggregate_id,
// sequence) primary key makes a concurrent append to the same aggregate fail
// loudly (optimistic concurrency).
func appendEventsTx(ctx context.Context, tx *sql.Tx, aggregateID string, expectedVersion int, events []domain.Event) error {
	seq := expectedVersion
	for _, e := range events {
		seq++
		eventType, payload, err := marshalEvent(e)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO checkout_events (aggregate_id, sequence, event_type, payload, occurred_at)
			VALUES ($1, $2, $3, $4, $5)
		`, aggregateID, seq, eventType, payload, e.OccurredAt())
		if err != nil {
			return fmt.Errorf("append %s#%d: %w", e.EventType(), seq, err)
		}
	}
	return nil
}

// SnapshotEvery controls how often the adapter persists an aggregate
// snapshot: after every N committed events, the latest folded state is
// stored so subsequent Loads can skip replaying the early history. The
// value is a trade-off between snapshot write cost and the longest tail
// replay we ever pay for; ten keeps the dev event log honest while still
// exercising the path in tests.
const SnapshotEvery = 10

// LoadEvents returns an aggregate's events in sequence order. Useful for
// rehydrating the write-side aggregate; the read side uses the projection
// tables instead.
func (p Postgres) LoadEvents(ctx context.Context, aggregateID string) ([]domain.Event, error) {
	ctx, span := adapterTracer.Start(ctx, "checkout.adapter.LoadEvents", trace.WithAttributes(
		attribute.String("checkout.aggregate_id", aggregateID),
	))
	defer span.End()
	start := time.Now()
	defer func() {
		observability.Metrics().DBQueryDurationSec.Record(ctx, time.Since(start).Seconds(),
			metric.WithAttributes(attribute.String("query", "checkout.load_events")),
		)
	}()

	rows, err := p.db.QueryContext(ctx, `
		SELECT event_type, payload FROM checkout_events
		WHERE aggregate_id = $1 ORDER BY sequence
	`, aggregateID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("load events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []domain.Event
	for rows.Next() {
		var typ string
		var payload []byte
		if err := rows.Scan(&typ, &payload); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("scan event: %w", err)
		}
		e, err := unmarshalEvent(typ, payload)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return events, err
	}
	span.SetAttributes(attribute.Int("checkout.event_count", len(events)))
	return events, nil
}

// LoadEventsAfter returns the events with sequence strictly greater than
// afterVersion, in sequence order. It powers the snapshot-aware Load path:
// the snapshot version becomes the cursor, and only the tail is replayed.
func (p Postgres) LoadEventsAfter(ctx context.Context, aggregateID string, afterVersion int) ([]domain.Event, error) {
	ctx, span := adapterTracer.Start(ctx, "checkout.adapter.LoadEventsAfter", trace.WithAttributes(
		attribute.String("checkout.aggregate_id", aggregateID),
		attribute.Int("checkout.after_version", afterVersion),
	))
	defer span.End()
	start := time.Now()
	defer func() {
		observability.Metrics().DBQueryDurationSec.Record(ctx, time.Since(start).Seconds(),
			metric.WithAttributes(attribute.String("query", "checkout.load_events_after")),
		)
	}()

	rows, err := p.db.QueryContext(ctx, `
		SELECT event_type, payload FROM checkout_events
		WHERE aggregate_id = $1 AND sequence > $2 ORDER BY sequence
	`, aggregateID, afterVersion)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("load events after: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []domain.Event
	for rows.Next() {
		var typ string
		var payload []byte
		if err := rows.Scan(&typ, &payload); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, fmt.Errorf("scan event: %w", err)
		}
		e, err := unmarshalEvent(typ, payload)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return events, err
	}
	span.SetAttributes(attribute.Int("checkout.event_count", len(events)))
	return events, nil
}

// Load rebuilds an order aggregate. If a snapshot exists, the aggregate is
// seeded from it and only the events newer than the snapshot version are
// replayed; otherwise the full history is folded. Either path produces a
// byte-equivalent aggregate (see TestSnapshotEquivalentToFullReplay in
// checkout/domain).
func (p Postgres) Load(ctx context.Context, id string) (*domain.Order, error) {
	snapBase, sinceVer, ok, err := p.loadFromSnapshot(ctx, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		events, err := p.LoadEvents(ctx, id)
		if err != nil {
			return nil, err
		}
		if len(events) == 0 {
			return nil, domain.ErrOrderNotFound
		}
		return domain.RehydrateOrder(events), nil
	}
	tail, err := p.LoadEventsAfter(ctx, id, sinceVer)
	if err != nil {
		return nil, err
	}
	domain.ApplyTail(snapBase, tail)
	return snapBase, nil
}

// SaveSnapshot upserts an aggregate snapshot, refusing to overwrite a row
// that is already at or beyond the supplied version. The version-guard
// keeps a concurrent appender from regressing the snapshot pointer.
func (p Postgres) SaveSnapshot(ctx context.Context, aggregateID string, version int, payload []byte) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO checkout_aggregate_snapshot (aggregate_id, version, payload, created_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (aggregate_id) DO UPDATE
			SET version = EXCLUDED.version,
			    payload = EXCLUDED.payload,
			    created_at = now()
			WHERE checkout_aggregate_snapshot.version < EXCLUDED.version
	`, aggregateID, version, payload)
	if err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}
	return nil
}

// LoadSnapshot returns the stored snapshot payload for an aggregate (with
// its version), or ok=false when no row exists.
func (p Postgres) LoadSnapshot(ctx context.Context, aggregateID string) (int, []byte, bool, error) {
	var version int
	var payload []byte
	err := p.db.QueryRowContext(ctx, `
		SELECT version, payload FROM checkout_aggregate_snapshot
		WHERE aggregate_id = $1
	`, aggregateID).Scan(&version, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, false, nil
	}
	if err != nil {
		return 0, nil, false, fmt.Errorf("load snapshot: %w", err)
	}
	return version, payload, true, nil
}

// loadFromSnapshot reads the snapshot row for the aggregate and decodes it
// into a partially-rehydrated *domain.Order. ok=false means there is no
// snapshot row yet — callers should fall back to a full event replay.
func (p Postgres) loadFromSnapshot(ctx context.Context, aggregateID string) (*domain.Order, int, bool, error) {
	version, payload, ok, err := p.LoadSnapshot(ctx, aggregateID)
	if err != nil || !ok {
		return nil, 0, ok, err
	}
	snap, err := unmarshalSnapshot(payload)
	if err != nil {
		return nil, 0, false, err
	}
	// Rehydrate the aggregate at the snapshot version with no tail events;
	// the caller folds the tail events that came after.
	return domain.RehydrateOrderFromSnapshot(snap, nil), version, true, nil
}

// projectEventTx updates the read model (checkout_order / _item) for a single
// event, inside the same transaction the event was appended in — so the read
// model is always consistent with the event log.
//
// PaymentSucceeded also folds into analytics_daily_sales: a second projection
// over the same event stream, demonstrating that a CQRS read side is free to
// have multiple readers fed by independent projections.
func projectEventTx(ctx context.Context, tx *sql.Tx, e domain.Event) error {
	switch ev := e.(type) {
	case domain.OrderPlaced:
		return projectOrderPlaced(ctx, tx, ev)
	case domain.PaymentSucceeded:
		if err := setOrderStatus(ctx, tx, ev.OrderID, string(domain.StatusPaid)); err != nil {
			return err
		}
		return projectAnalyticsPaid(ctx, tx, ev.OrderID)
	case domain.PaymentFailed:
		return setOrderStatus(ctx, tx, ev.OrderID, string(domain.StatusFailed))
	case domain.OrderCancelled:
		return setOrderStatus(ctx, tx, ev.OrderID, string(domain.StatusCancelled))
	case domain.OrderShipped:
		return projectOrderShipped(ctx, tx, ev)
	case domain.OrderDelivered:
		return setOrderStatus(ctx, tx, ev.OrderID, string(domain.StatusDelivered))
	case domain.OrderRefunded:
		return setOrderStatus(ctx, tx, ev.OrderID, string(domain.StatusRefunded))
	default:
		return fmt.Errorf("no projection for event %s", e.EventType())
	}
}

// projectAnalyticsPaid bumps the analytics_daily_sales counter for the day
// (in UTC) of the order's placed_at and its currency. The order's total +
// currency + placed_at come from checkout_order, which has just been
// written by projectOrderPlaced earlier in the same transaction (or by an
// earlier event during a CLI rebuild). Idempotent under the UPSERT: every
// PaymentSucceeded event for a given order_id contributes exactly one
// (orders_count++, revenue_minor += total) increment.
func projectAnalyticsPaid(ctx context.Context, tx *sql.Tx, orderID string) error {
	var totalAmt int64
	var currency string
	var placedAt time.Time
	err := tx.QueryRowContext(ctx, `
		SELECT total_amount, total_currency, placed_at
		FROM checkout_order WHERE id = $1
	`, orderID).Scan(&totalAmt, &currency, &placedAt)
	if err != nil {
		return fmt.Errorf("analytics lookup order %s: %w", orderID, err)
	}
	day := placedAt.UTC().Truncate(24 * time.Hour)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO analytics_daily_sales (day, currency, orders_count, revenue_minor)
		VALUES ($1, $2, 1, $3)
		ON CONFLICT (day, currency) DO UPDATE SET
			orders_count = analytics_daily_sales.orders_count + 1,
			revenue_minor = analytics_daily_sales.revenue_minor + EXCLUDED.revenue_minor
	`, day, currency, totalAmt); err != nil {
		return fmt.Errorf("project analytics: %w", err)
	}
	return nil
}

// projectOrderShipped updates the order status and stores the optional
// carrier/tracking metadata on the read model row.
func projectOrderShipped(ctx context.Context, tx *sql.Tx, ev domain.OrderShipped) error {
	if _, err := tx.ExecContext(ctx, `
		UPDATE checkout_order
		SET status = $2, carrier = $3, tracking_code = $4
		WHERE id = $1
	`, ev.OrderID, string(domain.StatusShipped), ev.Carrier, ev.TrackingCode); err != nil {
		return fmt.Errorf("project shipped: %w", err)
	}
	return nil
}

func projectOrderPlaced(ctx context.Context, tx *sql.Tx, ev domain.OrderPlaced) error {
	// Fold the single event into an Order so we can reuse its derived totals
	// (the aggregate already applies the threshold-aware shipping/tax math).
	o := domain.RehydrateOrder([]domain.Event{ev})

	var customerID sql.NullString
	if o.CustomerID() != "" {
		customerID = sql.NullString{String: o.CustomerID(), Valid: true}
	}
	ship := o.ShipTo()
	method := o.ShippingMethod()
	pay := o.PaymentMethod()

	_, err := tx.ExecContext(ctx, `
		INSERT INTO checkout_order
			(id, user_id, customer_id, total_amount, total_currency, status, placed_at,
			 ship_name, ship_street1, ship_street2, ship_city, ship_zip, ship_country,
			 ship_method_code, ship_method_label, ship_cost,
			 payment_method_code, payment_method_label, tax_amount,
			 discount_code, discount_amount)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
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
			ship_cost = EXCLUDED.ship_cost,
			payment_method_code = EXCLUDED.payment_method_code,
			payment_method_label = EXCLUDED.payment_method_label,
			tax_amount = EXCLUDED.tax_amount,
			discount_code = EXCLUDED.discount_code,
			discount_amount = EXCLUDED.discount_amount
	`,
		o.ID(), o.UserID(), customerID, o.TotalAmount(), o.TotalCurrency(),
		string(o.Status()), o.PlacedAt(),
		ship.Name(), ship.Street1(), ship.Street2(), ship.City(), ship.Zip(), ship.Country(),
		method.Code(), method.Label(), o.ShippingCost(),
		pay.Code(), pay.Label(), o.TaxAmount(),
		o.DiscountCode(), o.DiscountAmount(),
	)
	if err != nil {
		return fmt.Errorf("project order: %w", err)
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM checkout_order_item WHERE order_id = $1`, o.ID()); err != nil {
		return fmt.Errorf("clear items: %w", err)
	}
	for i, ln := range o.Items() {
		itemID := fmt.Sprintf("%s-%d", o.ID(), i)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO checkout_order_item
				(id, order_id, product_id, product_name, qty, price_amount, price_currency)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, itemID, o.ID(), ln.ProductID(), ln.ProductName(), ln.Quantity(), ln.PriceAmount(), ln.PriceCurrency())
		if err != nil {
			return fmt.Errorf("project item: %w", err)
		}
	}
	return nil
}

func setOrderStatus(ctx context.Context, tx *sql.Tx, orderID, status string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE checkout_order SET status = $2 WHERE id = $1`, orderID, status); err != nil {
		return fmt.Errorf("project status: %w", err)
	}
	return nil
}
