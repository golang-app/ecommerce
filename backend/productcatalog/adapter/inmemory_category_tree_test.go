package adapter_test

import (
	"context"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

func TestInMemory_CategoryHierarchy(t *testing.T) {
	ctx := context.Background()
	storage := adapter.NewInMemory()

	clothing := domain.RebuildCategory("cat-clothing", "Clothing", "clothing", 1, "")
	men := domain.RebuildCategory("cat-men", "Men", "men", 1, "cat-clothing")
	shirts := domain.RebuildCategory("cat-shirts", "Shirts", "shirts", 1, "cat-men")

	_ = storage.CreateCategory(ctx, clothing)
	_ = storage.CreateCategory(ctx, men)
	_ = storage.CreateCategory(ctx, shirts)

	// Test HasChildCategories
	hasChildren, err := storage.HasChildCategories(ctx, "cat-clothing")
	if err != nil || !hasChildren {
		t.Fatalf("expected cat-clothing to have children, got %v, err=%v", hasChildren, err)
	}

	hasChildren, err = storage.HasChildCategories(ctx, "cat-shirts")
	if err != nil || hasChildren {
		t.Fatalf("expected cat-shirts to have NO children, got %v, err=%v", hasChildren, err)
	}

	// Test DescendantCategoryIDs
	descendants, err := storage.DescendantCategoryIDs(ctx, "cat-clothing")
	if err != nil {
		t.Fatalf("unexpected error getting descendants: %v", err)
	}
	expectedDescendants := map[string]bool{"cat-clothing": true, "cat-men": true, "cat-shirts": true}
	if len(descendants) != 3 {
		t.Fatalf("expected 3 descendants, got %d (%v)", len(descendants), descendants)
	}
	for _, id := range descendants {
		if !expectedDescendants[id] {
			t.Errorf("unexpected descendant ID: %s", id)
		}
	}

	// Test CategoryByPath
	target, breadcrumbs, err := storage.CategoryByPath(ctx, "clothing/men/shirts")
	if err != nil {
		t.Fatalf("unexpected error resolving path: %v", err)
	}
	if target.ID() != "cat-shirts" {
		t.Errorf("expected target ID 'cat-shirts', got %q", target.ID())
	}
	if len(breadcrumbs) != 3 {
		t.Fatalf("expected 3 breadcrumbs, got %d", len(breadcrumbs))
	}
	if breadcrumbs[0].Slug() != "clothing" || breadcrumbs[1].Slug() != "men" || breadcrumbs[2].Slug() != "shirts" {
		t.Errorf("unexpected breadcrumbs chain: %+v", breadcrumbs)
	}
}
