// Package promo is the composition root for the promo (discount codes)
// bounded context. The context owns its data wholly (promo_code +
// promo_redemption from migration 000026); the only outward dependency is
// at the checkout boundary, where Service.Resolve / Service.Redeem are
// invoked. See backend/checkout/app/checkout.go for the integration.
package promo

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/promo/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/promo/app"
)

// New wires the production storage adapter and returns both the
// application.BoundedContext envelope and the concrete *app.Service so the
// layout package can declare a narrow interface against its public methods
// without leaking the storage type.
func New(db *sql.DB) (application.BoundedContext, *app.Service) {
	storage := adapter.NewPostgres(db)
	srv := app.NewService(storage)
	return &boundedContext{}, srv
}

type boundedContext struct{}
