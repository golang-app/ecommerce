// Package reviews is the composition root for the reviews bounded context:
// it wires the postgres storage adapter and a VerifiedBuyerChecker (built
// from a tiny port the caller satisfies — typically the checkout query
// side's HasPurchasedProduct) into the application Service. The exported
// New() returns both the application.BoundedContext envelope and the
// concrete *app.Service so the layout context can depend on it directly.
package reviews

import (
	"context"
	"database/sql"

	"github.com/bkielbasa/go-ecommerce/backend/internal/application"
	"github.com/bkielbasa/go-ecommerce/backend/reviews/adapter"
	"github.com/bkielbasa/go-ecommerce/backend/reviews/app"
)

// VerifiedBuyerSource is the tiny ACL port the composition root satisfies.
// The caller (cmd/web) passes any value whose HasPurchasedProduct matches
// this signature — in practice the checkout query service. Keeping the port
// here (rather than reaching into checkout/query) means the reviews context
// has no compile-time dependency on checkout.
type VerifiedBuyerSource interface {
	HasPurchasedProduct(ctx context.Context, customerID, productID string) (bool, error)
}

// New wires the production adapter and verified-buyer port. The returned
// *app.Service is intentionally exposed (not just as an interface) so the
// layout context can declare a narrow interface against its public methods
// without leaking the storage type.
func New(db *sql.DB, buyers VerifiedBuyerSource) (application.BoundedContext, *app.Service) {
	storage := adapter.NewPostgres(db)
	checker := verifiedBuyerAdapter{buyers: buyers}
	srv := app.NewService(storage, checker)
	return &boundedContext{}, srv
}

// verifiedBuyerAdapter satisfies app.VerifiedBuyerChecker by delegating to
// the supplied VerifiedBuyerSource. The parameter order flip
// (HasPurchased(customerID, productID) -> HasPurchasedProduct(customerID,
// productID)) is intentional: the reviews port names match the query the
// checker is answering, while the source signature mirrors the checkout
// query method.
type verifiedBuyerAdapter struct {
	buyers VerifiedBuyerSource
}

func (a verifiedBuyerAdapter) HasPurchased(ctx context.Context, customerID, productID string) (bool, error) {
	return a.buyers.HasPurchasedProduct(ctx, customerID, productID)
}

type boundedContext struct{}
