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
	if got.OrderID() != "ord-1" {
		t.Fatalf("expected order id ord-1; got %q", got.OrderID())
	}

	reloaded, err := store.Find(context.Background(), got.ID())
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status() != domain.StatusSucceeded {
		t.Fatalf("reload: expected succeeded; got %q", reloaded.Status())
	}
	if reloaded.OrderID() != "ord-1" {
		t.Fatalf("reload: expected order id ord-1; got %q", reloaded.OrderID())
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

func TestService_ProviderManagement(t *testing.T) {
	storage := adapter.NewInMemoryStorage()
	srv := app.NewService(storage, &stubProvider{}, func() string { return "ch-test" }, nil)
	ctx := context.Background()

	// 1. ListProviders returns the 2 default providers
	providers, err := srv.ListProviders(ctx)
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if len(providers) != 2 {
		t.Fatalf("want 2 providers, got %d", len(providers))
	}
	if providers[0].ID() != domain.ProviderFake && providers[0].ID() != domain.ProviderStripe {
		t.Errorf("unexpected provider id: %s", providers[0].ID())
	}

	// 2. FindProvider("stripe") returns stripe config
	stripe, err := srv.FindProvider(ctx, domain.ProviderStripe)
	if err != nil {
		t.Fatalf("FindProvider(stripe): %v", err)
	}
	if stripe.ID() != domain.ProviderStripe {
		t.Errorf("ID = %q, want %q", stripe.ID(), domain.ProviderStripe)
	}
	if !stripe.IsEnabled() {
		t.Errorf("expected stripe to be enabled by default")
	}

	// 3. UpdateProvider modifies enabled and config, and updates updated_at
	initialUpdatedAt := stripe.UpdatedAt()
	updateTime := initialUpdatedAt.Add(5 * time.Minute)
	timeSrv := app.NewService(storage, &stubProvider{}, func() string { return "ch-test" }, func() time.Time { return updateTime })

	newConfig := map[string]string{
		"fail_card_ending_in": "9999",
		"publishable_key":     "pk_test_updated",
	}
	err = timeSrv.UpdateProvider(ctx, domain.ProviderStripe, false, newConfig)
	if err != nil {
		t.Fatalf("UpdateProvider(stripe): %v", err)
	}

	updatedStripe, err := srv.FindProvider(ctx, domain.ProviderStripe)
	if err != nil {
		t.Fatalf("FindProvider after update: %v", err)
	}
	if updatedStripe.IsEnabled() {
		t.Errorf("expected stripe to be disabled after update")
	}
	if got := updatedStripe.ConfigValue("fail_card_ending_in"); got != "9999" {
		t.Errorf("fail_card_ending_in = %q, want 9999", got)
	}
	if got := updatedStripe.ConfigValue("publishable_key"); got != "pk_test_updated" {
		t.Errorf("publishable_key = %q, want pk_test_updated", got)
	}
	if !updatedStripe.UpdatedAt().Equal(updateTime) {
		t.Errorf("updated_at = %v, want %v", updatedStripe.UpdatedAt(), updateTime)
	}

	// 4. UpdateProvider("nonexistent") returns error
	err = srv.UpdateProvider(ctx, "nonexistent", true, map[string]string{})
	if err == nil {
		t.Fatalf("expected error updating nonexistent provider, got nil")
	}
	if !errors.Is(err, app.ErrProviderNotFound) {
		t.Errorf("expected ErrProviderNotFound, got %v", err)
	}
}

func TestService_PendingCharge_Confirm_Reject(t *testing.T) {
	storage := adapter.NewInMemoryStorage()
	srv := app.NewService(storage, &stubProvider{}, func() string { return "ch-pend-1" }, nil)
	ctx := context.Background()

	// 1. CreatePendingCharge creates a charge with status StatusPending, given orderID, amount, currency, and provider
	charge, err := srv.CreatePendingCharge(ctx, "ord-500", 3000, "USD", "fake")
	if err != nil {
		t.Fatalf("CreatePendingCharge: %v", err)
	}
	if charge.ID() != "ch-pend-1" {
		t.Errorf("charge.ID = %q, want ch-pend-1", charge.ID())
	}
	if charge.Status() != domain.StatusPending {
		t.Errorf("status = %v, want pending", charge.Status())
	}
	if charge.OrderID() != "ord-500" {
		t.Errorf("orderID = %v, want ord-500", charge.OrderID())
	}
	if charge.Amount() != 3000 {
		t.Errorf("amount = %d, want 3000", charge.Amount())
	}
	if charge.Currency() != "USD" {
		t.Errorf("currency = %q, want USD", charge.Currency())
	}
	if charge.Provider() != "fake" {
		t.Errorf("provider = %q, want fake", charge.Provider())
	}

	// Verify saved in storage
	stored, err := storage.Find(ctx, charge.ID())
	if err != nil {
		t.Fatalf("storage.Find: %v", err)
	}
	if stored.Status() != domain.StatusPending {
		t.Errorf("stored status = %v, want pending", stored.Status())
	}

	// 2. ConfirmCharge transitions charge to StatusSucceeded and sets provider reference
	confirmed, err := srv.ConfirmCharge(ctx, charge.ID())
	if err != nil {
		t.Fatalf("ConfirmCharge: %v", err)
	}
	if confirmed.Status() != domain.StatusSucceeded {
		t.Errorf("status = %v, want succeeded", confirmed.Status())
	}
	if confirmed.ProviderRef() == "" {
		t.Errorf("expected provider reference to be set")
	}

	// ConfirmCharge is idempotent on second call
	confirmed2, err := srv.ConfirmCharge(ctx, charge.ID())
	if err != nil {
		t.Fatalf("ConfirmCharge (second call): %v", err)
	}
	if confirmed2.Status() != domain.StatusSucceeded {
		t.Errorf("status = %v, want succeeded", confirmed2.Status())
	}
	if confirmed2.ProviderRef() != confirmed.ProviderRef() {
		t.Errorf("providerRef = %q, want %q", confirmed2.ProviderRef(), confirmed.ProviderRef())
	}

	// 3. RejectCharge transitions charge to StatusFailed and sets provider reference with reason
	srv2 := app.NewService(storage, &stubProvider{}, func() string { return "ch-pend-2" }, nil)
	charge2, err := srv2.CreatePendingCharge(ctx, "ord-501", 1500, "USD", "fake")
	if err != nil {
		t.Fatalf("CreatePendingCharge: %v", err)
	}

	rejected, err := srv2.RejectCharge(ctx, charge2.ID(), "insufficient_funds")
	if err != nil {
		t.Fatalf("RejectCharge: %v", err)
	}
	if rejected.Status() != domain.StatusFailed {
		t.Errorf("status = %v, want failed", rejected.Status())
	}
	if rejected.ProviderRef() != "insufficient_funds" {
		t.Errorf("providerRef = %q, want insufficient_funds", rejected.ProviderRef())
	}

	// RejectCharge is idempotent on second call
	rejected2, err := srv2.RejectCharge(ctx, charge2.ID(), "insufficient_funds")
	if err != nil {
		t.Fatalf("RejectCharge (second call): %v", err)
	}
	if rejected2.Status() != domain.StatusFailed {
		t.Errorf("status = %v, want failed", rejected2.Status())
	}

	// 4. ConfirmCharge on a failed charge returns an error
	_, err = srv2.ConfirmCharge(ctx, charge2.ID())
	if err == nil {
		t.Fatalf("expected error confirming failed charge, got nil")
	}

	// RejectCharge on a succeeded charge returns an error
	_, err = srv.RejectCharge(ctx, charge.ID(), "too_late")
	if err == nil {
		t.Fatalf("expected error rejecting succeeded charge, got nil")
	}
}

func TestService_FindByOrderID(t *testing.T) {
	storage := adapter.NewInMemoryStorage()
	srv := app.NewService(storage, &stubProvider{}, func() string { return "ch-ord-1" }, nil)
	ctx := context.Background()

	// 1. FindByOrderID returns ErrChargeNotFound when no charge exists
	_, err := srv.FindByOrderID(ctx, "ord-999")
	if err == nil {
		t.Fatalf("expected error for nonexistent order, got nil")
	}
	if !errors.Is(err, app.ErrChargeNotFound) {
		t.Errorf("expected ErrChargeNotFound, got %v", err)
	}

	// 2. Create pending charge and verify FindByOrderID returns it
	charge, err := srv.CreatePendingCharge(ctx, "ord-999", 2500, "EUR", "fake")
	if err != nil {
		t.Fatalf("CreatePendingCharge: %v", err)
	}

	found, err := srv.FindByOrderID(ctx, "ord-999")
	if err != nil {
		t.Fatalf("FindByOrderID: %v", err)
	}
	if found.ID() != charge.ID() {
		t.Errorf("ID = %q, want %q", found.ID(), charge.ID())
	}
	if found.OrderID() != "ord-999" {
		t.Errorf("OrderID = %q, want ord-999", found.OrderID())
	}
	if found.Amount() != 2500 {
		t.Errorf("Amount = %d, want 2500", found.Amount())
	}
	if found.Currency() != "EUR" {
		t.Errorf("Currency = %q, want EUR", found.Currency())
	}
	if found.Provider() != "fake" {
		t.Errorf("Provider = %q, want fake", found.Provider())
	}
}
