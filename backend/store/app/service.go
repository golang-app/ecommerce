// Package app is the application layer for the store bounded context.
// It orchestrates the Storage port with the domain value object to power
// per-request store resolution (ResolveByHost) and the admin-side CRUD
// flows. The resolver intentionally falls back to the configured
// default store when the request Host does not match a known row —
// that keeps `localhost:8080` (the dev affordance) always pointing at
// *some* store, regardless of the seeded host values.
package app

import (
	"context"
	"errors"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/store/domain"
)

var (
	// ErrStoreNotFound is returned by Storage when the requested store
	// cannot be located (by id or by host). The service uses it to
	// branch into the default-store fallback path.
	ErrStoreNotFound = errors.New("store not found")
	// ErrNoDefaultStore is returned when no store is marked as default.
	// In a healthy install this is impossible — the migration's partial
	// unique index plus the seed entries guarantee exactly one default
	// — but the error is surfaced for completeness and to make tests
	// of the empty-database edge case explicit.
	ErrNoDefaultStore = errors.New("no default store configured")
)

// Storage is the persistence port. Implementations live in
// store/adapter (postgres for production, in-memory for tests). The
// adapter is responsible for enforcing the "exactly one default"
// invariant — the partial unique index on (is_default) WHERE is_default
// is the safety net, and Create / Update should clear the flag on every
// other row when the caller marks one default.
type Storage interface {
	Create(ctx context.Context, s domain.Store) error
	Update(ctx context.Context, s domain.Store) error
	Delete(ctx context.Context, id string) error
	Find(ctx context.Context, id string) (domain.Store, error)
	FindByHost(ctx context.Context, host string) (domain.Store, error)
	Default(ctx context.Context) (domain.Store, error)
	ListAll(ctx context.Context) ([]domain.Store, error)
}

// Service is the application facade for the store context. It is a
// pass-through over Storage for CRUD plus the ResolveByHost helper
// that the layout's request middleware calls.
type Service struct {
	storage Storage
}

// NewService binds the application facade to a Storage port.
func NewService(storage Storage) *Service {
	return &Service{storage: storage}
}

// Create persists a new store. Validation lives on domain.NewStore;
// the service simply routes the call.
func (s *Service) Create(ctx context.Context, store domain.Store) error {
	return s.storage.Create(ctx, store)
}

// Update replaces every field on an existing store.
func (s *Service) Update(ctx context.Context, store domain.Store) error {
	return s.storage.Update(ctx, store)
}

// Delete drops the store row.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.storage.Delete(ctx, id)
}

// Find loads a single store by id.
func (s *Service) Find(ctx context.Context, id string) (domain.Store, error) {
	return s.storage.Find(ctx, id)
}

// ListAll returns every store ordered by position then name. The set is
// small (one row per storefront facade) so the call is cheap on every
// render — the layout uses it to power the footer switcher.
func (s *Service) ListAll(ctx context.Context) ([]domain.Store, error) {
	return s.storage.ListAll(ctx)
}

// Default returns the store marked as default, or ErrNoDefaultStore
// when none exists.
func (s *Service) Default(ctx context.Context) (domain.Store, error) {
	return s.storage.Default(ctx)
}

// ResolveByHost returns the store bound to the request Host header. The
// match is by exact host string; case is normalised to lowercase so
// "EU.localhost:8080" and "eu.localhost:8080" map to the same row.
//
// When no store matches (typically dev hitting `localhost:8080`
// without the configured host alias, or a misconfigured DNS entry in
// production), the resolver falls back to the configured default
// store. That fallback is the user-facing affordance that keeps the
// storefront rendering when the operator's host config has drifted.
// Only when *also* no default exists does ResolveByHost surface an
// error — callers should treat that as fatal/unrecoverable.
func (s *Service) ResolveByHost(ctx context.Context, host string) (domain.Store, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host != "" {
		st, err := s.storage.FindByHost(ctx, host)
		if err == nil {
			return st, nil
		}
		if !errors.Is(err, ErrStoreNotFound) {
			return domain.Store{}, err
		}
	}
	return s.storage.Default(ctx)
}
