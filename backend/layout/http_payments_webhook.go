package layout

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/bkielbasa/go-ecommerce/backend/internal/fakestripe"
)

// paymentsWebhookService is the narrow seam the webhook handler needs
// onto payments.app.Service: just the MarkSucceeded / MarkFailed
// status transitions. The Charge/Place workflow stays inside the
// payments application service; the webhook is purely an asynchronous
// reconciliation surface.
//
// Keeping this an interface (rather than the concrete
// *payments/app.Service) lets the layout tests drive it with an
// in-memory double — and, in production, it just happens to be
// satisfied by the same struct we pass to the checkout-side adapter.
type paymentsWebhookService interface {
	MarkSucceeded(ctx context.Context, providerRef string) error
	MarkFailed(ctx context.Context, providerRef, reason string) error
}

// paymentsWebhookEvent is the minimal subset of the provider's webhook
// payload we care about. Real Stripe sends a fat envelope ("event":
// {"type":"...", "data":{"object":{...}}}); we accept that shape but
// only pull the two fields the handler reacts to. Tolerating extra
// fields keeps the handler forward-compatible with provider schema
// changes.
//
// The provider's `type` field is the discriminator — for a real Stripe
// integration the values would be e.g. `payment_intent.succeeded` /
// `payment_intent.payment_failed`. We mirror those literal strings
// because the demo's whole point is faithfully translating provider
// vocabulary at the boundary.
type paymentsWebhookEvent struct {
	Type string `json:"type"`
	Data struct {
		Object struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"object"`
	} `json:"data"`
}

const (
	webhookTypePaymentSucceeded = "payment_intent.succeeded"
	webhookTypePaymentFailed    = "payment_intent.payment_failed"
)

// paymentsWebhookHandler is a stand-alone http.Handler (not a method
// on httpHandler) because the dependencies it needs — the webhook
// secret + the payments service — are out of the storefront handler's
// usual orbit. Registering it as a small, focused handler also makes
// the CSRF bypass explicit (it is mounted under /webhooks/ which the
// CSRF middleware special-cases).
//
// SIGNATURE VERIFICATION. The first thing the handler does is read
// the body in full, then call fakestripe.Verify against the
// configured secret and the value of the Stripe-Signature header. A
// failed signature is a 400 — we never act on an unverified payload.
// The body has to be read before verifying (HMAC needs the bytes) and
// before parsing (JSON would consume the stream); buffering it is
// the standard trade-off.
//
// CSRF. Webhook requests come from outside the browser, carry no
// session cookie, and are authenticated by the HMAC signature alone.
// They MUST bypass CSRF — see csrf.go where the /webhooks/ prefix is
// explicitly exempted.
//
// IDEMPOTENCY. Both MarkSucceeded and MarkFailed are no-ops if the
// Charge is already in the target state; this is exactly the
// "subscriber must tolerate redelivery" property the Outbox + Inbox
// patterns rely on for at-least-once delivery elsewhere. The webhook
// inherits the same constraint because real providers redeliver
// until they get a 2xx; the handler's idempotency makes that safe.
//
// OUTBOX HAND-OFF. In a fuller demo the webhook would also stage a
// payments.OrderPaymentSettled integration event into the outbox so
// downstream subscribers (analytics, audit) could react. The
// checkout context already publishes OrderPaid on successful Place,
// so for the synchronous demo path the webhook is a redundant
// secondary channel — we intentionally do NOT double-publish here,
// to avoid duplicating the existing OrderPaid flow. A real
// asynchronous integration (delayed captures, disputes) is where
// that outbox hand-off becomes load-bearing.
func paymentsWebhookHandler(secret string, srv paymentsWebhookService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "cannot read body", http.StatusBadRequest)
			return
		}
		sig := r.Header.Get("Stripe-Signature")
		if err := fakestripe.Verify(secret, sig, body); err != nil {
			http.Error(w, "invalid signature", http.StatusBadRequest)
			return
		}
		var ev paymentsWebhookEvent
		if err := json.Unmarshal(body, &ev); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		if ev.Data.Object.ID == "" {
			http.Error(w, "missing intent id", http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		switch ev.Type {
		case webhookTypePaymentSucceeded:
			if err := srv.MarkSucceeded(ctx, ev.Data.Object.ID); err != nil {
				http.Error(w, "settle failed", http.StatusInternalServerError)
				return
			}
		case webhookTypePaymentFailed:
			if err := srv.MarkFailed(ctx, ev.Data.Object.ID, ev.Data.Object.Status); err != nil {
				http.Error(w, "settle failed", http.StatusInternalServerError)
				return
			}
		default:
			// Unknown event types are ack'd with 200 so the
			// provider does not redeliver them forever. A real
			// integration would log + alert; the demo just drops
			// them on the floor.
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
