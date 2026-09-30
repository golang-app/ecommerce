package domain_test

import (
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

func TestNewProductImage_Validation(t *testing.T) {
	_, err := domain.NewProductImage("", "prod-1", "http://example.com/img.jpg", domain.ImagePositionGallery)
	if err == nil {
		t.Error("expected error for empty id, got nil")
	}

	_, err = domain.NewProductImage("img-1", "", "http://example.com/img.jpg", domain.ImagePositionGallery)
	if err == nil {
		t.Error("expected error for empty productID, got nil")
	}

	_, err = domain.NewProductImage("img-1", "prod-1", "", domain.ImagePositionGallery)
	if err == nil {
		t.Error("expected error for empty url, got nil")
	}

	_, err = domain.NewProductImage("img-1", "prod-1", "http://example.com/img.jpg", domain.ImagePosition("INVALID"))
	if err == nil {
		t.Error("expected error for invalid position, got nil")
	}

	img, err := domain.NewProductImage("img-1", "prod-1", "http://example.com/img.jpg", domain.ImagePositionPrimary)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if img.ID() != "img-1" {
		t.Errorf("expected ID 'img-1', got %s", img.ID())
	}
	if img.ProductID() != "prod-1" {
		t.Errorf("expected ProductID 'prod-1', got %s", img.ProductID())
	}
	if img.URL() != "http://example.com/img.jpg" {
		t.Errorf("expected URL 'http://example.com/img.jpg', got %s", img.URL())
	}
	if img.Position() != domain.ImagePositionPrimary {
		t.Errorf("expected Position %s, got %s", domain.ImagePositionPrimary, img.Position())
	}
	if !img.IsPrimary() {
		t.Error("expected IsPrimary to be true")
	}
}

func TestProduct_ImagesAndPrimary(t *testing.T) {
	pID, err := domain.NewProductId("p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	price, err := domain.NewPrice(1000, "USD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	prod, err := domain.NewProduct(pID, "Test", "Desc", price, "http://example.com/thumb.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// When no images, PrimaryImage falls back to thumbnail
	if prod.PrimaryImage() != "http://example.com/thumb.jpg" {
		t.Errorf("expected fallback to thumbnail, got %s", prod.PrimaryImage())
	}

	img1, _ := domain.NewProductImage("img-1", "p1", "http://example.com/gallery.jpg", domain.ImagePositionGallery)
	img2, _ := domain.NewProductImage("img-2", "p1", "http://example.com/primary.jpg", domain.ImagePositionPrimary)

	prod = prod.WithImages([]domain.ProductImage{img1, img2})
	if len(prod.Images()) != 2 {
		t.Errorf("expected 2 images, got %d", len(prod.Images()))
	}
	if prod.PrimaryImage() != "http://example.com/primary.jpg" {
		t.Errorf("expected primary image URL, got %s", prod.PrimaryImage())
	}
}
