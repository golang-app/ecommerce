package app_test

import (
	"context"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

func TestProductService_ProductImages(t *testing.T) {
	ctx := context.Background()
	repo := adapter.NewInMemory()
	svc := app.NewProductService(repo)

	// Try adding images to non-existent product
	_, err := svc.AddProductImages(ctx, "non-existent", []string{"http://example.com/1.jpg"})
	if err == nil {
		t.Error("expected error for non-existent product, got nil")
	}

	// Create a product without thumbnail
	pid, _ := domain.NewProductId("p-gallery-test")
	price, _ := domain.NewPrice(2500, "USD")
	prod, _ := domain.NewProduct(pid, "Gallery Product", "Description", price, "")
	if err := repo.Add(ctx, prod); err != nil {
		t.Fatalf("failed to add product: %v", err)
	}

	// First batch of images: first image should be PRIMARY, second GALLERY
	imgs, err := svc.AddProductImages(ctx, "p-gallery-test", []string{
		"http://example.com/first.jpg",
		"http://example.com/second.jpg",
	})
	if err != nil {
		t.Fatalf("unexpected error adding images: %v", err)
	}
	if len(imgs) != 2 {
		t.Fatalf("expected 2 images, got %d", len(imgs))
	}
	if imgs[0].Position() != domain.ImagePositionPrimary {
		t.Errorf("expected first image to be PRIMARY, got %s", imgs[0].Position())
	}
	if imgs[1].Position() != domain.ImagePositionGallery {
		t.Errorf("expected second image to be GALLERY, got %s", imgs[1].Position())
	}

	// Product thumbnail should be synced with first image
	p, err := svc.Find(ctx, "p-gallery-test")
	if err != nil {
		t.Fatalf("unexpected error finding product: %v", err)
	}
	if p.Thumbnail() != "http://example.com/first.jpg" {
		t.Errorf("expected thumbnail to be 'http://example.com/first.jpg', got %s", p.Thumbnail())
	}

	// Second batch: since product already has images, new uploads should be GALLERY
	imgs2, err := svc.AddProductImages(ctx, "p-gallery-test", []string{"http://example.com/third.jpg"})
	if err != nil {
		t.Fatalf("unexpected error adding image: %v", err)
	}
	if imgs2[0].Position() != domain.ImagePositionGallery {
		t.Errorf("expected third image to be GALLERY, got %s", imgs2[0].Position())
	}

	// Promote third image to primary
	if err := svc.SetPrimaryProductImage(ctx, "p-gallery-test", imgs2[0].ID()); err != nil {
		t.Fatalf("unexpected error promoting image: %v", err)
	}
	p, _ = svc.Find(ctx, "p-gallery-test")
	if p.Thumbnail() != "http://example.com/third.jpg" {
		t.Errorf("expected thumbnail to update to 'http://example.com/third.jpg', got %s", p.Thumbnail())
	}

	// Delete third image (current primary)
	if err := svc.DeleteProductImage(ctx, "p-gallery-test", imgs2[0].ID()); err != nil {
		t.Fatalf("unexpected error deleting image: %v", err)
	}
	list, err := svc.ProductImages(ctx, "p-gallery-test")
	if err != nil {
		t.Fatalf("unexpected error listing images: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 images remaining, got %d", len(list))
	}
}
