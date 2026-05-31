// Package adapter holds the recommendation context's persistence
// adapters: an in-memory pair (Storage + WeightsStorage) used by
// unit tests, and a Postgres pair used in production. The two halves
// share no code — the in-memory version is deliberately tiny and
// keeps the test surface obvious.
package adapter

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/recommendation/domain"
)

// InMemoryStorage backs Storage in unit tests. UpsertTopN replaces
// the seed's slice under a mutex so a concurrent reader sees the
// old slice in full or the new one in full — matching the
// postgres adapter's transactional atomicity.
type InMemoryStorage struct {
	mu        sync.Mutex
	rows      map[string][]domain.Recommendation
	computed  map[string]time.Time
	clockFunc func() time.Time
}

// NewInMemoryStorage returns an empty storage with the system clock
// as the computed-at source.
func NewInMemoryStorage() *InMemoryStorage {
	return &InMemoryStorage{
		rows:      map[string][]domain.Recommendation{},
		computed:  map[string]time.Time{},
		clockFunc: func() time.Time { return time.Now().UTC() },
	}
}

// WithClock overrides the clock so tests can assert on the
// computed_at timestamps deterministically.
func (s *InMemoryStorage) WithClock(now func() time.Time) *InMemoryStorage {
	if now != nil {
		s.clockFunc = now
	}
	return s
}

// UpsertTopN atomically replaces the seed's list.
func (s *InMemoryStorage) UpsertTopN(_ context.Context, productID string, recs []domain.Recommendation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]domain.Recommendation, len(recs))
	copy(cp, recs)
	s.rows[productID] = cp
	s.computed[productID] = s.clockFunc()
	return nil
}

// TopN returns up to `limit` rows for the seed, sorted by position.
func (s *InMemoryStorage) TopN(_ context.Context, productID string, limit int) ([]domain.Recommendation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := s.rows[productID]
	if len(rows) == 0 {
		return nil, nil
	}
	out := make([]domain.Recommendation, len(rows))
	copy(out, rows)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// LastComputedAt returns the per-seed computed timestamp.
func (s *InMemoryStorage) LastComputedAt(_ context.Context, productID string) (time.Time, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.computed[productID]
	return t, ok, nil
}

// OverallLastRefresh returns the latest computed_at across every
// seed; ok=false when the store is empty.
func (s *InMemoryStorage) OverallLastRefresh(_ context.Context) (time.Time, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var latest time.Time
	seen := false
	for _, t := range s.computed {
		if !seen || t.After(latest) {
			latest = t
			seen = true
		}
	}
	return latest, seen, nil
}

// InMemoryWeights backs WeightsStorage in unit tests. Get returns
// the persisted value or domain.DefaultWeights() if Set has never
// been called.
type InMemoryWeights struct {
	mu      sync.Mutex
	set     bool
	weights domain.Weights
}

// NewInMemoryWeights returns a store with no persisted weights yet
// (Get will fall back to defaults until the first Set).
func NewInMemoryWeights() *InMemoryWeights {
	return &InMemoryWeights{}
}

// Get returns the persisted weights or domain defaults.
func (w *InMemoryWeights) Get(_ context.Context) (domain.Weights, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.set {
		return domain.DefaultWeights(), nil
	}
	return w.weights, nil
}

// Set persists the supplied weights.
func (w *InMemoryWeights) Set(_ context.Context, weights domain.Weights) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.weights = weights
	w.set = true
	return nil
}
