// Package inbox implements the Inbox pattern — the consumer-side
// complement to the Transactional Outbox (see internal/outbox).
//
// PROBLEM. The Outbox upgrades publisher-to-bus delivery from
// at-most-once to at-least-once: a crash between Publish and MarkSent
// in the outbox dispatcher will republish the same row on the next
// tick. That puts the burden of idempotency on every subscriber —
// each one has to find its own per-event dedupe story (a natural-id
// no-op like cart Clear, a sent_emails ledger, an in-memory map that
// doesn't survive restarts, …). The shapes drift, the guarantees
// rot, and the at-least-once contract leaks into business code.
//
// SOLUTION. A single persistent table — inbox_handled — records, per
// subscriber, which originating event ids have already been
// processed. The dispatcher passes the outbox row id through to each
// id-aware subscriber (eventbus.HandlerWithID); a thin wrapper
// (inbox.Wrap) calls MarkHandled before invoking the underlying
// handler. A redelivery short-circuits at the wrapper and the
// business handler never runs twice.
//
// CONTRACT. Combined with a content-stable event id (the outbox row
// id satisfies that — same row → same id across redeliveries) the
// Inbox gives you effectively exactly-once at the subscriber
// boundary. The classic caveat applies: this is exactly-once
// PROCESSING (the inbox row is written in its own transaction, the
// side effect runs separately), not the impossible exactly-once
// DELIVERY. Side effects that themselves emit further events should
// stage those into their own Outbox so the chain stays durable.
package inbox

import (
	"context"
	"database/sql"
	"fmt"
)

// Postgres is the durable inbox store backed by the inbox_handled
// table. The table is content-agnostic: the package only deals with a
// subscriber name (a stable string the composition root assigns) and
// an event id (the outbox row id supplied by the dispatcher).
type Postgres struct {
	db *sql.DB
}

// NewPostgres builds a Postgres inbox store from an *sql.DB. The
// returned value is intentionally a value type so the composition
// root passes it around by copy (it is just a thin wrapper over the
// pool handle; no per-store state to share).
func NewPostgres(db *sql.DB) Postgres {
	return Postgres{db: db}
}

// MarkHandled records that (subscriber, eventID) has been processed.
//
// Return contract:
//   - alreadyHandled == false, err == nil  -> the row was newly
//     inserted; the caller MUST invoke the underlying handler.
//   - alreadyHandled == true,  err == nil  -> a row for the same
//     (subscriber, eventID) already existed (ON CONFLICT DO NOTHING
//     suppressed the insert); the caller MUST skip the handler.
//   - err != nil                            -> storage failure; the
//     caller MUST NOT invoke the handler. The Outbox dispatcher will
//     retry the same row on the next tick and MarkHandled will get
//     another shot.
//
// Detection uses RowsAffected() rather than a CTE/xmax trick. The two
// give the same answer for this query shape but RowsAffected is the
// portable, ordinary-looking option a future maintainer recognises
// immediately.
func (p Postgres) MarkHandled(ctx context.Context, subscriber string, eventID int64) (alreadyHandled bool, err error) {
	res, err := p.db.ExecContext(ctx, `
		INSERT INTO inbox_handled (subscriber, event_id)
		VALUES ($1, $2)
		ON CONFLICT (subscriber, event_id) DO NOTHING
	`, subscriber, eventID)
	if err != nil {
		return false, fmt.Errorf("inbox: mark handled (%s, %d): %w", subscriber, eventID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("inbox: rows affected (%s, %d): %w", subscriber, eventID, err)
	}
	// n == 0 means ON CONFLICT DO NOTHING fired — the row already
	// existed, i.e. this is a duplicate delivery. n == 1 means the
	// insert took effect and the caller now owns the side effect.
	return n == 0, nil
}
