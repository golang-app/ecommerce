package adapter

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/reviews/app"
	"github.com/bkielbasa/go-ecommerce/backend/reviews/domain"
)

// InMemory is the test-friendly Storage adapter. It mirrors the postgres
// adapter's contract: the unique-index conflict is surfaced as
// app.ErrDuplicateReview; soft-deleted rows are excluded from ByProduct,
// AggregateForProducts and HasReviewed.
type InMemory struct {
	mu      sync.Mutex
	reviews []domain.Review
}

// NewInMemory builds an empty in-memory store, used by the service-level
// unit tests.
func NewInMemory() *InMemory {
	return &InMemory{}
}

// Insert mirrors the postgres unique-index conflict: a second active review
// from the same customer for the same product is rejected with
// app.ErrDuplicateReview.
func (m *InMemory) Insert(ctx context.Context, r domain.Review) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.reviews {
		if existing.IsDeleted() {
			continue
		}
		if existing.ProductID() == r.ProductID() && existing.CustomerID() == r.CustomerID() {
			return app.ErrDuplicateReview
		}
	}
	m.reviews = append(m.reviews, r)
	return nil
}

// SoftDelete stamps deleted_at on the matching review. Missing ids are a
// no-op (mirroring the postgres UPDATE that simply affects 0 rows).
func (m *InMemory) SoftDelete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for i, r := range m.reviews {
		if r.ID() != id || r.IsDeleted() {
			continue
		}
		m.reviews[i] = domain.RebuildReview(r.ID(), r.ProductID(), r.CustomerID(), r.Body(), r.Rating(), r.CreatedAt(), &now)
		return nil
	}
	return nil
}

// ByProduct returns the active reviews for a product, newest-first, capped
// at limit.
func (m *InMemory) ByProduct(ctx context.Context, productID string, limit int) ([]domain.Review, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Review
	for _, r := range m.reviews {
		if r.IsDeleted() || r.ProductID() != productID {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt().After(out[j].CreatedAt()) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// AggregateForProducts returns one Aggregate per product id that has at
// least one active review.
func (m *InMemory) AggregateForProducts(ctx context.Context, productIDs []string) (map[string]domain.Aggregate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	wanted := make(map[string]bool, len(productIDs))
	for _, id := range productIDs {
		wanted[id] = true
	}

	type acc struct {
		sum   int
		count int
	}
	totals := map[string]*acc{}
	for _, r := range m.reviews {
		if r.IsDeleted() {
			continue
		}
		if !wanted[r.ProductID()] {
			continue
		}
		a, ok := totals[r.ProductID()]
		if !ok {
			a = &acc{}
			totals[r.ProductID()] = a
		}
		a.sum += r.Rating()
		a.count++
	}
	out := make(map[string]domain.Aggregate, len(totals))
	for pid, a := range totals {
		out[pid] = domain.RebuildAggregate(pid, float64(a.sum)/float64(a.count), a.count)
	}
	return out, nil
}

// HasReviewed reports whether an active review by this customer exists for
// the product.
func (m *InMemory) HasReviewed(ctx context.Context, productID, customerID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.reviews {
		if r.IsDeleted() {
			continue
		}
		if r.ProductID() == productID && r.CustomerID() == customerID {
			return true, nil
		}
	}
	return false, nil
}
