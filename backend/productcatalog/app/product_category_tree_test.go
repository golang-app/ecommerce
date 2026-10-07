//go:build !integration

package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

func TestProductService_CyclePrevention(t *testing.T) {
	ctx := context.Background()
	store := adapter.NewInMemory()
	svc := app.NewProductService(store)

	_ = svc.CreateCategory(ctx, "Clothing", "clothing", "")
	cats, _ := svc.Categories(ctx)
	clothingID := cats[0].ID()

	_ = svc.CreateCategory(ctx, "Men", "men", clothingID)
	cats, _ = svc.Categories(ctx)
	var menID string
	for _, c := range cats {
		if c.Slug() == "men" {
			menID = c.ID()
		}
	}

	// Attempt to set Men as parent of Clothing (direct cycle: Clothing -> Men -> Clothing)
	err := svc.UpdateCategory(ctx, clothingID, "Clothing", "clothing", menID, 1)
	if !errors.Is(err, domain.ErrCyclicCategoryHierarchy) {
		t.Fatalf("expected ErrCyclicCategoryHierarchy, got %v", err)
	}
}

func TestProductService_SafeDeletion(t *testing.T) {
	ctx := context.Background()
	store := adapter.NewInMemory()
	svc := app.NewProductService(store)

	_ = svc.CreateCategory(ctx, "Electronics", "electronics", "")
	cats, _ := svc.Categories(ctx)
	rootID := cats[0].ID()

	_ = svc.CreateCategory(ctx, "Laptops", "laptops", rootID)

	// Attempt to delete Electronics while Laptops exists
	err := svc.DeleteCategory(ctx, rootID)
	if !errors.Is(err, domain.ErrCategoryHasChildren) {
		t.Fatalf("expected ErrCategoryHasChildren when deleting parent, got %v", err)
	}
}

func TestProductService_ListCascadesSubcategoryProducts(t *testing.T) {
	ctx := context.Background()
	store := adapter.NewInMemory()
	svc := app.NewProductService(store)

	_ = svc.CreateCategory(ctx, "Clothing", "clothing", "")
	cats, _ := svc.Categories(ctx)
	rootID := cats[0].ID()

	_ = svc.CreateCategory(ctx, "Shirts", "shirts", rootID)
	cats, _ = svc.Categories(ctx)
	var shirtsCat domain.Category
	for _, c := range cats {
		if c.Slug() == "shirts" {
			shirtsCat = c
		}
	}

	p, _ := domain.NewProduct(domain.ProductID("prod-1"), "Oxford Shirt", "desc", domain.MustNewPrice(5000, domain.MustNewCurrency("USD")), "thumb")
	_ = store.Add(ctx, p)
	store.SeedProductCategories("prod-1", shirtsCat)

	// Query by parent slug "clothing"
	res, err := svc.List(ctx, app.ProductQuery{CategorySlug: "clothing"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 1 || res[0].ID() != "prod-1" {
		t.Errorf("expected product 'prod-1' in subcategory to appear under parent 'clothing', got %v", res)
	}
}
