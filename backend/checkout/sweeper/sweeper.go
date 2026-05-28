// Package sweeper runs a periodic background worker that releases stock
// reservations held by orders stuck in the pending state past their TTL.
//
// Checkout reserves stock up front when an order is placed and either marks
// the order paid on a successful charge or releases the stock on an explicit
// failure. If neither path completes (process crash, a hung async payment
// confirmation, an abandoned cart after stock was reserved) the order stays
// pending and the reservation is held forever. The sweeper closes that gap:
// every interval it lists pending orders whose placed_at is older than a
// configurable TTL and fails each one, which runs through the existing
// release-the-reservation path. Processing is sequential — concurrency
// across multiple replicas would need a DB lease, which is out of scope.
package sweeper

import (
	"context"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracer for the reservation sweeper; each periodic tick gets its own span so
// it's easy to graph sweep frequency and outcome in Jaeger.
var tracer = observability.Tracer("github.com/bkielbasa/go-ecommerce/backend/checkout/sweeper")

// ExpireCommand is the subset of the checkout write side the sweeper needs.
// Satisfied by app.CheckoutService.ExpirePending; the indirection keeps the
// sweeper trivially fakeable in tests.
type ExpireCommand interface {
	ExpirePending(ctx context.Context, orderID string) error
}

// expiredPendingLister is the narrow read-side seam the sweeper needs.
// query.Service satisfies it structurally; tests can pass a fake.
type expiredPendingLister interface {
	ListExpiredPending(ctx context.Context, olderThan time.Time) ([]string, error)
}

// Sweeper periodically expires pending orders past their reservation TTL.
type Sweeper struct {
	queries  expiredPendingLister
	commands ExpireCommand
	ttl      time.Duration
	interval time.Duration
	logger   logrus.FieldLogger
	now      func() time.Time
}

// New builds a Sweeper. ttl is how old a pending order must be before it is
// expired; interval is how often the sweep runs. Both should be positive —
// non-positive values disable the sweeper at Run time so a misconfigured
// deployment can't crash or spin.
func New(queries query.Service, commands ExpireCommand, ttl, interval time.Duration, logger logrus.FieldLogger) *Sweeper {
	return newSweeper(queries, commands, ttl, interval, logger)
}

// newSweeper builds a Sweeper from the narrow read-side seam. The exported
// New pins the dependency to the concrete query.Service to keep the
// composition root explicit; tests use this constructor with a fake.
func newSweeper(queries expiredPendingLister, commands ExpireCommand, ttl, interval time.Duration, logger logrus.FieldLogger) *Sweeper {
	return &Sweeper{
		queries:  queries,
		commands: commands,
		ttl:      ttl,
		interval: interval,
		logger:   logger,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// Run blocks until ctx is cancelled, sweeping every interval. The first
// sweep happens after one tick — never immediately — so that bootstrapping
// the storage layer can't be raced. If ttl or interval is non-positive the
// sweeper logs a warning and returns without scheduling anything.
func (s *Sweeper) Run(ctx context.Context) {
	if s.interval <= 0 || s.ttl <= 0 {
		s.logger.WithFields(logrus.Fields{
			"interval": s.interval.String(),
			"ttl":      s.ttl.String(),
		}).Warn("reservation sweeper disabled: non-positive interval or ttl")
		return
	}

	s.logger.WithFields(logrus.Fields{
		"interval": s.interval.String(),
		"ttl":      s.ttl.String(),
	}).Info("reservation sweeper started")

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("reservation sweeper stopped")
			return
		case <-ticker.C:
			s.sweep(ctx, s.now())
		}
	}
}

// sweep performs a single sweep pass against the given "now". Split out so
// tests can drive it deterministically without a ticker.
func (s *Sweeper) sweep(ctx context.Context, now time.Time) {
	cutoff := now.Add(-s.ttl)
	ctx, span := tracer.Start(ctx, "Sweeper.sweep", trace.WithAttributes(
		attribute.String("sweeper.cutoff", cutoff.Format(time.RFC3339)),
	))
	defer span.End()

	ids, err := s.queries.ListExpiredPending(ctx, cutoff)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		s.logger.WithError(err).WithField("cutoff", cutoff).Warn("reservation sweeper: list expired pending failed")
		return
	}
	span.SetAttributes(attribute.Int("sweeper.orders_found", len(ids)))
	if len(ids) == 0 {
		return
	}

	s.logger.WithFields(logrus.Fields{
		"count":  len(ids),
		"cutoff": cutoff,
	}).Info("reservation sweeper: expiring pending orders")

	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := s.commands.ExpirePending(ctx, id); err != nil {
			s.logger.WithError(err).WithField("order_id", id).Warn("reservation sweeper: expire pending failed")
			continue
		}
		s.logger.WithField("order_id", id).Info("reservation sweeper: pending order expired, reservation released")
	}
}
