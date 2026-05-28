package layout

import (
	"net/http"
	"strings"
)

// SearchPage renders the storefront search results page. It reuses the catalog
// template (filter rail + lazy grid) with no active category; the actual search
// term is forwarded to the lazy /api/v1/products fetch via the ActiveCategory/
// Search data the catalog template encodes into its hx-get URL.
func (handler httpHandler) SearchPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	categories, err := handler.catalogSrv.Categories(ctx)
	if err != nil {
		handler.logger.WithError(err).Warn("cannot get categories")
		categories = nil
	}

	// Search shows the cross-catalog facets (no category scope). The user can
	// still narrow with the rail; the hidden q input keeps the search active.
	facets, err := handler.catalogSrv.Facets(ctx, "")
	if err != nil {
		handler.logger.WithError(err).Warn("cannot get facets")
		facets = nil
	}

	handler.renderTemplate(w, r, "productCatalog/catalog", map[string]any{
		"Categories":     categories,
		"Facets":         facets,
		"ActiveCategory": "",
		"Search":         q,
		"SearchQuery":    q,
	})
}
