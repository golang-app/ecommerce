package adapter

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/app"
	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/domain"
	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/integration"
	"github.com/lib/pq"
)

// pgUniqueViolation is the SQLSTATE code postgres returns when a write
// hits a unique-constraint conflict. We map it to
// domain.ErrAlreadyExists so callers can branch on the sentinel without
// reaching for pq.Error themselves.
const pgUniqueViolation = "23505"

// OutboxAppender stages integration events into the outbox table inside
// an active database transaction.
type OutboxAppender interface {
	AppendTx(ctx context.Context, tx *sql.Tx, kind string, payload []byte) error
}

// Postgres is the production Storage adapter. Parameterised SQL
// throughout; the only interpolation into the statement strings is
// the placeholder index, never a value.
type Postgres struct {
	db          *sql.DB
	outbox      OutboxAppender
	orderDetail app.OrderDetailReader
}

// NewPostgres builds the production adapter bound to the supplied DB
// connection and optional outbox appender.
func NewPostgres(db *sql.DB, outbox OutboxAppender) *Postgres {
	return &Postgres{db: db, outbox: outbox}
}

// WithOrderDetailReader wires the reader used to populate ECST payloads on ship.
func (p *Postgres) WithOrderDetailReader(r app.OrderDetailReader) *Postgres {
	p.orderDetail = r
	return p
}

// Create inserts a fresh fulfillment row. A unique-constraint
// conflict on order_id is surfaced as domain.ErrAlreadyExists —
// callers (the application service's OnOrderPaid) treat it as the
// idempotency guard against duplicate OrderPaid deliveries.
func (p *Postgres) Create(ctx context.Context, f domain.Fulfillment) error {
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO fulfillment (
			id, order_id, status,
			carrier, tracking_code,
			scheduled_at, shipped_at, delivered_at,
			refund_reason, version
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`,
		f.ID(), f.OrderID(), string(f.Status()),
		f.Carrier(), f.TrackingCode(),
		nullableTime(f.ScheduledAt()),
		nullableTime(f.ShippedAt()),
		nullableTime(f.DeliveredAt()),
		f.RefundReason(), f.Version(),
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && string(pqErr.Code) == pgUniqueViolation {
			return domain.ErrAlreadyExists
		}
		return fmt.Errorf("insert fulfillment: %w", err)
	}
	return nil
}

// Update writes a state transition under optimistic concurrency: the
// UPDATE matches both the id and the previous version (current
// Version() - 1). A 0-rows-affected result is mapped to
// app.ErrOptimisticLock so the caller can retry against a fresh load.
// When an outbox is wired, integration events are extracted from the
// aggregate's pending events and staged into the outbox inside the same
// transaction.
func (p *Postgres) Update(ctx context.Context, f domain.Fulfillment) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	res, err := tx.ExecContext(ctx, `
		UPDATE fulfillment
		SET status = $2,
		    carrier = $3,
		    tracking_code = $4,
		    scheduled_at = $5,
		    shipped_at = $6,
		    delivered_at = $7,
		    refund_reason = $8,
		    version = $9
		WHERE id = $1 AND version = $10
	`,
		f.ID(),
		string(f.Status()),
		f.Carrier(),
		f.TrackingCode(),
		nullableTime(f.ScheduledAt()),
		nullableTime(f.ShippedAt()),
		nullableTime(f.DeliveredAt()),
		f.RefundReason(),
		f.Version(),
		f.Version()-1,
	)
	if err != nil {
		return fmt.Errorf("update fulfillment: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return app.ErrOptimisticLock
	}

	if p.outbox != nil {
		records, err := extractIntegrationEvents(ctx, f, p.orderDetail)
		if err != nil {
			return err
		}
		for _, rec := range records {
			if err = p.outbox.AppendTx(ctx, tx, rec.Kind, rec.Payload); err != nil {
				return fmt.Errorf("append outbox: %w", err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// Find loads the row by id, mapping sql.ErrNoRows to app.ErrNotFound.
func (p *Postgres) Find(ctx context.Context, id string) (domain.Fulfillment, error) {
	return p.scanOne(ctx, `
		SELECT id, order_id, status, carrier, tracking_code,
		       scheduled_at, shipped_at, delivered_at,
		       refund_reason, version
		FROM fulfillment
		WHERE id = $1
	`, id)
}

// FindByOrder loads the row keyed by order id; sql.ErrNoRows maps to
// app.ErrNotFound. The unique constraint on order_id guarantees at
// most one row.
func (p *Postgres) FindByOrder(ctx context.Context, orderID string) (domain.Fulfillment, error) {
	return p.scanOne(ctx, `
		SELECT id, order_id, status, carrier, tracking_code,
		       scheduled_at, shipped_at, delivered_at,
		       refund_reason, version
		FROM fulfillment
		WHERE order_id = $1
	`, orderID)
}

// ListAll returns every row, scheduled-newest first. Used by admin
// surfaces / future reporting; the index on (status) supports
// status-filtered variants if/when needed.
func (p *Postgres) ListAll(ctx context.Context) ([]domain.Fulfillment, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, order_id, status, carrier, tracking_code,
		       scheduled_at, shipped_at, delivered_at,
		       refund_reason, version
		FROM fulfillment
		ORDER BY scheduled_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list fulfillments: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Fulfillment
	for rows.Next() {
		f, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// scanOne is the shared shape behind Find / FindByOrder.
func (p *Postgres) scanOne(ctx context.Context, q string, arg string) (domain.Fulfillment, error) {
	row := p.db.QueryRowContext(ctx, q, arg)
	f, err := scanFromRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Fulfillment{}, app.ErrNotFound
	}
	if err != nil {
		return domain.Fulfillment{}, fmt.Errorf("scan fulfillment: %w", err)
	}
	return f, nil
}

// rowScanner is the narrow interface implemented by both *sql.Row and
// *sql.Rows so scanFromRow / scanRow share the same code path.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanFromRow(s rowScanner) (domain.Fulfillment, error) {
	return scanRow(s)
}

func scanRow(s rowScanner) (domain.Fulfillment, error) {
	var (
		id, orderID, status        string
		carrier, trackingCode      string
		scheduledAt                sql.NullTime
		shippedAt, deliveredAt     sql.NullTime
		refundReason               string
		version                    int
	)
	if err := s.Scan(
		&id, &orderID, &status,
		&carrier, &trackingCode,
		&scheduledAt, &shippedAt, &deliveredAt,
		&refundReason, &version,
	); err != nil {
		return domain.Fulfillment{}, err
	}
	return domain.Rebuild(
		id, orderID,
		domain.Status(status),
		carrier, trackingCode,
		nullTimeOrZero(scheduledAt),
		nullTimeOrZero(shippedAt),
		nullTimeOrZero(deliveredAt),
		refundReason,
		version,
	), nil
}

// nullableTime converts a zero time.Time into a NULL on the wire so
// the schema's nullable timestamptz columns carry the operational
// "not yet" meaning rather than a misleading epoch value.
func nullableTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

// nullTimeOrZero unpacks a sql.NullTime back into a regular time.Time
// (zero when null).
func nullTimeOrZero(n sql.NullTime) time.Time {
	if !n.Valid {
		return time.Time{}
	}
	return n.Time
}

type outboxRecord struct {
	Kind    string
	Payload []byte
}

// extractIntegrationEvents maps the aggregate's pending domain events
// into the integration events fulfillment publishes outward through the outbox.
func extractIntegrationEvents(ctx context.Context, f domain.Fulfillment, detailReader app.OrderDetailReader) ([]outboxRecord, error) {
	var out []outboxRecord
	for _, e := range f.PendingEvents() {
		switch ev := e.(type) {
		case domain.FulfillmentDelivered:
			payload, err := json.Marshal(integration.OrderDelivered{
				OrderID: f.OrderID(),
				At:      ev.At,
			})
			if err != nil {
				return nil, fmt.Errorf("encode OrderDelivered: %w", err)
			}
			out = append(out, outboxRecord{
				Kind:    integration.OrderDelivered{}.EventName(),
				Payload: payload,
			})
		case domain.FulfillmentRefunded:
			payload, err := json.Marshal(integration.OrderRefunded{
				OrderID: f.OrderID(),
				Reason:  ev.Reason,
				At:      ev.At,
			})
			if err != nil {
				return nil, fmt.Errorf("encode OrderRefunded: %w", err)
			}
			out = append(out, outboxRecord{
				Kind:    integration.OrderRefunded{}.EventName(),
				Payload: payload,
			})
		case domain.FulfillmentShipped:
			notifPayload, err := json.Marshal(integration.OrderShipped{
				OrderID:      f.OrderID(),
				Carrier:      f.Carrier(),
				TrackingCode: f.TrackingCode(),
				At:           ev.At,
			})
			if err != nil {
				return nil, fmt.Errorf("encode OrderShipped: %w", err)
			}
			out = append(out, outboxRecord{
				Kind:    integration.OrderShipped{}.EventName(),
				Payload: notifPayload,
			})

			if detailReader != nil {
				detail, err := detailReader.OrderDetail(ctx, f.OrderID())
				if err == nil {
					items := make([]integration.LineDTO, 0, len(detail.Items))
					for _, ln := range detail.Items {
						items = append(items, integration.LineDTO{
							ProductID:     ln.ProductID,
							ProductName:   ln.ProductName,
							Quantity:      ln.Quantity,
							PriceAmount:   ln.PriceAmount,
							PriceCurrency: ln.PriceCurrency,
						})
					}
					ecstPayload, err := json.Marshal(integration.OrderShippedECST{
						OrderID:      f.OrderID(),
						CustomerID:   detail.CustomerID,
						Email:        detail.Email,
						Carrier:      f.Carrier(),
						TrackingCode: f.TrackingCode(),
						ShipTo: integration.ShippingAddressDTO{
							Name:    detail.ShipTo.Name,
							Street1: detail.ShipTo.Street1,
							Street2: detail.ShipTo.Street2,
							City:    detail.ShipTo.City,
							Zip:     detail.ShipTo.Zip,
							Country: detail.ShipTo.Country,
						},
						Items:        items,
						Subtotal:     detail.Subtotal,
						Tax:          detail.Tax,
						ShippingCost: detail.ShippingCost,
						Total:        detail.Total,
						Currency:     detail.Currency,
						At:           ev.At,
					})
					if err != nil {
						return nil, fmt.Errorf("encode OrderShippedECST: %w", err)
					}
					out = append(out, outboxRecord{
						Kind:    integration.OrderShippedECST{}.EventName(),
						Payload: ecstPayload,
					})
				}
			}
		}
	}
	return out, nil
}
