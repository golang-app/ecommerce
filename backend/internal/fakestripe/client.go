// Package fakestripe is a MOCK that emulates a real Stripe-like payment
// provider so the payments bounded context has something external to
// integrate against.
//
// IMPORTANT: this is not a payments domain object. The vocabulary in
// this package — PaymentIntent, ClientSecret, "requires_action",
// idempotency-key semantics on the create endpoint, HMAC-signed
// webhook bodies — is deliberately the OUTSIDER's vocabulary. Real
// Stripe ships almost exactly these names; pretending otherwise here
// would defeat the whole point of the exercise.
//
// The whole demo's point is the Anti-Corruption Layer drawn between
// fakestripe and the payments bounded context (see
// payments/adapter/fakestripe_acl.go). Only that ACL is allowed to
// import this package — the payments domain MUST stay free of
// outside-vocabulary types. If you find yourself importing fakestripe
// anywhere else, you're either (a) writing another adapter (fine) or
// (b) leaking the external model into the domain (not fine; the
// translation is the whole exercise).
//
// What is faithfully modelled here:
//
//   - CreatePaymentIntent has idempotency-key semantics: a second
//     call carrying the same Idempotency-Key returns the SAME
//     PaymentIntent (status and all) instead of charging a second
//     time. Real Stripe does this in the wire SDK.
//
//   - The provider's status vocabulary is "requires_action",
//     "succeeded", "failed" — DELIBERATELY DIFFERENT from our domain
//     statuses (pending / succeeded / failed). The translation is the
//     ACL's job. Mapping "requires_action" -> our "pending" is the
//     load-bearing example: the outside world's "the customer must do
//     SCA" is, from our point of view, just "we don't know yet".
//
//   - Verify is HMAC-SHA256 of the body with the configured secret,
//     same shape as Stripe's `Stripe-Signature` header (minus the
//     timestamp tolerance window, which would only obscure the shape).
//     A toy implementation, but the verification IS the integration
//     contract for webhooks: the only reason an unsigned-webhook
//     handler isn't a security hole is the secret nobody outside
//     Stripe knows.
//
// Failure-mode configuration is via Client.FailCardEndingIn. Set it to
// e.g. "0000" so a `Source` (card token) ending in "0000" yields a
// "failed" intent; leave it empty and every charge succeeds. This is
// how dev / tests exercise the failure path without integrating a real
// declined-card workflow.
package fakestripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Provider-flavoured status strings. They are part of fakestripe's
// PUBLIC vocabulary, the same way Stripe's `requires_action` /
// `succeeded` / `failed` are part of theirs. Translation onto the
// payments domain statuses lives in the ACL, NOT here.
const (
	StatusRequiresAction = "requires_action"
	StatusSucceeded      = "succeeded"
	StatusFailed         = "failed"
)

// PaymentIntentRequest is the input to CreatePaymentIntent. The fields
// match real Stripe's PaymentIntent.create call closely enough that the
// ACL translation is unambiguous:
//
//   - Amount is in minor units (cents), same as Stripe.
//   - Currency is the lowercase ISO 4217 code. Stripe lowercases it; we
//     accept either and don't normalise — the ACL passes through what
//     the domain has.
//   - Source is the card / payment-method token. In real Stripe this
//     is `pm_xxx`; in the mock we accept anything stringy. The mock
//     only inspects its suffix for the FailCardEndingIn trigger.
//   - IdempotencyKey is the request-level retry key. A second
//     CreatePaymentIntent call carrying the same key returns the
//     previously-created PaymentIntent verbatim, exactly like Stripe.
type PaymentIntentRequest struct {
	Amount         int64
	Currency       string
	Source         string
	IdempotencyKey string
}

// PaymentIntent is the provider-flavoured response. Status is one of
// the constants above; mapping to the payments domain's
// (pending / succeeded / failed) is the ACL's responsibility.
//
// ClientSecret is what a real Stripe.js / Elements integration would
// hand to the browser so the customer can complete an SCA challenge.
// The mock fills it in for shape; nothing in this codebase reads it.
type PaymentIntent struct {
	ID           string
	ClientSecret string
	Status       string
	Created      time.Time
}

// Client is the (in-process) handle to the fake provider. The struct
// is intentionally small: a counter for monotonic intent IDs, an
// idempotency map for retry-safe creates, a clock seam for tests, and
// a single failure-mode knob. Real client SDKs are stateless except
// for HTTP config; this stays close to that shape.
type Client struct {
	// FailCardEndingIn drives the "failed" branch. A non-empty value
	// triggers a StatusFailed intent for any Source whose tail
	// matches. Empty (the production default) means every charge
	// succeeds. The field is read-only after NewClient returns;
	// concurrent reads are fine without a mutex.
	FailCardEndingIn string

	now    func() time.Time
	seq    atomic.Uint64
	idemMu sync.Mutex
	// idemMap is the idempotency table real Stripe maintains
	// server-side: (key) -> the response previously returned for
	// that key. A second create with the same key replays the
	// stored PaymentIntent without producing a new one.
	idemMap map[string]PaymentIntent
}

// NewClient builds a fake provider client. failCardEndingIn drives the
// failure mode; pass "" to have every charge succeed.
func NewClient(failCardEndingIn string) *Client {
	return &Client{
		FailCardEndingIn: failCardEndingIn,
		now:              func() time.Time { return time.Now().UTC() },
		idemMap:          map[string]PaymentIntent{},
	}
}

// WithClock substitutes the time source. Test-only seam; production
// callers never need it.
func (c *Client) WithClock(now func() time.Time) *Client {
	c.now = now
	return c
}

// Preseed installs a pre-built PaymentIntent under the given
// idempotency key. The next CreatePaymentIntent call that carries
// that same key will return the stored intent verbatim — exactly
// like a regular idempotent retry of an earlier call. Test-only;
// production code never calls this. Exposed so tests of upstream
// translators (the ACL) can deterministically produce any of the
// provider's status values, including the "requires_action" state
// the fake doesn't otherwise mint.
func (c *Client) Preseed(idempotencyKey string, intent PaymentIntent) {
	c.idemMu.Lock()
	defer c.idemMu.Unlock()
	c.idemMap[idempotencyKey] = intent
}

// ErrInvalidAmount is returned for non-positive Amount. Real Stripe
// reports `amount_too_small`; we collapse the family because the only
// caller is our ACL and a single error is enough.
var ErrInvalidAmount = errors.New("fakestripe: amount must be positive")

// CreatePaymentIntent is the create endpoint. It honours the
// idempotency-key contract: a second call with the same key returns
// the SAME PaymentIntent without re-deciding success/failure. The
// status is decided once, at first create time, by inspecting Source
// against FailCardEndingIn.
//
// A blank IdempotencyKey is allowed for clients that don't care about
// retry safety — those calls just don't get the dedupe; every blank-key
// invocation produces a fresh intent. Real Stripe behaves the same.
func (c *Client) CreatePaymentIntent(ctx context.Context, req PaymentIntentRequest) (PaymentIntent, error) {
	if err := ctx.Err(); err != nil {
		return PaymentIntent{}, err
	}
	if req.Amount <= 0 {
		return PaymentIntent{}, ErrInvalidAmount
	}

	if req.IdempotencyKey != "" {
		c.idemMu.Lock()
		if prev, ok := c.idemMap[req.IdempotencyKey]; ok {
			c.idemMu.Unlock()
			return prev, nil
		}
		c.idemMu.Unlock()
	}

	status := StatusSucceeded
	if c.FailCardEndingIn != "" && strings.HasSuffix(req.Source, c.FailCardEndingIn) {
		status = StatusFailed
	}

	id := fmt.Sprintf("pi_%d", c.seq.Add(1))
	intent := PaymentIntent{
		ID:           id,
		ClientSecret: id + "_secret_mock",
		Status:       status,
		Created:      c.now(),
	}

	if req.IdempotencyKey != "" {
		c.idemMu.Lock()
		// Re-check under the lock to handle a race where two
		// concurrent calls passed the optimistic check above
		// before either had time to record itself; first writer
		// wins, the second returns the stored intent.
		if prev, ok := c.idemMap[req.IdempotencyKey]; ok {
			c.idemMu.Unlock()
			return prev, nil
		}
		c.idemMap[req.IdempotencyKey] = intent
		c.idemMu.Unlock()
	}
	return intent, nil
}

// Verify checks a webhook signature against the configured secret. The
// header is expected to be the lowercase hex HMAC-SHA256 of body with
// secret. Real Stripe's `Stripe-Signature` header carries a timestamp
// and one or more `v1=` signatures; we strip that to the essential
// HMAC equality check because the timestamp tolerance window adds
// nothing to the demonstration.
//
// Constant-time comparison is mandatory: a length-equal short-circuit
// would leak the prefix of a forged signature to a timing attacker.
func Verify(secret, header string, body []byte) error {
	if secret == "" {
		return errors.New("fakestripe: webhook secret is empty")
	}
	if header == "" {
		return errors.New("fakestripe: signature header is missing")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(header)) {
		return errors.New("fakestripe: invalid signature")
	}
	return nil
}

// Sign is the symmetric helper Verify reads against. The webhook
// emitter (or a test) calls Sign(body) to mint the header value a
// downstream Verify will accept. Exposed because the demo's webhook
// handler is on the receiving end and tests need to mint a valid
// payload without re-implementing the HMAC.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
