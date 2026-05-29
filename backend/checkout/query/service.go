package query

import (
	"context"
	"time"
)

// Repository is the read side's data source — it reads the projection tables
// and returns read models, never the write aggregate.
type Repository interface {
	Find(ctx context.Context, id string) (OrderView, error)
	ListByCustomer(ctx context.Context, customerID string) ([]OrderSummary, error)
	ListAll(ctx context.Context) ([]OrderSummary, error)
	// ListExpiredPending returns the ids of orders still in the pending
	// status whose placed_at is strictly older than olderThan. The
	// reservation TTL sweeper uses this to find orphaned reservations to
	// release.
	ListExpiredPending(ctx context.Context, olderThan time.Time) ([]string, error)
	// HasPurchasedProduct reports whether the customer has at least one
	// fulfilled order (paid / shipped / delivered) containing any variant
	// of the catalog product. It exists to power the reviews context's
	// verified-buyer gate; see the reviews bounded context for the use site.
	HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error)
	// TodaysSales returns the analytics_daily_sales rows for "today" in UTC,
	// keyed by currency. The map is empty when no paid orders have been
	// recorded today; callers should treat that as "no card to render".
	TodaysSales(ctx context.Context) (map[string]DailySalesRow, error)
}

// Service is the checkout query side. It is intentionally separate from the
// command-side CheckoutService.
type Service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return Service{repo: repo}
}

// Find returns the detail read model for an order.
func (s Service) Find(ctx context.Context, id string) (OrderView, error) {
	return s.repo.Find(ctx, id)
}

// ListByCustomer returns the customer's orders newest-first as summaries.
// Anonymous (empty) customers have no order history.
func (s Service) ListByCustomer(ctx context.Context, customerID string) ([]OrderSummary, error) {
	if customerID == "" {
		return nil, nil
	}
	return s.repo.ListByCustomer(ctx, customerID)
}

// ListAll returns every order newest-first as summaries, regardless of
// customer. Intended for the admin order list.
func (s Service) ListAll(ctx context.Context) ([]OrderSummary, error) {
	return s.repo.ListAll(ctx)
}

// ListExpiredPending returns the ids of pending orders placed before
// olderThan — i.e. reservations whose TTL has elapsed without the order
// being confirmed or explicitly failed. Used by the reservation sweeper.
func (s Service) ListExpiredPending(ctx context.Context, olderThan time.Time) ([]string, error) {
	return s.repo.ListExpiredPending(ctx, olderThan)
}

// HasPurchasedProduct reports whether the customer has bought at least one
// variant of the given catalog product in a paid/shipped/delivered order.
// Returns false (no error) for anonymous (empty) customers.
func (s Service) HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error) {
	if customerID == "" || productID == "" {
		return false, nil
	}
	return s.repo.HasPurchasedProduct(ctx, customerID, productID)
}

// TodaysSales returns the analytics_daily_sales rows for today (UTC) by
// currency. Powers the admin "today's revenue" card.
func (s Service) TodaysSales(ctx context.Context) (map[string]DailySalesRow, error) {
	return s.repo.TodaysSales(ctx)
}
