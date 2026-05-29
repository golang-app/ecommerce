// Package app holds the reviews application service. It enforces the
// verified-buyer rule (via the VerifiedBuyerChecker port, which the
// composition root wires to the checkout query side) and the
// one-review-per-(customer, product) rule (the unique index in storage maps
// to ErrDuplicateReview). Admin soft-delete is a thin pass-through.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/reviews/domain"
)

// ErrDuplicateReview is returned by Submit when a (customer, product) pair
// already has an active review. Storage adapters map their underlying
// uniqueness violation onto this sentinel.
var ErrDuplicateReview = errors.New("review already exists for this product")

// ErrNotVerifiedBuyer is returned by Submit when the verified-buyer check
// reports the customer has not purchased the product.
var ErrNotVerifiedBuyer = errors.New("only verified buyers can leave a review")

// Storage is the persistence port for reviews. Adapters live in
// reviews/adapter (postgres + in-memory). The unique index on
// (product_id, customer_id) WHERE deleted_at IS NULL is mapped to
// ErrDuplicateReview by Insert. ByProduct / AggregateForProducts only
// surface approved reviews (the storefront calls them); the admin queue
// uses ListByStatus / ListAll to see every row.
type Storage interface {
	Insert(ctx context.Context, r domain.Review) error
	SoftDelete(ctx context.Context, id string) error
	SetStatus(ctx context.Context, id string, status domain.Status) error
	ByProduct(ctx context.Context, productID string, limit int) ([]domain.Review, error)
	AggregateForProducts(ctx context.Context, productIDs []string) (map[string]domain.Aggregate, error)
	HasReviewed(ctx context.Context, productID, customerID string) (bool, error)
	ListByStatus(ctx context.Context, status domain.Status, limit int) ([]domain.Review, error)
	ListAll(ctx context.Context, limit int) ([]domain.Review, error)
}

// VerifiedBuyerChecker is the cross-context port used to gate Submit. The
// composition root wires it to the checkout query side (a thin adapter
// calling checkoutQry.HasPurchasedProduct).
type VerifiedBuyerChecker interface {
	HasPurchased(ctx context.Context, customerID, productID string) (bool, error)
}

// Service is the application-level facade over Storage + VerifiedBuyerChecker.
// All cross-cutting concerns (id generation, time, verified-buyer policy) live
// here so the storage adapters stay dumb and the domain stays free of I/O.
type Service struct {
	storage Storage
	buyers  VerifiedBuyerChecker
	now     func() time.Time
	newID   func() string
}

// NewService wires the service against a Storage and a VerifiedBuyerChecker.
// Time and id generation default to the standard library; tests can override
// them with WithClock/WithIDGenerator.
func NewService(storage Storage, buyers VerifiedBuyerChecker) *Service {
	return &Service{
		storage: storage,
		buyers:  buyers,
		now:     time.Now,
		newID:   newRandomID,
	}
}

// WithClock overrides the time source — used by tests to pin createdAt.
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// WithIDGenerator overrides the id generator — used by tests to make ids
// predictable.
func (s *Service) WithIDGenerator(newID func() string) *Service {
	s.newID = newID
	return s
}

// Submit enforces the verified-buyer rule, rejects duplicate reviews up
// front (so the storefront returns a friendly error before hitting the DB),
// builds a validated domain.Review and writes it. Duplicate submissions that
// race past the up-front check are still caught by the unique-index
// conflict in Insert and returned as ErrDuplicateReview.
func (s *Service) Submit(ctx context.Context, productID, customerID, body string, rating int) error {
	productID = strings.TrimSpace(productID)
	customerID = strings.TrimSpace(customerID)
	if productID == "" {
		return fmt.Errorf("%w: product id is required", domain.ErrInvalidReview)
	}
	if customerID == "" {
		return fmt.Errorf("%w: customer id is required", domain.ErrInvalidReview)
	}

	bought, err := s.buyers.HasPurchased(ctx, customerID, productID)
	if err != nil {
		return fmt.Errorf("verified-buyer check: %w", err)
	}
	if !bought {
		return ErrNotVerifiedBuyer
	}

	already, err := s.storage.HasReviewed(ctx, productID, customerID)
	if err != nil {
		return fmt.Errorf("has-reviewed check: %w", err)
	}
	if already {
		return ErrDuplicateReview
	}

	// New submissions always land as pending — moderation policy lives
	// here so the domain stays a pure value object and the adapters stay
	// dumb. Admins flip the status via Approve / Reject.
	review, err := domain.NewReview(s.newID(), productID, customerID, body, rating, s.now(), domain.StatusPending)
	if err != nil {
		return err
	}
	return s.storage.Insert(ctx, review)
}

// Delete soft-deletes a review by id (admin-only at the HTTP layer).
func (s *Service) Delete(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("review id is required")
	}
	return s.storage.SoftDelete(ctx, id)
}

// Approve flips a review's moderation state to approved. From this point
// the row participates in ByProduct / AggregateForProducts (the storefront).
func (s *Service) Approve(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("review id is required")
	}
	return s.storage.SetStatus(ctx, id, domain.StatusApproved)
}

// Reject flips a review's moderation state to rejected. The row stays in
// storage (so the partial unique index still blocks resubmission until an
// admin soft-deletes it) but disappears from the storefront.
func (s *Service) Reject(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("review id is required")
	}
	return s.storage.SetStatus(ctx, id, domain.StatusRejected)
}

// ListForProduct returns the most recent active reviews for a product,
// newest first, capped at limit. A non-positive limit falls back to a
// sensible default so callers (templates) can pass 0 to mean "the default
// page-size".
func (s *Service) ListForProduct(ctx context.Context, productID string, limit int) ([]domain.Review, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.storage.ByProduct(ctx, productID, limit)
}

// AggregateForProducts returns the (avg, count) pair for every requested
// product id. Products with no active reviews are simply absent from the
// returned map so the template can render an empty state.
func (s *Service) AggregateForProducts(ctx context.Context, productIDs []string) (map[string]domain.Aggregate, error) {
	if len(productIDs) == 0 {
		return map[string]domain.Aggregate{}, nil
	}
	return s.storage.AggregateForProducts(ctx, productIDs)
}

// HasReviewed reports whether the customer has an active review for the
// product. Used by the storefront to decide whether to render the review
// form on the product page.
func (s *Service) HasReviewed(ctx context.Context, productID, customerID string) (bool, error) {
	if strings.TrimSpace(productID) == "" || strings.TrimSpace(customerID) == "" {
		return false, nil
	}
	return s.storage.HasReviewed(ctx, productID, customerID)
}

// ListPending returns the pending moderation queue (newest first, capped at
// limit). Powers the default tab on the admin reviews page.
func (s *Service) ListPending(ctx context.Context, limit int) ([]domain.Review, error) {
	if limit <= 0 {
		limit = 100
	}
	return s.storage.ListByStatus(ctx, domain.StatusPending, limit)
}

// ListAll returns every review (any status, excluding soft-deleted),
// newest first, capped at limit. Powers the "all" tab on the admin reviews
// page.
func (s *Service) ListAll(ctx context.Context, limit int) ([]domain.Review, error) {
	if limit <= 0 {
		limit = 100
	}
	return s.storage.ListAll(ctx, limit)
}

// newRandomID returns a 16-byte hex string — 128 bits of randomness is well
// beyond what reviews collisions would ever need; keeping it lib-free avoids
// pulling in a UUID dep just for this context.
func newRandomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		// Falling back to a time-based id keeps the service alive even when
		// the OS RNG hiccups; collisions are still astronomically unlikely
		// at any realistic review volume.
		return fmt.Sprintf("rev-%d", time.Now().UnixNano())
	}
	return "rev-" + hex.EncodeToString(buf)
}
