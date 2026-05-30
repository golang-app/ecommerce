package checkout

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"

	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/app"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/domain"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/query"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
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

// StockMovements is the audit-log seam: every reservation/release/refund the
// checkout context drives is also recorded in the catalogue's stock movement
// log. Implemented by productcatalog/app.ProductService.
type StockMovements interface {
	Record(ctx context.Context, variantID string, delta int, reason, refOrderID string) error
}

// New wires the checkout context and returns its command service (write side,
// event-sourced) and query service (read side, projection-backed) separately,
// keeping the CQRS split explicit at the composition root.
//
// Cross-context side effects (e.g. clearing the cart, sending the order
// confirmation email) are driven by integration events published via the
// Transactional Outbox: Save stages them into outbox_event inside the same
// transaction that commits the domain events, and the outbox dispatcher
// (wired in cmd/web) republishes them onto the in-process bus.
//
// outbox is the seam through which the adapter stages those rows; pass nil
// to disable the integration entirely (matches the previous behaviour and
// what older tests expect).
//
// payment is the checkout PaymentProcessor port implementation. In
// production wiring it is the payments-backed adapter
// (checkout/adapter.PaymentsProcessor) that calls into the payments
// bounded context. Tests may pass a fake; a no-op "always succeeds"
// implementation is no longer provided here because the demo's whole
// point is that checkout calls payments through an explicit port.
//
// movements may be nil — checkout then runs without writing audit rows, which
// is the historical behaviour. taxStrategy and shippingStrategy are the
// pluggable pricing policies the service hands to domain.PriceQuote; passing
// nil for either falls back to the zero-config defaults (FlatTaxStrategy{} /
// ThresholdShippingStrategy{}), matching the historical "no tax, no
// automatic free-shipping override" behaviour.
func New(db *sql.DB, cart CartReader, outbox adapter.OutboxAppender, payment app.PaymentProcessor, stock StockReserver, movements StockMovements, taxStrategy domain.TaxStrategy, shippingStrategy domain.ShippingStrategy) (application.BoundedContext, app.CheckoutService, query.Service) {
	storage := adapter.NewPostgres(db, outbox)
	cmd := app.NewCheckoutService(cart, storage, payment, stock, movements, newOrderID, taxStrategy, shippingStrategy)
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
