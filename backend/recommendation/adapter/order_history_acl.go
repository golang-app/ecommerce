package adapter

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/recommendation/domain"
)

// OrderHistoryACL implements app.OrderHistoryReader by reading
// directly from the checkout context's read-side projection
// (checkout_order + checkout_order_item) and resolving variant ids
// up to product ids via productcatalog_variant.
//
// One SQL query produces every (productA, productB, count) triple
// in the lookback window. The self-join uses oi1.product_id <
// oi2.product_id (the variant ids) to deduplicate (A,B) vs (B,A)
// before the GROUP BY, then a HAVING clause filters out self-pairs
// at the resolved-product level (two different variants of the
// same product still co-purchase, but the recommendation context
// treats that as the trivial pair and drops it).
//
// IMPORTANT — variant vs product. The checkout context's
// checkout_order_item.product_id is actually the catalogue's
// variant id (the cart adds variants; see HasPurchasedProduct in
// checkout/adapter/postgres.go for the same nuance). To answer
// "did these two PRODUCTS co-purchase?" the query joins each line
// through productcatalog_variant to recover the parent product id.
type OrderHistoryACL struct {
	db *sql.DB
}

// NewOrderHistoryACL wires the adapter against the shared DB.
func NewOrderHistoryACL(db *sql.DB) *OrderHistoryACL {
	return &OrderHistoryACL{db: db}
}

// CoPurchasePairs returns every product-id pair that co-occurred
// in a paid (or further along) order placed at or after `since`,
// with the number of orders in which the pair appeared. The map
// key is the unordered domain.Pair (constructed from sorted ids)
// so callers don't have to remember which side is A and which is B.
func (a *OrderHistoryACL) CoPurchasePairs(ctx context.Context, since time.Time) (map[domain.Pair]int, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT v1.product_id, v2.product_id, COUNT(DISTINCT o.id)
		FROM checkout_order o
		JOIN checkout_order_item oi1 ON oi1.order_id = o.id
		JOIN checkout_order_item oi2 ON oi2.order_id = o.id AND oi2.id <> oi1.id
		JOIN productcatalog_variant v1 ON v1.id = oi1.product_id
		JOIN productcatalog_variant v2 ON v2.id = oi2.product_id
		WHERE o.status IN ('paid', 'shipped', 'delivered')
		  AND o.placed_at >= $1
		  AND v1.product_id < v2.product_id
		GROUP BY v1.product_id, v2.product_id
	`, since)
	if err != nil {
		return nil, fmt.Errorf("co-purchase pairs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[domain.Pair]int{}
	for rows.Next() {
		var a, b string
		var count int
		if err := rows.Scan(&a, &b, &count); err != nil {
			return nil, fmt.Errorf("scan pair: %w", err)
		}
		if a == b {
			// Defensive: the WHERE v1.product_id < v2.product_id
			// guard already rules this out, but a future schema
			// change might break the assumption. Drop it explicitly.
			continue
		}
		pair := domain.NewPair(a, b)
		out[pair] += count
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}
