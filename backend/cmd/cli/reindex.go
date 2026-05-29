package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	pcdomain "github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	"github.com/bkielbasa/go-ecommerce/backend/search"
	searchapp "github.com/bkielbasa/go-ecommerce/backend/search/app"
	searchdomain "github.com/bkielbasa/go-ecommerce/backend/search/domain"
	"github.com/spf13/cobra"
)

// newReindexCmd wires the search bounded context and the product catalogue
// directly (rather than reusing the cli's main ProductService — which has
// the no-op indexer) and rebuilds the product slice of the search index
// from scratch. Run after the `seeds` subcommand or whenever the catalogue
// drifts away from the index (e.g. after a production migration).
func newReindexCmd(db *sql.DB) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "Rebuild the search index from the live catalogue",
		Long: `Drops every product-kind document from the search index and
re-walks the product catalogue, publishing each product as a fresh
search Document. Safe to run repeatedly: it is a wipe-then-rebuild,
not an incremental sync.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			_, searchSrv := search.New(db)
			// Build a productcatalog service WITHOUT a search indexer
			// (we drive indexing manually below), so reading the
			// catalogue does not trigger a recursive Index call.
			_, catalogSrv := productcatalog.New(db, pcapp.NoopSearchIndexer)

			n, err := reindexProducts(ctx, searchSrv, catalogSrv)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "reindexed %d products\n", n); err != nil {
				return fmt.Errorf("write reindex summary: %w", err)
			}
			return nil
		},
	}
	return cmd
}

// reindexProducts wipes the product slice of the search index and
// republishes every product. The returned count is the number of
// documents indexed; a failure during enumeration aborts the rebuild
// (the index is already empty for the kind, so re-running the command
// picks up where it left off).
func reindexProducts(ctx context.Context, searchSrv *searchapp.Service, catalogSrv pcapp.ProductService) (int, error) {
	if err := searchSrv.RemoveAllOfKind(ctx, searchdomain.KindProduct); err != nil {
		return 0, fmt.Errorf("remove product documents: %w", err)
	}

	products, err := catalogSrv.AllProducts(ctx)
	if err != nil {
		return 0, fmt.Errorf("load products: %w", err)
	}

	now := time.Now().UTC()
	n := 0
	for _, p := range products {
		doc, err := buildProductDocument(p, now)
		if err != nil {
			return n, fmt.Errorf("translate product %s: %w", p.ID(), err)
		}
		if err := searchSrv.Index(ctx, doc); err != nil {
			return n, fmt.Errorf("index product %s: %w", p.ID(), err)
		}
		n++
	}
	return n, nil
}

// buildProductDocument duplicates the productcatalog ACL's translation
// (productToDocument) because that helper is unexported. Keeping a single
// source of truth would mean exporting it from productcatalog/app; both
// translators are short and easy to keep in step here, and a follow-up
// commit can hoist the helper if a third caller appears.
func buildProductDocument(p pcdomain.Product, updatedAt time.Time) (searchdomain.Document, error) {
	tags := make([]string, 0, len(p.Categories()))
	for _, c := range p.Categories() {
		tags = append(tags, c.Name())
	}
	priceFrom := p.PriceFrom()
	meta := map[string]string{
		"currency":    priceFrom.Currency().String(),
		"price_minor": fmt.Sprintf("%d", priceFrom.Amount()),
	}
	if t := p.Thumbnail(); t != "" {
		meta["thumbnail"] = t
	}
	url := fmt.Sprintf("/product/%s", p.ID())
	return searchdomain.NewDocument(
		searchdomain.KindProduct,
		string(p.ID()),
		p.Name(),
		p.Description(),
		url,
		tags,
		meta,
		updatedAt,
	)
}
