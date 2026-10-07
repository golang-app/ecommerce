package domain_test

import (
	"errors"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
)

func TestNewCategory_WithParentID(t *testing.T) {
	c, err := domain.NewCategory("cat-1", "Shirts", "shirts", 1, "cat-parent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.ID() != "cat-1" {
		t.Errorf("expected ID 'cat-1', got %q", c.ID())
	}
	if c.Name() != "Shirts" {
		t.Errorf("expected Name 'Shirts', got %q", c.Name())
	}
	if c.Slug() != "shirts" {
		t.Errorf("expected Slug 'shirts', got %q", c.Slug())
	}
	if c.Position() != 1 {
		t.Errorf("expected Position 1, got %d", c.Position())
	}
	if c.ParentID() != "cat-parent" {
		t.Errorf("expected ParentID 'cat-parent', got %q", c.ParentID())
	}
	if c.IsRoot() {
		t.Errorf("expected IsRoot to be false for category with parent")
	}
}

func TestNewCategory_RootCategory(t *testing.T) {
	c, err := domain.NewCategory("cat-root", "Clothing", "clothing", 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.ParentID() != "" {
		t.Errorf("expected empty ParentID, got %q", c.ParentID())
	}
	if !c.IsRoot() {
		t.Errorf("expected IsRoot to be true for root category")
	}
}

func TestNewCategory_SelfParentingRejected(t *testing.T) {
	_, err := domain.NewCategory("cat-1", "Clothing", "clothing", 0, "cat-1")
	if !errors.Is(err, domain.ErrInvalidCategory) {
		t.Errorf("expected ErrInvalidCategory when parentID == id, got %v", err)
	}
}

func TestRebuildCategory_WithParentID(t *testing.T) {
	c := domain.RebuildCategory("cat-1", "Shirts", "shirts", 1, "cat-parent")
	if c.ParentID() != "cat-parent" {
		t.Errorf("expected ParentID 'cat-parent', got %q", c.ParentID())
	}
	if c.IsRoot() {
		t.Errorf("expected IsRoot to be false")
	}
}

func TestDomainErrors(t *testing.T) {
	if domain.ErrCategoryHasChildren == nil {
		t.Errorf("expected ErrCategoryHasChildren to be defined")
	}
	if domain.ErrCyclicCategoryHierarchy == nil {
		t.Errorf("expected ErrCyclicCategoryHierarchy to be defined")
	}
}
