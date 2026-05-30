// Package adapter holds the payments bounded context's adapters: the
// storage adapters for the domain.Charge value object (in-memory +
// postgres) and the Anti-Corruption Layer that translates between the
// payments domain and the external fakestripe provider
// (fakestripe_acl.go).
package adapter

import (
	"context"
	"sync"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/payments/app"
	"github.com/bkielbasa/go-ecommerce/backend/payments/domain"
)

// InMemoryStorage is the test-friendly Storage. The single mutex
// matches the postgres adapter's atomic-insert semantics: the
// idempotency-key uniqueness check happens under the same lock that
// stores the row, so a concurrent double-insert produces
// ErrIdempotencyKeyConflict exactly as the unique constraint would.
type InMemoryStorage struct {
	mu      sync.Mutex
	byID    map[string]domain.Charge
	byKey   map[string]string // idempotency key -> charge id
	byRef   map[string]string // provider ref -> charge id
}

// NewInMemoryStorage returns an empty in-memory store.
func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		byID:  map[string]domain.Charge{},
		byKey: map[string]string{},
		byRef: map[string]string{},
	}
}

// Insert persists a new Charge. A duplicate idempotency key is mapped
// to app.ErrIdempotencyKeyConflict so the service can recover by
// reading the existing row.
func (s *InMemoryStorage) Insert(_ context.Context, c domain.Charge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.IdempotencyKey() != "" {
		if _, ok := s.byKey[c.IdempotencyKey()]; ok {
			return app.ErrIdempotencyKeyConflict
		}
	}
	s.byID[c.ID()] = c
	if c.IdempotencyKey() != "" {
		s.byKey[c.IdempotencyKey()] = c.ID()
	}
	if c.ProviderRef() != "" {
		s.byRef[c.ProviderRef()] = c.ID()
	}
	return nil
}

// Find returns the Charge by id or app.ErrChargeNotFound.
func (s *InMemoryStorage) Find(_ context.Context, id string) (domain.Charge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.byID[id]
	if !ok {
		return domain.Charge{}, app.ErrChargeNotFound
	}
	return c, nil
}

// UpdateStatus rebuilds the row in place. We keep the original
// timestamps except for updatedAt and use WithStatus so the rules
// stay in domain.Charge.
func (s *InMemoryStorage) UpdateStatus(_ context.Context, id string, status domain.Status, providerRef string, updatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.byID[id]
	if !ok {
		return app.ErrChargeNotFound
	}
	updated := c.WithStatus(status, providerRef, updatedAt)
	s.byID[id] = updated
	if updated.ProviderRef() != "" {
		s.byRef[updated.ProviderRef()] = id
	}
	return nil
}

// FindByIdempotencyKey returns the Charge for a key if one is on
// file; the bool is false when no row exists (NOT an error — the
// caller wants to know so it can insert).
func (s *InMemoryStorage) FindByIdempotencyKey(_ context.Context, key string) (domain.Charge, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byKey[key]
	if !ok {
		return domain.Charge{}, false, nil
	}
	c, ok := s.byID[id]
	if !ok {
		return domain.Charge{}, false, nil
	}
	return c, true, nil
}

// FindByProviderRef returns the Charge previously settled against the
// given provider reference. app.ErrChargeNotFound when nothing matches.
func (s *InMemoryStorage) FindByProviderRef(_ context.Context, providerRef string) (domain.Charge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byRef[providerRef]
	if !ok {
		return domain.Charge{}, app.ErrChargeNotFound
	}
	c, ok := s.byID[id]
	if !ok {
		return domain.Charge{}, app.ErrChargeNotFound
	}
	return c, nil
}
