package checkout

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"

	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/app"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/internal/eventbus"
)

// CartReader and CartClearer let New accept the cart service from the cart
// bounded context without importing cart.app directly (avoids package
// cycles and keeps the contract explicit).
type CartReader interface {
	Get(ctx context.Context, sessID string) (*cartDomain.Cart, error)
}

// StockReserver is implemented by the product catalogue; checkout reserves
// stock atomically before placing an order.
type StockReserver interface {
	Reserve(ctx context.Context, quantities map[string]int) error
	Release(ctx context.Context, quantities map[string]int) error
}

// New wires the checkout context and returns its command service (write side,
// event-sourced) and query service (read side, projection-backed) separately,
// keeping the CQRS split explicit at the composition root. Cross-context side
// effects (e.g. clearing the cart) are driven by integration events published
// on bus, not by direct calls.
func New(db *sql.DB, cart CartReader, bus *eventbus.Bus, stock StockReserver) (application.BoundedContext, app.CheckoutService, query.Service) {
	storage := adapter.NewPostgres(db)
	payment := adapter.NewFakePayment()
	cmd := app.NewCheckoutService(cart, storage, payment, stock, bus, newOrderID)
	queries := query.NewService(storage)
	return &boundedContext{}, cmd, queries
}

type boundedContext struct{}

// newOrderID returns a short hex id prefixed with "ord-".
func newOrderID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "ord-" + hex.EncodeToString(b)
}
