// Package search is the composition root for the search bounded context.
// It wires the postgres storage adapter into the application Service and
// returns both the application.BoundedContext envelope and the concrete
// *app.Service so callers can use it as both an Indexer (productcatalog)
// and a Querier (layout) without going through a narrower facade.
package search

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/search/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/search/app"
)

// New wires the production storage adapter and returns the bounded-context
// envelope plus the concrete *app.Service. The Service satisfies both the
// Indexer and Querier roles; production passes the same instance into
// productcatalog (for indexing on writes) and layout (for searching).
func New(db *sql.DB) (application.BoundedContext, *app.Service) {
	storage := adapter.NewPostgres(db)
	srv := app.NewService(storage)
	return &boundedContext{}, srv
}

type boundedContext struct{}
