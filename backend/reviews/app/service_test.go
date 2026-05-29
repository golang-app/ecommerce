package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/reviews/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/reviews/app"
	"github.com/bkielbasa/go-ecommerce/backend/reviews/domain"
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

	// ByProduct now filters to approved-only (storefront semantics) and a
	// fresh Submit lands as pending, so the storefront query is empty.
	visible, err := storage.ByProduct(ctx, "prod-1", 10)
	if err != nil {
		t.Fatalf("ByProduct: %v", err)
	}
	if len(visible) != 0 {
		t.Fatalf("expected 0 approved reviews; got %d", len(visible))
	}

	// The pending list is the source of truth for "did the row get stored".
	pending, err := storage.ListByStatus(ctx, domain.StatusPending, 10)
	if err != nil {
		t.Fatalf("ListByStatus: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected exactly one stored pending review; got %d", len(pending))
	}
}

func TestSubmit_AcceptsBuyerAndApprovalRequiredForAggregate(t *testing.T) {
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

	// Both reviews land as pending — invisible to storefront ByProduct.
	visible, err := storage.ByProduct(ctx, "prod-1", 10)
	if err != nil {
		t.Fatalf("ByProduct: %v", err)
	}
	if len(visible) != 0 {
		t.Fatalf("expected 0 approved reviews before moderation; got %d", len(visible))
	}

	// Aggregate is empty too (only approved rows contribute).
	agg, err := srv.AggregateForProducts(ctx, []string{"prod-1"})
	if err != nil {
		t.Fatalf("AggregateForProducts (pre-approval): %v", err)
	}
	if _, ok := agg["prod-1"]; ok {
		t.Fatalf("expected no aggregate row before approval; got %#v", agg)
	}

	// Two pending rows must exist in the moderation queue.
	pending, err := srv.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending reviews; got %d", len(pending))
	}

	// Approve both — now they participate in the storefront view.
	for _, rv := range pending {
		if err := srv.Approve(ctx, rv.ID()); err != nil {
			t.Fatalf("Approve(%s): %v", rv.ID(), err)
		}
	}

	visible, err = storage.ByProduct(ctx, "prod-1", 10)
	if err != nil {
		t.Fatalf("ByProduct (post-approval): %v", err)
	}
	if len(visible) != 2 {
		t.Fatalf("expected 2 approved reviews after approval; got %d", len(visible))
	}

	agg, err = srv.AggregateForProducts(ctx, []string{"prod-1"})
	if err != nil {
		t.Fatalf("AggregateForProducts (post-approval): %v", err)
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

func TestApprove_FlipsVisibilityFromHidden(t *testing.T) {
	srv, _ := newServiceFor(t, stubBuyers{purchases: map[string]bool{
		"alice@example.com|prod-1": true,
	}})

	ctx := context.Background()
	if err := srv.Submit(ctx, "prod-1", "alice@example.com", "great mug", 5); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Pre-approval: storefront sees nothing for prod-1.
	before, err := srv.ListForProduct(ctx, "prod-1", 10)
	if err != nil {
		t.Fatalf("ListForProduct (pre-approval): %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("storefront ListForProduct should hide pending; got %d", len(before))
	}

	pending, err := srv.ListPending(ctx, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("ListPending: err=%v len=%d", err, len(pending))
	}
	if pending[0].Status() != domain.StatusPending {
		t.Fatalf("expected status=pending; got %q", pending[0].Status())
	}

	if err := srv.Approve(ctx, pending[0].ID()); err != nil {
		t.Fatalf("Approve: %v", err)
	}

	after, err := srv.ListForProduct(ctx, "prod-1", 10)
	if err != nil {
		t.Fatalf("ListForProduct (post-approval): %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("storefront ListForProduct should show approved; got %d", len(after))
	}
	if !after[0].IsApproved() {
		t.Fatalf("expected approved row in storefront list; got status %q", after[0].Status())
	}
}

func TestReject_KeepsRowHidden(t *testing.T) {
	srv, _ := newServiceFor(t, stubBuyers{purchases: map[string]bool{
		"alice@example.com|prod-1": true,
	}})

	ctx := context.Background()
	if err := srv.Submit(ctx, "prod-1", "alice@example.com", "spam", 1); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	pending, _ := srv.ListPending(ctx, 10)
	if err := srv.Reject(ctx, pending[0].ID()); err != nil {
		t.Fatalf("Reject: %v", err)
	}

	visible, err := srv.ListForProduct(ctx, "prod-1", 10)
	if err != nil {
		t.Fatalf("ListForProduct: %v", err)
	}
	if len(visible) != 0 {
		t.Fatalf("rejected reviews must stay hidden; got %d", len(visible))
	}

	// ListAll still shows the rejected row for the admin "all" tab.
	all, err := srv.ListAll(ctx, 10)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 1 || all[0].Status() != domain.StatusRejected {
		t.Fatalf("expected one rejected row in ListAll; got %#v", all)
	}
}
