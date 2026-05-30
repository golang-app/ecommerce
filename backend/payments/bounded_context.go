// Package payments is the composition root for the (internal)
// payments bounded context.
//
// The context exists to demonstrate three textbook patterns in one
// place: an Anti-Corruption Layer in front of an external provider
// (see payments/adapter/fakestripe_acl.go), an asynchronous webhook
// handler against that same provider
// (see backend/layout/http_payments_webhook.go), and an
// idempotency-key contract that threads end-to-end from the caller
// (checkout) through payments through to the provider.
//
// payments is INTERNAL — it is not an Open Host Service. Only one
// other context calls it (checkout, through its own
// PaymentProcessor port + a small adapter). Keeping it internal lets
// us evolve the payments-domain types without coordinating with
// other contexts.
package payments

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/internal/fakestripe"
	"github.com/bkielbasa/go-ecommerce/backend/payments/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/payments/app"
)

// New wires the production payments context: a postgres Storage, the
// ACL around the supplied fakestripe client, and the application
// Service. Returns the application.BoundedContext envelope and the
// concrete *app.Service the layout / checkout adapter consume.
func New(db *sql.DB, client *fakestripe.Client) (application.BoundedContext, *app.Service) {
	storage := adapter.NewPostgresStorage(db)
	provider := adapter.NewProvider(client)
	srv := app.NewService(storage, provider, newChargeID, nil)
	return &boundedContext{}, srv
}

// NewInMemory is the test-friendly variant: same wiring, but with the
// in-memory storage adapter. Keeps the unit tests inside checkout /
// layout free of a database when they only need a working payments
// pipeline.
func NewInMemory(client *fakestripe.Client) (application.BoundedContext, *app.Service) {
	storage := adapter.NewInMemoryStorage()
	provider := adapter.NewProvider(client)
	srv := app.NewService(storage, provider, newChargeID, nil)
	return &boundedContext{}, srv
}

type boundedContext struct{}

// newChargeID returns a short hex id prefixed with "ch-". Mirrors the
// checkout context's newOrderID — both contexts mint their own ids
// without coordinating, so the prefix is what tells the two apart in
// logs.
func newChargeID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "ch-" + hex.EncodeToString(b)
}
