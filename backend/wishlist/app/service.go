// Package app holds the wishlist application service. The wishlist
// bounded context is intentionally tiny — it owns the per-customer set of
// saved variants and exposes idempotent Add / Remove / Toggle plus a couple
// of read helpers. Cross-cutting concerns (auth, CSRF, htmx swaps) live in
// the layout package; the service stays a thin wrapper around Storage.
package app

import (
	"context"
	"strings"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/wishlist/domain"
)

// Storage is the persistence port for wishlist items. Both Add and Remove
// are idempotent: Add swallows the (customer_id, variant_id) primary-key
// conflict (SQLSTATE 23505 in postgres) so a double-click on the heart
// button is a no-op, and Remove silently affects zero rows when the entry
// is already gone. ListByCustomer returns entries newest-first.
type Storage interface {
	Add(ctx context.Context, customerID, variantID string, addedAt time.Time) error
	Remove(ctx context.Context, customerID, variantID string) error
	ListByCustomer(ctx context.Context, customerID string) ([]domain.Item, error)
	Contains(ctx context.Context, customerID, variantID string) (bool, error)
}

// Service is the application-level facade over Storage. It is intentionally
// thin: each operation is a near-direct pass-through, with the current time
// injected for Add so tests can pin addedAt deterministically.
type Service struct {
	storage Storage
	now     func() time.Time
}

// NewService wires the service against a Storage. Time defaults to the
// standard library; tests can override via WithClock.
func NewService(storage Storage) *Service {
	return &Service{storage: storage, now: time.Now}
}

// WithClock overrides the time source — used by tests to pin addedAt.
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// Add stores the (customer, variant) bookmark. Idempotent: if the entry
// already exists the call succeeds without touching addedAt.
func (s *Service) Add(ctx context.Context, customerID, variantID string) error {
	return s.storage.Add(ctx, customerID, variantID, s.now())
}

// Remove deletes the (customer, variant) bookmark. Idempotent: removing an
// absent entry is a no-op.
func (s *Service) Remove(ctx context.Context, customerID, variantID string) error {
	return s.storage.Remove(ctx, customerID, variantID)
}

// ListByCustomer returns the customer's wishlist, newest-first.
func (s *Service) ListByCustomer(ctx context.Context, customerID string) ([]domain.Item, error) {
	if strings.TrimSpace(customerID) == "" {
		return nil, nil
	}
	return s.storage.ListByCustomer(ctx, customerID)
}

// Contains reports whether (customer, variant) is currently in the
// wishlist. Used by the product page to decide between the outline heart
// (add) and the filled heart (remove).
func (s *Service) Contains(ctx context.Context, customerID, variantID string) (bool, error) {
	if strings.TrimSpace(customerID) == "" || strings.TrimSpace(variantID) == "" {
		return false, nil
	}
	return s.storage.Contains(ctx, customerID, variantID)
}

// Toggle flips the (customer, variant) bookmark: it adds the variant if it
// is not in the wishlist and removes it if it is. The bool returned is the
// post-call state — true means the variant is now in the wishlist, false
// means it was removed. The HTTP layer uses this to render the next button
// state for the htmx outerHTML swap.
func (s *Service) Toggle(ctx context.Context, customerID, variantID string) (bool, error) {
	present, err := s.storage.Contains(ctx, customerID, variantID)
	if err != nil {
		return false, err
	}
	if present {
		if err := s.storage.Remove(ctx, customerID, variantID); err != nil {
			return false, err
		}
		return false, nil
	}
	if err := s.storage.Add(ctx, customerID, variantID, s.now()); err != nil {
		return false, err
	}
	return true, nil
}
