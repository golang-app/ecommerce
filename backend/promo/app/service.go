// Package app is the application layer for the promo bounded context: it
// orchestrates the Storage port with the domain value objects to support
// admin CRUD and the checkout-side Resolve/Redeem flow.
//
// The checkout context never touches Storage directly — it goes through
// Service.Resolve / Service.Redeem which apply the business rules
// (validity, max uses, per-customer limits) and reject anonymous customers
// outright. Anonymous rejection is deliberate: per-customer caps need a
// stable identity to be meaningful, so rather than invent a fragile
// "anonymous bucket" we tell the user to log in. See ErrCodeAnonymous.
package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/promo/domain"
)

// Storage is the persistence port. Implementations live in promo/adapter
// (postgres for the real wiring, in-memory for tests). Redeem MUST be
// atomic: it re-reads the code under lock, checks max_uses and the
// per-customer count, inserts the redemption row, and increments
// used_count — all in a single transaction. The (code, order_id) PK on
// promo_redemption gives free idempotency on retries.
type Storage interface {
	Create(ctx context.Context, c domain.Code) error
	Update(ctx context.Context, c domain.Code) error
	Delete(ctx context.Context, code string) error
	Find(ctx context.Context, code string) (domain.Code, error)
	ListAll(ctx context.Context) ([]domain.Code, error)
	CountRedemptionsByCustomer(ctx context.Context, code, customerID string) (int, error)
	Redeem(ctx context.Context, r Redemption) error
}

// Redemption is the per-order ledger entry written to promo_redemption.
type Redemption struct {
	Code        string
	OrderID     string
	CustomerID  string
	AmountMinor int64
	Currency    string
}

var (
	// ErrCodeNotFound is returned when the supplied code does not match a
	// catalogue row. The checkout handler maps it to a flash message.
	ErrCodeNotFound = errors.New("promo code not found")
	// ErrCodeExpired covers both ends of the validity window (before
	// valid_from or after valid_until).
	ErrCodeExpired = errors.New("promo code is not active")
	// ErrCodeMaxUsesReached is the global cap.
	ErrCodeMaxUsesReached = errors.New("promo code is no longer available")
	// ErrCodeCustomerLimit is the per-customer cap.
	ErrCodeCustomerLimit = errors.New("promo code already used")
	// ErrCodeAnonymous is returned when a logged-out customer submits a
	// promo code. The decision is documented at the top of the package —
	// we keep it simple and require a real account.
	ErrCodeAnonymous = errors.New("please sign in to use a promo code")
	// ErrCodeAlreadyExists is returned by Create when the code text
	// collides with an existing row.
	ErrCodeAlreadyExists = errors.New("promo code already exists")
)

// Service is the application facade.
type Service struct {
	storage Storage
	now     func() time.Time
}

// NewService wires the service against a Storage port. now is overridable
// for tests; the production wiring uses time.Now.
func NewService(storage Storage) *Service {
	return &Service{storage: storage, now: func() time.Time { return time.Now().UTC() }}
}

// WithClock injects a deterministic time source — used by unit tests to
// pin the validity-window checks.
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// Create persists a new promo code. Validation lives in domain.NewCode;
// the service just routes the call.
func (s *Service) Create(ctx context.Context, c domain.Code) error {
	return s.storage.Create(ctx, c)
}

// Update replaces every field of an existing code (the admin edit form
// always submits the full row).
func (s *Service) Update(ctx context.Context, c domain.Code) error {
	return s.storage.Update(ctx, c)
}

// Delete removes the catalogue row; the ON DELETE CASCADE on
// promo_redemption keeps the ledger consistent.
func (s *Service) Delete(ctx context.Context, code string) error {
	return s.storage.Delete(ctx, code)
}

// Find returns a single catalogue row by code text.
func (s *Service) Find(ctx context.Context, code string) (domain.Code, error) {
	return s.storage.Find(ctx, code)
}

// ListAll returns every code newest-first for the admin list page.
func (s *Service) ListAll(ctx context.Context) ([]domain.Code, error) {
	return s.storage.ListAll(ctx)
}

// Resolve looks up the code, runs every business rule, and returns the
// computed Discount the checkout pricing math can consume. The order is:
//   - non-empty code text
//   - customer must be signed in (anonymous use is rejected)
//   - code exists in the catalogue
//   - code is currently within its validity window
//   - global max_uses not exceeded
//   - per_customer_max not exceeded
//   - finally, apply the code to the supplied subtotal / shipping
func (s *Service) Resolve(ctx context.Context, code, customerID string, subtotal, shippingCost int64) (domain.Discount, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return domain.Discount{}, ErrCodeNotFound
	}
	if strings.TrimSpace(customerID) == "" {
		return domain.Discount{}, ErrCodeAnonymous
	}

	c, err := s.storage.Find(ctx, code)
	if err != nil {
		return domain.Discount{}, err
	}
	if !c.IsActiveAt(s.now()) {
		return domain.Discount{}, ErrCodeExpired
	}
	if c.MaxUsesReached() {
		return domain.Discount{}, ErrCodeMaxUsesReached
	}
	if c.PerCustomerMax() > 0 {
		used, err := s.storage.CountRedemptionsByCustomer(ctx, c.CodeText(), customerID)
		if err != nil {
			return domain.Discount{}, err
		}
		if used >= c.PerCustomerMax() {
			return domain.Discount{}, ErrCodeCustomerLimit
		}
	}
	return c.Apply(subtotal, shippingCost), nil
}

// Redeem persists the per-order redemption row and bumps used_count. The
// PK on promo_redemption makes Storage.Redeem idempotent on the same
// (code, order_id) so a checkout retry that re-runs after the order was
// already redeemed is a safe no-op.
func (s *Service) Redeem(ctx context.Context, code, orderID, customerID string, d domain.Discount) error {
	if strings.TrimSpace(code) == "" {
		return nil
	}
	return s.storage.Redeem(ctx, Redemption{
		Code:        code,
		OrderID:     orderID,
		CustomerID:  customerID,
		AmountMinor: d.AmountMinor(),
		Currency:    d.Currency(),
	})
}
