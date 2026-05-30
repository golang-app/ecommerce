package adapter

import (
	"context"
	"errors"
	"fmt"

	checkoutapp "github.com/bkielbasa/go-ecommerce/backend/checkout/app"
	paymentsdomain "github.com/bkielbasa/go-ecommerce/backend/payments/domain"
)

// PaymentsService is the narrow seam onto payments.app.Service that
// the checkout PaymentsProcessor needs. Declared as an interface so
// the adapter stays trivially testable without spinning up a real
// payments service.
type PaymentsService interface {
	Charge(ctx context.Context, orderID string, amount int64, currency, source, idempotencyKey string) (paymentsdomain.Charge, error)
}

// PaymentsProcessor satisfies checkout/app.PaymentProcessor by calling
// the payments bounded context's Service. It is the THIRD ACL in the
// demonstration:
//
//  1. payments/adapter/fakestripe_acl.go — translates between the
//     payments domain and the external fakestripe provider.
//  2. (this file) — translates between checkout's narrow
//     "charge a card" port and the payments service's full
//     Charge workflow.
//  3. checkout/app/checkout.go — calls the PaymentProcessor port,
//     unaware of what concrete implementation backs it.
//
// Each layer owns its own vocabulary: checkout knows "charge", payments
// knows "Charge (the value object)", fakestripe knows "PaymentIntent".
// Nothing leaks. That's the demo's whole point.
//
// Idempotency key and order id are sourced from the request context;
// the producer is checkout/app.Place via WithChargeContext. Threading
// them that way preserves the existing PaymentProcessor signature —
// widening the port would force every other implementation to absorb
// the same change just to demonstrate this pattern.
type PaymentsProcessor struct {
	srv PaymentsService
}

// NewPaymentsProcessor builds a checkout PaymentProcessor backed by
// the payments Service.
func NewPaymentsProcessor(srv PaymentsService) PaymentsProcessor {
	return PaymentsProcessor{srv: srv}
}

// Charge satisfies checkout/app.PaymentProcessor. amount and currency
// are passed through verbatim; cardNumber lands in the provider's
// Source field via payments.app.ChargeRequest.
//
// A failed Charge (provider declined, or non-nil error) is surfaced
// as a non-nil error so checkout's existing decline-path logic
// (release stock, mark order failed) keeps working unchanged.
func (p PaymentsProcessor) Charge(ctx context.Context, amount int64, currency, cardNumber string) error {
	orderID, idempotencyKey := checkoutapp.ChargeContextValues(ctx)
	result, err := p.srv.Charge(ctx, orderID, amount, currency, cardNumber, idempotencyKey)
	if err != nil {
		return fmt.Errorf("payments charge: %w", err)
	}
	switch result.Status() {
	case paymentsdomain.StatusSucceeded:
		return nil
	case paymentsdomain.StatusPending:
		// In a synchronous demo the only thing checkout can do
		// with a still-pending charge is treat it as not-yet-paid.
		// The webhook handler will move it on later; for now
		// surfacing it as an error preserves the existing
		// place-flow contract (succeeded -> commit, anything else
		// -> rollback).
		return errors.New("payments charge: pending (awaiting provider confirmation)")
	case paymentsdomain.StatusFailed:
		return errors.New("payments charge: declined by provider")
	default:
		return fmt.Errorf("payments charge: unexpected status %q", result.Status())
	}
}
