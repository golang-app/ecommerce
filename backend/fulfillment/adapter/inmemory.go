package adapter

import (
	"context"
	"sort"
	"sync"

	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/app"
	"github.com/bkielbasa/go-ecommerce/backend/fulfillment/domain"
)

// InMemory is the test-friendly Storage adapter. It mirrors the
// postgres adapter's contract: Create returns domain.ErrAlreadyExists
// on a duplicate order id, Update returns app.ErrOptimisticLock when
// the expected version (Version() - 1) does not match the stored row.
type InMemory struct {
	mu    sync.Mutex
	rows  map[string]domain.Fulfillment // keyed by id
	order map[string]string             // orderID -> id index
}

// NewInMemory builds an empty in-memory store, used by service-level
// unit tests.
func NewInMemory() *InMemory {
	return &InMemory{
		rows:  map[string]domain.Fulfillment{},
		order: map[string]string{},
	}
}

// Create inserts a fresh row. A second Create for the same order id is
// rejected with domain.ErrAlreadyExists — the postgres adapter relies
// on the table's UNIQUE constraint to surface the same sentinel.
//
// Stored rows are stripped of pending events (via domain.Rebuild) so a
// subsequent FindByOrder doesn't replay them as if they were freshly
// raised by the latest command.
func (m *InMemory) Create(ctx context.Context, f domain.Fulfillment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.order[f.OrderID()]; ok {
		return domain.ErrAlreadyExists
	}
	m.rows[f.ID()] = clean(f)
	m.order[f.OrderID()] = f.ID()
	return nil
}

// Update writes a state transition under optimistic concurrency. The
// expected version is Version() - 1 because each successful command
// already bumped the in-memory version; the storage row carries the
// previously-committed version.
func (m *InMemory) Update(ctx context.Context, f domain.Fulfillment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.rows[f.ID()]
	if !ok {
		return app.ErrNotFound
	}
	if current.Version() != f.Version()-1 {
		return app.ErrOptimisticLock
	}
	m.rows[f.ID()] = clean(f)
	return nil
}

// clean returns a copy of f without any in-flight pending events.
// Storage rows model the persisted state only; pending events are an
// in-memory bridge to the publisher and must not leak across loads.
func clean(f domain.Fulfillment) domain.Fulfillment {
	return domain.Rebuild(
		f.ID(), f.OrderID(),
		f.Status(),
		f.Carrier(), f.TrackingCode(),
		f.ScheduledAt(), f.ShippedAt(), f.DeliveredAt(),
		f.RefundReason(),
		f.Version(),
	)
}

// Find returns the row by its id.
func (m *InMemory) Find(ctx context.Context, id string) (domain.Fulfillment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.rows[id]
	if !ok {
		return domain.Fulfillment{}, app.ErrNotFound
	}
	return f, nil
}

// FindByOrder returns the row keyed by order id.
func (m *InMemory) FindByOrder(ctx context.Context, orderID string) (domain.Fulfillment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.order[orderID]
	if !ok {
		return domain.Fulfillment{}, app.ErrNotFound
	}
	return m.rows[id], nil
}

// ListAll returns every row, scheduled-newest first.
func (m *InMemory) ListAll(ctx context.Context) ([]domain.Fulfillment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]domain.Fulfillment, 0, len(m.rows))
	for _, f := range m.rows {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ScheduledAt().After(out[j].ScheduledAt())
	})
	return out, nil
}
