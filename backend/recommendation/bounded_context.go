// Package recommendation is the composition root for the
// recommendation Supporting bounded context. It materialises a
// top-N "you might also like" list per product by combining two
// signals — order-history co-purchase and content similarity — and
// exposes the result through one read-side method
// (app.Service.Recommendations) that the storefront calls.
//
// SHAPE
//
// The context owns two tables (recommendation_link,
// recommendation_weights, migration 000041) and pulls everything
// else through two upstream ACLs:
//
//   - CatalogReader — reads product summaries, "products in category"
//     and "FTS-similar by title+description" from the productcatalog
//     tables.
//   - OrderHistoryReader — reads co-purchase pairs from the checkout
//     context's read-side projection (one SQL query per refresh).
//
// HTTP routes for the admin page + the storefront section live in
// the layout package, the same convention the fulfillment and
// repricing contexts already follow.
package recommendation

import (
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/recommendation/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/recommendation/app"
)

// New wires the production stack: postgres storage + weights,
// service, and refresher. The catalog ACL and history ACL are
// supplied by the caller so the composition root can substitute
// them in tests; in production both live in this package's adapter
// subpackage.
func New(db *sql.DB, catalog app.CatalogReader, history app.OrderHistoryReader) (application.BoundedContext, *app.Service, *app.Refresher) {
	storage := adapter.NewPostgres(db)
	weights := adapter.NewPostgresWeights(db)
	svc := app.NewService(storage, catalog, weights)
	refresher := app.NewRefresher(catalog, history, storage, weights)
	return &boundedContext{}, svc, refresher
}

// boundedContext is the application.BoundedContext envelope. The
// recommendation context registers no HTTP routes of its own — the
// storefront integration is a tweak to the product detail template,
// and the admin pages live in layout/http_admin_recommendations.go.
// Returning an empty struct keeps the AddBoundedContext call in
// main.go uniform with every other context, even though there is
// nothing to mux-register.
type boundedContext struct{}
