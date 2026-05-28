package app

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	searchdomain "github.com/bkielbasa/go-ecommerce/backend/search/domain"
)

// SearchIndexer is the Anti-Corruption Layer port productcatalog calls
// after every mutation. It mirrors the shape of search/app.Indexer
// (Index/Remove) but is declared HERE — in the consumer of the search
// OHS — so the productcatalog package does not have to import
// search/app. The only search-package symbol leaking in is
// search/domain.Document, which IS the published language; importing
// that is fine (and unavoidable).
//
// Production wires *search/app.Service into this slot. Tests and the
// CLI write paths (where indexing is irrelevant) pass NoopSearchIndexer.
type SearchIndexer interface {
	Index(ctx context.Context, doc searchdomain.Document) error
	Remove(ctx context.Context, kind searchdomain.Kind, id string) error
}

// noopSearchIndexer satisfies SearchIndexer without touching any storage.
// It exists so tests and CLI paths that do not need a real index can wire
// productcatalog without spinning up the search bounded context.
type noopSearchIndexer struct{}

func (noopSearchIndexer) Index(_ context.Context, _ searchdomain.Document) error {
	return nil
}

func (noopSearchIndexer) Remove(_ context.Context, _ searchdomain.Kind, _ string) error {
	return nil
}

// NoopSearchIndexer is the do-nothing SearchIndexer wired by callers that
// do not need indexing (tests, the `cli` binary's catalogue write paths).
var NoopSearchIndexer SearchIndexer = noopSearchIndexer{}

// productToDocument is the translation step of the ACL: it maps a
// productcatalog Product into the search OHS's published language
// (search/domain.Document). The product's display URL ("/product/<id>")
// is constructed here, which is where the storefront also links from
// catalog cards, so search hits and grid links agree.
//
// The meta map carries display-time metadata (currency, price minor
// units, thumbnail) so future consumers (e.g. a unified search results
// page) can render cards without re-fetching the product.
func productToDocument(p domain.Product, updatedAt time.Time) (searchdomain.Document, error) {
	tags := make([]string, 0, len(p.Categories()))
	for _, c := range p.Categories() {
		tags = append(tags, c.Name())
	}
	priceFrom := p.PriceFrom()
	meta := map[string]string{
		"currency":    priceFrom.Currency().String(),
		"price_minor": strconv.FormatInt(priceFrom.Amount(), 10),
	}
	if t := p.Thumbnail(); t != "" {
		meta["thumbnail"] = t
	}
	productURL := fmt.Sprintf("/product/%s", url.PathEscape(string(p.ID())))
	return searchdomain.NewDocument(
		searchdomain.KindProduct,
		string(p.ID()),
		p.Name(),
		p.Description(),
		productURL,
		tags,
		meta,
		updatedAt,
	)
}
