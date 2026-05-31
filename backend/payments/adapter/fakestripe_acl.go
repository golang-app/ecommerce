// fakestripe_acl.go is the Anti-Corruption Layer between the payments
// bounded context and the internal/fakestripe mock provider.
//
// WHAT THE ACL TRANSLATES.
//
//  1. SHAPE. payments/app expresses a charge attempt as a
//     ChargeRequest (amount, currency, source). fakestripe expresses
//     the same thing as a PaymentIntentRequest (amount, currency,
//     source, idempotency key, etc.) — same conceptual operation,
//     different field set. The ACL maps one to the other.
//
//  2. VOCABULARY. fakestripe statuses are
//     "requires_action" / "succeeded" / "failed"; payments-domain
//     statuses are pending / succeeded / failed. The mapping is:
//
//       fakestripe.StatusRequiresAction -> domain.StatusPending
//       fakestripe.StatusSucceeded      -> domain.StatusSucceeded
//       fakestripe.StatusFailed         -> domain.StatusFailed
//
//     "requires_action" -> "pending" is the load-bearing
//     translation: the provider's "SCA challenge in flight" state is,
//     from our point of view, simply "we don't know yet". A real
//     Stripe wire-up would receive a `payment_intent.succeeded`
//     webhook later that flips the same Charge into `succeeded` via
//     payments.Service.MarkSucceeded — which is precisely the
//     webhook handler this PR also introduces.
//
//  3. IDENTITY. The provider gives us back a PaymentIntent.ID
//     ("pi_xxx"); we copy it into ChargeResult.ProviderRef. That
//     opaque token is the only piece of provider state that ever
//     reaches the payments domain, and it is treated as a black box
//     (never parsed, never inspected — only used for join lookups
//     when webhooks arrive).
//
// This file is the ONLY thing in the repository that imports
// internal/fakestripe (other than fakestripe's own tests). If a future
// change adds a second importer, the import graph stops enforcing the
// ACL — and the demonstration breaks.
package adapter

import (
	"context"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/internal/fakestripe"
	"github.com/bkielbasa/go-ecommerce/backend/payments/app"
	"github.com/bkielbasa/go-ecommerce/backend/payments/domain"
)

// Provider is the ACL onto fakestripe. It satisfies app.Provider so
// the application service has zero compile-time knowledge of the
// fakestripe types.
//
// The provider forwards ChargeRequest.IdempotencyKey straight to the
// underlying client so the SAME retry-safety token threads through
// every layer: from the checkout-side derivation off the order id,
// into payments_charge.idempotency_key (UNIQUE), into the provider's
// own idempotency table. End-to-end the contract is "one logical
// charge, one token, every layer dedupes against it".
type Provider struct {
	client *fakestripe.Client
}

// NewProvider builds the ACL around a fakestripe client.
func NewProvider(client *fakestripe.Client) *Provider {
	return &Provider{client: client}
}

// Charge translates the payments-domain ChargeRequest into the
// fakestripe-flavoured PaymentIntentRequest, invokes the provider,
// then translates the response status back into the payments-domain
// vocabulary. Errors from the provider propagate verbatim — the
// service wraps them with payments-domain context.
func (p *Provider) Charge(ctx context.Context, req app.ChargeRequest) (app.ChargeResult, error) {
	intent, err := p.client.CreatePaymentIntent(ctx, fakestripe.PaymentIntentRequest{
		Amount:         req.Amount,
		Currency:       req.Currency,
		Source:         req.Source,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return app.ChargeResult{}, fmt.Errorf("fakestripe acl: %w", err)
	}
	status, err := translateStatus(intent.Status)
	if err != nil {
		return app.ChargeResult{}, err
	}
	return app.ChargeResult{Status: status, ProviderRef: intent.ID}, nil
}

// translateStatus is the documented mapping from provider vocabulary
// to domain vocabulary. Any value the provider may legitimately
// return MUST appear here; an unknown value is an integration bug
// (the provider added a new state the ACL hasn't been taught about)
// and surfaces as an error so we never silently treat it as
// "succeeded".
func translateStatus(providerStatus string) (domain.Status, error) {
	switch providerStatus {
	case fakestripe.StatusRequiresAction:
		// Provider says "SCA challenge required"; we say "not
		// terminal yet". A later webhook will move it on.
		return domain.StatusPending, nil
	case fakestripe.StatusSucceeded:
		return domain.StatusSucceeded, nil
	case fakestripe.StatusFailed:
		return domain.StatusFailed, nil
	default:
		return "", fmt.Errorf("fakestripe acl: unknown provider status %q", providerStatus)
	}
}
