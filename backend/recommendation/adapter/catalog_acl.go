package adapter

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/bkielbasa/go-ecommerce/backend/recommendation/domain"
)

// CatalogACL implements app.CatalogReader by reading directly from
// the productcatalog tables (productcatalog_product,
// productcatalog_product_category, productcatalog_product_attribute,
// productcatalog_attribute_type). It is deliberately a SQL adapter
// rather than a Go-level wrapper around productcatalog/app.ProductService
// because:
//
//   - The recommendation context needs ProductSummary, not
//     productcatalog's full domain.Product (variants, option types,
//     attribute sets). Pulling the full aggregate just to throw 90%
//     of it away on every refresh would balloon the demo's
//     refresher latency for no benefit.
//   - SimilarByText needs a Postgres FTS query against the
//     catalogue table. Adding a new method to productcatalog/app
//     would expose an FTS detail through the catalogue's public
//     surface; keeping the query here keeps productcatalog's API
//     unchanged and contains the FTS concern inside the consumer.
//
// The composition root wires this adapter into recommendation.New;
// it is the only place the recommendation context touches the
// productcatalog tables directly. If a future change splits the
// catalogue into its own database, this adapter is the one file
// that needs replacing.
type CatalogACL struct {
	db *sql.DB
}

// NewCatalogACL wires the adapter against the shared DB.
func NewCatalogACL(db *sql.DB) *CatalogACL {
	return &CatalogACL{db: db}
}

// loadAttributes returns the product's (key=value) attribute pairs
// as a flat map, ready for the scorer's Jaccard. Numeric attributes
// are stringified; missing values are skipped.
func (a *CatalogACL) loadAttributes(ctx context.Context, productID string) (map[string]string, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT t.name, COALESCE(pa.text_value, ''), pa.num_value
		FROM productcatalog_product_attribute pa
		JOIN productcatalog_attribute_type t ON t.id = pa.attribute_type_id
		WHERE pa.product_id = $1
	`, productID)
	if err != nil {
		return nil, fmt.Errorf("load attributes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]string{}
	for rows.Next() {
		var name, textVal string
		var numVal sql.NullFloat64
		if err := rows.Scan(&name, &textVal, &numVal); err != nil {
			return nil, fmt.Errorf("scan attribute: %w", err)
		}
		switch {
		case textVal != "":
			out[name] = textVal
		case numVal.Valid:
			out[name] = fmt.Sprintf("%g", numVal.Float64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// firstCategory returns the first category the product belongs to
// (alphabetical order so the choice is deterministic across calls).
// An empty string means "no category", which the scorer treats as
// "never matches".
func (a *CatalogACL) firstCategory(ctx context.Context, productID string) (string, error) {
	var category sql.NullString
	err := a.db.QueryRowContext(ctx, `
		SELECT category_id
		FROM productcatalog_product_category
		WHERE product_id = $1
		ORDER BY category_id
		LIMIT 1
	`, productID).Scan(&category)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load category: %w", err)
	}
	return category.String, nil
}

// hydrate builds a ProductSummary for the given product id. Missing
// products return ok=false (no error) so the refresher can skip
// stale ids gracefully.
func (a *CatalogACL) hydrate(ctx context.Context, productID string) (domain.ProductSummary, bool, error) {
	var (
		id, name, desc string
		price          int64
	)
	err := a.db.QueryRowContext(ctx, `
		SELECT id, name, description, price_amount
		FROM productcatalog_product
		WHERE id = $1
	`, productID).Scan(&id, &name, &desc, &price)
	if err == sql.ErrNoRows {
		return domain.ProductSummary{}, false, nil
	}
	if err != nil {
		return domain.ProductSummary{}, false, fmt.Errorf("load product: %w", err)
	}
	categoryID, err := a.firstCategory(ctx, id)
	if err != nil {
		return domain.ProductSummary{}, false, err
	}
	attrs, err := a.loadAttributes(ctx, id)
	if err != nil {
		return domain.ProductSummary{}, false, err
	}
	return domain.ProductSummary{
		ID:          id,
		CategoryID:  categoryID,
		Name:        name,
		Description: desc,
		PriceMinor:  price,
		Attributes:  attrs,
	}, true, nil
}

// ProductByID implements app.CatalogReader.
func (a *CatalogACL) ProductByID(ctx context.Context, id string) (domain.ProductSummary, bool, error) {
	return a.hydrate(ctx, id)
}

// AllProductIDs implements app.CatalogReader.
func (a *CatalogACL) AllProductIDs(ctx context.Context) ([]string, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id FROM productcatalog_product ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list product ids: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// ProductsInCategory implements app.CatalogReader. The query joins
// productcatalog_product_category to scope results to the given
// category id, ordering by id so the output is deterministic across
// calls (the demo's seed is small enough that "any K" works fine
// without an explicit ranking signal).
func (a *CatalogACL) ProductsInCategory(ctx context.Context, categoryID string, limit int) ([]domain.ProductSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := a.db.QueryContext(ctx, `
		SELECT p.id
		FROM productcatalog_product p
		JOIN productcatalog_product_category pc ON pc.product_id = p.id
		WHERE pc.category_id = $1
		ORDER BY p.id
		LIMIT $2
	`, categoryID, limit)
	if err != nil {
		return nil, fmt.Errorf("list category products: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return a.hydrateMany(ctx, ids)
}

// SimilarByText implements app.CatalogReader by running a
// Postgres FTS query against the catalogue table. The query text is
// the seed's name + description; the search config is `simple`
// (lowercase + tokenise, no stemming) to match the existing search
// bounded context's choice.
//
// The query uses a CTE to build the seed's tsvector once, then
// ranks every OTHER product against it. ts_rank is used (not
// ts_rank_cd) because the catalogue's "text" is a single
// title+description blob with no positional structure worth weighting.
func (a *CatalogACL) SimilarByText(ctx context.Context, productID string, limit int) ([]domain.ProductSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := a.db.QueryContext(ctx, `
		WITH seed AS (
		    SELECT name || ' ' || description AS body
		    FROM productcatalog_product
		    WHERE id = $1
		)
		SELECT p.id
		FROM productcatalog_product p, seed,
		     plainto_tsquery('simple', seed.body) q
		WHERE p.id <> $1
		  AND to_tsvector('simple', p.name || ' ' || p.description) @@ q
		ORDER BY ts_rank(to_tsvector('simple', p.name || ' ' || p.description), q) DESC
		LIMIT $2
	`, productID, limit)
	if err != nil {
		return nil, fmt.Errorf("text similar: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return a.hydrateMany(ctx, ids)
}

// hydrateMany loops over ids and produces a slice of summaries.
// Missing products are silently skipped — the upstream query may
// have raced with a delete, and the caller is fine with a shorter
// slice in that edge case.
func (a *CatalogACL) hydrateMany(ctx context.Context, ids []string) ([]domain.ProductSummary, error) {
	out := make([]domain.ProductSummary, 0, len(ids))
	for _, id := range ids {
		p, ok, err := a.hydrate(ctx, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}
