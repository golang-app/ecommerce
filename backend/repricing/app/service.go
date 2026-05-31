// Package app holds the repricing application service: the
// orchestrating layer between the bulk-reprice saga, storage and the
// product catalogue's price-update surface.
//
// SAGA SHAPE
//
//	Start  -> persist row in StatusScheduled, snapshot the planned
//	          product ids, then kick off a background goroutine that
//	          calls RunActive on a fresh context (Start MUST NOT block
//	          the admin's HTTP round-trip).
//
//	RunActive -> if the row is StatusScheduled, transition it to
//	             StatusInProgress; iterate the planned product ids,
//	             for each load the current price, multiply by
//	             (1 + percent/100), call PriceUpdater.UpdateProductPrice,
//	             and RecordItemProcessed (or RecordItemFailed) per item.
//	             On full pass, Complete. On ctx.Cancel mid-flight,
//	             leave status=in_progress so a later call resumes from
//	             processedItems.
//
// Resumability is achieved by snapshotting the planned product ids
// at Start time into an in-memory plan that RunActive can re-read
// (the snapshot lives on the *Service so a single process restart
// loses it; a production-grade saga would persist the plan as a
// `repricing_item` table). The trade-off is documented inline at
// the snapshot field. Re-runs after a restart will re-load the
// planned ids from the CategoryReader, which is acceptable when the
// catalogue is stable between restarts (the common case for this
// demo).
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/repricing/domain"
)

// ErrNotFound is returned by Storage.Find / FindActive when no row
// matches.
var ErrNotFound = errors.New("repricing: not found")

// ErrOptimisticLock is returned by Storage.Update when the row's
// version does not match the expected version.
var ErrOptimisticLock = errors.New("repricing: optimistic lock conflict")

// ErrAlreadyActive is returned by Start when an existing reprice is
// still scheduled / in_progress.
var ErrAlreadyActive = errors.New("repricing: a reprice is already active")

// Storage is the persistence port for reprice records. The adapter
// package supplies an in-memory implementation (tests) and a
// Postgres-backed one (production).
type Storage interface {
	// Create inserts a fresh reprice row. A partial unique index
	// on the active statuses guarantees at-most-one active row in
	// the table; a violation surfaces as ErrAlreadyActive.
	Create(ctx context.Context, r domain.Reprice) error
	// Update writes a state transition under optimistic
	// concurrency; a mismatch returns ErrOptimisticLock.
	Update(ctx context.Context, r domain.Reprice) error
	// Find returns the row by id, ErrNotFound if missing.
	Find(ctx context.Context, id string) (domain.Reprice, error)
	// FindActive returns the scheduled-or-in-progress row if one
	// exists. ok=false (and a zero Reprice) means no active row.
	FindActive(ctx context.Context) (domain.Reprice, bool, error)
	// ListAll returns every row, newest-started first. Used by
	// the admin list view.
	ListAll(ctx context.Context) ([]domain.Reprice, error)
}

// CategoryReader is the seam onto which Start snapshots the planned
// product ids for the category. Implemented in the composition root
// by a thin adapter over productcatalog/app.ProductService.List.
type CategoryReader interface {
	// ProductsInCategory returns every product id in the given
	// category slug. An empty slug / no products returns a nil
	// slice (Start rejects an empty plan).
	ProductsInCategory(ctx context.Context, categorySlug string) ([]string, error)
}

// PriceUpdater is the seam onto which RunActive applies the
// per-item price change. The saga supplies the precomputed new
// minor-unit amount; the composition-root adapter wraps
// productcatalog/app.ProductService.UpdateProduct (which rewrites
// the whole product row, but the adapter copies every other field
// verbatim — only the price changes).
type PriceUpdater interface {
	// CurrentPriceMinor returns the product's current base price
	// in minor units. The saga reads it just before writing back
	// so the percentage is applied to the live value.
	CurrentPriceMinor(ctx context.Context, productID string) (int64, error)
	// UpdateProductPrice replaces a product's base price with
	// the given minor-unit amount.
	UpdateProductPrice(ctx context.Context, productID string, newAmountMinor int64) error
}

// Service is the application facade.
type Service struct {
	storage  Storage
	reader   CategoryReader
	updater  PriceUpdater
	logger   Logger
	now      func() time.Time
	newID    func() string
	runAsync func(func())

	// plans snapshots the planned product ids for an active saga
	// so RunActive does not have to re-read the catalogue on
	// every resume. The snapshot is process-local: a restart
	// loses the in-memory map, and the next RunActive call will
	// re-snapshot from CategoryReader. Documenting the trade-off
	// here rather than introducing a `repricing_item` table keeps
	// the demo's footprint small while making the upgrade path
	// explicit.
	mu    sync.Mutex
	plans map[string][]string
}

// Logger is the minimal log seam the service uses for non-fatal
// warnings.
type Logger interface {
	Warnf(format string, args ...any)
	Infof(format string, args ...any)
}

// nopLogger is the default when no logger is supplied.
type nopLogger struct{}

func (nopLogger) Warnf(string, ...any) {}
func (nopLogger) Infof(string, ...any) {}

// NewService wires the service against its dependencies.
func NewService(storage Storage, reader CategoryReader, updater PriceUpdater) *Service {
	return &Service{
		storage:  storage,
		reader:   reader,
		updater:  updater,
		logger:   nopLogger{},
		now:      func() time.Time { return time.Now().UTC() },
		newID:    newRandomID,
		runAsync: func(fn func()) { go fn() },
		plans:    map[string][]string{},
	}
}

// WithClock overrides the time source — used by tests to pin
// transition timestamps.
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// WithIDGenerator overrides the id generator so tests can use
// predictable ids.
func (s *Service) WithIDGenerator(newID func() string) *Service {
	s.newID = newID
	return s
}

// WithLogger wires the log seam.
func (s *Service) WithLogger(l Logger) *Service {
	if l == nil {
		l = nopLogger{}
	}
	s.logger = l
	return s
}

// WithAsyncRunner overrides the background-goroutine launcher.
// Tests substitute a synchronous runner that invokes the function
// inline so RunActive can be driven deterministically from the test
// goroutine; production keeps the default `go fn()`.
func (s *Service) WithAsyncRunner(runner func(func())) *Service {
	if runner == nil {
		runner = func(fn func()) { go fn() }
	}
	s.runAsync = runner
	return s
}

// Start kicks off a new bulk-reprice saga. It refuses when an
// active reprice already exists (the partial unique index on the
// storage table is the belt-and-braces second check), snapshots
// the planned product ids, persists a new row in StatusScheduled,
// and then — the demo trade-off — spawns a background goroutine
// that drives RunActive against a freshly-rooted context.
//
// Trade-off. In production the background driver would be a job
// queue (with retries and dead-letter routing) and the admin would
// not need to keep the page open for the saga to finish. The
// goroutine-per-Start approach below makes the demo single-process
// and self-contained at the cost of losing the in-memory plan
// snapshot when the binary restarts mid-run (RunActive on the
// active row picks it back up by re-loading the plan from
// CategoryReader, which is fine when the catalogue is stable
// between restarts).
func (s *Service) Start(ctx context.Context, categorySlug string, percentChange float64) (string, error) {
	if _, exists, err := s.storage.FindActive(ctx); err != nil {
		return "", fmt.Errorf("repricing: find active: %w", err)
	} else if exists {
		return "", ErrAlreadyActive
	}

	productIDs, err := s.reader.ProductsInCategory(ctx, categorySlug)
	if err != nil {
		return "", fmt.Errorf("repricing: load category products: %w", err)
	}
	if len(productIDs) == 0 {
		return "", fmt.Errorf("repricing: category %q has no products: %w", categorySlug, domain.ErrInvalidReprice)
	}

	id := s.newID()
	r, err := domain.NewReprice(id, categorySlug, percentChange, len(productIDs), s.now())
	if err != nil {
		return "", err
	}
	if err := s.storage.Create(ctx, r); err != nil {
		return "", fmt.Errorf("repricing: create: %w", err)
	}

	s.snapshotPlan(id, productIDs)

	// Detach the background driver from the HTTP request's
	// context so cancelling the form submission does not kill
	// the saga. The saga inherits the rest of the process
	// lifetime instead.
	s.runAsync(func() {
		bgCtx := context.Background()
		if err := s.RunActive(bgCtx); err != nil {
			s.logger.Warnf("repricing: background run failed: %v", err)
		}
	})
	return id, nil
}

// RunActive loads the active reprice and walks its planned items.
// Safe to invoke from multiple goroutines: the optimistic
// concurrency check on Update means only one walker advances the
// row at a time, and any conflict reloads from the persisted state
// before continuing.
//
// When the row is already StatusCompleted / StatusFailed RunActive
// returns nil (no work to do). When no active row exists, returns
// nil — the admin polling page calls RunActive without checking
// first and a no-op is the natural answer.
func (s *Service) RunActive(ctx context.Context) error {
	r, exists, err := s.storage.FindActive(ctx)
	if err != nil {
		return fmt.Errorf("repricing: find active: %w", err)
	}
	if !exists {
		return nil
	}

	if r.Status() == domain.StatusScheduled {
		if err := r.Start(); err != nil {
			return fmt.Errorf("repricing: start: %w", err)
		}
		if err := s.storage.Update(ctx, r); err != nil {
			return fmt.Errorf("repricing: persist start: %w", err)
		}
	}

	productIDs, err := s.plan(ctx, r)
	if err != nil {
		return fmt.Errorf("repricing: load plan: %w", err)
	}

	// Resume from where we left off: skip the first
	// ProcessedItems items so a restart after a crash does not
	// double-apply the percentage change.
	for idx := r.ProcessedItems(); idx < len(productIDs); idx++ {
		if err := ctx.Err(); err != nil {
			// Cooperative cancellation: leave the row in
			// StatusInProgress so a later RunActive resumes.
			return nil
		}
		productID := productIDs[idx]
		if err := s.applyItem(ctx, &r, productID); err != nil {
			if errors.Is(err, ErrOptimisticLock) {
				reloaded, ferr := s.storage.Find(ctx, r.ID())
				if ferr != nil {
					return fmt.Errorf("repricing: reload after conflict: %w", ferr)
				}
				r = reloaded
				idx = r.ProcessedItems() - 1
				continue
			}
			return err
		}
	}

	if r.ProcessedItems() >= r.TotalItems() && r.Status() == domain.StatusInProgress {
		if err := r.Complete(s.now()); err != nil {
			return fmt.Errorf("repricing: complete: %w", err)
		}
		if err := s.storage.Update(ctx, r); err != nil {
			return fmt.Errorf("repricing: persist complete: %w", err)
		}
		s.forgetPlan(r.ID())
	}
	return nil
}

// applyItem loads the product's current price, computes the new
// minor-unit amount, applies the update and records the outcome on
// the Reprice row. A per-item failure is recorded as a non-fatal
// RecordItemFailed; storage failures while updating the row itself
// bubble up.
func (s *Service) applyItem(ctx context.Context, r *domain.Reprice, productID string) error {
	current, err := s.updater.CurrentPriceMinor(ctx, productID)
	if err != nil {
		s.logger.Warnf("repricing: load price %s: %v", productID, err)
		if recordErr := r.RecordItemFailed(err.Error(), s.now()); recordErr != nil {
			return fmt.Errorf("repricing: record failed: %w", recordErr)
		}
		return s.storage.Update(ctx, *r)
	}
	newAmount := applyPercent(current, r.PercentChange())
	if err := s.updater.UpdateProductPrice(ctx, productID, newAmount); err != nil {
		s.logger.Warnf("repricing: update %s: %v", productID, err)
		if recordErr := r.RecordItemFailed(err.Error(), s.now()); recordErr != nil {
			return fmt.Errorf("repricing: record failed: %w", recordErr)
		}
	} else if recordErr := r.RecordItemProcessed(s.now()); recordErr != nil {
		return fmt.Errorf("repricing: record processed: %w", recordErr)
	}
	return s.storage.Update(ctx, *r)
}

// applyPercent returns oldAmount * (1 + percent/100), rounded to
// the nearest minor unit. The clamp at zero protects against
// absurd inputs (percent <= -100) — even though NewReprice already
// bounds percent to +/-50, defending here keeps the rounding step
// self-contained.
func applyPercent(oldAmount int64, percent float64) int64 {
	multiplier := 1 + percent/100.0
	if multiplier < 0 {
		multiplier = 0
	}
	scaled := float64(oldAmount) * multiplier
	rounded := math.Round(scaled)
	if rounded < 0 {
		rounded = 0
	}
	return int64(rounded)
}

// snapshotPlan stores the planned product ids on the in-memory map
// so RunActive does not have to re-read the catalogue on the same
// run.
func (s *Service) snapshotPlan(id string, productIDs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]string, len(productIDs))
	copy(cp, productIDs)
	s.plans[id] = cp
}

// plan returns the cached plan for the given reprice or reloads it
// from CategoryReader if not cached (which happens after a
// restart).
func (s *Service) plan(ctx context.Context, r domain.Reprice) ([]string, error) {
	s.mu.Lock()
	cached, ok := s.plans[r.ID()]
	s.mu.Unlock()
	if ok {
		return cached, nil
	}
	productIDs, err := s.reader.ProductsInCategory(ctx, r.CategoryID())
	if err != nil {
		return nil, err
	}
	s.snapshotPlan(r.ID(), productIDs)
	return productIDs, nil
}

// forgetPlan drops the in-memory plan for the given id (called on
// Complete so the map does not grow unbounded over many sagas).
func (s *Service) forgetPlan(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.plans, id)
}

// ByID returns the reprice row with the given id.
func (s *Service) ByID(ctx context.Context, id string) (domain.Reprice, error) {
	if id == "" {
		return domain.Reprice{}, ErrNotFound
	}
	return s.storage.Find(ctx, id)
}

// Active returns the currently-active reprice (scheduled or
// in_progress). ok=false means no active reprice.
func (s *Service) Active(ctx context.Context) (domain.Reprice, bool, error) {
	return s.storage.FindActive(ctx)
}

// ListAll returns every reprice row, newest-started first.
func (s *Service) ListAll(ctx context.Context) ([]domain.Reprice, error) {
	return s.storage.ListAll(ctx)
}

// newRandomID returns a 16-byte hex string prefixed with "rep-".
func newRandomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("rep-%d", time.Now().UnixNano())
	}
	return "rep-" + hex.EncodeToString(buf)
}
