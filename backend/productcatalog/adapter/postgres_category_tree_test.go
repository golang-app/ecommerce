package adapter_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	_ "github.com/lib/pq"
)

func testPostgresDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping postgres integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	return db
}

func TestPostgres_CategoryHierarchyAndCTE(t *testing.T) {
	db := testPostgresDB(t)
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	storage := adapter.NewPostgres(db)

	root, err := domain.NewCategory("test-cat-root", "Electronics", "test-electronics", 1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sub, err := domain.NewCategory("test-cat-sub", "Phones", "test-phones", 1, "test-cat-root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_ = storage.DeleteCategory(ctx, sub.ID())
	_ = storage.DeleteCategory(ctx, root.ID())

	if err := storage.CreateCategory(ctx, root); err != nil {
		t.Fatalf("failed to create root: %v", err)
	}
	defer func() { _ = storage.DeleteCategory(ctx, root.ID()) }()

	if err := storage.CreateCategory(ctx, sub); err != nil {
		t.Fatalf("failed to create sub: %v", err)
	}
	defer func() { _ = storage.DeleteCategory(ctx, sub.ID()) }()

	hasChildren, err := storage.HasChildCategories(ctx, root.ID())
	if err != nil || !hasChildren {
		t.Errorf("expected root to have children, got %v (err=%v)", hasChildren, err)
	}

	descendants, err := storage.DescendantCategoryIDs(ctx, root.ID())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(descendants) < 2 {
		t.Errorf("expected at least 2 descendants, got %v", descendants)
	}

	target, crumbs, err := storage.CategoryByPath(ctx, "test-electronics/test-phones")
	if err != nil {
		t.Errorf("unexpected error finding by path: %v", err)
	}
	if target.ID() != sub.ID() {
		t.Errorf("expected target ID %q, got %q", sub.ID(), target.ID())
	}
	if len(crumbs) != 2 {
		t.Errorf("expected 2 breadcrumbs, got %d", len(crumbs))
	}
}
