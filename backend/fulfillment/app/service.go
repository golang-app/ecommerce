// Package app holds the fulfillment application service: the
// orchestrating layer between domain commands, storage, and the
// integration event publisher. Each command follows the same shape —
// load the record, apply the domain command, save, then drain pending
// events onto the bus.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/domain"
	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/integration"
	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
)

// ErrNotFound is returned by Storage.Find / FindByOrder when no row
// matches. Mirrors the sentinel pattern used in checkout/domain so
// callers can branch with errors.Is.
var ErrNotFound = errors.New("fulfillment: not found")

// ErrOptimisticLock is returned by Storage.Update when the row's
// version column does not match the expected version — i.e. someone
// else updated the row since we loaded it. Callers should reload and
// retry; the service surface itself returns the error verbatim so the
// HTTP layer can surface a sensible message.
var ErrOptimisticLock = errors.New("fulfillment: optimistic lock conflict")

// Storage is the persistence port for fulfillment records. The
// adapter package supplies an in-memory implementation (tests) and a
// Postgres-backed one (production). Update checks the row's version
// column for optimistic concurrency: a mismatch returns
// ErrOptimisticLock.
type Storage interface {
	// Create inserts a fresh fulfillment row. Adapter returns
	// domain.ErrAlreadyExists when (order_id) collides with an
	// existing row (unique constraint).
	Create(ctx context.Context, f domain.Fulfillment) error
	// Update writes a state transition. Adapter MUST match on the
	// expected version (Version() - 1 after a command); a mismatch
	// returns ErrOptimisticLock.
	Update(ctx context.Context, f domain.Fulfillment) error
	// Find returns the row by its own id, ErrNotFound if missing.
	Find(ctx context.Context, id string) (domain.Fulfillment, error)
	// FindByOrder returns the row keyed by order id, ErrNotFound
	// if there is no fulfillment for the order yet.
	FindByOrder(ctx context.Context, orderID string) (domain.Fulfillment, error)
	// ListAll returns every fulfillment, newest-scheduled first.
	// Used by admin / reporting surfaces.
	ListAll(ctx context.Context) ([]domain.Fulfillment, error)
}

// EventPublisher is the seam onto which the service drains pending
// domain events translated into integration events. The composition
// root wires it to the in-process eventbus.Bus; tests can substitute a
// recording publisher.
type EventPublisher interface {
	Publish(ctx context.Context, e eventbus.Event)
}

// nopPublisher is the default when no publisher is supplied —
// keeps the service constructible in tests that don't care about the
// outbound bus.
type nopPublisher struct{}

func (nopPublisher) Publish(context.Context, eventbus.Event) {}

// StockReleaser is the seam onto which the refund flow releases stock
// previously reserved at checkout time. The composition root wires it
// to the productcatalog stock adapter — same port the checkout
// context used to call directly before the refund flow moved here.
// A nil StockReleaser disables the release (matches tests that don't
// wire a catalogue at all).
type StockReleaser interface {
	Release(ctx context.Context, quantities map[string]int) error
}

type nopReleaser struct{}

func (nopReleaser) Release(context.Context, map[string]int) error { return nil }

// OrderLineSource is the seam the service uses at refund time to
// discover which variants and quantities belong to the order being
// refunded. Implemented by checkout's query side
// (HasPurchasedProduct's sibling — see the adapter wired in cmd/web).
// A nil source disables the release call (best-effort, matches the
// historical behaviour where checkout knew the lines itself).
type OrderLineSource interface {
	OrderQuantities(ctx context.Context, orderID string) (map[string]int, error)
}

// Service is the application facade. Methods follow the same shape:
//
//  1. load the record (or no-op if missing for OnOrderPaid)
//  2. apply the domain command
//  3. save (Create / Update)
//  4. publish pending events
type Service struct {
	storage   Storage
	publisher EventPublisher
	stock     StockReleaser
	lines     OrderLineSource
	now       func() time.Time
	newID     func() string
}

// NewService wires the service against its storage adapter. Optional
// dependencies (publisher, stock releaser, line source, clock, id
// generator) are set via With… methods on the returned value.
func NewService(storage Storage) *Service {
	return &Service{
		storage:   storage,
		publisher: nopPublisher{},
		stock:     nopReleaser{},
		now:       func() time.Time { return time.Now().UTC() },
		newID:     newRandomID,
	}
}

// WithPublisher wires an integration event publisher (the in-process
// bus in production).
func (s *Service) WithPublisher(p EventPublisher) *Service {
	if p == nil {
		p = nopPublisher{}
	}
	s.publisher = p
	return s
}

// WithStockReleaser wires the productcatalog stock port so Refund can
// return reserved units to the catalogue.
func (s *Service) WithStockReleaser(r StockReleaser) *Service {
	if r == nil {
		r = nopReleaser{}
	}
	s.stock = r
	return s
}

// WithOrderLines wires the seam onto which Refund discovers the
// order's quantities. Without it the stock release on refund is a
// best-effort no-op.
func (s *Service) WithOrderLines(src OrderLineSource) *Service {
	s.lines = src
	return s
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

// OnOrderPaid is the subscriber the composition root binds to the
// checkout context's OrderPaid integration event. It spawns a new
// Fulfillment in StatusScheduled and publishes
// FulfillmentScheduled-derived events (none today; the integration
// surface is shipped/delivered/refunded only).
//
// IDEMPOTENT. The transactional outbox delivers OrderPaid
// at-least-once: a duplicate publish (process crash between dispatch
// and mark-sent) MUST be a no-op. We check FindByOrder first and
// silently return nil when a record already exists — Create's unique
// constraint is the belt-and-braces second check.
func (s *Service) OnOrderPaid(ctx context.Context, orderID string, at time.Time) error {
	if orderID == "" {
		return errors.New("fulfillment: empty order id")
	}
	existing, err := s.storage.FindByOrder(ctx, orderID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("fulfillment: find by order: %w", err)
	}
	if err == nil {
		// Duplicate delivery from the outbox — silently no-op so
		// subscribers stay safe under at-least-once.
		_ = existing
		return nil
	}

	f := domain.NewFulfillment(s.newID(), orderID, at)
	if err := s.storage.Create(ctx, f); err != nil {
		// Lost a race against another concurrent delivery — also
		// no-op so we don't fail the bus handler.
		if errors.Is(err, domain.ErrAlreadyExists) {
			return nil
		}
		return fmt.Errorf("fulfillment: create: %w", err)
	}
	s.publishPending(ctx, &f)
	return nil
}

// Label records the carrier + tracking code on a scheduled
// fulfillment. The admin's "ship" form runs Label + Ship back-to-back
// — keep them separate so the state machine can grow a "labeled but
// not shipped" pause in future without changing the command surface.
func (s *Service) Label(ctx context.Context, orderID, carrier, trackingCode string) error {
	return s.transition(ctx, orderID, func(f *domain.Fulfillment) error {
		return f.Label(carrier, trackingCode, s.now())
	})
}

// Ship marks a labeled fulfillment shipped. Emits the
// fulfillment.OrderShipped integration event.
func (s *Service) Ship(ctx context.Context, orderID string) error {
	return s.transition(ctx, orderID, func(f *domain.Fulfillment) error {
		return f.Ship(s.now())
	})
}

// Deliver marks a shipped fulfillment delivered. Emits
// fulfillment.OrderDelivered.
func (s *Service) Deliver(ctx context.Context, orderID string) error {
	return s.transition(ctx, orderID, func(f *domain.Fulfillment) error {
		return f.Deliver(s.now())
	})
}

// Refund transitions any active fulfillment to refunded and releases
// the reserved stock back to the catalogue. The stock-release path
// mirrors the historical checkout.Refund flow (which used to live on
// the checkout service); moving it here keeps the operational concern
// inside the operational context.
func (s *Service) Refund(ctx context.Context, orderID, reason string) error {
	if err := s.transition(ctx, orderID, func(f *domain.Fulfillment) error {
		return f.Refund(reason, s.now())
	}); err != nil {
		return err
	}
	s.releaseStock(ctx, orderID)
	return nil
}

// ByOrder returns the current fulfillment for the given order id.
// Powers the admin / customer order pages.
func (s *Service) ByOrder(ctx context.Context, orderID string) (domain.Fulfillment, error) {
	if orderID == "" {
		return domain.Fulfillment{}, ErrNotFound
	}
	return s.storage.FindByOrder(ctx, orderID)
}

// transition loads the row by orderID, applies the supplied command,
// persists the update, and drains pending events onto the publisher.
// Used by Label / Ship / Deliver / Refund.
func (s *Service) transition(ctx context.Context, orderID string, cmd func(*domain.Fulfillment) error) error {
	f, err := s.storage.FindByOrder(ctx, orderID)
	if err != nil {
		return err
	}
	if err := cmd(&f); err != nil {
		return err
	}
	if err := s.storage.Update(ctx, f); err != nil {
		return fmt.Errorf("fulfillment: update: %w", err)
	}
	s.publishPending(ctx, &f)
	return nil
}

// publishPending translates each pending domain event into the
// matching integration event and pushes it onto the bus. Failures are
// logged by the bus (which our caller wires); the publisher itself is
// fire-and-forget so we don't gate the command success on a downstream
// consumer.
func (s *Service) publishPending(ctx context.Context, f *domain.Fulfillment) {
	for _, e := range f.PendingEvents() {
		switch ev := e.(type) {
		case domain.FulfillmentShipped:
			// Pull the carrier+tracking off the (already updated)
			// value object — the domain event itself does not
			// carry them, but the row's columns do.
			s.publisher.Publish(ctx, integration.OrderShipped{
				OrderID:      ev.OrderID,
				Carrier:      f.Carrier(),
				TrackingCode: f.TrackingCode(),
				At:           ev.At,
			})
		case domain.FulfillmentDelivered:
			s.publisher.Publish(ctx, integration.OrderDelivered{
				OrderID: ev.OrderID,
				At:      ev.At,
			})
		case domain.FulfillmentRefunded:
			s.publisher.Publish(ctx, integration.OrderRefunded{
				OrderID: ev.OrderID,
				Reason:  ev.Reason,
				At:      ev.At,
			})
		default:
			// FulfillmentScheduled / FulfillmentLabeled are
			// internal-only: no integration event today.
		}
	}
	f.ClearPending()
}

// releaseStock pulls the order's quantities from the configured
// OrderLineSource and hands them to the StockReleaser. Both seams are
// optional — when either is nil the call is a no-op so the service
// stays constructible in test setups that don't wire a catalogue.
func (s *Service) releaseStock(ctx context.Context, orderID string) {
	if s.lines == nil {
		return
	}
	qty, err := s.lines.OrderQuantities(ctx, orderID)
	if err != nil || len(qty) == 0 {
		return
	}
	_ = s.stock.Release(ctx, qty)
}

// newRandomID returns a 16-byte hex string prefixed with "ful-". Lib-
// free for the same reason the reviews context's id generator is:
// avoid a UUID dep just for this.
func newRandomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("ful-%d", time.Now().UnixNano())
	}
	return "ful-" + hex.EncodeToString(buf)
}
