package adapter

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/wishlist/domain"
)

// InMemory is the test-friendly Storage adapter. It mirrors the postgres
// adapter's contract: Add is idempotent on the (customer, variant) key,
// Remove silently affects zero rows when the entry is missing, and
// ListByCustomer returns entries newest-first.
type InMemory struct {
	mu    sync.Mutex
	items []domain.Item
}

// NewInMemory builds an empty in-memory store, used by the service-level
// unit tests.
func NewInMemory() *InMemory {
	return &InMemory{}
}

// Add stores a new (customer, variant) entry. A duplicate is silently
// ignored to mirror the postgres ON CONFLICT DO NOTHING.
func (m *InMemory) Add(ctx context.Context, customerID, variantID string, addedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, it := range m.items {
		if it.CustomerID() == customerID && it.VariantID() == variantID {
			return nil
		}
	}
	m.items = append(m.items, domain.Rebuild(customerID, variantID, addedAt))
	return nil
}

// Remove deletes the matching entry. Missing entries are a no-op.
func (m *InMemory) Remove(ctx context.Context, customerID, variantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, it := range m.items {
		if it.CustomerID() == customerID && it.VariantID() == variantID {
			m.items = append(m.items[:i], m.items[i+1:]...)
			return nil
		}
	}
	return nil
}

// ListByCustomer returns the customer's wishlist newest-first.
func (m *InMemory) ListByCustomer(ctx context.Context, customerID string) ([]domain.Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.Item
	for _, it := range m.items {
		if it.CustomerID() == customerID {
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AddedAt().After(out[j].AddedAt()) })
	return out, nil
}

// Contains reports whether the (customer, variant) pair is currently in
// the wishlist.
func (m *InMemory) Contains(ctx context.Context, customerID, variantID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, it := range m.items {
		if it.CustomerID() == customerID && it.VariantID() == variantID {
			return true, nil
		}
	}
	return false, nil
}
