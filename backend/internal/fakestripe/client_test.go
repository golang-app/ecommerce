package fakestripe_test

import (
	"context"
	"testing"

	"github.com/bkielbasa/go-ecommerce/backend/internal/fakestripe"
)

// Same idempotency key, two calls -> same PaymentIntent. The provider
// must NOT issue a second intent for a key it has already seen, even
// if the request body differs (real Stripe stores the response for the
// first request only). For the demo we just assert the ID matches.
func TestCreatePaymentIntent_IdempotencyKeyReturnsSameIntent(t *testing.T) {
	client := fakestripe.NewClient("")
	ctx := context.Background()

	first, err := client.CreatePaymentIntent(ctx, fakestripe.PaymentIntentRequest{
		Amount:         1234,
		Currency:       "usd",
		Source:         "tok_visa",
		IdempotencyKey: "ord-abc",
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}

	second, err := client.CreatePaymentIntent(ctx, fakestripe.PaymentIntentRequest{
		Amount:         1234,
		Currency:       "usd",
		Source:         "tok_visa",
		IdempotencyKey: "ord-abc",
	})
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same intent id; got %q vs %q", first.ID, second.ID)
	}
	if first.Status != second.Status {
		t.Fatalf("expected same status; got %q vs %q", first.Status, second.Status)
	}
}

// A blank idempotency key disables dedupe — every call mints a fresh
// intent. Mirrors real Stripe.
func TestCreatePaymentIntent_BlankKeyMintsFresh(t *testing.T) {
	client := fakestripe.NewClient("")
	ctx := context.Background()

	a, err := client.CreatePaymentIntent(ctx, fakestripe.PaymentIntentRequest{Amount: 100, Currency: "usd", Source: "tok"})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	b, err := client.CreatePaymentIntent(ctx, fakestripe.PaymentIntentRequest{Amount: 100, Currency: "usd", Source: "tok"})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if a.ID == b.ID {
		t.Fatalf("expected distinct intents for blank key; both were %q", a.ID)
	}
}

// FailCardEndingIn drives the failed-status branch; the suffix match
// is on the Source (card token) field. The point is to give dev /
// tests a deterministic way to exercise the failure path.
func TestCreatePaymentIntent_FailCardEndingInTriggersFailedStatus(t *testing.T) {
	client := fakestripe.NewClient("0000")
	ctx := context.Background()

	failing, err := client.CreatePaymentIntent(ctx, fakestripe.PaymentIntentRequest{Amount: 100, Currency: "usd", Source: "tok_4242424242420000"})
	if err != nil {
		t.Fatalf("failing: %v", err)
	}
	if failing.Status != fakestripe.StatusFailed {
		t.Fatalf("expected failed status; got %q", failing.Status)
	}

	ok, err := client.CreatePaymentIntent(ctx, fakestripe.PaymentIntentRequest{Amount: 100, Currency: "usd", Source: "tok_4242424242424242"})
	if err != nil {
		t.Fatalf("ok: %v", err)
	}
	if ok.Status != fakestripe.StatusSucceeded {
		t.Fatalf("expected succeeded status; got %q", ok.Status)
	}
}

func TestVerify_AcceptsCorrectSignature(t *testing.T) {
	body := []byte(`{"id":"evt_1","type":"payment_intent.succeeded"}`)
	sig := fakestripe.Sign("whsec_dev", body)
	if err := fakestripe.Verify("whsec_dev", sig, body); err != nil {
		t.Fatalf("expected valid signature; got %v", err)
	}
}

func TestVerify_RejectsTampered(t *testing.T) {
	body := []byte(`{"id":"evt_1","type":"payment_intent.succeeded"}`)
	sig := fakestripe.Sign("whsec_dev", body)
	tampered := append([]byte{}, body...)
	tampered[0] = 'X'
	if err := fakestripe.Verify("whsec_dev", sig, tampered); err == nil {
		t.Fatalf("expected mismatch error; got nil")
	}
}
