package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/recommendation/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/recommendation/app"
	"github.com/bkielbasa/go-ecommerce/backend/recommendation/domain"
)

// stubCatalog is the in-memory CatalogReader the service tests
// drive. ProductsInCategory returns the slice the test populated;
// SimilarByText / AllProductIDs are not exercised by service tests
// (they belong to the refresher's tests).
type stubCatalog struct {
	products map[string]domain.ProductSummary
	byCat    map[string][]string
	similar  map[string][]string
}

func newStubCatalog() *stubCatalog {
	return &stubCatalog{
		products: map[string]domain.ProductSummary{},
		byCat:    map[string][]string{},
		similar:  map[string][]string{},
	}
}

func (s *stubCatalog) add(p domain.ProductSummary) {
	s.products[p.ID] = p
	if p.CategoryID != "" {
		s.byCat[p.CategoryID] = append(s.byCat[p.CategoryID], p.ID)
	}
}

func (s *stubCatalog) ProductByID(_ context.Context, id string) (domain.ProductSummary, bool, error) {
	p, ok := s.products[id]
	return p, ok, nil
}

func (s *stubCatalog) AllProductIDs(_ context.Context) ([]string, error) {
	out := make([]string, 0, len(s.products))
	for id := range s.products {
		out = append(out, id)
	}
	return out, nil
}

func (s *stubCatalog) ProductsInCategory(_ context.Context, categoryID string, limit int) ([]domain.ProductSummary, error) {
	ids := s.byCat[categoryID]
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	out := make([]domain.ProductSummary, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.products[id])
	}
	return out, nil
}

func (s *stubCatalog) SimilarByText(_ context.Context, productID string, limit int) ([]domain.ProductSummary, error) {
	ids := s.similar[productID]
	if limit > 0 && len(ids) > limit {
		ids = ids[:limit]
	}
	out := make([]domain.ProductSummary, 0, len(ids))
	for _, id := range ids {
		if p, ok := s.products[id]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

// TestRecommendations_ColdStartFallsBackToCategory exercises the
// service's cold-start branch: storage is empty (no refresh has
// run), so Recommendations must consult the catalog ACL's
// ProductsInCategory and return the seed's peers, with the seed
// itself excluded.
func TestRecommendations_ColdStartFallsBackToCategory(t *testing.T) {
	ctx := context.Background()
	cat := newStubCatalog()
	cat.add(domain.ProductSummary{ID: "seed", CategoryID: "shoes", Name: "Seed"})
	cat.add(domain.ProductSummary{ID: "peer-1", CategoryID: "shoes", Name: "Peer 1"})
	cat.add(domain.ProductSummary{ID: "peer-2", CategoryID: "shoes", Name: "Peer 2"})
	cat.add(domain.ProductSummary{ID: "peer-3", CategoryID: "shoes", Name: "Peer 3"})

	storage := adapter.NewInMemoryStorage()
	weights := adapter.NewInMemoryWeights()
	svc := app.NewService(storage, cat, weights)

	got, err := svc.Recommendations(ctx, "seed")
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d, want 3 peers in same category", len(got))
	}
	for _, p := range got {
		if p.ID == "seed" {
			t.Fatalf("cold-start fallback must exclude the seed itself")
		}
		if p.CategoryID != "shoes" {
			t.Fatalf("expected only same-category peers, got %s", p.CategoryID)
		}
	}
}

// TestRecommendations_HydratesPersistedTopN verifies the happy
// path: storage has rows, so the service skips the cold-start
// fallback and just hydrates each recommended id via the catalog.
func TestRecommendations_HydratesPersistedTopN(t *testing.T) {
	ctx := context.Background()
	cat := newStubCatalog()
	cat.add(domain.ProductSummary{ID: "seed", CategoryID: "shoes", Name: "Seed"})
	cat.add(domain.ProductSummary{ID: "rec-1", CategoryID: "shoes", Name: "Rec 1"})
	cat.add(domain.ProductSummary{ID: "rec-2", CategoryID: "shoes", Name: "Rec 2"})

	storage := adapter.NewInMemoryStorage()
	if err := storage.UpsertTopN(ctx, "seed", []domain.Recommendation{
		{ProductID: "seed", RecommendedID: "rec-1", Score: 0.9, Position: 0},
		{ProductID: "seed", RecommendedID: "rec-2", Score: 0.7, Position: 1},
	}); err != nil {
		t.Fatal(err)
	}
	weights := adapter.NewInMemoryWeights()
	svc := app.NewService(storage, cat, weights)

	got, err := svc.Recommendations(ctx, "seed")
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(got) != 2 || got[0].ID != "rec-1" || got[1].ID != "rec-2" {
		t.Fatalf("unexpected order/contents: %+v", got)
	}
}

// TestRecommendations_StaleRowsAreDropped verifies that a
// recommendation pointing at a now-deleted product is silently
// skipped rather than failing the call. The storefront expects an
// "always returns something or empty, never errors" contract for
// the section render.
func TestRecommendations_StaleRowsAreDropped(t *testing.T) {
	ctx := context.Background()
	cat := newStubCatalog()
	cat.add(domain.ProductSummary{ID: "seed", CategoryID: "shoes", Name: "Seed"})
	cat.add(domain.ProductSummary{ID: "rec-1", CategoryID: "shoes", Name: "Rec 1"})
	// rec-2 is deliberately NOT in the catalog — simulates a stale
	// row left behind by a deleted product.

	storage := adapter.NewInMemoryStorage()
	_ = storage.UpsertTopN(ctx, "seed", []domain.Recommendation{
		{ProductID: "seed", RecommendedID: "rec-1", Position: 0},
		{ProductID: "seed", RecommendedID: "rec-2", Position: 1},
	})
	svc := app.NewService(storage, cat, adapter.NewInMemoryWeights())
	got, err := svc.Recommendations(ctx, "seed")
	if err != nil {
		t.Fatalf("Recommendations: %v", err)
	}
	if len(got) != 1 || got[0].ID != "rec-1" {
		t.Fatalf("expected only rec-1 (rec-2 should be filtered), got %+v", got)
	}
}

// TestSetWeights_RejectsNegative wires SetWeights through the
// service's validation and asserts the domain error sentinel
// reaches the caller — the admin handler relies on it to render a
// "negative weights aren't allowed" flash.
func TestSetWeights_RejectsNegative(t *testing.T) {
	svc := app.NewService(adapter.NewInMemoryStorage(), newStubCatalog(), adapter.NewInMemoryWeights())
	err := svc.SetWeights(context.Background(), domain.Weights{CoPurchase: -1})
	if !errors.Is(err, domain.ErrInvalidWeights) {
		t.Fatalf("expected ErrInvalidWeights, got %v", err)
	}
}

// TestLastRefresh_ReportsLatestComputedAt verifies the admin
// dashboard's "last refreshed" line gets the most recent
// computed_at across every seed.
func TestLastRefresh_ReportsLatestComputedAt(t *testing.T) {
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	clock := t0
	storage := adapter.NewInMemoryStorage().WithClock(func() time.Time { return clock })
	if err := storage.UpsertTopN(ctx, "a", []domain.Recommendation{{ProductID: "a", RecommendedID: "b"}}); err != nil {
		t.Fatal(err)
	}
	clock = t0.Add(time.Hour)
	if err := storage.UpsertTopN(ctx, "c", []domain.Recommendation{{ProductID: "c", RecommendedID: "d"}}); err != nil {
		t.Fatal(err)
	}
	svc := app.NewService(storage, newStubCatalog(), adapter.NewInMemoryWeights())
	got, ok, err := svc.LastRefresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected a refresh timestamp")
	}
	if !got.Equal(t0.Add(time.Hour)) {
		t.Fatalf("LastRefresh = %v, want %v", got, t0.Add(time.Hour))
	}
}
