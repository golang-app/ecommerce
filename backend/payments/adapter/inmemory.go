// Package adapter holds the payments bounded context's adapters: the
// storage adapters for the domain.Charge value object (in-memory +
// postgres) and the Anti-Corruption Layer that translates between the
// payments domain and the external fakestripe provider
// (fakestripe_acl.go).
package adapter

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/payments/app"
	"github.com/bkielbasa/go-ecommerce/backend/payments/domain"
)

var _ app.Storage = (*InMemoryStorage)(nil)

// InMemoryStorage is the test-friendly Storage. The single mutex
// matches the postgres adapter's atomic-insert semantics: the
// idempotency-key uniqueness check happens under the same lock that
// stores the row, so a concurrent double-insert produces
// ErrIdempotencyKeyConflict exactly as the unique constraint would.
type InMemoryStorage struct {
	mu        sync.Mutex
	byID      map[string]domain.Charge
	byKey     map[string]string // idempotency key -> charge id
	byRef     map[string]string // provider ref -> charge id
	byOrderID map[string]string // order id -> charge id
	providers map[string]domain.ProviderConfig
}

// NewInMemoryStorage returns an in-memory store initialized with default providers.
func NewInMemoryStorage() *InMemoryStorage {
	now := time.Now().UTC()
	return &InMemoryStorage{
		byID:      map[string]domain.Charge{},
		byKey:     map[string]string{},
		byRef:     map[string]string{},
		byOrderID: map[string]string{},
		providers: map[string]domain.ProviderConfig{
			domain.ProviderStripe: domain.NewProviderConfig(
				domain.ProviderStripe,
				true,
				map[string]string{
					"fail_card_ending_in": "0000",
					"publishable_key":     "",
					"secret_key":          "",
					"webhook_secret":      "whsec_dev_only_do_not_use_in_production",
				},
				now,
			),
			domain.ProviderFake: domain.NewProviderConfig(
				domain.ProviderFake,
				true,
				map[string]string{
					"name":        "Fake Payment Simulator",
					"description": "Interactive payment simulator for testing payment outcomes.",
				},
				now,
			),
		},
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
	if c.OrderID() != "" {
		s.byOrderID[c.OrderID()] = c.ID()
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

// FindByOrderID returns the most recent Charge associated with orderID,
// or app.ErrChargeNotFound if none exists.
func (s *InMemoryStorage) FindByOrderID(_ context.Context, orderID string) (domain.Charge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if orderID == "" {
		return domain.Charge{}, app.ErrChargeNotFound
	}
	id, ok := s.byOrderID[orderID]
	if !ok {
		return domain.Charge{}, app.ErrChargeNotFound
	}
	c, ok := s.byID[id]
	if !ok {
		return domain.Charge{}, app.ErrChargeNotFound
	}
	return c, nil
}

// ListProviders returns all provider configurations ordered by ID ascending.
func (s *InMemoryStorage) ListProviders(_ context.Context) ([]domain.ProviderConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]domain.ProviderConfig, 0, len(s.providers))
	for _, p := range s.providers {
		res = append(res, p)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].ID() < res[j].ID()
	})
	return res, nil
}

// FindProvider returns the ProviderConfig by provider id, or app.ErrProviderNotFound.
func (s *InMemoryStorage) FindProvider(_ context.Context, id string) (domain.ProviderConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.providers[id]
	if !ok {
		return domain.ProviderConfig{}, app.ErrProviderNotFound
	}
	return p, nil
}

// SaveProvider stores or updates a provider configuration.
func (s *InMemoryStorage) SaveProvider(_ context.Context, cfg domain.ProviderConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providers[cfg.ID()] = cfg
	return nil
}

