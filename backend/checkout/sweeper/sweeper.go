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
	"database/sql"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// DefaultSweeperLockID is a unique PostgreSQL advisory lock identifier
// for the checkout reservation sweeper. ("goecom_s" in hex)
const DefaultSweeperLockID = int64(7453303666270428531)

// Locker is an optional distributed lock seam used to ensure only one
// replica executes the sweep pass at a time.
type Locker interface {
	// TryLock attempts to acquire the lock without blocking.
	// Returns acquired=true and an unlock func if successful, or acquired=false if already held.
	TryLock(ctx context.Context) (acquired bool, unlock func(), err error)
}

// PostgresLocker implements Locker using PostgreSQL session-level advisory locks.
type PostgresLocker struct {
	db     *sql.DB
	lockID int64
}

// NewPostgresLocker builds a Locker backed by pg_try_advisory_lock on the given DB.
func NewPostgresLocker(db *sql.DB, lockID int64) *PostgresLocker {
	return &PostgresLocker{db: db, lockID: lockID}
}

// TryLock attempts to obtain a session-level advisory lock using a dedicated database connection.
func (l *PostgresLocker) TryLock(ctx context.Context) (bool, func(), error) {
	conn, err := l.db.Conn(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("sweeper: acquire db connection: %w", err)
	}

	var acquired bool
	err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", l.lockID).Scan(&acquired)
	if err != nil {
		_ = conn.Close()
		return false, nil, fmt.Errorf("sweeper: query pg_try_advisory_lock: %w", err)
	}

	if !acquired {
		_ = conn.Close()
		return false, nil, nil
	}

	unlock := func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		defer func() { _ = conn.Close() }()
		_, _ = conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock($1)", l.lockID)
	}

	return true, unlock, nil
}

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
	locker   Locker
	ttl      time.Duration
	interval time.Duration
	logger   logrus.FieldLogger
	now      func() time.Time
}

// New builds a Sweeper. ttl is how old a pending order must be before it is
// expired; interval is how often the sweep runs. Both should be positive —
// non-positive values disable the sweeper at Run time so a misconfigured
// deployment can't crash or spin. An optional Locker may be provided to
// coordinate sweeps across multiple running replicas.
func New(queries query.Service, commands ExpireCommand, ttl, interval time.Duration, logger logrus.FieldLogger, locker ...Locker) *Sweeper {
	var l Locker
	if len(locker) > 0 {
		l = locker[0]
	}
	return newSweeperWithLocker(queries, commands, ttl, interval, logger, l)
}

// newSweeper builds a Sweeper from the narrow read-side seam without a locker.
func newSweeper(queries expiredPendingLister, commands ExpireCommand, ttl, interval time.Duration, logger logrus.FieldLogger) *Sweeper {
	return newSweeperWithLocker(queries, commands, ttl, interval, logger, nil)
}

// newSweeperWithLocker builds a Sweeper with an optional distributed locker.
func newSweeperWithLocker(queries expiredPendingLister, commands ExpireCommand, ttl, interval time.Duration, logger logrus.FieldLogger, locker Locker) *Sweeper {
	return &Sweeper{
		queries:  queries,
		commands: commands,
		locker:   locker,
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
	if s.locker != nil {
		acquired, unlock, err := s.locker.TryLock(ctx)
		if err != nil {
			s.logger.WithError(err).Warn("reservation sweeper: failed to acquire advisory lock")
			return
		}
		if !acquired {
			s.logger.Debug("reservation sweeper: sweep pass skipped, another replica holds the advisory lock")
			return
		}
		defer unlock()
	}

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
