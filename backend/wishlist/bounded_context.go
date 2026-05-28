// Package wishlist is the composition root for the wishlist bounded
// context: it wires the postgres storage adapter into the application
// Service. Compared to reviews this context has no cross-context port —
// the wishlist owns its data wholly and depends only on
// productcatalog_variant via the table's foreign key.
package wishlist

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/wishlist/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/wishlist/app"
)

// New wires the production adapter and returns both the
// application.BoundedContext envelope and the concrete *app.Service so the
// layout context can declare a narrow interface against its public methods
// without leaking the storage type.
func New(db *sql.DB) (application.BoundedContext, *app.Service) {
	storage := adapter.NewPostgres(db)
	srv := app.NewService(storage)
	return &boundedContext{}, srv
}

type boundedContext struct{}
