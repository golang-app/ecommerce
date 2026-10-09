package layout

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/bkielbasa/go-ecommerce/backend/cart/domain"
	"github.com/bkielbasa/go-ecommerce/backend/internal/https"
	"github.com/bkielbasa/go-ecommerce/backend/internal/observability"
	pcapp "github.com/bkielbasa/go-ecommerce/backend/productcatalog/app"
	pcdomain "github.com/bkielbasa/go-ecommerce/backend/productcatalog/domain"
	searchapp "github.com/bkielbasa/go-ecommerce/backend/search/app"
	searchdomain "github.com/bkielbasa/go-ecommerce/backend/search/domain"
	"github.com/gorilla/mux"
)

// AllProducts renders the filterable product grid fragment (no base layout).
// The query string drives the filter: `category` selects the scope, while each
// filterable attribute type contributes either `<typeID>_min`/`<typeID>_max`
// (numeric) or a repeated `<typeID>` checkbox group (enum).
func (handler httpHandler) AllProducts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	category := q.Get("category")
	search := strings.TrimSpace(q.Get("q"))

	var products []pcdomain.Product
	var err error

	if search != "" {
		// Search-driven path: route the query through the search OHS and
		// hydrate the returned hits via the catalogue. Facet/category
		// rail values are intentionally ignored here — the rail is a
		// best-effort secondary filter while search is the primary
		// intent. Failures degrade silently to an empty result rather
		// than 500ing.
		products = handler.searchProducts(ctx, search)
	} else {
		query := pcapp.ProductQuery{
			CategorySlug:   category,
			NumericRanges:  map[string]pcapp.Range{},
			EnumSelections: map[string][]string{},
		}

		// Use the facets for this scope to know which params are numeric vs enum.
		facets, fErr := handler.catalogSrv.Facets(ctx, category)
		if fErr != nil {
			handler.logger.WithError(fErr).WithField("category", category).Warn("cannot get facets")
			facets = nil
		}

		for _, f := range facets {
			typeID := f.Type.ID()
			switch {
			case f.Type.IsNumeric():
				minStr := q.Get(typeID + "_min")
				maxStr := q.Get(typeID + "_max")
				if minStr == "" && maxStr == "" {
					continue
				}
				rng := pcapp.Range{}
				if minStr != "" {
					if v, errParse := strconv.ParseFloat(minStr, 64); errParse == nil {
						rng.Min = &v
					}
				}
				if maxStr != "" {
					if v, errParse := strconv.ParseFloat(maxStr, 64); errParse == nil {
						rng.Max = &v
					}
				}
				if rng.Min == nil && rng.Max == nil {
					continue
				}
				query.NumericRanges[typeID] = rng
			case f.Type.IsEnum():
				if vals := q[typeID]; len(vals) > 0 {
					query.EnumSelections[typeID] = vals
				}
			}
		}

		products, err = handler.catalogSrv.List(ctx, query)
		if err != nil {
			https.InternalError(w, "internal-error", "cannot get list of all products")
			handler.logger.WithError(err).Error("cannot get list of all products")
			return
		}
	}

	resp := map[string]any{
		"Products": products,
		"Search":   search,
	}

	files := []string{
		"./layout/tmpl/productCatalog/allProducts.gohtml",
	}

	// The grid fragment uses {{ money .PriceFrom.Amount }} to render the
	// price in the customer's selected display currency. We bind the
	// same FuncMap renderTemplate installs so the HTMX-only grid keeps
	// parity with the full-page render path.
	var ts = template.Must(template.New("").Funcs(template.FuncMap{
		"html": func(value interface{}) template.HTML {
			return template.HTML(fmt.Sprint(value))
		},
		"add": func(a, b string) float64 {
			return 666
		},
		"money": moneyFunc(handler.rates, handler.currentCurrency(r)),
	}).ParseFiles(files...))
	err = ts.ExecuteTemplate(w, "allProducts.gohtml", resp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// searchProducts queries the search OHS for product hits and hydrates each
// hit through catalogSrv.Find so the grid template gets full Product
// values (with variants, price, thumbnail). All failures degrade silently
// to an empty result — the storefront prefers an empty grid over a 500
// when the search index is misbehaving, since the catalogue itself is
// still browseable via the category rail.
func (handler httpHandler) searchProducts(ctx context.Context, q string) []pcdomain.Product {
	// Record the user-visible search request — moved out of
	// ProductService.List when search migrated to the OHS.
	observability.SearchQueriesInc(ctx)
	if handler.searchSrv == nil {
		return nil
	}
	hits, err := handler.searchSrv.Search(ctx, q, searchapp.QueryOptions{
		Kinds: []searchdomain.Kind{searchdomain.KindProduct},
	})
	if err != nil {
		handler.logger.WithError(err).WithField("query", q).Warn("search OHS query failed; returning empty grid")
		return nil
	}
	out := make([]pcdomain.Product, 0, len(hits))
	for _, h := range hits {
		p, err := handler.catalogSrv.Find(ctx, h.Document.ID())
		if err != nil {
			// A hit that no longer resolves to a product almost always
			// means the catalogue and the search index drifted (admin
			// deleted the product before the index was updated). Skip
			// it; `cli reindex` will tidy up later.
			handler.logger.WithError(err).WithField("product_id", h.Document.ID()).Warn("search hit does not resolve to a product; skipping")
			continue
		}
		out = append(out, p)
	}
	return out
}

func (handler httpHandler) Product(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["productID"]
	product, err := handler.catalogSrv.Find(r.Context(), id)

	session, _ := store.Get(r, "ecommerce")

	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			session.AddFlash("Product does not exists", "error")
			_ = session.Save(r, w)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		session.AddFlash("cannot get list of all products", "error")
		_ = session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Resolve the initially-selected variant: the combination formed by the
	// first value of each option type (which is what the selects default to).
	selected := map[string]string{}
	for _, ot := range product.OptionTypes() {
		if len(ot.Values()) > 0 {
			selected[ot.Name()] = ot.Values()[0]
		}
	}
	variant := product.DefaultVariant()
	if product.HasOptions() {
		if v, ok := product.ResolveVariant(selected); ok {
			variant = v
		}
	}

	// Reviews block: aggregate badge, list of recent reviews, and the
	// CanReview / AlreadyReviewed flags that decide whether to render the
	// submission form. All three calls degrade gracefully — a failure in
	// the reviews service must never block the product page from loading.
	productID := string(product.ID())
	reviews, err := handler.reviewsSrv.ListForProduct(r.Context(), productID, 20)
	if err != nil {
		handler.logger.WithError(err).Warn("cannot list reviews for product")
		reviews = nil
	}
	aggMap, err := handler.reviewsSrv.AggregateForProducts(r.Context(), []string{productID})
	if err != nil {
		handler.logger.WithError(err).Warn("cannot aggregate reviews for product")
		aggMap = nil
	}
	aggregate := aggMap[productID]

	customerID := handler.currentCustomerID(r)
	canReview := false
	alreadyReviewed := false
	if customerID != "" {
		// CanReview is "the buyer has purchased this product"; we ask the
		// checkout query directly so the rendered hint matches the same
		// check Submit enforces.
		bought, qErr := handler.checkoutQry.HasPurchasedProduct(r.Context(), customerID, productID)
		if qErr != nil {
			handler.logger.WithError(qErr).Warn("cannot verify purchase for review form")
		}
		canReview = bought
		if canReview {
			done, hErr := handler.reviewsSrv.HasReviewed(r.Context(), productID, customerID)
			if hErr != nil {
				handler.logger.WithError(hErr).Warn("cannot check existing review")
			}
			alreadyReviewed = done
		}
	}

	// Wishlist state for the currently-resolved variant drives the
	// heart-button render. Anonymous visitors see the "log in to save"
	// hint regardless of state, so we skip the lookup for them.
	inWishlist := false
	if customerID != "" && !variant.IsZero() {
		saved, wErr := handler.wishlistSrv.Contains(r.Context(), customerID, variant.ID())
		if wErr != nil {
			handler.logger.WithError(wErr).Warn("cannot check wishlist state")
		}
		inWishlist = saved
	}

	handler.renderTemplate(w, r, "productCatalog/show", map[string]any{
		"Product":         product,
		"Variant":         variant,
		"Reviews":         reviews,
		"Aggregate":       aggregate,
		"CanReview":       canReview,
		"AlreadyReviewed": alreadyReviewed,
		"InWishlist":      inWishlist,
	})
}

// ProductVariant resolves the option selection (query params) to a variant
// and returns the variant box partial (price + add-to-cart). Driven by the
// option selects via HTMX.
func (handler httpHandler) ProductVariant(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["productID"]
	product, err := handler.catalogSrv.Find(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	selected := map[string]string{}
	for _, ot := range product.OptionTypes() {
		selected[ot.Name()] = r.URL.Query().Get(ot.Name())
	}
	variant, _ := product.ResolveVariant(selected)

	// Re-check wishlist state for the newly-resolved variant so the heart
	// button inside the variant-box partial reflects the per-variant
	// bookmark. Anonymous browsers skip the lookup — they render the
	// "log in to save" hint regardless.
	customerID := handler.currentCustomerID(r)
	loggedIn := customerID != ""
	inWishlist := false
	if loggedIn && !variant.IsZero() {
		saved, wErr := handler.wishlistSrv.Contains(r.Context(), customerID, variant.ID())
		if wErr != nil {
			handler.logger.WithError(wErr).Warn("cannot check wishlist state")
		}
		inWishlist = saved
	}

	// The variant-response fragment embeds the variant-box partial, which
	// renders the price via {{ money .Variant.Price.Amount }}. We have to
	// install the same currency-aware helper renderTemplate uses so the
	// HTMX swap doesn't fail with an "undefined function: money" parse
	// error at execute time.
	ts := template.Must(template.New("").Funcs(template.FuncMap{
		"dict":  templateDict,
		"money": moneyFunc(handler.rates, handler.currentCurrency(r)),
	}).ParseGlob("./layout/tmpl/partials/*.gohtml"))
	if err := ts.ExecuteTemplate(w, "variant-response", map[string]any{
		"Variant":     variant,
		"ProductName": product.Name(),
		"LoggedIn":    loggedIn,
		"InWishlist":  inWishlist,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
