package checkout

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"

	cartDomain "github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/checkout/app"
	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
)

// CartReader and CartClearer let New accept the cart service from the cart
// bounded context without importing cart.app directly (avoids package
// cycles and keeps the contract explicit).
type CartReader interface {
	Get(ctx context.Context, sessID string) (*cartDomain.Cart, error)
}

type CartClearer interface {
	Clear(ctx context.Context, sessID string) error
}

func New(db *sql.DB, cart CartReader, cartClr CartClearer) (application.BoundedContext, app.CheckoutService) {
	storage := adapter.NewPostgres(db)
	payment := adapter.NewFakePayment()
	srv := app.NewCheckoutService(cart, cartClr, storage, payment, newOrderID)
	return &boundedContext{}, srv
}

type boundedContext struct{}

// newOrderID returns a short hex id prefixed with "ord-".
func newOrderID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "ord-" + hex.EncodeToString(b)
}
