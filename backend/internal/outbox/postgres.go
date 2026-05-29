// Package outbox — Postgres storage adapter.
//
// WHY. The in-process eventbus.Bus is at-most-once: Publish dispatches
// to subscribers synchronously and forgets the event the moment it
// returns. If the publisher's own transaction has committed but the
// process crashes before Publish (or before a subscriber finishes),
// the integration event is lost forever. That's the at-most-once gap
// the Transactional Outbox pattern closes.
//
// HOW. The producer's transaction is extended with one extra INSERT
// into outbox_event (same DB, same tx — guaranteed by Postgres to
// commit or roll back together with the business write). A separate
// background dispatcher polls unsent rows, publishes each one to the
// in-process bus, and marks the row sent. A crash between commit and
// dispatch is harmless: the next poll picks the row up again.
//
// CONTRACT. The pattern upgrades delivery from at-most-once to
// at-least-once. Subscribers MUST therefore be idempotent — they may
// see the same event twice (e.g. on a process restart between
// publish-to-bus and mark-sent). Idempotency strategies range from
// natural-id no-ops (clearing a cart twice clears nothing the second
// time) to dedupe tables keyed by the integration event's order id.
package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Row is a single outbox record loaded for dispatch. ID is the primary
// key the dispatcher passes back to MarkSent.
type Row struct {
	ID        int64
	Kind      string
	Payload   []byte
	CreatedAt time.Time
}

// Postgres is the durable outbox store backed by the outbox_event
// table. It deliberately does NOT open its own transactions for
// AppendTx — the producer's tx is supplied by the caller so the
// business write and the outbox INSERT share one atomic commit.
type Postgres struct {
	db *sql.DB
}

// NewPostgres builds a Postgres outbox store from an *sql.DB. The DB
// is only used by Unsent/MarkSent; AppendTx works against a
// caller-supplied *sql.Tx.
func NewPostgres(db *sql.DB) Postgres {
	return Postgres{db: db}
}

// AppendTx stages an integration event inside the caller's
// transaction. kind is the event name (used later by the dispatcher
// to pick the right decoder); payload is the JSON body. Returning an
// error from this call MUST cause the caller to roll their tx back —
// otherwise the business write would commit without the outbox row
// and the at-most-once gap would reappear.
func (p Postgres) AppendTx(ctx context.Context, tx *sql.Tx, kind string, payload []byte) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO outbox_event (kind, payload)
		VALUES ($1, $2)
	`, kind, payload)
	if err != nil {
		return fmt.Errorf("outbox: append %s: %w", kind, err)
	}
	return nil
}

// Unsent returns up to limit unsent rows, oldest first. The partial
// index outbox_event_unsent_idx keeps this query cheap regardless of
// how many already-sent rows have accumulated.
func (p Postgres) Unsent(ctx context.Context, limit int) ([]Row, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, kind, payload, created_at
		FROM outbox_event
		WHERE sent_at IS NULL
		ORDER BY created_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("outbox: query unsent: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Kind, &r.Payload, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("outbox: scan unsent: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbox: rows unsent: %w", err)
	}
	return out, nil
}

// MarkSent records that the row with the given id has been published.
// A row marked sent will never be picked up by Unsent again.
func (p Postgres) MarkSent(ctx context.Context, id int64) error {
	_, err := p.db.ExecContext(ctx, `
		UPDATE outbox_event SET sent_at = now() WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("outbox: mark sent %d: %w", id, err)
	}
	return nil
}
