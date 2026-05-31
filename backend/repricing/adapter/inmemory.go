package adapter

import (
	"context"
	"sort"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/repricing/app"
	"github.com/bkielbasa/go-ecommerce/backend/repricing/domain"
)

// InMemory is the test-friendly Storage adapter. It mirrors the
// postgres adapter's contract: Create returns app.ErrAlreadyActive
// when an active reprice already exists (modelling the partial
// unique index on the production table); Update returns
// app.ErrOptimisticLock when the expected version (Version() - 1)
// does not match the stored row.
type InMemory struct {
	mu   sync.Mutex
	rows map[string]domain.Reprice
}

// NewInMemory builds an empty in-memory store, used by service-level
// unit tests.
func NewInMemory() *InMemory {
	return &InMemory{
		rows: map[string]domain.Reprice{},
	}
}

// Create inserts a fresh row. A second Create while an existing
// row is scheduled / in_progress is rejected with
// app.ErrAlreadyActive — mirroring the postgres adapter, which
// relies on the partial unique index.
func (m *InMemory) Create(ctx context.Context, r domain.Reprice) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if isActive(r.Status()) {
		for _, existing := range m.rows {
			if isActive(existing.Status()) {
				return app.ErrAlreadyActive
			}
		}
	}
	if _, ok := m.rows[r.ID()]; ok {
		return app.ErrAlreadyActive
	}
	m.rows[r.ID()] = r
	return nil
}

// isActive returns true for the two statuses that occupy the
// "at most one active" slot.
func isActive(s domain.Status) bool {
	return s == domain.StatusScheduled || s == domain.StatusInProgress
}

// Update writes a state transition under optimistic concurrency.
// The expected version is Version() - 1.
func (m *InMemory) Update(ctx context.Context, r domain.Reprice) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.rows[r.ID()]
	if !ok {
		return app.ErrNotFound
	}
	if current.Version() != r.Version()-1 {
		return app.ErrOptimisticLock
	}
	m.rows[r.ID()] = r
	return nil
}

// Find returns the row by its id.
func (m *InMemory) Find(ctx context.Context, id string) (domain.Reprice, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return domain.Reprice{}, app.ErrNotFound
	}
	return r, nil
}

// FindActive returns the at-most-one active row, ok=false when none
// exists.
func (m *InMemory) FindActive(ctx context.Context) (domain.Reprice, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rows {
		if isActive(r.Status()) {
			return r, true, nil
		}
	}
	return domain.Reprice{}, false, nil
}

// ListAll returns every row, newest-started first.
func (m *InMemory) ListAll(ctx context.Context) ([]domain.Reprice, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Reprice, 0, len(m.rows))
	for _, r := range m.rows {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt().After(out[j].StartedAt())
	})
	return out, nil
}
