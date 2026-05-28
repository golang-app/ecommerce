// Package adapter holds the promo storage adapters. The in-memory adapter
// mirrors the postgres adapter's contract; both implement promo/app.Storage.
package adapter

import (
	"context"
	"sort"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/promo/app"
	"github.com/bkielbasa/go-ecommerce/backend/promo/domain"
)

// InMemory is the test-friendly Storage. A single mutex protects the
// catalogue + ledger so the atomic "check + insert + increment" Redeem
// step matches the postgres SELECT FOR UPDATE semantics.
type InMemory struct {
	mu          sync.Mutex
	codes       map[string]domain.Code
	redemptions []app.Redemption
}

// NewInMemory builds an empty in-memory store.
func NewInMemory() *InMemory {
	return &InMemory{codes: map[string]domain.Code{}}
}

// Create inserts a new code. A duplicate (PK collision) is reported as
// ErrCodeAlreadyExists to match the postgres adapter's unique-violation
// mapping.
func (m *InMemory) Create(_ context.Context, c domain.Code) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.codes[c.CodeText()]; ok {
		return app.ErrCodeAlreadyExists
	}
	m.codes[c.CodeText()] = c
	return nil
}

// Update replaces an existing code. used_count is preserved (callers must
// not rebuild it on the fly because the admin form doesn't surface it).
func (m *InMemory) Update(_ context.Context, c domain.Code) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.codes[c.CodeText()]
	if !ok {
		return app.ErrCodeNotFound
	}
	// Carry the running redemption tally across the edit.
	merged := domain.RebuildCode(
		c.CodeText(), c.Kind(), c.ValueMinor(), c.Currency(),
		c.ValidFrom(), c.ValidUntil(), c.MaxUses(), c.PerCustomerMax(),
		existing.UsedCount(), existing.CreatedAt(),
	)
	m.codes[c.CodeText()] = merged
	return nil
}

// Delete removes the code; the in-memory ledger keeps any old redemptions
// (postgres CASCADEs them, but the only consumer here is the service
// tests so the difference is academic).
func (m *InMemory) Delete(_ context.Context, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.codes[code]; !ok {
		return app.ErrCodeNotFound
	}
	delete(m.codes, code)
	return nil
}

// Find returns the catalogue row by code text.
func (m *InMemory) Find(_ context.Context, code string) (domain.Code, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.codes[code]
	if !ok {
		return domain.Code{}, app.ErrCodeNotFound
	}
	return c, nil
}

// ListAll returns every code newest-first.
func (m *InMemory) ListAll(_ context.Context) ([]domain.Code, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Code, 0, len(m.codes))
	for _, c := range m.codes {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt().After(out[j].CreatedAt()) })
	return out, nil
}

// CountRedemptionsByCustomer returns the per-(code,customer) tally.
func (m *InMemory) CountRedemptionsByCustomer(_ context.Context, code, customerID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.redemptions {
		if r.Code == code && r.CustomerID == customerID {
			n++
		}
	}
	return n, nil
}

// Redeem mirrors the postgres atomic check: under the single mutex we
// re-verify the code, the max_uses cap and the per-customer cap, insert
// the redemption (idempotent on the (code, order_id) PK), and bump
// used_count.
func (m *InMemory) Redeem(_ context.Context, r app.Redemption) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.codes[r.Code]
	if !ok {
		return app.ErrCodeNotFound
	}
	// Idempotency: the same order being redeemed twice is a no-op.
	for _, existing := range m.redemptions {
		if existing.Code == r.Code && existing.OrderID == r.OrderID {
			return nil
		}
	}
	if c.MaxUsesReached() {
		return app.ErrCodeMaxUsesReached
	}
	if c.PerCustomerMax() > 0 {
		n := 0
		for _, existing := range m.redemptions {
			if existing.Code == r.Code && existing.CustomerID == r.CustomerID {
				n++
			}
		}
		if n >= c.PerCustomerMax() {
			return app.ErrCodeCustomerLimit
		}
	}
	m.redemptions = append(m.redemptions, r)
	// Bump used_count by rebuilding the value object with the new tally.
	m.codes[r.Code] = domain.RebuildCode(
		c.CodeText(), c.Kind(), c.ValueMinor(), c.Currency(),
		c.ValidFrom(), c.ValidUntil(), c.MaxUses(), c.PerCustomerMax(),
		c.UsedCount()+1, c.CreatedAt(),
	)
	return nil
}
