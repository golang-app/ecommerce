// Package app holds the recommendation application service: the
// orchestrating layer between the materialised top-N read model, the
// catalog/order-history ACLs and the five-weight scoring Strategy.
//
// CONTEXT SHAPE
//
// The recommendation context is a Supporting bounded context with
// two upstream relationships:
//
//   - productcatalog (the product catalogue) — read-only, via
//     CatalogReader. The composition root supplies an ACL that
//     translates productcatalog's domain.Product into the local
//     ProductSummary so the recommendation context never imports
//     productcatalog/domain.
//   - checkout (order history) — read-only, via OrderHistoryReader.
//     The composition root supplies an ACL that runs ONE SQL query
//     against checkout_order_item joined through productcatalog_variant
//     to return product-id pairs with their frequency.
//
// The refresher pulls from these two ACLs on a timer; the storefront
// reads through the application Service, which falls back to
// productcatalog's "popular in category" when the top-N is empty
// (cold start). The storefront only ever talks to ONE port —
// Service.Recommendations — and never sees the fallback path
// directly.
package app

import (
	"context"
	"errors"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/recommendation/domain"
)

// ErrNotFound is returned by Storage methods when no row matches.
// Wrapped via fmt.Errorf("...: %w", ...) by callers that want to
// add context; tests can branch with errors.Is.
var ErrNotFound = errors.New("recommendation: not found")

// CatalogReader is the read-side ACL onto the productcatalog
// context. The composition root wires an adapter that uses
// productcatalog/app.ProductService for the first three methods and
// runs an FTS query against the catalogue table for SimilarByText.
// The recommendation context never imports productcatalog's domain
// or app packages — every method here returns the LOCAL
// ProductSummary value object so the context boundary stays
// one-way.
type CatalogReader interface {
	// ProductByID hydrates one product's summary. ok=false (without
	// an error) means the product no longer exists — the storefront
	// silently drops stale rows from the recommendation list
	// rather than 500ing.
	ProductByID(ctx context.Context, id string) (domain.ProductSummary, bool, error)
	// AllProductIDs returns every product id in the catalogue. The
	// refresher walks this list once per tick; for the demo's ~50
	// products this is cheap. A future incremental refresher could
	// replace it with "ids changed since last refresh".
	AllProductIDs(ctx context.Context) ([]string, error)
	// ProductsInCategory returns up to `limit` products in the
	// given category, excluding nothing — the caller is expected to
	// filter out the seed itself. Used both for candidate
	// generation (refresher) and the cold-start fallback (service).
	ProductsInCategory(ctx context.Context, categoryID string, limit int) ([]domain.ProductSummary, error)
	// SimilarByText runs a Postgres FTS query against the catalogue
	// table using the seed product's title+description as the query
	// text, returning up to `limit` other products ranked by
	// ts_rank. Used by the refresher for content-similarity
	// candidate generation; the scorer itself uses a cheaper
	// in-memory Jaccard instead of a second FTS round-trip.
	SimilarByText(ctx context.Context, productID string, limit int) ([]domain.ProductSummary, error)
}

// OrderHistoryReader is the read-side ACL onto the checkout
// context. ONE method, one SQL query: enumerate pairs of products
// that appeared in the same paid (or later) order during the lookup
// window, with their frequency. The refresher caches the result for
// the rest of the tick — the per-seed loop reads the same map and
// never re-queries.
type OrderHistoryReader interface {
	// CoPurchasePairs returns the (product_a, product_b) -> count
	// map for orders placed at or after `since` whose status is
	// paid/shipped/delivered. Self-pairs (a == b) are filtered out
	// by the adapter; the map key is the unordered Pair value
	// object so (a,b) and (b,a) collapse to one entry.
	CoPurchasePairs(ctx context.Context, since time.Time) (map[domain.Pair]int, error)
}

// Storage is the persistence port for the materialised top-N read
// model. The adapter package supplies an in-memory implementation
// (tests) and a Postgres-backed one (production). UpsertTopN is the
// hot write path the refresher calls once per seed; TopN and
// LastComputedAt are read by the storefront / admin dashboard.
type Storage interface {
	// UpsertTopN atomically replaces the seed's existing top-N
	// list with the supplied slice. "Atomic" here means a reader
	// either sees the old list in full or the new list in full —
	// never a half-built mix. The postgres adapter wraps the
	// DELETE+INSERTs in one transaction; the in-memory adapter
	// swaps a slice under a mutex.
	UpsertTopN(ctx context.Context, productID string, recs []domain.Recommendation) error
	// TopN returns up to `limit` recommendations for the seed,
	// ordered by position ascending. An empty result means no
	// refresh has run yet (cold start) — the application service
	// falls back to productcatalog's "popular in category" path.
	TopN(ctx context.Context, productID string, limit int) ([]domain.Recommendation, error)
	// LastComputedAt is the timestamp of the most recent
	// UpsertTopN for this seed. ok=false means no row exists yet;
	// callers should treat that as "never refreshed".
	LastComputedAt(ctx context.Context, productID string) (time.Time, bool, error)
	// OverallLastRefresh is the latest computed_at across every
	// seed, used by the admin dashboard's "last refresh" line. An
	// empty table yields ok=false.
	OverallLastRefresh(ctx context.Context) (time.Time, bool, error)
}

// WeightsStorage is the persistence port for the admin-tunable
// scoring weights. There is exactly ONE row at id=1 in the
// recommendation_weights table; Get returns the row's contents (or
// domain.DefaultWeights() if the row is somehow missing — the
// migration seeds it, so this is purely defensive), Set persists a
// new value with an updated_at bump.
type WeightsStorage interface {
	// Get returns the persisted weights. The singleton row is
	// seeded by migration 000041 so this should always succeed; if
	// the row is somehow missing the implementation falls back to
	// domain.DefaultWeights() rather than erroring — the demo
	// prefers "the refresher keeps running with stock weights"
	// over "the refresher won't start".
	Get(ctx context.Context) (domain.Weights, error)
	// Set persists the new weights and updates the row's
	// updated_at timestamp. Validation (non-negative) is the
	// caller's responsibility — the storage layer accepts whatever
	// the application service approved.
	Set(ctx context.Context, w domain.Weights) error
}
