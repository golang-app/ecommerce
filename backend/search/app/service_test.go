package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/search/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/search/app"
	"github.com/bkielbasa/go-ecommerce/backend/search/domain"
)

// newServiceWithSeed builds a service backed by the in-memory adapter and
// seeds two product-kind documents plus one of an experimental "blog" kind
// so the Kinds filter has something to discriminate against. Using a
// string literal for the blog kind (instead of a typed constant) is
// deliberate — the OHS pattern says producers add their own kinds without
// the search package changing.
func newServiceWithSeed(t *testing.T) *app.Service {
	t.Helper()
	storage := adapter.NewInMemory()
	srv := app.NewService(storage)

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	docs := []domain.Document{
		mustDoc(t, domain.KindProduct, "p-mug", "Ceramic Mug", "speckled stoneware mug", "/product/p-mug", []string{"kitchen"}, nil, now),
		mustDoc(t, domain.KindProduct, "p-spoon", "Walnut Spoon", "hand-carved wooden spoon", "/product/p-spoon", []string{"kitchen"}, nil, now),
		mustDoc(t, domain.Kind("blog"), "b-1", "Spoon Care Guide", "how to oil your wooden spoon", "/blog/b-1", nil, nil, now),
	}
	for _, d := range docs {
		if err := srv.Index(context.Background(), d); err != nil {
			t.Fatalf("seed Index(%s/%s): %v", d.Kind(), d.ID(), err)
		}
	}
	return srv
}

func mustDoc(t *testing.T, kind domain.Kind, id, title, body, url string, tags []string, meta map[string]string, updatedAt time.Time) domain.Document {
	t.Helper()
	d, err := domain.NewDocument(kind, id, title, body, url, tags, meta, updatedAt)
	if err != nil {
		t.Fatalf("NewDocument(%s/%s): %v", kind, id, err)
	}
	return d
}

func TestSearch_HitsIndexedDocument(t *testing.T) {
	srv := newServiceWithSeed(t)

	hits, err := srv.Search(context.Background(), "mug", app.QueryOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("Search(mug): expected 1 hit, got %d (%+v)", len(hits), hits)
	}
	if got := hits[0].Document.ID(); got != "p-mug" {
		t.Errorf("Search(mug): hit id = %q, want %q", got, "p-mug")
	}
}

func TestSearch_KindsFilterRestrictsResults(t *testing.T) {
	srv := newServiceWithSeed(t)

	// "spoon" matches BOTH the product (Walnut Spoon) and the blog post
	// (Spoon Care Guide). Filtering by KindProduct must drop the blog hit.
	all, err := srv.Search(context.Background(), "spoon", app.QueryOptions{})
	if err != nil {
		t.Fatalf("Search(spoon, all): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("Search(spoon, all): expected 2 hits, got %d", len(all))
	}

	products, err := srv.Search(context.Background(), "spoon", app.QueryOptions{Kinds: []domain.Kind{domain.KindProduct}})
	if err != nil {
		t.Fatalf("Search(spoon, products): %v", err)
	}
	if len(products) != 1 {
		t.Fatalf("Search(spoon, products): expected 1 hit, got %d", len(products))
	}
	if got := products[0].Document.Kind(); got != domain.KindProduct {
		t.Errorf("Search(spoon, products): hit kind = %q, want %q", got, domain.KindProduct)
	}
}

func TestRemove_RemovesFromSearch(t *testing.T) {
	srv := newServiceWithSeed(t)

	if err := srv.Remove(context.Background(), domain.KindProduct, "p-mug"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	hits, err := srv.Search(context.Background(), "mug", app.QueryOptions{})
	if err != nil {
		t.Fatalf("Search after Remove: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected zero hits after Remove, got %d (%+v)", len(hits), hits)
	}
}

func TestRemoveAllOfKind_ClearsKindButLeavesOthers(t *testing.T) {
	srv := newServiceWithSeed(t)

	if err := srv.RemoveAllOfKind(context.Background(), domain.KindProduct); err != nil {
		t.Fatalf("RemoveAllOfKind: %v", err)
	}

	// Products should be gone, the blog post should remain.
	productHits, err := srv.Search(context.Background(), "spoon", app.QueryOptions{Kinds: []domain.Kind{domain.KindProduct}})
	if err != nil {
		t.Fatalf("Search products: %v", err)
	}
	if len(productHits) != 0 {
		t.Errorf("expected 0 product hits after RemoveAllOfKind, got %d", len(productHits))
	}

	blogHits, err := srv.Search(context.Background(), "spoon", app.QueryOptions{Kinds: []domain.Kind{domain.Kind("blog")}})
	if err != nil {
		t.Fatalf("Search blog: %v", err)
	}
	if len(blogHits) != 1 {
		t.Errorf("expected 1 blog hit to survive, got %d", len(blogHits))
	}
}

func TestSearch_EmptyQueryReturnsNoHits(t *testing.T) {
	srv := newServiceWithSeed(t)

	hits, err := srv.Search(context.Background(), "   ", app.QueryOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected empty result for whitespace query, got %d (%+v)", len(hits), hits)
	}
}
