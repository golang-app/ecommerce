package adapter_test

import (
	"context"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/internal/fakestripe"
	"github.com/bkielbasa/go-ecommerce/backend/payments/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/payments/app"
	"github.com/bkielbasa/go-ecommerce/backend/payments/domain"
)

// runACL exercises the ACL translation for each provider status.
// We pick the input that makes the production fakestripe.Client
// produce the desired status — FailCardEndingIn for failed,
// default for succeeded, and Preseed for "requires_action" (the
// only state the fake doesn't otherwise mint).
func runACL(t *testing.T, providerStatus string) (app.ChargeResult, error) {
	t.Helper()
	// FailCardEndingIn "x" lets us deterministically pick failed vs
	// succeeded via the Source string; for "requires_action" we
	// need to inject the status another way, so we mint the client
	// then poke the idempotency map. That's brittle — but the
	// translation under test is `translateStatus`, which is what
	// the ACL exposes via Charge.
	switch providerStatus {
	case fakestripe.StatusSucceeded:
		client := fakestripe.NewClient("")
		acl := adapter.NewProvider(client)
		return acl.Charge(context.Background(), app.ChargeRequest{Amount: 100, Currency: "usd", Source: "tok_visa"})
	case fakestripe.StatusFailed:
		client := fakestripe.NewClient("0000")
		acl := adapter.NewProvider(client)
		return acl.Charge(context.Background(), app.ChargeRequest{Amount: 100, Currency: "usd", Source: "tok_4242424242420000"})
	default:
		// The fake client doesn't natively produce
		// "requires_action"; we exercise that path by
		// pre-seeding its idempotency map so the next call
		// with key=k returns a pre-built intent in that
		// status. This is documented as a TEST seam below.
		client := fakestripe.NewClient("")
		client.Preseed("key-ra", fakestripe.PaymentIntent{
			ID:     "pi_pre",
			Status: providerStatus,
		})
		acl := adapter.NewProvider(client)
		return acl.Charge(context.Background(), app.ChargeRequest{Amount: 100, Currency: "usd", Source: "tok_any", IdempotencyKey: "key-ra"})
	}
}

func TestACL_TranslateRequiresActionToPending(t *testing.T) {
	got, err := runACL(t, fakestripe.StatusRequiresAction)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != domain.StatusPending {
		t.Fatalf("expected pending; got %q", got.Status)
	}
}

func TestACL_TranslateSucceeded(t *testing.T) {
	got, err := runACL(t, fakestripe.StatusSucceeded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != domain.StatusSucceeded {
		t.Fatalf("expected succeeded; got %q", got.Status)
	}
	if got.ProviderRef == "" {
		t.Fatalf("expected non-empty provider ref")
	}
}

func TestACL_TranslateFailed(t *testing.T) {
	got, err := runACL(t, fakestripe.StatusFailed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != domain.StatusFailed {
		t.Fatalf("expected failed; got %q", got.Status)
	}
}
