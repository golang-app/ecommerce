// Package repricing is the composition root for the repricing
// (Process Manager) bounded context. It owns the bulk "reprice every
// product in a category by N%" workflow as a state-stored saga; see
// repricing/domain and repricing/app for the implementation.
//
// The context does not register HTTP routes itself — the admin UI
// lives in the layout package (http_admin_repricing.go) which talks
// to *app.Service through a narrow interface. Mirrors the
// fulfillment context's split.
package repricing

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/repricing/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/repricing/app"
)

// New wires the production adapter onto the supplied *sql.DB and
// binds the application service to the supplied CategoryReader and
// PriceUpdater seams (the composition root provides adapters over
// productcatalog).
func New(db *sql.DB, reader app.CategoryReader, updater app.PriceUpdater) (application.BoundedContext, *app.Service) {
	storage := adapter.NewPostgres(db)
	srv := app.NewService(storage, reader, updater)
	return &boundedContext{}, srv
}

type boundedContext struct{}
