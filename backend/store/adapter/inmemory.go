// Package adapter holds the store storage adapters. The in-memory
// adapter mirrors the postgres adapter's contract; both implement
// store/app.Storage.
package adapter

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/store/app"
	"github.com/bkielbasa/go-ecommerce/backend/store/domain"
)

// InMemory is the test-friendly Storage. A single mutex protects the
// map so the "ensure exactly one default" update can be atomic.
type InMemory struct {
	mu     sync.Mutex
	stores map[string]domain.Store
}

// NewInMemory builds an empty in-memory store.
func NewInMemory() *InMemory {
	return &InMemory{stores: map[string]domain.Store{}}
}

// Create inserts a new store row. When the new store is marked
// default, every other row is cleared first so the "exactly one
// default" invariant matches the postgres adapter's behaviour.
func (m *InMemory) Create(_ context.Context, s domain.Store) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.stores[s.ID()]; ok {
		return errAlreadyExists
	}
	if s.IsDefault() {
		m.clearOtherDefaults(s.ID())
	}
	m.stores[s.ID()] = s
	return nil
}

// Update replaces an existing store row. When the updated store is
// marked default, every other row is cleared first.
func (m *InMemory) Update(_ context.Context, s domain.Store) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.stores[s.ID()]; !ok {
		return app.ErrStoreNotFound
	}
	if s.IsDefault() {
		m.clearOtherDefaults(s.ID())
	}
	m.stores[s.ID()] = s
	return nil
}

// Delete drops a store row.
func (m *InMemory) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.stores[id]; !ok {
		return app.ErrStoreNotFound
	}
	delete(m.stores, id)
	return nil
}

// Find loads a single store by id.
func (m *InMemory) Find(_ context.Context, id string) (domain.Store, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.stores[id]; ok {
		return s, nil
	}
	return domain.Store{}, app.ErrStoreNotFound
}

// FindByHost locates the store whose host matches (case-insensitive).
func (m *InMemory) FindByHost(_ context.Context, host string) (domain.Store, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	host = strings.ToLower(strings.TrimSpace(host))
	for _, s := range m.stores {
		if strings.ToLower(s.Host()) == host {
			return s, nil
		}
	}
	return domain.Store{}, app.ErrStoreNotFound
}

// Default returns the store marked as default.
func (m *InMemory) Default(_ context.Context) (domain.Store, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.stores {
		if s.IsDefault() {
			return s, nil
		}
	}
	return domain.Store{}, app.ErrNoDefaultStore
}

// ListAll returns every store ordered by position then name.
func (m *InMemory) ListAll(_ context.Context) ([]domain.Store, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Store, 0, len(m.stores))
	for _, s := range m.stores {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Position() != out[j].Position() {
			return out[i].Position() < out[j].Position()
		}
		return out[i].Name() < out[j].Name()
	})
	return out, nil
}

// clearOtherDefaults flips is_default to false on every row except the
// one with the supplied id. Called by Create/Update inside the same
// lock as the upsert.
func (m *InMemory) clearOtherDefaults(keepID string) {
	for id, s := range m.stores {
		if id == keepID || !s.IsDefault() {
			continue
		}
		m.stores[id] = domain.RebuildStore(
			s.ID(), s.Slug(), s.Name(), s.Currency(),
			s.Host(), false, s.Position(),
		)
	}
}

// errAlreadyExists is the in-memory analogue of a postgres unique
// violation on the id PK. It is package-scoped because the adapter is
// only used by tests — the production wiring goes through the postgres
// adapter, which surfaces the violation as a wrapped error.
var errAlreadyExists = &alreadyExistsError{}

type alreadyExistsError struct{}

func (*alreadyExistsError) Error() string { return "store already exists" }
