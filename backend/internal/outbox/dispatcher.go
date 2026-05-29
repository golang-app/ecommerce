package outbox

import (
	"context"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
	"github.com/sirupsen/logrus"
)

// Store is the narrow read/update seam the dispatcher needs. It is
// satisfied by Postgres but kept as an interface so the dispatcher is
// trivially fakeable in unit tests.
type Store interface {
	Unsent(ctx context.Context, limit int) ([]Row, error)
	MarkSent(ctx context.Context, id int64) error
}

// Publisher is the subset of *eventbus.Bus the dispatcher uses. The
// concrete bus satisfies it; tests pass a fake to assert what was
// published.
//
// PublishWithID threads the outbox row's id through to id-aware
// subscribers (eventbus.HandlerWithID) — that id is what
// internal/inbox uses as a content-stable dedupe key, completing the
// at-least-once -> effectively exactly-once story at the subscriber
// boundary.
type Publisher interface {
	PublishWithID(ctx context.Context, eventID int64, e eventbus.Event)
}

// Decoder turns a stored (kind, payload) row back into the integration
// event the in-process subscribers expect. The dispatcher is
// content-agnostic — the composition root supplies a closure that
// switches on kind and unmarshals into the matching Go type.
type Decoder func(kind string, payload []byte) (eventbus.Event, error)

// Dispatcher polls the outbox table and publishes unsent rows onto
// the in-process bus. It exits cleanly on ctx.Done().
//
// The polling loop is intentionally simple: one batch per tick,
// sequential publish, no parallelism. The pattern's value is the
// SHAPE (durable hand-off, at-least-once), not throughput; a real
// deployment with high event volume would add a worker pool and/or
// SKIP LOCKED row leasing, both of which are out of scope here.
type Dispatcher struct {
	store    Store
	bus      Publisher
	decode   Decoder
	logger   logrus.FieldLogger
	interval time.Duration
	limit    int
}

// dispatchBatchLimit is the per-tick row cap. Small on purpose — the
// dispatcher's job is steady drain, not bulk replay.
const dispatchBatchLimit = 100

// NewDispatcher builds a Dispatcher. interval is how often the loop
// polls for unsent rows; a non-positive value disables the dispatcher
// at Run time so a misconfigured deployment can't spin.
func NewDispatcher(store Store, bus Publisher, decode Decoder, logger logrus.FieldLogger, interval time.Duration) *Dispatcher {
	return &Dispatcher{
		store:    store,
		bus:      bus,
		decode:   decode,
		logger:   logger,
		interval: interval,
		limit:    dispatchBatchLimit,
	}
}

// Run blocks until ctx is cancelled, dispatching every interval. The
// first dispatch happens after one tick — never immediately — so the
// process has a beat to settle (DB connection, subscribers
// registered) before the loop starts. If interval is non-positive the
// dispatcher logs a warning and returns without scheduling anything.
func (d *Dispatcher) Run(ctx context.Context) {
	if d.interval <= 0 {
		if d.logger != nil {
			d.logger.WithField("interval", d.interval.String()).Warn("outbox dispatcher disabled: non-positive interval")
		}
		return
	}

	if d.logger != nil {
		d.logger.WithField("interval", d.interval.String()).Info("outbox dispatcher started")
	}

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if d.logger != nil {
				d.logger.Info("outbox dispatcher stopped")
			}
			return
		case <-ticker.C:
			d.dispatchOnce(ctx)
		}
	}
}

// dispatchOnce drains up to one batch of unsent rows. Split out from
// Run so tests can drive a single tick deterministically without a
// real ticker.
//
// Per-row failure modes:
//   - decode error: row is logged and skipped (left unsent so a code
//     fix can rerun it; nothing in the queue would unblock by retrying
//     the same broken bytes).
//   - publish: the bus's Publish only logs handler errors, it never
//     returns one, so this step always succeeds from our point of
//     view. A subscriber failure means the row is still considered
//     dispatched here; idempotency in subscribers is what makes that
//     safe across retries.
//   - mark-sent error: row stays unsent and the next tick will see it
//     again. Subscribers MUST tolerate the duplicate publish that
//     produces.
func (d *Dispatcher) dispatchOnce(ctx context.Context) {
	rows, err := d.store.Unsent(ctx, d.limit)
	if err != nil {
		if d.logger != nil {
			d.logger.WithError(err).Warn("outbox dispatcher: list unsent failed")
		}
		return
	}
	for _, r := range rows {
		if err := ctx.Err(); err != nil {
			return
		}
		event, err := d.decode(r.Kind, r.Payload)
		if err != nil {
			if d.logger != nil {
				d.logger.WithError(err).WithFields(logrus.Fields{
					"outbox.id":   r.ID,
					"outbox.kind": r.Kind,
				}).Warn("outbox dispatcher: decode failed")
			}
			continue
		}
		d.bus.PublishWithID(ctx, r.ID, event)
		if err := d.store.MarkSent(ctx, r.ID); err != nil {
			if d.logger != nil {
				d.logger.WithError(err).WithFields(logrus.Fields{
					"outbox.id":   r.ID,
					"outbox.kind": r.Kind,
				}).Warn("outbox dispatcher: mark sent failed; will retry next tick")
			}
			// Leave the row unsent. The next tick re-publishes it —
			// subscribers must be idempotent. This is the
			// at-least-once contract in action.
			continue
		}
	}
}
