// Package store is the composition root for the store bounded context.
// A Store is a request-bound storefront facade: each request is mapped
// to exactly one store by inspecting its Host header, and the store's
// configured currency drives the display layer (the `money` template
// helper). The context owns the `store` table introduced by migration
// 000029_stores; every other context is currency-agnostic and reads
// the active store off the request context.
package store

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/store/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/store/app"
)

// New wires the production storage adapter and returns both the
// application.BoundedContext envelope and the concrete *app.Service so
// the layout package can declare a narrow interface against its public
// methods without leaking the storage type.
func New(db *sql.DB) (application.BoundedContext, *app.Service) {
	storage := adapter.NewPostgres(db)
	srv := app.NewService(storage)
	return &boundedContext{}, srv
}

type boundedContext struct{}
