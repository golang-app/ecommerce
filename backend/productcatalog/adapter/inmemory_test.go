package adapter_test

import (
	"context"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

func TestInMemory_ProductImages(t *testing.T) {
	ctx := context.Background()
	repo := adapter.NewInMemory()

	pid, err := domain.NewProductId("p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	price, err := domain.NewPrice(1000, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p, err := domain.NewProduct(pid, "Test Product", "Desc", price, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := repo.Add(ctx, p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	img1, err := domain.NewProductImage("img-1", "p1", "http://example.com/1.jpg", domain.ImagePositionPrimary)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	img2, err := domain.NewProductImage("img-2", "p1", "http://example.com/2.jpg", domain.ImagePositionGallery)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := repo.AddProductImages(ctx, []domain.ProductImage{img1, img2}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify images query
	imgs, err := repo.ProductImages(ctx, "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(imgs) != 2 {
		t.Fatalf("expected 2 images, got %d", len(imgs))
	}
	if !imgs[0].IsPrimary() {
		t.Errorf("expected imgs[0] to be primary")
	}

	// Verify product hydration
	prod, err := repo.Find(ctx, "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prod.Images()) != 2 {
		t.Fatalf("expected 2 images on product, got %d", len(prod.Images()))
	}
	if prod.Thumbnail() != "http://example.com/1.jpg" {
		t.Errorf("expected thumbnail 'http://example.com/1.jpg', got %s", prod.Thumbnail())
	}

	// Promote img2 to primary
	if err := repo.SetPrimaryProductImage(ctx, "p1", "img-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	imgs, err = repo.ProductImages(ctx, "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if imgs[0].ID() != "img-2" || !imgs[0].IsPrimary() {
		t.Errorf("expected img-2 to be primary first image, got %+v", imgs[0])
	}

	prod, err = repo.Find(ctx, "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prod.Thumbnail() != "http://example.com/2.jpg" {
		t.Errorf("expected thumbnail 'http://example.com/2.jpg', got %s", prod.Thumbnail())
	}

	// Delete img2 (which is primary), img1 should be promoted to primary
	if err := repo.DeleteProductImage(ctx, "p1", "img-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	imgs, err = repo.ProductImages(ctx, "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(imgs) != 1 {
		t.Fatalf("expected 1 image remaining, got %d", len(imgs))
	}
	if imgs[0].ID() != "img-1" || !imgs[0].IsPrimary() {
		t.Errorf("expected img-1 to be promoted to primary, got %+v", imgs[0])
	}

	prod, err = repo.Find(ctx, "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prod.Thumbnail() != "http://example.com/1.jpg" {
		t.Errorf("expected thumbnail 'http://example.com/1.jpg', got %s", prod.Thumbnail())
	}
}
