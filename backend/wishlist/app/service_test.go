package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/wishlist/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/wishlist/app"
)

// newServiceFor builds a Service backed by the in-memory adapter with a
// pinned clock so addedAt values are deterministic.
func newServiceFor(t *testing.T) (*app.Service, *adapter.InMemory) {
	t.Helper()
	storage := adapter.NewInMemory()
	srv := app.NewService(storage).
		WithClock(func() time.Time { return time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC) })
	return srv, storage
}

// TestToggle_FlipsState is the behavioural contract the product-page heart
// button relies on: clicking on an absent variant adds it (and reports
// "now present"); clicking again removes it (and reports "now absent").
// The Storage view must agree with each reported state.
func TestToggle_FlipsState(t *testing.T) {
	srv, storage := newServiceFor(t)
	ctx := context.Background()

	added, err := srv.Toggle(ctx, "alice@example.com", "var-1")
	if err != nil {
		t.Fatalf("first Toggle: unexpected err: %v", err)
	}
	if !added {
		t.Fatalf("first Toggle: added = false, want true (item should have been added)")
	}

	present, err := storage.Contains(ctx, "alice@example.com", "var-1")
	if err != nil {
		t.Fatalf("Contains after add: %v", err)
	}
	if !present {
		t.Fatalf("Contains after add = false, want true")
	}

	added, err = srv.Toggle(ctx, "alice@example.com", "var-1")
	if err != nil {
		t.Fatalf("second Toggle: unexpected err: %v", err)
	}
	if added {
		t.Fatalf("second Toggle: added = true, want false (item should have been removed)")
	}

	present, err = storage.Contains(ctx, "alice@example.com", "var-1")
	if err != nil {
		t.Fatalf("Contains after remove: %v", err)
	}
	if present {
		t.Fatalf("Contains after remove = true, want false")
	}
}

// TestAdd_IsIdempotent guards the postgres ON CONFLICT DO NOTHING contract
// at the service level: a double-click on the heart button must not
// produce duplicates and must not error.
func TestAdd_IsIdempotent(t *testing.T) {
	srv, storage := newServiceFor(t)
	ctx := context.Background()

	if err := srv.Add(ctx, "alice@example.com", "var-1"); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := srv.Add(ctx, "alice@example.com", "var-1"); err != nil {
		t.Fatalf("second Add (idempotent): %v", err)
	}

	list, err := storage.ListByCustomer(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("ListByCustomer: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly one wishlist item; got %d", len(list))
	}
}

// TestListByCustomer_NewestFirst pins the ordering contract — the account
// page renders the list straight from the service so the storage adapter's
// sort matters.
func TestListByCustomer_NewestFirst(t *testing.T) {
	storage := adapter.NewInMemory()
	srv := app.NewService(storage)

	ctx := context.Background()
	srv = srv.WithClock(func() time.Time { return time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC) })
	if err := srv.Add(ctx, "alice@example.com", "var-old"); err != nil {
		t.Fatalf("Add old: %v", err)
	}
	srv = srv.WithClock(func() time.Time { return time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC) })
	if err := srv.Add(ctx, "alice@example.com", "var-new"); err != nil {
		t.Fatalf("Add new: %v", err)
	}

	list, err := srv.ListByCustomer(ctx, "alice@example.com")
	if err != nil {
		t.Fatalf("ListByCustomer: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 items; got %d", len(list))
	}
	if list[0].VariantID() != "var-new" || list[1].VariantID() != "var-old" {
		t.Fatalf("expected newest first; got %s then %s", list[0].VariantID(), list[1].VariantID())
	}
}
