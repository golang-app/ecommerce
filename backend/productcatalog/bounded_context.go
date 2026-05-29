package productcatalog

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
)

// New wires the production storage adapter and returns the bounded-context
// envelope plus the concrete ProductService. The supplied SearchIndexer is
// invoked after every successful product mutation (Add/Update/Delete plus
// the variant-management operations that change displayed price/name);
// pass app.NoopSearchIndexer to opt out (e.g. in the CLI, or in tests).
func New(db *sql.DB, searchIdx app.SearchIndexer) (application.BoundedContext, app.ProductService) {
	storage := adapter.NewPostgres(db)
	appServ := app.NewProductService(storage).WithSearchIndexer(searchIdx)

	return &boundedContext{}, appServ
}

type boundedContext struct {
}
