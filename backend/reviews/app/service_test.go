package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/reviews/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/reviews/app"
)

// stubBuyers is a fake VerifiedBuyerChecker driven by a static map of
// (customer, product) -> hasPurchased. It lets each test express the
// verified-buyer state without spinning up the checkout context.
type stubBuyers struct {
	purchases map[string]bool
}

func (s stubBuyers) HasPurchased(_ context.Context, customerID, productID string) (bool, error) {
	return s.purchases[customerID+"|"+productID], nil
}

func newServiceFor(t *testing.T, buyers stubBuyers) (*app.Service, *adapter.InMemory) {
	t.Helper()
	storage := adapter.NewInMemory()
	srv := app.NewService(storage, buyers).
		WithClock(func() time.Time { return time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC) })
	// Deterministic ids so we can spot duplicates by hand if a future
	// regression starts using the id in equality checks.
	var n int
	srv = srv.WithIDGenerator(func() string {
		n++
		return "rev-test-" + time.Now().Format("150405") + "-" + string(rune('a'+n-1))
	})
	return srv, storage
}

func TestSubmit_RejectsNonBuyer(t *testing.T) {
	srv, storage := newServiceFor(t, stubBuyers{purchases: map[string]bool{}})

	err := srv.Submit(context.Background(), "prod-1", "alice@example.com", "great mug", 5)
	if !errors.Is(err, app.ErrNotVerifiedBuyer) {
		t.Fatalf("Submit(non-buyer) err = %v, want ErrNotVerifiedBuyer", err)
	}

	// Storage must remain empty — a rejected submission MUST NOT have
	// written a review row.
	list, err := storage.ByProduct(context.Background(), "prod-1", 10)
	if err != nil {
		t.Fatalf("ByProduct: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no reviews in storage after rejected submit; got %d", len(list))
	}
}

func TestSubmit_RejectsDuplicate(t *testing.T) {
	srv, storage := newServiceFor(t, stubBuyers{purchases: map[string]bool{
		"alice@example.com|prod-1": true,
	}})

	ctx := context.Background()
	if err := srv.Submit(ctx, "prod-1", "alice@example.com", "first review", 4); err != nil {
		t.Fatalf("first Submit: unexpected err: %v", err)
	}
	if err := srv.Submit(ctx, "prod-1", "alice@example.com", "second review", 5); !errors.Is(err, app.ErrDuplicateReview) {
		t.Fatalf("second Submit: err = %v, want ErrDuplicateReview", err)
	}

	list, err := storage.ByProduct(ctx, "prod-1", 10)
	if err != nil {
		t.Fatalf("ByProduct: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected exactly one stored review; got %d", len(list))
	}
}

func TestSubmit_AcceptsBuyerAndComputesAggregate(t *testing.T) {
	srv, storage := newServiceFor(t, stubBuyers{purchases: map[string]bool{
		"alice@example.com|prod-1": true,
		"bob@example.com|prod-1":   true,
	}})

	ctx := context.Background()
	if err := srv.Submit(ctx, "prod-1", "alice@example.com", "alice loved it", 5); err != nil {
		t.Fatalf("Submit(alice): %v", err)
	}
	if err := srv.Submit(ctx, "prod-1", "bob@example.com", "bob was meh", 3); err != nil {
		t.Fatalf("Submit(bob): %v", err)
	}

	list, err := storage.ByProduct(ctx, "prod-1", 10)
	if err != nil {
		t.Fatalf("ByProduct: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 reviews; got %d", len(list))
	}

	agg, err := srv.AggregateForProducts(ctx, []string{"prod-1"})
	if err != nil {
		t.Fatalf("AggregateForProducts: %v", err)
	}
	got, ok := agg["prod-1"]
	if !ok {
		t.Fatalf("expected aggregate for prod-1; got %#v", agg)
	}
	if got.Count() != 2 {
		t.Errorf("aggregate Count = %d, want 2", got.Count())
	}
	const wantAvg = 4.0
	if got.Average() != wantAvg {
		t.Errorf("aggregate Average = %v, want %v", got.Average(), wantAvg)
	}
}
