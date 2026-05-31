package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/payments/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/payments/app"
	"github.com/bkielbasa/go-ecommerce/backend/payments/domain"
)

// stubProvider is the simplest possible ACL double: it returns a
// fixed ChargeResult per call (or an error). Used to drive the
// service through both branches without spinning up fakestripe.
type stubProvider struct {
	result app.ChargeResult
	err    error
	calls  int
}

func (s *stubProvider) Charge(_ context.Context, _ app.ChargeRequest) (app.ChargeResult, error) {
	s.calls++
	return s.result, s.err
}

func newService(t *testing.T, p app.Provider) (*app.Service, *adapter.InMemoryStorage) {
	t.Helper()
	store := adapter.NewInMemoryStorage()
	idx := 0
	idGen := func() string {
		idx++
		return "ch-test-" + string(rune('A'+idx-1))
	}
	clock := func() time.Time { return time.Unix(0, 0).UTC() }
	return app.NewService(store, p, idGen, clock), store
}

// Succeeded path: insert pending -> provider returns succeeded -> the
// returned Charge is in the succeeded terminal status with the
// provider's reference attached, and a stored Find returns the same.
func TestCharge_SucceededPathPersistsProviderRef(t *testing.T) {
	provider := &stubProvider{
		result: app.ChargeResult{Status: domain.StatusSucceeded, ProviderRef: "pi_1"},
	}
	srv, store := newService(t, provider)

	got, err := srv.Charge(context.Background(), "ord-1", 1234, "usd", "tok_visa", "key-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status() != domain.StatusSucceeded {
		t.Fatalf("expected succeeded; got %q", got.Status())
	}
	if got.ProviderRef() != "pi_1" {
		t.Fatalf("expected provider ref pi_1; got %q", got.ProviderRef())
	}

	reloaded, err := store.Find(context.Background(), got.ID())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status() != domain.StatusSucceeded {
		t.Fatalf("reload: expected succeeded; got %q", reloaded.Status())
	}
}

// Failed path: provider returns failed -> we persist status=failed.
func TestCharge_FailedPathPersistsStatus(t *testing.T) {
	provider := &stubProvider{
		result: app.ChargeResult{Status: domain.StatusFailed, ProviderRef: "pi_2"},
	}
	srv, _ := newService(t, provider)

	got, err := srv.Charge(context.Background(), "ord-2", 100, "usd", "tok_fail", "key-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status() != domain.StatusFailed {
		t.Fatalf("expected failed; got %q", got.Status())
	}
}

// Idempotency-key reuse: a second Charge call with the SAME key
// returns the existing Charge and does NOT touch the provider.
func TestCharge_IdempotencyKeyReuseReturnsExisting(t *testing.T) {
	provider := &stubProvider{
		result: app.ChargeResult{Status: domain.StatusSucceeded, ProviderRef: "pi_3"},
	}
	srv, _ := newService(t, provider)

	first, err := srv.Charge(context.Background(), "ord-3", 500, "usd", "tok", "key-shared")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := srv.Charge(context.Background(), "ord-3", 500, "usd", "tok", "key-shared")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ID() != second.ID() {
		t.Fatalf("expected same Charge ID; got %q vs %q", first.ID(), second.ID())
	}
	if provider.calls != 1 {
		t.Fatalf("expected provider called once for idempotent retry; got %d", provider.calls)
	}
}

// MarkSucceeded: webhook-driven transition flips a pending Charge
// into succeeded; a second call is a no-op (idempotent).
func TestMarkSucceeded_TransitionsThenNoOps(t *testing.T) {
	provider := &stubProvider{
		// requires_action gets translated to pending by the ACL;
		// here we go straight to pending for the stub.
		result: app.ChargeResult{Status: domain.StatusPending, ProviderRef: "pi_4"},
	}
	srv, store := newService(t, provider)

	charge, err := srv.Charge(context.Background(), "ord-4", 100, "usd", "tok", "key-4")
	if err != nil {
		t.Fatalf("place: %v", err)
	}
	if charge.Status() != domain.StatusPending {
		t.Fatalf("expected pending; got %q", charge.Status())
	}

	if err := srv.MarkSucceeded(context.Background(), "pi_4"); err != nil {
		t.Fatalf("first webhook: %v", err)
	}
	reloaded, _ := store.Find(context.Background(), charge.ID())
	if reloaded.Status() != domain.StatusSucceeded {
		t.Fatalf("expected succeeded after webhook; got %q", reloaded.Status())
	}

	// Second call should be a no-op — and crucially, not return an
	// error. This is the idempotency the webhook handler relies on
	// to tolerate at-least-once redelivery from the provider.
	if err := srv.MarkSucceeded(context.Background(), "pi_4"); err != nil {
		t.Fatalf("second webhook (idempotent): %v", err)
	}
}

// Provider error: the service records the attempt as failed AND
// returns the wrapped error so the caller can roll back its own
// transaction.
func TestCharge_ProviderErrorRecordsFailed(t *testing.T) {
	wantErr := errors.New("network unreachable")
	provider := &stubProvider{err: wantErr}
	srv, store := newService(t, provider)

	got, err := srv.Charge(context.Background(), "ord-5", 100, "usd", "tok", "key-5")
	if err == nil {
		t.Fatalf("expected error; got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped %v; got %v", wantErr, err)
	}
	if got.Status() != domain.StatusFailed {
		t.Fatalf("expected failed; got %q", got.Status())
	}

	// Reload via the storage seam: the row exists and is failed.
	reloaded, ok, ferr := store.FindByIdempotencyKey(context.Background(), "key-5")
	if ferr != nil {
		t.Fatalf("find by key: %v", ferr)
	}
	if !ok {
		t.Fatalf("expected row recorded for failed attempt")
	}
	if reloaded.Status() != domain.StatusFailed {
		t.Fatalf("expected stored row to be failed; got %q", reloaded.Status())
	}
}
