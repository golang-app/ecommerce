package query

import "context"

// Repository is the read side's data source — it reads the projection tables
// and returns read models, never the write aggregate.
type Repository interface {
	Find(ctx context.Context, id string) (OrderView, error)
	ListByCustomer(ctx context.Context, customerID string) ([]OrderSummary, error)
	ListAll(ctx context.Context) ([]OrderSummary, error)
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
