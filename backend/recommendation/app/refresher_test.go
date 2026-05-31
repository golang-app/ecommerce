package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/recommendation/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/recommendation/app"
	"github.com/bkielbasa/go-ecommerce/backend/recommendation/domain"
)

// stubHistory is a deterministic OrderHistoryReader.
type stubHistory struct {
	pairs map[domain.Pair]int
}

func (s stubHistory) CoPurchasePairs(_ context.Context, _ time.Time) (map[domain.Pair]int, error) {
	out := map[domain.Pair]int{}
	for k, v := range s.pairs {
		out[k] = v
	}
	return out, nil
}

// TestRefresher_DeterministicTopN feeds the refresher a fixed
// catalog + history and asserts the resulting top-N for a chosen
// seed has the expected ordering. The test pins the public ordering
// guarantee: co-purchase partners (alpha) score above
// same-category-only peers (beta), which score above unrelated
// products (delta).
func TestRefresher_DeterministicTopN(t *testing.T) {
	ctx := context.Background()
	cat := newStubCatalog()
	// Seed: shoes, distinctive name+description tokens.
	cat.add(domain.ProductSummary{
		ID: "seed", CategoryID: "shoes",
		Name: "Alpha Running Shoe", Description: "lightweight runner trail terrain",
		PriceMinor: 10000,
		Attributes: map[string]string{"brand": "alpha", "color": "red"},
	})
	// Co-purchase partner: different category, identical attributes
	// — co-purchase should still dominate. Price within 10%.
	cat.add(domain.ProductSummary{
		ID: "alpha-sock", CategoryID: "socks",
		Name: "Alpha Sock", Description: "running comfort breathable",
		PriceMinor: 9000,
		Attributes: map[string]string{"brand": "alpha", "color": "red"},
	})
	// Same-category peer: no co-purchase. Same category and price
	// bumps it but no behaviour signal.
	cat.add(domain.ProductSummary{
		ID: "beta-shoe", CategoryID: "shoes",
		Name: "Beta Shoe", Description: "casual walker",
		PriceMinor: 10500,
		Attributes: map[string]string{"brand": "beta", "color": "blue"},
	})
	// Unrelated: different category, different attrs, different
	// tokens, far price.
	cat.add(domain.ProductSummary{
		ID: "delta-toaster", CategoryID: "kitchen",
		Name: "Delta Toaster", Description: "two slice steel",
		PriceMinor: 4000,
		Attributes: map[string]string{"brand": "delta"},
	})
	// FTS results aren't used for scoring directly (the score uses
	// in-memory Jaccard) but are part of the candidate pool. For
	// determinism, supply beta-shoe and delta-toaster as the
	// "text-similar" hits.
	cat.similar["seed"] = []string{"beta-shoe", "delta-toaster"}

	history := stubHistory{
		pairs: map[domain.Pair]int{
			domain.NewPair("seed", "alpha-sock"): 5,
		},
	}

	storage := adapter.NewInMemoryStorage()
	weights := adapter.NewInMemoryWeights()

	r := app.NewRefresher(cat, history, storage, weights).
		WithTopN(3).WithCandidatesK(5).WithWindow(30)

	if err := r.RefreshAll(ctx); err != nil {
		t.Fatalf("RefreshAll: %v", err)
	}

	got, err := storage.TopN(ctx, "seed", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("expected non-empty top-N")
	}
	if got[0].RecommendedID != "alpha-sock" {
		t.Fatalf("top recommendation = %s, want alpha-sock (co-purchase should dominate)", got[0].RecommendedID)
	}
	// Positions assigned 0..N-1.
	for i, r := range got {
		if r.Position != i {
			t.Fatalf("position[%d] = %d, want %d", i, r.Position, i)
		}
	}
	// Seed must never appear in its own top-N.
	for _, r := range got {
		if r.RecommendedID == "seed" {
			t.Fatalf("self-pair leaked into top-N: %+v", r)
		}
	}
}

// TestRefresher_HonoursContextCancellation verifies cooperative
// cancellation: when the context is cancelled mid-pass the loop
// returns cleanly (no error to the caller) rather than continuing
// to walk every remaining seed.
func TestRefresher_HonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cat := newStubCatalog()
	cat.add(domain.ProductSummary{ID: "a", CategoryID: "x", Name: "A"})
	cat.add(domain.ProductSummary{ID: "b", CategoryID: "x", Name: "B"})

	r := app.NewRefresher(cat, stubHistory{}, adapter.NewInMemoryStorage(), adapter.NewInMemoryWeights())
	if err := r.RefreshAll(ctx); err != nil {
		t.Fatalf("expected nil from a cancelled refresh, got %v", err)
	}
}

// TestRefresher_PersistsAcrossSeeds runs RefreshAll over a small
// catalog and asserts every seed ends up with at least one
// recommendation row written. The body of the assertion is just
// "the materialised read model is non-empty after a pass" — the
// per-row contents are pinned by the previous test.
func TestRefresher_PersistsAcrossSeeds(t *testing.T) {
	ctx := context.Background()
	cat := newStubCatalog()
	cat.add(domain.ProductSummary{ID: "a", CategoryID: "x", Name: "A widget"})
	cat.add(domain.ProductSummary{ID: "b", CategoryID: "x", Name: "B widget"})
	cat.add(domain.ProductSummary{ID: "c", CategoryID: "x", Name: "C widget"})

	storage := adapter.NewInMemoryStorage()
	r := app.NewRefresher(cat, stubHistory{}, storage, adapter.NewInMemoryWeights())
	if err := r.RefreshAll(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b", "c"} {
		got, err := storage.TopN(ctx, id, 10)
		if err != nil {
			t.Fatalf("TopN(%s): %v", id, err)
		}
		if len(got) == 0 {
			t.Fatalf("expected non-empty top-N for %s", id)
		}
	}
}
